.pragma library

// Timeline message annotation: when to show day separators, sender names,
// and whether messages are grouped visually.

.import "Format.js" as Format

// annotate returns {showDay, dayLabel, showSender, groupedWithOlder} for
// each message of a newest-first list.
function annotate(newestFirst, isGroup, nowMs) {
  return newestFirst.map((msg, index) => annotateAt(newestFirst, index, isGroup, nowMs));
}

// annotateAt returns the annotation for one message. Its older neighbour
// is the next index, because the list is shown bottom to top.
function annotateAt(newestFirst, index, isGroup, nowMs) {
  const msg = newestFirst[index];
  const older = newestFirst[index + 1] ?? null;
  const showDay = !older || !sameDay(msg.created, older.created);

  return {
    showDay,
    dayLabel: showDay ? Format.dayLabel(msg.created, nowMs) : '',
    showSender: isGroup && !msg.outgoing && (showDay || older.outgoing || older.senderId !== msg.senderId),
    groupedWithOlder: !showDay && sameAuthor(msg, older) && msg.created - older.created < GROUP_WINDOW_MS
  };
}

// GROUP_WINDOW_MS is how close together messages from one author must be
// to sit together without extra spacing.
var GROUP_WINDOW_MS = 5 * 60 * 1000;

// sameDay reports whether two timestamps fall on the same local calendar day.
function sameDay(msA, msB) {
  return new Date(msA).toDateString() === new Date(msB).toDateString();
}

// sameAuthor reports whether two messages come from the same side and sender.
function sameAuthor(a, b) {
  return a.outgoing === b.outgoing && a.senderId === b.senderId;
}

// insertIndex is where message belongs in a newest-first list: before the
// first message older than or as old as it, so a message from earlier
// history lands in its place rather than at the bottom.
function insertIndex(newestFirst, message) {
  const at = newestFirst.findIndex((m) => m.created <= message.created);
  return at === -1 ? newestFirst.length : at;
}

// row is a message as a timeline model row. A ListModel needs every row to
// share a shape, so media, replyTo, reactions and mentions, each an
// object, a list or absent, are kept as JSON strings: setting a
// ListModel role straight to an array of objects, rather than at the
// row's first insert, silently leaves the role empty, and reacting to a
// message is exactly that case, an update rather than an insert.
// mediaPath, where its download is, starts empty; mediaFailed, whether
// the last download attempt failed, starts false; mediaFailedReason,
// the safe category of that failure (see server/errors.go), starts
// empty too. mentionsMe is a plain boolean, which a ListModel role
// already handles directly, so it needs no such encoding. retryAt, the
// helper's next scheduled automatic retry for a failed outgoing message
// (Unix milliseconds), defaults to 0 while none is scheduled, a plain
// number a ListModel role already handles directly too.
function row(message) {
  return Object.assign({}, message, {
    media: message.media ? JSON.stringify(message.media) : '',
    replyTo: message.replyTo ? JSON.stringify(message.replyTo) : '',
    mediaPath: message.mediaPath ?? '',
    mediaFailed: message.mediaFailed ?? false,
    mediaFailedReason: message.mediaFailedReason ?? '',
    reactions: JSON.stringify(message.reactions ?? []),
    mentions: JSON.stringify(message.mentions ?? []),
    mentionsMe: message.mentionsMe ?? false,
    retryAt: message.retryAt ?? 0
  });
}

// readJsonField reads item's name field, which row() may have encoded as
// a JSON string for the ListModel role to hold, the object itself when
// item is a message straight from the helper, or fallback when it
// carries none.
function readJsonField(item, name, fallback) {
  const value = item[name];
  if (!value) {
    return fallback;
  }

  return typeof value === 'string' ? JSON.parse(value) : value;
}

// media reads a row's media, or null when it has none. It also accepts a
// message as the helper sends it.
function media(item) {
  return readJsonField(item, 'media', null);
}

// replyTo reads a row's quoted message, or null when it answers nothing.
// It also accepts a message as the helper sends it.
function replyTo(item) {
  return readJsonField(item, 'replyTo', null);
}

// reactions reads a row's reaction chips, or an empty list when it has
// none. It also accepts a message as the helper sends it.
function reactions(item) {
  return readJsonField(item, 'reactions', []);
}

// mentions reads a row's @-mention tokens, or an empty list when it has
// none. It also accepts a message as the helper sends it.
function mentions(item) {
  return readJsonField(item, 'mentions', []);
}
