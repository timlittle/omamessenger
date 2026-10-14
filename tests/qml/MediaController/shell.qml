// Checks MediaController's download-failure wiring against a scripted
// service: opening a video whose media.fetch fails with a reason that
// means the file is truly gone (not-found) reports a specific sentence
// naming the media's kind, rather than the helper's generic "something
// went wrong" text; a reason that is worth retrying (a plain timeout)
// keeps that generic text instead. The sentence's own wording is
// Format.mediaUnavailableMessage's job, proven exhaustively in
// tests/unit/format.test.cjs; this only checks the controller reaches it
// with the right media kind and reason.
//
// It also checks the fix for clicking a video or file repeatedly before
// it is ready: opening the same id twice before media.fetch answers
// sends only one fetch and opens the result only once, and opening an
// already-cached id twice back to back (the same way two clicks with no
// gap between them would reach this controller) opens it only once too.
// openExternally stands in for the real Qt.openUrlExternally throughout,
// so "opens" here means this test's own recorder, never a real external
// application.
import QtQuick
import Quickshell
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  // openCalls records every path controller.openExternally was asked to
  // open, standing in for the real Qt.openUrlExternally.
  property var openCalls: []

  QtObject {
    id: service

    property var uiState: ({})
    property var accounts: []
    // initialMessages seeds an undownloaded video and file for the
    // failure checks below, plus a second, already-cached video (v2)
    // for the repeated-click-on-cached-media check: a file, unlike a
    // photo or an available voice player, always falls through
    // openMedia's plain download-and-open path.
    property var initialMessages: [
      { id: "v1", conversationId: "c1", senderId: "s", senderName: "S", text: "[Video]", outgoing: false, status: "received", created: 30, media: { kind: "video", width: 800, height: 600, duration: 5 }, mediaPath: "" },
      { id: "v2", conversationId: "c1", senderId: "s", senderName: "S", text: "[Video]", outgoing: false, status: "received", created: 20, media: { kind: "video", width: 800, height: 600, duration: 5 }, mediaPath: "/tmp/v2.mp4" },
      { id: "f1", conversationId: "c1", senderId: "s", senderName: "S", text: "[File]", outgoing: false, status: "received", created: 10, media: { kind: "file", fileName: "notes.pdf" }, mediaPath: "" }
    ];
    // pendingFetch holds each media.fetch callback until the test answers it.
    property var pendingFetch: ({})
    // fetchRequests records every message id media.fetch was called for,
    // in order, so a click that should have been suppressed shows up as
    // an extra entry rather than silently passing.
    property var fetchRequests: []

    function request(method, params, callback) {
      if (method === "messages.list") { callback(null, { hasMore: false, messages: service.initialMessages }); return; }
      if (method === "media.fetch") {
        service.fetchRequests.push(params.messageId);
        service.pendingFetch[params.messageId] = callback;
        return;
      }
      callback(null, {});
    }
  }

  ConversationController {
    id: controller
    service: service
    openExternally: (path) => root.openCalls.push(path)
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

  // answerFetchSuccess resolves a held media.fetch call with path.
  function answerFetchSuccess(id, path) {
    const callback = service.pendingFetch[id];
    delete service.pendingFetch[id];
    callback(null, { path: path });
  }

  function run(): void {
    controller.open({ id: "c1", title: "Group" });

    if (!root.checkTrulyGoneReasonNamesTheKind()) return;
    if (!root.checkRetryableReasonKeepsTheGenericText()) return;
    if (!root.checkRapidClicksFetchAndOpenOnce()) return;
    if (!root.checkCachedMediaOpensOnceForBackToBackClicks()) return;

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

  // checkRapidClicksFetchAndOpenOnce verifies opening an undownloaded
  // video twice before its media.fetch answers sends only one fetch
  // request, and opens the result only once: the bug this fixes let an
  // impatient second click while a video was still downloading end up
  // opening a second player once the download finished.
  function checkRapidClicksFetchAndOpenOnce(): bool {
    root.openCalls = [];
    service.fetchRequests = [];

    controller.openMedia("v1");
    controller.openMedia("v1");

    if (service.fetchRequests.length !== 1) {
      return Check.fail("fetchRequests = " + JSON.stringify(service.fetchRequests) + ", want exactly one");
    }

    root.answerFetchSuccess("v1", "/tmp/v1.mp4");

    if (root.openCalls.length !== 1 || root.openCalls[0] !== "/tmp/v1.mp4") {
      return Check.fail("openCalls = " + JSON.stringify(root.openCalls) + ", want exactly one open of /tmp/v1.mp4");
    }

    // The download has finished and nothing is in flight any more, so a
    // later click opens it again rather than staying suppressed forever.
    controller.openMedia("v1");
    if (root.openCalls.length !== 2) {
      return Check.fail("a click after the first open finished did not open it again");
    }
    return true;
  }

  // checkCachedMediaOpensOnceForBackToBackClicks verifies opening an
  // already-downloaded video twice with nothing in between (the same
  // way two clicks with no gap would reach this controller) opens it
  // only once: before this fix, the already-cached path had no guard
  // at all, so every click opened a fresh player.
  function checkCachedMediaOpensOnceForBackToBackClicks(): bool {
    root.openCalls = [];

    controller.openMedia("v2");
    controller.openMedia("v2");

    if (root.openCalls.length !== 1 || root.openCalls[0] !== "/tmp/v2.mp4") {
      return Check.fail("openCalls = " + JSON.stringify(root.openCalls) + ", want exactly one open of /tmp/v2.mp4");
    }
    return true;
  }
}
