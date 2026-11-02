package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// writeSoundFixture writes data to a file of its own in t.TempDir() and
// returns its path, the shape SetNotificationSound expects a picked
// file to arrive as.
func writeSoundFixture(t *testing.T, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestSetNotificationSound_StoresATranscodedCopy(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	data := []byte("a notification sound, not real audio - the fake transcoder just copies it through")
	path := writeSoundFixture(t, "ping.mp3", data)

	conv, err := f.commands.SetNotificationSound(ctx, chat.ID, path)
	if err != nil || conv.NotificationSound != "custom" {
		t.Fatalf("SetNotificationSound() = %+v, %v; want notificationSound \"custom\"", conv, err)
	}
	if f.transcoder.lastCall() != "sound" {
		t.Errorf("transcoder's last call = %q, want \"sound\"", f.transcoder.lastCall())
	}

	resolved, err := f.commands.ResolveNotificationSound(ctx, chat.ID, "", "")
	if err != nil || resolved == "" {
		t.Fatalf("ResolveNotificationSound() = %q, %v; want a playable path", resolved, err)
	}

	copied, copyErr := os.ReadFile(resolved)
	if copyErr != nil || string(data) != string(copied) {
		t.Errorf("stored copy = %q, %v; want the original bytes unchanged (the fake transcoder only copies)", copied, copyErr)
	}
}

func TestSetNotificationSound_RejectsAFileTheTranscoderRefuses(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	path := writeSoundFixture(t, "not-audio.mp3", []byte("whatever ffmpeg itself would refuse"))

	f.transcoder.err = errors.New("ffmpeg: not a recognised audio stream")
	if _, err := f.commands.SetNotificationSound(ctx, chat.ID, path); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("SetNotificationSound() with a refused file = %v, want ErrInvalidInput", err)
	}

	conv, err := f.store.Conversation(ctx, chat.ID)
	if err != nil || conv.NotificationSound != "" {
		t.Errorf("conversation after a rejected file = %+v, %v; want notificationSound still \"\"", conv, err)
	}
}

func TestSetNotificationSound_RejectsAMissingFile(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	if _, err := f.commands.SetNotificationSound(ctx, chat.ID, filepath.Join(t.TempDir(), "never-written.mp3")); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("SetNotificationSound() with a missing file = %v, want ErrInvalidInput", err)
	}
	if f.transcoder.callCount() != 0 {
		t.Errorf("transcoder calls = %d, want 0: a missing file must never even try", f.transcoder.callCount())
	}
}

func TestSetNotificationSound_RequiresAConversationAndAFile(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	path := writeSoundFixture(t, "ping.mp3", []byte("whatever, the fake transcoder copies anything"))

	if _, err := f.commands.SetNotificationSound(ctx, "", path); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("SetNotificationSound() with no conversationId = %v, want ErrInvalidInput", err)
	}

	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	if _, err := f.commands.SetNotificationSound(ctx, chat.ID, ""); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("SetNotificationSound() with no path = %v, want ErrInvalidInput", err)
	}
}

func TestResetNotificationSound_ClearsItAndRemovesTheStoredFile(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	path := writeSoundFixture(t, "ping.mp3", []byte("whatever, the fake transcoder copies anything"))

	if _, err := f.commands.SetNotificationSound(ctx, chat.ID, path); err != nil {
		t.Fatal(err)
	}

	conv, err := f.commands.ResetNotificationSound(ctx, chat.ID)
	if err != nil || conv.NotificationSound != "" {
		t.Fatalf("ResetNotificationSound() = %+v, %v; want notificationSound \"\"", conv, err)
	}

	resolved, err := f.commands.ResolveNotificationSound(ctx, chat.ID, "", "")
	if err != nil || resolved != "" {
		t.Errorf("ResolveNotificationSound() after reset = %q, %v; want \"\"", resolved, err)
	}
}

func TestResolveNotificationSound_ConversationWithNoCustomSoundResolvesToNothing(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)

	resolved, err := f.commands.ResolveNotificationSound(ctx, chat.ID, "", "")
	if err != nil || resolved != "" {
		t.Errorf("ResolveNotificationSound() for a plain conversation = %q, %v; want \"\"", resolved, err)
	}
}

func TestResolveNotificationSound_ServiceResolvesATranscodedCopy(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	data := []byte("whatsapp's own default sound, not real audio")
	path := writeSoundFixture(t, "whatsapp-default.mp3", data)

	resolved, err := f.commands.ResolveNotificationSound(ctx, "", domain.ServiceWhatsApp, path)
	if err != nil || resolved == "" {
		t.Fatalf("ResolveNotificationSound() for a service default = %q, %v; want a playable path", resolved, err)
	}

	got, readErr := os.ReadFile(resolved)
	if readErr != nil || string(got) != string(data) {
		t.Errorf("resolved copy = %q, %v; want the original bytes unchanged", got, readErr)
	}
}

func TestResolveNotificationSound_ServiceWithARefusedFileResolvesToNothingNotAnError(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	path := writeSoundFixture(t, "not-audio.mp3", []byte("whatever ffmpeg itself would refuse"))
	f.transcoder.err = errors.New("ffmpeg: not a recognised audio stream")

	resolved, err := f.commands.ResolveNotificationSound(ctx, "", domain.ServiceTelegram, path)
	if err != nil || resolved != "" {
		t.Errorf("ResolveNotificationSound() for a refused service default = %q, %v; want \"\", nil", resolved, err)
	}
}

func TestResolveNotificationSound_RequiresAConversationOrAService(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	if _, err := f.commands.ResolveNotificationSound(t.Context(), "", "", ""); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("ResolveNotificationSound() with neither = %v, want ErrInvalidInput", err)
	}
}
