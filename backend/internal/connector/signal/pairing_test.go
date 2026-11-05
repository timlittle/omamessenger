package signal

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	sstore "go.mau.fi/mautrix-signal/pkg/signalmeow/store"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// fakeProvisioner returns a provisioner that plays back responses in
// order, one per call, so a test can script a linking attempt without
// reaching Signal's servers.
func fakeProvisioner(responses ...[]signalmeow.ProvisioningResponse) provisioner {
	call := 0
	return func(context.Context, sstore.DeviceStore, string, bool) chan signalmeow.ProvisioningResponse {
		ch := make(chan signalmeow.ProvisioningResponse, 4)
		if call < len(responses) {
			for _, r := range responses[call] {
				ch <- r
			}
		}
		call++
		close(ch)
		return ch
	}
}

// TestLink_Succeeds checks that a URL followed by completed data
// reports the linked device, with one QR step reported in between.
func TestLink_Succeeds(t *testing.T) {
	t.Parallel()

	aci := uuid.New()
	provision := fakeProvisioner([]signalmeow.ProvisioningResponse{
		{State: signalmeow.StateProvisioningURLReceived, ProvisioningURL: "sgnl://linkdevice?uuid=test"},
		{State: signalmeow.StateProvisioningDataReceived, ProvisioningData: &sstore.DeviceData{ACI: aci}},
	})

	var steps []connector.AuthStep
	data, err := link(t.Context(), nil, provision, func(s connector.AuthStep) { steps = append(steps, s) })
	if err != nil {
		t.Fatalf("link() error = %v", err)
	}
	if data.ACI != aci {
		t.Errorf("ACI = %v, want %v", data.ACI, aci)
	}
	if len(steps) != 1 || steps[0].Kind != "qr" || steps[0].QR == "" {
		t.Errorf("steps = %+v, want one qr step with an image", steps)
	}
}

// TestLink_Refused checks that Signal refusing the attempt reports
// its error, without retrying.
func TestLink_Refused(t *testing.T) {
	t.Parallel()

	provision := fakeProvisioner([]signalmeow.ProvisioningResponse{
		{State: signalmeow.StateProvisioningError, Err: signalmeow.DeviceLinkError{StatusCode: 403, Message: "nope"}},
	})

	if _, err := link(t.Context(), nil, provision, func(connector.AuthStep) {}); err == nil {
		t.Error("link() succeeded, want the refusal reported")
	}
}

// TestLink_ChannelClosedBeforeURL checks that a provisioner whose
// channel closes with no response at all is reported as an
// unexpected end, rather than hanging or panicking.
func TestLink_ChannelClosedBeforeURL(t *testing.T) {
	t.Parallel()

	provision := func(context.Context, sstore.DeviceStore, string, bool) chan signalmeow.ProvisioningResponse {
		ch := make(chan signalmeow.ProvisioningResponse)
		close(ch)
		return ch
	}

	if _, err := link(t.Context(), nil, provision, func(connector.AuthStep) {}); err == nil {
		t.Error("link() succeeded despite the provisioner closing with no response")
	}
}

// TestLink_ChannelClosedAfterURL checks the same for a channel that
// closes right after the URL step, with no final result.
func TestLink_ChannelClosedAfterURL(t *testing.T) {
	t.Parallel()

	provision := func(context.Context, sstore.DeviceStore, string, bool) chan signalmeow.ProvisioningResponse {
		ch := make(chan signalmeow.ProvisioningResponse, 1)
		ch <- signalmeow.ProvisioningResponse{State: signalmeow.StateProvisioningURLReceived, ProvisioningURL: "sgnl://x"}
		close(ch)
		return ch
	}

	if _, err := link(t.Context(), nil, provision, func(connector.AuthStep) {}); err == nil {
		t.Error("link() succeeded despite the provisioner closing with no final result")
	}
}

// TestLink_RefreshesAndTimesOut checks that an attempt nobody answers
// draws a fresh code every qrRefreshInterval and eventually gives up,
// inside a synctest bubble so this costs no real time.
func TestLink_RefreshesAndTimesOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		attempts := 0
		provision := func(context.Context, sstore.DeviceStore, string, bool) chan signalmeow.ProvisioningResponse {
			attempts++
			ch := make(chan signalmeow.ProvisioningResponse, 1)
			ch <- signalmeow.ProvisioningResponse{State: signalmeow.StateProvisioningURLReceived, ProvisioningURL: "sgnl://x"}
			return ch // never closed and never sends a second response: the code just ages out
		}

		_, err := link(t.Context(), nil, provision, func(connector.AuthStep) {})
		if !errors.Is(err, errLinkTimedOut) {
			t.Errorf("link() error = %v, want %v", err, errLinkTimedOut)
		}
		if attempts != maxQRRefreshes {
			t.Errorf("attempts = %d, want %d", attempts, maxQRRefreshes)
		}
	})
}

// TestLink_ContextCancelled checks that cancelling ctx stops a
// linking attempt promptly rather than waiting out the refresh
// interval.
func TestLink_ContextCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		provision := func(context.Context, sstore.DeviceStore, string, bool) chan signalmeow.ProvisioningResponse {
			ch := make(chan signalmeow.ProvisioningResponse, 1)
			ch <- signalmeow.ProvisioningResponse{State: signalmeow.StateProvisioningURLReceived, ProvisioningURL: "sgnl://x"}
			return ch
		}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		if _, err := link(ctx, nil, provision, func(connector.AuthStep) {}); err == nil {
			t.Error("link() succeeded after cancellation, want an error")
		}
	})
}
