package whatsapp

import (
	"context"
	"errors"
	"fmt"

	"go.mau.fi/whatsmeow"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// QRHint tells people where to scan the code.
const QRHint = "Open WhatsApp on your phone, go to Settings → Linked Devices → Link a Device, and scan this code."

// linkCodeHint tells people where to type a phone-pairing code.
func linkCodeHint(code string) string {
	return "Open WhatsApp on your phone, go to Settings → Linked Devices → Link a Device → Link with phone number instead, and enter this code: " + code
}

// answer is something the user typed in reply to a sign-in step.
type answer struct {
	step  string
	value string
}

// pair signs in by QR code, switching to a phone number's link code when
// the user asks for one, until WhatsApp confirms the pairing or ctx ends.
// Both ways end with the same event from WhatsApp, so once a pairing
// attempt is under way this only has to watch one channel for it.
func pair(ctx context.Context, dev device, answers <-chan answer, report func(connector.AuthStep)) error {
	codes, err := dev.qrCodes(ctx)
	if err != nil {
		return fmt.Errorf("whatsapp: get pairing codes: %w", err)
	}

	if err := dev.connect(ctx); err != nil {
		return fmt.Errorf("whatsapp: connect: %w", err)
	}

	usingPhone := false
	for {
		select {
		case item, ok := <-codes:
			if !ok {
				return errors.New("whatsapp: pairing ended unexpectedly")
			}

			done, err := handlePairingItem(item, usingPhone, report)
			if done {
				return err
			}
		case a := <-answers:
			if a.step != "phone" {
				continue
			}

			usingPhone = pairByPhone(ctx, dev, a.value, report)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// handlePairingItem reports a rotated QR code, or says whether pairing is
// over and with what error, if any.
func handlePairingItem(item whatsmeow.QRChannelItem, usingPhone bool, report func(connector.AuthStep)) (done bool, err error) {
	switch item.Event {
	case whatsmeow.QRChannelEventCode:
		if usingPhone {
			return false, nil
		}

		png, err := qrPNG(item.Code)
		if err != nil {
			return true, err
		}

		report(connector.AuthStep{Kind: "qr", QR: png, Hint: QRHint})

		return false, nil
	case "success":
		return true, nil
	case "timeout":
		return true, errors.New("whatsapp: the pairing attempt timed out; try again")
	case whatsmeow.QRChannelEventError:
		return true, fmt.Errorf("whatsapp: pair: %w", item.Error)
	default:
		// err-client-outdated, err-unexpected-state and the
		// scanned-without-multidevice notice: all end the attempt except
		// the last, which leaves the same code valid for another scan.
		if item.Event == "err-scanned-without-multidevice" {
			return false, nil
		}

		return true, fmt.Errorf("whatsapp: pair: %s", item.Event)
	}
}

// pairByPhone asks WhatsApp to pair with number and reports the link code
// to type on the phone, or reports the failure and asks for the number
// again. It reports whether a pairing attempt is now in progress by
// phone, so rotated QR codes are not shown over the link code.
func pairByPhone(ctx context.Context, dev device, number string, report func(connector.AuthStep)) bool {
	code, err := dev.pairPhone(ctx, number)
	if err != nil {
		report(connector.AuthStep{Kind: "phone", Hint: "That did not work: " + err.Error() + ". Enter the number again, with its country code."})
		return false
	}

	report(connector.AuthStep{Kind: "linkcode", Hint: linkCodeHint(code)})

	return true
}
