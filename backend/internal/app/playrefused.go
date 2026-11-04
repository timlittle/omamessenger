package app

import "sync"

// refusedPlayCopiesLimit bounds how many ids refusedPlayCopies
// remembers at once, oldest dropped first, since a hostile
// conversation could otherwise grow this set unbounded.
const refusedPlayCopiesLimit = 500

// refusedPlayCopies remembers messages whose play copy has already
// failed to build once, so playCopy can fail fast on a later call
// rather than running the sandboxed transcoder again (see
// docs/decisions.md). Cleared on restart.
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
