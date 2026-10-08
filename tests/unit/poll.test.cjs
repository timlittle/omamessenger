// Tests for ui/lib/Poll.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Poll = load('lib/Poll.js');

test('percentages shares the vote by total voters', () => {
  const poll = { totalVoters: 4, options: [{ votes: 3 }, { votes: 1 }] };
  assert.deepEqual(Poll.percentages(poll), [75, 25]);
});

test('percentages is every option at zero before anyone has voted', () => {
  const poll = { totalVoters: 0, options: [{ votes: 0 }, { votes: 0 }] };
  assert.deepEqual(Poll.percentages(poll), [0, 0]);
});

test('percentages handles a poll with no options', () => {
  assert.deepEqual(Poll.percentages({ totalVoters: 0, options: [] }), []);
  assert.deepEqual(Poll.percentages({}), []);
});

test('percentages rounds to the nearest whole number', () => {
  const poll = { totalVoters: 3, options: [{ votes: 1 }, { votes: 1 }, { votes: 1 }] };
  assert.deepEqual(Poll.percentages(poll), [33, 33, 33]);
});

test('voterLabel pluralizes the vote count', () => {
  assert.strictEqual(Poll.voterLabel(0), '0 votes');
  assert.strictEqual(Poll.voterLabel(1), '1 vote');
  assert.strictEqual(Poll.voterLabel(5), '5 votes');
  assert.strictEqual(Poll.voterLabel(undefined), '0 votes');
});

test('toggleOption in a single-choice poll always replaces the selection', () => {
  assert.deepEqual(Poll.toggleOption([], 'a', false), ['a']);
  assert.deepEqual(Poll.toggleOption(['a'], 'b', false), ['b']);
});

test('toggleOption in a multiple-choice poll adds an unselected option', () => {
  assert.deepEqual(Poll.toggleOption([], 'a', true), ['a']);
  assert.deepEqual(Poll.toggleOption(['a'], 'b', true), ['a', 'b']);
});

test('toggleOption in a multiple-choice poll removes a selected option', () => {
  assert.deepEqual(Poll.toggleOption(['a', 'b'], 'a', true), ['b']);
});

test('applyLocalVote marks the chosen option and adds a first-time voter', () => {
  const poll = { totalVoters: 2, options: [{ id: 'a', text: 'Pizza', votes: 2, chosen: false }, { id: 'b', text: 'Salad', votes: 0, chosen: false }] };
  const got = Poll.applyLocalVote(poll, ['a']);

  assert.deepEqual(got.options, [
    { id: 'a', text: 'Pizza', votes: 3, chosen: true },
    { id: 'b', text: 'Salad', votes: 0, chosen: false }
  ]);
  assert.strictEqual(got.totalVoters, 3);
});

test('applyLocalVote moves an existing vote without growing the voter count', () => {
  const poll = { totalVoters: 3, options: [{ id: 'a', votes: 2, chosen: true }, { id: 'b', votes: 1, chosen: false }] };
  const got = Poll.applyLocalVote(poll, ['b']);

  assert.deepEqual(got.options, [{ id: 'a', votes: 1, chosen: false }, { id: 'b', votes: 2, chosen: true }]);
  assert.strictEqual(got.totalVoters, 3);
});

test('applyLocalVote supports choosing several options in a multiple-choice poll', () => {
  const poll = { totalVoters: 0, options: [{ id: 'a', votes: 0, chosen: false }, { id: 'b', votes: 0, chosen: false }] };
  const got = Poll.applyLocalVote(poll, ['a', 'b']);

  assert.deepEqual(got.options.map((o) => o.chosen), [true, true]);
  assert.strictEqual(got.totalVoters, 1);
});

test('applyLocalVote never drops a vote count below zero', () => {
  const poll = { totalVoters: 1, options: [{ id: 'a', votes: 0, chosen: true }] };
  const got = Poll.applyLocalVote(poll, []);

  assert.strictEqual(got.options[0].votes, 0);
});
