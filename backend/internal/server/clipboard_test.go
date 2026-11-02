package server_test

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestMediaPaste_ReturnsTheClipboardImage(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	s.clipboard.types = []string{"image/png"}
	s.clipboard.data = map[string][]byte{"image/png": pngBytes(t, 2, 2)}

	pasted, err := call[struct {
		Path   string `json:"path"`
		Kind   string `json:"kind"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}](t, s, "media.paste", nil)
	if err != nil || pasted.Kind != domain.MediaPhoto || pasted.Width != 2 || pasted.Height != 2 || pasted.Path == "" {
		t.Errorf("media.paste = %+v, %v", pasted, err)
	}
}

func TestMediaPaste_RejectsAnEmptyClipboard(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	s.clipboard.types = []string{"text/plain"}

	if _, err := call[struct{}](t, s, "media.paste", nil); code(err) != -32602 {
		t.Errorf("media.paste(no image) code = %d, want invalid params", code(err))
	}
}

// TestClipboardReadText_ReturnsTheClipboardsText confirms clipboard.readText
// reaches the clipboard's own bounded text read, for Ctrl+V's own
// plain-text fallback once an image paste finds nothing.
func TestClipboardReadText_ReturnsTheClipboardsText(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	s.clipboard.data = map[string][]byte{"text/plain": []byte("see you at 6")}

	got, err := call[struct {
		Text string `json:"text"`
	}](t, s, "clipboard.readText", nil)
	if err != nil || got.Text != "see you at 6" {
		t.Errorf("clipboard.readText = %+v, %v, want \"see you at 6\"", got, err)
	}
}

// TestClipboardReadText_RejectsAnEmptyClipboard confirms a clipboard
// with no text reports invalid input, the same code media.paste uses
// for no image, rather than returning an empty success the composer
// might insert as nothing.
func TestClipboardReadText_RejectsAnEmptyClipboard(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	if _, err := call[struct{}](t, s, "clipboard.readText", nil); code(err) != -32602 {
		t.Errorf("clipboard.readText(empty clipboard) code = %d, want invalid params", code(err))
	}
}
