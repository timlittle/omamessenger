import QtQuick
import "../lib/Highlight.js" as Highlight
import "../lib/Timeline.js" as Timeline

// Owns the highlighted message in an open conversation: the id j/k move
// through, by id rather than index, so loading older history or a new
// message arriving never moves it to a different message. Split out of
// ConversationController to keep that file within the size guideline.
// Every function here reads a timeline passed in by the caller, rather
// than holding one itself, since ConversationController's timeline is
// reset for each conversation it opens.
//
// QtObject rather than Item: it holds no child objects.
QtObject {
  id: root

  // highlightedId is the message id j/k move through. It starts on the
  // newest message each time a conversation (re)loads.
  property string highlightedId: ""

  // _pendingId is a message id openMessage asked to land on, applied once
  // the conversation's initial page has loaded (see applyInitial): the
  // message is not in timeline yet at the moment requestInitial runs,
  // since loading it is what the caller's loadInitial is about to do.
  property string _pendingId: ""

  // scrollRequested asks the caller to scroll the message view to a
  // loaded message, by its local id.
  signal scrollRequested(string id)

  // linksRequested asks the caller to open the highlighted message's
  // link: Qt.openUrlExternally on it directly when there is exactly one,
  // or let the user choose among several when there is more than one.
  signal linksRequested(var urls)

  // reset moves the highlight to timeline's newest loaded message.
  // Idempotent: calling it again while already there changes nothing.
  function reset(timeline: var): void {
    root.highlightedId = timeline.newestId();
  }

  // clear empties the highlight, for a conversation that is closing.
  function clear(): void {
    root.highlightedId = "";
  }

  // requestInitial records messageId as the one to land on once
  // timeline's initial page has loaded, for applyInitial to pick up.
  function requestInitial(messageId: string): void {
    root._pendingId = messageId;
  }

  // applyInitial runs once a conversation's initial page has loaded: it
  // lands on whatever requestInitial asked for, if that message turned
  // out to be in the page, otherwise the newest message, same as reset
  // on its own. Best effort: a message older than the page that loads is
  // not found, and the highlight falls back to the newest message.
  function applyInitial(timeline: var): void {
    const pending = root._pendingId;
    root._pendingId = "";

    if (pending && timeline.find(pending)) {
      root.highlightedId = pending;
      root.scrollRequested(pending);
      return;
    }

    root.reset(timeline);
  }

  // moveOlder moves the highlight one message toward older, stopping at
  // the oldest loaded message rather than overscrolling: it returns true
  // there instead, so the caller can fetch more history, leaving the
  // highlight where it is until that page arrives.
  function moveOlder(timeline: var): bool {
    const ids = timeline.ids();
    if (ids.length === 0) return false;
    if (Highlight.atOldest(ids, root.highlightedId)) return true;

    root.highlightedId = Highlight.older(ids, root.highlightedId);
    root.scrollRequested(root.highlightedId);
    return false;
  }

  // moveNewer moves the highlight one message toward newer, stopping at
  // the newest loaded message the same way moveOlder stops at the oldest.
  function moveNewer(timeline: var): void {
    const ids = timeline.ids();
    if (ids.length === 0) return;

    root.highlightedId = Highlight.newer(ids, root.highlightedId);
    root.scrollRequested(root.highlightedId);
  }

  // openLink finds the highlighted message's link or links (the link
  // preview's own URL, or every link in its text when it has no preview)
  // and asks the caller to open them: nothing happens when it carries
  // none.
  function openLink(timeline: var): void {
    const message = timeline.find(root.highlightedId);
    if (!message) return;

    const found = Highlight.links(message);
    if (found.length > 0) root.linksRequested(found);
  }

  // goToQuote moves the highlight to the message the highlighted one
  // replies to, and scrolls to it, the same lookup by remote id the
  // quote's own click already uses. Nothing happens when it answers
  // nothing, or the quoted message is not loaded, the same as that click.
  function goToQuote(timeline: var): void {
    const message = timeline.find(root.highlightedId);
    const quote = message ? Timeline.replyTo(message) : null;
    if (!quote || !quote.remoteId) return;

    const id = timeline.localIdForRemote(quote.remoteId);
    if (!id) return;

    root.highlightedId = id;
    root.scrollRequested(id);
  }
}
