package transcode

// joboutput.go pre-creates the single file the sandboxed ffmpeg is
// ever allowed to write: sandbox.Run binds only this one file,
// read-write, never the job directory it lives in - which also holds
// the hard-linked input (see jobinput.go) - so a fully compromised
// ffmpeg can reach that input only through its own, separate read-only
// bind, never through anything reachable from its output (see
// sandbox.Spec's own doc comment and docs/decisions.md).

import (
	"fmt"
	"os"
	"path/filepath"
)

// createJobOutput creates an empty, 0600 regular file named name
// inside jobDir and returns its path: the one location a job's own
// sandbox.Spec.Output may ever point Run at. O_EXCL refuses to
// proceed if that name is somehow already taken - jobDir is always
// freshly made by os.MkdirTemp for exactly one job, so this should
// never actually happen, but this way run finds out rather than
// silently reusing, or truncating, whatever is already there.
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
