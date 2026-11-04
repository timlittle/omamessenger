package transcode

// joboutput.go pre-creates the single file the sandboxed ffmpeg is
// allowed to write, since sandbox.Run binds only this file, not the
// job directory it lives in (see docs/decisions.md).

import (
	"fmt"
	"os"
	"path/filepath"
)

// createJobOutput creates an empty, 0600 regular file named name
// inside jobDir and returns its path. O_EXCL guards against silently
// reusing an unexpected existing file.
func createJobOutput(jobDir, name string) (string, error) {
	path := filepath.Join(jobDir, name)

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("transcode: output file: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("transcode: output file: %w", err)
	}

	return path, nil
}
