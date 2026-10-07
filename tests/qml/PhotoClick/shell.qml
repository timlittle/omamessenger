// Confirms a suspected regression: that clicking a message's photo still
// opens the in-app photo viewer once the hover toolbar (added after this
// was last recorded working) has appeared over the bubble, the way a real
// mouse does it -- move onto the photo first, so the hover chrome shows,
// then click, rather than teleporting straight onto it. Mum's seeded chat
// carries two photos, her newest message and an older one several
// messages back, so this checks both: the older one is the one a mouse
// swallowed by a neighbouring message's hover chrome would most likely
// miss.
import QtQuick
import QtTest
import Quickshell
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  property int step: 0
  property int attempts: 0
  property string openedId: ""
  property var openedMedia: null

  property var steps: [
    root.waitForList,
    root.openMum,
    root.waitForMumHistory,
    root.clickNewestPhoto,
    root.waitForViewerOnNewest,
    root.closeViewer,
    root.waitForViewerClosed,
    root.clickOlderPhoto,
    root.waitForViewerOnOlder,
    root.closeViewer,
    root.waitForViewerClosed
  ]

  // fail stops the test with a reason on stderr.
  function fail(reason: string): void {
    console.error("FAIL step " + root.step + ": " + reason);
    Qt.exit(1);
  }

  // panel is the live Panel.
  function panel(): var {
    return panelLoader.item;
  }

  // messageModel is the open conversation's loaded timeline, newest first.
  function messageModel(): var {
    return Check.find(root.panel(), "messageListView").model;
  }

  // photoMessages returns every loaded message that carries a photo, in
  // model order (newest first).
  function photoMessages(): var {
    const model = root.messageModel();
    const out = [];
    for (let i = 0; i < model.count; i++) {
      const row = model.get(i);
      if (!row.media) continue;
      const media = JSON.parse(row.media);
      if (media.kind === "photo") out.push(row);
    }
    return out;
  }

  // photoViewFor returns the "photoView" item inside the loaded delegate
  // for messageId, or null when that delegate is not currently realized.
  function photoViewFor(messageId: string): var {
    const items = Check.find(root.panel(), "messageListView").contentItem.children;
    for (let i = 0; i < items.length; i++) {
      if (items[i].modelData && items[i].modelData.id === messageId) return Check.find(items[i], "photoView");
    }
    return null;
  }

  // hoverThenClick walks the pointer onto target in small steps, the way a
  // real mouse arrives rather than teleporting there, so any hover chrome
  // that only appears once the pointer rests over the bubble has a chance
  // to show itself before the click lands. It clicks at target's own
  // center every time, so a toolbar swallowing that exact point would
  // make the click miss.
  function hoverThenClick(target: var): void {
    const startX = target.width / 2;
    const startY = -40; // starts above the item, where a neighbouring message's own hover chrome could float
    const endX = target.width / 2;
    const endY = target.height / 2;
    const steps = 12;
    for (let i = 1; i <= steps; i++) {
      t.mouseMove(target, startX + (endX - startX) * i / steps, startY + (endY - startY) * i / steps);
    }
    t.mouseClick(target, endX, endY);
  }

  // waitForList holds until the fake accounts have seeded all 11 chats.
  function waitForList(): var {
    const listView = Check.find(root.panel(), "conversationListView");
    return listView && listView.count === 11;
  }

  // openMum jumps to Mum's chat with Ctrl+K, the conversation switcher.
  function openMum(): var {
    t.keyClick(Qt.Key_K, Qt.ControlModifier);
    for (const ch of "mum") t.keyClick(ch);
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForMumHistory holds until Mum's chat is open and her older
  // history, which carries the second, non-newest photo, has loaded.
  function waitForMumHistory(): var {
    const title = Check.find(root.panel(), "conversationTitle");
    if (!title || title.text !== "Mum") return false;
    return root.messageModel().count >= 50;
  }

  // clickNewestPhoto scrolls to Mum's newest message, her photo, moves the
  // mouse onto it and clicks it.
  function clickNewestPhoto(): var {
    const photos = root.photoMessages();
    if (photos.length < 2) return Check.fail(`Mum's loaded history carries ${photos.length} photo message(s), want at least 2`);
    if (photos[0].id !== root.messageModel().get(0).id) return Check.fail("Mum's newest message is not a photo");

    root.openedId = photos[0].id;
    root.openedMedia = JSON.parse(photos[0].media);
    Check.find(root.panel(), "conversationView").scrollToMessage(root.openedId);
    return root.driveClick(root.openedId);
  }

  // clickOlderPhoto scrolls to Mum's other photo, an earlier message that
  // is not the newest, moves the mouse onto it and clicks it.
  function clickOlderPhoto(): var {
    const photos = root.photoMessages();
    const older = photos.find(p => p.id !== root.messageModel().get(0).id);
    if (!older) return Check.fail("found no photo message other than the newest one");

    root.openedId = older.id;
    root.openedMedia = JSON.parse(older.media);
    Check.find(root.panel(), "conversationView").scrollToMessage(root.openedId);
    return root.driveClick(root.openedId);
  }

  // driveClick finds messageId's photo once its delegate is realized and
  // clicks it; it holds (returns false) while the delegate is not yet
  // built, the way every wait step here does, since the step runner retries.
  function driveClick(messageId: string): var {
    const view = root.photoViewFor(messageId);
    if (!view) return false;

    root.hoverThenClick(view);
    return true;
  }

  // waitForViewerOnNewest and waitForViewerOnOlder both hold until the
  // in-app photo viewer is open and showing the message just clicked, with
  // its image fully loaded, not just requested -- the same "never became
  // ready" symptom the regression report described.
  function waitForViewerOnNewest(): var {
    return root.waitForViewerReady();
  }

  function waitForViewerOnOlder(): var {
    return root.waitForViewerReady();
  }

  // waitForViewerReady holds until the viewer shows root.openedId's photo
  // with its image loaded.
  function waitForViewerReady(): var {
    const viewer = Check.find(root.panel(), "photoViewer");
    if (!viewer || !viewer.visible) return false;
    if (!viewer.photo) return false;
    if (viewer.photo.width !== root.openedMedia.width || viewer.photo.height !== root.openedMedia.height) {
      return Check.fail(`viewer shows a ${viewer.photo.width}x${viewer.photo.height} photo, want the clicked ${root.openedMedia.width}x${root.openedMedia.height} one`);
    }

    const image = Check.find(viewer, "photoImage");
    if (!image || image.status !== Image.Ready) return false;
    return true;
  }

  // closeViewer clicks the viewer's close button.
  function closeViewer(): var {
    const closeButton = Check.find(root.panel(), "closeButton");
    t.mouseClick(closeButton);
    return true;
  }

  // waitForViewerClosed holds until the viewer has closed.
  function waitForViewerClosed(): var {
    const viewer = Check.find(root.panel(), "photoViewer");
    return !viewer || !viewer.visible;
  }

  // runStep runs the current step and advances, retries or fails. 15 s
  // (150 attempts at 100 ms) matches the timeout the regression report
  // used: "the viewer never became ready within 15 s".
  function runStep(): void {
    const result = root.steps[root.step]();
    if (typeof result === "string") return root.fail(result);

    if (result === true) {
      root.step++;
      root.attempts = 0;
      if (root.step === root.steps.length) {
        console.log("PASS PhotoClick");
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

  TestCase {
    id: t

    when: false
  }

  Timer {
    id: stepTimer

    interval: 100
    onTriggered: root.runStep()
  }

  // A backstop: it only fires if a step hangs without failing.
  Timer {
    running: true
    interval: 55000
    onTriggered: root.fail("timed out")
  }

  // Start once Quickshell has finished loading; Qt.exit() is ignored
  // before then.
  Timer {
    running: true
    interval: 50
    onTriggered: {
      root.panel().open("{}");
      root.runStep();
    }
  }
}
