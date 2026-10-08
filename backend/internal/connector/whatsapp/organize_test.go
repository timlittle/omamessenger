package whatsapp

// SetPinned and SetArchived are driven through a fake device, as send.go
// and receipts.go are, because the fake is the only way to see what
// Connector asked whatsmeow to send without reaching WhatsApp's servers.

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
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestSetPinned_SendsThePinPatch(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

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

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)
	c.setOrganized(directChat.RemoteID, boolPtr(true), nil)

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

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)
	for _, number := range []string{"15550000001", "15550000002", "15550000003"} {
		remote := remoteID(types.NewJID(number, types.DefaultUserServer))
		c.setOrganized(remote, boolPtr(true), nil)
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

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)
	for _, number := range []string{"15550000001", "15550000002", "15550000003"} {
		remote := remoteID(types.NewJID(number, types.DefaultUserServer))
		c.setOrganized(remote, boolPtr(true), nil)
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

	dev := newFakeDevice()
	dev.appStateErr = errors.New("server unavailable")
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	if err := c.SetPinned(t.Context(), directChat, true); !errors.Is(err, dev.appStateErr) {
		t.Errorf("SetPinned = %v, want it to wrap the device's error", err)
	}
	if sink.Has("organized " + directChat.RemoteID + " true false") {
		t.Errorf("events = %q, want nothing reported for a failed pin", sink.Lines())
	}
}

func TestSetPinned_Fails(t *testing.T) {
	t.Parallel()

	connected := connectedTo(newFakeDevice(), &connectortest.Sink{})
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
		dev := newFakeDevice()
		dev.appStateBlocks = true
		var sink connectortest.Sink
		c := connectedTo(dev, &sink)

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

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)
	c.setOrganized(directChat.RemoteID, boolPtr(true), nil)

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

	// Archiving unpins WhatsApp's own record of the chat, so the cached
	// state this connector reports must agree once the patch succeeds.
	if !sink.Has("organized " + directChat.RemoteID + " false true") {
		t.Errorf("events = %q, want pinned false and archived true", sink.Lines())
	}
}

func TestSetArchived_SendsTheUnarchivePatchWithoutRepinning(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)
	c.setOrganized(directChat.RemoteID, nil, boolPtr(true))

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

	dev := newFakeDevice()
	dev.appStateErr = errors.New("server unavailable")
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	if err := c.SetArchived(t.Context(), directChat, true); !errors.Is(err, dev.appStateErr) {
		t.Errorf("SetArchived = %v, want it to wrap the device's error", err)
	}
}

func TestSetArchived_Fails(t *testing.T) {
	t.Parallel()

	connected := connectedTo(newFakeDevice(), &connectortest.Sink{})
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

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	if err := c.SetPinned(t.Context(), directChat, true); err != nil {
		t.Fatal(err)
	}
	if !sink.Has("organized " + directChat.RemoteID + " true false") {
		t.Fatalf("events = %q, want the pin reported right after SetPinned", sink.Lines())
	}
	sink.Take()

	// A history resync reports this same chat with WhatsApp's own
	// snapshot, which has not caught up with the pin just sent: this
	// must not revert it.
	media := newTestMediaStore(t)
	e := &events.HistorySync{Data: &waHistorySync.HistorySync{
		Conversations: []*waHistorySync.Conversation{{
			ID: strPtr(directChat.RemoteID), Name: strPtr("Nadia"),
			Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)},
		}},
	}}
	c.handleHistorySync(t.Context(), &sink, dev, media, e)

	if sink.Has("organized " + directChat.RemoteID + " false false") {
		t.Errorf("events = %q, want the just-set pin kept rather than reverted by a stale resync", sink.Lines())
	}
}

func TestHandlePin_LiveEchoStillOverridesALocalPin(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	if err := c.SetPinned(t.Context(), directChat, true); err != nil {
		t.Fatal(err)
	}
	sink.Take()

	// The phone itself unpinned the chat: a live echo is a real,
	// current update and must always be trusted, even over a pin this
	// process set a moment ago.
	jid, err := jidFromRemoteID(directChat.RemoteID)
	if err != nil {
		t.Fatal(err)
	}
	c.handlePin(t.Context(), &sink, dev, &events.Pin{JID: jid, Action: &waSyncAction.PinAction{Pinned: boolPtr(false)}})

	if !sink.Has("organized " + directChat.RemoteID + " false false") {
		t.Errorf("events = %q, want the live unpin to take effect", sink.Lines())
	}
}

func TestSyncConversation_AppliesAPinThatArrivedBeforeTheConversationExisted(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	// The phone's pin arrives as a live app-state echo before history
	// sync has ever reported this conversation. A real app drops this
	// first report, since it has no conversation to attach it to yet,
	// but this connector's own cache still remembers it.
	jid, err := jidFromRemoteID(directChat.RemoteID)
	if err != nil {
		t.Fatal(err)
	}
	c.handlePin(t.Context(), &sink, dev, &events.Pin{JID: jid, Action: &waSyncAction.PinAction{Pinned: boolPtr(true)}})
	sink.Take()

	// History sync now creates the conversation. WhatsApp's own synced
	// snapshot carries no pin timestamp for it, since the pin lives only
	// in app state, not in this blob: that must not leave it unpinned.
	media := newTestMediaStore(t)
	e := &events.HistorySync{Data: &waHistorySync.HistorySync{
		Conversations: []*waHistorySync.Conversation{{
			ID: strPtr(directChat.RemoteID), Name: strPtr("Nadia"),
			Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)},
		}},
	}}
	c.handleHistorySync(t.Context(), &sink, dev, media, e)

	if !sink.Has("organized " + directChat.RemoteID + " true false") {
		t.Errorf("events = %q, want the early pin reported once the conversation exists", sink.Lines())
	}
}

func TestSyncConversation_NeverRevertsAPinKnownFromAppState(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	jid, err := jidFromRemoteID(directChat.RemoteID)
	if err != nil {
		t.Fatal(err)
	}
	media := newTestMediaStore(t)
	syncIt := func() {
		e := &events.HistorySync{Data: &waHistorySync.HistorySync{
			Conversations: []*waHistorySync.Conversation{{
				ID: strPtr(directChat.RemoteID), Name: strPtr("Nadia"),
				Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)},
			}},
		}}
		c.handleHistorySync(t.Context(), &sink, dev, media, e)
	}

	// The conversation already exists, from an earlier sync with no pin
	// of its own, and only afterwards does the phone's pin arrive live.
	syncIt()
	c.handlePin(t.Context(), &sink, dev, &events.Pin{JID: jid, Action: &waSyncAction.PinAction{Pinned: boolPtr(true)}})
	sink.Take()

	// A later resync still carries WhatsApp's own snapshot with no pin
	// timestamp: the pin confirmed live a moment ago must survive it.
	syncIt()

	if sink.Has("organized " + directChat.RemoteID + " false false") {
		t.Errorf("events = %q, want the app-state pin kept rather than reverted by a later resync", sink.Lines())
	}
}

func TestSyncConversation_AppliesAnArchiveThatArrivedBeforeTheConversationExisted(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)

	jid, err := jidFromRemoteID(directChat.RemoteID)
	if err != nil {
		t.Fatal(err)
	}
	c.handleArchive(t.Context(), &sink, dev, &events.Archive{JID: jid, Action: &waSyncAction.ArchiveChatAction{Archived: boolPtr(true)}})
	sink.Take()

	// History sync's own Archived field defaults to false unless the
	// sync carries it, same as Pinned can: this must not undo an
	// archive already confirmed live.
	media := newTestMediaStore(t)
	e := &events.HistorySync{Data: &waHistorySync.HistorySync{
		Conversations: []*waHistorySync.Conversation{{
			ID: strPtr(directChat.RemoteID), Name: strPtr("Nadia"),
			Messages: []*waHistorySync.HistorySyncMsg{historyMsg("H1", "hi", false)},
		}},
	}}
	c.handleHistorySync(t.Context(), &sink, dev, media, e)

	if !sink.Has("organized " + directChat.RemoteID + " false true") {
		t.Errorf("events = %q, want the early archive reported once the conversation exists", sink.Lines())
	}
}

func TestHandlePinAndArchive_ApplyToAPhoneKeyedConversationAddressedByLID(t *testing.T) {
	t.Parallel()

	dev := newFakeDevice()
	var sink connectortest.Sink
	c := connectedTo(dev, &sink)
	phone := types.NewJID("15551234567", types.DefaultUserServer)
	lid := types.NewJID("987654", types.HiddenUserServer)
	dev.lidPhones = map[string]types.JID{lid.String(): phone}

	// WhatsApp can address the same chat by the phone JID it is already
	// stored under or by its LID interchangeably; a pin or archive sent
	// by LID must land on the phone-keyed conversation, not a separate
	// one.
	c.handlePin(t.Context(), &sink, dev, &events.Pin{JID: lid, Action: &waSyncAction.PinAction{Pinned: boolPtr(true)}})
	if !sink.Has("organized " + remoteID(phone) + " true false") {
		t.Errorf("events = %q, want the LID-addressed pin applied to the phone-keyed conversation", sink.Lines())
	}

	c.handleArchive(t.Context(), &sink, dev, &events.Archive{JID: lid, Action: &waSyncAction.ArchiveChatAction{Archived: boolPtr(true)}})
	if !sink.Has("organized " + remoteID(phone) + " true true") {
		t.Errorf("events = %q, want the LID-addressed archive applied to the phone-keyed conversation", sink.Lines())
	}
}

func TestSetArchived_TimesOutWhenWhatsAppNeverAnswers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev := newFakeDevice()
		dev.appStateBlocks = true
		var sink connectortest.Sink
		c := connectedTo(dev, &sink)

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
