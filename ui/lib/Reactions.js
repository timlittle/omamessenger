.pragma library

// Pure logic for message reaction chips: the picker's emoji list, and
// toggling the user's own reaction, shared by the conversation controller
// and the reaction picker.

// COMMON_EMOJI is the short list the reaction picker offers.
var COMMON_EMOJI = ['👍', '❤️', '😂', '😮', '😢', '🙏', '🎉', '🔥'];

// asArray normalizes a message's reactions into a genuine array. A
// freshly loaded message carries a plain array from the server, but once
// a QML ListModel row's "reactions" role has been written to, reading
// that role back gives a read-only list-model object instead (Qt wraps
// any array of objects stored in a model role this way), which has no
// array methods of its own.
function asArray(reactions) {
  if (!reactions) {
    return [];
  }

  if (typeof reactions.count === 'number' && typeof reactions.get === 'function') {
    const out = [];
    for (let i = 0; i < reactions.count; i++) out.push(reactions.get(i));
    return out;
  }

  return reactions;
}

// emojiToSend decides what emoji messages.react should send when the
// user picks emoji on a message: clicking the reaction already theirs
// clears it, any other pick replaces it, since only one reaction per
// person reaches the service.
function emojiToSend(reactions, emoji) {
  const mine = asArray(reactions).find((r) => r.mine);
  return mine && mine.emoji === emoji ? '' : emoji;
}

// applyLocal returns reactions updated as if the service had already
// applied a reaction change, for instant feedback before the server's
// message.updated event confirms it: the caller's previous reaction, if
// any, is removed, then emoji is added, unless it is '', which only
// clears.
function applyLocal(reactions, emoji) {
  const without = asArray(reactions)
    .map((r) => (r.mine ? { emoji: r.emoji, count: r.count - 1, mine: false } : r))
    .filter((r) => r.count > 0);

  if (!emoji) return without;

  const at = without.findIndex((r) => r.emoji === emoji);
  if (at === -1) return without.concat([{ emoji: emoji, count: 1, mine: true }]);

  const updated = without.slice();
  updated[at] = { emoji: emoji, count: updated[at].count + 1, mine: true };
  return updated;
}
