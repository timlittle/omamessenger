// Package server connects the UI to the application over JSON-RPC 2.0, one
// JSON object per line on the helper's stdin and stdout. Requests call
// app.Commands; application events go to the UI as notifications.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/sourcegraph/jsonrpc2"

	"github.com/timlittle/omamessenger/backend/internal/app"
)

// Protocol is the protocol version reported by the hello method. Increase
// it when a change would break an older UI.
const Protocol = 2

// Server is one connection to the UI.
type Server struct {
	version string
	log     *log.Logger

	mu       sync.Mutex
	conn     *jsonrpc2.Conn
	methods  map[string]method
	closing  bool
	handlers sync.WaitGroup
}

// New returns a server that is not yet connected. Events published before
// Start are dropped, because no UI is listening.
func New(version string, logger *log.Logger) *Server {
	return &Server{version: version, log: logger}
}

// Start answers requests from rw with commands until ctx is cancelled or
// the UI disconnects. It returns at once; call Wait to block.
func (s *Server) Start(ctx context.Context, rw io.ReadWriteCloser, commands *app.Commands) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.methods = methods(commands, s.version)
	s.conn = jsonrpc2.NewConn(ctx, jsonrpc2.NewPlainObjectStream(rw), s, jsonrpc2.SetLogger(s.log))
}

// Wait blocks until the connection closes and every request in progress
// has been answered.
func (s *Server) Wait() {
	<-s.connection().DisconnectNotify()

	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()

	s.handlers.Wait()
}

// Publish sends an event to the UI as a JSON-RPC notification. It never
// fails: if the UI has gone, the helper is about to stop anyway.
func (s *Server) Publish(ctx context.Context, event string, data any) {
	conn := s.connection()
	if conn == nil {
		return
	}

	_ = conn.Notify(ctx, event, data) // see the doc comment
}

// Handle starts answering one request. Requests run concurrently, so a slow
// send never holds up the conversation list.
func (s *Server) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closing {
		return
	}

	s.handlers.Go(func() { s.answer(ctx, conn, req) })
}

// connection returns the connection, or nil before Start.
func (s *Server) connection() *jsonrpc2.Conn {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.conn
}

// answer runs the method and replies. Reply errors mean the UI has gone,
// which the connection already reports, so they are not handled here.
func (s *Server) answer(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	result, err := s.call(ctx, req)
	if req.Notif {
		return
	}

	if err != nil {
		_ = conn.ReplyWithError(ctx, req.ID, rpcError(err))
		return
	}

	_ = conn.Reply(ctx, req.ID, result)
}

// call runs the requested method. A panic becomes an internal error, so
// one bad request cannot stop the helper.
func (s *Server) call(ctx context.Context, req *jsonrpc2.Request) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Printf("panic in %s: %v", req.Method, r)
			err = fmt.Errorf("panic in %s", req.Method)
		}
	}()

	m, ok := s.methods[req.Method]
	if !ok {
		return nil, errUnknownMethod
	}

	params := json.RawMessage(`{}`)
	if req.Params != nil {
		params = *req.Params
	}

	return m(ctx, params)
}
