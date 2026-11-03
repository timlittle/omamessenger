import QtQuick
import "../lib/Links.js" as Links

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

  // _host is target's own authority host (see Links.urlHost), or ""
  // for a link with no separate one to show (a mailto:, say).
  readonly property string _host: Links.urlHost(root.target)

  // confirmedOpen opens target anyway.
  signal confirmedOpen()
  // copied puts target on the clipboard instead of opening it.
  signal copied()
  // cancelled leaves target unopened.
  signal cancelled()

  objectName: "linkOpenConfirm"
  titleObjectName: "linkOpenQuestion"
  title: "This link's text does not match where it goes"
  // The one line this question must never let a long userinfo pad out
  // of view: a security review found the description below, elided
  // from the right, could lose the real host entirely behind text
  // engineered to read as a different, convincing domain (see
  // docs/decisions.md and ChoiceDialog.emphasisText). Hidden for a
  // link with no separate authority host at all, rather than naming
  // an empty one.
  emphasisText: root._host !== "" ? "Really opens: " + root._host : ""
  description: "It will open:\n" + root.target
  // Elides the middle, not the end, so both the scheme and whatever
  // sits nearest the real host stay in view here too; emphasisText
  // above is still the one place that host is guaranteed in full.
  descriptionElide: Text.ElideMiddle
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
