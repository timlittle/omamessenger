import QtQuick
import "../lib/Timeline.js" as Timeline
import "../lib/Rpc.js" as Rpc

// The open conversation's loaded messages: a ListModel kept newest-first,
// its day/grouping annotations, and paging through messages.list. Split
// out of ConversationController to keep that file within the size
// guideline; it is still controller code, not a view, so it calls
// service.request directly.
Item {
  id: root

  // model is the loaded timeline, newest first.
  readonly property alias model: messagesModel

  // annotations is Timeline.annotate's output, kept in step with model.
  property var annotations: []

  // hasMore is true while an older page of messages may still exist.
  property bool hasMore: true

  // lastError is the safe text of the most recent request failure.
  property string lastError: ""

  // _loadingMore guards messages.list so only one older page loads at once.
  property bool _loadingMore: false

  // reset clears the loaded messages, for a conversation about to load.
  function reset(): void {
    messagesModel.clear();
    root.hasMore = true;
    root.annotations = [];
    root._loadingMore = false;
  }

  // loadInitial replaces the model with conversationId's newest page.
  // guard is checked before applying the result, so a reply that arrives
  // after the conversation was closed or switched is dropped. isGroup
  // feeds the annotations, since this component holds no conversation
  // state of its own.
  function loadInitial(service: var, conversationId: string, guard: var, isGroup: bool): void {
    root.reset();
    service.request("messages.list", { conversationId: conversationId, limit: 50 }, function(error, result) {
      if (error) { root.lastError = Rpc.errorText(error); return; }
      if (!guard()) return;

      root.hasMore = result.hasMore;
      root._appendOlder(result.messages, isGroup);
    });
  }

  // loadOlder fetches the page before the oldest loaded message, guarded
  // so only one page loads at a time and only while hasMore.
  function loadOlder(service: var, conversationId: string, isGroup: bool): void {
    if (root._loadingMore || !root.hasMore || messagesModel.count === 0) return;

    root._loadingMore = true;
    const before = messagesModel.get(messagesModel.count - 1).id;
    service.request("messages.list", { conversationId: conversationId, before: before, limit: 50 }, function(error, result) {
      root._loadingMore = false;
      if (error) { root.lastError = Rpc.errorText(error); return; }

      root.hasMore = result.hasMore;
      root._appendOlder(result.messages, isGroup);
    });
  }

  // upsert updates one message in place, or inserts it as the newest when
  // it is not loaded yet.
  function upsert(message: var, isGroup: bool): void {
    for (let i = 0; i < messagesModel.count; i++) {
      if (messagesModel.get(i).id === message.id) {
        messagesModel.set(i, message);
        root._recomputeAnnotations(isGroup);
        return;
      }
    }

    messagesModel.insert(0, message);
    root._recomputeAnnotations(isGroup);
  }

  // newestFailedId returns the newest failed outgoing message's id, or ""
  // when none failed.
  function newestFailedId(): string {
    for (let i = 0; i < messagesModel.count; i++) {
      const m = messagesModel.get(i);
      if (m.outgoing && m.status === "failed") return m.id;
    }
    return "";
  }

  // _appendOlder adds a messages.list page, oldest-first as the helper
  // sends it, to the end of the newest-first model.
  function _appendOlder(page: var, isGroup: bool): void {
    for (const m of page.slice().reverse()) messagesModel.append(m);
    root._recomputeAnnotations(isGroup);
  }

  // _recomputeAnnotations rebuilds the day separators and grouping from
  // the current message list. isGroup is read from the caller each time,
  // since this component holds no conversation state of its own.
  function _recomputeAnnotations(isGroup: bool): void {
    const snapshot = [];
    for (let i = 0; i < messagesModel.count; i++) snapshot.push(messagesModel.get(i));
    root.annotations = Timeline.annotate(snapshot, isGroup, Date.now());
  }

  ListModel { id: messagesModel }
}
