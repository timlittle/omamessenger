package whatsapp

import (
	"slices"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestRewriteMentions_ReplacesTheTokenWithTheResolvedName(t *testing.T) {
	t.Parallel()

	alice := types.NewJID("15550001111", types.DefaultUserServer)
	dev := newFakeDevice()
	dev.contactNames = map[string]string{alice.String(): "Alice"}

	text, mentions, me := rewriteMentions(t.Context(), dev, "hi @15550001111 how are you", []string{alice.String()})

	if text != "hi @Alice how are you" {
		t.Errorf("text = %q, want %q", text, "hi @Alice how are you")
	}
	want := []domain.Mention{{UserID: remoteID(alice), Name: "Alice", Offset: 3, Length: 6}}
	if !slices.Equal(mentions, want) {
		t.Errorf("mentions = %+v, want %+v", mentions, want)
	}
	if me {
		t.Error("mentionsMe = true, want false")
	}
}

func TestRewriteMentions_FlagsASelfMention(t *testing.T) {
	t.Parallel()

	self := types.NewJID("15559998888", types.DefaultUserServer)
	dev := newFakeDevice()
	dev.selfJID = self

	_, _, me := rewriteMentions(t.Context(), dev, "hey @15559998888", []string{self.String()})
	if !me {
		t.Error("mentionsMe = false, want true")
	}
}

func TestRewriteMentions_NoMentionedJIDsLeavesTextUnchanged(t *testing.T) {
	t.Parallel()

	text, mentions, me := rewriteMentions(t.Context(), newFakeDevice(), "hi @15550001111", nil)
	if text != "hi @15550001111" || mentions != nil || me {
		t.Errorf("got %q, %+v, %t; want the text unchanged and no mentions", text, mentions, me)
	}
}

func TestRewriteMentions_TokenNotInMentionedJIDsIsLeftAlone(t *testing.T) {
	t.Parallel()

	alice := types.NewJID("15550001111", types.DefaultUserServer)
	text, mentions, _ := rewriteMentions(t.Context(), newFakeDevice(), "call @15559999999 maybe", []string{alice.String()})
	if text != "call @15559999999 maybe" || mentions != nil {
		t.Errorf("got %q, %+v; want unchanged, since the token names someone not in MentionedJID", text, mentions)
	}
}

func TestOutgoingMentions_RewritesTheTokenAndListsTheJID(t *testing.T) {
	t.Parallel()

	alice := types.NewJID("15550001111", types.DefaultUserServer)
	text, jids := outgoingMentions("hi @Alice how are you", []domain.Mention{
		{UserID: remoteID(alice), Name: "Alice", Offset: 3, Length: 6},
	})

	if text != "hi @15550001111 how are you" {
		t.Errorf("text = %q, want the phone number back", text)
	}
	if !slices.Equal(jids, []string{remoteID(alice)}) {
		t.Errorf("jids = %v, want [%s]", jids, remoteID(alice))
	}
}

func TestOutgoingMentions_MultipleMentionsKeepTheirOwnPositions(t *testing.T) {
	t.Parallel()

	alice := types.NewJID("15550001111", types.DefaultUserServer)
	bob := types.NewJID("15550002222", types.DefaultUserServer)
	text, jids := outgoingMentions("@Alice and @Bob, hi", []domain.Mention{
		{UserID: remoteID(alice), Name: "Alice", Offset: 0, Length: 6},
		{UserID: remoteID(bob), Name: "Bob", Offset: 11, Length: 4},
	})

	if want := "@15550001111 and @15550002222, hi"; text != want {
		t.Errorf("text = %q, want %q", text, want)
	}
	if !slices.Equal(jids, []string{remoteID(alice), remoteID(bob)}) {
		t.Errorf("jids = %v", jids)
	}
}

func TestOutgoingMentions_SkipsAMentionThatIsNotAWhatsAppID(t *testing.T) {
	t.Parallel()

	text, jids := outgoingMentions("hi @Alice", []domain.Mention{{UserID: "not-a-jid", Offset: 3, Length: 6}})
	if text != "hi @Alice" || jids != nil {
		t.Errorf("got %q, %v; want unchanged", text, jids)
	}
}

func TestOutgoingMentions_EmptyForNoMentions(t *testing.T) {
	t.Parallel()

	text, jids := outgoingMentions("hi", nil)
	if text != "hi" || jids != nil {
		t.Errorf("got %q, %v; want unchanged", text, jids)
	}
}

func TestOutgoingMessage_CarriesMentions(t *testing.T) {
	t.Parallel()

	alice := types.NewJID("15550001111", types.DefaultUserServer)
	m := domain.Message{Text: "hi @Alice", Mentions: []domain.Mention{{UserID: remoteID(alice), Name: "Alice", Offset: 3, Length: 6}}}

	got := outgoingMessage(t.Context(), nil, "", types.NewJID("group", types.GroupServer), m)
	ext := got.GetExtendedTextMessage()
	if ext == nil {
		t.Fatalf("message = %+v, want an ExtendedTextMessage", got)
	}
	if ext.GetText() != "hi @15550001111" {
		t.Errorf("text = %q", ext.GetText())
	}
	if want := []string{remoteID(alice)}; !slices.Equal(ext.GetContextInfo().GetMentionedJID(), want) {
		t.Errorf("mentionedJID = %v, want %v", ext.GetContextInfo().GetMentionedJID(), want)
	}
}

func TestOutgoingMessage_PlainTextWithNoMentions(t *testing.T) {
	t.Parallel()

	got := outgoingMessage(t.Context(), nil, "", types.NewJID("", types.DefaultUserServer), domain.Message{Text: "hi"})
	if got.GetConversation() != "hi" {
		t.Errorf("message = %+v, want a plain Conversation", got)
	}
}

func TestMessage_RendersMentionsInIncomingText(t *testing.T) {
	t.Parallel()

	alice := types.NewJID("15550001111", types.DefaultUserServer)
	dev := newFakeDevice()
	dev.contactNames = map[string]string{alice.String(): "Alice"}

	got := message(t.Context(), dev, testInfo(), &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text:        strPtr("hi @15550001111"),
		ContextInfo: &waE2E.ContextInfo{MentionedJID: []string{alice.String()}},
	}})

	if got.Text != "hi @Alice" {
		t.Errorf("text = %q, want %q", got.Text, "hi @Alice")
	}
	want := []domain.Mention{{UserID: remoteID(alice), Name: "Alice", Offset: 3, Length: 6}}
	if !slices.Equal(got.Mentions, want) {
		t.Errorf("mentions = %+v, want %+v", got.Mentions, want)
	}
}
