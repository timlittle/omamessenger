// Tests for ui/lib/Reopen.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Reopen = load('lib/Reopen.js');

test('shouldReopen is true when the window was left open, the setting is on, and it has not reopened yet', () => {
  assert.strictEqual(Reopen.shouldReopen(true, true, false), true);
});

test('shouldReopen is false for a fresh install, where nothing was ever stored', () => {
  assert.strictEqual(Reopen.shouldReopen(false, true, false), false);
});

test('shouldReopen is false when the "Reopen after restart" setting is off', () => {
  assert.strictEqual(Reopen.shouldReopen(true, false, false), false);
});

test('shouldReopen is false once this Service has already reopened it once', () => {
  assert.strictEqual(Reopen.shouldReopen(true, true, true), false);
});

test('shouldReopen is false when neither the stored state nor the setting allow it', () => {
  assert.strictEqual(Reopen.shouldReopen(false, false, false), false);
});
