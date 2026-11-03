package domain_test

// limits_messageid_test.go covers domain.ValidMessageIDs, kept apart
// from limits_remoteid_test.go and limits_test.go the same way
// limits_replyquote_test.go already is, since this checks a message's
// own RemoteID and SenderID rather than capping a field in place: both
// are the dedup key and the handle react, edit and delete key off, so
// they are validated, never truncated or cleared (see
// docs/decisions.md).

import (
	"strings"
	"testing"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestValidMessageIDs_AcceptsRealSizedIDs confirms a message whose
// RemoteID and SenderID both sit within MaxRemoteIDLength, the shape
// every real WhatsApp or Telegram id takes, passes.
func TestValidMessageIDs_AcceptsRealSizedIDs(t *testing.T) {
	t.Parallel()

	m := domain.Message{RemoteID: "15551234567@s.whatsapp.net", SenderID: "alex"}
	if !domain.ValidMessageIDs(m) {
		t.Errorf("ValidMessageIDs(%+v) = false, want true", m)
	}
}

// TestValidMessageIDs_RejectsAnOversizedRemoteID confirms a RemoteID
// past MaxRemoteIDLength is rejected: whatsmeow reads a message's own
// stanza id off the wire with no length check of its own, up to the
// frame limit, so nothing but this check stops it riding straight onto
// a stored message.
func TestValidMessageIDs_RejectsAnOversizedRemoteID(t *testing.T) {
	t.Parallel()

	m := domain.Message{RemoteID: strings.Repeat("1", domain.MaxRemoteIDLength+1), SenderID: "alex"}
	if domain.ValidMessageIDs(m) {
		t.Errorf("ValidMessageIDs(%+v) = true, want false", m)
	}
}

// TestValidMessageIDs_RejectsAnOversizedSenderID confirms a SenderID
// past MaxRemoteIDLength is rejected: whatsmeow's types.ParseJID keeps
// a JID's user part exactly as given, with no length check of its own,
// so a sender id derived from it carries the same risk as RemoteID.
func TestValidMessageIDs_RejectsAnOversizedSenderID(t *testing.T) {
	t.Parallel()

	m := domain.Message{RemoteID: "ok", SenderID: strings.Repeat("9", domain.MaxRemoteIDLength+1) + "@s.whatsapp.net"}
	if domain.ValidMessageIDs(m) {
		t.Errorf("ValidMessageIDs(%+v) = true, want false", m)
	}
}
