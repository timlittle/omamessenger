import QtQuick
import "../lib/Rail.js" as Rail
import "../lib/Selection.js" as Selection
import "../lib/ListSync.js" as ListSync
import "../lib/Actions.js" as Actions
import "../lib/Rpc.js" as Rpc

// Owns the rail filter, search and the visible conversation list: the
// only controller that calls conversations.list and conversations.setMuted.
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
  readonly property var railItems: Rail.items(root.service ? root.service.accounts : [], root._all)

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

  // _all holds every conversation the helper reported, unfiltered by the
  // rail or a search.
  property var _all: []

  // _searchResults holds the server's matches for the active query.
  property var _searchResults: []

  // focusRequested asks the caller to move keyboard focus into the search
  // field, after search.focus set searchFocused.
  signal focusRequested()

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
      "unread.next": () => root._select(Selection.nextUnread(root._visible(), root.selectedId)),
      "chat.mute": () => root._toggleMute(),
      "search.focus": () => { root.searchFocused = true; root.focusRequested(); }
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
  function _visible(): var {
    const source = root.query.length > 0 ? root._searchResults : root._all;
    return Rail.filter(source, root.railKey);
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
    const state = root.service.uiState;
    const id = (state.pane === "conversation" && state.activeId) ? state.activeId : root.selectedId;
    const conversation = id ? root.findConversation(id) : null;
    if (!conversation) return;

    root.service.request("conversations.setMuted", { conversationId: id, muted: !conversation.muted }, function(error, result) {
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

    root.service.request("conversations.list", { query: root.query }, function(error, result) {
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
  // results too.
  function _applyConversationUpdated(conversation: var): void {
    root._all = ListSync.upsertById(root._all, conversation, (a, b) => b.lastActivity - a.lastActivity);

    if (root.query.length > 0 && root._searchResults.some((c) => c.id === conversation.id)) {
      root._searchResults = ListSync.upsertById(root._searchResults, conversation, (a, b) => b.lastActivity - a.lastActivity);
    }

    root._syncModel();
  }

  // _syncModel brings the ListModel in line with the current visible set,
  // in place: structural changes through ListSync.planSync, then a field
  // refresh so unread counts and previews stay current.
  function _syncModel(): void {
    const visible = root._visible();
    const oldIds = [];
    for (let i = 0; i < listModel.count; i++) oldIds.push(listModel.get(i).id);

    for (const op of ListSync.planSync(oldIds, visible.map((c) => c.id))) root._applyOp(op, visible);
    for (let i = 0; i < visible.length; i++) listModel.set(i, root._row(visible[i]));
  }

  // _row is a conversation as a list row. The helper leaves out match when
  // nothing matched a search, but every row needs every field the view
  // requires.
  function _row(conversation: var): var {
    return Object.assign({}, conversation, { match: conversation.match ?? "" });
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

  Component.onCompleted: {
    if (!root.service) return;

    const state = root.service.uiState;
    root.railKey = state.railKey || "all";
    root.selectedId = state.selectedId || "";
    root.query = state.query || "";
    if (root.service.status === "ready") root._loadAll();
  }

  ListModel { id: listModel }

  Connections {
    target: root.service

    function onStatusChanged() {
      if (root.service.status === "ready") root._loadAll();
    }

    function onEvent(name, data) {
      if (name === "conversation.updated") root._applyConversationUpdated(data);
      else if (name === "unread.changed") root._syncModel();
    }
  }
}
