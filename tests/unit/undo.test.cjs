'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Undo = load('lib/Undo.js');

test('describeToggle: archiving names the title and the opposite call', () => {
  const got = Undo.describeToggle('conversations.setArchived', 'archived', 'c1', true, 'Archived', 'Unarchived', 'Nadia');

  assert.deepEqual(got, {
    notice: 'Archived Nadia · u to undo',
    method: 'conversations.setArchived',
    params: { conversationId: 'c1', archived: false }
  });
});

test('describeToggle: unarchiving uses labelOff and restores the field to true', () => {
  const got = Undo.describeToggle('conversations.setArchived', 'archived', 'c1', false, 'Archived', 'Unarchived', 'Nadia');

  assert.deepEqual(got, {
    notice: 'Unarchived Nadia · u to undo',
    method: 'conversations.setArchived',
    params: { conversationId: 'c1', archived: true }
  });
});

test('describeToggle: pinning and hiding read the same way with their own labels and field', () => {
  assert.deepEqual(
    Undo.describeToggle('conversations.setPinned', 'pinned', 'c1', true, 'Pinned', 'Unpinned', 'Bea'),
    { notice: 'Pinned Bea · u to undo', method: 'conversations.setPinned', params: { conversationId: 'c1', pinned: false } }
  );
  assert.deepEqual(
    Undo.describeToggle('conversations.setHidden', 'hidden', 'c1', true, 'Hid', 'Unhid', 'Bea'),
    { notice: 'Hid Bea · u to undo', method: 'conversations.setHidden', params: { conversationId: 'c1', hidden: false } }
  );
});

test('describeToggle: an empty title still shows the label alone', () => {
  const got = Undo.describeToggle('conversations.setMuted', 'muted', 'c1', true, 'Muted', 'Unmuted', '');

  assert.strictEqual(got.notice, 'Muted · u to undo');
});

test('describeBlock: names the title and undoes through contacts.unblock', () => {
  const got = Undo.describeBlock('c1', 'Ben');

  assert.deepEqual(got, {
    notice: 'Blocked Ben · u to undo',
    method: 'contacts.unblock',
    params: { conversationId: 'c1' }
  });
});
