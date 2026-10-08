package whatsapp

// normalize_business.go extracts the human-visible text from WhatsApp
// Business's own interactive message kinds that businessText (see
// normalize_message.go) does not read directly: a carousel or a
// header carrying its own media, a list a business sent or the row a
// person picked from one, an order, a shared product, a legacy
// highly-structured template and the two kinds of button response a
// person's own reply arrives as. Each extractor reads only the field
// WhatsApp's own apps show as that kind's visible text, in the same
// fallback order they use, and never an internal id such as a row,
// button or product id.

import (
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
)

// syncedBusinessName is the verified business name carried by whichever
// of sc's own synced messages first has one, or "" when none does.
// History sync carries a message's verified business name in a
// different proto field than a live message does (see
// historyMessageInfo and verifiedName), so this reads it directly
// rather than going through a types.MessageInfo built for something
// else.
func syncedBusinessName(sc *waHistorySync.Conversation) string {
	for _, hm := range sc.GetMessages() {
		if name := hm.GetMessage().GetVerifiedBizName(); name != "" {
			return name
		}
	}

	return ""
}

// headerMediaCaption is the caption on the photo, document or video a
// business attached to an interactive message's header, or "" when
// the header carries no media, or media with no caption of its own.
func headerMediaCaption(h *waE2E.InteractiveMessage_Header) string {
	switch {
	case h.GetImageMessage() != nil:
		return h.GetImageMessage().GetCaption()
	case h.GetDocumentMessage() != nil:
		return h.GetDocumentMessage().GetCaption()
	case h.GetVideoMessage() != nil:
		return h.GetVideoMessage().GetCaption()
	default:
		return ""
	}
}

// carouselText is a carousel message's own visible text: its cards'
// body text, header title or footer text, each read the same way a
// plain interactive message's is, joined in order with "; " so a
// person still sees something of every card rather than only the
// first. Each card is read one level deep only, never recursing into
// a card that is itself, implausibly, another carousel.
func carouselText(c *waE2E.InteractiveMessage_CarouselMessage) string {
	var texts []string
	for _, card := range c.GetCards() {
		if text := cardText(card); text != "" {
			texts = append(texts, text)
		}
	}

	return strings.Join(texts, "; ")
}

// cardText is one carousel card's own visible text, read the same way
// interactiveText reads a plain interactive message's body, header
// title and footer, but without carouselText's own recursion.
func cardText(card *waE2E.InteractiveMessage) string {
	if text := card.GetBody().GetText(); text != "" {
		return text
	}
	if title := card.GetHeader().GetTitle(); title != "" {
		return title
	}

	return card.GetFooter().GetText()
}

// commerceText is the visible text of one of WhatsApp Business's own
// commerce message kinds: a list or the row picked from one, an
// order, a shared product, or a legacy highly-structured template
// notification. It is "" for anything else, split out of businessText
// (see normalize_message.go) so neither function's own switch grows
// past this codebase's complexity limit.
func commerceText(msg *waE2E.Message) string {
	switch {
	case msg.GetListMessage() != nil:
		return listText(msg.GetListMessage())
	case msg.GetListResponseMessage() != nil:
		return listResponseText(msg.GetListResponseMessage())
	case msg.GetOrderMessage() != nil:
		return orderText(msg.GetOrderMessage())
	case msg.GetProductMessage() != nil:
		return productText(msg.GetProductMessage())
	case msg.GetHighlyStructuredMessage() != nil:
		return highlyStructuredText(msg.GetHighlyStructuredMessage())
	default:
		return ""
	}
}

// listText is a list message's own visible text: its title, falling
// back to its description, since a business may send either without
// the other.
func listText(m *waE2E.ListMessage) string {
	if title := m.GetTitle(); title != "" {
		return title
	}

	return m.GetDescription()
}

// listResponseText is the text WhatsApp echoes back for the row a
// person picked from a list: its title, falling back to its
// description, never the row's own internal id.
func listResponseText(m *waE2E.ListResponseMessage) string {
	if title := m.GetTitle(); title != "" {
		return title
	}

	return m.GetDescription()
}

// orderText is an order message's own visible text: the note a
// business attached to the order, falling back to the order's title.
func orderText(m *waE2E.OrderMessage) string {
	if text := m.GetMessage(); text != "" {
		return text
	}

	return m.GetOrderTitle()
}

// productText is a shared product's own visible text: the message
// body a business attached, falling back to the product's own title.
func productText(m *waE2E.ProductMessage) string {
	if body := m.GetBody(); body != "" {
		return body
	}

	return m.GetProduct().GetTitle()
}

// highlyStructuredText is a legacy template notification's own visible
// text: the modern hydrated template WhatsApp nests inside it for a
// client that can render one, or "" when a message carries only the
// older template's own placeholders and parameters, which are not
// text meant to be shown as received.
func highlyStructuredText(m *waE2E.HighlyStructuredMessage) string {
	return templateText(m.GetHydratedHsm())
}

// buttonsResponseText is the display text WhatsApp echoes back for the
// button a person picked, never the button's own internal id.
func buttonsResponseText(m *waE2E.ButtonsResponseMessage) string {
	return m.GetSelectedDisplayText()
}

// interactiveResponseText is the display text WhatsApp echoes back for
// a reply to an interactive message: its own body text. A native-flow
// response carries only machine parameters for whatever flow it
// answered, nothing a person would read.
func interactiveResponseText(m *waE2E.InteractiveResponseMessage) string {
	return m.GetBody().GetText()
}
