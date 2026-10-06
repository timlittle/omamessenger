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

function errorText(error) {
  if (error === null || error === undefined) {
    return 'Error';
  }

  var code = error.code;
  switch (code) {
    case 'bad_request':
      return 'Invalid request';
    case 'not_found':
      return 'Not found';
    case 'unknown_method':
      return 'Unknown method';
    case 'internal':
      return 'Something went wrong';
    default:
      return 'Error';
  }
}
