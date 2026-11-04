// A regression sweep against catastrophic regex backtracking (ReDoS)
// across every ui/lib function a security review found, or confirmed,
// reads a regex over sender-controlled text with no length limit of
// its own at this layer (a message's own text, a link's own URL, the
// visible text naming it). A security review found one real instance,
// ui/lib/Links.js's old normalizeTarget: a regex with no start anchor
// before a backtracking quantifier (/\/+$/) makes the engine retry the
// backtrack from every position in a long run of the quantified
// character, costing the square of the run's length rather than its
// length - exactly the shape already fixed once, for a different
// regex, in Format.splitTrailingPunctuation (which Highlight.links
// now reaches too, through Format.messageLinkAnchors' own linkify
// call, rather than keeping a second copy of its own), and missed
// when Links.js's own copy was added later. Each case below feeds the
// function a run of
// 65000 of its own regex's own boundary character - the character
// that character class or quantifier is built to consume - and checks
// it still answers well under a generous bound, so a future change
// that reintroduces this shape, in this function or a new one, fails
// the build rather than shipping a link click or an open key that can
// stall the whole shell's GUI thread for seconds.
//
// tests/unit/format.test.cjs and tests/unit/highlight.test.cjs already
// cover Format.linkify and Highlight.links in detail, including the
// exact adversarial case that found the original trailing-punctuation
// shape; they are not repeated here, only confirmed still linear as
// part of this same sweep.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { load } = require('./load.cjs');

const Links = load('lib/Links.js');
const Format = load('lib/Format.js');
const Highlight = load('lib/Highlight.js');
const Snooze = load('lib/Snooze.js');

// TIME_BOUND_MS is the generous ceiling every case asserts under: fast
// enough that a real linear implementation is nowhere near it, but
// nothing a quadratic one at this input size could ever land inside
// either (see this file's own header for the measured contrast).
const TIME_BOUND_MS = 2000;

// timed runs fn and returns [result, elapsedMs], the same measurement
// every case below needs.
function timed(fn) {
  const start = Date.now();
  const result = fn();
  return [result, Date.now() - start];
}

const message = (fields) => Object.assign({ outgoing: false, status: 'received' }, fields);

test('Links.sameTarget strips a long trailing slash run in linear time', () => {
  const adversarial = `https://a${'/'.repeat(65000)}b`;
  const [result, elapsedMs] = timed(() => Links.sameTarget(adversarial, adversarial));

  assert.strictEqual(result, true);
  assert.ok(elapsedMs < TIME_BOUND_MS, `Links.sameTarget took ${elapsedMs}ms on a 65000-slash run, want well under ${TIME_BOUND_MS}ms`);
});

test('Format.linkify stays linear on a long run of trailing punctuation', () => {
  const adversarial = `http://${'!'.repeat(65000)}A`;
  const [, elapsedMs] = timed(() => Format.linkify(Format.escapeHtml(adversarial), null, []));

  assert.ok(elapsedMs < TIME_BOUND_MS, `Format.linkify took ${elapsedMs}ms on a 65000-"!" run, want well under ${TIME_BOUND_MS}ms`);
});

test('Highlight.links stays linear on a long run of trailing punctuation', () => {
  const adversarial = `http://${'!'.repeat(65000)}A`;
  const [, elapsedMs] = timed(() => Highlight.links(message({ text: adversarial })));

  assert.ok(elapsedMs < TIME_BOUND_MS, `Highlight.links took ${elapsedMs}ms on a 65000-"!" run, want well under ${TIME_BOUND_MS}ms`);
});

// Format.highlight's own entity-aware split, /(&[^;]*;)/, excludes ';'
// from its quantified class, so a run of '&' with no ';' anywhere
// after it is the shape that would force it to backtrack that class
// all the way to zero, at every '&' in the run, if it ever lost that
// exclusion.
test('Format.highlight stays linear on a long run of "&" with no terminator', () => {
  const adversarial = '&'.repeat(65000);
  const [, elapsedMs] = timed(() => Format.highlight(adversarial, 'x'));

  assert.ok(elapsedMs < TIME_BOUND_MS, `Format.highlight took ${elapsedMs}ms on a 65000-"&" run, want well under ${TIME_BOUND_MS}ms`);
});

// Format.initials' own aside-stripping regex, /\([^)]*\)/g, excludes
// ')' from its quantified class the same way; a run of "(" with no
// ")" anywhere after it is this regex's own equivalent adversarial
// shape.
test('Format.initials stays linear on a long run of "(" with no terminator', () => {
  const adversarial = `Sam ${'('.repeat(65000)}`;
  const [, elapsedMs] = timed(() => Format.initials(adversarial));

  assert.ok(elapsedMs < TIME_BOUND_MS, `Format.initials took ${elapsedMs}ms on a 65000-"(" run, want well under ${TIME_BOUND_MS}ms`);
});

// Snooze.parse's own duration, clock and weekday-clock regexes are
// all anchored at both ends (^...$), so a non-matching run never has
// more than one position to try in the first place; this is the
// typed-by-the-user path (a snooze or "send later" prompt), never a
// sender's own text, but it is swept here too since it does read a
// regex over text with no length limit of its own.
// Format.messageHtml's own mention path (mentionHtml) calls
// continuesURLToken-backed lookups twice per mention, against the
// full message text, at every mention boundary. A security review's
// exact adversarial shape: a sender mentions themselves (a push name
// with no spaces) 64 times back to back, then about 64,000 characters
// with no whitespace - one unbroken run every boundary would have had
// to rescan to its own end, under the old per-call forward scan. On
// this machine the old per-call scan measured at 500-600ms for this
// exact input and a fixed, single-sweep lookup measures at under 10ms,
// so MENTION_SCAN_TIME_BOUND_MS - tighter than the shared
// TIME_BOUND_MS other cases in this file use - is set to land clearly
// between the two rather than reuse a bound the unfixed code could
// still have slipped under on a fast machine.
const MENTION_SCAN_TIME_BOUND_MS = 300;

test('Format.messageHtml stays linear on 64 mentions followed by a long unbroken run', () => {
  const mentions = [];
  let text = '';
  for (let i = 0; i < 64; i++) {
    mentions.push({ offset: text.length, length: 4 });
    text += '@Eve';
  }
  text += 'a'.repeat(64000);

  const [, elapsedMs] = timed(() => Format.messageHtml(text, null, null, mentions, []));

  assert.ok(elapsedMs < MENTION_SCAN_TIME_BOUND_MS, `Format.messageHtml took ${elapsedMs}ms on 64 mentions plus a 64000-character run, want well under ${MENTION_SCAN_TIME_BOUND_MS}ms`);
});

test('Snooze.parse stays linear on a long non-matching digit run', () => {
  const adversarial = '9'.repeat(65000);
  const [, elapsedMs] = timed(() => Snooze.parse(adversarial, Date.now()));

  assert.ok(elapsedMs < TIME_BOUND_MS, `Snooze.parse took ${elapsedMs}ms on a 65000-digit run, want well under ${TIME_BOUND_MS}ms`);
});
