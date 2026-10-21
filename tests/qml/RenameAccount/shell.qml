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
    if (!root.checkOneAccountGoesStraightToTheAliasField()) return;
    if (!root.checkCancellingTheAliasFieldSendsNothing()) return;
    if (!root.checkChoosesThenSaves()) return;
    if (!root.checkCancellingTheChooserSendsNothing()) return;
    if (!root.checkRemovingTheAccountClosesItsRenameDialog()) return;

    stepper.pass();
  }

  // checkDoesNothingWithNoAccounts proves "Rename account" is a no-op
  // without an account to rename, rather than opening an empty dialog.
  function checkDoesNothingWithNoAccounts(): bool {
    controller.run("account.rename");
    if (controller.renaming) return Check.fail("account.rename opened with no accounts");
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
}
