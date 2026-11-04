import QtQuick
import "../lib/Timeline.js" as Timeline
import "../lib/Rpc.js" as Rpc

// The open conversation's loaded messages: a ListModel kept newest-first,
// its day/grouping annotations, and paging through messages.list. Split
// out of ConversationController to keep that file within the size
// guideline; it is still controller code, not a view, so it calls
// service.request directly.
//
// messagesModel is capped at Timeline.MAX_LOADED_ROWS so a flood of
// live messages can't grow it without bound. _order and _index are
// cheap side records every lookup below uses instead of scanning the model.
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

  // notice is a brief, non-error confirmation for something that just
  // worked, such as copying a message's photo or video to the
  // clipboard or saving it to the downloads directory (see
  // MediaController). Shown in the same banner row as lastError, which
  // always takes priority over it while both are set; cleared the same
  // way, by Panel's banner timer or dismissing it.
  property string notice: ""

  // _loadingMore guards messages.list so only one older page loads at once.
  property bool _loadingMore: false

  // _order is every loaded message's {id, created}, newest first, kept in
  // lockstep with messagesModel.
  property var _order: []

  // _index maps a loaded message's id to its position, so lookups are O(1).
  property var _index: ({})

  // initialLoaded fires once loadInitial's page has been applied to the
  // model, for whoever resets the highlighted message to the newest one
  // each time a conversation (re)opens.
  signal initialLoaded()

  // reset clears the loaded messages, for a conversation about to load.
  // Opening a conversation is a fresh start, so any error or notice left
  // over from whatever was open before goes with it, rather than
  // lingering over this one until something else happens to clear it.
  function reset(): void {
    messagesModel.clear();
    root._order = [];
    root._index = ({});
    root.hasMore = true;
    root.historyUnavailable = false;
    root.annotations = [];
    root._loadingMore = false;
    root.lastError = "";
    root.notice = "";
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

      root.lastError = "";
      root.hasMore = result.hasMore;
      root.historyUnavailable = !!result.historyUnavailable;
      root._appendOlder(result.messages, isGroup);
      root.initialLoaded();
    });
  }

  // loadOlder fetches the page before the oldest loaded message, guarded
  // so only one page loads at a time and only while hasMore. guard is
  // checked before applying the result, the same as loadInitial's, so a
  // reply that arrives after the conversation was closed or switched away
  // from is dropped instead of being spliced into whichever conversation
  // is open by then.
  function loadOlder(service: var, conversationId: string, guard: var, isGroup: bool): void {
    if (root._loadingMore || !root.hasMore || messagesModel.count === 0) return;

    root._loadingMore = true;
    const before = root._order[root._order.length - 1].id;
    service.request("messages.list", { conversationId: conversationId, before: before, limit: 50 }, function(error, result) {
      root._loadingMore = false;
      if (error) { root.lastError = Rpc.errorText(error); return; }
      if (!guard()) return;

      root.lastError = "";
      root.hasMore = result.hasMore;
      root.historyUnavailable = !!result.historyUnavailable;
      root._appendOlder(result.messages, isGroup);
    });
  }

  // upsert updates one message in place, or inserts it where its time puts
  // it when it is not loaded yet. Evicts the oldest rows once
  // Timeline.MAX_LOADED_ROWS is exceeded, even protectId (see
  // docs/decisions.md); returns true when protectId was the row evicted,
  // so the caller can move the highlight off it.
  function upsert(message: var, isGroup: bool, protectId: string): bool {
    const result = Timeline.applyUpsert(root._order, root._index, message, Timeline.MAX_LOADED_ROWS, protectId);
    root._order = result.order;
    root._index = result.index;

    if (result.update) {
      const merged = Timeline.mergeRow(messagesModel.get(result.at), message);
      messagesModel.set(result.at, Timeline.row(merged));
    } else {
      messagesModel.insert(result.at, Timeline.row(message));
      root._insertAnnotation(result.at);
      if (result.evicted > 0) {
        messagesModel.remove(result.order.length, result.evicted);
        root._trimAnnotations(result.order.length);
        root.hasMore = true;
      }
    }

    root._reannotateTouched(result.touched, isGroup);
    return !!result.protectIdEvicted;
  }

  // remove drops a message from the model by id, doing nothing when it is
  // not loaded: the service already removed it, so there is nothing left
  // to find.
  function remove(id: string, isGroup: bool): void {
    const result = Timeline.applyRemove(root._order, root._index, id);
    if (!result.removed) return;

    root._order = result.order;
    root._index = result.index;
    messagesModel.remove(result.at);
    root._removeAnnotation(result.at);
    root._reannotateTouched(result.touched, isGroup);
  }

  // messageById returns a loaded message's full data, or null when it is
  // not loaded.
  function messageById(id: string): var {
    const at = root._index[id];
    return at !== undefined ? messagesModel.get(at) : null;
  }

  // localIdForRemote returns the loaded message id whose remote id is
  // remoteId, or "" when it is not loaded.
  function localIdForRemote(remoteId: string): string {
    const row = root._snapshot().find((m) => m.remoteId === remoteId);
    return row ? row.id : "";
  }

  // mediaPath returns where a message's downloaded media is, or "".
  function mediaPath(id: string): string {
    const at = root._index[id];
    return at !== undefined ? messagesModel.get(at).mediaPath : "";
  }

  // mediaPlayPath returns the in-window play copy a play fetch produced for
  // a video, voice note or audio message, or "" (see docs/api.md's playPath).
  function mediaPlayPath(id: string): string {
    const at = root._index[id];
    return at !== undefined ? messagesModel.get(at).mediaPlayPath : "";
  }

  // mediaPlayable returns whether mediaPlayPath is set.
  function mediaPlayable(id: string): bool {
    const at = root._index[id];
    return at !== undefined ? !!messagesModel.get(at).mediaPlayable : false;
  }

  // mediaPlayKnown returns whether a play fetch has actually answered for
  // this row, so mediaPlayPath can be trusted rather than treated as unset.
  function mediaPlayKnown(id: string): bool {
    const at = root._index[id];
    return at !== undefined ? !!messagesModel.get(at).mediaPlayKnown : false;
  }

  // mediaPreparing returns whether a play fetch for this row is in flight,
  // so the control can show "Preparing…" during a transcode.
  function mediaPreparing(id: string): bool {
    const at = root._index[id];
    return at !== undefined ? !!messagesModel.get(at).mediaPreparing : false;
  }

  // media returns a message's parsed photo or video media (kind, width,
  // height, duration, thumb), or null when it has none or is not loaded.
  function media(id: string): var {
    const at = root._index[id];
    return at !== undefined ? Timeline.media(messagesModel.get(at)) : null;
  }

  // photoNeighbor returns the id of the nearest other loaded message with
  // a photo, relative to id: delta > 0 looks toward newer messages, delta
  // < 0 toward older ones. It returns "" when id is not loaded or there is
  // no such neighbour.
  function photoNeighbor(id: string, delta: int): string {
    const at = root._index[id];
    if (at === undefined) return "";

    // The array is newest-first, so moving toward newer messages steps
    // backward through it.
    const step = delta > 0 ? -1 : 1;
    for (let i = at + step; i >= 0 && i < messagesModel.count; i += step) {
      const candidate = Timeline.media(messagesModel.get(i));
      if (candidate && candidate.kind === "photo") return messagesModel.get(i).id;
    }
    return "";
  }

  // setMediaPath records where a message's downloaded media is.
  function setMediaPath(id: string, path: string): void {
    const at = root._index[id];
    if (at !== undefined) messagesModel.setProperty(at, "mediaPath", path);
  }

  // setMediaPlayPath records a play fetch's result and marks this row's
  // play state known (see mediaPlayKnown).
  function setMediaPlayPath(id: string, playPath: string): void {
    const at = root._index[id];
    if (at === undefined) return;

    messagesModel.setProperty(at, "mediaPlayPath", playPath);
    messagesModel.setProperty(at, "mediaPlayable", !!playPath);
    messagesModel.setProperty(at, "mediaPlayKnown", true);
  }

  // setMediaPreparing records whether a play fetch for this row is in flight.
  function setMediaPreparing(id: string, preparing: bool): void {
    const at = root._index[id];
    if (at !== undefined) messagesModel.setProperty(at, "mediaPreparing", preparing);
  }

  // mediaRisky returns the helper's own verdict on whether a message's
  // downloaded media is active or executable content that must be
  // confirmed before it opens externally (see media.fetch's risky in
  // docs/api.md): false before it has answered for a row still here,
  // and true once the row is gone, since a verdict that cannot be found
  // is not one that said safe. Never computed here
  // from the message's own media object: only media.fetch's result,
  // through setMediaRisky, ever sets it.
  function mediaRisky(id: string): bool {
    const at = root._index[id];
    return at !== undefined ? !!messagesModel.get(at).mediaRisky : true;
  }

  // setMediaRisky records the helper's own risky verdict for a
  // message's downloaded media, from media.fetch's result.
  function setMediaRisky(id: string, risky: bool): void {
    const at = root._index[id];
    if (at !== undefined) messagesModel.setProperty(at, "mediaRisky", risky);
  }

  // setMediaFailed records whether a message's last media fetch failed,
  // so a photo with no preview shows "Photo unavailable", or a voice
  // note shows "Unavailable", instead of staying an empty box forever.
  // reason is the failure's safe category (see server/errors.go), kept
  // alongside failed so the voice note player can name it; it is
  // ignored, and left as whatever it was, when failed is false.
  function setMediaFailed(id: string, failed: bool, reason: string): void {
    const at = root._index[id];
    if (at === undefined) return;

    messagesModel.setProperty(at, "mediaFailed", failed);
    messagesModel.setProperty(at, "mediaFailedReason", failed ? reason : "");
  }

  // ids returns every loaded message's id, newest first, for moving the
  // highlighted message: the highlight is always one of these ids, never
  // an index, so a reorder or a new page never moves it to the wrong one.
  function ids(): var {
    return root._order.map((o) => o.id);
  }

  // newestId returns the newest loaded message's id, or "" when none is
  // loaded, for resetting the highlight when a conversation (re)loads.
  function newestId(): string {
    return root._order.length > 0 ? root._order[0].id : "";
  }

  // find returns a loaded message by id, or null when it is not loaded.
  function find(id: string): var {
    const at = root._index[id];
    return at !== undefined ? messagesModel.get(at) : null;
  }

  // snapshot returns every loaded message as a plain array, newest
  // first, for a caller outside this file that needs array methods
  // (find, some, …) a QML ListModel does not have: model itself stays
  // the raw ListModel, for views to bind to directly.
  function snapshot(): var {
    return root._snapshot();
  }

  // setReactions replaces a message's reaction chips in place, for
  // instant feedback before the server's message.updated event confirms
  // the real ones. The role holds reactions as a JSON string, the same
  // way row() stores media and replyTo: writing an array of objects to
  // an existing ListModel row, rather than at its first insert, silently
  // leaves the role empty.
  function setReactions(id: string, reactions: var): void {
    const at = root._index[id];
    if (at !== undefined) messagesModel.setProperty(at, "reactions", JSON.stringify(reactions));
  }

  // setMedia replaces a message's media in place, the same way
  // setReactions does for its own role, for a poll's instant local vote
  // feedback before the server's message.updated event confirms it.
  function setMedia(id: string, media: var): void {
    const at = root._index[id];
    if (at !== undefined) messagesModel.setProperty(at, "media", JSON.stringify(media));
  }

  // _appendOlder adds a messages.list page to the newest-first model,
  // each message where its time puts it: older history the helper fetched
  // for this page may already have arrived as events, so the page is not
  // always older than what is shown. Messages already shown are skipped.
  // Also capped at Timeline.MAX_LOADED_ROWS, but unlike upsert's overflow,
  // this forces hasMore false since paging further back would only
  // re-fetch discarded rows.
  function _appendOlder(page: var, isGroup: bool): void {
    const loaded = root._snapshot();
    const shown = new Set(loaded.map((m) => m.id));
    const order = root._order.slice();
    for (const m of page.slice().reverse()) {
      if (shown.has(m.id)) continue;

      const at = Timeline.insertIndex(loaded, m);
      loaded.splice(at, 0, m);
      order.splice(at, 0, { id: m.id, created: m.created });
      messagesModel.insert(at, Timeline.row(m));
      shown.add(m.id);
    }

    const windowed = Timeline.windowNewest(order, Timeline.MAX_LOADED_ROWS);
    const evicted = order.length - windowed.length;
    if (evicted > 0) {
      messagesModel.remove(windowed.length, evicted);
      loaded.length = windowed.length;
      root.hasMore = false;
    }

    root._order = windowed;
    root._index = Timeline.reindex(windowed);
    root.annotations = Timeline.annotate(loaded, isGroup, Date.now());
  }

  // _snapshot copies the loaded messages, newest first.
  function _snapshot(): var {
    const out = [];
    for (let i = 0; i < messagesModel.count; i++) out.push(messagesModel.get(i));
    return out;
  }

  // _insertAnnotation keeps root.annotations in step with messagesModel
  // when upsert inserts a row at position at, with a placeholder that
  // _reannotateTouched overwrites right after.
  function _insertAnnotation(at: int): void {
    const next = root.annotations.slice();
    next.splice(at, 0, null);
    root.annotations = next;
  }

  // _removeAnnotation is _insertAnnotation's mirror for remove.
  function _removeAnnotation(at: int): void {
    const next = root.annotations.slice();
    next.splice(at, 1);
    root.annotations = next;
  }

  // _trimAnnotations drops every annotation past the first keep rows,
  // mirroring the rows upsert's windowing just evicted.
  function _trimAnnotations(keep: int): void {
    if (root.annotations.length > keep) root.annotations = root.annotations.slice(0, keep);
  }

  // _reannotateTouched recomputes the annotation at each of indices only,
  // so a live event's cost stays proportional to the rows it actually touched.
  function _reannotateTouched(indices: var, isGroup: bool): void {
    if (indices.length === 0) return;

    const nowMs = Date.now();
    const next = root.annotations.slice();
    const count = messagesModel.count;
    for (const i of indices) {
      if (i < 0 || i >= count) continue;

      const msg = messagesModel.get(i);
      const older = (i + 1 < count) ? messagesModel.get(i + 1) : null;
      next[i] = Timeline.annotateForPair(msg, older, isGroup, nowMs);
    }
    root.annotations = next;
  }

  ListModel { id: messagesModel }
}
