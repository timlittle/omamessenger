// Regression guard for keyboard parity across ui/lib: every control a
// mouse can click in ui/components must reach the same effect from the
// keyboard, either through a real key binding or through the command
// palette. This spans three files on purpose (Keymap.js owns the
// bindings, Actions.js owns which controller runs them, Palette.js is
// what the command palette searches with), so it sits in its own file
// rather than mirroring just one of them.
//
// QML is not JavaScript, so this file cannot scan ui/components itself;
// MOUSE_ACTIONS below is a maintained map from every interactive mouse
// control found there (built by reading each component for a signal a
// MouseArea or a button fires) to the Keymap.js action that reaches the
// same effect by key. How a new mouse control gets covered:
//   - it runs an action Keymap.js already binds a key to, or offers
//     through the palette: add a row here naming that action, and the
//     first test below checks it for you;
//   - it has no keyboard path yet: add a row with action set to null and
//     a comment naming the gap, the same way the gaps below are recorded.
//     That keeps `make check` green while the gap stays visible in this
//     file and in the keyboard-flow audit, instead of only in someone's
//     memory.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Keymap = load('lib/Keymap.js');

// keyed reports whether action has at least one real key binding.
function keyed(action) {
  const binding = Keymap.BINDINGS.find((b) => b.action === action);
  return !!binding && binding.keys.length > 0;
}

// commanded reports whether action is offered through the command palette.
function commanded(action) {
  const binding = Keymap.BINDINGS.find((b) => b.action === action);
  return !!binding && binding.command === true;
}

// MOUSE_ACTIONS: control -> the Keymap.js action a keyboard user reaches
// the same effect through, or null when there is no keyboard path yet (a
// tracked gap; see the comment on each one and the keyboard-flow audit).
const MOUSE_ACTIONS = {
  'ServiceRail entry click (switch rail filter)': 'rail.all', // one of rail.all/whatsapp/telegram/next/prev covers every entry; see rail.test.cjs
  'ServiceRail "+" button (new chat)': 'chat.new',
  'ServiceRail "?" button (help)': 'palette.commands', // ServiceRail.onHelp runs this very action
  'ConversationRow click (open a chat)': 'chat.open',
  'ListColumn "Add an account" button (empty state)': 'account.add',
  'ListColumn "Show all / Show fewer" button': 'list.showAll',
  'MessageHoverToolbar "+" button (react)': 'message.react',
  'MessageHoverToolbar reply button': 'message.reply',
  'ReactionChips chip click (toggle own reaction)': 'message.react', // the picker's common emoji can reproduce the same toggle
  'MessageDelegate "t to retry" click': 'message.retry',
  'Composer attach button (file picker)': 'compose.attachFile',
  'Composer reply-cancel "x"': 'escape', // Escape's cancel-reply step reaches the same effect, see Navigation.escapeAction
  'Composer remove-attachment "x"': 'escape', // Escape's clear-attachment step
  'PhotoView / FileView bubble click (open)': 'message.open',
  'PhotoViewer close "x"': 'escape',
  'PhotoViewer "Open in image viewer" button': 'viewer.openExternal',
  'LinkPreview click (open the URL)': 'message.openLink',
  'MessageBubbleContent text link click (open the URL)': 'message.openLink',
  'ReplyQuote click (jump to the quoted message)': 'message.goToQuote',
  'PaletteRow click (run a command / open a conversation)': 'palette.accept',
  'NewChatDialog account button click': 'dialog.nextAccount',
  'NewChatContactRow click (open)': 'dialog.accept',
  'CloseConfirm Keep/Quit/Cancel buttons': 'escape', // Escape cancels the same question; Enter accepts the focused choice
  'RemoveAccount account button click (choose which to remove)': null, // reached by Up/Down + Enter, not a Keymap action: see RemoveAccount.qml's KeyNavigation chain
  'RemoveAccount cancel button': 'escape',
  'AccountSetup service-chooser button click': 'account.add',
  'Panel helper-install "Retry" button': 'helper.retryInstall',
};

test('every mouse action with a keyboard counterpart stays reachable by key or the palette', () => {
  for (const [control, action] of Object.entries(MOUSE_ACTIONS)) {
    if (action === null) continue; // a tracked gap, listed in the audit instead of asserted here

    assert.ok(keyed(action) || commanded(action),
      `${control} runs "${action}", which has no key binding and is not offered as a command`);
  }
});

test('every Keymap binding has a real key or is offered as a command, so nothing silently becomes mouse-only', () => {
  for (const binding of Keymap.BINDINGS) {
    assert.ok(binding.keys.length > 0 || binding.command === true,
      `${binding.action} has no key and command is not true`);
  }
});
