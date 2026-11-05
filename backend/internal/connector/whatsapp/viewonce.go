package whatsapp

// This file isolates view-once detection, split out of
// normalize_message.go: message still calls these to decide whether to
// drop a photo, video or voice note's media and show a placeholder
// instead (see domain.ViewOncePlaceholder).

import (
	"go.mau.fi/whatsmeow/proto/waE2E"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// isViewOnceWrapped reports whether msg, before unwrap peels it, is
// wrapped in one of WhatsApp's view-once envelopes, including one
// nested inside an ephemeral wrapper: whatsmeow's own live event
// dispatch already peels a view-once wrapper before a connector ever
// sees it live, carrying the signal instead on the inner media's own
// ViewOnce field (see mediaViewOnce), but a history sync's
// WebMessageInfo never goes through that, so the wrapper itself is
// still what to look for there.
func isViewOnceWrapped(msg *waE2E.Message) bool {
	switch {
	case msg.GetViewOnceMessage().GetMessage() != nil,
		msg.GetViewOnceMessageV2().GetMessage() != nil,
		msg.GetViewOnceMessageV2Extension().GetMessage() != nil:
		return true
	case msg.GetEphemeralMessage().GetMessage() != nil:
		return isViewOnceWrapped(msg.GetEphemeralMessage().GetMessage())
	default:
		return false
	}
}

// mediaViewOnce reports whether content's own media, once unwrapped,
// carries WhatsApp's per-message view-once flag: how a live message
// still says it was view-once once whatsmeow has already peeled its
// wrapper away.
func mediaViewOnce(content *waE2E.Message) bool {
	switch {
	case content.GetImageMessage() != nil:
		return content.GetImageMessage().GetViewOnce()
	case content.GetVideoMessage() != nil:
		return content.GetVideoMessage().GetViewOnce()
	case content.GetPtvMessage() != nil:
		return content.GetPtvMessage().GetViewOnce()
	case content.GetAudioMessage() != nil:
		return content.GetAudioMessage().GetViewOnce()
	default:
		return false
	}
}

// viewOnceKind names the kind of media a view-once message carried, for
// domain.ViewOncePlaceholder's wording; a message with no media kind
// this connector recognises reads as a photo, the common case.
func viewOnceKind(content *waE2E.Message) string {
	switch {
	case content.GetVideoMessage() != nil, content.GetPtvMessage() != nil:
		return domain.MediaVideo
	case content.GetAudioMessage() != nil:
		return domain.MediaVoice
	default:
		return domain.MediaPhoto
	}
}
