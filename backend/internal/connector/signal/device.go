package signal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mau.fi/mautrix-signal/pkg/libsignalgo"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/events"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/backuppb"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"
	sstore "go.mau.fi/mautrix-signal/pkg/signalmeow/store"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/types"
	"go.mau.fi/util/dbutil"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver, no cgo of its own
)

// device is the signalmeow client surface the connector drives once
// linked: receive loops, naming, history and attachment download,
// kept together as WhatsApp's own device interface is.
type device interface {
	// aci is this device's own account identifier.
	aci() uuid.UUID

	// number is this device's own phone number, or "" if Signal has not
	// given one (an account registered with a username only).
	number() string

	// startReceiveLoops connects and begins receiving, reporting
	// connection status changes until stopReceiveLoops is called.
	startReceiveLoops(ctx context.Context) (<-chan signalmeow.SignalConnectionStatus, error)

	// stopReceiveLoops disconnects and stops the receive loops.
	stopReceiveLoops() error

	// unlink tells Signal to remove this device from the account.
	unlink(ctx context.Context) error

	// close releases the account's session database.
	close() error

	// contactName is Signal's own name for aci, or "", false when
	// nothing is known yet.
	contactName(ctx context.Context, aci uuid.UUID) (name string, found bool)

	// groupInfo is a group's current title, members and
	// disappearing-message timer.
	groupInfo(ctx context.Context, gid types.GroupIdentifier) (title string, members []signalmeow.GroupMember, disappearingSeconds int64, err error)

	// groupIdentifierForMasterKey derives the identifier a backup's
	// group master key names, matching a live message's ChatID.
	groupIdentifierForMasterKey(ctx context.Context, masterKey types.SerializedGroupMasterKey) (types.GroupIdentifier, error)

	// waitForTransfer waits for Signal's primary phone to offer this
	// device's history transfer archive, once linking has completed.
	waitForTransfer(ctx context.Context) (*signalmeow.TransferArchiveMetadata, error)

	// fetchAndProcessTransfer downloads and imports the history transfer
	// archive meta describes into the local backup store.
	fetchAndProcessTransfer(ctx context.Context, meta *signalmeow.TransferArchiveMetadata) error

	// backupChats lists every chat the history transfer imported.
	backupChats(ctx context.Context) ([]*sstore.BackupChat, error)

	// backupRecipient resolves a backup chat's recipient id to who it is.
	backupRecipient(ctx context.Context, recipientID uint64) (*backuppb.Recipient, error)

	// backupChatItems pages chatID's imported messages, older than
	// anchor when forward is false, up to limit of them.
	backupChatItems(ctx context.Context, chatID uint64, anchor time.Time, forward bool, limit int) ([]*backuppb.ChatItem, error)

	// downloadAttachment downloads and decrypts an attachment straight
	// into file. plaintextHash is checked instead of ptr's own digest
	// when a backup history import verified it that way (see
	// backupmedia.go); a live attachment passes nil.
	downloadAttachment(ctx context.Context, ptr *signalpb.AttachmentPointer, plaintextHash []byte, file *os.File) error

	// sendDirect sends content to recipient's own account, as a
	// one-to-one message or a sync copy to this device's own account.
	sendDirect(ctx context.Context, recipient uuid.UUID, content *signalpb.Content) error

	// sendGroup sends content to a group's current members.
	sendGroup(ctx context.Context, gid types.GroupIdentifier, content *signalpb.Content) error

	// uploadAttachment uploads body to Signal's media servers and
	// returns the pointer a DataMessage attaches it with.
	uploadAttachment(ctx context.Context, body []byte) (*signalpb.AttachmentPointer, error)

	// reportSpam reports aci's message, named by serverGUID and token,
	// to Signal as spam.
	reportSpam(ctx context.Context, aci uuid.UUID, serverGUID string, token []byte) error
}

// sigDevice adapts a signalmeow client and its session database to device.
type sigDevice struct {
	cli     *signalmeow.Client
	session *sessionStore
}

// newSigDevice wraps dev as the client that evtHandler is told about
// every event signalmeow reports for it.
func newSigDevice(dev *sstore.Device, session *sessionStore, evtHandler func(events.SignalEvent) bool) *sigDevice {
	return &sigDevice{cli: signalmeow.NewClient(dev, zerolog.Nop(), evtHandler), session: session}
}

// aci is this device's own account identifier.
func (d *sigDevice) aci() uuid.UUID { return d.cli.Store.ACI }

// number is this device's own phone number.
func (d *sigDevice) number() string { return d.cli.Store.Number }

// startReceiveLoops connects and begins receiving.
func (d *sigDevice) startReceiveLoops(ctx context.Context) (<-chan signalmeow.SignalConnectionStatus, error) {
	return d.cli.StartReceiveLoops(ctx)
}

// stopReceiveLoops disconnects and stops the receive loops.
func (d *sigDevice) stopReceiveLoops() error { return d.cli.StopReceiveLoops() }

// unlink tells Signal to remove this device from the account.
func (d *sigDevice) unlink(ctx context.Context) error { return d.cli.Unlink(ctx) }

// close releases the account's session database.
func (d *sigDevice) close() error { return d.session.close() }

// contactName is Signal's own name for aci.
func (d *sigDevice) contactName(ctx context.Context, aci uuid.UUID) (string, bool) {
	recipient, err := d.cli.ContactByACI(ctx, aci)
	if err != nil || recipient == nil {
		return "", false
	}

	if name := recipient.ContactName; name != "" {
		return name, true
	}

	return recipient.Profile.Name, recipient.Profile.Name != ""
}

// groupInfo is a group's current title, members and disappearing timer.
func (d *sigDevice) groupInfo(ctx context.Context, gid types.GroupIdentifier) (string, []signalmeow.GroupMember, int64, error) {
	group, _, err := d.cli.RetrieveGroupByID(ctx, gid, 0)
	if err != nil {
		return "", nil, 0, fmt.Errorf("signal: fetch group: %w", err)
	}

	members := make([]signalmeow.GroupMember, len(group.Members))
	for i, m := range group.Members {
		members[i] = *m
	}

	return group.Title, members, int64(group.DisappearingMessagesDuration), nil
}

// groupIdentifierForMasterKey derives the group identifier masterKey
// names.
func (d *sigDevice) groupIdentifierForMasterKey(ctx context.Context, masterKey types.SerializedGroupMasterKey) (types.GroupIdentifier, error) {
	return d.cli.StoreMasterKey(ctx, masterKey)
}

// waitForTransfer waits for the history transfer archive to be offered.
func (d *sigDevice) waitForTransfer(ctx context.Context) (*signalmeow.TransferArchiveMetadata, error) {
	return d.cli.WaitForTransfer(ctx)
}

// fetchAndProcessTransfer downloads and imports the history archive.
func (d *sigDevice) fetchAndProcessTransfer(ctx context.Context, meta *signalmeow.TransferArchiveMetadata) error {
	return d.cli.FetchAndProcessTransfer(ctx, meta)
}

// backupChats lists every chat the history transfer imported.
func (d *sigDevice) backupChats(ctx context.Context) ([]*sstore.BackupChat, error) {
	return d.cli.Store.BackupStore.GetBackupChats(ctx)
}

// backupRecipient resolves a backup chat's recipient id.
func (d *sigDevice) backupRecipient(ctx context.Context, recipientID uint64) (*backuppb.Recipient, error) {
	return d.cli.Store.BackupStore.GetBackupRecipient(ctx, recipientID)
}

// backupChatItems pages chatID's imported messages.
func (d *sigDevice) backupChatItems(ctx context.Context, chatID uint64, anchor time.Time, forward bool, limit int) ([]*backuppb.ChatItem, error) {
	return d.cli.Store.BackupStore.GetBackupChatItems(ctx, chatID, anchor, forward, limit)
}

// downloadAttachment downloads and decrypts an attachment into file.
func (*sigDevice) downloadAttachment(ctx context.Context, ptr *signalpb.AttachmentPointer, plaintextHash []byte, file *os.File) error {
	_, err := signalmeow.DownloadAttachmentWithPointer(ctx, ptr, plaintextHash, file)
	return err
}

// errSendFailed reports a direct send signalmeow refused with no
// error of its own to wrap.
var errSendFailed = fmt.Errorf("signal: send failed")

// sendDirect sends content to recipient.
func (d *sigDevice) sendDirect(ctx context.Context, recipient uuid.UUID, content *signalpb.Content) error {
	result := d.cli.SendMessage(ctx, libsignalgo.NewACIServiceID(recipient), content)
	if !result.WasSuccessful {
		if result.Error != nil {
			return result.Error
		}

		return errSendFailed
	}

	return nil
}

// sendGroup sends content to a group's current members.
func (d *sigDevice) sendGroup(ctx context.Context, gid types.GroupIdentifier, content *signalpb.Content) error {
	_, err := d.cli.SendGroupMessage(ctx, gid, content)
	return err
}

// uploadAttachment uploads body to Signal's media servers.
func (d *sigDevice) uploadAttachment(ctx context.Context, body []byte) (*signalpb.AttachmentPointer, error) {
	return d.cli.UploadAttachment(ctx, body)
}

// reportSpam reports aci's message as spam.
func (d *sigDevice) reportSpam(ctx context.Context, aci uuid.UUID, serverGUID string, token []byte) error {
	return d.cli.ReportSpam(ctx, aci, serverGUID, token)
}

// sessionStore is an account's Signal session database, plus a
// sidecar file remembering which account identifier it last linked as.
type sessionStore struct {
	container *sstore.Container
	db        *sql.DB
	aciPath   string
}

// openSession opens dir/accountID.db, creating it and its directory
// with owner-only permissions on the first run, and upgrades it to
// signalmeow's latest schema.
func openSession(ctx context.Context, dir, accountID string) (*sessionStore, error) {
	path := sessionPath(dir, accountID)
	if err := createPrivateFile(path); err != nil {
		return nil, fmt.Errorf("signal: open session: %w", err)
	}

	dsn := "file:" + (&url.URL{Path: path}).EscapedPath() +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("signal: open session: %w", err)
	}

	database, err := dbutil.NewWithDB(sqlDB, "sqlite3")
	if err != nil {
		_ = sqlDB.Close() // the database could not be wrapped; nothing to flush
		return nil, fmt.Errorf("signal: wrap session: %w", err)
	}

	container := sstore.NewStore(database, dbutil.ZeroLogger(zerolog.Nop()))
	if err := container.Upgrade(ctx); err != nil {
		_ = sqlDB.Close() // the database could not be prepared; nothing to flush
		return nil, fmt.Errorf("signal: prepare session: %w", err)
	}

	return &sessionStore{container: container, db: sqlDB, aciPath: aciSidecarPath(dir, accountID)}, nil
}

// PutDevice saves a newly linked device's data, so sessionStore itself
// satisfies signalmeow's store.DeviceStore for PerformProvisioning.
func (s *sessionStore) PutDevice(ctx context.Context, dd *sstore.DeviceData) error {
	return s.container.PutDevice(ctx, dd)
}

// DeviceByACI loads the device last linked as aci.
func (s *sessionStore) DeviceByACI(ctx context.Context, aci uuid.UUID) (*sstore.Device, error) {
	return s.container.DeviceByACI(ctx, aci)
}

// DeviceByPNI loads the device last linked under the phone number
// identity pni.
func (s *sessionStore) DeviceByPNI(ctx context.Context, pni uuid.UUID) (*sstore.Device, error) {
	return s.container.DeviceByPNI(ctx, pni)
}

// linkedACI reads the account identifier this session last linked as,
// or found false when this account has never completed linking.
func (s *sessionStore) linkedACI() (aci uuid.UUID, found bool) {
	raw, err := os.ReadFile(s.aciPath)
	if err != nil {
		return uuid.Nil, false
	}

	var sidecar struct{ ACI uuid.UUID }
	if err := json.Unmarshal(raw, &sidecar); err != nil {
		return uuid.Nil, false
	}

	return sidecar.ACI, sidecar.ACI != uuid.Nil
}

// saveLinkedACI remembers aci as this session's linked account, so a
// later run of this connector finds the same device again.
func (s *sessionStore) saveLinkedACI(aci uuid.UUID) error {
	raw, err := json.Marshal(struct{ ACI uuid.UUID }{ACI: aci})
	if err != nil {
		return fmt.Errorf("signal: save linked account: %w", err)
	}

	return os.WriteFile(s.aciPath, raw, 0o600)
}

// close releases the session database.
func (s *sessionStore) close() error { return s.db.Close() }

// sessionPath is where an account's Signal session database lives.
func sessionPath(dir, accountID string) string {
	return filepath.Join(dir, accountID+".db")
}

// aciSidecarPath is where sessionStore remembers which account
// identifier accountID last linked as.
func aciSidecarPath(dir, accountID string) string {
	return filepath.Join(dir, accountID+".aci.json")
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
