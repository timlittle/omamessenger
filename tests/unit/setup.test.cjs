// Tests for ui/lib/Setup.js
'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Setup = load('lib/Setup.js');

test('apiID reads a positive whole number, ignoring spaces', () => {
  assert.strictEqual(Setup.apiID(' 123456 '), 123456);
});

test('apiID rejects anything else as 0', () => {
  for (const text of ['', 'abc', '12a', '-5', '0', '1.5']) {
    assert.strictEqual(Setup.apiID(text), 0, text);
  }
});

test('credentialsReady needs an API id and a hash', () => {
  assert.strictEqual(Setup.credentialsReady('123', ' abc '), true);
  assert.strictEqual(Setup.credentialsReady('123', '  '), false);
  assert.strictEqual(Setup.credentialsReady('x', 'abc'), false);
});

test('field describes what each answered step asks for', () => {
  assert.deepEqual(Setup.field('phone'), { step: 'phone', placeholder: 'Phone number, with country code', password: false });
  assert.deepEqual(Setup.field('code'), { step: 'code', placeholder: 'Login code', password: false });
  assert.deepEqual(Setup.field('password'), { step: 'password', placeholder: 'Two-step verification password', password: true });
});

test('field is null for stages without a typed answer', () => {
  for (const stage of ['credentials', 'waiting', 'qr', 'linkcode', '']) {
    assert.strictEqual(Setup.field(stage), null, stage);
  }
});

test('qrSource turns the helper\'s base64 PNG into an image URL', () => {
  assert.strictEqual(Setup.qrSource('cG5n'), 'data:image/png;base64,cG5n');
  assert.strictEqual(Setup.qrSource(''), '');
});

test('serviceMnemonic names the letter a known service answers to, avoiding j and k', () => {
  assert.strictEqual(Setup.serviceMnemonic('telegram'), 't');
  assert.strictEqual(Setup.serviceMnemonic('whatsapp'), 'w');
  assert.strictEqual(Setup.serviceMnemonic('signal'), '');
});

test('mnemonicLabel prefixes the letter, or leaves text alone without one', () => {
  assert.strictEqual(Setup.mnemonicLabel('t', 'Telegram'), 't  Telegram');
  assert.strictEqual(Setup.mnemonicLabel('', 'Signal'), 'Signal');
});

test('wrapIndex moves through the list and wraps onto -1, the "nothing chosen" slot', () => {
  assert.strictEqual(Setup.wrapIndex(-1, 1, 2), 0);
  assert.strictEqual(Setup.wrapIndex(0, 1, 2), 1);
  assert.strictEqual(Setup.wrapIndex(1, 1, 2), -1);
  assert.strictEqual(Setup.wrapIndex(-1, -1, 2), 1);
  assert.strictEqual(Setup.wrapIndex(0, -1, 2), -1);
});

test('wrapIndex stays at -1 with nothing to move through', () => {
  assert.strictEqual(Setup.wrapIndex(-1, 1, 0), -1);
  assert.strictEqual(Setup.wrapIndex(-1, -1, 0), -1);
});
