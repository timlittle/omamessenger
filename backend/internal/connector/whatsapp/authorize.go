package whatsapp

// authorize.go decides whether a live edit or revoke protocol message
// is allowed to act on the message it names. WhatsApp's end-to-end
// encryption authenticates who sent a stanza, but the protocol itself
// never checks that an edit or revoke's own sender is the same person
// who sent the message it targets: the edit or revoke travels inside
// the sender's own encrypted payload, so WhatsApp's server cannot
// check it either, and whatsmeow only unwraps it. Without a check here,
// anyone in a chat could rewrite or delete anyone else's message by
// naming its id (see docs/decisions.md).

import (
	"context"

	"go.mau.fi/whatsmeow/types"
)

// authorizedActor reports whether info names the one sender allowed to
// act on a message, given key, whoever message_keys recorded as that
// message's sender when it first arrived (see keys.go), and found,
// whether a key was recorded at all. A phone-number and hidden-id (LID)
// form of the same person are treated as equal through personID's own
// normalisation; this account itself is also accepted when the
// original message was also sent by this account, so an edit or
// revoke made from one of this account's own other linked devices
// still works. found false - an id this connector never saved a key
// for - is never authorized: there is nothing recorded to check the
// claimed edit or revoke against, so the safer answer is to drop it
// rather than apply it on trust.
//
// This intentionally never authorizes a group admin deleting someone
// else's message: WhatsApp's wire format carries no field that marks a
// revoke as an admin action rather than an ordinary one (anyone can set
// a revoke's Participant to anyone's id; see docs/decisions.md), and
// this account's own outgoing DeleteMessages already refuses the same
// case for a revoke this account tries to send (see delete.go's
// errCannotRevoke) for the same reason.
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
