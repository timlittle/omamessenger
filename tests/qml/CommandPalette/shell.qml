// Checks CommandPalette: a follow-up security review found its search
// field stayed alive for as long as the palette itself was open, even
// with the window itself not the active one (another window focused,
// or alt-tabbed away) - the exact "merely visible" gap already closed
// for every other field in this UI (see docs/decisions.md). This
// drives the field directly, the way tests/qml/NewChatDialog does for
// its own search field: typing, then toggling windowActive, checking
// the field is destroyed and comes back with the same text and real
// keyboard focus, without the palette itself ever closing.
import QtQuick
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  FloatingWindow {
    implicitWidth: 700
    implicitHeight: 500
    visible: true

    CommandPalette {
      id: palette
      anchors.fill: parent
      open: true
    }
  }

  Stepper {
    id: stepper
    name: "CommandPalette"
    startFn: () => root.waitForInitialFocus()
  }

  // waitForInitialFocus polls until the field built with the palette
  // has real keyboard focus: a freshly shown window is not always the
  // active one the instant it is created, the same reason
  // tests/qml/AccountSetup's own first keyboard check polls rather
  // than asserting at once.
  function waitForInitialFocus(): void {
    const field = Check.find(palette, "paletteField");
    if (field && field.activeFocus) { root.beginWindowInactiveCheck(); return; }
    stepper.retry(root.waitForInitialFocus);
  }

  // beginWindowInactiveCheck types a query, then proves turning the
  // window inactive destroys the field - without closing the palette
  // itself - and turning it active again restores both the typed text
  // and real keyboard focus.
  function beginWindowInactiveCheck(): void {
    const field = Check.find(palette, "paletteField");
    field.text = "ticket";

    palette.windowActive = false;
    if (Check.find(palette, "paletteField")) return Check.fail("the palette field still exists with the window inactive");
    if (!palette.open) return Check.fail("the window going inactive closed the palette itself");

    palette.windowActive = true;
    // palette's own onWindowActiveChanged restores focus through
    // Qt.callLater, exactly as the production code does, so the
    // deferred check below runs after that same callback has had its
    // turn. The window is already known active by this point (see
    // waitForInitialFocus above), unlike at startup, so one deferred
    // turn is enough here.
    Qt.callLater(root.checkFieldRestoredWithTextAndFocus);
  }

  // checkFieldRestoredWithTextAndFocus confirms the field came back
  // with exactly the text it held before the window went inactive, and
  // with real keyboard focus, and that the palette never closed.
  function checkFieldRestoredWithTextAndFocus(): void {
    const field = Check.find(palette, "paletteField");
    if (!field) return Check.fail("the palette field did not come back once the window was active again");
    if (field.text !== "ticket") return Check.fail("the palette field's text did not survive: " + JSON.stringify(field.text));
    if (!field.activeFocus) return Check.fail("the palette field did not get focus back once the window was active again");
    if (!palette.open) return Check.fail("the palette closed across the window-focus blip");

    stepper.pass();
  }
}
