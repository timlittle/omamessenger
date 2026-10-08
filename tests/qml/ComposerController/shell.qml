// Checks ComposerController.pasteImage against a scripted service whose
// media.paste reply is held back until the test fires it by hand: pasting
// an image answers asynchronously, so a paste started in one conversation
// must not attach, or fall back to a plain paste, in whatever conversation
// is open by the time the helper replies.
import QtQuick
import Quickshell
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  property int fallbackCount: 0

  // fakeService holds media.paste's callback rather than answering at
  // once, so the test controls exactly when the reply lands relative to
  // the conversation switching underneath it.
  QtObject {
    id: fakeService

    property var pending: null

    function request(method: string, params: var, callback: var): void {
      fakeService.pending = callback;
    }
  }

  // fakeConversation stands in for the ConversationController pasteImage
  // reads the open conversation's id from; nothing else about it matters
  // for this test.
  QtObject {
    id: fakeConversation

    property string activeId: "a"
  }

  ComposerController {
    id: composer
    service: fakeService
    conversation: fakeConversation

    onPasteFallbackRequested: root.fallbackCount++
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // takePending returns the service's held-back callback and clears it,
  // so each case starts from a clean slate.
  function takePending(): var {
    const callback = fakeService.pending;
    fakeService.pending = null;
    return callback;
  }

  // run drives every case in turn: a reply for the conversation that is
  // still open attaches normally; a reply for one that is not, whether
  // it succeeds or fails, is dropped outright.
  function run(): void {
    composer.pasteImage();
    const sameConversationReply = root.takePending();
    if (!sameConversationReply) {
      Check.fail("pasteImage did not call media.paste");
      return;
    }

    sameConversationReply(null, { path: "/tmp/same.png" });
    if (composer.attachmentPath !== "/tmp/same.png") {
      Check.fail("a reply for the conversation still open did not attach: " + composer.attachmentPath);
      return;
    }

    composer.attachmentPath = "";
    composer.pasteImage();
    const switchedBeforeSuccess = root.takePending();
    fakeConversation.activeId = "b";
    switchedBeforeSuccess(null, { path: "/tmp/leaked.png" });
    if (composer.attachmentPath !== "") {
      Check.fail("a media.paste reply for a closed conversation attached to the one now open: " + composer.attachmentPath);
      return;
    }

    composer.pasteImage();
    const switchedBeforeError = root.takePending();
    fakeConversation.activeId = "c";
    switchedBeforeError({ code: -1, message: "no image on the clipboard" }, null);
    if (root.fallbackCount !== 0) {
      Check.fail("a failed media.paste for a closed conversation fell back to a plain paste in the one now open");
      return;
    }

    composer.pasteImage();
    const sameConversationError = root.takePending();
    sameConversationError({ code: -1, message: "no image on the clipboard" }, null);
    if (root.fallbackCount !== 1) {
      Check.fail("a failed media.paste for the conversation still open did not fall back to a plain paste");
      return;
    }

    console.log("PASS ComposerController");
    Qt.exit(0);
  }
}
