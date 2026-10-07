'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Selection = load('lib/Selection.js');

test('move: moves forward by delta', () => {
  const ids = ['c1', 'c2', 'c3'];

  assert.strictEqual(Selection.move(ids, 'c1', 1), 'c2');
  assert.strictEqual(Selection.move(ids, 'c1', 2), 'c3');
  assert.strictEqual(Selection.move(ids, 'c2', 1), 'c3');
});

test('move: moves backward by negative delta', () => {
  const ids = ['c1', 'c2', 'c3'];

  assert.strictEqual(Selection.move(ids, 'c3', -1), 'c2');
  assert.strictEqual(Selection.move(ids, 'c3', -2), 'c1');
  assert.strictEqual(Selection.move(ids, 'c2', -1), 'c1');
});

test('move: clamps forward at the end', () => {
  const ids = ['c1', 'c2', 'c3'];

  assert.strictEqual(Selection.move(ids, 'c3', 1), 'c3');
  assert.strictEqual(Selection.move(ids, 'c2', 5), 'c3');
});

test('move: clamps backward at the start', () => {
  const ids = ['c1', 'c2', 'c3'];

  assert.strictEqual(Selection.move(ids, 'c1', -1), 'c1');
  assert.strictEqual(Selection.move(ids, 'c2', -5), 'c1');
});

test('move: defaults to first id when selection is unknown', () => {
  const ids = ['c1', 'c2', 'c3'];

  assert.strictEqual(Selection.move(ids, 'unknown', 0), 'c1');
  assert.strictEqual(Selection.move(ids, 'unknown', 1), 'c2');
  assert.strictEqual(Selection.move(ids, 'unknown', -1), 'c1');
});

test('move: defaults to first id when selection is missing', () => {
  const ids = ['c1', 'c2', 'c3'];

  assert.strictEqual(Selection.move(ids, null, 0), 'c1');
  assert.strictEqual(Selection.move(ids, undefined, 1), 'c2');
  assert.strictEqual(Selection.move(ids, '', 2), 'c3');
});

test('move: returns empty string for empty ids', () => {
  assert.strictEqual(Selection.move([], 'any', 1), '');
  assert.strictEqual(Selection.move([], null, 0), '');
});

test('move: returns empty string for null or undefined ids', () => {
  assert.strictEqual(Selection.move(null, 'any', 1), '');
  assert.strictEqual(Selection.move(undefined, 'any', 0), '');
});

test('move: single element clamps at same position', () => {
  const ids = ['c1'];

  assert.strictEqual(Selection.move(ids, 'c1', 1), 'c1');
  assert.strictEqual(Selection.move(ids, 'c1', -1), 'c1');
  assert.strictEqual(Selection.move(ids, 'unknown', 5), 'c1');
});

test('edge: returns first id for "top"', () => {
  const ids = ['c1', 'c2', 'c3'];

  assert.strictEqual(Selection.edge(ids, 'top'), 'c1');
});

test('edge: returns last id for "bottom"', () => {
  const ids = ['c1', 'c2', 'c3'];

  assert.strictEqual(Selection.edge(ids, 'bottom'), 'c3');
});

test('edge: returns empty string for empty ids', () => {
  assert.strictEqual(Selection.edge([], 'top'), '');
  assert.strictEqual(Selection.edge([], 'bottom'), '');
});

test('edge: returns empty string for null or undefined ids', () => {
  assert.strictEqual(Selection.edge(null, 'top'), '');
  assert.strictEqual(Selection.edge(undefined, 'bottom'), '');
});

test('edge: single element is both top and bottom', () => {
  const ids = ['c1'];

  assert.strictEqual(Selection.edge(ids, 'top'), 'c1');
  assert.strictEqual(Selection.edge(ids, 'bottom'), 'c1');
});

test('nextUnread: finds next unread conversation forward', () => {
  const conversations = [
    { id: 'c1', unread: 0, muted: false },
    { id: 'c2', unread: 1, muted: false },
    { id: 'c3', unread: 0, muted: false }
  ];

  assert.strictEqual(Selection.nextUnread(conversations, 'c1'), 'c2');
});

test('nextUnread: skips the current conversation', () => {
  const conversations = [
    { id: 'c1', unread: 3, muted: false },
    { id: 'c2', unread: 0, muted: false },
    { id: 'c3', unread: 1, muted: false }
  ];

  // Starting from c1 (which has unread), should jump to c3, not stay at c1
  assert.strictEqual(Selection.nextUnread(conversations, 'c1'), 'c3');
});

test('nextUnread: skips muted conversations', () => {
  const conversations = [
    { id: 'c1', unread: 0, muted: false },
    { id: 'c2', unread: 5, muted: true },
    { id: 'c3', unread: 1, muted: false }
  ];

  assert.strictEqual(Selection.nextUnread(conversations, 'c1'), 'c3');
});

test('nextUnread: wraps around from the end', () => {
  const conversations = [
    { id: 'c1', unread: 1, muted: false },
    { id: 'c2', unread: 0, muted: false },
    { id: 'c3', unread: 0, muted: false }
  ];

  assert.strictEqual(Selection.nextUnread(conversations, 'c3'), 'c1');
});

test('nextUnread: returns empty string when no unread', () => {
  const conversations = [
    { id: 'c1', unread: 0, muted: false },
    { id: 'c2', unread: 0, muted: false },
    { id: 'c3', unread: 0, muted: false }
  ];

  assert.strictEqual(Selection.nextUnread(conversations, 'c1'), '');
});

test('nextUnread: returns empty string when all unread are muted', () => {
  const conversations = [
    { id: 'c1', unread: 1, muted: true },
    { id: 'c2', unread: 0, muted: false },
    { id: 'c3', unread: 2, muted: true }
  ];

  assert.strictEqual(Selection.nextUnread(conversations, 'c2'), '');
});

test('nextUnread: starts from beginning when selection is unknown', () => {
  const conversations = [
    { id: 'c1', unread: 1, muted: false },
    { id: 'c2', unread: 0, muted: false },
    { id: 'c3', unread: 0, muted: false }
  ];

  assert.strictEqual(Selection.nextUnread(conversations, 'unknown'), 'c1');
});

test('nextUnread: starts from beginning when selection is missing', () => {
  const conversations = [
    { id: 'c1', unread: 0, muted: false },
    { id: 'c2', unread: 1, muted: false },
    { id: 'c3', unread: 0, muted: false }
  ];

  assert.strictEqual(Selection.nextUnread(conversations, null), 'c2');
  assert.strictEqual(Selection.nextUnread(conversations, undefined), 'c2');
  assert.strictEqual(Selection.nextUnread(conversations, ''), 'c2');
});

test('nextUnread: returns empty string for empty conversations', () => {
  assert.strictEqual(Selection.nextUnread([], 'any'), '');
});

test('nextUnread: returns empty string for null or undefined conversations', () => {
  assert.strictEqual(Selection.nextUnread(null, 'any'), '');
  assert.strictEqual(Selection.nextUnread(undefined, 'any'), '');
});

test('nextUnread: requires unread > 0, not just truthy', () => {
  const conversations = [
    { id: 'c1', unread: 0, muted: false },
    { id: 'c2', unread: false, muted: false },
    { id: 'c3', unread: 1, muted: false }
  ];

  assert.strictEqual(Selection.nextUnread(conversations, 'c1'), 'c3');
});

test('nextUnread: finds unread in complex scenario', () => {
  const conversations = [
    { id: 'c1', unread: 0, muted: false },
    { id: 'c2', unread: 3, muted: true },  // muted, skip
    { id: 'c3', unread: 0, muted: false },
    { id: 'c4', unread: 2, muted: false },  // found
    { id: 'c5', unread: 1, muted: false }
  ];

  assert.strictEqual(Selection.nextUnread(conversations, 'c1'), 'c4');
});

test('nextUnread: wraps and finds unread that was skipped earlier', () => {
  const conversations = [
    { id: 'c1', unread: 1, muted: false },
    { id: 'c2', unread: 0, muted: false },
    { id: 'c3', unread: 0, muted: false },
    { id: 'c4', unread: 0, muted: false }
  ];

  // Starting from c4, should wrap and find c1
  assert.strictEqual(Selection.nextUnread(conversations, 'c4'), 'c1');
});

test('nextUnread: without a selection, finds an unread conversation at the end', () => {
  const conversations = [
    { id: 'c1', unread: 0, muted: false },
    { id: 'c2', unread: 0, muted: false },
    { id: 'c3', unread: 2, muted: false }
  ];

  assert.strictEqual(Selection.nextUnread(conversations, ''), 'c3');
  assert.strictEqual(Selection.nextUnread(conversations, 'unknown'), 'c3');
});

test('nextUnread: the only unread conversation is found again from itself', () => {
  const conversations = [{ id: 'c1', unread: 1, muted: false }, { id: 'c2', unread: 0, muted: false }];

  assert.strictEqual(Selection.nextUnread(conversations, 'c1'), 'c1');
});

test('afterRemoval: selects the next id when the removed one was not last', () => {
  const ids = ['c1', 'c2', 'c3'];

  assert.strictEqual(Selection.afterRemoval(ids, 'c1'), 'c2');
  assert.strictEqual(Selection.afterRemoval(ids, 'c2'), 'c3');
});

test('afterRemoval: selects the previous id when the removed one was last', () => {
  const ids = ['c1', 'c2', 'c3'];

  assert.strictEqual(Selection.afterRemoval(ids, 'c3'), 'c2');
});

test('afterRemoval: returns empty string when the removed id was the only one', () => {
  assert.strictEqual(Selection.afterRemoval(['c1'], 'c1'), '');
});

test('afterRemoval: returns empty string when the id is not in the list', () => {
  assert.strictEqual(Selection.afterRemoval(['c1', 'c2'], 'unknown'), '');
});

test('afterRemoval: returns empty string for null or undefined ids', () => {
  assert.strictEqual(Selection.afterRemoval(null, 'c1'), '');
  assert.strictEqual(Selection.afterRemoval(undefined, 'c1'), '');
});
