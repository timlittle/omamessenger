package fake_test

import (
	"strings"
	"testing"
	"testing/synctest"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/connector/fake"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestNewDemo_HasACompactNeutralList confirms the demo recording's fake
// accounts stay small enough to look clean in a short clip, and that none
// of the names reads like a real, personal contact.
func TestNewDemo_HasACompactNeutralList(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &connectortest.Sink{}
		stop := runFake(t, fake.NewDemo(), sink)
		synctest.Wait()
		stop()

		conversations := sink.Messages()
		if len(conversations) < 3 || len(conversations) > 5 {
			t.Fatalf("demo seed has %d conversations, want 3-5 for a clean list", len(conversations))
		}

		for _, line := range sink.Lines() {
			title, ok := strings.CutPrefix(line, "conversation ")
			if !ok {
				continue
			}
			lower := strings.ToLower(title)
			if strings.Contains(lower, "mum") || strings.Contains(lower, "dentist") {
				t.Errorf("conversation %q reads like a real, personal contact", title)
			}
		}
	})
}

// TestNewDemo_OneConversationHasALoadedPhotoAndALinkPreview confirms the
// demo seed gives the recording a single chat to open that shows both a
// photo and a link preview, without needing to scroll to find either.
func TestNewDemo_OneConversationHasALoadedPhotoAndALinkPreview(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &connectortest.Sink{}
		stop := runFake(t, fake.NewDemo(), sink)
		synctest.Wait()
		stop()

		found := false
		for _, messages := range sink.Messages() {
			hasPhoto, hasLink := false, false
			for _, m := range messages {
				if m.Media == nil {
					continue
				}
				if m.Media.Kind == domain.MediaPhoto {
					hasPhoto = true
				}
				if m.Media.Kind == domain.MediaLink {
					hasLink = true
				}
			}
			if hasPhoto && hasLink {
				found = true
			}
		}

		if !found {
			t.Error("no demo conversation carries both a photo and a link preview")
		}
	})
}
