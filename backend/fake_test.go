//go:build fake

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestNewTranscoder_ReportsPlaybackAvailable confirms the test
// build's own transcoder always reports playback available: it needs
// no real ffmpeg or bwrap, so QML tests, Flows and a demo recording
// all get in-window playback regardless of what is installed.
func TestNewTranscoder_ReportsPlaybackAvailable(t *testing.T) {
	t.Parallel()

	if _, available := newTranscoder(t.TempDir()); !available {
		t.Fatal("newTranscoder() reports playback unavailable in a test build")
	}
}

// TestPassthroughTranscoder_CopiesInputThroughEveryProfile confirms
// every profile method of the passthrough transcoder writes the
// input's own bytes unchanged into a fresh file under outDir, the
// same job-directory contract the real sandboxed transcoder follows
// (see docs/decisions.md).
func TestPassthroughTranscoder_CopiesInputThroughEveryProfile(t *testing.T) {
	t.Parallel()

	in := filepath.Join(t.TempDir(), "in.bin")
	want := []byte("some input bytes")
	if err := os.WriteFile(in, want, 0o600); err != nil {
		t.Fatal(err)
	}

	var transcoder passthroughTranscoder
	profiles := map[string]func(context.Context, string, string) (string, error){
		"video":      transcoder.Video,
		"animation":  transcoder.Animation,
		"audio":      transcoder.Audio,
		"sound":      transcoder.Sound,
		"stillframe": transcoder.StillFrame,
	}

	for name, run := range profiles {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			out, err := run(t.Context(), in, t.TempDir())
			if err != nil {
				t.Fatalf("%s() = %v", name, err)
			}
			if filepath.Dir(out) == t.TempDir() {
				t.Errorf("%s() wrote directly into outDir %q, want a fresh job directory under it", name, out)
			}

			got, err := os.ReadFile(out)
			if err != nil || string(got) != string(want) {
				t.Errorf("%s() wrote %q, %v; want the input copied through unchanged", name, got, err)
			}
		})
	}
}

// TestPassthroughCopy_StopsOnACancelledContext confirms a cancelled
// context stops the copy rather than running it anyway.
func TestPassthroughCopy_StopsOnACancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	in := filepath.Join(t.TempDir(), "in.bin")
	if err := os.WriteFile(in, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := passthroughCopy(ctx, in, t.TempDir(), ".mp4"); err == nil {
		t.Fatal("passthroughCopy() with a cancelled context succeeded")
	}
}

// TestPassthroughCopy_ReportsAMissingInput confirms a missing input
// file is a plain error, never a silently empty output.
func TestPassthroughCopy_ReportsAMissingInput(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "never-written.bin")
	if _, err := passthroughCopy(t.Context(), missing, t.TempDir(), ".mp4"); err == nil {
		t.Fatal("passthroughCopy() for a missing input succeeded")
	}
}
