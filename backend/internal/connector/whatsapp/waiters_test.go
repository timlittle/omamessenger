package whatsapp

// waiters.go's policy differences (overwrite versus refuse a second
// register for the same key) are already proven at the behaviour level
// by retry_test.go and history_ondemand_test.go. These tests cover the
// generic table itself: that register, deliver and cleanup agree on a
// key regardless of which policy is asked for.

import "testing"

// TestWaiterTable_DeliverReachesTheRegisteredChannel confirms a value
// delivered for a key reaches the channel register returned for that
// same key.
func TestWaiterTable_DeliverReachesTheRegisteredChannel(t *testing.T) {
	t.Parallel()

	var tbl waiterTable[int]

	ch, ok := tbl.register("k1", false)
	if !ok {
		t.Fatal("register = false, want true for an unused key")
	}

	tbl.deliver("k1", 42)

	if v := <-ch; v != 42 {
		t.Errorf("delivered value = %d, want 42", v)
	}
}

// TestWaiterTable_DeliverForAnUnknownKeyIsDropped confirms deliver never
// blocks or panics when nothing has registered its key.
func TestWaiterTable_DeliverForAnUnknownKeyIsDropped(t *testing.T) {
	t.Parallel()

	var tbl waiterTable[int]

	tbl.deliver("ghost", 1)
}

// TestWaiterTable_RegisterRefuseExistingRejectsASecondRegistration
// confirms register reports false, without disturbing the first
// channel, when refuseExisting is true and the key is already taken.
func TestWaiterTable_RegisterRefuseExistingRejectsASecondRegistration(t *testing.T) {
	t.Parallel()

	var tbl waiterTable[int]

	first, ok := tbl.register("k1", true)
	if !ok {
		t.Fatal("first register = false, want true")
	}

	if _, ok := tbl.register("k1", true); ok {
		t.Fatal("second register = true, want false while the first is still waiting")
	}

	tbl.deliver("k1", 7)
	if v := <-first; v != 7 {
		t.Errorf("delivered value = %d, want 7 to reach the first, undisturbed channel", v)
	}
}

// TestWaiterTable_RegisterWithoutRefuseExistingOverwrites confirms a
// second register for the same key, with refuseExisting false,
// succeeds and replaces the slot: a delivery afterwards reaches the
// second channel, not the first.
func TestWaiterTable_RegisterWithoutRefuseExistingOverwrites(t *testing.T) {
	t.Parallel()

	var tbl waiterTable[int]

	first, ok := tbl.register("k1", false)
	if !ok {
		t.Fatal("first register = false, want true")
	}

	second, ok := tbl.register("k1", false)
	if !ok {
		t.Fatal("second register = false, want true when refuseExisting is false")
	}

	tbl.deliver("k1", 9)

	select {
	case v := <-first:
		t.Errorf("first channel received %d, want it never delivered to once overwritten", v)
	default:
	}

	if v := <-second; v != 9 {
		t.Errorf("delivered value = %d, want 9 to reach the second channel", v)
	}
}

// TestWaiterTable_CleanupRemovesTheEntry confirms a delivery for a key
// is dropped, not delivered, once cleanup has removed it, and that
// registering the same key again afterwards works normally.
func TestWaiterTable_CleanupRemovesTheEntry(t *testing.T) {
	t.Parallel()

	var tbl waiterTable[int]

	ch, _ := tbl.register("k1", false)
	tbl.cleanup("k1", ch)
	tbl.deliver("k1", 1)

	select {
	case v := <-ch:
		t.Errorf("channel received %d after cleanup, want nothing delivered", v)
	default:
	}

	again, ok := tbl.register("k1", true)
	if !ok {
		t.Fatal("register after cleanup = false, want true for a now-free key")
	}

	tbl.deliver("k1", 5)
	if v := <-again; v != 5 {
		t.Errorf("delivered value = %d, want 5 after re-registering", v)
	}
}

// TestWaiterTable_CleanupLeavesAnOverwritingSecondRegistrationAlone
// confirms that when two concurrent callers register the same key
// without refusing (as retry.go's media retry waiter does), the first
// caller's cleanup, running after the second has already overwritten
// the slot, does not delete the second's still-live entry: a value
// delivered for the key afterwards must still reach the second
// caller, not be dropped.
func TestWaiterTable_CleanupLeavesAnOverwritingSecondRegistrationAlone(t *testing.T) {
	t.Parallel()

	var tbl waiterTable[int]

	first, ok := tbl.register("k1", false)
	if !ok {
		t.Fatal("first register = false, want true")
	}

	second, ok := tbl.register("k1", false)
	if !ok {
		t.Fatal("second register = false, want true")
	}

	tbl.cleanup("k1", first) // the first caller gives up, unaware it was overwritten

	tbl.deliver("k1", 9)

	select {
	case v := <-second:
		if v != 9 {
			t.Errorf("delivered value = %d, want 9", v)
		}
	default:
		t.Error("second channel received nothing, want the first caller's cleanup to have left it alone")
	}
}
