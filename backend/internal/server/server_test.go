package server_test

import (
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/server"
)

func TestServer_AnswersRequests(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	got, err := call[map[string]any](t, s, "hello", nil)
	if err != nil {
		t.Fatal(err)
	}

	if got["protocol"] != float64(server.Protocol) || got["version"] != "1.2.3" || got["demo"] != false {
		t.Errorf("hello = %v", got)
	}
}

func TestServer_PublishesEventsAsNotifications(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	s.ingest.Typing(t.Context(), "wa", "r-chat", "Alex", true)

	s.events.await(t, app.EventTyping)
}

func TestServer_DropsEventsBeforeStart(t *testing.T) {
	t.Parallel()

	srv := server.New("1", nil)
	srv.Publish(t.Context(), app.EventTyping, nil) // must not panic
}

func TestServer_RejectsUnknownMethods(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	if _, err := call[any](t, s, "nope", nil); code(err) != server.CodeMethodNotFound {
		t.Errorf("nope = %v, want method not found", err)
	}

	// Outside demo mode the demo method does not exist.
	if _, err := call[any](t, s, "demo.inject", map[string]string{"conversationId": "chat"}); code(err) != server.CodeMethodNotFound {
		t.Errorf("demo.inject = %v, want method not found", err)
	}
}

func TestServer_IgnoresNotificationsFromTheUI(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	if err := s.client.Notify(t.Context(), "hello", nil); err != nil {
		t.Fatal(err)
	}

	// The connection still works afterwards.
	if _, err := call[any](t, s, "accounts.list", nil); err != nil {
		t.Errorf("accounts.list after a notification = %v", err)
	}
}

func TestServer_MapsErrorsToCodes(t *testing.T) {
	t.Parallel()

	s := connect(t, true)
	tests := []struct {
		method string
		params any
		want   int64
	}{
		{"messages.send", map[string]any{"conversationId": "chat", "text": " "}, server.CodeInvalidParams},
		{"messages.send", map[string]any{"conversationId": 7}, server.CodeInvalidParams},
		{"messages.send", map[string]any{"conversationId": "missing", "text": "hi"}, server.CodeNotFound},
		{"demo.inject", map[string]any{"conversationId": "chat"}, server.CodeNotFound},
	}

	for _, tt := range tests {
		if _, err := call[any](t, s, tt.method, tt.params); code(err) != tt.want {
			t.Errorf("%s(%v) = %v, want code %d", tt.method, tt.params, err, tt.want)
		}
	}
}

func TestServer_WaitReturnsWhenTheUIDisconnects(t *testing.T) {
	t.Parallel()

	s := connect(t, false)
	_ = s.client.Close()

	done := make(chan struct{})
	go func() {
		s.server.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return after the UI disconnected")
	}
}
