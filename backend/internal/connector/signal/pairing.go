package signal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	sstore "go.mau.fi/mautrix-signal/pkg/signalmeow/store"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// linkHint tells people where to scan the code.
const linkHint = "Open Signal on your phone, go to Settings → Linked Devices → Link New Device, and scan this code."

// qrRefreshInterval is how long a linking QR code stays valid before a
// fresh one is drawn: Signal's own servers close the provisioning
// socket after 60 seconds, and Signal Desktop refreshes at 45, so this
// matches the same rhythm WhatsApp's own pairing uses.
const qrRefreshInterval = 45 * time.Second

// maxQRRefreshes bounds how many codes one linking attempt draws before
// giving up and asking the user to start again.
const maxQRRefreshes = 20

// errLinkTimedOut reports that nobody scanned any of the codes drawn
// during one linking attempt.
var errLinkTimedOut = errors.New("signal: the pairing attempt timed out; try again")

// provisioner is signalmeow.PerformProvisioning's own shape: it starts
// a linking attempt and reports its steps over the returned channel.
// It is a field, not a direct call, so pairing can be tested without
// reaching Signal's servers.
type provisioner func(ctx context.Context, deviceStore sstore.DeviceStore, deviceName string, allowBackup bool) chan signalmeow.ProvisioningResponse

// deviceName is what this helper calls itself in Signal's linked
// devices list.
const deviceName = "OmaMessenger"

// link signs this account in by QR code, drawing a fresh one every
// qrRefreshInterval until one is scanned, report fails, or the linking
// attempt as a whole times out.
func link(ctx context.Context, store sstore.DeviceStore, provision provisioner, report func(connector.AuthStep)) (*sstore.DeviceData, error) {
	for attempt := 0; attempt < maxQRRefreshes; attempt++ {
		data, retry, err := linkAttempt(ctx, store, provision, report)
		if !retry {
			return data, err
		}
	}

	return nil, errLinkTimedOut
}

// linkAttempt draws one QR code and waits for it to be scanned or to
// age out. retry is true when the code aged out and a fresh one
// should be drawn.
func linkAttempt(ctx context.Context, store sstore.DeviceStore, provision provisioner, report func(connector.AuthStep)) (data *sstore.DeviceData, retry bool, err error) {
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	responses := provision(attemptCtx, store, deviceName, true)

	resp, err := nextProvisioningResponse(ctx, responses)
	if err != nil {
		return nil, false, err
	}
	if resp.Err != nil {
		return nil, false, fmt.Errorf("signal: link: %w", resp.Err)
	}
	if resp.State != signalmeow.StateProvisioningURLReceived {
		return nil, false, fmt.Errorf("signal: link: unexpected state %v", resp.State)
	}

	png, err := qrPNG(resp.ProvisioningURL)
	if err != nil {
		return nil, false, err
	}
	report(connector.AuthStep{Kind: "qr", QR: png, Hint: linkHint})

	return awaitLinkResult(ctx, responses)
}

// awaitLinkResult waits for the scanned code to finish linking, or for
// it to age out first, in which case retry is reported so a fresh code
// is drawn.
func awaitLinkResult(ctx context.Context, responses <-chan signalmeow.ProvisioningResponse) (data *sstore.DeviceData, retry bool, err error) {
	select {
	case resp, ok := <-responses:
		if !ok {
			return nil, false, errors.New("signal: linking ended unexpectedly")
		}
		if resp.Err != nil {
			return nil, false, fmt.Errorf("signal: link: %w", resp.Err)
		}
		if resp.State != signalmeow.StateProvisioningDataReceived {
			return nil, false, fmt.Errorf("signal: link: unexpected state %v", resp.State)
		}

		return resp.ProvisioningData, false, nil
	case <-time.After(qrRefreshInterval):
		return nil, true, nil
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
}

// nextProvisioningResponse waits for the first response on responses,
// or for ctx to end first.
func nextProvisioningResponse(ctx context.Context, responses <-chan signalmeow.ProvisioningResponse) (signalmeow.ProvisioningResponse, error) {
	select {
	case resp, ok := <-responses:
		if !ok {
			return signalmeow.ProvisioningResponse{}, errors.New("signal: linking ended unexpectedly")
		}

		return resp, nil
	case <-ctx.Done():
		return signalmeow.ProvisioningResponse{}, ctx.Err()
	}
}
