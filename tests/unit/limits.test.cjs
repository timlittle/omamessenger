// Tests for ui/lib/Limits.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { load } = require('./load.cjs');

const Limits = load('lib/Limits.js');

// Every exported name here backs a field's own `maximumLength:` in
// ui/components/ (see field-paste-guard.test.cjs, which checks those
// QML files declare one at all); this only checks the values
// themselves are sane, not how they are used.
const NAMES = ['PALETTE_QUERY', 'SEARCH_QUERY', 'CONTACT_QUERY', 'API_ID', 'API_HASH', 'SETUP_ANSWER', 'ACCOUNT_ALIAS'];

for (const name of NAMES) {
  test(`${name} is a positive whole number`, () => {
    assert.equal(typeof Limits[name], 'number');
    assert.ok(Number.isInteger(Limits[name]) && Limits[name] > 0, `${name} = ${Limits[name]}`);
  });
}
