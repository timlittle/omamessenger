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
.import "Keymap.js" as Keymap

// LINK_PATTERN matches http(s) links in a message's raw text, the shape
// messageHtml's own linkify in Format.js uses on the escaped version. It
// runs on the unescaped text, since this file never builds HTML.
var LINK_PATTERN = /https?:\/\/[^\s]+/g;

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
// key does to it right now. Reply, react and delete are always on
// offer, since every message can take any of them; the rest only ever
// apply to this particular message, so they only show when it actually
// carries a photo, video, file or voice note, a link, a quote, or is
// itself a failed outgoing send:
//   "r reply · e react · d delete"
//   "r reply · e react · d delete · Enter open"     (carries media)
//   "r reply · e react · d delete · Enter play"     (carries a voice note)
//   "r reply · e react · d delete · o open link"    (carries a link)
//   "r reply · e react · d delete · p go to quote"  (is a reply)
//   "r reply · e react · d delete · t retry"        (failed to send)
//   "r reply · e react · d delete · v vote"         (carries an open poll)
// Each key names the action's own effective key (see KeyBindings.js),
// so a keys.conf override shows up here too, not just its own default.
function hints(message, bindings) {
  const key = (action) => Keymap.keyFor(action, bindings);
  const parts = [`${key('message.reply')} reply`, `${key('message.react')} react`, `${key('message.delete')} delete`];
  const media = Timeline.media(message);
  // A poll has nothing for the open key to open: voting is its own key,
  // added below, only while it is still open.
  if (media && media.kind === 'voice') parts.push(`${key('message.open')} play`);
  else if (media && media.kind !== 'poll') parts.push(`${key('message.open')} open`);
  if (links(message).length > 0) parts.push(`${key('message.openLink')} open link`);
  if (Timeline.replyTo(message)) parts.push(`${key('message.goToQuote')} go to quote`);
  if (message.outgoing && message.status === 'failed') parts.push(`${key('message.retry')} retry`);
  if (media && media.kind === 'poll' && media.poll && !media.poll.closed) parts.push(`${key('message.vote')} vote`);
  return parts.join(' · ');
}

// links returns every URL the message's open key (o) can reach: the link
// preview's own URL when it carries one, since that is what the mouse
// opens, otherwise every http(s) link found in its text, in the order
// they appear.
function links(message) {
  const media = Timeline.media(message);
  if (media && media.kind === 'link' && media.url) {
    return [media.url];
  }

  const text = message && message.text ? message.text : '';
  const found = text.match(LINK_PATTERN) || [];
  return found.map(stripTrailingPunctuation);
}

// stripTrailingPunctuation drops sentence punctuation a link's own text
// swept up, such as the period ending "see http://x.com." or the
// closing parenthesis of "(http://x.com)", the same trim Format.js's
// linkify applies to a link found in escaped text.
function stripTrailingPunctuation(url) {
  return url.match(/^(.*?)[.,;:!?)]*$/)[1];
}

// primaryLink returns the one link the open key (o) opens when the
// message carries exactly one: the mouse path's own choice, the link
// preview's URL or the first link in the text. Callers needing every
// link, for a message with several, use links() instead.
function primaryLink(message) {
  const found = links(message);
  return found.length > 0 ? found[0] : '';
}

// atOldest reports whether currentId is already the oldest loaded
// message, the moment pressing k should fetch more history instead of
// moving the highlight: the moment ConversationController.loadOlder()
// takes over, rather than Highlight.older() clamping on an id it cannot
// move past yet.
function atOldest(ids, currentId) {
  return ids.length > 0 && ids[ids.length - 1] === currentId;
}

// afterWriting decides what Escape should highlight once it leaves
// writing mode: jumpId, a message a command-palette jump landed on and
// nothing has invalidated since, when it is still loaded, so the search
// that opened the conversation is not lost the moment the user looks
// around; otherwise the newest loaded message, the same as opening a
// conversation fresh. ids is newest first, so that is ids[0].
function afterWriting(ids, jumpId) {
  if (jumpId && ids.includes(jumpId)) return jumpId;
  return ids.length > 0 ? ids[0] : '';
}
