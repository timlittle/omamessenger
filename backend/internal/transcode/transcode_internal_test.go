package transcode

// transcode_internal_test.go tests checkOutput and resultError
// directly, as an internal test: both are unexported, and checking
// the output-too-large boundary this way needs only a few bytes,
// rather than writing a real profile's own multi-megabyte cap's worth
// of data through the public API just to cross it.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/sandbox"
)

func TestCheckOutput(t *testing.T) {
	t.Parallel()

	write := func(t *testing.T, data []byte) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "out")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
		return path
	}

	t.Run("missing", func(t *testing.T) {
		t.Parallel()

		err := checkOutput(filepath.Join(t.TempDir(), "never-written"), 100)
		if !errors.Is(err, ErrFailed) {
			t.Fatalf("checkOutput() = %v, want %v", err, ErrFailed)
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		err := checkOutput(write(t, nil), 100)
		if !errors.Is(err, ErrFailed) {
			t.Fatalf("checkOutput() = %v, want %v", err, ErrFailed)
		}
	})

	t.Run("too large", func(t *testing.T) {
		t.Parallel()

		err := checkOutput(write(t, []byte("0123456789")), 9)
		if !errors.Is(err, ErrTooLong) {
			t.Fatalf("checkOutput() = %v, want %v", err, ErrTooLong)
		}
	})

	t.Run("within limits", func(t *testing.T) {
		t.Parallel()

		if err := checkOutput(write(t, []byte("0123456789")), 10); err != nil {
			t.Fatalf("checkOutput() = %v, want nil", err)
		}
	})
}

func TestResultError(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		result  sandbox.Result
		wantErr error // nil means resultError must itself return nil
	}{
		"succeeded":    {result: sandbox.Result{ExitCode: 0}, wantErr: nil},
		"timed out":    {result: sandbox.Result{TimedOut: true}, wantErr: ErrTimeout},
		"too large":    {result: sandbox.Result{TooLarge: true}, wantErr: ErrTooLong},
		"nonzero exit": {result: sandbox.Result{ExitCode: 1}, wantErr: ErrFailed},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := resultError(tt.result)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("resultError() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("resultError() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
