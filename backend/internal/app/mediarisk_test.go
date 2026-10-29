package app_test

import (
	"bytes"
	"compress/gzip"
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

// TestMediaRisk_FlagsSVGFromContentAlone reproduces the security
// review's own confirmed finding: a real SVG almost never starts with
// an "<?xml" line the way TestMediaRisk_FlagsActiveContentEvenWhenSafelyNamed's
// own fixture happens to, which is the one shape Go's sniffer reports
// as XML at all. Named here so neither the extension nor a sniffed
// "text/xml" result can be doing the work - only the content itself,
// the signal a sender cannot forge, decides this.
func TestMediaRisk_FlagsSVGFromContentAlone(t *testing.T) {
	t.Parallel()

	path := writeMediaFixture(t, "payload.txt", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	if !app.MediaRisk(path) {
		t.Errorf("MediaRisk(%q) = false, want true for a bare SVG with no extension or xml prolog to give it away", path)
	}
}

// TestMediaRisk_FlagsGzippedSVGFromContentAlone covers ".svgz": a
// gzip-compressed SVG, which Go's own sniffer only ever reports as
// generic gzip content, never looking inside to see what is actually
// compressed.
func TestMediaRisk_FlagsGzippedSVGFromContentAlone(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	path := writeMediaFixture(t, "payload.dat", buf.Bytes())
	if !app.MediaRisk(path) {
		t.Errorf("MediaRisk(%q) = false, want true for a gzip-compressed SVG", path)
	}
}

// TestMediaRisk_FlagsAGenericXMLDocument confirms a plain XML document
// - not itself an SVG, but still something a browser can be made to
// run a script from, via an XSLT processing instruction - gets the
// same confirmation an HTML page or an SVG already does.
func TestMediaRisk_FlagsAGenericXMLDocument(t *testing.T) {
	t.Parallel()

	path := writeMediaFixture(t, "data.dat", []byte(`<?xml version="1.0"?><records><record>1</record></records>`))
	if !app.MediaRisk(path) {
		t.Errorf("MediaRisk(%q) = false, want true for a generic XML document", path)
	}
}

// TestMediaRisk_FlagsAMIMEWebArchive covers ".mht"/".mhtml": a
// multipart MIME message whose root part is the same active "text/html"
// page a browser would otherwise be confirmed before opening, which
// Go's own sniffer has no entry for at all and would otherwise report
// as plain text.
func TestMediaRisk_FlagsAMIMEWebArchive(t *testing.T) {
	t.Parallel()

	data := []byte("MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/related; boundary=\"----boundary\"\r\n\r\n" +
		"------boundary\r\nContent-Type: text/html\r\n\r\n<html><body>hi</body></html>\r\n------boundary--\r\n")
	path := writeMediaFixture(t, "saved-page.dat", data)
	if !app.MediaRisk(path) {
		t.Errorf("MediaRisk(%q) = false, want true for a MIME web archive", path)
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
