// Tests for ui/lib/Navigation.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Navigation = load('lib/Navigation.js');

// keyContext precedence tests: help > dialog > search > compose > conversation > list

test('keyContext returns list when nothing is open', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: false,
    pane: 'list'
  };
  assert.strictEqual(Navigation.keyContext(state), 'list');
});

test('keyContext returns conversation when pane is conversation', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: false,
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.keyContext(state), 'conversation');
});

test('keyContext returns compose when composeFocused is true', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: true,
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.keyContext(state), 'compose');
});

test('keyContext returns search when searchFocused is true', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: true,
    composeFocused: false,
    pane: 'list'
  };
  assert.strictEqual(Navigation.keyContext(state), 'search');
});

test('keyContext returns dialog when dialogOpen is true', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: true,
    searchFocused: false,
    composeFocused: false,
    pane: 'list'
  };
  assert.strictEqual(Navigation.keyContext(state), 'dialog');
});

test('keyContext returns palette when paletteOpen is true', () => {
  const state = {
    paletteOpen: true,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: false,
    pane: 'list'
  };
  assert.strictEqual(Navigation.keyContext(state), 'palette');
});

test('keyContext: palette wins over all other contexts', () => {
  const state = {
    paletteOpen: true,
    dialogOpen: true,
    searchFocused: true,
    composeFocused: true,
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.keyContext(state), 'palette');
});

test('keyContext: dialog wins over search, compose, conversation, list', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: true,
    searchFocused: true,
    composeFocused: true,
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.keyContext(state), 'dialog');
});

test('keyContext: search wins over compose, conversation, list', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: true,
    composeFocused: true,
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.keyContext(state), 'search');
});

test('keyContext: compose wins over conversation, list', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: true,
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.keyContext(state), 'compose');
});

test('keyContext: conversation wins over list', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: false,
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.keyContext(state), 'conversation');
});

// escapeAction: Escape chain

test('escapeAction: close-palette when paletteOpen', () => {
  const state = {
    paletteOpen: true,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: false,
    query: '',
    activeId: 'c1',
    pane: 'list'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'close-palette');
});

test('escapeAction: palette takes precedence over all', () => {
  const state = {
    paletteOpen: true,
    dialogOpen: true,
    searchFocused: true,
    composeFocused: true,
    query: 'test',
    activeId: 'c1',
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'close-palette');
});

test('escapeAction: close-dialog when dialogOpen', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: true,
    searchFocused: false,
    composeFocused: false,
    query: '',
    activeId: 'c1',
    pane: 'list'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'close-dialog');
});

test('escapeAction: dialog takes precedence over search, compose, etc', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: true,
    searchFocused: true,
    composeFocused: true,
    query: 'test',
    activeId: 'c1',
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'close-dialog');
});

test('escapeAction: clear-search when searchFocused with non-empty query', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: true,
    composeFocused: false,
    query: 'test',
    activeId: 'c1',
    pane: 'list'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'clear-search');
});

test('escapeAction: leave-search when searchFocused with empty query', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: true,
    composeFocused: false,
    query: '',
    activeId: 'c1',
    pane: 'list'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'leave-search');
});

test('escapeAction: leave-search when searchFocused with null query', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: true,
    composeFocused: false,
    query: null,
    activeId: 'c1',
    pane: 'list'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'leave-search');
});

test('escapeAction: search takes precedence over compose, conversation, etc', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: true,
    composeFocused: true,
    query: 'test',
    activeId: 'c1',
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'clear-search');
});

test('escapeAction: leave-compose when composeFocused', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: true,
    query: '',
    activeId: 'c1',
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'leave-compose');
});

test('escapeAction: cancel-reply when composeFocused while replying', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: true,
    replying: true,
    query: '',
    activeId: 'c1',
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'cancel-reply');
});

test('escapeAction: clear-attachment when composeFocused with a pending attachment', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: true,
    hasAttachment: true,
    query: '',
    activeId: 'c1',
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'clear-attachment');
});

test('escapeAction: clear-attachment takes precedence over cancel-reply', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: true,
    hasAttachment: true,
    replying: true,
    query: '',
    activeId: 'c1',
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'clear-attachment');
});

test('escapeAction: cancel-reply once the attachment is gone', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: true,
    hasAttachment: false,
    replying: true,
    query: '',
    activeId: 'c1',
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'cancel-reply');
});

test('escapeAction: leave-compose once the attachment is gone and not replying', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: true,
    hasAttachment: false,
    replying: false,
    query: '',
    activeId: 'c1',
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'leave-compose');
});

test('escapeAction: compose takes precedence over conversation, etc', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: true,
    query: 'test',
    activeId: 'c1',
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'leave-compose');
});

test('escapeAction: close-conversation when pane is conversation with activeId', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: false,
    query: '',
    activeId: 'c1',
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'close-conversation');
});

test('escapeAction: close-conversation only when activeId exists', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: false,
    query: '',
    activeId: '',
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'hide-window');
});


test('escapeAction: clear-search when pane is list with non-empty query', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: false,
    query: 'test',
    activeId: '',
    pane: 'list'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'clear-search');
});

test('escapeAction: hide-window when nothing else applies', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: false,
    query: '',
    activeId: '',
    pane: 'list'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'hide-window');
});

test('escapeAction: hide-window when pane is list with empty query', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: false,
    query: '',
    activeId: '',
    pane: 'list'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'hide-window');
});

// Edge cases
test('escapeAction: close-conversation wins over clear-search query', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: false,
    query: 'test',
    activeId: 'c1',
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'close-conversation');
});

test('escapeAction: clear-search when searchFocused and has query', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: true,
    composeFocused: false,
    query: 'search term',
    activeId: 'c1',
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'clear-search');
});

test('escapeAction: leave-search when searchFocused and no query, even with activeId', () => {
  const state = {
    paletteOpen: false,
    dialogOpen: false,
    searchFocused: true,
    composeFocused: false,
    query: '',
    activeId: 'c1',
    pane: 'conversation'
  };
  assert.strictEqual(Navigation.escapeAction(state), 'leave-search');
});

test('escapeAction closes a conversation still open behind the list', () => {
  const state = { pane: 'list', activeId: 'c1', query: 'abc' };

  assert.strictEqual(Navigation.escapeAction(state), 'close-conversation');
});

test('the close question takes keys before anything else', () => {
  const state = { confirmOpen: true, paletteOpen: true, dialogOpen: true, pane: 'conversation', activeId: 'c1' };

  assert.strictEqual(Navigation.keyContext(state), 'confirm');
  assert.strictEqual(Navigation.escapeAction(state), 'cancel-close');
});

test('keyContext puts account setup above everything but the close question', () => {
  assert.strictEqual(Navigation.keyContext({ setupOpen: true, paletteOpen: true, pane: 'list' }), 'setup');
  assert.strictEqual(Navigation.keyContext({ setupOpen: true, confirmOpen: true, pane: 'list' }), 'confirm');
});

test('keyContext answers to setup\'s own step, such as "phone" or "removeAccount", when the caller names one', () => {
  assert.strictEqual(Navigation.keyContext({ setupOpen: true, setupContext: 'phone', paletteOpen: true, pane: 'list' }), 'phone');
  assert.strictEqual(Navigation.keyContext({ setupOpen: true, setupContext: 'removeAccount', pane: 'list' }), 'removeAccount');
  assert.strictEqual(Navigation.keyContext({ setupOpen: true, setupContext: 'phone', confirmOpen: true, pane: 'list' }), 'confirm');
});

test('escapeAction closes account setup before anything beneath it', () => {
  assert.strictEqual(Navigation.escapeAction({ setupOpen: true, dialogOpen: true }), 'close-setup');
  assert.strictEqual(Navigation.escapeAction({ setupOpen: true, confirmOpen: true }), 'cancel-close');
});

test('keyContext puts the photo viewer above the palette but below setup and the close question', () => {
  assert.strictEqual(Navigation.keyContext({ viewerOpen: true, paletteOpen: true, dialogOpen: true, pane: 'conversation' }), 'viewer');
  assert.strictEqual(Navigation.keyContext({ viewerOpen: true, setupOpen: true, pane: 'list' }), 'setup');
  assert.strictEqual(Navigation.keyContext({ viewerOpen: true, confirmOpen: true, pane: 'list' }), 'confirm');
});

test('escapeAction closes the photo viewer before anything beneath it, but not before setup or the close question', () => {
  assert.strictEqual(Navigation.escapeAction({ viewerOpen: true, paletteOpen: true, dialogOpen: true, searchFocused: true }), 'close-viewer');
  assert.strictEqual(Navigation.escapeAction({ viewerOpen: true, setupOpen: true }), 'close-setup');
  assert.strictEqual(Navigation.escapeAction({ viewerOpen: true, confirmOpen: true }), 'cancel-close');
});

test('keyContext puts the health check report above the palette but below the photo viewer, setup and the close question', () => {
  assert.strictEqual(Navigation.keyContext({ doctorOpen: true, paletteOpen: true, dialogOpen: true, pane: 'conversation' }), 'doctor');
  assert.strictEqual(Navigation.keyContext({ doctorOpen: true, viewerOpen: true, pane: 'list' }), 'viewer');
  assert.strictEqual(Navigation.keyContext({ doctorOpen: true, setupOpen: true, pane: 'list' }), 'setup');
  assert.strictEqual(Navigation.keyContext({ doctorOpen: true, confirmOpen: true, pane: 'list' }), 'confirm');
});

test('escapeAction closes the health check report before anything beneath it, but not before the photo viewer, setup or the close question', () => {
  assert.strictEqual(Navigation.escapeAction({ doctorOpen: true, paletteOpen: true, dialogOpen: true, searchFocused: true }), 'close-doctor');
  assert.strictEqual(Navigation.escapeAction({ doctorOpen: true, viewerOpen: true }), 'close-viewer');
  assert.strictEqual(Navigation.escapeAction({ doctorOpen: true, setupOpen: true }), 'close-setup');
  assert.strictEqual(Navigation.escapeAction({ doctorOpen: true, confirmOpen: true }), 'cancel-close');
});

test('keyContext returns reactionPicker when open, above dialog, search and compose', () => {
  const state = { reactionPickerOpen: true, dialogOpen: true, searchFocused: true, composeFocused: true, pane: 'conversation' };
  assert.strictEqual(Navigation.keyContext(state), 'reactionPicker');
});

test('keyContext: the viewer, the palette and setup still win over the reaction picker', () => {
  assert.strictEqual(Navigation.keyContext({ reactionPickerOpen: true, viewerOpen: true, pane: 'list' }), 'viewer');
  assert.strictEqual(Navigation.keyContext({ reactionPickerOpen: true, paletteOpen: true, pane: 'list' }), 'palette');
  assert.strictEqual(Navigation.keyContext({ reactionPickerOpen: true, setupOpen: true, pane: 'list' }), 'setup');
});

test('escapeAction closes the reaction picker before the dialog beneath it', () => {
  assert.strictEqual(Navigation.escapeAction({ reactionPickerOpen: true, dialogOpen: true }), 'close-reaction-picker');
});

test('keyContext returns pollVote when open, above dialog, search and compose', () => {
  const state = { pollVoteOpen: true, dialogOpen: true, searchFocused: true, composeFocused: true, pane: 'conversation' };
  assert.strictEqual(Navigation.keyContext(state), 'pollVote');
});

test('keyContext: the viewer, the palette, setup and the reaction picker still win over poll vote mode', () => {
  assert.strictEqual(Navigation.keyContext({ pollVoteOpen: true, viewerOpen: true, pane: 'list' }), 'viewer');
  assert.strictEqual(Navigation.keyContext({ pollVoteOpen: true, paletteOpen: true, pane: 'list' }), 'palette');
  assert.strictEqual(Navigation.keyContext({ pollVoteOpen: true, setupOpen: true, pane: 'list' }), 'setup');
  assert.strictEqual(Navigation.keyContext({ pollVoteOpen: true, reactionPickerOpen: true, pane: 'list' }), 'reactionPicker');
});

test('keyContext returns deleteConfirm when open, above dialog, search and compose', () => {
  const state = { deleteConfirmOpen: true, dialogOpen: true, searchFocused: true, composeFocused: true, pane: 'conversation' };
  assert.strictEqual(Navigation.keyContext(state), 'deleteConfirm');
});

test('keyContext: the viewer, the palette, setup, the reaction picker and poll vote mode still win over the delete question', () => {
  assert.strictEqual(Navigation.keyContext({ deleteConfirmOpen: true, viewerOpen: true, pane: 'list' }), 'viewer');
  assert.strictEqual(Navigation.keyContext({ deleteConfirmOpen: true, paletteOpen: true, pane: 'list' }), 'palette');
  assert.strictEqual(Navigation.keyContext({ deleteConfirmOpen: true, setupOpen: true, pane: 'list' }), 'setup');
  assert.strictEqual(Navigation.keyContext({ deleteConfirmOpen: true, reactionPickerOpen: true, pane: 'list' }), 'reactionPicker');
  assert.strictEqual(Navigation.keyContext({ deleteConfirmOpen: true, pollVoteOpen: true, pane: 'list' }), 'pollVote');
});

test('escapeAction closes the delete question before the dialog beneath it, but not before the reaction picker', () => {
  assert.strictEqual(Navigation.escapeAction({ deleteConfirmOpen: true, dialogOpen: true }), 'close-delete-confirm');
  assert.strictEqual(Navigation.escapeAction({ deleteConfirmOpen: true, reactionPickerOpen: true }), 'close-reaction-picker');
});

test('escapeAction: leave-unread-view when nothing else is open', () => {
  const state = { pane: 'list', activeId: '', query: '', unreadView: true };

  assert.strictEqual(Navigation.escapeAction(state), 'leave-unread-view');
});

test('escapeAction: closing the conversation opened from the unread view wins over leaving the view', () => {
  const state = { pane: 'conversation', activeId: 'c1', query: '', unreadView: true };

  assert.strictEqual(Navigation.escapeAction(state), 'close-conversation');
});

test('escapeAction: clearing a query typed while the unread view is on wins over leaving the view', () => {
  const state = { pane: 'list', activeId: '', query: 'abc', unreadView: true };

  assert.strictEqual(Navigation.escapeAction(state), 'clear-search');
});

test('escapeAction: hide-window once the unread view is already off', () => {
  const state = { pane: 'list', activeId: '', query: '', unreadView: false };

  assert.strictEqual(Navigation.escapeAction(state), 'hide-window');
});
