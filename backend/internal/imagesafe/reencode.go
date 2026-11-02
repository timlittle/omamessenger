package imagesafe

import (
	"bytes"
	"image"
	"image/jpeg" // also registers the JPEG decoder reencodeStill uses
	"image/png"  // also registers the PNG decoder reencodeStill uses

	_ "golang.org/x/image/webp" // registers the still-WebP decoder reencodeStill uses; animated WebP has no Go decoder at all
)

// jpegQuality is the quality Reencode writes an opaque photo at: high
// enough that recompression is not visibly lossy, far smaller than
// the PNG this package would otherwise have to write for every photo.
const jpegQuality = 90

// reencodeStill is Reencode's own implementation for every profile
// but AnimatedGIF's own GIF path (see Reencode): it checks data's
// declared dimensions against profile from a cheap header read alone,
// before ever decoding a single pixel, then decodes fully and writes
// a fresh encode of the result. ok is false when data's header
// already fails profile, when it fails to decode at all (an animated
// WebP, truncated input, or anything a decoder's own panic turns into
// a plain refusal - see decodeImage), or when the pixels actually
// decoded still turn out to be over profile's caps: a confirmation
// exactly like finishPNG and finishJPEG's old ones, never trusting a
// header's own claim once real pixels exist to check it against.
func reencodeStill(data []byte, profile Profile) (out []byte, format OutputFormat, width, height int, ok bool) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || !withinCaps(int64(cfg.Width), int64(cfg.Height), profile) {
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

	return encodeImage(img, width, height)
}

// decodeImage decodes data with this package's own registered
// decoders (PNG, JPEG, GIF - to its first frame only - and still
// WebP), recovering from any panic a decoder raises on a crafted
// input it does not itself defend against: a decoder panicking here
// must refuse the image, the same as any other decode failure, never
// crash the helper (see errors.md). A GIF decodes to its first frame
// because image.Decode, unlike gif.DecodeAll, never reads the rest of
// the animation at all.
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
// as PNG at png.BestSpeed when it carries any real transparency (see
// hasAlpha), or JPEG at jpegQuality otherwise: the one choice Reencode
// makes about its own output format, independent of whatever format
// data arrived as. ok is false only if the standard library's own
// encoder itself fails, which neither ever does for a decoded
// image.Image.
func encodeImage(img image.Image, width, height int) (out []byte, format OutputFormat, w, h int, ok bool) {
	var buf bytes.Buffer

	if hasAlpha(img) {
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
