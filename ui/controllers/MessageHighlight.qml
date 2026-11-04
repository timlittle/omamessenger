import QtQuick
import "../lib/Highlight.js" as Highlight
import "../lib/Timeline.js" as Timeline

// Owns the highlighted message in an open conversation, tracked by id
// rather than index so loading older history never moves it. j/k also
// reach past the newest message into the chat's own queued "send
// later" messages (see highlightedScheduledId).
QtObject {
  id: root

  // highlightedId is the message id j/k move through. It starts on the
  // newest message each time a conversation (re)loads.
  property string highlightedId: ""

  // highlightedScheduledId is the id of the queued "send later" message
  // j/k point at once they've moved past the newest message; mutually
  // exclusive with highlightedId.
  property string highlightedScheduledId: ""

  // _pendingId is a message id openMessage asked to land on once the
  // conversation's initial page has loaded (see applyInitial).
  property string _pendingId: ""

  // _jumpId is the message id a command-palette jump last landed on, so
  // leaveWriting can return to it; cleared on any other highlight move.
  property string _jumpId: ""

  // scrollRequested asks the caller to scroll the message view to a
  // loaded message, by its local id.
  signal scrollRequested(string id)

  // linksRequested asks the caller to open the highlighted message's link(s), as {url, displayText}.
  signal linksRequested(var urls)

  // reset moves the highlight to timeline's newest loaded message and drops any remembered jump target.
  function reset(timeline: var): void {
    root.highlightedId = timeline.newestId();
    root.highlightedScheduledId = "";
    root._jumpId = "";
  }

  // clear empties the highlight and any remembered jump target.
  function clear(): void {
    root.highlightedId = "";
    root.highlightedScheduledId = "";
    root._jumpId = "";
  }

  // clearJump drops a remembered command-palette jump target without touching the highlight itself.
  function clearJump(): void {
    root._jumpId = "";
  }

  // leaveWriting moves the highlight back to a remembered jump target
  // if still loaded, otherwise to the newest message.
  function leaveWriting(timeline: var): void {
    const target = Highlight.afterWriting(timeline.ids(), root._jumpId);
    if (target && target === root._jumpId) {
      root.highlightedId = target;
      root.highlightedScheduledId = "";
      root.scrollRequested(target);
      return;
    }

    root.reset(timeline);
  }

  // requestInitial records messageId as the one to land on once the initial page has loaded.
  function requestInitial(messageId: string): void {
    root._pendingId = messageId;
  }

  // applyInitial lands on whatever requestInitial asked for if found in
  // the loaded page, otherwise falls back to the newest message.
  function applyInitial(timeline: var): void {
    const pending = root._pendingId;
    root._pendingId = "";

    if (pending && timeline.find(pending)) {
      root.highlightedId = pending;
      root.highlightedScheduledId = "";
      root._jumpId = pending;
      root.scrollRequested(pending);
      return;
    }

    root.reset(timeline);
  }

  // moveOlder moves the highlight one message toward older, back out
  // of the queued "send later" messages onto loaded messages, then
  // returns true at the oldest so the caller can fetch more history.
  function moveOlder(timeline: var, scheduledIds: var): bool {
    if (root.highlightedScheduledId) {
      const index = scheduledIds.indexOf(root.highlightedScheduledId);
      if (index > 0) {
        root.highlightedScheduledId = scheduledIds[index - 1];
        return false;
      }

      root.highlightedScheduledId = "";
      root.highlightedId = timeline.newestId();
      root.scrollRequested(root.highlightedId);
      return false;
    }

    const ids = timeline.ids();
    if (ids.length === 0) return false;
    if (Highlight.atOldest(ids, root.highlightedId)) return true;

    root._jumpId = "";
    root.highlightedId = Highlight.older(ids, root.highlightedId);
    root.scrollRequested(root.highlightedId);
    return false;
  }

  // moveNewer moves the highlight one message toward newer, past the
  // newest loaded message into scheduledIds if there are any, clamping at the last one.
  function moveNewer(timeline: var, scheduledIds: var): void {
    if (root.highlightedScheduledId) {
      const index = scheduledIds.indexOf(root.highlightedScheduledId);
      if (index >= 0 && index < scheduledIds.length - 1) root.highlightedScheduledId = scheduledIds[index + 1];
      return;
    }

    const ids = timeline.ids();
    if (ids.length === 0) {
      if (scheduledIds.length > 0) { root.highlightedId = ""; root.highlightedScheduledId = scheduledIds[0]; }
      return;
    }

    const next = Highlight.newer(ids, root.highlightedId);
    if (next === root.highlightedId && scheduledIds.length > 0) {
      root.highlightedId = "";
      root.highlightedScheduledId = scheduledIds[0];
      return;
    }

    if (next !== root.highlightedId) root._jumpId = "";
    root.highlightedId = next;
    root.scrollRequested(root.highlightedId);
  }

  // openLink finds the highlighted message's link(s) and asks the
  // caller to open them, as the same {url, displayText} anchors the
  // bubble shows (see docs/decisions.md for why not re-paired).
  function openLink(timeline: var): void {
    const message = timeline.find(root.highlightedId);
    if (!message) return;

    const found = Highlight.links(message);
    if (found.length > 0) root.linksRequested(found);
  }

  // goToQuote moves the highlight to the message the highlighted one
  // replies to, if loaded, and scrolls to it.
  function goToQuote(timeline: var): void {
    const message = timeline.find(root.highlightedId);
    const quote = message ? Timeline.replyTo(message) : null;
    if (!quote || !quote.remoteId) return;

    const id = timeline.localIdForRemote(quote.remoteId);
    if (!id) return;

    root._jumpId = "";
    root.highlightedId = id;
    root.scrollRequested(id);
  }
}
