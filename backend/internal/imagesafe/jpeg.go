package imagesafe

import (
	"bytes"
	"image"
	_ "image/jpeg" // registers the JPEG decoder Sanitize's own confirmation decode uses
)

// jpegMaxSegments bounds how many marker segments sanitizeJPEG will
// walk before the scan data itself starts, the same guard pngMaxChunks
// gives the PNG walk.
const jpegMaxSegments = 10_000

// jpegKeptMarkers are the header markers sanitizeJPEG keeps: the start
// of frame variants (needed for dimensions and the decoder's own
// setup), Huffman and quantization tables, the restart interval,
// DNL, the scan header itself, and APP0 (plain JFIF, no payload a
// decoder treats as metadata). Every other APPn (EXIF in APP1, an ICC
// profile in APP2, Photoshop data in APP13, Adobe's APP14, ...) and
// COM are dropped, along with anything this map does not name.
var jpegKeptMarkers = map[byte]bool{
	0xC0: true, 0xC1: true, 0xC2: true, 0xC3: true,
	0xC5: true, 0xC6: true, 0xC7: true,
	0xC9: true, 0xCA: true, 0xCB: true,
	0xCD: true, 0xCE: true, 0xCF: true,
	0xC4: true, // DHT
	0xCC: true, // DAC
	0xDB: true, // DQT
	0xDD: true, // DRI
	0xDC: true, // DNL
	0xDA: true, // SOS
	0xE0: true, // APP0 / JFIF
}

// jpegSOFMarkers are the start-of-frame markers that carry an image's
// height and width, read once to check against profile.
var jpegSOFMarkers = map[byte]bool{
	0xC0: true, 0xC1: true, 0xC2: true, 0xC3: true,
	0xC5: true, 0xC6: true, 0xC7: true,
	0xC9: true, 0xCA: true, 0xCB: true,
	0xCD: true, 0xCE: true, 0xCF: true,
}

// sanitizeJPEG rebuilds data's marker stream keeping only
// jpegKeptMarkers, after checking the first start-of-frame segment's
// declared dimensions against profile. ok is false for anything
// structurally wrong: a segment whose length overruns the buffer, no
// start-of-frame found, or dimensions over profile's caps.
func sanitizeJPEG(data []byte, profile Profile) ([]byte, bool) {
	if len(data) < 2 {
		return nil, false
	}

	var out bytes.Buffer
	out.Write(data[0:2]) // SOI

	width, height, pos, ok := walkJPEGHeader(data, &out)
	if !ok || !withinCaps(width, height, profile) {
		return nil, false
	}
	if !copyJPEGRemainder(data, pos, &out) {
		return nil, false
	}

	return finishJPEG(out.Bytes(), width, height)
}

// walkJPEGHeader copies every header segment up to and including the
// first SOS, dropping the ones jpegKeptMarkers excludes, and returns
// the dimensions read from the first start-of-frame segment and the
// position right after that SOS segment's own payload (where entropy
// data begins).
func walkJPEGHeader(data []byte, out *bytes.Buffer) (width, height int64, pos int, ok bool) {
	pos = 2
	width, height = -1, -1

	for i := 0; i < jpegMaxSegments; i++ {
		marker, segEnd, standalone, ok := nextJPEGMarker(data, pos)
		if !ok {
			return 0, 0, 0, false
		}
		if jpegKeptMarkers[marker] || standalone {
			out.Write(data[pos:segEnd])
		}
		if jpegSOFMarkers[marker] && width < 0 {
			if w, h, ok := jpegDimensions(data[pos:segEnd]); ok {
				width, height = w, h
			}
		}

		pos = segEnd
		if marker == 0xDA { // SOS: header ends, entropy-coded data follows
			return width, height, pos, width >= 0
		}
	}

	return 0, 0, 0, false
}

// nextJPEGMarker reads the marker starting at pos: its byte, the
// position right after its segment (itself for a standalone marker
// with no length or payload), and whether it was standalone.
func nextJPEGMarker(data []byte, pos int) (marker byte, segEnd int, standalone, ok bool) {
	if pos+2 > len(data) || data[pos] != 0xFF {
		return 0, 0, false, false
	}

	marker = data[pos+1]
	if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
		return marker, pos + 2, true, true
	}
	if pos+4 > len(data) {
		return 0, 0, false, false
	}

	length := int(data[pos+2])<<8 | int(data[pos+3])
	if length < 2 || pos+2+length > len(data) {
		return 0, 0, false, false
	}

	return marker, pos + 2 + length, false, true
}

// jpegDimensions reads height and width from a start-of-frame
// segment's own payload: one precision byte, then height and width as
// two big-endian uint16s.
func jpegDimensions(segment []byte) (width, height int64, ok bool) {
	if len(segment) < 9 {
		return 0, 0, false
	}

	payload := segment[4:]
	return int64(payload[3])<<8 | int64(payload[4]), int64(payload[1])<<8 | int64(payload[2]), true
}

// copyJPEGRemainder copies everything from pos to the end of data -
// the entropy-coded scan, any restart markers inside it, and any
// later header segments and scans a progressive JPEG adds - straight
// through: none of it is metadata a decoder could mistake for
// something to inflate, so nothing here needs filtering, only a
// marker-aware skip so an 0xFF that starts a later header segment is
// not mistaken for stuffing or restart.
func copyJPEGRemainder(data []byte, pos int, out *bytes.Buffer) bool {
	for pos < len(data) {
		next := bytes.IndexByte(data[pos:], 0xFF)
		if next < 0 {
			return false // ran out of data before ever finding EOI
		}

		next += pos
		out.Write(data[pos : next+1])
		if next+1 >= len(data) {
			return false
		}

		following := data[next+1]
		out.WriteByte(following)
		pos = next + 2
		if following == 0xD9 { // EOI
			return true
		}
	}

	return false
}

// finishJPEG confirms out still decodes, and to the same dimensions
// jpegDimensions found, before trusting it as sanitizeJPEG's result.
func finishJPEG(out []byte, width, height int64) ([]byte, bool) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil || int64(cfg.Width) != width || int64(cfg.Height) != height {
		return nil, false
	}

	return out, true
}
