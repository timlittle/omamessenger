'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Selection = load('lib/Selection.js');

// Shared id list for the move/edge cases below; a handful of rows use
// their own list to cover the empty, null and single-element cases.
const ids3 = ['c1', 'c2', 'c3'];

const moveCases = [
  ['moves forward by one', ids3, 'c1', 1, 'c2'],
  ['moves forward by two', ids3, 'c1', 2, 'c3'],
  ['moves forward from the middle', ids3, 'c2', 1, 'c3'],
  ['moves backward by one', ids3, 'c3', -1, 'c2'],
  ['moves backward by two', ids3, 'c3', -2, 'c1'],
  ['moves backward from the middle', ids3, 'c2', -1, 'c1'],
  ['clamps forward at the end', ids3, 'c3', 1, 'c3'],
  ['clamps forward past the end', ids3, 'c2', 5, 'c3'],
  ['clamps backward at the start', ids3, 'c1', -1, 'c1'],
  ['clamps backward past the start', ids3, 'c2', -5, 'c1'],
  ['defaults to first id, staying put, when selection is unknown', ids3, 'unknown', 0, 'c1'],
  ['defaults to first id, then moves, when selection is unknown', ids3, 'unknown', 1, 'c2'],
  ['defaults to first id and clamps when selection is unknown', ids3, 'unknown', -1, 'c1'],
  ['defaults to first id when selection is null', ids3, null, 0, 'c1'],
  ['defaults to first id when selection is undefined', ids3, undefined, 1, 'c2'],
  ['defaults to first id when selection is an empty string', ids3, '', 2, 'c3'],
  ['returns empty string for empty ids', [], 'any', 1, ''],
  ['returns empty string for empty ids with no selection', [], null, 0, ''],
  ['returns empty string for null ids', null, 'any', 1, ''],
  ['returns empty string for undefined ids', undefined, 'any', 0, ''],
  ['single element clamps forward', ['c1'], 'c1', 1, 'c1'],
  ['single element clamps backward', ['c1'], 'c1', -1, 'c1'],
  ['single element ignores an unknown selection', ['c1'], 'unknown', 5, 'c1']
];

for (const [name, ids, from, delta, want] of moveCases) {
  test(`move: ${name}`, () => {
    assert.strictEqual(Selection.move(ids, from, delta), want);
  });
}

const edgeCases = [
  ['returns first id for "top"', ids3, 'top', 'c1'],
  ['returns last id for "bottom"', ids3, 'bottom', 'c3'],
  ['returns empty string for empty ids ("top")', [], 'top', ''],
  ['returns empty string for empty ids ("bottom")', [], 'bottom', ''],
  ['returns empty string for null ids', null, 'top', ''],
  ['returns empty string for undefined ids', undefined, 'bottom', ''],
  ['single element is top', ['c1'], 'top', 'c1'],
  ['single element is bottom', ['c1'], 'bottom', 'c1']
];

for (const [name, ids, edge, want] of edgeCases) {
  test(`edge: ${name}`, () => {
    assert.strictEqual(Selection.edge(ids, edge), want);
  });
}

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

const afterRemovalCases = [
  ['selects the next id when the removed one was not last (first)', ['c1', 'c2', 'c3'], 'c1', 'c2'],
  ['selects the next id when the removed one was not last (middle)', ['c1', 'c2', 'c3'], 'c2', 'c3'],
  ['selects the previous id when the removed one was last', ['c1', 'c2', 'c3'], 'c3', 'c2'],
  ['returns empty string when the removed id was the only one', ['c1'], 'c1', ''],
  ['returns empty string when the id is not in the list', ['c1', 'c2'], 'unknown', ''],
  ['returns empty string for null ids', null, 'c1', ''],
  ['returns empty string for undefined ids', undefined, 'c1', '']
];

for (const [name, ids, removedId, want] of afterRemovalCases) {
  test(`afterRemoval: ${name}`, () => {
    assert.strictEqual(Selection.afterRemoval(ids, removedId), want);
  });
}
