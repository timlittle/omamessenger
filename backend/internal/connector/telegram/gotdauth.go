package telegram

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"

	gotd "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"rsc.io/qr"
)

// gotdAuth signs in through gotd, turning Telegram's error codes into the
// outcomes the sign-in sequence understands.
type gotdAuth struct {
	client   *gotd.Client
	loggedIn qrlogin.LoggedIn
	phone    string
	codeHash string
}

// qrLoggedIn listens for the update that says a QR code was scanned.
func qrLoggedIn(d tg.UpdateDispatcher) qrlogin.LoggedIn {
	return qrlogin.OnLoginToken(d)
}

// qr shows QR codes until one is scanned.
func (a *gotdAuth) qr(ctx context.Context, show func(png string) error) error {
	_, err := a.client.QR().Auth(ctx, a.loggedIn, func(ctx context.Context, token qrlogin.Token) error {
		image, err := qrPNG(token)
		if err != nil {
			return err
		}

		return show(image)
	})

	return translate(err)
}

// sendCode asks Telegram to send a login code and says where it went.
func (a *gotdAuth) sendCode(ctx context.Context, phone string) (string, error) {
	sent, err := a.client.Auth().SendCode(ctx, phone, auth.SendCodeOptions{})
	if err != nil {
		return "", fmt.Errorf("telegram: send code: %w", err)
	}

	code, ok := sent.(*tg.AuthSentCode)
	if !ok {
		return "", errors.New("telegram: send code: unexpected reply")
	}

	a.phone, a.codeHash = phone, code.PhoneCodeHash

	return codeHint(code.Type), nil
}

// signIn checks the login code.
func (a *gotdAuth) signIn(ctx context.Context, code string) error {
	_, err := a.client.Auth().SignIn(ctx, a.phone, code, a.codeHash)

	return translate(err)
}

// password checks the two-step verification password.
func (a *gotdAuth) password(ctx context.Context, password string) error {
	_, err := a.client.Auth().Password(ctx, password)

	return translate(err)
}

// translate turns Telegram's sign-in errors into the sequence's outcomes.
func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, auth.ErrPasswordAuthNeeded), tgerr.Is(err, "SESSION_PASSWORD_NEEDED"):
		return errPasswordNeeded
	case tgerr.Is(err, "PHONE_CODE_INVALID", "PHONE_CODE_EMPTY"):
		return errWrongCode
	case errors.Is(err, auth.ErrPasswordInvalid), tgerr.Is(err, "PASSWORD_HASH_INVALID"):
		return errWrongPassword
	default:
		return fmt.Errorf("telegram: sign in: %w", err)
	}
}

// codeHint says where Telegram sent the login code.
func codeHint(t tg.AuthSentCodeTypeClass) string {
	switch t.(type) {
	case *tg.AuthSentCodeTypeApp:
		return "Telegram sent a code to your Telegram app on another device."
	case *tg.AuthSentCodeTypeSMS:
		return "Telegram sent a code by SMS."
	case *tg.AuthSentCodeTypeCall, *tg.AuthSentCodeTypeFlashCall:
		return "Telegram is calling you with a code."
	default:
		return "Telegram sent you a code."
	}
}

// qrPNG draws a login token as a base64 PNG.
func qrPNG(token qrlogin.Token) (string, error) {
	image, err := token.Image(qr.M)
	if err != nil {
		return "", fmt.Errorf("telegram: draw QR code: %w", err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, image); err != nil {
		return "", fmt.Errorf("telegram: draw QR code: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
