// Checks ConversationController's in-app photo viewer against a scripted
// service: opening a photo that is already downloaded shows it at once
// without asking the helper again; opening one that is not downloaded yet
// shows the viewer while it asks, and fills in the path once the helper
// answers; closing clears it; and stepping moves to the next or previous
// photo in the open conversation, skipping the video between them, and
// downloads a photo it steps to that is not here yet. Video and file
// media are never driven through here, since opening them for real calls
// Qt.openUrlExternally.
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
    // initialMessages is the page messages.list answers with, newest and
    // oldest mixed on purpose: insertIndex places each by its own time.
    property var initialMessages: [
      { id: "p1", conversationId: "c1", senderId: "s", senderName: "S", text: "[Photo]", outgoing: false, status: "received", created: 40, media: { kind: "photo", width: 400, height: 300, thumb: "" }, mediaPath: "/tmp/p1.jpg" },
      { id: "p2", conversationId: "c1", senderId: "s", senderName: "S", text: "[Photo]", outgoing: false, status: "received", created: 30, media: { kind: "photo", width: 200, height: 200, thumb: "" }, mediaPath: "" },
      { id: "v1", conversationId: "c1", senderId: "s", senderName: "S", text: "[Video]", outgoing: false, status: "received", created: 20, media: { kind: "video", width: 800, height: 600, duration: 5 }, mediaPath: "" },
      { id: "p3", conversationId: "c1", senderId: "s", senderName: "S", text: "[Photo]", outgoing: false, status: "received", created: 10, media: { kind: "photo", width: 100, height: 100, thumb: "" }, mediaPath: "" }
    ];
    // fetchRequests records every media.fetch call, by message id.
    property var fetchRequests: []
    // pendingFetch holds each media.fetch callback until the test answers it.
    property var pendingFetch: ({})

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
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // answerFetch resolves a held media.fetch call with a downloaded path.
  function answerFetch(id, path) {
    const callback = service.pendingFetch[id];
    delete service.pendingFetch[id];
    callback(null, { path: path });
  }

  function run(): void {
    controller.open({ id: "c1", title: "Group" });

    if (!root.checkAlreadyDownloadedPhotoOpensAtOnce()) return;
    if (!root.checkUndownloadedPhotoAsksAndFillsIn()) return;
    if (!root.checkCloseViewer()) return;
    if (!root.checkStepViewer()) return;

    console.log("PASS ConversationController");
    Qt.exit(0);
  }

  // checkAlreadyDownloadedPhotoOpensAtOnce verifies a photo with a path
  // already here opens in the in-app viewer without a new download.
  function checkAlreadyDownloadedPhotoOpensAtOnce(): bool {
    service.fetchRequests = [];
    controller.openMedia("p1");

    if (controller.viewerId !== "p1") return Check.fail("viewerId is " + controller.viewerId + ", want p1");
    if (!controller.viewerOpen) return Check.fail("viewerOpen is false for an open photo");
    if (controller.viewerPath !== "/tmp/p1.jpg") return Check.fail("viewerPath is " + controller.viewerPath + ", want /tmp/p1.jpg");
    if (!controller.viewerPhoto || controller.viewerPhoto.kind !== "photo") return Check.fail("viewerPhoto did not read back the photo's media");
    if (service.fetchRequests.length !== 0) return Check.fail("opening an already-downloaded photo asked the helper to fetch it again");
    return true;
  }

  // checkUndownloadedPhotoAsksAndFillsIn verifies a photo without a path
  // yet shows the viewer right away (so it can show the blurred preview)
  // and asks the helper for it, then fills in viewerPath once answered.
  function checkUndownloadedPhotoAsksAndFillsIn(): bool {
    service.fetchRequests = [];
    controller.openMedia("p2");

    if (controller.viewerId !== "p2") return Check.fail("viewerId is " + controller.viewerId + ", want p2");
    if (controller.viewerPath !== "") return Check.fail("viewerPath is " + controller.viewerPath + " before the download finished");
    if (service.fetchRequests.indexOf("p2") < 0) return Check.fail("opening an undownloaded photo did not ask the helper to fetch it");

    root.answerFetch("p2", "/tmp/p2.jpg");
    if (controller.viewerPath !== "/tmp/p2.jpg") return Check.fail("viewerPath did not fill in once the download finished");
    return true;
  }

  // checkCloseViewer verifies closing clears the viewer.
  function checkCloseViewer(): bool {
    controller.closeViewer();
    if (controller.viewerOpen) return Check.fail("closeViewer left the viewer open");
    if (controller.viewerId !== "") return Check.fail("closeViewer left viewerId as " + controller.viewerId);
    return true;
  }

  // checkStepViewer verifies stepping moves between the conversation's
  // photos: first to p2, already downloaded by the earlier check so it
  // needs no new fetch; then past the video to p3, downloading it since
  // it is not here yet; then back, ending where it started.
  function checkStepViewer(): bool {
    service.fetchRequests = [];
    controller.openMedia("p1");

    controller.stepViewer(-1);
    if (controller.viewerId !== "p2") return Check.fail("stepping to an older photo gave " + controller.viewerId + ", want p2");
    if (controller.viewerPath !== "/tmp/p2.jpg") return Check.fail("viewerPath for the already-downloaded p2 is " + controller.viewerPath);
    if (service.fetchRequests.length !== 0) return Check.fail("stepping to an already-downloaded photo asked the helper again");

    controller.stepViewer(-1);
    if (controller.viewerId !== "p3") return Check.fail("stepping again gave " + controller.viewerId + ", want p3 (skipping the video)");
    if (service.fetchRequests.indexOf("p3") < 0) return Check.fail("stepping to an undownloaded photo did not ask to fetch it");
    root.answerFetch("p3", "/tmp/p3.jpg");
    if (controller.viewerPath !== "/tmp/p3.jpg") return Check.fail("the stepped-to photo's path never filled in");

    controller.stepViewer(1);
    controller.stepViewer(1);
    if (controller.viewerId !== "p1") return Check.fail("stepping back twice gave " + controller.viewerId + ", want p1");

    controller.stepViewer(1);
    if (controller.viewerId !== "p1") return Check.fail("stepping past the newest photo moved away from it: " + controller.viewerId);

    return true;
  }
}
