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
