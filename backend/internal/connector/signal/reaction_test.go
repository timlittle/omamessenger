package signal

import (
	"testing"

	"github.com/google/uuid"
)

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
