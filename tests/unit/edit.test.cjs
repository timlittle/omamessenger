// Tests for ui/lib/Edit.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Edit = load('lib/Edit.js');

function msg(overrides) {
  return Object.assign({ id: 'm1', outgoing: true, remoteId: 'r1', media: null }, overrides);
}

test('isEditable accepts an outgoing text message that reached the service', () => {
  assert.strictEqual(Edit.isEditable(msg()), true);
});

test('isEditable rejects an incoming message', () => {
  assert.strictEqual(Edit.isEditable(msg({ outgoing: false })), false);
});

test('isEditable rejects a message still pending, with no remote id yet', () => {
  assert.strictEqual(Edit.isEditable(msg({ remoteId: '' })), false);
});

test('isEditable rejects a message carrying media', () => {
  assert.strictEqual(Edit.isEditable(msg({ media: { kind: 'photo' } })), false);
});

test('isEditable rejects a message carrying media as a JSON string, the ListModel row shape', () => {
  assert.strictEqual(Edit.isEditable(msg({ media: JSON.stringify({ kind: 'photo' }) })), false);
});

test('isEditable rejects null', () => {
  assert.strictEqual(Edit.isEditable(null), false);
});

test('lastEditableId finds the newest editable message, messages kept newest first', () => {
  const messages = [
    msg({ id: 'newest', outgoing: false }),
    msg({ id: 'editable-1' }),
    msg({ id: 'editable-2' })
  ];
  assert.strictEqual(Edit.lastEditableId(messages), 'editable-1');
});

test('lastEditableId returns "" when nothing qualifies', () => {
  const messages = [msg({ outgoing: false }), msg({ remoteId: '' })];
  assert.strictEqual(Edit.lastEditableId(messages), '');
});

test('lastEditableId returns "" for an empty or missing list', () => {
  assert.strictEqual(Edit.lastEditableId([]), '');
  assert.strictEqual(Edit.lastEditableId(undefined), '');
});

test('upMeansEdit is true for an empty composer with something to edit', () => {
  assert.strictEqual(Edit.upMeansEdit('', [msg()]), true);
});

test('upMeansEdit is true for a composer holding only whitespace', () => {
  assert.strictEqual(Edit.upMeansEdit('   \n ', [msg()]), true);
});

test('upMeansEdit is false once the composer holds text, even with something to edit', () => {
  assert.strictEqual(Edit.upMeansEdit('still typing', [msg()]), false);
});

test('upMeansEdit is false for an empty composer with nothing to edit', () => {
  assert.strictEqual(Edit.upMeansEdit('', [msg({ outgoing: false })]), false);
});
