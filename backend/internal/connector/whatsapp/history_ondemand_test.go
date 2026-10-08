package whatsapp

// history_ondemand.go's LoadOlder flow is driven through LoadOlder
// itself and a scripted events.HistorySync answer, exactly as
// retry_test.go drives FetchMedia and a scripted events.MediaRetry:
// dev.fireEvent delivers the on-demand sync the same way whatsmeow's own
// dispatcher would, through handleEvents registered the way Run
// registers it.

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// onDemandSync builds the events.HistorySync a real phone sends in
// answer to an on-demand request: one conversation, chatID, carrying
// msgs.
func onDemandSync(chatID string, msgs ...*waHistorySync.HistorySyncMsg) *events.HistorySync {
	return &events.HistorySync{Data: &waHistorySync.HistorySync{
		SyncType:      waHistorySync.HistorySync_ON_DEMAND.Enum(),
		Conversations: []*waHistorySync.Conversation{{ID: strPtr(chatID), Messages: msgs}},
	}}
}

// TestLoadOlder_SendsTheRequestWithTheRightAnchorAndCount confirms
// LoadOlder builds its on-demand request from message_keys' row for the
// anchor message, and asks for exactly the given limit.
func TestLoadOlder_SendsTheRequestWithTheRightAnchorAndCount(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev, sink, c, media := connectedMediaFixture(t)
		unregister := c.handleEvents(t.Context(), dev, media, sink)
		defer unregister()

		if err := media.putMessageKey(t.Context(), directChat.RemoteID, "m1", messageKey{fromMe: false, timestamp: 1000}); err != nil {
			t.Fatal(err)
		}

		done := make(chan int, 1)
		go func() {
			n, err := c.LoadOlder(t.Context(), directChat, "m1", 25)
			if err != nil {
				t.Error(err)
			}
			done <- n
		}()
		synctest.Wait()

		if len(dev.requestHistoryCalls) != 1 {
			t.Fatalf("requestHistoryCalls = %d, want exactly one request", len(dev.requestHistoryCalls))
		}
		call := dev.requestHistoryCalls[0]
		if call.count != 25 {
			t.Errorf("count = %d, want 25", call.count)
		}
		if call.anchor.Chat != directPeer || call.anchor.ID != "m1" || call.anchor.IsFromMe || !call.anchor.Timestamp.Equal(time.UnixMilli(1000)) {
			t.Errorf("anchor = %+v, want chat %v, id m1, not from me, timestamp 1000ms", call.anchor, directPeer)
		}

		dev.fireEvent(onDemandSync(directPeer.String(), historyMsg("m0", "earlier", false)))
		synctest.Wait()

		if n := <-done; n != 1 {
			t.Errorf("LoadOlder = %d, want 1", n)
		}
	})
}

// TestLoadOlder_AMatchingOnDemandHistorySyncResolvesItAndStoresHistory
// confirms the phone's answer is processed the normal way history sync
// always is: the messages are reported through the Sink's History, and
// LoadOlder's count matches how many actually were.
func TestLoadOlder_AMatchingOnDemandHistorySyncResolvesItAndStoresHistory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev, sink, c, media := connectedMediaFixture(t)
		unregister := c.handleEvents(t.Context(), dev, media, sink)
		defer unregister()

		if err := media.putMessageKey(t.Context(), directChat.RemoteID, "m2", messageKey{timestamp: 2000}); err != nil {
			t.Fatal(err)
		}

		done := make(chan int, 1)
		go func() {
			n, err := c.LoadOlder(t.Context(), directChat, "m2", 50)
			if err != nil {
				t.Error(err)
			}
			done <- n
		}()
		synctest.Wait()

		dev.fireEvent(onDemandSync(directPeer.String(),
			historyMsg("m0", "earlier", false), historyMsg("m1", "earliest", false)))
		synctest.Wait()

		if n := <-done; n != 2 {
			t.Errorf("LoadOlder = %d, want 2", n)
		}
		if !sink.Has("history "+directChat.RemoteID+" m0") || !sink.Has("history "+directChat.RemoteID+" m1") {
			t.Errorf("events = %q, want both older messages stored as history", sink.Lines())
		}
	})
}

// TestLoadOlder_IgnoresAnOnDemandSyncForADifferentChat confirms an
// on-demand answer naming a different chat never resolves a LoadOlder
// call waiting on another one.
func TestLoadOlder_IgnoresAnOnDemandSyncForADifferentChat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev, sink, c, media := connectedMediaFixture(t)
		unregister := c.handleEvents(t.Context(), dev, media, sink)
		defer unregister()

		if err := media.putMessageKey(t.Context(), directChat.RemoteID, "m1", messageKey{timestamp: 1000}); err != nil {
			t.Fatal(err)
		}

		done := make(chan int, 1)
		go func() {
			n, _ := c.LoadOlder(t.Context(), directChat, "m1", 50)
			done <- n
		}()
		synctest.Wait()

		dev.fireEvent(onDemandSync("15559990000@s.whatsapp.net", historyMsg("x1", "not for this chat", false)))
		synctest.Wait()

		select {
		case n := <-done:
			t.Fatalf("LoadOlder resolved = %d, want it still waiting for its own chat's answer", n)
		default:
		}

		dev.fireEvent(onDemandSync(directPeer.String(), historyMsg("m0", "earlier", false)))
		synctest.Wait()

		if n := <-done; n != 1 {
			t.Errorf("LoadOlder = %d, want 1 once its own chat's answer arrives", n)
		}
	})
}

// TestLoadOlder_RejectsASecondRequestForTheSameChatWhileOneIsInFlight
// confirms a second LoadOlder for a chat that already has a request
// waiting on the phone is rejected cleanly, rather than piling a second
// request behind it, and that the first call still resolves normally.
func TestLoadOlder_RejectsASecondRequestForTheSameChatWhileOneIsInFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dev, sink, c, media := connectedMediaFixture(t)
		unregister := c.handleEvents(t.Context(), dev, media, sink)
		defer unregister()

		if err := media.putMessageKey(t.Context(), directChat.RemoteID, "m1", messageKey{timestamp: 1000}); err != nil {
			t.Fatal(err)
		}

		done := make(chan int, 1)
		go func() {
			n, _ := c.LoadOlder(t.Context(), directChat, "m1", 50)
			done <- n
		}()
		synctest.Wait()

		if _, err := c.LoadOlder(t.Context(), directChat, "m1", 50); !errors.Is(err, errHistoryRequestInProgress) {
			t.Errorf("second LoadOlder = %v, want errHistoryRequestInProgress", err)
		}

		dev.fireEvent(onDemandSync(directPeer.String(), historyMsg("m0", "earlier", false)))
		synctest.Wait()

		if n := <-done; n != 1 {
			t.Errorf("first LoadOlder = %d, want 1", n)
		}
	})
}

// TestLoadOlder_TimesOutAsHistoryUnavailableWhenThePhoneNeverAnswers
// confirms a request nobody ever answers still returns, as
// connector.ErrHistoryUnavailable, once onDemandHistoryTimeout elapses,
// rather than waiting forever.
func TestLoadOlder_TimesOutAsHistoryUnavailableWhenThePhoneNeverAnswers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, _, c, media := connectedMediaFixture(t)
		if err := media.putMessageKey(t.Context(), directChat.RemoteID, "m1", messageKey{timestamp: 1000}); err != nil {
			t.Fatal(err)
		}

		done := make(chan error, 1)
		go func() {
			_, err := c.LoadOlder(t.Context(), directChat, "m1", 50)
			done <- err
		}()

		time.Sleep(onDemandHistoryTimeout)
		synctest.Wait()

		select {
		case err := <-done:
			if !errors.Is(err, connector.ErrHistoryUnavailable) {
				t.Errorf("LoadOlder = %v, want connector.ErrHistoryUnavailable once the timeout elapses", err)
			}
		default:
			t.Fatal("LoadOlder did not return once the timeout elapsed")
		}
	})
}

// TestLoadOlder_WithNoAnchorDoesNothing confirms LoadOlder asks the
// phone for nothing, rather than erroring, when there is no older
// message to anchor from: beforeRemoteID is "" (nothing stored yet) or
// message_keys has no row for it.
func TestLoadOlder_WithNoAnchorDoesNothing(t *testing.T) {
	t.Parallel()

	dev, _, c, _ := connectedMediaFixture(t)

	n, err := c.LoadOlder(t.Context(), directChat, "", 50)
	if err != nil || n != 0 {
		t.Errorf("LoadOlder(no beforeRemoteID) = %d, %v, want 0, nil", n, err)
	}

	n, err = c.LoadOlder(t.Context(), directChat, "unknown", 50)
	if err != nil || n != 0 {
		t.Errorf("LoadOlder(unknown anchor) = %d, %v, want 0, nil", n, err)
	}

	if len(dev.requestHistoryCalls) != 0 {
		t.Errorf("requestHistoryCalls = %d, want none sent with no anchor", len(dev.requestHistoryCalls))
	}
}

// TestLoadOlder_NotConnectedReturnsAnError confirms LoadOlder reports
// errNotConnected, the same as Send and MarkRead, when no run is
// currently connected.
func TestLoadOlder_NotConnectedReturnsAnError(t *testing.T) {
	t.Parallel()

	c := &Connector{account: domain.Account{ID: "wa-1", Service: domain.ServiceWhatsApp}}

	if _, err := c.LoadOlder(t.Context(), directChat, "m1", 50); !errors.Is(err, errNotConnected) {
		t.Errorf("LoadOlder while disconnected = %v, want errNotConnected", err)
	}
}
