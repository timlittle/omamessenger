package telegram

import (
	"slices"
	"testing"

	"github.com/gotd/td/tg"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestMentionsFromEntities_ReadsMentionNameEntities(t *testing.T) {
	t.Parallel()

	e := testEntities()
	ents := []tg.MessageEntityClass{
		&tg.MessageEntityMentionName{Offset: 4, Length: 6, UserID: 42},
	}

	got, me := mentionsFromEntities(ents, e, 7)
	want := []domain.Mention{{UserID: "user:42:99", Name: "Nadia Rahman", Offset: 4, Length: 6}}
	if !slices.Equal(got, want) {
		t.Errorf("mentions = %+v, want %+v", got, want)
	}
	if me {
		t.Error("mentionsMe = true, want false: selfID does not match the mentioned user")
	}
}

func TestMentionsFromEntities_FlagsASelfMention(t *testing.T) {
	t.Parallel()

	got, me := mentionsFromEntities([]tg.MessageEntityClass{
		&tg.MessageEntityMentionName{Offset: 0, Length: 5, UserID: 42},
	}, testEntities(), 42)

	if !me {
		t.Error("mentionsMe = false, want true: selfID matches the mentioned user")
	}
	if len(got) != 1 {
		t.Fatalf("mentions = %+v, want one entry", got)
	}
}

func TestMentionsFromEntities_SkipsAUserNotInEntities(t *testing.T) {
	t.Parallel()

	got, me := mentionsFromEntities([]tg.MessageEntityClass{
		&tg.MessageEntityMentionName{Offset: 0, Length: 5, UserID: 999},
	}, testEntities(), 0)

	if got != nil || me {
		t.Errorf("mentions, me = %+v, %t, want nil, false", got, me)
	}
}

func TestOutgoingEntities_BuildsMentionNameEntitiesForKnownUsers(t *testing.T) {
	t.Parallel()

	got := outgoingEntities([]domain.Mention{{UserID: "user:42:99", Name: "Nadia", Offset: 3, Length: 6}})
	want := []tg.MessageEntityClass{&tg.InputMessageEntityMentionName{
		Offset: 3, Length: 6, UserID: &tg.InputUser{UserID: 42, AccessHash: 99},
	}}
	if !slices.EqualFunc(got, want, func(a, b tg.MessageEntityClass) bool {
		return a.(*tg.InputMessageEntityMentionName).String() == b.(*tg.InputMessageEntityMentionName).String()
	}) {
		t.Errorf("entities = %+v, want %+v", got, want)
	}
}

func TestOutgoingEntities_SkipsAMentionThatIsNotATelegramUserID(t *testing.T) {
	t.Parallel()

	if got := outgoingEntities([]domain.Mention{{UserID: "not-a-telegram-id", Offset: 0, Length: 1}}); got != nil {
		t.Errorf("entities = %+v, want nil", got)
	}
}

func TestOutgoingEntities_EmptyForNoMentions(t *testing.T) {
	t.Parallel()

	if got := outgoingEntities(nil); got != nil {
		t.Errorf("entities = %+v, want nil", got)
	}
}
