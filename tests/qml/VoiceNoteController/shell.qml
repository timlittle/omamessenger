// Checks VoiceNoteController: toggling a note plays it, toggling the
// same note again pauses rather than restarts it, and starting a second
// note takes over playback from the first, since only one note plays at
// a time. It also checks the fallback this whole feature exists for:
// pointing the controller at a runtime file that fails to load (standing
// in for a machine with no QtMultimedia installed) reports "not
// available" and makes every method a safe no-op, rather than the UI
// ever reaching for a player that is not there. Finally, it checks the
// same toggling through ConversationController.openMedia, which is what
// Enter on a highlighted voice note, and clicking its play button,
// both call: the first open downloads the note once and starts it, and
// a second open on the same message pauses it without downloading again.
import QtQuick
import Quickshell
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  function run(): void {
    if (!root.checkAvailable()) return;
    if (!root.checkToggleStartsAndPauses()) return;
    if (!root.checkSecondNoteTakesOver()) return;
    if (!root.checkUnavailableFallback()) return;
    if (!root.checkOpenMediaDownloadsAndToggles()) return;

    console.log("PASS VoiceNoteController");
    Qt.exit(0);
  }

  // checkAvailable verifies the real controller, loading its real
  // runtime, reports itself available: QtMultimedia is installed on
  // every machine these offscreen tests run on.
  function checkAvailable(): bool {
    if (!controller.available) return Check.fail("controller reports unavailable with its real runtime");
    return true;
  }

  // checkToggleStartsAndPauses verifies toggling a note plays it, and
  // toggling the same id again pauses it without forgetting it.
  function checkToggleStartsAndPauses(): bool {
    controller.toggle("m1", testRoot + "/tone.ogg", 5000);
    if (controller.playingId !== "m1") return Check.fail("playingId is \"" + controller.playingId + "\", want \"m1\"");
    if (!controller.playing) return Check.fail("note did not start playing");
    if (controller.durationMs !== 5000) return Check.fail("durationMs is " + controller.durationMs + ", want the 5000 hint before the real duration is known");

    controller.toggle("m1", testRoot + "/tone.ogg", 5000);
    if (controller.playingId !== "m1") return Check.fail("pausing a note forgot which one it was");
    if (controller.playing) return Check.fail("toggling a playing note a second time did not pause it");

    controller.toggle("m1", testRoot + "/tone.ogg", 5000);
    if (!controller.playing) return Check.fail("toggling a paused note a third time did not resume it");
    return true;
  }

  // checkSecondNoteTakesOver verifies starting a different note replaces
  // the one that was loaded, rather than playing both at once.
  function checkSecondNoteTakesOver(): bool {
    controller.toggle("m2", testRoot + "/tone.ogg", 9000);
    if (controller.playingId !== "m2") return Check.fail("a second note did not take over from the first");
    if (!controller.playing) return Check.fail("the second note did not start playing");
    if (controller.durationMs !== 9000) return Check.fail("durationMs did not reset to the second note's own hint");
    if (controller.positionMs !== 0) return Check.fail("positionMs did not reset for the second note");
    return true;
  }

  // checkUnavailableFallback verifies a controller whose runtime file
  // cannot load reports itself unavailable and leaves every method a
  // no-op, the path a machine without QtMultimedia takes.
  function checkUnavailableFallback(): bool {
    if (broken.available) return Check.fail("controller reports available with a runtime file that does not exist");

    broken.toggle("m3", testRoot + "/tone.ogg", 1000);
    if (broken.playingId !== "") return Check.fail("toggle on an unavailable controller still started a note");
    return true;
  }

  // checkOpenMediaDownloadsAndToggles verifies ConversationController's
  // own voice branch: opening a voice note with no path yet downloads it
  // through the service, then starts it playing; opening the same note
  // again pauses it, without asking the service for its media a second
  // time.
  function checkOpenMediaDownloadsAndToggles(): bool {
    conversationController.open({ id: "c1", title: "Voice chat" });

    conversationController.openMedia("vm1");
    if (fakeService.fetchRequests.length !== 1) return Check.fail("opening an undownloaded voice note fetched it " + fakeService.fetchRequests.length + " times, want 1");
    if (controller.playingId !== "vm1") return Check.fail("opening a voice note through openMedia did not start it");
    if (!controller.playing) return Check.fail("opening a voice note through openMedia did not set playing");

    conversationController.openMedia("vm1");
    if (fakeService.fetchRequests.length !== 1) return Check.fail("opening an already-downloaded voice note fetched it again");
    if (controller.playing) return Check.fail("opening a playing voice note a second time did not pause it");
    return true;
  }

  // testRoot is this test's own directory, so a path passed to toggle()
  // resolves the same way whichever directory the test runner uses.
  readonly property string testRoot: String(Qt.resolvedUrl(".")).replace("file://", "")

  VoiceNoteController {
    id: controller
  }

  VoiceNoteController {
    id: broken
    runtimeSource: Qt.resolvedUrl("does-not-exist.qml")
  }

  QtObject {
    id: fakeService

    property var uiState: ({})
    property var accounts: []
    // fetchRequests records every media.fetch call, by message id.
    property var fetchRequests: []

    function request(method, params, callback) {
      if (method === "messages.list") {
        callback(null, { hasMore: false, messages: [
          { id: "vm1", conversationId: "c1", senderId: "s", senderName: "S", text: "[Voice message]", outgoing: false, status: "received", created: 10, media: { kind: "voice", duration: 7 }, mediaPath: "" }
        ] });
        return;
      }
      if (method === "media.fetch") {
        fakeService.fetchRequests.push(params.messageId);
        callback(null, { path: testRoot + "/tone.ogg" });
        return;
      }
      callback(null, {});
    }
  }

  ConversationController {
    id: conversationController
    service: fakeService
    voiceController: controller
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }
}
