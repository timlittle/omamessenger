// Checks account setup end to end against a scripted service: adding
// sends accounts.add at once with OmaMessenger's keys, your own API id and
// hash are checked before they are sent, each auth.step shows
// its stage, answers go out as auth.submit, connecting closes setup,
// cancelling a half-added account removes it, and a step that arrived
// while the panel was gone is shown when it is created again, where
// cancelling keeps the saved account.
//
// The rest drives the service chooser, the QR/phone switch and the
// removal question with real key events only — no Tab, no arrows, no
// mouse — through the same routeKey dispatch Panel.qml uses: adding a
// Telegram account by pressing t, switching to the phone step and back
// out with Escape; adding WhatsApp with w then its own QR/phone switch
// down to the link-code step; removing the second of two accounts with
// j then y, and cancelling a removal with n; and that typing letters
// into a field (even ones that are mnemonics elsewhere) only ever types,
// never fires a shortcut. No helper runs, so nothing reaches Telegram.
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

  // last returns the most recent request the service received.
  function last(): var {
    return service.requests[service.requests.length - 1] || {};
  }

  // waitForFocus holds until condition is true, then runs next; after
  // five seconds it fails, naming what should have had focus by then.
  function waitForFocus(condition: var, reason: string, next: var): void {
    if (condition()) return next();

    stepper.attempts++;
    if (stepper.attempts >= 100) return Check.fail(reason + " never got keyboard focus");
    stepper.retry(() => root.waitForFocus(condition, reason, next));
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

  // routeKey is the same dispatch Panel.routeKey does: match the active
  // context, then hand the action to whichever controller owns it. Only
  // AccountController is wired up, since that is everything these two
  // dialogs ever reach.
  function routeKey(key: int, modifiers: int, text: string): bool {
    const context = Navigation.keyContext({
      setupOpen: controller.open || controller.removing,
      setupContext: controller.navContext,
      pane: "list"
    });
    const action = Keymap.match(context, key, modifiers, text);
    if (!action || !controller.handles(action)) return false;

    return controller.run(action) !== false;
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
      chooseIndex: controller.chooseIndex
      qr: controller.qr
      hint: controller.hint
      error: controller.lastError
      busy: controller.busy
      routeKey: root.routeKey

      onServiceChosen: serviceId => controller.chooseService(serviceId)
      onCredentialsSubmitted: (apiId, apiHash) => controller.submitCredentials(apiId, apiHash)
      onPhoneRequested: controller.usePhone()
      onQrRequested: controller.useQr()
      onAnswered: value => controller.answer(value)
      onCancelled: controller.cancel()
    }

    RemoveAccount {
      id: removeView
      open: controller.removing
      accounts: service.accounts
      removeIndex: controller.removeIndex
      error: controller.lastError
      routeKey: root.routeKey

      onChosen: accountId => controller.remove(accountId)
      onCancelled: controller.cancel()
    }
  }

  TestCase {
    id: t
    when: false
  }

  Stepper {
    id: stepper
    name: "AccountSetup"
    intervalMs: 20
    deadlineMs: 20000
    startDelayMs: 0
    onTimeout: () => Check.fail("timed out before the checks finished")
    startFn: root.run
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

    stepper.attempts = 0;
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

  // anyFocused reports whether the setup Item itself or one of its text
  // fields currently holds real keyboard focus: whichever it is, a key
  // event reaches routeKey, so a wait only needs to know focus landed
  // somewhere in this step, not on a specific control.
  function anyFocused(): bool {
    return view.activeFocus || Check.find(view, "apiIdField").activeFocus
      || Check.find(view, "answerField").activeFocus;
  }

  // beginKeyboardChecks opens the service chooser with two services and
  // waits for the setup Item itself to hold focus, with nothing clicked.
  function beginKeyboardChecks(): void {
    service.services = [{ id: "telegram", name: "Telegram" }, { id: "whatsapp", name: "WhatsApp" }];
    service.requests = [];
    controller.run("account.add");

    root.waitForFocus(() => view.activeFocus, "the service chooser", root.checkChooseServiceStartsOnCancel);
  }

  // checkChooseServiceStartsOnCancel checks the chooser starts with
  // nothing picked (Cancel, the safe default) highlighted.
  function checkChooseServiceStartsOnCancel(): void {
    if (controller.chooseIndex !== -1) return Check.fail("the chooser did not start on Cancel");

    root.checkChooseServiceSelectionFollowsJK();
  }

  // checkChooseServiceSelectionFollowsJK presses j then k and checks the
  // visible selection moves with it: a model-driven highlight, not just
  // an invisible Qt focus ring.
  function checkChooseServiceSelectionFollowsJK(): void {
    t.keyClick("j");
    if (controller.chooseIndex !== 0 || !Check.find(view, "serviceButton-telegram").selected)
      return Check.fail("j did not highlight the first service");

    t.keyClick("k");
    if (controller.chooseIndex !== -1) return Check.fail("k did not move the highlight back to Cancel");

    root.checkAddTelegramByLetter();
  }

  // checkAddTelegramByLetter presses t to add Telegram straight away,
  // without navigating to it first, then shows its QR code.
  function checkAddTelegramByLetter(): void {
    t.keyClick("t");
    if (controller.stage !== "waiting" || controller.serviceName !== "Telegram")
      return Check.fail("t did not add Telegram: " + controller.stage);

    service.event("auth.step", { accountId: controller.accountId, kind: "qr", qr: "cG5n", hint: "Scan it" });
    root.waitForFocus(() => controller.stage === "qr" && root.anyFocused(), "the QR step",
      root.checkPhoneStepByLetterP);
  }

  // checkPhoneStepByLetterP presses p to switch to the phone step once
  // the QR code is showing.
  function checkPhoneStepByLetterP(): void {
    t.keyClick("p");
    root.waitForFocus(() => controller.stage === "phone" && root.anyFocused(), "the phone step",
      root.checkEscapeBacksOutToQr);
  }

  // checkEscapeBacksOutToQr presses Escape at the phone step: it must
  // back out to the QR code it came from, not cancel the whole sign-in.
  function checkEscapeBacksOutToQr(): void {
    t.keyClick(Qt.Key_Escape);
    if (controller.stage !== "qr") return Check.fail("Escape at the phone step did not back out to the QR code");
    if (!controller.open) return Check.fail("Escape at the phone step cancelled setup instead of backing out");

    controller.cancel();
    service.services = [];
    stepper.attempts = 0;
    root.beginAddWhatsAppByLetter();
  }

  // beginAddWhatsAppByLetter reopens the chooser with two services again
  // and waits for it to take focus.
  function beginAddWhatsAppByLetter(): void {
    service.services = [{ id: "telegram", name: "Telegram" }, { id: "whatsapp", name: "WhatsApp" }];
    service.requests = [];
    controller.run("account.add");

    root.waitForFocus(() => view.activeFocus, "the service chooser", root.checkAddWhatsAppByLetterW);
  }

  // checkAddWhatsAppByLetterW presses w to add WhatsApp straight away,
  // then shows its QR code.
  function checkAddWhatsAppByLetterW(): void {
    t.keyClick("w");
    if (controller.stage !== "waiting" || controller.serviceName !== "WhatsApp")
      return Check.fail("w did not add WhatsApp: " + controller.stage);

    service.event("auth.step", { accountId: controller.accountId, kind: "qr", qr: "cG5n", hint: "Scan it" });
    root.waitForFocus(() => controller.stage === "qr" && root.anyFocused(), "WhatsApp's QR step",
      root.checkWhatsAppPhoneByP);
  }

  // checkWhatsAppPhoneByP presses p to switch to the phone step, then
  // waits for the phone number field itself to take focus so the typed
  // number below lands in it.
  function checkWhatsAppPhoneByP(): void {
    t.keyClick("p");
    root.waitForFocus(() => Check.find(view, "answerField").activeFocus, "the phone number field",
      root.checkWhatsAppTypesPhoneNumber);
  }

  // checkWhatsAppTypesPhoneNumber types a phone number and submits it
  // with Enter, then checks WhatsApp's link-code step shows.
  function checkWhatsAppTypesPhoneNumber(): void {
    for (const ch of "+15551234567") t.keyClick(ch);
    t.keyClick(Qt.Key_Return);

    const sent = root.last();
    if (sent.method !== "auth.submit" || sent.params.step !== "phone") return Check.fail("the phone number was not sent");

    service.event("auth.step", { accountId: controller.accountId, kind: "linkcode", hint: "Enter this code on your phone: ABCD-1234" });
    if (controller.stage !== "linkcode") return Check.fail("the link-code step did not show: " + controller.stage);

    controller.cancel();
    service.services = [];
    stepper.attempts = 0;
    root.beginRemoveSecondOfTwo();
  }

  // beginRemoveSecondOfTwo opens the removal question with two accounts
  // and waits for it to take focus, with nothing picked yet.
  function beginRemoveSecondOfTwo(): void {
    service.accounts = [
      { id: "wa-1", service: "whatsapp", name: "Personal" },
      { id: "tg-1", service: "telegram", name: "Work" }
    ];
    service.requests = [];
    controller.run("account.remove");

    root.waitForFocus(() => removeView.activeFocus, "the removal question", root.checkRemoveStartsOnCancel);
  }

  // checkRemoveStartsOnCancel checks nothing is highlighted but the safe
  // default, Cancel.
  function checkRemoveStartsOnCancel(): void {
    if (controller.removeIndex !== -1 || !Check.find(removeView, "cancelButton").selected)
      return Check.fail("the removal question did not start on Cancel");

    root.checkRemoveSecondWithJThenY();
  }

  // checkRemoveSecondWithJThenY presses j twice to highlight the second
  // of the two accounts, checking the visible selection follows each
  // press, then y to remove it.
  function checkRemoveSecondWithJThenY(): void {
    t.keyClick("j");
    if (controller.removeIndex !== 0 || !Check.find(removeView, "removeButton-wa-1").selected)
      return Check.fail("j did not highlight the first account");

    t.keyClick("j");
    if (controller.removeIndex !== 1 || !Check.find(removeView, "removeButton-tg-1").selected)
      return Check.fail("a second j did not highlight the second account");

    t.keyClick("y");
    const sent = root.last();
    if (sent.method !== "accounts.remove" || sent.params.accountId !== "tg-1")
      return Check.fail("y on the highlighted account sent " + JSON.stringify(sent));

    service.event("account.removed", { accountId: "tg-1" });
    if (controller.removing) return Check.fail("removal still showing after the account was removed");

    stepper.attempts = 0;
    root.beginCancelRemovalWithN();
  }

  // beginCancelRemovalWithN reopens the removal question and waits for
  // it to take focus again.
  function beginCancelRemovalWithN(): void {
    service.requests = [];
    controller.run("account.remove");

    root.waitForFocus(() => removeView.activeFocus, "the removal question", root.checkCancelRemovalWithN);
  }

  // checkCancelRemovalWithN highlights an account with j, then cancels
  // with n: nothing should be removed, and the question should close.
  function checkCancelRemovalWithN(): void {
    t.keyClick("j");
    if (controller.removeIndex !== 0) return Check.fail("j did not highlight an account");

    const before = service.requests.length;
    t.keyClick("n");
    if (service.requests.length !== before) return Check.fail("n removed an account instead of cancelling");
    if (controller.removing) return Check.fail("n did not close the removal question");

    root.beginTypingLettersIntoFields();
  }

  // beginTypingLettersIntoFields opens the credentials step, whose
  // fields take arbitrary text (an API id and hash), and waits for the
  // first one to take focus.
  function beginTypingLettersIntoFields(): void {
    controller.run("account.addOwnKeys");
    root.waitForFocus(() => Check.find(view, "apiIdField").activeFocus, "the API id field",
      root.checkLettersTypeIntoFields);
  }

  // checkLettersTypeIntoFields types letters and digits that are
  // shortcuts elsewhere (t, w, p, q, y, n, j, k and digits) into both
  // credentials fields and checks every one landed as plain text, never
  // firing a shortcut or changing the step.
  function checkLettersTypeIntoFields(): void {
    const idField = Check.find(view, "apiIdField");
    idField.text = ""; // an earlier check already left "123" typed in here
    for (const ch of "tpqwyn") t.keyClick(ch);
    if (idField.text !== "tpqwyn") return Check.fail("the API id field got " + JSON.stringify(idField.text));

    t.keyClick(Qt.Key_Return);
    const hashField = Check.find(view, "apiHashField");
    hashField.text = "";
    for (const ch of "jk19") t.keyClick(ch);
    if (hashField.text !== "jk19") return Check.fail("the API hash field got " + JSON.stringify(hashField.text));
    if (controller.stage !== "credentials") return Check.fail("typing changed the step to " + controller.stage);

    console.log("PASS AccountSetup");
    Qt.exit(0);
  }
}
