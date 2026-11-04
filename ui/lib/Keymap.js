.pragma library

// Key bindings for the OmaMessenger UI, following Slack's shortcuts where
// Slack has one, with j/k-style keys as extras. Matches key events to
// actions and lists the commands the command palette offers.

// KEY holds the Qt key codes the bindings use; they are defined here
// because the node tests have no Qt. Letters use their uppercase ASCII code
// and digits their ASCII code, as Qt does.
var KEY = {
  Escape: 0x01000000, Tab: 0x01000001, Backtab: 0x01000002,
  Return: 0x01000004, Enter: 0x01000005, Insert: 0x01000006, Home: 0x01000010,
  End: 0x01000011, Left: 0x01000012, Up: 0x01000013, Right: 0x01000014, Down: 0x01000015,
  PageUp: 0x01000016, PageDown: 0x01000017,
  Slash: 0x2f, Question: 0x3f, Space: 0x20, Period: 0x2e
};

// MOD holds the Qt keyboard modifier masks.
var MOD = { Shift: 0x02000000, Ctrl: 0x04000000, Alt: 0x08000000 };

// KEY_NAMES shows spec names the way people type them.
var KEY_NAMES = { Slash: '/', Question: '?', Up: '↑', Down: '↓', Left: '←', Right: '→', Escape: 'Esc', Period: '.' };

// BINDINGS define every key action: action name, key specs, contexts
// (or "global"), label, whether the footer hints it, and whether the
// command palette offers it.
var BINDINGS = [
  { action: 'palette.commands', keys: ['Ctrl+Slash', 'Ctrl+Shift+Question', 'Ctrl+Shift+P'], contexts: ['global'], label: 'Command palette', hint: true },
  { action: 'palette.conversations', keys: ['Ctrl+K', 'Ctrl+T'], contexts: ['global'], label: 'Jump to conversation', hint: true, command: true },
  { action: 'list.showAll', keys: [], contexts: ['global'], label: 'Show or hide all chats', command: true },
  // Every unread conversation across every service, overriding the rail filter.
  { action: 'list.unread', keys: ['Ctrl+Shift+A'], contexts: ['global'], label: 'Show unread conversations', command: true },
  { action: 'search.focus', keys: ['Ctrl+G'], contexts: ['global'], label: 'Search messages', command: true },
  { action: 'chat.new', keys: ['Ctrl+N', 'Ctrl+Shift+K'], contexts: ['global'], label: 'New message', hint: true, command: true },
  // compose's own Ctrl+J binding wins while writing, so this only fires outside the composer.
  { action: 'unread.next', keys: ['Ctrl+J', 'Alt+Shift+Down'], contexts: ['global'], label: 'Next unread conversation', command: true },
  { action: 'unread.prev', keys: ['Alt+Shift+Up'], contexts: ['global'], label: 'Previous unread conversation', command: true },
  { action: 'chat.next', keys: ['Alt+Down'], contexts: ['global'], label: 'Next conversation', command: true },
  { action: 'chat.prev', keys: ['Alt+Up'], contexts: ['global'], label: 'Previous conversation', command: true },
  { action: 'account.add', keys: [], contexts: ['global'], label: 'Add an account', command: true },
  { action: 'account.remove', keys: [], contexts: ['global'], label: 'Remove an account', command: true },
  { action: 'account.rename', keys: [], contexts: ['global'], label: 'Rename account', command: true },
  { action: 'account.addOwnKeys', keys: [], contexts: ['global'], label: 'Add a Telegram account with your own API keys', command: true },
  // Off means MarkRead never reaches the service, so a chat read here stays unread elsewhere.
  { action: 'settings.toggleReadReceipts', keys: [], contexts: ['global'], label: 'Toggle read receipts', command: true },
  { action: 'rail.all', keys: ['Ctrl+0'], contexts: ['global'], label: 'Show all services', command: true },
  { action: 'rail.whatsapp', keys: ['Ctrl+1'], contexts: ['global'], label: 'Show WhatsApp', command: true },
  { action: 'rail.telegram', keys: ['Ctrl+2'], contexts: ['global'], label: 'Show Telegram', command: true },
  { action: 'rail.next', keys: ['Ctrl+Tab'], contexts: ['global'], label: 'Next account or service', command: true },
  { action: 'rail.prev', keys: ['Ctrl+Shift+Tab'], contexts: ['global'], label: 'Previous account or service', command: true },
  { action: 'window.hide', keys: ['Ctrl+W'], contexts: ['global'], label: 'Close window', command: true },
  { action: 'app.quit', keys: ['Ctrl+Q'], contexts: ['global'], label: 'Quit OmaMessenger', command: true },
  { action: 'helper.retryInstall', keys: ['Ctrl+R'], contexts: ['global'], label: 'Retry installing the helper', command: true },
  { action: 'helper.doctor', keys: [], contexts: ['global'], label: 'Run health check', command: true },
  { action: 'whatsNew.show', keys: [], contexts: ['global'], label: "What's new", command: true },
  // These manage keys.conf rather than binding a key of their own.
  { action: 'keys.openConfig', keys: [], contexts: ['global'], label: 'Open key bindings file', command: true },
  { action: 'keys.showBindings', keys: [], contexts: ['global'], label: 'Show key bindings', command: true },
  { action: 'scheduled.show', keys: [], contexts: ['global'], label: 'Scheduled messages', command: true, keywords: ['send later'] },
  // Acts on the open chat's queued message, directly or via a picker if there are several.
  { action: 'scheduled.sendNow', keys: [], contexts: ['conversation'], label: 'Send scheduled message now', command: true, keywords: ['send later', 'scheduled'] },
  { action: 'scheduled.edit', keys: [], contexts: ['conversation'], label: 'Edit scheduled message', command: true, keywords: ['send later', 'scheduled'] },
  { action: 'scheduled.cancel', keys: [], contexts: ['conversation'], label: 'Cancel scheduled message', command: true, keywords: ['send later', 'scheduled'] },
  { action: 'escape', keys: ['Escape'], contexts: ['global'], label: 'Back' },
  // Routes paste through the bounded helper path in every field except the
  // composer, which owns these keys itself via compose.attach (see docs/decisions.md).
  { action: 'field.paste', keys: ['Ctrl+V', 'Shift+Insert'], contexts: ['global'], label: 'Paste' },

  { action: 'cursor.down', keys: ['j', 'Down'], contexts: ['list'], label: 'Next chat' },
  { action: 'cursor.up', keys: ['k', 'Up'], contexts: ['list'], label: 'Previous chat' },
  { action: 'cursor.top', keys: ['g', 'Home'], contexts: ['list'], label: 'First chat' },
  { action: 'cursor.bottom', keys: ['G', 'End'], contexts: ['list'], label: 'Last chat' },
  { action: 'chat.open', keys: ['Enter', 'l', 'o', 'i'], contexts: ['list'], label: 'Open chat', hint: true },
  { action: 'pane.conversation', keys: ['Tab'], contexts: ['list'], label: 'Go to conversation' },
  { action: 'chat.mute', keys: ['m'], contexts: ['list', 'conversation'], label: 'Mute or unmute chat', command: true },
  { action: 'chat.pin', keys: [], contexts: ['list', 'conversation'], label: 'Pin or unpin chat', command: true },
  // Only does something on a chat that is already pinned.
  { action: 'chat.movePinUp', keys: ['Ctrl+Shift+Up'], contexts: ['list', 'conversation'], label: 'Move pin up', command: true },
  { action: 'chat.movePinDown', keys: ['Ctrl+Shift+Down'], contexts: ['list', 'conversation'], label: 'Move pin down', command: true },
  { action: 'chat.archive', keys: [], contexts: ['list', 'conversation'], label: 'Archive or unarchive chat', command: true },
  { action: 'chat.hide', keys: [], contexts: ['list', 'conversation'], label: 'Hide or unhide chat', command: true },
  // keywords adds "read" as a standalone search term for the palette (see Palette.js's bestScore).
  { action: 'chat.markUnread', keys: [], contexts: ['list', 'conversation'], label: 'Mark as unread', command: true, keywords: ['read', 'unread'] },
  { action: 'chat.markRead', keys: [], contexts: ['list', 'conversation'], label: 'Mark as read', command: true, keywords: ['read'] },
  // Only ever applies to a direct chat's one contact; on a group it fails with a safe error.
  { action: 'chat.block', keys: [], contexts: ['list', 'conversation'], label: 'Block contact', command: true },
  { action: 'chat.unblock', keys: [], contexts: ['list', 'conversation'], label: 'Unblock contact', command: true },
  { action: 'chat.reportSpam', keys: [], contexts: ['list', 'conversation'], label: 'Report as spam', command: true },
  { action: 'chat.setNotificationSound', keys: [], contexts: ['list', 'conversation'], label: 'Set notification sound…', command: true },
  { action: 'chat.resetNotificationSound', keys: [], contexts: ['list', 'conversation'], label: 'Reset notification sound', command: true },
  { action: 'chat.playNotificationSound', keys: [], contexts: ['conversation'], label: 'Play notification sound', command: true },
  // Shift+A, not a bare "a", so a stray keypress can't archive the chat by accident.
  // Also unarchives an already-archived chat (see ListController._archiveAndMarkRead).
  { action: 'chat.archiveRead', keys: ['A'], contexts: ['list'], label: 'Archive and mark read', command: true },
  { action: 'list.archiveAllRead', keys: [], contexts: ['global'], label: 'Archive all read conversations', command: true },
  // Reverses whatever the toast's "u to undo" is currently offering; a no-op once there is none.
  { action: 'chat.undo', keys: ['u'], contexts: ['list', 'conversation'], label: 'Undo last action', command: true },
  { action: 'chat.snoozeLaterToday', keys: [], contexts: ['list', 'conversation'], label: 'Snooze until later today', command: true },
  { action: 'chat.snoozeTomorrow', keys: [], contexts: ['list', 'conversation'], label: 'Snooze until tomorrow', command: true },
  { action: 'chat.snoozeNextWeek', keys: [], contexts: ['list', 'conversation'], label: 'Snooze until next week', command: true },
  { action: 'chat.snoozeCustom', keys: [], contexts: ['list', 'conversation'], label: 'Snooze until…', command: true },
  { action: 'chat.unsnooze', keys: [], contexts: ['list', 'conversation'], label: 'Remove snooze', command: true },

  { action: 'message.highlightNewer', keys: ['j', 'Down'], contexts: ['conversation'], label: 'Highlight the next message' },
  { action: 'message.highlightOlder', keys: ['k', 'Up'], contexts: ['conversation'], label: 'Highlight the previous message' },
  { action: 'scroll.pageDown', keys: ['Ctrl+D', 'PageDown'], contexts: ['conversation'], label: 'Page down' },
  { action: 'scroll.pageUp', keys: ['Ctrl+U', 'PageUp'], contexts: ['conversation'], label: 'Page up' },
  { action: 'scroll.newest', keys: ['G', 'End'], contexts: ['conversation'], label: 'Newest message' },
  { action: 'scroll.oldest', keys: ['g', 'Home'], contexts: ['conversation'], label: 'Oldest message' },
  { action: 'compose.focus', keys: ['i', 'a'], contexts: ['conversation'], label: 'Write a message', hint: true },
  { action: 'pane.list', keys: ['h', 'Tab'], contexts: ['conversation'], label: 'Back to the list' },
  { action: 'message.open', keys: ['Enter'], contexts: ['conversation'], label: 'Open the highlighted message' },
  { action: 'message.reply', keys: ['r'], contexts: ['conversation'], label: 'Reply to the highlighted message', command: true },
  { action: 'message.react', keys: ['e'], contexts: ['conversation'], label: 'React to the highlighted message', command: true },
  // Does nothing on a message that is not editable (see Edit.isEditable).
  { action: 'message.edit', keys: ['E'], contexts: ['conversation'], label: 'Edit the highlighted message', command: true },
  { action: 'message.delete', keys: ['d'], contexts: ['conversation'], label: 'Delete the highlighted message', command: true },
  { action: 'message.retry', keys: ['t'], contexts: ['conversation'], label: 'Retry the highlighted message', command: true },
  { action: 'message.openLink', keys: ['o'], contexts: ['conversation'], label: "Open the highlighted message's link", command: true },
  { action: 'message.goToQuote', keys: ['p'], contexts: ['conversation'], label: 'Go to the replied-to message', command: true },
  { action: 'message.play', keys: ['Space'], contexts: ['conversation'], label: 'Play or pause the highlighted video', command: true },
  { action: 'message.copyMedia', keys: ['c'], contexts: ['conversation'], label: "Copy the highlighted photo or video", command: true },
  { action: 'message.saveMedia', keys: ['s'], contexts: ['conversation'], label: "Save the highlighted photo or video", command: true },
  { action: 'message.openExternal', keys: ['x'], contexts: ['conversation'], label: "Open the highlighted photo or video externally", command: true },
  // Not hinted in the footer; its hint comes from Highlight.hints when a poll is highlighted.
  { action: 'message.vote', keys: ['v'], contexts: ['conversation'], label: 'Vote in the highlighted poll', command: true },

  { action: 'message.send', keys: ['Enter'], contexts: ['compose'], label: 'Send', hint: true },
  // Wins over the global unread.next binding on the same key while writing.
  { action: 'compose.newline', keys: ['Shift+Enter', 'Ctrl+J'], contexts: ['compose'], label: 'New line', hint: true },
  { action: 'compose.attach', keys: ['Ctrl+V', 'Shift+Insert'], contexts: ['compose'], label: 'Paste a clipboard image as an attachment' },
  { action: 'compose.attachFile', keys: [], contexts: ['conversation', 'compose'], label: 'Attach a file', command: true },
  // Queues the composer's text to send at a chosen time (see WindowController._scheduleDraft).
  { action: 'compose.sendLater', keys: ['Ctrl+L'], contexts: ['compose'], label: 'Send later', hint: true, command: true },
  // Only fires when the composer is empty and the last message is editable (see Edit.upMeansEdit).
  { action: 'compose.editLast', keys: ['Up'], contexts: ['compose'], label: 'Edit your last message' },
  // Opens the spelling picker for the word the caret is inside; a no-op elsewhere.
  { action: 'compose.spellSuggestions', keys: ['Ctrl+Period'], contexts: ['compose'], label: 'Spelling suggestions', hint: true, command: true },
  { action: 'search.accept', keys: ['Enter', 'Down'], contexts: ['search'], label: 'First result', hint: true },

  { action: 'viewer.next', keys: ['Right'], contexts: ['viewer'], label: 'Next photo', hint: true },
  { action: 'viewer.prev', keys: ['Left'], contexts: ['viewer'], label: 'Previous photo', hint: true },
  { action: 'viewer.openExternal', keys: ['o'], contexts: ['viewer'], label: 'Open in your own application', hint: true, command: true },

  { action: 'whatsNew.down', keys: ['j', 'Down'], contexts: ['whatsNew'], label: 'Next release' },
  { action: 'whatsNew.up', keys: ['k', 'Up'], contexts: ['whatsNew'], label: 'Previous release' },

  { action: 'dialog.down', keys: ['Down', 'Ctrl+J'], contexts: ['dialog'], label: 'Next contact' },
  { action: 'dialog.up', keys: ['Up', 'Ctrl+K'], contexts: ['dialog'], label: 'Previous contact' },
  { action: 'dialog.accept', keys: ['Enter'], contexts: ['dialog'], label: 'Start chat', hint: true },
  { action: 'dialog.nextAccount', keys: ['Ctrl+Tab'], contexts: ['dialog'], label: 'Next account', hint: true },

  { action: 'palette.down', keys: ['Down', 'Ctrl+J', 'Ctrl+N'], contexts: ['palette'], label: 'Next item' },
  { action: 'palette.up', keys: ['Up', 'Ctrl+K', 'Ctrl+P'], contexts: ['palette'], label: 'Previous item' },
  { action: 'palette.accept', keys: ['Enter'], contexts: ['palette'], label: 'Run', hint: true },

  { action: 'reaction.left', keys: ['Left'], contexts: ['reactionPicker'], label: 'Previous emoji' },
  { action: 'reaction.right', keys: ['Right'], contexts: ['reactionPicker'], label: 'Next emoji' },
  { action: 'reaction.accept', keys: ['Enter'], contexts: ['reactionPicker'], label: 'React', hint: true },

  // Space toggles an option (for multi-choice polls); Enter also casts the vote once any is checked.
  { action: 'pollVote.down', keys: ['j', 'Down'], contexts: ['pollVote'], label: 'Next option' },
  { action: 'pollVote.up', keys: ['k', 'Up'], contexts: ['pollVote'], label: 'Previous option' },
  { action: 'pollVote.toggle', keys: ['Space'], contexts: ['pollVote'], label: 'Check or uncheck the highlighted option', hint: true },
  { action: 'pollVote.accept', keys: ['Enter'], contexts: ['pollVote'], label: 'Cast the vote', hint: true },
  { action: 'pollVote.cancel', keys: ['Escape'], contexts: ['pollVote'], label: 'Cancel', hint: true },

  // The @-mention picker. Up/Down, not j/k, since those would just be typed as text.
  { action: 'mention.down', keys: ['Down'], contexts: ['mentionPicker'], label: 'Next member' },
  { action: 'mention.up', keys: ['Up'], contexts: ['mentionPicker'], label: 'Previous member' },
  { action: 'mention.accept', keys: ['Tab', 'Enter'], contexts: ['mentionPicker'], label: 'Insert mention', hint: true },
  { action: 'mention.cancel', keys: ['Escape'], contexts: ['mentionPicker'], label: 'Close without inserting', hint: true },

  // The spelling suggestions picker, opened with compose.spellSuggestions or a right-click.
  { action: 'spell.down', keys: ['Down'], contexts: ['spellPicker'], label: 'Next suggestion' },
  { action: 'spell.up', keys: ['Up'], contexts: ['spellPicker'], label: 'Previous suggestion' },
  { action: 'spell.accept', keys: ['Tab', 'Enter'], contexts: ['spellPicker'], label: 'Replace word', hint: true },
  { action: 'spell.addToDictionary', keys: ['a'], contexts: ['spellPicker'], label: 'Add to dictionary', hint: true },
  { action: 'spell.ignore', keys: ['i'], contexts: ['spellPicker'], label: 'Ignore for this session', hint: true },
  { action: 'spell.cancel', keys: ['Escape'], contexts: ['spellPicker'], label: 'Close', hint: true },

  // The service chooser, the first step of adding an account.
  { action: 'setup.down', keys: ['j', 'Down'], contexts: ['chooseService'], label: 'Next service' },
  { action: 'setup.up', keys: ['k', 'Up'], contexts: ['chooseService'], label: 'Previous service' },
  { action: 'setup.accept', keys: ['Enter'], contexts: ['chooseService'], label: 'Choose the highlighted service', hint: true },
  { action: 'setup.chooseTelegram', keys: ['t'], contexts: ['chooseService'], label: 'Choose Telegram', hint: true },
  { action: 'setup.chooseWhatsapp', keys: ['w'], contexts: ['chooseService'], label: 'Choose WhatsApp', hint: true },

  // The QR and phone steps can switch to each other without starting over.
  { action: 'qr.usePhone', keys: ['p'], contexts: ['qr'], label: 'Use phone number instead', hint: true },
  { action: 'phone.useQr', keys: ['q'], contexts: ['phone'], label: 'Use QR code instead', hint: true },
  // Backs out to the QR code rather than cancelling the whole sign-in.
  { action: 'phone.back', keys: ['Escape'], contexts: ['phone'], label: 'Back to the QR code' },

  { action: 'remove.down', keys: ['j', 'Down'], contexts: ['removeAccount'], label: 'Next account' },
  { action: 'remove.up', keys: ['k', 'Up'], contexts: ['removeAccount'], label: 'Previous account' },
  { action: 'remove.pick1', keys: ['1'], contexts: ['removeAccount'], label: 'Highlight the first account' },
  { action: 'remove.pick2', keys: ['2'], contexts: ['removeAccount'], label: 'Highlight the second account' },
  { action: 'remove.pick3', keys: ['3'], contexts: ['removeAccount'], label: 'Highlight the third account' },
  { action: 'remove.pick4', keys: ['4'], contexts: ['removeAccount'], label: 'Highlight the fourth account' },
  { action: 'remove.pick5', keys: ['5'], contexts: ['removeAccount'], label: 'Highlight the fifth account' },
  { action: 'remove.pick6', keys: ['6'], contexts: ['removeAccount'], label: 'Highlight the sixth account' },
  { action: 'remove.pick7', keys: ['7'], contexts: ['removeAccount'], label: 'Highlight the seventh account' },
  { action: 'remove.pick8', keys: ['8'], contexts: ['removeAccount'], label: 'Highlight the eighth account' },
  { action: 'remove.pick9', keys: ['9'], contexts: ['removeAccount'], label: 'Highlight the ninth account' },
  { action: 'remove.accept', keys: ['Enter', 'y'], contexts: ['removeAccount'], label: 'Remove the highlighted account', hint: true },
  { action: 'remove.cancel', keys: ['n'], contexts: ['removeAccount'], label: 'Cancel', hint: true },

  { action: 'rename.down', keys: ['j', 'Down'], contexts: ['renameAccount'], label: 'Next account' },
  { action: 'rename.up', keys: ['k', 'Up'], contexts: ['renameAccount'], label: 'Previous account' },
  { action: 'rename.pick1', keys: ['1'], contexts: ['renameAccount'], label: 'Highlight the first account' },
  { action: 'rename.pick2', keys: ['2'], contexts: ['renameAccount'], label: 'Highlight the second account' },
  { action: 'rename.pick3', keys: ['3'], contexts: ['renameAccount'], label: 'Highlight the third account' },
  { action: 'rename.pick4', keys: ['4'], contexts: ['renameAccount'], label: 'Highlight the fourth account' },
  { action: 'rename.pick5', keys: ['5'], contexts: ['renameAccount'], label: 'Highlight the fifth account' },
  { action: 'rename.pick6', keys: ['6'], contexts: ['renameAccount'], label: 'Highlight the sixth account' },
  { action: 'rename.pick7', keys: ['7'], contexts: ['renameAccount'], label: 'Highlight the seventh account' },
  { action: 'rename.pick8', keys: ['8'], contexts: ['renameAccount'], label: 'Highlight the eighth account' },
  { action: 'rename.pick9', keys: ['9'], contexts: ['renameAccount'], label: 'Highlight the ninth account' },
  { action: 'rename.accept', keys: ['Enter'], contexts: ['renameAccount'], label: 'Rename the highlighted account', hint: true },
  { action: 'rename.cancel', keys: ['n'], contexts: ['renameAccount'], label: 'Cancel', hint: true },

  // The close question: Cancel, Quit and Keep.
  { action: 'close.left', keys: ['h', 'Left'], contexts: ['confirm'], label: 'Previous choice' },
  { action: 'close.right', keys: ['l', 'Right'], contexts: ['confirm'], label: 'Next choice' },
  { action: 'close.accept', keys: ['Enter'], contexts: ['confirm'], label: 'Choose the highlighted answer', hint: true },
  { action: 'close.cancel', keys: ['c'], contexts: ['confirm'], label: 'Cancel', hint: true },
  { action: 'close.quit', keys: ['q'], contexts: ['confirm'], label: 'Quit', hint: true },
  { action: 'close.keep', keys: ['k'], contexts: ['confirm'], label: 'Keep in background', hint: true },

  // The delete question. Cancel is the default highlight, so a stray Enter never deletes anything.
  { action: 'delete.left', keys: ['h', 'Left'], contexts: ['deleteConfirm'], label: 'Previous choice' },
  { action: 'delete.right', keys: ['l', 'Right'], contexts: ['deleteConfirm'], label: 'Next choice' },
  { action: 'delete.accept', keys: ['Enter'], contexts: ['deleteConfirm'], label: 'Choose the highlighted answer', hint: true },
  { action: 'delete.everyone', keys: ['e'], contexts: ['deleteConfirm'], label: 'Delete for everyone', hint: true },
  { action: 'delete.forMe', keys: ['m'], contexts: ['deleteConfirm'], label: 'Delete for me', hint: true },
  { action: 'delete.cancel', keys: ['n'], contexts: ['deleteConfirm'], label: 'Cancel', hint: true },

  { action: 'archiveAll.accept', keys: ['Enter', 'y'], contexts: ['archiveConfirm'], label: 'Archive the read conversations', hint: true },
  { action: 'archiveAll.cancel', keys: ['n'], contexts: ['archiveConfirm'], label: 'Cancel', hint: true },

  { action: 'blockConfirm.accept', keys: ['Enter', 'y'], contexts: ['blockConfirm'], label: 'Confirm', hint: true },
  { action: 'blockConfirm.cancel', keys: ['n'], contexts: ['blockConfirm'], label: 'Cancel', hint: true },

  // A file that can run code or load a web page: Enter picks the safe default, not Open.
  { action: 'openConfirm.save', keys: ['Enter', 's'], contexts: ['openConfirm'], label: 'Save to Downloads', hint: true },
  { action: 'openConfirm.accept', keys: ['y'], contexts: ['openConfirm'], label: 'Open anyway', hint: true },
  { action: 'openConfirm.cancel', keys: ['n'], contexts: ['openConfirm'], label: 'Cancel', hint: true }
];

// effectiveBindings returns service's own effective bindings (defaults
// merged with the user's keys.conf overrides, computed in Service.qml),
// or the plain defaults for a service that has none yet: missing,
// still starting, or too old to report them. Every key-aware view reads
// its bindings through this rather than repeating the same fallback.
function effectiveBindings(service) {
  return (service && service.effectiveBindings) || BINDINGS;
}

// match returns the action for a key press in a context, or "". Bindings
// for the context win over global ones. list defaults to BINDINGS; a
// caller with a keys.conf override merged in (see KeyBindings.js) passes
// its own effective list instead.
function match(context, key, modifiers, text, list) {
  const bindings = list || BINDINGS;
  const candidates = bindings.filter((b) => b.contexts.includes(context))
    .concat(bindings.filter((b) => b.contexts.includes('global')));
  const hit = candidates.find((b) => b.keys.some((spec) => specMatches(parseSpec(spec), key, modifiers, text)));

  return hit ? hit.action : '';
}

// parseSpec turns a key spec such as "Ctrl+K", "G" or "?" into what a key
// press must look like. A lone uppercase letter means Shift; "/" and "?"
// match the typed text, because their key codes differ between layouts.
function parseSpec(spec) {
  const parts = spec.split('+');
  const name = parts.pop();
  const lone = parts.length === 0;

  if (lone && (name === '/' || name === '?')) {
    return { text: name };
  }

  const isUpperLetter = /^[A-Z]$/.test(name);

  return {
    keys: keyCodes(name),
    shift: parts.includes('Shift') || (lone && isUpperLetter),
    ctrl: parts.includes('Ctrl'),
    alt: parts.includes('Alt')
  };
}

// keyCodes returns the Qt key codes a spec name stands for. Enter covers
// both Enter keys, and Tab covers Backtab, which Qt sends for Shift+Tab.
function keyCodes(name) {
  if (name === 'Enter') {
    return [KEY.Return, KEY.Enter];
  }

  if (name === 'Tab') {
    return [KEY.Tab, KEY.Backtab];
  }

  if (name in KEY) {
    return [KEY[name]];
  }

  return /^[a-zA-Z0-9]$/.test(name) ? [name.toUpperCase().charCodeAt(0)] : [];
}

// specMatches reports whether a key press matches a parsed spec. Text specs
// ignore Shift, which some layouts need to type them.
function specMatches(spec, key, modifiers, text) {
  const has = (mask) => (modifiers & mask) !== 0;

  if (spec.text) {
    return text === spec.text && !has(MOD.Ctrl) && !has(MOD.Alt);
  }

  return spec.keys.includes(key)
    && has(MOD.Shift) === spec.shift && has(MOD.Ctrl) === spec.ctrl && has(MOD.Alt) === spec.alt;
}

// bindingsFor returns the bindings worth hinting in a context's footer:
// the context's own first, then global ones. list defaults to BINDINGS,
// same as match.
function bindingsFor(context, list) {
  const bindings = list || BINDINGS;
  const hinted = (b) => b.hint;

  return bindings.filter((b) => b.contexts.includes(context) && hinted(b))
    .concat(bindings.filter((b) => b.contexts.includes('global') && hinted(b)));
}

// display shows a key spec the way people read it: "Ctrl+Slash" as
// "Ctrl+/", "Alt+Down" as "Alt+↓".
function display(spec) {
  return spec.split('+').map((part) => KEY_NAMES[part] ?? part).join('+');
}

// keyFor returns the display text for action's primary key, or "" when it has none.
function keyFor(action, list) {
  const bindings = list || BINDINGS;
  const binding = bindings.find((b) => b.action === action);
  return binding && binding.keys.length > 0 ? display(binding.keys[0]) : '';
}

// commands returns what the command palette offers: every command
// binding with its first key, plus keywords and whether it is
// chat-scoped, both read by Palette.js's ranking.
function commands(list) {
  const bindings = list || BINDINGS;
  return bindings
    .filter((b) => b.command)
    .map((b) => ({
      action: b.action, label: b.label, keys: b.keys.length > 0 ? display(b.keys[0]) : '',
      keywords: b.keywords || [], scoped: b.contexts.includes('list') || b.contexts.includes('conversation')
    }));
}

// composeHint names the composer's current mode: what starts writing,
// or what stops it, reading the wording off the real key bindings.
function composeHint(writing, list) {
  const bindings = list || BINDINGS;
  const action = writing ? 'escape' : 'compose.focus';
  const key = display(bindings.find((b) => b.action === action).keys[0]);

  return writing ? `Writing · ${key} to stop` : `${key} to write`;
}
