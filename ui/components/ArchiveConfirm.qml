import QtQuick

// Asks whether to archive one conversation, since a stray key once
// archived a pinned chat with no warning at all, leaving the list
// showing it both "Pinned" and "Archived" (see docs/decisions.md): the
// same y/n idiom ArchiveAllConfirm's own bulk question uses. Unarchiving
// never asks, since it only ever reverses something already confirmed.
ChoiceDialog {
  id: root

  // conversationTitle names the conversation this would archive; title
  // is already ChoiceDialog's own property for the question itself.
  property string conversationTitle: ""

  // confirmed archives the conversation the question named.
  signal confirmed()
  // cancelled leaves it as it is.
  signal cancelled()

  objectName: "archiveOneConfirm"
  titleObjectName: "archiveOneQuestion"
  title: "Archive " + root.conversationTitle + "?"
  choices: [
    { objectName: "cancelButton", text: "n  Cancel", selected: false, value: "cancel" },
    { objectName: "archiveButton", text: "y  Archive", selected: true, value: "confirm" }
  ]

  onPicked: value => {
    if (value === "confirm") root.confirmed();
    else root.cancelled();
  }
}
