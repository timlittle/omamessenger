// mediarefused_test.go is an internal test (package app, not
// app_test): refusedDisplayCopiesLimit is 500, and proving its own
// eviction through 501 real FetchMedia calls against a fixture would
// be slow and only indirectly about the thing being proven. The
// integration this type exists for - a second FetchMedia failing fast
// rather than downloading and decoding again - is covered through the
// public API instead, in
// media_displaycopy_test.go's TestFetchMedia_FailsFastOnASecondFetchAfterARefusedDisplayCopy.
package app

import (
	"fmt"
	"testing"
)

// TestRefusedDisplayCopies_RemembersAndLooksUp covers the basic
// contract: nothing is found before remember is called, and
// remember's own reason comes back afterward.
func TestRefusedDisplayCopies_RemembersAndLooksUp(t *testing.T) {
	t.Parallel()

	r := newRefusedDisplayCopies()
	if _, ok := r.lookup("m1.photo"); ok {
		t.Fatal("lookup() on an empty set = ok, want not found")
	}

	r.remember("m1.photo", MediaTooLarge)

	reason, ok := r.lookup("m1.photo")
	if !ok || reason != MediaTooLarge {
		t.Errorf("lookup() = %v, %v; want %v, true", reason, ok, MediaTooLarge)
	}
}

// TestRefusedDisplayCopies_EvictsTheOldestPastItsLimit covers the
// bound that keeps a hostile conversation's worth of distinct refused
// messages from growing this set forever.
func TestRefusedDisplayCopies_EvictsTheOldestPastItsLimit(t *testing.T) {
	t.Parallel()

	r := newRefusedDisplayCopies()
	for i := range refusedDisplayCopiesLimit + 1 {
		r.remember(fmt.Sprintf("m%d.photo", i), MediaDownloadFailed)
	}

	if _, ok := r.lookup("m0.photo"); ok {
		t.Error("lookup() found the oldest entry, want it evicted past the limit")
	}

	newest := fmt.Sprintf("m%d.photo", refusedDisplayCopiesLimit)
	if _, ok := r.lookup(newest); !ok {
		t.Error("lookup() did not find the most recently remembered entry")
	}
}

// TestRefusedDisplayCopies_RememberingAgainDoesNotDuplicateTheEntry
// covers remember called twice for the same id - recoverUnsafeImage's
// own retry failing the same way the first attempt's bare download
// did not, say - which must update the reason in place rather than
// letting the eviction order carry two entries for one id.
func TestRefusedDisplayCopies_RememberingAgainDoesNotDuplicateTheEntry(t *testing.T) {
	t.Parallel()

	r := newRefusedDisplayCopies()
	r.remember("m1.photo", MediaDownloadFailed)
	r.remember("m1.photo", MediaTooLarge)

	if got := len(r.order); got != 1 {
		t.Fatalf("order length = %d, want 1: remembering the same id again must not grow it", got)
	}

	reason, ok := r.lookup("m1.photo")
	if !ok || reason != MediaTooLarge {
		t.Errorf("lookup() = %v, %v; want the latest reason %v, true", reason, ok, MediaTooLarge)
	}
}
