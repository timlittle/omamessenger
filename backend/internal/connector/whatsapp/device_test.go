package whatsapp

// openDevice and sessionPath are unexported, with no public way to open
// a real session database, so this test reaches into the package rather
// than through Connector's exported API. It uses a real SQLite database
// in a temporary directory; nothing here reaches WhatsApp's servers.

import (
	"os"
	"path/filepath"
	"testing"

	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/types"
)

func TestSessionPath_NamesTheAccountsDatabase(t *testing.T) {
	t.Parallel()

	got := sessionPath("/data/whatsapp", "wa-1")
	if want := filepath.Join("/data/whatsapp", "wa-1.db"); got != want {
		t.Errorf("sessionPath = %q, want %q", got, want)
	}
}

// Neither of the two tests below run in parallel with each other: both
// create a brand new WhatsApp device, and go.mau.fi/libsignal's package
// logger lazily initializes itself with no synchronization the first
// time anything signs with a key, which the race detector (rightly)
// flags when two goroutines hit that first use at once.

func TestOpenDevice_CreatesAPrivateFreshSession(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "whatsapp")
	dev, err := openDevice(t.Context(), dir, "wa-1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dev.close() }()

	if dev.isPaired() {
		t.Error("a freshly created device reports isPaired, want false")
	}

	info, err := os.Stat(sessionPath(dir, "wa-1"))
	if err != nil {
		t.Fatalf("session file was not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("session file mode = %o, want 0600", perm)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("session directory was not created: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("session directory mode = %o, want 0700", perm)
	}
}

func TestOpenDevice_EnablesAutomaticMessageRerequestFromPhone(t *testing.T) {
	t.Parallel()

	// Without this, a message whatsmeow cannot decrypt (such as a bot
	// replying from another linked device before a session exists with
	// it) is only ever recovered if the sender's own retry succeeds; if
	// the sender never answers the retry receipt, whatsmeow otherwise
	// never asks the primary phone to resend it, and the placeholder
	// never gets replaced (see live.go's handleUndecryptable).
	dir := filepath.Join(t.TempDir(), "whatsapp")
	dev, err := openDevice(t.Context(), dir, "wa-1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dev.close() }()

	if !dev.cli.AutomaticMessageRerequestFromPhone {
		t.Error("AutomaticMessageRerequestFromPhone = false, want true")
	}
}

func TestOpenDevice_ReopensAnExistingSession(t *testing.T) {
	dir := t.TempDir()
	first, err := openDevice(t.Context(), dir, "wa-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.close(); err != nil {
		t.Fatal(err)
	}

	second, err := openDevice(t.Context(), dir, "wa-1")
	if err != nil {
		t.Fatalf("reopening an existing session failed: %v", err)
	}
	defer func() { _ = second.close() }()

	if second.isPaired() {
		t.Error("a session that was never paired reports isPaired, want false")
	}
}

// pairedTestDevice opens a fresh device and gives it the account id a
// real pairing would: whatsmeow's own sub-stores (contacts, LIDs, and
// so on) are wired up only once a device has an id and is saved, which
// a test that never actually pairs must still trigger by hand.
func pairedTestDevice(t *testing.T, own types.JID) *waDevice {
	t.Helper()

	dev, err := openDevice(t.Context(), t.TempDir(), "wa-1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dev.close() })

	dev.cli.Store.ID = &own
	// PutDevice needs a signed identity to save, the way a real pairing
	// leaves behind; the byte slices only need to be non-nil, never NULL
	// columns, not valid signatures.
	dev.cli.Store.Account = &waAdv.ADVSignedDeviceIdentity{
		Details: []byte{}, AccountSignature: make([]byte, 64), AccountSignatureKey: make([]byte, 32), DeviceSignature: make([]byte, 64),
	}
	if err := dev.container.PutDevice(t.Context(), dev.cli.Store); err != nil {
		t.Fatal(err)
	}

	return dev
}

func TestContactName_ResolvesAPhoneJIDFromTheLocalContactStore(t *testing.T) {
	phone := types.NewJID("15551234567", types.DefaultUserServer)
	dev := pairedTestDevice(t, phone)

	if err := dev.cli.Store.Contacts.PutContactName(t.Context(), phone, "Nadia", "Nadia Rahman"); err != nil {
		t.Fatal(err)
	}

	if got := dev.contactName(t.Context(), phone); got != "Nadia Rahman" {
		t.Errorf("contactName(phone) = %q, want the saved full name", got)
	}
}

func TestContactName_ResolvesALIDThroughItsPhoneMapping(t *testing.T) {
	phone := types.NewJID("15551234567", types.DefaultUserServer)
	dev := pairedTestDevice(t, phone)

	lid := types.NewJID("987654", types.HiddenUserServer)
	if err := dev.cli.Store.LIDs.PutLIDMapping(t.Context(), lid, phone); err != nil {
		t.Fatal(err)
	}
	if err := dev.cli.Store.Contacts.PutContactName(t.Context(), phone, "Nadia", "Nadia Rahman"); err != nil {
		t.Fatal(err)
	}

	if got := dev.contactName(t.Context(), lid); got != "Nadia Rahman" {
		t.Errorf("contactName(lid) = %q, want the name saved for its mapped phone JID", got)
	}
}

func TestContactName_IsEmptyWhenNothingIsKnown(t *testing.T) {
	own := types.NewJID("15551234567", types.DefaultUserServer)
	dev := pairedTestDevice(t, own)

	lid := types.NewJID("987654", types.HiddenUserServer)
	if got := dev.contactName(t.Context(), lid); got != "" {
		t.Errorf("contactName(unmapped lid) = %q, want \"\"", got)
	}
}

func TestIsSelfChat_MatchesTheAccountsOwnPhoneJIDOrLID(t *testing.T) {
	own := types.NewJID("15551234567", types.DefaultUserServer)
	dev := pairedTestDevice(t, own)

	ownLID := types.NewJID("111222", types.HiddenUserServer)
	dev.cli.Store.LID = ownLID

	other := types.NewJID("15559998888", types.DefaultUserServer)

	if !dev.isSelfChat(t.Context(), own) {
		t.Error("isSelfChat(own phone JID) = false, want true")
	}
	if !dev.isSelfChat(t.Context(), ownLID) {
		t.Error("isSelfChat(own LID) = false, want true")
	}
	if dev.isSelfChat(t.Context(), other) {
		t.Error("isSelfChat(someone else) = true, want false")
	}
}

func TestIsSelfChat_MatchesOwnLIDEvenBeforeTheDeviceHasCachedIt(t *testing.T) {
	own := types.NewJID("15551234567", types.DefaultUserServer)
	dev := pairedTestDevice(t, own)

	// dev.cli.Store.LID is deliberately left empty, as it can be for a
	// while after pairing: this still has to recognise the account's
	// own LID through whatsmeow's separately-synced LID mapping store.
	ownLID := types.NewJID("111222", types.HiddenUserServer)
	if err := dev.cli.Store.LIDs.PutLIDMapping(t.Context(), ownLID, own); err != nil {
		t.Fatal(err)
	}

	if !dev.isSelfChat(t.Context(), ownLID) {
		t.Error("isSelfChat(own LID, uncached) = false, want true")
	}
}

func TestIsSelfChat_IsFalseWhenNotYetPaired(t *testing.T) {
	dev, err := openDevice(t.Context(), t.TempDir(), "wa-1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dev.close() })

	other := types.NewJID("15559998888", types.DefaultUserServer)
	if dev.isSelfChat(t.Context(), other) {
		t.Error("isSelfChat before pairing = true, want false: there is no own JID to compare against")
	}
}

func TestSelfChatID_IsTheAccountsOwnPhoneJID(t *testing.T) {
	own := types.NewJID("15551234567", types.DefaultUserServer)
	dev := pairedTestDevice(t, own)

	if got, want := dev.selfChatID(), remoteID(own); got != want {
		t.Errorf("selfChatID() = %q, want %q", got, want)
	}
}

func TestOpenDevice_RejectsAnUnwritableDirectory(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "whatsapp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o700) }() // restore so t.TempDir() can clean up

	if _, err := openDevice(t.Context(), dir, "wa-1"); err == nil {
		t.Error("openDevice in an unwritable directory = nil error, want one")
	}
}
