package videosafe

import "io"

// cursor reads a file through bounded, absolute-offset reads, shared
// by both the ISO BMFF and EBML walkers: a file is never read
// sequentially or loaded into memory, so this package's own cost
// stays small regardless of how large the file on disk is. boxes
// bounds how many box or element headers either walker may still
// read, shared across one whole Validate call. f is an io.ReaderAt
// rather than *os.File itself - every real caller still passes an
// *os.File, which already satisfies it - solely so this package's own
// tests can inject a reader that fails partway through, the one way
// to reach a readAt failure that a well-formed, correctly-sized file
// never does.
type cursor struct {
	f     io.ReaderAt
	size  int64
	boxes int
}

// span is a box or element's body: offset and length, both counted
// from the start of the file.
type span struct {
	off int64
	len int64
}

// readAt fills buf from off, reporting false rather than a short read
// or an offset outside the file, so every caller can treat any false
// as "this file's header is too short or malformed" without a
// separate bounds check of its own first.
func (c *cursor) readAt(buf []byte, off int64) bool {
	if off < 0 || off+int64(len(buf)) > c.size {
		return false
	}

	n, err := c.f.ReadAt(buf, off)
	return err == nil && n == len(buf)
}
