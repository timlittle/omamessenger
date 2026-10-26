'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const TypingGuard = load('lib/TypingGuard.js');

const nav = TypingGuard.NAVIGATION_ACTIONS;

test('next: the first plain key is always allowed', () => {
  const got = TypingGuard.next('chat.mute', 1000, {}, nav);

  assert.strictEqual(got.allow, true);
  assert.deepEqual(got.state, { lastAt: 1000, lastAction: 'chat.mute' });
});

test('next: a second plain key within the burst window is suppressed', () => {
  const first = TypingGuard.next('chat.mute', 1000, {}, nav);
  const second = TypingGuard.next('chat.archive', 1150, first.state, nav);

  assert.strictEqual(second.allow, false);
});

test('next: a second plain key after the burst window is allowed', () => {
  const first = TypingGuard.next('chat.mute', 1000, {}, nav);
  const second = TypingGuard.next('chat.archive', 1301, first.state, nav);

  assert.strictEqual(second.allow, true);
});

test('next: a burst keeps suppressing as long as keys keep arriving quickly', () => {
  let state = {};
  const result1 = TypingGuard.next('a', 1000, state, nav);
  state = result1.state;
  const result2 = TypingGuard.next('s', 1100, state, nav); // suppressed, burst starts
  state = result2.state;
  const result3 = TypingGuard.next('d', 1200, state, nav); // still within 300ms of result2

  assert.strictEqual(result2.allow, false);
  assert.strictEqual(result3.allow, false);
});

test('next: navigation actions are always allowed and never tracked', () => {
  for (const action of nav) {
    const got = TypingGuard.next(action, 1000, { lastAt: 999, lastAction: 'anything' }, nav);
    assert.strictEqual(got.allow, true);
    assert.deepEqual(got.state, { lastAt: 999, lastAction: 'anything' });
  }
});

test('next: navigating does not itself start a burst for the shortcut that follows', () => {
  const navResult = TypingGuard.next('cursor.down', 1000, {}, nav);
  const shortcutResult = TypingGuard.next('chat.mute', 1010, navResult.state, nav);

  assert.strictEqual(shortcutResult.allow, true);
});

test('next: rapid navigation keeps working no matter how fast it repeats', () => {
  let state = {};
  for (let t = 0; t < 10; t++) {
    const got = TypingGuard.next('cursor.down', t * 10, state, nav);
    assert.strictEqual(got.allow, true);
    state = got.state;
  }
});
