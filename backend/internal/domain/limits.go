package domain

import "unicode/utf8"

// limits.go bounds every sender-controlled collection a message or its
// media carries, so a connector that passes through whatever a service
// reports can never make one message grow enough rich text, mentions,
// reactions or poll options to slow the UI's layout pass, or carry a
// link preview's text, or a link's own URL, past anything a legitimate
// page's own metadata would ever need (see ui/lib/Format.js and
// docs/decisions.md). SanitizeIncoming is the one place every cap here
// is applied, used by app.Ingest for both a newly arrived message and
// one the service reports edited, so neither path can drift from the
// other's limits.

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

// MaxReactionEmojiLength bounds one reaction chip's own emoji text, in
// UTF-16 code units. Real emoji, including one combined from several
// code points (a skin tone or gender modifier, a flag, a family), need
// only a handful of units; a sender's own ReactionMessage.Text carries
// whatever they put there, with no length limit of its own, so a chip
// beyond this is dropped outright rather than truncated - cutting an
// emoji mid-cluster would render as mojibake, not a shorter valid one.
const MaxReactionEmojiLength = 32

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

// MaxURLLength bounds a link's own URL - a formatting span's URL or a
// link preview's Media.URL - in bytes. 2048 is the long-standing
// practical web limit (Internet Explorer's own historical cap, still
// the figure most servers, crawlers and link-preview generators were
// built around), so no legitimate link a page or a sender's own client
// generates needs anything near it. A sender fully controls both
// fields, with no length limit of their own, so a URL beyond this is
// dropped outright, the same drop-rather-than-truncate treatment
// CapReactions already gives an oversized emoji: cutting a URL changes
// where it points, rather than merely shortening what it says, so a
// truncated one must never be kept.
const MaxURLLength = 2048

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

// MaxDisplayNameLength bounds a sender-controlled display name, in
// UTF-16 code units: a contact's own name, a push name, a group's
// title, a message's SenderName, a reply quote's SenderName, or a
// group member's own name. Generous for any real name or group title
// - shown on a single line wherever it appears - while still bounding
// what a hostile sender's own profile or group settings can make a
// connector pass through unchanged.
const MaxDisplayNameLength = 256

// MaxStickerEmojiLength bounds a sticker's own Emoji fallback text, in
// UTF-16 code units, the same reasoning as MaxReactionEmojiLength: a
// real emoji, even one combined from several code points, needs only a
// handful of units, but WhatsApp's StickerMessage.Emojis and
// Telegram's DocumentAttributeSticker.Alt carry whatever the sender put
// there, with no length limit of their own. ui/components/StickerView.qml
// renders this as a single, unwrapped line whenever the sticker has no
// thumbnail or downloaded image, so an unbounded value stalls the
// shell's layout pass the same way an unbounded reaction chip would.
const MaxStickerEmojiLength = 32

// MaxFileNameLength bounds a sender-controlled file name, in bytes:
// generous for any real attachment's own name, shown as a single line
// in the file view, while still bounding what a hostile sender's own
// metadata can make a connector pass through unchanged. A byte count,
// not UTF-16 units, since a file name is a path component rather than
// text meant to be measured the way a message's own Text is; CapFileName
// still never splits the UTF-8 encoding of one rune across the cut.
const MaxFileNameLength = 255

// MaxMediaDimension bounds a photo, video or sticker's own claimed
// Width and Height, in pixels: a sender's own metadata, with no limit
// of its own, read only for display (ui/lib/Format.js's photoSize)
// and to tell one attachment from another
// (ui/lib/Timeline.js's mediaIdentityChanged), never for sizing a
// decode - that always goes through the file's own verified size
// instead (see docs/decisions.md). A security review found the UI
// had nowhere else to turn for a real GIF video before that fix, so
// this is defence in depth for any future sourceSize calculation that
// ever again reads this claim directly: no real photo or video from
// either service approaches this, so it only ever bounds a hostile
// claim like 2000000x1, never a legitimate one.
const MaxMediaDimension = 1 << 16 // 65536

// MaxInlineThumbLen bounds a message's own embedded preview
// (Media.Thumb), in bytes of its base64 text, the one piece of a
// message's media a connector embeds directly in the message itself
// rather than something media.fetch downloads only once the user asks
// for it. The open conversation's shell keeps every loaded message's
// own Thumb in memory for as long as it stays open (see
// ui/controllers/MessageTimeline.qml), so a sender flooding a chat can
// make it hold one of these per message, unbounded, for however many
// it sends - unlike a downloaded sticker's own bytes, which land on
// disk, not the shell's memory, and still pass imagesafe.Thumbnail's
// own, much larger bound first, at the connector. Real previews are a
// handful of kilobytes; 64 KiB of base64 text is generous for one
// while keeping a flood of them far short of what imagesafe.Thumbnail
// alone would still let through.
const MaxInlineThumbLen = 64 << 10

// MaxPreviewLength bounds a conversation's own Preview, in UTF-16 code
// units: ui/lib/Format.js's previewText only ever shows the first 200
// characters of it, prefixed in a group by the sender's own name, so a
// margin above that covers every prefix the UI adds without ever
// needing to store or send a message's full text, which a sender fully
// controls (see store.bumpConversation and store.scanConversationWith).
const MaxPreviewLength = 300

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
// capRanges), keeps at most MaxFormattingSpans of what remains, and
// drops any kept span's own URL past MaxURLLength (see CapURL): the
// span's range and kind stay, since the UI already treats a link span
// with no URL as plain text.
func CapSpans(spans []Span, textLen int) []Span {
	kept := capRanges(spans, textLen, MaxFormattingSpans, func(s Span) capRange {
		return capRange{s.Start, s.End}
	})

	for i := range kept {
		kept[i].URL = CapURL(kept[i].URL)
	}

	return kept
}

// CapMentions drops a mention that no longer fits text (see capRanges),
// keeps at most MaxMentions of what remains, and caps each kept
// mention's own Name at MaxDisplayNameLength: unlike SenderName, a
// mention's Name has no length limit of its own at the connector, and
// one hostile name can arrive mentioned up to MaxMentions times in a
// single message.
func CapMentions(mentions []Mention, textLen int) []Mention {
	kept := capRanges(mentions, textLen, MaxMentions, func(m Mention) capRange {
		return capRange{m.Offset, m.Offset + m.Length}
	})

	for i := range kept {
		kept[i].Name = CapDisplayName(kept[i].Name)
	}

	return kept
}

// CapReactions drops a reaction chip whose own emoji text runs past
// MaxReactionEmojiLength, then keeps at most MaxReactions of what
// remains. A connector folds a live reaction change into its message's
// full tally itself (WhatsApp reports one person's reaction at a time,
// never a conversation's own tally), so this is the one place that
// tally is bounded before it ever reaches the UI, matching whatever
// bound, if any, the connector's own cache already applies.
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

// CapDisplayName cuts s to MaxDisplayNameLength, the one cap every
// sender-controlled display name - a contact's name, a push name, a
// group's title, a message's or reply quote's SenderName, a member's
// own name - goes through once, wherever it enters the helper.
func CapDisplayName(s string) string {
	return capText(s, MaxDisplayNameLength)
}

// CapPreview cuts s to MaxPreviewLength, the one cap a conversation's
// own Preview goes through: once when store.bumpConversation first
// stores it, and again on every later read (see
// store.scanConversationWith), so a row a release before this cap
// existed already wrote in full is repaired the same way CapDisplayName
// repairs a legacy Title or PreviewSender.
func CapPreview(s string) string {
	return capText(s, MaxPreviewLength)
}

// CapFileName cuts s to at most MaxFileNameLength bytes, never
// splitting a rune's own UTF-8 encoding across the cut.
func CapFileName(s string) string {
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
// rather than truncating it: a formatting span's and a link preview's
// own URL both carry whatever a sender's client put there, with no
// length limit of their own, and cutting a URL short changes where it
// points rather than merely shortening what it says.
func CapURL(url string) string {
	if len(url) > MaxURLLength {
		return ""
	}

	return url
}

// CapMedia bounds a poll's options and text, a link preview's own
// text fields and URL, a file name, or a claimed width and height, in
// place, returning media unchanged for a nil media. FileName is
// capped for every kind that carries one (file, video, voice note or
// sticker), not only one media.Kind, since a sender names their own
// attachment the same way whichever kind it is. URL is capped for
// every kind too, even though only a link preview sets it today, so a
// future kind that starts carrying one is covered without a second
// change. Width and Height are capped at MaxMediaDimension the same
// way, for every kind: defence in depth, since the UI itself never
// sizes a decode from either field any more (see
// MaxMediaDimension's own doc comment).
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

// capDimension clamps n to [0, MaxMediaDimension]: a negative claim
// drops to zero the same as an unknown size already reads, and one
// above the cap drops to the cap itself, never truncated some other
// way - there is no "closer to the truth" value for a claim this far
// from anything a real file could be.
func capDimension(n int) int {
	if n < 0 {
		return 0
	}
	if n > MaxMediaDimension {
		return MaxMediaDimension
	}

	return n
}

// CapPoll bounds poll's own question length and its option count and
// each option's text length, the same cap CapMedia gives a poll
// carried as part of a message. Ingest.PollUpdated calls this
// directly for a poll reported changing on its own, without a full
// message - a live vote or a tally update - the same way Ingest.Reacted
// calls domain.CapReactions for a reaction tally reported the same way.
func CapPoll(poll Poll) Poll {
	capPoll(&poll)
	return poll
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
// service reports edited, and store.scanMessage calls it again on
// every read, so a row an older release stored before these caps
// existed is repaired every time it is read back, not only for a
// message ingested since - no path can diverge from another's caps.
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
	}

	return m
}
