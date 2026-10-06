package server

import (
	"errors"

	"github.com/sourcegraph/jsonrpc2"

	"github.com/timlittle/omamessenger/backend/internal/app"
	"github.com/timlittle/omamessenger/backend/internal/domain"
)

// Error codes sent to the UI. The JSON-RPC codes are standard; the others
// use the range JSON-RPC reserves for servers.
const (
	CodeInvalidParams  = jsonrpc2.CodeInvalidParams
	CodeMethodNotFound = jsonrpc2.CodeMethodNotFound
	CodeInternal       = jsonrpc2.CodeInternalError
	CodeNotFound       = -32001
)

// Errors detected by the server itself.
var (
	errUnknownMethod = errors.New("unknown method")
	errBadParams     = errors.New("invalid params")
)

// rpcError turns an error into a JSON-RPC error. Only messages written for
// the user cross the protocol; anything unexpected becomes a fixed
// "internal error" so no internal detail or message content leaks.
func rpcError(err error) *jsonrpc2.Error {
	switch {
	case errors.Is(err, errUnknownMethod):
		return &jsonrpc2.Error{Code: CodeMethodNotFound, Message: "unknown method"}
	case errors.Is(err, errBadParams):
		return &jsonrpc2.Error{Code: CodeInvalidParams, Message: "invalid params"}
	case errors.Is(err, app.ErrInvalidInput):
		return &jsonrpc2.Error{Code: CodeInvalidParams, Message: err.Error()}
	case errors.Is(err, domain.ErrNotFound):
		return &jsonrpc2.Error{Code: CodeNotFound, Message: "not found"}
	default:
		return &jsonrpc2.Error{Code: CodeInternal, Message: "internal error"}
	}
}
