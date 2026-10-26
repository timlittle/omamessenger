package whatsapp

// appstate.go is shared by every app-state patch this connector sends -
// pin and archive (organize.go) and the chat-read mutation (markread.go)
// - for the one failure a real account's log showed all three sharing:
// WhatsApp's server rejecting the patch outright (whatsmeow.ErrAppStateUpdate),
// not just the 409 version conflict whatsmeow's own SendAppState already
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
// the same ErrAppStateUpdate. sendAppStateRecovering adds the one thing
// whatsmeow's own retry does not do: a full incremental fetch
// (fetchAppState, every page, not just one response) before trying the
// patch again itself.
//
// This connector cannot tell a 409 version conflict apart from any
// other server rejection without comparing the error's own text, which
// errors.md forbids: whatsmeow wraps both the same way, in
// ErrAppStateUpdate, and the structured error code only ever reaches
// this connector formatted into that error's message. Retrying after a
// fetch is harmless either way - a rejection that was never a version
// conflict at all just fails the same way again, no worse off for the
// extra round trip.

import (
	"context"
	"errors"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// sendAppStateRecovering sends patch through dev, and, when WhatsApp's
// server rejects it, fetches this account's own local catch-up for
// patch.Type and retries exactly once more before giving up: see this
// file's own doc comment for why whatsmeow's own one-time conflict
// retry is not always enough on its own. Any other failure, such as a
// timeout or a missing app-state key, is returned at once, since
// catching up cannot fix either.
func sendAppStateRecovering(ctx context.Context, dev device, patch appstate.PatchInfo) error {
	err := dev.sendAppState(ctx, patch)
	if !errors.Is(err, whatsmeow.ErrAppStateUpdate) {
		return err
	}

	if fetchErr := dev.fetchAppState(ctx, patch.Type); fetchErr != nil {
		// The original rejection is more useful to the caller than a
		// failure to even ask for a catch-up would be.
		return err
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
