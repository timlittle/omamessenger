pragma ComponentBehavior: Bound
import QtQuick
import "../lib/Rail.js" as Rail

// Asks which account to remove. Removing signs it out and deletes its
// chats from this computer, so the safe choice, Cancel, is highlighted
// first; j/k, Up/Down or a number highlight an account, Enter or y
// removes whichever is highlighted, n or Escape cancels.
ChoiceDialog {
  id: root

  // accounts are the accounts that can be removed.
  property var accounts: []
  // removeIndex is the highlighted account: -1 for Cancel, the safe
  // default.
  property int removeIndex: -1
  // error is the last failure, shown under the choices.
  property string error: ""
  // knownServices: the helper's own services, from hello, for each
  // account's label; falls back to the built-in labels when empty.
  property var knownServices: []

  // chosen reports the account to remove.
  signal chosen(string accountId)
  // cancelled closes the question without removing anything.
  signal cancelled()

  objectName: "removeAccount"
  title: "Remove an account?"
  description: root.accounts.length > 0
    ? "This signs the account out and deletes its chats from this computer. They stay on your phone. j/k or a number highlight one, Enter or y removes it."
    : "There are no accounts to remove."
  errorText: root.error
  layout: "list"
  choices: root.accounts.map((account, index) => ({
    objectName: "removeButton-" + account.id,
    text: (index < 9 ? (index + 1) + "  " : "") + "Remove " + Rail.accountDescription(account, root.knownServices),
    selected: index === root.removeIndex,
    value: account.id
  }))
  trailingChoices: [
    { objectName: "cancelButton", text: "n  Cancel", selected: root.removeIndex === -1, value: "cancel" }
  ]

  onPicked: value => {
    if (value === "cancel") root.cancelled();
    else root.chosen(value);
  }
}
