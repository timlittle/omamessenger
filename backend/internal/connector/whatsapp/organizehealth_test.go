package whatsapp

// organizehealth_test.go drives sendOrganizePatch, the shared choke
// point SetPinned, SetArchived, sendChatReadState and deleteForMe all
// go through (see organize_test.go, markread_test.go and
// delete_test.go for each one's own wiring test), and the health
// tracking it relies on: skipping WhatsApp's server once a patch type
// is flagged local-only, persisting that across a simulated restart,
// and clearing it again once a background recheck verifies it.

import (
	"errors"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestSendOrganizePatch_SkipsWhatsAppOnceFlaggedLocalOnly(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.appStateErr = whatsmeow.ErrAppStateUpdate
	dev.fetchAppStateErr = appstate.ErrMismatchingLTHash
	dev.fetchAppStateFullSyncErr = appstate.ErrMismatchingLTHash
	_, _, c := connectedFixture(t)

	err := c.sendOrganizePatch(t.Context(), dev, testPatch())
	if !errors.Is(err, connector.ErrOrganizeLocalOnly) {
		t.Fatalf("sendOrganizePatch = %v, want connector.ErrOrganizeLocalOnly once both catch-ups find corruption", err)
	}
	if len(dev.appStatePatches) != 1 || len(dev.fetchAppStateCalls) != 2 {
		t.Fatalf("appStatePatches = %d, fetchAppStateCalls = %d, want 1 and 2 on first discovery", len(dev.appStatePatches), len(dev.fetchAppStateCalls))
	}

	// A second call for the same patch type must not repeat any of
	// that expensive recovery: it reports the same sentinel at once,
	// with no new call to the device at all.
	err = c.sendOrganizePatch(t.Context(), dev, testPatch())
	if !errors.Is(err, connector.ErrOrganizeLocalOnly) {
		t.Errorf("second sendOrganizePatch = %v, want connector.ErrOrganizeLocalOnly", err)
	}
	if len(dev.appStatePatches) != 1 || len(dev.fetchAppStateCalls) != 2 {
		t.Errorf("appStatePatches = %d, fetchAppStateCalls = %d, want no new calls once flagged", len(dev.appStatePatches), len(dev.fetchAppStateCalls))
	}
}

func TestSendOrganizePatch_DoesNotFlagAPlainRejection(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.appStateErr = whatsmeow.ErrAppStateUpdate
	dev.fetchAppStateErr = errors.New("network down")
	dev.fetchAppStateFullSyncErr = errors.New("network down")
	_, _, c := connectedFixture(t)

	err := c.sendOrganizePatch(t.Context(), dev, testPatch())
	if !errors.Is(err, connector.ErrOrganizeRejected) {
		t.Errorf("sendOrganizePatch = %v, want connector.ErrOrganizeRejected for a plain rejection", err)
	}
	if c.organizeLocalOnly(appstate.WAPatchRegularLow) {
		t.Errorf("organizeLocalOnly = true, want it left false for a rejection that was never found unverifiable")
	}
}

func TestOrganizeHealth_PersistsAcrossARestart(t *testing.T) {
	t.Parallel()

	media := newTestMediaStore(t)
	dev := newFakeDevice()
	before := newConnectorWithMedia(dev, &connectortest.Sink{}, media)
	before.setOrganizeLocalOnly(t.Context(), appstate.WAPatchRegularLow, true)

	unsyncable, err := media.organizeUnsyncable(t.Context(), appstate.WAPatchRegularLow)
	if err != nil || !unsyncable {
		t.Fatalf("organizeUnsyncable after setting = %v, %v, want true", unsyncable, err)
	}

	// A fresh connector, standing in for a helper restart, resumes the
	// same state once it loads what the previous run persisted, rather
	// than starting blind.
	after := New(domain.Account{ID: "wa"}, t.TempDir())
	resumed, err := media.organizeUnsyncable(t.Context(), appstate.WAPatchRegularLow)
	if err != nil {
		t.Fatal(err)
	}
	after.loadOrganizeHealth(appstate.WAPatchRegularLow, resumed)
	if !after.organizeLocalOnly(appstate.WAPatchRegularLow) {
		t.Errorf("organizeLocalOnly after loading = false, want true, resumed from a previous run")
	}
}

func TestRecheckOrganizeHealth_DoesNothingWhenNothingIsFlagged(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	_, _, c := connectedFixture(t)

	c.recheckOrganizeHealth(t.Context(), dev)
	if len(dev.fetchAppStateCalls) != 0 {
		t.Errorf("fetchAppState calls = %v, want none while nothing is flagged", dev.fetchAppStateCalls)
	}
}

func TestRecheckOrganizeHealth_ClearsTheFlagOnceAnIncrementalFetchSucceeds(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	_, _, c := connectedFixture(t)
	c.loadOrganizeHealth(appstate.WAPatchRegularLow, true)

	c.recheckOrganizeHealth(t.Context(), dev)
	if c.organizeLocalOnly(appstate.WAPatchRegularLow) {
		t.Errorf("organizeLocalOnly after a successful recheck = true, want false")
	}
	if len(dev.fetchAppStateCalls) != 1 || dev.fetchAppStateCalls[0] != (fetchAppStateCall{name: appstate.WAPatchRegularLow, fullSync: false}) {
		t.Errorf("fetchAppState calls = %v, want one incremental fetch", dev.fetchAppStateCalls)
	}
}

func TestRecheckOrganizeHealth_LeavesTheFlagSetWhenTheFetchStillFails(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	dev.fetchAppStateErr = appstate.ErrMismatchingLTHash
	_, _, c := connectedFixture(t)
	c.loadOrganizeHealth(appstate.WAPatchRegularLow, true)

	c.recheckOrganizeHealth(t.Context(), dev)
	if !c.organizeLocalOnly(appstate.WAPatchRegularLow) {
		t.Errorf("organizeLocalOnly after a failed recheck = false, want it to stay true")
	}
}
