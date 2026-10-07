package clipboard_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/clipboard"
)

// stubCommand returns a Command that runs name and args through sh -c,
// so tests fake wl-paste without it being installed.
func stubCommand(script string) func(context.Context, string, ...string) *exec.Cmd {
	return func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", script)
	}
}

func TestTypes_ListsEachLine(t *testing.T) {
	t.Parallel()

	w := clipboard.Wayland{Command: stubCommand("printf 'text/plain\\nimage/png\\n'")}

	got, err := w.Types(t.Context())
	if err != nil {
		t.Fatalf("Types() error = %v", err)
	}

	want := []string{"text/plain", "image/png"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Types() = %v, want %v", got, want)
	}
}

func TestTypes_ReportsAnEmptyClipboardAsNoTypes(t *testing.T) {
	t.Parallel()

	w := clipboard.Wayland{Command: stubCommand("true")}

	got, err := w.Types(t.Context())
	if err != nil || len(got) != 0 {
		t.Errorf("Types(empty clipboard) = %v, %v, want no types and no error", got, err)
	}
}

func TestTypes_FailsWhenTheCommandFails(t *testing.T) {
	t.Parallel()

	w := clipboard.Wayland{Command: stubCommand("exit 1")}

	if _, err := w.Types(t.Context()); err == nil {
		t.Error("Types() with a failing command = nil error, want one")
	}
}

func TestRead_WritesTheCommandsOutput(t *testing.T) {
	t.Parallel()

	w := clipboard.Wayland{Command: stubCommand("printf 'image bytes'")}

	var buf strings.Builder
	if err := w.Read(t.Context(), "image/png", &buf); err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if buf.String() != "image bytes" {
		t.Errorf("Read() wrote %q, want %q", buf.String(), "image bytes")
	}
}

func TestRead_FailsWhenTheCommandFails(t *testing.T) {
	t.Parallel()

	w := clipboard.Wayland{Command: stubCommand("exit 1")}

	if err := w.Read(t.Context(), "image/png", new(strings.Builder)); err == nil {
		t.Error("Read() with a failing command = nil error, want one")
	}
}
