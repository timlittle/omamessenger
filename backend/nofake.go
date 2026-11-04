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

// newTranscoder resolves the system ffmpeg and bubblewrap and returns the
// real sandboxed transcoder, and whether both tools were found on PATH.
// Missing tools never fail startup, just disable in-window playback.
func newTranscoder(workRoot string) (app.Transcoder, bool) {
	ffmpeg, ffmpegErr := exec.LookPath("ffmpeg")
	_, bwrapErr := exec.LookPath("bwrap")

	t := transcode.NewTranscoder(sandbox.NewRunner(workRoot), ffmpeg)

	return t, ffmpegErr == nil && bwrapErr == nil
}
