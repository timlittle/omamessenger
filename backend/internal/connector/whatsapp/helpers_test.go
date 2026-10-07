package whatsapp

// Pairing fakes shared by this package's tests. They stand in for a real
// whatsmeow client so Run and the pairing sequence can be driven and
// checked without ever reaching WhatsApp's servers.

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// newTestMediaStore opens a media store in a fresh temporary directory,
// closing it when the test ends.
func newTestMediaStore(t *testing.T) *mediaStore {
	t.Helper()

	store, err := openMediaStore(t.Context(), filepath.Join(t.TempDir(), "whatsapp"), "wa-1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.close() })

	return store
}

// fakeDevice is a hand-written double for device: a test drives it by
// pushing items onto its QR channel and calling status, and reads back
// what the connector called on it.
type fakeDevice struct {
	mu sync.Mutex

	paired     bool
	connectErr error
	qrErr      error
	pairCode   string
	pairErr    error

	connects     int
	disconnects  int
	closed       bool
	loggedOut    bool
	pairedPhones []string
	handler      func(status string)
	unregistered bool

	codes chan whatsmeow.QRChannelItem

	eventHandlers     []func(evt any)
	eventUnregistered int

	groupNames map[string]string
	groupErr   error
	groupCalls []types.JID
}

// newFakeDevice returns a fake with an open QR channel and no session.
func newFakeDevice() *fakeDevice {
	return &fakeDevice{codes: make(chan whatsmeow.QRChannelItem, 8)}
}

// connect records the call and reports connectErr. An already-paired
// device simulates whatsmeow firing its Connected event right away, as
// a reconnect to an existing session does; a device still pairing does
// not, since real pairing's handshake only finishes well after Connect
// returns, so a test fires that event itself once it sends success.
func (d *fakeDevice) connect(context.Context) error {
	d.mu.Lock()
	d.connects++
	err, handler, paired := d.connectErr, d.handler, d.paired
	d.mu.Unlock()

	if err == nil && paired && handler != nil {
		handler(statusConnected)
	}

	return err
}

// disconnect records the call.
func (d *fakeDevice) disconnect() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.disconnects++
}

// close records the call.
func (d *fakeDevice) close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.closed = true

	return nil
}

// isPaired reports the session a test set up.
func (d *fakeDevice) isPaired() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.paired
}

// logOut records the call.
func (d *fakeDevice) logOut(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.loggedOut = true

	return nil
}

// qrCodes returns the fake's scripted channel, or qrErr.
func (d *fakeDevice) qrCodes(context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	if d.qrErr != nil {
		return nil, d.qrErr
	}

	return d.codes, nil
}

// pairPhone records number and reports the scripted code or error.
func (d *fakeDevice) pairPhone(_ context.Context, number string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.pairedPhones = append(d.pairedPhones, number)
	if d.pairErr != nil {
		return "", d.pairErr
	}

	return d.pairCode, nil
}

// onStatus records the handler and returns an unregister func that marks
// itself called.
func (d *fakeDevice) onStatus(handler func(status string)) func() {
	d.mu.Lock()
	d.handler = handler
	d.mu.Unlock()

	return func() {
		d.mu.Lock()
		defer d.mu.Unlock()

		d.unregistered = true
	}
}

// status invokes the registered handler, simulating a whatsmeow event.
func (d *fakeDevice) status(s string) {
	d.mu.Lock()
	h := d.handler
	d.mu.Unlock()

	if h != nil {
		h(s)
	}
}

// onEvent records handler and returns an unregister func that counts its
// calls.
func (d *fakeDevice) onEvent(handler func(evt any)) func() {
	d.mu.Lock()
	d.eventHandlers = append(d.eventHandlers, handler)
	d.mu.Unlock()

	return func() {
		d.mu.Lock()
		defer d.mu.Unlock()

		d.eventUnregistered++
	}
}

// fire hands evt to every handler registered with onEvent, simulating a
// whatsmeow event arriving.
func (d *fakeDevice) fire(evt any) {
	d.mu.Lock()
	handlers := append([]func(evt any){}, d.eventHandlers...)
	d.mu.Unlock()

	for _, h := range handlers {
		h(evt)
	}
}

// groupName records the call and reports the scripted name or error.
func (d *fakeDevice) groupName(_ context.Context, jid types.JID) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.groupCalls = append(d.groupCalls, jid)
	if d.groupErr != nil {
		return "", d.groupErr
	}

	return d.groupNames[jid.String()], nil
}

// strPtr takes the address of a string literal, since the generated
// protobuf structs this package normalizes hold every optional field as
// a pointer.
func strPtr(s string) *string { return &s }

// boolPtr takes the address of a bool literal, for the same reason.
func boolPtr(b bool) *bool { return &b }

// u32 takes the address of a uint32 literal, for the same reason.
func u32(n uint32) *uint32 { return &n }

// u64 takes the address of a uint64 literal, for the same reason.
func u64(n uint64) *uint64 { return &n }

// contains reports whether s holds substr, without pulling in strings
// just for one assertion that an error message leaked nothing.
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}
