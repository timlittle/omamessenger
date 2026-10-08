// Records the README's short demo GIFs: drives the real Panel, offscreen,
// against the fake helper's small demo seed (OMA_FAKE_DEMO=1; see
// backend/internal/connector/fake/demo.go), one feature per recording, and
// saves a steady stream of PNG frames while it goes. `make demo` runs this
// once per scenario, the same isolated way `make test-qml` runs tests/qml/,
// setting OMA_DEMO_SCENARIO to pick which one below runs, then assembles
// each run's frames into docs/demo/<scenario>.gif with ffmpeg. It is not
// under tests/qml/ so `make test-qml` never runs it, and it is not a
// pass/fail test: it logs PASS or FAIL the same way so a broken run is easy
// to spot, but its job is the recording, not an assertion.
//
// Every scenario has to read in well under ten seconds, so each shows only
// one feature, in a few strong moments. A step performs one user-visible
// action — a shortcut, typing a query, a click — or holds the picture
// briefly so a viewer can read it, the same polling step engine
// tests/qml/Flows/shell.qml uses. A small on-screen label names each
// action, for the recording only: nothing like it exists in the product.
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

  // scenarioName selects which recording below runs, set by the Makefile's
  // demo target through OMA_DEMO_SCENARIO, one quickshell run per
  // scenario. Falls back to the first one so running this file directly
  // during development still records something.
  readonly property string scenarioName: {
    const requested = Quickshell.env("OMA_DEMO_SCENARIO");
    return requested && requested.length > 0 ? requested : "list-and-send";
  }

  // unreadTitles are the demo seed's conversations Ctrl+J actually lands
  // on: unread and not muted. Weekend Hike carries an unread badge too,
  // but it is muted, so unread.next's own wrap-and-skip (Selection.js)
  // never lands there.
  readonly property var unreadTitles: ["Priya Patel", "Design Team"]

  // _navTitle is the title captureTitle last recorded, so a later wait
  // step can tell a jump actually moved somewhere new.
  property string _navTitle: ""

  property var steps: root.stepsFor(root.scenarioName)

  // _photoViewer is found once the panel exists (see the startup Timer
  // below) and kept, so keyLabel's own anchors binding below can read its
  // "open" property directly and move out of the way reactively: the
  // viewer's own header sits in the same top-right corner the label
  // otherwise uses.
  property var _photoViewer: null

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

  // waitForReady holds until the fake helper's demo seed — one WhatsApp
  // account and one Telegram account — has connected and the list has
  // loaded its handful of chats.
  function waitForReady(): var {
    const accounts = helperService.accounts;
    const model = Check.find(root.panel(), "conversationListView").model;
    return model.count === 4 && accounts.length === 2 && accounts.every((a) => a.status === "connected");
  }

  // startCapturing begins saving frames; everything before this point was
  // just the helper connecting, which no GIF needs to show.
  function startCapturing(): var {
    root.capturing = true;
    return true;
  }

  // pressKey returns a step that shows label, sends key with modifiers
  // (Qt.NoModifier for a plain key), and always succeeds at once.
  function pressKey(label: string, key: int, modifiers: int): var {
    return function() {
      root.showLabel(label);
      t.keyClick(key, modifiers);
      return true;
    };
  }

  // pressChatStep presses chat.next (Alt+↓), or chat.prev (Alt+↑) when
  // the open conversation is already the list's last row: Selection.move
  // does not wrap, so "next" from the bottom row is a silent no-op. A
  // direct step (called by the engine each tick, like waitForReady),
  // not a factory: which row is last shifts run to run — reading a
  // conversation drops its unread badge, which can reorder it below one
  // of the others — so this checks the live list right before pressing,
  // rather than deciding a direction up front.
  function pressChatStep(): var {
    const list = Check.find(root.panel(), "conversationListView").model;
    let atEnd = false;
    for (let i = 0; i < list.count; i++) {
      if (list.get(i).title === root.title()) atEnd = i === list.count - 1;
    }

    if (atEnd) {
      root.showLabel("Alt+↑  →  next chat");
      t.keyClick(Qt.Key_Up, Qt.AltModifier);
    } else {
      root.showLabel("Alt+↓  →  next chat");
      t.keyClick(Qt.Key_Down, Qt.AltModifier);
    }
    return true;
  }

  // openViaPalette returns a step that opens Ctrl+K, the conversation
  // switcher, types query and presses Enter: the fastest way to a known
  // chat by name.
  function openViaPalette(label: string, query: string): var {
    return function() {
      root.showLabel(label);
      t.keyClick(Qt.Key_K, Qt.ControlModifier);
      root.typeText(query);
      t.keyClick(Qt.Key_Return);
      return true;
    };
  }

  // waitForTitle returns a step that holds until the open conversation's
  // title is exactly want.
  function waitForTitle(want: string): var {
    return function() { return root.title() === want; };
  }

  // waitForTitleAndCount returns a step that holds until the open
  // conversation is want and its seeded messages have all loaded.
  function waitForTitleAndCount(want: string, count: int): var {
    return function() {
      if (root.title() !== want) return false;
      const model = Check.find(root.panel(), "messageListView").model;
      return model.count === count;
    };
  }

  // captureTitle records the open conversation's current title (or ""),
  // so a later waitForTitleChanged or waitForUnreadTitleChanged step can
  // tell a jump actually landed somewhere new.
  function captureTitle(): var {
    root._navTitle = root.title();
    return true;
  }

  // waitForTitleChanged returns a step that holds until the open
  // conversation's title differs from what captureTitle last recorded.
  function waitForTitleChanged(): var {
    return function() {
      const current = root.title();
      return !!current && current !== root._navTitle;
    };
  }

  // waitForUnreadTitleChanged returns a step like waitForTitleChanged,
  // but only accepts a title that is one of unreadTitles: the chats
  // Ctrl+J is meant to land on.
  function waitForUnreadTitleChanged(): var {
    return function() {
      const current = root.title();
      if (!current || current === root._navTitle) return false;
      return root.unreadTitles.indexOf(current) !== -1;
    };
  }

  // focusComposer presses i to move keyboard focus into the composer,
  // unless it already has it.
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

  // waitForComposerBlurred holds until the composer's text field has
  // lost keyboard focus, the mirror of waitForComposerFocused.
  function waitForComposerBlurred(): var {
    const input = Check.find(root.panel(), "composerInput");
    return !!input && !input.activeFocus;
  }

  // composeMessage returns a step that shows text as the label and types
  // it into whichever field has focus, showing the composer at full
  // contrast before it is sent.
  function composeMessage(text: string): var {
    return function() {
      root.showLabel("\"" + text + "\"");
      root.typeText(text);
      return true;
    };
  }

  // sendComposed returns a step that presses Enter to send text, already
  // typed by a prior composeMessage step.
  function sendComposed(text: string): var {
    return function() {
      root.showLabel("\"" + text + "\"  ⏎");
      t.keyClick(Qt.Key_Return);
      return true;
    };
  }

  // waitForDelivered returns a step that holds until text's message has
  // sent or delivered.
  function waitForDelivered(text: string): var {
    return function() {
      const status = root.messageStatus(text);
      return status === "sent" || status === "delivered";
    };
  }

  // reactToHighlighted leaves the composer with Escape, which moves the
  // highlight onto the message just sent, then presses e to open the
  // picker for it: the direct shortcut, rather than the hover toolbar's
  // "+" or the command palette.
  function reactToHighlighted(): var {
    root.showLabel("Esc  ·  e  →  react");
    t.keyClick(Qt.Key_Escape);
    t.keyClick(Qt.Key_E);
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

  // waitForReplyBanner holds until the composer shows which message it
  // is about to answer.
  function waitForReplyBanner(): var {
    const banner = Check.find(root.panel(), "replyBanner");
    return !!banner && banner.visible;
  }

  // waitForPriyaChatReady holds until Priya's chat is open, her newest
  // photo has actually downloaded and decoded (not the blurred
  // placeholder, not a broken image), and her link preview card shows.
  function waitForPriyaChatReady(): var {
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

  // openHighlighted returns a step that presses Enter on the highlighted
  // message: opens a photo in the full viewer, or starts a voice note
  // playing, the same action the message list's own Enter takes.
  function openHighlighted(label: string): var {
    return function() {
      root.showLabel(label);
      t.keyClick(Qt.Key_Return);
      return true;
    };
  }

  // waitForPhotoViewerReady holds until the full-window photo viewer is
  // open and its own copy of the image, not the bubble's, has actually
  // decoded.
  function waitForPhotoViewerReady(): var {
    const viewer = Check.find(root.panel(), "photoViewer");
    if (!viewer || !viewer.open) return false;

    const image = Check.find(viewer, "photoImage");
    if (!image) return false;
    if (image.status === Image.Error) return Check.fail("the opened photo failed to load");
    if (image.status !== Image.Ready) return false;
    return String(image.source).indexOf("file://") === 0;
  }

  // waitForPhotoViewerClosed holds until the full-window photo viewer has
  // closed.
  function waitForPhotoViewerClosed(): var {
    const viewer = Check.find(root.panel(), "photoViewer");
    return !viewer || !viewer.open;
  }

  // waitForVoiceReady holds until Jordan's chat is open and its newest
  // message shows a voice note player, which lags slightly behind the
  // model filling once a chat opens.
  function waitForVoiceReady(): var {
    if (root.title() !== "Jordan Lee") return false;
    const player = Check.find(root.panel(), "voiceNotePlayer");
    return !!player && player.visible;
  }

  // waitForVoicePlaying holds until the helper has downloaded the voice
  // note and the in-window player reports a real path and started
  // playing, never landing on the "Unavailable" state a silent fetch
  // failure would leave it in.
  function waitForVoicePlaying(): var {
    const button = Check.find(root.panel(), "voicePlayButton");
    const time = Check.find(root.panel(), "voiceTimeLabel");
    if (time && time.text === "Unavailable") return Check.fail("the voice note reports Unavailable instead of playing");
    return !!button && button.text === "⏸";
  }

  // openPaletteAndType returns a step that opens Ctrl+K and types query,
  // leaving the palette open for a message search's debounce to answer.
  function openPaletteAndType(label: string, query: string): var {
    return function() {
      root.showLabel(label);
      t.keyClick(Qt.Key_K, Qt.ControlModifier);
      root.typeText(query);
      return true;
    };
  }

  // waitForMessageRow holds until the open command palette shows a row
  // whose text contains needle, case-insensitively: the debounced
  // "Messages" section has answered.
  function waitForMessageRow(needle: string): var {
    return function() {
      const list = Check.find(root.panel(), "paletteList");
      if (!list) return false;
      return Check.texts(list).some((node) => String(node.text).toLowerCase().indexOf(needle) >= 0);
    };
  }

  // acceptHighlighted returns a step that shows label and presses Enter
  // on whichever row the palette currently highlights.
  function acceptHighlighted(label: string): var {
    return function() {
      root.showLabel(label);
      t.keyClick(Qt.Key_Return);
      return true;
    };
  }

  // waitForMessageVisible holds until titleWant is open and the message
  // whose text contains needle has a loaded delegate on screen: proof the
  // palette's "Messages" row landed on the exact message, scrolled into
  // view, not just whatever chat opened. Landing also sets the exact
  // message as the highlighted one, same as any other open, but that
  // never has a visible moment to show here: opening a conversation
  // always focuses the composer at once (see ConversationController's
  // resetHighlight), and a highlight bar never shows while writing.
  function waitForMessageVisible(titleWant: string, needle: string): var {
    return function() {
      if (root.title() !== titleWant) return false;

      const model = Check.find(root.panel(), "messageListView").model;
      for (let i = 0; i < model.count; i++) {
        if (String(model.get(i).text).toLowerCase().indexOf(needle) < 0) continue;
        return !!root.delegateFor(model.get(i).id);
      }
      return false;
    };
  }

  // stepsFor returns the step list for scenario name: one feature, a
  // few strong moments, the only thing each recording needs to show.
  function stepsFor(name: string): var {
    switch (name) {
    case "list-and-send": return [
      root.waitForReady,
      root.startCapturing,
      root.holdFor(8),
      root.openViaPalette("Ctrl+K  →  Design Team", "design"),
      root.waitForTitle("Design Team"),
      root.holdFor(6),
      root.focusComposer,
      root.waitForComposerFocused,
      root.composeMessage("Count me in!"),
      root.holdFor(6),
      root.sendComposed("Count me in!"),
      root.waitForDelivered("Count me in!"),
      root.holdFor(10)
    ];
    // Ctrl+J and Alt+↓/↑ open a conversation through
    // ConversationController._openPreservingMode, which keeps whichever
    // mode the user was already in (see its own doc comment): jumping
    // from the list, not already writing, lands back in scrolling mode,
    // so j/k work at once with no Escape needed. Ctrl+K's plain "open a
    // conversation" path has no such guard and always ends up writing;
    // the media and reply-reaction scenarios below account for that.
    case "keyboard-nav": return [
      root.waitForReady,
      root.startCapturing,
      root.holdFor(6),
      root.captureTitle,
      root.pressKey("Ctrl+J  →  unread", Qt.Key_J, Qt.ControlModifier),
      root.waitForUnreadTitleChanged(),
      root.holdFor(6),
      root.captureTitle,
      root.pressKey("Ctrl+J  →  unread", Qt.Key_J, Qt.ControlModifier),
      root.waitForUnreadTitleChanged(),
      root.holdFor(6),
      root.captureTitle,
      root.pressChatStep,
      root.waitForTitleChanged(),
      root.waitForComposerBlurred,
      root.holdFor(5),
      root.pressKey("k  (scroll)", Qt.Key_K, Qt.NoModifier),
      root.pressKey("k  (scroll)", Qt.Key_K, Qt.NoModifier),
      root.holdFor(6),
      root.focusComposer,
      root.waitForComposerFocused,
      root.holdFor(8)
    ];
    case "palette-search": return [
      root.waitForReady,
      root.startCapturing,
      root.holdFor(6),
      root.openPaletteAndType("Ctrl+K  →  \"venue\"", "venue"),
      root.waitForMessageRow("venue"),
      root.holdFor(8),
      root.acceptHighlighted("⏎  →  jump"),
      root.waitForMessageVisible("Priya Patel", "venue"),
      root.holdFor(10)
    ];
    case "media": return [
      root.waitForReady,
      root.startCapturing,
      root.holdFor(5),
      root.openViaPalette("Ctrl+K  →  Priya", "priya"),
      root.waitForPriyaChatReady,
      root.waitForComposerFocused,
      root.holdFor(5),
      root.pressKey("Esc  (scroll)", Qt.Key_Escape, Qt.NoModifier),
      root.waitForComposerBlurred,
      root.openHighlighted("⏎  →  open photo"),
      root.waitForPhotoViewerReady,
      root.holdFor(8),
      root.pressKey("Esc  (close)", Qt.Key_Escape, Qt.NoModifier),
      root.waitForPhotoViewerClosed,
      root.openViaPalette("Ctrl+K  →  Jordan", "jordan"),
      root.waitForVoiceReady,
      root.waitForComposerFocused,
      root.holdFor(3),
      root.pressKey("Esc  (scroll)", Qt.Key_Escape, Qt.NoModifier),
      root.waitForComposerBlurred,
      root.openHighlighted("⏎  →  play voice"),
      root.waitForVoicePlaying,
      root.holdFor(8)
    ];
    case "reply-reaction": return [
      root.waitForReady,
      root.startCapturing,
      root.holdFor(6),
      root.openViaPalette("Ctrl+K  →  Weekend Hike", "weekend"),
      root.waitForTitleAndCount("Weekend Hike", 5),
      root.waitForComposerFocused,
      root.holdFor(4),
      root.pressKey("Esc  (scroll)", Qt.Key_Escape, Qt.NoModifier),
      root.waitForComposerBlurred,
      root.pressKey("r  →  reply", Qt.Key_R, Qt.NoModifier),
      root.waitForReplyBanner,
      root.holdFor(4),
      root.focusComposer,
      root.waitForComposerFocused,
      root.composeMessage("Count me in!"),
      root.holdFor(5),
      root.sendComposed("Count me in!"),
      root.waitForDelivered("Count me in!"),
      root.holdFor(6),
      root.reactToHighlighted,
      root.waitForReactionPicker,
      root.holdFor(4),
      root.pickReaction,
      root.waitForReactionApplied,
      root.holdFor(8)
    ];
    default: return [() => root.fail("unknown scenario \"" + name + "\"")];
    }
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

    const index = root.frameIndex;
    root.frameIndex++;
    item.grabToImage((result) => {
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
        console.log("PASS " + root.scenarioName);
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

    // Positioned in the top-right corner normally: every step's content
    // sits in the list, the conversation body or the composer, none of
    // which reach up here. The one exception is the photo viewer, whose
    // own header (the size label and the close button) lives in that
    // same corner; while it is open the label moves to bottom-centre,
    // above its footer hints, over the photo itself instead. Plain x/y
    // bindings, not anchors: toggling between two anchor lines (right
    // vs. horizontalCenter) left a stale one active from whichever was
    // set first, pulling the label to the middle of the window instead
    // of either corner.
    readonly property bool _overViewer: !!root._photoViewer && root._photoViewer.open
    visible: text.length > 0
    width: label.implicitWidth + 32
    height: label.implicitHeight + 16
    radius: 8
    color: "#1a1a1aE6"
    z: 10000
    x: parent ? (keyLabel._overViewer ? (parent.width - keyLabel.width) / 2 : parent.width - keyLabel.width - 16) : 0
    y: parent ? (keyLabel._overViewer ? parent.height - keyLabel.height - 56 : 16) : 0

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
  //
  // The three columns (rail, list, conversation) are forced onto their
  // own texture layer once, up front: offscreen, grabToImage otherwise
  // sometimes paints them over a full-window overlay despite them being
  // behind it in both z and paint order, so a dimmed background behind
  // the reaction picker or the command palette came out solid black
  // instead. Compositing the columns as one flat layer, the way a real
  // GPU-backed window already effectively does, fixes the order for
  // real rather than hiding the columns for the frames it would show.
  Timer {
    running: true
    interval: 50
    onTriggered: {
      root.panel().open("{}");
      keyLabel.parent = Check.find(root.panel(), "keyArea");
      root._photoViewer = Check.find(root.panel(), "photoViewer");

      const rail = Check.find(root.panel(), "serviceRail");
      if (rail && rail.parent) rail.parent.layer.enabled = true;

      root.runStep();
    }
  }
}
