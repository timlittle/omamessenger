package whatsapp

// appstate_test.go drives sendAppStateRecovering and classifyAppStateErr
// directly, through a fake device, rather than only indirectly through
// organize_test.go's and markread_test.go's own pin, archive and
// mark-read tests: both are shared by all three app-state patches this
// connector sends, so their own behaviour is proven once here.

import (
	"context"
	"errors"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// testPatch is a minimal, valid app-state patch for exercising
// sendAppStateRecovering's own retry logic, standing in for any of the
// three real patches (pin, archive, mark-read) that all carry the same
// WAPatchRegularLow type.
func testPatch() appstate.PatchInfo {
	return appstate.BuildPin(types.NewJID("15551234567", types.DefaultUserServer), true)
}

func TestSendAppStateRecovering_SucceedsOnTheFirstTry(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()

	if err := sendAppStateRecovering(t.Context(), dev, testPatch()); err != nil {
		t.Fatal(err)
	}
	if len(dev.fetchAppStateCalls) != 0 {
		t.Errorf("fetchAppState calls = %v, want none when the first send already succeeds", dev.fetchAppStateCalls)
	}
	if len(dev.appStatePatches) != 1 {
		t.Errorf("patches sent = %d, want 1", len(dev.appStatePatches))
	}
}

func TestSendAppStateRecovering_RetriesOnceAfterARejectionAndSucceeds(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.appStateErr = whatsmeow.ErrAppStateUpdate
	dev.appStateErrCount = 1

	if err := sendAppStateRecovering(t.Context(), dev, testPatch()); err != nil {
		t.Fatal(err)
	}
	if len(dev.fetchAppStateCalls) != 1 || dev.fetchAppStateCalls[0] != appstate.WAPatchRegularLow {
		t.Errorf("fetchAppState calls = %v, want one catch-up for %s", dev.fetchAppStateCalls, appstate.WAPatchRegularLow)
	}
	if len(dev.appStatePatches) != 2 {
		t.Errorf("patches sent = %d, want the patch retried once after the catch-up", len(dev.appStatePatches))
	}
}

func TestSendAppStateRecovering_ReturnsTheOriginalRejectionWhenTheCatchUpFails(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.appStateErr = whatsmeow.ErrAppStateUpdate
	dev.fetchAppStateErr = errors.New("network down")

	err := sendAppStateRecovering(t.Context(), dev, testPatch())
	if !errors.Is(err, whatsmeow.ErrAppStateUpdate) {
		t.Errorf("sendAppStateRecovering = %v, want the original rejection when the catch-up itself fails", err)
	}
	if len(dev.appStatePatches) != 1 {
		t.Errorf("patches sent = %d, want no retry once the catch-up fails", len(dev.appStatePatches))
	}
}

func TestSendAppStateRecovering_NeverFetchesForAFailureThatIsNotARejection(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.appStateErr = errors.New("offline")

	err := sendAppStateRecovering(t.Context(), dev, testPatch())
	if !errors.Is(err, dev.appStateErr) {
		t.Errorf("sendAppStateRecovering = %v, want it to wrap the device's own error", err)
	}
	if len(dev.fetchAppStateCalls) != 0 {
		t.Errorf("fetchAppState calls = %v, want none for a failure that was never a rejection", dev.fetchAppStateCalls)
	}
	if len(dev.appStatePatches) != 1 {
		t.Errorf("patches sent = %d, want no retry for a failure that was never a rejection", len(dev.appStatePatches))
	}
}

func TestSendAppStateRecovering_PropagatesAContextTimeoutWithoutFetching(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.appStateBlocks = true

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := sendAppStateRecovering(ctx, dev, testPatch())
	if !errors.Is(err, context.Canceled) {
		t.Errorf("sendAppStateRecovering = %v, want context.Canceled", err)
	}
	if len(dev.fetchAppStateCalls) != 0 {
		t.Errorf("fetchAppState calls = %v, want none for a cancelled context", dev.fetchAppStateCalls)
	}
}

func TestClassifyAppStateErr_SortsEachFailureIntoItsGenericCategory(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		err      error
		wantJoin error
	}{
		"nil passes through unchanged":        {err: nil, wantJoin: nil},
		"a missing app-state key":             {err: appstate.ErrKeyNotFound, wantJoin: connector.ErrOrganizeSyncIncomplete},
		"a server rejection":                  {err: whatsmeow.ErrAppStateUpdate, wantJoin: connector.ErrOrganizeRejected},
		"anything else is returned unchanged": {err: errors.New("offline"), wantJoin: nil},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := classifyAppStateErr(tt.err)
			if tt.err == nil {
				if got != nil {
					t.Errorf("classifyAppStateErr(nil) = %v, want nil", got)
				}
				return
			}

			if !errors.Is(got, tt.err) {
				t.Errorf("classifyAppStateErr(%v) = %v, want it to still wrap the original error", tt.err, got)
			}
			if tt.wantJoin != nil && !errors.Is(got, tt.wantJoin) {
				t.Errorf("classifyAppStateErr(%v) = %v, want it to also wrap %v", tt.err, got, tt.wantJoin)
			}
			if tt.wantJoin == nil {
				if errors.Is(got, connector.ErrOrganizeSyncIncomplete) || errors.Is(got, connector.ErrOrganizeRejected) {
					t.Errorf("classifyAppStateErr(%v) = %v, want no generic category for an error this connector cannot name", tt.err, got)
				}
			}
		})
	}
}
