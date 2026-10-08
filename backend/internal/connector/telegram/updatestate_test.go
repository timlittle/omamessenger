package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestUpdateStorage_RoundTripsAcrossInstances saves a full set of state
// through one updateStorage instance and reloads it through a second,
// the way a helper restart opens the same file a stopped run wrote:
// every field, the per-channel pts and both access hash kinds must come
// back exactly as saved.
func TestUpdateStorage_RoundTripsAcrossInstances(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := updateStatePath(dir, "tg-1")
	ctx := t.Context()

	first, err := newUpdateStorage(path)
	if err != nil {
		t.Fatal(err)
	}

	const userID, channelID, otherUserID = 7, 500, 42
	if err := first.SetState(ctx, userID, updates.State{Pts: 1, Qts: 2, Date: 3, Seq: 4}); err != nil {
		t.Fatal(err)
	}
	if err := first.SetPts(ctx, userID, 10); err != nil {
		t.Fatal(err)
	}
	if err := first.SetQts(ctx, userID, 20); err != nil {
		t.Fatal(err)
	}
	if err := first.SetDate(ctx, userID, 30); err != nil {
		t.Fatal(err)
	}
	if err := first.SetSeq(ctx, userID, 40); err != nil {
		t.Fatal(err)
	}
	if err := first.SetDateSeq(ctx, userID, 50, 60); err != nil {
		t.Fatal(err)
	}
	if err := first.SetChannelPts(ctx, userID, channelID, 99); err != nil {
		t.Fatal(err)
	}
	if err := first.SetChannelAccessHash(ctx, userID, channelID, 123456); err != nil {
		t.Fatal(err)
	}
	if err := first.SetUserAccessHash(ctx, userID, otherUserID, 654321); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("update state file mode = %v (%v), want 0600", info.Mode().Perm(), err)
	}

	second, err := newUpdateStorage(path)
	if err != nil {
		t.Fatal(err)
	}

	state, ok, err := second.GetState(ctx, userID)
	want := updates.State{Pts: 10, Qts: 20, Date: 50, Seq: 60}
	if err != nil || !ok || state != want {
		t.Fatalf("GetState after reload = %+v, ok=%v, err=%v, want %+v", state, ok, err, want)
	}

	pts, ok, err := second.GetChannelPts(ctx, userID, channelID)
	if err != nil || !ok || pts != 99 {
		t.Fatalf("GetChannelPts after reload = %d, ok=%v, err=%v, want 99", pts, ok, err)
	}

	hash, ok, err := second.GetChannelAccessHash(ctx, userID, channelID)
	if err != nil || !ok || hash != 123456 {
		t.Fatalf("GetChannelAccessHash after reload = %d, ok=%v, err=%v, want 123456", hash, ok, err)
	}

	userHash, ok, err := second.GetUserAccessHash(ctx, userID, otherUserID)
	if err != nil || !ok || userHash != 654321 {
		t.Fatalf("GetUserAccessHash after reload = %d, ok=%v, err=%v, want 654321", userHash, ok, err)
	}

	seen := map[int64]int{}
	if err := second.ForEachChannels(ctx, userID, func(_ context.Context, id int64, pts int) error {
		seen[id] = pts
		return nil
	}); err != nil {
		t.Fatalf("ForEachChannels after reload = %v", err)
	}
	if seen[channelID] != 99 {
		t.Errorf("ForEachChannels after reload saw %v, want channel %d at pts 99", seen, channelID)
	}
}

// TestUpdateStorage_GetBeforeAnySaveReportsNotFound reproduces what a
// brand-new account sees: nothing saved yet, so every getter reports
// "not found" rather than an error, matching updates.Manager's own
// "forget and fetch the current position" fallback for that case.
func TestUpdateStorage_GetBeforeAnySaveReportsNotFound(t *testing.T) {
	t.Parallel()

	s, err := newUpdateStorage(filepath.Join(t.TempDir(), "tg-new.updates.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	if _, ok, err := s.GetState(ctx, 1); ok || err != nil {
		t.Errorf("GetState = ok=%v, err=%v, want not found", ok, err)
	}
	if _, ok, err := s.GetChannelPts(ctx, 1, 2); ok || err != nil {
		t.Errorf("GetChannelPts = ok=%v, err=%v, want not found", ok, err)
	}
	if _, ok, err := s.GetChannelAccessHash(ctx, 1, 2); ok || err != nil {
		t.Errorf("GetChannelAccessHash = ok=%v, err=%v, want not found", ok, err)
	}
	if _, ok, err := s.GetUserAccessHash(ctx, 1, 2); ok || err != nil {
		t.Errorf("GetUserAccessHash = ok=%v, err=%v, want not found", ok, err)
	}
}

// TestUpdateStorage_SettersFailWithoutASavedState checks every pts,
// qts, date and seq setter against updates.StateStorage's documented
// contract: each must report an error for an account SetState has never
// been called for, which is how gotd's own in-memory storage behaves.
func TestUpdateStorage_SettersFailWithoutASavedState(t *testing.T) {
	t.Parallel()

	s, err := newUpdateStorage(filepath.Join(t.TempDir(), "tg-missing.updates.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	cases := map[string]func() error{
		"SetPts":          func() error { return s.SetPts(ctx, 1, 1) },
		"SetQts":          func() error { return s.SetQts(ctx, 1, 1) },
		"SetDate":         func() error { return s.SetDate(ctx, 1, 1) },
		"SetSeq":          func() error { return s.SetSeq(ctx, 1, 1) },
		"SetDateSeq":      func() error { return s.SetDateSeq(ctx, 1, 1, 1) },
		"SetChannelPts":   func() error { return s.SetChannelPts(ctx, 1, 2, 1) },
		"ForEachChannels": func() error { return s.ForEachChannels(ctx, 1, func(context.Context, int64, int) error { return nil }) },
	}

	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			if err := fn(); !errors.Is(err, errUpdateStateMissing) {
				t.Errorf("%s before SetState = %v, want errUpdateStateMissing", name, err)
			}
		})
	}
}

// TestUpdateStorage_LoadFillsMissingMapsFromAPartialFile reproduces a
// file saved by an earlier, narrower version of this struct (or simply
// one holding only a state with every map field omitted, since Go's
// json package omits a nil map and an empty map identically only when
// tagged omitempty; this file never does, but a hand-edited or
// truncated one still could): loading it must not panic on a nil map
// the rest of this type assumes always exists.
func TestUpdateStorage_LoadFillsMissingMapsFromAPartialFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "tg-partial.updates.json")
	if err := os.WriteFile(path, []byte(`{"states":{"7":{"Pts":1,"Qts":0,"Date":0,"Seq":0}}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := newUpdateStorage(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.SetChannelAccessHash(t.Context(), 7, 500, 1); err != nil {
		t.Errorf("SetChannelAccessHash on a file with no channel-hash map = %v", err)
	}
}

// TestUpdateStorage_RecoversFromACorruptFile reproduces the file a
// crash mid-write could leave behind: truncated, invalid JSON. Loading
// it must start fresh rather than fail, since failing here would stop
// this account's Run from ever starting again until someone deletes
// the file by hand.
func TestUpdateStorage_RecoversFromACorruptFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "tg-corrupt.updates.json")
	if err := os.WriteFile(path, []byte(`{"states":{"7":{"Pts":1,"Qts"`), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := newUpdateStorage(path)
	if err != nil {
		t.Fatalf("newUpdateStorage on a corrupt file = %v, want nil error so Run can still start", err)
	}

	if _, ok, err := s.GetState(t.Context(), 7); ok || err != nil {
		t.Errorf("GetState after recovering from a corrupt file = ok=%v, err=%v, want not found", ok, err)
	}

	if err := s.SetState(t.Context(), 7, updates.State{Pts: 1}); err != nil {
		t.Fatalf("SetState after recovering from a corrupt file = %v, want it usable", err)
	}
}

// TestUpdateStorage_RecoversFromAnUnreadableFile reproduces a state
// file this process cannot read at all - permissions changed under it,
// say - which must be as recoverable as a corrupt one.
func TestUpdateStorage_RecoversFromAnUnreadableFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "tg-unreadable.updates.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) }) // let TempDir's own cleanup remove it

	s, err := newUpdateStorage(path)
	if err != nil {
		t.Fatalf("newUpdateStorage on an unreadable file = %v, want nil error so Run can still start", err)
	}

	if err := s.SetState(t.Context(), 7, updates.State{Pts: 1}); err != nil {
		t.Fatalf("SetState after recovering from an unreadable file = %v, want it usable", err)
	}
}

// TestUpdateStorage_WriteIsAtomic confirms a save never leaves the
// file's temporary sibling behind, and that the file on disk at every
// point is either the previous full save or the new one - never a
// half-written one in between - by reading it back after each save in
// a sequence.
func TestUpdateStorage_WriteIsAtomic(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "tg-atomic.updates.json")
	s, err := newUpdateStorage(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.SetState(t.Context(), 7, updates.State{Pts: 1}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("a temporary file was left behind: %v", err)
	}

	reloaded, err := newUpdateStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	if state, ok, err := reloaded.GetState(t.Context(), 7); err != nil || !ok || state.Pts != 1 {
		t.Fatalf("GetState after one save = %+v, ok=%v, err=%v, want Pts 1", state, ok, err)
	}

	if err := s.SetPts(t.Context(), 7, 2); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("a temporary file was left behind after a second save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk updateStateFile
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("file on disk after a second save is not valid JSON: %v", err)
	}
}

// telegramDifferenceRequest is Telegram's user id for this test's
// account, passed to updates.Manager.Run; it is also the key every
// saved state in these tests is stored under.
const telegramDifferenceRequestUserID = int64(7)

// seedKnownDirectChat runs a connector's own sync against fake so one
// direct chat, "user:42:99", is known before the test drives the
// update manager: every update the gap test delivers afterwards names a
// message in that conversation, and the connector only accepts an edit,
// delete or read update for a conversation it already knows.
func seedKnownDirectChat(t *testing.T, c *Connector, fake *fakeTelegram, sink *connectortest.Sink) {
	t.Helper()

	fake.reply(&tg.ContactsGetContactsRequest{}, &tg.ContactsContactsNotModified{})
	fake.reply(&tg.MessagesGetDialogsRequest{}, &tg.MessagesDialogs{
		Dialogs: []tg.DialogClass{&tg.Dialog{Peer: &tg.PeerUser{UserID: 42}}},
		Users:   []tg.UserClass{&tg.User{ID: 42, AccessHash: 99, FirstName: "Nadia"}},
	})
	fake.reply(&tg.MessagesGetHistoryRequest{}, &tg.MessagesMessagesNotModified{})

	if err := c.sync(t.Context(), tg.NewClient(fake), sink); err != nil {
		t.Fatalf("seed sync: %v", err)
	}
}

// TestConnector_AppliesTheWholeGapOnceAfterARestart reproduces a helper
// restart with saved update state: a new message, an edit, a delete and
// a read-inbox update, everything Telegram's own updates.getDifference
// can hand back in one gap, must all reach the sink exactly once, and
// the manager's own final position must be the one the gap's response
// carried, so the next restart resumes from there rather than replaying
// the same gap again.
func TestConnector_AppliesTheWholeGapOnceAfterARestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		path := updateStatePath(dir, "tg-1")

		seed, err := newUpdateStorage(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := seed.SetState(t.Context(), telegramDifferenceRequestUserID, updates.State{Pts: 10}); err != nil {
			t.Fatal(err)
		}

		fake := newFakeTelegram()
		c := New(domain.Account{ID: "tg-1", Service: domain.ServiceTelegram}, dir)
		var sink connectortest.Sink
		seedKnownDirectChat(t, c, fake, &sink)

		dispatcher := tg.NewUpdateDispatcher()
		c.handleUpdates(dispatcher, &sink)

		storage, err := newUpdateStorage(path)
		if err != nil {
			t.Fatal(err)
		}
		gaps := updates.New(updates.Config{Handler: dispatcher, Storage: storage, AccessHasher: storage, UserAccessHasher: storage})

		now := int(time.Now().Unix())
		fake.reply(&tg.UpdatesGetDifferenceRequest{}, &tg.UpdatesDifference{
			NewMessages: []tg.MessageClass{&tg.Message{ID: 101, PeerID: &tg.PeerUser{UserID: 42}, Message: "hi", Date: now}},
			OtherUpdates: []tg.UpdateClass{
				&tg.UpdateEditMessage{Message: &tg.Message{ID: 102, PeerID: &tg.PeerUser{UserID: 42}, Message: "edited", Date: now}},
				&tg.UpdateDeleteMessages{Messages: []int{999}},
				&tg.UpdateReadHistoryInbox{Peer: &tg.PeerUser{UserID: 42}, MaxID: 50, StillUnreadCount: 0},
			},
			Users: []tg.UserClass{&tg.User{ID: 42, AccessHash: 99, FirstName: "Nadia"}},
			State: tg.UpdatesState{Pts: 20, Date: now},
		})

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() {
			done <- gaps.Run(ctx, tg.NewClient(fake), telegramDifferenceRequestUserID, updates.AuthOptions{})
		}()
		synctest.Wait()

		const remote = "user:42:99"
		for _, line := range []string{
			"incoming " + remote + " 101",
			"edited " + remote + " 102",
			"deleted " + remote + " 999",
			"unread " + remote + " 0",
		} {
			if !sink.Has(line) {
				t.Errorf("events = %q, want %q applied from the gap", sink.Lines(), line)
			}
		}

		reloaded, err := newUpdateStorage(path)
		if err != nil {
			t.Fatal(err)
		}
		if state, ok, err := reloaded.GetState(t.Context(), telegramDifferenceRequestUserID); err != nil || !ok || state.Pts != 20 {
			t.Errorf("saved pts after the gap = %+v, ok=%v, err=%v, want Pts 20", state, ok, err)
		}

		cancel()
		synctest.Wait()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Errorf("gaps.Run after cancel = %v, want context.Canceled", err)
		}
	})
}

// TestConnector_FallsBackGracefullyWhenTheDifferenceIsTooLong
// reproduces an offline gap too large for Telegram to list update by
// update: updates.getDifference answers differenceTooLong once, naming
// the pts to resume from, and gotd itself (not this connector) is
// responsible for saving that pts and asking again rather than looping
// forever or returning an error. This only checks the outcome this
// connector depends on: the run keeps going and the new position is
// the one persisted, so a second restart does not hit the same gap.
func TestConnector_FallsBackGracefullyWhenTheDifferenceIsTooLong(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		path := updateStatePath(dir, "tg-2")

		seed, err := newUpdateStorage(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := seed.SetState(t.Context(), telegramDifferenceRequestUserID, updates.State{Pts: 5}); err != nil {
			t.Fatal(err)
		}

		fake := newFakeTelegram()
		now := int(time.Now().Unix())
		fake.replySequence(&tg.UpdatesGetDifferenceRequest{},
			&tg.UpdatesDifferenceTooLong{Pts: 50},
			&tg.UpdatesDifferenceEmpty{Date: now, Seq: 0},
		)

		storage, err := newUpdateStorage(path)
		if err != nil {
			t.Fatal(err)
		}
		dispatcher := tg.NewUpdateDispatcher()
		gaps := updates.New(updates.Config{Handler: dispatcher, Storage: storage, AccessHasher: storage, UserAccessHasher: storage})

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() {
			done <- gaps.Run(ctx, tg.NewClient(fake), telegramDifferenceRequestUserID, updates.AuthOptions{})
		}()
		synctest.Wait()

		select {
		case err := <-done:
			t.Fatalf("gaps.Run returned early with %v, want it still running after the fallback", err)
		default:
		}

		reloaded, err := newUpdateStorage(path)
		if err != nil {
			t.Fatal(err)
		}
		if state, ok, err := reloaded.GetState(t.Context(), telegramDifferenceRequestUserID); err != nil || !ok || state.Pts != 50 {
			t.Errorf("saved pts after differenceTooLong = %+v, ok=%v, err=%v, want Pts 50", state, ok, err)
		}

		cancel()
		synctest.Wait()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Errorf("gaps.Run after cancel = %v, want context.Canceled", err)
		}
	})
}
