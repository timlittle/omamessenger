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
import "Check.js" as Check

ShellRoot {
  id: root

  property int pollAttempts: 0
  property string expectedTitle: ""
  property var _next: null

  // retry schedules fn to run again shortly, for a condition that depends
  // on a reply from the helper or on a key event finishing its round trip.
  function retry(fn: var): void {
    root._next = fn;
    retryTimer.start();
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
    onTriggered: Check.fail("timed out before the checks finished")
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
    const listView = Check.find(panel, "conversationListView");
    if (listView && listView.count === 11) return root.checkThirdConversation(listView);

    root.pollAttempts++;
    if (root.pollAttempts >= 100)
      return Check.fail("got " + (listView ? listView.count : "no list view") + " conversations after retrying, want 11");
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
    const title = Check.find(panel, "conversationTitle");
    const composer = Check.find(panel, "composerInput");

    if (title && title.text === root.expectedTitle && composer && composer.activeFocus)
      return root.sendMessage(composer);

    root.pollAttempts++;
    if (root.pollAttempts >= 100)
      return Check.fail("opening the third conversation never finished: title=\""
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
    const messages = Check.find(panel, "messageListView");

    if (messages && messages.model) {
      for (let i = 0; i < messages.model.count; i++) {
        const m = messages.model.get(i);
        if (m.text === "hello" && m.status === "delivered") return root.checkEscapeChain();
      }
    }

    root.pollAttempts++;
    if (root.pollAttempts >= 100) return Check.fail("sent message never reached delivered");
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
    root.waitForQuestion(() => {
      if (fakeShell.hideCalls.length !== 0) return Check.fail("the window hid before asking");
      // Enter chooses the default, Keep in background.
      t.keyClick(Qt.Key_Return);
      root.waitForHide();
    });
  }

  // waitForQuestion holds until the close question is showing, then runs next.
  function waitForQuestion(next: var): void {
    const question = root.find("closeConfirm");
    if (question && question.visible) return next();

    root.pollAttempts++;
    if (root.pollAttempts >= 100) return Check.fail("the close question never showed");
    root.retry(() => root.waitForQuestion(next));
  }

  // find looks an object up by name anywhere in the panel.
  function find(name: string): var {
    return Check.find(panel, name);
  }

  // waitForHide holds for the third Escape's hide to reach the shell
  // facade, since leaving the composer blurs it asynchronously.
  function waitForHide(): void {
    if (fakeShell.hideCalls.length === 1) {
      if (fakeShell.hideCalls[0] !== "io.github.omamessenger")
        return Check.fail("shell.hide called with \"" + fakeShell.hideCalls[0] + "\"");
      return root.checkRailFilter();
    }

    if (fakeShell.hideCalls.length > 1)
      return Check.fail("escape chain called shell.hide " + fakeShell.hideCalls.length + " times, want 1");

    root.pollAttempts++;
    if (root.pollAttempts >= 100)
      return Check.fail("escape chain never hid the window, called shell.hide " + fakeShell.hideCalls.length + " times");
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
    const listView = Check.find(panel, "conversationListView");
    if (listView && listView.count === 5) return root.checkHelp();

    root.pollAttempts++;
    if (root.pollAttempts >= 100)
      return Check.fail("Ctrl+2 gave " + (listView ? listView.count : "?") + " rows, want 5");
    root.retry(root.waitForRailFilter);
  }

  // checkHelp opens the command palette with Ctrl+/, finds "New message"
  // by typing part of it, and runs it with Enter, which opens the new-chat
  // dialog. Escape then closes the dialog.
  function checkHelp(): void {
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);

    const palette = Check.find(panel, "commandPalette");
    if (!palette || !palette.visible) return Check.fail("Ctrl+/ did not open the command palette");

    for (const ch of "new mes") t.keyClick(ch);
    if (palette.items.length === 0 || palette.items[0].label !== "New message")
      return Check.fail("typing \"new mes\" listed " + JSON.stringify(palette.items.map(i => i.label)));
    if (palette.items[0].keys !== "Ctrl+N") return Check.fail("the palette does not show New message's shortcut");

    t.keyClick(Qt.Key_Return);
    const dialog = Check.find(panel, "newChatDialog");
    if (palette.visible) return Check.fail("running a command left the palette open");
    if (!dialog || !dialog.visible) return Check.fail("running New message did not open the new-chat dialog");

    t.keyClick(Qt.Key_Escape);
    if (dialog.visible) return Check.fail("Escape did not close the new-chat dialog");

    root.checkMinimumSize();
  }

  // checkMinimumSize resizes the window to the documented minimum and
  // checks every column still has real width, rather than collapsing.
  function checkMinimumSize(): void {
    const win = Check.find(panel, "panelWindow");
    win.width = Style.space(760);
    win.height = Style.space(540);

    const rail = Check.find(panel, "serviceRail");
    const listColumn = Check.find(panel, "listColumn");
    const conversationView = Check.find(panel, "conversationView");

    if (!rail || rail.width <= 0) return Check.fail("service rail has no width at the minimum size");
    if (!listColumn || listColumn.width <= 0) return Check.fail("list column has no width at the minimum size");
    if (!conversationView || conversationView.width <= 0) return Check.fail("conversation view has no width at the minimum size");
    if (listColumn.width > Style.space(360) + 1)
      return Check.fail("list column is " + listColumn.width + " wide, more than its maximum of " + Style.space(360));
    if (conversationView.width < Style.space(300))
      return Check.fail("conversation view is only " + conversationView.width + " wide at the minimum size");

    root.checkQuit();
  }

  // checkQuit asks to close with Ctrl+W, chooses Quit, and checks the
  // helper stops; reopening the window starts it again.
  function checkQuit(): void {
    t.keyClick(Qt.Key_W, Qt.ControlModifier);
    root.pollAttempts = 0;
    root.waitForQuestion(() => {
      // Left moves from Keep in background to Quit.
      t.keyClick(Qt.Key_Left);
      t.keyClick(Qt.Key_Return);
      if (service.status !== "stopped") return Check.fail("Left then Enter on the close question left the helper " + service.status);

      panel.open("{}");
      root.pollAttempts = 0;
      root.waitForReadyAgain();
    });
  }

  // waitForReadyAgain holds until reopening has restarted the helper.
  function waitForReadyAgain(): void {
    if (service.status === "ready") return root.checkCompositorClose();

    root.pollAttempts++;
    if (root.pollAttempts >= 100) return Check.fail("reopening did not restart the helper: " + service.status);
    root.retry(root.waitForReadyAgain);
  }

  // checkCompositorClose closes the window the way the compositor does,
  // which cannot be refused, and checks it comes back asking.
  function checkCompositorClose(): void {
    const hidesBefore = fakeShell.hideCalls.length;
    root.find("panelWindow").visible = false;

    root.pollAttempts = 0;
    root.waitForQuestion(() => {
      if (!root.find("panelWindow").visible) return Check.fail("the window did not come back to ask");
      if (fakeShell.hideCalls.length !== hidesBefore) return Check.fail("the window reported hidden before asking");

      t.keyClick(Qt.Key_Escape);
      if (root.find("closeConfirm").visible) return Check.fail("Escape did not cancel the close question");
      console.log("PASS Panel");
      Qt.exit(0);
    });
  }
}
