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
// share a shape, so media and replyTo, each an object or absent, are kept
// as JSON strings, mediaPath, where its download is, starts empty, and
// reactions, absent on a message with none, defaults to an empty list.
function row(message) {
  return Object.assign({}, message, {
    media: message.media ? JSON.stringify(message.media) : '',
    replyTo: message.replyTo ? JSON.stringify(message.replyTo) : '',
    mediaPath: message.mediaPath ?? '',
    reactions: message.reactions ?? []
  });
}

// media reads a row's media, or null when it has none. It also accepts a
// message as the helper sends it.
function media(item) {
  if (!item.media) {
    return null;
  }

  return typeof item.media === 'string' ? JSON.parse(item.media) : item.media;
}

// replyTo reads a row's quoted message, or null when it answers nothing.
// It also accepts a message as the helper sends it.
function replyTo(item) {
  if (!item.replyTo) {
    return null;
  }

  return typeof item.replyTo === 'string' ? JSON.parse(item.replyTo) : item.replyTo;
}
