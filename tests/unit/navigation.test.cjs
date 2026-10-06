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

test('escapeAction closes account setup before anything beneath it', () => {
  assert.strictEqual(Navigation.escapeAction({ setupOpen: true, dialogOpen: true }), 'close-setup');
  assert.strictEqual(Navigation.escapeAction({ setupOpen: true, confirmOpen: true }), 'cancel-close');
});
