package signal

import (
	"fmt"
	"strconv"

	"github.com/google/uuid"
)

// messageRemoteID is a message's remote id: sender aci plus sent
// timestamp, since Signal's wire protocol carries no id of its own.
func messageRemoteID(aci uuid.UUID, sentTimestamp uint64) string {
	return aci.String() + ":" + strconv.FormatUint(sentTimestamp, 10)
}

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
