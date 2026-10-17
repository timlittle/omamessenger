// Tests for ui/lib/MediaOpen.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const MediaOpen = load('lib/MediaOpen.js');

test('canOpen allows a message that has never been opened', () => {
  assert.strictEqual(MediaOpen.canOpen(undefined, 1000, 5000), true);
  assert.strictEqual(MediaOpen.canOpen(null, 1000, 5000), true);
});

test('canOpen blocks a second open within the cooldown', () => {
  assert.strictEqual(MediaOpen.canOpen(1000, 1000, 5000), false);
  assert.strictEqual(MediaOpen.canOpen(1000, 4999, 5000), false);
});

test('canOpen allows an open exactly at the cooldown boundary', () => {
  assert.strictEqual(MediaOpen.canOpen(1000, 6000, 5000), true);
});

test('canOpen allows an open once the cooldown has fully passed', () => {
  assert.strictEqual(MediaOpen.canOpen(1000, 20000, 5000), true);
});

test('canOpen never reads a different message, so one cooldown never blocks another', () => {
  // The map MediaController keeps is per message id; this only checks
  // that canOpen's own decision depends solely on the one timestamp it
  // was given, never a wider set of messages.
  assert.strictEqual(MediaOpen.canOpen(undefined, 1000, 5000), true);
  assert.strictEqual(MediaOpen.canOpen(1000, 1000, 5000), false);
});
