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

    root.beginWindowInactiveCheck();
  }

  // beginWindowInactiveCheck proves the fix for a follow-up security
  // review: the search field used to stay alive for as long as the
  // dialog was open, even with the window itself not the active one
  // (another window focused, or alt-tabbed away) - the exact "merely
  // visible" gap already closed for every other field in this UI (see
  // docs/decisions.md). query is set directly here, as the fixed input
  // this component takes from its caller (DialogController, in the
  // real app) - that round trip is DialogController's own job, proven
  // in tests/qml/Controllers, not this component's - then turning the
  // window inactive must destroy the field, and turning it active
  // again must bring it back still seeded from query, with real
  // keyboard focus restored.
  function beginWindowInactiveCheck(): void {
    dialog.query = "looking for someone";
    const field = Check.find(dialog, "newChatSearchField");
    if (!field) return Check.fail("the search field does not exist to begin the window-inactive check");
    if (field.text !== "looking for someone") return Check.fail("the search field did not seed from query: " + JSON.stringify(field.text));
    field.forceActiveFocus();

    dialog.windowActive = false;
    if (Check.find(dialog, "newChatSearchField")) return Check.fail("the search field still exists with the window inactive");

    dialog.windowActive = true;
    // dialog's own onWindowActiveChanged restores focus through
    // Qt.callLater, exactly as the production code does, so the
    // deferred check below runs after that same callback has had its
    // turn.
    Qt.callLater(root.checkSearchFieldRestoredWithTextAndFocus);
  }

  // checkSearchFieldRestoredWithTextAndFocus confirms the search field
  // came back still seeded from query and with real keyboard focus.
  function checkSearchFieldRestoredWithTextAndFocus(): void {
    const field = Check.find(dialog, "newChatSearchField");
    if (!field) return Check.fail("the search field did not come back once the window was active again");
    if (field.text !== "looking for someone") return Check.fail("the search field's text did not survive: " + JSON.stringify(field.text));
    if (!field.activeFocus) return Check.fail("the search field did not get focus back once the window was active again");

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
