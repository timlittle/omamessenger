package signal

import (
	"encoding/base64"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/imagesafe"
)

// genericSenderName is shown for a sender this connector has not yet
// resolved a name for, matching how the other connectors show one.
const genericSenderName = "Signal user"

// genericGroupTitle stands in for a group whose details could not be
// fetched; the store refuses a conversation without a title.
const genericGroupTitle = "Signal group"

// noteToSelfTitle names the chat with this account itself, as Signal does.
const noteToSelfTitle = "Note to Self"

// normalizeDataMessage turns a DataMessage into the domain message
// events.go reports. The returned pointer, when not nil, is the
// attachment to remember for a later FetchMedia (see fetch.go).
func normalizeDataMessage(aci uuid.UUID, senderName string, sentTimestamp uint64, msg *signalpb.DataMessage, outgoing bool) (domain.Message, *signalpb.AttachmentPointer) {
	m := domain.Message{
		RemoteID:   messageRemoteID(aci, sentTimestamp),
		SenderID:   senderRemoteID(aci),
		SenderName: senderName,
		Text:       msg.GetBody(),
		Outgoing:   outgoing,
		Created:    int64(sentTimestamp),
	}

	if outgoing {
		m.Status = domain.StatusSent
	} else {
		m.Status = domain.StatusReceived
	}

	m.Formatting = bodyRanges(msg.GetBodyRanges())
	m.Mentions = mentions(msg.GetBodyRanges())

	if seconds := msg.GetExpireTimer(); seconds > 0 {
		m.ExpireAfter = int64(seconds)
		m.ArmOnRead = !outgoing
	}

	ptr := applyMedia(&m, msg)

	return m, ptr
}

// applyMedia fills m's Media or ViewOnce flag from msg's one
// attachment, sticker, link preview or poll, and returns its pointer.
func applyMedia(m *domain.Message, msg *signalpb.DataMessage) *signalpb.AttachmentPointer {
	switch {
	case msg.GetIsViewOnce():
		m.ViewOnce = true
		return nil
	case msg.GetSticker() != nil:
		m.Media = stickerMedia(msg.GetSticker())
		return msg.GetSticker().GetData()
	case len(msg.GetAttachments()) > 0:
		m.Media = attachmentMedia(msg.GetAttachments()[0])
		return msg.GetAttachments()[0]
	case msg.GetPollCreate() != nil:
		m.Media = pollCreateMedia(msg.GetPollCreate())
		return nil
	case len(msg.GetPreview()) > 0:
		m.Media = previewMedia(msg.GetPreview()[0])
		return nil
	default:
		return nil
	}
}

// attachmentMedia maps a photo, video, voice note or file attachment,
// keeping its CDN pointer fields for a later FetchMedia (see fetch.go).
func attachmentMedia(ptr *signalpb.AttachmentPointer) *domain.Media {
	media := &domain.Media{
		Kind:     attachmentKind(ptr),
		FileName: ptr.GetFileName(),
		Size:     int64(ptr.GetSize()),
		Width:    int(ptr.GetWidth()),
		Height:   int(ptr.GetHeight()),
		Thumb:    sanitizedThumb(ptr.GetThumbnail()),
	}

	if media.Kind == domain.MediaVoice {
		media.Duration = int(ptr.GetAudioDurationSeconds())
	}
	if hasFlag(ptr.GetFlags(), uint32(signalpb.AttachmentPointer_GIF)) {
		media.Animated = true
	}

	return media
}

// attachmentKind decides a domain media kind from an attachment
// pointer's content type and flags.
func attachmentKind(ptr *signalpb.AttachmentPointer) string {
	if hasFlag(ptr.GetFlags(), uint32(signalpb.AttachmentPointer_VOICE_MESSAGE)) {
		return domain.MediaVoice
	}

	switch {
	case startsWith(ptr.GetContentType(), "image/"):
		return domain.MediaPhoto
	case startsWith(ptr.GetContentType(), "video/"):
		return domain.MediaVideo
	default:
		return domain.MediaFile
	}
}

// hasFlag reports whether flags has every bit set in bit.
func hasFlag(flags, bit uint32) bool {
	return flags&bit == bit
}

// startsWith reports whether s begins with prefix, without importing
// strings for one call site.
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// stickerMedia maps a sticker to its own media kind, keeping its CDN
// pointer for FetchMedia.
func stickerMedia(sticker *signalpb.DataMessage_Sticker) *domain.Media {
	return &domain.Media{
		Kind:  domain.MediaSticker,
		Emoji: sticker.GetEmoji(),
		Thumb: sanitizedThumb(sticker.GetData().GetThumbnail()),
	}
}

// previewMedia maps a link preview's title, description and image.
func previewMedia(p *signalpb.Preview) *domain.Media {
	url := p.GetUrl()
	if !domain.AllowedLinkURL(url) {
		url = ""
	}

	return &domain.Media{
		Kind:        domain.MediaFile,
		URL:         url,
		Title:       p.GetTitle(),
		Description: p.GetDescription(),
		Thumb:       sanitizedThumb(p.GetImage().GetThumbnail()),
	}
}

// pollCreateMedia maps a new poll's question and options, with every
// option starting at zero votes: live votes update it afterwards (see
// poll.go).
func pollCreateMedia(create *signalpb.DataMessage_PollCreate) *domain.Media {
	options := make([]domain.PollOption, len(create.GetOptions()))
	for i, text := range create.GetOptions() {
		options[i] = domain.PollOption{ID: formatOptionID(i), Text: text}
	}

	return &domain.Media{
		Kind: domain.MediaPoll,
		Poll: &domain.Poll{
			Question:       create.GetQuestion(),
			Options:        options,
			MultipleChoice: create.GetAllowMultiple(),
		},
	}
}

// formatOptionID turns a poll option's index into the stable id the
// UI and PollUpdater address it by.
func formatOptionID(index int) string {
	const base = 'a'
	return string(rune(base + index))
}

// sanitizedThumb base64-encodes an embedded preview image once
// sanitized, or "" when there is none or it fails sanitization.
func sanitizedThumb(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	safe, _, _, ok := imagesafe.Sanitize(data, imagesafe.Thumbnail)
	if !ok {
		return ""
	}

	return base64.StdEncoding.EncodeToString(safe)
}

// bodyRanges maps Signal's style ranges to the UI's formatting spans,
// dropping anything not naming a style.
func bodyRanges(ranges []*signalpb.BodyRange) []domain.Span {
	var spans []domain.Span

	for _, r := range ranges {
		kind, ok := spanKind(r.GetStyle())
		if !ok {
			continue
		}

		start := int(r.GetStart())
		spans = append(spans, domain.Span{Kind: kind, Start: start, End: start + int(r.GetLength())})
	}

	return spans
}

// spanKind maps one BodyRange style to the matching domain span kind.
func spanKind(style signalpb.BodyRange_Style) (string, bool) {
	switch style {
	case signalpb.BodyRange_BOLD:
		return domain.SpanBold, true
	case signalpb.BodyRange_ITALIC:
		return domain.SpanItalic, true
	case signalpb.BodyRange_SPOILER:
		return domain.SpanSpoiler, true
	case signalpb.BodyRange_STRIKETHROUGH:
		return domain.SpanStrike, true
	case signalpb.BodyRange_MONOSPACE:
		return domain.SpanCode, true
	default:
		return "", false
	}
}

// outgoingBodyRanges is bodyRanges and mentions run in reverse: it
// builds the BodyRange list a DataMessage sends for m's formatting
// spans and mentions. The UI already measures every span and mention
// against the same literal text it put in m.Text (an @-mention is a
// literal "@name" substring, the same convention WhatsApp's own
// connector uses), so no text rewriting is needed here, only the wire
// ranges that mark it.
func outgoingBodyRanges(spans []domain.Span, mentionsList []domain.Mention) []*signalpb.BodyRange {
	ranges := make([]*signalpb.BodyRange, 0, len(spans)+len(mentionsList))

	for _, s := range spans {
		style, ok := outgoingSpanStyle(s.Kind)
		if !ok {
			continue
		}

		ranges = append(ranges, &signalpb.BodyRange{
			Start: u32p(uint32(s.Start)), Length: u32p(uint32(s.End - s.Start)),
			AssociatedValue: &signalpb.BodyRange_Style_{Style: style},
		})
	}

	for _, m := range mentionsList {
		ranges = append(ranges, &signalpb.BodyRange{
			Start: u32p(uint32(m.Offset)), Length: u32p(uint32(m.Length)),
			AssociatedValue: &signalpb.BodyRange_MentionAci{MentionAci: m.UserID},
		})
	}

	if len(ranges) == 0 {
		return nil
	}

	return ranges
}

// outgoingSpanStyle is spanKind run in reverse: the BodyRange style a
// formatting span's own kind sends as, or found false for a kind
// Signal's own clients do not render (such as an underline, which
// Signal has no style for).
func outgoingSpanStyle(kind string) (signalpb.BodyRange_Style, bool) {
	switch kind {
	case domain.SpanBold:
		return signalpb.BodyRange_BOLD, true
	case domain.SpanItalic:
		return signalpb.BodyRange_ITALIC, true
	case domain.SpanSpoiler:
		return signalpb.BodyRange_SPOILER, true
	case domain.SpanStrike:
		return signalpb.BodyRange_STRIKETHROUGH, true
	case domain.SpanCode:
		return signalpb.BodyRange_MONOSPACE, true
	default:
		return 0, false
	}
}

// mentions maps Signal's "@-mention" style ranges to the UI's
// mentions, naming each by id; the UI resolves the display name.
func mentions(ranges []*signalpb.BodyRange) []domain.Mention {
	var out []domain.Mention

	for _, r := range ranges {
		aci := r.GetMentionAci()
		if aci == "" {
			continue
		}

		out = append(out, domain.Mention{
			UserID: aci,
			Offset: int(r.GetStart()),
			Length: int(r.GetLength()),
		})
	}

	return out
}
