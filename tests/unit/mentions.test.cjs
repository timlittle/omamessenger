// Tests for ui/lib/Mentions.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Mentions = load('lib/Mentions.js');

test('activeQuery finds the @query the caret sits in', () => {
  assert.deepEqual(Mentions.activeQuery('hi @na', 6), { start: 3, query: 'na' });
  assert.deepEqual(Mentions.activeQuery('@', 1), { start: 0, query: '' });
});

test('activeQuery is null with no @ before the caret', () => {
  assert.strictEqual(Mentions.activeQuery('hi there', 8), null);
});

test('activeQuery is null once whitespace ends the query', () => {
  assert.strictEqual(Mentions.activeQuery('hi @nadia see', 13), null);
});

test('activeQuery ignores an @ in the middle of a word, like an email address', () => {
  assert.strictEqual(Mentions.activeQuery('user@host', 9), null);
});

test('activeQuery still finds the query when the caret sits before the end of the text', () => {
  assert.deepEqual(Mentions.activeQuery('hi @nad there', 7), { start: 3, query: 'nad' });
});

test('filterMembers matches case-insensitively, substring anywhere in the name', () => {
  const members = [{ id: '1', name: 'Nadia Rahman' }, { id: '2', name: 'Ben Carter' }];
  assert.deepEqual(Mentions.filterMembers(members, 'nad'), [members[0]]);
  assert.deepEqual(Mentions.filterMembers(members, 'MAN'), [members[0]]);
  assert.deepEqual(Mentions.filterMembers(members, 'art'), [members[1]]);
});

test('filterMembers sorts a name that starts with the query first', () => {
  const members = [{ id: '1', name: 'Barnadine' }, { id: '2', name: 'Nadia' }];
  assert.deepEqual(Mentions.filterMembers(members, 'nad').map((m) => m.id), ['2', '1']);
});

test('filterMembers with an empty query returns every member', () => {
  const members = [{ id: '1', name: 'Nadia' }, { id: '2', name: 'Ben' }];
  assert.deepEqual(Mentions.filterMembers(members, ''), members);
});

test('filterMembers on an empty or missing list returns nothing', () => {
  assert.deepEqual(Mentions.filterMembers([], 'a'), []);
  assert.deepEqual(Mentions.filterMembers(undefined, 'a'), []);
});

test('insertMention replaces the query span with the mention token and a trailing space', () => {
  const got = Mentions.insertMention('hi @na there', 3, 6, { id: '1', name: 'Nadia' });
  assert.deepEqual(got, { text: 'hi @Nadia  there', cursor: 10 });
});

test('insertMention at the end of the text leaves nothing after it', () => {
  const got = Mentions.insertMention('hi @na', 3, 6, { id: '1', name: 'Nadia' });
  assert.deepEqual(got, { text: 'hi @Nadia ', cursor: 10 });
});

test('resolveMentions finds each inserted member\'s token, in order', () => {
  const inserted = [{ id: '1', name: 'Nadia' }, { id: '2', name: 'Ben' }];
  const got = Mentions.resolveMentions('hi @Nadia and @Ben, how are you', inserted);
  assert.deepEqual(got, [
    { userId: '1', name: 'Nadia', offset: 3, length: 6 },
    { userId: '2', name: 'Ben', offset: 14, length: 4 }
  ]);
});

test('resolveMentions drops a mention whose token was deleted', () => {
  const inserted = [{ id: '1', name: 'Nadia' }, { id: '2', name: 'Ben' }];
  const got = Mentions.resolveMentions('hi @Ben only', inserted);
  assert.deepEqual(got, [{ userId: '2', name: 'Ben', offset: 3, length: 4 }]);
});

test('resolveMentions finds the same name mentioned twice at two positions', () => {
  const inserted = [{ id: '1', name: 'Nadia' }, { id: '1', name: 'Nadia' }];
  const got = Mentions.resolveMentions('hi @Nadia and also @Nadia', inserted);
  assert.deepEqual(got, [
    { userId: '1', name: 'Nadia', offset: 3, length: 6 },
    { userId: '1', name: 'Nadia', offset: 19, length: 6 }
  ]);
});

test('resolveMentions with none inserted returns nothing', () => {
  assert.deepEqual(Mentions.resolveMentions('hi there', []), []);
  assert.deepEqual(Mentions.resolveMentions('hi there', undefined), []);
});
