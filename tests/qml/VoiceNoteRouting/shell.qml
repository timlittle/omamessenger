// Checks that a voice note stored before the helper's "voice" media kind
// existed, which is permanently stuck with kind "file" and a file name
// like "voice-message.ogg" (see Media.js and ConversationController's
// openMedia), still renders and plays as a voice note rather than the
// image view: the bug this guards against showed a real voice note's
// audio file reaching PhotoView, which then failed to decode it as a
// photo. A real photo and a plain, non-audio file are checked the same
// way, so the fix narrows dispatch without breaking either.
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
    voiceController: fakeVoiceController
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
    if (!root.checkDelegate(legacyVoiceDelegate, "voiceNotePlayer", "a legacy voice note stored as a file")) return;
    if (!root.checkDelegate(photoDelegate, "photoView", "a real photo")) return;
    if (!root.checkDelegate(fileDelegate, "fileView", "a plain, non-audio file")) return;
    if (!root.checkControllerPlaysLegacyVoiceNote()) return;

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

  // checkControllerPlaysLegacyVoiceNote verifies ConversationController's
  // openMedia sends a legacy file-kind voice note to the voice player,
  // not to the "open in your own application" path a plain file gets.
  function checkControllerPlaysLegacyVoiceNote(): bool {
    controller.timeline.upsert({
      id: "legacy-voice", remoteId: "", senderId: "s1", senderName: "", text: "", outgoing: false,
      status: "received", created: 0, media: { kind: "file", fileName: "voice-message.ogg", duration: 9 }
    }, false);
    controller.timeline.setMediaPath("legacy-voice", "/tmp/voice-message.ogg");

    controller.openMedia("legacy-voice");

    if (fakeVoiceController.calls.length !== 1)
      return Check.fail("opening a legacy voice note did not play it through the voice controller");
    if (fakeVoiceController.calls[0].path !== "/tmp/voice-message.ogg")
      return Check.fail("voice controller got path \"" + fakeVoiceController.calls[0].path + "\", want \"/tmp/voice-message.ogg\"");
    return true;
  }
}
