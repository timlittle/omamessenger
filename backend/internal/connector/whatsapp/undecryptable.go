package whatsapp

// undecryptable.go tracks messages whatsmeow could not decrypt, so a
// later redelivery of the same id can replace the stored placeholder
// instead of being silently deduplicated away by the store (see
// live.go's handleUndecryptable). A message that never gets
// redelivered - by bad luck, or because a sender crafted it to never
// decrypt - must not be allowed to grow this cache forever, so it gets
// the same bound and eviction react.go gives its own reaction cache.

import "slices"

// maxUndecryptableTargets bounds how many messages this connector
// waits on a decryptable redelivery for at once. Generous for any real
// account's own backlog of stuck messages; past it, the oldest message
// waited on so far is forgotten to make room for a new one.
const maxUndecryptableTargets = 2000

// markUndecryptable records that the message named by
// conversationRemoteID and messageRemoteID was stored only as a
// placeholder, because whatsmeow could not decrypt it, evicting the
// oldest message already waited on if maxUndecryptableTargets is
// already reached (see resolveUndecryptable).
func (c *Connector) markUndecryptable(conversationRemoteID, messageRemoteID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.undecryptable == nil {
		c.undecryptable = map[string]bool{}
	}

	key := conversationRemoteID + "/" + messageRemoteID
	if c.undecryptable[key] {
		return
	}

	if len(c.undecryptableOrder) >= maxUndecryptableTargets {
		oldest := c.undecryptableOrder[0]
		c.undecryptableOrder = c.undecryptableOrder[1:]
		delete(c.undecryptable, oldest)
	}

	c.undecryptable[key] = true
	c.undecryptableOrder = append(c.undecryptableOrder, key)
}

// resolveUndecryptable reports whether the message named by
// conversationRemoteID and messageRemoteID was waiting on a placeholder
// (see markUndecryptable), forgetting it either way so a later message
// that happens to reuse the same id is never treated as a replacement
// again.
func (c *Connector) resolveUndecryptable(conversationRemoteID, messageRemoteID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := conversationRemoteID + "/" + messageRemoteID
	found := c.undecryptable[key]
	delete(c.undecryptable, key)
	if i := slices.Index(c.undecryptableOrder, key); i >= 0 {
		c.undecryptableOrder = slices.Delete(c.undecryptableOrder, i, i+1)
	}

	return found
}
