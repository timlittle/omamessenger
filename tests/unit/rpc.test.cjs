'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { load } = require('./load.cjs');

const Rpc = load('lib/Rpc.js');

test('encodeRequest with default params', () => {
  const line = Rpc.encodeRequest(1, 'hello', undefined);
  const parsed = JSON.parse(line.slice(0, -1));
  assert.equal(parsed.id, 1);
  assert.equal(parsed.method, 'hello');
  assert.deepEqual(parsed.params, {});
  assert.equal(line[line.length - 1], '\n');
});

test('encodeRequest with explicit params', () => {
  const params = { query: 'test' };
  const line = Rpc.encodeRequest(2, 'conversations.list', params);
  const parsed = JSON.parse(line.slice(0, -1));
  assert.equal(parsed.id, 2);
  assert.equal(parsed.method, 'conversations.list');
  assert.deepEqual(parsed.params, params);
  assert.equal(line[line.length - 1], '\n');
});

test('encodeRequest with null params', () => {
  const line = Rpc.encodeRequest(3, 'test', null);
  const parsed = JSON.parse(line.slice(0, -1));
  assert.deepEqual(parsed.params, null);
});

test('parseLine with valid response', () => {
  const line = '{"id":1,"result":{"version":"1.0"}}';
  const result = Rpc.parseLine(line);
  assert.equal(result.kind, 'response');
  assert.equal(result.id, 1);
  assert.equal(JSON.stringify(result.result), JSON.stringify({ version: '1.0' }));
  assert.equal(result.error, undefined);
});

test('parseLine with response error', () => {
  const line = '{"id":2,"error":{"code":"not_found","message":"Not found"}}';
  const result = Rpc.parseLine(line);
  assert.equal(result.kind, 'response');
  assert.equal(result.id, 2);
  assert.equal(result.result, undefined);
  assert.equal(result.error.code, 'not_found');
  assert.equal(result.error.message, 'Not found');
});

test('parseLine with valid event', () => {
  const line = '{"event":"message.added","data":{"id":"123","text":"hello"}}';
  const result = Rpc.parseLine(line);
  assert.equal(result.kind, 'event');
  assert.equal(result.name, 'message.added');
  assert.equal(JSON.stringify(result.data), JSON.stringify({ id: '123', text: 'hello' }));
});

test('parseLine with invalid JSON', () => {
  const result = Rpc.parseLine('not json');
  assert.equal(result.kind, 'invalid');
  assert.equal(Object.keys(result).length, 1);
});

test('parseLine with malformed JSON', () => {
  const result = Rpc.parseLine('{"id": 1, "method": "test"');
  assert.equal(result.kind, 'invalid');
  assert.equal(Object.keys(result).length, 1);
});

test('parseLine with null', () => {
  const result = Rpc.parseLine('null');
  assert.equal(result.kind, 'invalid');
});

test('parseLine with number', () => {
  const result = Rpc.parseLine('42');
  assert.equal(result.kind, 'invalid');
});

test('parseLine with string', () => {
  const result = Rpc.parseLine('"hello"');
  assert.equal(result.kind, 'invalid');
});

test('parseLine with array', () => {
  const result = Rpc.parseLine('[1, 2, 3]');
  assert.equal(result.kind, 'invalid');
});

test('parseLine with empty array', () => {
  const result = Rpc.parseLine('[]');
  assert.equal(result.kind, 'invalid');
});

test('parseLine with object missing id and event', () => {
  const result = Rpc.parseLine('{"data":"test"}');
  assert.equal(result.kind, 'invalid');
});

test('parseLine with non-numeric id', () => {
  const result = Rpc.parseLine('{"id":"1","result":"test"}');
  assert.equal(result.kind, 'invalid');
});

test('parseLine with string id', () => {
  const result = Rpc.parseLine('{"id":"abc","result":"test"}');
  assert.equal(result.kind, 'invalid');
});

test('parseLine with non-string event', () => {
  const result = Rpc.parseLine('{"event":123,"data":{}}');
  assert.equal(result.kind, 'invalid');
});

test('parseLine with numeric event', () => {
  const result = Rpc.parseLine('{"event":42,"data":{}}');
  assert.equal(result.kind, 'invalid');
});

test('parseLine with both id and event', () => {
  // When both are present, id takes precedence (response)
  const line = '{"id":1,"event":"test","result":"value"}';
  const result = Rpc.parseLine(line);
  assert.equal(result.kind, 'response');
  assert.equal(result.id, 1);
});

test('errorText for bad_request', () => {
  const text = Rpc.errorText({ code: 'bad_request', message: 'bad request: text is required' });
  assert.equal(text, 'Bad request: text is required.');
  assert.equal(Rpc.errorText({ code: 'bad_request' }), 'The helper could not accept that request.');
});

test('errorText for not_found', () => {
  const text = Rpc.errorText({ code: 'not_found', message: 'Resource not found' });
  assert.equal(text, 'That conversation or message no longer exists.');
});

test('errorText for unknown_method', () => {
  const text = Rpc.errorText({ code: 'unknown_method', message: 'Method does not exist' });
  assert.equal(text, 'This helper version does not support that action. Update the helper.');
});

test('errorText for internal', () => {
  const text = Rpc.errorText({ code: 'internal', message: 'Database connection failed' });
  assert.equal(text, 'Something went wrong in the helper. Try again.');
});

test('errorText for unknown code', () => {
  const text = Rpc.errorText({ code: 'unknown_code', message: 'Some error' });
  assert.equal(text, 'Unexpected error from the helper.');
});

test('errorText for null', () => {
  const text = Rpc.errorText(null);
  assert.equal(text, 'Unexpected error from the helper.');
});

test('errorText for undefined', () => {
  const text = Rpc.errorText(undefined);
  assert.equal(text, 'Unexpected error from the helper.');
});

test('errorText ignores internal message details', () => {
  const text = Rpc.errorText({ code: 'internal', message: 'Password: secret123' });
  assert.equal(text, 'Something went wrong in the helper. Try again.');
  assert.ok(!text.includes('secret'));
});

test('parseLine with empty object', () => {
  const result = Rpc.parseLine('{}');
  assert.equal(result.kind, 'invalid');
});

test('parseLine with event and no data field', () => {
  const line = '{"event":"test.event"}';
  const result = Rpc.parseLine(line);
  assert.equal(result.kind, 'event');
  assert.equal(result.name, 'test.event');
  assert.equal(result.data, undefined);
});

test('parseLine response with missing result and error', () => {
  const line = '{"id":42}';
  const result = Rpc.parseLine(line);
  assert.equal(result.kind, 'response');
  assert.equal(result.id, 42);
  assert.equal(result.result, undefined);
  assert.equal(result.error, undefined);
});
