package domain_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestCapSpans_DropsASpanThatRunsPastTheTextsEnd covers a span left
// dangling once TruncateIncomingText has cut the text it formats down:
// it must be dropped, never kept pointing past where the text now ends.
func TestCapSpans_DropsASpanThatRunsPastTheTextsEnd(t *testing.T) {
	t.Parallel()

	spans := []domain.Span{
		{Kind: domain.SpanBold, Start: 0, End: 3},
		{Kind: domain.SpanItalic, Start: 2, End: 10},
	}

	got := domain.CapSpans(spans, 5)
	if len(got) != 1 || got[0].Start != 0 || got[0].End != 3 {
		t.Errorf("CapSpans(spans, 5) = %+v, want only the span that still fits", got)
	}
}

// TestCapSpans_DropsANegativeOrInvertedSpan covers a span whose own
// start or end a connector never means to send negative, or end before
// start: CapSpans never trusts either without checking.
func TestCapSpans_DropsANegativeOrInvertedSpan(t *testing.T) {
	t.Parallel()

	spans := []domain.Span{
		{Kind: domain.SpanBold, Start: -1, End: 3},
		{Kind: domain.SpanItalic, Start: 5, End: 2},
		{Kind: domain.SpanStrike, Start: 1, End: 4},
	}

	got := domain.CapSpans(spans, 10)
	if len(got) != 1 || got[0].Kind != domain.SpanStrike {
		t.Errorf("CapSpans = %+v, want only the one valid span", got)
	}
}

// TestCapSpans_KeepsAtMostMaxFormattingSpans covers a sender-controlled
// entity list with no limit of its own: CapSpans must never pass more
// than MaxFormattingSpans through, however many a message's own
// formatting metadata claims.
func TestCapSpans_KeepsAtMostMaxFormattingSpans(t *testing.T) {
	t.Parallel()

	spans := make([]domain.Span, domain.MaxFormattingSpans+50)
	for i := range spans {
		spans[i] = domain.Span{Kind: domain.SpanBold, Start: 0, End: 1}
	}

	got := domain.CapSpans(spans, 1)
	if len(got) != domain.MaxFormattingSpans {
		t.Errorf("CapSpans kept %d spans, want exactly %d", len(got), domain.MaxFormattingSpans)
	}
}

// TestCapMentions_DropsAMentionThatRunsPastTheTextsEnd mirrors
// TestCapSpans_DropsASpanThatRunsPastTheTextsEnd for a mention.
func TestCapMentions_DropsAMentionThatRunsPastTheTextsEnd(t *testing.T) {
	t.Parallel()

	mentions := []domain.Mention{
		{UserID: "u1", Name: "Alex", Offset: 0, Length: 3},
		{UserID: "u2", Name: "Sam", Offset: 2, Length: 10},
	}

	got := domain.CapMentions(mentions, 5)
	if len(got) != 1 || got[0].UserID != "u1" {
		t.Errorf("CapMentions(mentions, 5) = %+v, want only the mention that still fits", got)
	}
}

// TestCapMentions_KeepsAtMostMaxMentions mirrors
// TestCapSpans_KeepsAtMostMaxFormattingSpans for mentions.
func TestCapMentions_KeepsAtMostMaxMentions(t *testing.T) {
	t.Parallel()

	mentions := make([]domain.Mention, domain.MaxMentions+20)
	for i := range mentions {
		mentions[i] = domain.Mention{UserID: "u", Name: "x", Offset: 0, Length: 1}
	}

	got := domain.CapMentions(mentions, 1)
	if len(got) != domain.MaxMentions {
		t.Errorf("CapMentions kept %d mentions, want exactly %d", len(got), domain.MaxMentions)
	}
}

// TestCapReactions_KeepsAtMostMaxReactions covers a reaction tally with
// no limit of its own: a sender's message cannot be made to carry more
// chips than the space under it could ever usefully show.
func TestCapReactions_KeepsAtMostMaxReactions(t *testing.T) {
	t.Parallel()

	reactions := make([]domain.Reaction, domain.MaxReactions+10)
	for i := range reactions {
		reactions[i] = domain.Reaction{Emoji: "x", Count: 1}
	}

	got := domain.CapReactions(reactions)
	if len(got) != domain.MaxReactions {
		t.Errorf("CapReactions kept %d reactions, want exactly %d", len(got), domain.MaxReactions)
	}
}

// TestCapReactions_LeavesAShortListAlone confirms a reaction list
// already within the cap passes through unchanged.
func TestCapReactions_LeavesAShortListAlone(t *testing.T) {
	t.Parallel()

	reactions := []domain.Reaction{{Emoji: "👍", Count: 3}}
	got := domain.CapReactions(reactions)
	if len(got) != 1 {
		t.Errorf("CapReactions changed a short list: %+v", got)
	}
}

// TestCapMedia_NilIsUnchanged confirms a message with no media at all
// is never given one.
func TestCapMedia_NilIsUnchanged(t *testing.T) {
	t.Parallel()

	if got := domain.CapMedia(nil); got != nil {
		t.Errorf("CapMedia(nil) = %+v, want nil", got)
	}
}

// TestCapMedia_BoundsAPollsOptionsAndText covers a poll whose service-
// reported option count or option text has no limit of its own: both
// are capped, and the question itself is capped too.
func TestCapMedia_BoundsAPollsOptionsAndText(t *testing.T) {
	t.Parallel()

	options := make([]domain.PollOption, domain.MaxPollOptions+10)
	for i := range options {
		options[i] = domain.PollOption{ID: "o", Text: strings.Repeat("a", domain.MaxPollOptionTextLength+10)}
	}

	media := &domain.Media{
		Kind: domain.MediaPoll,
		Poll: &domain.Poll{Question: strings.Repeat("q", domain.MaxPollQuestionLength+10), Options: options},
	}

	got := domain.CapMedia(media)
	if len(got.Poll.Options) != domain.MaxPollOptions {
		t.Errorf("poll kept %d options, want %d", len(got.Poll.Options), domain.MaxPollOptions)
	}
	if n := domain.UTF16Len(got.Poll.Question); n > domain.MaxPollQuestionLength {
		t.Errorf("poll question is %d UTF-16 units, want at most %d", n, domain.MaxPollQuestionLength)
	}
	for _, o := range got.Poll.Options {
		if n := domain.UTF16Len(o.Text); n > domain.MaxPollOptionTextLength {
			t.Errorf("poll option text is %d UTF-16 units, want at most %d", n, domain.MaxPollOptionTextLength)
		}
	}
}

// TestCapMedia_BoundsALinkPreviewsText covers a link preview whose
// title, site name or description a page's own metadata fully
// controls, with no limit of its own.
func TestCapMedia_BoundsALinkPreviewsText(t *testing.T) {
	t.Parallel()

	media := &domain.Media{
		Kind:        domain.MediaLink,
		SiteName:    strings.Repeat("s", domain.MaxLinkPreviewFieldLength+10),
		Title:       strings.Repeat("t", domain.MaxLinkPreviewFieldLength+10),
		Description: strings.Repeat("d", domain.MaxLinkPreviewDescriptionLength+10),
	}

	got := domain.CapMedia(media)
	if n := domain.UTF16Len(got.SiteName); n > domain.MaxLinkPreviewFieldLength {
		t.Errorf("site name is %d units, want at most %d", n, domain.MaxLinkPreviewFieldLength)
	}
	if n := domain.UTF16Len(got.Title); n > domain.MaxLinkPreviewFieldLength {
		t.Errorf("title is %d units, want at most %d", n, domain.MaxLinkPreviewFieldLength)
	}
	if n := domain.UTF16Len(got.Description); n > domain.MaxLinkPreviewDescriptionLength {
		t.Errorf("description is %d units, want at most %d", n, domain.MaxLinkPreviewDescriptionLength)
	}
}

// TestCapMedia_PollWithNoPollIsUnchanged covers the defensive nil
// check in capPoll: a message whose Kind is MediaPoll always carries a
// Poll in practice, but CapMedia must never assume that and must never
// invent one.
func TestCapMedia_PollWithNoPollIsUnchanged(t *testing.T) {
	t.Parallel()

	media := &domain.Media{Kind: domain.MediaPoll, Poll: nil}
	got := domain.CapMedia(media)
	if got.Poll != nil {
		t.Errorf("CapMedia invented a poll where there was none: %+v", got.Poll)
	}
}

// TestCapMedia_LeavesShortLinkPreviewFieldsAlone confirms a link
// preview already within every cap passes through unchanged.
func TestCapMedia_LeavesShortLinkPreviewFieldsAlone(t *testing.T) {
	t.Parallel()

	media := &domain.Media{Kind: domain.MediaLink, SiteName: "Example", Title: "A short title", Description: "A short description"}
	got := domain.CapMedia(media)
	if got.SiteName != "Example" || got.Title != "A short title" || got.Description != "A short description" {
		t.Errorf("CapMedia changed already-short link preview fields: %+v", got)
	}
}

// TestCapMedia_NeverSplitsASurrogatePairAcrossTheCut covers text whose
// cut point would otherwise land between a surrogate pair's own two
// UTF-16 units - a rune outside the Basic Multilingual Plane, which a
// naive "units == max" check can jump straight over without ever
// landing on it exactly.
func TestCapMedia_NeverSplitsASurrogatePairAcrossTheCut(t *testing.T) {
	t.Parallel()

	media := &domain.Media{Kind: domain.MediaLink, Title: strings.Repeat("😀", domain.MaxLinkPreviewFieldLength)}
	got := domain.CapMedia(media)

	if n := domain.UTF16Len(got.Title); n > domain.MaxLinkPreviewFieldLength {
		t.Errorf("title is %d UTF-16 units, want at most %d", n, domain.MaxLinkPreviewFieldLength)
	}
	if !utf8.ValidString(got.Title) {
		t.Error("CapMedia split a multi-byte rune")
	}
}

// TestCapMedia_LeavesOtherKindsAlone confirms a photo, video, file or
// voice note - none of which carries a sender-controlled text field
// this package bounds - passes through with no change at all.
func TestCapMedia_LeavesOtherKindsAlone(t *testing.T) {
	t.Parallel()

	media := &domain.Media{Kind: domain.MediaPhoto, Title: strings.Repeat("t", domain.MaxLinkPreviewFieldLength+10)}
	got := domain.CapMedia(media)
	if got.Title != media.Title {
		t.Error("CapMedia changed a photo's field it has no business bounding")
	}
}

// TestSanitizeIncoming_AppliesEveryCapTogether confirms the one
// function Ingest calls for both a newly arrived message and an edited
// one truncates text and caps every collection the text's new length
// then also bounds.
func TestSanitizeIncoming_AppliesEveryCapTogether(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", domain.MaxIncomingTextLength+1000)
	m := domain.Message{
		Text:       long,
		Formatting: []domain.Span{{Kind: domain.SpanBold, Start: 0, End: 1}, {Kind: domain.SpanItalic, Start: domain.MaxIncomingTextLength + 500, End: domain.MaxIncomingTextLength + 600}},
		Reactions:  make([]domain.Reaction, domain.MaxReactions+5),
	}

	got := domain.SanitizeIncoming(m)
	if len(got.Text) > domain.MaxIncomingTextLength {
		t.Errorf("SanitizeIncoming left text at %d bytes, want at most %d", len(got.Text), domain.MaxIncomingTextLength)
	}
	if len(got.Formatting) != 1 {
		t.Errorf("SanitizeIncoming kept %d spans, want only the one still inside the truncated text", len(got.Formatting))
	}
	if len(got.Reactions) != domain.MaxReactions {
		t.Errorf("SanitizeIncoming kept %d reactions, want %d", len(got.Reactions), domain.MaxReactions)
	}
}
