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

  // upsert updates one message in place, or inserts it where its time puts
  // it when it is not loaded yet: new messages at the top, and older
  // history fetched from the service further down.
  function upsert(message: var, isGroup: bool): void {
    const loaded = root._snapshot();
    const at = loaded.findIndex((m) => m.id === message.id);
    if (at !== -1) messagesModel.set(at, Timeline.row(Object.assign({ mediaPath: loaded[at].mediaPath }, message)));
    else messagesModel.insert(Timeline.insertIndex(loaded, message), Timeline.row(message));

    root._recomputeAnnotations(isGroup);
  }

  // setMediaPath records where a message's downloaded media is.
  function setMediaPath(id: string, path: string): void {
    const at = root._snapshot().findIndex((m) => m.id === id);
    if (at !== -1) messagesModel.setProperty(at, "mediaPath", path);
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

  // _appendOlder adds a messages.list page to the newest-first model,
  // each message where its time puts it: older history the helper fetched
  // for this page may already have arrived as events, so the page is not
  // always older than what is shown. Messages already shown are skipped.
  function _appendOlder(page: var, isGroup: bool): void {
    const loaded = root._snapshot();
    const shown = new Set(loaded.map((m) => m.id));
    for (const m of page.slice().reverse()) {
      if (shown.has(m.id)) continue;

      const at = Timeline.insertIndex(loaded, m);
      loaded.splice(at, 0, m);
      messagesModel.insert(at, Timeline.row(m));
    }
    root._recomputeAnnotations(isGroup);
  }

  // _snapshot copies the loaded messages, newest first.
  function _snapshot(): var {
    const out = [];
    for (let i = 0; i < messagesModel.count; i++) out.push(messagesModel.get(i));
    return out;
  }

  // _recomputeAnnotations rebuilds the day separators and grouping from
  // the current message list. isGroup is read from the caller each time,
  // since this component holds no conversation state of its own.
  function _recomputeAnnotations(isGroup: bool): void {
    root.annotations = Timeline.annotate(root._snapshot(), isGroup, Date.now());
  }

  ListModel { id: messagesModel }
}
