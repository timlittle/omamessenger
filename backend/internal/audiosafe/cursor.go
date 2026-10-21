package audiosafe

import "io"

// cursor reads a file through bounded, absolute-offset reads, shared
// by every format walker in this package: a file is never read
// sequentially or loaded into memory in full, so this package's own
// cost stays small regardless of how large the file on disk is. f is
// an io.ReaderAt rather than *os.File itself, solely so this
// package's own tests can inject a reader that fails partway through,
// the one way to reach a readAt failure that a well-formed file never
// does (see videosafe's own cursor, which this mirrors).
type cursor struct {
	f     io.ReaderAt
	size  int64
	boxes int
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
