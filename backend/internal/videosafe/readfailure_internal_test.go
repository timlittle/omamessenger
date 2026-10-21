package videosafe

// readfailure_internal_test.go reaches the defensive branches every
// readAt call site falls back to once a read past a box or element's
// own header fails: cursor.readAt's bounds check already rules this
// out for any well-formed, correctly-sized file (a span this
// package's own walker found is always within the file by
// construction), so the only way to exercise it at all is a reader
// that fails on demand - something the public Validate(path) API,
// always backed by a real *os.File, has no way to ask for. This file
// is an internal test, rather than black-box, for exactly that
// reason; its own fixtures are built from plain bytes rather than the
// builders in the external test package's helpers_test.go, which an
// internal test file cannot see.

import (
	"encoding/binary"
	"io"
	"testing"
)

// endFailingReaderAt serves reads from data normally, except once the
// requested range's own end byte would pass failPast, where it
// reports a short read instead: standing in for a disk I/O error or a
// file truncated after this package already found a box or element's
// header but before it could read the rest of it.
type endFailingReaderAt struct {
	data     []byte
	failPast int64
}

// ReadAt implements io.ReaderAt.
func (r *endFailingReaderAt) ReadAt(buf []byte, off int64) (int, error) {
	if off < 0 || off+int64(len(buf)) > r.failPast {
		return 0, io.ErrUnexpectedEOF
	}
	return copy(buf, r.data[off:off+int64(len(buf))]), nil
}

// box builds a plain 32-bit-size ISO BMFF box: the same shape
// mp4Box in the external test package builds, written out again
// here since an internal test file cannot see that one.
func box(boxType string, body []byte) []byte {
	buf := make([]byte, 8+len(body))
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(buf)))
	copy(buf[4:8], boxType)
	copy(buf[8:], body)
	return buf
}

// mediaHeader builds an mvhd/mdhd body (version 0), the same shape
// mp4MediaHeader in the external test package builds.
func mediaHeader(timescale, duration uint32) []byte {
	buf := make([]byte, 20)
	binary.BigEndian.PutUint32(buf[12:16], timescale)
	binary.BigEndian.PutUint32(buf[16:20], duration)
	return buf
}

// sampleDescription builds a minimal stsd body with one video sample
// entry, the same shape mp4SampleDescription in the external test
// package builds.
func sampleDescription(codec string, width, height uint16) []byte {
	buf := make([]byte, 8+44)
	binary.BigEndian.PutUint32(buf[4:8], 1) // entry_count
	binary.BigEndian.PutUint32(buf[8:12], 44)
	copy(buf[12:16], codec)
	binary.BigEndian.PutUint16(buf[8+32:8+34], width)
	binary.BigEndian.PutUint16(buf[8+34:8+36], height)
	return buf
}

// sampleSize builds a minimal stsz body, the same shape
// mp4SampleSize in the external test package builds.
func sampleSize(sampleCount uint32) []byte {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint32(buf[8:12], sampleCount)
	return buf
}

// vint encodes value as an EBML size vint exactly length bytes wide,
// setting the matching marker bit.
func vint(value uint64, length int) []byte {
	buf := make([]byte, length)
	for i := length - 1; i >= 0; i-- {
		buf[i] = byte(value)
		value >>= 8
	}
	buf[0] |= 0x80 >> (length - 1)
	return buf
}

// elem wraps body in an EBML element with the given raw ID bytes
// (written exactly as the real vint-encoded ID would be: a leading
// byte with its length marker already set) and an 8-byte size vint.
func elem(id []byte, body []byte) []byte {
	out := append(append([]byte{}, id...), vint(uint64(len(body)), 8)...)
	return append(out, body...)
}

// TestCursor_ReadAtFailurePropagates covers every box reader that
// reads a box's header successfully (through firstChild) and then
// fails on a second, larger read of its body: boxHeader's own header
// read, and every higher-level ISO BMFF reader built on top of it.
func TestCursor_ReadAtFailurePropagates(t *testing.T) {
	t.Parallel()

	hdlr := box("hdlr", append(make([]byte, 8), []byte("vide")...))
	mdhd := box("mdhd", mediaHeader(1000, 5000))
	stsd := box("stsd", sampleDescription("avc1", 1280, 720))
	stsz := box("stsz", sampleSize(0))
	stbl := box("stbl", append(append([]byte{}, stsd...), stsz...))

	hdlrOff := int64(0)
	data := hdlr
	mdhdOff := int64(len(data))
	data = append(data, mdhd...)
	stblOff := int64(len(data))
	data = append(data, stbl...)
	stblBodyOff := stblOff + 8
	stblBodyLen := int64(len(stbl)) - 8
	stsdOff := stblBodyOff
	stszOff := stsdOff + int64(len(stsd))

	run := func(name string, failPast int64, fn func(c *cursor) bool) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := &cursor{f: &endFailingReaderAt{data: data, failPast: failPast}, size: int64(len(data)), boxes: maxBoxBudget}
			if fn(c) {
				t.Error("got true, want false once the read past the header fails")
			}
		})
	}

	run("boxHeader's own header read", 4, func(c *cursor) bool {
		_, _, _, ok := c.boxHeader(0, c.size)
		return ok
	})
	run("handlerType", hdlrOff+8+8, func(c *cursor) bool {
		_, ok := c.handlerType(hdlrOff, int64(len(hdlr)))
		return ok
	})
	run("boxDuration version 0", mdhdOff+8+8, func(c *cursor) bool {
		_, ok := c.boxDuration(mdhdOff, int64(len(mdhd)), "mdhd")
		return ok
	})
	run("sampleEntryCodec", stsdOff+8, func(c *cursor) bool {
		_, ok := c.sampleEntryCodec(stblBodyOff, stblBodyLen)
		return ok
	})
	run("sampleEntryDims", stsdOff+8, func(c *cursor) bool {
		_, _, ok := c.sampleEntryDims(stblBodyOff, stblBodyLen)
		return ok
	})
	run("sampleCount", stszOff+8, func(c *cursor) bool {
		_, ok := c.sampleCount(stblBodyOff, stblBodyLen)
		return ok
	})
}

// TestCursor_BoxHeaderExtendedSizeReadFails covers boxHeader's own
// 64-bit extended-size read failing after its ordinary 8-byte header
// read already succeeded.
func TestCursor_BoxHeaderExtendedSizeReadFails(t *testing.T) {
	t.Parallel()

	data := make([]byte, 16)
	data[3] = 1 // size field 1: a 64-bit extended size follows
	copy(data[4:8], "moov")

	c := &cursor{f: &endFailingReaderAt{data: data, failPast: 8}, size: int64(len(data)), boxes: maxBoxBudget}
	if _, _, _, ok := c.boxHeader(0, c.size); ok {
		t.Error("got true, want false once the extended-size read fails")
	}
}

// TestCursor_BoxDurationVersion1ReadFails covers boxDuration's
// version-1 (64-bit) layout failing its own larger read, the
// counterpart to TestCursor_ReadAtFailurePropagates' version-0 case.
func TestCursor_BoxDurationVersion1ReadFails(t *testing.T) {
	t.Parallel()

	body := make([]byte, 32)
	body[0] = 1 // version
	binary.BigEndian.PutUint32(body[20:24], 1000)
	binary.BigEndian.PutUint64(body[24:32], 5000)
	data := box("mvhd", body)

	c := &cursor{f: &endFailingReaderAt{data: data, failPast: 9}, size: int64(len(data)), boxes: maxBoxBudget}
	if _, ok := c.boxDuration(0, int64(len(data)), "mvhd"); ok {
		t.Error("got true, want false once the version-1 duration read fails")
	}
}

// TestCursor_ElementReadFailurePropagates covers every EBML reader
// that reads an element's id and size successfully and then fails on
// a second, larger read of its own body: element's own header read,
// and uintElement, stringElement and floatElement built on it.
func TestCursor_ElementReadFailurePropagates(t *testing.T) {
	t.Parallel()

	widthElem := elem([]byte{0xB0}, []byte{0, 0, 2, 128})     // a PixelWidth element, value 640
	codecElem := elem([]byte{0x86}, []byte("V_VP9"))          // a CodecID element
	durationElem := elem([]byte{0x44, 0x89}, make([]byte, 8)) // a Duration element, the real two-byte ID

	data := widthElem
	codecOff := int64(len(data))
	data = append(data, codecElem...)
	durationOff := int64(len(data))
	data = append(data, durationElem...)

	run := func(name string, failPast int64, fn func(c *cursor) bool) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := &cursor{f: &endFailingReaderAt{data: data, failPast: failPast}, size: int64(len(data)), boxes: maxBoxBudget}
			if fn(c) {
				t.Error("got true, want false once the read past the element header fails")
			}
		})
	}

	run("element's own header read", 2, func(c *cursor) bool {
		_, _, _, ok := c.element(0, c.size)
		return ok
	})
	// Each case below lets the generic 12-byte element header scan
	// (see element's own head buffer) succeed, then fails only the
	// larger, second read of the element's own value.
	run("uintElement", 0+12, func(c *cursor) bool {
		_, ok := c.uintElement(span{off: 0, len: int64(len(widthElem))}, 0xB0)
		return ok
	})
	run("stringElement", codecOff+12, func(c *cursor) bool {
		_, ok := c.stringElement(span{off: codecOff, len: int64(len(codecElem))}, 0x86)
		return ok
	})
	run("floatElement", durationOff+12, func(c *cursor) bool {
		_, ok := c.floatElement(span{off: durationOff, len: int64(len(durationElem))}, 0x4489)
		return ok
	})
}

// TestReadID_RejectsAnInvalidVint and TestReadVint_RejectsAnInvalidVint
// cover the vint decoders' own remaining edge cases directly: an
// empty buffer and a leading byte of 0x00, which no valid EBML vint
// ever starts with.
func TestReadID_RejectsAnInvalidVint(t *testing.T) {
	t.Parallel()

	cases := map[string][]byte{
		"empty buffer":         {},
		"invalid leading byte": {0x00, 0x00},
	}

	for name, buf := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, _, ok := readID(buf); ok {
				t.Errorf("readID(%v) succeeded, want it rejected", buf)
			}
		})
	}
}

func TestReadVint_RejectsAnInvalidVint(t *testing.T) {
	t.Parallel()

	cases := map[string][]byte{
		"empty buffer":         {},
		"invalid leading byte": {0x00, 0x00},
	}

	for name, buf := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, _, _, ok := readVint(buf); ok {
				t.Errorf("readVint(%v) succeeded, want it rejected", buf)
			}
		})
	}
}
