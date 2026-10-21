package domain

// limits.go bounds every sender-controlled collection a message or its
// media carries, so a connector that passes through whatever a service
// reports can never make one message grow enough rich text, mentions,
// reactions or poll options to slow the UI's layout pass, or carry a
// link preview's text past anything a legitimate page's own metadata
// would ever need (see ui/lib/Format.js and docs/decisions.md).
// SanitizeIncoming is the one place every cap here is applied, used by
// app.Ingest for both a newly arrived message and one the service
// reports edited, so neither path can drift from the other's limits.

// MaxFormattingSpans bounds how many rich-text spans (links, bold,
// italic, underline, strike, code, pre, spoiler) one message's
// Formatting carries. Real messages carry a handful; even heavily
// formatted text rarely passes a few dozen, so this is generous while
// still bounding what a hostile sender's entity list can make a
// connector pass through unchanged.
const MaxFormattingSpans = 256

// MaxMentions bounds how many "@name" tokens one message's Mentions
// carries. Both services cap how many accounts a single message can
// actually mention well below this.
const MaxMentions = 64

// MaxReactions bounds how many distinct emoji chips one message's
// Reactions carries. A conversation's reaction palette is realistically
// a handful of emoji; a chip beyond this adds nothing the space under a
// message could still usefully show.
const MaxReactions = 64

// MaxPollOptions bounds how many choices one poll carries. Telegram's
// own clients cap a poll at 10 options; this leaves room for a future
// service with a higher cap while still bounding a hostile poll's size.
const MaxPollOptions = 64

// MaxPollOptionTextLength bounds one poll option's own text, in UTF-16
// code units: generous for any real answer, far below what would make
// an option's own row need more than a couple of lines.
const MaxPollOptionTextLength = 300

// MaxPollQuestionLength bounds a poll's own question, in UTF-16 code
// units, the same reasoning as MaxPollOptionTextLength.
const MaxPollQuestionLength = 300

// MaxLinkPreviewFieldLength bounds a link preview's title and site
// name, in UTF-16 code units: both are shown on a single line, so
// neither needs to be longer than a long headline.
const MaxLinkPreviewFieldLength = 300

// MaxLinkPreviewDescriptionLength bounds a link preview's description,
// in UTF-16 code units: shown as a short snippet under the title, never
// a full article - the page a link points to fully controls this text,
// the same as a sender controls their own message.
const MaxLinkPreviewDescriptionLength = 2000

// MaxGroupMembers bounds how many members Members ever returns for one
// conversation: generous for any real group a person actually
// participates in, while bounding the memory and layout cost a
// maliciously inflated member list could otherwise force on the
// mention picker.
const MaxGroupMembers = 5000

// MaxContactResults bounds how many contacts one search ever returns:
// generous for any real address book, while bounding what a service
// that floods the local contact sync with fabricated entries could
// make one search return.
const MaxContactResults = 1000

// capRange is a [start, end) range over Text in UTF-16 code units, the
// shape a formatting span's and a mention's own position share.
type capRange struct {
	start, end int
}

// capRanges drops any item whose range runs outside [0, textLen) - its
// text no longer exists once TruncateIncomingText has cut the message
// down - then keeps at most max of whatever remains, in order.
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

// CapSpans drops a formatting span that no longer fits text (see
// capRanges) and keeps at most MaxFormattingSpans of what remains.
func CapSpans(spans []Span, textLen int) []Span {
	return capRanges(spans, textLen, MaxFormattingSpans, func(s Span) capRange {
		return capRange{s.Start, s.End}
	})
}

// CapMentions drops a mention that no longer fits text (see capRanges)
// and keeps at most MaxMentions of what remains.
func CapMentions(mentions []Mention, textLen int) []Mention {
	return capRanges(mentions, textLen, MaxMentions, func(m Mention) capRange {
		return capRange{m.Offset, m.Offset + m.Length}
	})
}

// CapReactions keeps at most MaxReactions of a message's reaction
// chips.
func CapReactions(reactions []Reaction) []Reaction {
	if len(reactions) <= MaxReactions {
		return reactions
	}

	return reactions[:MaxReactions]
}

// CapMedia bounds a poll's options and text, or a link preview's own
// text fields, in place, returning media unchanged for a nil media or
// any other kind, neither of which carries a sender-controlled text
// field this package bounds.
func CapMedia(media *Media) *Media {
	if media == nil {
		return nil
	}

	switch media.Kind {
	case MediaPoll:
		capPoll(media.Poll)
	case MediaLink:
		media.SiteName = capText(media.SiteName, MaxLinkPreviewFieldLength)
		media.Title = capText(media.Title, MaxLinkPreviewFieldLength)
		media.Description = capText(media.Description, MaxLinkPreviewDescriptionLength)
	}

	return media
}

// capPoll bounds a poll's own question length and its option count and
// each option's text length, in place. It does nothing for a nil poll:
// a message whose Kind is MediaPoll always carries one, but CapMedia
// never assumes that holds.
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

// capText cuts s to max UTF-16 code units, never splitting a UTF-8
// rune or a surrogate pair's own two units across the cut, the same
// rune-safe treatment TruncateIncomingText gives a message's own text.
// Checking whether a rune would push the running total past max,
// rather than waiting for it to land on max exactly, matters once a
// rune outside the Basic Multilingual Plane counts as two units: one
// of those can otherwise jump straight over max without ever equalling
// it.
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
// collection a message or its media carries, so a hostile sender can
// never grow a single message past what the UI can safely render.
// app.Ingest calls this for both a newly arrived message and one the
// service reports edited, so neither path can diverge from the other's
// caps.
func SanitizeIncoming(m Message) Message {
	m.Text = TruncateIncomingText(m.Text)
	textLen := UTF16Len(m.Text)

	m.Formatting = CapSpans(m.Formatting, textLen)
	m.Mentions = CapMentions(m.Mentions, textLen)
	m.Reactions = CapReactions(m.Reactions)
	m.Media = CapMedia(m.Media)

	return m
}
