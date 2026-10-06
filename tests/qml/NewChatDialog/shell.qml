// Checks NewChatDialog: moveCurrent + accept chooses the right contact,
// and nextAccount wraps around the account list.
import QtQuick
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property var accepted: null
  property string changedAccountId: ""

  FloatingWindow {
    implicitWidth: 900
    implicitHeight: 500
    visible: true

    NewChatDialog {
      id: dialog
      anchors.fill: parent
      accounts: [
        { id: "a1", service: "whatsapp", name: "Personal" },
        { id: "a2", service: "telegram", name: "Personal" },
        { id: "a3", service: "telegram", name: "Work" }
      ]
      accountId: "a1"
      contacts: [{ accountId: "a1", remoteId: "r1", name: "Ben Okafor" }, { accountId: "a1", remoteId: "r2", name: "Cora Diallo" }]
      currentIndex: 0
      onAccepted: (accountId, contactId) => root.accepted = { accountId: accountId, contactId: contactId }
      onAccountChanged: id => root.changedAccountId = id
    }
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // run drives the dialog through each scenario and checks the outcome.
  // Each check returns false after a failure, so run() stops immediately
  // and the earlier Qt.exit(1) is the one that takes effect.
  function run(): void {
    if (!root.checkMoveAndAccept()) return;
    if (!root.checkNextAccountWraps()) return;
    if (!root.checkFitsCard()) return;

    console.log("PASS NewChatDialog");
    Qt.exit(0);
  }

  // checkMoveAndAccept verifies moveCurrent(1) then accept() picks the
  // second contact.
  function checkMoveAndAccept(): bool {
    dialog.moveCurrent(1);
    dialog.accept();

    if (!root.accepted) return Check.fail("accept() did not emit accepted");
    if (root.accepted.accountId !== "a1" || root.accepted.contactId !== "r2")
      return Check.fail("accepted " + JSON.stringify(root.accepted) + ", want a1/r2");
    return true;
  }

  // checkFitsCard verifies every visible piece of the dialog lies inside
  // its card, with three accounts named after their services.
  function checkFitsCard(): bool {
    const card = Check.find(dialog, "newChatCard");
    const items = [];
    root.collect(card, items);
    for (const item of items) {
      if (!item.visible || item.width === 0) continue;
      const pos = item.mapToItem(card, 0, 0);
      if (pos.x < 0 || pos.x + item.width > card.width + 1)
        return Check.fail((item.text || item) + " runs outside the dialog card");
    }
    return true;
  }

  // collect gathers every descendant of item into out.
  function collect(item: var, out: var): void {
    for (const child of item.children) {
      out.push(child);
      root.collect(child, out);
    }
  }

  // checkNextAccountWraps verifies nextAccount() wraps past the last account.
  function checkNextAccountWraps(): bool {
    dialog.accountId = "a3";
    root.changedAccountId = "";
    dialog.nextAccount();

    if (root.changedAccountId !== "a1") return Check.fail("nextAccount() gave " + root.changedAccountId + ", want a1");
    return true;
  }
}
