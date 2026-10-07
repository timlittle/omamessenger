package server_test

import (
	"slices"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/cache"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/server"
)

func TestServer_AnswersRequests(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	got, err := call[map[string]any](t, s, "hello", nil)
	if err != nil {
		t.Fatal(err)
	}

	if got["protocol"] != float64(server.Protocol) || got["version"] != "1.2.3" {
		t.Errorf("hello = %v", got)
	}
}

func TestServer_HelloListsAvailableServices(t *testing.T) {
	t.Parallel()

	s := connect(t, false)

	got, err := call[struct {
		Services []domain.Service `json:"services"`
	}](t, s, "hello", nil)
	if err != nil {
		t.Fatal(err)
	}

	want := []domain.Service{{ID: domain.ServiceTelegram, Name: "Telegram"}}
	if !slices.Equal(got.Services, want) {
		t.Errorf("hello services = %+v, want %+v", got.Services, want)
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

	// Without fake connectors the inject method does not exist.
	if _, err := call[any](t, s, "fake.inject", map[string]string{"conversationId": "chat"}); code(err) != server.CodeMethodNotFound {
		t.Errorf("fake.inject = %v, want method not found", err)
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
		{"fake.inject", map[string]any{"conversationId": "chat"}, server.CodeNotFound},
		{"media.fetch", map[string]any{"messageId": " "}, server.CodeInvalidParams},
		{"media.fetch", map[string]any{"messageId": "missing"}, server.CodeNotFound},
		{"messages.react", map[string]any{"messageId": " ", "emoji": "👍"}, server.CodeInvalidParams},
		{"messages.react", map[string]any{"messageId": "missing", "emoji": "👍"}, server.CodeNotFound},
	}

	for _, tt := range tests {
		if _, err := call[any](t, s, tt.method, tt.params); code(err) != tt.want {
			t.Errorf("%s(%v) = %v, want code %d", tt.method, tt.params, err, tt.want)
		}
	}
}

// TestMediaFetch_ReportsTheSafeReasonInItsErrorData confirms a failed
// media.fetch still answers with the fixed "internal error" message
// (see protocol.md), but carries its safe reason category in the
// error's data field, for the UI's "Unavailable" tooltip to read.
func TestMediaFetch_ReportsTheSafeReasonInItsErrorData(t *testing.T) {
	t.Parallel()

	s := connectWithMedia(t, fakeMediaFetcher{err: domain.ErrMediaDecryptFailed}, cache.New(t.TempDir(), 1<<20))
	msg := domain.Message{
		ID: "m1", ConversationID: "chat", RemoteID: "r-1", Text: "[Voice message]", Created: 1,
		Media: &domain.Media{Kind: domain.MediaVoice, FileName: "voice-message.ogg"},
	}
	if _, _, err := s.store.AddMessage(t.Context(), msg); err != nil {
		t.Fatal(err)
	}

	_, err := call[any](t, s, "media.fetch", map[string]any{"messageId": "m1"})
	if code(err) != server.CodeInternal {
		t.Fatalf("media.fetch = %v, want internal error", err)
	}

	data, ok := errorData[struct {
		Reason string `json:"reason"`
	}](err)
	if !ok || data.Reason != "decrypt" {
		t.Errorf("error data = %+v, ok=%v, want reason \"decrypt\"", data, ok)
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
