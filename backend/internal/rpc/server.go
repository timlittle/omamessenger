// Package rpc implements the JSON-lines framing between Quickshell and the
// helper: one request per input line, one response or event per output line.
// It knows nothing about the methods it serves; see package api for those.
package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

const maxLineBytes = 1 << 20

var errMalformedRequest = errors.New("malformed request")

// Method handles a decoded JSON params object for one protocol method.
type Method func(context.Context, json.RawMessage) (any, error)

// Handler maps protocol method names to methods.
type Handler map[string]Method

// ErrorCoder turns a method error into a protocol error code and message.
// It must never return internal details such as message text.
type ErrorCoder func(error) (code, message string)

type request struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type protocolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type response struct {
	ID     int            `json:"id"`
	Result any            `json:"result,omitempty"`
	Error  *protocolError `json:"error,omitempty"`
}

type event struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

// Stream serializes responses and events to one protocol output stream.
type Stream struct {
	w  io.Writer
	mu sync.Mutex
}

// NewStream returns a Stream writing frames to w.
func NewStream(w io.Writer) *Stream { return &Stream{w: w} }

// Emit writes one event frame. Errors are returned so the host can decide how
// to handle a broken UI connection; callers must not log event payloads.
func (s *Stream) Emit(name string, data any) error {
	return s.write(event{Event: name, Data: data})
}

func (s *Stream) write(value any) error {
	line, err := json.Marshal(value)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(line) > 0 {
		n, err := s.w.Write(line)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		line = line[n:]
	}
	return nil
}

// Serve reads request lines from r until EOF or ctx is canceled, dispatches
// each to its method concurrently, and writes responses to out. Events
// emitted on the same Stream never interleave with responses. An io.Closer
// input is closed on cancel so a blocked read returns. A nil coder reports
// every method error as "internal".
func Serve(ctx context.Context, r io.Reader, out *Stream, h Handler, coder ErrorCoder) error {
	if ctx == nil {
		return errors.New("rpc server requires a context")
	}
	stop := closeOnCancel(ctx, r)
	defer stop()
	if coder == nil {
		coder = internalOnly
	}
	sess := &session{ctx: ctx, stream: out, handler: h, coder: coder}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), maxLineBytes)
	for ctx.Err() == nil && scanner.Scan() {
		if err := sess.handleLine(scanner.Bytes()); err != nil {
			sess.wait()
			return err
		}
	}
	return sess.finish(scanner.Err())
}

func internalOnly(error) (string, string) { return "internal", "internal error" }

// closeOnCancel closes r when ctx ends; the returned stop releases the watcher.
func closeOnCancel(ctx context.Context, r io.Reader) (stop func()) {
	closer, ok := r.(io.Closer)
	if !ok {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = closer.Close()
		case <-done:
		}
	}()
	return func() { close(done) }
}

// session is the state of one Serve call.
type session struct {
	ctx      context.Context
	stream   *Stream
	handler  Handler
	coder    ErrorCoder
	handlers sync.WaitGroup
	mu       sync.Mutex
	writeErr error
}

// handleLine answers malformed and unknown requests directly and starts a
// goroutine for the rest. Only a failed direct write is returned.
func (sess *session) handleLine(raw []byte) error {
	req, err := decodeRequest(append([]byte(nil), raw...))
	if err != nil {
		return sess.stream.write(errorFrame(0, "bad_request", errMalformedRequest.Error()))
	}
	method, ok := sess.handler[req.Method]
	if !ok {
		return sess.stream.write(errorFrame(req.ID, "unknown_method", "unknown method"))
	}
	sess.handlers.Add(1)
	go sess.dispatch(req, method)
	return nil
}

func (sess *session) dispatch(req request, method Method) {
	defer sess.handlers.Done()
	result, err := method(sess.ctx, req.Params)
	frame := response{ID: req.ID, Result: result}
	if err != nil {
		code, message := sess.coder(err)
		frame = errorFrame(req.ID, code, message)
	}
	if err := sess.stream.write(frame); err != nil {
		sess.mu.Lock()
		if sess.writeErr == nil {
			sess.writeErr = err
		}
		sess.mu.Unlock()
	}
}

func (sess *session) wait() { sess.handlers.Wait() }

// finish waits for in-flight methods and reports the first failure: an
// oversized line (answered with bad_request), then any write error. Errors
// caused by cancellation are not failures.
func (sess *session) finish(scanErr error) error {
	if scanErr != nil {
		_ = sess.stream.write(errorFrame(0, "bad_request", "request line exceeds the 1 MiB limit"))
	}
	sess.wait()
	if sess.ctx.Err() != nil {
		return nil
	}
	if scanErr != nil {
		return scanErr
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()
	return sess.writeErr
}

func errorFrame(id int, code, message string) response {
	return response{ID: id, Error: &protocolError{Code: code, Message: message}}
}

// decodeRequest parses one request line. The id field must be present and
// non-negative, method must be non-empty, and params, when present and not
// null, must be a JSON object.
func decodeRequest(line []byte) (request, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil || fields == nil {
		return request{}, errMalformedRequest
	}
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return req, errMalformedRequest
	}
	if _, hasID := fields["id"]; !hasID || req.ID < 0 || req.Method == "" {
		return req, errMalformedRequest
	}
	params, err := normalizeParams(req.Params)
	req.Params = params
	return req, err
}

func normalizeParams(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`{}`), nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return raw, errMalformedRequest
	}
	return raw, nil
}
