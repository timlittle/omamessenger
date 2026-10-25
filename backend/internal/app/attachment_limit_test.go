package app_test

// attachment_limit_test.go checks Commands.AttachmentLimit and the
// per-account refusal Send applies ahead of the global
// MaxAttachmentSize backstop (see attachment_test.go): split out of
// it, which was already near the file-length gate, the same reasoning
// connector_fakes_test.go documents for its own split.

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestSend_RejectsAnAttachmentOverTheAccountsOwnLimit confirms a
// connector-reported limit narrower than MaxAttachmentSize is the one
// that applies, naming both the limit and the kind refused, and that
// looking it up asks the limiter for the account and kind the
// attachment is actually going out as.
func TestSend_RejectsAnAttachmentOverTheAccountsOwnLimit(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.limiter.limits["wa file"] = 10

	path := writeTestFile(t, "notes.txt", []byte("more than ten bytes of plain text"))

	_, err := f.commands.Send(ctx, "chat", "hi", app.SendOptions{AttachmentPath: path})
	if !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("Send(over account limit) = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "files") || !strings.Contains(err.Error(), "10 B") {
		t.Errorf("error = %q, want it to mention files and the 10 B limit", err)
	}
	if !slices.Contains(f.limiter.asked, "wa file") {
		t.Errorf("limiter asked = %v, want it to have asked about wa's file limit", f.limiter.asked)
	}
}

// TestSend_AllowsAnAttachmentUnderTheAccountsOwnLimit confirms a
// connector-reported limit only narrows what Send accepts; a file that
// fits within it still sends.
func TestSend_AllowsAnAttachmentUnderTheAccountsOwnLimit(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	ctx := t.Context()
	f.conversation(t, "chat", "Chat", domain.KindDirect)
	f.limiter.limits["wa file"] = 1024

	path := writeTestFile(t, "notes.txt", []byte("short"))

	m, err := f.commands.Send(ctx, "chat", "hi", app.SendOptions{AttachmentPath: path})
	if err != nil || m.Media == nil {
		t.Fatalf("Send(under account limit) = %+v, %v, want it to send", m, err)
	}
}

// TestAttachmentLimit_FallsBackToMaxAttachmentSize confirms
// Commands.AttachmentLimit answers MaxAttachmentSize for an account
// the limiter has nothing scripted for, the same default Send's own
// backstop already applied before per-account limits existed.
func TestAttachmentLimit_FallsBackToMaxAttachmentSize(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)

	if limit := f.commands.AttachmentLimit("wa", domain.MediaFile); limit != app.MaxAttachmentSize {
		t.Errorf("AttachmentLimit(no connector limit) = %d, want %d", limit, app.MaxAttachmentSize)
	}
}

// TestAttachmentLimit_PrefersTheConnectorsOwnLimit confirms
// Commands.AttachmentLimit reports a connector's own narrower limit
// instead of the MaxAttachmentSize default.
func TestAttachmentLimit_PrefersTheConnectorsOwnLimit(t *testing.T) {
	t.Parallel()

	f := newFixture(t, false)
	f.limiter.limits["wa photo"] = 16 << 20

	if limit := f.commands.AttachmentLimit("wa", domain.MediaPhoto); limit != 16<<20 {
		t.Errorf("AttachmentLimit(connector limit) = %d, want %d", limit, 16<<20)
	}
}
