package clipboard_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

// TestRead_FailsWhenOutputExceedsTheCap confirms a clipboard source
// that answers with more than MaxPasteLen bytes is refused rather than
// read fully into memory: the clipboard belongs to whichever other
// program last put something on it, so its size is never trusted any
// more than a message attachment's.
func TestRead_FailsWhenOutputExceedsTheCap(t *testing.T) {
	t.Parallel()

	w := clipboard.Wayland{Command: stubCommand(fmt.Sprintf("head -c %d /dev/zero", clipboard.MaxPasteLen+1))}

	if err := w.Read(t.Context(), "image/png", io.Discard); err == nil {
		t.Error("Read() over the cap = nil error, want one")
	}
}

// TestWrite_SendsDataToTheCommand confirms Write hands data to the
// command's own stdin, the way wl-copy expects what it is to put on
// the clipboard.
func TestWrite_SendsDataToTheCommand(t *testing.T) {
	t.Parallel()

	dst := filepath.Join(t.TempDir(), "out")
	w := clipboard.Wayland{Command: stubCommand("cat > " + dst)}

	if err := w.Write(t.Context(), "image/png", strings.NewReader("image bytes")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "image bytes" {
		t.Errorf("Write() wrote %q, %v, want %q", got, err, "image bytes")
	}
}

// TestWrite_FailsWhenTheCommandFails confirms a failing wl-copy is
// reported rather than treated as a successful copy.
func TestWrite_FailsWhenTheCommandFails(t *testing.T) {
	t.Parallel()

	w := clipboard.Wayland{Command: stubCommand("exit 1")}

	if err := w.Write(t.Context(), "image/png", strings.NewReader("x")); err == nil {
		t.Error("Write() with a failing command = nil error, want one")
	}
}
