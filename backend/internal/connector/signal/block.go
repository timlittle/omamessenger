package signal

// block.go implements connector.Blocker: blocking or unblocking a
// direct chat's contact by sending a message-request-response sync
// message to this account's own other devices, the same mechanism
// Signal's own apps use. App layer already refuses a group for
// SetBlocked (app/block.go), so conv.RemoteID is always a direct
// chat's own account id here.

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var _ connector.Blocker = (*Connector)(nil)

// blockTimeout bounds how long SetBlocked and ReportSpam wait for
// Signal to accept their sync message.
const blockTimeout = 30 * time.Second

// SetBlocked blocks or unblocks conv's contact with Signal.
func (c *Connector) SetBlocked(ctx context.Context, conv domain.Conversation, blocked bool) error {
	dev, _, err := c.session()
	if err != nil {
		return err
	}

	aci, err := parseACI(conv.RemoteID)
	if err != nil {
		return fmt.Errorf("signal: set blocked: %w", err)
	}

	blockCtx, cancel := context.WithTimeout(ctx, blockTimeout)
	defer cancel()

	respType := signalpb.SyncMessage_MessageRequestResponse_ACCEPT
	if blocked {
		respType = signalpb.SyncMessage_MessageRequestResponse_BLOCK
	}
	if err := c.syncMessageRequestResponse(blockCtx, dev, aci, respType); err != nil {
		return fmt.Errorf("signal: set blocked: %w", err)
	}

	return nil
}

// syncMessageRequestResponse tells this account's own other devices
// that it accepted, blocked, or blocked and reported spam from the
// contact named by aci, the sync message Signal's own apps send for
// any of those actions.
func (c *Connector) syncMessageRequestResponse(ctx context.Context, dev device, aci uuid.UUID, respType signalpb.SyncMessage_MessageRequestResponse_Type) error {
	content := signalmeow.WrapSyncMessage(&signalpb.SyncMessage{
		Content: &signalpb.SyncMessage_MessageRequestResponse_{
			MessageRequestResponse: &signalpb.SyncMessage_MessageRequestResponse{
				Type: respType.Enum(), ThreadAciBinary: aci[:],
			},
		},
	})

	return dev.sendDirect(ctx, c.selfID(), content)
}
