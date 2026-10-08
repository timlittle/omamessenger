# Keyboard shortcuts

OmaMessenger is keyboard-first; every action also works with the mouse. **Ctrl+/** opens the command palette, which lists every command with its shortcut, so this reference only needs updating when a shortcut's keys change, not when a new command is added.

## Global

| Keys | Does |
| --- | --- |
| Ctrl+/ or Ctrl+Shift+P | Command palette |
| Ctrl+K or Ctrl+T | Jump to a conversation |
| Ctrl+G | Search messages |
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
| h | Back to the list |

The highlight starts on the newest message when the conversation opens or when you leave the composer with Esc. The composer shows which mode it is in: dimmed, with an "i to write" hint, while scrolling; full contrast, with a "Writing · Esc to stop" hint, once it has focus.

| Keys | Does |
| --- | --- |
| Enter | Send |
| Shift+Enter | Start a new line instead of sending |
| Ctrl+V | Paste a clipboard image as an attachment |

Ctrl+J always jumps to the next unread conversation instead, even while writing (see Global); it lands there in writing mode too.

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
