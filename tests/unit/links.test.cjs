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
