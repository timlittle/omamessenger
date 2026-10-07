.pragma library

// SenderColor: a stable colour index for one sender in a group chat. A
// sender's messages arrive in any order and can be re-rendered any number
// of times, so the index has to come from the sender's id itself rather
// than from when they were first seen.

// colorIndex returns a stable index in [0, paletteSize) for id: the same
// id always yields the same index, and different ids spread across the
// range. An empty or missing id (a message without a sender yet) safely
// returns 0 instead of throwing.
function colorIndex(id, paletteSize) {
  const size = Math.max(1, Math.trunc(paletteSize));
  const text = String(id ?? '');
  if (!text) return 0;

  return hash(text) % size;
}

// hash is the FNV-1a string hash: fast, dependency-free, and spread
// enough for a handful of sender ids across a short palette.
function hash(text) {
  let h = 0x811c9dc5;
  for (let i = 0; i < text.length; i++) {
    h ^= text.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return h >>> 0;
}
