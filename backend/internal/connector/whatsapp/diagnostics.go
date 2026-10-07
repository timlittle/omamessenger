package whatsapp

// diagnostics.go logs privacy-safe lines to stderr for the three things
// that can make a message silently never appear: content this
// connector deliberately drops and why, a message whatsmeow received
// but could not decrypt, and a message content kind this connector
// does not recognise yet. Every line here carries only a reason, a
// count or a proto field name, never a JID, a name or message text;
// tools/nologcontent enforces that mechanically for the domain types it
// knows about, and this file never passes one of those to log.Printf in
// the first place. No command-line flag gates this: the helper has no
// log-level setting to check, so these go to the same stderr every
// other diagnostic in this helper already uses (see docs/decisions.md).

import (
	"log"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// reasonContentless is logDropped's reason for a message that is one of
// WhatsApp's own protocol or system notices (see isContentless).
const reasonContentless = "contentless"

// reasonNoRealContent is logDropped's reason for a history-synced
// conversation this connector has never reported before, dropped
// because its synced messages carry nothing a person actually sent
// (see hasRealContent in history.go).
const reasonNoRealContent = "no-real-content"

// logDropped reports a message this connector deliberately did not
// store, and why, so a report of "messages missing" can be checked
// against what was dropped on purpose.
func logDropped(reason string) {
	log.Printf("whatsapp: dropped message (reason=%s)", reason)
}

// logUndecryptable reports a message whatsmeow received but could not
// decrypt (see live.go's handleUndecryptable), so a report of "messages
// missing" can be checked against how often this happens.
func logUndecryptable(unavailable bool, mode events.DecryptFailMode) {
	log.Printf("whatsapp: undecryptable message (unavailable=%v, mode=%s)", unavailable, mode)
}

// logUnknownKind reports a message content kind this connector does
// not recognise yet, by its proto field name alone.
func logUnknownKind(field string) {
	log.Printf("whatsapp: unknown message kind (field=%s)", field)
}

// logPhoneResend reports a message the primary phone resent after
// AutomaticMessageRerequestFromPhone asked it to (see device.go and
// live.go's handleContent), replacing a placeholder first reported as
// undecryptable, so a report of "messages missing" can confirm the
// phone actually answered the request.
func logPhoneResend() {
	log.Printf("whatsapp: phone resent an undecryptable message")
}

// recognizedContentFields are the waE2E.Message fields this connector
// already turns into a real message (see messageText and its helpers
// in normalize_message.go), handles as its own event (a reaction, an
// edit or a revoke; see normalize_events.go), drops as contentless
// housekeeping (see isContentless), or that unwrap already peels away
// before a connector ever has to recognise them, plus the handful of
// companion fields WhatsApp sends alongside real content (a session's
// key distribution, or MessageContextInfo's bot and device metadata).
// Anything else reaching mediaPlaceholder's generic "[Message]" is
// content this connector has never been taught to show.
var recognizedContentFields = map[string]bool{
	"conversation": true, "extendedTextMessage": true, "imageMessage": true,
	"videoMessage": true, "audioMessage": true, "documentMessage": true,
	"stickerMessage": true, "contactMessage": true, "contactsArrayMessage": true,
	"locationMessage": true, "liveLocationMessage": true,
	"groupInviteMessage": true, "stickerPackMessage": true,
	"interactiveMessage": true, "buttonsMessage": true,
	"templateMessage": true, "templateButtonReplyMessage": true,
	"albumMessage": true, "associatedChildMessage": true,
	"pollCreationMessage": true, "pollCreationMessageV2": true, "pollCreationMessageV3": true,
	"pollCreationMessageV4": true, "pollCreationMessageV5": true, "pollCreationMessageV6": true,
	"reactionMessage": true, "protocolMessage": true, "pollUpdateMessage": true,
	"pinInChatMessage": true, "keepInChatMessage": true,
	"ephemeralMessage": true, "viewOnceMessage": true, "viewOnceMessageV2": true,
	"viewOnceMessageV2Extension": true, "deviceSentMessage": true,
	"senderKeyDistributionMessage":               true,
	"fastRatchetKeySenderKeyDistributionMessage": true,
	"messageContextInfo":                         true,
	"call":                                       true, "callLogMesssage": true, "bcallMessage": true,
	"messageHistoryBundle": true, "messageHistoryNotice": true,
	"placeholderMessage": true, "secretEncryptedMessage": true,
	"groupRootKeyShare": true, "rootSecretDistributeMessage": true,
}

// unknownContentKind reports the proto field name of the first content
// msg carries that this connector does not recognise (see
// recognizedContentFields), or false when every field WhatsApp
// populated is already one this connector handles. It reads only field
// names through reflection, never a field's value, so this stays safe
// to log.
func unknownContentKind(msg *waE2E.Message) (string, bool) {
	var unknown string
	msg.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		if name := string(fd.Name()); unknown == "" && !recognizedContentFields[name] {
			unknown = name
		}

		return true
	})

	return unknown, unknown != ""
}
