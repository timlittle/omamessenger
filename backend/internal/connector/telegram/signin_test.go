package telegram

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// scriptedAuth plays Telegram's side of sign-in. qrResult is what the QR
// login returns once it has shown a code; codes and passwords list the
// values Telegram accepts.
type scriptedAuth struct {
	qrResult  error
	qrBlocks  bool
	needs2FA  bool
	codes     []string
	passwords []string
	phones    []string
}

func (a *scriptedAuth) qr(ctx context.Context, show func(png string) error) error {
	if err := show("cG5n"); err != nil {
		return err
	}

	if a.qrBlocks {
		<-ctx.Done()
		return ctx.Err()
	}

	return a.qrResult
}

func (a *scriptedAuth) sendCode(_ context.Context, phone string) (string, error) {
	a.phones = append(a.phones, phone)

	return "Sent to your Telegram app", nil
}

func (a *scriptedAuth) signIn(_ context.Context, code string) error {
	switch {
	case !slices.Contains(a.codes, code):
		return errWrongCode
	case a.needs2FA:
		return errPasswordNeeded
	default:
		return nil
	}
}

func (a *scriptedAuth) password(_ context.Context, pw string) error {
	if !slices.Contains(a.passwords, pw) {
		return errWrongPassword
	}

	return nil
}

// person answers sign-in steps the way someone at the keyboard would: only
// once a step is shown, with the next reply they have for that kind of
// step. It records the kinds of step shown, in order.
type person struct {
	mu      sync.Mutex
	kinds   []string
	replies map[string][]string
	answers chan answer
}

// newPerson returns a person with replies given as step, value pairs.
func newPerson(pairs ...string) *person {
	p := &person{replies: map[string][]string{}, answers: make(chan answer, len(pairs)/2+1)}
	for i := 0; i < len(pairs); i += 2 {
		p.replies[pairs[i]] = append(p.replies[pairs[i]], pairs[i+1])
	}

	return p
}

// report shows a step; the phone number is offered in reply to the QR code.
func (p *person) report(step connector.AuthStep) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.kinds = append(p.kinds, step.Kind)

	kind := step.Kind
	if kind == "qr" {
		kind = "phone"
	}

	if len(p.replies[kind]) == 0 {
		return
	}

	p.answers <- answer{step: kind, value: p.replies[kind][0]}
	p.replies[kind] = p.replies[kind][1:]
}

// shown returns the kinds of step shown so far.
func (p *person) shown() []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	return slices.Clone(p.kinds)
}

func TestSignIn_ByQR(t *testing.T) {
	t.Parallel()

	p := newPerson()
	if err := signIn(t.Context(), &scriptedAuth{}, p.answers, p.report); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(p.shown(), []string{"qr"}) {
		t.Errorf("steps = %v, want [qr]", p.shown())
	}
}

func TestSignIn_ByQRWithTwoStepVerification(t *testing.T) {
	t.Parallel()

	p := newPerson("password", "wrong", "password", "right")
	api := &scriptedAuth{qrResult: errPasswordNeeded, passwords: []string{"right"}}
	if err := signIn(t.Context(), api, p.answers, p.report); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(p.shown(), []string{"qr", "password", "password"}) {
		t.Errorf("steps = %v, want qr, then password asked twice", p.shown())
	}
}

func TestSignIn_ByPhoneInsteadOfQR(t *testing.T) {
	t.Parallel()

	p := newPerson("phone", "+447700900000", "code", "00000", "code", "12345", "password", "pw")
	api := &scriptedAuth{qrBlocks: true, codes: []string{"12345"}, needs2FA: true, passwords: []string{"pw"}}

	if err := signIn(t.Context(), api, p.answers, p.report); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(api.phones, []string{"+447700900000"}) {
		t.Errorf("codes sent to %v", api.phones)
	}

	want := []string{"qr", "code", "code", "password"}
	if !slices.Equal(p.shown(), want) {
		t.Errorf("steps = %v, want %v", p.shown(), want)
	}
}

func TestSignIn_StopsWhenCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	p := newPerson()
	if err := signIn(ctx, &scriptedAuth{qrBlocks: true}, p.answers, p.report); !errors.Is(err, context.Canceled) {
		t.Errorf("signIn = %v, want context.Canceled", err)
	}
}

func TestSignIn_ReportsOtherFailures(t *testing.T) {
	t.Parallel()

	p := newPerson()
	broken := errors.New("network down")
	if err := signIn(t.Context(), &scriptedAuth{qrResult: broken}, p.answers, p.report); !errors.Is(err, broken) {
		t.Errorf("signIn = %v, want the network error", err)
	}
}
