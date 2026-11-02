package imagesafe_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
)

// solidImage returns a plain filled image of the given size, enough
// of a real picture for any of the stdlib encoders below.
func solidImage(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}
	return img
}

// buildPNG encodes a solid image of the given size as PNG.
func buildPNG(width, height int) []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, solidImage(width, height))
	return buf.Bytes()
}

// insertPNGChunk returns src with one extra chunk of the given type
// and payload inserted right after its IHDR, exactly where an
// ancillary chunk like zTXt or iCCP would sit in a real file.
func insertPNGChunk(src []byte, kind string, payload []byte) []byte {
	ihdrEnd := 8 + 8 + 13 + 4 // signature + length/type + IHDR body + CRC
	var out bytes.Buffer
	out.Write(src[:ihdrEnd])
	writeChunk(&out, kind, payload)
	out.Write(src[ihdrEnd:])
	return out.Bytes()
}

// writeChunk appends one well-formed PNG chunk - length, type,
// payload and a correct CRC - to out.
func writeChunk(out *bytes.Buffer, kind string, payload []byte) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(payload)))
	out.Write(length[:])
	out.WriteString(kind)
	out.Write(payload)

	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.ChecksumIEEE(append([]byte(kind), payload...)))
	out.Write(crc[:])
}

// setPNGDimensions rewrites a PNG's IHDR width and height in place,
// without touching anything else, so a file can declare a size far
// past what it actually encodes.
func setPNGDimensions(src []byte, width, height uint32) []byte {
	out := append([]byte{}, src...)
	binary.BigEndian.PutUint32(out[16:20], width)
	binary.BigEndian.PutUint32(out[20:24], height)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.ChecksumIEEE(out[12:29]))
	copy(out[29:33], crc[:])
	return out
}

// buildTransparentPNG encodes a solid image of the given size as PNG,
// with one corner pixel made partly transparent: enough for a test to
// confirm Reencode's own hasAlpha check finds real transparency,
// never just an alpha-capable colour type whose pixels all happen to
// be opaque (see TestReencode_OpaqueAlphaChannelStillBecomesJPEG).
func buildTransparentPNG(width, height int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}
	img.SetNRGBA(0, 0, color.NRGBA{G: 200, A: 128})

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// buildSolidColorGIF encodes an animation of the given size with one
// solid-colour frame per entry in colors, for a test that needs each
// frame visually distinct from the others - such as confirming only
// the first frame is ever used once a GIF is reduced to a still image
// (see TestReencode_GIFTakesFirstFrameOnly).
func buildSolidColorGIF(width, height int, colors ...color.Color) []byte {
	g := &gif.GIF{LoopCount: 0}
	for _, c := range colors {
		palette := color.Palette{color.RGBA{A: 255}, c}
		frame := image.NewPaletted(image.Rect(0, 0, width, height), palette)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				frame.SetColorIndex(x, y, 1)
			}
		}
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, 10)
	}

	var buf bytes.Buffer
	_ = gif.EncodeAll(&buf, g)
	return buf.Bytes()
}

// buildJPEG encodes a solid image of the given size as a baseline JPEG.
func buildJPEG(width, height int) []byte {
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, solidImage(width, height), nil)
	return buf.Bytes()
}

// insertFakeMarkersWithFill returns src (a real baseline JPEG) with
// count copies of a marker - 0xFF, extraFill further 0xFF fill bytes,
// then marker - spliced into its own entropy data, right before EOI:
// the exact shape a pre-rewrite security review found could hide an
// unbounded number of scan or table markers from this package's old,
// hand-written marker walk. Reencode no longer walks markers itself -
// decodeImage hands the whole file straight to Go's own jpeg decoder -
// so this now only proves that decoder, not a parser this package
// still owns, never panics or hangs on the shape that broke the old
// one (see FuzzReencode).
func insertFakeMarkersWithFill(src []byte, marker byte, extraFill, count int) []byte {
	eoi := bytes.LastIndex(src, []byte{0xFF, 0xD9})
	if eoi < 0 {
		panic("test JPEG has no EOI marker")
	}

	unit := append(bytes.Repeat([]byte{0xFF}, extraFill+1), marker)
	var out bytes.Buffer
	out.Write(src[:eoi])
	out.Write(bytes.Repeat(unit, count))
	out.Write(src[eoi:])
	return out.Bytes()
}

// buildGIF encodes a looping animation of frameCount solid frames at
// the given size, with a Netscape loop extension, exactly what a real
// animated sticker or GIF produces.
func buildGIF(width, height, frameCount int) []byte {
	g := &gif.GIF{LoopCount: 0}
	palette := []color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}}
	for i := 0; i < frameCount; i++ {
		frame := image.NewPaletted(image.Rect(0, 0, width, height), palette)
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, 10)
	}
	var buf bytes.Buffer
	_ = gif.EncodeAll(&buf, g)
	return buf.Bytes()
}

// setGIFDimensions rewrites a GIF's logical screen width and height
// in place, without touching anything else.
func setGIFDimensions(src []byte, width, height uint16) []byte {
	out := append([]byte{}, src...)
	binary.LittleEndian.PutUint16(out[6:8], width)
	binary.LittleEndian.PutUint16(out[8:10], height)
	return out
}

// buildWebPSimple hand-builds a minimal non-extended WebP: just a
// "VP8 " chunk whose key frame header declares the given size, backed
// by arbitrary payload bytes a real decoder would need but this
// package's own parser never reads.
func buildWebPSimple(width, height int) []byte {
	vp8 := vp8Payload(width, height)
	return riffFile(chunk("VP8 ", vp8))
}

// buildWebPAnimated hand-builds an animated, extended WebP: a VP8X
// chunk, an ANIM chunk, and frameCount ANMF chunks each declaring the
// given frame size.
func buildWebPAnimated(width, height, frameCount int) []byte {
	var body bytes.Buffer
	body.Write(chunk("VP8X", vp8xPayload(width, height, false, false, false)))
	body.Write(chunk("ANIM", []byte{0, 0, 0, 0, 0, 0}))
	for i := 0; i < frameCount; i++ {
		body.Write(chunk("ANMF", anmfPayload(width, height)))
	}
	return riffFile(body.Bytes())
}

// vp8Payload is a VP8 key frame header declaring width and height,
// padded with arbitrary bytes a real decoder would need next.
func vp8Payload(width, height int) []byte {
	p := make([]byte, 10)
	p[3], p[4], p[5] = 0x9d, 0x01, 0x2a
	binary.LittleEndian.PutUint16(p[6:8], uint16(width)&0x3FFF)
	binary.LittleEndian.PutUint16(p[8:10], uint16(height)&0x3FFF)
	return p
}

// vp8xPayload is a VP8X chunk declaring the given canvas size, with
// its ICC, Exif and XMP flag bits set as asked.
func vp8xPayload(width, height int, iccp, exif, xmp bool) []byte {
	p := make([]byte, 10)
	if iccp {
		p[0] |= 0x20
	}
	if exif {
		p[0] |= 0x08
	}
	if xmp {
		p[0] |= 0x04
	}
	put24(p[4:7], width-1)
	put24(p[7:10], height-1)
	return p
}

// anmfPayload is one ANMF frame header declaring the given size,
// followed by a nested VP8 chunk of the same size.
func anmfPayload(width, height int) []byte {
	p := make([]byte, 16)
	put24(p[6:9], width-1)
	put24(p[9:12], height-1)
	return append(p, chunk("VP8 ", vp8Payload(width, height))...)
}

// put24 writes a 24-bit little-endian value into dst.
func put24(dst []byte, v int) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v >> 16)
}

// chunk builds one RIFF chunk: tag, little-endian size, payload and,
// for an odd-length payload, a zero pad byte.
func chunk(fourCC string, payload []byte) []byte {
	var out bytes.Buffer
	out.WriteString(fourCC)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(payload)))
	out.Write(size[:])
	out.Write(payload)
	if len(payload)%2 == 1 {
		out.WriteByte(0)
	}
	return out.Bytes()
}

// riffFile wraps body in a RIFF/WEBP header sized to match.
func riffFile(body []byte) []byte {
	var out bytes.Buffer
	out.WriteString("RIFF")
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(4+len(body)))
	out.Write(size[:])
	out.WriteString("WEBP")
	out.Write(body)
	return out.Bytes()
}
