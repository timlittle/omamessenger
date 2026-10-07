// Checks account setup end to end against a scripted service: adding
// sends accounts.add at once with OmaMessenger's keys, your own API id and
// hash are checked before they are sent, each auth.step shows
// its stage, answers go out as auth.submit, connecting closes setup,
// cancelling a half-added account removes it, and a step that arrived
// while the panel was gone is shown when it is created again, where
// cancelling keeps the saved account. The rest drives the service chooser
// and the removal question with real key events rather than calling the
// controller directly: every step opens with keyboard focus already on
// its primary control, including the steps that have nothing to type or
// click, and both choice lists take Up/Down, wrapping onto Cancel, with
// Enter picking the highlighted one. No helper runs, so nothing reaches
// Telegram.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  property int pollAttempts: 0
  property var _next: null

  // last returns the most recent request the service received.
  function last(): var {
    return service.requests[service.requests.length - 1] || {};
  }

  // retry schedules fn to run again shortly: a stage or focus change that
  // AccountSetup or RemoveAccount defers with Qt.callLater has not
  // happened yet by the time the call that caused it returns, only once
  // this test's own call stack has unwound back to the event loop.
  function retry(fn: var): void {
    root._next = fn;
    retryTimer.start();
  }

  // waitForFocus holds until condition is true, then runs next; after
  // five seconds it fails, naming what should have had focus by then.
  function waitForFocus(condition: var, reason: string, next: var): void {
    if (condition()) return next();

    root.pollAttempts++;
    if (root.pollAttempts >= 100) return Check.fail(reason + " never got keyboard focus");
    root.retry(() => root.waitForFocus(condition, reason, next));
  }

  QtObject {
    id: service

    property string status: "ready"
    property var accounts: []
    property var pendingAuth: null
    property var requests: []
    property var services: []

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
      services: service.services
      serviceName: controller.serviceName
      qr: controller.qr
      hint: controller.hint
      error: controller.lastError
      busy: controller.busy

      onServiceChosen: serviceId => controller.chooseService(serviceId)
      onCredentialsSubmitted: (apiId, apiHash) => controller.submitCredentials(apiId, apiHash)
      onPhoneRequested: controller.usePhone()
      onAnswered: value => controller.answer(value)
      onCancelled: controller.cancel()
    }

    RemoveAccount {
      id: removeView
      open: controller.removing
      accounts: service.accounts
      error: controller.lastError

      onChosen: accountId => controller.remove(accountId)
      onCancelled: controller.cancel()
    }
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
    interval: 0
    onTriggered: root.run()
  }

  // run drives each scripted scenario in turn, then hands off to the
  // keyboard-driven ones, which poll instead of returning a plain bool
  // since they wait on deferred focus changes.
  function run(): void {
    if (!root.checkAddsWithoutKeys()) return;
    if (!root.checkCredentials()) return;
    if (!root.checkSignInByPhone()) return;
    if (!root.checkSignInToWhatsAppByLinkCode()) return;
    if (!root.checkCancelRemoves()) return;
    if (!root.checkResumesPendingStep()) return;
    if (!root.checkRemovesAnAccount()) return;
    if (!root.checkChoosesAServiceWhenThereAreSeveral()) return;

    root.pollAttempts = 0;
    root.beginKeyboardChecks();
  }

  // checkAddsWithoutKeys adds the account straight away, with no keys.
  function checkAddsWithoutKeys(): bool {
    controller.run("account.add");
    const sent = root.last();
    if (sent.method !== "accounts.add" || sent.params.apiId !== undefined) return Check.fail("account.add sent " + JSON.stringify(sent));
    if (!controller.open || controller.stage !== "waiting") return Check.fail("account.add is not waiting: " + controller.stage);

    controller.cancel();
    service.requests = [];
    return true;
  }

  // checkCredentials refuses a non-numeric id, then adds the account with
  // the user's own keys.
  function checkCredentials(): bool {
    controller.run("account.addOwnKeys");
    if (!controller.open || controller.stage !== "credentials") return Check.fail("account.addOwnKeys did not open setup");

    Check.find(view, "apiIdField").text = "abc";
    Check.find(view, "apiHashField").text = "hash";
    view.submit();
    if (service.requests.length !== 0 || !controller.lastError) return Check.fail("a bad API id was sent");

    Check.find(view, "apiIdField").text = "123";
    view.submit();
    const sent = root.last();
    if (sent.method !== "accounts.add" || sent.params.apiId !== 123 || sent.params.apiHash !== "hash") {
      return Check.fail("accounts.add sent " + JSON.stringify(sent));
    }

    if (controller.accountId !== "tg-1" || controller.stage !== "waiting") return Check.fail("not waiting for tg-1");
    return true;
  }

  // checkSignInByPhone shows the QR code, switches to phone, sends the
  // phone and code, and closes once connected.
  function checkSignInByPhone(): bool {
    service.event("auth.step", { accountId: "tg-1", kind: "qr", qr: "cG5n", hint: "Scan it" });
    if (controller.stage !== "qr" || !Check.find(view, "qrImage").visible) return Check.fail("QR code not shown");

    controller.usePhone();
    Check.find(view, "answerField").text = "+447700900000";
    view.submit();
    if (root.last().method !== "auth.submit" || root.last().params.step !== "phone") return Check.fail("phone not sent");

    service.event("auth.step", { accountId: "tg-1", kind: "code", hint: "That code did not work. Check it and try again." });
    if (Check.find(view, "stepHint").text.indexOf("did not work") < 0) return Check.fail("wrong-code hint not shown");

    service.event("auth.step", { accountId: "tg-1", kind: "password", hint: "Two-step" });
    if (!Check.find(view, "answerField").password) return Check.fail("password not hidden");

    service.event("account.updated", { id: "tg-1", status: "error", detail: "Wrong password" });
    if (controller.lastError !== "Wrong password") return Check.fail("error not shown: " + controller.lastError);

    service.event("account.updated", { id: "tg-1", status: "connected" });
    if (controller.open) return Check.fail("setup still open after connecting");
    return true;
  }

  // checkSignInToWhatsAppByLinkCode shows WhatsApp's own QR code, then
  // switches to a phone number and shows the 8-character code to type
  // on the phone instead of asking for a typed answer.
  function checkSignInToWhatsAppByLinkCode(): bool {
    service.services = [{ id: "telegram", name: "Telegram" }, { id: "whatsapp", name: "WhatsApp" }];
    service.requests = [];

    controller.run("account.add");
    Check.find(view, "serviceButton-whatsapp").clicked();
    if (controller.stage !== "waiting" || controller.serviceName !== "WhatsApp") {
      return Check.fail("not waiting for whatsapp: " + controller.stage);
    }

    const accountId = controller.accountId;
    service.event("auth.step", { accountId: accountId, kind: "qr", qr: "cG5n", hint: "Scan it" });
    if (controller.stage !== "qr" || !Check.find(view, "qrImage").visible) return Check.fail("WhatsApp QR code not shown");

    controller.usePhone();
    Check.find(view, "answerField").text = "+15551234567";
    view.submit();
    if (root.last().method !== "auth.submit" || root.last().params.step !== "phone") return Check.fail("phone number not sent");

    service.event("auth.step", { accountId: accountId, kind: "linkcode", hint: "Enter this code on your phone: ABCD-1234" });
    if (controller.stage !== "linkcode") return Check.fail("linkcode step not shown: " + controller.stage);
    if (Check.find(view, "continueButton").visible) return Check.fail("Continue shown for a step with nothing to type");
    if (Check.find(view, "answerField").visible) return Check.fail("a typed answer field shown for the linkcode step");
    if (Check.find(view, "stepHint").text.indexOf("ABCD-1234") < 0) return Check.fail("link code not shown in the hint");

    service.event("account.updated", { id: accountId, status: "connected" });
    if (controller.open) return Check.fail("setup still open after connecting");

    service.services = [];
    return true;
  }

  // checkCancelRemoves removes an account that never signed in.
  function checkCancelRemoves(): bool {
    controller.begin();
    controller.cancel();

    const sent = root.last();
    if (sent.method !== "accounts.remove" || sent.params.accountId !== "tg-1") return Check.fail("cancel sent " + JSON.stringify(sent));
    if (controller.open) return Check.fail("setup still open after cancel");
    return true;
  }

  // checkResumesPendingStep opens a new controller at a waiting step.
  function checkResumesPendingStep(): bool {
    service.pendingAuth = { accountId: "tg-2", kind: "code", hint: "Check your app" };
    const resumed = controllerComponent.createObject(null, { service: service });
    if (!resumed.open || resumed.stage !== "code" || resumed.accountId !== "tg-2") {
      return Check.fail("pending step not resumed: " + resumed.stage);
    }

    const before = service.requests.length;
    resumed.cancel();
    if (service.requests.length !== before) return Check.fail("cancelling a saved account's sign-in removed it");
    return true;
  }

  // checkRemovesAnAccount asks which account, removes the one chosen, and
  // closes once the helper reports it gone; cancelling removes nothing.
  function checkRemovesAnAccount(): bool {
    controller.run("account.remove");
    if (!controller.removing) return Check.fail("account.remove did not ask which account");

    let before = service.requests.length;
    controller.cancel();
    if (controller.removing || service.requests.length !== before) return Check.fail("cancelling removal removed something");

    controller.run("account.remove");
    controller.remove("tg-9");
    const sent = root.last();
    if (sent.method !== "accounts.remove" || sent.params.accountId !== "tg-9") return Check.fail("remove sent " + JSON.stringify(sent));

    service.event("account.removed", { accountId: "tg-9" });
    if (controller.removing) return Check.fail("removal still showing after the account was removed");
    return true;
  }

  // checkChoosesAServiceWhenThereAreSeveral shows a chooser when the
  // helper offers more than one service, and adds the account for
  // whichever is picked, naming it in setup's wording.
  function checkChoosesAServiceWhenThereAreSeveral(): bool {
    service.services = [{ id: "telegram", name: "Telegram" }, { id: "whatsapp", name: "WhatsApp" }];
    service.requests = [];

    controller.run("account.add");
    if (controller.stage !== "chooseService" || service.requests.length !== 0) {
      return Check.fail("account.add did not show a chooser: " + controller.stage);
    }

    const button = Check.find(view, "serviceButton-whatsapp");
    if (!button) return Check.fail("no chooser button for whatsapp");

    button.clicked();
    const sent = root.last();
    if (sent.method !== "accounts.add" || sent.params.service !== "whatsapp") return Check.fail("chooseService sent " + JSON.stringify(sent));
    if (controller.serviceName !== "WhatsApp" || controller.stage !== "waiting") return Check.fail("not waiting for whatsapp");

    controller.cancel();
    service.services = [];
    return true;
  }

  // beginKeyboardChecks opens the credentials step and checks the API id
  // field already has focus, with nothing clicked.
  function beginKeyboardChecks(): void {
    controller.run("account.addOwnKeys");
    root.waitForFocus(() => Check.find(view, "apiIdField").activeFocus, "the API id field",
      root.checkWaitingDefaultsToCancel);
  }

  // checkWaitingDefaultsToCancel adds an account with no chooser needed
  // and checks the waiting step, which has nothing to type or click,
  // still focuses Cancel: before this, nothing did, so Tab and Escape
  // both landed nowhere.
  function checkWaitingDefaultsToCancel(): void {
    controller.cancel();
    service.requests = [];
    controller.run("account.add");
    root.waitForFocus(() => Check.find(view, "cancelButton").activeFocus, "Cancel while waiting to connect",
      root.checkLinkcodeDefaultsToCancel);
  }

  // checkLinkcodeDefaultsToCancel shows WhatsApp's link-code step, which
  // is read rather than typed, and checks it focuses Cancel too.
  function checkLinkcodeDefaultsToCancel(): void {
    const accountId = controller.accountId;
    service.event("auth.step", { accountId: accountId, kind: "linkcode", hint: "Enter this code on your phone: ABCD-1234" });
    root.waitForFocus(() => Check.find(view, "cancelButton").activeFocus, "Cancel at the link-code step",
      root.afterLinkcodeDefault);
  }

  // afterLinkcodeDefault leaves this account setup and starts the service
  // chooser's keyboard navigation.
  function afterLinkcodeDefault(): void {
    controller.cancel();
    root.pollAttempts = 0;
    root.beginChooseServiceKeyboardNav();
  }

  // beginChooseServiceKeyboardNav opens the chooser with two services and
  // checks the first one has focus, with nothing clicked yet.
  function beginChooseServiceKeyboardNav(): void {
    service.services = [{ id: "telegram", name: "Telegram" }, { id: "whatsapp", name: "WhatsApp" }];
    service.requests = [];
    controller.run("account.add");

    root.waitForFocus(() => Check.find(view, "serviceButton-telegram").activeFocus, "the first service choice",
      root.chooseServiceStepDown1);
  }

  // chooseServiceStepDown1 presses Down and checks focus moved to the
  // second choice.
  function chooseServiceStepDown1(): void {
    t.keyClick(Qt.Key_Down);
    root.waitForFocus(() => Check.find(view, "serviceButton-whatsapp").activeFocus, "the second service choice after Down",
      root.chooseServiceStepDown2);
  }

  // chooseServiceStepDown2 presses Down again and checks focus wraps onto
  // Cancel past the last choice.
  function chooseServiceStepDown2(): void {
    t.keyClick(Qt.Key_Down);
    root.waitForFocus(() => Check.find(view, "cancelButton").activeFocus, "Cancel after wrapping past the last choice",
      root.chooseServiceStepUp);
  }

  // chooseServiceStepUp presses Up from Cancel and checks focus wraps back
  // onto the last choice.
  function chooseServiceStepUp(): void {
    t.keyClick(Qt.Key_Up);
    root.waitForFocus(() => Check.find(view, "serviceButton-whatsapp").activeFocus, "the last choice after wrapping Up from Cancel",
      root.chooseServiceAccept);
  }

  // chooseServiceAccept presses Enter on the highlighted choice and checks
  // it, not the service the test started with, is the one added.
  function chooseServiceAccept(): void {
    t.keyClick(Qt.Key_Return);
    const sent = root.last();
    if (sent.method !== "accounts.add" || sent.params.service !== "whatsapp")
      return Check.fail("Enter on the highlighted choice sent " + JSON.stringify(sent));
    if (controller.serviceName !== "WhatsApp" || controller.stage !== "waiting")
      return Check.fail("Enter did not choose WhatsApp: " + controller.stage);

    controller.cancel();
    service.services = [];
    root.pollAttempts = 0;
    root.beginRemoveAccountKeyboardNav();
  }

  // beginRemoveAccountKeyboardNav opens the removal question with two
  // accounts and checks Cancel, the safe choice, has focus by default.
  function beginRemoveAccountKeyboardNav(): void {
    service.accounts = [
      { id: "wa-1", service: "whatsapp", name: "Personal" },
      { id: "tg-1", service: "telegram", name: "Work" }
    ];
    service.requests = [];
    controller.run("account.remove");

    root.waitForFocus(() => Check.find(removeView, "cancelButton").activeFocus, "Cancel, the safe choice, by default",
      root.removeStepDownToFirst);
  }

  // removeStepDownToFirst presses Down from Cancel and checks focus
  // landed on the first account.
  function removeStepDownToFirst(): void {
    t.keyClick(Qt.Key_Down);
    root.waitForFocus(() => Check.find(removeView, "removeButton-wa-1").activeFocus, "the first account after Down from Cancel",
      root.removeStepDownToLast);
  }

  // removeStepDownToLast presses Down again and checks focus moved to the
  // second account.
  function removeStepDownToLast(): void {
    t.keyClick(Qt.Key_Down);
    root.waitForFocus(() => Check.find(removeView, "removeButton-tg-1").activeFocus, "the second account after Down",
      root.removeStepWrap);
  }

  // removeStepWrap presses Down past the last account and checks focus
  // wraps onto Cancel.
  function removeStepWrap(): void {
    t.keyClick(Qt.Key_Down);
    root.waitForFocus(() => Check.find(removeView, "cancelButton").activeFocus, "Cancel after wrapping past the last account",
      root.removeStepUp);
  }

  // removeStepUp presses Up from Cancel and checks focus wraps back onto
  // the last account, the destructive choice, which must stay reachable.
  function removeStepUp(): void {
    t.keyClick(Qt.Key_Up);
    root.waitForFocus(() => Check.find(removeView, "removeButton-tg-1").activeFocus, "the last account after wrapping Up from Cancel",
      root.removeStepAccept);
  }

  // removeStepAccept presses Enter on the highlighted account and checks
  // that one, not the other, is removed.
  function removeStepAccept(): void {
    t.keyClick(Qt.Key_Return);
    const sent = root.last();
    if (sent.method !== "accounts.remove" || sent.params.accountId !== "tg-1")
      return Check.fail("Enter on the highlighted account sent " + JSON.stringify(sent));

    service.event("account.removed", { accountId: "tg-1" });
    if (controller.removing) return Check.fail("removal still showing after the account was removed");

    root.pollAttempts = 0;
    root.beginRemoveAccountDefaultIsSafe();
  }

  // beginRemoveAccountDefaultIsSafe reopens the question and waits for
  // Cancel's default focus again before pressing Enter with no
  // navigation at all.
  function beginRemoveAccountDefaultIsSafe(): void {
    service.requests = [];
    controller.run("account.remove");

    root.waitForFocus(() => Check.find(removeView, "cancelButton").activeFocus, "Cancel, the safe default, on reopening",
      root.removeAccountDefaultAccept);
  }

  // removeAccountDefaultAccept presses Enter straight away and checks it
  // chose Cancel, not a destructive removal, since nothing was navigated.
  function removeAccountDefaultAccept(): void {
    t.keyClick(Qt.Key_Return);
    if (service.requests.length !== 0) return Check.fail("Enter with no navigation removed an account instead of cancelling");
    if (controller.removing) return Check.fail("Enter on Cancel did not close the removal question");

    console.log("PASS AccountSetup");
    Qt.exit(0);
  }
}
