package app

// organizenotice.go turns connector.ErrOrganizeLocalOnly into a safe, one-time
// toast instead of the generic failure banner (see docs/decisions.md).

import (
	"context"
	"errors"
	"sync"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// organizeLocalOnlyNotice is the one-time toast shown the first time an
// account's connector reports connector.ErrOrganizeLocalOnly.
const organizeLocalOnlyNotice = "WhatsApp can't sync pins, archive and read state for this account yet, so they're kept on this computer."

// organizeNotices remembers which accounts this helper run has already shown
// organizeLocalOnlyNotice for, so it is published once per account per run.
type organizeNotices struct {
	mu    sync.Mutex
	shown map[string]bool
}

// markShown records accountID as having been shown the notice,
// reporting true the first time, false on every later call for the
// same account.
func (n *organizeNotices) markShown(accountID string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.shown == nil {
		n.shown = map[string]bool{}
	}
	if n.shown[accountID] {
		return false
	}
	n.shown[accountID] = true

	return true
}

// organizeLocalOnlyHandled reports whether err wraps connector.ErrOrganizeLocalOnly,
// publishing the one-time notice if so. A caller that gets true treats err as nil.
func (c *Commands) organizeLocalOnlyHandled(ctx context.Context, accountID string, err error) bool {
	if !errors.Is(err, connector.ErrOrganizeLocalOnly) {
		return false
	}

	if c.organizeNotices.markShown(accountID) {
		c.events.publish(ctx, EventAccountNotice, AccountNotice{AccountID: accountID, Message: organizeLocalOnlyNotice})
	}

	return true
}
