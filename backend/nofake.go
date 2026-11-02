//go:build !fake

package main

import (
	"os/exec"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/sandbox"
	"github.com/timlittle/omamessenger/backend/internal/transcode"
)

// fakeConnectors returns nothing: release builds run only real accounts.
func fakeConnectors() ([]connector.Connector, app.Injector) {
	return nil, nil
}

// newTranscoder resolves the system ffmpeg and bubblewrap and returns
// the real sandboxed transcoder, and whether both tools were actually
// found on PATH. false is never a reason to fail startup: the helper
// runs exactly the same either way, just without in-window video or
// voice playback, which Doctor reports (see docs/decisions.md).
// workRoot is sandbox.NewRunner's own scratch root and the parent
// every transcode job's own directory is created under; the caller
// creates and clears it first (see resetWorkRoot).
func newTranscoder(workRoot string) (app.Transcoder, bool) {
	ffmpeg, ffmpegErr := exec.LookPath("ffmpeg")
	_, bwrapErr := exec.LookPath("bwrap")

	t := transcode.NewTranscoder(sandbox.NewRunner(workRoot), ffmpeg)

	return t, ffmpegErr == nil && bwrapErr == nil
}
