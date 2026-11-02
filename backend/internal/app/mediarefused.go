package app

import (
	"errors"
	"sync"
)

// refusedDisplayCopiesLimit bounds how many refusals
// refusedDisplayCopies remembers at once, oldest dropped first: a
// hostile conversation could in principle carry an unbounded number
// of distinct refused photos, and this map must never grow with it
// for a run's whole lifetime. Generous for any one conversation's
// worth of them.
const refusedDisplayCopiesLimit = 500

// refusedDisplayCopies remembers messages whose display copy has
// already failed imagesafe's own safety check once - including the
// one re-download retry recoverUnsafeImage gives it - so FetchMedia
// can fail fast on a later call for the same message rather than
// downloading and decoding it all over again: the UI auto-fetches
// every visible photo, so without this, a hostile one would pay that
// cost again on every scroll (see docs/decisions.md). Keyed by
// cacheIdentity, the same id-plus-kind pairing the downloaded media
// cache itself uses, so an edit that changes a message's media kind
// is never stuck behind an earlier kind's own refusal. Cleared on
// restart: it holds nothing durable, only a safe reason category
// already fine to log (see MediaFetchReason), and a wrong refusal is
// re-learned soon enough regardless.
type refusedDisplayCopies struct {
	mu    sync.Mutex
	order []string // oldest id first, for eviction
	by    map[string]MediaFetchReason
}

// newRefusedDisplayCopies returns an empty refusedDisplayCopies.
func newRefusedDisplayCopies() *refusedDisplayCopies {
	return &refusedDisplayCopies{by: map[string]MediaFetchReason{}}
}

// remember records id as refused for reason, evicting the oldest
// entry once this would push the set past refusedDisplayCopiesLimit.
func (r *refusedDisplayCopies) remember(id string, reason MediaFetchReason) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.by[id]; !ok {
		r.order = append(r.order, id)
	}
	r.by[id] = reason

	for len(r.order) > refusedDisplayCopiesLimit {
		delete(r.by, r.order[0])
		r.order = r.order[1:]
	}
}

// lookup reports id's remembered refusal reason, and whether it has
// one at all.
func (r *refusedDisplayCopies) lookup(id string) (MediaFetchReason, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	reason, ok := r.by[id]

	return reason, ok
}

// errRefusedDisplayCopyCached reports a FetchMedia that failed fast
// from refusedDisplayCopies rather than a fresh download or decode
// this time. Only the helper's own log and MediaFetchError.Error ever
// read this; the UI sees nothing but the same safe Reason it always
// would (see MediaFetchError).
var errRefusedDisplayCopyCached = errors.New("media: display copy already refused this run")
