package signal

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// TestReact_SendsReactionAndReportsTally checks that React sends
// Signal's own Reaction field and reports the resulting chips through
// the sink.
func TestReact_SendsReactionAndReportsTally(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	sender := uuid.New()
	dev := newFakeDevice(self)
	c := newTestConnector(dev)
	sink := &connectortest.Sink{}
	c.connected(dev, sink, mustOpenMediaStore(t))

	conv := domain.Conversation{RemoteID: sender.String()}
	target := messageRemoteID(sender, uint64(time.Now().UnixMilli()))
	if err := c.React(t.Context(), conv, target, "👍"); err != nil {
		t.Fatalf("React() error = %v", err)
	}

	if len(dev.sentDirect) != 1 {
		t.Fatalf("sentDirect = %d, want 1", len(dev.sentDirect))
	}
	reaction := dev.sentDirect[0].content.GetDataMessage().GetReaction()
	if reaction.GetEmoji() != "👍" || reaction.GetRemove() {
		t.Errorf("reaction = %+v, want emoji 👍, not a removal", reaction)
	}
}

// TestReact_ClearingReusesLastKnownEmoji checks that clearing a
// reaction sends Remove with the emoji this run last recorded for
// self, since Signal's own protocol needs one even to remove it.
func TestReact_ClearingReusesLastKnownEmoji(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	sender := uuid.New()
	dev := newFakeDevice(self)
	c := newTestConnector(dev)
	sink := &connectortest.Sink{}
	c.connected(dev, sink, mustOpenMediaStore(t))

	conv := domain.Conversation{RemoteID: sender.String()}
	target := messageRemoteID(sender, uint64(time.Now().UnixMilli()))
	if err := c.React(t.Context(), conv, target, "😂"); err != nil {
		t.Fatalf("first React() error = %v", err)
	}
	if err := c.React(t.Context(), conv, target, ""); err != nil {
		t.Fatalf("clearing React() error = %v", err)
	}

	reaction := dev.sentDirect[1].content.GetDataMessage().GetReaction()
	if !reaction.GetRemove() || reaction.GetEmoji() != "😂" {
		t.Errorf("clearing reaction = %+v, want Remove with 😂", reaction)
	}
}

// TestReact_ClearingWithNothingRecordedUsesFallback checks that
// clearing a reaction this run never recorded self setting still
// sends Remove, with the fallback emoji Signal's protocol needs.
func TestReact_ClearingWithNothingRecordedUsesFallback(t *testing.T) {
	t.Parallel()

	self := uuid.New()
	sender := uuid.New()
	dev := newFakeDevice(self)
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	conv := domain.Conversation{RemoteID: sender.String()}
	target := messageRemoteID(sender, uint64(time.Now().UnixMilli()))
	if err := c.React(t.Context(), conv, target, ""); err != nil {
		t.Fatalf("React() error = %v", err)
	}

	reaction := dev.sentDirect[0].content.GetDataMessage().GetReaction()
	if !reaction.GetRemove() || reaction.GetEmoji() != fallbackClearEmoji {
		t.Errorf("reaction = %+v, want Remove with %q", reaction, fallbackClearEmoji)
	}
}

// TestReact_NotConnected checks that nothing connected reports
// errNotConnected rather than panicking.
func TestReact_NotConnected(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	if err := c.React(t.Context(), domain.Conversation{}, "m1", "👍"); err == nil {
		t.Error("React() with nothing connected succeeded, want an error")
	}
}

// TestReact_RejectsMalformedMessageID checks that a message id that
// does not parse is refused with an error.
func TestReact_RejectsMalformedMessageID(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice(uuid.New())
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	if err := c.React(t.Context(), domain.Conversation{RemoteID: uuid.New().String()}, "not-a-message-id", "👍"); err == nil {
		t.Error("React() with a malformed message id succeeded, want an error")
	}
}

// TestReact_DeviceErrorPropagates checks that a failure sending the
// reaction is returned rather than silently ignored.
func TestReact_DeviceErrorPropagates(t *testing.T) {
	t.Parallel()

	sender := uuid.New()
	dev := newFakeDevice(uuid.New())
	dev.sendDirectErr = errors.New("send failed")
	c := newTestConnector(dev)
	c.connected(dev, &connectortest.Sink{}, mustOpenMediaStore(t))

	conv := domain.Conversation{RemoteID: sender.String()}
	target := messageRemoteID(sender, uint64(time.Now().UnixMilli()))
	if err := c.React(t.Context(), conv, target, "👍"); err == nil {
		t.Error("React() with a failing send succeeded, want an error")
	}
}

// TestSetReaction_TalliesAndClears checks that reacting adds a chip,
// a second reactor with the same emoji increases its count, and
// clearing a reaction (an empty emoji) removes that reactor's chip.
func TestSetReaction_TalliesAndClears(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	self := uuid.New()
	other := uuid.New()

	chips := c.setReaction("conv1/msg1", self, "👍", self)
	if len(chips) != 1 || chips[0].Count != 1 || !chips[0].Mine {
		t.Fatalf("chips = %+v", chips)
	}

	chips = c.setReaction("conv1/msg1", other, "👍", self)
	if len(chips) != 1 || chips[0].Count != 2 {
		t.Fatalf("chips after second reactor = %+v", chips)
	}

	chips = c.setReaction("conv1/msg1", self, "", self)
	if len(chips) != 1 || chips[0].Count != 1 || chips[0].Mine {
		t.Fatalf("chips after clearing self = %+v", chips)
	}
}

// TestSetReaction_Eviction checks that tracking more than
// maxTrackedReactions evicts the oldest first.
func TestSetReaction_Eviction(t *testing.T) {
	t.Parallel()

	c := &Connector{}
	for i := 0; i < maxTrackedReactions+1; i++ {
		c.setReaction(uuid.New().String(), uuid.New(), "👍", uuid.Nil)
	}

	if len(c.reactions) != maxTrackedReactions {
		t.Errorf("len(reactions) = %d, want %d", len(c.reactions), maxTrackedReactions)
	}
}
