.pragma library

// Timeline message annotation: when to show day separators, sender names,
// and whether messages are grouped visually.

.import "Format.js" as Format

// annotate returns {showDay, dayLabel, showSender, groupedWithOlder} for
// each message of a newest-first list.
function annotate(newestFirst, isGroup, nowMs) {
  return newestFirst.map((msg, index) => annotateAt(newestFirst, index, isGroup, nowMs));
}

// CONFIRMED_STATUSES are the delivery states that will never be
// retried (see domain.StatusAdvances on the helper side). The moment
// an outgoing message with its own attachment first reaches one, the
// helper has just retired that attachment into its download cache and
// removed the outgoing copy a cached mediaPath from before may still
// point at.
var CONFIRMED_STATUSES = new Set(['sent', 'delivered', 'read']);

// mediaJustRetired reports whether message's update to oldRow is the
// first time an outgoing message carrying media reaches a confirmed
// delivery state, so upsert knows to drop a cached mediaPath (and any
// mediaFailed flag) from before rather than keep it: the file it
// points at may already be gone, and the next time it is shown it must
// be resolved again instead of staying stuck on it.
function mediaJustRetired(oldRow, message) {
  return !!message.outgoing && !!message.media && !CONFIRMED_STATUSES.has(oldRow.status) && CONFIRMED_STATUSES.has(message.status);
}

// MEDIA_IDENTITY_KEYS are the fields that identify which attachment a
// message's media points at, enough to tell a caption edit apart from
// an edit that swaps in different media.
var MEDIA_IDENTITY_KEYS = ['kind', 'fileName', 'size', 'width', 'height', 'duration', 'thumb', 'url'];

// mediaIdentityChanged reports whether message's media is a different
// attachment from oldRow's, so upsert knows any cached mediaPath and
// its verdicts must not be kept across the change.
function mediaIdentityChanged(oldRow, message) {
  return !mediaSignaturesMatch(mediaSignature(media(oldRow)), mediaSignature(message.media ?? null));
}

// mediaSignature returns a comparable snapshot of media's identity, or
// null for no media.
function mediaSignature(attachment) {
  if (!attachment) return null;

  const signature = {};
  for (const key of MEDIA_IDENTITY_KEYS) signature[key] = attachment[key] ?? null;
  return signature;
}

// mediaSignaturesMatch reports whether two mediaSignature results (or
// either, or both, null for no media) describe the same attachment.
function mediaSignaturesMatch(a, b) {
  if (!a && !b) return true;
  if (!a || !b) return false;

  return MEDIA_IDENTITY_KEYS.every((key) => a[key] === b[key]);
}

// annotateAt returns the annotation for one message. Its older neighbour
// is the next index, because the list is shown bottom to top.
function annotateAt(newestFirst, index, isGroup, nowMs) {
  return annotateForPair(newestFirst[index], newestFirst[index + 1] ?? null, isGroup, nowMs);
}

// annotateForPair is annotateAt's per-row logic, taking a message and its
// older neighbour directly so a live upsert or remove can recompute just
// the rows it touched instead of the whole list.
function annotateForPair(msg, older, isGroup, nowMs) {
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

// MAX_LOADED_ROWS bounds how many messages MessageTimeline keeps loaded
// for the open conversation, so one event never costs more than a fixed
// amount of work no matter how many messages a sender floods it with
// (see docs/decisions.md).
var MAX_LOADED_ROWS = 2000;

// windowNewest drops rows past cap from the oldest end of a newest-first
// array, so a flood of live messages never grows the window past a
// fixed bound; the highlighted row falls out like any other once it
// ages past cap (see docs/decisions.md). rows is never mutated.
function windowNewest(rows, cap) {
  if (rows.length <= cap) return rows;

  return rows.slice(0, cap);
}

// reindex builds an id -> position map from order, a newest-first list
// of {id, created} pairs.
function reindex(order) {
  const index = Object.create(null);
  for (let i = 0; i < order.length; i++) index[order[i].id] = i;
  return index;
}

// mergeRow builds the row upsert writes for message when id is already
// loaded: cached media state carries over from oldRow untouched, unless
// the attachment was just retired (mediaJustRetired) or swapped for
// different media (mediaIdentityChanged), in which case it is dropped
// and resolved again.
function mergeRow(oldRow, message) {
  const mediaReplaced = mediaJustRetired(oldRow, message) || mediaIdentityChanged(oldRow, message);
  const kept = mediaReplaced
    ? { mediaPath: '', mediaPlayPath: '', mediaPlayable: false, mediaPlayKnown: false, mediaPreparing: false, mediaRisky: false, mediaFailed: false, mediaFailedReason: '' }
    : {
        mediaPath: oldRow.mediaPath, mediaPlayPath: oldRow.mediaPlayPath,
        mediaPlayable: oldRow.mediaPlayable, mediaPlayKnown: oldRow.mediaPlayKnown,
        mediaPreparing: oldRow.mediaPreparing, mediaRisky: oldRow.mediaRisky,
        mediaFailed: oldRow.mediaFailed, mediaFailedReason: oldRow.mediaFailedReason
      };

  const merged = Object.assign({}, kept, message);
  if (!mediaReplaced && message.media) merged.media = carryVerifiedDimensions(oldRow, message.media);
  return merged;
}

// carryVerifiedDimensions adds oldRow's verifiedWidth/verifiedHeight
// back onto newMedia, since the service's own update never carries
// them: without this an unrelated update (a reaction, a status change)
// would silently erase the verified size mergeRow already decided to
// keep (see docs/decisions.md).
function carryVerifiedDimensions(oldRow, newMedia) {
  const before = media(oldRow);
  if (!before || (!before.verifiedWidth && !before.verifiedHeight)) return newMedia;

  return Object.assign({ verifiedWidth: before.verifiedWidth || 0, verifiedHeight: before.verifiedHeight || 0 }, newMedia);
}

// applyUpsert decides how message changes a loaded timeline: order is
// the {id, created} pairs newest first, index maps ids to position.
// cap bounds order's size (see windowNewest); protectId is the
// highlighted message, so the caller can tell if it was just evicted
// (protectIdEvicted). Returns the new order/index to use in place of
// the old ones; order and index are consumed, not copied.
function applyUpsert(order, index, message, cap, protectId) {
  const at = index[message.id];
  if (at !== undefined) {
    order[at] = { id: message.id, created: message.created };
    return { update: true, at, order, index, evicted: 0, protectIdEvicted: false, touched: at > 0 ? [at - 1, at] : [at] };
  }

  const insertAt = insertIndex(order, message);
  order.splice(insertAt, 0, { id: message.id, created: message.created });

  const windowed = windowNewest(order, cap);
  const evicted = order.length - windowed.length;
  const protectIdEvicted = evicted > 0 && !!protectId && order.slice(windowed.length).some((r) => r.id === protectId);
  const nextIndex = reindex(windowed);

  const touched = insertAt > 0 ? [insertAt - 1, insertAt] : [insertAt];
  if (evicted > 0 && windowed.length > 0) touched.push(windowed.length - 1);

  return { update: false, at: insertAt, order: windowed, index: nextIndex, evicted, protectIdEvicted, touched };
}

// applyRemove decides how dropping id changes a loaded timeline, the
// same state shape and consumed-not-copied contract as applyUpsert.
// Does nothing if id is not loaded.
function applyRemove(order, index, id) {
  const at = index[id];
  if (at === undefined) return { removed: false, at: -1, order, index, touched: [] };

  order.splice(at, 1);

  return { removed: true, at, order, index: reindex(order), touched: at > 0 ? [at - 1] : [] };
}

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
// share a shape, so media, replyTo, reactions, mentions and formatting,
// each an object, a list or absent, are kept as JSON strings: setting a
// ListModel role straight to an array of objects, rather than at the
// row's first insert, silently leaves the role empty, and reacting to a
// message is exactly that case, an update rather than an insert.
// mediaPath, where its download is, starts empty; mediaFailed, whether
// the last download attempt failed, starts false; mediaFailedReason,
// the safe category of that failure (see server/errors.go), starts
// empty too. mediaPlayPath, the in-window play copy a play fetch
// produced, starts empty until one answers; mediaPlayable just mirrors
// whether mediaPlayPath is set. mediaPreparing is true only while a
// play fetch is in flight, for the "Preparing…" state. mediaPlayKnown
// is whether a play fetch has actually answered this session, so a
// cached mediaPlayPath can be told apart from "never asked yet".
// mentionsMe is a plain boolean, which a ListModel role already
// handles directly, so it needs no such encoding. retryAt, the
// helper's next scheduled automatic retry for a failed outgoing
// message (Unix milliseconds), defaults to 0 while none is scheduled,
// a plain number a ListModel role already handles directly too.
function row(message) {
  return Object.assign({}, message, {
    media: message.media ? JSON.stringify(message.media) : '',
    replyTo: message.replyTo ? JSON.stringify(message.replyTo) : '',
    mediaPath: message.mediaPath ?? '',
    mediaPlayPath: message.mediaPlayPath ?? '',
    mediaPlayable: !!(message.mediaPlayPath ?? ''),
    mediaPlayKnown: message.mediaPlayKnown ?? false,
    mediaPreparing: message.mediaPreparing ?? false,
    mediaRisky: message.mediaRisky ?? false,
    mediaFailed: message.mediaFailed ?? false,
    mediaFailedReason: message.mediaFailedReason ?? '',
    reactions: JSON.stringify(message.reactions ?? []),
    mentions: JSON.stringify(message.mentions ?? []),
    mentionsMe: message.mentionsMe ?? false,
    formatting: JSON.stringify(message.formatting ?? []),
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

// formatting reads a row's rich-text formatting spans, or an empty list
// when it has none. It also accepts a message as the helper sends it.
function formatting(item) {
  return readJsonField(item, 'formatting', []);
}
