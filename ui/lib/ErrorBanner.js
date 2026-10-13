.pragma library

// Pure decision for the dismissible helper-error banner Panel shows: which
// of several controllers' lastError text, if any, to display right now.

// pick returns the first non-empty string in errors, in priority order, or
// "" when every source is clear. Panel used to hard-code this as a chain
// of || on six controllers; WindowController calls this instead, so the
// banner's priority is one tested function rather than inline QML.
function pick(errors) {
  for (const text of errors) {
    if (text) return text;
  }
  return '';
}
