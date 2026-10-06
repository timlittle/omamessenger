// Drives the real Panel and Service against the real demo helper with
// genuine key events, the way a person actually uses the window: the
// list fills with the demo conversations, j j Enter opens the third one
// and focuses the composer, typing a message and pressing Enter sends it
// and it reaches delivered, three Escapes step back through compose, the
// open conversation and finally hide the window through the host's shell
// facade, Ctrl+2 narrows the rail to Telegram, F1 opens help and Escape
// closes it, and every column still has width at the minimum window
// size. XDG_DATA_HOME is set by the test runner, so this never touches
// real data, and the mock shell facade stands in for Omarchy's host.
import QtQuick
import QtTest
import Quickshell
import qs.Commons
import "ui"

ShellRoot {
  id: root

  property int pollAttempts: 0
  property string expectedTitle: ""
  property var _next: null

  // fail stops the test with a reason on stderr.
  function fail(reason: string): void {
    console.error("FAIL " + reason);
    Qt.exit(1);
  }

  // retry schedules fn to run again shortly, for a condition that depends
  // on a reply from the helper or on a key event finishing its round trip.
  function retry(fn: var): void {
    root._next = fn;
    retryTimer.start();
  }

  // findByObjectName searches item and its descendants for a matching
  // objectName, since Panel exposes nothing of its internals but this.
  // It walks `data`, the default property every Item and window type
  // stores its declared children in, rather than `children`, which
  // leaves out windows and other non-visual objects.
  function findByObjectName(item: var, name: string): var {
    if (!item) return null;
    if (item.objectName === name) return item;

    const kids = item.data || item.children || [];
    for (const child of kids) {
      const found = root.findByObjectName(child, name);
      if (found) return found;
    }
    return null;
  }

  Service {
    id: service
  }

  QtObject {
    id: fakeShell

    property var hideCalls: []

    function hide(id) { fakeShell.hideCalls.push(id); panel.close(); }
    function serviceFor(id) { return service; }
    function toggle(id, payloadJson) { panel.open(payloadJson); }
    function summon(id, payloadJson) { panel.open(payloadJson); }
  }

  Panel {
    id: panel
    service: service
    shell: fakeShell
  }

  TestCase {
    id: t
    when: false
  }

  Timer {
    id: retryTimer
    interval: 100
    onTriggered: root._next()
  }

  // A deliberately unreachable deadline: it only fires, and fails the
  // test with a reason, if something above never happens.
  Timer {
    running: true
    interval: 55000
    onTriggered: root.fail("timed out before the checks finished")
  }

  // Checks run once Quickshell has finished loading; Qt.exit() is
  // ignored before then.
  Timer {
    running: true
    interval: 50
    onTriggered: root.start()
  }

  // start opens the window and begins waiting for the demo data to load.
  function start(): void {
    panel.open("{}");
    root.waitForConversations();
  }

  // waitForConversations retries while the demo connectors are still
  // seeding, the same race every other test against the demo helper has
  // to account for.
  function waitForConversations(): void {
    const listView = root.findByObjectName(panel, "conversationListView");
    if (listView && listView.count === 11) return root.checkThirdConversation(listView);

    root.pollAttempts++;
    if (root.pollAttempts >= 100)
      return root.fail("got " + (listView ? listView.count : "no list view") + " conversations after retrying, want 11");
    root.retry(root.waitForConversations);
  }

  // checkThirdConversation reads the row at index 2 (0-based) straight
  // from the list's own model, which exists whether or not that row's
  // delegate happens to be instantiated, then drives j, j, Enter and
  // waits for it to open with the composer focused.
  function checkThirdConversation(listView: var): void {
    root.expectedTitle = listView.model.get(2).title;

    t.keyClick(Qt.Key_J);
    t.keyClick(Qt.Key_J);
    t.keyClick(Qt.Key_Return);

    root.pollAttempts = 0;
    root.waitForConversationOpen();
  }

  // waitForConversationOpen holds until the header names the row j, j
  // landed on and the composer has keyboard focus.
  function waitForConversationOpen(): void {
    const title = root.findByObjectName(panel, "conversationTitle");
    const composer = root.findByObjectName(panel, "composerInput");

    if (title && title.text === root.expectedTitle && composer && composer.activeFocus)
      return root.sendMessage(composer);

    root.pollAttempts++;
    if (root.pollAttempts >= 100)
      return root.fail("opening the third conversation never finished: title=\""
        + (title ? title.text : "?") + "\" want \"" + root.expectedTitle
        + "\" composerFocus=" + (composer ? composer.activeFocus : "?"));
    root.retry(root.waitForConversationOpen);
  }

  // sendMessage types "hello" character by character, the way a person
  // would, then presses Enter, which must send the message rather than
  // insert a newline.
  function sendMessage(composer: var): void {
    for (const ch of "hello") t.keyClick(ch);
    t.keyClick(Qt.Key_Return);

    root.pollAttempts = 0;
    root.waitForDelivered();
  }

  // waitForDelivered holds until the sent message's status reaches
  // delivered through a message.updated event, read straight from the
  // message list's own model.
  function waitForDelivered(): void {
    const messages = root.findByObjectName(panel, "messageListView");

    if (messages && messages.model) {
      for (let i = 0; i < messages.model.count; i++) {
        const m = messages.model.get(i);
        if (m.text === "hello" && m.status === "delivered") return root.checkEscapeChain();
      }
    }

    root.pollAttempts++;
    if (root.pollAttempts >= 100) return root.fail("sent message never reached delivered");
    root.retry(root.waitForDelivered);
  }

  // checkEscapeChain steps Escape back through leaving the composer and
  // closing the conversation, then checks the final Escape hides the
  // window through the host's shell facade.
  function checkEscapeChain(): void {
    t.keyClick(Qt.Key_Escape);
    t.keyClick(Qt.Key_Escape);
    t.keyClick(Qt.Key_Escape);

    root.pollAttempts = 0;
    root.waitForHide();
  }

  // waitForHide holds for the third Escape's hide to reach the shell
  // facade, since leaving the composer blurs it asynchronously.
  function waitForHide(): void {
    if (fakeShell.hideCalls.length === 1) {
      if (fakeShell.hideCalls[0] !== "io.github.omamessenger")
        return root.fail("shell.hide called with \"" + fakeShell.hideCalls[0] + "\"");
      return root.checkRailFilter();
    }

    if (fakeShell.hideCalls.length > 1)
      return root.fail("escape chain called shell.hide " + fakeShell.hideCalls.length + " times, want 1");

    root.pollAttempts++;
    if (root.pollAttempts >= 100)
      return root.fail("escape chain never hid the window, called shell.hide " + fakeShell.hideCalls.length + " times");
    root.retry(root.waitForHide);
  }

  // checkRailFilter reopens the window and narrows the rail to Telegram.
  function checkRailFilter(): void {
    panel.open("{}");
    t.keyClick(Qt.Key_2, Qt.ControlModifier);

    root.pollAttempts = 0;
    root.waitForRailFilter();
  }

  // waitForRailFilter holds for the Telegram-only row count the demo
  // data gives, the same number tests/qml/Controllers/shell.qml checks.
  function waitForRailFilter(): void {
    const listView = root.findByObjectName(panel, "conversationListView");
    if (listView && listView.count === 5) return root.checkHelp();

    root.pollAttempts++;
    if (root.pollAttempts >= 100)
      return root.fail("Ctrl+2 gave " + (listView ? listView.count : "?") + " rows, want 5");
    root.retry(root.waitForRailFilter);
  }

  // checkHelp opens the shortcut sheet with F1 and closes it with Escape.
  function checkHelp(): void {
    t.keyClick(Qt.Key_F1);

    const help = root.findByObjectName(panel, "shortcutHelp");
    if (!help || !help.visible) return root.fail("F1 did not open the shortcut help");

    t.keyClick(Qt.Key_Escape);
    if (help.visible) return root.fail("Escape did not close the shortcut help");

    root.checkMinimumSize();
  }

  // checkMinimumSize resizes the window to the documented minimum and
  // checks every column still has real width, rather than collapsing.
  function checkMinimumSize(): void {
    const win = root.findByObjectName(panel, "panelWindow");
    win.width = Style.space(760);
    win.height = Style.space(540);

    const rail = root.findByObjectName(panel, "serviceRail");
    const listColumn = root.findByObjectName(panel, "listColumn");
    const conversationView = root.findByObjectName(panel, "conversationView");

    if (!rail || rail.width <= 0) return root.fail("service rail has no width at the minimum size");
    if (!listColumn || listColumn.width <= 0) return root.fail("list column has no width at the minimum size");
    if (!conversationView || conversationView.width <= 0) return root.fail("conversation view has no width at the minimum size");

    console.log("PASS Panel");
    Qt.exit(0);
  }
}
