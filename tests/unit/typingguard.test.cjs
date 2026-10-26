'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const TypingGuard = load('lib/TypingGuard.js');

const nav = TypingGuard.NAVIGATION_ACTIONS;
const guarded = TypingGuard.GUARDED_ACTIONS;

// runBurst runs a sequence of [action, atMs] pairs through next() in
// order, starting from an empty state (or the one passed in), and
// returns the list of {action, allow} results in the same order - a
// burst typed out as a table, the shape every scenario below reads.
function runBurst(sequence, state) {
  let s = state || {};
  const results = [];
  for (const [action, at] of sequence) {
    const got = TypingGuard.next(action, at, s, nav, guarded);
    results.push({ action, allow: got.allow });
    s = got.state;
  }
  return results;
}

test('next: the first guarded action, with nothing before it, is always allowed', () => {
  const got = TypingGuard.next('chat.mute', 1000, {}, nav, guarded);
  assert.strictEqual(got.allow, true);
});

test('next: a second guarded action within the burst window is held back', () => {
  const results = runBurst([['chat.mute', 1000], ['chat.archiveRead', 1150]]);
  assert.deepEqual(results.map((r) => r.allow), [true, false]);
});

test('next: a second guarded action after the burst window is allowed', () => {
  const results = runBurst([['chat.mute', 1000], ['chat.archiveRead', 1301]]);
  assert.deepEqual(results.map((r) => r.allow), [true, true]);
});

test('next: a burst keeps holding back guarded actions as long as keys keep arriving quickly', () => {
  const results = runBurst([['chat.mute', 1000], ['chat.archiveRead', 1100], ['message.delete', 1200]]);
  assert.deepEqual(results.map((r) => r.allow), [true, false, false]);
});

test('next: an unmatched key (an ordinary typed letter) is always allowed on its own, but still extends the burst', () => {
  const results = runBurst([['', 1000], ['chat.mute', 1100]]);
  assert.strictEqual(results[0].allow, true);
  assert.strictEqual(results[1].allow, false, 'a guarded action right after an unmatched letter is still a burst');
});

test('next: a mode switch is never held back on its own, even right after another guarded action', () => {
  const results = runBurst([['chat.mute', 1000], ['compose.focus', 1050]]);
  assert.strictEqual(results[1].allow, true);
});

test('GUARDED_ACTIONS names only the conversation-changing actions, not openers or mode switches', () => {
  for (const action of ['compose.focus', 'message.edit', 'message.reply', 'message.react', 'chat.open', 'escape', 'chat.unblock']) {
    assert.ok(!guarded.includes(action), `${action} should not be guarded`);
  }
  for (const action of ['chat.archive', 'chat.archiveRead', 'chat.mute', 'chat.pin', 'chat.hide', 'chat.block', 'chat.reportSpam', 'message.delete']) {
    assert.ok(guarded.includes(action), `${action} should be guarded`);
  }
});

// The cases below are the ones the report that redesigned this file was
// checked against (see docs/decisions.md): a fast burst that looks like
// a typed word must change nothing, while every deliberate chain of
// shortcuts it names keeps working.

test('a burst that looks like typing "hello" suppresses every action in it, including chat.open and compose.focus, which are never guarded', () => {
  // h matches nothing in the list context (allowed: nothing has
  // happened yet to call a burst); e matches nothing either, and is
  // itself the "any printable key" that follows an unmatched one, so
  // it is already held back, same as l (chat.open, twice over) and o.
  const results = runBurst([['', 1000], ['', 1050], ['chat.open', 1100], ['chat.open', 1150], ['chat.open', 1200]]);
  assert.deepEqual(results.map((r) => r.allow), [true, false, false, false, false]);
});

test('any number of real, matched shortcuts chained quickly never suppresses any of them: there is no unmatched key to signal a typed word', () => {
  const results = runBurst([['message.reply', 1000], ['message.react', 1050], ['message.open', 1100], ['compose.focus', 1150]]);
  assert.deepEqual(results.map((r) => r.allow), [true, true, true, true]);
});

test('Shift+E then i (edit then write) are both allowed, even within 100ms: only two keys, both matched', () => {
  const results = runBurst([['message.edit', 1000], ['compose.focus', 1100]]);
  assert.deepEqual(results.map((r) => r.allow), [true, true]);
});

test('r then i (reply then write) are both allowed, the same as Shift+E then i', () => {
  const results = runBurst([['message.reply', 1000], ['compose.focus', 1100]]);
  assert.deepEqual(results.map((r) => r.allow), [true, true]);
});

test('Esc then E both work: reset() drops whatever the burst had tracked before Escape, which this guard is never even called for', () => {
  // A word-looking burst already flagged...
  const flagged = runBurst([['', 1000], ['', 1050], ['chat.open', 1100]]);
  assert.strictEqual(flagged[2].allow, false, 'setup: the burst should be flagged by now');

  // ...but Escape (reset) between it and the next key starts over, so
  // a deliberate Shift+E right afterwards is not swallowed by a burst
  // that happened before Escape was pressed.
  const afterEscape = TypingGuard.next('message.edit', 1120, TypingGuard.reset(), nav, guarded);
  assert.strictEqual(afterEscape.allow, true);
});

test('j/k/arrow navigation repeated quickly keeps working, even past three presses', () => {
  const results = runBurst([['cursor.down', 0], ['cursor.down', 10], ['cursor.up', 20], ['cursor.down', 30], ['cursor.up', 40]]);
  assert.deepEqual(results.map((r) => r.allow), [true, true, true, true, true]);
});

test('gg (cursor.top pressed twice quickly) keeps working', () => {
  const results = runBurst([['cursor.top', 1000], ['cursor.top', 1050]]);
  assert.deepEqual(results.map((r) => r.allow), [true, true]);
});

test('navigating does not itself start a burst for the guarded action that follows', () => {
  const navResult = TypingGuard.next('cursor.down', 1000, {}, nav, guarded);
  const guardedAction = TypingGuard.next('chat.mute', 1010, navResult.state, nav, guarded);
  assert.strictEqual(guardedAction.allow, true);
});

test('a navigation letter typed as part of a word is suppressed too, once an unmatched key has shown the burst really is one: j in "jam" must not scroll', () => {
  // j (cursor.down, matched) is nav on its own; a is unmatched, which
  // is what marks the rest of the burst - m (chat.mute) and the second
  // j - as part of a typed word rather than real shortcuts, so both
  // are held back even though a lone navigation key on its own never
  // would be.
  const results = runBurst([['cursor.down', 1000], ['', 1050], ['chat.mute', 1100], ['cursor.down', 1150]]);
  assert.deepEqual(results.map((r) => r.allow), [true, true, false, false]);
});

test('a burst made only of navigation keys is allowed, however many presses arrive: none of them is ever unmatched', () => {
  let state = {};
  for (let i = 0; i < 10; i++) {
    const got = TypingGuard.next('cursor.down', i * 10, state, nav, guarded);
    assert.strictEqual(got.allow, true, `press ${i} should still be allowed`);
    state = got.state;
  }
});

test('navigation is never suppressed after two real, deliberate guarded actions in a row - only an actually unmatched key means a burst is really a typed word', () => {
  const results = runBurst([['chat.mute', 1000], ['chat.pin', 1010], ['cursor.down', 1020]]);
  assert.deepEqual(results.map((r) => r.allow), [true, false, true]);
});

test('a long run of fast navigation never counts towards suppressing the very next shortcut', () => {
  // Scrolling with k eight times, none of them unmatched, then
  // reaching for r (reply) right afterwards must not be mistaken for a
  // typed word just because the navigation run was long: nothing in
  // it was ever unmatched, so the burst is not flagged for r either.
  let state = {};
  for (let i = 0; i < 8; i++) {
    const got = TypingGuard.next('cursor.up', i * 10, state, nav, guarded);
    state = got.state;
  }
  const reply = TypingGuard.next('message.reply', 90, state, nav, guarded);
  assert.strictEqual(reply.allow, true);
});

test('reset returns an empty state, the same shape next() starts from', () => {
  assert.deepEqual(TypingGuard.reset(), {});
});
