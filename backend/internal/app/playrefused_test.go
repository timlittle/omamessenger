// playrefused_test.go is an internal test (package app, not
// app_test): refusedPlayCopiesLimit is 500, and proving its own
// eviction through 501 real play fetches against a fixture would be
// slow and only indirectly about the thing being proven. The
// integration this type exists for - a second play fetch failing fast
// rather than transcoding again - is covered through the public API
// instead, in
// play_test.go's TestFetchMedia_FailsFastOnASecondPlayFetchAfterAFailedTranscode.
package app

import (
	"fmt"
	"testing"
)

// TestRefusedPlayCopies_RemembersAndLooksUp covers the basic
// contract: nothing is found before remember is called, and remember
// makes refused report true afterward.
func TestRefusedPlayCopies_RemembersAndLooksUp(t *testing.T) {
	t.Parallel()

	r := newRefusedPlayCopies()
	if r.refused("m1.video-play") {
		t.Fatal("refused() on an empty set = true, want false")
	}

	r.remember("m1.video-play")

	if !r.refused("m1.video-play") {
		t.Error("refused() after remember() = false, want true")
	}
}

// TestRefusedPlayCopies_EvictsTheOldestPastItsLimit covers the bound
// that keeps a hostile conversation's worth of distinct failed play
// copies from growing this set forever.
func TestRefusedPlayCopies_EvictsTheOldestPastItsLimit(t *testing.T) {
	t.Parallel()

	r := newRefusedPlayCopies()
	for i := range refusedPlayCopiesLimit + 1 {
		r.remember(fmt.Sprintf("m%d.video-play", i))
	}

	if r.refused("m0.video-play") {
		t.Error("refused() found the oldest entry, want it evicted past the limit")
	}

	newest := fmt.Sprintf("m%d.video-play", refusedPlayCopiesLimit)
	if !r.refused(newest) {
		t.Error("refused() did not find the most recently remembered entry")
	}
}

// TestRefusedPlayCopies_RememberingAgainDoesNotDuplicateTheEntry
// covers remember called twice for the same id, which must not let
// the eviction order carry two entries for one id.
func TestRefusedPlayCopies_RememberingAgainDoesNotDuplicateTheEntry(t *testing.T) {
	t.Parallel()

	r := newRefusedPlayCopies()
	r.remember("m1.video-play")
	r.remember("m1.video-play")

	if got := len(r.order); got != 1 {
		t.Fatalf("order length = %d, want 1: remembering the same id again must not grow it", got)
	}
	if !r.refused("m1.video-play") {
		t.Error("refused() = false, want true")
	}
}
