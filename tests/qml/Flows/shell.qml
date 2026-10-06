// Drives the whole window through the flows a person relies on, with real
// key events against the demo helper:
// - search for "ticket" finds Alex Chen, Enter opens it, its unread clears
//   and the bar widget's count follows
// - a draft survives the panel being destroyed and recreated
// - Ctrl+K jumps to Sam, whose first send fails, and r retries it
// - a message arriving in Sam's chat while the window is hidden stays unread
// - Ctrl+N, cycling every account, "Ben", Enter opens a new chat with Ben
// Each step polls until its condition holds, because every answer comes
// back from the helper asynchronously.
import QtQuick
import QtTest
import Quickshell
import "ui"

ShellRoot {
  id: root

  property int step: 0
  property int attempts: 0
  property string expected: ""
  property int samUnread: -1
  property var steps: [
    root.waitForList,
    root.searchTicket,
    root.waitForSearch,
    root.openMatch,
    root.waitForAlexRead,
    root.typeDraft,
    root.recreatePanel,
    root.waitForDraftRestored,
    root.openSam,
    root.waitForSamFailed,
    root.retryWithR,
    root.waitForSamDelivered,
    root.hideWithSamOpen,
    root.messageWhileHidden,
    root.waitForSamUnread,
    root.reopen,
    root.newChatWithBen,
    root.waitForBen
  ]

  // fail stops the test with a reason on stderr.
  function fail(reason: string): void {
    console.error("FAIL step " + root.step + ": " + reason);
    Qt.exit(1);
  }

  // find returns the descendant of item with the given objectName.
  function find(item: var, name: string): var {
    if (!item) return null;
    if (item.objectName === name) return item;

    for (const child of (item.data || item.children || [])) {
      const found = root.find(child, name);
      if (found) return found;
    }
    return null;
  }

  // panel is the live Panel, which the Loader may have recreated.
  function panel(): var {
    return panelLoader.item;
  }

  // listModel is the conversation list the window shows.
  function listModel(): var {
    return root.find(root.panel(), "conversationListView").model;
  }

  // rowIndex returns the visible row with the given title, or -1.
  function rowIndex(title: string): int {
    const model = root.listModel();
    for (let i = 0; i < model.count; i++) {
      if (model.get(i).title === title) return i;
    }
    return -1;
  }

  // messageStatus returns the status of the newest message with text, or "".
  function messageStatus(text: string): string {
    const model = root.find(root.panel(), "messageListView").model;
    for (let i = 0; i < model.count; i++) {
      if (model.get(i).text === text) return model.get(i).status;
    }
    return "";
  }

  // title is the open conversation's header title.
  function title(): string {
    const header = root.find(root.panel(), "conversationTitle");
    return header ? header.text : "";
  }

  // type sends each character of text as a key click.
  function type(text: string): void {
    for (const ch of text) t.keyClick(ch);
  }

  // waitForList holds until the demo helper has seeded all 11 chats.
  function waitForList(): var {
    return root.listModel().count === 11;
  }

  // searchTicket focuses search with Ctrl+G and types a query.
  function searchTicket(): var {
    t.keyClick(Qt.Key_G, Qt.ControlModifier);
    root.type("ticket");
    return true;
  }

  // waitForSearch holds until only Alex Chen matches.
  function waitForSearch(): var {
    const model = root.listModel();
    return model.count === 1 && model.get(0).title === "Alex Chen";
  }

  // openMatch opens the first search result with Enter.
  function openMatch(): var {
    root.expected = String(helperService.unreadTotal);
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForAlexRead holds until Alex Chen is open, its unread message is
  // read and the bar widget shows the lower total.
  function waitForAlexRead(): var {
    if (root.title() !== "Alex Chen") return false;
    if (String(helperService.unreadTotal) === root.expected) return false;

    const badge = root.find(barWidget, "unreadBadge");
    if (badge.count !== helperService.unreadTotal)
      return "bar shows " + badge.count + ", service has " + helperService.unreadTotal;
    return true;
  }

  // typeDraft leaves unsent text in the composer.
  function typeDraft(): var {
    root.type("draft kept");
    return true;
  }

  // recreatePanel destroys the panel and builds a new one, as Omarchy does
  // each time the window is hidden and summoned.
  function recreatePanel(): var {
    panelLoader.active = false;
    panelLoader.active = true;
    root.panel().open("{}");
    return true;
  }

  // waitForDraftRestored holds until the new panel reopens Alex Chen with
  // the draft still in the composer.
  function waitForDraftRestored(): var {
    const composer = root.find(root.panel(), "composerInput");
    return root.title() === "Alex Chen" && composer && composer.text === "draft kept";
  }

  // openSam jumps to Sam's chat with Ctrl+K, the conversation switcher,
  // and sends a message there.
  function openSam(): var {
    t.keyClick(Qt.Key_Escape);
    t.keyClick(Qt.Key_K, Qt.ControlModifier);
    root.type("sam");
    t.keyClick(Qt.Key_Return);
    root.type("are you there");
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForSamFailed holds until Sam's spotty signal fails the send.
  function waitForSamFailed(): var {
    return root.title() === "Sam (spotty signal)" && root.messageStatus("are you there") === "failed";
  }

  // retryWithR leaves the composer and retries with r.
  function retryWithR(): var {
    t.keyClick(Qt.Key_Escape);
    t.keyClick(Qt.Key_R);
    return true;
  }

  // waitForSamDelivered holds until the retried message is delivered.
  function waitForSamDelivered(): var {
    return root.messageStatus("are you there") === "delivered";
  }

  // hideWithSamOpen closes the window with Sam's chat still open and
  // keeps OmaMessenger running in the background.
  function hideWithSamOpen(): var {
    t.keyClick(Qt.Key_W, Qt.ControlModifier);
    const question = root.find(root.panel(), "closeConfirm");
    if (!question || !question.visible) return false;

    t.keyClick(Qt.Key_Return);
    return !root.find(root.panel(), "panelWindow").visible;
  }

  // messageWhileHidden has a demo message arrive in Sam's chat. The list
  // may still be filtered by the earlier search, so it asks the helper.
  function messageWhileHidden(): var {
    root.samUnread = -1;
    helperService.request("conversations.list", { query: "Sam (spotty" }, function(error, result) {
      helperService.request("demo.inject", { conversationId: result[0].id }, function() {});
    });
    return true;
  }

  // waitForSamUnread holds until that message counts as unread: nobody is
  // looking at a hidden window, so it must not be read on arrival.
  function waitForSamUnread(): var {
    helperService.request("conversations.list", { query: "Sam (spotty" }, function(error, result) {
      root.samUnread = result[0].unread;
    });
    return root.samUnread > 0;
  }

  // reopen shows the window again for the remaining steps.
  function reopen(): var {
    root.panel().open("{}");
    return true;
  }

  // newChatWithBen starts a chat with a WhatsApp contact. Ctrl+Tab once per
  // account cycles all the way round, back to the WhatsApp account the
  // dialog opens on.
  function newChatWithBen(): var {
    t.keyClick(Qt.Key_N, Qt.ControlModifier);
    for (let i = 0; i < helperService.accounts.length; i++) t.keyClick(Qt.Key_Tab, Qt.ControlModifier);
    root.type("Ben");
    root.expected = "";
    return true;
  }

  // waitForBen presses Enter once Ben is listed, then holds until his
  // conversation is open.
  function waitForBen(): var {
    if (root.title() === "Ben Okafor") return true;

    const dialog = root.find(root.panel(), "newChatDialog");
    if (root.expected === "" && dialog && dialog.contacts.length === 1 && dialog.contacts[0].name === "Ben Okafor") {
      root.expected = "sent";
      t.keyClick(Qt.Key_Return);
    }
    return false;
  }

  // runStep runs the current step and advances, retries or fails.
  function runStep(): void {
    const result = root.steps[root.step]();
    if (typeof result === "string") return root.fail(result);

    if (result === true) {
      root.step++;
      root.attempts = 0;
      if (root.step === root.steps.length) {
        console.log("PASS Flows");
        return Qt.exit(0);
      }
    } else if (++root.attempts > 100) {
      return root.fail("condition not met within 10 s");
    }

    stepTimer.start();
  }

  // Named so Panel's own service property, inside the Loader, does not
  // shadow it and bind to itself.
  Service {
    id: helperService
  }

  QtObject {
    id: fakeShell

    function hide(id) { root.panel().close(); }
    function serviceFor(id) { return helperService; }
    function toggle(id, payloadJson) { root.panel().open(payloadJson); }
    function summon(id, payloadJson) { root.panel().open(payloadJson); }
  }

  Loader {
    id: panelLoader

    sourceComponent: Panel {
      service: helperService
      shell: fakeShell
    }
  }

  BarWidget {
    id: barWidget

    bar: ({ shell: fakeShell })
  }

  TestCase {
    id: t

    when: false
  }

  Timer {
    id: stepTimer

    interval: 100
    onTriggered: root.runStep()
  }

  // A backstop: it only fires if a step hangs without failing.
  Timer {
    running: true
    interval: 55000
    onTriggered: root.fail("timed out")
  }

  // Start once Quickshell has finished loading; Qt.exit() is ignored
  // before then.
  Timer {
    running: true
    interval: 50
    onTriggered: {
      root.panel().open("{}");
      root.runStep();
    }
  }
}
