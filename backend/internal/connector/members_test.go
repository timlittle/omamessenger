package connector_test

// members_test.go checks that the Manager routes a members request to
// the conversation's own connector, when it lists any, and finds
// nothing for one that does not.

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// withMembers is a connector that lists a group's members, recording
// which conversation was asked.
type withMembers struct {
	fakeConnector
	asked   []string
	members []domain.Member
}

// Members reports the members configured on the fake, recording the
// conversation it was asked about.
func (c *withMembers) Members(_ context.Context, conv domain.Conversation) ([]domain.Member, error) {
	c.asked = append(c.asked, conv.RemoteID)

	return c.members, nil
}

func TestMembers_AsksConnectorsThatListThem(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		want := []domain.Member{{ID: "1", Name: "Alice"}, {ID: "2", Name: "Bob"}}
		tg := &withMembers{fakeConnector: fakeConnector{id: "tg"}, members: want}
		m, _ := startManager(t, ctx, tg, &fakeConnector{id: "plain"})

		got, err := m.Members(ctx, domain.Conversation{AccountID: "tg", RemoteID: "group:1"})
		if err != nil {
			t.Fatalf("Members: %v", err)
		}
		if len(got) != 2 || got[0].Name != "Alice" || got[1].Name != "Bob" {
			t.Errorf("Members = %v, want %v", got, want)
		}
		if len(tg.asked) != 1 || tg.asked[0] != "group:1" {
			t.Errorf("asked = %v, want [group:1]", tg.asked)
		}

		got, err = m.Members(ctx, domain.Conversation{AccountID: "plain"})
		if err != nil || got != nil {
			t.Errorf("Members from a connector without the capability = %v, %v; want nil, nil", got, err)
		}

		if _, err := m.Members(ctx, domain.Conversation{AccountID: "nobody"}); err == nil {
			t.Error("Members for an unknown account: want an error")
		}

		cancel()
		m.Wait()
	})
}
