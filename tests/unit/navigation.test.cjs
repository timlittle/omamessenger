// Tests for ui/lib/Navigation.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Navigation = load('lib/Navigation.js');

// Shared defaults for the keyContext/escapeAction cases below; each case
// overrides only the fields its scenario cares about.
const base = {
  paletteOpen: false,
  dialogOpen: false,
  searchFocused: false,
  composeFocused: false,
  replying: false,
  hasAttachment: false,
  query: '',
  activeId: 'c1',
  pane: 'list'
};

// keyContext precedence, palette down to list (the busier layers above
// palette are covered by the compact precedence-ladder tests below).
const keyContextCases = [
  { name: 'returns list when nothing is open', overrides: {}, expected: 'list' },
  { name: 'returns conversation when pane is conversation', overrides: { pane: 'conversation' }, expected: 'conversation' },
  { name: 'returns compose when composeFocused is true', overrides: { composeFocused: true, pane: 'conversation' }, expected: 'compose' },
  { name: 'returns search when searchFocused is true', overrides: { searchFocused: true }, expected: 'search' },
  { name: 'returns dialog when dialogOpen is true', overrides: { dialogOpen: true }, expected: 'dialog' },
  { name: 'returns palette when paletteOpen is true', overrides: { paletteOpen: true }, expected: 'palette' },
  { name: 'palette wins over all other contexts', overrides: { paletteOpen: true, dialogOpen: true, searchFocused: true, composeFocused: true, pane: 'conversation' }, expected: 'palette' },
  { name: 'dialog wins over search, compose, conversation, list', overrides: { dialogOpen: true, searchFocused: true, composeFocused: true, pane: 'conversation' }, expected: 'dialog' },
  { name: 'search wins over compose, conversation, list', overrides: { searchFocused: true, composeFocused: true, pane: 'conversation' }, expected: 'search' },
  { name: 'compose wins over conversation, list', overrides: { composeFocused: true, pane: 'conversation' }, expected: 'compose' },
  { name: 'conversation wins over list', overrides: { pane: 'conversation' }, expected: 'conversation' }
];

for (const c of keyContextCases) {
  test(`keyContext: ${c.name}`, () => {
    assert.strictEqual(Navigation.keyContext({ ...base, ...c.overrides }), c.expected);
  });
}

// escapeAction's Escape chain: undo the innermost thing first, hide the
// window only once there is nothing left to undo.
const escapeActionCases = [
  { name: 'close-palette when paletteOpen', overrides: { paletteOpen: true }, expected: 'close-palette' },
  { name: 'palette takes precedence over all', overrides: { paletteOpen: true, dialogOpen: true, searchFocused: true, composeFocused: true, query: 'test', pane: 'conversation' }, expected: 'close-palette' },
  { name: 'close-dialog when dialogOpen', overrides: { dialogOpen: true }, expected: 'close-dialog' },
  { name: 'dialog takes precedence over search, compose, etc', overrides: { dialogOpen: true, searchFocused: true, composeFocused: true, query: 'test', pane: 'conversation' }, expected: 'close-dialog' },
  { name: 'clear-search when searchFocused with non-empty query', overrides: { searchFocused: true, query: 'test' }, expected: 'clear-search' },
  { name: 'leave-search when searchFocused with empty query', overrides: { searchFocused: true }, expected: 'leave-search' },
  { name: 'leave-search when searchFocused with null query', overrides: { searchFocused: true, query: null }, expected: 'leave-search' },
  { name: 'search takes precedence over compose, conversation, etc', overrides: { searchFocused: true, composeFocused: true, query: 'test', pane: 'conversation' }, expected: 'clear-search' },
  { name: 'leave-compose when composeFocused', overrides: { composeFocused: true, pane: 'conversation' }, expected: 'leave-compose' },
  { name: 'cancel-reply when composeFocused while replying', overrides: { composeFocused: true, replying: true, pane: 'conversation' }, expected: 'cancel-reply' },
  { name: 'clear-attachment when composeFocused with a pending attachment', overrides: { composeFocused: true, hasAttachment: true, pane: 'conversation' }, expected: 'clear-attachment' },
  { name: 'clear-attachment takes precedence over cancel-reply', overrides: { composeFocused: true, hasAttachment: true, replying: true, pane: 'conversation' }, expected: 'clear-attachment' },
  { name: 'cancel-reply once the attachment is gone', overrides: { composeFocused: true, replying: true, pane: 'conversation' }, expected: 'cancel-reply' },
  { name: 'leave-compose once the attachment is gone and not replying', overrides: { composeFocused: true, pane: 'conversation' }, expected: 'leave-compose' },
  { name: 'compose takes precedence over conversation, etc', overrides: { composeFocused: true, query: 'test', pane: 'conversation' }, expected: 'leave-compose' },
  { name: 'close-conversation when pane is conversation with activeId', overrides: { pane: 'conversation' }, expected: 'close-conversation' },
  { name: 'close-conversation only when activeId exists', overrides: { activeId: '', pane: 'conversation' }, expected: 'hide-window' },
  { name: 'clear-search when pane is list with non-empty query', overrides: { query: 'test', activeId: '' }, expected: 'clear-search' },
  { name: 'hide-window when nothing else applies', overrides: { activeId: '' }, expected: 'hide-window' },
  { name: 'hide-window when pane is list with empty query', overrides: { activeId: '' }, expected: 'hide-window' },
  { name: 'close-conversation wins over clear-search query', overrides: { query: 'test', pane: 'conversation' }, expected: 'close-conversation' },
  { name: 'clear-search when searchFocused and has query', overrides: { searchFocused: true, query: 'search term', pane: 'conversation' }, expected: 'clear-search' },
  { name: 'leave-search when searchFocused and no query, even with activeId', overrides: { searchFocused: true, pane: 'conversation' }, expected: 'leave-search' },
  { name: 'closes a conversation still open behind the list', overrides: { query: 'abc' }, expected: 'close-conversation' }
];

for (const c of escapeActionCases) {
  test(`escapeAction: ${c.name}`, () => {
    assert.strictEqual(Navigation.escapeAction({ ...base, ...c.overrides }), c.expected);
  });
}


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

test('keyContext returns archiveConfirm when open, above dialog, search and compose', () => {
  const state = { archiveConfirmOpen: true, dialogOpen: true, searchFocused: true, composeFocused: true, pane: 'conversation' };
  assert.strictEqual(Navigation.keyContext(state), 'archiveConfirm');
});

test('keyContext: the viewer, the palette, setup, the reaction picker and the delete question still win over the archive-all question', () => {
  assert.strictEqual(Navigation.keyContext({ archiveConfirmOpen: true, viewerOpen: true, pane: 'list' }), 'viewer');
  assert.strictEqual(Navigation.keyContext({ archiveConfirmOpen: true, paletteOpen: true, pane: 'list' }), 'palette');
  assert.strictEqual(Navigation.keyContext({ archiveConfirmOpen: true, setupOpen: true, pane: 'list' }), 'setup');
  assert.strictEqual(Navigation.keyContext({ archiveConfirmOpen: true, reactionPickerOpen: true, pane: 'list' }), 'reactionPicker');
  assert.strictEqual(Navigation.keyContext({ archiveConfirmOpen: true, deleteConfirmOpen: true, pane: 'list' }), 'deleteConfirm');
});

test('escapeAction closes the archive-all question before the dialog beneath it, but not before the delete question', () => {
  assert.strictEqual(Navigation.escapeAction({ archiveConfirmOpen: true, dialogOpen: true }), 'cancel-archive-all');
  assert.strictEqual(Navigation.escapeAction({ archiveConfirmOpen: true, deleteConfirmOpen: true }), 'close-delete-confirm');
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

// buildState: Panel's key router and WindowController's Escape chain
// both call this to assemble keyContext/escapeAction's state from
// whichever controllers they pass it, instead of each listing them by
// hand (which is how Panel's own copy once left out the health check
// report).

test('buildState defaults every field when no controller is given', () => {
  assert.deepEqual(Navigation.buildState({}), {
    confirmOpen: false,
    setupOpen: false,
    setupContext: '',
    viewerOpen: false,
    doctorOpen: false,
    paletteOpen: false,
    reactionPickerOpen: false,
    pollVoteOpen: false,
    deleteConfirmOpen: false,
    archiveConfirmOpen: false,
    dialogOpen: false,
    searchFocused: false,
    composeFocused: false,
    hasAttachment: false,
    replying: false,
    pane: 'list',
    activeId: '',
    query: '',
    unreadView: false
  });
});

test('buildState reads every field from the controller that owns it, including doctorOpen', () => {
  const ctx = {
    service: { uiState: { pane: 'conversation', activeId: 'ignored-without-conversationController' } },
    windowController: { confirmingClose: true, doctorOpen: true, paletteOpen: true },
    accountController: { open: false, removing: true, navContext: 'removeAccount' },
    photoViewerController: { viewerOpen: true },
    reactionsController: { pickerOpen: true },
    pollsController: { voteTarget: 'm1' },
    deleteController: { open: true },
    listController: { archiveAllOpen: true, searchFocused: true, query: 'abc', unreadView: true },
    dialogController: { open: true },
    composerController: { composeFocused: true, attachmentPath: '/tmp/a.png', replying: true },
    conversationController: { pane: 'conversation' }
  };

  assert.deepEqual(Navigation.buildState(ctx), {
    confirmOpen: true,
    setupOpen: true,
    setupContext: 'removeAccount',
    viewerOpen: true,
    doctorOpen: true,
    paletteOpen: true,
    reactionPickerOpen: true,
    pollVoteOpen: true,
    deleteConfirmOpen: true,
    archiveConfirmOpen: true,
    dialogOpen: true,
    searchFocused: true,
    composeFocused: true,
    hasAttachment: true,
    replying: true,
    pane: 'conversation',
    activeId: 'ignored-without-conversationController',
    query: 'abc',
    unreadView: true
  });
});

test('buildState falls back to service.uiState.pane when no conversationController is given', () => {
  const ctx = { service: { uiState: { pane: 'conversation', activeId: 'c1' } } };

  const state = Navigation.buildState(ctx);
  assert.strictEqual(state.pane, 'conversation');
  assert.strictEqual(state.activeId, 'c1');
});

test('buildState setupOpen is true while either adding or removing an account', () => {
  assert.strictEqual(Navigation.buildState({ accountController: { open: true, removing: false } }).setupOpen, true);
  assert.strictEqual(Navigation.buildState({ accountController: { open: false, removing: true } }).setupOpen, true);
  assert.strictEqual(Navigation.buildState({ accountController: { open: false, removing: false } }).setupOpen, false);
});
