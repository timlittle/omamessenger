// Tests for ui/lib/Links.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Links = load('lib/Links.js');
const Format = load('lib/Format.js');

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
  assert.strictEqual(Links.fileURL('/home/user/Downloads/report.pdf'), 'file:///home/user/Downloads/report.pdf');
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

test('needsConfirmation is false when the visible text already is the url', () => {
  assert.strictEqual(Links.needsConfirmation('https://example.com/page', 'https://example.com/page'), false);
});

test('needsConfirmation ignores a scheme difference between http and https', () => {
  assert.strictEqual(Links.needsConfirmation('http://example.com/page', 'https://example.com/page'), false);
});

test('needsConfirmation ignores a leading "www." on either side', () => {
  assert.strictEqual(Links.needsConfirmation('https://www.example.com', 'https://example.com'), false);
  assert.strictEqual(Links.needsConfirmation('https://example.com', 'https://www.example.com'), false);
});

test('needsConfirmation ignores a trailing slash difference', () => {
  assert.strictEqual(Links.needsConfirmation('https://example.com/page/', 'https://example.com/page'), false);
});

test('needsConfirmation is true when the visible text names a different destination', () => {
  assert.strictEqual(Links.needsConfirmation('click here', 'https://evil.example.com'), true);
  assert.strictEqual(Links.needsConfirmation('https://good.example.com', 'https://evil.example.com'), true);
});

test('needsConfirmation is true for a mailto: link carrying a query string, even with matching text', () => {
  const url = 'mailto:a@example.com?body=hello';
  assert.strictEqual(Links.needsConfirmation(url, url), true);
});

test('needsConfirmation is false for a plain mailto: link with no query string', () => {
  const url = 'mailto:a@example.com';
  assert.strictEqual(Links.needsConfirmation(url, url), false);
});

test('resolveClickedLink returns a span\'s own visible text and url for its own anchor', () => {
  const text = 'see click here for more';
  const spans = [{ kind: 'link', start: 4, end: 14, url: 'https://evil.example.com' }];
  const href = Format.linkHref(0);
  assert.deepEqual(Links.resolveClickedLink(href, text, [], spans), { url: 'https://evil.example.com', displayText: 'click here' });
});

test('resolveClickedLink tells apart two spans that carry the same url with different visible text', () => {
  // The exact shape of the bypass this guards: a sender gives an
  // honest-looking span (its own text is the url itself, so it would
  // never need confirming on its own) and a disguise span the same
  // url, different text. Matching a click by url alone, rather than by
  // which anchor was actually touched, let a click on the disguise
  // report the honest span's text instead and skip the question.
  const url = 'https://evil.example/login';
  const honestText = `see ${url} `;
  const text = `${honestText}or go to Barclays here for your refund`;
  const spans = [
    { kind: 'link', start: 4, end: 4 + url.length, url },
    { kind: 'link', start: text.indexOf('Barclays here'), end: text.indexOf('Barclays here') + 'Barclays here'.length, url }
  ];

  const honest = Links.resolveClickedLink(Format.linkHref(0), text, [], spans);
  const disguise = Links.resolveClickedLink(Format.linkHref(1), text, [], spans);

  assert.deepEqual(honest, { url, displayText: url });
  assert.deepEqual(disguise, { url, displayText: 'Barclays here' });
  assert.strictEqual(Links.needsConfirmation(honest.displayText, honest.url), false);
  assert.strictEqual(Links.needsConfirmation(disguise.displayText, disguise.url), true);
});

test('resolveClickedLink fails closed for an href naming no anchor at all, asking when its scheme is allowed', () => {
  const text = 'see https://good.example for more';
  const resolved = Links.resolveClickedLink('https://sneaky.example/elsewhere', text, [], []);
  assert.deepEqual(resolved, { url: 'https://sneaky.example/elsewhere', displayText: '' });
  assert.strictEqual(Links.needsConfirmation(resolved.displayText, resolved.url), true);
});

test('resolveClickedLink refuses an href naming no anchor whose scheme is not allowed at all', () => {
  assert.strictEqual(Links.resolveClickedLink('javascript:alert(1)', 'plain text', [], []), null);
});

test('resolveClickedLink refuses an href naming an index past the end of the anchors it rendered', () => {
  const text = 'see https://good.example for more';
  assert.strictEqual(Links.resolveClickedLink(Format.linkHref(5), text, [], []), null);
});

test('resolveClickedLink needs no confirmation for a plain auto-linked url', () => {
  const text = 'see https://good.example for more';
  const resolved = Links.resolveClickedLink(Format.linkHref(0), text, [], []);
  assert.deepEqual(resolved, { url: 'https://good.example', displayText: 'https://good.example' });
  assert.strictEqual(Links.needsConfirmation(resolved.displayText, resolved.url), false);
});

test('previewDisplayText is the url itself only when the message text shows it', () => {
  assert.equal(Links.previewDisplayText('https://a.example/x', 'see https://a.example/x', 'A'), 'https://a.example/x');
  assert.equal(Links.previewDisplayText('https://evil.example/login', 'refund: https://www.paypal.com/refund', 'PayPal'), 'PayPal');
  assert.equal(Links.previewDisplayText('https://evil.example/login', '', ''), 'Link preview');
});

test('a preview whose URL is not in its message text needs confirming', () => {
  const shown = Links.previewDisplayText('https://evil.example/login', 'refund: https://www.paypal.com/refund', 'PayPal');
  assert.ok(Links.needsConfirmation(shown, 'https://evil.example/login'));
});
