package telegram

import (
	"errors"
	"slices"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestResolvePlaceholderHash_UpgradesOnceTheDialogListNamesTheUser
// confirms the background lookup a fresh placeholder starts finds the
// user's real access hash among the account's recent dialogs and
// reports the upgrade, so a reply can reach the right peer without
// waiting for the next reconnect's full sync.
func TestResolvePlaceholderHash_UpgradesOnceTheDialogListNamesTheUser(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.MessagesGetDialogsRequest{}, &tg.MessagesDialogs{
		Dialogs: []tg.DialogClass{&tg.Dialog{Peer: &tg.PeerUser{UserID: 42}}},
		Users:   []tg.UserClass{nadia()},
	})

	sink := &recordingSink{Sink: &connectortest.Sink{}}
	c := connectedTo(f, sink)
	c.learn("user:42:0") // what resolveConversation's placeholder branch has always already done by the time it starts this lookup

	c.resolvePlaceholderHash(t.Context(), sink, 42)
	c.resolving.Wait()

	want := []remoteIDChangeCall{{"tg", "user:42:0", "user:42:99"}}
	if !slices.Equal(sink.remoteIDs, want) {
		t.Errorf("remoteIDs = %+v, want %+v", sink.remoteIDs, want)
	}
}

// TestResolvePlaceholderHash_GivesUpQuietlyWhenTheUserIsNotInTheList
// confirms a user not among the account's recent dialogs - a stranger
// whose chat Telegram has not yet surfaced there - is left alone
// rather than upgraded to a guessed or zero hash; the placeholder
// stays as it was until a later message, sync or send resolves it.
func TestResolvePlaceholderHash_GivesUpQuietlyWhenTheUserIsNotInTheList(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.MessagesGetDialogsRequest{}, &tg.MessagesDialogs{})

	sink := &recordingSink{Sink: &connectortest.Sink{}}
	c := connectedTo(f, sink)

	c.resolvePlaceholderHash(t.Context(), sink, 42)
	c.resolving.Wait()

	if len(sink.remoteIDs) != 0 {
		t.Errorf("remoteIDs = %+v, want none", sink.remoteIDs)
	}
}

// TestResolvePlaceholderHash_GivesUpQuietlyWhenNotConnected confirms
// the background lookup started for a connector that has since
// disconnected (or never finished connecting) does nothing rather
// than panicking or blocking: Run waits for it through c.resolving
// either way.
func TestResolvePlaceholderHash_GivesUpQuietlyWhenNotConnected(t *testing.T) {
	t.Parallel()

	sink := &recordingSink{Sink: &connectortest.Sink{}}
	c := New(domain.Account{ID: "tg"}, "")

	c.resolvePlaceholderHash(t.Context(), sink, 42)
	c.resolving.Wait()

	if len(sink.remoteIDs) != 0 {
		t.Errorf("remoteIDs = %+v, want none", sink.remoteIDs)
	}
}

// TestResolveSendPeer_ResolvesAnUnresolvedPlaceholderThenLearnsIt
// confirms Send's own peer resolution finds a placeholder's real
// access hash, uses it for the request, and reports the upgrade the
// same way the background lookup does, so the store's own copy of
// the conversation is corrected at the same moment the send succeeds.
func TestResolveSendPeer_ResolvesAnUnresolvedPlaceholderThenLearnsIt(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.MessagesGetDialogsRequest{}, &tg.MessagesDialogs{
		Dialogs: []tg.DialogClass{&tg.Dialog{Peer: &tg.PeerUser{UserID: 42}}},
		Users:   []tg.UserClass{nadia()},
	})

	sink := &recordingSink{Sink: &connectortest.Sink{}}
	c := connectedTo(f, sink)

	peer, remote, err := c.resolveSendPeer(t.Context(), sink, tg.NewClient(f), "user:42:0")
	if err != nil {
		t.Fatalf("resolveSendPeer: %v", err)
	}

	if user, ok := peer.(*tg.InputPeerUser); !ok || user.AccessHash != 99 {
		t.Errorf("peer = %+v, want access hash 99", peer)
	}
	if remote != "user:42:99" {
		t.Errorf("remote = %q, want %q", remote, "user:42:99")
	}

	want := []remoteIDChangeCall{{"tg", "user:42:0", "user:42:99"}}
	if !slices.Equal(sink.remoteIDs, want) {
		t.Errorf("remoteIDs = %+v, want %+v", sink.remoteIDs, want)
	}
}

// TestResolveSendPeer_RefusesWhenTheHashIsStillUnknown confirms a send
// to a placeholder whose user is still nowhere in the account's
// recent dialogs is refused with errUnresolvedPeer rather than ever
// going out with a zero or guessed access hash.
func TestResolveSendPeer_RefusesWhenTheHashIsStillUnknown(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.MessagesGetDialogsRequest{}, &tg.MessagesDialogs{})

	sink := &connectortest.Sink{}
	c := connectedTo(f, sink)

	_, _, err := c.resolveSendPeer(t.Context(), sink, tg.NewClient(f), "user:42:0")
	if !errors.Is(err, errUnresolvedPeer) {
		t.Errorf("resolveSendPeer error = %v, want errUnresolvedPeer", err)
	}
}

// TestResolveSendPeer_PassesThroughAnAlreadyResolvedPeer confirms a
// conversation that already has a real access hash is sent as is,
// with no dialog lookup and no upgrade to report.
func TestResolveSendPeer_PassesThroughAnAlreadyResolvedPeer(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	sink := &recordingSink{Sink: &connectortest.Sink{}}
	c := connectedTo(f, sink)

	peer, remote, err := c.resolveSendPeer(t.Context(), sink, tg.NewClient(f), "user:42:99")
	if err != nil {
		t.Fatalf("resolveSendPeer: %v", err)
	}

	if user, ok := peer.(*tg.InputPeerUser); !ok || user.AccessHash != 99 {
		t.Errorf("peer = %+v, want access hash 99 unchanged", peer)
	}
	if remote != "user:42:99" {
		t.Errorf("remote = %q, want %q", remote, "user:42:99")
	}
	if len(f.sent()) != 0 {
		t.Errorf("requests sent = %d, want none: an already-resolved peer needs no lookup", len(f.sent()))
	}
	if len(sink.remoteIDs) != 0 {
		t.Errorf("remoteIDs = %+v, want none: nothing changed", sink.remoteIDs)
	}
}

// TestResolveSendPeer_RejectsAMalformedRemoteID confirms a malformed
// conversation remote id fails the same way inputPeer itself does,
// before any lookup is attempted.
func TestResolveSendPeer_RejectsAMalformedRemoteID(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	sink := &connectortest.Sink{}
	c := connectedTo(f, sink)

	if _, _, err := c.resolveSendPeer(t.Context(), sink, tg.NewClient(f), "bad"); err == nil {
		t.Error("resolveSendPeer succeeded, want an error")
	}
}
