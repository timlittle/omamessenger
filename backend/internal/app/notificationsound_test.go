package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

// TestResolveNotificationSound_ServiceReusesTheSameTranscodeWhenUnchanged
// covers the actual security finding: resolveServiceSound runs on
// every incoming notification, so without this, a contact spamming
// messages would occupy the helper's one transcode slot on every
// single one (see docs/decisions.md). A second resolve of the exact
// same, untouched manifest file must not transcode it again.
func TestResolveNotificationSound_ServiceReusesTheSameTranscodeWhenUnchanged(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	path := writeSoundFixture(t, "whatsapp-default.mp3", []byte("whatsapp's own default sound"))

	if _, err := f.commands.ResolveNotificationSound(ctx, "", domain.ServiceWhatsApp, path); err != nil {
		t.Fatalf("first ResolveNotificationSound() error = %v", err)
	}
	if _, err := f.commands.ResolveNotificationSound(ctx, "", domain.ServiceWhatsApp, path); err != nil {
		t.Fatalf("second ResolveNotificationSound() error = %v", err)
	}

	if got := f.transcoder.callCount(); got != 1 {
		t.Errorf("transcoder calls = %d, want exactly 1: a second resolve of the exact same file must not transcode again", got)
	}
}

// TestResolveNotificationSound_ServiceRetranscodesWhenTheFileChanges
// covers the other side: once the manifest file's own content and
// modification time actually change, resolving it again must
// transcode it again and serve the new result.
func TestResolveNotificationSound_ServiceRetranscodesWhenTheFileChanges(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	path := writeSoundFixture(t, "whatsapp-default.mp3", []byte("first version"))

	if _, err := f.commands.ResolveNotificationSound(ctx, "", domain.ServiceWhatsApp, path); err != nil {
		t.Fatalf("first ResolveNotificationSound() error = %v", err)
	}

	later := time.Now().Add(time.Hour)
	if err := os.WriteFile(path, []byte("second, different version"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}

	resolved, err := f.commands.ResolveNotificationSound(ctx, "", domain.ServiceWhatsApp, path)
	if err != nil {
		t.Fatalf("second ResolveNotificationSound() error = %v", err)
	}

	if got := f.transcoder.callCount(); got != 2 {
		t.Errorf("transcoder calls = %d, want exactly 2: a changed file must be retranscoded", got)
	}
	if got, err := os.ReadFile(resolved); err != nil || string(got) != "second, different version" {
		t.Errorf("resolved content = %q, %v; want the new version", got, err)
	}
}

// TestResolveNotificationSound_ServiceRetranscodesWhenTheStoredSoundGoesMissing
// covers the safety net: an unchanged source alone must not be
// enough to skip transcoding if the sound it last produced is no
// longer actually there to serve.
func TestResolveNotificationSound_ServiceRetranscodesWhenTheStoredSoundGoesMissing(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	path := writeSoundFixture(t, "whatsapp-default.mp3", []byte("whatsapp's own default sound"))

	resolved, err := f.commands.ResolveNotificationSound(ctx, "", domain.ServiceWhatsApp, path)
	if err != nil || resolved == "" {
		t.Fatalf("first ResolveNotificationSound() = %q, %v", resolved, err)
	}
	if err := os.Remove(resolved); err != nil {
		t.Fatal(err)
	}

	resolvedAgain, err := f.commands.ResolveNotificationSound(ctx, "", domain.ServiceWhatsApp, path)
	if err != nil || resolvedAgain == "" {
		t.Fatalf("second ResolveNotificationSound() = %q, %v", resolvedAgain, err)
	}

	if got := f.transcoder.callCount(); got != 2 {
		t.Errorf("transcoder calls = %d, want exactly 2: a stored sound removed since must be retranscoded, not just trusted by its unchanged source", got)
	}
	if _, err := os.Stat(resolvedAgain); err != nil {
		t.Errorf("stat resolved path after re-transcode: %v", err)
	}
}

// TestResolveNotificationSound_ServiceSerializesConcurrentResolves
// covers the other half of the same finding: two notifications from
// the same service arriving close together must share one transcode,
// never both run the sandboxed transcoder for a sound neither has
// actually changed. Real concurrency, not a blocking fake: the lock
// guarantees exactly one real transcode regardless of scheduling
// order, since every goroutine but the first to acquire it sees the
// first one's own completed work once its own turn comes.
func TestResolveNotificationSound_ServiceSerializesConcurrentResolves(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	path := writeSoundFixture(t, "whatsapp-default.mp3", []byte("whatsapp's own default sound"))

	const concurrent = 20
	var wg sync.WaitGroup
	for range concurrent {
		wg.Go(func() {
			if _, err := f.commands.ResolveNotificationSound(ctx, "", domain.ServiceWhatsApp, path); err != nil {
				t.Errorf("ResolveNotificationSound() error = %v", err)
			}
		})
	}
	wg.Wait()

	if got := f.transcoder.callCount(); got != 1 {
		t.Errorf("transcoder calls = %d, want exactly 1: concurrent resolves of an unchanged sound must share one transcode", got)
	}
}

// TestResolveNotificationSound_ServiceDoesNotRetryAnUnchangedFileThatAlreadyFailed
// covers the other half of the same security finding
// TestResolveNotificationSound_ServiceReusesTheSameTranscodeWhenUnchanged
// already covers for a successful transcode: resolveServiceSound runs
// on every incoming notification, so a service's default sound that
// fails to transcode once - corrupt, not actually audio - must not
// occupy the helper's one transcode slot on every single later
// notification either, not just while it keeps succeeding.
func TestResolveNotificationSound_ServiceDoesNotRetryAnUnchangedFileThatAlreadyFailed(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	path := writeSoundFixture(t, "not-audio.mp3", []byte("whatever ffmpeg itself would refuse"))
	f.transcoder.err = errors.New("ffmpeg: not a recognised audio stream")

	resolved, err := f.commands.ResolveNotificationSound(ctx, "", domain.ServiceTelegram, path)
	if err != nil || resolved != "" {
		t.Fatalf("first ResolveNotificationSound() = %q, %v; want \"\", nil", resolved, err)
	}
	if got := f.transcoder.callCount(); got != 1 {
		t.Fatalf("transcoder calls after the first resolve = %d, want 1", got)
	}

	resolved, err = f.commands.ResolveNotificationSound(ctx, "", domain.ServiceTelegram, path)
	if err != nil || resolved != "" {
		t.Fatalf("second ResolveNotificationSound() = %q, %v; want \"\", nil", resolved, err)
	}
	if got := f.transcoder.callCount(); got != 1 {
		t.Errorf("transcoder calls after the second resolve = %d, want still 1: an unchanged file that already failed must not be retried", got)
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

// TestResolveNotificationSound_NeverServesAnOldStyleRawFileLeftUnderTheSameKey
// reproduces the actual security finding: an earlier development
// build, before this helper transcoded notification sounds at all,
// copied a user's raw chosen file straight into the sound store under
// a fixed name, unvalidated (see docs/decisions.md). This never calls
// any private helper to learn that old name; it derives the sibling
// legacy path directly from a real resolved path, by swapping its
// extension for ".sound" - the one extension every build before this
// fix ever wrote - so it reproduces the exact collision an upgrade
// could hit, whatever the current key format happens to be.
func TestResolveNotificationSound_NeverServesAnOldStyleRawFileLeftUnderTheSameKey(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Alex", domain.KindDirect)
	transcoded := []byte("a real, transcoded notification sound")
	path := writeSoundFixture(t, "ping.mp3", transcoded)

	if _, err := f.commands.SetNotificationSound(ctx, chat.ID, path); err != nil {
		t.Fatalf("SetNotificationSound() error = %v", err)
	}

	resolved, err := f.commands.ResolveNotificationSound(ctx, chat.ID, "", "")
	if err != nil || resolved == "" {
		t.Fatalf("ResolveNotificationSound() = %q, %v; want a playable path", resolved, err)
	}

	legacy := strings.TrimSuffix(resolved, filepath.Ext(resolved)) + ".sound"
	if err := os.WriteFile(legacy, []byte("the user's own unvalidated file, never transcoded"), 0o600); err != nil {
		t.Fatal(err)
	}

	again, err := f.commands.ResolveNotificationSound(ctx, chat.ID, "", "")
	if err != nil || again != resolved {
		t.Fatalf("ResolveNotificationSound() with a legacy file present = %q, %v; want the same transcoded path %q", again, err, resolved)
	}

	got, err := os.ReadFile(again)
	if err != nil || string(got) != string(transcoded) {
		t.Errorf("content served = %q, %v; want the transcoded bytes, never the legacy file's", got, err)
	}
}

func TestResolveNotificationSound_RequiresAConversationOrAService(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	if _, err := f.commands.ResolveNotificationSound(t.Context(), "", "", ""); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("ResolveNotificationSound() with neither = %v, want ErrInvalidInput", err)
	}
}
