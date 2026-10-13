// Checks MediaController's download-failure wiring against a scripted
// service: opening a video whose media.fetch fails with a reason that
// means the file is truly gone (not-found) reports a specific sentence
// naming the media's kind, rather than the helper's generic "something
// went wrong" text; a reason that is worth retrying (a plain timeout)
// keeps that generic text instead. The sentence's own wording is
// Format.mediaUnavailableMessage's job, proven exhaustively in
// tests/unit/format.test.cjs; this only checks the controller reaches it
// with the right media kind and reason.
import QtQuick
import Quickshell
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  QtObject {
    id: service

    property var uiState: ({})
    property var accounts: []
    // initialMessages seeds one undownloaded video and one undownloaded
    // file, so a failure's reported sentence can be checked against two
    // media kinds; a file, unlike a photo or an available voice player,
    // always falls through openMedia's plain download-and-open path.
    property var initialMessages: [
      { id: "v1", conversationId: "c1", senderId: "s", senderName: "S", text: "[Video]", outgoing: false, status: "received", created: 20, media: { kind: "video", width: 800, height: 600, duration: 5 }, mediaPath: "" },
      { id: "f1", conversationId: "c1", senderId: "s", senderName: "S", text: "[File]", outgoing: false, status: "received", created: 10, media: { kind: "file", fileName: "notes.pdf" }, mediaPath: "" }
    ];
    // pendingFetch holds each media.fetch callback until the test answers it.
    property var pendingFetch: ({})

    function request(method, params, callback) {
      if (method === "messages.list") { callback(null, { hasMore: false, messages: service.initialMessages }); return; }
      if (method === "media.fetch") { service.pendingFetch[params.messageId] = callback; return; }
      callback(null, {});
    }
  }

  ConversationController {
    id: controller
    service: service
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // answerFetch resolves a held media.fetch call with error.
  function answerFetch(id, error) {
    const callback = service.pendingFetch[id];
    delete service.pendingFetch[id];
    callback(error, null);
  }

  function run(): void {
    controller.open({ id: "c1", title: "Group" });

    if (!root.checkTrulyGoneReasonNamesTheKind()) return;
    if (!root.checkRetryableReasonKeepsTheGenericText()) return;

    console.log("PASS MediaController");
    Qt.exit(0);
  }

  // checkTrulyGoneReasonNamesTheKind verifies opening a video that
  // fails with reason "not-found" reports the specific sentence naming
  // it as a video, not the helper's generic error text.
  function checkTrulyGoneReasonNamesTheKind(): bool {
    controller.openMedia("v1");
    root.answerFetch("v1", { code: -32603, message: "internal error", data: { reason: "not-found" } });

    if (controller.lastError !== "This video is no longer available.") {
      return Check.fail("lastError = " + controller.lastError + ", want the video-specific sentence");
    }
    return true;
  }

  // checkRetryableReasonKeepsTheGenericText verifies a reason worth
  // retrying (a timeout) still shows the helper's own generic sentence,
  // rather than claiming the file is gone for good.
  function checkRetryableReasonKeepsTheGenericText(): bool {
    controller.openMedia("f1");
    root.answerFetch("f1", { code: -32603, message: "internal error", data: { reason: "timeout" } });

    if (controller.lastError !== "Something went wrong in the helper. Try again.") {
      return Check.fail("lastError = " + controller.lastError + ", want the helper's generic sentence");
    }
    return true;
  }
}
