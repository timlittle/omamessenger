package whatsapp

// Pairing fakes shared by this package's tests. They stand in for a real
// whatsmeow client so Run and the pairing sequence can be driven and
// checked without ever reaching WhatsApp's servers.

import (
	"context"
	"sync"

	"go.mau.fi/whatsmeow"
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
