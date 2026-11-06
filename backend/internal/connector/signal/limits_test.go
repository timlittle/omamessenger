package signal

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestAttachmentLimit_SameForEveryKind checks that Signal's own 100
// MiB cap applies to every media kind, not just a subset.
func TestAttachmentLimit_SameForEveryKind(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	for _, kind := range []string{domain.MediaPhoto, domain.MediaVideo, domain.MediaFile, domain.MediaVoice, ""} {
		if got := c.AttachmentLimit(kind); got != attachmentLimit {
			t.Errorf("AttachmentLimit(%q) = %d, want %d", kind, got, attachmentLimit)
		}
	}
}
