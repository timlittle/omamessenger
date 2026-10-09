// Checks DoctorReport: it is hidden until open, shows one line per check
// with its name and detail text, a passing check and a failing one are
// marked with different glyphs (never colour alone), and the Close
// button reports closed().
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property int closedCount: 0

  property var checks: [
    { name: "Database", ok: true, detail: "open and up to date" },
    { name: "Desktop notifications", ok: false, detail: "notification service unreachable" }
  ]

  FloatingWindow {
    id: win
    implicitWidth: 500
    implicitHeight: 400
    visible: true

    DoctorReport {
      id: hidden
      open: false
      checks: root.checks
    }

    DoctorReport {
      id: report
      anchors.fill: parent
      open: true
      checks: root.checks
      onClosed: root.closedCount++
    }
  }

  TestCase {
    id: t
    when: false
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  function run(): void {
    if (hidden.visible)
      return Check.fail("a closed report is still visible");

    const texts = Check.texts(report).map((item) => item.text);
    if (!texts.some((text) => text.includes("Database") && text.includes("open and up to date")))
      return Check.fail("passing check not shown with its name and detail: " + JSON.stringify(texts));
    if (!texts.some((text) => text.includes("Desktop notifications") && text.includes("notification service unreachable")))
      return Check.fail("failing check not shown with its name and detail: " + JSON.stringify(texts));

    const glyphs = [];
    const found = [];
    (function collect(item) {
      for (const child of item.children) {
        if (child.objectName === "doctorCheckGlyph") glyphs.push(child.text);
        collect(child);
      }
    })(report);
    if (glyphs.length !== 2 || glyphs[0] === glyphs[1])
      return Check.fail("the passing and failing checks use the same glyph, colour would be the only cue: " + JSON.stringify(glyphs));

    const closeButton = Check.find(report, "doctorCloseButton");
    if (!closeButton) return Check.fail("doctorCloseButton not found");

    t.mouseClick(closeButton, closeButton.width / 2, closeButton.height / 2);
    if (root.closedCount !== 1)
      return Check.fail("Close did not report closed(): closedCount = " + root.closedCount);

    console.log("PASS DoctorReport");
    Qt.exit(0);
  }
}
