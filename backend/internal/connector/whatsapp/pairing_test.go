package whatsapp

// handlePairingItem and pairByPhone are unexported pieces of pair's loop,
// tested directly here for the outcomes a live pairing attempt is hard
// to force deterministically, alongside pair's own behaviour covered in
// connector_test.go.

import (
	"errors"
	"testing"

	"go.mau.fi/whatsmeow"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

func TestHandlePairingItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		item       whatsmeow.QRChannelItem
		usingPhone bool
		wantDone   bool
		wantErr    bool
		wantReport bool
	}{
		{
			name:       "a rotated code is reported while using the QR",
			item:       whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode, Code: "1@abc"},
			wantReport: true,
		},
		{
			name:       "a rotated code is ignored once paired by phone",
			item:       whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode, Code: "1@abc"},
			usingPhone: true,
		},
		{
			name:     "success ends the attempt without an error",
			item:     whatsmeow.QRChannelItem{Event: "success"},
			wantDone: true,
		},
		{
			name:     "timeout ends the attempt with an error",
			item:     whatsmeow.QRChannelItem{Event: "timeout"},
			wantDone: true,
			wantErr:  true,
		},
		{
			name:     "a pairing error ends the attempt",
			item:     whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventError, Error: errors.New("boom")},
			wantDone: true,
			wantErr:  true,
		},
		{
			name: "scanned without multidevice leaves the same code valid",
			item: whatsmeow.QRChannelItem{Event: "err-scanned-without-multidevice"},
		},
		{
			name:     "an unrecognised terminal event ends the attempt",
			item:     whatsmeow.QRChannelItem{Event: "err-client-outdated"},
			wantDone: true,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var reported []connector.AuthStep
			done, err := handlePairingItem(tt.item, tt.usingPhone, func(s connector.AuthStep) { reported = append(reported, s) })

			if done != tt.wantDone {
				t.Errorf("done = %v, want %v", done, tt.wantDone)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, want an error: %v", err, tt.wantErr)
			}
			if (len(reported) > 0) != tt.wantReport {
				t.Errorf("reported = %v, want a report: %v", reported, tt.wantReport)
			}
		})
	}
}

func TestPairByPhone_ReportsTheLinkCodeOnSuccess(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.pairCode = "ABCD-1234"

	var reported []connector.AuthStep
	usingPhone := pairByPhone(t.Context(), dev, "+15551234567", func(s connector.AuthStep) { reported = append(reported, s) })

	if !usingPhone {
		t.Error("pairByPhone = false, want true after a successful pairing code")
	}
	if len(reported) != 1 || reported[0].Kind != "linkcode" || reported[0].Hint == "" {
		t.Fatalf("reported = %+v, want one linkcode step with a hint", reported)
	}
}

func TestPairByPhone_ReportsTheFailureAndAsksAgain(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.pairErr = errors.New("phone number is too short")

	var reported []connector.AuthStep
	usingPhone := pairByPhone(t.Context(), dev, "123", func(s connector.AuthStep) { reported = append(reported, s) })

	if usingPhone {
		t.Error("pairByPhone = true, want false after a failed pairing attempt")
	}
	if len(reported) != 1 || reported[0].Kind != "phone" {
		t.Fatalf("reported = %+v, want one phone step", reported)
	}
}
