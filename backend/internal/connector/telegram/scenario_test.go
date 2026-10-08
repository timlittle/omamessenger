package telegram

// scenario_test.go adopts the shared Restart, Reorder and
// DuplicateDelivery checks from connectortest. Telegram's own update
// dispatcher never reports a chat pinned or archived on its own (see
// sync.go): that state, like every conversation's existence, only ever
// arrives through a full dialog resync, which always carries both
// together, so there is no ordering question to script for it the way
// WhatsApp's live pin and archive events raise one. The events this
// scenario reorders are the ones Telegram's live updates genuinely can
// deliver independently: a conversation becoming known, live messages,
// a read receipt and a delete.

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// telegramDialog is the server-side state of one conversation the
// scenario has registered, so a resync can always rebuild the fake's
// dialog list from what is currently known rather than one event's own
// fields.
type telegramDialog struct {
	peer             tg.PeerClass
	user             *tg.User
	chat             *tg.Chat
	pinned, archived bool
	unread           int
}

// telegramDriver runs the shared scenarios against one fakeTelegram, so
// Restart can hand out a second connector instance over the same fake,
// the way a helper restart reconnects with the same server-side state.
type telegramDriver struct {
	fake    *fakeTelegram
	dialogs map[string]*telegramDialog
	order   []string

	current *Connector
	sink    connector.Sink
}

// newTelegramDriver returns a driver over a fresh fake with nothing
// synced yet.
func newTelegramDriver(t *testing.T) connectortest.Driver {
	t.Helper()

	return &telegramDriver{fake: newFakeTelegram(), dialogs: map[string]*telegramDialog{}}
}

// Connector returns a new connector instance over this driver's fake,
// with every in-memory field starting empty, as a process restart
// would see it.
func (d *telegramDriver) Connector(t *testing.T) connector.Connector {
	t.Helper()

	return New(domain.Account{ID: "tg-1", Service: domain.ServiceTelegram}, "")
}

// Run wires c to this driver's fake as if Run had already signed it in,
// remembers both for Deliver, and returns a stop func that disconnects
// it.
func (d *telegramDriver) Run(t *testing.T, c connector.Connector, sink connector.Sink) func() {
	t.Helper()

	tc, ok := c.(*Connector)
	if !ok {
		t.Fatalf("connector = %T, want *Connector", c)
	}

	tc.connected(tg.NewClient(d.fake), sink)
	d.current, d.sink = tc, sink

	return func() { tc.disconnected() }
}

// Backend returns the fake Telegram API Deliver and Run acted on.
func (d *telegramDriver) Backend() any { return d.fake }

// Deliver turns one scripted event into a resync of the fake's current
// dialog state, a direct call to a live update handler, or a delete,
// whichever is the real mechanism Telegram would use to report it.
func (d *telegramDriver) Deliver(t *testing.T, e connectortest.Event) {
	t.Helper()

	ctx := t.Context()

	switch e.Kind {
	case connectortest.EventConversation:
		d.ensureDialog(e.ConversationRemoteID, e.Title)
		d.resync(t, ctx)
	case connectortest.EventMessage:
		d.deliverMessage(t, ctx, e)
	case connectortest.EventOrganize:
		if dl := d.dialogs[e.ConversationRemoteID]; dl != nil {
			if e.Pinned != nil {
				dl.pinned = *e.Pinned
			}
			if e.Archived != nil {
				dl.archived = *e.Archived
			}
			d.resync(t, ctx)
		}
	case connectortest.EventRead:
		d.current.unread(ctx, d.sink, peerKey(e.ConversationRemoteID), e.Unread)
	case connectortest.EventDelete:
		d.deliverDelete(ctx, e)
	}
}

// ensureDialog registers remote as a known conversation with title, or
// updates its title if already registered, keeping whatever pinned,
// archived or unread state an earlier event already set for it.
func (d *telegramDriver) ensureDialog(remote, title string) {
	peer, user, chat := telegramPeer(remote, title)

	dl, ok := d.dialogs[remote]
	if !ok {
		dl = &telegramDialog{}
		d.dialogs[remote] = dl
		d.order = append(d.order, remote)
	}
	dl.peer, dl.user, dl.chat = peer, user, chat
}

// telegramPeer builds the peer, and the user or chat entity naming it
// title, for a remote id of the form "user:<id>:<hash>" or "chat:<id>".
func telegramPeer(remote, title string) (tg.PeerClass, *tg.User, *tg.Chat) {
	parts := strings.Split(remote, ":")

	switch parts[0] {
	case "user":
		id, _ := strconv.ParseInt(parts[1], 10, 64)
		hash, _ := strconv.ParseInt(parts[2], 10, 64)

		return &tg.PeerUser{UserID: id}, &tg.User{ID: id, AccessHash: hash, FirstName: title}, nil
	case "chat":
		id, _ := strconv.ParseInt(parts[1], 10, 64)

		return &tg.PeerChat{ChatID: id}, nil, &tg.Chat{ID: id, Title: title, Photo: &tg.ChatPhotoEmpty{}}
	default:
		return nil, nil, nil
	}
}

// resync rebuilds the fake's contacts, dialogs and history replies from
// every dialog this driver has registered, then runs a full sync, the
// same resync Run performs on every connect.
func (d *telegramDriver) resync(t *testing.T, ctx context.Context) {
	t.Helper()

	var dialogs []tg.DialogClass
	var users []tg.UserClass
	var chats []tg.ChatClass

	for _, remote := range d.order {
		dl := d.dialogs[remote]
		folder := 0
		if dl.archived {
			folder = archiveFolderID
		}
		dialogs = append(dialogs, &tg.Dialog{Peer: dl.peer, Pinned: dl.pinned, FolderID: folder, UnreadCount: dl.unread})
		if dl.user != nil {
			users = append(users, dl.user)
		}
		if dl.chat != nil {
			chats = append(chats, dl.chat)
		}
	}

	d.fake.reply(&tg.ContactsGetContactsRequest{}, &tg.ContactsContactsNotModified{})
	d.fake.reply(&tg.MessagesGetDialogsRequest{}, &tg.MessagesDialogs{Dialogs: dialogs, Users: users, Chats: chats})
	d.fake.reply(&tg.MessagesGetHistoryRequest{}, &tg.MessagesMessagesNotModified{})

	if err := d.current.sync(ctx, tg.NewClient(d.fake), d.sink); err != nil {
		t.Fatalf("resync: %v", err)
	}
}

// deliverMessage reports e as a live message, with the entities naming
// its sender: a real Telegram update always carries these, which is
// what lets a message introduce a brand-new conversation on its own
// (see connector.go's newMessage), not only one a resync already
// listed.
func (d *telegramDriver) deliverMessage(t *testing.T, ctx context.Context, e connectortest.Event) {
	t.Helper()

	dl, ok := d.dialogs[e.ConversationRemoteID]
	if !ok {
		d.ensureDialog(e.ConversationRemoteID, "")
		dl = d.dialogs[e.ConversationRemoteID]
	}

	id, err := strconv.Atoi(e.MessageRemoteID)
	if err != nil {
		t.Fatalf("message remote id %q: %v", e.MessageRemoteID, err)
	}

	msg := &tg.Message{ID: id, PeerID: dl.peer, Message: e.Text, Date: int(time.Now().Unix())}
	entities := tg.Entities{}
	if dl.user != nil {
		entities.Users = map[int64]*tg.User{dl.user.ID: dl.user}
	}
	if dl.chat != nil {
		entities.Chats = map[int64]*tg.Chat{dl.chat.ID: dl.chat}
	}
	d.current.newMessage(ctx, d.sink, msg, entities)
}

// deliverDelete reports e's message ids deleted, scoped the way
// Telegram itself scopes a delete: to the one channel named, or to
// every non-channel conversation this connector knows, since a user or
// basic group chat shares one message id space per account.
func (d *telegramDriver) deliverDelete(ctx context.Context, e connectortest.Event) {
	ids := make([]int, 0, len(e.DeleteRemoteIDs))
	for _, s := range e.DeleteRemoteIDs {
		if n, err := strconv.Atoi(s); err == nil {
			ids = append(ids, n)
		}
	}

	if strings.HasPrefix(e.ConversationRemoteID, "channel:") {
		d.current.deleteMessages(ctx, d.sink, []string{e.ConversationRemoteID}, ids)
		return
	}

	d.current.deleteMessages(ctx, d.sink, d.current.nonChannelRemotes(), ids)
}

// reorderableEvents is one direct chat becoming known, two live
// messages and a read receipt: every one of these can plausibly arrive
// in any order relative to the others, the way Telegram's own live
// dispatcher reports them, which is what Reorder needs of the events
// it permutes. A delete is deliberately left out of this set: deleting
// a message before it has ever arrived is not an ordering any real
// connector's service can produce, only an artefact Reorder's
// permutations would otherwise manufacture.
func reorderableEvents() []connectortest.Event {
	const remote = "user:42:99"

	return []connectortest.Event{
		{Kind: connectortest.EventConversation, ConversationRemoteID: remote, Title: "Nadia", ConversationKind: domain.KindDirect},
		{Kind: connectortest.EventMessage, ConversationRemoteID: remote, MessageRemoteID: "101", Text: "hi", Live: true},
		{Kind: connectortest.EventMessage, ConversationRemoteID: remote, MessageRemoteID: "102", Text: "there", Live: true},
		{Kind: connectortest.EventRead, ConversationRemoteID: remote, Unread: 0},
	}
}

// scenarioEvents is reorderableEvents with a delete of the first
// message appended in its one realistic position, after the message it
// removes: the full, realistically ordered scenario Restart seeds and
// DuplicateDelivery replays.
func scenarioEvents() []connectortest.Event {
	const remote = "user:42:99"

	return append(reorderableEvents(),
		connectortest.Event{Kind: connectortest.EventDelete, ConversationRemoteID: remote, DeleteRemoteIDs: []string{"101"}})
}

// TestConformance_ScenarioReorder runs the shared Reorder check against
// Telegram.
func TestConformance_ScenarioReorder(t *testing.T) {
	t.Parallel()

	connectortest.CheckReorder(t, newTelegramDriver, reorderableEvents())
}

// TestConformance_ScenarioDuplicateDelivery runs the shared
// DuplicateDelivery check against Telegram.
func TestConformance_ScenarioDuplicateDelivery(t *testing.T) {
	t.Parallel()

	connectortest.CheckDuplicateDelivery(t, newTelegramDriver, scenarioEvents())
}

// TestConformance_ScenarioRestart runs the shared Restart check against
// Telegram: MarkRead, a pin and a reply must all still work from a
// second connector instance that never saw the seed events live, since
// Telegram's own remote ids carry their access hash and so need
// nothing cached in memory to address a conversation again.
func TestConformance_ScenarioRestart(t *testing.T) {
	t.Parallel()

	connectortest.CheckRestart(t, newTelegramDriver, scenarioEvents(),
		func(t *testing.T, d connectortest.Driver, c connector.Connector, sink *connectortest.Sink) {
			t.Helper()

			fake, ok := d.Backend().(*fakeTelegram)
			if !ok {
				t.Fatalf("backend = %T, want *fakeTelegram", d.Backend())
			}

			fake.reply(&tg.MessagesReadHistoryRequest{}, &tg.MessagesAffectedMessages{})
			if err := c.MarkRead(t.Context(), domain.Conversation{RemoteID: "user:42:99"}); err != nil {
				t.Errorf("MarkRead after restart = %v", err)
			}

			organizer, ok := c.(connector.Organizer)
			if !ok {
				t.Fatal("the Telegram connector does not implement connector.Organizer")
			}
			fake.reply(&tg.MessagesToggleDialogPinRequest{}, &tg.BoolTrue{})
			if err := organizer.SetPinned(t.Context(), domain.Conversation{RemoteID: "user:42:99"}, false); err != nil {
				t.Errorf("SetPinned after restart = %v", err)
			}

			fake.reply(&tg.MessagesSendMessageRequest{}, &tg.UpdateShortSentMessage{ID: 200})
			reply := domain.Message{ID: "reply-1", Text: "still here", ReplyTo: &domain.Reply{RemoteID: "102"}}
			if err := c.Send(t.Context(), domain.Conversation{RemoteID: "user:42:99", Kind: domain.KindDirect}, reply); err != nil {
				t.Errorf("Send a reply after restart = %v", err)
			}

			sent := fake.sent()
			if len(sent) < 3 {
				t.Fatalf("requests sent = %d, want at least the 3 sent after restart", len(sent))
			}
			last := sent[len(sent)-3:]
			if _, ok := last[0].(*tg.MessagesReadHistoryRequest); !ok {
				t.Errorf("request %T, want MessagesReadHistoryRequest for MarkRead", last[0])
			}
			if _, ok := last[1].(*tg.MessagesToggleDialogPinRequest); !ok {
				t.Errorf("request %T, want MessagesToggleDialogPinRequest for SetPinned", last[1])
			}
			if _, ok := last[2].(*tg.MessagesSendMessageRequest); !ok {
				t.Errorf("request %T, want MessagesSendMessageRequest for the reply", last[2])
			}
		})
}
