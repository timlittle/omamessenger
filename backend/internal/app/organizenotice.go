package app

// organizenotice.go turns connector.ErrOrganizeLocalOnly - a
// connector reporting that a pin, archive, mark-read or delete change
// already stands locally but could not be told to the service, with
// no point retrying until some outside condition clears (WhatsApp's
// own app-state verification bug today, see docs/decisions.md) - into
// a safe, one-time toast instead of the generic failure banner
// SetPinned, SetArchived and DeleteMessages would otherwise show.

import (
	"context"
	"errors"
	"sync"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// organizeLocalOnlyNotice is the one-time toast text shown instead of
// a failure banner the first time an account's connector reports
// connector.ErrOrganizeLocalOnly. It names WhatsApp because every
// connector able to report this sentinel today is one; a second
// service that can report it would need its own wording, not this one
// stretched to fit.
const organizeLocalOnlyNotice = "WhatsApp can't sync pins, archive and read state for this account yet, so they're kept on this computer."

// organizeNotices remembers which accounts this helper run has already
// shown organizeLocalOnlyNotice for, so it is published once per
// account per run rather than on every pin, archive, mark-read or
// delete that finds the condition still standing. A run is chosen over
// persisting the dismissal, the way the "What's new" hint persists
// which version was last seen: the underlying condition is already
// logged once by the connector and resumed across a restart there (see
// whatsapp/organizehealth.go), so showing this quiet toast again after
// a restart is a reasonable, low-noise reminder rather than something
// that needs its own stored dismissal.
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

// organizeLocalOnlyHandled reports whether err wraps
// connector.ErrOrganizeLocalOnly, and if so, publishes the one-time
// notice for accountID the first time this run sees it for that
// account. The local change the caller already applied stands either
// way; a caller that gets true back treats err as if it were nil,
// never as a failure to report to the user.
func (c *Commands) organizeLocalOnlyHandled(ctx context.Context, accountID string, err error) bool {
	if !errors.Is(err, connector.ErrOrganizeLocalOnly) {
		return false
	}

	if c.organizeNotices.markShown(accountID) {
		c.events.publish(ctx, EventAccountNotice, AccountNotice{AccountID: accountID, Message: organizeLocalOnlyNotice})
	}

	return true
}
