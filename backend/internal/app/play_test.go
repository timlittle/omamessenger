package app_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestFetchMedia_PlayPathOnlyWhenPlayIsRequested confirms play is the
// one thing that decides whether FetchMedia even tries to build a play
// copy at all: the same video message fetched with play false gets no
// playPath and never calls the transcoder, fetched again with play
// true gets one.
func TestFetchMedia_PlayPathOnlyWhenPlayIsRequested(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Video]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaVideo},
	}); err != nil {
		t.Fatal(err)
	}

	_, playPath, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false)
	if err != nil || playPath != "" {
		t.Fatalf("FetchMedia(play=false) = playPath %q, %v; want \"\", nil", playPath, err)
	}
	if f.transcoder.callCount() != 0 {
		t.Errorf("transcoder calls = %d, want 0 when play was never asked for", f.transcoder.callCount())
	}

	_, playPath, _, _, err = f.commands.FetchMedia(ctx, "m1", false, true)
	if err != nil || playPath == "" {
		t.Fatalf("FetchMedia(play=true) = playPath %q, %v; want a real path, nil", playPath, err)
	}
	if f.transcoder.lastCall() != "video" {
		t.Errorf("transcoder's last call = %q, want \"video\"", f.transcoder.lastCall())
	}
}

// TestFetchMedia_PlaysAnAnimatedVideoThroughTheAnimationProfile
// confirms a video marked Animated (a "GIF video") is transcoded
// through the Animation profile, never Video: the one difference
// between them is whether the source's own audio is kept.
func TestFetchMedia_PlaysAnAnimatedVideoThroughTheAnimationProfile(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[GIF]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaVideo, Animated: true},
	}); err != nil {
		t.Fatal(err)
	}

	_, playPath, _, _, err := f.commands.FetchMedia(ctx, "m1", false, true)
	if err != nil || playPath == "" {
		t.Fatalf("FetchMedia(play=true) = playPath %q, %v; want a real path, nil", playPath, err)
	}
	if f.transcoder.lastCall() != "animation" {
		t.Errorf("transcoder's last call = %q, want \"animation\"", f.transcoder.lastCall())
	}
}

// TestFetchMedia_PlaysVoiceAndAudioThroughTheAudioProfile confirms
// both a voice note and an ordinary audio message are transcoded
// through the Audio profile.
func TestFetchMedia_PlaysVoiceAndAudioThroughTheAudioProfile(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	for _, m := range []domain.Message{
		{ID: "voice", ConversationID: chat.ID, RemoteID: "1", Text: "[Voice]", Created: 1, Media: &domain.Media{Kind: domain.MediaVoice}},
		{ID: "audio", ConversationID: chat.ID, RemoteID: "2", Text: "[Audio]", Created: 2, Media: &domain.Media{Kind: domain.MediaAudio}},
	} {
		if _, _, err := f.store.AddMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	for _, id := range []string{"voice", "audio"} {
		_, playPath, _, _, err := f.commands.FetchMedia(ctx, id, false, true)
		if err != nil || playPath == "" {
			t.Fatalf("FetchMedia(%q, play=true) = playPath %q, %v; want a real path, nil", id, playPath, err)
		}
		if f.transcoder.lastCall() != "audio" {
			t.Errorf("transcoder's last call for %q = %q, want \"audio\"", id, f.transcoder.lastCall())
		}
	}
}

// TestFetchMedia_NoPlayPathWhenTheTranscoderFails confirms a
// transcode failure never becomes a *MediaFetchError: the UI falls
// back to opening the file externally, so FetchMedia still succeeds,
// just without a playPath.
func TestFetchMedia_NoPlayPathWhenTheTranscoderFails(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Video]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaVideo},
	}); err != nil {
		t.Fatal(err)
	}

	f.transcoder.err = errors.New("ffmpeg exploded")
	path, playPath, _, _, err := f.commands.FetchMedia(ctx, "m1", false, true)
	if err != nil || path == "" || playPath != "" {
		t.Fatalf("FetchMedia(play=true, transcoder failing) = path %q, playPath %q, %v; want the original path, no playPath, no error", path, playPath, err)
	}
}

// TestFetchMedia_TranscodesAMessagesPlayCopyOnlyOnce confirms a second
// play fetch of the same message shares the first copy, through the
// media cache's own per-id lock, rather than running the transcoder
// again.
func TestFetchMedia_TranscodesAMessagesPlayCopyOnlyOnce(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Video]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaVideo},
	}); err != nil {
		t.Fatal(err)
	}

	_, first, _, _, err := f.commands.FetchMedia(ctx, "m1", false, true)
	if err != nil || first == "" {
		t.Fatalf("first FetchMedia(play=true) = %q, %v; want a real path, nil", first, err)
	}
	if calls := f.transcoder.callCount(); calls != 1 {
		t.Fatalf("transcoder calls after the first play fetch = %d, want 1", calls)
	}

	_, second, _, _, err := f.commands.FetchMedia(ctx, "m1", false, true)
	if err != nil || second != first {
		t.Fatalf("second FetchMedia(play=true) = %q, %v; want the same path %q again, nil", second, err, first)
	}
	if calls := f.transcoder.callCount(); calls != 1 {
		t.Errorf("transcoder calls after the second play fetch = %d, want still 1 (shared, not re-run)", calls)
	}
}

// TestFetchMedia_PlaysAnOutgoingAttachmentTheSameWay confirms an
// outgoing message's own local attachment - never downloaded - still
// gets a play copy built from it, the same way a downloaded message
// does.
func TestFetchMedia_PlaysAnOutgoingAttachmentTheSameWay(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)

	path := writeTestFile(t, "clip.mp4", onePlayableMP4())
	sent, err := f.commands.Send(ctx, chat.ID, "a clip", app.SendOptions{AttachmentPath: path})
	if err != nil {
		t.Fatal(err)
	}

	_, playPath, _, _, err := f.commands.FetchMedia(ctx, sent.ID, false, true)
	if err != nil || playPath == "" {
		t.Fatalf("FetchMedia(outgoing, play=true) = playPath %q, %v; want a real path, nil", playPath, err)
	}
	if f.transcoder.lastCall() != "video" {
		t.Errorf("transcoder's last call = %q, want \"video\"", f.transcoder.lastCall())
	}
}

// TestFetchMedia_SkipsThePlayCopyWhenTheClaimedDurationIsOverTheCap
// confirms a message whose own claimed duration is already past what
// the chosen profile ever encodes is never sent to the transcoder at
// all: transcoding it would only ever produce a silently truncated
// copy, so the UI should open it externally instead.
func TestFetchMedia_SkipsThePlayCopyWhenTheClaimedDurationIsOverTheCap(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Video]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaVideo, Duration: 60 * 60}, // one hour, far past any profile's own cap
	}); err != nil {
		t.Fatal(err)
	}

	_, playPath, _, _, err := f.commands.FetchMedia(ctx, "m1", false, true)
	if err != nil || playPath != "" {
		t.Fatalf("FetchMedia(play=true, over-cap duration) = playPath %q, %v; want \"\", nil", playPath, err)
	}
	if calls := f.transcoder.callCount(); calls != 0 {
		t.Errorf("transcoder calls = %d, want 0: an over-cap duration must never even try", calls)
	}
}

// TestFetchMedia_PlayNeverAppliesToAPhotoOrSticker confirms play is
// simply ignored for a kind it never applies to, rather than failing.
func TestFetchMedia_PlayNeverAppliesToAPhotoOrSticker(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Photo]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaPhoto},
	}); err != nil {
		t.Fatal(err)
	}

	f.media.data = testPNGBytes(t, 2, 2)
	path, playPath, _, _, err := f.commands.FetchMedia(ctx, "m1", false, true)
	if err != nil || path == "" || playPath != "" {
		t.Fatalf("FetchMedia(photo, play=true) = path %q, playPath %q, %v; want a path, no playPath, no error", path, playPath, err)
	}
	if calls := f.transcoder.callCount(); calls != 0 {
		t.Errorf("transcoder calls = %d, want 0 for a photo", calls)
	}
}

// TestFetchMedia_PlayCopyNameNeverMatchesTheOriginalsOwnLookup
// confirms the cache naming invariant decisions.md records: a play
// copy's own cache slot must never be found by a lookup for the
// original's own id (cacheIdentity(m) alone, with no further
// separator), the exact shape of bug a "." boundary instead of "-"
// would reopen. Voice is the kind this can actually happen with,
// since - unlike video and photo - its original is cached through
// FetchNamed under cacheIdentity(m) directly (see fixedMediaName),
// the same mechanism findByID resolves a play copy's own id through.
func TestFetchMedia_PlayCopyNameNeverMatchesTheOriginalsOwnLookup(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	chat := f.conversation(t, "chat", "Chat", domain.KindDirect)
	if _, _, err := f.store.AddMessage(ctx, domain.Message{
		ID: "m1", ConversationID: chat.ID, RemoteID: "40", Text: "[Voice]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaVoice},
	}); err != nil {
		t.Fatal(err)
	}

	original, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, true)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(f.mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == filepath.Base(original) {
			continue
		}
		if filepath.Base(original)[:len(filepath.Base(original))-len(filepath.Ext(original))] == e.Name() {
			t.Errorf("a derived file %q sits right where the original's own id alone would resolve to", e.Name())
		}
	}

	// The real invariant: fetching "m1" again (play=false, as any
	// ordinary, non-playing fetch would) must still return the
	// original, never the play copy a buggy "." separator could make
	// findByID return instead.
	again, _, _, _, err := f.commands.FetchMedia(ctx, "m1", false, false)
	if err != nil || again != original {
		t.Fatalf("FetchMedia(play=false) after a play fetch = %q, %v; want the original %q again, nil", again, err, original)
	}
}
