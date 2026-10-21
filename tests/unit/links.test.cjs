// Tests for ui/lib/Links.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Links = load('lib/Links.js');

test('isAllowed accepts http, https, mailto and tg', () => {
  assert.strictEqual(Links.isAllowed('http://example.com'), true);
  assert.strictEqual(Links.isAllowed('https://example.com'), true);
  assert.strictEqual(Links.isAllowed('mailto:a@example.com'), true);
  assert.strictEqual(Links.isAllowed('tg://resolve?domain=x'), true);
});

test('isAllowed rejects every other scheme and anything with none at all', () => {
  assert.strictEqual(Links.isAllowed('javascript:alert(1)'), false);
  assert.strictEqual(Links.isAllowed('file:///etc/passwd'), false);
  assert.strictEqual(Links.isAllowed('data:text/html,<script>'), false);
  assert.strictEqual(Links.isAllowed('no scheme at all'), false);
  assert.strictEqual(Links.isAllowed(''), false);
  assert.strictEqual(Links.isAllowed(null), false);
  assert.strictEqual(Links.isAllowed(undefined), false);
});

test('fileURL percent-encodes a plain path with no special characters', () => {
  assert.strictEqual(Links.fileURL('/home/tim/Downloads/report.pdf'), 'file:///home/tim/Downloads/report.pdf');
});

test('fileURL encodes a hash, so it is never read as a URL fragment', () => {
  assert.strictEqual(Links.fileURL('/tmp/invoice #2.pdf'), 'file:///tmp/invoice%20%232.pdf');
});

test('fileURL encodes a question mark, so it is never read as a query string', () => {
  assert.strictEqual(Links.fileURL('/tmp/what?.txt'), 'file:///tmp/what%3F.txt');
});

test('fileURL encodes a literal percent sign, so it is never read as an escape', () => {
  assert.strictEqual(Links.fileURL('/tmp/100% done.txt'), 'file:///tmp/100%25%20done.txt');
});

test('fileURL encodes spaces', () => {
  assert.strictEqual(Links.fileURL('/tmp/my file.jpg'), 'file:///tmp/my%20file.jpg');
});

test('fileURL encodes non-ASCII text', () => {
  assert.strictEqual(Links.fileURL('/tmp/résumé 日本語.pdf'), 'file:///tmp/r%C3%A9sum%C3%A9%20%E6%97%A5%E6%9C%AC%E8%AA%9E.pdf');
});

test('fileURL leaves path separators alone, encoding each segment on its own', () => {
  assert.strictEqual(Links.fileURL('/a b/c d/e.txt'), 'file:///a%20b/c%20d/e.txt');
});

test('fileURL returns an empty string for an empty or missing path', () => {
  assert.strictEqual(Links.fileURL(''), '');
  assert.strictEqual(Links.fileURL(null), '');
  assert.strictEqual(Links.fileURL(undefined), '');
});
