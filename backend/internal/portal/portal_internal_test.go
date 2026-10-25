// decodeFileURI's own edge cases, white-box since Pick already covers
// the ordinary success and non-file-scheme paths in portal_test.go.
package portal

import "testing"

func TestDecodeFileURI_ReturnsThePathForAFileURI(t *testing.T) {
	t.Parallel()

	path, err := decodeFileURI("file:///home/user/vacation.jpg")
	if err != nil {
		t.Fatalf("decodeFileURI: %v", err)
	}
	if path != "/home/user/vacation.jpg" {
		t.Errorf("path = %q, want /home/user/vacation.jpg", path)
	}
}

func TestDecodeFileURI_RefusesANonFileScheme(t *testing.T) {
	t.Parallel()

	if _, err := decodeFileURI("http://example.com/evil"); err == nil {
		t.Error("decodeFileURI accepted a non-file scheme")
	}
}

func TestDecodeFileURI_RefusesAFileURIWithNoPath(t *testing.T) {
	t.Parallel()

	if _, err := decodeFileURI("file://"); err == nil {
		t.Error("decodeFileURI accepted a file uri with no path")
	}
}

func TestDecodeFileURI_FailsToParseAMalformedURI(t *testing.T) {
	t.Parallel()

	if _, err := decodeFileURI("file://\x7f"); err == nil {
		t.Error("decodeFileURI accepted a malformed uri")
	}
}
