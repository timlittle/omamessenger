package signal

// sigDevice adapts a real signalmeow client, which this test drives
// against a real session database the same way device_test.go does,
// never reaching Signal's own servers. Its network-bound methods
// (startReceiveLoops, contactName, groupInfo, waitForTransfer,
// fetchAndProcessTransfer, downloadAttachment, unlink and
// groupIdentifierForMasterKey) are not exercised here, the same gap
// the WhatsApp connector's own device wrapper has for connect,
// disconnect, qrCodes, pairPhone and sendMessage: a real Signal
// account is the only way to reach them, and privacy.md and
// resources.md forbid a test doing that.

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/libsignalgo"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/events"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/backuppb"
	sstore "go.mau.fi/mautrix-signal/pkg/signalmeow/store"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/types"
)

// newTestSigDevice opens a fresh session and links identity as a
// device with no network, returning the real sigDevice wrapping it.
func newTestSigDevice(t *testing.T, number string) *sigDevice {
	t.Helper()

	session, err := openSession(t.Context(), t.TempDir(), "acc1")
	if err != nil {
		t.Fatalf("openSession() error = %v", err)
	}
	t.Cleanup(func() { _ = session.close() })

	identityKeyPair, err := libsignalgo.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("generate identity key pair: %v", err)
	}

	aci := uuid.New()
	data := &sstore.DeviceData{ACI: aci, ACIIdentityKeyPair: identityKeyPair, DeviceID: 1, Password: "pw", Number: number}
	if err := session.PutDevice(t.Context(), data); err != nil {
		t.Fatalf("PutDevice() error = %v", err)
	}

	dev, err := session.DeviceByACI(t.Context(), aci)
	if err != nil || dev == nil {
		t.Fatalf("DeviceByACI() = %v, %v", dev, err)
	}

	return newSigDevice(dev, session, func(events.SignalEvent) bool { return true })
}

// TestSigDevice_NumberAndACI checks the two identity accessors read
// straight through to the underlying client's store.
func TestSigDevice_NumberAndACI(t *testing.T) {
	t.Parallel()

	dev := newTestSigDevice(t, "+15551234567")

	if dev.number() != "+15551234567" {
		t.Errorf("number() = %q", dev.number())
	}
	if dev.aci() == uuid.Nil {
		t.Error("aci() = nil")
	}
}

// TestSigDevice_BackupStore checks the local backup-store
// delegations: a chat and its recipient, added the way a history
// import does, come back out through backupChats, backupRecipient
// and backupChatItems. None of this reaches Signal's servers: the
// backup store is this account's own local SQLite table.
func TestSigDevice_BackupStore(t *testing.T) {
	t.Parallel()

	dev := newTestSigDevice(t, "")
	ctx := t.Context()

	if err := dev.cli.Store.BackupStore.AddBackupRecipient(ctx, &backuppb.Recipient{Id: 1}); err != nil {
		t.Fatalf("AddBackupRecipient() error = %v", err)
	}
	if err := dev.cli.Store.BackupStore.AddBackupChat(ctx, &backuppb.Chat{Id: 10, RecipientId: 1}); err != nil {
		t.Fatalf("AddBackupChat() error = %v", err)
	}
	if err := dev.cli.Store.BackupStore.AddBackupChatItem(ctx, &backuppb.ChatItem{ChatId: 10, AuthorId: 1, DateSent: 1}); err != nil {
		t.Fatalf("AddBackupChatItem() error = %v", err)
	}

	chats, err := dev.backupChats(ctx)
	if err != nil || len(chats) != 1 {
		t.Fatalf("backupChats() = %v, %v", chats, err)
	}

	recipient, err := dev.backupRecipient(ctx, 1)
	if err != nil || recipient == nil {
		t.Fatalf("backupRecipient() = %v, %v", recipient, err)
	}

	items, err := dev.backupChatItems(ctx, 10, time.Now(), false, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("backupChatItems() = %v, %v", items, err)
	}
}

// TestSigDevice_GroupIdentifierForMasterKey checks the group
// identifier derivation, which is pure local cryptography with no
// network of its own (see signalmeow's groupIdentifierFromMasterKey).
func TestSigDevice_GroupIdentifierForMasterKey(t *testing.T) {
	t.Parallel()

	dev := newTestSigDevice(t, "")

	masterKey := types.SerializedGroupMasterKey(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	gid, err := dev.groupIdentifierForMasterKey(t.Context(), masterKey)
	if err != nil {
		t.Fatalf("groupIdentifierForMasterKey() error = %v", err)
	}
	if gid == "" {
		t.Error("groupIdentifierForMasterKey() = empty identifier")
	}
}

// TestSigDevice_Close checks that close releases the session without
// error.
func TestSigDevice_Close(t *testing.T) {
	t.Parallel()

	dev := newTestSigDevice(t, "")
	if err := dev.close(); err != nil {
		t.Errorf("close() error = %v", err)
	}
}
