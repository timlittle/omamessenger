// Checks account setup end to end against a scripted service: adding
// sends accounts.add at once with OmaMessenger's keys, your own API id and
// hash are checked before they are sent, each auth.step shows
// its stage, answers go out as auth.submit, connecting closes setup,
// cancelling a half-added account removes it, and a step that arrived
// while the panel was gone is shown when it is created again, where
// cancelling keeps the saved account. No helper runs, so nothing reaches
// Telegram.
import QtQuick
import Quickshell
import "ui/components"
import "ui/controllers"

ShellRoot {
  id: root

  // fail stops the test with a reason on stderr; each check returns its
  // result so run() stops at the first failure.
  function fail(reason: string): bool {
    console.error("FAIL " + reason);
    Qt.exit(1);
    return false;
  }

  // last returns the most recent request the service received.
  function last(): var {
    return service.requests[service.requests.length - 1] || {};
  }

  // child finds a descendant of item by objectName.
  function child(item: var, name: string): var {
    for (const c of item.children) {
      if (c.objectName === name) return c;
      const found = root.child(c, name);
      if (found) return found;
    }
    return null;
  }

  QtObject {
    id: service

    property string status: "ready"
    property var accounts: []
    property var pendingAuth: null
    property var requests: []

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {
      service.requests = service.requests.concat([{ method: method, params: params }]);
      const replies = { "accounts.add": { id: "tg-1", service: "telegram", name: "Telegram", status: "connecting" } };
      callback(null, replies[method] || {});
    }
  }

  AccountController {
    id: controller
    service: service
  }

  Component {
    id: controllerComponent
    AccountController {}
  }

  FloatingWindow {
    implicitWidth: 800
    implicitHeight: 600
    visible: true

    AccountSetup {
      id: view
      visible: controller.open
      stage: controller.stage
      qr: controller.qr
      hint: controller.hint
      error: controller.lastError
      busy: controller.busy

      onCredentialsSubmitted: (apiId, apiHash) => controller.submitCredentials(apiId, apiHash)
      onPhoneRequested: controller.usePhone()
      onAnswered: value => controller.answer(value)
      onCancelled: controller.cancel()
    }
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // run drives each scenario in turn.
  function run(): void {
    if (!root.checkAddsWithoutKeys()) return;
    if (!root.checkCredentials()) return;
    if (!root.checkSignInByPhone()) return;
    if (!root.checkCancelRemoves()) return;
    if (!root.checkResumesPendingStep()) return;

    console.log("PASS AccountSetup");
    Qt.exit(0);
  }

  // checkAddsWithoutKeys adds the account straight away, with no keys.
  function checkAddsWithoutKeys(): bool {
    controller.run("account.add");
    const sent = root.last();
    if (sent.method !== "accounts.add" || sent.params.apiId !== undefined) return root.fail("account.add sent " + JSON.stringify(sent));
    if (!controller.open || controller.stage !== "waiting") return root.fail("account.add is not waiting: " + controller.stage);

    controller.cancel();
    service.requests = [];
    return true;
  }

  // checkCredentials refuses a non-numeric id, then adds the account with
  // the user's own keys.
  function checkCredentials(): bool {
    controller.run("account.addOwnKeys");
    if (!controller.open || controller.stage !== "credentials") return root.fail("account.addOwnKeys did not open setup");

    root.child(view, "apiIdField").text = "abc";
    root.child(view, "apiHashField").text = "hash";
    view.submit();
    if (service.requests.length !== 0 || !controller.lastError) return root.fail("a bad API id was sent");

    root.child(view, "apiIdField").text = "123";
    view.submit();
    const sent = root.last();
    if (sent.method !== "accounts.add" || sent.params.apiId !== 123 || sent.params.apiHash !== "hash") {
      return root.fail("accounts.add sent " + JSON.stringify(sent));
    }

    if (controller.accountId !== "tg-1" || controller.stage !== "waiting") return root.fail("not waiting for tg-1");
    return true;
  }

  // checkSignInByPhone shows the QR code, switches to phone, sends the
  // phone and code, and closes once connected.
  function checkSignInByPhone(): bool {
    service.event("auth.step", { accountId: "tg-1", kind: "qr", qr: "cG5n", hint: "Scan it" });
    if (controller.stage !== "qr" || !root.child(view, "qrImage").visible) return root.fail("QR code not shown");

    controller.usePhone();
    root.child(view, "answerField").text = "+447700900000";
    view.submit();
    if (root.last().method !== "auth.submit" || root.last().params.step !== "phone") return root.fail("phone not sent");

    service.event("auth.step", { accountId: "tg-1", kind: "code", hint: "That code did not work. Check it and try again." });
    if (root.child(view, "stepHint").text.indexOf("did not work") < 0) return root.fail("wrong-code hint not shown");

    service.event("auth.step", { accountId: "tg-1", kind: "password", hint: "Two-step" });
    if (!root.child(view, "answerField").password) return root.fail("password not hidden");

    service.event("account.updated", { id: "tg-1", status: "error", detail: "Wrong password" });
    if (controller.lastError !== "Wrong password") return root.fail("error not shown: " + controller.lastError);

    service.event("account.updated", { id: "tg-1", status: "connected" });
    if (controller.open) return root.fail("setup still open after connecting");
    return true;
  }

  // checkCancelRemoves removes an account that never signed in.
  function checkCancelRemoves(): bool {
    controller.begin();
    controller.cancel();

    const sent = root.last();
    if (sent.method !== "accounts.remove" || sent.params.accountId !== "tg-1") return root.fail("cancel sent " + JSON.stringify(sent));
    if (controller.open) return root.fail("setup still open after cancel");
    return true;
  }

  // checkResumesPendingStep opens a new controller at a waiting step.
  function checkResumesPendingStep(): bool {
    service.pendingAuth = { accountId: "tg-2", kind: "code", hint: "Check your app" };
    const resumed = controllerComponent.createObject(null, { service: service });
    if (!resumed.open || resumed.stage !== "code" || resumed.accountId !== "tg-2") {
      return root.fail("pending step not resumed: " + resumed.stage);
    }

    const before = service.requests.length;
    resumed.cancel();
    if (service.requests.length !== before) return root.fail("cancelling a saved account's sign-in removed it");
    return true;
  }
}
