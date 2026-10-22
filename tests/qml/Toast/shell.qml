// Checks Toast: hidden until it has text, shown with a checkmark glyph
// paired with the text (never colour alone), auto-dismissing itself once
// dismissDelayMs elapses, and dismissing on a click without waiting for
// that timer. The priority between the update hint and an ordinary
// notice, and the Escape chain's own gating on focus, are controller-level
// wiring proven in tests/qml/UpdateHint instead.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property int autoDismissCount: 0
  property int clickDismissCount: 0

  FloatingWindow {
    id: win
    implicitWidth: 500
    implicitHeight: 400
    visible: true

    Toast {
      id: hiddenToast
      text: ""
    }

    Toast {
      id: autoToast
      anchors.horizontalCenter: parent.horizontalCenter
      y: 40
      text: "Copied to clipboard"
      dismissDelayMs: 60
      onDismissed: root.autoDismissCount++
    }

    Toast {
      id: clickToast
      anchors.horizontalCenter: parent.horizontalCenter
      y: 120
      text: "Saved to ~/Downloads/photo.png"
      dismissDelayMs: 60000
      onDismissed: root.clickDismissCount++
    }
  }

  TestCase {
    id: t
    when: false
  }

  Stepper {
    id: stepper
    name: "Toast"
    intervalMs: 20
    deadlineMs: 10000
    onTimeout: () => Check.fail("timed out before the checks finished")
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.checkRendering()
  }

  // checkRendering confirms a toast with no text is invisible, and one
  // with text shows the checkmark glyph paired with the words, never a
  // bare glyph or colour alone.
  function checkRendering(): void {
    if (hiddenToast.visible) return Check.fail("a toast with no text is visible");

    if (!autoToast.visible) return Check.fail("a toast with text is not visible");
    const label = Check.find(autoToast, "toastLabel");
    if (!label || label.text.indexOf("Copied to clipboard") < 0)
      return Check.fail("toast label does not show its text: " + (label && label.text));
    if (label.text.indexOf("✓") < 0)
      return Check.fail("toast label has no glyph paired with its text: " + label.text);
    if (label.textFormat !== Text.PlainText)
      return Check.fail("toast label textFormat is " + label.textFormat + ", want PlainText");

    root.checkClickDismiss();
  }

  // checkClickDismiss clicks the second toast, whose own delay is far
  // longer than this test's deadline, and confirms dismissed() fired
  // from the click rather than the timer.
  function checkClickDismiss(): void {
    t.mouseClick(Check.find(clickToast, "toastDismissArea"));
    if (root.clickDismissCount !== 1)
      return Check.fail("clicking the toast did not fire dismissed(): count = " + root.clickDismissCount);

    root.checkAutoDismiss();
  }

  // checkAutoDismiss polls until the first toast's short dismissDelayMs
  // fires its own timer, with nothing clicked.
  function checkAutoDismiss(): void {
    stepper.retry(root.waitForAutoDismiss);
  }

  function waitForAutoDismiss(): void {
    if (root.autoDismissCount === 1) {
      console.log("PASS Toast");
      Qt.exit(0);
      return;
    }

    stepper.attempts++;
    if (stepper.attempts >= stepper.maxAttempts) return Check.fail("the toast never auto-dismissed on its own");
    stepper.retry(root.waitForAutoDismiss);
  }
}
