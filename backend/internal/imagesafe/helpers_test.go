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

// buildJPEG encodes a solid image of the given size as a baseline JPEG.
func buildJPEG(width, height int) []byte {
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, solidImage(width, height), nil)
	return buf.Bytes()
}

// insertJPEGSegment returns src with one marker segment of the given
// marker byte and payload inserted right after the SOI, exactly where
// APPn or COM segments sit in a real file.
func insertJPEGSegment(src []byte, marker byte, payload []byte) []byte {
	var out bytes.Buffer
	out.Write(src[0:2]) // SOI
	out.WriteByte(0xFF)
	out.WriteByte(marker)

	var length [2]byte
	binary.BigEndian.PutUint16(length[:], uint16(len(payload)+2))
	out.Write(length[:])
	out.Write(payload)
	out.Write(src[2:])
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

// insertGIFExtension returns src with one extension block - introducer,
// label and its sub-blocks - inserted right after the header, logical
// screen descriptor and any global color table, exactly where a
// Comment or Application extension sits in a real file.
func insertGIFExtension(src []byte, label byte, blocks [][]byte) []byte {
	packed := src[10]
	pos := 13
	if packed&0x80 != 0 {
		pos += 3 * (1 << ((packed & 0x07) + 1))
	}

	var out bytes.Buffer
	out.Write(src[:pos])
	out.WriteByte(0x21)
	out.WriteByte(label)
	for _, b := range blocks {
		out.WriteByte(byte(len(b)))
		out.Write(b)
	}
	out.WriteByte(0)
	out.Write(src[pos:])
	return out.Bytes()
}

// setGIFDimensions rewrites a GIF's logical screen width and height
// in place, without touching anything else.
func setGIFDimensions(src []byte, width, height uint16) []byte {
	out := append([]byte{}, src...)
	binary.LittleEndian.PutUint16(out[6:8], width)
	binary.LittleEndian.PutUint16(out[8:10], height)
	return out
}

// addGIFGlobalColorTable sets src's logical screen descriptor's
// global-color-table flag and inserts a two-colour table right after
// it: gif.EncodeAll never produces one of its own, using a local
// table per frame instead, so a test covering it needs one added by
// hand.
func addGIFGlobalColorTable(src []byte) []byte {
	out := append([]byte{}, src...)
	out[10] |= 0x80 // flag bit; size bits already 0, meaning two colours

	var buf bytes.Buffer
	buf.Write(out[:13])
	buf.Write([]byte{255, 0, 0, 0, 0, 255}) // red, blue
	buf.Write(out[13:])
	return buf.Bytes()
}

// buildWebPSimple hand-builds a minimal non-extended WebP: just a
// "VP8 " chunk whose key frame header declares the given size, backed
// by arbitrary payload bytes a real decoder would need but this
// package's own parser never reads.
func buildWebPSimple(width, height int) []byte {
	vp8 := vp8Payload(width, height)
	return riffFile(chunk("VP8 ", vp8))
}

// buildWebPExtended hand-builds an extended WebP: a VP8X chunk
// declaring the given canvas size, with ICCP, EXIF and XMP chunks
// when asked for, followed by a plain VP8 chunk of the same size.
func buildWebPExtended(width, height int, iccp, exif, xmp bool) []byte {
	var body bytes.Buffer
	body.Write(chunk("VP8X", vp8xPayload(width, height, iccp, exif, xmp)))
	if iccp {
		body.Write(chunk("ICCP", []byte("fake-icc-profile-data")))
	}
	if exif {
		body.Write(chunk("EXIF", []byte("fake-exif-metadata-block")))
	}
	if xmp {
		body.Write(chunk("XMP ", []byte("<x:xmpmeta>fake</x:xmpmeta>")))
	}
	body.Write(chunk("VP8 ", vp8Payload(width, height)))
	return riffFile(body.Bytes())
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

// buildWebPLossless hand-builds a minimal non-extended, lossless
// WebP: just a "VP8L" chunk whose signature and packed size field
// declare the given size.
func buildWebPLossless(width, height int) []byte {
	return riffFile(chunk("VP8L", vp8lPayload(width, height)))
}

// vp8lPayload is a VP8L signature byte followed by its packed
// width-1/height-1 size field.
func vp8lPayload(width, height int) []byte {
	bits := uint32(width-1)&0x3FFF | (uint32(height-1)&0x3FFF)<<14
	p := make([]byte, 5)
	p[0] = 0x2f
	binary.LittleEndian.PutUint32(p[1:5], bits)
	return p
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
