import QtQuick
import "../lib/Actions.js" as Actions

// Owns the in-app photo viewer: which message's photo is shown, stepping
// to a neighbouring one and closing, in or out to the user's own image
// viewer. conversation supplies the loaded timeline the viewer reads and
// the download it asks for; set it once, from whoever wires the
// controllers together.
//
// QtObject rather than Item: it holds no child objects.
QtObject {
  id: root

  // conversation is the ConversationController whose timeline this
  // controller shows photos from.
  property var conversation: null

  // viewerId is the message id of the photo shown in the in-app viewer,
  // or "" when it is closed.
  property string viewerId: ""

  // viewerOpen is whether the in-app photo viewer is showing, for the
  // Escape chain.
  readonly property bool viewerOpen: root.viewerId !== ""

  // viewerPhoto is the open photo's media (kind, width, height, thumb),
  // or null while the viewer is closed.
  readonly property var viewerPhoto: root.viewerId ? root.conversation.timeline.media(root.viewerId) : null

  // viewerPath is where the open photo's full image was downloaded, or
  // "" until that finishes.
  readonly property string viewerPath: root.viewerId ? root.conversation.timeline.mediaPath(root.viewerId) : ""

  // handles reports whether this controller owns action.
  function handles(action: string): bool {
    return Actions.owner(action) === "photoViewer";
  }

  // run performs action, the only entry point a key router needs.
  function run(action: string): void {
    const handlers = {
      "viewer.next": () => root.step(1),
      "viewer.prev": () => root.step(-1)
    };

    const handler = handlers[action];
    if (handler) handler();
  }

  // show displays id's photo in the in-app viewer, downloading it first
  // if it is not here yet.
  function show(id: string): void {
    root.viewerId = id;
    if (!root.conversation.timeline.mediaPath(id)) root.conversation.downloadMedia(id, true, () => {});
  }

  // close hides the in-app photo viewer.
  function close(): void {
    root.viewerId = "";
  }

  // openExternally opens the viewed photo in the user's own image viewer
  // and closes the in-app view, for anyone who wants that instead.
  function openExternally(): void {
    const path = root.viewerPath;
    root.close();
    if (path) Qt.openUrlExternally("file://" + path);
  }

  // step moves the in-app viewer to the next photo in the open
  // conversation: delta > 0 for a newer one, delta < 0 for an older one.
  // It does nothing when there isn't one.
  function step(delta: int): void {
    if (!root.viewerId) return;

    const next = root.conversation.timeline.photoNeighbor(root.viewerId, delta);
    if (next) root.show(next);
  }
}
