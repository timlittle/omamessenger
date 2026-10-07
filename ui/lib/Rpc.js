.pragma library

// Encoding and decoding for the helper's JSON-RPC 2.0 protocol: one JSON
// object per line on the helper's stdin and stdout. Requests carry an id;
// events from the helper are notifications without one.

// CODES are the JSON-RPC error codes the helper sends.
var CODES = {
  invalidParams: -32602,
  methodNotFound: -32601,
  internal: -32603,
  notFound: -32001
};

// MESSAGES are the fixed sentences shown for each error code.
var MESSAGES = {
  [-32602]: 'The helper could not accept that request.',
  [-32601]: 'This helper version does not support that action. Update the helper.',
  [-32603]: 'Something went wrong in the helper. Try again.',
  [-32001]: 'That conversation or message no longer exists.'
};

// encodeRequest returns one request line for the helper's stdin.
function encodeRequest(id, method, params) {
  return JSON.stringify({ jsonrpc: '2.0', id, method, params: params ?? {} }) + '\n';
}

// parseLine classifies one line from the helper as a response
// ({kind, id, result, error}), an event ({kind, name, data}) or invalid.
function parseLine(line) {
  let message;
  try {
    message = JSON.parse(line);
  } catch (e) {
    return { kind: 'invalid' };
  }

  if (message === null || typeof message !== 'object' || Array.isArray(message)) {
    return { kind: 'invalid' };
  }

  if (typeof message.id === 'number') {
    return { kind: 'response', id: message.id, result: message.result, error: message.error };
  }

  if (!('id' in message) && typeof message.method === 'string') {
    return { kind: 'event', name: message.method, data: message.params };
  }

  return { kind: 'invalid' };
}

// errorText turns a helper error into a short sentence for the UI. Only
// invalid-params messages are written for the user, so only they are shown;
// every other code uses a fixed sentence and no internal detail leaks.
function errorText(error) {
  if (!error) {
    return 'Unexpected error from the helper.';
  }

  if (error.code === CODES.invalidParams && error.message && error.message !== 'invalid params') {
    const text = String(error.message).replace(/^invalid input: /, '');
    return text.charAt(0).toUpperCase() + text.slice(1) + '.';
  }

  return MESSAGES[error.code] ?? 'Unexpected error from the helper.';
}

// errorReason reads the safe category a failed media.fetch's error
// carries in its data field (see server/errors.go), or "" when there is
// none: an older helper, or an error that is not about media at all.
function errorReason(error) {
  return (error && error.data && error.data.reason) || '';
}
