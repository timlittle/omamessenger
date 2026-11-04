package whatsapp

// authorize.go decides whether a live edit or revoke protocol message
// is allowed to act on the message it names. WhatsApp's protocol
// never checks that an edit or revoke's sender is the same person who
// sent the targeted message, so without a check here, anyone in a
// chat could rewrite or delete anyone else's message (see docs/decisions.md).

import (
	"context"

	"go.mau.fi/whatsmeow/types"
)

// authorizedActor reports whether info names the one sender allowed
// to act on a message, given key (whoever message_keys recorded as
// its original sender, see keys.go) and found (whether a key was
// recorded at all). A phone-number and hidden-id (LID) form of the
// same person are treated as equal through personID's normalisation;
// this account is also accepted for a message it originally sent
// itself. found false is never authorized, since there is nothing
// recorded to check against.
//
// Intentionally never authorizes a group admin deleting someone
// else's message: WhatsApp's wire format carries no field marking a
// revoke as an admin action (see docs/decisions.md).
func authorizedActor(ctx context.Context, dev device, key messageKey, found bool, info types.MessageInfo) bool {
	if !found {
		return false
	}

	if key.fromMe || info.IsFromMe {
		return key.fromMe && info.IsFromMe
	}

	sender, err := jidFromRemoteID(key.senderID)
	if err != nil {
		return false
	}

	return personID(ctx, dev, sender) == personID(ctx, dev, info.Sender)
}
