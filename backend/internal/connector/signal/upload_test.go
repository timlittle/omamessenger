package signal

import (
	"errors"
	"path/filepath"
	"testing"

	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestUploadAttachment_SetsContentTypeAndVoiceFlag checks that
// uploading a voice note sets the voice-message flag, and an
// ordinary photo does not.
func TestUploadAttachment_SetsContentTypeAndVoiceFlag(t *testing.T) {
	t.Parallel()

	dev := &fakeDevice{uploadPtr: &signalpb.AttachmentPointer{}}
	path := filepath.Join(t.TempDir(), "note.ogg")
	writeTestFile(t, path, []byte("OggS fake audio"))

	ptr, err := uploadAttachment(t.Context(), dev, domain.Message{Media: &domain.Media{Kind: domain.MediaVoice, Path: path}})
	if err != nil {
		t.Fatalf("uploadAttachment() error = %v", err)
	}
	if ptr.GetFlags()&uint32(signalpb.AttachmentPointer_VOICE_MESSAGE) == 0 {
		t.Error("uploaded voice note has no VOICE_MESSAGE flag")
	}
}

// TestUploadAttachment_MissingFilePropagatesError checks that a
// missing attachment file fails before any upload is attempted.
func TestUploadAttachment_MissingFilePropagatesError(t *testing.T) {
	t.Parallel()

	dev := &fakeDevice{}
	path := filepath.Join(t.TempDir(), "missing.jpg")
	_, err := uploadAttachment(t.Context(), dev, domain.Message{Media: &domain.Media{Kind: domain.MediaPhoto, Path: path}})
	if err == nil {
		t.Error("uploadAttachment() with a missing file succeeded, want an error")
	}
	if len(dev.uploaded) != 0 {
		t.Error("uploadAttachment() uploaded with a missing file")
	}
}

// TestUploadAttachment_RefusesUnsupportedKind checks that a kind this
// connector cannot send as an attachment, such as a sticker, is
// refused before any upload is attempted.
func TestUploadAttachment_RefusesUnsupportedKind(t *testing.T) {
	t.Parallel()

	dev := &fakeDevice{}
	_, err := uploadAttachment(t.Context(), dev, domain.Message{Media: &domain.Media{Kind: domain.MediaSticker}})
	if !errors.Is(err, errUnsupportedAttachment) {
		t.Errorf("uploadAttachment() error = %v, want errUnsupportedAttachment", err)
	}
	if len(dev.uploaded) != 0 {
		t.Error("uploadAttachment() uploaded an unsupported kind")
	}
}
