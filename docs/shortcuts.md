# Keyboard shortcuts

OmaMessenger is keyboard-first; every action also works with the mouse. **Ctrl+/** opens the command palette, which lists every command with its shortcut, so this reference only needs updating when a shortcut's keys change, not when a new command is added.

## Global

| Keys | Does |
| --- | --- |
| Ctrl+/ or Ctrl+Shift+P | Command palette |
| Ctrl+K, Ctrl+T or Ctrl+G | Jump to a conversation or search messages, in one palette |
| Ctrl+N or Ctrl+Shift+K | New message |
| Ctrl+J, Alt+Shift+↓ / ↑ | Next / previous unread conversation (works from the list, an open conversation or the unread view, and keeps whichever mode you were in: writing stays writing, scrolling stays scrolling) |
| Ctrl+Shift+A | Show only unread conversations, across every service and account (Esc or the same shortcut returns to the previous list) |
| Alt+↓ / ↑ | Next / previous conversation (same as above: works from any mode and keeps it) |
| Ctrl+0 / 1 / 2 | All / WhatsApp / Telegram |
| Ctrl+Tab, Ctrl+Shift+Tab | Next / previous account or service |
| Ctrl+W | Close the window (asks whether to keep running) |
| Ctrl+Q | Quit |
| Ctrl+R | Retry installing the helper, if it failed to install |
| Esc | Step back: close the palette, a dialog, account setup or the photo viewer, clear the search, leave the composer or the conversation, or leave the unread view |

Ctrl+K and Ctrl+G both open the same palette, so there is one search to reach for instead of two: typing filters conversations by title at once, unread ones first, and, after a short pause, also searches message text, listing matches in their own "Messages" section below, each showing who sent it, which conversation it is in and a snippet. Up/Down move the highlight across both sections together (j/k are not special here: they are just typed into the search field); Enter on a conversation opens it, and Enter on a message opens its conversation and highlights that exact message.

"Toggle read receipts", in the command palette (Ctrl+/), switches incognito mode: off, OmaMessenger never tells a service a chat here was read, so the sender and this account's own other devices keep showing it unread, even though it still clears locally. The footer shows "Read receipts off" whenever incognito mode is on, so it is never a silent surprise. It is also a plugin setting, under the same name.

## Conversation list

| Keys | Does |
| --- | --- |
| j / k | Move the selection down / up |
| Enter, i, l or o | Open the selected conversation |
| m | Mute / unmute |
| a | Archive and mark read |

Pin, hide and snooze are in the command palette, and are local to this computer only. Archiving syncs to the service, like mute and pin.

**Archive all read conversations** (command palette only) archives every conversation in the current list with no unread messages, except pinned ones, after asking "Archive N read conversations?" since it is a bulk action: Enter or y confirms, n or Esc cancels.

**Snooze**: the command palette offers "Snooze until later today" (+3 hours), "tomorrow" (09:00), "next week" (Monday 09:00) and "Snooze until…" for a custom time, typed as a duration (`2h`, `30m`), a clock time (`18:00`) or a weekday and clock time (`mon 9:00`). A snoozed conversation is hidden from the standard list, like hide, until its time arrives, when it returns to the top of the list marked "Reminder" and triggers a desktop notification. "Show all" shows a still-snoozed conversation dimmed, labelled "Snoozed until …". **Remove snooze** (command palette) clears it early.

Ctrl+Shift+A shows the "Unread" view in place of the usual list: every unread conversation across every service and account, overriding whichever rail filter was active rather than narrowing it, the way Slack's own all-unreads view does. Opening a chat from it and reading it keeps that chat visible until you leave it or open another, so it never disappears from under the cursor. With nothing unread, it says "No unread conversations · Esc to leave".

## Open conversation

While not writing, j/k move a highlighted message instead of scrolling by lines, and the view follows it. The highlighted message shows a key-hint row naming what you can press for it.

| Keys | Does |
| --- | --- |
| j / k | Move the highlighted message down / up |
| Ctrl+D, PageDown / Ctrl+U, PageUp | Page down / up |
| G, End / g, Home | Jump to the newest / oldest message |
| i or a | Start writing |
| Enter | Open the highlighted message's photo, video or file, or play/pause its voice note |
| r | Reply to the highlighted message |
| e | React to the highlighted message |
| d | Delete the highlighted message |
| o | Open the highlighted message's link, if it has one |
| p | Go to the message it replies to, if it is one |
| t | Retry the highlighted message, if it failed to send |
| v | Vote in the highlighted message's poll, if it carries an open one |
| h | Back to the list |

The highlight starts on the newest message when the conversation opens or when you leave the composer with Esc. The composer shows which mode it is in: dimmed, with an "i to write" hint, while scrolling; full contrast, with a "Writing · Esc to stop" hint, once it has focus.

| Keys | Does |
| --- | --- |
| Enter | Send |
| Shift+Enter | Start a new line instead of sending |
| Ctrl+V | Paste a clipboard image as an attachment |

Ctrl+J always jumps to the next unread conversation instead, even while writing (see Global); it lands there in writing mode too.

### Voting in a poll

v, on a highlighted message with an open poll, moves the highlight into its options instead of the message list.

| Keys | Does |
| --- | --- |
| j / k, ↓ / ↑ | Move the highlighted option |
| Space | Check or uncheck the highlighted option (a multiple-choice poll only) |
| Enter | Cast the vote: whatever is checked, or the highlighted option alone if nothing was |
| Esc | Cancel without voting |

Clicking an option votes for it directly, without opening vote mode first. A closed poll shows its results but takes no clicks and has no vote mode.

### @-mention picker

Typing "@" in a group conversation opens a picker of the group's members, filtered as you keep typing their name. It closes on its own once the query no longer matches an "@name" in progress.

| Keys | Does |
| --- | --- |
| ↓ / ↑ | Move the highlighted member |
| Tab or Enter | Insert the highlighted member's name as a mention |
| Esc | Close the picker without inserting, staying in the composer |

A message that mentions you is shown with a subtle highlight and an "@" mark.

## Adding an account

The service chooser, shown when more than one service is offered, and the QR and phone steps all work without the mouse or Tab.

| Keys | Does |
| --- | --- |
| j / k, ↓ / ↑ | Move the highlighted service |
| Enter | Choose the highlighted service |
| t / w | Jump straight to Telegram / WhatsApp |
| p | At the QR step, use a phone number instead |
| q | At the phone step, go back to the QR code |
| Esc | At the phone step, back to the QR code; anywhere else, cancel setup |

## Removing an account

| Keys | Does |
| --- | --- |
| j / k, ↓ / ↑ | Move the highlighted account |
| 1-9 | Jump straight to that account |
| Enter or y | Remove the highlighted account (does nothing if Cancel, the default, is highlighted) |
| n or Esc | Cancel without removing anything |

## Closing the window

Ctrl+W asks whether to keep OmaMessenger running in the background or quit.

| Keys | Does |
| --- | --- |
| h / l, ← / → | Move the highlight between Cancel, Quit and Keep in background |
| Enter | Choose the highlighted answer (Keep in background by default) |
| c / q / k | Cancel / Quit / Keep in background, straight away |
| Esc | Cancel |

## Deleting a message

d, on a highlighted message, asks whether to delete it. Your own messages offer deleting for everyone; any message offers deleting for you only.

| Keys | Does |
| --- | --- |
| h / l, ← / → | Move the highlight between the choices on offer |
| Enter | Choose the highlighted answer (Cancel by default) |
| e / m / n | Delete for everyone (your own messages only) / delete for me / cancel, straight away |
| Esc | Cancel |

## Photo viewer

Clicking a photo opens it inside the window, sized to fit.

| Keys | Does |
| --- | --- |
| ← / → | Previous / next photo in the conversation |
| o | Open in your own image viewer |

**Open in image viewer** (o) opens the photo in your own application; videos and files always open externally.

## Remapping keys

Every shortcut above is a default; override one by editing `keys.conf` in `$XDG_CONFIG_HOME/omamessenger` (or `~/.config/omamessenger` when `XDG_CONFIG_HOME` is not set). The command palette's **Open key bindings file** creates it, pre-filled with every action and its default keys as commented-out lines, and opens it in your own editor. It does not exist until you ask for it, and nothing here ever touches `~/.config/omarchy`.

The format is one override per line:

```
action.name = Key[, Key…]
```

- `#` starts a whole-line comment; blank lines are ignored
- a key is written the way this file's own template shows it, such as `Ctrl+J`, `Alt+Shift+Down` or a bare letter like `g`; a bare uppercase letter means Shift
- several keys for one action are comma-separated: `unread.next = Ctrl+J, Alt+Shift+Down`
- an override *replaces* that action's default keys; it does not add to them

The file is re-read automatically whenever you save it; no restart needed. An unknown action name or a key OmaMessenger cannot parse is reported, not fatal: that one line is ignored and everything else in the file still applies. If two actions end up wanting the same key in the same context, that is reported too, and the default wins for both, the same as if neither line had been written. The command palette, the footer's key hints and a highlighted message's own key-hint row all show the effective keys, overrides included.

The command palette's **Show key bindings** lists every effective binding by context, marking which are overridden by `keys.conf` and showing any conflicts or errors from the file. For a bug report, or anywhere outside the running shell, `make keys` prints the same listing from a checkout of this repository, reading the same file.
