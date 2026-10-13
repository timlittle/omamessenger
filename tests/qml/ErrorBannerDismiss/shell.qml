// Drives the real Panel, real key events and a real mouse click to prove
// the helper-error banner can actually be dismissed: Escape clears it
// first and alone, without also closing the command palette open behind
// it in the same press, and a click on the banner's own dismiss button
// clears it too. The banner's merge priority and escapeAction's
// "dismiss-error" priority are already proven exhaustively in
// tests/unit/errorbanner.test.cjs and tests/unit/navigation.test.cjs, and
// the controllers' own wiring to them in tests/qml/ErrorBanner; this is
// the one place real keyboard delivery and a real mouse click on the real
// button can be exercised at all.
import QtQuick
import QtTest
import Quickshell
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  property var steps: [
    root.waitForList,
    root.openPaletteAndTriggerError,
    root.checkBannerVisible,
    root.checkEscapeDismissesBannerOnly,
    root.triggerErrorAgain,
    root.checkClickDismisses
  ]

  // panel is the live Panel.
  function panel(): var {
    return panelLoader.item;
  }

  // windowController is the real WindowController the panel wires up,
  // whose lastError feeds the banner.
  function windowController(): var {
    return Check.find(root.panel(), "windowController");
  }

  // waitForList holds until the fixture's accounts have all connected,
  // the same wait tests/qml/OverlayLifecycle uses.
  function waitForList(): var {
    const accounts = helperService.accounts;
    return Check.find(root.panel(), "conversationListView").count === 11
      && accounts.length === 3 && accounts.every((a) => a.status === "connected");
  }

  // openPaletteAndTriggerError opens the command palette, so something
  // else is open behind the banner, then sets a failure directly on the
  // real WindowController: a fixed input, the same as any other
  // component test, rather than engineering a genuine helper failure the
  // fake connector has no reachable way to produce.
  function openPaletteAndTriggerError(): var {
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    root.windowController().lastError = "component test failure";
    return true;
  }

  // checkBannerVisible polls until the palette has real focus and the
  // banner shows the failure just set above.
  function checkBannerVisible(): var {
    const field = Check.find(root.panel(), "paletteField");
    if (!field || !field.activeFocus) return false;

    const banner = Check.find(root.panel(), "errorBanner");
    const line = Check.find(root.panel(), "errorLine");
    if (!banner.visible || line.text !== "component test failure")
      return Check.fail("the banner did not show the failure: visible=" + banner.visible + " text=" + line.text);
    return true;
  }

  // checkEscapeDismissesBannerOnly presses Escape once and checks it
  // cleared only the banner: the command palette behind it must still be
  // open, proving Escape did not also close it in the same press.
  function checkEscapeDismissesBannerOnly(): var {
    t.keyClick(Qt.Key_Escape);
    if (Check.find(root.panel(), "errorBanner").visible) return Check.fail("Escape did not dismiss the banner");
    if (!Check.find(root.panel(), "commandPalette")) return Check.fail("Escape also closed the command palette in the same press");

    t.keyClick(Qt.Key_Escape);
    return true;
  }

  // triggerErrorAgain sets a second failure, for the click check below.
  function triggerErrorAgain(): var {
    root.windowController().lastError = "component test failure again";
    return Check.find(root.panel(), "errorBanner").visible || false;
  }

  // checkClickDismisses clicks the banner's own dismiss button and
  // checks the banner is gone.
  function checkClickDismisses(): var {
    t.mouseClick(Check.find(root.panel(), "dismissErrorButton"));
    if (Check.find(root.panel(), "errorBanner").visible) return Check.fail("clicking the dismiss button did not clear the banner");

    console.log("PASS ErrorBannerDismiss");
    Qt.exit(0);
  }

  // Named so Panel's own service property, inside the Loader, does not
  // shadow it and bind to itself.
  Service {
    id: helperService
  }

  FakeShell {
    id: fakeShell
    panel: root.panel()
    service: helperService
  }

  Loader {
    id: panelLoader

    sourceComponent: Panel {
      service: helperService
      shell: fakeShell
    }
  }

  TestCase {
    id: t
    when: false
  }

  Stepper {
    id: stepper
    name: "ErrorBannerDismiss"
    steps: root.steps
    deadlineMs: 30000
    startFn: () => { root.panel().open("{}"); stepper.runStep(); }
  }
}
