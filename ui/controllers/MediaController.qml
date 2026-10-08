import QtQuick
import "../lib/Media.js" as Media
import "../lib/Rpc.js" as Rpc

// Downloads and opens a message's photo, video, file or voice note, for
// the open conversation. Split out of ConversationController to keep
// that file within the size guideline. Every function here takes the
// timeline to read and update as a parameter, rather than holding one
// itself, since ConversationController's timeline is reset for each
// conversation it opens.
//
// QtObject rather than Item: it holds no child objects.
QtObject {
  id: root

  // service is the Service instance that owns the helper connection.
  property var service: null

  // photoViewer is the PhotoViewerController a photo opens into.
  property var photoViewer: null

  // voiceController is the VoiceNoteController a voice note plays or
  // pauses through, when it is there and QtMultimedia loaded.
  property var voiceController: null

  // _fetching holds the ids of messages whose media is downloading, so a
  // photo already on its way is never asked for twice.
  property var _fetching: ({})

  // fetchMedia downloads a message's photo and shows it once it is here.
  // A photo asks each time its row is shown, so one already on its way is
  // not asked for twice; if the download fails the preview stays.
  function fetchMedia(timeline: var, id: string): void {
    root.downloadMedia(timeline, id, false, () => {});
  }

  // openMedia opens a message's photo, video, file or voice note. A photo
  // opens in the in-app viewer: Omarchy's window rule floats the external
  // image viewer small and keeps keyboard focus on this window, so its
  // close keys never reach it. A voice note plays or pauses in place
  // through voiceController, when it is there and QtMultimedia loaded;
  // otherwise it falls through to the same "open in your own application"
  // path a video or file already gets. Media.kindFor also catches a
  // voice note stored before the helper's "voice" media kind existed,
  // which still carries kind "file" forever (see Media.js), so it plays
  // in place too rather than opening externally.
  function openMedia(timeline: var, id: string): void {
    const media = timeline.media(id);
    const kind = Media.kindFor(media);
    if (kind === "photo") { if (root.photoViewer) root.photoViewer.show(id); return; }
    if (kind === "voice" && root.voiceController && root.voiceController.available) { root._toggleVoice(timeline, id, media); return; }

    const path = timeline.mediaPath(id);
    if (path) Qt.openUrlExternally("file://" + path);
    else root.downloadMedia(timeline, id, true, (downloaded) => Qt.openUrlExternally("file://" + downloaded));
  }

  // _toggleVoice plays or pauses a voice note, downloading it first if it
  // has not been fetched yet; voice notes are small, so this is quick and
  // happens the same way a photo's own missing full image would.
  function _toggleVoice(timeline: var, id: string, media: var): void {
    const path = timeline.mediaPath(id);
    if (path) { root.voiceController.toggle(id, path, (media.duration || 0) * 1000); return; }

    root.downloadMedia(timeline, id, true, (downloaded) => root.voiceController.toggle(id, downloaded, (media.duration || 0) * 1000));
  }

  // downloadMedia asks the helper for a message's media once, records
  // where it is, then runs done with the path. A failure is reported to
  // the user only when they asked for the media, rather than a photo
  // fetching itself, but is always recorded on the message, so a photo
  // with no preview shows "Photo unavailable" instead of an empty box
  // that a failed, silent auto-fetch would otherwise leave forever, and
  // always logged to the console with its code and safe reason
  // category, so a failed fetch is never silent even when nothing asked
  // to see its error text. The photo viewer reuses this rather than
  // asking the helper itself, so there is one place that tracks an
  // in-flight download.
  function downloadMedia(timeline: var, id: string, report: bool, done: var): void {
    if (root._fetching[id]) return;

    root._fetching[id] = true;
    root.service.request("media.fetch", { messageId: id }, function(error, result) {
      delete root._fetching[id];
      if (error) {
        const reason = Rpc.errorReason(error);
        console.warn("media.fetch failed for " + id + ": code=" + error.code + " reason=" + (reason || "unknown"));
        timeline.setMediaFailed(id, true, reason);
        if (report) timeline.lastError = Rpc.errorText(error);
        return;
      }

      timeline.setMediaFailed(id, false, "");
      timeline.setMediaPath(id, result.path);
      done(result.path);
    });
  }
}
