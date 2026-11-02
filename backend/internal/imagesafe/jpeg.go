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

// jpegMaxScans bounds how many start-of-scan (SOS) markers
// copyJPEGRemainder will let through after the header's own first
// one: a progressive JPEG legitimately adds another for each later
// scan, but a real encoder uses on the order of ten (one or two
// passes per colour component). libjpeg-turbo's own djpeg tool bounds
// decoding at 100 scans by default (-maxscans), generous enough for
// any real encoder's output while still refusing the shape a security
// review found: a tiny scan repeated roughly a million times inside a
// file well under this package's own size cap, each repeat walking
// libjpeg across every block of the image again - minutes to hours
// of CPU inside omarchy-shell, with no click needed, since the photo
// bubble that shows it decodes it again every time it scrolls back
// into view.
const jpegMaxScans = 100

// jpegMaxRepeatedTableSegments bounds how many DHT (Huffman table)
// and DQT (quantization table) segments, combined, copyJPEGRemainder
// will let through after the header: a real progressive encode
// repeats these only a handful of times, once or twice per scan, so
// a count far beyond any real file's own use is refused the same way
// a repeated scan is. Checking this costs one more integer compare
// per marker already being read, so there is no reason to leave it
// unbounded just because rebuilding one table is cheaper than
// decoding one scan.
const jpegMaxRepeatedTableSegments = 1_000

// jpegMaxFillBytes bounds the total count of 0xFF fill bytes (the
// JPEG spec's own allowance for any number of them to precede a
// marker, see B.1.1.5) copyJPEGRemainder will skip across its whole
// walk of the remainder: libjpeg-turbo's next_marker skips a run of
// these unconditionally, with no cap of its own, before classifying
// whatever byte follows as the marker code. A handful of fill bytes
// before an occasional marker is ordinary; a count far past that
// serves no real encoder and only costs this walk more cycles for no
// reason, the same shape as the scan and table caps above.
const jpegMaxFillBytes = 64 * 1024

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
// declared dimensions against profile, and returns those dimensions,
// confirmed by finishJPEG's own decode, alongside the rebuilt bytes:
// the real size a caller may need to bound a later decode by (see
// connector.EnsureImageSafe), never the size any later, unrelated
// metadata might claim instead. ok is false for anything structurally
// wrong: a segment whose length overruns the buffer, no
// start-of-frame found, or dimensions over profile's caps.
func sanitizeJPEG(data []byte, profile Profile) (out []byte, width, height int, ok bool) {
	if len(data) < 2 {
		return nil, 0, 0, false
	}

	var buf bytes.Buffer
	buf.Write(data[0:2]) // SOI

	w, h, pos, ok := walkJPEGHeader(data, &buf)
	if !ok || !withinCaps(w, h, profile) {
		return nil, 0, 0, false
	}
	if !copyJPEGRemainder(data, pos, &buf) {
		return nil, 0, 0, false
	}

	return finishJPEG(buf.Bytes(), w, h)
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
		if jpegKeptMarkers[marker] || standalone || isAdobeSegment(marker, data[pos:segEnd]) {
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

// adobeSegmentLength is the full length, marker included, of APP14's
// one real shape: "Adobe", a version, two flag words and the colour
// transform byte.
const adobeSegmentLength = 16

// isAdobeSegment reports whether segment is an APP14 "Adobe" block of
// exactly that fixed size. It is kept, unlike every other APPn but
// JFIF, because its transform byte tells a decoder how the colour
// channels are encoded: without it an RGB JPEG decodes with the wrong
// colours and a CMYK one not at all. Its fixed size leaves no room to
// carry anything else.
func isAdobeSegment(marker byte, segment []byte) bool {
	return marker == 0xEE && len(segment) == adobeSegmentLength && bytes.HasPrefix(segment[4:], []byte("Adobe"))
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
// something to inflate, so nothing here needs filtering beyond a
// marker-aware skip so an 0xFF that starts a later header segment is
// not mistaken for stuffing or restart. Classifying that marker byte
// first skips any run of further 0xFF fill bytes exactly as
// libjpeg-turbo's own next_marker does (see jpegMaxFillBytes): a
// security review found that skipping this step let extra fill bytes
// before a marker - FF FF DA rather than a plain FF DA - hide a
// start-of-scan, DHT or DQT marker from the counts below entirely,
// since the second FF was taken as the marker byte instead. The run is
// collapsed in the output down to the single 0xFF a marker ordinarily
// begins with: fill bytes carry no meaning of their own, so nothing a
// decoder cares about changes, and a long run can no longer be used
// to hide a marker from these counts by padding past where a naive
// scan would look for one. A later start-of-scan marker, and a later
// Huffman or quantization table segment, are each counted and bounded
// (see jpegMaxScans and jpegMaxRepeatedTableSegments): libjpeg itself
// sets no limit of its own on how many times a scan or a table may
// repeat, and walks every block of the image again on each scan, so
// this package's own copy must refuse a count no real encoder would
// ever produce rather than let it through unfiltered.
func copyJPEGRemainder(data []byte, pos int, out *bytes.Buffer) bool {
	scans, tableSegments, fillBytes := 0, 0, 0
	for pos < len(data) {
		next := bytes.IndexByte(data[pos:], 0xFF)
		if next < 0 {
			return false // ran out of data before ever finding EOI
		}

		next += pos
		out.Write(data[pos : next+1])
		pos = next + 1

		for pos < len(data) && data[pos] == 0xFF {
			fillBytes++
			if fillBytes > jpegMaxFillBytes {
				return false
			}
			pos++
		}
		if pos >= len(data) {
			return false
		}

		following := data[pos]
		out.WriteByte(following)
		pos++

		switch following {
		case 0xD9: // EOI
			return true
		case 0xDA: // SOS: another scan
			scans++
			if scans > jpegMaxScans {
				return false
			}
		case 0xC4, 0xDB: // DHT, DQT
			tableSegments++
			if tableSegments > jpegMaxRepeatedTableSegments {
				return false
			}
		}
	}

	return false
}

// finishJPEG confirms out still decodes, and to the same dimensions
// jpegDimensions found, before trusting it as sanitizeJPEG's result.
// width and height are returned back out, verified by this same
// decode, once that confirmation passes.
func finishJPEG(out []byte, width, height int64) ([]byte, int, int, bool) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil || int64(cfg.Width) != width || int64(cfg.Height) != height {
		return nil, 0, 0, false
	}

	return out, cfg.Width, cfg.Height, true
}
