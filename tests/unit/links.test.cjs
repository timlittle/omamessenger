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

// isAllowed is a thin wrapper over Format.spanURL (see this file's own
// header comment), which carries the exhaustive hidden-character and
// http(s)-authority cases (tests/unit/format.test.cjs); these confirm
// the one check every link this UI opens actually goes through sees
// the same refusal, matching the helper's own domain.AllowedLinkURL.

test('isAllowed refuses a backslash anywhere in the URL, the review\'s own paypal.com/evil.example case', () => {
  assert.strictEqual(Links.isAllowed('https://evil.example\\@paypal.com/login'), false);
});

test('isAllowed refuses an http(s) URL with no real "//" authority, which a browser still resolves a host for', () => {
  assert.strictEqual(Links.isAllowed('https:evil.example'), false);
  assert.strictEqual(Links.isAllowed('https:\\\\evil.example'), false);
  assert.strictEqual(Links.isAllowed('https:/\\evil.example'), false);
});

test('isAllowed refuses a URL carrying a hidden Unicode format character', () => {
  assert.strictEqual(Links.isAllowed('https://paypal-login.ru' + String.fromCharCode(65279) + '.paypal.com/login'), false);
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
  assert.deepEqual(Links.resolveClickedLink(href, text, [], spans), { url: 'https://evil.example.com', displayText: 'click here', suspicious: false });
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

  assert.deepEqual(honest, { url, displayText: url, suspicious: false });
  assert.deepEqual(disguise, { url, displayText: 'Barclays here', suspicious: false });
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
  assert.deepEqual(resolved, { url: 'https://good.example', displayText: 'https://good.example', suspicious: false });
  assert.strictEqual(Links.needsConfirmation(resolved.displayText, resolved.url), false);
});

test('previewDisplayText is the url itself only when it is one of the anchors the message\'s own text actually renders', () => {
  assert.equal(Links.previewDisplayText('https://a.example/x', 'see https://a.example/x', [], [], 'A'), 'https://a.example/x');
  assert.equal(Links.previewDisplayText('https://evil.example/login', 'refund: https://www.paypal.com/refund', [], [], 'PayPal'), 'PayPal');
  assert.equal(Links.previewDisplayText('https://evil.example/login', '', [], [], ''), 'Link preview');
});

test('previewDisplayText rejects a lookalike domain that is only a textual prefix of a different link the text really shows', () => {
  // A security review found the old check a plain substring match of
  // the preview's own url against the raw message text: a shorter,
  // different domain that merely reads as a prefix of the text's own
  // unrelated link - paypal.co is a literal substring of
  // paypal.com/signin - said "shown" and skipped the question, even
  // though the two are different domains the sender fully controls
  // independently of one another.
  const shown = Links.previewDisplayText('https://paypal.co', 'go to https://paypal.com/signin', [], [], 'PayPal');
  assert.equal(shown, 'PayPal');
  assert.ok(Links.needsConfirmation(shown, 'https://paypal.co'));
});

test('previewDisplayText recognises the url once it is itself an anchor\'s own visible text, even through a formatting span', () => {
  // The exact-match fix reads the message's own rendered anchors
  // (Format.messageLinkAnchors), not its raw text, so a link whose
  // visible text is the url itself - never typed out literally, but
  // rendered that way by a span - still counts as shown, which the
  // old substring check could not do.
  const spans = [{ kind: 'link', start: 0, end: 'https://a.example'.length, url: 'https://evil.example' }];
  assert.equal(Links.previewDisplayText('https://a.example', 'https://a.example', [], spans, 'A'), 'https://a.example');
});

test('previewDisplayText needs confirming when a url only matches an anchor\'s own href, never what the anchor actually shows as text', () => {
  // A later review found the exact-match fix above still checked only
  // an anchor's own url (its href), not its displayText: a span whose
  // visible text reads as something else entirely - "Click here",
  // say - still counted a preview sharing that same href as "shown",
  // even though the user never saw that url as text anywhere in the
  // message. The card's own title is the sender's to choose, so
  // "shown" must mean the url was itself visible, not merely that
  // some other link in the message happens to share its target.
  const spans = [{ kind: 'link', start: 0, end: 'Click here'.length, url: 'https://evil.example/login' }];
  const shown = Links.previewDisplayText('https://evil.example/login', 'Click here', [], spans, 'My Bank');
  assert.equal(shown, 'My Bank');
  assert.ok(Links.needsConfirmation(shown, 'https://evil.example/login'));
});

test('previewDisplayText still finds the url shown as text behind an unrelated, differently-targeted anchor earlier in the same message', () => {
  // Confirms the fix checks every anchor's own displayText, not just
  // the first one: an honest anchor (its own text is the preview's
  // url) and a disguise anchor (different text, some other target)
  // can both exist in the same message without the disguise hiding
  // the honest one from this check.
  const url = 'https://a.example';
  const spans = [
    { kind: 'link', start: 0, end: url.length, url },
    { kind: 'link', start: url.length + 1, end: url.length + 1 + 'click here'.length, url: 'https://evil.example' }
  ];
  const text = `${url} click here`;
  assert.equal(Links.previewDisplayText(url, text, [], spans, 'A'), url);
});

test('a preview whose URL is not in its message text needs confirming', () => {
  const shown = Links.previewDisplayText('https://evil.example/login', 'refund: https://www.paypal.com/refund', [], [], 'PayPal');
  assert.ok(Links.needsConfirmation(shown, 'https://evil.example/login'));
});

test('previewDisplayText matches a raw ampersand in the message text against a preview url carrying the same raw ampersand', () => {
  // A security review found a plain autolink's own anchors used to
  // record a matched url HTML-escaped ('&' as '&amp;'): a WhatsApp
  // link preview's own url, reported raw by the connector, could then
  // never match even when the two were really the same link, since
  // one side read "&amp;" and the other "&". Format.linkify now
  // decodes the matched text back to raw before recording it (see
  // tests/unit/format.test.cjs's own unescapeHtml cases), so the two
  // now compare equal.
  const url = 'https://example.com?a=1&b=2';
  assert.equal(Links.previewDisplayText(url, `see ${url}`, [], [], 'Example'), url);
});

// sameTarget strips a trailing slash run in linear time, not quadratic.
// Mirrors tests/unit/format.test.cjs's and tests/unit/highlight.test.cjs's
// own adversarial cases: a regex with no start anchor before a
// backtracking quantifier (here /\/+$/) makes the engine retry from
// every position in a long run of the quantified character, costing
// the square of the run's length. A sender's own link can carry a
// slash run this long with no limit of its own at this layer, and
// both a click and the "o" key run this twice (needsConfirmation
// normalizes the visible text and the href), so a single adversarial
// link must not be able to stall the shell.
test('sameTarget strips a long run of trailing slashes in linear time, not quadratic', () => {
  const adversarial = `https://a${'/'.repeat(65000)}b`;
  const start = Date.now();
  const result = Links.sameTarget(adversarial, adversarial);
  const elapsedMs = Date.now() - start;

  assert.strictEqual(result, true);
  assert.ok(elapsedMs < 2000, `sameTarget took ${elapsedMs}ms on an adversarial trailing-slash run, want well under 2000ms`);
});

test('sameTarget still drops a short, ordinary trailing slash', () => {
  assert.strictEqual(Links.sameTarget('https://example.com/page///', 'https://example.com/page'), true);
});

const urlHostCases = [
  ['a plain host', 'https://example.com/path', 'example.com'],
  ['a host with a port', 'https://example.com:8443/path', 'example.com'],
  ['userinfo stripped, keeping only the real host after the last "@"',
    'https://user:pass@example.com/path', 'example.com'],
  // The review's own example: a convincing fake name padded into the
  // userinfo, with the real host - the one this must return - only
  // after the final "@".
  ['a long userinfo padding does not hide the real host after it',
    'https://bank.example.com' + 'a'.repeat(1500) + '@evil.example/login', 'evil.example'],
  ['the last "@" wins when the userinfo itself contains one',
    'https://name@work@example.com/path', 'example.com'],
  ['an IPv4 host', 'https://192.168.0.1/path', '192.168.0.1'],
  ['an IPv4 host with a port', 'https://192.168.0.1:8080/path', '192.168.0.1'],
  ['a bracketed IPv6 host, shown exactly as written', 'https://[2001:db8::1]/path', '[2001:db8::1]'],
  ['a bracketed IPv6 host with a port', 'https://[2001:db8::1]:8080/path', '[2001:db8::1]'],
  ['a punycode host, shown exactly as written, never decoded', 'https://xn--e1aybc.xn--p1ai/path', 'xn--e1aybc.xn--p1ai'],
  ['no path at all', 'https://example.com', 'example.com'],
  ['a query string with no path', 'https://example.com?a=1', 'example.com'],
  ['a fragment with no path', 'https://example.com#section', 'example.com'],
  ['a mailto: link has no "//" authority, so no separate host to show', 'mailto:person@example.com', ''],
  ['empty input', '', ''],
  ['missing entirely', undefined, '']
];

for (const [name, url, want] of urlHostCases) {
  test(`urlHost ${name}`, () => {
    assert.strictEqual(Links.urlHost(url), want);
  });
}

test('urlHost stays linear on a long run of userinfo padding, not quadratic', () => {
  // lastIndexOf and a bounded regex replace are both already linear,
  // but this guards the shape explicitly: a sender's own padding
  // before the real "@" has no length limit of its own at this layer.
  const adversarial = `https://${'a'.repeat(65000)}@evil.example/login`;
  const start = Date.now();
  const host = Links.urlHost(adversarial);
  const elapsedMs = Date.now() - start;

  assert.strictEqual(host, 'evil.example');
  assert.ok(elapsedMs < 2000, `urlHost took ${elapsedMs}ms on a 65000-character userinfo, want well under 2000ms`);
});

// The following four tests reproduce a security review's own named
// examples: a bidi override, a link entity or formatting span cut
// short of what reads as one hostname, and a lookalike host. Each
// invisible or confusable character is built from its code point
// (see tests/unit/no-hidden-characters.test.cjs), never written
// literally into this file.

test('a right-to-left override before a plain autolink is flagged suspicious even though its text and url already match', () => {
  // U+202E before the scheme sits outside the matched text entirely,
  // so url and displayText are identical, exactly as any plain
  // autolink's already are - sameTarget alone would never object.
  const rtlOverride = String.fromCodePoint(0x202e);
  const url = 'https://nigol.sh/moc.lapyap.www';
  const text = rtlOverride + url;

  const anchors = Format.messageLinkAnchors(text, [], []);
  assert.deepEqual(anchors, [{ url, displayText: url, suspicious: true }]);
  assert.strictEqual(Links.needsConfirmation(anchors[0].displayText, anchors[0].url, anchors[0].suspicious), true);
});

test('a Telegram link entity covering only a prefix of what reads as one hostname is flagged suspicious', () => {
  const prefix = 'https://paypal-login.ru';
  const text = prefix + '.paypal.com/login';
  const spans = [{ kind: 'link', start: 0, end: prefix.length, url: prefix }];

  const anchors = Format.messageLinkAnchors(text, [], spans);
  assert.deepEqual(anchors, [{ url: prefix, displayText: prefix, suspicious: true }]);
  assert.strictEqual(Links.needsConfirmation(anchors[0].displayText, anchors[0].url, anchors[0].suspicious), true);
});

test('a spoiler span over only part of a plain autolinked URL is flagged suspicious, even with no visible seam of its own', () => {
  // "spoiler" adds no tag at all (see Format.js's STYLE_TAGS), so the
  // bubble itself shows no seam where the cut happens; this is the one
  // place that still catches it.
  const prefix = 'https://paypal-login.ru';
  const text = prefix + '.paypal.com/login';
  const spans = [{ kind: 'spoiler', start: prefix.length, end: text.length }];

  const anchors = Format.messageLinkAnchors(text, [], spans);
  assert.deepEqual(anchors, [{ url: prefix, displayText: prefix, suspicious: true }]);
});

test('hostForDisplay spells out a lookalike host\'s non-ASCII characters as their own code points', () => {
  const cyrillicA = String.fromCodePoint(0x0430);
  const lookalike = 'p' + cyrillicA + 'ypal.com';

  assert.strictEqual(Links.hostForDisplay(lookalike), 'p\\u{430}ypal.com');
  assert.strictEqual(Links.hostForDisplay('paypal.com'), 'paypal.com');
});
