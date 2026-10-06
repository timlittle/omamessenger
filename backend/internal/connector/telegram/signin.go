package telegram

import (
	"context"
	"errors"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// Sign-in outcomes the Telegram adapter reports, so the sign-in sequence
// can react to them without knowing Telegram's error codes.
var (
	errPasswordNeeded = errors.New("telegram: two-step verification password needed")
	errWrongCode      = errors.New("telegram: wrong code")
	errWrongPassword  = errors.New("telegram: wrong password")
)

// QRHint tells people where to scan the code.
const QRHint = "Open Telegram on your phone, go to Settings → Devices → Link Desktop Device, and scan this code."

// authAPI is the part of Telegram sign-in this sequence drives.
type authAPI interface {
	// qr shows login codes until one is scanned, calling show with each
	// as a base64 PNG. It returns errPasswordNeeded when the account has
	// two-step verification.
	qr(ctx context.Context, show func(png string) error) error

	// sendCode sends a login code to a phone number and says where it went.
	sendCode(ctx context.Context, phone string) (hint string, err error)

	// signIn checks a login code.
	signIn(ctx context.Context, code string) error

	// password checks the two-step verification password.
	password(ctx context.Context, password string) error
}

// answer is something the user typed in reply to a sign-in step.
type answer struct {
	step  string
	value string
}

// signIn signs in by QR code, switching to phone and code if the user
// gives a phone number instead, and asks for the two-step verification
// password when the account has one. Wrong codes and passwords are asked
// for again.
func signIn(ctx context.Context, api authAPI, answers <-chan answer, report func(connector.AuthStep)) error {
	qrCtx, cancelQR := context.WithCancel(ctx)
	defer cancelQR()

	qrDone := make(chan error, 1)
	go func() {
		qrDone <- api.qr(qrCtx, func(png string) error {
			report(connector.AuthStep{Kind: "qr", QR: png, Hint: QRHint})
			return nil
		})
	}()

	for {
		select {
		case err := <-qrDone:
			return afterLogin(ctx, api, err, answers, report)
		case a := <-answers:
			if a.step != "phone" {
				continue
			}

			cancelQR()
			<-qrDone

			return signInByPhone(ctx, api, a.value, answers, report)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// signInByPhone sends a code to phone and checks the codes typed in until
// one works.
func signInByPhone(ctx context.Context, api authAPI, phone string, answers <-chan answer, report func(connector.AuthStep)) error {
	hint, err := api.sendCode(ctx, phone)
	if err != nil {
		return err
	}

	for {
		report(connector.AuthStep{Kind: "code", Hint: hint})

		code, err := next(ctx, answers, "code")
		if err != nil {
			return err
		}

		err = api.signIn(ctx, code)
		if !errors.Is(err, errWrongCode) {
			return afterLogin(ctx, api, err, answers, report)
		}

		hint = "That code did not work. Check it and try again."
	}
}

// afterLogin finishes a login attempt, asking for the two-step
// verification password if Telegram wants it.
func afterLogin(ctx context.Context, api authAPI, err error, answers <-chan answer, report func(connector.AuthStep)) error {
	if !errors.Is(err, errPasswordNeeded) {
		return err
	}

	hint := "This account has two-step verification. Enter its password."
	for {
		report(connector.AuthStep{Kind: "password", Hint: hint})

		pw, err := next(ctx, answers, "password")
		if err != nil {
			return err
		}

		err = api.password(ctx, pw)
		if !errors.Is(err, errWrongPassword) {
			return err
		}

		hint = "That password did not work. Try again."
	}
}

// next waits for an answer to the given step, ignoring answers to others.
func next(ctx context.Context, answers <-chan answer, step string) (string, error) {
	for {
		select {
		case a := <-answers:
			if a.step == step {
				return a.value, nil
			}
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}
