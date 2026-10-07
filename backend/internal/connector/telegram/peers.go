package telegram

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gotd/td/tg"
)

// errBadRemoteID reports a conversation id this connector did not make.
var errBadRemoteID = errors.New("telegram: not a Telegram conversation id")

// remoteID writes a Telegram peer as a conversation's remote id. Users and
// channels carry their access hash, which Telegram needs before it will
// send to them, so a stored conversation can be sent to without fetching
// anything first.
func remoteID(peer tg.InputPeerClass) string {
	switch p := peer.(type) {
	case *tg.InputPeerUser:
		return fmt.Sprintf("user:%d:%d", p.UserID, p.AccessHash)
	case *tg.InputPeerChat:
		return fmt.Sprintf("chat:%d", p.ChatID)
	case *tg.InputPeerChannel:
		return fmt.Sprintf("channel:%d:%d", p.ChannelID, p.AccessHash)
	default:
		return ""
	}
}

// inputPeer reads a remote id back into the peer Telegram's API takes.
// Each number must be in the exact form remoteID writes it in, such as
// "0" rather than "00" or "+0": a remote id is compared as a string
// elsewhere, such as when the store recognises a redelivered message, so
// a non-canonical id that parses to the same number but round-trips to a
// different string would break that comparison.
func inputPeer(id string) (tg.InputPeerClass, error) {
	parts := strings.Split(id, ":")
	numbers := make([]int64, 0, 2)
	for _, part := range parts[1:] {
		n, ok := canonicalInt64(part)
		if !ok {
			return nil, fmt.Errorf("%w: %q", errBadRemoteID, id)
		}

		numbers = append(numbers, n)
	}

	switch {
	case parts[0] == "user" && len(numbers) == 2:
		return &tg.InputPeerUser{UserID: numbers[0], AccessHash: numbers[1]}, nil
	case parts[0] == "chat" && len(numbers) == 1:
		return &tg.InputPeerChat{ChatID: numbers[0]}, nil
	case parts[0] == "channel" && len(numbers) == 2:
		return &tg.InputPeerChannel{ChannelID: numbers[0], AccessHash: numbers[1]}, nil
	default:
		return nil, fmt.Errorf("%w: %q", errBadRemoteID, id)
	}
}

// canonicalInt64 parses s as a number, reporting false if s is not the
// exact decimal form strconv.FormatInt would produce for it.
func canonicalInt64(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || strconv.FormatInt(n, 10) != s {
		return 0, false
	}

	return n, true
}
