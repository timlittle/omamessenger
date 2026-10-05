# Backend architecture

Generated from `backend/internal/archtest`. Run `go test -mod=vendor ./backend/internal/archtest -args -update` after changing package dependencies or structs.

```mermaid
graph TD
  p0["backend (main)"]
  p1["api"]
  p2["app"]
  p3["archtest"]
  p4["connector"]
  p5["connector/clocktest"]
  p6["connector/demo"]
  p7["domain"]
  p8["notify"]
  p9["rpc"]
  p10["store"]
  p0 --> p1
  p0 --> p2
  p0 --> p4
  p0 --> p6
  p0 --> p8
  p0 --> p9
  p0 --> p10
  p1 --> p2
  p1 --> p7
  p1 --> p9
  p2 --> p4
  p2 --> p7
  p2 --> p8
  p2 --> p10
  p4 --> p7
  p6 --> p4
  p6 --> p7
  p10 --> p7
```

| package | Ca | Ce | I | A | distance | LCOM4 |
|---|---:|---:|---:|---:|---:|---|
| `backend (main)` | 0 | 7 | 1.00 | 0.00 | 0.00 | Config:0, discardWriter:1 |
| `api` | 1 | 3 | 0.75 | 1.00 | 0.75 | — |
| `app` | 2 | 4 | 0.67 | 0.07 | 0.27 | App:1, ContactsListParams:0, ConversationParams:0, ConversationsListParams:0, FocusParams:0, HelloResult:0, InjectParams:0, MessagesListParams:0, MessagesListResult:0, OpenConversationParams:0, RetryParams:0, SendMessageParams:0, SetMutedParams:0, SettingsParams:0, settings:0 |
| `archtest` | 0 | 0 | 0.00 | 0.00 | 1.00 | — |
| `connector` | 3 | 1 | 0.25 | 0.71 | 0.04 | Manager:1, RealClock:2, connectionState:1, trackedSink:1 |
| `connector/clocktest` | 0 | 0 | 0.00 | 0.00 | 1.00 | Clock:1, timer:0 |
| `connector/demo` | 1 | 2 | 0.67 | 0.00 | 0.33 | Injector:1, accountScript:0, conversationScript:0, demoConnector:1, suite:0 |
| `domain` | 5 | 0 | 0.00 | 0.00 | 1.00 | Account:0, Contact:0, Conversation:0, Message:0 |
| `notify` | 2 | 0 | 0.00 | 0.50 | 0.50 | Desktop:1 |
| `rpc` | 2 | 0 | 0.00 | 0.00 | 1.00 | Stream:1, event:0, protocolError:0, request:0, response:0, session:1 |
| `store` | 2 | 1 | 0.33 | 0.00 | 0.67 | Store:1 |
