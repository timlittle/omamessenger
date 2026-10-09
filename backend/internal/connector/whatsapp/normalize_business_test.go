package whatsapp

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestHandleHistorySync_TitlesAnUnsavedBusinessWithItsVerifiedName
// confirms a business account, such as a shop sending order updates,
// which is rarely saved in the user's own contacts so dev.contactName
// finds nothing for it, is titled with its verified name carried on
// the synced message itself, rather than falling back to its phone
// number or "Unknown contact".
func TestHandleHistorySync_TitlesAnUnsavedBusinessWithItsVerifiedName(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev, media := newFakeDevice(), newTestMediaStore(t)
	var sink connectortest.Sink

	msg := historyMsg("H1", "Continue this conversation on the app", false)
	msg.Message.VerifiedBizName = strPtr("Acme Shop")
	e := &events.HistorySync{Data: &waHistorySync.HistorySync{
		Conversations: []*waHistorySync.Conversation{{
			ID: strPtr("15559876543@s.whatsapp.net"), Messages: []*waHistorySync.HistorySyncMsg{msg},
		}},
	}}
	c.handleHistorySync(t.Context(), &sink, dev, media, e)

	if !sink.Has("conversation 15559876543@s.whatsapp.net Acme Shop") {
		t.Errorf("events = %q, want the chat titled with the business's verified name", sink.Lines())
	}
}

// TestMessageText_ExtractsBusinessInteractiveKinds confirms messageText
// reads the human-visible text out of every WhatsApp Business message
// kind this file was added to handle, rather than falling back to the
// generic "[Message]" placeholder for each of them.
func TestMessageText_ExtractsBusinessInteractiveKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  *waE2E.Message
		want string
	}{
		{
			"interactive header media caption, no body or header title",
			&waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{
				Header: &waE2E.InteractiveMessage_Header{
					Media: &waE2E.InteractiveMessage_Header_ImageMessage{ImageMessage: &waE2E.ImageMessage{Caption: strPtr("Continue this conversation on the app")}},
				},
			}},
			"Continue this conversation on the app",
		},
		{
			"interactive header document caption",
			&waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{
				Header: &waE2E.InteractiveMessage_Header{
					Media: &waE2E.InteractiveMessage_Header_DocumentMessage{DocumentMessage: &waE2E.DocumentMessage{Caption: strPtr("Your invoice")}},
				},
			}},
			"Your invoice",
		},
		{
			"carousel joins each card's own text",
			&waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{
				InteractiveMessage: &waE2E.InteractiveMessage_CarouselMessage_{CarouselMessage: &waE2E.InteractiveMessage_CarouselMessage{
					Cards: []*waE2E.InteractiveMessage{
						{Body: &waE2E.InteractiveMessage_Body{Text: strPtr("Card one")}},
						{Header: &waE2E.InteractiveMessage_Header{Title: strPtr("Card two")}},
					},
				}},
			}},
			"Card one; Card two",
		},
		{
			"carousel with nothing in any card falls back to the placeholder",
			&waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{
				InteractiveMessage: &waE2E.InteractiveMessage_CarouselMessage_{CarouselMessage: &waE2E.InteractiveMessage_CarouselMessage{
					Cards: []*waE2E.InteractiveMessage{{}},
				}},
			}},
			"[Message]",
		},
		{
			"template's newer interactive-message format",
			&waE2E.Message{TemplateMessage: &waE2E.TemplateMessage{
				Format: &waE2E.TemplateMessage_InteractiveMessageTemplate{
					InteractiveMessageTemplate: &waE2E.InteractiveMessage{Body: &waE2E.InteractiveMessage_Body{Text: strPtr("Your order shipped")}},
				},
			}},
			"Your order shipped",
		},
		{
			"hydrated template falls back to its title, then its footer",
			&waE2E.Message{TemplateMessage: &waE2E.TemplateMessage{
				HydratedTemplate: &waE2E.TemplateMessage_HydratedFourRowTemplate{
					Title:              &waE2E.TemplateMessage_HydratedFourRowTemplate_HydratedTitleText{HydratedTitleText: "Order update"},
					HydratedFooterText: strPtr("Thanks for shopping"),
				},
			}},
			"Order update",
		},
		{
			"buttons response echoes the picked button's display text",
			&waE2E.Message{ButtonsResponseMessage: &waE2E.ButtonsResponseMessage{
				Response: &waE2E.ButtonsResponseMessage_SelectedDisplayText{SelectedDisplayText: "Yes, cancel my order"},
			}},
			"Yes, cancel my order",
		},
		{
			"interactive response echoes its own body text",
			&waE2E.Message{InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{
				Body: &waE2E.InteractiveResponseMessage_Body{Text: strPtr("Track my order")},
			}},
			"Track my order",
		},
		{
			"list message prefers its title over its description",
			&waE2E.Message{ListMessage: &waE2E.ListMessage{Title: strPtr("Pick a size"), Description: strPtr("Choose one")}},
			"Pick a size",
		},
		{
			"list message falls back to its description with no title",
			&waE2E.Message{ListMessage: &waE2E.ListMessage{Description: strPtr("Choose one")}},
			"Choose one",
		},
		{
			"list response echoes the picked row's title",
			&waE2E.Message{ListResponseMessage: &waE2E.ListResponseMessage{Title: strPtr("Medium")}},
			"Medium",
		},
		{
			"list response falls back to its description with no title",
			&waE2E.Message{ListResponseMessage: &waE2E.ListResponseMessage{Description: strPtr("Size: Medium")}},
			"Size: Medium",
		},
		{
			"order prefers the business's own note over its title",
			&waE2E.Message{OrderMessage: &waE2E.OrderMessage{Message: strPtr("Packed and ready"), OrderTitle: strPtr("Order #123")}},
			"Packed and ready",
		},
		{
			"order falls back to its title with no note",
			&waE2E.Message{OrderMessage: &waE2E.OrderMessage{OrderTitle: strPtr("Order #123")}},
			"Order #123",
		},
		{
			"product prefers the attached body over the product's own title",
			&waE2E.Message{ProductMessage: &waE2E.ProductMessage{
				Body:    strPtr("Here is what you asked about"),
				Product: &waE2E.ProductMessage_ProductSnapshot{Title: strPtr("Running shoes")},
			}},
			"Here is what you asked about",
		},
		{
			"product falls back to its own title with no body",
			&waE2E.Message{ProductMessage: &waE2E.ProductMessage{Product: &waE2E.ProductMessage_ProductSnapshot{Title: strPtr("Running shoes")}}},
			"Running shoes",
		},
		{
			"highly structured reads its nested hydrated template",
			&waE2E.Message{HighlyStructuredMessage: &waE2E.HighlyStructuredMessage{
				HydratedHsm: &waE2E.TemplateMessage{HydratedTemplate: &waE2E.TemplateMessage_HydratedFourRowTemplate{HydratedContentText: strPtr("Delivered")}},
			}},
			"Delivered",
		},
		{
			"highly structured with no hydrated template falls back to the placeholder",
			&waE2E.Message{HighlyStructuredMessage: &waE2E.HighlyStructuredMessage{}},
			"[Message]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := messageText(tt.msg); got != tt.want {
				t.Errorf("messageText(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
