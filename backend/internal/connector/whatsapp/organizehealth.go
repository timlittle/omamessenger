package whatsapp

// organizehealth.go is where this connector remembers, across the
// process and across a restart, that one of the app-state collections
// it patches (always WAPatchRegularLow for pin, archive and the
// chat-read mutation, see appstate.go's own doc comment; WAPatchRegularHigh
// for delete-for-me) can no longer be verified against WhatsApp's
// server at all. A real account's logs showed the symptom: an
// incremental fetch AND a full-sync fetch both failing because the
// local copy is corrupt, with the retried patch still rejected by the
// server afterwards (see sendOrganizePatch in appstate.go). That is
// the open upstream bug at
// https://github.com/tulir/whatsmeow/issues/1301: on an account whose
// phone has moved its chats to LID addressing, WhatsApp's own
// regular_low snapshot fails its own verification, so this companion
// can never catch up and every regular_low write keeps getting
// rejected. Once sendOrganizePatch finds that, every later call for
// the same patch type skips WhatsApp's server entirely, reporting
// connector.ErrOrganizeLocalOnly at once instead of repeating the same
// expensive two-round recovery for nothing; runOrganizeHealthCheck is
// the only thing that ever tries again, so an upstream fix or
// WhatsApp repairing the account itself is noticed without needing a
// restart (see docs/decisions.md for why this is not instead fixed by
// relaxing verification).

import (
	"context"
	"log"
	"time"

	"go.mau.fi/whatsmeow/appstate"
)

// organizeHealthCheckInterval is how often runOrganizeHealthCheck
// retries a patch type found unsyncable, after checking once right
// away at Run's own start: daily is often enough for a condition that
// otherwise needs a restart to notice has cleared.
const organizeHealthCheckInterval = 24 * time.Hour

// organizeLocalOnly reports whether patchType's own app-state
// collection has already been found unverifiable for this account.
func (c *Connector) organizeLocalOnly(patchType appstate.WAPatchName) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.organizeUnsyncable[patchType]
}

// loadOrganizeHealth resumes patchType's unsyncable state exactly as
// a previous run last persisted it (see mediaStore.organizeUnsyncable),
// without logging or writing it straight back: resuming already-known
// state is not itself a new discovery (see logOrganizeHealthChanged).
func (c *Connector) loadOrganizeHealth(patchType appstate.WAPatchName, unsyncable bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.organizeUnsyncable == nil {
		c.organizeUnsyncable = map[appstate.WAPatchName]bool{}
	}
	c.organizeUnsyncable[patchType] = unsyncable
}

// setOrganizeLocalOnly records whether patchType can currently be
// trusted to reach WhatsApp's server, persisting the change to this
// account's own media store when one is open, and logging once on
// each actual change - becoming true the first time sendOrganizePatch
// finds it unverifiable, and clearing again the moment
// recheckOrganizeHealth verifies it - rather than on every call that
// finds it already set.
func (c *Connector) setOrganizeLocalOnly(ctx context.Context, patchType appstate.WAPatchName, unsyncable bool) {
	c.mu.Lock()
	if c.organizeUnsyncable == nil {
		c.organizeUnsyncable = map[appstate.WAPatchName]bool{}
	}
	changed := c.organizeUnsyncable[patchType] != unsyncable
	c.organizeUnsyncable[patchType] = unsyncable
	c.mu.Unlock()

	if !changed {
		return
	}

	logOrganizeHealthChanged(patchType, unsyncable)

	media := c.mediaFor()
	if media == nil {
		return
	}
	if err := media.putOrganizeUnsyncable(ctx, patchType, unsyncable); err != nil {
		log.Printf("whatsapp: organize health persist failed")
	}
}

// logOrganizeHealthChanged is the one place sendOrganizePatch's
// discovery, or recheckOrganizeHealth's recovery, is logged (see
// setOrganizeLocalOnly): every later call that finds the state
// unchanged logs nothing further, so a flagged account does not spam
// this line on every pin, archive, mark-read or delete.
func logOrganizeHealthChanged(patchType appstate.WAPatchName, unsyncable bool) {
	if unsyncable {
		log.Printf("whatsapp: regular_low unverifiable, keeping organize changes local (upstream whatsmeow#1301, type=%s)", patchType)
		return
	}

	log.Printf("whatsapp: %s verified again, resuming organize sync", patchType)
}

// runOrganizeHealthCheck retries recheckOrganizeHealth once right
// away, then every organizeHealthCheckInterval, until ctx is
// cancelled - Run's own per-run context (see waitForStop in
// connector.go), so this never outlives the dev it was given.
func (c *Connector) runOrganizeHealthCheck(ctx context.Context, dev device) {
	c.recheckOrganizeHealth(ctx, dev)

	ticker := time.NewTicker(organizeHealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.recheckOrganizeHealth(ctx, dev)
		}
	}
}

// recheckOrganizeHealth retries one incremental fetch of
// WAPatchRegularLow - the one patch type the real account diagnosing
// this found permanently rejected (see this file's own doc comment) -
// clearing organizeLocalOnly the moment that fetch succeeds, so
// SetPinned, SetArchived and the chat-read mutation resume talking to
// WhatsApp's server again without needing a restart. It does nothing,
// and makes no call at all, while nothing is currently flagged, so a
// healthy account never gets any of this extra traffic.
func (c *Connector) recheckOrganizeHealth(ctx context.Context, dev device) {
	if !c.organizeLocalOnly(appstate.WAPatchRegularLow) {
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, organizeTimeout)
	defer cancel()
	if err := dev.fetchAppState(checkCtx, appstate.WAPatchRegularLow, false); err != nil {
		logAppStateFetchFailed(appstate.WAPatchRegularLow, false, err)
		return
	}

	c.setOrganizeLocalOnly(ctx, appstate.WAPatchRegularLow, false)
}
