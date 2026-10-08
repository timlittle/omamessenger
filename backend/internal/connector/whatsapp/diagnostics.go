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
	"errors"
	"log"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
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

// logSystemChatDropped reports a message in WhatsApp's own "0" system
// chat (see titleFallback) that was not stored, and why: that chat's
// only content is WhatsApp's own announcements and security notices,
// so a report that they never appear is worth telling apart from an
// everyday dropped message, without ever naming the chat by its JID,
// which this line has no need for since there is only the one.
func logSystemChatDropped(reason, fields string) {
	log.Printf("whatsapp: system chat message dropped (reason=%s, fields=%s)", reason, fields)
}

// logContentlessDrop reports a message that carried nothing a person
// sent (see isContentless): as logSystemChatDropped when chat is
// WhatsApp's own "0" system account, so its announcements and
// security notices missing from the conversation list is never
// confused with an everyday contentless drop, or as an ordinary
// logDropped otherwise.
func logContentlessDrop(chat types.JID, content *waE2E.Message) {
	if chat == types.PSAJID {
		logSystemChatDropped(reasonContentless, fieldPaths(content))
		return
	}

	logDropped(reasonContentless)
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

// expiredDownloadErrs are the whatsmeow download errors WhatsApp's CDN
// returns once a message's media link has aged out: the case retry.go
// asks the primary phone to fix by re-uploading, rather than one this
// connector can do anything else about on its own.
var expiredDownloadErrs = []error{
	whatsmeow.ErrMediaDownloadFailedWith404,
	whatsmeow.ErrMediaDownloadFailedWith410,
}

// isExpiredDownload reports whether err is one of expiredDownloadErrs.
func isExpiredDownload(err error) bool {
	for _, want := range expiredDownloadErrs {
		if errors.Is(err, want) {
			return true
		}
	}

	return false
}

// downloadFailureClass is the safe category logDownloadFailed reports
// for one failed download attempt: "decrypt" for a hash or HMAC
// mismatch, "expired" for the 404 or 410 that sends retry.go to the
// primary phone, "http" for any other status WhatsApp's server
// answered with, and "network" for a failure that never reached it at
// all.
func downloadFailureClass(err error) string {
	var httpErr whatsmeow.DownloadHTTPError

	switch {
	case isDecryptFailure(err):
		return "decrypt"
	case isExpiredDownload(err):
		return "expired"
	case errors.As(err, &httpErr):
		return "http"
	default:
		return "network"
	}
}

// logDownloadFailed reports a failed media download attempt by its
// safe class alone (see downloadFailureClass), never the path, JID or
// key the attempt carried.
func logDownloadFailed(class string) {
	log.Printf("whatsapp: media download failed (class=%s)", class)
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
	"interactiveMessage": true, "buttonsMessage": true, "buttonsResponseMessage": true,
	"interactiveResponseMessage": true,
	"templateMessage":            true, "templateButtonReplyMessage": true,
	"listMessage": true, "listResponseMessage": true, "orderMessage": true,
	"productMessage": true, "highlyStructuredMessage": true,
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

// fieldPathDepth bounds how many levels populatedFieldPaths descends
// into a message's own nested fields, so logPlaceholderMessage's line
// can never grow unbounded chasing a deeply nested proto.
const fieldPathDepth = 3

// logPlaceholderMessage reports that a message still fell back to the
// generic "[Message]" placeholder (see messageText), and which proto
// fields it populated, by name alone, up to fieldPathDepth deep, so a
// business message kind this connector has not been taught to show
// real text for yet can be found and fixed. fields never carries a
// field's own value, only that something at that path is set.
func logPlaceholderMessage(fields string) {
	log.Printf("whatsapp: placeholder message (fields=%s)", fields)
}

// fieldPaths is the comma-separated, depth-bounded field-name paths
// msg populated, for logPlaceholderMessage and logContentlessDrop.
func fieldPaths(msg *waE2E.Message) string {
	return strings.Join(populatedFieldPaths(msg.ProtoReflect(), fieldPathDepth), ",")
}

// populatedFieldPaths lists the dotted field-name paths msg populated,
// each read through reflection alone and never a field's own value: a
// nested message field is expanded into "<field>.<nested field>" up to
// depth levels deep, so a diagnostic can tell, say, an interactive
// message's unrecognised card shape apart from its header alone,
// while a list, map or a nested field with nothing populated inside
// it is reported by its own field name only.
func populatedFieldPaths(msg protoreflect.Message, depth int) []string {
	if depth <= 0 {
		return nil
	}

	var paths []string
	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		name := string(fd.Name())
		if fd.Kind() == protoreflect.MessageKind && !fd.IsList() && !fd.IsMap() {
			if nested := populatedFieldPaths(v.Message(), depth-1); len(nested) > 0 {
				for _, n := range nested {
					paths = append(paths, name+"."+n)
				}

				return true
			}
		}
		paths = append(paths, name)

		return true
	})

	return paths
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
