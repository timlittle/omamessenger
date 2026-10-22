// gif_test.go is an internal test (package fake, not fake_test) because
// placeholderGIF is unexported and there is no public API that returns
// the fake connector's own GIF bytes: production code never needs them
// outside this package.
package fake

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// TestPlaceholderGIF_PassesTheHelperSOwnGIFCheck guards
// tests/qml/Flows's own end-to-end check that a scripted real GIF
// document animates: if this fixture's bytes ever stopped being a
// genuine, sanitizable GIF image, app.DecodeAsForMedia's
// connector.SanitizeIfGIF call (the same one a real sender's download
// goes through) would start reporting false, and that test would time
// out waiting for a frame that never plays, far from where the real
// cause actually is.
func TestPlaceholderGIF_PassesTheHelperSOwnGIFCheck(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "run.gif")
	if err := os.WriteFile(path, placeholderGIF(), 0o600); err != nil {
		t.Fatal(err)
	}

	isGIF, err := connector.SanitizeIfGIF(path)
	if err != nil || !isGIF {
		t.Fatalf("SanitizeIfGIF() = %t, %v, want true, nil", isGIF, err)
	}
}
