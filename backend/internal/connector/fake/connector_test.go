package fake_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector/fake"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestRun_ConnectsAfterDelayAndGoesOfflineOnStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := newRecordingSink()
		stop := runFake(t, fake.New(), sink)

		synctest.Wait()
		if !sink.has("status tg-work connecting") || sink.has("status wa-personal connected") {
			t.Fatalf("at start: %v", sink.take())
		}

		time.Sleep(600 * time.Millisecond)
		synctest.Wait()
		if !sink.has("status wa-personal connected") || sink.has("status tg-work connected") {
			t.Fatalf("at 600ms: %v", sink.take())
		}

		time.Sleep(1400 * time.Millisecond)
		synctest.Wait()
		if !sink.has("status tg-work connected") {
			t.Fatalf("at 2s: %v", sink.take())
		}

		stop()
		if !sink.has("status wa-personal offline") {
			t.Errorf("after stop: %v", sink.take())
		}
	})
}

func TestRun_RefusesASecondRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		suite := fake.New()
		stop := runFake(t, suite, newRecordingSink())
		defer stop()

		synctest.Wait()
		if err := suite.Connectors()[0].Run(t.Context(), newRecordingSink()); !errors.Is(err, fake.ErrAlreadyRunning) {
			t.Errorf("second Run = %v, want ErrAlreadyRunning", err)
		}
	})
}

func TestSend_DirectChatGetsReceiptsAndAReply(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := newRecordingSink()
		suite := fake.New()
		stop := runFake(t, suite, sink)
		defer stop()

		waitConnected(sink)

		wa := suite.Connectors()[0]
		if err := wa.Send(t.Context(), conversation("wa-personal", "wa:mum", domain.KindDirect), domain.Message{ID: "m1"}); err != nil {
			t.Fatal(err)
		}

		time.Sleep(replyWait)
		synctest.Wait()

		want := []string{
			"outgoing m1 fake-m1 sent",
			"outgoing m1 fake-m1 delivered",
			"outgoing m1 fake-m1 read",
			"typing wa:mum true",
			"typing wa:mum false",
			"incoming wa:mum fake-reply-m1",
		}
		if got := sink.take(); !slices.Equal(got, want) {
			t.Errorf("events = %v, want %v", got, want)
		}
	})
}

func TestSend_GroupHasNoReadReceiptOrReply(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := newRecordingSink()
		suite := fake.New()
		stop := runFake(t, suite, sink)
		defer stop()

		waitConnected(sink)

		group := conversation("wa-personal", "wa:climbing-crew", domain.KindGroup)
		if err := suite.Connectors()[0].Send(t.Context(), group, domain.Message{ID: "g1"}); err != nil {
			t.Fatal(err)
		}

		time.Sleep(replyWait)
		synctest.Wait()

		want := []string{"outgoing g1 fake-g1 sent", "outgoing g1 fake-g1 delivered"}
		if got := sink.take(); !slices.Equal(got, want) {
			t.Errorf("events = %v, want %v", got, want)
		}
	})
}

func TestSend_FlakyConversationFailsFirstAttempt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := newRecordingSink()
		suite := fake.New()
		stop := runFake(t, suite, sink)
		defer stop()

		waitConnected(sink)

		wa := suite.Connectors()[0]
		sam := conversation("wa-personal", "wa:sam-spotty", domain.KindDirect)
		if err := wa.Send(t.Context(), sam, domain.Message{ID: "s1"}); err != nil {
			t.Fatal(err)
		}

		time.Sleep(time.Second)
		synctest.Wait()

		if got := sink.take(); !slices.Equal(got, []string{"outgoing s1  failed"}) {
			t.Fatalf("first attempt = %v, want a failure", got)
		}

		if err := wa.Send(t.Context(), sam, domain.Message{ID: "s1"}); err != nil {
			t.Fatal(err)
		}

		time.Sleep(time.Second)
		synctest.Wait()

		if !sink.has("outgoing s1 fake-s1 sent") {
			t.Errorf("retry = %v, want sent", sink.take())
		}
	})
}

func TestStoppedConnector_RefusesWork(t *testing.T) {
	t.Parallel()

	wa := fake.New().Connectors()[0]
	conv := conversation("wa-personal", "wa:mum", domain.KindDirect)

	if err := wa.Send(t.Context(), conv, domain.Message{ID: "m"}); !errors.Is(err, fake.ErrNotRunning) {
		t.Errorf("Send = %v, want ErrNotRunning", err)
	}

	if err := wa.MarkRead(t.Context(), conv); !errors.Is(err, fake.ErrNotRunning) {
		t.Errorf("MarkRead = %v, want ErrNotRunning", err)
	}
}

func TestRunningConnector_HonoursCancelledRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		suite := fake.New()
		stop := runFake(t, suite, newRecordingSink())
		defer stop()

		synctest.Wait()
		wa := suite.Connectors()[0]
		conv := conversation("wa-personal", "wa:mum", domain.KindDirect)

		if err := wa.MarkRead(t.Context(), conv); err != nil {
			t.Errorf("MarkRead = %v", err)
		}

		cancelled, cancel := context.WithCancel(t.Context())
		cancel()

		if err := wa.Send(cancelled, conv, domain.Message{ID: "m"}); err == nil {
			t.Error("Send with a cancelled context succeeded")
		}

		if err := wa.MarkRead(cancelled, conv); err == nil {
			t.Error("MarkRead with a cancelled context succeeded")
		}
	})
}

// replyWait is long enough for every receipt and the reply to a send.
const replyWait = 5 * time.Second
