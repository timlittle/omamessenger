.pragma library

// Key bindings for the OmaMessenger UI: global and context-specific shortcuts.
// Matches key events to actions and provides help text and footer hints.

// KEY holds the Qt key codes the bindings use; they are defined here
// because the node tests have no Qt. Letters use their uppercase ASCII code
// and digits their ASCII code, as Qt does.
var KEY = {
  Escape: 0x01000000, Tab: 0x01000001, Backtab: 0x01000002,
  Return: 0x01000004, Enter: 0x01000005, Home: 0x01000010,
  End: 0x01000011, Up: 0x01000013, Down: 0x01000015,
  PageUp: 0x01000016, PageDown: 0x01000017, F1: 0x01000030
};

// MOD holds the Qt keyboard modifier masks.
var MOD = { Shift: 0x02000000, Ctrl: 0x04000000, Alt: 0x08000000 };

// BINDINGS define every key action, grouped by context and global. Each binding
// has an action name, key specs (how to press it), which contexts it applies in
// (or ["global"]), a human label for hints and help, whether it's worth showing
// in footer hints, and whether it's only active in demo mode.
var BINDINGS = [
  // Global bindings: Ctrl, Alt, F-keys, or Escape never steal typing.
  { action: 'search.focus', keys: ['Ctrl+K'], contexts: ['global'], label: 'Search', hint: true },
  { action: 'chat.new', keys: ['Ctrl+N'], contexts: ['global'], label: 'New chat', hint: true },
  { action: 'rail.all', keys: ['Ctrl+0'], contexts: ['global'], label: 'All services' },
  { action: 'rail.whatsapp', keys: ['Ctrl+1'], contexts: ['global'], label: 'WhatsApp' },
  { action: 'rail.telegram', keys: ['Ctrl+2'], contexts: ['global'], label: 'Telegram' },
  { action: 'rail.next', keys: ['Ctrl+Tab'], contexts: ['global'], label: 'Next service' },
  { action: 'rail.prev', keys: ['Ctrl+Shift+Tab'], contexts: ['global'], label: 'Previous service' },
  { action: 'help.toggle', keys: ['F1'], contexts: ['global'], label: 'Shortcuts', hint: true },
  { action: 'demo.inject', keys: ['Ctrl+Shift+D'], contexts: ['global'], label: 'Add demo messages', demoOnly: true },
  { action: 'escape', keys: ['Escape'], contexts: ['global'], label: 'Back' },

  // List context: conversation list.
  { action: 'cursor.down', keys: ['j', 'Down'], contexts: ['list'], label: 'Next chat' },
  { action: 'cursor.up', keys: ['k', 'Up'], contexts: ['list'], label: 'Previous chat' },
  { action: 'cursor.top', keys: ['g', 'Home'], contexts: ['list'], label: 'First chat' },
  { action: 'cursor.bottom', keys: ['G', 'End'], contexts: ['list'], label: 'Last chat' },
  { action: 'chat.open', keys: ['Enter', 'l', 'o', 'i'], contexts: ['list'], label: 'Open chat', hint: true },
  { action: 'pane.conversation', keys: ['Tab'], contexts: ['list'], label: 'Open chat' },
  { action: 'chat.mute', keys: ['m'], contexts: ['list'], label: 'Mute chat' },
  { action: 'unread.next', keys: ['u'], contexts: ['list'], label: 'Next unread' },
  { action: 'search.focus', keys: ['/'], contexts: ['list'], label: 'Search' },
  { action: 'help.toggle', keys: ['?'], contexts: ['list'], label: 'Shortcuts' },
  { action: 'window.hide', keys: ['q'], contexts: ['list'], label: 'Hide' },

  // Conversation context: open message thread.
  { action: 'scroll.down', keys: ['j', 'Down'], contexts: ['conversation'], label: 'Scroll down' },
  { action: 'scroll.up', keys: ['k', 'Up'], contexts: ['conversation'], label: 'Scroll up' },
  { action: 'scroll.pageDown', keys: ['Ctrl+D', 'PageDown'], contexts: ['conversation'], label: 'Page down' },
  { action: 'scroll.pageUp', keys: ['Ctrl+U', 'PageUp'], contexts: ['conversation'], label: 'Page up' },
  { action: 'scroll.newest', keys: ['G', 'End'], contexts: ['conversation'], label: 'Newest message' },
  { action: 'scroll.oldest', keys: ['g', 'Home'], contexts: ['conversation'], label: 'Oldest message' },
  { action: 'compose.focus', keys: ['i', 'a', 'Enter'], contexts: ['conversation'], label: 'Compose', hint: true },
  { action: 'chat.next', keys: ['J'], contexts: ['conversation'], label: 'Next chat' },
  { action: 'chat.prev', keys: ['K'], contexts: ['conversation'], label: 'Previous chat' },
  { action: 'pane.list', keys: ['h', 'Tab'], contexts: ['conversation'], label: 'Show list' },
  { action: 'message.retry', keys: ['r'], contexts: ['conversation'], label: 'Retry message' },
  { action: 'chat.mute', keys: ['m'], contexts: ['conversation'], label: 'Mute chat' },
  { action: 'unread.next', keys: ['u'], contexts: ['conversation'], label: 'Next unread' },
  { action: 'search.focus', keys: ['/'], contexts: ['conversation'], label: 'Search' },
  { action: 'help.toggle', keys: ['?'], contexts: ['conversation'], label: 'Shortcuts' },
  { action: 'window.hide', keys: ['q'], contexts: ['conversation'], label: 'Hide' },

  // Compose context: text input in conversation.
  { action: 'message.send', keys: ['Enter'], contexts: ['compose'], label: 'Send', hint: true },

  // Search context: search input.
  { action: 'search.accept', keys: ['Enter', 'Down'], contexts: ['search'], label: 'First result', hint: true },

  // Dialog context: modal dialogs.
  { action: 'dialog.down', keys: ['Down', 'Ctrl+J'], contexts: ['dialog'], label: 'Next option' },
  { action: 'dialog.up', keys: ['Up', 'Ctrl+K'], contexts: ['dialog'], label: 'Previous option' },
  { action: 'dialog.accept', keys: ['Enter'], contexts: ['dialog'], label: 'Select', hint: true },
  { action: 'dialog.nextAccount', keys: ['Ctrl+Tab'], contexts: ['dialog'], label: 'Next account' },

  // Help context: help sheet.
  { action: 'help.toggle', keys: ['?'], contexts: ['help'], label: 'Close help' },
  { action: 'help.close', keys: ['q'], contexts: ['help'], label: 'Close help' }
];

// match returns the action for a key press in a context, or "". Bindings
// for the context win over global ones.
function match(context, key, modifiers, text, demo) {
  const candidates = BINDINGS.filter((b) => b.contexts.includes(context))
    .concat(BINDINGS.filter((b) => b.contexts.includes('global')));
  const hit = candidates.find((b) => (!b.demoOnly || demo)
    && b.keys.some((spec) => specMatches(parseSpec(spec), key, modifiers, text)));

  return hit ? hit.action : '';
}

// parseSpec turns a key spec such as "Ctrl+K", "G" or "?" into what a key
// press must look like. A lone uppercase letter means Shift; "/" and "?"
// match the typed text, because their key codes differ between layouts.
function parseSpec(spec) {
  const parts = spec.split('+');
  const name = parts.pop();
  const lone = parts.length === 0;

  if (name === '/' || name === '?') {
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

// HELP_CONTEXTS orders the help sheet's sections.
var HELP_CONTEXTS = ['global', 'list', 'conversation', 'compose', 'search', 'dialog', 'help'];

// helpSections groups the bindings by context for the help sheet.
function helpSections() {
  return HELP_CONTEXTS
    .map((context) => ({
      title: context.charAt(0).toUpperCase() + context.slice(1),
      rows: BINDINGS.filter((b) => b.contexts.includes(context))
        .map((b) => ({ keys: b.keys.join(', '), label: b.label }))
    }))
    .filter((section) => section.rows.length > 0);
}
