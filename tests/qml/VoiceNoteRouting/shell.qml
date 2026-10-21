// Checks that media dispatch and opening never guess a decoder from a
// sender's claimed file name: a message stored before the helper's
// "voice" media kind existed, permanently stuck with kind "file" and a
// file name like "voice-message.ogg" (see Media.js), now renders as a
// plain file row and opens only through the gated external-open path,
// never the in-window voice player - the fix for a reviewed bug where a
// sender's own name alone decided which decoder a bubble's audio
// reached. A real photo, a real voice note and a plain, non-audio file
// are checked the same way, so the fix narrows dispatch without
// breaking any of them.
import QtQuick
import Quickshell
import "ui/components"
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  readonly property var noAnnotation: ({ showDay: false, dayLabel: "", showSender: false, groupedWithOlder: false })

  QtObject {
    id: fakeVoiceController

    property bool available: true
    property var calls: []

    function toggle(id, path, durationHintMs) {
      fakeVoiceController.calls.push({ id: id, path: path, durationHintMs: durationHintMs });
    }
  }

  ConversationController {
    id: controller

    property var _openCalls: []

    voiceController: fakeVoiceController
    openExternally: (path) => controller._openCalls.push(path)
  }

  MessageDelegate {
    id: legacyVoiceDelegate
    width: 400
    annotation: root.noAnnotation
    message: ({
      id: "legacy-voice", outgoing: false, status: "received", created: 0, senderName: "", senderId: "s1",
      text: "", media: { kind: "file", fileName: "voice-message.ogg", duration: 9 },
      mediaPath: "/tmp/legacy-voice-message.ogg"
    })
  }

  MessageDelegate {
    id: voiceDelegate
    width: 400
    annotation: root.noAnnotation
    message: ({
      id: "voice", outgoing: false, status: "received", created: 0, senderName: "", senderId: "s1",
      text: "", media: { kind: "voice", duration: 9 }
    })
  }

  MessageDelegate {
    id: photoDelegate
    width: 400
    annotation: root.noAnnotation
    message: ({
      id: "photo", outgoing: false, status: "received", created: 0, senderName: "", senderId: "s1",
      text: "", media: { kind: "photo", width: 100, height: 100 }
    })
  }

  MessageDelegate {
    id: fileDelegate
    width: 400
    annotation: root.noAnnotation
    message: ({
      id: "file", outgoing: false, status: "received", created: 0, senderName: "", senderId: "s1",
      text: "", media: { kind: "file", fileName: "report.pdf" }
    })
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // run checks both the view dispatch and the controller's own routing.
  function run(): void {
    if (!root.checkDelegate(legacyVoiceDelegate, "fileView", "a legacy voice note stored as a file")) return;
    if (!root.checkDelegate(voiceDelegate, "voiceNotePlayer", "a real voice note")) return;
    if (!root.checkDelegate(photoDelegate, "photoView", "a real photo")) return;
    if (!root.checkDelegate(fileDelegate, "fileView", "a plain, non-audio file")) return;
    if (!root.checkControllerOpensLegacyFileExternally()) return;
    if (!root.checkPausedNoteShowsPlayGlyph()) return;

    console.log("PASS VoiceNoteRouting");
    Qt.exit(0);
  }

  // checkDelegate verifies that exactly one of photoView, fileView and
  // voiceNotePlayer is visible for delegate, and that it is want.
  function checkDelegate(delegate: var, want: string, label: string): bool {
    const views = { photoView: Check.find(delegate, "photoView"), fileView: Check.find(delegate, "fileView"), voiceNotePlayer: Check.find(delegate, "voiceNotePlayer") };
    for (const name in views) {
      const shouldShow = name === want;
      if (views[name].visible !== shouldShow)
        return Check.fail(label + ": " + name + ".visible is " + views[name].visible + ", want " + shouldShow);
    }
    // The decode error this guards against came from an invisible
    // PhotoView still being handed the shared mediaPath; it must stay
    // unset for anything that is not the photo or video view.
    if (want !== "photoView" && views.photoView.path !== "")
      return Check.fail(label + ": the hidden photo view still got a path to decode: " + views.photoView.path);
    return true;
  }

  // checkControllerOpensLegacyFileExternally verifies
  // ConversationController's openMedia sends a legacy file-kind "voice
  // note" through the same gated external-open path a plain file gets,
  // never the in-window voice player: the sender's own claimed file
  // name must never choose a decoder (see docs/decisions.md).
  function checkControllerOpensLegacyFileExternally(): bool {
    controller.timeline.upsert({
      id: "legacy-voice", remoteId: "", senderId: "s1", senderName: "", text: "", outgoing: false,
      status: "received", created: 0, media: { kind: "file", fileName: "voice-message.ogg", duration: 9 }
    }, false);
    controller.timeline.setMediaPath("legacy-voice", "/tmp/voice-message.ogg");

    controller.openMedia("legacy-voice");

    if (fakeVoiceController.calls.length !== 0)
      return Check.fail("a legacy file-kind \"voice note\" was played through the voice controller, guessed from its file name");
    if (controller._openCalls.length !== 1 || controller._openCalls[0] !== "/tmp/voice-message.ogg")
      return Check.fail("opening it did not reach the gated external-open path: " + JSON.stringify(controller._openCalls));
    return true;
  }

  // checkPausedNoteShowsPlayGlyph verifies a message bubble reads the
  // real playing flag from voiceNotes, not just whether this note is
  // the one loaded (playingId): the bug this guards against showed the
  // pause glyph forever once a note had played once, because the
  // bubble treated "loaded" and "playing" as the same thing, so pausing
  // a note never visibly flipped its button back.
  function checkPausedNoteShowsPlayGlyph(): bool {
    voiceDelegate.voiceNotes = { available: true, playingId: "voice", playing: false, positionMs: 0, durationMs: 0 };

    let button = Check.find(voiceDelegate, "voicePlayButton");
    if (button.text !== "▶")
      return Check.fail("a loaded but paused note shows \"" + button.text + "\", want the play glyph");

    voiceDelegate.voiceNotes = { available: true, playingId: "voice", playing: true, positionMs: 0, durationMs: 0 };

    button = Check.find(voiceDelegate, "voicePlayButton");
    if (button.text !== "⏸")
      return Check.fail("a loaded, playing note shows \"" + button.text + "\", want the pause glyph");
    return true;
  }
}
