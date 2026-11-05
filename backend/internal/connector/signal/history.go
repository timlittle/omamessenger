package signal

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/backuppb"
	sstore "go.mau.fi/mautrix-signal/pkg/signalmeow/store"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var _ connector.HistoryLoader = (*Connector)(nil)

// historyPageSize is how many of a chat's newest messages importHistory
// reports right after linking; LoadOlder pages further back from there
// as the user scrolls.
const historyPageSize = 50

// importHistory waits for the phone to offer this fresh link's history
// transfer and imports it. Best effort: no answer leaves no history.
func (c *Connector) importHistory(ctx context.Context, dev device, sink connector.Sink) {
	meta, err := dev.waitForTransfer(ctx)
	if err != nil {
		return
	}

	if err := dev.fetchAndProcessTransfer(ctx, meta); err != nil {
		return
	}

	chats, err := dev.backupChats(ctx)
	if err != nil {
		return
	}

	for _, chat := range chats {
		c.importChat(ctx, dev, sink, chat)
	}
}

// importChat reports one imported chat's conversation state and newest
// page of messages.
func (c *Connector) importChat(ctx context.Context, dev device, sink connector.Sink, chat *backupChat) {
	remoteID, kind, ok := c.resolveChatIdentity(ctx, dev, chat.RecipientId)
	if !ok {
		return
	}

	c.rememberChatID(remoteID, chat.Id)

	conv := domain.Conversation{
		AccountID: c.account.ID, Service: domain.ServiceSignal,
		RemoteID: remoteID, Kind: kind,
		Pinned: chat.PinnedOrder != nil, Archived: chat.Archived,
	}
	if conv.Pinned {
		conv.PinOrder = int(chat.GetPinnedOrder())
	}
	if until := chat.MuteUntilMs; until != nil {
		conv.Muted = *until > uint64(time.Now().UnixMilli())
	}
	sink.Conversation(ctx, conv)

	if ms := chat.ExpirationTimerMs; ms != nil {
		reportDisappearingTimer(ctx, sink, c.account.ID, remoteID, int64(*ms/1000))
	}

	items, err := dev.backupChatItems(ctx, chat.Id, time.Now(), false, historyPageSize)
	if err != nil {
		return
	}

	for _, item := range items {
		if m, ok := c.normalizeBackupItem(ctx, dev, item); ok {
			sink.History(ctx, c.account.ID, remoteID, m)
		}
	}
}

// backupChat names signalmeow's own BackupChat locally, so history.go
// does not repeat its store package's import alias at every call site.
type backupChat = sstore.BackupChat

// LoadOlder reports up to limit messages older than beforeRemoteID in
// conv, paging the transferred history archive; never Signal's servers.
func (c *Connector) LoadOlder(ctx context.Context, conv domain.Conversation, beforeRemoteID string, limit int) (int, error) {
	dev, sink, err := c.session()
	if err != nil {
		return 0, fmt.Errorf("signal: load older: %w", connector.ErrHistoryUnavailable)
	}

	chatID, ok := c.chatIDFor(conv.RemoteID)
	if !ok {
		return 0, nil
	}

	anchor := anchorFor(beforeRemoteID)

	items, err := dev.backupChatItems(ctx, chatID, anchor, false, limit)
	if err != nil {
		return 0, fmt.Errorf("signal: load older: %w", connector.ErrHistoryUnavailable)
	}

	found := 0
	for _, item := range items {
		if m, ok := c.normalizeBackupItem(ctx, dev, item); ok {
			sink.History(ctx, c.account.ID, conv.RemoteID, m)
			found++
		}
	}

	return found, nil
}

// anchorFor turns a message remote id (see messageRemoteID) into the
// time GetBackupChatItems pages older messages from, or now when
// beforeRemoteID is "", the newest point.
func anchorFor(beforeRemoteID string) time.Time {
	_, timestamp, ok := strings.Cut(beforeRemoteID, ":")
	if !ok {
		return time.Now()
	}

	ms, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return time.Now()
	}

	return time.UnixMilli(ms)
}

// rememberChatID records remoteID's transferred-history chat id, so a
// later LoadOlder for the same conversation can page it.
func (c *Connector) rememberChatID(remoteID string, chatID uint64) {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()

	if c.chatIDs == nil {
		c.chatIDs = map[string]uint64{}
	}
	c.chatIDs[remoteID] = chatID
}

// chatIDFor returns remoteID's transferred-history chat id, or found
// false when this run never imported one for it.
func (c *Connector) chatIDFor(remoteID string) (uint64, bool) {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()

	id, ok := c.chatIDs[remoteID]

	return id, ok
}

// resolveChatIdentity resolves a backup chat's recipient to the same
// remote id and kind a live message for it would carry.
func (c *Connector) resolveChatIdentity(ctx context.Context, dev device, recipientID uint64) (remoteID, kind string, ok bool) {
	recipient, err := dev.backupRecipient(ctx, recipientID)
	if err != nil || recipient == nil {
		return "", "", false
	}

	switch dest := recipient.Destination.(type) {
	case *backuppb.Recipient_Contact:
		aci, ok := uuid.FromBytes(dest.Contact.GetAci())
		if ok != nil || aci == uuid.Nil {
			return "", "", false
		}

		return senderRemoteID(aci), domain.KindDirect, true
	case *backuppb.Recipient_Self:
		return senderRemoteID(c.selfID()), domain.KindDirect, true
	case *backuppb.Recipient_Group:
		// SerializedGroupMasterKey is base64 text, not raw bytes cast to
		// a string (signalmeow's own masterKeyToBytes decodes it that
		// way); the backup protobuf's masterKey field is raw bytes.
		masterKey := base64.StdEncoding.EncodeToString(dest.Group.GetMasterKey())
		gid, err := dev.groupIdentifierForMasterKey(ctx, types.SerializedGroupMasterKey(masterKey))
		if err != nil {
			return "", "", false
		}

		return string(gid), domain.KindGroup, true
	default:
		return "", "", false
	}
}

// normalizeBackupItem maps one transferred message to a domain
// message, or ok false for an item kind not imported yet.
func (c *Connector) normalizeBackupItem(ctx context.Context, dev device, item *backuppb.ChatItem) (domain.Message, bool) {
	standard, ok := item.GetItem().(*backuppb.ChatItem_StandardMessage)
	if !ok || standard.StandardMessage == nil {
		return domain.Message{}, false
	}

	author, err := dev.backupRecipient(ctx, item.GetAuthorId())
	if err != nil || author == nil {
		return domain.Message{}, false
	}

	aci, outgoing, ok := backupAuthorACI(author, c.selfID())
	if !ok {
		return domain.Message{}, false
	}

	m := domain.Message{
		RemoteID:   messageRemoteID(aci, item.GetDateSent()),
		SenderID:   senderRemoteID(aci),
		SenderName: c.resolveSenderName(ctx, aci),
		Text:       standard.StandardMessage.GetText().GetBody(),
		Outgoing:   outgoing,
		Created:    int64(item.GetDateSent()),
	}
	if outgoing {
		m.Status = domain.StatusSent
	} else {
		m.Status = domain.StatusReceived
	}
	if ms := item.ExpiresInMs; ms != nil {
		m.ExpireAfter = int64(*ms / 1000)
		m.ArmOnRead = !outgoing
	}

	return m, true
}

// backupAuthorACI resolves a backup item's author recipient to who
// sent it, and whether that was this account itself.
func backupAuthorACI(author *backuppb.Recipient, self uuid.UUID) (uuid.UUID, bool, bool) {
	switch dest := author.Destination.(type) {
	case *backuppb.Recipient_Contact:
		aci, err := uuid.FromBytes(dest.Contact.GetAci())
		if err != nil || aci == uuid.Nil {
			return uuid.Nil, false, false
		}

		return aci, false, true
	case *backuppb.Recipient_Self:
		return self, true, true
	default:
		return uuid.Nil, false, false
	}
}
