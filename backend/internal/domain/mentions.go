package domain

// Mention is one "@name" token in a message's text: who it names, and
// where it sits in Text. Offset and Length are in UTF-16 code units, the
// unit both Telegram's message entities and a JavaScript string's own
// indices use, so the UI can place and highlight a mention with no
// conversion and a connector that needs byte positions in the UTF-8
// Text (see ByteOffsetsForUTF16) converts only at its own edge.
type Mention struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
}

// Member is one participant of a group conversation: enough to name
// them in a mention picker and to address a mention back to the
// service. ID is opaque to the UI, in whatever form the owning
// connector's own remote ids take.
type Member struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// utf16RuneLen is how many UTF-16 code units r encodes as: two for a
// rune outside the Basic Multilingual Plane, which needs a surrogate
// pair, one for every other rune.
func utf16RuneLen(r rune) int {
	if r > 0xFFFF {
		return 2
	}

	return 1
}

// UTF16Len counts s in UTF-16 code units, the length a JavaScript string
// would report for the same text.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16RuneLen(r)
	}

	return n
}

// ByteOffsetsForUTF16 converts a UTF-16 offset and length into the byte
// range they address in s, or reports false when the range runs past
// the end of s. offset and length are in UTF-16 code units, as a
// Mention's own Offset and Length are; this is what a connector whose
// wire format has no concept of UTF-16 positions (WhatsApp's mention
// text, unlike Telegram's entities) uses to splice s by a mention's
// position.
func ByteOffsetsForUTF16(s string, offset, length int) (start, end int, ok bool) {
	if offset < 0 || length < 0 {
		return 0, 0, false
	}

	start, end = -1, -1
	units := 0
	for i, r := range s {
		if units == offset {
			start = i
		}
		if units == offset+length {
			end = i
		}

		units += utf16RuneLen(r)
	}

	if units == offset {
		start = len(s)
	}
	if units == offset+length {
		end = len(s)
	}

	if start == -1 || end == -1 {
		return 0, 0, false
	}

	return start, end, true
}
