package whatsapp

// SetBlocked is driven through a fake device, the way edit_test.go
// drives EditMessage, since the fake is the only way to see what was
// asked without reaching WhatsApp's servers. blockCall and
// fakeDevice.updateBlocklist live here rather than in helpers_test.go,
// which was already close to this repository's line limit, the same
// reasoning organize_test.go's own sendAppState fake follows (see
// device.go and docs/decisions.md): fakeDevice's blockErr and
// blockCalls fields still have to live in the struct itself.

import (
	"context"
	"errors"
	"testing"

	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// blockCall records one call to updateBlocklist.
type blockCall struct {
	jid   types.JID
	block bool
}

// updateBlocklist records the call and reports blockErr.
func (d *fakeDevice) updateBlocklist(_ context.Context, jid types.JID, block bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.blockCalls = append(d.blockCalls, blockCall{jid: jid, block: block})

	return d.blockErr
}

func TestSetBlocked_BlocksAndUnblocks(t *testing.T) {
	t.Parallel()

	dev, _, c := connectedFixture(t)

	if err := c.SetBlocked(t.Context(), directChat, true); err != nil {
		t.Fatal(err)
	}
	if err := c.SetBlocked(t.Context(), directChat, false); err != nil {
		t.Fatal(err)
	}

	if len(dev.blockCalls) != 2 || dev.blockCalls[0].jid != directPeer || !dev.blockCalls[0].block || dev.blockCalls[1].block {
		t.Errorf("blockCalls = %+v, want a block then an unblock of %v", dev.blockCalls, directPeer)
	}
}

func TestSetBlocked_FailsForABadConversationID(t *testing.T) {
	t.Parallel()

	_, _, c := connectedFixture(t)

	err := c.SetBlocked(t.Context(), domain.Conversation{RemoteID: "not-a-jid"}, true)
	if !errors.Is(err, errBadRemoteID) {
		t.Errorf("SetBlocked = %v, want errBadRemoteID", err)
	}
}

func TestSetBlocked_WrapsAWhatsAppError(t *testing.T) {
	t.Parallel()

	dev, _, c := connectedFixture(t)
	dev.blockErr = errors.New("server unavailable")

	if err := c.SetBlocked(t.Context(), directChat, true); !errors.Is(err, dev.blockErr) {
		t.Errorf("SetBlocked = %v, want it to wrap the device's error", err)
	}
}
