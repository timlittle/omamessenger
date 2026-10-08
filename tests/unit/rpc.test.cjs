'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./load.cjs');

const Rpc = load('lib/Rpc.js');

test('encodeRequest writes one JSON-RPC 2.0 line', () => {
  const line = Rpc.encodeRequest(2, 'conversations.list', { query: 'test' });

  assert.ok(line.endsWith('\n'));
  assert.deepEqual(JSON.parse(line), {
    jsonrpc: '2.0', id: 2, method: 'conversations.list', params: { query: 'test' }
  });
});

test('encodeRequest sends {} when there are no params', () => {
  for (const params of [undefined, null]) {
    assert.deepEqual(JSON.parse(Rpc.encodeRequest(1, 'hello', params)).params, {});
  }
});

test('parseLine reads a response', () => {
  const got = Rpc.parseLine('{"jsonrpc":"2.0","id":1,"result":{"version":"1.0"}}');

  assert.strictEqual(got.kind, 'response');
  assert.strictEqual(got.id, 1);
  assert.deepEqual(got.result, { version: '1.0' });
  assert.strictEqual(got.error, undefined);
});

test('parseLine reads an error response', () => {
  const got = Rpc.parseLine('{"jsonrpc":"2.0","id":2,"error":{"code":-32001,"message":"not found"}}');

  assert.strictEqual(got.kind, 'response');
  assert.strictEqual(got.error.code, Rpc.CODES.notFound);
});

test('parseLine reads an event', () => {
  const got = Rpc.parseLine('{"jsonrpc":"2.0","method":"message.added","params":{"id":"m1"}}');

  assert.strictEqual(got.kind, 'event');
  assert.strictEqual(got.name, 'message.added');
  assert.deepEqual(got.data, { id: 'm1' });
});

test('parseLine rejects anything else', () => {
  const lines = [
    'not json', '{"id": 1', 'null', '42', '"hello"', '[1, 2]', '{}',
    '{"id":"1","result":1}', '{"method":42}', '{"id":null,"method":"x"}'
  ];

  for (const line of lines) {
    assert.deepEqual(Rpc.parseLine(line), { kind: 'invalid' }, line);
  }
});

test('errorText shows messages written for the user', () => {
  const error = { code: Rpc.CODES.invalidParams, message: 'invalid input: message text is empty' };

  assert.strictEqual(Rpc.errorText(error), 'Message text is empty.');
});

test('errorText uses fixed sentences for everything else', () => {
  const cases = [
    [{ code: Rpc.CODES.invalidParams, message: 'invalid params' }, 'The helper could not accept that request.'],
    [{ code: Rpc.CODES.notFound, message: 'not found' }, 'That conversation or message no longer exists.'],
    [{ code: Rpc.CODES.methodNotFound }, 'This helper version does not support that action. Update the helper.'],
    [{ code: Rpc.CODES.internal, message: 'Password: secret123' }, 'Something went wrong in the helper. Try again.'],
    [{ code: 7 }, 'Unexpected error from the helper.'],
    [null, 'Unexpected error from the helper.'],
    [undefined, 'Unexpected error from the helper.']
  ];

  for (const [error, want] of cases) {
    assert.strictEqual(Rpc.errorText(error), want);
  }
});

test('errorReason reads the safe category from an error\'s data field', () => {
  const error = { code: Rpc.CODES.internal, message: 'internal error', data: { reason: 'decrypt' } };

  assert.strictEqual(Rpc.errorReason(error), 'decrypt');
});

test('errorReason is empty for an error with no data, or none at all', () => {
  assert.strictEqual(Rpc.errorReason({ code: Rpc.CODES.internal, message: 'internal error' }), '');
  assert.strictEqual(Rpc.errorReason(null), '');
});

test('guard: reports whether the captured value still matches the current one', () => {
  let current = 'cafe later';
  const isCurrent = Rpc.guard(() => current, 'cafe');

  assert.strictEqual(isCurrent(), false);

  current = 'cafe';
  assert.strictEqual(isCurrent(), true);
});
