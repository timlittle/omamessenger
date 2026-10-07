// Records the README demo GIF: drives the real Panel, offscreen, against
// the fake helper's small demo seed (OMA_FAKE_DEMO=1; see
// backend/internal/connector/fake/demo.go), and saves a steady stream of
// PNG frames while it goes. `make demo` runs this the same isolated way
// `make test-qml` runs tests/qml/, then assembles the frames into
// docs/demo.gif and docs/demo.png with ffmpeg. It is not under tests/qml/
// so `make test-qml` never runs it, and it is not a pass/fail test: it
// logs PASS or FAIL the same way so a broken run is easy to spot, but its
// job is the recording, not an assertion.
//
// The whole thing has to read in well under ten seconds, so it shows only
// a few strong moments: the list with unread badges, a chat with a real
// loaded photo and a link preview, replying (showing the composer's
// writing-mode border) and sending, then reacting. Each step below
// performs one user-visible action — a shortcut, typing a query, a click
// — or holds the picture briefly so a viewer can read it, the same
// polling step engine tests/qml/Flows/shell.qml uses. A small on-screen
// label names each action, for the recording only: nothing like it exists
// in the product.
import QtQuick
import QtTest
import Quickshell
import "ui"
import "ui/lib/Timeline.js" as Timeline
import "Check.js" as Check

ShellRoot {
  id: root

  property int step: 0
  property int attempts: 0
  readonly property string testRoot: String(Qt.resolvedUrl(".")).replace("file://", "")
  // framesDir is where captured frames are saved. It has to sit inside
  // this root: Quickshell resolves a relative URL that would escape its
  // own `-p` root to a blackhole instead of a real path. The recipe that
  // runs this disables Quickshell's file watcher and creates the
  // directory before quickshell starts.
  readonly property string framesDir: root.testRoot + "/frames"
  // frameIndex numbers the saved frames in capture order.
  property int frameIndex: 0
  // capturing is true for the stretch of the recording worth keeping:
  // from the moment the seeded accounts are ready to the last held frame.
  property bool capturing: false

  property var steps: [
    root.waitForReady,
    root.startCapturing,
    root.holdFor(8),
    root.openPriya,
    root.waitForChatReady,
    root.holdFor(10),
    root.focusComposer,
    root.waitForComposerFocused,
    root.sendReply,
    root.holdFor(6),
    root.submitReply,
    root.waitForReplyDelivered,
    root.holdFor(8),
    root.openReactCommand,
    root.waitForReactionPicker,
    root.pickReaction,
    root.waitForReactionApplied,
    root.holdFor(10)
  ]

  // fail stops the recording with a reason on stderr, the same way a
  // broken test would, so a bad run is never mistaken for a finished GIF.
  function fail(reason: string): void {
    console.error("FAIL step " + root.step + ": " + reason);
    root.capturing = false;
    Qt.exit(1);
  }

  // panel is the live Panel instance.
  function panel(): var {
    return panelLoader.item;
  }

  // title is the open conversation's header title, or "" when none is.
  function title(): string {
    const header = Check.find(root.panel(), "conversationTitle");
    return header ? header.text : "";
  }

  // messageStatus returns the status of the newest loaded message with
  // the given text, or "" when none matches.
  function messageStatus(text: string): string {
    const model = Check.find(root.panel(), "messageListView").model;
    for (let i = 0; i < model.count; i++) {
      if (model.get(i).text === text) return model.get(i).status;
    }
    return "";
  }

  // delegateFor returns the loaded message delegate for messageId, or
  // null while it is not built, because the row is not on screen yet.
  function delegateFor(messageId: string): var {
    const items = Check.find(root.panel(), "messageListView").contentItem.children;
    for (let i = 0; i < items.length; i++) {
      if (items[i].modelData && items[i].modelData.id === messageId) return items[i];
    }
    return null;
  }

  // showLabel puts text in the on-screen keystroke label, for the frames
  // captured right after.
  function showLabel(text: string): void {
    keyLabel.text = text;
  }

  // typeText sends each character of text as a key click, into whichever
  // field currently holds keyboard focus.
  function typeText(text: string): void {
    for (const ch of text) t.keyClick(ch);
  }

  // holdFor returns a step that holds the picture for n ticks (n/10
  // seconds at the 100 ms step interval), so a viewer has time to read
  // whatever the step before it just showed.
  function holdFor(n: int): var {
    let seen = 0;
    return function() {
      seen++;
      return seen >= n;
    };
  }

  // waitForReady holds until the fake helper's demo seed has connected
  // and the list has loaded its handful of chats.
  function waitForReady(): var {
    const accounts = helperService.accounts;
    const model = Check.find(root.panel(), "conversationListView").model;
    return model.count === 4 && accounts.length === 1 && accounts.every((a) => a.status === "connected");
  }

  // startCapturing begins saving frames; everything before this point was
  // just the helper connecting, which the GIF does not need to show.
  function startCapturing(): var {
    root.capturing = true;
    return true;
  }

  // openPriya jumps to Priya Patel with Ctrl+K, the conversation switcher:
  // her chat carries both the photo and the link preview this records.
  function openPriya(): var {
    root.showLabel("Ctrl+K  →  Priya");
    t.keyClick(Qt.Key_K, Qt.ControlModifier);
    root.typeText("priya");
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForChatReady holds until Priya's chat is open, her newest photo
  // has actually downloaded and decoded (not the blurred placeholder,
  // not a broken image), and her link preview card shows.
  function waitForChatReady(): var {
    if (root.title() !== "Priya Patel") return false;

    const model = Check.find(root.panel(), "messageListView").model;
    let photoId = "", linkId = "";
    for (let i = 0; i < model.count; i++) {
      if (model.get(i).text === "[Photo]") photoId = model.get(i).id;
      if (model.get(i).text.indexOf("venue") >= 0) linkId = model.get(i).id;
    }
    if (!photoId || !linkId) return false;

    // Every message delegate carries its own LinkPreview and photoImage,
    // visible or not, so each lookup is scoped to the one message it
    // belongs to rather than taking whichever Check.find meets first.
    const photoDelegate = root.delegateFor(photoId);
    if (!photoDelegate) return false;
    const image = Check.find(photoDelegate, "photoImage");
    if (!image) return false;
    if (image.status === Image.Error) return Check.fail("the chat's photo failed to load");
    if (image.status !== Image.Ready) return false;
    if (String(image.source).indexOf("file://") !== 0) return false;

    const linkDelegate = root.delegateFor(linkId);
    if (!linkDelegate) return false;
    const preview = Check.find(linkDelegate, "linkPreview");
    return !!preview && preview.visible;
  }

  // focusComposer presses i to move keyboard focus into the composer.
  function focusComposer(): var {
    const input = Check.find(root.panel(), "composerInput");
    if (input && input.activeFocus) return true;

    root.showLabel("i  (write)");
    t.keyClick(Qt.Key_I);
    return true;
  }

  // waitForComposerFocused holds until the composer's text field has
  // keyboard focus, so typing lands in it rather than being swallowed by
  // an unbound key in the conversation pane.
  function waitForComposerFocused(): var {
    const input = Check.find(root.panel(), "composerInput");
    return !!input && input.activeFocus;
  }

  // sendReply types a short reply, showing the composer's writing-mode
  // border before it is sent.
  function sendReply(): var {
    root.showLabel("\"Count me in!\"");
    root.typeText("Count me in!");
    return true;
  }

  // submitReply presses Enter to send the typed reply.
  function submitReply(): var {
    root.showLabel("\"Count me in!\"  ⏎");
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForReplyDelivered holds until the reply has sent.
  function waitForReplyDelivered(): var {
    const status = root.messageStatus("Count me in!");
    return status === "sent" || status === "delivered";
  }

  // openReactCommand opens the command palette and runs "react to the
  // newest message" through it, rather than the hover toolbar's "+".
  function openReactCommand(): var {
    root.showLabel("Ctrl+/  →  react");
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    root.typeText("react");
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForReactionPicker holds until the emoji picker is open.
  function waitForReactionPicker(): var {
    const picker = Check.find(root.panel(), "reactionPicker");
    return !!picker && picker.visible;
  }

  // pickReaction accepts the picker's first emoji.
  function pickReaction(): var {
    root.showLabel("⏎  (👍)");
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForReactionApplied holds until the newest message's reaction
  // chip shows: applied at once, locally, before the helper confirms it.
  function waitForReactionApplied(): var {
    const model = Check.find(root.panel(), "messageListView").model;
    if (model.count === 0) return false;
    return Timeline.reactions(model.get(0)).length > 0;
  }

  // _OVERLAY_NAMES are every full-window overlay's object name: whichever
  // of them is open hides the columns row for the single frame captured
  // while it is; see captureFrame for why.
  readonly property var _OVERLAY_NAMES: ["photoViewer", "reactionPicker", "commandPalette", "newChatDialog", "closeConfirm", "accountSetup", "removeAccount"]

  // _openOverlay returns whichever full-window overlay is currently open,
  // or null when none is.
  function _openOverlay(): var {
    for (const name of root._OVERLAY_NAMES) {
      const item = Check.find(root.panel(), name);
      if (item && item.visible) return item;
    }
    return null;
  }

  // captureFrame grabs the window's own content, overlay label included,
  // and saves it as the next numbered frame. It grabs Panel's own
  // "keyArea", not the window's contentItem: the window's content item is
  // Quickshell's own proxy for the real backing window and never reports
  // a QML engine for grabToImage, offscreen; keyArea is a real item one
  // level down, sized to the whole window, and every overlay nests under
  // it, so grabbing it misses nothing.
  function captureFrame(): void {
    const item = Check.find(root.panel(), "keyArea");
    if (!item) return;

    // Offscreen, grabToImage sometimes paints the rail, list and
    // conversation columns over a full-window overlay despite them being
    // behind it in both z and paint order; hiding that whole row for just
    // the frame being grabbed works around it without changing anything
    // the product itself ever shows. The row has no object name of its
    // own, so it is found as the rail's parent.
    const overlayOpen = root._openOverlay() !== null;
    const rail = Check.find(root.panel(), "serviceRail");
    const columns = overlayOpen && rail ? rail.parent : null;
    if (columns) columns.visible = false;

    const index = root.frameIndex;
    root.frameIndex++;
    item.grabToImage((result) => {
      if (columns) columns.visible = true;
      const n = String(index).length >= 5 ? String(index) : ("00000" + index).slice(-5);
      result.saveToFile(root.framesDir + "/frame-" + n + ".png");
    });
  }

  // runStep runs the current step and advances, retries or fails, the
  // same engine tests/qml/Flows/shell.qml uses.
  function runStep(): void {
    const result = root.steps[root.step]();
    if (typeof result === "string") return root.fail(result);

    if (result === true) {
      root.step++;
      root.attempts = 0;
      if (root.step === root.steps.length) {
        root.capturing = false;
        console.log("PASS Demo");
        return Qt.exit(0);
      }
    } else if (++root.attempts > 150) {
      return root.fail("condition not met within 15 s");
    }

    stepTimer.start();
  }

  Service {
    id: helperService
  }

  QtObject {
    id: fakeShell

    function hide(id) { root.panel().close(); }
    function serviceFor(id) { return helperService; }
    function toggle(id, payloadJson) { root.panel().open(payloadJson); }
    function summon(id, payloadJson) { root.panel().open(payloadJson); }
  }

  Loader {
    id: panelLoader

    sourceComponent: Panel {
      service: helperService
      shell: fakeShell
    }
  }

  // keyLabel is the recording's own keystroke overlay: reparented onto
  // the Panel's window once it exists, so it shows up in every grabbed
  // frame. Nothing like it exists in the real UI.
  Rectangle {
    id: keyLabel

    property string text: ""

    visible: text.length > 0
    width: label.implicitWidth + 32
    height: label.implicitHeight + 16
    radius: 8
    color: "#1a1a1aE6"
    z: 10000
    anchors.bottom: parent ? parent.bottom : undefined
    anchors.horizontalCenter: parent ? parent.horizontalCenter : undefined
    anchors.bottomMargin: 24

    Text {
      id: label
      anchors.centerIn: parent
      text: keyLabel.text
      color: "#ffffff"
      font.pixelSize: 18
      font.bold: true
    }
  }

  TestCase {
    id: t

    when: false
  }

  Timer {
    id: stepTimer

    interval: 100
    onTriggered: root.runStep()
  }

  Timer {
    interval: 100
    running: root.capturing
    repeat: true
    onTriggered: root.captureFrame()
  }

  // A backstop: it only fires if a step hangs without failing.
  Timer {
    running: true
    interval: 90000
    onTriggered: root.fail("timed out")
  }

  // Start once Quickshell has finished loading; Qt.exit() is ignored
  // before then. Placing the overlay needs "keyArea" to exist, which
  // open() creates. The window keeps Panel's own default size: setting
  // implicitWidth/implicitHeight here, before or after open(), changed
  // nothing offscreen, so the recording is at whatever size that is.
  Timer {
    running: true
    interval: 50
    onTriggered: {
      root.panel().open("{}");
      keyLabel.parent = Check.find(root.panel(), "keyArea");

      root.runStep();
    }
  }
}
