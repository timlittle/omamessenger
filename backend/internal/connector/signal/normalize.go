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

// normalizeDataMessage turns a DataMessage into the domain message
// events.go's event handler reports, given the sender and chat already
// known from the event's own envelope and senderName already resolved
// (or genericSenderName when nothing has resolved one yet). outgoing
// is true only for this account's own sent messages, carried on a sync
// message rather than received live. The returned pointer, when not
// nil, is the attachment events.go should remember for a later
// MediaFetcher.FetchMedia (see fetch.go); never set for a link
// preview, which already carries everything FetchMedia would add.
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

// applyMedia fills m's Media or ViewOnce flag from msg's attachment,
// sticker, link preview or poll, in that priority: a message carries
// at most one of these, matching every other connector's one-Media
// model. It returns the attachment pointer behind an attachment or
// sticker, for the caller to remember for FetchMedia.
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
// imagesafe has sanitized it, so nothing unsanitized ever reaches the
// UI. Returns "" when there is nothing embedded or it fails
// sanitization.
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

// mentions maps Signal's "@-mention" style ranges to the UI's
// mentions, naming the mentioned account by its id: the connector has
// no name resolved for every possible member at message-normalize
// time, so the UI resolves a display name from its own member list.
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
