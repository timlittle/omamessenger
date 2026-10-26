package whatsapp

// appstate_test.go drives sendAppStateRecovering and classifyAppStateErr
// directly, through a fake device, rather than only indirectly through
// organize_test.go's and markread_test.go's own pin, archive and
// mark-read tests: both are shared by all three app-state patches this
// connector sends, so their own behaviour is proven once here.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/connector"
)

// fakeServerRejection stands in for the error whatsmeow's own
// SendAppState returns when the server's response carried an <error>
// node, wrapping whatsmeow.ErrAppStateUpdate with the exact XML-ish
// text SendAppState formats into its own message (see appstate.go's
// own doc comment), so appStateRejectionDetail has something real to
// parse without this test package ever reaching WhatsApp's servers.
func fakeServerRejection(errorTag string) error {
	return fmt.Errorf("%w (regular_low): %s", whatsmeow.ErrAppStateUpdate, errorTag)
}

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
	if len(dev.fetchAppStateCalls) != 1 || dev.fetchAppStateCalls[0] != (fetchAppStateCall{name: appstate.WAPatchRegularLow, fullSync: false}) {
		t.Errorf("fetchAppState calls = %v, want one incremental catch-up for %s", dev.fetchAppStateCalls, appstate.WAPatchRegularLow)
	}
	if len(dev.appStatePatches) != 2 {
		t.Errorf("patches sent = %d, want the patch retried once after the catch-up", len(dev.appStatePatches))
	}
}

func TestSendAppStateRecovering_FallsBackToAFullSyncWhenTheIncrementalCatchUpStillFails(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.appStateErr = whatsmeow.ErrAppStateUpdate
	dev.appStateErrCount = 1
	dev.fetchAppStateErr = errors.New("mismatching LTHash")

	if err := sendAppStateRecovering(t.Context(), dev, testPatch()); err != nil {
		t.Fatal(err)
	}
	want := []fetchAppStateCall{
		{name: appstate.WAPatchRegularLow, fullSync: false},
		{name: appstate.WAPatchRegularLow, fullSync: true},
	}
	if len(dev.fetchAppStateCalls) != len(want) || dev.fetchAppStateCalls[0] != want[0] || dev.fetchAppStateCalls[1] != want[1] {
		t.Errorf("fetchAppState calls = %v, want an incremental catch-up followed by a full sync", dev.fetchAppStateCalls)
	}
	if len(dev.appStatePatches) != 2 {
		t.Errorf("patches sent = %d, want only the initial send and the retry after the full sync, none after the failed incremental fetch", len(dev.appStatePatches))
	}
}

func TestSendAppStateRecovering_ReturnsTheOriginalRejectionWhenBothCatchUpsFail(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.appStateErr = whatsmeow.ErrAppStateUpdate
	dev.fetchAppStateErr = errors.New("network down")
	dev.fetchAppStateFullSyncErr = errors.New("network down")

	err := sendAppStateRecovering(t.Context(), dev, testPatch())
	if !errors.Is(err, whatsmeow.ErrAppStateUpdate) {
		t.Errorf("sendAppStateRecovering = %v, want the original rejection when both catch-ups fail", err)
	}
	if len(dev.appStatePatches) != 1 {
		t.Errorf("patches sent = %d, want no retry once neither catch-up ever ran", len(dev.appStatePatches))
	}
}

// TestSendAppStateRecovering_LogsTheServerCodeAndTextWhenEveryAttemptFails
// is not run with t.Parallel so capturing the log output stays
// reliable (see diagnostics_test.go's captureLog).
func TestSendAppStateRecovering_LogsTheServerCodeAndTextWhenEveryAttemptFails(t *testing.T) {
	buf := captureLog(t)

	dev := newFakeDevice()
	dev.appStateErr = fakeServerRejection(`<error code="409" text="conflict"/>`)
	dev.fetchAppStateErr = errors.New("network down")
	dev.fetchAppStateFullSyncErr = errors.New("network down")

	if err := sendAppStateRecovering(t.Context(), dev, testPatch()); !errors.Is(err, whatsmeow.ErrAppStateUpdate) {
		t.Fatalf("sendAppStateRecovering = %v, want the original rejection", err)
	}

	logs := buf.String()
	if !strings.Contains(logs, "server_code=409") || !strings.Contains(logs, "server_text=conflict") {
		t.Errorf("log output = %q, want it to carry the server's own code and text", logs)
	}
	if !strings.Contains(logs, "error_class=rejected") {
		t.Errorf("log output = %q, want error_class=rejected", logs)
	}
}

// TestSendAppStateRecovering_LogsNoJIDOrOtherNodeContentOnFailure
// confirms logAppStateRejected never carries anything from the
// server's <error> node beyond its code and text attributes, even
// when the node itself carries a JID or other content: not run with
// t.Parallel, for the same reason as the test above.
func TestSendAppStateRecovering_LogsNoJIDOrOtherNodeContentOnFailure(t *testing.T) {
	buf := captureLog(t)

	dev := newFakeDevice()
	dev.appStateErr = fakeServerRejection(`<error code="403" text="forbidden" by="15559876543@s.whatsapp.net"><reason>account 15559876543 banned</reason></error>`)
	dev.fetchAppStateErr = errors.New("network down")
	dev.fetchAppStateFullSyncErr = errors.New("network down")

	_ = sendAppStateRecovering(t.Context(), dev, testPatch())

	logs := buf.String()
	if strings.Contains(logs, "15559876543") {
		t.Errorf("log output = %q, want no phone number or JID from the error node", logs)
	}
	if strings.Contains(logs, "banned") || strings.Contains(logs, "reason") {
		t.Errorf("log output = %q, want nothing from the node beyond its code and text attributes", logs)
	}
	if !strings.Contains(logs, "server_code=403") || !strings.Contains(logs, "server_text=forbidden") {
		t.Errorf("log output = %q, want the safe code and text still logged", logs)
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

// TestAppStateErrClass_SortsEveryAppStateErrorIntoItsSafeCategory
// confirms the category logAppStateRejected and logMarkRead both report
// for a failed app-state patch never reflects anything beyond these
// four safe words.
func TestAppStateErrClass_SortsEveryAppStateErrorIntoItsSafeCategory(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err  error
		want string
	}{
		"no app-state key yet":   {appstate.ErrKeyNotFound, "key"},
		"the server rejected it": {whatsmeow.ErrAppStateUpdate, "rejected"},
		"a timed-out context":    {context.DeadlineExceeded, "timeout"},
		"a cancelled context":    {context.Canceled, "timeout"},
		"a plain network error":  {errors.New("dial tcp: connection refused"), "other"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := appStateErrClass(tt.err); got != tt.want {
				t.Errorf("appStateErrClass(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

// TestFetchAppStateErrClass_RecognisesACorruptLocalCopy confirms the
// appstate package's own hash and MAC mismatch sentinels, and the
// missing-previous-operation one, all sort into "corrupt" -
// sendAppStateRecovering's own cue to fall back to a full resync -
// while a failure that merely could not reach the server at all stays
// "other".
func TestFetchAppStateErrClass_RecognisesACorruptLocalCopy(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err  error
		want string
	}{
		"mismatching LTHash":                   {appstate.ErrMismatchingLTHash, "corrupt"},
		"mismatching patch MAC":                {appstate.ErrMismatchingPatchMAC, "corrupt"},
		"mismatching content MAC":              {appstate.ErrMismatchingContentMAC, "corrupt"},
		"mismatching index MAC":                {appstate.ErrMismatchingIndexMAC, "corrupt"},
		"missing previous set value operation": {appstate.ErrMissingPreviousSetValueOperation, "corrupt"},
		"no app-state key yet":                 {appstate.ErrKeyNotFound, "key"},
		"a timed-out context":                  {context.DeadlineExceeded, "timeout"},
		"a plain network error":                {errors.New("dial tcp: connection refused"), "other"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := fetchAppStateErrClass(tt.err); got != tt.want {
				t.Errorf("fetchAppStateErrClass(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
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
