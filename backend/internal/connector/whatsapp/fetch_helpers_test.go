package whatsapp

// fakeDevice's own media-download methods: downloadMedia (the
// in-memory path), downloadMediaToFile (the streamed-to-file path) and
// their shared bookkeeping, recordDownload. Split out of
// helpers_test.go, which holds the rest of this package's pairing and
// connection fakes, to keep neither file past the project's own
// file-length limit.

import (
	"context"

	"go.mau.fi/whatsmeow"
)

// downloadMedia records ref and reports the scripted bytes or error, or
// blocks on ctx when downloadBlocks is set, as a real download that
// never hears back from WhatsApp's media servers does. A path-specific
// error in downloadErrPaths overrides the plain downloadErr for that
// one reference only.
func (d *fakeDevice) downloadMedia(ctx context.Context, ref mediaRef) ([]byte, error) {
	blocks, data, err := d.recordDownload(ref)
	if blocks {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if err != nil {
		// Real whatsmeow still returns the decrypted bytes alongside
		// whatsmeow.ErrInvalidMediaSHA256: its own downloadAndDecrypt
		// sets data before running that specific check. A test that
		// wants to drive recoverStaleDigest sets downloadData as well
		// as downloadErr to mimic that; every other scripted error
		// means no data at all, matching every failure that happens
		// before whatsmeow ever gets to decrypt anything.
		return data, err
	}

	return data, nil
}

// downloadMediaToFile is downloadMedia's own streamed-to-file
// counterpart, driven by the same downloadData, downloadErr,
// downloadErrPaths and downloadBlocks fields: it writes downloadData to
// file through file.Write rather than returning it, so a test can
// still drive connector.BoundedWriter's own cap (via boundedFile) by
// scripting data larger than a test's fetch allows.
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
	// scripted error, exactly like downloadMedia already returns data
	// and err together above.
	if len(data) > 0 {
		if _, writeErr := file.Write(data); writeErr != nil {
			return writeErr
		}
	}

	return err
}

// recordDownload is downloadMedia and downloadMediaToFile's shared
// bookkeeping: it records ref and reads back whichever of this fake's
// scripted fields apply to it, under its own lock.
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
