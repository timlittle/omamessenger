package domain

import (
	"strings"
	"unicode/utf8"
)

// limits.go bounds every sender-controlled collection a message or its
// media carries, so a hostile sender can never grow one message past
// what the UI can safely render. SanitizeIncoming applies every cap
// here, for both a newly arrived message and an edit.

// MaxFormattingSpans bounds how many rich-text spans one message's
// Formatting carries.
const MaxFormattingSpans = 256

// MaxMentions bounds how many "@name" tokens one message's Mentions
// carries.
const MaxMentions = 64

// MaxReactions bounds how many distinct emoji chips one message's
// Reactions carries.
const MaxReactions = 64

// MaxReactionEmojiLength bounds one reaction chip's own emoji text, in
// UTF-16 code units. A chip beyond this is dropped outright rather than
// truncated, since cutting an emoji mid-cluster would render as mojibake.
const MaxReactionEmojiLength = 32

// MaxPollOptions bounds how many choices one poll carries.
const MaxPollOptions = 64

// MaxPollOptionTextLength bounds one poll option's own text, in UTF-16
// code units.
const MaxPollOptionTextLength = 300

// MaxPollQuestionLength bounds a poll's own question, in UTF-16 code
// units.
const MaxPollQuestionLength = 300

// MaxLinkPreviewFieldLength bounds a link preview's title and site
// name, in UTF-16 code units.
const MaxLinkPreviewFieldLength = 300

// MaxLinkPreviewDescriptionLength bounds a link preview's description,
// in UTF-16 code units.
const MaxLinkPreviewDescriptionLength = 2000

// MaxURLLength bounds a link's own URL - a formatting span's URL or a
// link preview's Media.URL - in bytes. A URL beyond this is dropped
// outright rather than truncated, since cutting it changes where it
// points.
const MaxURLLength = 2048

// MaxGroupMembers bounds how many members Members ever returns for one
// conversation.
const MaxGroupMembers = 5000

// MaxContactResults bounds how many contacts one search ever returns.
const MaxContactResults = 1000

// MaxDisplayNameLength bounds a sender-controlled display name, in
// UTF-16 code units: a contact's name, a push name, a group's title,
// or a message's SenderName.
const MaxDisplayNameLength = 256

// MaxStickerEmojiLength bounds a sticker's own Emoji fallback text, in
// UTF-16 code units, the same reasoning as MaxReactionEmojiLength.
const MaxStickerEmojiLength = 32

// MaxFileNameLength bounds a sender-controlled file name, in bytes
// rather than UTF-16 units, since a file name is a path component, not
// display text.
const MaxFileNameLength = 255

// MaxMediaDimension bounds a photo, video or sticker's own claimed
// Width and Height, in pixels. These are read only for display and
// identity, never for sizing a decode (see docs/decisions.md); this is
// defence in depth against any future code that reads the claim directly.
const MaxMediaDimension = 1 << 16 // 65536

// MaxInlineThumbLen bounds a message's own embedded preview
// (Media.Thumb), in bytes of its base64 text. The open conversation
// keeps every loaded message's Thumb in memory, so this bounds what a
// flood of messages can hold open at once.
const MaxInlineThumbLen = 64 << 10

// MaxRemoteIDLength bounds a sender- or service-chosen identifier that
// is not display text: a mention's UserID, or a reply quote's
// RemoteID. Generous for the longest real id either service produces,
// while still bounding a hostile claim neither service's own parsing
// limits.
const MaxRemoteIDLength = 64

// CapRemoteID drops id entirely once it runs past MaxRemoteIDLength
// rather than truncating it: a cut id could read as a different,
// shorter real id it was never sent as.
func CapRemoteID(id string) string {
	if len(id) > MaxRemoteIDLength {
		return ""
	}

	return id
}

// ValidMessageIDs reports whether m's own RemoteID and SenderID both
// sit within MaxRemoteIDLength. RemoteID is the dedup key other
// messages match against, so it must be rejected outright rather than
// truncated or cleared like the fields SanitizeIncoming repairs in place.
func ValidMessageIDs(m Message) bool {
	return len(m.RemoteID) <= MaxRemoteIDLength && len(m.SenderID) <= MaxRemoteIDLength
}

// MaxPreviewLength bounds a conversation's own Preview, in UTF-16 code
// units.
const MaxPreviewLength = 300

// capRange is a [start, end) range over Text in UTF-16 code units, the
// shape a formatting span's and a mention's own position share.
type capRange struct {
	start, end int
}

// capRanges drops any item whose range runs outside [0, textLen), then
// keeps at most max of whatever remains, in order.
func capRanges[T any](items []T, textLen, max int, rangeOf func(T) capRange) []T {
	if len(items) == 0 {
		return items
	}

	limit := max
	if len(items) < limit {
		limit = len(items)
	}

	kept := make([]T, 0, limit)
	for _, item := range items {
		r := rangeOf(item)
		if r.start < 0 || r.end < r.start || r.end > textLen {
			continue
		}

		kept = append(kept, item)
		if len(kept) == max {
			break
		}
	}

	return kept
}

// CapSpans drops a formatting span that no longer fits text, keeps at
// most MaxFormattingSpans of what remains, and drops any kept span's
// own URL past MaxURLLength.
func CapSpans(spans []Span, textLen int) []Span {
	kept := capRanges(spans, textLen, MaxFormattingSpans, func(s Span) capRange {
		return capRange{s.Start, s.End}
	})

	for i := range kept {
		kept[i].URL = CapURL(kept[i].URL)
	}

	return kept
}

// CapMentions drops a mention that no longer fits text, keeps at most
// MaxMentions of what remains, caps each kept mention's Name at
// MaxDisplayNameLength, and clears its UserID past MaxRemoteIDLength.
// A mention with its UserID cleared is still kept, since its Name,
// Offset and Length still describe real text in the message.
func CapMentions(mentions []Mention, textLen int) []Mention {
	kept := capRanges(mentions, textLen, MaxMentions, func(m Mention) capRange {
		return capRange{m.Offset, m.Offset + m.Length}
	})

	for i := range kept {
		kept[i].Name = CapDisplayName(kept[i].Name)
		kept[i].UserID = CapRemoteID(kept[i].UserID)
	}

	return kept
}

// CapReactions drops a reaction chip whose own emoji text runs past
// MaxReactionEmojiLength, then keeps at most MaxReactions of what
// remains.
func CapReactions(reactions []Reaction) []Reaction {
	kept := make([]Reaction, 0, min(len(reactions), MaxReactions))
	for _, r := range reactions {
		if UTF16Len(r.Emoji) > MaxReactionEmojiLength {
			continue
		}

		kept = append(kept, r)
		if len(kept) == MaxReactions {
			break
		}
	}

	return kept
}

// CapDisplayName cuts s to MaxDisplayNameLength after stripping every
// control or bidi/format character (see stripControlAndFormatRunes).
func CapDisplayName(s string) string {
	return capText(stripControlAndFormatRunes(s), MaxDisplayNameLength)
}

// isControlOrFormatRune reports whether r is a character a display
// name or file name must never carry unescaped: an ASCII control
// character, or a Unicode bidi/invisible-format character that could
// disguise a dangerous file extension as a safe one.
func isControlOrFormatRune(r rune) bool {
	return isASCIIControlRune(r) || isBidiOrFormatRune(r)
}

// isASCIIControlRune reports whether r is an ASCII control character,
// including every newline form (0x00-0x1F, and DEL, 0x7F).
func isASCIIControlRune(r rune) bool {
	return r <= 0x1F || r == 0x7F
}

// bidiOrFormatRanges are the inclusive Unicode rune ranges
// isBidiOrFormatRune flags, each a [low, high] pair.
var bidiOrFormatRanges = [...][2]rune{
	{0x200B, 0x200F},
	{0x202A, 0x202E},
	{0x2060, 0x2064},
	{0x2066, 0x2069},
	{0xFEFF, 0xFEFF},
}

// isBidiOrFormatRune reports whether r is one of the Unicode bidi
// override or invisible-format characters isControlOrFormatRune also
// flags. U+200D, the zero-width joiner, is kept: stripping it would
// break multi-codepoint emoji apart.
func isBidiOrFormatRune(r rune) bool {
	if r == 0x200D {
		return false
	}

	for _, rng := range bidiOrFormatRanges {
		if r >= rng[0] && r <= rng[1] {
			return true
		}
	}

	return false
}

// stripControlAndFormatRunes drops every rune isControlOrFormatRune
// flags from s, keeping every other rune, including a kept zero-width
// joiner, exactly where it was.
func stripControlAndFormatRunes(s string) string {
	if !strings.ContainsFunc(s, isControlOrFormatRune) {
		return s
	}

	var b strings.Builder
	for _, r := range s {
		if !isControlOrFormatRune(r) {
			b.WriteRune(r)
		}
	}

	return b.String()
}

// CapPreview cuts s to MaxPreviewLength.
func CapPreview(s string) string {
	return capText(s, MaxPreviewLength)
}

// CapFileName cuts s to at most MaxFileNameLength bytes, never
// splitting a rune's UTF-8 encoding across the cut, after stripping
// every control or bidi/format character.
func CapFileName(s string) string {
	s = stripControlAndFormatRunes(s)
	if len(s) <= MaxFileNameLength {
		return s
	}

	cut := MaxFileNameLength
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}

	return s[:cut]
}

// CapURL drops url entirely once it runs past MaxURLLength bytes,
// rather than truncating it, since cutting a URL short changes where
// it points.
func CapURL(url string) string {
	if len(url) > MaxURLLength {
		return ""
	}

	return url
}

// CapMedia bounds a poll's options and text, a link preview's text
// fields and URL, a file name, and a claimed width and height, in
// place, returning media unchanged for a nil media. FileName, URL,
// Width and Height are capped for every kind, not just the kinds that
// use them today, so a future kind is covered automatically.
func CapMedia(media *Media) *Media {
	if media == nil {
		return nil
	}

	media.FileName = CapFileName(media.FileName)
	media.URL = CapURL(media.URL)
	media.Width = capDimension(media.Width)
	media.Height = capDimension(media.Height)
	if len(media.Thumb) > MaxInlineThumbLen {
		media.Thumb = ""
	}

	switch media.Kind {
	case MediaPoll:
		capPoll(media.Poll)
	case MediaLink:
		media.SiteName = capText(media.SiteName, MaxLinkPreviewFieldLength)
		media.Title = capText(media.Title, MaxLinkPreviewFieldLength)
		media.Description = capText(media.Description, MaxLinkPreviewDescriptionLength)
	case MediaSticker:
		if UTF16Len(media.Emoji) > MaxStickerEmojiLength {
			media.Emoji = ""
		}
	}

	return media
}

// capDimension clamps n to [0, MaxMediaDimension].
func capDimension(n int) int {
	if n < 0 {
		return 0
	}
	if n > MaxMediaDimension {
		return MaxMediaDimension
	}

	return n
}

// CapPoll bounds poll's question length, option count and each
// option's text length. Ingest.PollUpdated calls this for a poll
// reported changing on its own, without a full message.
func CapPoll(poll Poll) Poll {
	capPoll(&poll)
	return poll
}

// capPoll bounds a poll's question length, option count and each
// option's text length, in place. It does nothing for a nil poll.
func capPoll(poll *Poll) {
	if poll == nil {
		return
	}

	poll.Question = capText(poll.Question, MaxPollQuestionLength)
	if len(poll.Options) > MaxPollOptions {
		poll.Options = poll.Options[:MaxPollOptions]
	}

	for i, opt := range poll.Options {
		poll.Options[i].Text = capText(opt.Text, MaxPollOptionTextLength)
	}
}

// capText cuts s to max UTF-16 code units, never splitting a rune or a
// surrogate pair across the cut.
func capText(s string, max int) string {
	units := 0
	for i, r := range s {
		if units+utf16RuneLen(r) > max {
			return s[:i]
		}

		units += utf16RuneLen(r)
	}

	return s
}

// SanitizeIncoming truncates text and caps every sender-controlled
// collection a message or its media carries. app.Ingest calls this for
// both a newly arrived message and an edit; store.scanMessage calls it
// again on every read, so an older stored row is repaired too.
func SanitizeIncoming(m Message) Message {
	m.Text = TruncateIncomingText(m.Text)
	textLen := UTF16Len(m.Text)

	m.SenderName = CapDisplayName(m.SenderName)
	m.Formatting = CapSpans(m.Formatting, textLen)
	m.Mentions = CapMentions(m.Mentions, textLen)
	m.Reactions = CapReactions(m.Reactions)
	m.Media = CapMedia(m.Media)
	if m.ReplyTo != nil {
		m.ReplyTo.SenderName = CapDisplayName(m.ReplyTo.SenderName)
		m.ReplyTo.Text = Excerpt(m.ReplyTo.Text)
		m.ReplyTo.RemoteID = CapRemoteID(m.ReplyTo.RemoteID)
		m.ReplyTo.SenderID = CapRemoteID(m.ReplyTo.SenderID)
	}

	return m
}
