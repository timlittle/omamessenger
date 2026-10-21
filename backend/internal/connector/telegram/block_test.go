package telegram

import (
	"errors"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestSetBlocked_BlocksAndUnblocksThePeer checks that SetBlocked sends
// contacts.block for a block and contacts.unblock for an unblock, each
// naming the conversation's peer.
func TestSetBlocked_BlocksAndUnblocksThePeer(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.ContactsBlockRequest{}, &tg.BoolTrue{})
	f.reply(&tg.ContactsUnblockRequest{}, &tg.BoolTrue{})

	c := connectedAndKnown(f, &connectortest.Sink{}, chatWithNadia.RemoteID)
	if err := c.SetBlocked(t.Context(), chatWithNadia, true); err != nil {
		t.Fatal(err)
	}
	if err := c.SetBlocked(t.Context(), chatWithNadia, false); err != nil {
		t.Fatal(err)
	}

	block, ok := f.sent()[0].(*tg.ContactsBlockRequest)
	if !ok || block.ID == nil {
		t.Errorf("first request = %+v, want a contacts.block of the peer", f.sent()[0])
	}
	if _, ok := f.sent()[1].(*tg.ContactsUnblockRequest); !ok {
		t.Errorf("second request = %+v, want contacts.unblock", f.sent()[1])
	}
}

// TestReportSpam_ReportsThePeer checks that ReportSpam sends
// messages.reportSpam naming the conversation's peer.
func TestReportSpam_ReportsThePeer(t *testing.T) {
	t.Parallel()

	f := newFakeTelegram()
	f.reply(&tg.MessagesReportSpamRequest{}, &tg.BoolTrue{})

	c := connectedAndKnown(f, &connectortest.Sink{}, chatWithNadia.RemoteID)
	if err := c.ReportSpam(t.Context(), chatWithNadia); err != nil {
		t.Fatal(err)
	}

	if _, ok := f.sent()[0].(*tg.MessagesReportSpamRequest); !ok {
		t.Errorf("request = %+v, want messages.reportSpam", f.sent()[0])
	}
}

// TestSetBlocked_Fails checks that SetBlocked refuses before signing in
// and on a malformed peer.
func TestSetBlocked_Fails(t *testing.T) {
	t.Parallel()

	connected := connectedTo(newFakeTelegram(), &connectortest.Sink{})
	for name, err := range map[string]error{
		"before signing in": New(domain.Account{ID: "tg"}, "").SetBlocked(t.Context(), chatWithNadia, true),
		"a malformed peer":  connected.SetBlocked(t.Context(), domain.Conversation{RemoteID: "bad"}, true),
	} {
		if err == nil {
			t.Errorf("SetBlocked %s succeeded, want an error", name)
		}
	}
}

// TestReportSpam_Fails checks that ReportSpam refuses before signing in
// and on a malformed peer.
func TestReportSpam_Fails(t *testing.T) {
	t.Parallel()

	connected := connectedTo(newFakeTelegram(), &connectortest.Sink{})
	for name, err := range map[string]error{
		"before signing in": New(domain.Account{ID: "tg"}, "").ReportSpam(t.Context(), chatWithNadia),
		"a malformed peer":  connected.ReportSpam(t.Context(), domain.Conversation{RemoteID: "bad"}),
	} {
		if err == nil {
			t.Errorf("ReportSpam %s succeeded, want an error", name)
		}
	}
}

// TestSetBlocked_WrapsAFailure checks that a failed request surfaces
// through SetBlocked, rather than being silently swallowed.
func TestSetBlocked_WrapsAFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("flood wait")
	f := newFakeTelegram()
	f.failNext(&tg.ContactsBlockRequest{}, wantErr)

	c := connectedAndKnown(f, &connectortest.Sink{}, chatWithNadia.RemoteID)
	if err := c.SetBlocked(t.Context(), chatWithNadia, true); !errors.Is(err, wantErr) {
		t.Errorf("SetBlocked = %v, want it to wrap %v", err, wantErr)
	}
}
