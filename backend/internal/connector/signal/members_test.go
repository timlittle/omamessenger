package signal

import (
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestMembers_DirectChat checks that a direct chat, whose remote id is
// never a group identifier, reports no members rather than an error.
func TestMembers_DirectChat(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, nil)

	members, err := c.Members(t.Context(), domain.Conversation{RemoteID: uuid.New().String()})
	if err != nil || members != nil {
		t.Errorf("Members() = %v, %v, want nil, nil", members, err)
	}
}

// TestMembers_Group checks that a group's members are resolved by
// Signal's own identifier and named from the device's contact names.
func TestMembers_Group(t *testing.T) {
	t.Parallel()

	alice := uuid.New()
	dev := newFakeDevice(uuid.New())
	dev.names[alice] = "Alice"
	dev.groupMembers = []signalmeow.GroupMember{{ACI: alice}}

	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, nil)

	groupID := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	members, err := c.Members(t.Context(), domain.Conversation{RemoteID: groupID})
	if err != nil {
		t.Fatalf("Members() error = %v", err)
	}
	if len(members) != 1 || members[0].Name != "Alice" {
		t.Errorf("Members() = %+v", members)
	}
}

// TestMembers_NotConnected checks that asking for members with no run
// connected reports errNotConnected rather than panicking.
func TestMembers_NotConnected(t *testing.T) {
	t.Parallel()

	c := &Connector{}

	groupID := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	if _, err := c.Members(t.Context(), domain.Conversation{RemoteID: groupID}); err == nil {
		t.Error("Members() succeeded with no run connected, want an error")
	}
}
