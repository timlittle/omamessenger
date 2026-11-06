package signal

// spam.go implements connector.SpamReporter: reporting a direct
// chat's contact as spam. Signal's own ReportSpam call names one
// message at a time by its server GUID and report-spam token (see
// storage.go), so this reports up to a handful of the contact's most
// recent messages still on record, the same count the official Signal
// bridge reports, then sends the block-and-spam sync message so the
// contact is blocked too.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

var _ connector.SpamReporter = (*Connector)(nil)

// spamTimeout bounds how long ReportSpam waits for Signal to accept
// each report and the sync message that follows.
const spamTimeout = 30 * time.Second

// maxSpamReports bounds how many of a contact's recent messages
// ReportSpam reports, matching the official Signal bridge's own limit.
const maxSpamReports = 3

// errNoSpamMessages reports a contact with no received message this
// connector kept a server GUID for, so there is nothing to report.
var errNoSpamMessages = errors.New("signal: report spam: no received messages available to report")

// ReportSpam reports conv's contact to Signal as spam, then blocks
// them.
func (c *Connector) ReportSpam(ctx context.Context, conv domain.Conversation) error {
	dev, _, err := c.session()
	if err != nil {
		return err
	}

	media := c.mediaFor()
	if media == nil {
		return errNotConnected
	}

	aci, err := parseACI(conv.RemoteID)
	if err != nil {
		return fmt.Errorf("signal: report spam: %w", err)
	}

	spamCtx, cancel := context.WithTimeout(ctx, spamTimeout)
	defer cancel()

	reported, err := reportRecentSpam(spamCtx, dev, media, conv.RemoteID, aci)
	if err != nil {
		return err
	}
	if !reported {
		return errNoSpamMessages
	}

	if err := c.syncMessageRequestResponse(spamCtx, dev, aci, signalpb.SyncMessage_MessageRequestResponse_BLOCK_AND_SPAM); err != nil {
		return fmt.Errorf("signal: report spam: %w", err)
	}

	return nil
}

// reportRecentSpam reports each of conversationRemoteID's recent
// incoming messages with a saved server GUID, up to maxSpamReports,
// and says whether at least one was reported.
func reportRecentSpam(ctx context.Context, dev device, media *mediaStore, conversationRemoteID string, aci uuid.UUID) (bool, error) {
	metas, err := media.recentIncoming(ctx, conversationRemoteID, maxSpamReports)
	if err != nil {
		return false, fmt.Errorf("signal: report spam: %w", err)
	}

	reported := false
	for _, meta := range metas {
		if meta.serverGUID == "" {
			continue
		}

		if err := dev.reportSpam(ctx, aci, meta.serverGUID, meta.reportToken); err != nil {
			return reported, fmt.Errorf("signal: report spam: %w", err)
		}
		reported = true
	}

	return reported, nil
}
