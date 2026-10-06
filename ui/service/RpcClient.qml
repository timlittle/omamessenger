import QtQuick
import "../lib/Rpc.js" as Rpc

// Sends JSON-RPC 2.0 requests over any transport that exposes write(line)
// and emits a line(text) signal for each line it reads: HelperProcess in
// production, a fake with the same shape in tests. A request that gets no
// reply within 15 seconds fails as cancelPending would; call cancelPending
// directly when the transport itself is about to restart.
//
// Item rather than QtObject: only a type with a default property can hold
// the Connections and Timer children below without naming a property.
Item {
  id: root

  // transport moves the bytes: anything with write(line) and line(text).
  property var transport: null

  // event is emitted for every JSON-RPC notification the helper sends.
  signal event(string name, var data)

  // _pending maps a request id to its callback and send time.
  property var _pending: ({})

  // _pendingCount mirrors the size of _pending as a real property, since
  // mutating the contents of a var property does not notify bindings on
  // its own.
  property int _pendingCount: 0

  // _nextId is the next request id to hand out.
  property int _nextId: 1

  // _timeoutMs is how long a request waits for a reply.
  readonly property int _timeoutMs: 15000

  // request sends method with params over the transport and calls
  // callback(error, result) once the helper replies, or on timeout.
  function request(method: string, params: var, callback: var): void {
    const id = root._nextId++;
    root._pending[id] = { callback: callback, sentAt: Date.now() };
    root._pendingCount++;
    root.transport.write(Rpc.encodeRequest(id, method, params));
  }

  // cancelPending fails every request still waiting for a reply, because
  // the helper that would have answered them is restarting or gone.
  function cancelPending(): void {
    for (const id of Object.keys(root._pending)) {
      root._fail(id, { code: 0, message: "The helper restarted." });
    }
  }

  // _fail removes a pending request and runs its callback with an error.
  function _fail(id: string, error: var): void {
    const entry = root._pending[id];
    if (!entry) return;

    delete root._pending[id];
    root._pendingCount--;
    entry.callback(error, undefined);
  }

  // _handleLine classifies one line from the transport and either
  // resolves a pending request or reports a notification.
  function _handleLine(text: string): void {
    const message = Rpc.parseLine(text);
    if (message.kind === "response") root._resolve(message);
    else if (message.kind === "event") root.event(message.name, message.data);
  }

  // _resolve runs the callback for the request a response answers.
  function _resolve(message: var): void {
    const entry = root._pending[message.id];
    if (!entry) return;

    delete root._pending[message.id];
    root._pendingCount--;
    entry.callback(message.error, message.result);
  }

  // _expirePending fails any request that has waited past the timeout.
  function _expirePending(): void {
    const now = Date.now();
    for (const id of Object.keys(root._pending)) {
      if (now - root._pending[id].sentAt >= root._timeoutMs) {
        root._fail(id, { code: 0, message: "The helper did not respond in time." });
      }
    }
  }

  Connections {
    target: root.transport
    function onLine(text) { root._handleLine(text); }
  }

  Timer {
    interval: 1000
    repeat: true
    running: root._pendingCount > 0
    onTriggered: root._expirePending()
  }
}
