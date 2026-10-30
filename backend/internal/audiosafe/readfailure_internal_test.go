package audiosafe

// readfailure_internal_test.go reaches the defensive branches every
// readAt call site falls back to once a read past a box's own header
// fails: cursor.readAt's bounds check already rules this out for any
// well-formed, correctly-sized file (a span this package's own walker
// found is always within the file by construction), so the only way
// to exercise it at all is a reader that fails on demand - something
// the public Validate(path) API, always backed by a real *os.File,
// has no way to ask for. This is the same reasoning, and the same
// fake reader shape, as videosafe's own readfailure_internal_test.go.

import (
	"encoding/binary"
	"io"
	"testing"
)

// endFailingReaderAt serves reads from data normally, except once the
// requested range's own end byte would pass failPast, where it
// reports a short read instead: standing in for a disk I/O error or a
// file truncated after this package already found a box's header but
// before it could read the rest of it.
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

// box builds a plain 32-bit-size ISO BMFF box: the same shape mp4Box
// in the external test package builds, written out again here since
// an internal test file cannot see that one.
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

// mediaHeaderV1 builds an mvhd/mdhd body with version 1 (64-bit
// timestamps and duration), the less common of the two layouts
// boxDuration reads.
func mediaHeaderV1(timescale uint32, duration uint64) []byte {
	buf := make([]byte, 32)
	buf[0] = 1 // version
	binary.BigEndian.PutUint32(buf[20:24], timescale)
	binary.BigEndian.PutUint64(buf[24:32], duration)
	return buf
}

// TestCursor_ReadAtFailurePropagates covers every box reader that
// reads a box's header successfully (through firstChild) and then
// fails on a second, larger read of its body: boxHeader's own header
// read, handlerType, boxDuration (both layout versions) and
// sampleEntryCodec.
func TestCursor_ReadAtFailurePropagates(t *testing.T) {
	t.Parallel()

	hdlr := box("hdlr", append(make([]byte, 8), []byte("soun")...))
	mdhd := box("mdhd", mediaHeader(1000, 5000))
	stsd := box("stsd", append(make([]byte, 8), []byte("mp4a0000mp4a0000")...))

	hdlrOff := int64(0)
	data := hdlr
	mdhdOff := int64(len(data))
	data = append(data, mdhd...)
	stsdOff := int64(len(data))
	data = append(data, stsd...)

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
	run("boxDuration version 0", mdhdOff+8+1, func(c *cursor) bool {
		_, ok := c.boxDuration(mdhdOff, int64(len(mdhd)), "mdhd")
		return ok
	})
	run("sampleEntryCodec", stsdOff+8, func(c *cursor) bool {
		_, ok := c.sampleEntryCodec(stsdOff, int64(len(stsd)))
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

// TestCursor_BoxHeaderExtendedSizePastLimit covers boxHeader refusing
// an extended-size box whose own 16-byte header would not fit in its
// container, the extended-size counterpart to an ordinary header
// extending past its container.
func TestCursor_BoxHeaderExtendedSizePastLimit(t *testing.T) {
	t.Parallel()

	data := make([]byte, 12) // room for an 8-byte header, not the 8 more bytes an extended size needs
	data[3] = 1
	copy(data[4:8], "moov")

	c := &cursor{f: &endFailingReaderAt{data: data, failPast: int64(len(data))}, size: int64(len(data)), boxes: maxBoxBudget}
	if _, _, _, ok := c.boxHeader(0, c.size); ok {
		t.Error("got true, want false when the extended-size header itself does not fit in the container")
	}
}

// TestCursor_BoxDurationVersion1ReadFails covers boxDuration's
// version-1 (64-bit) layout failing its own larger read, the
// counterpart to TestCursor_ReadAtFailurePropagates' version-0 case.
func TestCursor_BoxDurationVersion1ReadFails(t *testing.T) {
	t.Parallel()

	data := box("mvhd", mediaHeaderV1(1000, 5000))

	c := &cursor{f: &endFailingReaderAt{data: data, failPast: 9}, size: int64(len(data)), boxes: maxBoxBudget}
	if _, ok := c.boxDuration(0, int64(len(data)), "mvhd"); ok {
		t.Error("got true, want false once the version-1 duration read fails")
	}
}

// TestCursor_BoxDurationTooShort covers boxDuration refusing a
// duration box too short for its own declared version's layout: too
// short for version 0's 20 bytes, and too short for version 1's 32.
func TestCursor_BoxDurationTooShort(t *testing.T) {
	t.Parallel()

	t.Run("version 0, too short", func(t *testing.T) {
		t.Parallel()

		data := box("mdhd", make([]byte, 10))
		c := &cursor{f: &endFailingReaderAt{data: data, failPast: int64(len(data))}, size: int64(len(data)), boxes: maxBoxBudget}
		if _, ok := c.boxDuration(0, int64(len(data)), "mdhd"); ok {
			t.Error("got true, want false for a version-0 duration box too short to read")
		}
	})

	t.Run("version 1, too short", func(t *testing.T) {
		t.Parallel()

		body := make([]byte, 10)
		body[0] = 1 // version 1
		data := box("mdhd", body)
		c := &cursor{f: &endFailingReaderAt{data: data, failPast: int64(len(data))}, size: int64(len(data)), boxes: maxBoxBudget}
		if _, ok := c.boxDuration(0, int64(len(data)), "mdhd"); ok {
			t.Error("got true, want false for a version-1 duration box too short to read")
		}
	})
}

// TestDurationSeconds_ZeroTimescale covers durationSeconds' own
// defensive branch directly: a zero timescale reports zero seconds
// rather than dividing by it.
func TestDurationSeconds_ZeroTimescale(t *testing.T) {
	t.Parallel()

	if got := durationSeconds(0, 5000); got != 0 {
		t.Errorf("durationSeconds(0, 5000) = %v, want 0", got)
	}
}
