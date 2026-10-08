package store

// mentions.go encodes and decodes a message's @-mention tokens for the
// messages table, the same JSON-in-a-column pattern reply_to and
// reactions already use (see messages.go and reactions.go).

import (
	"encoding/json"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// encodeMentions stores a message's mentions as JSON, or "" for none.
func encodeMentions(mentions []domain.Mention) (string, error) {
	if len(mentions) == 0 {
		return "", nil
	}

	b, err := json.Marshal(mentions)

	return string(b), err
}

// decodeMentions reads a message's mentions back from its stored JSON,
// or reports none for "".
func decodeMentions(encoded string) ([]domain.Mention, error) {
	if encoded == "" {
		return nil, nil
	}

	var mentions []domain.Mention
	err := json.Unmarshal([]byte(encoded), &mentions)

	return mentions, err
}
