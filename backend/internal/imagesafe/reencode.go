package imagesafe

import (
	"bytes"
	"image"
	"image/jpeg" // also registers the JPEG decoder reencodeStill uses
	"image/png"  // also registers the PNG decoder reencodeStill uses

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers the still-WebP decoder reencodeStill uses; animated WebP has no Go decoder at all
)

// jpegQuality is the quality Reencode writes an opaque photo at: high
// enough that recompression is not visibly lossy, far smaller than
// the PNG this package would otherwise have to write for every photo.
const jpegQuality = 90

// jpegMaxScans bounds how many start-of-scan markers a JPEG may
// contain before reencodeStill refuses to decode it at all. Go's own
// image/jpeg decoder re-walks every block of the image for each
// progressive scan and has no limit of its own, so a tiny scan
// repeated thousands of times inside a file far under profile's
// MaxInputLen costs real CPU time for bytes that cost nothing to
// send - and because store/thumbsafe re-encodes a stored thumbnail
// on every read, a hostile one would pay that cost again on every
// conversation load. 100 matches libjpeg-turbo's own -maxscans
// default, which guards the same shape (see docs/decisions.md for
// the measured cost this closes).
const jpegMaxScans = 100

// reencodeStill is Reencode's own implementation (see Reencode): it
// checks data's declared dimensions against profile from a cheap
// header read alone,
// before ever decoding a single pixel, then decodes fully and writes
// a fresh encode of the result. ok is false when data's header
// already fails profile, when a JPEG declares more than jpegMaxScans
// start-of-scan markers, when profile sets MaxDecodeBytes and the
// header alone already estimates a decode over it (see
// estimatedDecodeBytes), when it fails to decode at all (an animated
// WebP, truncated input, or anything a decoder's own panic turns into
// a plain refusal - see decodeImage), or when the pixels actually
// decoded still turn out to be over profile's caps: a confirmation
// exactly like finishPNG and finishJPEG's old ones, never trusting a
// header's own claim once real pixels exist to check it against. An
// image that passes every cap is shrunk to profile.DownscaleTo on its
// longer side, never enlarged, before being encoded (see
// downscaleToFit); width and height are always this function's own
// final result, after any such resize, never the original decoded
// size.
func reencodeStill(data []byte, profile Profile) (out []byte, format OutputFormat, width, height int, ok bool) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || !withinCaps(int64(cfg.Width), int64(cfg.Height), profile) {
		return nil, FormatUnknown, 0, 0, false
	}
	if isJPEG(data) && countJPEGScans(data) > jpegMaxScans {
		return nil, FormatUnknown, 0, 0, false
	}
	if profile.MaxDecodeBytes > 0 && estimatedDecodeBytes(data, int64(cfg.Width), int64(cfg.Height)) > profile.MaxDecodeBytes {
		return nil, FormatUnknown, 0, 0, false
	}

	img, ok := decodeImage(data)
	if !ok {
		return nil, FormatUnknown, 0, 0, false
	}

	bounds := img.Bounds()
	width, height = bounds.Dx(), bounds.Dy()
	if !withinCaps(int64(width), int64(height), profile) {
		return nil, FormatUnknown, 0, 0, false
	}

	// hasAlpha is checked before any resize, while img still carries its
	// own decoded type (*image.YCbCr for an opaque JPEG, say): that
	// lets it skip straight past the pixel scan for every format that
	// can never carry alpha (see hasAlpha). downscaleToFit's own
	// result is always a generic *image.RGBA, which would otherwise
	// force that scan - over the resized pixels, not even the real
	// photo's own - on every outgoing photo, never free for one as
	// large as Outgoing now accepts.
	alpha := hasAlpha(img)
	img, width, height = downscaleToFit(img, width, height, profile.DownscaleTo)

	return encodeImage(img, alpha, width, height)
}

// downscaleToFit returns img resized so its longer side is maxSide,
// its aspect ratio kept exactly (both sides scaled by the same
// factor), together with its own new width and height - or img,
// width and height unchanged when maxSide is zero (profiles that
// refuse an over-cap image outright rather than resizing it) or img
// already fits within it: never upscaled, only ever shrunk. Uses
// golang.org/x/image/draw's ApproxBiLinear, chosen over BiLinear or
// CatmullRom after measuring all three against a real 50-megapixel
// photo - about 8 times faster than either, for quality this
// package's own doc calls merely "reasonable" rather than "high" or
// "very high" (see docs/decisions.md for the measured times): this
// runs inside app.decodeSlots' own single slot, which every other
// photo decode - including an incoming one the user is simply
// scrolling past - waits behind, so the extra second or more either
// kernel interpolator costs is a real, felt delay for unrelated work,
// not merely a slower encode of this one photo.
func downscaleToFit(img image.Image, width, height, maxSide int) (image.Image, int, int) {
	longer := max(width, height)
	if maxSide <= 0 || longer <= maxSide {
		return img, width, height
	}

	scale := float64(maxSide) / float64(longer)
	dw, dh := scaledSize(width, scale), scaledSize(height, scale)

	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)

	return dst, dw, dh
}

// scaledSize applies scale to n, never below one pixel: an extreme
// aspect ratio could otherwise round a side down to zero.
func scaledSize(n int, scale float64) int {
	if s := int(float64(n) * scale); s > 0 {
		return s
	}

	return 1
}

// countJPEGScans counts every occurrence of JPEG's start-of-scan
// marker (0xFF 0xDA) anywhere in data, including one that happens to
// fall inside entropy-coded data rather than being a real marker: a
// plain linear byte count, not a real parser that tracks markers,
// byte stuffing or restart intervals. That is deliberate - an
// over-count only ever rejects more input against jpegMaxScans,
// never less, which is all this needs to guarantee.
func countJPEGScans(data []byte) int {
	count := 0
	for i := 0; i+1 < len(data); i++ {
		if data[i] == 0xFF && data[i+1] == 0xDA {
			count++
		}
	}

	return count
}

// decodeImage decodes data with this package's own registered
// decoders (PNG, JPEG and still WebP - Reencode already refused a GIF
// outright before ever calling this, see the package doc comment),
// recovering from any panic a decoder raises on a crafted input it
// does not itself defend against: a decoder panicking here must
// refuse the image, the same as any other decode failure, never
// crash the helper (see errors.md).
func decodeImage(data []byte) (img image.Image, ok bool) {
	defer func() {
		if recover() != nil { // coverage-ignore: none of this package's four registered decoders is known to panic on crafted input; this is a backstop, not a path a test can currently reach
			img, ok = nil, false
		}
	}()

	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}

	return decoded, true
}

// encodeImage writes img - already confirmed within profile's caps -
// as PNG at png.BestSpeed when alpha is true (see hasAlpha, called by
// reencodeStill before any resize, never here), or JPEG at
// jpegQuality otherwise: the one choice Reencode makes about its own
// output format, independent of whatever format data arrived as. ok
// is false only if the standard library's own encoder itself fails,
// which neither ever does for a decoded image.Image.
func encodeImage(img image.Image, alpha bool, width, height int) (out []byte, format OutputFormat, w, h int, ok bool) {
	var buf bytes.Buffer

	if alpha {
		if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img); err != nil { // coverage-ignore: the standard library's own PNG encoder has no failure mode for a decoded image.Image within this package's own caps
			return nil, FormatUnknown, 0, 0, false
		}

		return buf.Bytes(), FormatPNG, width, height, true
	}

	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil { // coverage-ignore: same as the PNG encoder above; jpeg.Encode's own documented failure (an unsupported Rectangle max of > 1<<32) is far past every profile's own MaxSide
		return nil, FormatUnknown, 0, 0, false
	}

	return buf.Bytes(), FormatJPEG, width, height, true
}

// hasAlpha reports whether img carries any pixel that is not fully
// opaque: a transparent background or a sticker's own cut-out shape,
// the one difference that decides whether encodeImage writes PNG or
// JPEG. A decoded image whose concrete type can never represent
// alpha at all - Gray, Gray16, CMYK or YCbCr, what image/jpeg and a
// grayscale image/png ever decode to - skips the scan below
// entirely, since every real photo is exactly one of these and
// scanning it would only cost time to reach the same answer the type
// alone already gives. Every other type, including a paletted GIF or
// PNG frame, is scanned: a palette can carry a transparent entry
// unused by a single pixel, and the scan is the only way to tell
// "has an alpha channel" apart from "is actually transparent
// anywhere", which is the question that matters here.
func hasAlpha(img image.Image) bool {
	switch img.(type) {
	case *image.Gray, *image.Gray16, *image.CMYK, *image.YCbCr:
		return false
	}

	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				return true
			}
		}
	}

	return false
}

// bytesPerPixelBaseline is what estimatedDecodeBytes charges every
// pixel of a baseline (non-progressive) JPEG, an 8-bit or paletted
// PNG, or a still WebP: none of these keep much more than their own
// final pixels live while decoding. Measured up to roughly 1.75 bytes
// per pixel for a baseline JPEG at FullImage's own cap (see
// docs/decisions.md); more than doubled here for headroom.
const bytesPerPixelBaseline int64 = 4

// bytesPerPixelExpensive is what estimatedDecodeBytes charges every
// pixel of a progressive JPEG, whatever its component count, or a
// 16-bit-per-channel PNG: a progressive decode keeps every block's
// coefficients live across every one of its scans, and a 16-bit
// image's own row buffers are twice as wide, so both cost far more
// per pixel than the baseline case above. Measured up to roughly 28
// bytes per pixel for the worst of either at FullImage's own cap (see
// docs/decisions.md); rounded up for headroom.
const bytesPerPixelExpensive int64 = 32

// estimatedDecodeBytes approximates, from data's own header alone, how
// many bytes decoding a width x height image like it will need at
// peak: cheap enough for reencodeStill to check before the real decode
// it exists to guard ever runs. A progressive JPEG or a 16-bit PNG is
// charged at bytesPerPixelExpensive; everything else at the far
// cheaper bytesPerPixelBaseline (see both for why). Deliberately not
// exact: calibrated with headroom against runtime.MemStats peaks
// actually measured for this package's own worst legitimate shapes at
// FullImage's new cap (see docs/decisions.md) - always on the side of
// too much, never too little.
func estimatedDecodeBytes(data []byte, width, height int64) int64 {
	pixels := width * height

	switch {
	case isJPEG(data) && jpegIsProgressive(data):
		return pixels * bytesPerPixelExpensive
	case isPNG(data) && pngBitDepth(data) > 8:
		return pixels * bytesPerPixelExpensive
	default:
		return pixels * bytesPerPixelBaseline
	}
}

// pngIHDRChunkTypeOffset is the fixed byte offset of a PNG's first
// chunk's own type: 8 (signature) + 4 (chunk length) bytes in, right
// after the length field every chunk starts with.
const pngIHDRChunkTypeOffset = 12

// pngIHDRBitDepthOffset is the fixed byte offset of a PNG's own
// declared bit depth, inside the IHDR chunk a well-formed PNG starts
// with: 8 (signature) + 4 (chunk length) + 4 (chunk type "IHDR") + 4
// (width) + 4 (height) bytes in. Cheap enough to read directly,
// without asking image/png to parse the whole header just for this
// one byte - but only once pngIHDRChunkTypeOffset confirms IHDR is
// actually there: image/png's own decoder orders only IHDR, PLTE,
// tRNS, IDAT and IEND against each other, silently ignoring any other
// chunk type wherever it appears, so one spliced in before IHDR shifts
// this fixed offset into that chunk's own bytes instead - exactly the
// gap a security review found could hide a 16-bit image's real depth
// behind whatever happened to sit there (see docs/decisions.md).
const pngIHDRBitDepthOffset = 24

// pngBitDepth reads data's own declared bit depth straight from its
// IHDR chunk, when IHDR is exactly where every real encoder puts it:
// the very first chunk, immediately after the signature. 16 - never
// 8, which would instead shrink estimatedDecodeBytes's own charge -
// whenever that is not confirmed: data is too short to hold a
// complete fixed header, or its first chunk's own type is not
// literally "IHDR", meaning something else precedes it. This never
// rejects a real image: reencodeStill's own earlier
// image.DecodeConfig call already refused anything too malformed to
// reach here, and no real encoder writes a chunk before IHDR; it only
// makes sure a shape this cheap read cannot confirm is never charged
// as if it were.
func pngBitDepth(data []byte) int {
	if len(data) <= pngIHDRBitDepthOffset {
		return 16
	}
	if string(data[pngIHDRChunkTypeOffset:pngIHDRChunkTypeOffset+4]) != "IHDR" {
		return 16
	}

	return int(data[pngIHDRBitDepthOffset])
}

// jpegIsProgressive reports whether data's first start-of-frame marker
// - walked past every other segment by its own declared length, so a
// payload byte that merely looks like a marker is never mistaken for
// one - is one of the four progressive DCT variants (0xC2, 0xC6, 0xCA
// or 0xCE) rather than baseline or extended-sequential (see
// estimatedDecodeBytes for why that distinction matters). A run of
// 0xFF fill bytes before a real marker, the same shape a pre-rewrite
// security review found bypassing this package's old hand-written
// scan count, is handled the same way countJPEGScans's own comment
// describes: skipped one byte at a time rather than assumed away.
//
// True - never false, which would instead shrink
// estimatedDecodeBytes's own charge - for anything this walk cannot
// confirm is cheap: an "FF 00" byte-stuffed pair, which only ever
// byte-stuffs a literal 0xFF inside entropy-coded scan data and so
// never legitimately appears before any SOS; a segment whose own
// declared length is too short to be real; or a file this walk's own
// bound runs out on before ever finding a start-of-frame marker at
// all, which reencodeStill's own earlier image.DecodeConfig call
// would already have refused for real malformed input, so this walk
// failing to reach the same marker means it has lost sync with the
// file, not that the file has none. A security review found the old
// code treated "FF 00" as a marker carrying a length field of its
// own, letting it skip clean over a real SOF2 placed just past one;
// none of these three cases can happen for a file real encoder
// writes, so this never costs a real photo anything (see
// docs/decisions.md).
func jpegIsProgressive(data []byte) bool {
	for i := 2; i+3 < len(data); {
		if data[i] != 0xFF {
			i++
			continue
		}

		marker := data[i+1]
		switch {
		case marker == 0xFF:
			i++
		case marker == 0x00:
			return true
		case jpegMarkerHasNoLength(marker):
			i += 2
		case jpegMarkerIsSOF(marker):
			return jpegMarkerIsProgressiveSOF(marker)
		default:
			length := int(data[i+2])<<8 | int(data[i+3])
			if length < 2 {
				return true
			}
			i += 2 + length
		}
	}

	return true
}

// jpegMarkerHasNoLength reports whether marker is TEM (0x01) or one of
// the eight restart markers (0xD0-0xD8): every JPEG marker but these
// two, SOI and EOI carries a two-byte length jpegIsProgressive must
// skip by, rather than simply stepping past the marker code itself.
func jpegMarkerHasNoLength(marker byte) bool {
	return marker == 0x01 || (marker >= 0xD0 && marker <= 0xD8)
}

// jpegMarkerIsSOF reports whether marker is one of the twelve real
// start-of-frame markers, 0xC0 through 0xCF excluding 0xC4 (DHT),
// 0xC8 (reserved "JPG") and 0xCC (DAC), which share that numeric range
// without being frame markers at all.
func jpegMarkerIsSOF(marker byte) bool {
	return marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC
}

// jpegMarkerIsProgressiveSOF reports whether marker - already
// confirmed a start-of-frame marker by jpegMarkerIsSOF - is one of the
// four progressive DCT variants (0xC2, 0xC6, 0xCA, 0xCE) rather than
// baseline or extended-sequential.
func jpegMarkerIsProgressiveSOF(marker byte) bool {
	return marker == 0xC2 || marker == 0xC6 || marker == 0xCA || marker == 0xCE
}
