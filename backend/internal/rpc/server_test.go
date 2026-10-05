package rpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/connector"
	"github.com/timlittle/omamessenger/backend/internal/connector/clocktest"
	"github.com/timlittle/omamessenger/backend/internal/domain"
	"github.com/timlittle/omamessenger/backend/internal/notify"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

type rpcConnector struct{ account domain.Account }

func (c *rpcConnector) Account() domain.Account { return c.account }
func (*rpcConnector) Run(ctx context.Context, _ connector.Sink) error {
	<-ctx.Done()
	return nil
}
func (*rpcConnector) Send(context.Context, domain.Conversation, domain.Message) error { return nil }
func (*rpcConnector) MarkRead(context.Context, domain.Conversation) error             { return nil }

type rpcInjector struct{}

func (rpcInjector) Inject(remoteID string) (domain.Message, error) {
	return domain.Message{ConversationID: remoteID, Text: "demo injected"}, nil
}

type rpcFixture struct {
	app     *app.App
	store   *store.Store
	ctx     context.Context
	cancel  context.CancelFunc
	manager *connector.Manager
	stream  *Stream
	output  *bytes.Buffer
}

func newRPCFixture(t *testing.T, w io.Writer) *rpcFixture {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "data", "messages.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	clock := clocktest.New(time.Unix(1000, 0))
	service := app.New(db, nil, &notify.Recorder{}, clock)
	stream := NewStream(w)
	service.Emit = func(name string, data any) { _ = stream.Emit(name, data) }
	manager := &connector.Manager{
		Store: db, Sink: service, Clock: clock,
		Connectors: []connector.Connector{&rpcConnector{account: domain.Account{ID: "wa", Service: domain.ServiceWhatsApp, Name: "Personal"}}},
	}
	service.Manager = manager
	ctx, cancel := context.WithCancel(context.Background())
	if err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		manager.Wait()
	})
	if _, _, err := db.EnsureConversation(domain.Conversation{
		ID: "chat", AccountID: "wa", RemoteID: "remote-chat", Kind: domain.KindDirect, Title: "Chat",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertContact(domain.Contact{AccountID: "wa", RemoteID: "new-contact", Name: "New Contact"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.AddMessage(domain.Message{ID: "unread", ConversationID: "chat", Text: "unread", Created: 1}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.AddMessage(domain.Message{ID: "retry", ConversationID: "chat", Text: "retry me", Outgoing: true, Status: domain.StatusPending, Created: 2}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpdateMessageStatus("retry", domain.StatusFailed); err != nil {
		t.Fatal(err)
	}
	service.Demo = true
	service.DemoInject = rpcInjector{}
	f := &rpcFixture{app: service, store: db, ctx: ctx, cancel: cancel, manager: manager, stream: stream}
	if out, ok := w.(*bytes.Buffer); ok {
		f.output = out
	}
	return f
}

func writeRequest(t *testing.T, w *io.PipeWriter, id int, method string, params string) {
	t.Helper()
	if _, err := fmt.Fprintf(w, `{"id":%d,"method":%q,"params":%s}`+"\n", id, method, params); err != nil {
		t.Fatal(err)
	}
}

func readFrames(t *testing.T, output string) ([]map[string]json.RawMessage, error) {
	t.Helper()
	frames := []map[string]json.RawMessage{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		var frame map[string]json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			return nil, fmt.Errorf("invalid output line %q: %w", scanner.Text(), err)
		}
		frames = append(frames, frame)
	}
	return frames, scanner.Err()
}

func responseByID(frames []map[string]json.RawMessage, id int) (map[string]json.RawMessage, bool) {
	for _, frame := range frames {
		var got int
		if err := json.Unmarshal(frame["id"], &got); err == nil && got == id {
			return frame, true
		}
	}
	return nil, false
}

func errorCode(t *testing.T, frame map[string]json.RawMessage) string {
	t.Helper()
	var responseError protocolError
	if err := json.Unmarshal(frame["error"], &responseError); err != nil {
		t.Fatalf("response error %s: %v", frame["error"], err)
	}
	return responseError.Code
}

func TestRegisterAndServeC3MethodsOverPipe(t *testing.T) {
	var output bytes.Buffer
	f := newRPCFixture(t, &output)
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- f.stream.Serve(f.ctx, reader, Register(f.app)) }()

	requests := []struct {
		id     int
		method string
		params string
	}{
		{1, "hello", `{}`},
		{2, "accounts.list", `{}`},
		{3, "conversations.list", `{}`},
		{4, "messages.list", `{"conversationId":"chat"}`},
		{5, "messages.send", `{"conversationId":"chat","text":"hello"}`},
		{6, "messages.retry", `{"messageId":"retry"}`},
		{7, "conversations.markRead", `{"conversationId":"chat"}`},
		{8, "conversations.setMuted", `{"conversationId":"chat","muted":true}`},
		{9, "conversations.open", `{"accountId":"wa","contactId":"new-contact"}`},
		{10, "contacts.list", `{"accountId":"wa","query":"new"}`},
		{11, "ui.setFocus", `{"conversationId":"chat","windowActive":true}`},
		{12, "settings.apply", `{"notifications":true,"notificationPreview":false,"demoChatter":false}`},
		{13, "demo.inject", `{"conversationId":"chat"}`},
		{14, "method.does.not.exist", `{}`},
		{15, "messages.list", `{"conversationId":"chat","limit":201}`},
		{16, "conversations.markRead", `{"conversationId":"missing"}`},
	}
	for _, request := range requests {
		writeRequest(t, writer, request.id, request.method, request.params)
	}
	if _, err := io.WriteString(writer, "{\"id\":17\n[]\n"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Serve(): %v", err)
	}
	frames, err := readFrames(t, output.String())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13} {
		frame, ok := responseByID(frames, id)
		if !ok || frame["error"] != nil || frame["result"] == nil {
			t.Errorf("method request %d response = %#v", id, frame)
		}
	}
	for id, want := range map[int]string{14: "unknown_method", 15: "bad_request", 16: "not_found"} {
		frame, ok := responseByID(frames, id)
		if !ok || errorCode(t, frame) != want {
			t.Errorf("request %d error response = %#v, want %q", id, frame, want)
		}
	}
	malformedCount := 0
	for _, frame := range frames {
		if errorCodeRaw, ok := frame["error"]; ok {
			var responseError protocolError
			if json.Unmarshal(errorCodeRaw, &responseError) == nil && responseError.Code == "bad_request" {
				if _, hasID := frame["id"]; hasID {
					var id int
					_ = json.Unmarshal(frame["id"], &id)
					if id == 0 {
						malformedCount++
					}
				}
			}
		}
	}
	if malformedCount < 2 {
		t.Errorf("got %d bad_request id=0 responses for malformed lines", malformedCount)
	}
	if len(Register(f.app)) != 13 {
		t.Errorf("Register() has %d C3 methods, want 13", len(Register(f.app)))
	}
}

func TestServeMapsInternalErrorsWithoutLeakingDetails(t *testing.T) {
	var output bytes.Buffer
	handler := Handler{"secret": func(context.Context, json.RawMessage) (any, error) {
		return nil, errors.New("private message text and session key")
	}}
	if err := Serve(context.Background(), strings.NewReader(`{"id":9,"method":"secret","params":{}}`), &output, handler); err != nil {
		t.Fatal(err)
	}
	frames, err := readFrames(t, output.String())
	if err != nil || len(frames) != 1 {
		t.Fatalf("frames = %#v, %v", frames, err)
	}
	if got := errorCode(t, frames[0]); got != "internal" {
		t.Errorf("internal error code = %q", got)
	}
	if strings.Contains(output.String(), "session key") || strings.Contains(output.String(), "private message") {
		t.Error("internal error response leaked the underlying error details")
	}
}

type fragmentedWriter struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *fragmentedWriter) Write(p []byte) (int, error) {
	first := len(p) / 2
	if first == 0 {
		first = len(p)
	}
	w.mu.Lock()
	_, _ = w.b.Write(p[:first])
	w.mu.Unlock()
	runtime.Gosched()
	w.mu.Lock()
	_, _ = w.b.Write(p[first:])
	w.mu.Unlock()
	return len(p), nil
}

func (w *fragmentedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

func TestConcurrentResponsesAndEventsDoNotInterleave(t *testing.T) {
	const requestCount = 48
	const eventCount = 24
	output := &fragmentedWriter{}
	stream := NewStream(output)
	entered := make(chan struct{})
	release := make(chan struct{})
	var arrived atomic.Int32
	handler := Handler{"echo": func(_ context.Context, raw json.RawMessage) (any, error) {
		if arrived.Add(1) == requestCount {
			close(entered)
		}
		<-release
		return map[string]string{"payload": strings.Repeat("x", 1024)}, nil
	}}
	var input strings.Builder
	for i := 1; i <= requestCount; i++ {
		fmt.Fprintf(&input, `{"id":%d,"method":"echo","params":{}}`+"\n", i)
	}
	done := make(chan error, 1)
	go func() { done <- stream.Serve(context.Background(), strings.NewReader(input.String()), handler) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("all concurrent handlers did not start")
	}
	var events sync.WaitGroup
	for i := 0; i < eventCount; i++ {
		events.Add(1)
		go func(i int) {
			defer events.Done()
			if err := stream.Emit("test.event", map[string]any{"index": i, "payload": strings.Repeat("e", 512)}); err != nil {
				t.Errorf("Emit(): %v", err)
			}
		}(i)
	}
	close(release)
	events.Wait()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	frames, err := readFrames(t, output.String())
	if err != nil {
		t.Fatal(err)
	}
	responses, eventsSeen := 0, 0
	for _, frame := range frames {
		if frame["event"] != nil {
			eventsSeen++
		} else if frame["id"] != nil && frame["result"] != nil {
			responses++
		}
	}
	if responses != requestCount || eventsSeen != eventCount {
		t.Errorf("valid frames responses=%d events=%d; want %d and %d", responses, eventsSeen, requestCount, eventCount)
	}
}

func TestServeRejectsOversizedLine(t *testing.T) {
	line := strings.Repeat("x", maxLineBytes+1)
	var output bytes.Buffer
	err := Serve(context.Background(), strings.NewReader(line), &output, Handler{})
	if err == nil {
		t.Fatal("Serve() accepted a line larger than 1 MiB")
	}
	frames, parseErr := readFrames(t, output.String())
	if parseErr != nil || len(frames) != 1 || errorCode(t, frames[0]) != "bad_request" {
		t.Fatalf("oversized-line response = %#v, %v", frames, parseErr)
	}
}
