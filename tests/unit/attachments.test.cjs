// Tests for ui/lib/Attachments.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Attachments = load('lib/Attachments.js');

test('attachmentLimitHint shows a round number as "Up to <size>"', () => {
  assert.strictEqual(Attachments.attachmentLimitHint(2 * 1024 * 1024 * 1024), 'Up to 2.0 GB');
  assert.strictEqual(Attachments.attachmentLimitHint(16 * 1024 * 1024), 'Up to 16.0 MB');
});

test('attachmentLimitHint shows nothing for an unknown limit', () => {
  assert.strictEqual(Attachments.attachmentLimitHint(0), '');
  assert.strictEqual(Attachments.attachmentLimitHint(null), '');
  assert.strictEqual(Attachments.attachmentLimitHint(undefined), '');
});

test('isNearOrOverLimit is false for an ordinary small attachment', () => {
  const limit = 2 * 1024 * 1024 * 1024;
  assert.strictEqual(Attachments.isNearOrOverLimit(5 * 1024, limit), false);
  assert.strictEqual(Attachments.isNearOrOverLimit(limit / 2, limit), false);
});

test('isNearOrOverLimit is true once size reaches 90% of limit, and once it exceeds it', () => {
  const limit = 1000;
  assert.strictEqual(Attachments.isNearOrOverLimit(899, limit), false);
  assert.strictEqual(Attachments.isNearOrOverLimit(900, limit), true);
  assert.strictEqual(Attachments.isNearOrOverLimit(1500, limit), true);
});

test('isNearOrOverLimit is false with no known size or limit', () => {
  assert.strictEqual(Attachments.isNearOrOverLimit(0, 1000), false);
  assert.strictEqual(Attachments.isNearOrOverLimit(500, 0), false);
  assert.strictEqual(Attachments.isNearOrOverLimit(null, 1000), false);
  assert.strictEqual(Attachments.isNearOrOverLimit(500, null), false);
});

test('attachmentSizeLabel shows nothing for an attachment well under its limit', () => {
  assert.strictEqual(Attachments.attachmentSizeLabel(1024, 2 * 1024 * 1024 * 1024), '');
});

test('attachmentSizeLabel shows size of limit once near it', () => {
  const limit = 1000;
  assert.strictEqual(Attachments.attachmentSizeLabel(950, limit), '950 B of 1000 B');
});

test('attachmentSizeLabel names the limit once over it, rather than just the size', () => {
  const limit = 1000;
  assert.strictEqual(Attachments.attachmentSizeLabel(1500, limit), '1.5 KB — over the 1000 B limit');
});
