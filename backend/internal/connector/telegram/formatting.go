package telegram

// formatting.go turns Telegram's text formatting entities into
// domain.Span, mirroring how mentions.go turns mention entities into
// domain.Mention. Telegram's own offsets are already in UTF-16 code
// units, the unit domain.Span uses throughout, so this needs no
// byte-position conversion for the simple, offset-only entities;
// MessageEntityURL and MessageEntityEmail, which carry only a
// position and need the linked text itself, still read through
// domain.ByteOffsetsForUTF16 to slice the UTF-8 Go string by it.

import (
	"strings"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// linkSchemes are the URL schemes a link span may open. Telegram's own
// entities should never carry anything else, but a message from a
// future client or a bot might, and this connector never turns an
// unknown scheme into something the UI would treat as safe to open.
var linkSchemes = map[string]bool{"http": true, "https": true, "mailto": true, "tg": true}

// spansFromEntities reads a message's formatting entities into spans
// over text: links, bold, italic, underline, strike, code, pre and
// spoiler. A mention entity, handled separately by mentions.go, and
// anything else this connector does not map are skipped.
func spansFromEntities(ents []tg.MessageEntityClass, text string) []domain.Span {
	var out []domain.Span

	for _, ent := range ents {
		if span, ok := linkSpan(ent, text); ok {
			out = append(out, span)
			continue
		}

		if kind, offset, length, ok := plainSpan(ent); ok {
			out = append(out, domain.Span{Kind: kind, Start: offset, End: offset + length})
		}
	}

	return out
}

// plainSpan maps a formatting entity that carries only its own
// position - bold, italic, underline, strike, code, pre or spoiler -
// to the span kind it becomes, or reports false for a link, mention or
// other entity it does not handle.
func plainSpan(ent tg.MessageEntityClass) (kind string, offset, length int, ok bool) {
	switch e := ent.(type) {
	case *tg.MessageEntityBold:
		return domain.SpanBold, e.Offset, e.Length, true
	case *tg.MessageEntityItalic:
		return domain.SpanItalic, e.Offset, e.Length, true
	case *tg.MessageEntityUnderline:
		return domain.SpanUnderline, e.Offset, e.Length, true
	case *tg.MessageEntityStrike:
		return domain.SpanStrike, e.Offset, e.Length, true
	case *tg.MessageEntityCode:
		return domain.SpanCode, e.Offset, e.Length, true
	case *tg.MessageEntityPre:
		return domain.SpanPre, e.Offset, e.Length, true
	case *tg.MessageEntitySpoiler:
		return domain.SpanSpoiler, e.Offset, e.Length, true
	default:
		return "", 0, 0, false
	}
}

// linkSpan turns a link-shaped entity into a link span: a
// MessageEntityTextURL already carries its own URL, while
// MessageEntityURL and MessageEntityEmail carry only a position, so
// their link text is read out of text itself. It reports false for any
// other entity, or for a link whose URL allowedLinkURL rejects.
func linkSpan(ent tg.MessageEntityClass, text string) (domain.Span, bool) {
	switch e := ent.(type) {
	case *tg.MessageEntityTextURL:
		return linkSpanAt(e.Offset, e.Length, e.URL)
	case *tg.MessageEntityURL:
		raw, ok := entityText(text, e.Offset, e.Length)
		if !ok {
			return domain.Span{}, false
		}
		return linkSpanAt(e.Offset, e.Length, raw)
	case *tg.MessageEntityEmail:
		raw, ok := entityText(text, e.Offset, e.Length)
		if !ok {
			return domain.Span{}, false
		}
		return linkSpanAt(e.Offset, e.Length, "mailto:"+raw)
	default:
		return domain.Span{}, false
	}
}

// linkSpanAt builds a link span at the UTF-16 range
// [offset, offset+length) for url, or reports false when
// allowedLinkURL rejects its scheme.
func linkSpanAt(offset, length int, url string) (domain.Span, bool) {
	if !allowedLinkURL(url) {
		return domain.Span{}, false
	}

	return domain.Span{Kind: domain.SpanLink, Start: offset, End: offset + length, URL: url}, true
}

// entityText reads the UTF-16 range [offset, offset+length) of text,
// converting it to the byte range domain.ByteOffsetsForUTF16 reports,
// or false when the range runs past the end of text.
func entityText(text string, offset, length int) (string, bool) {
	start, end, ok := domain.ByteOffsetsForUTF16(text, offset, length)
	if !ok {
		return "", false
	}

	return text[start:end], true
}

// allowedLinkURL reports whether url's scheme is one linkSchemes allows
// a link span to open.
func allowedLinkURL(url string) bool {
	scheme, _, ok := strings.Cut(url, ":")

	return ok && linkSchemes[strings.ToLower(scheme)]
}
