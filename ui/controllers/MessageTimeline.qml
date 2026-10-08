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

  // historyUnavailable is true once a loadOlder page reported that the
  // service could not be reached for older history right now, such as a
  // WhatsApp account whose phone never answered; it resets on the next
  // successful page, including loadInitial's.
  property bool historyUnavailable: false

  // lastError is the safe text of the most recent request failure.
  property string lastError: ""

  // _loadingMore guards messages.list so only one older page loads at once.
  property bool _loadingMore: false

  // initialLoaded fires once loadInitial's page has been applied to the
  // model, for whoever resets the highlighted message to the newest one
  // each time a conversation (re)opens.
  signal initialLoaded()

  // reset clears the loaded messages, for a conversation about to load.
  function reset(): void {
    messagesModel.clear();
    root.hasMore = true;
    root.historyUnavailable = false;
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
      root.historyUnavailable = !!result.historyUnavailable;
      root._appendOlder(result.messages, isGroup);
      root.initialLoaded();
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
      root.historyUnavailable = !!result.historyUnavailable;
      root._appendOlder(result.messages, isGroup);
    });
  }

  // upsert updates one message in place, or inserts it where its time puts
  // it when it is not loaded yet: new messages at the top, and older
  // history fetched from the service further down.
  function upsert(message: var, isGroup: bool): void {
    const loaded = root._snapshot();
    const at = loaded.findIndex((m) => m.id === message.id);
    if (at !== -1) {
      const kept = { mediaPath: loaded[at].mediaPath, mediaFailed: loaded[at].mediaFailed, mediaFailedReason: loaded[at].mediaFailedReason };
      messagesModel.set(at, Timeline.row(Object.assign(kept, message)));
    } else {
      messagesModel.insert(Timeline.insertIndex(loaded, message), Timeline.row(message));
    }

    root._recomputeAnnotations(isGroup);
  }

  // remove drops a message from the model by id, doing nothing when it is
  // not loaded: the service already removed it, so there is nothing left
  // to find.
  function remove(id: string, isGroup: bool): void {
    const at = root._snapshot().findIndex((m) => m.id === id);
    if (at === -1) return;

    messagesModel.remove(at);
    root._recomputeAnnotations(isGroup);
  }

  // messageById returns a loaded message's full data, or null when it is
  // not loaded.
  function messageById(id: string): var {
    return root._snapshot().find((m) => m.id === id) ?? null;
  }

  // localIdForRemote returns the loaded message id whose remote id is
  // remoteId, or "" when it is not loaded: the user has not scrolled to
  // it, or the service has not assigned it one yet.
  function localIdForRemote(remoteId: string): string {
    const row = root._snapshot().find((m) => m.remoteId === remoteId);
    return row ? row.id : "";
  }

  // mediaPath returns where a message's downloaded media is, or "".
  function mediaPath(id: string): string {
    const row = root._snapshot().find((m) => m.id === id);
    return row ? row.mediaPath : "";
  }

  // media returns a message's parsed photo or video media (kind, width,
  // height, duration, thumb), or null when it has none or is not loaded.
  function media(id: string): var {
    const row = root._snapshot().find((m) => m.id === id);
    return row ? Timeline.media(row) : null;
  }

  // photoNeighbor returns the id of the nearest other loaded message with
  // a photo, relative to id: delta > 0 looks toward newer messages, delta
  // < 0 toward older ones. It returns "" when id is not loaded or there is
  // no such neighbour.
  function photoNeighbor(id: string, delta: int): string {
    const loaded = root._snapshot();
    const at = loaded.findIndex((m) => m.id === id);
    if (at === -1) return "";

    // The array is newest-first, so moving toward newer messages steps
    // backward through it.
    const step = delta > 0 ? -1 : 1;
    for (let i = at + step; i >= 0 && i < loaded.length; i += step) {
      const candidate = Timeline.media(loaded[i]);
      if (candidate && candidate.kind === "photo") return loaded[i].id;
    }
    return "";
  }

  // setMediaPath records where a message's downloaded media is.
  function setMediaPath(id: string, path: string): void {
    const at = root._snapshot().findIndex((m) => m.id === id);
    if (at !== -1) messagesModel.setProperty(at, "mediaPath", path);
  }

  // setMediaFailed records whether a message's last media fetch failed,
  // so a photo with no preview shows "Photo unavailable", or a voice
  // note shows "Unavailable", instead of staying an empty box forever.
  // reason is the failure's safe category (see server/errors.go), kept
  // alongside failed so the voice note player can name it; it is
  // ignored, and left as whatever it was, when failed is false.
  function setMediaFailed(id: string, failed: bool, reason: string): void {
    const at = root._snapshot().findIndex((m) => m.id === id);
    if (at === -1) return;

    messagesModel.setProperty(at, "mediaFailed", failed);
    messagesModel.setProperty(at, "mediaFailedReason", failed ? reason : "");
  }

  // ids returns every loaded message's id, newest first, for moving the
  // highlighted message: the highlight is always one of these ids, never
  // an index, so a reorder or a new page never moves it to the wrong one.
  function ids(): var {
    return root._snapshot().map((m) => m.id);
  }

  // newestId returns the newest loaded message's id, or "" when none is
  // loaded, for resetting the highlight when a conversation (re)loads.
  function newestId(): string {
    return messagesModel.count > 0 ? messagesModel.get(0).id : "";
  }

  // find returns a loaded message by id, or null when it is not loaded.
  function find(id: string): var {
    const at = root._snapshot().findIndex((m) => m.id === id);
    return at === -1 ? null : messagesModel.get(at);
  }

  // setReactions replaces a message's reaction chips in place, for
  // instant feedback before the server's message.updated event confirms
  // the real ones. The role holds reactions as a JSON string, the same
  // way row() stores media and replyTo: writing an array of objects to
  // an existing ListModel row, rather than at its first insert, silently
  // leaves the role empty.
  function setReactions(id: string, reactions: var): void {
    const at = root._snapshot().findIndex((m) => m.id === id);
    if (at !== -1) messagesModel.setProperty(at, "reactions", JSON.stringify(reactions));
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
