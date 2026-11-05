package signal

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/backuppb"
	sstore "go.mau.fi/mautrix-signal/pkg/signalmeow/store"
	"google.golang.org/protobuf/proto"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestResolveChatIdentity_Contact checks a contact recipient resolves
// to the same remote id a live message from them would carry.
func TestResolveChatIdentity_Contact(t *testing.T) {
	t.Parallel()

	friend := uuid.New()
	friendBytes, err := friend.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice(uuid.New())
	dev.recipients = map[uint64]*backuppb.Recipient{
		1: {Destination: &backuppb.Recipient_Contact{Contact: &backuppb.Contact{Aci: friendBytes}}},
	}
	c := newTestConnector(dev)

	remoteID, kind, ok := c.resolveChatIdentity(t.Context(), dev, 1)
	if !ok || remoteID != friend.String() || kind != domain.KindDirect {
		t.Errorf("resolveChatIdentity() = %q, %q, %t", remoteID, kind, ok)
	}
}

// TestResolveChatIdentity_Self checks the backup's own "self" chat
// (this account's note-to-self) resolves to this account's own id.
func TestResolveChatIdentity_Self(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	dev := newFakeDevice(self)
	dev.recipients = map[uint64]*backuppb.Recipient{2: {Destination: &backuppb.Recipient_Self{Self: &backuppb.Self{}}}}
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{})

	remoteID, kind, ok := c.resolveChatIdentity(t.Context(), dev, 2)
	if !ok || remoteID != self.String() || kind != domain.KindDirect {
		t.Errorf("resolveChatIdentity() = %q, %q, %t", remoteID, kind, ok)
	}
}

// TestResolveChatIdentity_Group checks a group recipient resolves
// its master key to a group identifier.
func TestResolveChatIdentity_Group(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	dev.recipients = map[uint64]*backuppb.Recipient{
		3: {Destination: &backuppb.Recipient_Group{Group: &backuppb.Group{MasterKey: []byte("master-key-bytes")}}},
	}
	c := newTestConnector(dev)

	remoteID, kind, ok := c.resolveChatIdentity(t.Context(), dev, 3)
	if !ok || kind != domain.KindGroup || remoteID == "" {
		t.Errorf("resolveChatIdentity() = %q, %q, %t", remoteID, kind, ok)
	}
}

// TestResolveChatIdentity_Unresolvable checks a recipient this
// connector cannot address (a release-notes channel, say) is skipped
// rather than reported with a made-up id.
func TestResolveChatIdentity_Unresolvable(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	dev.recipients = map[uint64]*backuppb.Recipient{4: {Destination: &backuppb.Recipient_ReleaseNotes{}}}
	c := newTestConnector(dev)

	if _, _, ok := c.resolveChatIdentity(t.Context(), dev, 4); ok {
		t.Error("resolveChatIdentity() resolved a release-notes recipient")
	}

	if _, _, ok := c.resolveChatIdentity(t.Context(), dev, 999); ok {
		t.Error("resolveChatIdentity() resolved an unknown recipient id")
	}
}

// TestNormalizeBackupItem_StandardMessage checks a transferred text
// message maps to the domain message LoadOlder and importHistory
// report.
func TestNormalizeBackupItem_StandardMessage(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	senderBytes, err := sender.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice(uuid.New())
	dev.recipients = map[uint64]*backuppb.Recipient{
		1: {Destination: &backuppb.Recipient_Contact{Contact: &backuppb.Contact{Aci: senderBytes}}},
	}
	c := newTestConnector(dev)

	item := &backuppb.ChatItem{
		AuthorId: 1, DateSent: 1700000000000,
		ExpiresInMs: proto.Uint64(60000),
		Item:        &backuppb.ChatItem_StandardMessage{StandardMessage: &backuppb.StandardMessage{Text: &backuppb.Text{Body: "hello"}}},
	}

	m, ok := c.normalizeBackupItem(t.Context(), dev, item)
	if !ok {
		t.Fatal("normalizeBackupItem() reported not ok for a standard message")
	}
	if m.Text != "hello" || m.SenderID != sender.String() {
		t.Errorf("message = %+v", m)
	}
	if m.ExpireAfter != 60 || !m.ArmOnRead {
		t.Errorf("ExpireAfter = %d, ArmOnRead = %t", m.ExpireAfter, m.ArmOnRead)
	}
}

// TestNormalizeBackupItem_UnsupportedKind checks an item kind this
// connector does not import yet is skipped rather than reported as an
// empty message.
func TestNormalizeBackupItem_UnsupportedKind(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)

	item := &backuppb.ChatItem{AuthorId: 1, Item: &backuppb.ChatItem_GiftBadge{}}
	if _, ok := c.normalizeBackupItem(t.Context(), dev, item); ok {
		t.Error("normalizeBackupItem() imported a gift badge item")
	}
}

// TestBackupAuthorACI_Unresolvable checks that an author recipient
// this connector cannot address (such as a group) is reported not ok
// rather than attributed to the zero account.
func TestBackupAuthorACI_Unresolvable(t *testing.T) {
	t.Parallel()

	author := &backuppb.Recipient{Destination: &backuppb.Recipient_Group{Group: &backuppb.Group{}}}
	if _, _, ok := backupAuthorACI(author, uuid.New()); ok {
		t.Error("backupAuthorACI() resolved a group as a message author")
	}
}

// TestImportHistory_ReportsChatsAndMessages checks that importing a
// transfer with one direct chat reports its conversation state and
// its messages, and remembers the chat id for later paging.
func TestImportHistory_ReportsChatsAndMessages(t *testing.T) {
	t.Parallel()

	friend := uuid.New()
	friendBytes, err := friend.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice(uuid.New())
	dev.transferMeta = &signalmeow.TransferArchiveMetadata{CDN: 3, Key: "k"}
	dev.recipients = map[uint64]*backuppb.Recipient{
		1: {Destination: &backuppb.Recipient_Contact{Contact: &backuppb.Contact{Aci: friendBytes}}},
	}
	pinnedOrder := uint32(1)
	dev.chats = []*sstore.BackupChat{
		{Chat: &backuppb.Chat{Id: 10, RecipientId: 1, PinnedOrder: &pinnedOrder, Archived: false}},
	}
	dev.itemsByChat = map[uint64][]*backuppb.ChatItem{
		10: {{AuthorId: 1, DateSent: 1, Item: &backuppb.ChatItem_StandardMessage{StandardMessage: &backuppb.StandardMessage{Text: &backuppb.Text{Body: "hi"}}}}},
	}

	c := newTestConnector(dev)
	sink := &connectortest.Sink{}

	c.importHistory(t.Context(), dev, sink)

	conv, ok := sink.ConversationFor(friend.String())
	if !ok || !conv.Pinned || conv.PinOrder != 1 {
		t.Errorf("conversation = %+v, ok=%t", conv, ok)
	}

	history := sink.Messages()[friend.String()]
	if len(history) != 1 || history[0].Text != "hi" {
		t.Errorf("history = %+v", history)
	}

	if _, ok := c.chatIDFor(friend.String()); !ok {
		t.Error("chat id was not remembered for later paging")
	}
}

// TestImportHistory_FetchFails checks that a transfer offered but
// failing to download leaves no history imported, without panicking.
func TestImportHistory_FetchFails(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	dev.transferMeta = &signalmeow.TransferArchiveMetadata{CDN: 3, Key: "k"}
	dev.fetchErr = errors.New("download failed")
	c := newTestConnector(dev)
	sink := &connectortest.Sink{}

	c.importHistory(t.Context(), dev, sink)

	if len(sink.Lines()) != 0 {
		t.Errorf("importHistory reported something despite the fetch failing: %v", sink.Lines())
	}
}

// TestImportHistory_ListFails checks the same for a transfer that
// imports but whose chat list cannot be read back.
func TestImportHistory_ListFails(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	dev.transferMeta = &signalmeow.TransferArchiveMetadata{CDN: 3, Key: "k"}
	dev.backupChatsErr = errors.New("corrupt store")
	c := newTestConnector(dev)
	sink := &connectortest.Sink{}

	c.importHistory(t.Context(), dev, sink)

	if len(sink.Lines()) != 0 {
		t.Errorf("importHistory reported something despite listing chats failing: %v", sink.Lines())
	}
}

// TestImportChat_SkipsUnresolvableRecipient checks that a chat whose
// recipient this connector cannot resolve is skipped rather than
// reported under a made-up id.
func TestImportChat_SkipsUnresolvableRecipient(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	dev.recipients = map[uint64]*backuppb.Recipient{1: {Destination: &backuppb.Recipient_ReleaseNotes{}}}
	c := newTestConnector(dev)
	sink := &connectortest.Sink{}

	c.importChat(t.Context(), dev, sink, &sstore.BackupChat{Chat: &backuppb.Chat{Id: 10, RecipientId: 1}})

	if len(sink.Lines()) != 0 {
		t.Errorf("importChat reported an unresolvable recipient: %v", sink.Lines())
	}
}

// TestImportHistory_NoTransferOffered checks that a phone which never
// offers a transfer (or answered "Don't transfer") leaves the account
// with no history, and is not an error.
func TestImportHistory_NoTransferOffered(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	dev.transferErr = signalmeow.ErrNoEphemeralBackupKey
	c := newTestConnector(dev)
	sink := &connectortest.Sink{}

	c.importHistory(t.Context(), dev, sink)

	if len(sink.Lines()) != 0 {
		t.Errorf("importHistory reported something with no transfer: %v", sink.Lines())
	}
}

// TestLoadOlder_PagesTransferredHistory checks that LoadOlder reports
// older messages for a chat already imported, anchored at the given
// message's own timestamp.
func TestLoadOlder_PagesTransferredHistory(t *testing.T) {
	t.Parallel()

	friend := uuid.New()
	friendBytes, err := friend.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	dev := newFakeDevice(uuid.New())
	dev.recipients = map[uint64]*backuppb.Recipient{
		1: {Destination: &backuppb.Recipient_Contact{Contact: &backuppb.Contact{Aci: friendBytes}}},
	}
	dev.itemsByChat = map[uint64][]*backuppb.ChatItem{
		10: {{AuthorId: 1, DateSent: 1, Item: &backuppb.ChatItem_StandardMessage{StandardMessage: &backuppb.StandardMessage{Text: &backuppb.Text{Body: "older"}}}}},
	}

	c := newTestConnector(dev)
	sink := &connectortest.Sink{}
	c.connected(dev, sink)
	c.rememberChatID(friend.String(), 10)

	conv := domain.Conversation{RemoteID: friend.String()}
	n, err := c.LoadOlder(t.Context(), conv, messageRemoteID(friend, 2), 10)
	if err != nil {
		t.Fatalf("LoadOlder() error = %v", err)
	}
	if n != 1 {
		t.Errorf("LoadOlder() found = %d, want 1", n)
	}

	history := sink.Messages()[friend.String()]
	if len(history) != 1 || history[0].Text != "older" {
		t.Errorf("history = %+v", history)
	}
}

// TestLoadOlder_UnknownConversation checks that a conversation this
// run never imported history for reports zero with no error, the same
// as a service reporting nothing older left.
func TestLoadOlder_UnknownConversation(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{})

	n, err := c.LoadOlder(t.Context(), domain.Conversation{RemoteID: "unknown"}, "", 10)
	if err != nil || n != 0 {
		t.Errorf("LoadOlder() = %d, %v, want 0, nil", n, err)
	}
}

// TestNormalizeBackupItem_UnresolvableAuthor checks that an item whose
// author recipient this connector cannot address is skipped.
func TestNormalizeBackupItem_UnresolvableAuthor(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	dev.recipients = map[uint64]*backuppb.Recipient{1: {Destination: &backuppb.Recipient_ReleaseNotes{}}}
	c := newTestConnector(dev)

	item := &backuppb.ChatItem{
		AuthorId: 1,
		Item:     &backuppb.ChatItem_StandardMessage{StandardMessage: &backuppb.StandardMessage{Text: &backuppb.Text{Body: "hi"}}},
	}
	if _, ok := c.normalizeBackupItem(t.Context(), dev, item); ok {
		t.Error("normalizeBackupItem() resolved an unaddressable author")
	}
}

// TestLoadOlder_NotConnected checks that asking for older messages
// with no run connected reports the history-unavailable error.
func TestLoadOlder_NotConnected(t *testing.T) {
	t.Parallel()

	c := &Connector{}

	if _, err := c.LoadOlder(t.Context(), domain.Conversation{RemoteID: "x"}, "", 10); err == nil {
		t.Error("LoadOlder() succeeded with no run connected, want an error")
	} else if !errors.Is(err, connector.ErrHistoryUnavailable) {
		t.Errorf("LoadOlder() error = %v, want it to wrap connector.ErrHistoryUnavailable", err)
	}
}
