'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const SenderColor = load('lib/SenderColor.js');

test('the same id always picks the same colour index', () => {
  const first = SenderColor.colorIndex('user-42', 8);
  const second = SenderColor.colorIndex('user-42', 8);
  assert.strictEqual(first, second);
});

test('different ids spread across the palette', () => {
  const ids = ['alice', 'bob', 'carol', 'dave', 'erin', 'frank', 'grace', 'heidi', 'ivan', 'judy'];
  const indexes = new Set(ids.map((id) => SenderColor.colorIndex(id, 8)));

  assert.ok(indexes.size >= 4, `expected ids to spread across several colours, got ${indexes.size} distinct`);
});

test('an empty or missing id is safe and stays in range', () => {
  assert.strictEqual(SenderColor.colorIndex('', 8), 0);
  assert.strictEqual(SenderColor.colorIndex(undefined, 8), 0);
  assert.strictEqual(SenderColor.colorIndex(null, 8), 0);
});

test('the index always stays within the requested palette size', () => {
  for (const size of [1, 2, 5, 8, 16]) {
    for (const id of ['a', 'bb', 'long-sender-id-1234', '']) {
      const index = SenderColor.colorIndex(id, size);
      assert.ok(index >= 0 && index < size, `index ${index} out of range for size ${size}`);
    }
  }
});
