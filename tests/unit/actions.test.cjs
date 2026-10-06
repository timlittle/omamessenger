// Tests for ui/lib/Actions.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Actions = load('lib/Actions.js');
const Keymap = load('lib/Keymap.js');

test('OWNERS covers exactly the actions in Keymap.BINDINGS', () => {
  const keymapActions = [...new Set(Keymap.BINDINGS.map((b) => b.action))].sort();
  const ownedActions = Object.keys(Actions.OWNERS).sort();

  assert.deepEqual(ownedActions, keymapActions);
});

test('OWNERS only names the five controllers', () => {
  const controllers = new Set(['list', 'conversation', 'dialog', 'window', 'account']);

  for (const [action, controller] of Object.entries(Actions.OWNERS)) {
    assert.ok(controllers.has(controller), `${action} is owned by unknown controller ${controller}`);
  }
});

test('owner: returns the mapped controller for a known action', () => {
  assert.strictEqual(Actions.owner('cursor.down'), 'list');
  assert.strictEqual(Actions.owner('message.send'), 'conversation');
  assert.strictEqual(Actions.owner('chat.new'), 'dialog');
  assert.strictEqual(Actions.owner('escape'), 'window');
});

test('owner: returns "" for an action Keymap does not define', () => {
  assert.strictEqual(Actions.owner('not.a.real.action'), '');
});
