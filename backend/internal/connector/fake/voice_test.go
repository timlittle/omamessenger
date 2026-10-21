// voice_test.go is an internal test (package fake, not fake_test)
// because placeholderVoiceNote is unexported and there is no public
// API that returns the fake connector's own voice note bytes:
// production code never needs them outside this package.
package fake

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// TestPlaceholderVoiceNote_PassesTheHelperSOwnAudioValidation guards
// tests/qml/VoicePlayback's own real end-to-end check: if this
// fixture's bytes ever stopped validating as safe, playable audio
// (see audiosafe, reached here through connector.AudioPlayable, the
// same call app.Commands.AudioPlayableForMedia makes), that test
// would start timing out waiting for a pause glyph that never
// appears, far from where the real cause actually is.
func TestPlaceholderVoiceNote_PassesTheHelperSOwnAudioValidation(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "voice.ogg")
	if err := os.WriteFile(path, placeholderVoiceNote(), 0o600); err != nil {
		t.Fatal(err)
	}

	playable, err := connector.AudioPlayable(path)
	if err != nil || !playable {
		t.Fatalf("AudioPlayable() = %t, %v, want true, nil", playable, err)
	}
}
