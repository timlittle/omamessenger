package rpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func readFrames(t *testing.T, output string) []map[string]json.RawMessage {
	t.Helper()
	frames := []map[string]json.RawMessage{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		var frame map[string]json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			t.Fatalf("invalid output line %q: %v", scanner.Text(), err)
		}
		frames = append(frames, frame)
	}
	return frames
}

func responseByID(frames []map[string]json.RawMessage, id int) (map[string]json.RawMessage, bool) {
	for _, frame := range frames {
		var got int
		if err := json.Unmarshal(frame["id"], &got); err == nil && got == id && frame["event"] == nil {
			return frame, true
		}
	}
	return nil, false
}

func errorOf(t *testing.T, frame map[string]json.RawMessage) protocolError {
	t.Helper()
	var e protocolError
	if err := json.Unmarshal(frame["error"], &e); err != nil {
		t.Fatalf("response error %s: %v", frame["error"], err)
	}
	return e
}

func echoHandler() Handler {
	return Handler{"echo": func(_ context.Context, raw json.RawMessage) (any, error) {
		return raw, nil
	}}
}

func TestServeDispatchesAndAnswersBadRequests(t *testing.T) {
	input := strings.Join([]string{
		`{"id":1,"method":"echo","params":{"a":1}}`,
		`{"id":2,"method":"echo"}`,
		`{"id":3,"method":"echo","params":null}`,
		`{"id":4,"method":"nope","params":{}}`,
		`not json`,
		`[]`,
		`{"method":"echo","params":{}}`,
		`{"id":-1,"method":"echo","params":{}}`,
		`{"id":5,"method":"","params":{}}`,
		`{"id":6,"method":"echo","params":[]}`,
		`{"id":7,"method":"echo","params":"x"}`,
		`{"id":"8","method":"echo","params":{}}`,
	}, "\n") + "\n"
	var output bytes.Buffer
	if err := Serve(context.Background(), strings.NewReader(input), NewStream(&output), echoHandler(), nil); err != nil {
		t.Fatal(err)
	}
	frames := readFrames(t, output.String())
	for id, want := range map[int]string{1: `{"a":1}`, 2: `{}`, 3: `{}`} {
		frame, ok := responseByID(frames, id)
		if !ok || string(frame["result"]) != want {
			t.Errorf("request %d result = %s, want %s", id, frame["result"], want)
		}
	}
	if frame, ok := responseByID(frames, 4); !ok || errorOf(t, frame).Code != "unknown_method" {
		t.Errorf("unknown method response = %v", frame)
	}
	badRequests := 0
	for _, frame := range frames {
		if frame["error"] != nil && errorOf(t, frame).Code == "bad_request" && string(frame["id"]) == "0" {
			badRequests++
		}
	}
	if badRequests != 8 {
		t.Errorf("got %d bad_request id=0 frames for malformed lines, want 8", badRequests)
	}
}

func TestServeMapsErrorsThroughCoder(t *testing.T) {
	handler := Handler{"fail": func(context.Context, json.RawMessage) (any, error) {
		return nil, errors.New("private message text and session key")
	}}
	line := `{"id":9,"method":"fail","params":{}}` + "\n"

	var internal bytes.Buffer
	if err := Serve(context.Background(), strings.NewReader(line), NewStream(&internal), handler, nil); err != nil {
		t.Fatal(err)
	}
	frame, _ := responseByID(readFrames(t, internal.String()), 9)
	if got := errorOf(t, frame); got != (protocolError{Code: "internal", Message: "internal error"}) {
		t.Errorf("nil coder error = %+v", got)
	}
	if strings.Contains(internal.String(), "session key") {
		t.Error("nil coder leaked the underlying error text")
	}

	var coded bytes.Buffer
	coder := func(error) (string, string) { return "not_found", "not found" }
	if err := Serve(context.Background(), strings.NewReader(line), NewStream(&coded), handler, coder); err != nil {
		t.Fatal(err)
	}
	frame, _ = responseByID(readFrames(t, coded.String()), 9)
	if got := errorOf(t, frame); got.Code != "not_found" {
		t.Errorf("coder error = %+v", got)
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
	const requestCount, eventCount = 48, 24
	output := &fragmentedWriter{}
	stream := NewStream(output)
	entered, release := make(chan struct{}), make(chan struct{})
	var arrived atomic.Int32
	handler := Handler{"echo": func(context.Context, json.RawMessage) (any, error) {
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
	go func() { done <- Serve(context.Background(), strings.NewReader(input.String()), stream, handler, nil) }()
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
	responses, eventsSeen := 0, 0
	for _, frame := range readFrames(t, output.String()) {
		if frame["event"] != nil {
			eventsSeen++
		} else if frame["result"] != nil {
			responses++
		}
	}
	if responses != requestCount || eventsSeen != eventCount {
		t.Errorf("frames: responses=%d events=%d; want %d and %d", responses, eventsSeen, requestCount, eventCount)
	}
}

func TestServeRejectsOversizedLine(t *testing.T) {
	var output bytes.Buffer
	err := Serve(context.Background(), strings.NewReader(strings.Repeat("x", maxLineBytes+1)), NewStream(&output), Handler{}, nil)
	if err == nil {
		t.Fatal("Serve() accepted a line larger than 1 MiB")
	}
	frames := readFrames(t, output.String())
	if len(frames) != 1 || errorOf(t, frames[0]).Code != "bad_request" {
		t.Fatalf("oversized-line response = %#v", frames)
	}
}

func TestServeRequiresContext(t *testing.T) {
	var ctx context.Context // nil: the case under test
	if err := Serve(ctx, strings.NewReader(""), NewStream(io.Discard), Handler{}, nil); err == nil {
		t.Fatal("Serve(nil ctx) succeeded")
	}
}

func TestServeStopsOnCancelAndClosesInput(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, reader, NewStream(io.Discard), echoHandler(), nil) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve after cancel = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after cancel; blocked read was not closed")
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) { return 0, nil }

func TestServeReportsWriteErrors(t *testing.T) {
	broken := errors.New("broken pipe")
	err := Serve(context.Background(), strings.NewReader("not json\n"), NewStream(failingWriter{broken}), Handler{}, nil)
	if !errors.Is(err, broken) {
		t.Errorf("direct-answer write error = %v, want %v", err, broken)
	}
	line := `{"id":1,"method":"echo","params":{}}` + "\n"
	err = Serve(context.Background(), strings.NewReader(line), NewStream(failingWriter{broken}), echoHandler(), nil)
	if !errors.Is(err, broken) {
		t.Errorf("dispatched write error = %v, want %v", err, broken)
	}
}

func TestEmitAndWriteErrors(t *testing.T) {
	var output bytes.Buffer
	if err := NewStream(&output).Emit("account.updated", map[string]string{"id": "wa"}); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != `{"event":"account.updated","data":{"id":"wa"}}`+"\n" {
		t.Errorf("event frame = %q", got)
	}
	if err := NewStream(zeroWriter{}).Emit("x", nil); !errors.Is(err, io.ErrShortWrite) {
		t.Errorf("zero-byte write = %v, want io.ErrShortWrite", err)
	}
	if err := NewStream(io.Discard).Emit("x", make(chan int)); err == nil {
		t.Error("unencodable event data was accepted")
	}
}
