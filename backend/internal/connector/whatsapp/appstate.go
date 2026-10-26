package whatsapp

// appstate.go is shared by every app-state patch this connector sends -
// pin and archive (organize.go), delete-for-me (delete.go) and the
// chat-read mutation (markread.go) - for the one failure a real
// account's log showed all of them sharing: WhatsApp's server
// rejecting the patch outright (whatsmeow.ErrAppStateUpdate), not just
// the 409 version conflict whatsmeow's own SendAppState already
// retries once internally.
//
// Reading SendAppState in the pinned whatsmeow module (appstate.go, via
// go env GOMODCACHE) shows why that single internal retry is not always
// enough: on a 409, it applies only the patches included in that one
// conflict response, then retries once more with no further retry of
// its own. An account whose local copy of this patch type (always
// WAPatchRegularLow for pin, archive and mark-read: appstate.BuildPin,
// BuildArchive and BuildMarkChatAsRead all set it) fell behind by more
// than that one response's own page is still behind after the retry, so
// the second attempt conflicts again and whatsmeow gives up, returning
// the same ErrAppStateUpdate. sendAppStateRecovering adds two things
// whatsmeow's own retry does not do: a full incremental fetch
// (fetchAppState, every page, not just one response) before trying the
// patch again itself, and, when even that still fails, a full resync
// from the server's own snapshot (fetchAppState again, with fullSync
// true) before one final retry. The second step matters because a
// behind-by-more-than-one-page account is not the only way this patch
// type's local copy can go wrong: appstate/hash.go's DecodePatches
// returns ErrMismatchingLTHash when the running hash it keeps no
// longer agrees with the version number alongside it, and
// ErrMismatchingPatchMAC, ErrMismatchingContentMAC,
// ErrMismatchingIndexMAC or ErrMissingPreviousSetValueOperation when
// one mutation's own authentication fails - all of them a corrupted or
// diverged local copy, something no amount of incremental catching up
// can repair, since every incremental fetch starts from that same bad
// copy. FetchAppState's fullSync argument is the only way to fix that:
// it deletes this device's own stored version for the patch type first
// (Client.fetchAppState, same module), so the next fetch has nothing
// local left to agree or disagree with and rebuilds both the version
// and the hash from the server's snapshot alone.
//
// This connector cannot tell a 409 version conflict apart from any
// other server rejection without comparing the error's own text, which
// errors.md forbids: whatsmeow wraps both the same way, in
// ErrAppStateUpdate, and the structured error code only ever reaches
// this connector formatted into that error's message. Retrying after a
// fetch is harmless either way - a rejection that was never a version
// conflict at all just fails the same way again, no worse off for the
// extra round trip - so sendAppStateRecovering always works through
// both rounds rather than trying to guess which one a given failure
// needs.
//
// appStateRejectionDetail is the one place this file does read an
// error's own text, and only for logging: WhatsApp's server puts a
// numeric code and a short reason text on the <error> node whatsmeow
// formats into ErrAppStateUpdate's own message (see SendAppState in
// the same module), and extracting exactly those two named attributes
// - never anything else the node might carry, such as a JID - is safe
// under errors.md's rule against comparing error text, which is about
// using that text to decide what the code does next. Nothing here
// branches on what the regexp finds; logAppStateRejected only ever
// writes it to a log line after sendAppStateRecovering has already
// given up.

import (
	"context"
	"errors"
	"log"
	"regexp"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// sendAppStateRecovering sends patch through dev, and, when WhatsApp's
// server rejects it, works through two rounds of catching this
// account's own app-state up before giving up: first an incremental
// fetch and one retry (appStateCatchUp with fullSync false), then,
// only if that still fails, a full resync from the server's snapshot
// and one final retry (fullSync true). See this file's own doc
// comment for why a single incremental catch-up is not always enough
// on its own. Any failure that was never a rejection at all, such as a
// timeout or a missing app-state key, is returned at once, since
// neither round can fix it; the final rejection, if both rounds still
// leave one, is logged with the server's own code and reason text
// before it is returned.
func sendAppStateRecovering(ctx context.Context, dev device, patch appstate.PatchInfo) error {
	err := dev.sendAppState(ctx, patch)
	if !errors.Is(err, whatsmeow.ErrAppStateUpdate) {
		return err
	}

	err = appStateCatchUp(ctx, dev, patch, false, err)
	if !errors.Is(err, whatsmeow.ErrAppStateUpdate) {
		return err
	}

	err = appStateCatchUp(ctx, dev, patch, true, err)
	if errors.Is(err, whatsmeow.ErrAppStateUpdate) {
		logAppStateRejected(patch.Type, err)
	}

	return err
}

// appStateCatchUp fetches patch.Type's own app-state - incrementally,
// or, when fullSync is true, from the server's snapshot - and, once
// that succeeds, retries patch once more. rejection is the failure
// that made this attempt necessary; it is what appStateCatchUp
// returns unchanged when the fetch itself fails, since the caller
// already has nothing more useful to report, after logging that
// fetch's own error by its safe class (see fetchAppStateErrClass):
// previously swallowed entirely, which left no trace of why even an
// incremental catch-up could not run at all.
func appStateCatchUp(ctx context.Context, dev device, patch appstate.PatchInfo, fullSync bool, rejection error) error {
	if err := dev.fetchAppState(ctx, patch.Type, fullSync); err != nil {
		logAppStateFetchFailed(patch.Type, fullSync, err)
		return rejection
	}

	return dev.sendAppState(ctx, patch)
}

// classifyAppStateErr joins a generic connector.Err* sentinel into err
// for whichever whatsmeow-specific failure it was, so code outside
// this connector - never importing whatsmeow itself - can still tell a
// sync state problem (connector.ErrOrganizeSyncIncomplete, this
// account's own app-state keys or version never caught up) apart from
// an outright rejection (connector.ErrOrganizeRejected). A context
// timeout or anything else this connector has no name for is returned
// unchanged, the same as before.
func classifyAppStateErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, appstate.ErrKeyNotFound):
		return errors.Join(connector.ErrOrganizeSyncIncomplete, err)
	case errors.Is(err, whatsmeow.ErrAppStateUpdate):
		return errors.Join(connector.ErrOrganizeRejected, err)
	default:
		return err
	}
}

// appStateErrClass categorizes why sending an app-state patch failed,
// safe to log alongside it: "key" when whatsmeow has no app-state key
// to encrypt or decrypt it with yet, which happens when the account
// has not finished its post-pairing app-state sync; "rejected" when
// WhatsApp's own server reported the patch invalid; "timeout" for a
// context that ran out waiting on either; and "other" for anything
// else, such as a plain network failure. Never the error's own text,
// which whatsmeow does not guarantee free of a JID or similar detail -
// see this file's own doc comment for the one, narrow exception.
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

// fetchAppStateErrClass categorizes why a catch-up fetch for an
// app-state patch type failed, safe to log: "corrupt" when this
// account's own locally cached copy no longer agrees with the
// server's hash or a mutation's own MAC (see this file's own doc
// comment for the specific appstate package sentinels this covers),
// which is sendAppStateRecovering's own cue that an incremental fetch
// was never going to fix this and a full resync is what the next
// round tries; "key" and "timeout" the same as appStateErrClass; and
// "other" for anything else, such as a plain network failure.
func fetchAppStateErrClass(err error) string {
	switch {
	case errors.Is(err, appstate.ErrMismatchingLTHash),
		errors.Is(err, appstate.ErrMismatchingPatchMAC),
		errors.Is(err, appstate.ErrMismatchingContentMAC),
		errors.Is(err, appstate.ErrMismatchingIndexMAC),
		errors.Is(err, appstate.ErrMissingPreviousSetValueOperation):
		return "corrupt"
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
// rejected app-state patch's response, wherever it falls in that
// node's own opening tag; see this file's own doc comment for why
// reading just these two, by name, is the one case a regexp over an
// error's text is safe here.
var (
	appStateErrorCodePattern = regexp.MustCompile(`<error\b[^>]*\bcode="(\d+)"`)
	appStateErrorTextPattern = regexp.MustCompile(`<error\b[^>]*\btext="([^"]*)"`)
)

// appStateRejectionDetail returns the server's own code and reason
// text for a rejected app-state patch, read out of err's own formatted
// message (see this file's own doc comment), or "" for whichever one
// was not found - a server response with no <error> node at all, or
// one whose node carries neither attribute.
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

// logAppStateRejected reports an app-state patch of patchType that
// WhatsApp's server still rejected after sendAppStateRecovering's own
// catch-up attempts, with the safe class the rejection falls into and
// the server's own code and text when its reply carried one (see
// appStateRejectionDetail) - never anything else from that node, such
// as a JID, which privacy.md forbids logging.
func logAppStateRejected(patchType appstate.WAPatchName, err error) {
	code, text := appStateRejectionDetail(err)
	log.Printf("whatsapp: app state patch rejected (type=%s, error_class=%s, server_code=%s, server_text=%s)",
		patchType, appStateErrClass(err), code, text)
}

// logAppStateFetchFailed reports a catch-up fetch for patchType -
// incremental, or, when fullSync is true, a full resync - itself
// failing, by its safe class (see fetchAppStateErrClass): previously
// swallowed entirely by sendAppStateRecovering, which left no trace of
// why even an incremental catch-up could not run.
func logAppStateFetchFailed(patchType appstate.WAPatchName, fullSync bool, err error) {
	log.Printf("whatsapp: app state fetch failed (type=%s, full_sync=%t, error_class=%s)",
		patchType, fullSync, fetchAppStateErrClass(err))
}
