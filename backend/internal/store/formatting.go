package store

// formatting.go encodes and decodes a message's rich-text formatting
// spans for the messages table, the same JSON-in-a-column pattern
// mentions.go already uses.

import (
	"encoding/json"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// encodeFormatting stores a message's formatting spans as JSON, or ""
// for none.
func encodeFormatting(spans []domain.Span) (string, error) {
	if len(spans) == 0 {
		return "", nil
	}

	b, err := json.Marshal(spans)

	return string(b), err
}

// decodeFormatting reads a message's formatting spans back from its
// stored JSON, or reports none for "".
func decodeFormatting(encoded string) ([]domain.Span, error) {
	if encoded == "" {
		return nil, nil
	}

	var spans []domain.Span
	err := json.Unmarshal([]byte(encoded), &spans)

	return spans, err
}
