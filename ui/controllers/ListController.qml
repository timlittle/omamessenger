import QtQuick
import "../lib/Rail.js" as Rail
import "../lib/Selection.js" as Selection
import "../lib/ListSync.js" as ListSync
import "../lib/Actions.js" as Actions
import "../lib/Rpc.js" as Rpc

// Owns the rail filter, search and the visible conversation list: the
// only controller that calls conversations.list, conversations.setMuted,
// conversations.setPinned, conversations.setArchived and
// conversations.setHidden.
// The list itself is a ListModel kept in sync in place with ListSync, so
// opening or scrolling never resets because of an unrelated event.
//
// Item rather than QtObject: only a type with a default property can hold
// the ListModel and Connections children below without naming them.
Item {
  id: root

  // service is the Service instance that owns the helper connection.
  property var service: null

  // model is the conversations visible under the current rail filter and
  // search, newest first.
  readonly property alias model: listModel

  // railItems is the rail's entries, from Rail.items.
  readonly property var railItems: Rail.items(root.service ? root.service.accounts : [], root._all,
    root.service ? root.service.services : [])

  // all is every conversation the helper reported, for the palette's
  // jump-to-conversation list.
  readonly property var all: root._all

  // railKey is the active rail filter, restored from service.uiState.
  property string railKey: "all"

  // query is the active search text, restored from service.uiState.
  property string query: ""

  // selectedId is the list cursor, restored from service.uiState.
  property string selectedId: ""

  // searchFocused is true while the search field holds keyboard focus.
  property bool searchFocused: false

  // lastError is the safe text of the most recent request failure.
  property string lastError: ""

  // showAll shows every chat the standard list would otherwise fold away:
  // older than a month, hidden by the user, or archived with the service,
  // each dimmed, unless they are unread or open.
  property bool showAll: false

  // unreadView shows every unread chat across every service and account,
  // overriding the rail filter, like Slack's all-unreads view. It is not
  // saved to uiState: unlike the rail filter and search, it is a momentary
  // view the Ctrl+Shift+A shortcut applies after opening the window, not a
  // durable preference to restore next time.
  property bool unreadView: false

  // hiddenCount is how many chats the standard list hides right now.
  property int hiddenCount: 0

  // _all holds every conversation the helper reported, unfiltered by the
  // rail or a search.
  property var _all: []

  // _searchResults holds the server's matches for the active query.
  property var _searchResults: []

  // conversationFolded fires when id has just dropped out of the standard
  // list (hidden or archived while visible), so a caller showing it open
  // can close it instead of following the list's own reselect onto a
  // neighbour, which would mark that neighbour read as a side effect.
  signal conversationFolded(string id)

  // handles reports whether this controller owns action.
  function handles(action: string): bool {
    return Actions.owner(action) === "list";
  }

  // run performs action, the only entry point a key router needs.
  function run(action: string): void {
    const handlers = {
      "rail.all": () => root._setRail("all"),
      "rail.whatsapp": () => root._setRail("service:whatsapp"),
      "rail.telegram": () => root._setRail("service:telegram"),
      "rail.next": () => root._setRail(Rail.next(root.railItems, root.railKey, 1)),
      "rail.prev": () => root._setRail(Rail.next(root.railItems, root.railKey, -1)),
      "cursor.down": () => root._select(Selection.move(root.visibleIds(), root.selectedId, 1)),
      "cursor.up": () => root._select(Selection.move(root.visibleIds(), root.selectedId, -1)),
      "cursor.top": () => root._select(Selection.edge(root.visibleIds(), "top")),
      "cursor.bottom": () => root._select(Selection.edge(root.visibleIds(), "bottom")),
      "chat.mute": () => root._toggleMute(),
      "chat.pin": () => root._togglePin(),
      "chat.archive": () => root._toggleArchive(),
      "chat.hide": () => root._toggleHidden(),
      "list.showAll": () => root.setShowAll(!root.showAll),
      "list.unread": () => root.setUnreadView(!root.unreadView)
    };

    const handler = handlers[action];
    if (handler) handler();
  }

  // setQuery updates the search text and re-runs it against the helper.
  function setQuery(text: string): void {
    if (root.query === text) return;

    root.query = text;
    root._saveUiState({ query: text });
    root._search();
  }

  // setShowAll shows or hides every chat the standard list folds away
  // (older, hidden or archived), and remembers it.
  function setShowAll(show: bool): void {
    root.showAll = show;
    root._saveUiState({ showAll: show });
    root._syncModel();
  }

  // setUnreadView shows or hides the all-unreads view. It is left out of
  // uiState on purpose; see the unreadView property doc.
  function setUnreadView(show: bool): void {
    root.unreadView = show;
    root._syncModel();
  }

  // clearSearch empties the query and shows the rail-filtered list again.
  function clearSearch(): void {
    root.setQuery("");
  }

  // leaveSearch drops keyboard focus from the search field, without
  // changing the query.
  function leaveSearch(): void {
    root.searchFocused = false;
  }

  // selectId moves the list cursor to id without opening it.
  function selectId(id: string): void {
    root._select(id);
  }

  // setRail switches the active rail filter to key, for a click on a rail
  // entry the keyboard table has no binding for, such as one account
  // among several under the same service.
  function setRail(key: string): void {
    root._setRail(key);
  }

  // visibleIds returns the ids of the rows currently shown, in order.
  function visibleIds(): var {
    return root._visible().map((c) => c.id);
  }

  // findConversation returns the conversation with id, from whichever of
  // the full list or the search results holds it, or null.
  function findConversation(id: string): var {
    return root._all.find((c) => c.id === id) ?? root._searchResults.find((c) => c.id === id) ?? null;
  }

  // _visible returns the conversations the rail filter currently covers,
  // from the search results while a query is active, otherwise the full
  // list.
  // A search looks through every chat regardless of any other view;
  // otherwise the all-unreads view, when on, overrides the rail filter
  // rather than narrowing it; otherwise only the standard list shows,
  // unless show-all is on.
  function _visible(): var {
    if (root.query.length > 0) return Rail.filter(root._searchResults, root.railKey);
    if (root.unreadView) return Rail.unreadConversations(root._all, root._keepId());

    const filtered = Rail.filter(root._all, root.railKey);
    if (root.showAll) return filtered;

    return Rail.standard(filtered, Date.now(), root._keepId());
  }

  // _keepId is the open conversation's id, which must never disappear
  // from the standard list or be dimmed in show-all.
  function _keepId(): string {
    return root.service ? root.service.uiState.activeId : "";
  }

  // _select moves the list cursor to id and remembers it in uiState.
  function _select(id: string): void {
    if (!id) return;

    root.selectedId = id;
    root._saveUiState({ selectedId: id });
  }

  // _setRail changes the active rail filter and resyncs the list.
  function _setRail(key: string): void {
    root.railKey = key;
    root._saveUiState({ railKey: key });
    root._syncModel();
  }

  // _toggleMute mutes or unmutes whichever conversation is contextually
  // current: the open conversation if one is showing, else the list cursor.
  function _toggleMute(): void {
    root._setOnCurrent("conversations.setMuted", "muted");
  }

  // _togglePin pins or unpins whichever conversation is contextually
  // current, the same way _toggleMute does.
  function _togglePin(): void {
    root._setOnCurrent("conversations.setPinned", "pinned");
  }

  // _toggleArchive archives or unarchives whichever conversation is
  // contextually current, the same way _toggleMute does.
  function _toggleArchive(): void {
    root._setOnCurrent("conversations.setArchived", "archived");
  }

  // _toggleHidden hides or unhides whichever conversation is contextually
  // current, the same way _toggleMute does. Hiding never reaches the
  // service: it is local to this computer only.
  function _toggleHidden(): void {
    root._setOnCurrent("conversations.setHidden", "hidden");
  }

  // _setOnCurrent flips boolean field on whichever conversation is
  // contextually current: the open conversation if one is showing, else
  // the list cursor, by calling method with {conversationId, <field>}.
  function _setOnCurrent(method: string, field: string): void {
    const state = root.service.uiState;
    const id = (state.pane === "conversation" && state.activeId) ? state.activeId : root.selectedId;
    const conversation = id ? root.findConversation(id) : null;
    if (!conversation) return;

    const params = { conversationId: id };
    params[field] = !conversation[field];
    root.service.request(method, params, function(error, result) {
      if (error) { root.lastError = Rpc.errorText(error); return; }
      root._applyConversationUpdated(result);
    });
  }

  // _search asks the helper for conversations matching the query, or
  // falls back to the unfiltered list once the query is empty.
  function _search(): void {
    if (root.query.length === 0) {
      root._searchResults = [];
      root._syncModel();
      return;
    }

    // The helper answers requests concurrently, so a reply for an earlier
    // query can arrive after the latest one; only the latest counts.
    const query = root.query;
    root.service.request("conversations.list", { query: query }, function(error, result) {
      if (query !== root.query) return;
      if (error) { root.lastError = Rpc.errorText(error); return; }
      root._searchResults = result ?? [];
      root._syncModel();
    });
  }

  // _loadAll fetches every conversation, run once the helper is ready.
  function _loadAll(): void {
    root.service.request("conversations.list", {}, function(error, result) {
      if (error) { root.lastError = Rpc.errorText(error); return; }
      root._all = result ?? [];
      if (root.query.length > 0) root._search();
      else root._syncModel();
    });
  }

  // _applyConversationUpdated upserts one conversation into the full list
  // and, while it is already part of the active search, into the search
  // results too, then folds it away at once if hiding or archiving just
  // dropped it from the standard list.
  function _applyConversationUpdated(conversation: var): void {
    const beforeIds = root.visibleIds();
    root._all = ListSync.upsertById(root._all, conversation, Rail.compareConversations);

    if (root.query.length > 0 && root._searchResults.some((c) => c.id === conversation.id)) {
      root._searchResults = ListSync.upsertById(root._searchResults, conversation, Rail.compareConversations);
    }

    root._syncModel();
    root._foldIfNeeded(conversation.id, beforeIds);
  }

  // _foldIfNeeded moves the highlight off id once its row has just
  // vanished from the standard list: the neighbouring row it left behind
  // takes the highlight, by id, never by index. A query or show-all keeps
  // every row in view, so this is a no-op then. The caller finds out
  // through conversationFolded, so it can close id instead of opening the
  // neighbour onto it, which would mark the neighbour read.
  function _foldIfNeeded(id: string, beforeIds: var): void {
    if (!beforeIds.includes(id) || root._visible().some((c) => c.id === id)) return;

    if (root.selectedId === id) {
      const neighbor = Selection.afterRemoval(beforeIds, id);
      root.selectedId = neighbor;
      root._saveUiState({ selectedId: neighbor });
    }

    root.conversationFolded(id);
  }

  // _start restores the saved list state and loads the conversations once
  // the service is there. showAll replaces the older showOlder and
  // showArchived keys; a state saved before that change still turns it on
  // if either was on, so no one's chats resurface unexpectedly.
  function _start(): void {
    if (!root.service) return;

    const state = root.service.uiState ?? {}; // a service still starting has none yet
    root.railKey = state.railKey || "all";
    root.selectedId = state.selectedId || "";
    root.query = state.query || "";
    root.showAll = state.showAll ?? (state.showOlder || state.showArchived || false);
    if (root.service.status === "ready") root._loadAll();
  }

  // _dropAccount removes a removed account's conversations from the list,
  // moves the cursor off any of them the same way folding one away does,
  // and reports each one that was visible so an open conversation among
  // them closes instead of lingering on a deleted account.
  function _dropAccount(accountId: string): void {
    const beforeIds = root.visibleIds();
    const removedIds = root._all.filter((c) => c.accountId === accountId).map((c) => c.id);

    root._all = root._all.filter((c) => c.accountId !== accountId);
    root._searchResults = root._searchResults.filter((c) => c.accountId !== accountId);
    root._syncModel();

    if (removedIds.includes(root.selectedId)) {
      const neighbor = Selection.afterRemoval(beforeIds, root.selectedId);
      root.selectedId = neighbor;
      root._saveUiState({ selectedId: neighbor });
    }

    for (const id of removedIds) {
      if (beforeIds.includes(id)) root.conversationFolded(id);
    }
  }

  // _syncModel brings the ListModel in line with the current visible set,
  // in place: structural changes through ListSync.planSync, then a field
  // refresh so unread counts and previews stay current.
  function _syncModel(): void {
    const visible = root._visible();
    const filtered = Rail.filter(root._all, root.railKey);
    root.hiddenCount = root.query.length > 0 || root.showAll || root.unreadView ? 0 : filtered.length - visible.length;
    const oldIds = [];
    for (let i = 0; i < listModel.count; i++) oldIds.push(listModel.get(i).id);

    for (const op of ListSync.planSync(oldIds, visible.map((c) => c.id))) root._applyOp(op, visible);
    for (let i = 0; i < visible.length; i++) listModel.set(i, root._row(visible[i]));
  }

  // _row is a conversation as a list row. The helper leaves out match when
  // nothing matched a search, but every row needs every field the view
  // requires. dimmed and dimLabel only mean anything in show-all, where
  // the standard list's own rules decide which rows get drawn muted.
  function _row(conversation: var): var {
    const keepId = root._keepId();
    return Object.assign({}, conversation, {
      match: conversation.match ?? "",
      dimmed: root.showAll && Rail.isDimmed(conversation, Date.now(), keepId),
      dimLabel: root.showAll ? Rail.dimLabel(conversation, Date.now(), keepId) : ""
    });
  }

  // _applyOp performs one ListSync operation on the ListModel.
  function _applyOp(op: var, visible: var): void {
    if (op.op === "remove") listModel.remove(op.index);
    else if (op.op === "insert") listModel.insert(op.index, root._row(visible.find((c) => c.id === op.id)));
    else if (op.op === "move") listModel.move(op.from, op.to, 1);
  }

  // _saveUiState merges patch into service.uiState as a new object, so a
  // copy someone read earlier never changes underneath them.
  function _saveUiState(patch: var): void {
    root.service.uiState = Object.assign({}, root.service.uiState, patch);
  }

  // Omarchy may hand over the service after the panel is created, or
  // replace it, so start-up runs whenever it arrives.
  onServiceChanged: root._start()
  Component.onCompleted: root._start()

  ListModel { id: listModel }

  Connections {
    target: root.service

    function onStatusChanged() {
      if (root.service.status === "ready") root._loadAll();
    }

    function onEvent(name, data) {
      if (name === "conversation.updated") root._applyConversationUpdated(data);
      else if (name === "unread.changed") root._syncModel();
      else if (name === "account.removed") root._dropAccount(data.accountId);
    }
  }
}
