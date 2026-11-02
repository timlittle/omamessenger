package connector_test

// media_rawwrite_test.go statically guards that every connector writes a
// message's downloaded media to disk only through SaveMedia, never
// through a raw os.WriteFile or os.OpenFile: one shared, auditable
// write path for every kind, image included, now that checking an
// image's own content is app.Commands's own display copy's job, built
// once it is actually viewed, never a connector's own job at write
// time (see docs/decisions.md).

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// rawFileWrite matches a direct call to os.WriteFile or os.OpenFile:
// the two ways a package could put bytes on disk without going
// through SaveMedia.
var rawFileWrite = regexp.MustCompile(`\bos\.(WriteFile|OpenFile)\(`)

// sharedSaverCall matches a call to this package's own shared saver,
// so the guard test below can confirm a connector actually uses it
// rather than merely not using a raw file write.
var sharedSaverCall = regexp.MustCompile(`connector\.SaveMedia\(`)

// funcHeader matches a top-level function declaration's own line, to
// tell which function a later match in the same file falls under.
var funcHeader = regexp.MustCompile(`(?m)^func (?:\([^)]*\)\s*)?(\w+)\(`)

// rawWriteAllowlist names the one function per connector directory
// allowed a raw file write: a video, audio or document never reaches
// a QML Image, so streaming it straight to disk - rather than
// buffering it in memory only to call SaveMedia - costs nothing in
// safety. Every other function must go through this package's shared
// saver instead.
var rawWriteAllowlist = map[string]string{
	"telegram": "download",
}

// TestConnectors_WriteFetchedMediaOnlyThroughSharedSaver statically
// checks fetch.go in the whatsapp and telegram connector packages -
// the one file in each where FetchMedia decides how to write a
// message's downloaded media to disk: every raw os.WriteFile or
// os.OpenFile call falls under rawWriteAllowlist's one named
// exception, if the directory has one, and each file calls SaveMedia
// at least once. A future change that reached for a raw file write
// anywhere else - the exact mistake a marketplace review found in
// this package's sticker path - fails this test rather than reaching
// production unnoticed.
func TestConnectors_WriteFetchedMediaOnlyThroughSharedSaver(t *testing.T) {
	t.Parallel()

	for _, dir := range []string{"whatsapp", "telegram"} {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(dir, "fetch.go")
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			for _, fn := range functionsContaining(src, rawFileWrite) {
				if fn != rawWriteAllowlist[dir] {
					t.Errorf("%s: func %s calls os.WriteFile or os.OpenFile directly; use connector.SaveMedia instead", path, fn)
				}
			}
			if !sharedSaverCall.Match(src) {
				t.Errorf("%s never calls connector.SaveMedia at all", path)
			}
		})
	}
}

// functionsContaining returns the name of every top-level function in
// src whose body matches pattern at least once.
func functionsContaining(src []byte, pattern *regexp.Regexp) []string {
	headers := funcHeader.FindAllSubmatchIndex(src, -1)
	var names []string

	for i, h := range headers {
		end := len(src)
		if i+1 < len(headers) {
			end = headers[i+1][0]
		}
		if pattern.Match(src[h[0]:end]) {
			names = append(names, string(src[h[2]:h[3]]))
		}
	}

	return names
}

// TestFunctionsContaining_FindsAndSkipsFunctions proves the helper the
// guard test above depends on actually attributes a match to the
// right function, and reports none for a function that has no match
// at all, rather than silently checking nothing.
func TestFunctionsContaining_FindsAndSkipsFunctions(t *testing.T) {
	t.Parallel()

	src := []byte(`package x

func clean() {
	fmt.Println("nothing to see here")
}

func dirty(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}
`)

	got := functionsContaining(src, rawFileWrite)
	if len(got) != 1 || got[0] != "dirty" {
		t.Errorf("functionsContaining() = %v, want [dirty]", got)
	}
}

// TestRawFileWrite_CatchesADeliberateViolation proves the regexp
// above actually matches what it is meant to catch, rather than
// silently matching nothing.
func TestRawFileWrite_CatchesADeliberateViolation(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		`os.WriteFile(path, data, 0o600)`:           true,
		`os.OpenFile(path, os.O_CREATE, 0o600)`:     true,
		`connector.SaveMedia(path, data)`:           false,
		`// os.WriteFile(x, y, z) is disabled here`: true, // the check is textual, on purpose: see its own doc comment
	}

	for src, want := range cases {
		if got := rawFileWrite.MatchString(src); got != want {
			t.Errorf("rawFileWrite.MatchString(%q) = %v, want %v", src, got, want)
		}
	}
}
