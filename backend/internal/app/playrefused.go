package app

import "sync"

// refusedPlayCopiesLimit bounds how many ids refusedPlayCopies
// remembers at once, oldest dropped first: a hostile conversation
// could in principle carry an unbounded number of distinct videos
// whose play copy never builds, and this set must never grow with it
// for a run's whole lifetime. The same figure refusedDisplayCopies
// uses, for the same reason.
const refusedPlayCopiesLimit = 500

// refusedPlayCopies remembers messages whose play copy has already
// failed to build once - the transcoder timing out, running out of
// its own size budget, or simply being unavailable - so playCopy can
// fail fast on a later call for the same message rather than running
// the sandboxed transcoder all over again: reopening a conversation
// asks to play every message in it again, so without this, a video
// whose transcode a crafted file made time out or fail once would
// cost exactly as much every single time it is reopened (see
// docs/decisions.md). Keyed by the same play-copy cache id
// transcodedPlayCopy itself uses, so an edit that changes a message's
// media kind is never stuck behind an earlier kind's own refusal.
// Cleared on restart: refusing is re-learned soon enough regardless,
// and nothing durable is lost by forgetting it.
type refusedPlayCopies struct {
	mu    sync.Mutex
	order []string // oldest id first, for eviction
	by    map[string]bool
}

// newRefusedPlayCopies returns an empty refusedPlayCopies.
func newRefusedPlayCopies() *refusedPlayCopies {
	return &refusedPlayCopies{by: map[string]bool{}}
}

// remember records id as refused, evicting the oldest entry once this
// would push the set past refusedPlayCopiesLimit.
func (r *refusedPlayCopies) remember(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.by[id] {
		r.order = append(r.order, id)
	}
	r.by[id] = true

	for len(r.order) > refusedPlayCopiesLimit {
		delete(r.by, r.order[0])
		r.order = r.order[1:]
	}
}

// refused reports whether id has already been remembered as refused.
func (r *refusedPlayCopies) refused(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.by[id]
}
