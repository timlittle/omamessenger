package whatsapp

// appstate.go holds the shared recovery path for app-state patches sent
// by organize.go, delete.go and markread.go: when WhatsApp's server
// rejects a patch outright, whatsmeow's own built-in retry only
// re-applies one conflict response and gives up, so an account more
// than one page behind stays stuck. sendAppStateRecovering adds a full
// incremental re-fetch, and, if that still fails, a full resync from
// the server's snapshot, before a final retry.
//
// appStateRejectionDetail reads the server's error text, which
// errors.md otherwise forbids comparing on: it only extracts the two
// named attributes WhatsApp's <error> node carries, purely for
// logging, and never branches on what it finds.

import (
	"context"
	"errors"
	"log"
	"regexp"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// errAppStateUnverifiable wraps whatsmeow.ErrAppStateUpdate when both
// catch-up rounds failed on a corrupt local copy, not just one that
// fell behind: retrying later would cost traffic for no gain. Only
// sendOrganizePatch checks for this.
var errAppStateUnverifiable = errors.New("whatsapp: app state collection cannot be verified")

// sendAppStateRecovering sends patch through dev, and on rejection
// retries after an incremental catch-up, then after a full resync, before
// giving up and logging the rejection.
func sendAppStateRecovering(ctx context.Context, dev device, patch appstate.PatchInfo) error {
	err := dev.sendAppState(ctx, patch)
	if !errors.Is(err, whatsmeow.ErrAppStateUpdate) {
		return err
	}

	var incrementalClass string
	incrementalClass, err = appStateCatchUp(ctx, dev, patch, false, err)
	if !errors.Is(err, whatsmeow.ErrAppStateUpdate) {
		return err
	}

	var fullClass string
	fullClass, err = appStateCatchUp(ctx, dev, patch, true, err)
	if !errors.Is(err, whatsmeow.ErrAppStateUpdate) {
		return err
	}

	logAppStateRejected(patch.Type, err)
	if incrementalClass == appStateClassCorrupt && fullClass == appStateClassCorrupt {
		return errors.Join(err, errAppStateUnverifiable)
	}

	return err
}

// appStateCatchUp fetches patch.Type's app-state, incrementally or (if
// fullSync) from the server's snapshot, then retries patch. On a fetch
// failure it returns rejection unchanged, plus the failure's class so
// the caller can tell a corrupt copy apart from merely falling behind.
func appStateCatchUp(ctx context.Context, dev device, patch appstate.PatchInfo, fullSync bool, rejection error) (class string, err error) {
	if fetchErr := dev.fetchAppState(ctx, patch.Type, fullSync); fetchErr != nil {
		logAppStateFetchFailed(patch.Type, fullSync, fetchErr)
		return fetchAppStateErrClass(fetchErr), rejection
	}

	return "", dev.sendAppState(ctx, patch)
}

// sendOrganizePatch is the choke point every app-state write goes
// through: it skips the server once patch.Type is known unverifiable,
// otherwise sends via sendAppStateRecovering and records the first
// unverifiable result it sees.
func (c *Connector) sendOrganizePatch(ctx context.Context, dev device, patch appstate.PatchInfo) error {
	if c.organizeLocalOnly(patch.Type) {
		return connector.ErrOrganizeLocalOnly
	}

	err := classifyAppStateErr(sendAppStateRecovering(ctx, dev, patch))
	if errors.Is(err, connector.ErrOrganizeLocalOnly) {
		c.setOrganizeLocalOnly(ctx, patch.Type, true)
	}

	return err
}

// classifyAppStateErr joins a generic connector.Err* sentinel onto err
// so callers outside this connector can distinguish an incomplete sync
// from an outright rejection without importing whatsmeow.
func classifyAppStateErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errAppStateUnverifiable):
		return errors.Join(connector.ErrOrganizeLocalOnly, err)
	case errors.Is(err, appstate.ErrKeyNotFound):
		return errors.Join(connector.ErrOrganizeSyncIncomplete, err)
	case errors.Is(err, whatsmeow.ErrAppStateUpdate):
		return errors.Join(connector.ErrOrganizeRejected, err)
	default:
		return err
	}
}

// appStateClassCorrupt marks a catch-up fetch whose local copy no
// longer agrees with the server at all, rather than merely lagging.
const appStateClassCorrupt = "corrupt"

// appStateErrClass categorizes why sending an app-state patch failed,
// safe to log: "key", "rejected", "timeout" or "other".
func appStateErrClass(err error) string {
	switch {
	case errors.Is(err, appstate.ErrKeyNotFound):
		return "key"
	case errors.Is(err, whatsmeow.ErrAppStateUpdate):
		return "rejected"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "timeout"
	default:
		return "other"
	}
}

// fetchAppStateErrClass categorizes why a catch-up fetch failed, safe
// to log: "corrupt" when the local copy's hash or MAC no longer
// agrees with the server, "key" and "timeout" as in appStateErrClass,
// else "other".
func fetchAppStateErrClass(err error) string {
	switch {
	case errors.Is(err, appstate.ErrMismatchingLTHash),
		errors.Is(err, appstate.ErrMismatchingPatchMAC),
		errors.Is(err, appstate.ErrMismatchingContentMAC),
		errors.Is(err, appstate.ErrMismatchingIndexMAC),
		errors.Is(err, appstate.ErrMissingPreviousSetValueOperation):
		return appStateClassCorrupt
	case errors.Is(err, appstate.ErrKeyNotFound):
		return "key"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "timeout"
	default:
		return "other"
	}
}

// appStateErrorCodePattern and appStateErrorTextPattern each match one
// named attribute of the <error> node WhatsApp's server puts in a
// rejected patch's response.
var (
	appStateErrorCodePattern = regexp.MustCompile(`<error\b[^>]*\bcode="(\d+)"`)
	appStateErrorTextPattern = regexp.MustCompile(`<error\b[^>]*\btext="([^"]*)"`)
)

// appStateRejectionDetail returns the server's code and reason text
// for a rejected patch, read out of err's message, or "" when not
// found.
func appStateRejectionDetail(err error) (code, text string) {
	msg := err.Error()
	if m := appStateErrorCodePattern.FindStringSubmatch(msg); m != nil {
		code = m[1]
	}
	if m := appStateErrorTextPattern.FindStringSubmatch(msg); m != nil {
		text = m[1]
	}

	return code, text
}

// logAppStateRejected reports an app-state patch that WhatsApp's
// server still rejected after catch-up, with its error class and the
// server's code and text.
func logAppStateRejected(patchType appstate.WAPatchName, err error) {
	code, text := appStateRejectionDetail(err)
	log.Printf("whatsapp: app state patch rejected (type=%s, error_class=%s, server_code=%s, server_text=%s)",
		patchType, appStateErrClass(err), code, text)
}

// logAppStateFetchFailed reports a catch-up fetch itself failing, by
// its safe error class.
func logAppStateFetchFailed(patchType appstate.WAPatchName, fullSync bool, err error) {
	log.Printf("whatsapp: app state fetch failed (type=%s, full_sync=%t, error_class=%s)",
		patchType, fullSync, fetchAppStateErrClass(err))
}
