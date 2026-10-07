// Records the README demo GIF: drives the real Panel, offscreen, against
// the fake helper's scripted accounts, and saves a steady stream of PNG
// frames while it goes. `make demo` runs this the same isolated way
// `make test-qml` runs tests/qml/, then assembles the frames into
// docs/demo.gif and docs/demo.png with ffmpeg. It is not under tests/qml/
// so `make test-qml` never runs it, and it is not a pass/fail test: it
// logs PASS or FAIL the same way so a broken run is easy to spot, but its
// job is the recording, not an assertion.
//
// Each step below performs one user-visible action — a shortcut, typing a
// query, a click — or holds the picture for a while so a viewer can read
// it, the same polling step engine tests/qml/Flows/shell.qml uses. A
// small on-screen label names the key or click each action step took,
// for the recording only: nothing like it exists in the product.
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
    root.holdFor(18),
    root.openAlex,
    root.waitForAlexLinkPreview,
    root.holdFor(16),
    root.openMum,
    root.waitForMumPhoto,
    root.holdFor(10),
    root.scrollToOlderPhoto,
    root.clickOlderPhoto,
    root.waitForViewerReady,
    root.holdFor(10),
    root.stepToNewerPhoto,
    root.waitForViewerReady,
    root.holdFor(12),
    root.closeViewer,
    root.waitForViewerClosed,
    root.replyToPhoto,
    root.waitForReplyBanner,
    root.holdFor(8),
    root.focusComposer,
    root.waitForComposerFocused,
    root.sendReply,
    root.waitForReplyDelivered,
    root.holdFor(10),
    root.openReactCommand,
    root.waitForReactionPicker,
    root.holdFor(6),
    root.pickHeartReaction,
    root.waitForReactionApplied,
    root.holdFor(14),
    root.searchForTicket,
    root.waitForTicketSearch,
    root.holdFor(14),
    root.leaveSearch,
    root.searchForDentist,
    root.waitForDentistSearch,
    root.openDentist,
    root.waitForDentistOpen,
    root.holdFor(8),
    root.hideDentistChat,
    root.holdFor(6),
    root.closeAndClearSearch,
    root.waitForListWithoutDentist,
    root.showAllChats,
    root.waitForDentistDimmed,
    root.holdFor(16),
    root.openCommandPalette,
    root.holdFor(18),
    root.closePalette,
    root.holdFor(8)
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

  // listModel is the conversation list the window shows.
  function listModel(): var {
    return Check.find(root.panel(), "conversationListView").model;
  }

  // rowIndex returns the visible list row with the given title, or -1.
  function rowIndex(title: string): int {
    const model = root.listModel();
    for (let i = 0; i < model.count; i++) {
      if (model.get(i).title === title) return i;
    }
    return -1;
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

  // photoMessageIds returns the ids of every loaded photo message in the
  // open conversation, newest first.
  function photoMessageIds(): var {
    const model = Check.find(root.panel(), "messageListView").model;
    const ids = [];
    for (let i = 0; i < model.count; i++) {
      if (model.get(i).text === "[Photo]") ids.push(model.get(i).id);
    }
    return ids;
  }

  // photoImageFor returns the "photoImage" inside the loaded delegate for
  // messageId, or null while that delegate is not built, because the row
  // is not on screen yet.
  function photoImageFor(messageId: string): var {
    const items = Check.find(root.panel(), "messageListView").contentItem.children;
    for (let i = 0; i < items.length; i++) {
      if (items[i].modelData && items[i].modelData.id === messageId) return Check.find(items[i], "photoImage");
    }
    return null;
  }

  // viewerImage returns the in-app photo viewer's own image, scoped to
  // the viewer itself: the bubble behind it carries an image with the
  // same object name.
  function viewerImage(): var {
    const viewer = Check.find(root.panel(), "photoViewer");
    return viewer ? Check.find(viewer, "photoImage") : null;
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

  // waitForReady holds until the fake helper has seeded every account and
  // the list has loaded all of its chats.
  function waitForReady(): var {
    const accounts = helperService.accounts;
    return root.listModel().count === 11 && accounts.length === 3 && accounts.every((a) => a.status === "connected");
  }

  // startCapturing begins saving frames; everything before this point was
  // just the helper connecting, which the GIF does not need to show.
  function startCapturing(): var {
    root.capturing = true;
    return true;
  }

  // openAlex jumps to Alex Chen with Ctrl+K, the conversation switcher.
  function openAlex(): var {
    root.showLabel("Ctrl+K  →  Alex");
    t.keyClick(Qt.Key_K, Qt.ControlModifier);
    root.typeText("alex");
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForAlexLinkPreview holds until Alex Chen is open and showing the
  // link preview on her tickets message.
  function waitForAlexLinkPreview(): var {
    if (root.title() !== "Alex Chen") return false;
    const preview = Check.find(root.panel(), "linkPreview");
    return !!preview && preview.visible;
  }

  // openMum jumps to Mum, whose newest message is a photo.
  function openMum(): var {
    root.showLabel("Ctrl+K  →  Mum");
    t.keyClick(Qt.Key_K, Qt.ControlModifier);
    root.typeText("mum");
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForMumPhoto holds until Mum is open and her newest photo has
  // downloaded into the media cache, so its bubble shows the real image.
  function waitForMumPhoto(): var {
    if (root.title() !== "Mum") return false;
    const model = Check.find(root.panel(), "messageListView").model;
    if (model.count === 0 || model.get(0).text !== "[Photo]") return false;
    return model.get(0).mediaPath !== "";
  }

  // scrollToOlderPhoto pages up so Mum's older photo, further back in her
  // history, is on screen.
  function scrollToOlderPhoto(): var {
    root.showLabel("Ctrl+U  (scroll up)");
    t.keyClick(Qt.Key_U, Qt.ControlModifier);
    return true;
  }

  // clickOlderPhoto clicks Mum's older photo once its delegate has been
  // built, opening it in the in-app viewer.
  function clickOlderPhoto(): var {
    const ids = root.photoMessageIds();
    if (ids.length < 2) return Check.fail("Mum has only " + ids.length + " loaded photo(s), want 2");

    const image = root.photoImageFor(ids[1]);
    if (!image) return false;

    root.showLabel("click  (open photo)");
    t.mouseClick(image, image.width / 2, image.height / 2);
    return true;
  }

  // waitForViewerReady holds until the in-app viewer shows a fully loaded
  // image, not a spinner or a broken one.
  function waitForViewerReady(): var {
    const image = root.viewerImage();
    if (!image) return false;
    if (image.status === Image.Error) return Check.fail("the photo viewer's image failed to load");
    return image.status === Image.Ready && String(image.source) !== "";
  }

  // stepToNewerPhoto presses → to move the in-app viewer to Mum's newer
  // photo.
  function stepToNewerPhoto(): var {
    root.showLabel("→  (next photo)");
    t.keyClick(Qt.Key_Right);
    return true;
  }

  // closeViewer presses Escape to leave the in-app photo viewer.
  function closeViewer(): var {
    root.showLabel("Esc  (close viewer)");
    t.keyClick(Qt.Key_Escape);
    return true;
  }

  // waitForViewerClosed holds until the viewer has closed.
  function waitForViewerClosed(): var {
    const viewer = Check.find(root.panel(), "photoViewer");
    return !viewer || !viewer.visible;
  }

  // replyToPhoto presses Shift+R to reply to the newest message, Mum's
  // photo.
  function replyToPhoto(): var {
    root.showLabel("Shift+R  (reply)");
    t.keyClick(Qt.Key_R, Qt.ShiftModifier);
    return true;
  }

  // waitForReplyBanner holds until the composer shows the reply banner.
  function waitForReplyBanner(): var {
    const banner = Check.find(root.panel(), "replyBanner");
    return !!banner && banner.visible;
  }

  // focusComposer presses i to move keyboard focus into the composer:
  // starting a reply fills in who it answers but leaves the list or
  // conversation pane focused, the same as the real UI.
  function focusComposer(): var {
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

  // sendReply types a reply to the photo and sends it with Enter.
  function sendReply(): var {
    root.showLabel("\"What a lovely photo!\"  ⏎");
    root.typeText("What a lovely photo!");
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForReplyDelivered holds until the reply has sent.
  function waitForReplyDelivered(): var {
    const status = root.messageStatus("What a lovely photo!");
    return status === "sent" || status === "delivered";
  }

  // openReactCommand opens the command palette and runs "react to the
  // newest message" through it, rather than the "+" chip, to show that
  // path too.
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

  // pickHeartReaction moves the picker's highlight once, to the heart,
  // and accepts it.
  function pickHeartReaction(): var {
    root.showLabel("→  ⏎  (❤️)");
    t.keyClick(Qt.Key_Right);
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

  // searchForTicket focuses search with Ctrl+G and types a query that
  // only Alex Chen's tickets message matches.
  function searchForTicket(): var {
    root.showLabel("Ctrl+G  →  \"ticket\"");
    t.keyClick(Qt.Key_G, Qt.ControlModifier);
    root.typeText("ticket");
    return true;
  }

  // waitForTicketSearch holds until only Alex Chen matches.
  function waitForTicketSearch(): var {
    const model = root.listModel();
    return model.count === 1 && model.get(0).title === "Alex Chen";
  }

  // leaveSearch clears the query and leaves the search field with two
  // Escapes, back to the full list.
  function leaveSearch(): var {
    root.showLabel("Esc  Esc  (clear search)");
    t.keyClick(Qt.Key_Escape);
    t.keyClick(Qt.Key_Escape);
    return true;
  }

  // searchForDentist searches for the dentist conversation, the one this
  // recording hides and then reveals again with show-all.
  function searchForDentist(): var {
    root.showLabel("Ctrl+G  →  \"dentist\"");
    t.keyClick(Qt.Key_G, Qt.ControlModifier);
    root.typeText("dentist");
    return true;
  }

  // waitForDentistSearch holds until only the dentist matches.
  function waitForDentistSearch(): var {
    const model = root.listModel();
    return model.count === 1 && model.get(0).title.indexOf("Dentist") >= 0;
  }

  // openDentist opens the matched conversation with Enter.
  function openDentist(): var {
    root.showLabel("⏎  (open)");
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForDentistOpen holds until the dentist conversation is open.
  function waitForDentistOpen(): var {
    return root.title().indexOf("Dentist") >= 0;
  }

  // hideDentistChat runs the local "hide or unhide chat" command on the
  // open conversation through the command palette.
  function hideDentistChat(): var {
    root.showLabel("Ctrl+/  →  unhide chat");
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    root.typeText("unhide chat");
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // closeAndClearSearch leaves the dentist conversation and clears the
  // search query, with two Escapes, back to the plain list.
  function closeAndClearSearch(): var {
    root.showLabel("Esc  Esc  (back to list)");
    t.keyClick(Qt.Key_Escape);
    t.keyClick(Qt.Key_Escape);
    return true;
  }

  // waitForListWithoutDentist holds until the standard list no longer
  // shows the now-hidden dentist conversation.
  function waitForListWithoutDentist(): var {
    return root.title() === "" && root.rowIndex("Dr. Bartholomew Featherstonehaugh-Wainwright (Dentist)") === -1;
  }

  // showAllChats runs "show or hide all chats" through the command
  // palette, so the hidden dentist chat reappears, dimmed.
  function showAllChats(): var {
    root.showLabel("Ctrl+/  →  hide all chats");
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    root.typeText("hide all chats");
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForDentistDimmed holds until the dentist row is back, marked
  // dimmed and labelled "Hidden".
  function waitForDentistDimmed(): var {
    const i = root.rowIndex("Dr. Bartholomew Featherstonehaugh-Wainwright (Dentist)");
    if (i === -1) return false;
    const row = root.listModel().get(i);
    return row.dimmed === true && row.dimLabel === "Hidden";
  }

  // openCommandPalette opens the full command list with Ctrl+/, to show
  // what it offers.
  function openCommandPalette(): var {
    root.showLabel("Ctrl+/  (command palette)");
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    return true;
  }

  // closePalette leaves the command palette with Escape.
  function closePalette(): var {
    root.showLabel("Esc");
    t.keyClick(Qt.Key_Escape);
    return true;
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
