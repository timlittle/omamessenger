# Helper protocol

<!-- Generated from backend/internal/api/protocol_doc_test.go. After changing the protocol run: go test ./backend/internal/api -run TestProtocolDocCurrent -args -update -->

The helper and the UI exchange one JSON object per line. The helper reads requests on stdin and writes responses and events to stdout; stderr carries logs only. Lines are at most 1 MiB.

- Request: `{"id":<int>,"method":"<name>","params":{...}}`
- Response: `{"id":<int>,"result":<value>}` or `{"id":<int>,"error":{"code":"...","message":"..."}}`
- Event: `{"event":"<name>","data":{...}}`

Error codes are `bad_request`, `not_found`, `unknown_method` and `internal`. Any method can answer `internal`; its message never carries internal details. Shapes below are the JSON fields of the Go types in `backend/internal/app` and `backend/internal/domain`.

## Methods

| method | params | result | errors |
|---|---|---|---|
| `hello` | `{}` | `HelloResult {protocol, version, demo, unreadTotal}` |  |
| `accounts.list` | `{}` | `Account[]` |  |
| `conversations.list` | `ConversationsListParams {query}` | `Conversation[]` |  |
| `messages.list` | `MessagesListParams {conversationId, before, limit}` | `MessagesListResult {messages, hasMore}` | bad_request, not_found |
| `messages.send` | `SendMessageParams {conversationId, text}` | `Message {id, conversationId, remoteId, senderId, senderName, text, outgoing, status, created}` | bad_request, not_found |
| `messages.retry` | `RetryParams {messageId}` | `Message {id, conversationId, remoteId, senderId, senderName, text, outgoing, status, created}` | bad_request, not_found |
| `conversations.markRead` | `ConversationParams {conversationId}` | `{}` | not_found |
| `conversations.setMuted` | `SetMutedParams {conversationId, muted}` | `Conversation {id, accountId, service, remoteId, kind, title, members, preview, previewSender, previewOutgoing, unread, muted, lastActivity, match}` | not_found |
| `conversations.open` | `OpenConversationParams {accountId, contactId}` | `Conversation {id, accountId, service, remoteId, kind, title, members, preview, previewSender, previewOutgoing, unread, muted, lastActivity, match}` | bad_request, not_found |
| `contacts.list` | `ContactsListParams {accountId, query}` | `Contact[]` | bad_request, not_found |
| `ui.setFocus` | `FocusParams {conversationId, windowActive}` | `{}` | not_found |
| `settings.apply` | `SettingsParams {notifications, notificationPreview, demoChatter}` | `{}` |  |
| `demo.inject` (demo only) | `InjectParams {conversationId}` | `Message {id, conversationId, remoteId, senderId, senderName, text, outgoing, status, created}` | not_found |

## Events

| event | data | sent when |
|---|---|---|
| `account.updated` | `Account` | an account's status or detail changes |
| `conversation.updated` | `Conversation` | a conversation is created or its preview, unread count, muting or title changes |
| `message.added` | `Message` | any new message is stored, incoming or outgoing |
| `message.updated` | `Message` | a delivery status changes in a way domain.StatusAdvances allows |
| `typing` | `{conversationId, name, active}` | a connector reports a typing indicator |
| `unread.changed` | `{total}` | the unread total across unmuted conversations changes |
