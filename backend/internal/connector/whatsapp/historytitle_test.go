package whatsapp

import (
	"errors"
	"testing"

	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// groupJIDForNaming is the group resolveGroupName's tests resolve a
// name and member count for, directly, without going through a full
// history sync event.
var groupJIDForNaming = types.NewJID("12345-1600000000", types.GroupServer)

// TestResolveGroupName checks that a group's name and member count
// resolve from the sync, the cache or a bounded WhatsApp request, and
// that the request's own result reports the group's current
// disappearing timer.
func TestResolveGroupName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		dev           *fakeDevice
		syncedTitle   string
		syncedMembers int
		wantName      string
		wantMembers   int
		wantCalls     int
		wantTimer     string
	}{
		{
			"resolves an unknown group's name and members together",
			&fakeDevice{
				groupNames:   map[string]string{groupJIDForNaming.String(): "Climbing Crew"},
				groupMembers: map[string]int{groupJIDForNaming.String(): 12},
				groupTimers:  map[string]int64{groupJIDForNaming.String(): 604800},
			},
			"", 0, "Climbing Crew", 12, 1, "timer 12345-1600000000@g.us 604800",
		},
		{
			"falls back to a generic name when it cannot be resolved",
			&fakeDevice{groupErr: errors.New("unavailable")},
			"", 0, "Group", 0, 1, "",
		},
		{
			"keeps the sync's own member count without asking WhatsApp",
			&fakeDevice{},
			"Climbing Crew", 2, "Climbing Crew", 2, 0, "",
		},
		{
			// Scripted differently from the synced title, to prove the
			// fetched name is never used once a name is already known:
			// only its member count matters here, since the sync, as
			// WhatsApp's own history sync often does, named the group
			// but carried no participant list for it at all.
			"resolves member count even when the sync already named the group",
			&fakeDevice{groupNames: map[string]string{groupJIDForNaming.String(): "should never be used"}, groupMembers: map[string]int{groupJIDForNaming.String(): 3}},
			"Tim and Laura", 0, "Tim and Laura", 3, 1, "timer 12345-1600000000@g.us 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := New(domain.Account{ID: "wa"}, t.TempDir())
			sink := &connectortest.Sink{}
			name, members := c.resolveGroupName(t.Context(), syncSource{dev: tt.dev, sink: sink}, groupJIDForNaming, tt.syncedTitle, tt.syncedMembers)

			if name != tt.wantName || members != tt.wantMembers {
				t.Errorf("resolveGroupName = (%q, %d), want (%q, %d)", name, members, tt.wantName, tt.wantMembers)
			}
			if len(tt.dev.groupCalls) != tt.wantCalls {
				t.Errorf("group info requested %d times, want %d", len(tt.dev.groupCalls), tt.wantCalls)
			}
			if tt.wantTimer != "" && !sink.Has(tt.wantTimer) {
				t.Errorf("events = %q, want %q", sink.Lines(), tt.wantTimer)
			}
			if tt.wantTimer == "" && len(sink.Lines()) != 0 {
				t.Errorf("events = %q, want nothing reported when the lookup never ran or failed", sink.Lines())
			}
		})
	}
}

// TestResolveGroupName_CachesAfterItsFirstResolve checks that a second
// resolve for the same group reuses the first lookup's name and member
// count rather than asking WhatsApp again.
func TestResolveGroupName_CachesAfterItsFirstResolve(t *testing.T) {
	t.Parallel()

	c := New(domain.Account{ID: "wa"}, t.TempDir())
	dev := &fakeDevice{groupNames: map[string]string{groupJIDForNaming.String(): "Climbing Crew"}}
	sink := &connectortest.Sink{}

	c.resolveGroupName(t.Context(), syncSource{dev: dev, sink: sink}, groupJIDForNaming, "", 0)
	c.resolveGroupName(t.Context(), syncSource{dev: dev, sink: sink}, groupJIDForNaming, "", 0)

	if len(dev.groupCalls) != 1 {
		t.Errorf("group info requested %d times across two resolves, want 1 (cached after the first)", len(dev.groupCalls))
	}
}
