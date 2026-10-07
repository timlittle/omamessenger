package fake_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/connectortest"
	"github.com/timlittle/omamessenger/backend/internal/connector/fake"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

func TestRun_ConnectsAfterDelayAndGoesOfflineOnStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &connectortest.Sink{}
		stop := runFake(t, fake.New(), sink)

		synctest.Wait()
		if !sink.Has("status tg-work connecting") || sink.Has("status wa-personal connected") {
			t.Fatalf("at start: %v", sink.Take())
		}

		time.Sleep(600 * time.Millisecond)
		synctest.Wait()
		if !sink.Has("status wa-personal connected") || sink.Has("status tg-work connected") {
			t.Fatalf("at 600ms: %v", sink.Take())
		}

		time.Sleep(1400 * time.Millisecond)
		synctest.Wait()
		if !sink.Has("status tg-work connected") {
			t.Fatalf("at 2s: %v", sink.Take())
		}

		stop()
		if !sink.Has("status wa-personal offline") {
			t.Errorf("after stop: %v", sink.Take())
		}
	})
}

func TestRun_RefusesASecondRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		suite := fake.New()
		stop := runFake(t, suite, &connectortest.Sink{})
		defer stop()

		synctest.Wait()
		if err := suite.Connectors()[0].Run(t.Context(), &connectortest.Sink{}); !errors.Is(err, fake.ErrAlreadyRunning) {
			t.Errorf("second Run = %v, want ErrAlreadyRunning", err)
		}
	})
}

func TestSend_DirectChatGetsAReadReceipt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &connectortest.Sink{}
		suite := fake.New()
		stop := runFake(t, suite, sink)
		defer stop()

		waitConnected(sink)

		wa := suite.Connectors()[0]
		if err := wa.Send(t.Context(), conversation("wa-personal", "wa:mum", domain.KindDirect), domain.Message{ID: "m1"}); err != nil {
			t.Fatal(err)
		}

		time.Sleep(receiptWait)
		synctest.Wait()

		want := []string{
			"outgoing m1 fake-m1 sent",
			"outgoing m1 fake-m1 delivered",
			"outgoing m1 fake-m1 read",
		}
		if got := sink.Take(); !slices.Equal(got, want) {
			t.Errorf("events = %v, want %v", got, want)
		}
	})
}

func TestSend_GroupHasNoReadReceipt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &connectortest.Sink{}
		suite := fake.New()
		stop := runFake(t, suite, sink)
		defer stop()

		waitConnected(sink)

		group := conversation("wa-personal", "wa:climbing-crew", domain.KindGroup)
		if err := suite.Connectors()[0].Send(t.Context(), group, domain.Message{ID: "g1"}); err != nil {
			t.Fatal(err)
		}

		time.Sleep(receiptWait)
		synctest.Wait()

		want := []string{"outgoing g1 fake-g1 sent", "outgoing g1 fake-g1 delivered"}
		if got := sink.Take(); !slices.Equal(got, want) {
			t.Errorf("events = %v, want %v", got, want)
		}
	})
}

func TestSend_FlakyConversationFailsFirstAttempt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &connectortest.Sink{}
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

		if got := sink.Take(); !slices.Equal(got, []string{"outgoing s1  failed"}) {
			t.Fatalf("first attempt = %v, want a failure", got)
		}

		if err := wa.Send(t.Context(), sam, domain.Message{ID: "s1"}); err != nil {
			t.Fatal(err)
		}

		time.Sleep(time.Second)
		synctest.Wait()

		if !sink.Has("outgoing s1 fake-s1 sent") {
			t.Errorf("retry = %v, want sent", sink.Take())
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
		stop := runFake(t, suite, &connectortest.Sink{})
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

// receiptWait is long enough for every receipt of a send.
const receiptWait = 5 * time.Second

func TestLoadOlder_DeliversScriptedOlderHistoryOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &connectortest.Sink{}
		suite := fake.New()
		stop := runFake(t, suite, sink)
		defer stop()

		waitConnected(sink)
		seeded := sink.Messages()["wa:mum"]

		wa, ok := suite.Connectors()[0].(connector.HistoryLoader)
		if !ok {
			t.Fatal("fake connector does not load older history")
		}

		mum := conversation("wa-personal", "wa:mum", domain.KindDirect)
		n, err := wa.LoadOlder(t.Context(), mum, seeded[0].RemoteID, 50)
		if err != nil || n != 40 {
			t.Fatalf("LoadOlder = %d, %v; want 40", n, err)
		}

		older := sink.Messages()["wa:mum"][len(seeded):]
		for _, m := range older {
			if m.Outgoing || m.Created >= seeded[0].Created {
				t.Fatalf("older message %+v is not an incoming message from before the seeded history", m)
			}
		}

		if n, err := wa.LoadOlder(t.Context(), mum, older[0].RemoteID, 50); err != nil || n != 0 {
			t.Errorf("second LoadOlder = %d, %v; want the end of history", n, err)
		}

		alex := conversation("wa-personal", "wa:alex-chen", domain.KindDirect)
		if n, err := wa.LoadOlder(t.Context(), alex, "", 50); err != nil || n != 0 {
			t.Errorf("LoadOlder for a chat with no older history = %d, %v", n, err)
		}
	})
}

func TestFetchMedia_WritesTheScriptedPhoto(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sink := &connectortest.Sink{}
		suite := fake.New()
		stop := runFake(t, suite, sink)
		defer stop()

		waitConnected(sink)
		seeded := sink.Messages()["wa:mum"]
		newest := seeded[len(seeded)-1]
		if newest.Media == nil || newest.Media.Kind != domain.MediaPhoto {
			t.Fatalf("Mum's newest message = %+v, want a photo", newest)
		}

		wa, ok := suite.Connectors()[0].(connector.MediaFetcher)
		if !ok {
			t.Fatal("fake connector does not download media")
		}

		path := filepath.Join(t.TempDir(), "photo.jpg")
		if err := wa.FetchMedia(t.Context(), conversation("wa-personal", "wa:mum", domain.KindDirect), newest.RemoteID, path); err != nil {
			t.Fatal(err)
		}

		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Errorf("downloaded %v, %v; want a file", info, err)
		}
	})
}
