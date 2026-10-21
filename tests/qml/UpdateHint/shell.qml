// Checks the one-time "Updated to vX" hint's wiring into
// WindowController's noticeText: whether to show it at all is
// WhatsNew.shouldShowHint's own job, proven exhaustively in
// tests/unit/whatsnew.test.cjs; this only checks that service.showUpdateHint
// and service.appVersion reach the banner, that it wins over an ordinary
// confirmation notice while both are set, and that dismissing it (the
// banner's own dismiss, which Escape also reaches) keeps it gone for the
// rest of the run even though showUpdateHint itself never goes false on
// its own.
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
    if (windowController.noticeText !== want)
      return Check.fail("noticeText did not show the update hint: " + windowController.noticeText);

    conversationController.timeline.notice = "Copied to clipboard";
    if (windowController.noticeText !== want)
      return Check.fail("an ordinary notice overrode the update hint: " + windowController.noticeText);

    root.checkDismissSticks();
  }

  // checkDismissSticks dismisses the banner through the same chain
  // Escape uses (which clears every notice source at once, the same
  // way dismissErrorBanner already clears every error source at once),
  // then confirms the hint does not come back on a later, unrelated
  // notice even though showUpdateHint itself is still true.
  function checkDismissSticks(): void {
    windowController.run("escape");
    if (windowController.noticeText !== "")
      return Check.fail("escape left something showing: " + windowController.noticeText);

    conversationController.timeline.notice = "Saved";
    if (windowController.noticeText !== "Saved")
      return Check.fail("the update hint came back after being dismissed: " + windowController.noticeText);

    stepper.pass();
  }
}
