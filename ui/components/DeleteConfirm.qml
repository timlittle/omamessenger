import QtQuick
import qs.Commons

// Asks what deleting the highlighted message should do: delete it for
// everyone (only offered for a message this account sent), remove it
// from this account's own view only, or leave it. h/l (or Left/Right)
// move a highlight across whichever choices apply, Enter chooses it, a
// mnemonic letter jumps straight to one, and Escape cancels. Cancel is
// always the default highlight, so a stray Enter never deletes
// anything. Every button stays clickable by mouse too.
ChoiceDialog {
  id: root

  // targetIsOwn offers "Delete for everyone" only for a message this
  // account sent.
  property bool targetIsOwn: false
  // highlightIndex is the caller's highlighted choice, into whichever
  // choices targetIsOwn leaves on offer, left to right.
  property int highlightIndex: 0

  // everyone deletes the message for everyone.
  signal everyone()
  // forMe removes the message from this account's own view only.
  signal forMe()
  // cancelled leaves the message as it is.
  signal cancelled()

  // _everyoneIndex and _forMeIndex are where those choices sit in
  // highlightIndex's order, matching DeleteController.choices: with
  // "everyone" on offer it comes first, otherwise "for me" does.
  readonly property int _everyoneIndex: 0
  readonly property int _forMeIndex: root.targetIsOwn ? 1 : 0
  readonly property int _cancelIndex: root.targetIsOwn ? 2 : 1

  objectName: "deleteConfirm"
  title: "Delete this message?"
  description: root.targetIsOwn ? "You can delete it for everyone, or just remove it from your own view." : ""
  minCardWidth: Style.space(380)
  choices: [
    { objectName: "everyoneButton", text: "e  Delete for everyone", visible: root.targetIsOwn, selected: root.highlightIndex === root._everyoneIndex, value: "everyone" },
    { objectName: "forMeButton", text: "m  Delete for me", selected: root.highlightIndex === root._forMeIndex, value: "forMe" },
    { objectName: "cancelButton", text: "n  Cancel", selected: root.highlightIndex === root._cancelIndex, value: "cancel" }
  ]

  onPicked: value => {
    if (value === "everyone") root.everyone();
    else if (value === "forMe") root.forMe();
    else root.cancelled();
  }
}
