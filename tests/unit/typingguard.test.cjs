'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const TypingGuard = load('lib/TypingGuard.js');

const nav = TypingGuard.NAVIGATION_ACTIONS;
const guarded = TypingGuard.GUARDED_ACTIONS;

test('next: the first guarded action, with nothing before it, is always allowed', () => {
  const got = TypingGuard.next('chat.mute', 1000, {}, nav, guarded);

  assert.strictEqual(got.allow, true);
  assert.deepEqual(got.state, { lastAt: 1000, lastAction: 'chat.mute' });
});

test('next: a second guarded action within the burst window is held back', () => {
  const first = TypingGuard.next('chat.mute', 1000, {}, nav, guarded);
  const second = TypingGuard.next('chat.archiveRead', 1150, first.state, nav, guarded);

  assert.strictEqual(second.allow, false);
});

test('next: a second guarded action after the burst window is allowed', () => {
  const first = TypingGuard.next('chat.mute', 1000, {}, nav, guarded);
  const second = TypingGuard.next('chat.archiveRead', 1301, first.state, nav, guarded);

  assert.strictEqual(second.allow, true);
});

test('next: a burst keeps holding back guarded actions as long as keys keep arriving quickly', () => {
  const first = TypingGuard.next('chat.mute', 1000, {}, nav, guarded);
  const second = TypingGuard.next('chat.archiveRead', 1100, first.state, nav, guarded); // held back, burst continues
  const third = TypingGuard.next('message.delete', 1200, second.state, nav, guarded); // still within 300ms of second

  assert.strictEqual(second.allow, false);
  assert.strictEqual(third.allow, false);
});

test('next: an unmatched key (an ordinary typed letter) is always allowed, but still extends the burst', () => {
  const unmatched = TypingGuard.next('', 1000, {}, nav, guarded);
  const guardedAction = TypingGuard.next('chat.mute', 1100, unmatched.state, nav, guarded);

  assert.strictEqual(unmatched.allow, true);
  assert.strictEqual(guardedAction.allow, false, 'a guarded action right after an unmatched letter is still a burst');
});

test('next: typing an ordinary word (several unmatched keys), then reaching a guarded action, holds the guarded one back', () => {
  let state = {};
  for (const action of ['', '', 'chat.open']) {
    const got = TypingGuard.next(action, state.lastAt ? state.lastAt + 50 : 1000, state, nav, guarded);
    assert.strictEqual(got.allow, true, `${action || '(unmatched)'} should never itself be held back`);
    state = got.state;
  }

  const guardedAction = TypingGuard.next('chat.mute', state.lastAt + 50, state, nav, guarded);
  assert.strictEqual(guardedAction.allow, false);
});

test('next: a mode switch is never held back, even right after another guarded action', () => {
  const first = TypingGuard.next('chat.mute', 1000, {}, nav, guarded);
  const second = TypingGuard.next('compose.focus', 1050, first.state, nav, guarded);

  assert.strictEqual(second.allow, true);
});

test('next: Shift+E (edit) then i (focus the composer) are both allowed, even within 100ms', () => {
  const edit = TypingGuard.next('message.edit', 1000, {}, nav, guarded);
  const focus = TypingGuard.next('compose.focus', 1100, edit.state, nav, guarded);

  assert.strictEqual(edit.allow, true);
  assert.strictEqual(focus.allow, true);
});

test('next: chaining several non-guarded shortcuts fast never holds any of them back', () => {
  let state = {};
  for (const [action, at] of [['escape', 1000], ['message.edit', 1050], ['compose.focus', 1100], ['message.reply', 1150]]) {
    const got = TypingGuard.next(action, at, state, nav, guarded);
    assert.strictEqual(got.allow, true, `${action} should never be held back`);
    state = got.state;
  }
});

test('next: navigation actions are always allowed and never tracked', () => {
  for (const action of nav) {
    const got = TypingGuard.next(action, 1000, { lastAt: 999, lastAction: 'anything' }, nav, guarded);
    assert.strictEqual(got.allow, true);
    assert.deepEqual(got.state, { lastAt: 999, lastAction: 'anything' });
  }
});

test('next: navigating does not itself start a burst for the guarded action that follows', () => {
  const navResult = TypingGuard.next('cursor.down', 1000, {}, nav, guarded);
  const guardedAction = TypingGuard.next('chat.mute', 1010, navResult.state, nav, guarded);

  assert.strictEqual(guardedAction.allow, true);
});

test('next: rapid navigation keeps working no matter how fast it repeats', () => {
  let state = {};
  for (let t = 0; t < 10; t++) {
    const got = TypingGuard.next('cursor.down', t * 10, state, nav, guarded);
    assert.strictEqual(got.allow, true);
    state = got.state;
  }
});

test('GUARDED_ACTIONS names only the conversation-changing actions, not openers or mode switches', () => {
  for (const action of ['compose.focus', 'message.edit', 'message.reply', 'message.react', 'chat.open', 'escape', 'chat.unblock']) {
    assert.ok(!guarded.includes(action), `${action} should not be guarded`);
  }
  for (const action of ['chat.archive', 'chat.archiveRead', 'chat.mute', 'chat.pin', 'chat.hide', 'chat.block', 'chat.reportSpam', 'message.delete']) {
    assert.ok(guarded.includes(action), `${action} should be guarded`);
  }
});
