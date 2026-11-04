import QtQuick
import "../lib/Actions.js" as Actions

// Owns the in-app media viewer: which message's photo, video or GIF is
// shown, stepping to a neighbouring photo and closing, in or out to the
// user's own application. conversation supplies the loaded timeline the
// viewer reads and the download it asks for; set it once, from whoever
// wires the controllers together. Despite the name, it shows a video or
// a real GIF file too (see PhotoViewer.qml), since every property and
// function here - viewerPhoto, viewerPath, show, close, openExternally -
// already reads generically from the message's own media rather than
// anything specific to a photo; only step, which has no meaning for a
// video, says so explicitly.
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

  // viewerPlayPath is the open video's in-window play copy, or "" while
  // closed, showing a photo, or still waiting on one.
  readonly property string viewerPlayPath: root.viewerId ? root.conversation.timeline.mediaPlayPath(root.viewerId) : ""

  // viewerPreparing is whether the open video's play fetch is in flight.
  readonly property bool viewerPreparing: root.viewerId ? root.conversation.timeline.mediaPreparing(root.viewerId) : false

  // handles reports whether this controller owns action.
  function handles(action: string): bool {
    return Actions.owner(action) === "photoViewer";
  }

  // run performs action, the only entry point a key router needs.
  function run(action: string): void {
    const handlers = {
      "viewer.next": () => root.step(1),
      "viewer.prev": () => root.step(-1),
      "viewer.openExternal": () => root.openExternally()
    };

    const handler = handlers[action];
    if (handler) handler();
  }

  // show displays id's photo or video in the in-app viewer, downloading
  // it (and, for a video, its play copy) if it is not here yet. A video
  // claims the single decoding slot every video shares.
  function show(id: string): void {
    root.viewerId = id;
    if (root._isVideo(id)) {
      if (root.conversation.videoController) root.conversation.videoController.select(id);
      if (!root.conversation.timeline.mediaPlayKnown(id)) root.conversation.downloadMedia(id, true, true, () => {});
      return;
    }

    if (!root.conversation.timeline.mediaPath(id)) root.conversation.downloadMedia(id, true, false, () => {});
  }

  // close hides the in-app photo viewer, giving up the single decoding
  // slot if the video it was showing still held it. Asks unconditionally
  // rather than gating on id's current kind, since an edit may have
  // already swapped the media away from "video"; deselect no-ops if id
  // no longer holds the slot.
  function close(): void {
    const id = root.viewerId;
    root.viewerId = "";
    if (id && root.conversation.videoController) root.conversation.videoController.deselect(id);
  }

  // _isVideo reports whether id's own media is a video or GIF, for
  // show and close to know whether to touch the shared decoding slot
  // at all.
  function _isVideo(id: string): bool {
    const media = root.conversation.timeline.media(id);
    return !!media && media.kind === "video";
  }

  // openExternally opens the viewed photo, video or GIF in the user's
  // own application and closes the in-app view, for anyone who wants
  // that instead. Routed through conversation.openMediaExternal (see
  // MediaController._openIfAllowed) rather than calling
  // Qt.openUrlExternally itself, so the same cooldown and risky-open
  // question every other external open goes through still applies
  // here: the viewer offering its own, ungated shortcut around that
  // check was the exact gap a security review found.
  function openExternally(): void {
    const id = root.viewerId;
    root.close();
    if (id) root.conversation.openMediaExternal(id);
  }

  // step moves the in-app viewer to the next photo in the open
  // conversation: delta > 0 for a newer one, delta < 0 for an older one.
  // It does nothing when there isn't one, or while a video or GIF (not
  // a photo) is open, since photoNeighbor only ever looks for a photo
  // and jumping from a video to an unrelated photo would be surprising.
  function step(delta: int): void {
    if (!root.viewerId || !root.viewerPhoto || root.viewerPhoto.kind !== "photo") return;

    const next = root.conversation.timeline.photoNeighbor(root.viewerId, delta);
    if (next) root.show(next);
  }

  // _showable reports whether media is a kind this viewer may actually
  // show: a photo or a video, never a file, voice note or sticker.
  function _showable(media: var): bool {
    return !!media && (media.kind === "photo" || media.kind === "video");
  }

  // onViewerPhotoChanged closes the viewer once an edit swaps the open
  // message's media for a kind it may not show. Deferred with
  // Qt.callLater: closing synchronously here would be a binding loop,
  // since viewerPhoto's own binding is still being notified (see
  // docs/decisions.md).
  onViewerPhotoChanged: if (root.viewerId && !root._showable(root.viewerPhoto)) Qt.callLater(root.close)
}
