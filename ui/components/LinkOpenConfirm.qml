import QtQuick
import "../lib/Links.js" as Links

// Asks before opening a link whose own visible text does not read as
// the same destination as where it actually goes (see
// ui/lib/Links.js's needsConfirmation): a sender's own formatting can
// show one piece of text while linking somewhere else entirely, so
// Cancel, not Open, is the Enter-reachable default - the one choice
// that keeps a stray Enter from ever following a link nobody meant to
// read as safe.
//
// A security review found a link Links.isAllowed refuses outright -
// one with a backslash, another hidden character, or an http(s) URL
// with no real authority (see Format.spanURL) - could still reach
// this dialog needing confirmation, and urlHost's own authority split
// (never handling a backslash the way a browser's own parser does)
// could then show a convincing but wrong host: "Really opens:
// paypal.com" while a browser would actually open evil.example. Such
// a target shows this dialog's own refused state instead: no host
// line computed from it, the fixed message in place of "It will
// open:", and no Open or Copy choice left for a stray keypress to
// land on - just a way to close it.
ChoiceDialog {
  id: root

  // target is the link's own, real destination, shown in full so the
  // question answers exactly what a hover or a status line would.
  property string target: ""

  // _allowed is false for a target Links.isAllowed refuses outright,
  // which this dialog must never compute a host for or offer to open.
  readonly property bool _allowed: Links.isAllowed(root.target)

  // _host is target's own authority host (see Links.urlHost), or ""
  // for a link with no separate one to show (a mailto:, say) or one
  // this dialog refuses outright.
  readonly property string _host: root._allowed ? Links.urlHost(root.target) : ""

  // confirmedOpen opens target anyway.
  signal confirmedOpen()
  // copied puts target on the clipboard instead of opening it.
  signal copied()
  // cancelled leaves target unopened.
  signal cancelled()

  objectName: "linkOpenConfirm"
  titleObjectName: "linkOpenQuestion"
  title: root._allowed ? "This link's text does not match where it goes" : "This link was not opened"
  // The one line this question must never let a long userinfo pad out
  // of view: a security review found the description below, elided
  // from the right, could lose the real host entirely behind text
  // engineered to read as a different, convincing domain (see
  // docs/decisions.md and ChoiceDialog.emphasisText). Hidden for a
  // link with no separate authority host at all, rather than naming
  // an empty one - the same as for a refused link, which has no host
  // this dialog will compute at all.
  emphasisText: root._host !== "" ? "Really opens: " + root._host : ""
  description: root._allowed ? "It will open:\n" + root.target : Links.REFUSED_LINK_MESSAGE
  // Elides the middle, not the end, so both the scheme and whatever
  // sits nearest the real host stay in view here too; emphasisText
  // above is still the one place that host is guaranteed in full.
  descriptionElide: Text.ElideMiddle
  choices: root._allowed ? [
    { objectName: "cancelButton", text: "n  Cancel", selected: true, value: "cancel" },
    { objectName: "copyButton", text: "c  Copy link", selected: false, value: "copy" },
    { objectName: "openButton", text: "y  Open anyway", selected: false, value: "open" }
  ] : [
    { objectName: "cancelButton", text: "n  Close", selected: true, value: "cancel" }
  ]

  onPicked: value => {
    if (value === "copy") root.copied();
    else if (value === "open") root.confirmedOpen();
    else root.cancelled();
  }
}
