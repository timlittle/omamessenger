import QtQuick
import "../lib/Selection.js" as Selection
import "../lib/Actions.js" as Actions
import "../lib/Rpc.js" as Rpc

// Owns the open conversation: which one is open, sending, retrying,
// drafts and the typing indicator. The only controller that calls
// messages.send, messages.retry, conversations.markRead and ui.setFocus;
// paging through messages.list is delegated to the timeline child below.
//
// listController supplies the list cursor and the visible ids that
// chat.open, chat.next/prev and search.accept need; set it once, from
// whoever wires the controllers together.
//
// Item rather than QtObject: only a type with a default property can hold
// the timeline, Timer and Connections children below without naming them.
Item {
  id: root

  // _fetching holds the ids of messages whose media is downloading.
  property var _fetching: ({})

  // service is the Service instance that owns the helper connection.
  property var service: null

  // listController supplies the list cursor and visible ids this
  // controller reads but does not own.
  property var listController: null

  // activeId is the open conversation's id, or "" when none is open.
  property string activeId: ""

  // conversation is the open Conversation, or null when none is open.
  property var conversation: null

  // pane is "list" or "conversation": which column a narrow window shows.
  property string pane: "list"

  // messages is the loaded timeline, newest first.
  readonly property alias messages: timeline.model

  // annotations is Timeline.annotate's output, kept in step with messages.
  readonly property alias annotations: timeline.annotations

  // hasMore is true while an older page of messages may still exist.
  readonly property alias hasMore: timeline.hasMore

  // draft is the open conversation's unsent composer text.
  property string draft: ""

  // typing is true while the other side is composing a reply.
  property bool typing: false

  // typingName is who is typing, for the header subtitle.
  property string typingName: ""

  // windowActive is whether the user can see the window: shown and
  // focused. The helper reads messages on arrival only while it is, and
  // otherwise notifies.
  property bool windowActive: true

  // composeFocused mirrors whether the composer holds keyboard focus; the
  // caller sets it from the real text field so the Escape chain can read it.
  property bool composeFocused: false

  // subtitle is the line the header shows under the title: typing, group
  // size, or the account's connection status.
  readonly property string subtitle: root._subtitleText()

  // isGroup is true when the open conversation is a group chat.
  readonly property bool isGroup: root.conversation ? root.conversation.kind === "group" : false

  // lastError is the safe text of the most recent request failure.
  property string lastError: timeline.lastError

  // scroll asks the caller to move the message view: "down", "up",
  // "pageDown", "pageUp", "newest" or "oldest".
  signal scroll(string direction)

  // composeFocusRequested asks the caller to focus the composer.
  signal composeFocusRequested()

  // submitRequested asks the caller to submit whatever the composer holds.
  signal submitRequested()

  // leaveComposeRequested asks the caller to move focus out of the composer.
  signal leaveComposeRequested()

  // handles reports whether this controller owns action.
  function handles(action: string): bool {
    return Actions.owner(action) === "conversation";
  }

  // run performs action, the only entry point a key router needs.
  function run(action: string): void {
    const handlers = {
      "chat.open": () => root._openSelected(),
      "search.accept": () => root._openFirstVisible(),
      "pane.conversation": () => root._showPane(),
      "pane.list": () => root._hidePane(),
      "scroll.down": () => root.scroll("down"),
      "scroll.up": () => root.scroll("up"),
      "scroll.pageDown": () => root.scroll("pageDown"),
      "scroll.pageUp": () => root.scroll("pageUp"),
      "scroll.newest": () => root.scroll("newest"),
      "scroll.oldest": () => root.scroll("oldest"),
      "compose.focus": () => root.composeFocusRequested(),
      "chat.next": () => root._step(1),
      "chat.prev": () => root._step(-1),
      "message.retry": () => root.retryMessage(timeline.newestFailedId()),
      "message.send": () => root.submitRequested()
    };

    const handler = handlers[action];
    if (handler) handler();
  }

  // open shows conversation: loads its messages, marks it read, tells the
  // helper the user is looking at it, and restores its draft.
  function open(conversation: var): void {
    if (!conversation || !conversation.id) return;

    root._setActive(conversation.id, conversation);
    if (root.listController) root.listController.selectId(conversation.id);
  }

  // close leaves the open conversation: tells the helper no one is
  // looking, and switches the pane back to the list.
  function close(): void {
    if (!root.activeId) return;

    root.service.request("ui.setFocus", { conversationId: "", windowActive: false }, function() {});
    root.activeId = "";
    root.conversation = null;
    root.pane = "list";
    root._saveUiState({ activeId: "", pane: "list" });
    root._resetTyping();
  }

  // loadOlder fetches the page of messages before the oldest one loaded.
  function loadOlder(): void {
    if (!root.activeId) return;
    timeline.loadOlder(root.service, root.activeId, root.isGroup);
  }

  // send submits text to the open conversation.
  function send(text: string): void {
    if (!root.activeId || !text) return;

    root.service.request("messages.send", { conversationId: root.activeId, text: text }, function(error, result) {
      if (error) { timeline.lastError = Rpc.errorText(error); return; }
      root._upsertMessage(result);
    });
  }

  // fetchMedia downloads a message's photo and shows it once it is here.
  // A photo asks each time its row is shown, so one already on its way is
  // not asked for twice; if the download fails the preview stays.
  function fetchMedia(id: string): void {
    if (root._fetching[id]) return;

    root._fetching[id] = true;
    root.service.request("media.fetch", { messageId: id }, function(error, result) {
      delete root._fetching[id];
      if (!error) timeline.setMediaPath(id, result.path);
    });
  }

  // retryMessage resends one failed outgoing message by id.
  function retryMessage(id: string): void {
    if (!id) return;

    root.service.request("messages.retry", { messageId: id }, function(error, result) {
      if (error) { timeline.lastError = Rpc.errorText(error); return; }
      root._upsertMessage(result);
    });
  }

  // setWindowActive records whether the window is shown and focused, and
  // tells the helper.
  function setWindowActive(active: bool): void {
    if (root.windowActive === active) return;

    root.windowActive = active;
    root._reportFocus();
  }

  // _reportFocus tells the helper what the user is looking at. The panel
  // reports focus as it is created, possibly before Omarchy hands it the
  // service, so it is reported again when the service arrives.
  function _reportFocus(): void {
    if (!root.service) return;

    root.service.request("ui.setFocus", { conversationId: root.activeId, windowActive: root.windowActive }, function() {});
  }

  // setDraft stores the composer's text for the open conversation, so it
  // survives the panel being recreated.
  function setDraft(text: string): void {
    root.draft = text;
    if (!root.activeId) return;

    root._saveUiState({ drafts: Object.assign({}, root.service.uiState.drafts, { [root.activeId]: text }) });
  }

  // _openSelected opens whatever the list cursor currently points at.
  function _openSelected(): void {
    if (!root.listController || !root.listController.selectedId) return;
    root._openById(root.listController.selectedId);
  }

  // _openFirstVisible opens the first row a search matched.
  function _openFirstVisible(): void {
    if (!root.listController) return;

    const ids = root.listController.visibleIds();
    if (ids.length === 0) return;

    root.listController.leaveSearch();
    root._openById(ids[0]);
  }

  // _step opens the next or previous chat in the visible list, relative
  // to the one currently open.
  function _step(delta: int): void {
    if (!root.listController) return;

    const next = Selection.move(root.listController.visibleIds(), root.activeId, delta);
    if (next && next !== root.activeId) root._openById(next);
  }

  // _openById looks the conversation up through listController and opens it.
  function _openById(id: string): void {
    const conversation = root.listController ? root.listController.findConversation(id) : null;
    root._setActive(id, conversation);
    if (root.listController) root.listController.selectId(id);
  }

  // _setActive is the common work for opening a conversation, whatever
  // found it: the list cursor, a search result or the new-chat dialog.
  function _setActive(id: string, conversation: var): void {
    root.activeId = id;
    root.conversation = conversation;
    root.pane = "conversation";
    root.draft = root._draftFor(id);
    root._resetTyping();
    root._saveUiState({ activeId: id, pane: "conversation" });
    timeline.loadInitial(root.service, id, () => id === root.activeId, root.isGroup);
    root.service.request("conversations.markRead", { conversationId: id }, function() {});
    root.service.request("ui.setFocus", { conversationId: id, windowActive: root.windowActive }, function() {});
  }

  // _showPane brings the conversation column forward without reopening it.
  function _showPane(): void {
    if (!root.activeId) return;

    root.pane = "conversation";
    root._saveUiState({ pane: "conversation" });
  }

  // _hidePane brings the list column forward, leaving the conversation open.
  function _hidePane(): void {
    root.pane = "list";
    root._saveUiState({ pane: "list" });
  }

  // _upsertMessage applies a sent, retried or pushed message to the
  // timeline, ignoring one for a conversation that is not the open one.
  function _upsertMessage(message: var): void {
    if (message.conversationId !== root.activeId) return;
    timeline.upsert(message, root.isGroup);
  }

  // _draftFor reads a conversation's saved draft out of uiState.
  function _draftFor(id: string): string {
    const drafts = root.service.uiState.drafts || {};
    return drafts[id] || "";
  }

  // _handleTyping applies a typing event for the open conversation; events
  // for any other conversation are ignored.
  function _handleTyping(data: var): void {
    if (data.conversationId !== root.activeId) return;

    if (data.active) {
      root.typing = true;
      root.typingName = data.name;
      typingTimer.restart();
    } else {
      root._resetTyping();
    }
  }

  // _resetTyping clears the typing indicator and its expiry timer.
  function _resetTyping(): void {
    typingTimer.stop();
    root.typing = false;
    root.typingName = "";
  }

  // _subtitleText computes the header's subtitle: typing wins, then group
  // size, then the owning account's connection status.
  function _subtitleText(): string {
    if (root.typing) return root.typingName ? (root.typingName + " is typing…") : "typing…";
    if (!root.conversation) return "";
    if (root.conversation.kind === "group") return root.conversation.members + " members";

    const account = (root.service ? root.service.accounts : []).find((a) => a.id === root.conversation.accountId);
    return account ? (account.status.charAt(0).toUpperCase() + account.status.slice(1)) : "";
  }

  // _saveUiState merges patch into service.uiState as a new object, so a
  // copy someone read earlier never changes underneath them.
  function _saveUiState(patch: var): void {
    root.service.uiState = Object.assign({}, root.service.uiState, patch);
  }

  // Omarchy may hand over the service after the panel is created, or
  // replace it, so start-up runs whenever it arrives.
  onServiceChanged: {
    root._restore();
    root._reportFocus();
  }
  Component.onCompleted: root._restore()

  // _restore reopens the conversation that was open when the panel was
  // last destroyed, once the service is there.
  function _restore(): void {
    if (!root.service) return;

    const state = root.service.uiState ?? {}; // a service still starting has none yet
    root.pane = state.pane || "list";
    if (state.activeId) {
      root.activeId = state.activeId;
      root.conversation = root.listController ? root.listController.findConversation(state.activeId) : null;
      root.draft = root._draftFor(state.activeId);
      if (root.service.status === "ready") timeline.loadInitial(root.service, state.activeId, () => state.activeId === root.activeId, root.isGroup);
    }
  }

  MessageTimeline { id: timeline }

  Timer {
    id: typingTimer
    interval: 6000
    onTriggered: { root.typing = false; root.typingName = ""; }
  }

  Connections {
    target: root.service

    function onStatusChanged() {
      if (root.service.status === "ready" && root.activeId)
        timeline.loadInitial(root.service, root.activeId, () => true, root.isGroup);
    }

    function onEvent(name, data) {
      if (name === "message.added" || name === "message.updated") root._upsertMessage(data);
      else if (name === "typing") root._handleTyping(data);
      else if (name === "conversation.updated" && data.id === root.activeId) root.conversation = data;
    }
  }

  // A recreated panel starts before the list has loaded, so the open
  // conversation is filled in, and the helper told it is being looked at,
  // as soon as the list knows it.
  Connections {
    target: root.listController

    function onRailItemsChanged() {
      if (!root.activeId || root.conversation) return;

      root.conversation = root.listController.findConversation(root.activeId);
      if (root.conversation)
        root.service.request("ui.setFocus", { conversationId: root.activeId, windowActive: root.windowActive }, function() {});
    }
  }
}
