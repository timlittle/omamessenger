package spell

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

// withEmbeddedBytes temporarily replaces one of the package's embedded
// dictionary byte slices with replacement, restoring the original when
// the test ends. These are package-level go:embed vars, swapped here
// (an internal test) to exercise loadEmbeddedDict's error paths -
// corrupted or malformed data - without a second, fake copy of the
// bundled files on disk.
func withEmbeddedBytes(t *testing.T, slot *[]byte, replacement []byte) {
	t.Helper()

	original := *slot
	*slot = replacement
	t.Cleanup(func() { *slot = original })
}

// gzipBytes compresses data, for building a malformed-but-valid-gzip
// fixture.
func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

// TestLoadEmbeddedDict_NotGzip reports a decompress error, wrapping
// gunzip's own gzip.NewReader error, when an embedded file is not
// gzip-compressed at all.
func TestLoadEmbeddedDict_NotGzip(t *testing.T) {
	withEmbeddedBytes(t, &enUSAffGz, []byte("not gzip data"))

	_, err := loadEmbeddedDict("en_US")
	if err == nil || !strings.Contains(err.Error(), "decompress en_US affix file") {
		t.Errorf("loadEmbeddedDict with a corrupt affix file = %v, want a decompress affix file error", err)
	}
}

// TestLoadEmbeddedDict_DicNotGzip reports a decompress error naming the
// dictionary file specifically, when only it is corrupt.
func TestLoadEmbeddedDict_DicNotGzip(t *testing.T) {
	withEmbeddedBytes(t, &enUSDicGz, []byte("not gzip data"))

	_, err := loadEmbeddedDict("en_US")
	if err == nil || !strings.Contains(err.Error(), "decompress en_US dictionary") {
		t.Errorf("loadEmbeddedDict with a corrupt dictionary file = %v, want a decompress dictionary error", err)
	}
}

// TestLoadEmbeddedDict_UnparsableDic reports a parse error when the
// dictionary decompresses fine but is not a valid Hunspell .dic file.
func TestLoadEmbeddedDict_UnparsableDic(t *testing.T) {
	withEmbeddedBytes(t, &enUSDicGz, gzipBytes(t, []byte("not-a-word-count\n")))

	_, err := loadEmbeddedDict("en_US")
	if err == nil || !strings.Contains(err.Error(), "parse en_US dictionary") {
		t.Errorf("loadEmbeddedDict with an unparsable dictionary = %v, want a parse dictionary error", err)
	}
}

// TestLoadEmbeddedDict decompresses and parses both bundled dictionaries,
// and checks a few words each should and should not know, including the
// British/American spelling difference the two dictionaries exist to
// tell apart.
func TestLoadEmbeddedDict(t *testing.T) {
	t.Parallel()

	cases := []struct {
		locale  string
		correct []string
		wrong   []string
	}{
		{"en_GB", []string{"colour", "organise", "the", "spelling"}, []string{"zxqwplk", "color"}},
		{"en_US", []string{"color", "organize", "the", "spelling"}, []string{"zxqwplk", "colour"}},
	}

	for _, c := range cases {
		t.Run(c.locale, func(t *testing.T) {
			t.Parallel()

			dict, err := loadEmbeddedDict(c.locale)
			if err != nil {
				t.Fatalf("loadEmbeddedDict(%q): %v", c.locale, err)
			}

			for _, word := range c.correct {
				if !dict.Spell(word) {
					t.Errorf("%s: Spell(%q) = false, want true", c.locale, word)
				}
			}
			for _, word := range c.wrong {
				if dict.Spell(word) {
					t.Errorf("%s: Spell(%q) = true, want false", c.locale, word)
				}
			}
		})
	}
}

// TestLoadEmbeddedDict_UnknownLocale reports an error rather than a
// zero-value dictionary.
func TestLoadEmbeddedDict_UnknownLocale(t *testing.T) {
	t.Parallel()

	if _, err := loadEmbeddedDict("fr_FR"); err == nil {
		t.Error("loadEmbeddedDict(fr_FR) = nil error, want one")
	}
}
