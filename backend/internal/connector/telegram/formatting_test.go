package telegram

import (
	"slices"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestSpansFromEntities_MapsEachFormattingKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ent  tg.MessageEntityClass
		text string
		want domain.Span
	}{
		{"bold", &tg.MessageEntityBold{Offset: 0, Length: 4}, "bold text", domain.Span{Kind: domain.SpanBold, Start: 0, End: 4}},
		{"italic", &tg.MessageEntityItalic{Offset: 0, Length: 6}, "italic text", domain.Span{Kind: domain.SpanItalic, Start: 0, End: 6}},
		{"underline", &tg.MessageEntityUnderline{Offset: 0, Length: 9}, "underline text", domain.Span{Kind: domain.SpanUnderline, Start: 0, End: 9}},
		{"strike", &tg.MessageEntityStrike{Offset: 0, Length: 6}, "strike text", domain.Span{Kind: domain.SpanStrike, Start: 0, End: 6}},
		{"code", &tg.MessageEntityCode{Offset: 0, Length: 4}, "code text", domain.Span{Kind: domain.SpanCode, Start: 0, End: 4}},
		{"pre", &tg.MessageEntityPre{Offset: 0, Length: 5, Language: "go"}, "block text", domain.Span{Kind: domain.SpanPre, Start: 0, End: 5}},
		{"spoiler", &tg.MessageEntitySpoiler{Offset: 0, Length: 7}, "spoiler text", domain.Span{Kind: domain.SpanSpoiler, Start: 0, End: 7}},
		{
			"text url", &tg.MessageEntityTextURL{Offset: 0, Length: 4, URL: "https://example.com"}, "link text",
			domain.Span{Kind: domain.SpanLink, Start: 0, End: 4, URL: "https://example.com"},
		},
		{
			"bare url", &tg.MessageEntityURL{Offset: 0, Length: 19}, "https://example.com here",
			domain.Span{Kind: domain.SpanLink, Start: 0, End: 19, URL: "https://example.com"},
		},
		{
			"email", &tg.MessageEntityEmail{Offset: 0, Length: 15}, "ask@example.com please",
			domain.Span{Kind: domain.SpanLink, Start: 0, End: 15, URL: "mailto:ask@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := spansFromEntities([]tg.MessageEntityClass{tt.ent}, tt.text)
			if !slices.Equal(got, []domain.Span{tt.want}) {
				t.Errorf("spans = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSpansFromEntities_CountsOffsetsInUTF16CodeUnitsPastAnEmoji(t *testing.T) {
	t.Parallel()

	// "😀" lies outside the Basic Multilingual Plane and needs a
	// surrogate pair: two UTF-16 code units for one rune. A span placed
	// right after it proves the offset is read the way Telegram sends
	// it, in those units, rather than in Go's own byte or rune count,
	// either of which would land the span one or two characters into
	// "bold" instead of exactly at its start.
	text := "😀 bold"
	got := spansFromEntities([]tg.MessageEntityClass{&tg.MessageEntityBold{Offset: 3, Length: 4}}, text)
	want := []domain.Span{{Kind: domain.SpanBold, Start: 3, End: 7}}
	if !slices.Equal(got, want) {
		t.Errorf("spans = %+v, want %+v", got, want)
	}
}

func TestSpansFromEntities_DropsALinkWithADisallowedScheme(t *testing.T) {
	t.Parallel()

	got := spansFromEntities([]tg.MessageEntityClass{
		&tg.MessageEntityTextURL{Offset: 0, Length: 4, URL: "javascript:alert(1)"},
	}, "link")
	if got != nil {
		t.Errorf("spans = %+v, want none", got)
	}
}

func TestSpansFromEntities_AllowsATelegramDeepLink(t *testing.T) {
	t.Parallel()

	got := spansFromEntities([]tg.MessageEntityClass{
		&tg.MessageEntityTextURL{Offset: 0, Length: 4, URL: "tg://resolve?domain=omamessenger"},
	}, "open")
	want := []domain.Span{{Kind: domain.SpanLink, Start: 0, End: 4, URL: "tg://resolve?domain=omamessenger"}}
	if !slices.Equal(got, want) {
		t.Errorf("spans = %+v, want %+v", got, want)
	}
}

func TestSpansFromEntities_SkipsAMentionEntity(t *testing.T) {
	t.Parallel()

	got := spansFromEntities([]tg.MessageEntityClass{
		&tg.MessageEntityMentionName{Offset: 0, Length: 4, UserID: 1},
	}, "name")
	if got != nil {
		t.Errorf("spans = %+v, want none", got)
	}
}

func TestSpansFromEntities_EmptyForNoEntities(t *testing.T) {
	t.Parallel()

	if got := spansFromEntities(nil, "plain"); got != nil {
		t.Errorf("spans = %+v, want nil", got)
	}
}

func TestSpansFromEntities_DropsAURLEntityPastTheEndOfText(t *testing.T) {
	t.Parallel()

	got := spansFromEntities([]tg.MessageEntityClass{
		&tg.MessageEntityURL{Offset: 0, Length: 50},
	}, "too short")
	if got != nil {
		t.Errorf("spans = %+v, want none", got)
	}
}

func TestMessage_IncludesFormattingSpans(t *testing.T) {
	t.Parallel()

	m := &tg.Message{
		ID: 20, Date: 1, Message: "bold link", PeerID: &tg.PeerUser{UserID: 42},
		Entities: []tg.MessageEntityClass{
			&tg.MessageEntityBold{Offset: 0, Length: 4},
			&tg.MessageEntityTextURL{Offset: 5, Length: 4, URL: "https://example.com"},
		},
	}

	got := message(m, testEntities(), 0)
	want := []domain.Span{
		{Kind: domain.SpanBold, Start: 0, End: 4},
		{Kind: domain.SpanLink, Start: 5, End: 9, URL: "https://example.com"},
	}
	if !slices.Equal(got.Formatting, want) {
		t.Errorf("message.Formatting = %+v, want %+v", got.Formatting, want)
	}
}
