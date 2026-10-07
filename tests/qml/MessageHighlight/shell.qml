// Checks the highlighted-message keyboard feature with real key events,
// through the same routeKey dispatch Panel.qml uses, wired to the real
// controllers and ConversationView against a scripted fake service (not
// the demo helper, so every outcome is deterministic): opening a
// conversation starts the highlight on the newest message; k moves it
// toward older messages and j toward newer ones, each clamping at its
// end rather than overscrolling; k at the oldest loaded message loads
// more history instead of moving, and the highlight continues onto it
// once it arrives; the view follows the highlight to bring it into
// view; t retries the highlighted message only when it has failed; e
// opens the picker for it; r starts a reply to it; Enter opens its
// photo; o asks to open its link, or lets the caller choose among
// several; p moves the highlight to the message it replies to; o in the
// photo viewer reaches openExternally (never a real download here, so
// it only closes the viewer, never calling Qt.openUrlExternally for
// real); that the highlighted message's own visual cue hides the moment
// the composer takes focus and shows again once it leaves; and leaving
// the composer with Escape resets the highlight to the newest message,
// idempotently.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "ui/controllers"
import "ui/lib/Keymap.js" as Keymap
import "ui/lib/Navigation.js" as Navigation
import "Check.js" as Check

ShellRoot {
  id: root

  property int pollAttempts: 0
  property var _next: null
  // capturedLinks records the urls the last linksRequested signal
  // carried, so o's single-link path is checked without ever calling
  // the real Qt.openUrlExternally.
  property var capturedLinks: null

  // retry schedules fn to run again shortly, for the one step here that
  // depends on real Qt focus rather than the scripted service.
  function retry(fn: var): void {
    root._next = fn;
    retryTimer.start();
  }

  QtObject {
    id: service

    property var accounts: []
    property var services: []
    property var uiState: ({})
    property var pendingAuth: null
    property string status: "ready"
    property string detail: ""
    property int unreadTotal: 0
    property var retried: []
    readonly property real base: Date.now()

    // event is never emitted here: this fake answers every request
    // synchronously instead of pushing events, but ConversationController
    // listens for it, so it has to exist.
    signal event(string name, var data)

    function start(): void {}
    function installHelper(): void {}
    function quit(): void {}

    // fakeMessage builds a minimal message in conversation "c1", for
    // messages.list and messages.retry to answer with.
    function fakeMessage(id: string, created: real, fields: var): var {
      return Object.assign({ id: id, conversationId: "c1", senderId: "s1", senderName: "Alex",
        text: id, outgoing: false, status: "received", created: created }, fields || {});
    }

    // request answers synchronously, so every check below runs without
    // waiting on a real helper: a first page of five messages (m3
    // failed, m4 carrying a photo), one older message (m6) once asked
    // for, and a retried message's status flipped to delivered.
    function request(method: string, params: var, callback: var): void {
      if (method === "messages.list") {
        if (params.before) {
          callback(null, { hasMore: false, messages: [service.fakeMessage("m6", service.base - 5000, {})] });
          return;
        }
        callback(null, { hasMore: true, messages: [
          service.fakeMessage("m1", service.base, { remoteId: "rm1" }),
          service.fakeMessage("m2", service.base - 1000, { text: "see https://example.com/offer" }),
          service.fakeMessage("m3", service.base - 2000, { outgoing: true, status: "failed", text: "oops" }),
          service.fakeMessage("m4", service.base - 3000, { media: { kind: "photo", width: 10, height: 10, thumb: "" } }),
          service.fakeMessage("m5", service.base - 4000, { replyTo: { remoteId: "rm1", senderName: "Alex", text: "hi there" } })
        ] });
        return;
      }

      if (method === "messages.retry") {
        service.retried.push(params.messageId);
        callback(null, service.fakeMessage("m3", service.base - 2000, { outgoing: true, status: "delivered", text: "oops" }));
        return;
      }

      // media.fetch answers with no path, same as a real download that
      // never finishes here: an explicit "" rather than a missing field,
      // so mediaPath never coerces to the literal string "undefined",
      // which openExternally would then treat as a real path to open.
      if (method === "media.fetch") {
        callback(null, { path: "" });
        return;
      }

      callback(null, {});
    }
  }

  ConversationController {
    id: conversationController
    service: service
    composer: composerController
    photoViewer: photoViewerController
  }

  ComposerController {
    id: composerController
    service: service
    conversation: conversationController
  }

  ReactionsController {
    id: reactionsController
    service: service
    conversation: conversationController
  }

  PhotoViewerController {
    id: photoViewerController
    conversation: conversationController
  }

  WindowController {
    id: windowController
    service: service
    conversationController: conversationController
    composerController: composerController
  }

  // routeKey is the same dispatch Panel.routeKey does: match the active
  // context, then hand the action to whichever controller owns it.
  function routeKey(key: int, modifiers: int, text: string): bool {
    const context = Navigation.keyContext({
      confirmOpen: windowController.confirmingClose,
      setupOpen: false,
      viewerOpen: photoViewerController.viewerOpen,
      paletteOpen: false,
      reactionPickerOpen: reactionsController.pickerOpen,
      dialogOpen: false,
      searchFocused: false,
      composeFocused: composerController.composeFocused,
      pane: conversationController.pane
    });
    const action = Keymap.match(context, key, modifiers, text);
    if (!action) return false;

    const controllers = [conversationController, composerController, reactionsController, photoViewerController, windowController];
    const owner = controllers.find((c) => c.handles(action));
    if (!owner) return false;

    owner.run(action);
    return true;
  }

  // indexOf returns a loaded message's index in the ListView's model, or
  // -1, since the highlight is tracked by id, never an index.
  function indexOf(id: string): int {
    const listView = Check.find(view, "messageListView");
    for (let i = 0; i < listView.count; i++) {
      if (listView.model.get(i).id === id) return i;
    }
    return -1;
  }

  // delegateFor returns id's realized delegate, or null when the view
  // has not brought it into view.
  function delegateFor(id: string): var {
    const listView = Check.find(view, "messageListView");
    const i = root.indexOf(id);
    return i < 0 ? null : listView.itemAtIndex(i);
  }

  // hintText reads id's own key-hint row, or "" when it is not realized.
  function hintText(id: string): string {
    const d = root.delegateFor(id);
    return d ? Check.find(d, "highlightHints").text : "";
  }

  // isVisuallyHighlighted reads id's own bubble outline, which only
  // shows while root.highlighted is true for that delegate: false while
  // the composer has focus, even if the controller still names this
  // message highlightedId, since ConversationView hides the cue in
  // writing mode.
  function isVisuallyHighlighted(id: string): bool {
    const d = root.delegateFor(id);
    return !!d && Check.find(d, "bubble").border.width > 0;
  }

  FloatingWindow {
    id: win
    implicitWidth: 500
    implicitHeight: 400
    visible: true

    Item {
      id: keyArea
      anchors.fill: parent
      focus: true

      Keys.onPressed: event => {
        if (root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
      }

      ConversationView {
        id: view
        anchors.fill: parent
        conversation: conversationController.conversation
        messages: conversationController.messages
        annotations: conversationController.annotations
        highlightedId: conversationController.highlightedId
        draft: composerController.draft
        replyTarget: composerController.replyTarget
        attachmentPath: composerController.attachmentPath
        composeEnabled: true
        routeKey: root.routeKey
      }
    }
  }

  // Mirrors the subset of MessengerLayout's wiring this test needs: the
  // composer's own focus state read back into the controller the real
  // router reads, and the controller's scroll request followed.
  Connections {
    target: composerController
    function onComposeFocusRequested() { view.focusComposer() }
    function onLeaveComposeRequested() { view.composer.input.focus = false; keyArea.forceActiveFocus() }
  }

  Connections {
    target: view.composer
    function onWritingChanged() { composerController.composeFocused = view.composer.writing }
  }

  Connections {
    target: conversationController
    function onScrollToMessageRequested(id) { view.scrollToMessage(id) }
    function onLinksRequested(urls) { root.capturedLinks = urls }
  }

  TestCase {
    id: t
    when: false
  }

  Timer {
    id: retryTimer
    interval: 20
    onTriggered: root._next()
  }

  // A deliberately unreachable deadline: it only fires, and fails the
  // test with a reason, if something above never happens.
  Timer {
    running: true
    interval: 20000
    onTriggered: Check.fail("timed out before the checks finished")
  }

  Timer {
    running: true
    interval: 50
    onTriggered: root.start()
  }

  // start opens the one conversation and begins the checks.
  function start(): void {
    conversationController.open({ id: "c1", accountId: "a1", service: "whatsapp", remoteId: "r1",
      kind: "direct", title: "Alex", members: 0, preview: "", muted: false, unread: 0, lastActivity: Date.now() });
    root.checkOlderNavigation();
  }

  // checkOlderNavigation walks k from the newest message down to the
  // oldest loaded one, checking the hint text along the way, retrying
  // the one failed message with t, opening the one with a photo with
  // Enter, and that k at the oldest message loads more instead of
  // overscrolling.
  function checkOlderNavigation(): void {
    if (conversationController.highlightedId !== "m1")
      return Check.fail("opening the conversation did not start the highlight on the newest message: got " + conversationController.highlightedId);

    t.keyClick(Qt.Key_K);
    if (conversationController.highlightedId !== "m2")
      return Check.fail("k did not move the highlight to the next older message: got " + conversationController.highlightedId);

    t.keyClick(Qt.Key_K);
    if (conversationController.highlightedId !== "m3")
      return Check.fail("k did not reach the failed message: got " + conversationController.highlightedId);
    if (root.hintText("m3") !== "r reply · e react · t retry")
      return Check.fail("hint text for the failed message is \"" + root.hintText("m3") + "\"");

    t.keyClick(Qt.Key_T);
    if (JSON.stringify(service.retried) !== '["m3"]')
      return Check.fail("t did not retry the highlighted failed message: " + JSON.stringify(service.retried));
    if (conversationController.messages.get(root.indexOf("m3")).status !== "delivered")
      return Check.fail("retrying did not update the message's status");

    t.keyClick(Qt.Key_T);
    if (JSON.stringify(service.retried) !== '["m3"]')
      return Check.fail("t retried a message that was not failed: " + JSON.stringify(service.retried));

    t.keyClick(Qt.Key_K);
    if (conversationController.highlightedId !== "m4")
      return Check.fail("k did not reach the message with a photo: got " + conversationController.highlightedId);
    if (root.hintText("m4") !== "r reply · e react · Enter open")
      return Check.fail("hint text for the photo message is \"" + root.hintText("m4") + "\"");

    t.keyClick(Qt.Key_Return);
    if (photoViewerController.viewerId !== "m4")
      return Check.fail("Enter did not open the highlighted message's photo: viewerId=" + photoViewerController.viewerId);

    // o reaches openExternally in the viewer context; this fixture never
    // downloads a real path, so it only closes the viewer, never calling
    // Qt.openUrlExternally for real.
    t.keyClick(Qt.Key_O);
    if (photoViewerController.viewerOpen)
      return Check.fail("o did not close the photo viewer via openExternally");

    t.keyClick(Qt.Key_K);
    if (conversationController.highlightedId !== "m5")
      return Check.fail("k did not reach the oldest message on the first page: got " + conversationController.highlightedId);

    t.keyClick(Qt.Key_K);
    if (conversationController.highlightedId !== "m5")
      return Check.fail("k at the oldest message moved the highlight instead of loading more: got " + conversationController.highlightedId);
    if (root.indexOf("m6") < 0)
      return Check.fail("k at the oldest message did not load older history");

    root.checkOldestClamp();
  }

  // checkOldestClamp moves onto the newly loaded message, checks the
  // view scrolled to show it, then checks k stops there for good once
  // there is truly nothing older.
  function checkOldestClamp(): void {
    t.keyClick(Qt.Key_K);
    if (conversationController.highlightedId !== "m6")
      return Check.fail("k did not move onto the newly loaded older message: got " + conversationController.highlightedId);
    if (!root.delegateFor("m6"))
      return Check.fail("the view did not follow the highlight to the oldest loaded message");

    t.keyClick(Qt.Key_K);
    if (conversationController.highlightedId !== "m6")
      return Check.fail("k past the true oldest message moved the highlight: got " + conversationController.highlightedId);

    root.checkNewerClamp();
  }

  // checkNewerClamp walks j all the way back to the newest message and
  // checks it stops there rather than overscrolling.
  function checkNewerClamp(): void {
    for (let i = 0; i < 10; i++) t.keyClick(Qt.Key_J);
    if (conversationController.highlightedId !== "m1")
      return Check.fail("j did not clamp at the newest message: got " + conversationController.highlightedId);

    t.keyClick(Qt.Key_J);
    if (conversationController.highlightedId !== "m1")
      return Check.fail("j past the newest message moved the highlight: got " + conversationController.highlightedId);

    root.checkReplyAndReact();
  }

  // checkReplyAndReact moves off the newest message so a reply or a
  // reaction aimed at "the highlighted message" is distinguishable from
  // one aimed at the newest, then checks r and e each target it.
  function checkReplyAndReact(): void {
    t.keyClick(Qt.Key_K);
    if (conversationController.highlightedId !== "m2") return Check.fail("setup: k did not reach m2");

    t.keyClick(Qt.Key_R);
    if (!composerController.replyTarget || composerController.replyTarget.id !== "m2")
      return Check.fail("r did not start a reply to the highlighted message: " + JSON.stringify(composerController.replyTarget));
    composerController.cancelReply();

    t.keyClick(Qt.Key_E);
    if (!reactionsController.pickerOpen || reactionsController.pickerTarget !== "m2")
      return Check.fail("e did not open the picker for the highlighted message: open=" + reactionsController.pickerOpen + " target=" + reactionsController.pickerTarget);
    reactionsController.closePicker();

    root.checkOpenLinkAndGoToQuote();
  }

  // checkOpenLinkAndGoToQuote checks o reports the highlighted message's
  // own link through linksRequested, which the real caller opens with
  // Qt.openUrlExternally (never exercised here, so this checks what was
  // asked for rather than stubbing that call), and that p moves the
  // highlight to the message m5 replies to and scrolls there.
  function checkOpenLinkAndGoToQuote(): void {
    if (root.hintText("m2") !== "r reply · e react · o open link")
      return Check.fail("hint text for the linked message is \"" + root.hintText("m2") + "\"");

    root.capturedLinks = null;
    t.keyClick(Qt.Key_O);
    if (JSON.stringify(root.capturedLinks) !== '["https://example.com/offer"]')
      return Check.fail("o did not report the highlighted message's own link: " + JSON.stringify(root.capturedLinks));

    t.keyClick(Qt.Key_K);
    t.keyClick(Qt.Key_K);
    t.keyClick(Qt.Key_K);
    if (conversationController.highlightedId !== "m5") return Check.fail("setup: k,k,k did not reach m5");
    if (root.hintText("m5") !== "r reply · e react · p go to quote")
      return Check.fail("hint text for the reply is \"" + root.hintText("m5") + "\"");

    t.keyClick(Qt.Key_P);
    if (conversationController.highlightedId !== "m1")
      return Check.fail("p did not move the highlight to the quoted message: got " + conversationController.highlightedId);
    if (!root.delegateFor("m1")) return Check.fail("p did not scroll the view to the quoted message");

    t.keyClick(Qt.Key_K);
    if (conversationController.highlightedId !== "m2") return Check.fail("setup: k back to m2 failed");

    root.checkEscapeResetsHighlight();
  }

  // checkEscapeResetsHighlight checks m2's own highlight cue shows
  // while scrolling, focuses the composer with i, leaves it with
  // Escape, and checks the highlight came back to the newest message,
  // idempotently.
  function checkEscapeResetsHighlight(): void {
    if (!root.isVisuallyHighlighted("m2")) return Check.fail("m2 does not show its highlight cue before writing starts");

    t.keyClick(Qt.Key_I);
    root.pollAttempts = 0;
    root.waitForComposeFocus();
  }

  // waitForComposeFocus holds until i has really focused the composer's
  // input, a real Qt focus change rather than the scripted service, then
  // checks that writing hid m2's own highlight cue even though it is
  // still conversationController.highlightedId underneath.
  function waitForComposeFocus(): void {
    if (!composerController.composeFocused) {
      root.pollAttempts++;
      if (root.pollAttempts >= 100) return Check.fail("i never focused the composer");
      return root.retry(root.waitForComposeFocus);
    }

    if (root.isVisuallyHighlighted("m2")) return Check.fail("m2 still shows its highlight cue while the composer is writing");
    root.leaveComposeWithEscape();
  }

  // leaveComposeWithEscape presses Escape and waits for it to blur the
  // composer before checking the highlight.
  function leaveComposeWithEscape(): void {
    t.keyClick(Qt.Key_Escape);
    root.pollAttempts = 0;
    root.waitForComposeLeft();
  }

  function waitForComposeLeft(): void {
    if (!composerController.composeFocused) return root.finish();

    root.pollAttempts++;
    if (root.pollAttempts >= 100) return Check.fail("Escape never left the composer");
    root.retry(root.waitForComposeLeft);
  }

  // finish checks Escape reset the highlight to the newest message, that
  // its cue is visible again now writing has stopped, that resetHighlight
  // is idempotent, and ends the test.
  function finish(): void {
    if (conversationController.highlightedId !== "m1")
      return Check.fail("leaving the composer with Escape did not reset the highlight to the newest message: got " + conversationController.highlightedId);
    if (!root.isVisuallyHighlighted("m1")) return Check.fail("m1 does not show its highlight cue once writing has stopped");

    conversationController.resetHighlight();
    if (conversationController.highlightedId !== "m1")
      return Check.fail("resetHighlight is not idempotent");

    console.log("PASS MessageHighlight");
    Qt.exit(0);
  }
}
