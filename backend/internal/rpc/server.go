// Package rpc implements the local JSON-lines protocol between Quickshell and
// the Go application service.
package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/store"
)

const maxLineBytes = 1 << 20

var errMalformedRequest = errors.New("malformed request")

// Method handles a decoded JSON params object for one protocol method.
type Method func(context.Context, json.RawMessage) (any, error)

// Handler maps protocol method names to application operations.
type Handler map[string]Method

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

// Serve reads request lines until EOF, dispatches requests concurrently, and
// writes response/event frames without interleaving. An io.Closer input is
// closed when ctx is canceled so a blocked scan can exit.
func Serve(ctx context.Context, r io.Reader, w io.Writer, h Handler) error {
	return NewStream(w).Serve(ctx, r, h)
}

// Serve reads requests using this Stream, allowing App.Emit and responses to
// share the same serialization lock.
func (s *Stream) Serve(ctx context.Context, r io.Reader, h Handler) error {
	if ctx == nil {
		return errors.New("rpc server requires a context")
	}
	done := make(chan struct{})
	if closer, ok := r.(io.Closer); ok {
		go func() {
			select {
			case <-ctx.Done():
				_ = closer.Close()
			case <-done:
			}
		}()
	}
	defer close(done)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), maxLineBytes)
	var handlers sync.WaitGroup
	var writeErrMu sync.Mutex
	var writeErr error
	for scanner.Scan() {
		if ctx.Err() != nil {
			break
		}
		line := append([]byte(nil), scanner.Bytes()...)
		decoded, err := decodeRequest(line)
		if err != nil {
			if writeErr := s.write(response{ID: 0, Error: &protocolError{Code: "bad_request", Message: errMalformedRequest.Error()}}); writeErr != nil {
				return writeErr
			}
			continue
		}
		method, ok := h[decoded.Method]
		if !ok {
			if err := s.write(response{ID: decoded.ID, Error: &protocolError{Code: "unknown_method", Message: "unknown method"}}); err != nil {
				return err
			}
			continue
		}
		handlers.Add(1)
		go func(req request, call Method) {
			defer handlers.Done()
			result, err := call(ctx, req.Params)
			frame := response{ID: req.ID}
			if err != nil {
				frame.Error = mapError(err)
			} else {
				frame.Result = result
			}
			if err := s.write(frame); err != nil {
				writeErrMu.Lock()
				if writeErr == nil {
					writeErr = err
				}
				writeErrMu.Unlock()
			}
		}(decoded, method)
	}
	if err := scanner.Err(); err != nil {
		_ = s.write(response{ID: 0, Error: &protocolError{Code: "bad_request", Message: "request line exceeds the 1 MiB limit"}})
		handlers.Wait()
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	handlers.Wait()
	writeErrMu.Lock()
	defer writeErrMu.Unlock()
	if writeErr != nil {
		return writeErr
	}
	if ctx.Err() != nil {
		return nil
	}
	return nil
}

func decodeRequest(line []byte) (request, error) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return req, errMalformedRequest
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil || fields == nil {
		return req, errMalformedRequest
	}
	if _, hasID := fields["id"]; !hasID || req.ID < 0 || req.Method == "" {
		return req, errMalformedRequest
	}
	if len(req.Params) == 0 || string(req.Params) == "null" {
		req.Params = json.RawMessage(`{}`)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(req.Params, &object); err != nil || object == nil {
		return req, errMalformedRequest
	}
	return req, nil
}

func mapError(err error) *protocolError {
	switch {
	case errors.Is(err, app.ErrBadRequest):
		return &protocolError{Code: "bad_request", Message: err.Error()}
	case errors.Is(err, store.ErrNotFound):
		return &protocolError{Code: "not_found", Message: "not found"}
	case errors.Is(err, app.ErrUnknownMethod):
		return &protocolError{Code: "unknown_method", Message: "unknown method"}
	default:
		return &protocolError{Code: "internal", Message: "internal error"}
	}
}

// Register maps every C3 method name to the corresponding typed App method.
func Register(service *app.App) Handler {
	return Handler{
		"hello":                  bind(service.Hello),
		"accounts.list":          bind(service.AccountsList),
		"conversations.list":     bind(service.ConversationsList),
		"messages.list":          bind(service.MessagesList),
		"messages.send":          bind(service.SendMessage),
		"messages.retry":         bind(service.Retry),
		"conversations.markRead": bind(service.MarkRead),
		"conversations.setMuted": bind(service.SetMuted),
		"conversations.open":     bind(service.OpenConversation),
		"contacts.list":          bind(service.ContactsList),
		"ui.setFocus":            bind(service.SetFocus),
		"settings.apply":         bind(service.ApplySettings),
		"demo.inject":            bind(service.Inject),
	}
}

func bind[P any, R any](fn func(context.Context, P) (R, error)) Method {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var params P
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, fmt.Errorf("%w: invalid params", app.ErrBadRequest)
		}
		return fn(ctx, params)
	}
}
