'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const AccountColor = load('lib/AccountColor.js');

test('the same account id always picks the same colour index', () => {
  const first = AccountColor.colorIndex('account-7', 6);
  const second = AccountColor.colorIndex('account-7', 6);
  assert.strictEqual(first, second);
});

test('different account ids spread across the palette', () => {
  const ids = ['personal', 'work', 'telegram-main', 'whatsapp-family', 'alt', 'backup'];
  const indexes = new Set(ids.map((id) => AccountColor.colorIndex(id, 6)));

  assert.ok(indexes.size >= 3, `expected account ids to spread across several colours, got ${indexes.size} distinct`);
});

test('an empty or missing account id is safe and stays in range', () => {
  assert.strictEqual(AccountColor.colorIndex('', 6), 0);
  assert.strictEqual(AccountColor.colorIndex(undefined, 6), 0);
  assert.strictEqual(AccountColor.colorIndex(null, 6), 0);
});

test('the index always stays within the requested palette size', () => {
  for (const size of [1, 2, 4, 6, 12]) {
    for (const id of ['a', 'bb', 'account-id-1234', '']) {
      const index = AccountColor.colorIndex(id, size);
      assert.ok(index >= 0 && index < size, `index ${index} out of range for size ${size}`);
    }
  }
});
