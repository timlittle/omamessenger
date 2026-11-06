package signal

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// messageRemoteID is a message's remote id: sender aci plus sent
// timestamp, since Signal's wire protocol carries no id of its own.
func messageRemoteID(aci uuid.UUID, sentTimestamp uint64) string {
	return aci.String() + ":" + strconv.FormatUint(sentTimestamp, 10)
}

// errMalformedMessageRemoteID reports a message remote id that is not
// the "aci:timestamp" shape messageRemoteID builds.
var errMalformedMessageRemoteID = errors.New("signal: malformed message id")

// parseMessageRemoteID reverses messageRemoteID, for an action that
// needs the sender and timestamp a message's own id already encodes:
// an edit, a delete, a reaction or a poll vote all target one this way,
// with no separate lookup needed.
func parseMessageRemoteID(id string) (aci uuid.UUID, sentTimestamp uint64, err error) {
	rawACI, rawTimestamp, found := strings.Cut(id, ":")
	if !found {
		return uuid.Nil, 0, fmt.Errorf("%w: %q", errMalformedMessageRemoteID, id)
	}

	aci, err = parseACI(rawACI)
	if err != nil {
		return uuid.Nil, 0, fmt.Errorf("%w: %q", errMalformedMessageRemoteID, id)
	}

	sentTimestamp, err = strconv.ParseUint(rawTimestamp, 10, 64)
	if err != nil {
		return uuid.Nil, 0, fmt.Errorf("%w: %q", errMalformedMessageRemoteID, id)
	}

	return aci, sentTimestamp, nil
}

// boolp takes the address of a bool value, for the generated protobuf
// structs that hold every optional field as a pointer.
func boolp(b bool) *bool { return &b }

// u32p takes the address of a uint32 value, for the same reason.
func u32p(n uint32) *uint32 { return &n }

// u64p takes the address of a uint64 value, for the same reason.
func u64p(n uint64) *uint64 { return &n }

// strp takes the address of a string value, for the same reason.
func strp(s string) *string { return &s }

// groupChatLength is how long a group's chat id is as signalmeow gives
// it: a base64-encoded 32-byte group identifier. A direct chat's id is
// a plain UUID (36 characters) or a PNI-prefixed one, always shorter.
const groupChatLength = 44

// isGroupChat reports whether chatID, as signalmeow's own
// events.MessageInfo.ChatID gives it, names a group rather than a
// direct chat.
func isGroupChat(chatID string) bool {
	return len(chatID) == groupChatLength
}

// senderRemoteID is the remote id this connector uses for a sender:
// their account identifier, exactly as Signal's own protocol gives it.
func senderRemoteID(aci uuid.UUID) string {
	return aci.String()
}

// parseACI parses s as a plain account identifier, the form a direct
// chat's remote id and a sender's remote id both take.
func parseACI(s string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("signal: parse account id %q: %w", s, err)
	}

	return parsed, nil
}
