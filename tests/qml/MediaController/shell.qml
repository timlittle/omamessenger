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
// It also checks the fix for clicking a video or file repeatedly:
// opening the same id twice before media.fetch answers sends only one
// fetch and opens the result only once, and opening it again right
// after that open (or an already-cached id's, back to back) is ignored
// as a repeat rather than opening a second player — this is the
// cooldown MediaOpen.canOpen decides, proven exhaustively in
// tests/unit/mediaopen.test.cjs; this only checks the wiring: the
// cooldown is per message (a different message still opens at once)
// and it clears once mediaOpenCooldownMs has actually passed.
//
// Finally, it checks a race between a silent prefetch (ConversationController.
// fetchMedia, the same call a voice note's or photo's row makes the moment
// it becomes visible) and a deliberate open that follows before that
// prefetch has answered: the open must still open the file once the one
// shared fetch comes back, rather than being dropped because a fetch for
// that id was already in flight. This is the bug behind
// tests/qml/VoicePlayback's own intermittent failure - there the race
// shows up as the play button never flipping to pause, because a voice
// note's prefetch and its own open both call downloadMedia for the same id.
// openExternally stands in for the real Qt.openUrlExternally throughout,
// so "opens" here means this test's own recorder, never a real external
// application.
//
// Last, it checks that a video whose playback can actually work in the
// in-app viewer (a videoController reporting QtMultimedia available, or
// a real GIF file) opens into that viewer instead of falling through
// to the plain download-and-open path every other check here exercises,
// and that copying or saving a message's media sets a brief
// confirmation naming what happened, rather than doing it silently.
//
// It also checks a media.fetch result flagged risky opens the
// risky-open question instead of handing the file to openExternally,
// that cancelling leaves it unopened, and that clicking the same file
// again and confirming does open it, reusing the cached path and
// risky verdict rather than fetching it again.
import QtQuick
import Quickshell
import "ui/controllers"
import "Check.js" as Check

ShellRoot {
  id: root

  // openCalls records every path controller.openExternally was asked to
  // open, standing in for the real Qt.openUrlExternally.
  property var openCalls: []
  // copyCalls records every [path, mimeType] controller.copyToClipboard
  // was asked to copy, standing in for the real wl-copy.
  property var copyCalls: []

  QtObject {
    id: service

    property var uiState: ({})
    property var accounts: []
    // initialMessages seeds an undownloaded video and file for the
    // failure checks below, an already-cached video (v2) for the
    // repeated-click-on-cached-media check (a file, unlike a photo or
    // an available voice player, always falls through openMedia's plain
    // download-and-open path), and a third, never-clicked cached video
    // (v3) the cross-message cooldown check opens while v2 is on its
    // own cooldown, so that check never depends on another one's timing,
    // and a fourth, never-clicked undownloaded video (v4) the prefetch
    // race check uses, so it never depends on any id another check has
    // already fetched or cached, and a fifth, never-clicked undownloaded
    // video (v5) the sending service marked animated, for the auto-fetch
    // param check.
    property var initialMessages: [
      { id: "v1", conversationId: "c1", senderId: "s", senderName: "S", text: "[Video]", outgoing: false, status: "received", created: 30, media: { kind: "video", width: 800, height: 600, duration: 5 }, mediaPath: "" },
      { id: "v2", conversationId: "c1", senderId: "s", senderName: "S", text: "[Video]", outgoing: false, status: "received", created: 20, media: { kind: "video", width: 800, height: 600, duration: 5 }, mediaPath: "/tmp/v2.mp4" },
      { id: "f1", conversationId: "c1", senderId: "s", senderName: "S", text: "[File]", outgoing: false, status: "received", created: 10, media: { kind: "file", fileName: "notes.pdf" }, mediaPath: "" },
      { id: "v3", conversationId: "c1", senderId: "s", senderName: "S", text: "[Video]", outgoing: false, status: "received", created: 5, media: { kind: "video", width: 800, height: 600, duration: 5 }, mediaPath: "/tmp/v3.mp4" },
      { id: "v4", conversationId: "c1", senderId: "s", senderName: "S", text: "[Video]", outgoing: false, status: "received", created: 1, media: { kind: "video", width: 800, height: 600, duration: 5 }, mediaPath: "" },
      { id: "v5", conversationId: "c1", senderId: "s", senderName: "S", text: "[Video]", outgoing: false, status: "received", created: 0, media: { kind: "video", width: 800, height: 600, duration: 5, animated: true }, mediaPath: "" },
      // r1 is a never-clicked undownloaded file, for the risky-open
      // question check, which answers its media.fetch itself rather
      // than reusing another check's own fixture.
      { id: "r1", conversationId: "c1", senderId: "s", senderName: "S", text: "[File]", outgoing: false, status: "received", created: -1, media: { kind: "file", fileName: "invoice.html" }, mediaPath: "" }
    ];
    // pendingFetch holds each media.fetch callback until the test answers it.
    property var pendingFetch: ({})
    // fetchRequests records every message id media.fetch was called for,
    // in order, so a click that should have been suppressed shows up as
    // an extra entry rather than silently passing.
    property var fetchRequests: []
    // fetchParams records the full params object of the most recent
    // media.fetch call for each message id, so a check can confirm
    // whether auto was actually sent.
    property var fetchParams: ({})

    function request(method, params, callback) {
      if (method === "messages.list") { callback(null, { hasMore: false, messages: service.initialMessages }); return; }
      if (method === "media.fetch") {
        service.fetchRequests.push(params.messageId);
        service.fetchParams[params.messageId] = params;
        service.pendingFetch[params.messageId] = callback;
        return;
      }
      // A path deliberately outside any real home directory, so the
      // check below never depends on what $HOME happens to be on the
      // machine actually running this test.
      if (method === "media.save") { callback(null, { path: "/mnt/media-save-test/v2.mp4" }); return; }
      callback(null, {});
    }
  }

  ConversationController {
    id: controller
    service: service
    openExternally: (path) => root.openCalls.push(path)
    // A recorder, not the real wl-copy, the same reason openExternally
    // above is one.
    copyToClipboard: (path, mimeType) => root.copyCalls.push([path, mimeType])
    // Short enough that waiting it out in cooldownTimer below costs the
    // test run almost nothing, while still being a real, positive delay
    // rather than 0.
    mediaOpenCooldownMs: 80
  }

  // fakeVideoController stands in for the real VideoController's own
  // QtMultimedia probe, so checkOpenMediaShowsAPlayableVideoInTheViewer
  // can set it available without needing QtMultimedia to actually be
  // loaded.
  QtObject {
    id: fakeVideoController
    property bool available: true
  }

  // viewer is wired up only for that one check: every other check above
  // intentionally leaves photoViewer and videoController unset, so a
  // video with neither falls all the way through to openExternally, the
  // way it always did.
  PhotoViewerController {
    id: viewer
    conversation: controller
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // cooldownTimer waits past controller's cooldown before
  // checkCooldownClearsAfterItPasses re-opens v2, so the check proves
  // the cooldown actually expires rather than only that it blocks.
  Timer {
    id: cooldownTimer
    interval: 150
    onTriggered: root.finishAfterCooldown()
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

  // answerFetchSuccessRisky resolves a held media.fetch call with path,
  // flagged risky, the same as the helper does for active or
  // executable content (see app.MediaRisk and docs/decisions.md).
  function answerFetchSuccessRisky(id, path) {
    const callback = service.pendingFetch[id];
    delete service.pendingFetch[id];
    callback(null, { path: path, risky: true });
  }

  function run(): void {
    controller.open({ id: "c1", title: "Group" });

    if (!root.checkTrulyGoneReasonNamesTheKind()) return;
    if (!root.checkRetryableReasonKeepsTheGenericText()) return;
    if (!root.checkRapidClicksFetchAndOpenOnce()) return;
    if (!root.checkCachedMediaOpensOnceForBackToBackClicks()) return;
    if (!root.checkDifferentMessageStillOpensDuringAnothersCooldown()) return;
    if (!root.checkOpenDuringAnInFlightPrefetchStillOpensOnceAnswered()) return;

    cooldownTimer.start();
  }

  // finishAfterCooldown runs once cooldownTimer fires, after
  // mediaOpenCooldownMs has actually passed.
  function finishAfterCooldown(): void {
    if (!root.checkCooldownClearsAfterItPasses()) return;
    if (!root.checkOpenMediaShowsAPlayableVideoInTheViewer()) return;
    if (!root.checkCopyMediaShowsAConfirmation()) return;
    if (!root.checkSaveMediaShowsWhereItWentWithHomeFolded()) return;
    if (!root.checkFetchMediaSendsAutoForAnAnimatedVideo()) return;
    if (!root.checkFetchMediaOmitsAutoForAnOrdinaryFetch()) return;
    if (!root.checkRiskyMediaAsksBeforeOpeningAndRespectsTheAnswer()) return;

    console.log("PASS MediaController");
    Qt.exit(0);
  }

  // checkRiskyMediaAsksBeforeOpeningAndRespectsTheAnswer verifies a
  // media.fetch result flagged risky opens the risky-open question
  // instead of handing the file to openExternally; cancelling leaves it
  // unopened, and clicking again and confirming opens it, reusing the
  // cached path and risky verdict rather than fetching it again.
  function checkRiskyMediaAsksBeforeOpeningAndRespectsTheAnswer(): bool {
    root.openCalls = [];

    controller.openMedia("r1");
    root.answerFetchSuccessRisky("r1", "/tmp/r1.html");

    if (!controller.riskyOpenConfirmOpen)
      return Check.fail("riskyOpenConfirmOpen = false after a risky media.fetch result, want true");
    if (root.openCalls.length !== 0)
      return Check.fail("openCalls = " + JSON.stringify(root.openCalls) + ", want none before the question is answered");

    controller.cancelRiskyOpen();
    if (controller.riskyOpenConfirmOpen || root.openCalls.length !== 0)
      return Check.fail("cancelRiskyOpen left the question open, or opened the file anyway");

    // The same file, already cached and already known risky, asks
    // again on a second click rather than remembering a cancelled
    // answer; confirming this time actually opens it.
    controller.openMedia("r1");
    if (!controller.riskyOpenConfirmOpen)
      return Check.fail("a second click of the same risky file did not ask again");

    controller.confirmRiskyOpen();
    if (controller.riskyOpenConfirmOpen || root.openCalls.length !== 1 || root.openCalls[0] !== "/tmp/r1.html")
      return Check.fail("confirmRiskyOpen = " + JSON.stringify(root.openCalls) + ", want exactly one open of /tmp/r1.html");

    return true;
  }

  // checkFetchMediaSendsAutoForAnAnimatedVideo verifies the silent
  // fetch fetchMedia makes for a video the sending service marked
  // animated (v5, autoplaying like a silent "GIF") sends media.fetch's
  // auto param, so the helper applies its small, automatic-fetch size
  // cap rather than the far larger one a deliberate click is trusted
  // with (see media.fetch's auto param in docs/api.md).
  function checkFetchMediaSendsAutoForAnAnimatedVideo(): bool {
    controller.fetchMedia("v5");

    const params = service.fetchParams["v5"];
    if (!params || params.auto !== true)
      return Check.fail("params = " + JSON.stringify(params) + ", want auto:true for an animated video's own silent fetch");

    root.answerFetchSuccess("v5", "/tmp/v5.mp4");
    return true;
  }

  // checkFetchMediaOmitsAutoForAnOrdinaryFetch verifies the same silent
  // fetch for an ordinary file (f1, not a video the sender marked
  // animated) never sends auto at all, the same as every call before
  // media.fetch's auto param existed.
  function checkFetchMediaOmitsAutoForAnOrdinaryFetch(): bool {
    controller.fetchMedia("f1");

    const params = service.fetchParams["f1"];
    if (!params || params.auto)
      return Check.fail("params = " + JSON.stringify(params) + ", want no auto flag for an ordinary, non-animated fetch");

    root.answerFetchSuccess("f1", "/tmp/f1-notes.pdf");
    return true;
  }

  // checkCopyMediaShowsAConfirmation verifies copying v2's already-
  // cached video dispatches the real clipboard copy exactly once and
  // sets a brief confirmation, so Copy is never silent about having
  // done anything.
  function checkCopyMediaShowsAConfirmation(): bool {
    root.copyCalls = [];
    controller.timeline.lastError = "";
    controller.timeline.notice = "";

    controller.copyMedia("v2");

    if (root.copyCalls.length !== 1 || root.copyCalls[0][0] !== "/tmp/v2.mp4" || root.copyCalls[0][1] !== "video/mp4")
      return Check.fail("copyCalls = " + JSON.stringify(root.copyCalls) + ", want one copy of /tmp/v2.mp4 as video/mp4");
    if (controller.timeline.notice !== "Copied to clipboard")
      return Check.fail("notice = " + controller.timeline.notice + ", want \"Copied to clipboard\"");
    return true;
  }

  // checkSaveMediaShowsWhereItWentWithHomeFolded verifies saving v2
  // sets a confirmation naming where it ended up. The fixture's
  // media.save answers with a path outside any real home directory, so
  // this only checks the path reaches the notice unchanged; folding a
  // path actually under $HOME to "~" is Format.savedLocationLabel's own
  // job, proven exhaustively in tests/unit/format.test.cjs.
  function checkSaveMediaShowsWhereItWentWithHomeFolded(): bool {
    controller.timeline.notice = "";

    controller.saveMedia("v2");

    if (controller.timeline.notice !== "Saved to /mnt/media-save-test/v2.mp4")
      return Check.fail("notice = " + controller.timeline.notice + ", want \"Saved to /mnt/media-save-test/v2.mp4\"");
    return true;
  }

  // checkOpenMediaShowsAPlayableVideoInTheViewer verifies opening a
  // video whose playback can actually work in the in-app viewer (a
  // videoController reporting QtMultimedia available, here faked)
  // shows it there, through photoViewer.show, rather than falling
  // through to the plain download-and-open-externally path every check
  // above exercises. v2 already has a cached path from this file's own
  // fixtures, so this needs no new media.fetch.
  function checkOpenMediaShowsAPlayableVideoInTheViewer(): bool {
    root.openCalls = [];
    controller.videoController = fakeVideoController;
    controller.photoViewer = viewer;

    controller.openMedia("v2");

    if (viewer.viewerId !== "v2") return Check.fail("viewerId = " + viewer.viewerId + ", want v2 shown in the viewer");
    if (root.openCalls.length !== 0) return Check.fail("a playable video also opened externally: " + JSON.stringify(root.openCalls));

    viewer.close();
    controller.videoController = null;
    controller.photoViewer = null;
    return true;
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
  // request and opens the result only once, and that clicking it again
  // right after that open is ignored too: this is the issue the
  // cooldown fixes - repeatedly clicking a video kept opening more
  // players once the download finished.
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

    // A click right after the one that just opened it is still within
    // the cooldown, so it is ignored rather than opening a second player.
    controller.openMedia("v1");
    if (root.openCalls.length !== 1) {
      return Check.fail("a click within the cooldown opened the video again");
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

  // checkDifferentMessageStillOpensDuringAnothersCooldown verifies v2's
  // own cooldown (just started above) never blocks v3, a different,
  // never-clicked message: the cooldown is keyed by message id, not a
  // single global lock on "a media open just happened".
  function checkDifferentMessageStillOpensDuringAnothersCooldown(): bool {
    root.openCalls = [];

    controller.openMedia("v3");

    if (root.openCalls.length !== 1 || root.openCalls[0] !== "/tmp/v3.mp4") {
      return Check.fail("openCalls = " + JSON.stringify(root.openCalls) + ", want v3 to open while v2 is on cooldown");
    }
    return true;
  }

  // checkCooldownClearsAfterItPasses verifies that once
  // mediaOpenCooldownMs has actually passed, opening the same message
  // again opens it: the cooldown ignores a repeat click, it does not
  // lock the message out forever.
  function checkCooldownClearsAfterItPasses(): bool {
    root.openCalls = [];

    controller.openMedia("v2");

    if (root.openCalls.length !== 1 || root.openCalls[0] !== "/tmp/v2.mp4") {
      return Check.fail("openCalls = " + JSON.stringify(root.openCalls) + ", want v2 to open again once its cooldown passed");
    }
    return true;
  }

  // checkOpenDuringAnInFlightPrefetchStillOpensOnceAnswered verifies
  // that opening v4 while its own silent prefetch (controller.fetchMedia,
  // called the moment its row becomes visible) is still waiting on
  // media.fetch still opens the file once that one shared fetch answers,
  // rather than being dropped because downloadMedia saw a fetch for v4
  // already in flight and returned without ever queuing this open's own
  // callback.
  function checkOpenDuringAnInFlightPrefetchStillOpensOnceAnswered(): bool {
    root.openCalls = [];
    service.fetchRequests = [];

    controller.fetchMedia("v4");
    controller.openMedia("v4");

    if (service.fetchRequests.length !== 1) {
      return Check.fail("fetchRequests = " + JSON.stringify(service.fetchRequests) + ", want exactly one, not a second fetch while one is already in flight");
    }

    root.answerFetchSuccess("v4", "/tmp/v4.mp4");

    if (root.openCalls.length !== 1 || root.openCalls[0] !== "/tmp/v4.mp4") {
      return Check.fail("openCalls = " + JSON.stringify(root.openCalls) + ", want the open that arrived while the prefetch was in flight to open the file once answered");
    }
    return true;
  }
}
