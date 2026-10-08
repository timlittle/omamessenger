// Tests for ui/lib/Mentions.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Mentions = load('lib/Mentions.js');

const activeQueryCases = [
  ['finds the @query the caret sits in', 'hi @na', 6, { start: 3, query: 'na' }],
  ['finds an empty query right after a bare @', '@', 1, { start: 0, query: '' }],
  ['is null with no @ before the caret', 'hi there', 8, null],
  ['is null once whitespace ends the query', 'hi @nadia see', 13, null],
  ['ignores an @ in the middle of a word, like an email address', 'user@host', 9, null],
  ['still finds the query when the caret sits before the end of the text', 'hi @nad there', 7, { start: 3, query: 'nad' }]
];

for (const [name, text, caret, want] of activeQueryCases) {
  test(`activeQuery ${name}`, () => {
    assert.deepEqual(Mentions.activeQuery(text, caret), want);
  });
}

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
