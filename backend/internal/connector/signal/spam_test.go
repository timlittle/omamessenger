package signal

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.mau.fi/mautrix-signal/pkg/signalmeow/protobuf/signalpb"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestReportSpam_ReportsRecentMessagesThenBlocks checks that
// ReportSpam reports every recent message this connector kept a
// server GUID for, up to the cap, then sends the block-and-spam sync.
func TestReportSpam_ReportsRecentMessagesThenBlocks(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	sender := uuid.New()
	dev := newFakeDevice(self)
	c := newTestConnector(dev)
	media := mustOpenMediaStore(t)
	c.connected(dev, &connectortest.Sink{}, media)

	conv := domain.Conversation{RemoteID: sender.String()}
	for i := range maxSpamReports + 2 {
		id := messageRemoteID(sender, uint64(1700000000000+i))
		if err := media.putMessageMeta(t.Context(), conv.RemoteID, id, "guid-"+id, []byte("token")); err != nil {
			t.Fatal(err)
		}
	}

	if err := c.ReportSpam(t.Context(), conv); err != nil {
		t.Fatalf("ReportSpam() error = %v", err)
	}

	if len(dev.spamReports) != maxSpamReports {
		t.Errorf("spamReports = %d, want %d", len(dev.spamReports), maxSpamReports)
	}
	for _, report := range dev.spamReports {
		if report.aci != sender {
			t.Errorf("report aci = %v, want %v", report.aci, sender)
		}
	}

	if len(dev.sentDirect) != 1 {
		t.Fatalf("sentDirect = %d, want 1", len(dev.sentDirect))
	}
	resp := dev.sentDirect[0].content.GetSyncMessage().GetMessageRequestResponse()
	if resp.GetType() != signalpb.SyncMessage_MessageRequestResponse_BLOCK_AND_SPAM {
		t.Errorf("response type = %v, want BLOCK_AND_SPAM", resp.GetType())
	}
}

// TestReportSpam_NoMessagesIsRefused checks that a contact with
// nothing this connector kept a server GUID for refuses the report
// rather than silently blocking with nothing reported.
func TestReportSpam_NoMessagesIsRefused(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	err := c.ReportSpam(t.Context(), domain.Conversation{RemoteID: uuid.New().String()})
	if !errors.Is(err, errNoSpamMessages) {
		t.Errorf("ReportSpam() error = %v, want errNoSpamMessages", err)
	}
}

// TestReportSpam_NotConnected checks that nothing connected reports
// errNotConnected rather than panicking.
func TestReportSpam_NotConnected(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	if err := c.ReportSpam(t.Context(), domain.Conversation{}); !errors.Is(err, errNotConnected) {
		t.Errorf("ReportSpam() error = %v, want errNotConnected", err)
	}
}

// TestReportSpam_NoMediaStore checks a connector built without a
// media store, as a bare Connector in another package's test might
// be, reports errNotConnected rather than panicking.
func TestReportSpam_NoMediaStore(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, nil)

	if err := c.ReportSpam(t.Context(), domain.Conversation{RemoteID: uuid.New().String()}); !errors.Is(err, errNotConnected) {
		t.Errorf("ReportSpam() error = %v, want errNotConnected", err)
	}
}

// TestReportSpam_InvalidConversationID checks that a conversation
// remote id that is not a plain account id, such as a group's, is
// refused with an error rather than panicking.
func TestReportSpam_InvalidConversationID(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	if err := c.ReportSpam(t.Context(), domain.Conversation{RemoteID: "not-an-account-id"}); err == nil {
		t.Error("ReportSpam() with an invalid id succeeded, want an error")
	}
}

// TestReportSpam_SkipsMessagesWithNoGUID checks that a recent message
// saved with no server GUID is skipped rather than reported.
func TestReportSpam_SkipsMessagesWithNoGUID(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	media := mustOpenMediaStore(t)
	c.connected(dev, &connectortest.Sink{}, media)

	conv := domain.Conversation{RemoteID: sender.String()}
	noGUID := messageRemoteID(sender, 1)
	withGUID := messageRemoteID(sender, 2)
	if err := media.putMessageMeta(t.Context(), conv.RemoteID, noGUID, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := media.putMessageMeta(t.Context(), conv.RemoteID, withGUID, "guid-1", nil); err != nil {
		t.Fatal(err)
	}

	if err := c.ReportSpam(t.Context(), conv); err != nil {
		t.Fatalf("ReportSpam() error = %v", err)
	}
	if len(dev.spamReports) != 1 {
		t.Errorf("spamReports = %d, want 1", len(dev.spamReports))
	}
}

// TestReportSpam_DeviceErrorPropagates checks that a failure
// reporting one message, or sending the block-and-spam sync, is
// returned rather than silently ignored.
func TestReportSpam_DeviceErrorPropagates(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	dev.reportSpamErr = errors.New("report failed")
	c := newTestConnector(dev)
	media := mustOpenMediaStore(t)
	c.connected(dev, &connectortest.Sink{}, media)

	conv := domain.Conversation{RemoteID: sender.String()}
	if err := media.putMessageMeta(t.Context(), conv.RemoteID, messageRemoteID(sender, 1), "guid-1", nil); err != nil {
		t.Fatal(err)
	}

	if err := c.ReportSpam(t.Context(), conv); err == nil {
		t.Error("ReportSpam() with a failing report succeeded, want an error")
	}
}

// TestReportSpam_SyncFailurePropagates checks that a failure sending
// the block-and-spam sync after a successful report is returned.
func TestReportSpam_SyncFailurePropagates(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	media := mustOpenMediaStore(t)
	c.connected(dev, &connectortest.Sink{}, media)

	conv := domain.Conversation{RemoteID: sender.String()}
	if err := media.putMessageMeta(t.Context(), conv.RemoteID, messageRemoteID(sender, 1), "guid-1", nil); err != nil {
		t.Fatal(err)
	}
	dev.sendDirectErr = errors.New("sync failed")

	if err := c.ReportSpam(t.Context(), conv); err == nil {
		t.Error("ReportSpam() with a failing sync succeeded, want an error")
	}
}
