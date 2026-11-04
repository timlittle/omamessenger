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
// contain before reencodeStill refuses to decode it: Go's decoder
// re-walks every block per progressive scan with no limit of its own,
// so a tiny repeated scan costs real CPU for cheap input. 100 matches
// libjpeg-turbo's own -maxscans default (see docs/decisions.md).
const jpegMaxScans = 100

// reencodeStill is Reencode's own implementation: it checks data's
// declared dimensions against profile from a cheap header read, then
// decodes fully and writes a fresh encode of the result, re-checking
// the caps against the real decoded pixels. An image that passes is
// shrunk to profile.DownscaleTo on its longer side, never enlarged.
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

	// Checked before any resize, while img still carries its original
	// decoded type: downscaleToFit's result is always *image.RGBA,
	// which would otherwise force the alpha scan on every photo.
	alpha := hasAlpha(img)
	img, width, height = downscaleToFit(img, width, height, profile.DownscaleTo)

	return encodeImage(img, alpha, width, height)
}

// downscaleToFit returns img resized so its longer side is maxSide,
// aspect ratio kept exactly, or unchanged when maxSide is zero or img
// already fits: never upscaled. Uses ApproxBiLinear, measured about 8x
// faster than BiLinear or CatmullRom on a 50-megapixel photo, since
// this runs inside app.decodeSlots' single slot every other decode
// waits behind (see docs/decisions.md).
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
// marker (0xFF 0xDA) anywhere in data, a plain linear byte count
// rather than a real parser: an over-count only ever rejects more
// input against jpegMaxScans, never less, which is all this needs.
func countJPEGScans(data []byte) int {
	count := 0
	for i := 0; i+1 < len(data); i++ {
		if data[i] == 0xFF && data[i+1] == 0xDA {
			count++
		}
	}

	return count
}

// decodeImage decodes data with this package's registered decoders
// (PNG, JPEG, still WebP), recovering from any decoder panic on
// crafted input so it refuses the image rather than crashing (see errors.md).
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

// encodeImage writes img as PNG at png.BestSpeed when alpha is true,
// or JPEG at jpegQuality otherwise, independent of data's original format.
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
// opaque, deciding whether encodeImage writes PNG or JPEG. A type
// that can never carry alpha (Gray, Gray16, CMYK, YCbCr) skips the
// pixel scan entirely.
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

// bytesPerPixelBaseline is what estimatedDecodeBytes charges a
// baseline JPEG, 8-bit/paletted PNG or still WebP: measured up to
// about 1.75 bytes/pixel at FullImage's cap, doubled for headroom.
const bytesPerPixelBaseline int64 = 4

// bytesPerPixelExpensive is what estimatedDecodeBytes charges a
// progressive JPEG or 16-bit PNG, which keep far more live per pixel
// while decoding: measured up to about 28 bytes/pixel, rounded up.
const bytesPerPixelExpensive int64 = 32

// estimatedDecodeBytes approximates, from data's header alone, how
// many bytes decoding a width x height image will need at peak,
// cheap enough for reencodeStill to check before a real decode runs.
// Calibrated against measured runtime.MemStats peaks, always erring
// high (see docs/decisions.md).
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
// chunk type, right after the signature and chunk-length field.
const pngIHDRChunkTypeOffset = 12

// pngIHDRBitDepthOffset is the fixed byte offset of a PNG's declared
// bit depth inside its IHDR chunk, read directly only once
// pngIHDRChunkTypeOffset confirms IHDR is actually the first chunk: a
// chunk spliced in before it would otherwise shift this offset into
// the wrong bytes (see docs/decisions.md).
const pngIHDRBitDepthOffset = 24

// pngBitDepth reads data's declared bit depth straight from its IHDR
// chunk when IHDR is confirmed to be the first chunk, or 16 (never 8,
// which would shrink estimatedDecodeBytes's charge) otherwise.
func pngBitDepth(data []byte) int {
	if len(data) <= pngIHDRBitDepthOffset {
		return 16
	}
	if string(data[pngIHDRChunkTypeOffset:pngIHDRChunkTypeOffset+4]) != "IHDR" {
		return 16
	}

	return int(data[pngIHDRBitDepthOffset])
}

// jpegIsProgressive reports whether data's first start-of-frame
// marker is one of the four progressive DCT variants rather than
// baseline or extended-sequential (see estimatedDecodeBytes). Walks
// segments by their declared length rather than assuming any shape;
// defaults to true (never shrinking estimatedDecodeBytes's charge)
// for anything it cannot confirm cheap (see docs/decisions.md).
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

// jpegMarkerHasNoLength reports whether marker is TEM (0x01) or a
// restart marker (0xD0-0xD8): the markers that carry no length field.
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
