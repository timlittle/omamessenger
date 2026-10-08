// Tests for ui/lib/Snooze.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Snooze = load('lib/Snooze.js');

// A fixed "now": Tuesday 6 October 2026, 12:00 local time.
const nowMs = new Date(2026, 9, 6, 12, 0).getTime();

test('laterToday is three hours ahead', () => {
  assert.strictEqual(Snooze.laterToday(nowMs), nowMs + 3 * 60 * 60 * 1000);
});

test('tomorrow is nine in the morning the next day', () => {
  assert.strictEqual(Snooze.tomorrow(nowMs), new Date(2026, 9, 7, 9, 0).getTime());
});

test('nextWeek is nine in the morning next Monday', () => {
  assert.strictEqual(Snooze.nextWeek(nowMs), new Date(2026, 9, 12, 9, 0).getTime());
});

test('nextWeek chosen on a Monday means next week, not today', () => {
  const monday = new Date(2026, 9, 5, 8, 0).getTime(); // Monday 5 October
  assert.strictEqual(Snooze.nextWeek(monday), new Date(2026, 9, 12, 9, 0).getTime());
});

test('nextWeek chosen on a Sunday means next week, not tomorrow', () => {
  const sunday = new Date(2026, 9, 4, 8, 0).getTime(); // Sunday 4 October
  assert.strictEqual(Snooze.nextWeek(sunday), new Date(2026, 9, 12, 9, 0).getTime());
});

test('parse reads a plain hours duration', () => {
  assert.strictEqual(Snooze.parse('2h', nowMs), nowMs + 2 * 60 * 60 * 1000);
});

test('parse reads a plain minutes duration', () => {
  assert.strictEqual(Snooze.parse('30m', nowMs), nowMs + 30 * 60 * 1000);
});

test('parse reads a combined hours and minutes duration', () => {
  assert.strictEqual(Snooze.parse('1h30m', nowMs), nowMs + 90 * 60 * 1000);
});

test('parse reads a clock time still ahead today', () => {
  assert.strictEqual(Snooze.parse('18:00', nowMs), new Date(2026, 9, 6, 18, 0).getTime());
});

test('parse reads a clock time already passed as tomorrow', () => {
  assert.strictEqual(Snooze.parse('9:00', nowMs), new Date(2026, 9, 7, 9, 0).getTime());
});

test('parse reads a weekday and clock time for the next such day', () => {
  // Today is Tuesday; "mon 9:00" must mean next Monday, not today.
  assert.strictEqual(Snooze.parse('mon 9:00', nowMs), new Date(2026, 9, 12, 9, 0).getTime());
});

test('parse reads a weekday and clock time for later today when today is that day', () => {
  assert.strictEqual(Snooze.parse('tue 18:00', nowMs), new Date(2026, 9, 6, 18, 0).getTime());
});

test('parse reads a weekday and clock time for next week when today is that day but the time has passed', () => {
  assert.strictEqual(Snooze.parse('tue 9:00', nowMs), new Date(2026, 9, 13, 9, 0).getTime());
});

test('parse rejects an invalid clock time', () => {
  assert.strictEqual(Snooze.parse('25:00', nowMs), null);
  assert.strictEqual(Snooze.parse('9:60', nowMs), null);
});

test('parse rejects text that matches nothing', () => {
  assert.strictEqual(Snooze.parse('tomorrow please', nowMs), null);
  assert.strictEqual(Snooze.parse('', nowMs), null);
  assert.strictEqual(Snooze.parse('   ', nowMs), null);
});

test('parse is case-insensitive for weekdays', () => {
  assert.strictEqual(Snooze.parse('MON 9:00', nowMs), new Date(2026, 9, 12, 9, 0).getTime());
});

test('preview is empty while the text does not parse', () => {
  assert.deepEqual(Snooze.preview('whenever', nowMs), []);
});

test('preview names when a valid custom time would wake the chat', () => {
  const got = Snooze.preview('18:00', nowMs);
  assert.strictEqual(got.length, 1);
  assert.strictEqual(got[0].at, new Date(2026, 9, 6, 18, 0).getTime());
  assert.strictEqual(got[0].label, 'Snooze until 18:00');
});
