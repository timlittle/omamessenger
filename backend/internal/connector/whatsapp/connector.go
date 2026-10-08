// Package whatsapp connects a WhatsApp account through whatsmeow: it
// pairs by QR code or a phone number's link code, syncs history, follows
// live messages, receipts, typing and organizing changes, sends and
// downloads photos, videos and files alongside outgoing text, reports
// delivery and read progress, and normalizes whatsmeow's JIDs, messages
// and sync data into the domain types the rest of the helper uses.
package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// ErrAlreadyRunning reports a second concurrent Run on the same account.
var ErrAlreadyRunning = errors.New("whatsapp: already running")

// errNotPairing reports sign-in input when no pairing is waiting for it.
var errNotPairing = errors.New("whatsapp: not waiting to pair")

// errNotConnected reports Send or MarkRead asked of an account whose Run
// is not currently connected.
var errNotConnected = errors.New("whatsapp: not connected")

// Connector is one WhatsApp account.
type Connector struct {
	account   domain.Account
	answers   chan answer
	open      func(ctx context.Context) (device, error)
	openMedia func(ctx context.Context) (*mediaStore, error)

	mu      sync.Mutex
	running bool
	waiting bool
	dev     device
	sink    connector.Sink
	media   *mediaStore

	// sent is read and written by send.go and receipts.go: it matches a
	// receipt's chat and WhatsApp id back to the local message it
	// reports progress for. MarkRead (see markread.go) needs no
	// equivalent field: it reads a conversation's unread messages back
	// from the message_keys this connector persists instead.
	sent map[string]*sentMessage

	names         map[string]namedEntry        // contact, push and group names resolved so far, by remote id
	groupMembers  map[string]int               // a group's last known member count, by remote id
	chatKinds     map[string]string            // every conversation remote id this connector has reported, to its kind
	reactions     map[string]map[string]string // "<conversation remote id>/<message remote id>" to who reacted and with which emoji
	undecryptable map[string]bool              // "<conversation remote id>/<message remote id>" still waiting on a placeholder (see live.go's handleUndecryptable)

	// retryWaiters holds one channel per message remote id currently
	// waiting on the primary phone's answer to a media retry request
	// (see retry.go), so the event dispatcher (events.go) has somewhere
	// to deliver events.MediaRetry once it arrives. A concurrent fetch
	// for a different message gets its own entry and so its own
	// channel, never the other's. It has its own mutex (waiters.go), not
	// mu above: a second register for the same key always overwrites.
	retryWaiters waiterTable[*events.MediaRetry]

	// onDemandWaiters holds one channel per conversation remote id
	// currently waiting on the primary phone's answer to an on-demand
	// history request (see history_ondemand.go), so handleHistorySync
	// has somewhere to deliver the matching events.HistorySync once it
	// arrives. Only one entry can exist per chat at a time: a second
	// register for the same key is refused (see errHistoryRequestInProgress).
	onDemandWaiters waiterTable[int]
}

// nameRank orders how much a resolved name can be trusted, so
// rememberName never lets a later, weaker report replace a name already
// known to be better: a contact's saved name or a group's own name
// (WhatsApp's own best answer) always wins over a business's verified
// name, which in turn always wins over a bare push name, self-chosen
// and unverified, matching the priority WhatsApp's own apps give these
// (see contactDisplayName).
type nameRank int

const (
	nameRankPushName nameRank = iota + 1
	nameRankBusiness
	nameRankContact
)

// namedEntry is the best name resolved so far for one remote id, and how
// much it can be trusted.
type namedEntry struct {
	name string
	rank nameRank
}

var (
	_ connector.Connector      = (*Connector)(nil)
	_ connector.Authenticator  = (*Connector)(nil)
	_ connector.LogoutOnRemove = (*Connector)(nil)
	_ connector.MediaFetcher   = (*Connector)(nil)
	_ connector.Organizer      = (*Connector)(nil)
	_ connector.HistoryLoader  = (*Connector)(nil)
)

// New returns the connector for an account whose session is kept in
// dir, having identified this helper to WhatsApp's own device list (see
// identity.go) before Run can start pairing it.
func New(account domain.Account, dir string) *Connector {
	identifyDevice()

	return &Connector{
		account:   account,
		answers:   make(chan answer, 1),
		open:      func(ctx context.Context) (device, error) { return openDevice(ctx, dir, account.ID) },
		openMedia: func(ctx context.Context) (*mediaStore, error) { return openMediaStore(ctx, dir, account.ID) },
	}
}

// Account describes the WhatsApp account.
func (c *Connector) Account() domain.Account {
	return c.account
}

// Run opens the account's session, pairs it if it is not already paired,
// and then reports its connection status, history, live messages and
// organizing changes, and sends and tracks outgoing messages, until ctx
// is cancelled or the connection ends for good.
func (c *Connector) Run(ctx context.Context, sink connector.Sink) error {
	if err := c.startRun(); err != nil {
		return err
	}
	defer c.endRun()

	dev, err := c.open(ctx)
	if err != nil {
		return fmt.Errorf("whatsapp: open session: %w", err)
	}
	defer func() { _ = dev.close() }() // the connection below is what matters; a close failure changes nothing

	media, err := c.openMedia(ctx)
	if err != nil {
		return fmt.Errorf("whatsapp: open media store: %w", err)
	}
	defer func() { _ = media.close() }() // same as the session close above

	sink.AccountStatus(ctx, c.account.ID, domain.AccountConnecting, "")

	stopped := make(chan error, 1)
	unregister := dev.onStatus(func(status string) { c.reportStatus(ctx, sink, status, stopped) })
	defer unregister()

	// WhatsApp sends history and missed messages as soon as a session
	// connects, and right after pairing, before connectOrPair returns, so
	// the handlers listen first or those events are lost for good.
	unregisterEvents := c.handleEvents(ctx, dev, media, sink)
	defer unregisterEvents()

	if err := c.connectOrPair(ctx, dev, sink); err != nil {
		return err
	}
	defer dev.disconnect()

	c.connected(dev, sink, media)
	defer c.disconnected()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-stopped:
		return err
	}
}

// connectOrPair connects a session that is already paired, or pairs a
// new one first, reporting that the account needs attention while it
// waits for the user.
func (c *Connector) connectOrPair(ctx context.Context, dev device, sink connector.Sink) error {
	if dev.isPaired() {
		if err := dev.connect(ctx); err != nil {
			return fmt.Errorf("whatsapp: connect: %w", err)
		}

		return nil
	}

	sink.AccountStatus(ctx, c.account.ID, domain.AccountNeedsAuth, "")
	c.setWaiting(true)
	defer c.setWaiting(false)

	report := func(step connector.AuthStep) { sink.AuthStep(ctx, c.account.ID, step) }

	return pair(ctx, dev, c.answers, report)
}

// reportStatus turns a device status into the account status the sink
// shows, or, for a disconnect whatsmeow will not recover from on its
// own, ends Run so the Manager restarts it and tries pairing again.
func (c *Connector) reportStatus(ctx context.Context, sink connector.Sink, status string, stopped chan<- error) {
	switch status {
	case statusConnected:
		sink.AccountStatus(ctx, c.account.ID, domain.AccountConnected, "")
	case statusDisconnected:
		sink.AccountStatus(ctx, c.account.ID, domain.AccountConnecting, "")
	case statusStopped:
		select {
		case stopped <- errors.New("whatsapp: disconnected and will not reconnect on its own"):
		default:
		}
	}
}

// SubmitAuth answers the step pairing is waiting for.
func (c *Connector) SubmitAuth(ctx context.Context, step, value string) error {
	c.mu.Lock()
	waiting := c.waiting
	c.mu.Unlock()

	if !waiting {
		return errNotPairing
	}

	select {
	case c.answers <- answer{step: step, value: value}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Logout tells WhatsApp to unlink this device, if a run is currently
// connected. Manager.Remove calls it, best effort and bounded, before
// stopping this connector for good; when nothing is connected there is
// nothing to unlink, so it reports success rather than errNotConnected.
func (c *Connector) Logout(ctx context.Context) error {
	dev, _, err := c.session()
	if err != nil {
		return nil
	}

	return dev.logOut(ctx)
}

// connected records the device, sink and media store of a signed-in
// run, so Send, MarkRead, React and incoming events have something to
// act on.
func (c *Connector) connected(dev device, sink connector.Sink, media *mediaStore) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dev, c.sink, c.media = dev, sink, media
}

// disconnected forgets the run's device, sink and media store, so Send
// and MarkRead fail until the account reconnects.
func (c *Connector) disconnected() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dev, c.sink, c.media = nil, nil, nil
}

// session returns the device and sink of the current run, or
// errNotConnected when no run is connected.
func (c *Connector) session() (device, connector.Sink, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.dev == nil {
		return nil, nil, errNotConnected
	}

	return c.dev, c.sink, nil
}

// mediaFor returns the media store of the current run, or nil when this
// connector was never given one, such as a test built without one; a
// nil result means a reply or reaction can still be sent, just without
// anything saved locally to improve on the plain stanza-id fallback.
func (c *Connector) mediaFor() *mediaStore {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.media
}

// startRun records that this connector is running, refusing a second
// concurrent Run.
func (c *Connector) startRun() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return ErrAlreadyRunning
	}

	c.running = true

	return nil
}

// endRun records that Run has returned.
func (c *Connector) endRun() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.running = false
}

// setWaiting records whether pairing is waiting for the user.
func (c *Connector) setWaiting(waiting bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.waiting = waiting
}

// nameFor returns the best name known for remoteID, a contact, push or
// group name resolved so far, or "" when nothing has resolved one yet.
func (c *Connector) nameFor(remoteID string) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.names[remoteID].name
}

// improvedSenderName replaces m's sender name with this connector's own
// cached name for the sender (see nameFor and rememberName), but only
// when message or historyMessage could not resolve one of their own
// and fell all the way back to genericSenderName: a group's history
// sync often carries no push name of its own for each individual
// message, even though the account-wide push name list (see
// syncPushnames) or a later contact event already named that same
// sender from somewhere else. A message that already carries a real
// name, or one that is this account's own, is returned unchanged.
func (c *Connector) improvedSenderName(m domain.Message) domain.Message {
	if m.Outgoing || m.SenderName != genericSenderName {
		return m
	}

	if cached := c.nameFor(m.SenderID); cached != "" {
		m.SenderName = cached
	}

	return m
}

// rememberName records name for remoteID if rank is at least as
// trustworthy as whatever is already cached for it, so a later, weaker
// report, such as a push name arriving after a saved contact name
// already resolved one, can never replace a good name with a worse one.
// It returns whichever name ends up cached: the one just given, or the
// better one already there.
func (c *Connector) rememberName(remoteID, name string, rank nameRank) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, ok := c.names[remoteID]; ok && existing.rank > rank {
		return existing.name
	}

	if c.names == nil {
		c.names = map[string]namedEntry{}
	}
	c.names[remoteID] = namedEntry{name: name, rank: rank}

	return name
}

// groupMembersFor returns the member count last resolved for a group's
// remote id, and whether one has been resolved yet.
func (c *Connector) groupMembersFor(remoteID string) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	n, ok := c.groupMembers[remoteID]

	return n, ok
}

// setGroupMembers records a group's resolved member count, so a later
// sync or message for it does not have to ask WhatsApp again.
func (c *Connector) setGroupMembers(remoteID string, members int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.groupMembers == nil {
		c.groupMembers = map[string]int{}
	}
	c.groupMembers[remoteID] = members
}

// reportConversation reports conv and remembers its remote id and kind,
// so a later app-state sync (see contacts.go) knows which of this
// connector's known conversations are direct chats worth rechecking for
// a better name.
func (c *Connector) reportConversation(ctx context.Context, sink connector.Sink, conv domain.Conversation) {
	c.noteChat(conv.RemoteID, conv.Kind)
	sink.Conversation(ctx, conv)
}

// noteChat records remoteID's kind, for knownDirectChats.
func (c *Connector) noteChat(remoteID, kind string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.chatKinds == nil {
		c.chatKinds = map[string]string{}
	}
	c.chatKinds[remoteID] = kind
}

// knownChat reports whether this connector has already reported a
// conversation for remoteID during this run (see reportConversation
// and noteChat): once it has, a later history sync that only updates
// its pinned, archived or unread state, with no new messages of its
// own in that particular batch, still reaches it, rather than being
// mistaken for a chat that was never worth creating in the first
// place.
func (c *Connector) knownChat(remoteID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, ok := c.chatKinds[remoteID]

	return ok
}

// knownDirectChats lists the remote id of every direct chat this
// connector has reported so far, for handleAppStateSyncComplete's
// rescan.
func (c *Connector) knownDirectChats() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var out []string
	for remote, kind := range c.chatKinds {
		if kind == domain.KindDirect {
			out = append(out, remote)
		}
	}

	return out
}

// knownChatIDs lists the remote id of every conversation this
// connector has reported so far, for organize.go's pinnedCount: it can
// only count a chat WhatsApp's own chat settings have pinned once this
// connector knows that chat exists.
func (c *Connector) knownChatIDs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]string, 0, len(c.chatKinds))
	for remote := range c.chatKinds {
		out = append(out, remote)
	}

	return out
}

// reactTo records sender's reaction to a message as emoji, or clears it
// when emoji is "", and returns the message's full, recomputed tally.
func (c *Connector) reactTo(conversationRemoteID, messageRemoteID, sender, emoji string) []domain.Reaction {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.reactions == nil {
		c.reactions = map[string]map[string]string{}
	}

	key := conversationRemoteID + "/" + messageRemoteID
	bySender := c.reactions[key]
	if bySender == nil {
		bySender = map[string]string{}
		c.reactions[key] = bySender
	}

	if emoji == "" {
		delete(bySender, sender)
	} else {
		bySender[sender] = emoji
	}

	return reactionTally(bySender, "self")
}

// markUndecryptable records that the message named by
// conversationRemoteID and messageRemoteID was stored only as a
// placeholder, because whatsmeow could not decrypt it, so a later
// redelivery of the same id can replace it instead of being silently
// deduplicated away by the store (see resolveUndecryptable).
func (c *Connector) markUndecryptable(conversationRemoteID, messageRemoteID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.undecryptable == nil {
		c.undecryptable = map[string]bool{}
	}
	c.undecryptable[conversationRemoteID+"/"+messageRemoteID] = true
}

// resolveUndecryptable reports whether the message named by
// conversationRemoteID and messageRemoteID was waiting on a placeholder
// (see markUndecryptable), forgetting it either way so a later message
// that happens to reuse the same id is never treated as a replacement
// again.
func (c *Connector) resolveUndecryptable(conversationRemoteID, messageRemoteID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := conversationRemoteID + "/" + messageRemoteID
	found := c.undecryptable[key]
	delete(c.undecryptable, key)

	return found
}
