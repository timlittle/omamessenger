// Tests for ui/lib/Reactions.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Reactions = load('lib/Reactions.js');

test('COMMON_EMOJI offers eight picks', () => {
  assert.strictEqual(Reactions.COMMON_EMOJI.length, 8);
  assert.ok(Reactions.COMMON_EMOJI.includes('👍'));
});

test('emojiToSend clears a reaction clicked again', () => {
  const reactions = [{ emoji: '👍', count: 1, mine: true }];
  assert.strictEqual(Reactions.emojiToSend(reactions, '👍'), '');
});

test('emojiToSend switches to a different emoji', () => {
  const reactions = [{ emoji: '👍', count: 1, mine: true }];
  assert.strictEqual(Reactions.emojiToSend(reactions, '❤️'), '❤️');
});

test('emojiToSend adds a first reaction when there is none yet', () => {
  assert.strictEqual(Reactions.emojiToSend([], '👍'), '👍');
  assert.strictEqual(Reactions.emojiToSend(undefined, '👍'), '👍');
});

test('emojiToSend ignores other people\'s reactions', () => {
  const reactions = [{ emoji: '👍', count: 1, mine: false }];
  assert.strictEqual(Reactions.emojiToSend(reactions, '👍'), '👍');
});

test('applyLocal adds a first reaction', () => {
  assert.deepEqual(Reactions.applyLocal([], '👍'), [{ emoji: '👍', count: 1, mine: true }]);
});

test('applyLocal increments an existing chip someone else started', () => {
  const reactions = [{ emoji: '👍', count: 2, mine: false }];
  assert.deepEqual(Reactions.applyLocal(reactions, '👍'), [{ emoji: '👍', count: 3, mine: true }]);
});

test('applyLocal moves the caller from one chip to another', () => {
  const reactions = [{ emoji: '👍', count: 1, mine: true }, { emoji: '❤️', count: 2, mine: false }];
  assert.deepEqual(Reactions.applyLocal(reactions, '❤️'), [{ emoji: '❤️', count: 3, mine: true }]);
});

test('applyLocal removes a chip that drops to zero', () => {
  const reactions = [{ emoji: '👍', count: 1, mine: true }];
  assert.deepEqual(Reactions.applyLocal(reactions, ''), []);
});

test('applyLocal clearing leaves other people\'s reactions alone', () => {
  const reactions = [{ emoji: '👍', count: 1, mine: true }, { emoji: '❤️', count: 1, mine: false }];
  assert.deepEqual(Reactions.applyLocal(reactions, ''), [{ emoji: '❤️', count: 1, mine: false }]);
});

test('applyLocal tolerates an undefined reactions list', () => {
  assert.deepEqual(Reactions.applyLocal(undefined, '👍'), [{ emoji: '👍', count: 1, mine: true }]);
});
