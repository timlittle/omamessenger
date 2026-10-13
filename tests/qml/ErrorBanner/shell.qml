// Checks the dismissible helper-error banner's wiring: WindowController
// merges whichever controller last failed into errorBannerText (the merge
// priority itself is proven exhaustively in
// tests/unit/errorbanner.test.cjs), Escape dismisses it before anything
// else in the chain (that priority is proven in
// tests/unit/navigation.test.cjs), a click-equivalent dismiss and the
// auto-clear timer both reach every source including the open
// conversation's bound lastError, opening another conversation clears it,
// and a successful reply after a failure clears it on its own.
import QtQuick
import Quickshell
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  // service is a scripted double: request() just remembers the latest
  // callback per method, so the test answers it by hand instead of
  // waiting on a real helper.
  QtObject {
    id: service

    property var accounts: []
    property string status: "ready"
    property var uiState: ({ pane: "list", activeId: "" })
    property var pending: ({})

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {
      service.pending[method] = callback;
    }
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
    errorBannerTimeoutMs: 20
  }

  Stepper {
    id: stepper
    name: "ErrorBanner"
    intervalMs: 20
    deadlineMs: 10000
    onTimeout: () => Check.fail("timed out before the checks finished")
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.checkMergeAndEscape()
  }

  // checkMergeAndEscape sets a failure on the list controller, confirms
  // the banner shows it, then confirms Escape dismisses only the banner
  // and leaves whatever else is open (the search field) untouched.
  function checkMergeAndEscape(): void {
    listController.lastError = "could not load conversations";
    if (windowController.errorBannerText !== "could not load conversations")
      return Check.fail("errorBannerText did not pick up listController.lastError: " + windowController.errorBannerText);

    listController.searchFocused = true;
    windowController.run("escape");
    if (windowController.errorBannerText !== "") return Check.fail("escape did not dismiss the visible error");
    if (!listController.searchFocused) return Check.fail("escape also left the search field in the same press");

    listController.searchFocused = false;
    root.checkClickDismiss();
  }

  // checkClickDismiss confirms dismissErrorBanner, what the banner's own
  // dismiss button calls, reaches the open conversation's lastError too,
  // even though that one is a binding rather than a plain property.
  function checkClickDismiss(): void {
    conversationController.timeline.lastError = "could not send";
    if (windowController.errorBannerText !== "could not send")
      return Check.fail("errorBannerText did not pick up the open conversation's error");

    windowController.dismissErrorBanner();
    if (windowController.errorBannerText !== "") return Check.fail("dismissErrorBanner left the conversation's error showing");
    if (conversationController.lastError !== "") return Check.fail("dismissErrorBanner did not clear the bound conversation lastError");

    root.checkAutoClear();
  }

  // checkAutoClear confirms the banner clears itself once its (shortened,
  // for this test) timeout elapses, with nothing else dismissing it.
  function checkAutoClear(): void {
    listController.lastError = "could not archive";
    stepper.retry(root.waitForAutoClear);
  }

  // waitForAutoClear polls until the timer above fires.
  function waitForAutoClear(): void {
    if (windowController.errorBannerText === "") return root.checkNavigationClears();

    stepper.attempts++;
    if (stepper.attempts >= stepper.maxAttempts) return Check.fail("the error never cleared on its own");
    stepper.retry(root.waitForAutoClear);
  }

  // checkNavigationClears opens a conversation while an error from a
  // different controller is still showing, and confirms opening it
  // cleared the banner even though that error was never the
  // conversation's own: opening a conversation is a fresh start.
  function checkNavigationClears(): void {
    listController.lastError = "could not mute";
    conversationController.open({ id: "c1", title: "Test" });
    if (windowController.errorBannerText !== "") return Check.fail("opening a conversation did not clear the banner");

    root.checkSuccessClears();
  }

  // checkSuccessClears fails a send, confirms the banner shows it, then
  // answers a second send with success and confirms the banner clears on
  // its own, with no Escape, click or timer involved.
  function checkSuccessClears(): void {
    conversationController.send("first try", "", []);
    service.pending["messages.send"]({ code: -32603 });
    if (windowController.errorBannerText === "") return Check.fail("a failed send did not show in the banner");

    conversationController.send("second try", "", []);
    service.pending["messages.send"](null, { id: "m2", conversationId: "c1", text: "second try", created: Date.now(), outgoing: true });
    if (windowController.errorBannerText !== "") return Check.fail("a successful send left the earlier failure showing");

    stepper.pass();
  }
}
