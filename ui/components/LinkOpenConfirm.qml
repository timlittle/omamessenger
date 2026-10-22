import QtQuick

// Asks before opening a link whose own visible text does not read as
// the same destination as where it actually goes (see
// ui/lib/Links.js's needsConfirmation): a sender's own formatting can
// show one piece of text while linking somewhere else entirely, so
// Cancel, not Open, is the Enter-reachable default - the one choice
// that keeps a stray Enter from ever following a link nobody meant to
// read as safe.
ChoiceDialog {
  id: root

  // target is the link's own, real destination, shown in full so the
  // question answers exactly what a hover or a status line would.
  property string target: ""

  // confirmedOpen opens target anyway.
  signal confirmedOpen()
  // copied puts target on the clipboard instead of opening it.
  signal copied()
  // cancelled leaves target unopened.
  signal cancelled()

  objectName: "linkOpenConfirm"
  titleObjectName: "linkOpenQuestion"
  title: "This link's text does not match where it goes"
  description: "It will open:\n" + root.target
  choices: [
    { objectName: "cancelButton", text: "n  Cancel", selected: true, value: "cancel" },
    { objectName: "copyButton", text: "c  Copy link", selected: false, value: "copy" },
    { objectName: "openButton", text: "y  Open anyway", selected: false, value: "open" }
  ]

  onPicked: value => {
    if (value === "copy") root.copied();
    else if (value === "open") root.confirmedOpen();
    else root.cancelled();
  }
}
