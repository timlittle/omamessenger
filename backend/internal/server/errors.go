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
	var mediaErr *app.MediaFetchError

	switch {
	case errors.Is(err, errUnknownMethod):
		return &jsonrpc2.Error{Code: CodeMethodNotFound, Message: "unknown method"}
	case errors.Is(err, errBadParams):
		return &jsonrpc2.Error{Code: CodeInvalidParams, Message: "invalid params"}
	case errors.Is(err, app.ErrInvalidInput):
		return &jsonrpc2.Error{Code: CodeInvalidParams, Message: err.Error()}
	// Checked before the plain domain.ErrNotFound case below, since a
	// *MediaFetchError can wrap that same sentinel (a saved reference
	// gone missing) and still needs its reason category attached.
	case errors.As(err, &mediaErr):
		return mediaFetchRPCError(mediaErr)
	case errors.Is(err, domain.ErrNotFound):
		return &jsonrpc2.Error{Code: CodeNotFound, Message: "not found"}
	default:
		return &jsonrpc2.Error{Code: CodeInternal, Message: "internal error"}
	}
}

// mediaFetchErrorData is the only detail of a failed media.fetch that
// crosses the protocol: a coarse, safe reason category (see
// app.MediaFetchReason), for the UI's "Unavailable" tooltip to show.
type mediaFetchErrorData struct {
	Reason string `json:"reason"`
}

// mediaFetchRPCError turns a *app.MediaFetchError into the internal-error
// code every unexpected failure already uses, with its safe reason
// category attached as the error's data field; nothing else about what
// went wrong leaves the helper.
func mediaFetchRPCError(mediaErr *app.MediaFetchError) *jsonrpc2.Error {
	rpcErr := &jsonrpc2.Error{Code: CodeInternal, Message: "internal error"}
	rpcErr.SetError(mediaFetchErrorData{Reason: string(mediaErr.Reason)})

	return rpcErr
}
