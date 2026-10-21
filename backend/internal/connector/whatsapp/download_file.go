package whatsapp

// download_file.go streams a video, file or voice note straight to
// disk instead of holding the whole decrypted attachment in memory the
// way downloadMedia (device.go) does: whatsmeow's own
// DownloadMediaWithPathToFile copies the ciphertext to the destination
// file as it arrives and decrypts it in place there, so this
// connector's own memory never holds more than one chunk at a time
// regardless of how large the file is (see docs/decisions.md). fetch.go
// wraps the destination in boundedFile so a declared size that
// undersold what the server actually sends is still caught.

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
// actually lands, so a caller can reset an idle timeout exactly when
// the transfer is still making progress (see connector.IdleContext).
// Every other method - Read, Seek, ReadAt, WriteAt, Truncate, Stat,
// Close - passes straight through to the embedded file, which is what
// whatsmeow's own decrypt-in-place step still needs after the initial
// download this wraps; only Write is overridden.
type boundedFile struct {
	*os.File

	max     int64
	written int64
	onWrite func()
}

// Write appends p to the embedded file, refusing once the total
// written would exceed max.
func (b *boundedFile) Write(p []byte) (int, error) {
	if b.written+int64(len(p)) > b.max {
		return 0, errMediaTooLarge
	}

	n, err := b.File.Write(p)
	b.written += int64(n)
	if n > 0 && b.onWrite != nil {
		b.onWrite()
	}

	return n, err
}
