## Helper Protocol

The QML UI talks to the Go helper over the helper's stdin/stdout using JSON-RPC 2.0, one JSON object per line. The Go side uses `github.com/sourcegraph/jsonrpc2`; we do not write our own framing or dispatch. Why stdio and not a socket: see `docs/decisions.md`.

- Requests: `{"jsonrpc":"2.0","id":N,"method":"conversations.list","params":{…}}`
- Events from the helper are JSON-RPC notifications (no `id`): `{"jsonrpc":"2.0","method":"message.added","params":{…}}`
- Method names are `noun.verb` in lowerCamelCase: `messages.send`, `conversations.markRead`
- Params and results are named objects, never positional arrays; field names are lowerCamelCase
- Handlers are thin: decode params, call one `app` method, encode the result. No business logic in the `server` package
- Errors use the codes in `server/errors.go`: the standard JSON-RPC codes plus `-32001` for not found. Only invalid-input messages are written for the user and cross the protocol; anything unexpected becomes a fixed "internal error"
- Adding or changing a method: update `server/methods.go`, its test and `docs/api.md` in the same change. Bump `server.Protocol` when an older UI would break
- Never break an existing method's params or result shape; add an optional field or a new method instead
- stdout is reserved for protocol traffic. Diagnostics go to stderr
- The UI side lives in `ui/lib/Rpc.js` (encode, parse, error text) and `ui/service/`; components never build protocol messages
