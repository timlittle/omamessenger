package whatsapp

// download_file.go streams an attachment straight to disk instead of
// holding the whole decrypted file in memory (see docs/decisions.md).
// fetch.go and fetch_stream.go both wrap the destination in
// boundedFile so an undersold or zero declared size is still caught.
//
// boundedFile holds the real file as a named field, not embedded: a
// security review found embedding *os.File promoted its ReadFrom,
// which io.Copy picked over calling Write, bypassing the cap entirely.

import (
	"context"
	"os"

	"go.mau.fi/whatsmeow"
)

// downloadMediaToFile downloads and decrypts ref's attachment into
// file. Passing "" for the mms-type lets whatsmeow choose it from the
// app-info key alone, the same as downloadMedia.
func (d *waDevice) downloadMediaToFile(ctx context.Context, ref mediaRef, file whatsmeow.File) error {
	return d.cli.DownloadMediaWithPathToFile(
		ctx, ref.DirectPath, ref.FileEncSHA256, ref.FileSHA256, ref.MediaKey, appInfo(ref.Kind), "", false, file,
	)
}

// boundedFile wraps a real file, refusing a write that would take the
// total written past max and running onWrite after every write that
// lands, so a caller can reset an idle timeout on progress (see
// connector.IdleContext). Only Write counts bytes and enforces max;
// see the package comment for why file is a named field, not embedded.
type boundedFile struct {
	file *os.File

	max     int64
	written int64
	onWrite func()
}

// Write appends p to the wrapped file, refusing once the total
// written would exceed max.
func (b *boundedFile) Write(p []byte) (int, error) {
	if b.written+int64(len(p)) > b.max {
		return 0, errMediaTooLarge
	}

	n, err := b.file.Write(p)
	b.written += int64(n)
	if n > 0 && b.onWrite != nil {
		b.onWrite()
	}

	return n, err
}

// Read forwards to the wrapped file.
func (b *boundedFile) Read(p []byte) (int, error) {
	return b.file.Read(p)
}

// Seek forwards to the wrapped file.
func (b *boundedFile) Seek(offset int64, whence int) (int64, error) {
	return b.file.Seek(offset, whence)
}

// ReadAt forwards to the wrapped file.
func (b *boundedFile) ReadAt(p []byte, off int64) (int, error) {
	return b.file.ReadAt(p, off)
}

// WriteAt forwards to the wrapped file without re-checking max:
// whatsmeow only uses it to decrypt bytes already written, in place.
func (b *boundedFile) WriteAt(p []byte, off int64) (int, error) {
	return b.file.WriteAt(p, off)
}

// Truncate forwards to the wrapped file.
func (b *boundedFile) Truncate(size int64) error {
	return b.file.Truncate(size)
}

// Stat forwards to the wrapped file.
func (b *boundedFile) Stat() (os.FileInfo, error) {
	return b.file.Stat()
}
