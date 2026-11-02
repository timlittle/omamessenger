package whatsapp

// fakeDevice's own media-download method, downloadMediaToFile, and its
// bookkeeping, recordDownload. Split out of helpers_test.go, which
// holds the rest of this package's pairing and connection fakes, to
// keep neither file past the project's own file-length limit.

import (
	"bytes"
	"context"
	"io"

	"go.mau.fi/whatsmeow"
)

// downloadMediaToFile records ref and copies the scripted downloadData
// into file the same way real whatsmeow does -
// io.Copy(file, io.TeeReader(resp.Body, hasher)), see whatsmeow's own
// download-to-file.go - reporting downloadErr, or blocking on ctx when
// downloadBlocks is set, as a real download that never hears back from
// WhatsApp's media servers does. Going through io.Copy rather than
// file.Write directly matters: io.Copy prefers a destination's own
// ReadFrom over repeated Write calls, which is exactly how a security
// review found boundedFile's cap bypassed when it still embedded
// *os.File (see download_file.go). A path-specific error in
// downloadErrPaths overrides the plain downloadErr for that one
// reference only; scripting both downloadData and downloadErr drives
// recoverableStaleDigest, since real whatsmeow writes the decrypted
// bytes to file before running the plaintext-hash check that can fail
// that way. A test can also drive connector.BoundedWriter's own cap
// (via boundedFile) by scripting data larger than a test's fetch
// allows.
func (d *fakeDevice) downloadMediaToFile(ctx context.Context, ref mediaRef, file whatsmeow.File) error {
	blocks, data, err := d.recordDownload(ref)
	if blocks {
		<-ctx.Done()
		return ctx.Err()
	}

	// Real whatsmeow writes the decrypted bytes to the file before
	// running the plaintext-hash check that can fail with
	// whatsmeow.ErrInvalidMediaSHA256 (see recoverableStaleDigest), so
	// this writes data whenever there is any, even alongside a
	// scripted error.
	if len(data) > 0 {
		if _, copyErr := io.Copy(file, io.TeeReader(bytes.NewReader(data), io.Discard)); copyErr != nil {
			return copyErr
		}
		// Real whatsmeow then reads the whole file back, to check its
		// MAC and decrypt it in place, so a file the connector opened
		// for writing only fails here exactly as it would for real.
		if readErr := readBack(file); readErr != nil {
			return readErr
		}
	}

	return err
}

// readBack seeks file to its start and reads all of it, the way real
// whatsmeow re-reads a downloaded file before decrypting it.
func readBack(file whatsmeow.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := io.Copy(io.Discard, file)

	return err
}

// recordDownload is downloadMediaToFile's bookkeeping: it records ref
// and reads back whichever of this fake's scripted fields apply to
// it, under its own lock.
func (d *fakeDevice) recordDownload(ref mediaRef) (blocks bool, data []byte, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.downloadCalls = append(d.downloadCalls, ref)
	blocks, data, err = d.downloadBlocks, d.downloadData, d.downloadErr
	if pathErr, ok := d.downloadErrPaths[ref.DirectPath]; ok {
		err = pathErr
	}

	return blocks, data, err
}
