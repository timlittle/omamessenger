// Package telegram connects a Telegram account through gotd/td: it signs
// in by QR code or phone, syncs dialogs and recent history, follows live
// updates, and sends messages and read receipts.
package telegram

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"

	"github.com/gotd/td/session"
	gotd "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// errNotConnected reports work asked of an account that is not signed in.
var errNotConnected = errors.New("telegram: not connected")

// errNotSigningIn reports sign-in input when no sign-in is waiting for it.
var errNotSigningIn = errors.New("telegram: not waiting for sign-in")

// Connector is one Telegram account.
type Connector struct {
	account domain.Account
	dir     string
	answers chan answer

	// version is this helper's own release, reported to Telegram as the
	// device's app version (see identity.go); Provider.Connect sets it,
	// since New alone has no way to know it.
	version string

	mu      sync.Mutex
	api     *tg.Client
	sink    connector.Sink
	waiting bool
	self    int64             // this account's own user id, set once Run signs in
	remotes map[string]string // "user:42" to the full remote id with its access hash
	sent    map[string]string // "<remote id>/<message id>" to our message id
}

var (
	_ connector.Connector        = (*Connector)(nil)
	_ connector.Authenticator    = (*Connector)(nil)
	_ connector.HistoryLoader    = (*Connector)(nil)
	_ connector.MediaFetcher     = (*Connector)(nil)
	_ connector.MessageRefresher = (*Connector)(nil)
	_ connector.Organizer        = (*Connector)(nil)
	_ connector.Reactor          = (*Connector)(nil)
	_ connector.MemberLister     = (*Connector)(nil)
)

// New returns the connector for an account whose credentials and session
// are kept in dir.
func New(account domain.Account, dir string) *Connector {
	return &Connector{
		account: account,
		dir:     dir,
		answers: make(chan answer, 1),
		remotes: map[string]string{},
		sent:    map[string]string{},
	}
}

// Account describes the Telegram account.
func (c *Connector) Account() domain.Account {
	return c.account
}

// Run signs in if needed, syncs, and then follows updates until ctx is
// cancelled or the connection fails.
func (c *Connector) Run(ctx context.Context, sink connector.Sink) error {
	creds, err := loadCredentials(c.dir, c.account.ID)
	if err != nil {
		return err
	}

	dispatcher := tg.NewUpdateDispatcher()
	gaps, err := c.newGapManager(dispatcher)
	if err != nil {
		return err
	}
	client := gotd.NewClient(creds.APIID, creds.APIHash, gotd.Options{
		SessionStorage: &session.FileStorage{Path: sessionPath(c.dir, c.account.ID)},
		UpdateHandler:  gaps,
		Device:         deviceConfig(c.version),
	})
	c.handleUpdates(dispatcher, sink)

	return client.Run(ctx, func(ctx context.Context) error {
		sink.AccountStatus(ctx, c.account.ID, domain.AccountConnecting, "")
		if err := c.authorize(ctx, client, dispatcher, sink); err != nil {
			return err
		}

		self, err := client.Self(ctx)
		if err != nil {
			return fmt.Errorf("telegram: who am I: %w", err)
		}

		c.connected(client.API(), sink)
		c.setSelf(self.ID)
		sink.AccountStatus(ctx, c.account.ID, domain.AccountConnected, "Signed in as "+ownName(self))
		defer c.disconnected()

		if err := c.sync(ctx, client.API(), sink); err != nil {
			return err
		}

		return gaps.Run(ctx, client.API(), self.ID, updates.AuthOptions{})
	})
}

// newGapManager builds the update manager for this account, backed by
// its saved pts, qts, seq, date and access hashes (see updatestate.go),
// so a restart always resumes updates.getDifference from where an
// earlier run left off instead of losing whatever happened while the
// helper was not running.
func (c *Connector) newGapManager(dispatcher tg.UpdateDispatcher) (*updates.Manager, error) {
	stateStorage, err := newUpdateStorage(updateStatePath(c.dir, c.account.ID))
	if err != nil {
		return nil, err
	}

	return updates.New(updates.Config{
		Handler:          dispatcher,
		Storage:          stateStorage,
		AccessHasher:     stateStorage,
		UserAccessHasher: stateStorage,
		// The gap since this account's saved pts was too large for
		// Telegram to hand back as a list of updates, so gotd itself
		// falls back to resuming from the position Telegram gives it
		// instead, the same loss of history a very long offline gap
		// always risks. Logged so a real report of missing messages
		// can be told apart from a connector bug.
		OnTooLong:        func() { log.Printf("telegram: update gap too long, resyncing") },
		OnChannelTooLong: func(int64) { log.Printf("telegram: channel update gap too long, resyncing") },
	}), nil
}

// authorize signs the account in unless its saved session already is.
func (c *Connector) authorize(ctx context.Context, client *gotd.Client, d tg.UpdateDispatcher, sink connector.Sink) error {
	status, err := client.Auth().Status(ctx)
	if err != nil {
		return fmt.Errorf("telegram: sign-in status: %w", err)
	}

	if status.Authorized {
		return nil
	}

	sink.AccountStatus(ctx, c.account.ID, domain.AccountNeedsAuth, "")
	c.setWaiting(true)
	defer c.setWaiting(false)

	api := &gotdAuth{client: client, loggedIn: qrLoggedIn(d)}
	report := func(step connector.AuthStep) { sink.AuthStep(ctx, c.account.ID, step) }

	return signIn(ctx, api, c.answers, report)
}

// SubmitAuth answers the step sign-in is waiting for.
func (c *Connector) SubmitAuth(ctx context.Context, step, value string) error {
	c.mu.Lock()
	waiting := c.waiting
	c.mu.Unlock()

	if !waiting {
		return errNotSigningIn
	}

	select {
	case c.answers <- answer{step: step, value: value}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Send sends a message, uploading its attachment first when it has one,
// and reports it sent with Telegram's id.
func (c *Connector) Send(ctx context.Context, conv domain.Conversation, m domain.Message) error {
	api, sink, err := c.session()
	if err != nil {
		return err
	}

	peer, err := inputPeer(conv.RemoteID)
	if err != nil {
		return err
	}

	result, err := sendRequest(ctx, api, peer, m)
	if err != nil {
		return fmt.Errorf("telegram: send: %w", err)
	}

	if id, ok := sentID(result); ok {
		remote := fmt.Sprint(id)
		c.remember(conv.RemoteID, remote, m.ID)
		sink.OutgoingStatus(ctx, m.ID, remote, domain.StatusSent)
	}

	return nil
}

// inputReplyTo turns an outgoing message's Reply into what a send
// request (plain text or media) needs to thread it under the quoted
// message, or nil when there is no reply or its remote id is not known
// yet (the quoted message is itself still pending). A plain int, not the
// interface's typed nil, is returned so the request's "is there a
// reply" check sees an actual nil, not a non-nil interface over a nil
// pointer.
func inputReplyTo(reply *domain.Reply) tg.InputReplyToClass {
	if reply == nil || reply.RemoteID == "" {
		return nil
	}

	id, err := strconv.Atoi(reply.RemoteID)
	if err != nil {
		return nil
	}

	return &tg.InputReplyToMessage{ReplyToMsgID: id}
}

// MarkRead tells Telegram the conversation has been read.
func (c *Connector) MarkRead(ctx context.Context, conv domain.Conversation) error {
	api, _, err := c.session()
	if err != nil {
		return err
	}

	peer, err := inputPeer(conv.RemoteID)
	if err != nil {
		return err
	}

	if ch, ok := peer.(*tg.InputPeerChannel); ok {
		channel := &tg.InputChannel{ChannelID: ch.ChannelID, AccessHash: ch.AccessHash}
		_, err = api.ChannelsReadHistory(ctx, &tg.ChannelsReadHistoryRequest{Channel: channel})
	} else {
		_, err = api.MessagesReadHistory(ctx, &tg.MessagesReadHistoryRequest{Peer: peer})
	}

	if err != nil {
		return fmt.Errorf("telegram: mark read: %w", err)
	}

	return nil
}

// React sets or clears the user's reaction to a message. Telegram's
// response echoes the message's new reaction chips, which are reported
// at once rather than waiting for the matching live update.
func (c *Connector) React(ctx context.Context, conv domain.Conversation, messageRemoteID, emoji string) error {
	api, sink, err := c.session()
	if err != nil {
		return err
	}

	peer, err := inputPeer(conv.RemoteID)
	if err != nil {
		return err
	}

	msgID, err := strconv.Atoi(messageRemoteID)
	if err != nil {
		return fmt.Errorf("telegram: react: %w", err)
	}

	result, err := api.MessagesSendReaction(ctx, &tg.MessagesSendReactionRequest{
		Peer: peer, MsgID: msgID, Reaction: reactionList(emoji),
	})
	if err != nil {
		return fmt.Errorf("telegram: react: %w", err)
	}

	if rs, ok := reactionsFromUpdates(result); ok {
		sink.Reacted(ctx, c.account.ID, conv.RemoteID, messageRemoteID, rs)
	}

	return nil
}

// reactionList is the single reaction messages.sendReaction takes, or
// none to clear the user's reaction.
func reactionList(emoji string) []tg.ReactionClass {
	if emoji == "" {
		return nil
	}

	return []tg.ReactionClass{&tg.ReactionEmoji{Emoticon: emoji}}
}

// reactionsFromUpdates finds the reaction chips Telegram reported in its
// response to sendReaction, which echoes the message's new state.
func reactionsFromUpdates(result tg.UpdatesClass) ([]domain.Reaction, bool) {
	updates, ok := result.(*tg.Updates)
	if !ok {
		return nil, false
	}

	for _, u := range updates.Updates {
		if r, ok := u.(*tg.UpdateMessageReactions); ok {
			return reactions(r.Reactions), true
		}
	}

	return nil, false
}

// SetPinned pins or unpins the dialog with Telegram.
func (c *Connector) SetPinned(ctx context.Context, conv domain.Conversation, pinned bool) error {
	api, _, err := c.session()
	if err != nil {
		return err
	}

	peer, err := inputPeer(conv.RemoteID)
	if err != nil {
		return err
	}

	request := &tg.MessagesToggleDialogPinRequest{Pinned: pinned, Peer: &tg.InputDialogPeer{Peer: peer}}
	if _, err := api.MessagesToggleDialogPin(ctx, request); err != nil {
		return fmt.Errorf("telegram: set pinned: %w", err)
	}

	return nil
}

// SetArchived moves the dialog into Telegram's archive folder, or back to
// the default folder.
func (c *Connector) SetArchived(ctx context.Context, conv domain.Conversation, archived bool) error {
	api, _, err := c.session()
	if err != nil {
		return err
	}

	peer, err := inputPeer(conv.RemoteID)
	if err != nil {
		return err
	}

	folderID := 0
	if archived {
		folderID = archiveFolderID
	}

	folderPeers := []tg.InputFolderPeer{{Peer: peer, FolderID: folderID}}
	if _, err := api.FoldersEditPeerFolders(ctx, folderPeers); err != nil {
		return fmt.Errorf("telegram: set archived: %w", err)
	}

	return nil
}

// connected records the API and sink of a signed-in run.
func (c *Connector) connected(api *tg.Client, sink connector.Sink) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.api, c.sink = api, sink
}

// disconnected forgets the run's API, so sends fail until it reconnects.
func (c *Connector) disconnected() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.api, c.sink = nil, nil
}

// session returns the API and sink of the current run.
func (c *Connector) session() (*tg.Client, connector.Sink, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.api == nil {
		return nil, nil, errNotConnected
	}

	return c.api, c.sink, nil
}

// setSelf records this account's own Telegram user id, read by message()
// to flag a message that mentions it.
func (c *Connector) setSelf(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.self = id
}

// selfUserID is this account's own Telegram user id, or 0 before Run
// has signed in.
func (c *Connector) selfUserID() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.self
}

// setWaiting records whether sign-in is waiting for the user.
func (c *Connector) setWaiting(waiting bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.waiting = waiting
}

// remember records which of our messages Telegram knows by an id, so a
// later read receipt can mark it read.
func (c *Connector) remember(conversationRemoteID, messageRemoteID, localID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.sent[conversationRemoteID+"/"+messageRemoteID] = localID
}

// sentID finds the id Telegram gave a message we sent.
func sentID(result tg.UpdatesClass) (int, bool) {
	switch u := result.(type) {
	case *tg.UpdateShortSentMessage:
		return u.ID, true
	case *tg.Updates:
		for _, update := range u.Updates {
			if id, ok := update.(*tg.UpdateMessageID); ok {
				return id.ID, true
			}
		}
	}

	return 0, false
}

// randomID makes the random id Telegram uses to recognise a resent
// message.
func randomID() int64 {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails since Go 1.24

	return int64(binary.LittleEndian.Uint64(b[:]))
}
