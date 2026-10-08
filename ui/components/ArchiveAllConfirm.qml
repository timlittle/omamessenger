import QtQuick

// Asks whether to archive every read, unpinned conversation in the
// current list, since it is a bulk action: Enter or y confirms, n or
// Esc cancels, the same y/n idiom RemoveAccount's own question uses.
ChoiceDialog {
  id: root

  // count is how many conversations this would archive, named in the
  // question so the user knows the scope before confirming.
  property int count: 0

  // confirmed archives the conversations the question named.
  signal confirmed()
  // cancelled leaves every conversation as it is.
  signal cancelled()

  objectName: "archiveAllConfirm"
  titleObjectName: "archiveAllQuestion"
  title: "Archive " + root.count + " read conversation" + (root.count === 1 ? "" : "s") + "?"
  description: "Pinned and unread conversations are left as they are."
  choices: [
    { objectName: "cancelButton", text: "n  Cancel", selected: false, value: "cancel" },
    { objectName: "archiveButton", text: "y  Archive", selected: true, value: "confirm" }
  ]

  onPicked: value => {
    if (value === "confirm") root.confirmed();
    else root.cancelled();
  }
}
