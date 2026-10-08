package whatsapp

// scenario_test.go adopts the shared Restart, Reorder and
// DuplicateDelivery checks from connectortest, through a driver that
// feeds the connector's own unexported handlers the way live_test.go,
// history_test.go, organize_test.go and delete_test.go already do, and
// adds the WhatsApp-specific bugs real accounts hit that those generic
// checks do not model: the same chat addressed by a LID or a phone
// JID, an expired media link surviving a restart, and an
// undecryptable placeholder resolved by a resend.

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waMmsRetry"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// whatsappDriver runs the shared scenarios against one fake device and
// one on-disk media store, so Restart can hand out a second connector
// instance over the same media store the way a real restart reopens
// the same session and message_keys database.
type whatsappDriver struct {
	dev   *fakeDevice
	media *mediaStore

	current *Connector
	sink    connector.Sink
}

// newWhatsappDriver returns a driver over a freshly paired fake device
// and a media store on disk in a temporary directory.
func newWhatsappDriver(t *testing.T) connectortest.Driver {
	t.Helper()

	dev := newFakeDevice()
	dev.paired = true

	return &whatsappDriver{dev: dev, media: newTestMediaStore(t)}
}

// Connector returns a new connector instance wired to this driver's
// fake device and media store, with every in-memory field starting
// empty, as a process restart would see it.
func (d *whatsappDriver) Connector(t *testing.T) connector.Connector {
	t.Helper()

	return &Connector{
		account:   domain.Account{ID: "wa-1", Service: domain.ServiceWhatsApp},
		answers:   make(chan answer, 1),
		open:      func(context.Context) (device, error) { return d.dev, nil },
		openMedia: func(context.Context) (*mediaStore, error) { return d.media, nil },
		organize:  map[string]organizeState{},
		names:     map[string]namedEntry{},
		reactions: map[string]map[string]string{},
	}
}

// Run wires c to this driver's device and media store as if Run had
// already connected it, remembers both for Deliver, and returns a stop
// func that disconnects it.
func (d *whatsappDriver) Run(t *testing.T, c connector.Connector, sink connector.Sink) func() {
	t.Helper()

	wc, ok := c.(*Connector)
	if !ok {
		t.Fatalf("connector = %T, want *Connector", c)
	}

	wc.connected(d.dev, sink, d.media)
	d.current, d.sink = wc, sink

	return func() { wc.disconnected() }
}

// Backend returns the fake device Deliver and Run acted on.
func (d *whatsappDriver) Backend() any { return d.dev }

// Deliver turns one scripted event into the call this connector's own
// handlers expect.
func (d *whatsappDriver) Deliver(t *testing.T, e connectortest.Event) {
	t.Helper()

	ctx := t.Context()
	jid, err := jidFromRemoteID(e.ConversationRemoteID)
	if err != nil {
		t.Fatalf("event conversation remote id %q: %v", e.ConversationRemoteID, err)
	}

	switch e.Kind {
	case connectortest.EventConversation:
		d.syncConversation(ctx, e)
	case connectortest.EventMessage:
		d.deliverMessage(ctx, jid, e)
	case connectortest.EventOrganize:
		d.deliverOrganize(ctx, jid, e)
	case connectortest.EventRead:
		d.current.handleMarkChatAsRead(ctx, d.sink, d.dev, &events.MarkChatAsRead{
			JID: jid, Action: &waSyncAction.MarkChatAsReadAction{Read: boolPtr(true)},
		})
	case connectortest.EventDelete:
		for _, id := range e.DeleteRemoteIDs {
			d.current.handleDeleteForMe(ctx, d.sink, d.dev, &events.DeleteForMe{ChatJID: jid, MessageID: id})
		}
	}
}

// syncConversation reports a conversation becoming known, through a
// history sync carrying one seed message: WhatsApp drops a conversation
// with no real content the first time it is reported (see history.go's
// syncConversation), so an EventConversation with none of its own would
// otherwise vanish.
func (d *whatsappDriver) syncConversation(ctx context.Context, e connectortest.Event) {
	sc := &waHistorySync.Conversation{
		ID:       strPtr(e.ConversationRemoteID),
		Name:     strPtr(e.Title),
		Messages: []*waHistorySync.HistorySyncMsg{historyMsg(e.ConversationRemoteID+"-seed", "hello", false)},
	}
	d.current.handleHistorySync(ctx, d.sink, d.dev, d.media,
		&events.HistorySync{Data: &waHistorySync.HistorySync{Conversations: []*waHistorySync.Conversation{sc}}})
}

// deliverMessage reports a message, live or from history.
func (d *whatsappDriver) deliverMessage(ctx context.Context, chat types.JID, e connectortest.Event) {
	sender := chat
	if e.SenderID != "" {
		if j, err := jidFromRemoteID(e.SenderID); err == nil {
			sender = j
		}
	}

	if e.Live {
		d.current.handleMessage(ctx, d.sink, d.dev, d.media, &events.Message{
			Info: types.MessageInfo{
				MessageSource: types.MessageSource{Chat: chat, Sender: sender},
				ID:            types.MessageID(e.MessageRemoteID), PushName: e.SenderName, Timestamp: time.Now(),
			},
			Message: &waE2E.Message{Conversation: strPtr(e.Text)},
		})

		return
	}

	sc := &waHistorySync.Conversation{
		ID:       strPtr(e.ConversationRemoteID),
		Messages: []*waHistorySync.HistorySyncMsg{historyMsg(e.MessageRemoteID, e.Text, false)},
	}
	d.current.handleHistorySync(ctx, d.sink, d.dev, d.media,
		&events.HistorySync{Data: &waHistorySync.HistorySync{Conversations: []*waHistorySync.Conversation{sc}}})
}

// deliverOrganize reports a pin or an archive change, each field
// independently, the way WhatsApp's own live events do.
func (d *whatsappDriver) deliverOrganize(ctx context.Context, jid types.JID, e connectortest.Event) {
	if e.Pinned != nil {
		d.current.handlePin(ctx, d.sink, d.dev, &events.Pin{JID: jid, Action: &waSyncAction.PinAction{Pinned: boolPtr(*e.Pinned)}})
	}
	if e.Archived != nil {
		d.current.handleArchive(ctx, d.sink, d.dev, &events.Archive{JID: jid, Action: &waSyncAction.ArchiveChatAction{Archived: boolPtr(*e.Archived)}})
	}
}

// scenarioEvents is a representative set of updates for one direct
// chat: it becoming known, a live message, a pin, a second live
// message and the chat marked read, the same shape of update a real
// session produces.
func scenarioEvents() []connectortest.Event {
	remote := remoteID(types.NewJID("15551234567", types.DefaultUserServer))
	pinned := true

	return []connectortest.Event{
		{Kind: connectortest.EventConversation, ConversationRemoteID: remote, Title: "Nadia", ConversationKind: domain.KindDirect},
		{Kind: connectortest.EventMessage, ConversationRemoteID: remote, MessageRemoteID: "M1", Text: "hi", Live: true},
		{Kind: connectortest.EventOrganize, ConversationRemoteID: remote, Pinned: &pinned},
		{Kind: connectortest.EventMessage, ConversationRemoteID: remote, MessageRemoteID: "M2", Text: "there", Live: true},
		{Kind: connectortest.EventRead, ConversationRemoteID: remote},
	}
}

// TestConformance_ScenarioReorder runs the shared Reorder check against
// WhatsApp.
func TestConformance_ScenarioReorder(t *testing.T) {
	t.Parallel()

	connectortest.CheckReorder(t, newWhatsappDriver, scenarioEvents())
}

// TestConformance_ScenarioDuplicateDelivery runs the shared
// DuplicateDelivery check against WhatsApp.
func TestConformance_ScenarioDuplicateDelivery(t *testing.T) {
	t.Parallel()

	connectortest.CheckDuplicateDelivery(t, newWhatsappDriver, scenarioEvents())
}

// TestConformance_ScenarioRestart runs the shared Restart check against
// WhatsApp: MarkRead, a pin and a reply must all still work from a
// second connector instance that never saw the seed events live, using
// nothing but the media store's on-disk message_keys and the fake
// device's own paired state.
func TestConformance_ScenarioRestart(t *testing.T) {
	t.Parallel()

	remote := remoteID(types.NewJID("15551234567", types.DefaultUserServer))

	connectortest.CheckRestart(t, newWhatsappDriver, scenarioEvents(),
		func(t *testing.T, d connectortest.Driver, c connector.Connector, sink *connectortest.Sink) {
			t.Helper()

			dev, ok := d.Backend().(*fakeDevice)
			if !ok {
				t.Fatalf("backend = %T, want *fakeDevice", d.Backend())
			}

			if err := c.MarkRead(t.Context(), domain.Conversation{RemoteID: remote, Unread: 2}); err != nil {
				t.Errorf("MarkRead after restart = %v", err)
			}
			if len(dev.markReadCalls) == 0 {
				t.Error("MarkRead after restart sent no receipt, want the pre-restart message keys still found")
			}

			organizer, ok := c.(connector.Organizer)
			if !ok {
				t.Fatal("the WhatsApp connector does not implement connector.Organizer")
			}
			if err := organizer.SetPinned(t.Context(), domain.Conversation{RemoteID: remote}, false); err != nil {
				t.Errorf("SetPinned after restart = %v", err)
			}
			if len(dev.appStatePatches) == 0 {
				t.Error("SetPinned after restart sent no app-state patch")
			}

			reply := domain.Message{ID: "reply-1", Text: "still here", ReplyTo: &domain.Reply{RemoteID: "M2"}}
			if err := c.Send(t.Context(), domain.Conversation{RemoteID: remote, Kind: domain.KindDirect}, reply); err != nil {
				t.Errorf("Send a reply after restart = %v", err)
			}
			if len(dev.sent) == 0 {
				t.Error("the reply after restart never reached the fake device")
			}
		})
}

// TestScenario_ExpiredMediaSurvivesRestart reproduces fetching a
// message's media after a restart, when its CDN link has aged out: the
// retry-through-the-primary-phone flow (see retry.go) must still work
// from a second connector instance, using only the media reference and
// message key the first instance saved to the on-disk media store
// before it stopped.
func TestScenario_ExpiredMediaSurvivesRestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		dev.paired = true
		media := newTestMediaStore(t)
		key := testMediaKey(9)

		c1 := newConnectorOverDevAndMedia(dev, media)
		c1.connected(dev, &connectortest.Sink{}, media)
		seedRetryMessage(t, media, "msg-1", key, "/old")
		c1.disconnected()

		c2 := newConnectorOverDevAndMedia(dev, media)
		var sink connectortest.Sink
		c2.connected(dev, &sink, media)
		unregister := c2.handleEvents(t.Context(), dev, media, &sink)
		defer unregister()

		dev.downloadErrPaths = map[string]error{"/old": whatsmeow.ErrMediaDownloadFailedWith410}
		dev.downloadData = []byte("fresh bytes")

		path := filepath.Join(t.TempDir(), "out")
		done := make(chan error, 1)
		go func() { done <- c2.FetchMedia(t.Context(), directChat, "msg-1", path) }()
		synctest.Wait()

		if len(dev.mediaRetryCalls) != 1 {
			t.Fatalf("mediaRetryCalls after restart = %d, want exactly one retry receipt sent", len(dev.mediaRetryCalls))
		}

		dev.fireEvent(mediaRetryEvent(t, key, "msg-1", &waMmsRetry.MediaRetryNotification{
			Result: waMmsRetry.MediaRetryNotification_SUCCESS.Enum(), DirectPath: strPtr("/new"),
		}))
		synctest.Wait()

		if err := <-done; err != nil {
			t.Fatalf("FetchMedia after restart = %v, want it to succeed once the phone answers", err)
		}

		got, err := os.ReadFile(path)
		if err != nil || string(got) != "fresh bytes" {
			t.Errorf("downloaded %q, %v, want the bytes fetched with the new path", got, err)
		}
	})
}

// TestScenario_LoadOlderHistorySurvivesRestart reproduces scrolling back
// past what history sync already delivered, right after a restart: the
// anchor LoadOlder needs is message_keys' own row (see keys.go), saved
// to the on-disk media store by the first connector instance, so a
// second instance that starts with every in-memory field empty must
// still be able to ask the phone for history before it and resolve the
// answer, the same as retry.go's media-retry flow already does.
func TestScenario_LoadOlderHistorySurvivesRestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		dev.paired = true
		media := newTestMediaStore(t)

		c1 := newConnectorOverDevAndMedia(dev, media)
		c1.connected(dev, &connectortest.Sink{}, media)
		if err := media.putMessageKey(t.Context(), directChat.RemoteID, "m1", messageKey{timestamp: 1000}); err != nil {
			t.Fatal(err)
		}
		c1.disconnected()

		c2 := newConnectorOverDevAndMedia(dev, media)
		var sink connectortest.Sink
		c2.connected(dev, &sink, media)
		unregister := c2.handleEvents(t.Context(), dev, media, &sink)
		defer unregister()

		done := make(chan int, 1)
		go func() {
			n, err := c2.LoadOlder(t.Context(), directChat, "m1", 50)
			if err != nil {
				t.Error(err)
			}
			done <- n
		}()
		synctest.Wait()

		if len(dev.requestHistoryCalls) != 1 {
			t.Fatalf("requestHistoryCalls after restart = %d, want exactly one request", len(dev.requestHistoryCalls))
		}

		dev.fireEvent(onDemandSync(directPeer.String(), historyMsg("m0", "earlier", false)))
		synctest.Wait()

		if n := <-done; n != 1 {
			t.Errorf("LoadOlder after restart = %d, want 1", n)
		}
		if !sink.Has("history " + directChat.RemoteID + " m0") {
			t.Errorf("events = %q, want the backfilled message stored as history", sink.Lines())
		}
	})
}

// newConnectorOverDevAndMedia returns a fresh connector instance over
// dev and media, with every in-memory field starting empty, standing in
// for the connector a restart would build.
func newConnectorOverDevAndMedia(dev device, media *mediaStore) *Connector {
	return &Connector{
		account:   domain.Account{ID: "wa-1", Service: domain.ServiceWhatsApp},
		answers:   make(chan answer, 1),
		open:      func(context.Context) (device, error) { return dev, nil },
		openMedia: func(context.Context) (*mediaStore, error) { return media, nil },
		organize:  map[string]organizeState{},
		names:     map[string]namedEntry{},
		reactions: map[string]map[string]string{},
	}
}

// TestScenario_UndecryptableThenResent reproduces a message whatsmeow
// could not decrypt, stored as a placeholder, followed by the primary
// phone resending the same id once it can: the resend must replace the
// placeholder in place, never show up as a second, separate message.
func TestScenario_UndecryptableThenResent(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	media := newTestMediaStore(t)
	var sink connectortest.Sink
	c := newConnectorWithMedia(dev, &sink, media)

	info := types.MessageInfo{
		MessageSource: types.MessageSource{Chat: directPeer, Sender: directPeer},
		ID:            "M1", PushName: "Nadia", Timestamp: time.Unix(1, 0),
	}
	c.handleUndecryptable(t.Context(), &sink, dev, media, &events.UndecryptableMessage{Info: info})

	live := sink.LiveMessages()[directChat.RemoteID]
	if len(live) != 1 || live[0].Text != undecryptablePlaceholder {
		t.Fatalf("after the first delivery, live = %+v, want one placeholder", live)
	}

	c.handleMessage(t.Context(), &sink, dev, media, &events.Message{
		Info: info, Message: &waE2E.Message{Conversation: strPtr("the real text")}, UnavailableRequestID: "req-1",
	})

	if !sink.Has("edited " + directChat.RemoteID + " M1") {
		t.Errorf("events = %q, want the resend reported as editing the placeholder in place", sink.Lines())
	}

	// The placeholder itself legitimately arrived as a live message, so
	// it can show at once; Edited never appends to that list, and the
	// resend must not add a second entry to it.
	live = sink.LiveMessages()[directChat.RemoteID]
	if len(live) != 1 {
		t.Errorf("live messages after the resend = %d, want still 1: the resend replaces the placeholder in place rather than arriving as a second message", len(live))
	}
}

// TestScenario_IdentityAliases reproduces the same contact addressed
// sometimes by phone JID, sometimes by WhatsApp's hidden id (LID): a
// message, a pin sent by LID and a delete must all land on the one
// conversation stored under the phone JID, never split it into two.
func TestScenario_IdentityAliases(t *testing.T) {
	t.Parallel()

	phone := types.NewJID("15551234567", types.DefaultUserServer)
	lid := types.NewJID("987654", types.HiddenUserServer)
	dev := newFakeDevice()
	dev.lidPhones = map[string]types.JID{lid.String(): phone}
	dev.contactNames = map[string]string{phone.String(): "Nadia"}
	media := newTestMediaStore(t)
	var sink connectortest.Sink
	c := newConnectorWithMedia(dev, &sink, media)

	c.handleMessage(t.Context(), &sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: phone, Sender: phone}, ID: "M1", Timestamp: time.Unix(1, 0)},
		Message: &waE2E.Message{Conversation: strPtr("hi")},
	})
	c.handlePin(t.Context(), &sink, dev, &events.Pin{JID: lid, Action: &waSyncAction.PinAction{Pinned: boolPtr(true)}})
	c.handleMessage(t.Context(), &sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: lid, Sender: lid}, ID: "M2", Timestamp: time.Unix(2, 0)},
		Message: &waE2E.Message{Conversation: strPtr("there")},
	})
	c.handleDeleteForMe(t.Context(), &sink, dev, &events.DeleteForMe{ChatJID: lid, MessageID: "M1"})

	snap := sink.Snapshot()
	phoneRemote, lidRemote := remoteID(phone), remoteID(lid)

	if _, ok := snap[lidRemote]; ok {
		t.Errorf("a separate conversation was created for the LID: %+v", snap)
	}

	got, ok := snap[phoneRemote]
	if !ok {
		t.Fatalf("no conversation for the phone JID: %+v", snap)
	}
	if got.Title != "Nadia" {
		t.Errorf("title = %q, want Nadia regardless of which id addressed the chat", got.Title)
	}
	if !got.Pinned {
		t.Error("the pin sent by LID did not apply to the phone-keyed conversation")
	}
	if slices.Contains(got.MessageRemoteIDs, "M1") {
		t.Errorf("message ids = %v, want M1 deleted", got.MessageRemoteIDs)
	}
	if !slices.Contains(got.MessageRemoteIDs, "M2") {
		t.Errorf("message ids = %v, want M2, delivered by LID, kept", got.MessageRemoteIDs)
	}
}

// TestScenario_IdentityAliases_SelfChat reproduces the self-chat
// addressed by phone JID for a note sent from the phone and by LID for
// a reply from a bot running as another linked device: both must land
// in the one self-chat conversation.
func TestScenario_IdentityAliases_SelfChat(t *testing.T) {
	t.Parallel()

	phone := types.NewJID("15551234567", types.DefaultUserServer)
	lid := types.NewJID("111222", types.HiddenUserServer)
	dev := newFakeDevice()
	dev.selfJID, dev.selfLID = phone, lid
	media := newTestMediaStore(t)
	var sink connectortest.Sink
	c := newConnectorWithMedia(dev, &sink, media)

	c.handleMessage(t.Context(), &sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: phone, Sender: phone, IsFromMe: true}, ID: "M1", Timestamp: time.Unix(1, 0)},
		Message: &waE2E.Message{Conversation: strPtr("note to self")},
	})
	c.handleMessage(t.Context(), &sink, dev, media, &events.Message{
		Info:    types.MessageInfo{MessageSource: types.MessageSource{Chat: lid, Sender: lid, IsFromMe: true}, ID: "M2", Timestamp: time.Unix(2, 0)},
		Message: &waE2E.Message{Conversation: strPtr("bot reply")},
	})

	snap := sink.Snapshot()
	if _, ok := snap[remoteID(lid)]; ok {
		t.Errorf("the self-chat split into a second conversation keyed by LID: %+v", snap)
	}

	got, ok := snap[remoteID(phone)]
	if !ok || !slices.Contains(got.MessageRemoteIDs, "M1") || !slices.Contains(got.MessageRemoteIDs, "M2") {
		t.Errorf("self-chat snapshot = %+v, %v, want both messages in the one phone-keyed conversation", got, ok)
	}
}
