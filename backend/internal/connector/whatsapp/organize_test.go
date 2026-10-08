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
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

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
// alone either way.
func (d *fakeDevice) sendAppState(ctx context.Context, patch appstate.PatchInfo) error {
	d.mu.Lock()
	d.appStatePatches = append(d.appStatePatches, patch)
	blocks, err := d.appStateBlocks, d.appStateErr
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

func TestSyncConversation_NeverRevertsAPinJustSetLocally(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)

	if err := c.SetPinned(t.Context(), directChat, true); err != nil {
		t.Fatal(err)
	}
	if !sink.Has("organized " + directChat.RemoteID + " true false") {
		t.Fatalf("events = %q, want the pin reported right after SetPinned", sink.Lines())
	}
	sink.Take()

	// A history resync reports this same chat with its own sync blob,
	// which never carries pinned or archived at all any more (see
	// docs/decisions.md): this must not revert the pin, because
	// reportSyncedOrganize never reads either from the blob to begin
	// with, only from whatsmeow's own chat settings.
	media := newTestMediaStore(t)
	conv := &waHistorySync.Conversation{
		ID: strPtr(directChat.RemoteID), Name: strPtr("Nadia"),
		Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)},
	}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	if sink.Has("organized " + directChat.RemoteID + " false false") {
		t.Errorf("events = %q, want the just-set pin kept rather than reverted by a stale resync", sink.Lines())
	}
}

func TestHandlePin_LiveEchoStillOverridesALocalPin(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)

	if err := c.SetPinned(t.Context(), directChat, true); err != nil {
		t.Fatal(err)
	}
	sink.Take()

	// The phone itself unpinned the chat: whatsmeow's own chat settings
	// store already reflects that by the time the live echo fires (see
	// docs/decisions.md), so seeding it here stands in for that, and a
	// live echo must always be trusted, even over a pin this process
	// set a moment ago.
	jid, err := jidFromRemoteID(directChat.RemoteID)
	if err != nil {
		t.Fatal(err)
	}
	dev.setChatSettings(jid.String(), false, false)
	c.handlePin(t.Context(), sink, dev, nil, &events.Pin{JID: jid, Action: &waSyncAction.PinAction{Pinned: boolPtr(false)}})

	if !sink.Has("organized " + directChat.RemoteID + " false false") {
		t.Errorf("events = %q, want the live unpin to take effect", sink.Lines())
	}
}

func TestSyncConversation_AppliesAPinThatArrivedBeforeTheConversationExisted(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)

	// The phone's pin arrives as a live app-state echo, already applied
	// to whatsmeow's own chat settings store, before history sync has
	// ever reported this conversation. A real app drops this first
	// report, since it has no conversation to attach it to yet, but
	// whatsmeow's store still remembers the pin regardless.
	jid, err := jidFromRemoteID(directChat.RemoteID)
	if err != nil {
		t.Fatal(err)
	}
	dev.setChatSettings(jid.String(), true, false)
	c.handlePin(t.Context(), sink, dev, nil, &events.Pin{JID: jid, Action: &waSyncAction.PinAction{Pinned: boolPtr(true)}})
	sink.Take()

	// History sync now creates the conversation. Its own synced
	// snapshot carries no pin timestamp for it, since the pin lives
	// only in app state: reading whatsmeow's chat settings instead of
	// this sync's own fields is what still finds it pinned.
	media := newTestMediaStore(t)
	conv := &waHistorySync.Conversation{
		ID: strPtr(directChat.RemoteID), Name: strPtr("Nadia"),
		Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)},
	}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	if !sink.Has("organized " + directChat.RemoteID + " true false") {
		t.Errorf("events = %q, want the early pin reported once the conversation exists", sink.Lines())
	}
}

func TestSyncConversation_NeverRevertsAPinKnownFromAppState(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)

	jid, err := jidFromRemoteID(directChat.RemoteID)
	if err != nil {
		t.Fatal(err)
	}
	media := newTestMediaStore(t)
	syncIt := func() {
		conv := &waHistorySync.Conversation{
			ID: strPtr(directChat.RemoteID), Name: strPtr("Nadia"),
			Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)},
		}
		c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))
	}

	// The conversation already exists, from an earlier sync, and only
	// afterwards does the phone's pin arrive live.
	syncIt()
	dev.setChatSettings(jid.String(), true, false)
	c.handlePin(t.Context(), sink, dev, nil, &events.Pin{JID: jid, Action: &waSyncAction.PinAction{Pinned: boolPtr(true)}})
	sink.Take()

	// A later resync still carries nothing of its own about pinned or
	// archived: the pin confirmed live a moment ago survives it, since
	// it is read fresh from whatsmeow's chat settings every time.
	syncIt()

	if sink.Has("organized " + directChat.RemoteID + " false false") {
		t.Errorf("events = %q, want the app-state pin kept rather than reverted by a later resync", sink.Lines())
	}
}

func TestSyncConversation_AppliesAnArchiveThatArrivedBeforeTheConversationExisted(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)

	jid, err := jidFromRemoteID(directChat.RemoteID)
	if err != nil {
		t.Fatal(err)
	}
	dev.setChatSettings(jid.String(), false, true)
	c.handleArchive(t.Context(), sink, dev, nil, &events.Archive{JID: jid, Action: &waSyncAction.ArchiveChatAction{Archived: boolPtr(true)}})
	sink.Take()

	// History sync's own blob carries nothing about archived either:
	// this must not undo an archive already confirmed live.
	media := newTestMediaStore(t)
	conv := &waHistorySync.Conversation{
		ID: strPtr(directChat.RemoteID), Name: strPtr("Nadia"),
		Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)},
	}
	c.handleHistorySync(t.Context(), sink, dev, media, historySyncEvent(conv))

	if !sink.Has("organized " + directChat.RemoteID + " false true") {
		t.Errorf("events = %q, want the early archive reported once the conversation exists", sink.Lines())
	}
}

func TestHandlePinAndArchive_ApplyToAPhoneKeyedConversationAddressedByLID(t *testing.T) {
	t.Parallel()

	dev, sink, c := connectedFixture(t)
	phone := types.NewJID("15551234567", types.DefaultUserServer)
	lid := types.NewJID("987654", types.HiddenUserServer)
	dev.lidPhones = map[string]types.JID{lid.String(): phone}

	// WhatsApp can address the same chat by the phone JID it is already
	// stored under or by its LID interchangeably; whatsmeow's own chat
	// settings store is keyed by whichever form the app-state mutation
	// actually named, here the LID, the same as a pin or archive sent
	// by LID really would be. Either way it must land on the
	// phone-keyed conversation, not a separate one.
	dev.setChatSettings(lid.String(), true, false)
	c.handlePin(t.Context(), sink, dev, nil, &events.Pin{JID: lid, Action: &waSyncAction.PinAction{Pinned: boolPtr(true)}})
	if !sink.Has("organized " + remoteID(phone) + " true false") {
		t.Errorf("events = %q, want the LID-addressed pin applied to the phone-keyed conversation", sink.Lines())
	}

	dev.setChatSettings(lid.String(), true, true)
	c.handleArchive(t.Context(), sink, dev, nil, &events.Archive{JID: lid, Action: &waSyncAction.ArchiveChatAction{Archived: boolPtr(true)}})
	if !sink.Has("organized " + remoteID(phone) + " true true") {
		t.Errorf("events = %q, want the LID-addressed archive applied to the phone-keyed conversation", sink.Lines())
	}
}

func TestSetArchived_TimesOutWhenWhatsAppNeverAnswers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev, _, c := connectedFixture(t)
		dev.appStateBlocks = true

		done := make(chan error, 1)
		go func() { done <- c.SetArchived(t.Context(), directChat, true) }()

		time.Sleep(organizeTimeout)
		synctest.Wait()

		select {
		case err := <-done:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("SetArchived = %v, want context.DeadlineExceeded after the organize timeout", err)
			}
		default:
			t.Fatal("SetArchived did not return once its timeout elapsed")
		}
	})
}

func TestHandleMarkChatAsRead_ReportsUnreadZeroWhenMarkedRead(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	jid := types.NewJID("15551234567", types.DefaultUserServer)

	c.handleMarkChatAsRead(t.Context(), sink, dev, nil, &events.MarkChatAsRead{
		JID: jid, Action: &waSyncAction.MarkChatAsReadAction{Read: boolPtr(true)},
	})

	if !sink.Has("unread 15551234567@s.whatsapp.net 0") {
		t.Errorf("events = %q, want unread reset to 0, the same as the self-read receipt lane", sink.Lines())
	}
}

func TestHandleMarkChatAsRead_IgnoresAMarkedUnreadChange(t *testing.T) {
	t.Parallel()

	c, dev, sink := handlerFixture(t)
	jid := types.NewJID("15551234567", types.DefaultUserServer)

	c.handleMarkChatAsRead(t.Context(), sink, dev, nil, &events.MarkChatAsRead{
		JID: jid, Action: &waSyncAction.MarkChatAsReadAction{Read: boolPtr(false)},
	})

	if len(sink.Lines()) != 0 {
		t.Errorf("events = %q, want nothing reported for a chat marked unread", sink.Lines())
	}
}
