// Checks the fix for a spoofing bug: a message that is both highlighted
// and carries a link whose visible text does not match where it goes
// opens the mismatched-link question on "o", the same as the real UI.
// Before the fix, Navigation.js's keyContext/buildState never read
// WindowController.linkConfirmOpen, so with the question open the
// context stayed "conversation": Enter ran message.open and opened the
// highlighted photo anyway, and Escape closed the whole conversation
// instead of just the question. This drives real key events through
// the same routeKey dispatch Panel.qml uses (Navigation.buildState,
// Keymap.match, then whichever controller owns the action), wired to
// the real WindowController and ConversationController against a
// scripted fake service, so it proves the wiring, not Navigation.js's
// own precedence table (covered exhaustively in
// tests/unit/navigation.test.cjs).
import QtQuick
import QtTest
import Quickshell
import "ui/controllers"
import "ui/lib/Keymap.js" as Keymap
import "ui/lib/Navigation.js" as Navigation
import "Check.js" as Check

ShellRoot {
  id: root

  // openedLinks records every URL openExternalLink was asked to open.
  property var openedLinks: []

  QtObject {
    id: service

    property var accounts: []
    property var services: []
    property var uiState: ({})
    property var pendingAuth: null
    property string status: "ready"
    property string detail: ""
    property int unreadTotal: 0

    function start(): void {}
    function installHelper(): void {}
    function quit(): void {}

    // request answers messages.list with the one message this test
    // needs: highlighted by default (the only, newest message), with a
    // photo attachment and a link whose visible text ("Click here")
    // does not match where it actually goes.
    function request(method: string, params: var, callback: var): void {
      if (method === "messages.list") {
        callback(null, { hasMore: false, messages: [
          { id: "m1", conversationId: "c1", senderId: "s1", senderName: "Alex", text: "Click here",
            outgoing: false, status: "received", created: Date.now(),
            media: { kind: "photo", width: 400, height: 300 },
            formatting: [{ kind: "link", start: 0, end: 10, url: "https://evil.example/phish" }] }
        ] });
        return;
      }

      callback(null, {});
    }
  }

  ConversationController {
    id: conversationController
    service: service
    photoViewer: photoViewerController

    // The same wiring MessengerLayout.qml does for a single-link
    // message: hand it straight to the window controller's own
    // mismatched-text question rather than opening it outright.
    onLinksRequested: (urls) => windowController.openLink(urls[0].url, urls[0].displayText, urls[0].suspicious === true)
  }

  PhotoViewerController {
    id: photoViewerController
    conversation: conversationController
  }

  WindowController {
    id: windowController
    service: service
    conversationController: conversationController
    // The dialog only asks to open a remote URL through the real
    // openExternalLink; this test never needs that to actually happen,
    // only that "y" reaches it and "Enter"/"n"/Esc never do.
    openExternalLink: (url) => root.openedLinks.push(url)
  }

  // routeKey is the same dispatch Panel.qml's own routeKey runs:
  // compute the context from the real controllers' state, match the
  // key, then let whichever controller owns the action run it.
  function routeKey(key: int, modifiers: int, text: string): bool {
    const state = Navigation.buildState({
      service: service, windowController: windowController, photoViewerController: photoViewerController,
      conversationController: conversationController
    });
    const context = Navigation.keyContext(state);
    const action = Keymap.match(context, key, modifiers, text);
    if (!action) return false;

    const controllers = [conversationController, windowController];
    const owner = controllers.find((c) => c.handles(action));
    if (!owner) return false;

    owner.run(action);
    return true;
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
    }
  }

  TestCase {
    id: t
    when: false
  }

  Timer {
    running: true
    interval: 50
    onTriggered: root.start()
  }

  function start(): void {
    conversationController.open({ id: "c1", accountId: "a1", service: "whatsapp", remoteId: "r1",
      kind: "direct", title: "Alex", members: 0, preview: "", muted: false, unread: 0, lastActivity: Date.now() });
    root.checkEnterOpensTheQuestionNotTheMedia();
  }

  // checkEnterOpensTheQuestionNotTheMedia opens the mismatched-link
  // question with "o" on the highlighted photo message, then checks
  // Enter cancels the question rather than falling through to
  // message.open and opening the photo underneath it.
  function checkEnterOpensTheQuestionNotTheMedia(): void {
    if (conversationController.highlightedId !== "m1") return Check.fail("setup: m1 is not highlighted");

    t.keyClick(Qt.Key_O);
    if (!windowController.linkConfirmOpen) return Check.fail("\"o\" did not open the mismatched-link question");
    if (windowController.pendingLinkURL !== "https://evil.example/phish")
      return Check.fail("the question is not about the message's own link: " + windowController.pendingLinkURL);

    t.keyClick(Qt.Key_Return);
    if (windowController.linkConfirmOpen) return Check.fail("Enter did not cancel the open question");
    if (photoViewerController.viewerOpen) return Check.fail("Enter fell through and opened the highlighted photo");
    if (root.openedLinks.length !== 0) return Check.fail("Enter opened the link: " + JSON.stringify(root.openedLinks));

    root.checkEscapeClosesOnlyTheQuestion();
  }

  // checkEscapeClosesOnlyTheQuestion reopens the question and checks
  // Escape cancels it alone, leaving the conversation open behind it -
  // not close-conversation, which Escape would run if the context had
  // fallen through to "conversation" underneath the dialog.
  function checkEscapeClosesOnlyTheQuestion(): void {
    t.keyClick(Qt.Key_O);
    if (!windowController.linkConfirmOpen) return Check.fail("setup: \"o\" did not reopen the question");

    t.keyClick(Qt.Key_Escape);
    if (windowController.linkConfirmOpen) return Check.fail("Escape did not cancel the open question");
    if (conversationController.activeId !== "c1") return Check.fail("Escape closed the conversation instead of just the question");

    root.checkYOpensTheLink();
  }

  // checkYOpensTheLink reopens the question and checks "y" is the one
  // key that actually opens the link.
  function checkYOpensTheLink(): void {
    t.keyClick(Qt.Key_O);
    if (!windowController.linkConfirmOpen) return Check.fail("setup: \"o\" did not reopen the question");

    root.openedLinks = [];
    t.keyClick(Qt.Key_Y);
    if (windowController.linkConfirmOpen) return Check.fail("\"y\" did not close the question");
    if (JSON.stringify(root.openedLinks) !== '["https://evil.example/phish"]')
      return Check.fail("\"y\" did not open the link: " + JSON.stringify(root.openedLinks));

    console.log("PASS LinkConfirmKeys");
    Qt.exit(0);
  }
}
