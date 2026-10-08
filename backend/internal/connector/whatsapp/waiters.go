package whatsapp

// waiters.go holds the generic table retry.go and history_ondemand.go
// each keep one of: a map from a key (a message or chat remote id) to
// the channel of the goroutine waiting for WhatsApp's asynchronous
// answer to whatever it asked the primary phone, so the event dispatcher
// (events.go, history.go) has somewhere to deliver that answer once it
// arrives, matched back to the right caller by the same key.

import "sync"

// waiterTable matches a key to the channel of the one call currently
// waiting for its answer. V is the answer's own type: a media retry's
// *events.MediaRetry in retry.go, an on-demand history sync's reported
// message count (int) in history_ondemand.go.
type waiterTable[V any] struct {
	mu      sync.Mutex
	waiters map[string]chan V
}

// register reserves key's slot in the table and returns the channel to
// wait on. When refuseExisting is true, a second register for a key
// already waiting fails instead of replacing that first channel; the ok
// result reports which happened. Call cleanup, passing back the same
// channel, once the wait ends, successfully or not, so a request
// nobody is listening for any more cannot accumulate in the table
// forever.
func (t *waiterTable[V]) register(key string, refuseExisting bool) (ch <-chan V, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if refuseExisting {
		if _, exists := t.waiters[key]; exists {
			return nil, false
		}
	}

	c := make(chan V, 1)
	if t.waiters == nil {
		t.waiters = map[string]chan V{}
	}
	t.waiters[key] = c

	return c, true
}

// cleanup removes key's entry, once the call that registered ch has
// stopped waiting, but only when key's slot still holds that exact
// channel: register with refuseExisting false lets a second caller for
// the same key overwrite the first's slot with its own channel, and
// without this check the first caller's own deferred cleanup would then
// delete the second caller's still-live entry, dropping whatever answer
// was meant for it.
func (t *waiterTable[V]) cleanup(key string, ch <-chan V) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.waiters[key] == ch {
		delete(t.waiters, key)
	}
}

// deliver hands value to whichever call is waiting on key, or drops it
// when nothing is waiting: an answer for a key nobody registered, or one
// that already timed out and stopped listening.
func (t *waiterTable[V]) deliver(key string, value V) {
	t.mu.Lock()
	c := t.waiters[key]
	t.mu.Unlock()

	if c == nil {
		return
	}

	select {
	case c <- value:
	default:
	}
}
