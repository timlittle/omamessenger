package signal

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/libsignalgo"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/events"
	sstore "go.mau.fi/mautrix-signal/pkg/signalmeow/store"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// noopHandler satisfies openDevice's handler parameter in tests that
// do not exercise live events.
func noopHandler(events.SignalEvent) bool { return true }

// TestOpenDevice_LinksWhenNotYetLinked checks that an account with no
// saved link runs provisioning, remembers the resulting account, and
// reports a fresh link.
func TestOpenDevice_LinksWhenNotYetLinked(t *testing.T) {
	t.Parallel()

	aci := uuid.New()
	identityKeyPair, err := libsignalgo.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("generate identity key pair: %v", err)
	}

	data := &sstore.DeviceData{ACI: aci, ACIIdentityKeyPair: identityKeyPair, DeviceID: 1, Password: "pw"}
	provision := func(ctx context.Context, store sstore.DeviceStore, _ string, _ bool) chan signalmeow.ProvisioningResponse {
		ch := make(chan signalmeow.ProvisioningResponse, 2)
		ch <- signalmeow.ProvisioningResponse{State: signalmeow.StateProvisioningURLReceived, ProvisioningURL: "sgnl://x"}
		// A real provisioner saves the new device before reporting it
		// received; the fake must do the same for DeviceByACI to find it.
		if err := store.PutDevice(ctx, data); err != nil {
			t.Fatalf("PutDevice() error = %v", err)
		}
		ch <- signalmeow.ProvisioningResponse{State: signalmeow.StateProvisioningDataReceived, ProvisioningData: data}
		close(ch)
		return ch
	}

	dev, fresh, err := openDevice(t.Context(), t.TempDir(), "acc1", linkDeps{handler: noopHandler, report: func(connector.AuthStep) {}, provision: provision})
	if err != nil {
		t.Fatalf("openDevice() error = %v", err)
	}
	defer func() { _ = dev.close() }()

	if !fresh {
		t.Error("fresh = false, want true for a first-time link")
	}
	if dev.aci() != aci {
		t.Errorf("aci() = %v, want %v", dev.aci(), aci)
	}
}

// errProvisioner always reports err, for tests of openDevice's and
// link's own error handling.
func errProvisioner(err error) provisioner {
	return func(context.Context, sstore.DeviceStore, string, bool) chan signalmeow.ProvisioningResponse {
		ch := make(chan signalmeow.ProvisioningResponse, 1)
		ch <- signalmeow.ProvisioningResponse{State: signalmeow.StateProvisioningError, Err: err}
		close(ch)
		return ch
	}
}

// TestOpenDevice_LinkFails checks that a refused linking attempt
// reports the error, with no account saved as linked.
func TestOpenDevice_LinkFails(t *testing.T) {
	t.Parallel()

	_, _, err := openDevice(t.Context(), t.TempDir(), "acc1", linkDeps{handler: noopHandler, report: func(connector.AuthStep) {}, provision: errProvisioner(errLinkTimedOut)})
	if err == nil {
		t.Error("openDevice() succeeded despite the provisioner failing")
	}
}

// TestOpenDevice_SessionOpenFails checks that a directory that cannot
// be a session database (a plain file already sits where the
// directory should be) reports the open failure.
func TestOpenDevice_SessionOpenFails(t *testing.T) {
	t.Parallel()

	blocked := t.TempDir() + "/blocked"
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("seed blocking file: %v", err)
	}

	_, _, err := openDevice(t.Context(), blocked, "acc1", linkDeps{handler: noopHandler, report: func(connector.AuthStep) {}, provision: fakeProvisioner(nil)})
	if err == nil {
		t.Error("openDevice() succeeded with a session directory blocked by a file")
	}
}

// TestOpenDevice_LinkedButNeverStored checks that a provisioner
// reporting success without ever storing the device (a bug in a
// provisioner, or a race with another process clearing the store)
// reports an error rather than returning a device with no backing row.
func TestOpenDevice_LinkedButNeverStored(t *testing.T) {
	t.Parallel()

	provision := fakeProvisioner([]signalmeow.ProvisioningResponse{
		{State: signalmeow.StateProvisioningURLReceived, ProvisioningURL: "sgnl://x"},
		{State: signalmeow.StateProvisioningDataReceived, ProvisioningData: &sstore.DeviceData{ACI: uuid.New()}},
	})

	_, _, err := openDevice(t.Context(), t.TempDir(), "acc1", linkDeps{handler: noopHandler, report: func(connector.AuthStep) {}, provision: provision})
	if err == nil {
		t.Error("openDevice() succeeded despite the device never being stored")
	}
}

// TestOpenDevice_StaleLinkRelinks checks that a sidecar naming an
// account whose session row is gone (the database was reset without
// clearing the sidecar, say) falls through to linking again instead
// of failing outright.
func TestOpenDevice_StaleLinkRelinks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	session, err := openSession(t.Context(), dir, "acc1")
	if err != nil {
		t.Fatalf("openSession() error = %v", err)
	}
	if err := session.saveLinkedACI(uuid.New()); err != nil {
		t.Fatalf("saveLinkedACI() error = %v", err)
	}
	_ = session.close()

	aci := uuid.New()
	identityKeyPair, err := libsignalgo.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("generate identity key pair: %v", err)
	}
	data := &sstore.DeviceData{ACI: aci, ACIIdentityKeyPair: identityKeyPair, DeviceID: 1, Password: "pw"}
	provision := func(ctx context.Context, store sstore.DeviceStore, _ string, _ bool) chan signalmeow.ProvisioningResponse {
		ch := make(chan signalmeow.ProvisioningResponse, 2)
		ch <- signalmeow.ProvisioningResponse{State: signalmeow.StateProvisioningURLReceived, ProvisioningURL: "sgnl://x"}
		if err := store.PutDevice(ctx, data); err != nil {
			t.Fatalf("PutDevice() error = %v", err)
		}
		ch <- signalmeow.ProvisioningResponse{State: signalmeow.StateProvisioningDataReceived, ProvisioningData: data}
		close(ch)
		return ch
	}

	dev, fresh, err := openDevice(t.Context(), dir, "acc1", linkDeps{handler: noopHandler, report: func(connector.AuthStep) {}, provision: provision})
	if err != nil {
		t.Fatalf("openDevice() error = %v", err)
	}
	defer func() { _ = dev.close() }()

	if !fresh {
		t.Error("fresh = false, want true: the stale link should have triggered re-linking")
	}
	if dev.aci() != aci {
		t.Errorf("aci() = %v, want %v", dev.aci(), aci)
	}
}

// TestOpenDevice_ReusesLinkedAccount checks that a second call against
// the same directory finds the account already linked and skips
// provisioning entirely.
func TestOpenDevice_ReusesLinkedAccount(t *testing.T) {
	t.Parallel()

	aci := uuid.New()
	identityKeyPair, err := libsignalgo.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("generate identity key pair: %v", err)
	}

	data := &sstore.DeviceData{ACI: aci, ACIIdentityKeyPair: identityKeyPair, DeviceID: 1, Password: "pw"}
	calls := 0
	provision := func(ctx context.Context, store sstore.DeviceStore, _ string, _ bool) chan signalmeow.ProvisioningResponse {
		calls++
		ch := make(chan signalmeow.ProvisioningResponse, 2)
		ch <- signalmeow.ProvisioningResponse{State: signalmeow.StateProvisioningURLReceived, ProvisioningURL: "sgnl://x"}
		if err := store.PutDevice(ctx, data); err != nil {
			t.Fatalf("PutDevice() error = %v", err)
		}
		ch <- signalmeow.ProvisioningResponse{State: signalmeow.StateProvisioningDataReceived, ProvisioningData: data}
		close(ch)
		return ch
	}

	dir := t.TempDir()

	dev, fresh, err := openDevice(t.Context(), dir, "acc1", linkDeps{handler: noopHandler, report: func(connector.AuthStep) {}, provision: provision})
	if err != nil {
		t.Fatalf("first openDevice() error = %v", err)
	}
	_ = dev.close()
	if !fresh {
		t.Fatal("first call was not reported fresh")
	}
	if calls != 1 {
		t.Fatalf("calls = %d after first open, want 1", calls)
	}

	dev, fresh, err = openDevice(t.Context(), dir, "acc1", linkDeps{handler: noopHandler, report: func(connector.AuthStep) {}, provision: provision})
	if err != nil {
		t.Fatalf("second openDevice() error = %v", err)
	}
	defer func() { _ = dev.close() }()

	if fresh {
		t.Error("fresh = true on the second open, want false: it should reuse the saved link")
	}
	if calls != 1 {
		t.Errorf("calls = %d after second open, want still 1 (no re-provisioning)", calls)
	}
	if dev.aci() != aci {
		t.Errorf("aci() = %v, want %v", dev.aci(), aci)
	}
}
