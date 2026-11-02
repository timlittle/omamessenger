// Checks the rename-account flow end to end against a scripted service:
// with only one account, "Rename account" goes straight to the alias
// field, prefilled with its current alias; with more than one, it asks
// which one first, the same list RemoveAccount uses, driven through the
// key router's own actions (rename.down/up/accept/cancel), which is all
// a key event ever reaches - AccountSetup's own test already proves the
// underlying ChoiceDialog answers real j/k/Enter/number key events for
// an identical chooser, so this checks the wiring those actions drive,
// not the key dispatch itself. Saving sends accounts.setAlias trimmed
// and closes the dialog; cancelling at either stage sends nothing; and
// the helper reporting the account removed while its rename dialog is
// open closes it too. No helper runs, so nothing reaches a real service.
import QtQuick
import Quickshell
import "ui/components"
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  // last returns the most recent request the service received.
  function last(): var {
    return service.requests[service.requests.length - 1] || {};
  }

  QtObject {
    id: service

    property string status: "ready"
    property var accounts: []
    property var services: []
    property var requests: []

    signal event(string name, var data)

    function request(method: string, params: var, callback: var): void {
      service.requests = service.requests.concat([{ method: method, params: params }]);
      const replies = { "accounts.setAlias": { id: params.accountId, alias: params.alias } };
      callback(null, replies[method] || {});
    }
  }

  AccountController {
    id: controller
    service: service
  }

  FloatingWindow {
    implicitWidth: 800
    implicitHeight: 600
    visible: true

    RenameAccount {
      id: view
      open: controller.renaming
      stage: controller.renameAccountId === "" ? "choose" : "alias"
      accounts: service.accounts
      chooseIndex: controller.renameIndex
      currentAlias: controller.renameAlias
      error: controller.lastError
      busy: controller.busy

      onChosen: accountId => controller.chooseRename(accountId)
      onSubmitted: alias => controller.submitAlias(alias)
      onCancelled: controller.cancel()
    }
  }

  Stepper {
    id: stepper
    name: "RenameAccount"
    intervalMs: 20
    deadlineMs: 10000
    startDelayMs: 0
    onTimeout: () => Check.fail("timed out before the checks finished")
    startFn: root.run
  }

  function run(): void {
    if (!root.checkDoesNothingWithNoAccounts()) return;
    if (!root.checkAliasFieldDestroyedAtTheChooseStep()) return;
    if (!root.checkOneAccountGoesStraightToTheAliasField()) return;
    if (!root.checkCancellingTheAliasFieldSendsNothing()) return;
    if (!root.checkChoosesThenSaves()) return;
    if (!root.checkCancellingTheChooserSendsNothing()) return;
    if (!root.checkRemovingTheAccountClosesItsRenameDialog()) return;

    root.beginWindowInactiveCheck();
  }

  // checkDoesNothingWithNoAccounts proves "Rename account" is a no-op
  // without an account to rename, rather than opening an empty dialog.
  function checkDoesNothingWithNoAccounts(): bool {
    controller.run("account.rename");
    if (controller.renaming) return Check.fail("account.rename opened with no accounts");
    return true;
  }

  // checkAliasFieldDestroyedAtTheChooseStep proves the alias field does
  // not exist at all - not merely hidden - while the chooser is
  // showing: the regression this guards against left it alive, merely
  // invisible, reading every foreign Wayland clipboard offer the whole
  // time the chooser was up (see docs/decisions.md).
  function checkAliasFieldDestroyedAtTheChooseStep(): bool {
    service.accounts = [
      { id: "wa-1", service: "whatsapp", name: "WhatsApp", alias: "Personal WhatsApp" },
      { id: "tg-1", service: "telegram", name: "Telegram", alias: "" }
    ];

    controller.run("account.rename");
    if (view.stage !== "choose") return Check.fail("expected the chooser, got " + view.stage);
    if (Check.find(view, "aliasField")) return Check.fail("the alias field exists at the choose step");

    controller.run("rename.cancel");
    service.accounts = [];
    return true;
  }

  // checkOneAccountGoesStraightToTheAliasField skips the chooser for
  // the one account there is, and prefills its current alias.
  function checkOneAccountGoesStraightToTheAliasField(): bool {
    service.accounts = [{ id: "wa-1", service: "whatsapp", name: "WhatsApp", alias: "Personal WhatsApp" }];

    controller.run("account.rename");
    if (!controller.renaming || controller.renameAccountId !== "wa-1") return Check.fail("did not go straight to wa-1's alias field");
    if (view.stage !== "alias" || controller.renameAlias !== "Personal WhatsApp") {
      return Check.fail("alias not prefilled: " + controller.renameAlias);
    }

    Check.find(view, "aliasField").text = "  Work WhatsApp  ";
    Check.find(view, "saveButton").clicked();
    const sent = root.last();
    if (sent.method !== "accounts.setAlias" || sent.params.accountId !== "wa-1" || sent.params.alias !== "Work WhatsApp") {
      return Check.fail("setAlias sent " + JSON.stringify(sent));
    }
    if (controller.renaming) return Check.fail("still renaming after saving");

    return true;
  }

  // checkCancellingTheAliasFieldSendsNothing proves Cancel at the alias
  // field closes the dialog without sending anything.
  function checkCancellingTheAliasFieldSendsNothing(): bool {
    service.requests = [];
    controller.run("account.rename");
    if (!controller.renaming) return Check.fail("did not reopen for wa-1");

    Check.find(view, "cancelButton").clicked();
    if (controller.renaming || service.requests.length !== 0) return Check.fail("cancel at the alias field sent something");

    return true;
  }

  // checkChoosesThenSaves drives the chooser through rename.down and
  // rename.accept, the actions a real j/Down and Enter key resolve to
  // (see Keymap.js), then saves an alias for the second of two accounts.
  function checkChoosesThenSaves(): bool {
    service.accounts = [
      { id: "wa-1", service: "whatsapp", name: "WhatsApp", alias: "Work WhatsApp" },
      { id: "tg-1", service: "telegram", name: "Telegram", alias: "" }
    ];
    service.requests = [];

    controller.run("account.rename");
    if (view.stage !== "choose" || controller.renameIndex !== -1) return Check.fail("the chooser did not start on Cancel");

    controller.run("rename.down");
    if (controller.renameIndex !== 0 || !Check.find(view, "renameButton-wa-1").selected) return Check.fail("rename.down did not highlight the first account");

    controller.run("rename.down");
    if (controller.renameIndex !== 1 || !Check.find(view, "renameButton-tg-1").selected) return Check.fail("a second rename.down did not highlight the second account");

    controller.run("rename.accept");
    if (view.stage !== "alias" || controller.renameAccountId !== "tg-1") return Check.fail("rename.accept did not move to tg-1's alias field");
    if (controller.renameAlias !== "") return Check.fail("tg-1's empty alias was not shown as empty");

    Check.find(view, "aliasField").text = "Family Telegram";
    Check.find(view, "saveButton").clicked();
    const sent = root.last();
    if (sent.method !== "accounts.setAlias" || sent.params.accountId !== "tg-1" || sent.params.alias !== "Family Telegram") {
      return Check.fail("setAlias sent " + JSON.stringify(sent));
    }

    return true;
  }

  // checkCancellingTheChooserSendsNothing proves rename.cancel (what a
  // real n key resolves to) cancels the chooser itself without sending
  // anything or moving to the alias field.
  function checkCancellingTheChooserSendsNothing(): bool {
    service.requests = [];
    controller.run("account.rename");
    if (view.stage !== "choose") return Check.fail("did not reopen the chooser");

    controller.run("rename.cancel");
    if (controller.renaming || service.requests.length !== 0) return Check.fail("rename.cancel sent something");

    return true;
  }

  // checkRemovingTheAccountClosesItsRenameDialog proves the helper
  // reporting the account removed while its rename dialog is open
  // closes that dialog too, rather than leaving it open for an account
  // that no longer exists.
  function checkRemovingTheAccountClosesItsRenameDialog(): bool {
    controller.run("account.rename");
    controller.run("rename.down");
    controller.run("rename.accept");
    if (controller.renameAccountId !== "wa-1") return Check.fail("did not reach wa-1's alias field");

    service.event("account.removed", { accountId: "wa-1" });
    if (controller.renaming) return Check.fail("rename dialog still open after its account was removed");

    return true;
  }

  // beginWindowInactiveCheck proves the fix for a follow-up security
  // review: the alias field used to stay alive for as long as the
  // alias step was showing, even with the window itself not the active
  // one (another window focused, or alt-tabbed away) - the exact
  // "merely visible" gap already closed for every other field in this
  // UI (see docs/decisions.md). Opens a fresh rename, picks the first
  // account the same way checkChoosesThenSaves does, types a
  // replacement alias, then proves turning the window inactive
  // destroys the field and turning it active again restores both the
  // typed text and real keyboard focus.
  function beginWindowInactiveCheck(): void {
    service.requests = [];
    controller.run("account.rename");
    if (view.stage !== "choose") return Check.fail("expected the chooser to start the window-inactive check");

    controller.run("rename.down");
    controller.run("rename.accept");
    if (view.stage !== "alias") return Check.fail("did not reach the alias field for the window-inactive check");

    Check.find(view, "aliasField").text = "Renamed while testing";

    view.windowActive = false;
    if (Check.find(view, "aliasField")) return Check.fail("the alias field still exists with the window inactive");

    view.windowActive = true;
    // view's own onWindowActiveChanged restores focus through
    // Qt.callLater, exactly as the production code does, so the
    // deferred check below runs after that same callback has had its
    // turn - the one thing nothing earlier in this file needed to wait
    // for, since nothing earlier asserted on real focus.
    Qt.callLater(root.checkAliasFieldRestoredWithTextAndFocus);
  }

  // checkAliasFieldRestoredWithTextAndFocus confirms the alias field
  // came back with exactly the text it held before the window went
  // inactive, and with real keyboard focus.
  function checkAliasFieldRestoredWithTextAndFocus(): void {
    const field = Check.find(view, "aliasField");
    if (!field) return Check.fail("the alias field did not come back once the window was active again");
    if (field.text !== "Renamed while testing") return Check.fail("the alias field's text did not survive: " + JSON.stringify(field.text));
    if (!field.activeFocus) return Check.fail("the alias field did not get focus back once the window was active again");

    stepper.pass();
  }
}
