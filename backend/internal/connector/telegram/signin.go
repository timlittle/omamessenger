package telegram

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// passwordHintAttempts bounds how many times fetchPasswordHint retries a
// failed password-hint fetch before giving up: the extra round trips are
// invisible to the user, so it is worth absorbing one flaky request
// rather than silently losing a hint Telegram did have.
const passwordHintAttempts = 3

// passwordHintRetryDelay is how long fetchPasswordHint waits between
// attempts.
const passwordHintRetryDelay = 200 * time.Millisecond

// Sign-in outcomes the Telegram adapter reports, so the sign-in sequence
// can react to them without knowing Telegram's error codes.
var (
	errPasswordNeeded = errors.New("telegram: two-step verification password needed")
	errWrongCode      = errors.New("telegram: wrong code")
	errWrongPassword  = errors.New("telegram: wrong password")
)

// QRHint tells people where to scan the code.
const QRHint = "Open Telegram on your phone, go to Settings → Devices → Link Desktop Device, and scan this code."

// authAPI is the part of Telegram sign-in this sequence drives. It has
// more than two methods because each is a distinct step of gotd's own
// auth API (QR, phone code, password, and the password's hint), and
// signIn drives all of them through one fake in tests.
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

	// passwordHint reads the hint the user set for their two-step
	// verification password, or "" when the account has none to show.
	passwordHint(ctx context.Context) (string, error)
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
	passwordHint := fetchPasswordHint(ctx, api)

	for {
		report(connector.AuthStep{Kind: "password", Hint: hint, PasswordHint: passwordHint})

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

// fetchPasswordHint reads the two-step verification hint, retrying a
// failed fetch passwordHintAttempts times before giving up. The hint is a
// courtesy only: once every attempt has failed, the password step still
// works, just without a reminder shown. It logs which of present, empty
// or fetch-failed happened, so a report of a missing hint can be told
// apart from an account that genuinely set none.
func fetchPasswordHint(ctx context.Context, api authAPI) string {
	var hint string
	var err error

	for attempt := 0; attempt < passwordHintAttempts; attempt++ {
		hint, err = api.passwordHint(ctx)
		if err == nil {
			break
		}

		if attempt < passwordHintAttempts-1 {
			time.Sleep(passwordHintRetryDelay)
		}
	}

	logPasswordHintOutcome(hint, err)
	if err != nil {
		return ""
	}

	return hint
}

// logPasswordHintOutcome records which branch a password step's hint
// fetch took: present, empty or fetch-failed with its error's category,
// never the hint or error text itself.
func logPasswordHintOutcome(hint string, err error) {
	switch {
	case err != nil:
		log.Printf("telegram: password step (hint=fetch-failed, error=%s)", passwordHintErrorCategory(err))
	case hint != "":
		log.Printf("telegram: password step (hint=present)")
	default:
		log.Printf("telegram: password step (hint=empty)")
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
