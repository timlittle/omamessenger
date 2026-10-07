.pragma library

// Picking the highlighted message in an open conversation, and the
// key-hint text shown beside it. The highlight is a message id, never an
// index, so loading older history or a new message arriving never moves
// it to a different message: both only ever add rows to the loaded list,
// never touch the id itself, so whatever this file returns keeps pointing
// at the same message the user last chose.
//
// j and k reuse Selection.move, the same pure logic the conversation
// list's own cursor already uses, just with the delta sign flipped: the
// loaded messages are newest first, so moving toward older messages
// steps forward through that array and moving toward newer ones steps
// back.

.import "Selection.js" as Selection
.import "Timeline.js" as Timeline

// older returns the id before currentId when moving toward older
// messages (k/Up), stopping at the oldest loaded one rather than
// wrapping or returning nothing: pressing k at the end of the loaded
// history keeps the highlight exactly where it is, so the caller can
// tell "nothing moved" apart from "moved to the next message" by
// comparing the result against currentId. ids is newest first, so
// moving toward older messages steps forward through it.
function older(ids, currentId) {
  return Selection.move(ids, currentId, 1);
}

// newer returns the id before currentId when moving toward newer
// messages (j/Down), stopping at the newest loaded one the same way
// older() stops at the oldest.
function newer(ids, currentId) {
  return Selection.move(ids, currentId, -1);
}

// hints returns the key-hint text for message, naming what pressing a
// key does to it right now. Reply and react are always on offer, since
// every message can take either; Enter and retry only ever apply to
// this particular message, so they only show when it actually carries a
// photo, video or file, or is itself a failed outgoing send:
//   "r reply · e react"
//   "r reply · e react · Enter open"               (carries media)
//   "r reply · e react · t retry"                   (failed to send)
//   "r reply · e react · Enter open · t retry"      (both)
function hints(message) {
  const parts = ['r reply', 'e react'];
  if (Timeline.media(message)) parts.push('Enter open');
  if (message.outgoing && message.status === 'failed') parts.push('t retry');
  return parts.join(' · ');
}

// atOldest reports whether currentId is already the oldest loaded
// message, the moment pressing k should fetch more history instead of
// moving the highlight: the moment ConversationController.loadOlder()
// takes over, rather than Highlight.older() clamping on an id it cannot
// move past yet.
function atOldest(ids, currentId) {
  return ids.length > 0 && ids[ids.length - 1] === currentId;
}
