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
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver, no cgo
)

// device is the whatsmeow client surface the connector and its pairing
// sequence drive: connecting, pairing by QR code or a phone's link code,
// telling whether a session already exists, logging out, and watching
// for status changes. It is defined here, the consumer, so tests carry
// an account through connecting and pairing with a fake instead of ever
// reaching WhatsApp's servers. The methods stay together because Run and
// pairing use all of them to get one account connected, whichever way it
// signs in; splitting them would only scatter that one sequence across
// more interfaces.
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

	return &waDevice{cli: whatsmeow.NewClient(dev, waLog.Noop), container: container}, nil
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
