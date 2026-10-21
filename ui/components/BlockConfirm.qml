import QtQuick

// Asks whether to block a contact, or report one as spam (which also
// blocks them and hides the chat): the same y/n idiom
// ArchiveAllConfirm's own question uses, since Cancel is the only other
// choice. kind picks the wording; the action itself is always the
// destructive one here, since unblocking needs no confirmation (see
// ListController._unblockCurrent).
ChoiceDialog {
  id: root

  // kind is "block" or "spam", deciding the title and description shown.
  property string kind: "block"

  // confirmed blocks the contact, or reports it as spam, depending on
  // kind.
  signal confirmed()
  // cancelled leaves the contact and conversation as they are.
  signal cancelled()

  objectName: "blockConfirm"
  titleObjectName: "blockQuestion"
  title: root.kind === "spam" ? "Report this contact as spam?" : "Block this contact?"
  description: root.kind === "spam"
    ? "They will be blocked and the chat removed from your list."
    : "You will no longer receive messages from them."
  choices: [
    { objectName: "cancelButton", text: "n  Cancel", selected: false, value: "cancel" },
    { objectName: "confirmButton", text: root.kind === "spam" ? "y  Report" : "y  Block", selected: true, value: "confirm" }
  ]

  onPicked: value => {
    if (value === "confirm") root.confirmed();
    else root.cancelled();
  }
}
