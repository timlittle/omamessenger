import QtQuick

// Asks whether to archive one conversation before doing it (see docs/decisions.md).
ChoiceDialog {
  id: root

  // conversationTitle names the conversation this would archive.
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
