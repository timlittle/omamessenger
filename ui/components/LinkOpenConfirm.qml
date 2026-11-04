import QtQuick
import "../lib/Links.js" as Links

// Asks before opening a link whose visible text does not match where
// it actually goes. Cancel, not Open, is the Enter-reachable default.
// A link Links.isAllowed refuses outright shows a fixed refusal with
// no host and no Open/Copy choice, so a disguised or malformed URL
// can never compute a misleading host. An allowed host is shown
// through Links.hostForDisplay so a lookalike Unicode domain can't
// hide behind a normal-looking glyph.
ChoiceDialog {
  id: root

  // target is the link's own, real destination, shown in full so the
  // question answers exactly what a hover or a status line would.
  property string target: ""

  // _allowed is false when Links.isAllowed refuses target outright.
  readonly property bool _allowed: Links.isAllowed(root.target)

  // _host is target's authority host, or "" if there is none to show.
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
  // Guarantees the real host stays visible even if the description
  // below gets elided (see docs/decisions.md).
  emphasisText: root._host !== "" ? "Domain: " + Links.hostForDisplay(root._host) : ""
  description: root._allowed ? "It will open:\n" + root.target : Links.REFUSED_LINK_MESSAGE
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
