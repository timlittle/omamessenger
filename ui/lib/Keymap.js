.pragma library

// Key bindings for the OmaMessenger UI, following Slack's shortcuts where
// Slack has one, with j/k-style keys as extras. Matches key events to
// actions and lists the commands the command palette offers.

// KEY holds the Qt key codes the bindings use; they are defined here
// because the node tests have no Qt. Letters use their uppercase ASCII code
// and digits their ASCII code, as Qt does.
var KEY = {
  Escape: 0x01000000, Tab: 0x01000001, Backtab: 0x01000002,
  Return: 0x01000004, Enter: 0x01000005, Home: 0x01000010,
  End: 0x01000011, Left: 0x01000012, Up: 0x01000013, Right: 0x01000014, Down: 0x01000015,
  PageUp: 0x01000016, PageDown: 0x01000017,
  Slash: 0x2f, Question: 0x3f
};

// MOD holds the Qt keyboard modifier masks.
var MOD = { Shift: 0x02000000, Ctrl: 0x04000000, Alt: 0x08000000 };

// KEY_NAMES shows spec names the way people type them.
var KEY_NAMES = { Slash: '/', Question: '?', Up: '↑', Down: '↓', Left: '←', Right: '→', Escape: 'Esc' };

// BINDINGS define every key action. Each has an action name, key specs,
// the contexts it applies in (or "global"), a label, whether the footer
// hints it, and whether the command palette
// offers it. Global bindings use Ctrl or Alt, or are Escape, so they never
// steal typing.
var BINDINGS = [
  { action: 'palette.commands', keys: ['Ctrl+Slash', 'Ctrl+Shift+Question', 'Ctrl+Shift+P'], contexts: ['global'], label: 'Command palette', hint: true },
  { action: 'palette.conversations', keys: ['Ctrl+K', 'Ctrl+T'], contexts: ['global'], label: 'Jump to conversation', hint: true, command: true },
  { action: 'list.showAll', keys: [], contexts: ['global'], label: 'Show or hide all chats', command: true },
  { action: 'search.focus', keys: ['Ctrl+G'], contexts: ['global'], label: 'Search messages', command: true },
  { action: 'chat.new', keys: ['Ctrl+N', 'Ctrl+Shift+K'], contexts: ['global'], label: 'New message', hint: true, command: true },
  { action: 'unread.next', keys: ['Ctrl+J', 'Alt+Shift+Down'], contexts: ['global'], label: 'Next unread conversation', command: true },
  { action: 'unread.prev', keys: ['Alt+Shift+Up'], contexts: ['global'], label: 'Previous unread conversation', command: true },
  { action: 'chat.next', keys: ['Alt+Down'], contexts: ['global'], label: 'Next conversation', command: true },
  { action: 'chat.prev', keys: ['Alt+Up'], contexts: ['global'], label: 'Previous conversation', command: true },
  { action: 'account.add', keys: [], contexts: ['global'], label: 'Add an account', command: true },
  { action: 'account.remove', keys: [], contexts: ['global'], label: 'Remove an account', command: true },
  { action: 'account.addOwnKeys', keys: [], contexts: ['global'], label: 'Add a Telegram account with your own API keys', command: true },
  { action: 'rail.all', keys: ['Ctrl+0'], contexts: ['global'], label: 'Show all services', command: true },
  { action: 'rail.whatsapp', keys: ['Ctrl+1'], contexts: ['global'], label: 'Show WhatsApp', command: true },
  { action: 'rail.telegram', keys: ['Ctrl+2'], contexts: ['global'], label: 'Show Telegram', command: true },
  { action: 'rail.next', keys: ['Ctrl+Tab'], contexts: ['global'], label: 'Next account or service', command: true },
  { action: 'rail.prev', keys: ['Ctrl+Shift+Tab'], contexts: ['global'], label: 'Previous account or service', command: true },
  { action: 'window.hide', keys: ['Ctrl+W'], contexts: ['global'], label: 'Close window', command: true },
  { action: 'app.quit', keys: ['Ctrl+Q'], contexts: ['global'], label: 'Quit OmaMessenger', command: true },
  { action: 'helper.retryInstall', keys: ['Ctrl+R'], contexts: ['global'], label: 'Retry installing the helper', command: true },
  { action: 'escape', keys: ['Escape'], contexts: ['global'], label: 'Back' },

  { action: 'cursor.down', keys: ['j', 'Down'], contexts: ['list'], label: 'Next chat' },
  { action: 'cursor.up', keys: ['k', 'Up'], contexts: ['list'], label: 'Previous chat' },
  { action: 'cursor.top', keys: ['g', 'Home'], contexts: ['list'], label: 'First chat' },
  { action: 'cursor.bottom', keys: ['G', 'End'], contexts: ['list'], label: 'Last chat' },
  { action: 'chat.open', keys: ['Enter', 'l', 'o', 'i'], contexts: ['list'], label: 'Open chat', hint: true },
  { action: 'pane.conversation', keys: ['Tab'], contexts: ['list'], label: 'Go to conversation' },
  { action: 'chat.mute', keys: ['m'], contexts: ['list', 'conversation'], label: 'Mute or unmute chat', command: true },
  { action: 'chat.pin', keys: [], contexts: ['list', 'conversation'], label: 'Pin or unpin chat', command: true },
  { action: 'chat.archive', keys: [], contexts: ['list', 'conversation'], label: 'Archive or unarchive chat', command: true },
  { action: 'chat.hide', keys: [], contexts: ['list', 'conversation'], label: 'Hide or unhide chat', command: true },

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
  { action: 'message.retry', keys: ['t'], contexts: ['conversation'], label: 'Retry the highlighted message', command: true },
  { action: 'message.openLink', keys: ['o'], contexts: ['conversation'], label: "Open the highlighted message's link", command: true },
  { action: 'message.goToQuote', keys: ['p'], contexts: ['conversation'], label: 'Go to the replied-to message', command: true },

  { action: 'message.send', keys: ['Enter'], contexts: ['compose'], label: 'Send', hint: true },
  { action: 'compose.newline', keys: ['Shift+Enter', 'Ctrl+J'], contexts: ['compose'], label: 'New line', hint: true },
  { action: 'compose.attach', keys: ['Ctrl+V'], contexts: ['compose'], label: 'Paste a clipboard image as an attachment' },
  { action: 'compose.attachFile', keys: [], contexts: ['conversation', 'compose'], label: 'Attach a file', command: true },
  { action: 'search.accept', keys: ['Enter', 'Down'], contexts: ['search'], label: 'First result', hint: true },

  { action: 'viewer.next', keys: ['Right'], contexts: ['viewer'], label: 'Next photo', hint: true },
  { action: 'viewer.prev', keys: ['Left'], contexts: ['viewer'], label: 'Previous photo', hint: true },
  { action: 'viewer.openExternal', keys: ['o'], contexts: ['viewer'], label: 'Open in your own image viewer', hint: true, command: true },

  { action: 'dialog.down', keys: ['Down', 'Ctrl+J'], contexts: ['dialog'], label: 'Next contact' },
  { action: 'dialog.up', keys: ['Up', 'Ctrl+K'], contexts: ['dialog'], label: 'Previous contact' },
  { action: 'dialog.accept', keys: ['Enter'], contexts: ['dialog'], label: 'Start chat', hint: true },
  { action: 'dialog.nextAccount', keys: ['Ctrl+Tab'], contexts: ['dialog'], label: 'Next account', hint: true },

  { action: 'palette.down', keys: ['Down', 'Ctrl+J', 'Ctrl+N'], contexts: ['palette'], label: 'Next item' },
  { action: 'palette.up', keys: ['Up', 'Ctrl+K', 'Ctrl+P'], contexts: ['palette'], label: 'Previous item' },
  { action: 'palette.accept', keys: ['Enter'], contexts: ['palette'], label: 'Run', hint: true },

  { action: 'reaction.left', keys: ['Left'], contexts: ['reactionPicker'], label: 'Previous emoji' },
  { action: 'reaction.right', keys: ['Right'], contexts: ['reactionPicker'], label: 'Next emoji' },
  { action: 'reaction.accept', keys: ['Enter'], contexts: ['reactionPicker'], label: 'React', hint: true }
];

// match returns the action for a key press in a context, or "". Bindings
// for the context win over global ones.
function match(context, key, modifiers, text) {
  const candidates = BINDINGS.filter((b) => b.contexts.includes(context))
    .concat(BINDINGS.filter((b) => b.contexts.includes('global')));
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
// the context's own first, then global ones.
function bindingsFor(context) {
  const hinted = (b) => b.hint;

  return BINDINGS.filter((b) => b.contexts.includes(context) && hinted(b))
    .concat(BINDINGS.filter((b) => b.contexts.includes('global') && hinted(b)));
}

// display shows a key spec the way people read it: "Ctrl+Slash" as
// "Ctrl+/", "Alt+Down" as "Alt+↓".
function display(spec) {
  return spec.split('+').map((part) => KEY_NAMES[part] ?? part).join('+');
}

// keyFor returns the display text for action's primary key, or "" when
// it has none, for a button label that wants to show its own shortcut
// beside it rather than naming the key a second time by hand.
function keyFor(action) {
  const binding = BINDINGS.find((b) => b.action === action);
  return binding && binding.keys.length > 0 ? display(binding.keys[0]) : '';
}

// commands returns what the command palette offers: every command
// binding, with its first key shown so people learn it, if it has one.
function commands() {
  return BINDINGS
    .filter((b) => b.command)
    .map((b) => ({ action: b.action, label: b.label, keys: b.keys.length > 0 ? display(b.keys[0]) : '' }));
}

// composeHint names the composer's current mode in words: what starts
// writing, or what stops it once the input already has focus. It reads
// its wording off the real compose.focus and escape bindings, the same
// ones the key router already matches, rather than naming a key a
// second time by hand.
function composeHint(writing) {
  const action = writing ? 'escape' : 'compose.focus';
  const key = display(BINDINGS.find((b) => b.action === action).keys[0]);

  return writing ? `Writing · ${key} to stop` : `${key} to write`;
}
