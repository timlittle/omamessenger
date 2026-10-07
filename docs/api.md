# Helper API

Omarchy's shell starts the helper and talks to it over its stdin and stdout with [JSON-RPC 2.0](https://www.jsonrpc.org/specification), one JSON object per line. The helper exits when stdin closes or on SIGTERM.

```sh
oma-messenger-service [--data-dir DIR] [--db FILE] [--version]
```

The protocol version is `3`.

## Methods

| Method | Params | Result |
| --- | --- | --- |
| `hello` | | `{protocol, version, unreadTotal, services}`; `services` is `[{id, name}]`, the messaging services available to add an account for |
| `accounts.list` | | `[Account]` |
| `accounts.add` | `{service, apiId, apiHash, options}` | `Account`. `apiId` and `apiHash` are Telegram's own API keys, kept for compatibility; `options` is the general form, a map of a provider's own setup values (Telegram reads the same two keys from it as `apiId`/`apiHash` strings). Without either, the account uses OmaMessenger's own keys |
| `accounts.remove` | `{accountId}` | `{}`; signs out and deletes the account's session, credentials and messages |
| `auth.submit` | `{accountId, step, value}` | `{}`; answers an `auth.step`: `phone`, `code` or `password` |
| `contacts.list` | `{accountId, query}` | `[Contact]` |
| `conversations.list` | `{query}` | `[Conversation]`, newest first; `match` holds the newest matching message |
| `conversations.open` | `{accountId, contactId}` | `Conversation`, created if needed |
| `conversations.markRead` | `{conversationId}` | `{}` |
| `conversations.setMuted` | `{conversationId, muted}` | `Conversation` |
| `conversations.setPinned` | `{conversationId, pinned}` | `Conversation`; `conversations.list` always orders pinned conversations first |
| `conversations.setArchived` | `{conversationId, archived}` | `Conversation`; `conversations.list` still returns archived conversations, the UI hides them by default |
| `conversations.setHidden` | `{conversationId, hidden}` | `Conversation`; local to this computer only, never reported to the service. `conversations.list` still returns hidden conversations, the UI hides them by default |
| `messages.list` | `{conversationId, before, limit}` | `{messages, hasMore}`, oldest first; `limit` 1–200, default 50. Past the oldest stored message it fetches older history from the service, which arrives as `message.added` too |
| `messages.send` | `{conversationId, text, attachment, replyTo}` | `Message`; status `failed` if the service refused it. `attachment` is optional: `{path}` names a file on this machine to send, with `text` as its caption (`text` may then be empty); files over 2 GB are rejected. `replyTo`, also optional, is the local id of a message in the same conversation this one answers; it works together with `attachment` |
| `messages.retry` | `{messageId}` | `Message`; only for failed outgoing messages |
| `messages.react` | `{messageId, emoji}` | `Message`; sets the user's reaction to `emoji`, or clears it when `emoji` is `""`. Fails with invalid params if the service does not support reactions |
| `media.fetch` | `{messageId}` | `{path}`: the message's photo, video or file. For a message you sent, its own local copy is returned at once; otherwise it is downloaded into the media cache the first time |
| `media.paste` | | `{path, kind, width, height}`: an image copied off the clipboard into the outgoing media area, for the composer to attach to the next message sent; fails with an invalid-input error when the clipboard holds no image |
| `ui.setFocus` | `{conversationId, windowActive}` | `{}` |
| `settings.apply` | `{notifications, notificationPreview}` | `{}` |
| `fake.inject` | `{conversationId}` | `Message`; only in the test build |

## Events

Events are JSON-RPC notifications: `{"jsonrpc":"2.0","method":"<event>","params":<data>}`.

| Event | Data |
| --- | --- |
| `account.updated` | `Account`; `status` is `connecting`, `connected`, `needs-auth`, `error` or `offline` |
| `account.removed` | `{accountId}` |
| `auth.step` | `{accountId, kind, qr, hint}`: `kind` is `qr` (a base64 PNG to scan; reply with a `phone` to sign in by code instead), `code`, `password` or `linkcode` (WhatsApp's 8-character code, in `hint`, to type on the phone after submitting a `phone` step; no answer needed, it waits for the phone to confirm it) |
| `conversation.updated` | `Conversation` |
| `message.added` | `Message` |
| `message.updated` | `Message`; also sent when the service reports a message edited, or its reactions changed |
| `message.removed` | `{conversationId, messageId}`; sent when the service reports a message deleted |
| `unread.changed` | `{total}` |
| `typing` | `{conversationId, name, active}` |
| `notification.clicked` | `{conversationId}`: the user clicked a desktop notification; the UI opens that conversation |

A `Message` may carry `media`: `{kind, …}` where `kind` is `link` (with `url`, `siteName`, `title`, `description`), `photo`, `video` or `file`. `thumb` is a small base64 JPEG preview sent with the message. `edited` is `true` once the service reports the message changed since it was first sent. A `Message` may also carry `replyTo`: `{remoteId, senderName, text}`, the message it answers, with `senderName` and `text` (a short excerpt) filled in once the quoted message is known locally. `reactions` is `[{emoji, count, mine}]`, omitted when the message has none; a custom-emoji reaction Telegram sends is left out rather than shown as a misleading placeholder.

## Errors

| Code | Meaning |
| --- | --- |
| `-32602` | Invalid params or input. The message says what to fix and is safe to show. |
| `-32601` | Unknown method, for example `fake.inject` outside the test build. |
| `-32001` | The account, contact, conversation or message does not exist. |
| `-32603` | Internal error. Details stay in the helper. |

The shapes of `Account`, `Contact`, `Conversation` and `Message` are the JSON fields in [backend/internal/domain/domain.go](../backend/internal/domain/domain.go).

Adding or changing a method updates `server/methods.go`, its test and this file in the same change. Bump `server.Protocol` when an older UI would break; never break an existing method's params or result shape, add an optional field or a new method instead.
