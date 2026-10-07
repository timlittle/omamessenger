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
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

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

	// sendErr, sendBlocks, markReadErr and nextMessageID script the send
	// and read methods send_test.go and receipts_test.go drive; sent and
	// markReadCalls record what Send and MarkRead actually asked for.
	sendErr       error
	sendBlocks    bool
	nextMessageID types.MessageID
	sent          []sentCall
	markReadErr   error
	markReadCalls []markReadCall
	eventHandler  func(evt any)

	// duringConnect holds events connect delivers before it returns, the
	// way WhatsApp sends history and offline messages as soon as a
	// session connects.
	duringConnect []any

	// groupNames, groupErr and groupCalls script and record groupName,
	// which history.go and live.go call to resolve a group's name.
	groupNames map[string]string
	groupErr   error
	groupCalls []types.JID

	// appStateErr and appStateBlocks script sendAppState, which
	// organize_test.go drives to check pin and archive changes without
	// reaching WhatsApp's servers; appStatePatches records what was sent.
	appStateErr     error
	appStateBlocks  bool
	appStatePatches []appstate.PatchInfo
}

// sentCall records one call to sendMessage.
type sentCall struct {
	jid types.JID
	msg *waE2E.Message
	id  types.MessageID
}

// markReadCall records one call to markRead.
type markReadCall struct {
	ids    []types.MessageID
	chat   types.JID
	sender types.JID
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
	err, handler, paired, during := d.connectErr, d.handler, d.paired, d.duringConnect
	d.mu.Unlock()

	if err == nil && paired && handler != nil {
		handler(statusConnected)
	}
	for _, evt := range during {
		d.fireEvent(evt)
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

// sendMessage records the call and reports sendErr, or blocks on ctx
// when sendBlocks is set, as a real send that never hears back from the
// server does.
func (d *fakeDevice) sendMessage(ctx context.Context, jid types.JID, msg *waE2E.Message, id types.MessageID) (whatsmeow.SendResponse, error) {
	d.mu.Lock()
	d.sent = append(d.sent, sentCall{jid: jid, msg: msg, id: id})
	blocks, err := d.sendBlocks, d.sendErr
	d.mu.Unlock()

	if blocks {
		<-ctx.Done()
		return whatsmeow.SendResponse{}, ctx.Err()
	}
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	return whatsmeow.SendResponse{ID: id}, nil
}

// generateMessageID returns the scripted id, or a fixed one when the
// test does not care which.
func (d *fakeDevice) generateMessageID() types.MessageID {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.nextMessageID != "" {
		return d.nextMessageID
	}

	return "generated-id"
}

// markRead records the call and reports markReadErr.
func (d *fakeDevice) markRead(_ context.Context, ids []types.MessageID, chat, sender types.JID) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.markReadCalls = append(d.markReadCalls, markReadCall{ids: ids, chat: chat, sender: sender})

	return d.markReadErr
}

// onEvent records the handler and returns an unregister func that clears
// it.
func (d *fakeDevice) onEvent(handler func(evt any)) func() {
	d.mu.Lock()
	d.eventHandler = handler
	d.mu.Unlock()

	return func() {
		d.mu.Lock()
		defer d.mu.Unlock()

		d.eventHandler = nil
	}
}

// fireEvent invokes the registered event handler, simulating whatsmeow
// dispatching evt.
func (d *fakeDevice) fireEvent(evt any) {
	d.mu.Lock()
	h := d.eventHandler
	d.mu.Unlock()

	if h != nil {
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

// sendAppState records patch and reports appStateErr, or blocks on ctx
// when appStateBlocks is set, as a real patch that never hears back from
// the server does.
func (d *fakeDevice) sendAppState(ctx context.Context, patch appstate.PatchInfo) error {
	d.mu.Lock()
	d.appStatePatches = append(d.appStatePatches, patch)
	blocks, err := d.appStateBlocks, d.appStateErr
	d.mu.Unlock()

	if blocks {
		<-ctx.Done()
		return ctx.Err()
	}

	return err
}

// connectedTo returns a connector whose Send, MarkRead and event
// handling act on dev, as if Run had already connected it.
func connectedTo(dev device, sink connector.Sink) *Connector {
	c := &Connector{account: domain.Account{ID: "wa-1", Service: domain.ServiceWhatsApp}, answers: make(chan answer, 1)}
	c.connected(dev, sink)

	return c
}

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

// newInMemoryMediaStore opens a private, in-memory media store, for a
// connector under test that never reads a reference back out of this
// package; a test that does should open its own with newTestMediaStore.
func newInMemoryMediaStore(ctx context.Context) (*mediaStore, error) {
	return openMediaStoreDSN(ctx, "file::memory:")
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
