// Tests for ui/lib/ListSync.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const ListSync = load('lib/ListSync.js');

const planSyncOpsCases = [
  ['returns empty array for identical lists', ['a', 'b', 'c'], ['a', 'b', 'c'], []],
  ['returns empty array for empty lists', [], [], []],
  ['single insert when converting empty to one item', [], ['a'], [{ op: 'insert', index: 0, id: 'a' }]],
  ['single remove when converting one item to empty', ['a'], [], [{ op: 'remove', index: 0 }]]
];

for (const [name, oldIds, newIds, want] of planSyncOpsCases) {
  test(`planSync ${name}`, () => {
    assert.deepEqual(ListSync.planSync(oldIds, newIds), want);
  });
}

// These two only pin down the net effect (apply(ops) reaches newIds), not
// the exact ops planSync chooses to get there.
const planSyncRoundtripCases = [
  ['full reversal', ['a', 'b', 'c'], ['c', 'b', 'a']],
  ['disjoint lists', ['a', 'b', 'c'], ['x', 'y', 'z']]
];

for (const [name, oldIds, newIds] of planSyncRoundtripCases) {
  test(`planSync ${name}`, () => {
    const ops = ListSync.planSync(oldIds, newIds);
    assert.deepEqual(ListSync.apply(oldIds, ops), newIds);
  });
}

const applyCases = [
  ['insert at start', ['b', 'c'], [{ op: 'insert', index: 0, id: 'a' }], ['a', 'b', 'c']],
  ['insert at end', ['a', 'b'], [{ op: 'insert', index: 2, id: 'c' }], ['a', 'b', 'c']],
  ['remove from start', ['a', 'b', 'c'], [{ op: 'remove', index: 0 }], ['b', 'c']],
  ['remove from end', ['a', 'b', 'c'], [{ op: 'remove', index: 2 }], ['a', 'b']],
  ['move forward', ['a', 'b', 'c'], [{ op: 'move', from: 0, to: 2 }], ['b', 'c', 'a']],
  ['move backward', ['a', 'b', 'c'], [{ op: 'move', from: 2, to: 0 }], ['c', 'a', 'b']],
  ['a sequence of operations', ['a', 'b'], [
    { op: 'insert', index: 1, id: 'x' },
    { op: 'remove', index: 0 },
    { op: 'move', from: 1, to: 0 }
  ], ['b', 'x']]
];

for (const [name, ids, ops, want] of applyCases) {
  test(`apply ${name}`, () => {
    assert.deepEqual(ListSync.apply(ids, ops), want);
  });
}

test('apply returns same array when no ops', () => {
  const ids = ['a', 'b', 'c'];
  const result = ListSync.apply(ids, []);
  assert.deepEqual(result, ids);
  assert.notEqual(result, ids);
});

test('upsertById replaces existing item', () => {
  const list = [
    { id: 'a', value: 1 },
    { id: 'b', value: 2 }
  ];
  const item = { id: 'a', value: 10 };
  const compare = (x, y) => x.id.localeCompare(y.id);
  const result = ListSync.upsertById(list, item, compare);
  assert.strictEqual(result.length, 2);
  assert.deepEqual(result[0], { id: 'a', value: 10 });
});

test('upsertById inserts new item', () => {
  const list = [
    { id: 'a', value: 1 },
    { id: 'b', value: 2 }
  ];
  const item = { id: 'c', value: 3 };
  const compare = (x, y) => x.id.localeCompare(y.id);
  const result = ListSync.upsertById(list, item, compare);
  assert.strictEqual(result.length, 3);
  assert.deepEqual(result, [
    { id: 'a', value: 1 },
    { id: 'b', value: 2 },
    { id: 'c', value: 3 }
  ]);
});

test('upsertById sorts by compare after insert', () => {
  const list = [
    { id: 'a', value: 1 },
    { id: 'c', value: 3 }
  ];
  const item = { id: 'b', value: 2 };
  const compare = (x, y) => x.id.localeCompare(y.id);
  const result = ListSync.upsertById(list, item, compare);
  assert.deepEqual(result, [
    { id: 'a', value: 1 },
    { id: 'b', value: 2 },
    { id: 'c', value: 3 }
  ]);
});

test('upsertById sorts by compare after replace', () => {
  const list = [
    { id: 'a', value: 1 },
    { id: 'b', value: 2 },
    { id: 'c', value: 3 }
  ];
  const item = { id: 'b', value: 20 };
  const compare = (x, y) => x.value - y.value;
  const result = ListSync.upsertById(list, item, compare);
  assert.deepEqual(result, [
    { id: 'a', value: 1 },
    { id: 'c', value: 3 },
    { id: 'b', value: 20 }
  ]);
});

test('upsertById does not mutate input', () => {
  const list = [
    { id: 'a', value: 1 },
    { id: 'b', value: 2 }
  ];
  const item = { id: 'c', value: 3 };
  const compare = (x, y) => x.id.localeCompare(y.id);
  ListSync.upsertById(list, item, compare);
  assert.strictEqual(list.length, 2);
  assert.deepEqual(list, [
    { id: 'a', value: 1 },
    { id: 'b', value: 2 }
  ]);
});

test('upsertById empty list', () => {
  const list = [];
  const item = { id: 'a', value: 1 };
  const compare = (x, y) => x.id.localeCompare(y.id);
  const result = ListSync.upsertById(list, item, compare);
  assert.deepEqual(result, [{ id: 'a', value: 1 }]);
});

// Mulberry32 PRNG for reproducible random tests
function mulberry32(a) {
  return function() {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

test('property test: planSync with 500 random pairs', () => {
  const rng = mulberry32(42);

  for (let trial = 0; trial < 500; trial++) {
    // Generate random old and new lists.
    const oldLen = Math.floor(rng() * 20);
    const newLen = Math.floor(rng() * 20);

    const oldIds = [];
    for (let i = 0; i < oldLen; i++) {
      oldIds.push('id_' + i);
    }

    const newIds = [];
    const usedIds = new Set();
    for (let i = 0; i < newLen; i++) {
      let id;
      if (i < oldLen && rng() > 0.5) {
        // 50% chance: reuse an old id
        id = 'id_' + Math.floor(rng() * oldLen);
      } else {
        // Otherwise: new id
        id = 'id_old_' + i;
      }
      if (!usedIds.has(id)) {
        newIds.push(id);
        usedIds.add(id);
      }
    }

    // Apply the operations and check we get newIds.
    const ops = ListSync.planSync(oldIds, newIds);
    const result = ListSync.apply(oldIds, ops);

    assert.deepEqual(
      result,
      newIds,
      `Trial ${trial}: planSync(${JSON.stringify(oldIds)}, ${JSON.stringify(newIds)}) produced wrong result`
    );
  }
});
