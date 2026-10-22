// Tests for ui/lib/Toast.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Toast = load('lib/Toast.js');

test('pickText returns the hint while it is active and non-empty', () => {
  assert.strictEqual(Toast.pickText(true, 'Updated to v0.3.5', 'Copied to clipboard'), 'Updated to v0.3.5');
});

test('pickText falls back to the notice once the hint is not active', () => {
  assert.strictEqual(Toast.pickText(false, 'Updated to v0.3.5', 'Copied to clipboard'), 'Copied to clipboard');
});

test('pickText falls back to the notice when the hint text is empty even while active', () => {
  assert.strictEqual(Toast.pickText(true, '', 'Copied to clipboard'), 'Copied to clipboard');
});

test('pickText returns "" when neither the hint nor the notice has anything to say', () => {
  assert.strictEqual(Toast.pickText(false, 'Updated to v0.3.5', ''), '');
});

test('pickText treats a null notice as empty', () => {
  assert.strictEqual(Toast.pickText(false, 'Updated to v0.3.5', null), '');
});

test('displayText pairs non-empty text with a leading checkmark glyph', () => {
  assert.strictEqual(Toast.displayText('Copied to clipboard'), '✓ Copied to clipboard');
});

test('displayText returns "" for empty text, so the toast knows to hide', () => {
  assert.strictEqual(Toast.displayText(''), '');
});

test('displayText returns "" for a blank, whitespace-only string', () => {
  assert.strictEqual(Toast.displayText('   '), '');
});

test('displayText trims surrounding whitespace before pairing the glyph', () => {
  assert.strictEqual(Toast.displayText('  Saved to ~/Downloads/x.png  '), '✓ Saved to ~/Downloads/x.png');
});

test('displayText returns "" for a non-string value rather than throwing', () => {
  assert.strictEqual(Toast.displayText(undefined), '');
});

// Every test above has run once with a deliberately broken expectation
// (asserting the loser won, or the untrimmed/unprefixed text) to see it
// fail before this was written.
