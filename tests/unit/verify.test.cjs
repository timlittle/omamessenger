'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Verify = load('lib/Verify.js');

test('a verified name renders exactly as given', () => {
  assert.strictEqual(Verify.markedName('Mum', true), 'Mum');
});

test('an unverified name is marked with a leading ~', () => {
  assert.strictEqual(Verify.markedName('Mum', false), '~Mum');
});

test('a missing verified flag is treated as unverified, not trusted by default', () => {
  assert.strictEqual(Verify.markedName('Mum', undefined), '~Mum');
});

test('a self-chosen name of "You" still renders marked, never as the reserved label', () => {
  assert.strictEqual(Verify.markedName('You', false), '~You');
});

test('the reserved "You" label itself renders unmarked when verified', () => {
  assert.strictEqual(Verify.markedName('You', true), 'You');
});

test('titleSubtitle shows the phone number only when the title is unverified', () => {
  assert.strictEqual(Verify.titleSubtitle(false, '+15551234567'), '+15551234567');
  assert.strictEqual(Verify.titleSubtitle(true, '+15551234567'), '');
});

test('titleSubtitle is empty when the service gave no phone number, even if unverified', () => {
  assert.strictEqual(Verify.titleSubtitle(false, ''), '');
});

test('titleSubtitle treats a missing verified flag as unverified, like markedName', () => {
  assert.strictEqual(Verify.titleSubtitle(undefined, '+15551234567'), '+15551234567');
});
