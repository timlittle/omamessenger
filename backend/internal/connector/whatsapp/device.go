package whatsapp

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver, no cgo
)

// device is the whatsmeow client surface the connector and its pairing
// sequence drive: connecting, pairing by QR code or a phone's link code,
// telling whether a session already exists, logging out, watching for
// status changes, and sending and fetching messages and their media. It
// is defined here, the consumer, so tests carry an account through
// connecting, pairing and exchanging messages with a fake instead of
// ever reaching WhatsApp's servers. The methods stay together because
// Run, pairing and sending all use this one client to get one account
// connected and keep it talking, whichever way it signs in; splitting
// them would only scatter that one client's surface across more
// interfaces.
type device interface {
	// connect opens the WhatsApp connection.
	connect(ctx context.Context) error

	// disconnect closes the WhatsApp connection.
	disconnect()

	// close releases the account's session database.
	close() error

	// isPaired reports whether this device already has a session, so Run
	// can skip pairing and connect straight away.
	isPaired() bool

	// logOut tells WhatsApp to unlink this device.
	logOut(ctx context.Context) error

	// qrCodes starts a pairing attempt, returning the rotating codes and
	// the final outcome WhatsApp reports for it.
	qrCodes(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error)

	// pairPhone asks WhatsApp to pair with number, returning the
	// 8-character code to type on the phone.
	pairPhone(ctx context.Context, number string) (string, error)

	// onStatus reports "connected", "disconnected" or "loggedout"
	// whenever WhatsApp says so, until the returned func unregisters it.
	onStatus(handler func(status string)) (unregister func())

	// sendMessage sends msg to jid using the client-chosen id, returning
	// WhatsApp's response once its server has accepted it.
	sendMessage(ctx context.Context, jid types.JID, msg *waE2E.Message, id types.MessageID) (whatsmeow.SendResponse, error)

	// generateMessageID returns a fresh id for an outgoing message.
	generateMessageID() types.MessageID

	// markRead tells WhatsApp the messages named by ids, all sent by
	// sender in chat, have been read.
	markRead(ctx context.Context, ids []types.MessageID, chat, sender types.JID) error

	// onEvent reports every event whatsmeow fires for this device, until
	// the returned func unregisters it. Unlike onStatus, which filters
	// to connection status alone, this is the seam the event dispatcher
	// (see events.go) switches on by concrete type, so later waves can
	// add their own cases without widening this interface again.
	onEvent(handler func(evt any)) (unregister func())

	// groupInfo is a group's current name and member count, for a
	// history sync or a live message whose own data left either blank.
	groupInfo(ctx context.Context, jid types.JID) (name string, members int, err error)

	// contactName is WhatsApp's own name for jid: a LID is mapped to its
	// phone JID first, then whichever name the local contact store holds
	// for it, in WhatsApp's own priority order, or "" when nothing is
	// known yet. It never does network I/O: everything it reads was
	// already saved locally by an earlier sync.
	contactName(ctx context.Context, jid types.JID) string

	// isSelfChat reports whether jid is this account's own chat with
	// itself: its phone JID, its LID, or, when this device has not
	// cached its own LID yet, a LID that whatsmeow's local LID store
	// already maps to that phone JID. It never does network I/O:
	// everything it checks was already saved locally by an earlier
	// sync.
	isSelfChat(ctx context.Context, jid types.JID) bool

	// selfChatID is the canonical remote id this connector always uses
	// for the account's own self-chat, so a message, receipt or
	// organizing change addressed by phone JID or by LID always lands
	// in the same conversation: this account's own phone JID, which is
	// always known once paired, unlike its LID.
	selfChatID() string

	// pnForLID resolves jid, a WhatsApp "linked id" (LID), to the phone
	// JID whatsmeow's local LID store already maps it to, so any other
	// chat addressed by its hidden id also always lands in the same
	// conversation as one addressed by phone number. It returns an
	// empty JID when jid is not a LID or nothing is known yet; it never
	// does network I/O.
	pnForLID(ctx context.Context, jid types.JID) types.JID

	// downloadMedia downloads and decrypts a message's attachment, using
	// the reference it was saved with (see normalize_media.go).
	downloadMedia(ctx context.Context, ref mediaRef) ([]byte, error)

	// sendMediaRetryReceipt asks WhatsApp's primary phone to re-upload
	// the media of the message info identifies, because its CDN link has
	// aged out (see retry.go). The phone's answer arrives later as an
	// events.MediaRetry, decrypted with the same mediaKey.
	sendMediaRetryReceipt(ctx context.Context, info *types.MessageInfo, mediaKey []byte) error

	// uploadMedia encrypts and uploads data to WhatsApp's media servers
	// for an attachment of kind, returning what a message proto needs to
	// point at the result.
	uploadMedia(ctx context.Context, data []byte, kind mediaKind) (whatsmeow.UploadResponse, error)

	// sendAppState sends an app-state patch, such as a pin or archive
	// change, so WhatsApp's own record of the chat agrees with it.
	sendAppState(ctx context.Context, patch appstate.PatchInfo) error

	// requestOlderHistory asks WhatsApp's primary phone, through a peer
	// message only this account's own other devices receive, for count
	// messages older than anchor in its chat (see history_ondemand.go).
	// The phone's answer arrives later, asynchronously, as an
	// events.HistorySync of type ON_DEMAND.
	requestOlderHistory(ctx context.Context, anchor *types.MessageInfo, count int) error
}

// pairClientType and pairDisplayName name this companion to WhatsApp when
// pairing by phone number; WhatsApp only accepts a handful of browser and
// OS combinations here, unrelated to what actually runs this helper.
const (
	pairClientType  = whatsmeow.PairClientChrome
	pairDisplayName = "Chrome (Linux)"
)

// waDevice adapts a whatsmeow client and its session database to device.
type waDevice struct {
	cli       *whatsmeow.Client
	container *sqlstore.Container
}

// openDevice opens the account's session database in dir, creating both
// the directory and an empty session on its first run, and wraps the
// WhatsApp client it stores.
func openDevice(ctx context.Context, dir, accountID string) (*waDevice, error) {
	path := sessionPath(dir, accountID)
	if err := createPrivateFile(path); err != nil {
		return nil, fmt.Errorf("whatsapp: open session: %w", err)
	}

	container, err := openContainer(ctx, path)
	if err != nil {
		return nil, err
	}

	dev, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = container.Close() // the container is unusable; nothing to flush before closing
		return nil, fmt.Errorf("whatsapp: load session: %w", err)
	}

	cli := whatsmeow.NewClient(dev, waLog.Noop)
	// Without this, a message whatsmeow cannot decrypt is only ever
	// recovered if the original sender's own retry succeeds. This
	// account's self-chat gets replies from a bot running as another of
	// its own linked devices, and that bot does not answer whatsmeow's
	// retry receipts, so the primary phone is the only other copy of
	// the message left to ask; enabling this is what lets whatsmeow ask
	// it (see live.go's handleUndecryptable and handleContent).
	cli.AutomaticMessageRerequestFromPhone = true

	return &waDevice{cli: cli, container: container}, nil
}

// openContainer opens the SQLite database at path, in WAL mode with
// foreign keys on, and upgrades it to whatsmeow's latest schema. It uses
// the same pure-Go driver as the message store, so the helper stays
// buildable with CGO disabled.
func openContainer(ctx context.Context, path string) (*sqlstore.Container, error) {
	dsn := "file:" + (&url.URL{Path: path}).EscapedPath() +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: open session: %w", err)
	}

	container := sqlstore.NewWithDB(db, "sqlite", waLog.Noop)
	if err := container.Upgrade(ctx); err != nil {
		_ = container.Close() // the database could not be prepared; nothing to flush
		return nil, fmt.Errorf("whatsapp: prepare session: %w", err)
	}

	return container, nil
}

// connect opens the WhatsApp connection.
func (d *waDevice) connect(ctx context.Context) error {
	return d.cli.ConnectContext(ctx)
}

// disconnect closes the WhatsApp connection.
func (d *waDevice) disconnect() {
	d.cli.Disconnect()
}

// close releases the account's session database.
func (d *waDevice) close() error {
	return d.container.Close()
}

// isPaired reports whether this device's session already has an id.
func (d *waDevice) isPaired() bool {
	return d.cli.Store.ID != nil
}

// logOut tells WhatsApp to unlink this device.
func (d *waDevice) logOut(ctx context.Context) error {
	return d.cli.Logout(ctx)
}

// qrCodes starts a pairing attempt.
func (d *waDevice) qrCodes(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	return d.cli.GetQRChannel(ctx)
}

// pairPhone asks WhatsApp to pair with number.
func (d *waDevice) pairPhone(ctx context.Context, number string) (string, error) {
	return d.cli.PairPhone(ctx, number, true, pairClientType, pairDisplayName)
}

// onStatus reports this device's connection and sign-in status changes.
func (d *waDevice) onStatus(handler func(status string)) (unregister func()) {
	id := d.cli.AddEventHandler(func(evt any) {
		if status, ok := statusOf(evt); ok {
			handler(status)
		}
	})

	return func() { d.cli.RemoveEventHandler(id) }
}

// sendMessage sends msg to jid with WhatsApp, under the client-chosen id.
func (d *waDevice) sendMessage(ctx context.Context, jid types.JID, msg *waE2E.Message, id types.MessageID) (whatsmeow.SendResponse, error) {
	return d.cli.SendMessage(ctx, jid, msg, whatsmeow.SendRequestExtra{ID: id})
}

// generateMessageID returns a fresh id for an outgoing message.
func (d *waDevice) generateMessageID() types.MessageID {
	return d.cli.GenerateMessageID()
}

// markRead tells WhatsApp the messages named by ids, all sent by sender
// in chat, have been read as of now.
func (d *waDevice) markRead(ctx context.Context, ids []types.MessageID, chat, sender types.JID) error {
	return d.cli.MarkRead(ctx, ids, time.Now(), chat, sender)
}

// onEvent reports every event whatsmeow fires for this device.
func (d *waDevice) onEvent(handler func(evt any)) (unregister func()) {
	id := d.cli.AddEventHandler(handler)

	return func() { d.cli.RemoveEventHandler(id) }
}

// groupInfo asks WhatsApp for a group's current name and member count.
func (d *waDevice) groupInfo(ctx context.Context, jid types.JID) (string, int, error) {
	info, err := d.cli.GetGroupInfo(ctx, jid)
	if err != nil {
		return "", 0, fmt.Errorf("whatsapp: group info: %w", err)
	}

	return info.Name, info.ParticipantCount, nil
}

// contactName resolves jid to WhatsApp's own name for that person. A LID
// carries no contact record of its own on most accounts, so it is
// mapped to its phone JID through whatsmeow's own LID store first; if
// that fails, or the phone JID itself has no contact saved, jid is
// looked up as given, which covers the few contacts whatsmeow already
// keyed by LID.
func (d *waDevice) contactName(ctx context.Context, jid types.JID) string {
	if name := d.lookupContactName(ctx, jid); name != "" {
		return name
	}

	if jid.Server != types.HiddenUserServer {
		return ""
	}

	phone, err := d.cli.Store.LIDs.GetPNForLID(ctx, jid)
	if err != nil || phone.IsEmpty() {
		return ""
	}

	return d.lookupContactName(ctx, phone)
}

// lookupContactName is contactName's single contact-store lookup, tried
// against both a LID and its mapped phone JID.
func (d *waDevice) lookupContactName(ctx context.Context, jid types.JID) string {
	info, err := d.cli.Store.Contacts.GetContact(ctx, jid)
	if err != nil || !info.Found {
		return ""
	}

	return contactDisplayName(info)
}

// isSelfChat reports whether jid is this account's own self-chat: its
// phone JID, its LID, or, when this device's own cached LID (set once
// whatsmeow learns it, which can lag behind pairing) does not match, a
// LID that whatsmeow's local LID store already maps to this account's
// phone JID. That last check is what still recognises the self-chat
// when a message or a history sync addresses it by a LID before this
// device's own copy of that identifier has caught up.
func (d *waDevice) isSelfChat(ctx context.Context, jid types.JID) bool {
	target := jid.ToNonAD()

	own := d.cli.Store.GetJID()
	if own.IsEmpty() {
		return false
	}
	if target == own.ToNonAD() {
		return true
	}

	if lid := d.cli.Store.GetLID(); !lid.IsEmpty() && target == lid.ToNonAD() {
		return true
	}

	if target.Server != types.HiddenUserServer {
		return false
	}

	phone, err := d.cli.Store.LIDs.GetPNForLID(ctx, target)

	return err == nil && !phone.IsEmpty() && phone.ToNonAD() == own.ToNonAD()
}

// selfChatID is the canonical remote id for this account's own
// self-chat: its phone JID, which is always known once paired, unlike
// its LID (see isSelfChat).
func (d *waDevice) selfChatID() string {
	return remoteID(d.cli.Store.GetJID())
}

// pnForLID resolves jid's phone JID through whatsmeow's local LID
// store, the same lookup contactName uses, returning an empty JID when
// jid is not itself a LID or the mapping is not known yet.
func (d *waDevice) pnForLID(ctx context.Context, jid types.JID) types.JID {
	if jid.Server != types.HiddenUserServer {
		return types.JID{}
	}

	phone, err := d.cli.Store.LIDs.GetPNForLID(ctx, jid)
	if err != nil || phone.IsEmpty() {
		return types.JID{}
	}

	return phone
}

// downloadMedia downloads and decrypts a message's attachment. Passing
// "" for the mms-type lets whatsmeow choose it from the app-info key
// alone, which is all DownloadMediaWithPath needs.
func (d *waDevice) downloadMedia(ctx context.Context, ref mediaRef) ([]byte, error) {
	return d.cli.DownloadMediaWithPath(
		ctx, ref.DirectPath, ref.FileEncSHA256, ref.FileSHA256, ref.MediaKey, appInfo(ref.Kind), "", false,
	)
}

// uploadMedia encrypts and uploads data to WhatsApp's media servers for
// an attachment of kind.
func (d *waDevice) uploadMedia(ctx context.Context, data []byte, kind mediaKind) (whatsmeow.UploadResponse, error) {
	return d.cli.Upload(ctx, data, appInfo(kind))
}

// sendMediaRetryReceipt asks WhatsApp's primary phone to re-upload a
// message's media.
func (d *waDevice) sendMediaRetryReceipt(ctx context.Context, info *types.MessageInfo, mediaKey []byte) error {
	return d.cli.SendMediaRetryReceipt(ctx, info, mediaKey)
}

// sendAppState sends patch with WhatsApp.
func (d *waDevice) sendAppState(ctx context.Context, patch appstate.PatchInfo) error {
	return d.cli.SendAppState(ctx, patch)
}

// requestOlderHistory sends the primary phone an on-demand history
// request for count messages older than anchor.
func (d *waDevice) requestOlderHistory(ctx context.Context, anchor *types.MessageInfo, count int) error {
	_, err := d.cli.SendPeerMessage(ctx, d.cli.BuildHistorySyncRequest(anchor, count))

	return err
}

// Status values device reports through onStatus. statusStopped covers
// every disconnect whatsmeow will not recover from on its own (logged
// out, replaced by another client, banned, and so on; whatsmeow marks
// each of these with its events.PermanentDisconnect interface), as
// opposed to a plain statusDisconnected, which its own auto-reconnect is
// already handling.
const (
	statusConnected    = "connected"
	statusDisconnected = "disconnected"
	statusStopped      = "stopped"
)

// statusOf translates a whatsmeow event into a status onStatus reports,
// or false for an event this connector does not act on.
func statusOf(evt any) (string, bool) {
	switch evt.(type) {
	case *events.Connected:
		return statusConnected, true
	case *events.Disconnected:
		return statusDisconnected, true
	}

	if _, permanent := evt.(events.PermanentDisconnect); permanent {
		return statusStopped, true
	}

	return "", false
}

// sessionPath is where an account's WhatsApp session database lives.
func sessionPath(dir, accountID string) string {
	return filepath.Join(dir, accountID+".db")
}

// createPrivateFile makes sure path exists with owner-only permissions
// before SQLite opens it, because SQLite would create it world-readable.
func createPrivateFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}

	return f.Close()
}
