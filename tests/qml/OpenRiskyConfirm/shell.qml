// Checks OpenRiskyConfirm names the file it is actually about to open:
// a security review found the dialog's text was fixed, so a download
// that finished for a different file while the question was open could
// swap what Open anyway opens with nothing on screen to show it had
// changed (see MediaController._askBeforeOpening, fixed separately).
// This only checks the dialog's own rendering: the name shows in full
// for a short one, as plain text (never rich text, since it is
// sender-controlled), and a very long one elides rather than growing
// the card or overflowing it.
import QtQuick
import Quickshell
import qs.Commons
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  FloatingWindow {
    id: win
    implicitWidth: Style.space(500)
    implicitHeight: Style.space(400)
    visible: true

    OpenRiskyConfirm {
      id: dialog
      open: true
      fileName: "invoice.html"
    }
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.checkFileNameShows()
  }

  // checkFileNameShows confirms the description names the exact file
  // the dialog is about, as plain text.
  function checkFileNameShows(): void {
    const description = Check.find(dialog, "dialogDescription");
    if (!description) return Check.fail("no dialogDescription line found");
    if (description.textFormat !== Text.PlainText)
      return Check.fail("the file name is not shown as plain text: textFormat = " + description.textFormat);
    if (description.text.indexOf("invoice.html") < 0)
      return Check.fail("the description does not name the file: " + JSON.stringify(description.text));

    root.checkLongNameElides();
  }

  // checkLongNameElides confirms a very long file name elides instead
  // of growing the card or overflowing it, the same as a long URL does
  // in LinkOpenConfirm.
  function checkLongNameElides(): void {
    dialog.fileName = "a".repeat(2000) + ".html";

    const description = Check.find(dialog, "dialogDescription");
    if (!description) return Check.fail("no dialogDescription line found");
    if (description.elide !== Text.ElideMiddle)
      return Check.fail("a long file name does not elide: elide = " + description.elide);

    console.log("PASS OpenRiskyConfirm");
    Qt.exit(0);
  }
}
