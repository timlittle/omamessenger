// Tests for ui/lib/ScheduledAction.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const ScheduledAction = load('lib/ScheduledAction.js');

test('resolve reports none with nothing queued', () => {
  assert.deepEqual(ScheduledAction.resolve([]), { kind: 'none' });
});

test('resolve reports none for a missing argument', () => {
  assert.deepEqual(ScheduledAction.resolve(undefined), { kind: 'none' });
});

test('resolve reports the one message directly with exactly one queued', () => {
  const message = { id: 's1', conversationId: 'c1', text: 'see you soon', sendAt: 1000 };
  assert.deepEqual(ScheduledAction.resolve([message]), { kind: 'one', message });
});

test('resolve offers a picker over every one with more than one queued', () => {
  const messages = [
    { id: 's1', conversationId: 'c1', text: 'see you soon', sendAt: 1000 },
    { id: 's2', conversationId: 'c1', text: 'happy birthday', sendAt: 2000 }
  ];
  assert.deepEqual(ScheduledAction.resolve(messages), { kind: 'many', messages });
});
