// Tests for ui/lib/Highlight.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Highlight = load('lib/Highlight.js');

const ids = ['m1', 'm2', 'm3'];

test('older steps toward the end of the array and stops there', () => {
  assert.strictEqual(Highlight.older(ids, 'm1'), 'm2');
  assert.strictEqual(Highlight.older(ids, 'm2'), 'm3');
  assert.strictEqual(Highlight.older(ids, 'm3'), 'm3');
});

test('newer steps toward the start of the array and stops there', () => {
  assert.strictEqual(Highlight.newer(ids, 'm3'), 'm2');
  assert.strictEqual(Highlight.newer(ids, 'm2'), 'm1');
  assert.strictEqual(Highlight.newer(ids, 'm1'), 'm1');
});

test('older and newer fall back to the first id with no current one', () => {
  assert.strictEqual(Highlight.older([], 'm1'), '');
  assert.strictEqual(Highlight.newer(ids, ''), 'm1');
});

test('atOldest is true only for the last id in the array', () => {
  assert.strictEqual(Highlight.atOldest(ids, 'm3'), true);
  assert.strictEqual(Highlight.atOldest(ids, 'm1'), false);
  assert.strictEqual(Highlight.atOldest([], 'm1'), false);
});

const message = (fields) => Object.assign({ outgoing: false, status: 'received' }, fields);

test('hints names reply and react for a plain message', () => {
  assert.strictEqual(Highlight.hints(message({})), 'r reply · e react');
});

test('hints adds Enter open for a message carrying media', () => {
  const photo = message({ media: { kind: 'photo', width: 10, height: 10 } });
  assert.strictEqual(Highlight.hints(photo), 'r reply · e react · Enter open');
});

test('hints adds t retry only for a failed outgoing message', () => {
  const failed = message({ outgoing: true, status: 'failed' });
  assert.strictEqual(Highlight.hints(failed), 'r reply · e react · t retry');
});

test('hints does not add t retry for an incoming message reported as failed', () => {
  const incoming = message({ outgoing: false, status: 'failed' });
  assert.strictEqual(Highlight.hints(incoming), 'r reply · e react');
});

test('hints combines media and a failed retry', () => {
  const both = message({ outgoing: true, status: 'failed', media: { kind: 'file', fileName: 'a.pdf', size: 1 } });
  assert.strictEqual(Highlight.hints(both), 'r reply · e react · Enter open · t retry');
});
