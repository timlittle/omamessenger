package whatsapp_test

import (
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/connector/whatsapp"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestAttachmentLimit_PhotoAndVideoGetTheSmallerMediaCap confirms a
// photo or video reports the 16 MiB media cap, the one WhatsApp
// actually enforces for the image and video messages this connector
// sends (see limits.go).
func TestAttachmentLimit_PhotoAndVideoGetTheSmallerMediaCap(t *testing.T) {
	t.Parallel()

	c := whatsapp.New(domain.Account{ID: "wa"}, t.TempDir())

	for _, kind := range []string{domain.MediaPhoto, domain.MediaVideo} {
		if got := c.AttachmentLimit(kind); got != 16<<20 {
			t.Errorf("AttachmentLimit(%s) = %d, want %d", kind, got, int64(16<<20))
		}
	}
}

// TestAttachmentLimit_FileGetsTheLargerDocumentCap confirms a plain
// file reports the 2 GiB document cap, not the smaller media cap a
// photo or video gets.
func TestAttachmentLimit_FileGetsTheLargerDocumentCap(t *testing.T) {
	t.Parallel()

	c := whatsapp.New(domain.Account{ID: "wa"}, t.TempDir())

	if got := c.AttachmentLimit(domain.MediaFile); got != 2<<30 {
		t.Errorf("AttachmentLimit(file) = %d, want %d", got, int64(2<<30))
	}
}
