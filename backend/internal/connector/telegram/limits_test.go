package telegram

// limits_test.go checks AttachmentLimit directly: isPremium and setSelf
// are unexported, set only by Run's own sign-in, so there is no way to
// put a connector into a Premium or non-Premium state through the
// public API alone.

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestAttachmentLimit_NonPremiumAccountGetsTwoGiB confirms a newly
// created connector, which has not signed in as Premium, reports the
// free-account cap.
func TestAttachmentLimit_NonPremiumAccountGetsTwoGiB(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "tg"}, "")

	if got := c.AttachmentLimit(domain.MediaFile); got != attachmentLimit {
		t.Errorf("AttachmentLimit(non-Premium) = %d, want %d", got, attachmentLimit)
	}
}

// TestAttachmentLimit_PremiumAccountGetsFourGiB confirms a connector
// that signed in as a Premium account reports the doubled cap,
// regardless of the attachment's own kind.
func TestAttachmentLimit_PremiumAccountGetsFourGiB(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "tg"}, "")
	c.setSelf(1, true)

	for _, kind := range []string{domain.MediaPhoto, domain.MediaVideo, domain.MediaFile} {
		if got := c.AttachmentLimit(kind); got != premiumAttachmentLimit {
			t.Errorf("AttachmentLimit(Premium, %s) = %d, want %d", kind, got, premiumAttachmentLimit)
		}
	}
}
