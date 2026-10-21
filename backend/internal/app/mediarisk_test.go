package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// TestMediaRisk_FlagsActiveContentEvenWhenSafelyNamed confirms the
// published catalogue of active or executable content (see
// docs/decisions.md) is caught from the file's own sniffed bytes, the
// one signal a sender cannot forge, whatever name it is kept under.
func TestMediaRisk_FlagsActiveContentEvenWhenSafelyNamed(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		name string
		data []byte
	}{
		"a genuine web page":   {"page.html", []byte("<!DOCTYPE html><html><body>hi</body></html>")},
		"a genuine SVG image":  {"icon.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)},
		"unrecognised content": {"m1.bin", []byte{0x01, 0x02, 0x03, 0x04}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeMediaFixture(t, tt.name, tt.data)
			if !app.MediaRisk(path) {
				t.Errorf("MediaRisk(%q) = false, want true", path)
			}
		})
	}
}

// TestMediaRisk_PassesEverydayMediaThrough confirms the kinds this UI
// opens without asking today - a photo, a video, a PDF, a plain
// document - are never flagged.
func TestMediaRisk_PassesEverydayMediaThrough(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		name string
		data []byte
	}{
		"a jpeg photo":      {"m1.jpg", []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}},
		"a pdf document":    {"m1.pdf", []byte("%PDF-1.4\n")},
		"a plain text file": {"m1.txt", []byte("just some notes")},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := writeMediaFixture(t, tt.name, tt.data)
			if app.MediaRisk(path) {
				t.Errorf("MediaRisk(%q) = true, want false", path)
			}
		})
	}
}

// TestMediaRisk_TreatsAnUnreadableFileAsRisky confirms a file that
// cannot even be read back - gone, or a permission problem - is never
// treated as confirmed safe by default.
func TestMediaRisk_TreatsAnUnreadableFileAsRisky(t *testing.T) {
	t.Parallel()

	if !app.MediaRisk(filepath.Join(t.TempDir(), "gone.bin")) {
		t.Error("MediaRisk(missing file) = false, want true")
	}
}

// writeMediaFixture writes data under name in a fresh temporary
// directory and returns its path, for a test that needs a real file on
// disk for MediaRisk to sniff.
func writeMediaFixture(t *testing.T, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}
