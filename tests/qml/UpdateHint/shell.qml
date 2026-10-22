// Checks the one-time "Updated to vX" hint's wiring into
// WindowController's toastText: whether to show it at all is
// WhatsNew.shouldShowHint's own job, proven exhaustively in
// tests/unit/whatsnew.test.cjs, and the priority between it and an
// ordinary notice is Toast.pickText's own job, proven exhaustively in
// tests/unit/toast.test.cjs. This only checks that service.showUpdateHint
// and service.appVersion actually reach the toast, that an ordinary
// confirmation notice (such as copying or saving, see
// ConversationController.notice) reaches it too, and that dismissing it
// (through the same Escape chain Toast.qml's own auto-dismiss and click
// also reach, proven in tests/unit/navigation.test.cjs) keeps it gone for
// the rest of the run even though showUpdateHint itself never goes false
// on its own.
import QtQuick
import Quickshell
import "ui/controllers"
import "ui/lib/WhatsNew.js" as WhatsNew
import "Check.js" as Check

ShellRoot {
  id: root

  // service is a scripted double: just enough for WindowController and
  // the controllers it wires together, with showUpdateHint and
  // appVersion standing in for what Service.qml's own FileView and
  // whatsNew.getSeenVersion round trip would otherwise set.
  QtObject {
    id: service

    property var accounts: []
    property string status: "ready"
    property var uiState: ({ pane: "list", activeId: "" })
    property bool showUpdateHint: false
    property string appVersion: "0.3.5"

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {}
  }

  ListController {
    id: listController
    service: service
  }

  ConversationController {
    id: conversationController
    service: service
    listController: listController
  }

  WindowController {
    id: windowController
    service: service
    listController: listController
    conversationController: conversationController
  }

  Stepper {
    id: stepper
    name: "UpdateHint"
    intervalMs: 20
    deadlineMs: 10000
    onTimeout: () => Check.fail("timed out before the checks finished")
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.checkHintShowsAndWins()
  }

  // checkHintShowsAndWins turns the hint on and confirms it shows the
  // right words, then confirms it wins over an unrelated confirmation
  // notice while both are set.
  function checkHintShowsAndWins(): void {
    const want = WhatsNew.hintText(service.appVersion);
    service.showUpdateHint = true;
    if (windowController.toastText !== want)
      return Check.fail("toastText did not show the update hint: " + windowController.toastText);

    conversationController.timeline.notice = "Copied to clipboard";
    if (windowController.toastText !== want)
      return Check.fail("an ordinary notice overrode the update hint: " + windowController.toastText);

    root.checkDismissSticks();
  }

  // checkDismissSticks dismisses the toast through the same chain Escape
  // uses (which clears every toast source at once, the same way
  // dismissErrorBanner already clears every error source at once), then
  // confirms the hint does not come back on a later, unrelated notice
  // even though showUpdateHint itself is still true.
  function checkDismissSticks(): void {
    windowController.run("escape");
    if (windowController.toastText !== "")
      return Check.fail("escape left something showing: " + windowController.toastText);

    conversationController.timeline.notice = "Saved";
    if (windowController.toastText !== "Saved")
      return Check.fail("the update hint came back after being dismissed: " + windowController.toastText);

    stepper.pass();
  }
}
