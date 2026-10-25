'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Pin = load('lib/Pin.js');

const ids3 = ['a', 'b', 'c'];

const planMoveCases = [
  ['moving down by one', ids3, 'a', 1, { direction: 'down', count: 1 }],
  ['moving down by two', ids3, 'a', 2, { direction: 'down', count: 2 }],
  ['moving up by one', ids3, 'c', 1, { direction: 'up', count: 1 }],
  ['moving up by two', ids3, 'c', 0, { direction: 'up', count: 2 }],
  ['moving from the middle down', ids3, 'b', 2, { direction: 'down', count: 1 }],
  ['moving from the middle up', ids3, 'b', 0, { direction: 'up', count: 1 }],
  ['dropped back on its own place', ids3, 'b', 1, { direction: 'up', count: 0 }],
  ['dropped on the id not found', ids3, 'z', 1, { direction: 'down', count: 0 }],
  ['dropped at a negative index', ids3, 'a', -1, { direction: 'down', count: 0 }],
  ['dropped past the end', ids3, 'a', 3, { direction: 'down', count: 0 }],
  ['an empty pinned list', [], 'a', 0, { direction: 'down', count: 0 }]
];

for (const [name, pinnedIds, fromId, toIndex, want] of planMoveCases) {
  test(`planMove: ${name}`, () => {
    assert.deepEqual(Pin.planMove(pinnedIds, fromId, toIndex), want);
  });
}

const reorderedCases = [
  ['moves an id down, shifting the ones in between up', ids3, 'a', 2, ['b', 'c', 'a']],
  ['moves an id up, shifting the ones in between down', ids3, 'c', 0, ['c', 'a', 'b']],
  ['moving to its own place changes nothing', ids3, 'b', 1, ['a', 'b', 'c']],
  ['an id not in the list changes nothing', ids3, 'z', 0, ids3],
  ['a negative index changes nothing', ids3, 'a', -1, ids3],
  ['an index past the end changes nothing', ids3, 'a', 3, ids3]
];

for (const [name, pinnedIds, fromId, toIndex, want] of reorderedCases) {
  test(`reordered: ${name}`, () => {
    assert.deepEqual(Pin.reordered(pinnedIds, fromId, toIndex), want);
  });
}

test('reordered: never mutates the list it was given', () => {
  const original = ['a', 'b', 'c'];
  Pin.reordered(original, 'a', 2);
  assert.deepEqual(original, ['a', 'b', 'c']);
});
