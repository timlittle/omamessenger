package app

import (
	"errors"
	"sync"
)

// refusedDisplayCopiesLimit bounds how many refusals
// refusedDisplayCopies remembers at once, oldest dropped first, so a
// hostile conversation can never grow this map unbounded.
const refusedDisplayCopiesLimit = 500

// refusedDisplayCopies remembers messages whose display copy already
// failed imagesafe's check once, so FetchMedia can fail fast on a
// later call instead of re-downloading and re-decoding it (see
// docs/decisions.md). Keyed by cacheIdentity, so an edit that changes
// media kind is never stuck behind an earlier kind's refusal. Cleared
// on restart.
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
// from refusedDisplayCopies rather than a fresh download or decode.
// The UI only ever sees the same safe Reason it always would (see
// MediaFetchError).
var errRefusedDisplayCopyCached = errors.New("media: display copy already refused this run")
