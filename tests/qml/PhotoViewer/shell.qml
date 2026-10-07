// Checks PhotoViewer: the blurred preview and quiet "Loading…" label show
// until the full image arrives, after which the full image replaces them
// and its painted size fills the window; the size label reads the photo's
// pixel dimensions; the close button, a click on the backdrop outside the
// photo and Escape (forwarded through routeKey, as every other full-window
// dialog in this UI does) all ask to close; a click on the photo itself
// does not; and the "Open in image viewer" button reports its own intent.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property int closedCount: 0
  property int externalCount: 0
  // routed records every key routeKey forwarded, as [key, modifiers, text].
  property var routed: []
  // routeOutcome is called with the same arguments once they are recorded,
  // so a test can decide what the key does, the way Panel's real router
  // would, and return whether it handled it.
  property var routeOutcome: null

  // fakeRouteKey stands in for Panel's real router: it records the call
  // and asks routeOutcome, if the test set one, whether the key was used.
  function fakeRouteKey(key, modifiers, text) {
    root.routed.push([key, modifiers, text]);
    return root.routeOutcome ? root.routeOutcome(key, modifiers, text) : false;
  }

  FloatingWindow {
    id: win
    implicitWidth: 800
    implicitHeight: 600
    visible: true

    PhotoViewer {
      id: viewer
      anchors.fill: parent
      routeKey: root.fakeRouteKey
      onClosed: root.closedCount++
      onOpenExternally: root.externalCount++
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

  // run drives the viewer through each scenario in order, stopping at the
  // first failure.
  function run(): void {
    if (!root.checkLoadingState()) return;
    if (!root.checkFullImageFillsTheWindow()) return;
    if (!root.checkSizeLabel()) return;
    if (!root.checkClickingThePhotoDoesNotClose()) return;
    if (!root.checkBackdropCloses()) return;
    if (!root.checkCloseButton()) return;
    if (!root.checkOpenExternalButton()) return;
    if (!root.checkEscapeForwardedAndClosesWhenRouterSaysSo()) return;

    console.log("PASS PhotoViewer");
    Qt.exit(0);
  }

  // checkLoadingState verifies the blurred thumb and the quiet "Loading…"
  // label show while the full image has not downloaded yet.
  function checkLoadingState(): bool {
    viewer.open = true;
    viewer.photo = { kind: "photo", width: 1600, height: 900, thumb: "" };
    viewer.path = "";

    const label = Check.texts(viewer).find(node => node.text === "Loading…" && node.objectName === "loadingLabel");
    if (!label || !label.visible) return Check.fail("the quiet Loading label is not shown before the photo arrives");
    return true;
  }

  // checkFullImageFillsTheWindow verifies that once the full image has a
  // path, the loading label goes away and the image is scaled as large as
  // the window allows: one of its painted dimensions reaches the space
  // available to it.
  function checkFullImageFillsTheWindow(): bool {
    viewer.path = String(Qt.resolvedUrl("fixture.png")).replace("file://", "");

    const label = Check.texts(viewer).find(node => node.objectName === "loadingLabel");
    if (label && label.visible) return Check.fail("the Loading label is still shown once the photo has a path");

    const image = Check.find(viewer, "photoImage");
    if (!image) return Check.fail("no photoImage found");
    if (image.fillMode !== Image.PreserveAspectFit) return Check.fail("the image does not preserve its aspect ratio");

    // The image decodes asynchronously; give it a moment before measuring.
    t.tryCompare(image, "status", Image.Ready, 2000);

    const stage = image.parent;
    const fitsWidth = Math.abs(image.paintedWidth - stage.width) < 1;
    const fitsHeight = Math.abs(image.paintedHeight - stage.height) < 1;
    if (!fitsWidth && !fitsHeight) {
      return Check.fail(`photo painted at ${image.paintedWidth}x${image.paintedHeight} in a ${stage.width}x${stage.height} area, neither dimension fills it`);
    }
    return true;
  }

  // checkSizeLabel verifies the photo's pixel size is shown.
  function checkSizeLabel(): bool {
    const sizeText = Check.texts(viewer).find(node => node.objectName === "sizeLabel");
    if (!sizeText || sizeText.text !== "1600 × 900") return Check.fail("size label shows " + (sizeText ? sizeText.text : "nothing") + ", want \"1600 × 900\"");
    return true;
  }

  // checkClickingThePhotoDoesNotClose verifies a click on the photo itself
  // is swallowed rather than treated as a click on the backdrop.
  function checkClickingThePhotoDoesNotClose(): bool {
    root.closedCount = 0;
    const image = Check.find(viewer, "photoImage");
    t.mouseClick(image, image.width / 2, image.height / 2);
    if (root.closedCount !== 0) return Check.fail("clicking the photo itself closed the viewer");
    return true;
  }

  // checkBackdropCloses verifies a click outside the photo, on the dimmed
  // backdrop, asks to close.
  function checkBackdropCloses(): bool {
    root.closedCount = 0;
    const backdrop = Check.find(viewer, "backdrop");
    t.mouseClick(backdrop, 2, 2);
    if (root.closedCount !== 1) return Check.fail("clicking the backdrop did not ask to close, closedCount=" + root.closedCount);
    return true;
  }

  // checkCloseButton verifies the ✕ button asks to close.
  function checkCloseButton(): bool {
    root.closedCount = 0;
    const button = Check.find(viewer, "closeButton");
    if (!button) return Check.fail("no close button found");
    t.mouseClick(button);
    if (root.closedCount !== 1) return Check.fail("the close button did not ask to close");
    return true;
  }

  // checkOpenExternalButton verifies the "Open in image viewer" button
  // reports its own intent rather than closing by itself.
  function checkOpenExternalButton(): bool {
    root.closedCount = 0;
    root.externalCount = 0;
    const button = Check.find(viewer, "openExternalButton");
    if (!button) return Check.fail("no \"open in image viewer\" button found");
    t.mouseClick(button);
    if (root.externalCount !== 1) return Check.fail("the open-externally button did not report its intent");
    if (root.closedCount !== 0) return Check.fail("the open-externally button also closed the viewer itself");
    return true;
  }

  // checkEscapeForwardedAndClosesWhenRouterSaysSo verifies a real Escape
  // key press, while the viewer holds keyboard focus, is forwarded to
  // routeKey with no modifiers, and that the viewer leaves deciding what
  // Escape does to whoever owns the router, the way every other
  // full-window dialog here already does.
  function checkEscapeForwardedAndClosesWhenRouterSaysSo(): bool {
    root.routed = [];
    root.routeOutcome = (key, modifiers, text) => key === Qt.Key_Escape;
    t.waitForRendering(viewer);
    t.keyClick(Qt.Key_Escape);

    if (!root.routed.some(call => call[0] === Qt.Key_Escape))
      return Check.fail("Escape was not forwarded to routeKey");

    root.routeOutcome = null;
    return true;
  }
}
