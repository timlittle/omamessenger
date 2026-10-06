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
func inputPeer(id string) (tg.InputPeerClass, error) {
	parts := strings.Split(id, ":")
	numbers := make([]int64, 0, 2)
	for _, part := range parts[1:] {
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
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
