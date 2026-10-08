import QtQuick
import "../lib/Selection.js" as Selection
import "../lib/Highlight.js" as Highlight
import "../lib/Timeline.js" as Timeline
import "../lib/Actions.js" as Actions
import "../lib/Rpc.js" as Rpc
import "../lib/Media.js" as Media

// Owns the open conversation itself: which one is open, paging through
// its messages, sending, retrying, scrolling and the typing indicator.
// The only controller that calls messages.send, messages.retry,
// conversations.markRead and ui.setFocus; paging through messages.list
// is delegated to the timeline child below.
//
// composer and photoViewer are set once, from whoever wires the
// controllers together, and are read through null-safe calls so this
// controller still works alone in a test that has no use for them.
// listController supplies the list cursor and the visible ids that
// chat.open, chat.next/prev and search.accept need.
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

  // composer is the ComposerController whose draft, reply target and
  // attachment this controller reads when sending and clears when the
  // open conversation closes or switches.
  property var composer: null

  // photoViewer is the PhotoViewerController this controller hands a
  // photo to, and closes when the open conversation closes or switches.
  property var photoViewer: null

  // voiceController is the VoiceNoteController this controller asks to
  // play or pause a voice note, and reads playback state from for the
  // message list to show.
  property var voiceController: null

  // activeId is the open conversation's id, or "" when none is open.
  property string activeId: ""

  // conversation is the open Conversation, or null when none is open.
  property var conversation: null

  // pane is "list" or "conversation": which column a narrow window shows.
  property string pane: "list"

  // highlightedId is the message id j/k move through, by id rather than
  // index, so loading older history or a new message arriving never
  // moves it to a different message. It starts on the newest message
  // each time a conversation (re)loads, and r, e, t and Enter all act on
  // whichever message this names.
  property string highlightedId: ""

  // messages is the loaded timeline, newest first.
  readonly property alias messages: timeline.model

  // annotations is Timeline.annotate's output, kept in step with messages.
  readonly property alias annotations: timeline.annotations

  // hasMore is true while an older page of messages may still exist.
  readonly property alias hasMore: timeline.hasMore

  // historyUnavailable is true once scrolling back asked the service for
  // older history and it could not be reached right now, such as a
  // WhatsApp account whose phone never answered; the UI shows this as a
  // note rather than treating it the same as genuinely having no more.
  readonly property alias historyUnavailable: timeline.historyUnavailable

  // timeline exposes the loaded messages to the sibling controllers that
  // read or change them: the composer, the photo viewer and reactions.
  readonly property alias timeline: timeline

  // typing is true while the other side is composing a reply.
  property bool typing: false

  // typingName is who is typing, for the header subtitle.
  property string typingName: ""

  // windowActive is whether the user can see the window: shown and
  // focused. The helper reads messages on arrival only while it is, and
  // otherwise notifies.
  property bool windowActive: true

  // subtitle is the line the header shows under the title: typing, group
  // size, or the account's connection status.
  readonly property string subtitle: root._subtitleText()

  // isGroup is true when the open conversation is a group chat.
  readonly property bool isGroup: root.conversation ? root.conversation.kind === "group" : false

  // lastError is the safe text of the most recent request failure.
  property string lastError: timeline.lastError

  // voiceNotes is the playback state the message list reads for each
  // bubble's voice note player: which message, if any, is playing, how
  // far into it, and whether in-window playback is available at all. A
  // plain summary rather than voiceController itself, so a (recycled)
  // bubble only ever reads data, never calls into a controller.
  readonly property var voiceNotes: root.voiceController ? {
    available: root.voiceController.available,
    playingId: root.voiceController.playingId,
    positionMs: root.voiceController.positionMs,
    durationMs: root.voiceController.durationMs
  } : ({ available: false, playingId: "", positionMs: 0, durationMs: 0 })

  // scroll asks the caller to move the message view: "down", "up",
  // "pageDown", "pageUp", "newest" or "oldest".
  signal scroll(string direction)

  // scrollToMessageRequested asks the caller to scroll the message view
  // to a loaded message, by its local id.
  signal scrollToMessageRequested(string id)

  // linksRequested asks the caller to open the highlighted message's
  // link: Qt.openUrlExternally on it directly when there is exactly one,
  // the same choice the mouse path already makes, or let the user choose
  // among several when there is more than one. Opening stays with the
  // caller, rather than this controller, so it is checked by what it
  // asked for rather than by stubbing Qt.openUrlExternally.
  signal linksRequested(var urls)

  // handles reports whether this controller owns action.
  function handles(action: string): bool {
    return Actions.owner(action) === "conversation";
  }

  // run performs action, the only entry point a key router needs. It
  // returns false only for an action that found nothing to do, such as
  // pane.conversation with no conversation open, so the key router can
  // leave the key event unaccepted and let it fall through to the normal
  // focus chain instead of swallowing it for no reason.
  function run(action: string): bool {
    const handlers = {
      "chat.open": () => root._openSelected(),
      "search.accept": () => root._openFirstVisible(),
      "pane.conversation": () => root._showPane(),
      "pane.list": () => root._hidePane(),
      "message.highlightOlder": () => root._moveHighlight(true),
      "message.highlightNewer": () => root._moveHighlight(false),
      "message.open": () => root._openHighlighted(),
      "message.openLink": () => root._openHighlightedLink(),
      "message.goToQuote": () => root._goToHighlightedQuote(),
      "scroll.pageDown": () => root.scroll("pageDown"),
      "scroll.pageUp": () => root.scroll("pageUp"),
      "scroll.newest": () => root.scroll("newest"),
      "scroll.oldest": () => root.scroll("oldest"),
      "chat.next": () => root._step(1),
      "chat.prev": () => root._step(-1),
      "message.retry": () => root._retryHighlighted(),
      "unread.next": () => root._stepUnread(1),
      "unread.prev": () => root._stepUnread(-1)
    };

    const handler = handlers[action];
    return handler ? handler() !== false : true;
  }

  // open shows conversation: loads its messages, marks it read, tells the
  // helper the user is looking at it, and restores its draft.
  function open(conversation: var): void {
    if (!conversation || !conversation.id) return;

    root._setActive(conversation.id, conversation);
    if (root.listController) root.listController.selectId(conversation.id);
  }

  // closeIfOpen closes the open conversation if it is id: used when the
  // list reports id just folded out of the standard view, so the pane
  // goes back to the empty state instead of following the list's own
  // reselect onto a neighbour, which would mark that neighbour read as a
  // side effect of hiding or archiving this one.
  function closeIfOpen(id: string): void {
    if (root.activeId === id) root.close();
  }

  // close leaves the open conversation: tells the helper no one is
  // looking, and switches the pane back to the list.
  function close(): void {
    if (!root.activeId) return;

    root.service.request("ui.setFocus", { conversationId: "", windowActive: false }, function() {});
    root.activeId = "";
    root.conversation = null;
    root.pane = "list";
    root.highlightedId = "";
    if (root.composer) root.composer.cancelReply();
    if (root.photoViewer) root.photoViewer.close();
    root.saveUiState({ activeId: "", pane: "list" });
    root._resetTyping();
  }

  // loadOlder fetches the page of messages before the oldest one loaded.
  function loadOlder(): void {
    if (!root.activeId) return;
    timeline.loadOlder(root.service, root.activeId, root.isGroup);
  }

  // send submits text, and whatever the composer holds as its attachment,
  // to the open conversation, answering the message replyToId names, if
  // any. Sending clears the attachment and the reply, whether or not it
  // succeeds, the same as it clears the composer's text.
  function send(text: string, replyToId: string): void {
    const attachment = root.composer ? root.composer.attachmentPath : "";
    if (!root.activeId || (!text && !attachment)) return;

    const params = { conversationId: root.activeId, text: text };
    if (attachment) params.attachment = { path: attachment };
    if (replyToId) params.replyTo = replyToId;

    root.service.request("messages.send", params, function(error, result) {
      if (error) { timeline.lastError = Rpc.errorText(error); return; }
      root.applyMessage(result);
    });
    if (root.composer) {
      root.composer.attachmentPath = "";
      root.composer.cancelReply();
    }
  }

  // scrollToReply asks the caller to scroll to the message a reply
  // quotes, by the remote id Telegram gave it, when it is loaded.
  function scrollToReply(remoteId: string): void {
    const id = timeline.localIdForRemote(remoteId);
    if (id) root.scrollToMessageRequested(id);
  }

  // fetchMedia downloads a message's photo and shows it once it is here.
  // A photo asks each time its row is shown, so one already on its way is
  // not asked for twice; if the download fails the preview stays.
  function fetchMedia(id: string): void {
    root.downloadMedia(id, false, () => {});
  }

  // openMedia opens a message's photo, video, file or voice note. A photo
  // opens in the in-app viewer: Omarchy's window rule floats the external
  // image viewer small and keeps keyboard focus on this window, so its
  // close keys never reach it. A voice note plays or pauses in place
  // through voiceController, when it is there and QtMultimedia loaded;
  // otherwise it falls through to the same "open in your own application"
  // path a video or file already gets. Media.kindFor also catches a
  // voice note stored before the helper's "voice" media kind existed,
  // which still carries kind "file" forever (see Media.js), so it plays
  // in place too rather than opening externally.
  function openMedia(id: string): void {
    const media = timeline.media(id);
    const kind = Media.kindFor(media);
    if (kind === "photo") { if (root.photoViewer) root.photoViewer.show(id); return; }
    if (kind === "voice" && root.voiceController && root.voiceController.available) { root._toggleVoice(id, media); return; }

    const path = timeline.mediaPath(id);
    if (path) Qt.openUrlExternally("file://" + path);
    else root.downloadMedia(id, true, (downloaded) => Qt.openUrlExternally("file://" + downloaded));
  }

  // _toggleVoice plays or pauses a voice note, downloading it first if it
  // has not been fetched yet; voice notes are small, so this is quick and
  // happens the same way a photo's own missing full image would.
  function _toggleVoice(id: string, media: var): void {
    const path = timeline.mediaPath(id);
    if (path) { root.voiceController.toggle(id, path, (media.duration || 0) * 1000); return; }

    root.downloadMedia(id, true, (downloaded) => root.voiceController.toggle(id, downloaded, (media.duration || 0) * 1000));
  }

  // downloadMedia asks the helper for a message's media once, records
  // where it is, then runs done with the path. A failure is reported to
  // the user only when they asked for the media, rather than a photo
  // fetching itself, but is always recorded on the message, so a photo
  // with no preview shows "Photo unavailable" instead of an empty box
  // that a failed, silent auto-fetch would otherwise leave forever, and
  // always logged to the console with its code and safe reason
  // category, so a failed fetch is never silent even when nothing asked
  // to see its error text. The photo viewer reuses this rather than
  // asking the helper itself, so there is one place that tracks an
  // in-flight download.
  function downloadMedia(id: string, report: bool, done: var): void {
    if (root._fetching[id]) return;

    root._fetching[id] = true;
    root.service.request("media.fetch", { messageId: id }, function(error, result) {
      delete root._fetching[id];
      if (error) {
        const reason = Rpc.errorReason(error);
        console.warn("media.fetch failed for " + id + ": code=" + error.code + " reason=" + (reason || "unknown"));
        timeline.setMediaFailed(id, true, reason);
        if (report) timeline.lastError = Rpc.errorText(error);
        return;
      }

      timeline.setMediaFailed(id, false, "");
      timeline.setMediaPath(id, result.path);
      done(result.path);
    });
  }

  // retryMessage resends one failed outgoing message by id.
  function retryMessage(id: string): void {
    if (!id) return;

    root.service.request("messages.retry", { messageId: id }, function(error, result) {
      if (error) { timeline.lastError = Rpc.errorText(error); return; }
      root.applyMessage(result);
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
  // to whichever chat is "current": the open conversation, or, with none
  // open, the list's own cursor, so this works the same browsing the
  // list as it does once a conversation is open.
  function _step(delta: int): void {
    if (!root.listController) return;

    const around = root.activeId || root.listController.selectedId;
    const next = Selection.move(root.listController.visibleIds(), around, delta);
    if (next && next !== around) root._openPreservingMode(next);
  }

  // _stepUnread jumps to the next, or with a negative delta the previous,
  // unread conversation and opens it, from any mode: the list, an open
  // conversation, or the all-unreads view, the same as _step does for
  // chat.next/prev.
  function _stepUnread(delta: int): void {
    if (!root.listController) return;

    const ids = root.listController.visibleIds();
    const conversations = ids.map((id) => root.listController.findConversation(id)).filter((c) => c);
    const ordered = delta > 0 ? conversations : conversations.slice().reverse();
    const around = root.activeId || root.listController.selectedId;
    const next = Selection.nextUnread(ordered, around);
    if (next) root._openPreservingMode(next);
  }

  // _openPreservingMode opens id the way chat.next/prev and
  // unread.next/prev do: opening a conversation always focuses the
  // composer by default (see MessengerLayout's onActiveIdChanged), which
  // would otherwise drag a scrolling user into writing, or move them
  // away from the keyboard they were already writing on. This keeps
  // whichever mode they were already in instead.
  function _openPreservingMode(id: string): void {
    const wasWriting = root.composer ? root.composer.composeFocused : false;
    root._openById(id);
    if (!wasWriting && root.composer) root.composer.leaveComposeRequested();
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
    root.highlightedId = "";
    if (root.composer) root.composer.restore(id);
    if (root.photoViewer) root.photoViewer.close();
    root._resetTyping();
    root.saveUiState({ activeId: id, pane: "conversation" });
    timeline.loadInitial(root.service, id, () => id === root.activeId, root.isGroup);
    root.service.request("conversations.markRead", { conversationId: id }, function() {});
    root.service.request("ui.setFocus", { conversationId: id, windowActive: root.windowActive }, function() {});
  }

  // _showPane brings the conversation column forward without reopening
  // it, or does nothing when there is no open conversation to show: that
  // no-op reports false, so Tab falls through to the normal focus chain
  // (reaching the empty state's own buttons) instead of being swallowed.
  function _showPane(): bool {
    if (!root.activeId) return false;

    root.pane = "conversation";
    root.saveUiState({ pane: "conversation" });
    return true;
  }

  // _hidePane brings the list column forward, leaving the conversation
  // open. Always a real change, since this is only ever reached from the
  // "conversation" key context, which already means the pane was showing
  // the conversation.
  function _hidePane(): bool {
    root.pane = "list";
    root.saveUiState({ pane: "list" });
    return true;
  }

  // resetHighlight moves the highlight to the newest loaded message.
  // Idempotent: calling it again while already there changes nothing.
  // Called when a conversation (re)loads and when the user leaves
  // writing mode with Escape.
  function resetHighlight(): void {
    root.highlightedId = timeline.newestId();
  }

  // _moveHighlight moves the highlight one message toward older or newer,
  // stopping at either end rather than overscrolling; at the oldest
  // loaded message, moving further asks for more history instead,
  // leaving the highlight where it is until that page arrives.
  function _moveHighlight(older: bool): void {
    const ids = timeline.ids();
    if (ids.length === 0) return;

    if (older && Highlight.atOldest(ids, root.highlightedId)) { root.loadOlder(); return; }

    root.highlightedId = older ? Highlight.older(ids, root.highlightedId) : Highlight.newer(ids, root.highlightedId);
    root.scrollToMessageRequested(root.highlightedId);
  }

  // _openHighlighted opens the highlighted message's photo, video or
  // file; Enter does nothing when it carries none, rather than guessing
  // at some other action.
  function _openHighlighted(): void {
    if (root.highlightedId && timeline.media(root.highlightedId)) root.openMedia(root.highlightedId);
  }

  // _retryHighlighted resends the highlighted message, only when it is
  // itself a failed outgoing one.
  function _retryHighlighted(): void {
    const m = timeline.find(root.highlightedId);
    if (m && m.outgoing && m.status === "failed") root.retryMessage(m.id);
  }

  // _openHighlightedLink finds the highlighted message's link or links
  // (the link preview's own URL, or every link in its text when it has
  // no preview) and asks the caller to open them: nothing happens when
  // it carries none.
  function _openHighlightedLink(): void {
    const message = timeline.find(root.highlightedId);
    if (!message) return;

    const found = Highlight.links(message);
    if (found.length > 0) root.linksRequested(found);
  }

  // _goToHighlightedQuote moves the highlight to the message the
  // highlighted one replies to, and scrolls to it, the same lookup by
  // remote id the quote's own click already uses. Nothing happens when
  // it answers nothing, or the quoted message is not loaded, the same as
  // that click.
  function _goToHighlightedQuote(): void {
    const message = timeline.find(root.highlightedId);
    const quote = message ? Timeline.replyTo(message) : null;
    if (!quote || !quote.remoteId) return;

    const id = timeline.localIdForRemote(quote.remoteId);
    if (!id) return;

    root.highlightedId = id;
    root.scrollToMessageRequested(id);
  }

  // applyMessage applies a sent, retried or pushed message to the
  // timeline, ignoring one for a conversation that is not the open one.
  // Reactions and the composer's own send both land here.
  function applyMessage(message: var): void {
    if (message.conversationId !== root.activeId) return;
    timeline.upsert(message, root.isGroup);
  }

  // _removeMessage drops a deleted message from the timeline, ignoring
  // one for a conversation that is not the open one.
  function _removeMessage(data: var): void {
    if (data.conversationId !== root.activeId) return;
    timeline.remove(data.messageId, root.isGroup);
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

  // saveUiState merges patch into service.uiState as a new object, so a
  // copy someone read earlier never changes underneath them. The composer
  // reuses this to save a draft under the same durable state.
  function saveUiState(patch: var): void {
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
      if (root.composer) root.composer.restore(state.activeId);
      if (root.service.status === "ready") timeline.loadInitial(root.service, state.activeId, () => state.activeId === root.activeId, root.isGroup);
    }
  }

  MessageTimeline {
    id: timeline
    onInitialLoaded: root.resetHighlight()
  }

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
      if (name === "message.added" || name === "message.updated") root.applyMessage(data);
      else if (name === "message.removed") root._removeMessage(data);
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
