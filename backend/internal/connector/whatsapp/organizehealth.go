package whatsapp

// organizehealth.go remembers, across the process and a restart, that
// one of the app-state collections this connector patches can no
// longer be verified against WhatsApp's server (upstream bug
// https://github.com/tulir/whatsmeow/issues/1301). Once
// sendOrganizePatch finds that, later calls skip the server entirely
// and report connector.ErrOrganizeLocalOnly; runOrganizeHealthCheck
// is the only thing that retries (see docs/decisions.md).

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

// loadOrganizeHealth resumes patchType's unsyncable state as a
// previous run last persisted it, without logging it: resuming
// known state is not a new discovery (see logOrganizeHealthChanged).
func (c *Connector) loadOrganizeHealth(patchType appstate.WAPatchName, unsyncable bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.organizeUnsyncable == nil {
		c.organizeUnsyncable = map[appstate.WAPatchName]bool{}
	}
	c.organizeUnsyncable[patchType] = unsyncable
}

// setOrganizeLocalOnly records whether patchType can currently be
// trusted to reach WhatsApp's server, persisting the change and
// logging it, but only on an actual change.
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

// logOrganizeHealthChanged is the one place a discovery or recovery
// is logged (see setOrganizeLocalOnly), so a flagged account does not
// spam this line on every pin, archive, mark-read or delete.
func logOrganizeHealthChanged(patchType appstate.WAPatchName, unsyncable bool) {
	if unsyncable {
		log.Printf("whatsapp: regular_low unverifiable, keeping organize changes local (upstream whatsmeow#1301, type=%s)", patchType)
		return
	}

	log.Printf("whatsapp: %s verified again, resuming organize sync", patchType)
}

// runOrganizeHealthCheck retries recheckOrganizeHealth once right
// away, then every organizeHealthCheckInterval, until ctx is cancelled.
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
// WAPatchRegularLow, clearing organizeLocalOnly the moment it
// succeeds. Does nothing while nothing is currently flagged.
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
