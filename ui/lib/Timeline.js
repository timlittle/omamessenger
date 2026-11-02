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

// MEDIA_IDENTITY_KEYS are the fields that, together with Kind, identify
// which attachment a message's media points at: enough to tell a
// caption edit (same attachment, same kind) apart from an edit that
// swaps in different media altogether.
var MEDIA_IDENTITY_KEYS = ['kind', 'fileName', 'size', 'width', 'height', 'duration', 'thumb', 'url'];

// mediaIdentityChanged reports whether message's own media is a
// different attachment from oldRow's own stored media - a different
// kind, or the same kind pointing at different bytes (by
// MEDIA_IDENTITY_KEYS) - so upsert knows a cached mediaPath and its
// verdicts describe bytes this edit just replaced, not the ones now
// shown, and must never be kept across it. A service's end-to-end
// encrypted edit can change a message's media kind entirely (a voice
// note edited into a photo, say), and the helper's own download cache
// and safety checks are keyed by this message's id, not by its
// media's own identity, so nothing on that side alone stops this UI
// from reusing an older, unrelated file's path and verdicts if it is
// not cleared here.
function mediaIdentityChanged(oldRow, message) {
  return !mediaSignaturesMatch(mediaSignature(media(oldRow)), mediaSignature(message.media ?? null));
}

// mediaSignature returns a comparable snapshot of media's own identity
// (every MEDIA_IDENTITY_KEYS field), or null for no media at all.
// mediaIdentityChanged above compares two of these; MediaController's
// own downloadMedia takes one when a fetch starts and compares it
// against the message's current one when the answer arrives, so a
// stale answer - started under one kind, delivered after an edit
// swapped in another - never writes its path or verdicts back onto
// media it was never checked for (see docs/decisions.md).
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

// annotateForPair is annotateAt's own per-row logic, taking a message
// and its older neighbour (or null when it has none) directly rather
// than an array and an index: the only two rows a caller that just
// touched one or two loaded messages - a live upsert or remove - needs
// to recompute, without walking every other loaded row to get there.
// The expensive part for a large loaded list is the Date/toDateString
// pair sameDay builds for every row annotate() visits; this is how
// MessageTimeline avoids paying that for rows nothing changed about.
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

// MAX_LOADED_ROWS bounds how many messages MessageTimeline keeps
// loaded for the open conversation at once, however long it has stayed
// open and however many messages a sender floods it with: every event
// (message.added, message.updated, message.removed) costs work bounded
// by this constant, not by how many messages have ever arrived (see
// docs/decisions.md). 2000 rows is deep enough to scroll back through
// 40 loadOlder pages of 50 before the cap is ever felt in ordinary use,
// while keeping one event's worst case - a handful of small object
// copies and an id -> position rebuild - a small, fixed amount of work
// no flood can grow.
var MAX_LOADED_ROWS = 2000;

// windowNewest drops rows past cap from the oldest end - the back of a
// newest-first array - so a flood of live messages, or a long run of
// loadOlder pages, never grows past a fixed bound. cap is absolute: an
// earlier version let a "protected" id (the highlighted message) grow
// the window past cap so it was never dropped, but highlightedId never
// moves on its own as new messages arrive, so a flood that just kept
// posting made protectAt - and the window - grow by one row per
// message forever, the exact unbounded growth this function exists to
// prevent (see docs/decisions.md). The highlighted row now falls out
// like any other once it ages past cap; applyUpsert reports that to its
// caller (protectIdEvicted) so the highlight can move back to the
// newest message instead of pointing at a row that is no longer
// loaded. rows is never mutated.
function windowNewest(rows, cap) {
  if (rows.length <= cap) return rows;

  return rows.slice(0, cap);
}

// reindex builds an id -> position map from order, a newest-first list
// of {id, created} pairs (or full rows, which also carry an id).
// MAX_LOADED_ROWS keeps order a small constant size no matter how many
// events have arrived, so this never grows with a flood's size.
// Object.create(null) skips the prototype chain a flood's own ids
// could otherwise collide with (an id of "constructor", say) and is
// measurably cheaper to fill than a literal {} for a map rebuilt this
// often.
function reindex(order) {
  const index = Object.create(null);
  for (let i = 0; i < order.length; i++) index[order[i].id] = i;
  return index;
}

// mergeRow builds the row upsert writes for message when id is already
// loaded: a cached mediaPath, its verdicts (mediaPlayPath, mediaPlayable,
// mediaPlayKnown, mediaRisky, mediaPreparing), mediaFailed and the
// media's own verifiedWidth/verifiedHeight (see carryVerifiedDimensions)
// normally carry over from oldRow untouched, since the service's own
// update never carries any of them - but they are dropped instead the
// moment this update is either the message's attachment being retired
// into the download cache (see mediaJustRetired) or an edit that swaps
// its media for a different kind or different bytes of the same kind
// (see mediaIdentityChanged): in both cases the cached path, verdicts
// and verified size describe bytes that are no longer this row's own,
// and must be resolved and checked again rather than reused or shown
// as missing forever.
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

// carryVerifiedDimensions returns newMedia - message's own media, as
// the service's message.updated echo sent it - with oldRow's own
// verifiedWidth/verifiedHeight (see MediaController.setMedia) added
// back onto it, when oldRow had them. The echo is always the sender's
// original, unverified claim; it never carries these fields, since
// they are MediaController's own record of media.fetch's answer, not
// anything the message protocol sends (see ImageBounds.verifiedDimensions
// and docs/decisions.md's "Known residual gap", now closed). Without
// this, an unrelated update that keeps the same attachment - a
// reaction, a delivery-status change, a caption edit - would silently
// erase the full-file display stage's own verified size, the same way
// a cached mediaPath would be lost if mergeRow did not keep it too.
// Only called once mergeRow has already decided the media identity has
// not changed, so newMedia is always the same attachment oldRow's own
// media described.
function carryVerifiedDimensions(oldRow, newMedia) {
  const before = media(oldRow);
  if (!before || (!before.verifiedWidth && !before.verifiedHeight)) return newMedia;

  return Object.assign({ verifiedWidth: before.verifiedWidth || 0, verifiedHeight: before.verifiedHeight || 0 }, newMedia);
}

// applyUpsert decides how message changes a loaded timeline, without
// touching any QML model: order is the timeline's {id, created} pairs,
// newest first, kept in step with the real rows by whoever calls this
// (see MessageTimeline.qml); index maps those same ids to their
// position in order, so finding an existing message is O(1) instead of
// a scan. cap bounds how many rows order may hold afterward (see
// windowNewest), unconditionally: nothing, including protectId, ever
// grows order past it. protectId names the highlighted message, when
// there is one, purely so this can tell the caller whether this
// particular insert is the one that just evicted it (protectIdEvicted);
// it never changes which rows are kept.
//
// Returns update (true for an existing message, false for a new one),
// at (the row's final position), the new order and index to keep
// instead of the old ones, evicted (how many rows the cap just
// dropped, usually 0), protectIdEvicted (whether protectId was among
// them, for the caller to move the highlight off a row that no longer
// exists) and touched (every position whose annotation may now be
// wrong, for the caller to recompute - see annotateForPair).
//
// order and index are consumed, not copied: a flood calling this once
// per message would otherwise pay for an extra O(MAX_LOADED_ROWS) copy
// on top of the splice and reindex every insert already costs, and
// MessageTimeline.qml's own caller never keeps the old order or index
// around after the call to notice. Either call this only with values
// you are ready to replace with the result, or pass copies of your own.
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
// same shape of state applyUpsert takes and returns, and the same
// consumed-not-copied contract. removed is false, with order and
// index unchanged, when id is not loaded: the service already removed
// it, so there is nothing left to find. touched is the id that was
// immediately newer than the removed row, if any, since its own
// annotation depends on comparing itself to whichever row sits after
// it, and that is now a different message.
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
// produced (see MessageTimeline.setMediaPlayPath and media.fetch's
// playPath in docs/api.md), starts empty, the same as mediaPath,
// until a play fetch answers; mediaPlayable is always just whether
// mediaPlayPath is set, never a separate verdict of its own.
// mediaPreparing is true only while a play fetch for this row is
// actually in flight (see MessageTimeline.setMediaPreparing), for the
// video or voice control to show "Preparing…" instead of its usual
// play glyph. mediaRisky, the helper's own verdict on whether a
// downloaded file is active or executable content that must be
// confirmed before it opens externally (see
// MessageTimeline.setMediaRisky), starts false. mediaPlayKnown is
// whether a play fetch has actually answered for this row in this
// session, so a caller deciding whether to trust a cached
// mediaPlayPath (or its empty default) can tell a real answer apart
// from "never asked yet" (see MessageTimeline.mediaPlayKnown); starts
// false, even for a row whose mediaPath came already set.
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
