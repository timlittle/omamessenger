package whatsapp

// SetPinned and SetArchived are driven through a fake device, as send.go
// and receipts.go are, because the fake is the only way to see what
// Connector asked whatsmeow to send without reaching WhatsApp's servers.
// Every pinned or archived state these tests check comes from the
// fake's own chat settings (fakeDevice.settings), the same store this
// package's production code reads, rather than anything cached on
// Connector itself: it no longer caches either field (see
// docs/decisions.md).

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// chatSettingsEntry is one chat's pinned and archived state, mirroring
// whatsmeow's own whatsmeow_chat_settings table (see fakeDevice.settings
// in helpers_test.go).
type chatSettingsEntry struct {
	pinned, archived bool
}

// sendAppState records patch, applies its mutations to settings
// exactly as whatsmeow's own SendAppState updates its chat settings
// store before returning (see device.go and docs/decisions.md), and
// reports appStateErr, or blocks on ctx when appStateBlocks is set, as
// a real patch that never hears back from the server does: neither a
// blocked nor a failed send ever reaches WhatsApp, so settings is left
// alone either way. appStateErrCount, when positive, limits appStateErr
// to that many calls before this fake starts succeeding, standing in
// for a rejection sendAppStateRecovering's own retry, after a catch-up,
// resolves (see appstate_test.go); a zero or negative count fails every
// call, as before.
func (d *fakeDevice) sendAppState(ctx context.Context, patch appstate.PatchInfo) error {
	d.mu.Lock()
	d.appStatePatches = append(d.appStatePatches, patch)
	blocks, err := d.appStateBlocks, d.appStateErr
	if err != nil && d.appStateErrCount > 0 {
		d.appStateErrCount--
		if d.appStateErrCount == 0 {
			// The countdown is done: clear the scripted error so every
			// later call succeeds, rather than only this one.
			d.appStateErr = nil
		}
	}
	if !blocks && err == nil {
		d.applyAppState(patch)
	}
	d.mu.Unlock()

	if blocks {
		<-ctx.Done()
		return ctx.Err()
	}

	return err
}

// fetchAppState records the call in fetchAppStateCalls and reports
// fetchAppStateErr for an incremental catch-up or fetchAppStateFullSyncErr
// for a full sync, standing in for whatsmeow's own catch-up fetch
// without ever reaching WhatsApp's servers.
func (d *fakeDevice) fetchAppState(_ context.Context, name appstate.WAPatchName, fullSync bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.fetchAppStateCalls = append(d.fetchAppStateCalls, fetchAppStateCall{name: name, fullSync: fullSync})
	if fullSync {
		return d.fetchAppStateFullSyncErr
	}

	return d.fetchAppStateErr
}

// applyAppState updates a fake device's settings from patch's pin and
// archive mutations, the only two this connector ever sends, mirroring
// whatsmeow's own dispatchAppState; sendAppState (helpers_test.go)
// calls this with its own mutex already held.
func (d *fakeDevice) applyAppState(patch appstate.PatchInfo) {
	if d.settings == nil {
		d.settings = map[string]chatSettingsEntry{}
	}

	for _, m := range patch.Mutations {
		if len(m.Index) < 2 {
			continue
		}

		jid := m.Index[1]
		entry := d.settings[jid]
		switch m.Index[0] {
		case appstate.IndexPin:
			entry.pinned = m.Value.GetPinAction().GetPinned()
		case appstate.IndexArchive:
			entry.archived = m.Value.GetArchiveChatAction().GetArchived()
		}
		d.settings[jid] = entry
	}
}

// chatSettings reports jid's pinned and archived state exactly as
// sendAppState or a test's own seeding last set it for jid's string
// form, mirroring whatsmeow's own locally cached store.
func (d *fakeDevice) chatSettings(_ context.Context, jid types.JID) (bool, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	entry := d.settings[jid.String()]

	return entry.pinned, entry.archived, nil
}

// setChatSettings seeds settings for the JID named by remoteOrJID
// directly, standing in for a pinned or archived change whatsmeow's
// own app-state processing already applied to its store before a
// test's own handler call or assertion runs, without going through
// sendAppState's own patch recording. remoteOrJID is a JID's string
// form, which is exactly what this package's canonical remote ids
// already are.
func (d *fakeDevice) setChatSettings(remoteOrJID string, pinned, archived bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.settings == nil {
		d.settings = map[string]chatSettingsEntry{}
	}
	d.settings[remoteOrJID] = chatSettingsEntry{pinned: pinned, archived: archived}
}

// seedPinnedChat registers remote as a chat this connector already
// knows about (see Connector.chatKinds) and marks it pinned in dev's
// chat settings, standing in for a chat WhatsApp already had pinned
// before this test's own SetPinned call, so pinnedCount can count it.
func seedPinnedChat(c *Connector, dev *fakeDevice, remote string) {
	c.noteChat(remote, domain.KindDirect)
	dev.setChatSettings(remote, true, false)
}

func TestSetPinned_SendsThePinPatch(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)

	if err := c.SetPinned(t.Context(), directChat, true); err != nil {
		t.Fatal(err)
	}

	if len(dev.appStatePatches) != 1 {
		t.Fatalf("patches sent = %d, want 1", len(dev.appStatePatches))
	}
	action := dev.appStatePatches[0].Mutations[0].Value.GetPinAction()
	if action == nil || !action.GetPinned() {
		t.Errorf("mutation = %+v, want a pin action with Pinned true", dev.appStatePatches[0].Mutations[0])
	}
	if index := dev.appStatePatches[0].Mutations[0].Index; len(index) != 2 || index[0] != appstate.IndexPin || index[1] != directChat.RemoteID {
		t.Errorf("index = %v, want [%s %s]", index, appstate.IndexPin, directChat.RemoteID)
	}

	if !sink.Has("organized " + directChat.RemoteID + " true false") {
		t.Errorf("events = %q, want the pin reported", sink.Lines())
	}
}

func TestSetPinned_SendsTheUnpinPatch(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)
	dev.setChatSettings(directChat.RemoteID, true, false)

	if err := c.SetPinned(t.Context(), directChat, false); err != nil {
		t.Fatal(err)
	}

	action := dev.appStatePatches[0].Mutations[0].Value.GetPinAction()
	if action == nil || action.GetPinned() {
		t.Errorf("mutation = %+v, want a pin action with Pinned false", dev.appStatePatches[0].Mutations[0])
	}
	if !sink.Has("organized " + directChat.RemoteID + " false false") {
		t.Errorf("events = %q, want the unpin reported", sink.Lines())
	}
}

// TestSetPinned_AddressesTheMigratedChatByItsLID confirms a chat
// first known by its phone JID, once whatsmeow's own LID store has
// since learned that chat's LID, gets its pin patch addressed by that
// LID instead, the same fix sendChatReadState already has for the
// chat-read mutation (see markread_test.go's own version of this
// test): WhatsApp's server rejects a pin or archive patch every time
// while it is still addressed by a chat's now-superseded phone JID.
func TestSetPinned_AddressesTheMigratedChatByItsLID(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)
	lid := types.NewJID("98765432109", types.HiddenUserServer)
	dev.lidPhones = map[string]types.JID{lid.String(): directPeer}
	dev.setChatSettings(lid.String(), false, false)

	if err := c.SetPinned(t.Context(), directChat, true); err != nil {
		t.Fatal(err)
	}

	if len(dev.appStatePatches) != 1 {
		t.Fatalf("patches sent = %d, want 1", len(dev.appStatePatches))
	}
	if index := dev.appStatePatches[0].Mutations[0].Index; len(index) != 2 || index[1] != lid.String() {
		t.Errorf("index = %v, want the chat addressed by its LID %s", index, lid.String())
	}
	if !sink.Has("organized " + directChat.RemoteID + " true false") {
		t.Errorf("events = %q, want the pin reported under the conversation's own remote id even though the patch itself was sent by LID", sink.Lines())
	}
}

func TestSetPinned_RefusesAFourthPin(t *testing.T) {
	t.Parallel()

	dev, _, c := connectedFixture(t)
	for _, number := range []string{"15550000001", "15550000002", "15550000003"} {
		seedPinnedChat(c, dev, remoteID(types.NewJID(number, types.DefaultUserServer)))
	}

	err := c.SetPinned(t.Context(), directChat, true)
	if !errors.Is(err, connector.ErrPinLimit) {
		t.Errorf("SetPinned past the limit = %v, want ErrPinLimit", err)
	}
	if len(dev.appStatePatches) != 0 {
		t.Errorf("patches sent = %d, want none once the limit refuses it", len(dev.appStatePatches))
	}
}

// TestSetPinned_CountsALIDAddressedPinnedChatTowardTheLimit confirms
// pinnedCount still recognises a conversation WhatsApp's chat settings
// have pinned once its chat settings are keyed by its LID rather than
// its phone JID, the form they end up under once SetPinned itself
// starts addressing a migrated chat that way (see
// TestSetPinned_AddressesTheMigratedChatByItsLID).
func TestSetPinned_CountsALIDAddressedPinnedChatTowardTheLimit(t *testing.T) {
	t.Parallel()

	dev, _, c := connectedFixture(t)
	for _, number := range []string{"15550000001", "15550000002"} {
		seedPinnedChat(c, dev, remoteID(types.NewJID(number, types.DefaultUserServer)))
	}

	thirdRemote := remoteID(types.NewJID("15550000003", types.DefaultUserServer))
	thirdLID := types.NewJID("98765432108", types.HiddenUserServer)
	dev.lidPhones = map[string]types.JID{thirdLID.String(): types.NewJID("15550000003", types.DefaultUserServer)}
	c.noteChat(thirdRemote, domain.KindDirect)
	dev.setChatSettings(thirdLID.String(), true, false)

	err := c.SetPinned(t.Context(), directChat, true)
	if !errors.Is(err, connector.ErrPinLimit) {
		t.Errorf("SetPinned past the limit = %v, want ErrPinLimit once the LID-addressed pin is counted", err)
	}
}

func TestSetPinned_AllowsRepinningAnAlreadyPinnedChat(t *testing.T) {
	t.Parallel()

	dev, _, c := connectedFixture(t)
	for _, number := range []string{"15550000001", "15550000002", "15550000003"} {
		seedPinnedChat(c, dev, remoteID(types.NewJID(number, types.DefaultUserServer)))
	}
	fourthRemote := remoteID(types.NewJID("15550000003", types.DefaultUserServer))

	// Pinning a chat that is already counted among the three pinned
	// ones must not be refused for being the fourth.
	if err := c.SetPinned(t.Context(), domain.Conversation{RemoteID: fourthRemote}, true); err != nil {
		t.Errorf("SetPinned(already pinned) = %v, want no error", err)
	}
}

func TestSetPinned_WrapsAWhatsAppError(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)
	dev.appStateErr = errors.New("server unavailable")

	if err := c.SetPinned(t.Context(), directChat, true); !errors.Is(err, dev.appStateErr) {
		t.Errorf("SetPinned = %v, want it to wrap the device's error", err)
	}
	if sink.Has("organized " + directChat.RemoteID + " true false") {
		t.Errorf("events = %q, want nothing reported for a failed pin", sink.Lines())
	}
}

func TestSetPinned_Fails(t *testing.T) {
	t.Parallel()

	_, _, connected := connectedFixture(t)
	for name, err := range map[string]error{
		"before connecting":     New(domain.Account{ID: "wa"}, t.TempDir()).SetPinned(t.Context(), directChat, true),
		"a malformed remote id": connected.SetPinned(t.Context(), domain.Conversation{RemoteID: "bad"}, true),
	} {
		if err == nil {
			t.Errorf("SetPinned %s succeeded, want an error", name)
		}
	}
}

func TestSetPinned_TimesOutWhenWhatsAppNeverAnswers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev, _, c := connectedFixture(t)
		dev.appStateBlocks = true

		done := make(chan error, 1)
		go func() { done <- c.SetPinned(t.Context(), directChat, true) }()

		time.Sleep(organizeTimeout)
		synctest.Wait()

		select {
		case err := <-done:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("SetPinned = %v, want context.DeadlineExceeded after the organize timeout", err)
			}
		default:
			t.Fatal("SetPinned did not return once its timeout elapsed")
		}
	})
}

func TestSetArchived_SendsTheArchivePatchAndUnpinsIt(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)
	dev.setChatSettings(directChat.RemoteID, true, false)

	if err := c.SetArchived(t.Context(), directChat, true); err != nil {
		t.Fatal(err)
	}

	mutations := dev.appStatePatches[0].Mutations
	archiveAction := mutations[0].Value.GetArchiveChatAction()
	if archiveAction == nil || !archiveAction.GetArchived() {
		t.Errorf("mutation = %+v, want an archive action with Archived true", mutations[0])
	}
	if len(mutations) != 2 || mutations[1].Value.GetPinAction().GetPinned() {
		t.Errorf("mutations = %+v, want a second mutation that unpins", mutations)
	}

	// Archiving unpins WhatsApp's own record of the chat, so the chat
	// settings this connector reads back and reports must agree once
	// the patch succeeds.
	if !sink.Has("organized " + directChat.RemoteID + " false true") {
		t.Errorf("events = %q, want pinned false and archived true", sink.Lines())
	}
}

func TestSetArchived_SendsTheUnarchivePatchWithoutRepinning(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)
	dev.setChatSettings(directChat.RemoteID, false, true)

	if err := c.SetArchived(t.Context(), directChat, false); err != nil {
		t.Fatal(err)
	}

	if len(dev.appStatePatches[0].Mutations) != 1 {
		t.Errorf("mutations = %+v, want only the archive mutation", dev.appStatePatches[0].Mutations)
	}
	if !sink.Has("organized " + directChat.RemoteID + " false false") {
		t.Errorf("events = %q, want pinned left false and archived now false", sink.Lines())
	}
}

// TestSetArchived_AddressesTheMigratedChatByItsLID is SetArchived's
// own version of TestSetPinned_AddressesTheMigratedChatByItsLID.
func TestSetArchived_AddressesTheMigratedChatByItsLID(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)
	lid := types.NewJID("98765432109", types.HiddenUserServer)
	dev.lidPhones = map[string]types.JID{lid.String(): directPeer}
	dev.setChatSettings(lid.String(), true, false)

	if err := c.SetArchived(t.Context(), directChat, true); err != nil {
		t.Fatal(err)
	}

	if index := dev.appStatePatches[0].Mutations[0].Index; len(index) != 2 || index[1] != lid.String() {
		t.Errorf("index = %v, want the chat addressed by its LID %s", index, lid.String())
	}
	if !sink.Has("organized " + directChat.RemoteID + " false true") {
		t.Errorf("events = %q, want the archive reported under the conversation's own remote id even though the patch itself was sent by LID", sink.Lines())
	}
}

func TestSetArchived_WrapsAWhatsAppError(t *testing.T) {
	t.Parallel()

	dev, _, c := connectedFixture(t)
	dev.appStateErr = errors.New("server unavailable")

	if err := c.SetArchived(t.Context(), directChat, true); !errors.Is(err, dev.appStateErr) {
		t.Errorf("SetArchived = %v, want it to wrap the device's error", err)
	}
}

func TestSetArchived_Fails(t *testing.T) {
	t.Parallel()

	_, _, connected := connectedFixture(t)
	for name, err := range map[string]error{
		"before connecting":     New(domain.Account{ID: "wa"}, t.TempDir()).SetArchived(t.Context(), directChat, true),
		"a malformed remote id": connected.SetArchived(t.Context(), domain.Conversation{RemoteID: "bad"}, true),
	} {
		if err == nil {
			t.Errorf("SetArchived %s succeeded, want an error", name)
		}
	}
}
