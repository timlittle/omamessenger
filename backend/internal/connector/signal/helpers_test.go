package signal

import (
	"context"
	"os"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/events"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/backuppb"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"
	sstore "go.mau.fi/mautrix-signal/pkg/signalmeow/store"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// fakeDevice is a hand-written double for device, driving a run
// without any session database or network. t.Helper() is called by
// every method that fails a test directly; the rest just record calls
// or play back canned answers.
type fakeDevice struct {
	selfACI   uuid.UUID
	selfPhone string

	statusCh chan signalmeow.SignalConnectionStatus
	// connectDelay, when set, is slept before statusCh is sent a
	// Connected status, inside startReceiveLoops's own goroutine; it is
	// real time, which costs nothing inside a synctest bubble.
	connectDelay time.Duration
	startErr     error
	unlinkErr    error
	closed       bool
	unlinked     bool
	stopped      bool

	names map[uuid.UUID]string

	groupTitle       string
	groupMembers     []signalmeow.GroupMember
	groupSeconds     int64
	groupErr         error
	groupIdentifiers map[string]types.GroupIdentifier

	transferMeta *signalmeow.TransferArchiveMetadata
	transferErr  error
	fetchErr     error

	chats          []*sstore.BackupChat
	recipients     map[uint64]*backuppb.Recipient
	itemsByChat    map[uint64][]*backuppb.ChatItem
	backupChatsErr error

	downloadErr  error
	downloadData []byte
}

var _ device = (*fakeDevice)(nil)

// newFakeDevice returns a fake already "connected" as self.
func newFakeDevice(self uuid.UUID) *fakeDevice {
	return &fakeDevice{
		selfACI:  self,
		statusCh: make(chan signalmeow.SignalConnectionStatus, 4),
		names:    map[uuid.UUID]string{},
	}
}

func (d *fakeDevice) aci() uuid.UUID { return d.selfACI }
func (d *fakeDevice) number() string { return d.selfPhone }
func (d *fakeDevice) close() error   { d.closed = true; return nil }
func (d *fakeDevice) unlink(context.Context) error {
	d.unlinked = true
	return d.unlinkErr
}

func (d *fakeDevice) startReceiveLoops(context.Context) (<-chan signalmeow.SignalConnectionStatus, error) {
	if d.startErr != nil {
		return nil, d.startErr
	}

	go func() {
		time.Sleep(d.connectDelay)
		d.statusCh <- signalmeow.SignalConnectionStatus{Event: signalmeow.SignalConnectionEventConnected}
	}()

	return d.statusCh, nil
}

func (d *fakeDevice) stopReceiveLoops() error { d.stopped = true; return nil }

func (d *fakeDevice) contactName(_ context.Context, aci uuid.UUID) (string, bool) {
	name, ok := d.names[aci]
	return name, ok
}

func (d *fakeDevice) groupInfo(_ context.Context, _ types.GroupIdentifier) (string, []signalmeow.GroupMember, int64, error) {
	return d.groupTitle, d.groupMembers, d.groupSeconds, d.groupErr
}

func (d *fakeDevice) groupIdentifierForMasterKey(_ context.Context, masterKey types.SerializedGroupMasterKey) (types.GroupIdentifier, error) {
	if d.groupIdentifiers != nil {
		if gid, ok := d.groupIdentifiers[string(masterKey)]; ok {
			return gid, nil
		}
	}

	return types.GroupIdentifier(masterKey), nil
}

func (d *fakeDevice) waitForTransfer(context.Context) (*signalmeow.TransferArchiveMetadata, error) {
	return d.transferMeta, d.transferErr
}

func (d *fakeDevice) fetchAndProcessTransfer(context.Context, *signalmeow.TransferArchiveMetadata) error {
	return d.fetchErr
}

func (d *fakeDevice) backupChats(context.Context) ([]*sstore.BackupChat, error) {
	return d.chats, d.backupChatsErr
}

func (d *fakeDevice) backupRecipient(_ context.Context, id uint64) (*backuppb.Recipient, error) {
	return d.recipients[id], nil
}

func (d *fakeDevice) backupChatItems(_ context.Context, chatID uint64, _ time.Time, _ bool, limit int) ([]*backuppb.ChatItem, error) {
	items := d.itemsByChat[chatID]
	if len(items) > limit {
		items = items[:limit]
	}

	return items, nil
}

func (d *fakeDevice) downloadAttachment(_ context.Context, _ *signalpb.AttachmentPointer, file *os.File) error {
	if d.downloadErr != nil {
		return d.downloadErr
	}

	_, err := file.Write(d.downloadData)

	return err
}

// testOpener returns an opener that hands Run dev directly, with no
// pairing and no session database, and reports freshlyLinked.
func testOpener(dev device, freshlyLinked bool) opener {
	return func(context.Context, func(events.SignalEvent) bool, func(connector.AuthStep)) (device, bool, error) {
		return dev, freshlyLinked, nil
	}
}

// newTestConnector returns a connector wired to dev through a fake
// opener, with no session database and no network, and an in-memory
// media store so Run can open one as it would a real one.
func newTestConnector(dev *fakeDevice) *Connector {
	c := &Connector{account: domain.Account{ID: "acc1", Service: domain.ServiceSignal}}
	c.open = testOpener(dev, false)
	c.openMedia = newInMemoryMediaStore

	return c
}

// newInMemoryMediaStore opens a private, in-memory media store, for a
// connector under test that never reads a saved attachment, message
// metadata or poll back out of this package; a test that does opens
// its own with newTestMediaStore instead.
func newInMemoryMediaStore(ctx context.Context) (*mediaStore, error) {
	return openMediaStoreDSN(ctx, "file::memory:")
}
