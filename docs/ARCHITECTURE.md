# Backend architecture

Generated from `backend/internal/archtest`. Run `go test -mod=vendor ./backend/internal/archtest -args -update` after changing package dependencies or structs.

```mermaid
graph TD
  p0["backend (main)"]
  p1["app"]
  p2["archtest"]
  p3["connector"]
  p4["connector/clocktest"]
  p5["connector/demo"]
  p6["domain"]
  p7["notify"]
  p8["rpc"]
  p9["store"]
  p0 --> p1
  p0 --> p3
  p0 --> p5
  p0 --> p7
  p0 --> p8
  p0 --> p9
  p1 --> p3
  p1 --> p6
  p1 --> p7
  p1 --> p9
  p3 --> p6
  p3 --> p9
  p5 --> p3
  p5 --> p6
  p8 --> p1
  p8 --> p9
  p9 --> p6
```

| package | Ca | Ce | I | A | distance | LCOM4 |
|---|---:|---:|---:|---:|---:|---|
| `backend (main)` | 0 | 6 | 1.00 | 0.00 | 0.00 | Config:0, discardWriter:1 |
| `app` | 2 | 4 | 0.67 | 0.07 | 0.27 | App:1, ContactsListParams:0, ConversationParams:0, ConversationsListParams:0, FocusParams:0, HelloResult:0, InjectParams:0, MessagesListParams:0, MessagesListResult:0, OpenConversationParams:0, RetryParams:0, SendMessageParams:0, SetMutedParams:0, SettingsParams:0, settings:0 |
| `archtest` | 0 | 0 | 0.00 | 0.00 | 1.00 | — |
| `connector` | 3 | 2 | 0.40 | 0.67 | 0.07 | Manager:1, RealClock:2, connectionState:1, trackedSink:1 |
| `connector/clocktest` | 0 | 0 | 0.00 | 0.00 | 1.00 | Clock:1, timer:0 |
| `connector/demo` | 1 | 2 | 0.67 | 0.00 | 0.33 | Injector:1, accountScript:0, conversationScript:0, demoConnector:1, suite:0 |
| `domain` | 4 | 0 | 0.00 | 0.00 | 1.00 | Account:0, Contact:0, Conversation:0, Message:0 |
| `notify` | 2 | 0 | 0.00 | 0.50 | 0.50 | Desktop:1 |
| `rpc` | 1 | 2 | 0.67 | 0.00 | 0.33 | Stream:1, event:0, protocolError:0, request:0, response:0 |
| `store` | 4 | 1 | 0.20 | 0.00 | 0.80 | Store:1 |
