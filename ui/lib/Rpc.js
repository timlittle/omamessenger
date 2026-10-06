.pragma library

function encodeRequest(id, method, params) {
  var p = params !== undefined ? params : {};
  return JSON.stringify({ id: id, method: method, params: p }) + '\n';
}

function parseLine(line) {
  var obj;
  try {
    obj = JSON.parse(line);
  } catch (e) {
    return { kind: 'invalid' };
  }

  if (obj === null || typeof obj !== 'object' || Array.isArray(obj)) {
    return { kind: 'invalid' };
  }

  if ('id' in obj) {
    if (typeof obj.id !== 'number') {
      return { kind: 'invalid' };
    }
    return {
      kind: 'response',
      id: obj.id,
      result: obj.result,
      error: obj.error
    };
  }

  if ('event' in obj) {
    if (typeof obj.event !== 'string') {
      return { kind: 'invalid' };
    }
    return {
      kind: 'event',
      name: obj.event,
      data: obj.data
    };
  }

  return { kind: 'invalid' };
}

// MESSAGES are the user-facing sentences for each C3 error code.
var MESSAGES = {
  bad_request: 'The helper could not accept that request.',
  not_found: 'That conversation or message no longer exists.',
  unknown_method: 'This helper version does not support that action. Update the helper.',
  internal: 'Something went wrong in the helper. Try again.'
};

// errorText turns a C3 error into a short sentence for the UI. A bad_request
// message describes the caller's mistake and is safe to show; every other
// code uses a fixed sentence, so internal details never reach the screen.
function errorText(error) {
  if (!error) {
    return 'Unexpected error from the helper.';
  }
  if (error.code === 'bad_request' && error.message) {
    var text = String(error.message);
    return text.charAt(0).toUpperCase() + text.slice(1) + '.';
  }
  return MESSAGES[error.code] || 'Unexpected error from the helper.';
}
