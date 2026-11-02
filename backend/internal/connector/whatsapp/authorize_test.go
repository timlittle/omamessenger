package whatsapp

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// TestAuthorizedActor_AcceptsTheOriginalSender confirms the common
// case: the person message_keys recorded as a message's sender may
// edit or revoke it.
func TestAuthorizedActor_AcceptsTheOriginalSender(t *testing.T) {
	t.Parallel()

	sender := types.NewJID("15551234567", types.DefaultUserServer)
	key := messageKey{senderID: remoteID(sender)}

	info := types.MessageInfo{MessageSource: types.MessageSource{Sender: sender}}
	if !authorizedActor(t.Context(), &fakeDevice{}, key, true, info) {
		t.Error("authorizedActor(original sender) = false, want true")
	}
}

// TestAuthorizedActor_AcceptsThePhoneAndLIDFormsOfTheSameSender
// confirms a revoke or edit addressed by the opposite form (LID versus
// phone JID) of the message's real sender is still authorized, through
// the same whatsmeow LID-store normalisation personID already applies
// elsewhere in this connector.
func TestAuthorizedActor_AcceptsThePhoneAndLIDFormsOfTheSameSender(t *testing.T) {
	t.Parallel()

	phone := types.NewJID("15551234567", types.DefaultUserServer)
	lid := types.NewJID("987654", types.HiddenUserServer)
	dev := &fakeDevice{lidPhones: map[string]types.JID{lid.String(): phone}}
	key := messageKey{senderID: remoteID(phone)}

	info := types.MessageInfo{MessageSource: types.MessageSource{Sender: lid}}
	if !authorizedActor(t.Context(), dev, key, true, info) {
		t.Error("authorizedActor(sender's other address form) = false, want true")
	}
}

// TestAuthorizedActor_RefusesADifferentSender covers Vuln 1: someone
// other than the message's original sender must never be authorized to
// edit or revoke it.
func TestAuthorizedActor_RefusesADifferentSender(t *testing.T) {
	t.Parallel()

	key := messageKey{senderID: "15551234567@s.whatsapp.net"}
	mallory := types.MessageInfo{MessageSource: types.MessageSource{Sender: types.NewJID("15559876543", types.DefaultUserServer)}}
	if authorizedActor(t.Context(), &fakeDevice{}, key, true, mallory) {
		t.Error("authorizedActor(a different sender) = true, want false")
	}
}

// TestAuthorizedActor_RefusesAnUnknownMessage covers an id this
// connector never saved a key for: nothing recorded to check the
// claimed edit or revoke against, so it is never authorized.
func TestAuthorizedActor_RefusesAnUnknownMessage(t *testing.T) {
	t.Parallel()

	info := types.MessageInfo{MessageSource: types.MessageSource{Sender: types.NewJID("15551234567", types.DefaultUserServer)}}
	if authorizedActor(t.Context(), &fakeDevice{}, messageKey{}, false, info) {
		t.Error("authorizedActor(unknown message) = true, want false")
	}
}

// TestAuthorizedActor_AcceptsOurOwnOtherDevice confirms this account
// can still edit or revoke its own message from another linked device,
// even though such a message is saved with no sender id of its own
// (see keys.go's senderKeyID).
func TestAuthorizedActor_AcceptsOurOwnOtherDevice(t *testing.T) {
	t.Parallel()

	key := messageKey{fromMe: true}
	info := types.MessageInfo{MessageSource: types.MessageSource{IsFromMe: true}}
	if !authorizedActor(t.Context(), &fakeDevice{}, key, true, info) {
		t.Error("authorizedActor(our own other device) = false, want true")
	}
}

// TestAuthorizedActor_RefusesClaimingFromMeForSomeoneElsesMessage
// covers the other direction: an incoming message's id must never be
// treated as ours to revoke or edit just because an event claims
// IsFromMe.
func TestAuthorizedActor_RefusesClaimingFromMeForSomeoneElsesMessage(t *testing.T) {
	t.Parallel()

	key := messageKey{senderID: "15551234567@s.whatsapp.net"}
	info := types.MessageInfo{MessageSource: types.MessageSource{IsFromMe: true}}
	if authorizedActor(t.Context(), &fakeDevice{}, key, true, info) {
		t.Error("authorizedActor(fromMe claim over someone else's message) = true, want false")
	}
}
