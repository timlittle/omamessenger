//go:build fake

package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/fake"
)

// fakeConnectors returns the scripted fake accounts that test builds run,
// and the injector tests use to deliver messages. OMA_FAKE_DEMO switches
// to the smaller, curated set the README demo recording uses instead.
func fakeConnectors() ([]connector.Connector, app.Injector) {
	suite := fake.New()
	if os.Getenv("OMA_FAKE_DEMO") != "" {
		suite = fake.NewDemo()
	}

	connectors := suite.Connectors()

	return connectors, suite
}

// newTranscoder returns the passthrough transcoder test builds use in
// place of the real sandboxed one, and true: a test build has no
// ffmpeg or bubblewrap to be missing, so in-window playback is always
// reported available. This keeps QML tests, Flows and demo recordings
// working without bwrap or a real ffmpeg installed.
func newTranscoder(_ string) (app.Transcoder, bool) {
	return passthroughTranscoder{}, true
}

// passthroughTranscoder stands in for the real sandboxed transcoder:
// each profile copies its input straight to a fresh output file of
// its own inside a job directory under outDir, the same contract the
// real transcoder's own profiles follow (see docs/decisions.md), so
// app.Commands' own job-directory handling is exercised identically
// either way.
type passthroughTranscoder struct{}

func (passthroughTranscoder) Video(ctx context.Context, in, outDir string) (string, error) {
	return passthroughCopy(ctx, in, outDir, ".mp4")
}

func (passthroughTranscoder) Animation(ctx context.Context, in, outDir string) (string, error) {
	return passthroughCopy(ctx, in, outDir, ".mp4")
}

func (passthroughTranscoder) Audio(ctx context.Context, in, outDir string) (string, error) {
	return passthroughCopy(ctx, in, outDir, ".ogg")
}

func (passthroughTranscoder) Sound(ctx context.Context, in, outDir string) (string, error) {
	return passthroughCopy(ctx, in, outDir, ".wav")
}

func (passthroughTranscoder) StillFrame(ctx context.Context, in, outDir string) (string, error) {
	return passthroughCopy(ctx, in, outDir, ".png")
}

// passthroughCopy copies in's bytes to a fresh "out"+ext file inside
// a new job directory under outDir, honouring ctx so a cancelled
// request does not keep copying regardless.
func passthroughCopy(ctx context.Context, in, outDir, ext string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	jobDir, err := os.MkdirTemp(outDir, "transcode-*")
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(in)
	if err != nil {
		return "", err
	}

	out := filepath.Join(jobDir, "out"+ext)

	return out, os.WriteFile(out, data, 0o600) //nolint:gosec // deliberate: out is this function's own freshly made job directory joined with a fixed extension literal, never built from untrusted input
}
