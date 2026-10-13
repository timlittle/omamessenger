// Tests for ui/lib/ErrorBanner.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const ErrorBanner = load('lib/ErrorBanner.js');

test('pick returns "" when every source is empty', () => {
  assert.strictEqual(ErrorBanner.pick(['', '', '']), '');
});

test('pick returns "" for an empty list', () => {
  assert.strictEqual(ErrorBanner.pick([]), '');
});

test('pick returns the one non-empty source', () => {
  assert.strictEqual(ErrorBanner.pick(['', 'could not send', '']), 'could not send');
});

test('pick returns the first non-empty source, in priority order', () => {
  assert.strictEqual(ErrorBanner.pick(['first failure', 'second failure']), 'first failure');
});

test('pick skips leading empty sources to reach the first set one', () => {
  assert.strictEqual(ErrorBanner.pick(['', '', 'third failure', 'fourth failure']), 'third failure');
});

// Every test above has run once with a deliberately broken expectation
// (asserting the wrong source won) to see it fail before this was written.
