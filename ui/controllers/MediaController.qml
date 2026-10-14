import QtQuick
import "../lib/Format.js" as Format
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

  // openExternally hands a local file to the user's own application.
  // The default calls the real Qt.openUrlExternally; a test overrides
  // it with a recorder, so opening a video or file can be proven to
  // dispatch exactly once without ever calling the real, user-visible
  // function.
  property var openExternally: (path) => Qt.openUrlExternally("file://" + path)

  // _fetching holds the ids of messages whose media is downloading, so a
  // photo already on its way is never asked for twice.
  property var _fetching: ({})

  // _opening holds the ids of messages whose already-downloaded video or
  // file a click just handed to openExternally, cleared on the next
  // turn of the event loop rather than at once: this is what makes two
  // opens reaching this function back to back, with nothing in between,
  // open it only once (see the issue this fixes - repeatedly clicking a
  // video opened it several times over). It is not a guard against a
  // second click once that turn has passed, since nothing tells this
  // controller when, or whether, the external player it handed the file
  // to ever closes; the download itself stays guarded for its whole
  // duration by _fetching above, which is the window the reported bug
  // actually clicked through.
  property var _opening: ({})

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
    if (path) { root._openCached(id, path); return; }

    root.downloadMedia(timeline, id, true, (downloaded) => root.openExternally(downloaded));
  }

  // _openCached hands an already-downloaded video or file to
  // openExternally, guarded by _opening so a second open reaching this
  // function before the event loop has moved on does nothing extra.
  function _openCached(id: string, path: string): void {
    if (root._opening[id]) return;

    root._opening[id] = true;
    root.openExternally(path);
    Qt.callLater(() => delete root._opening[id]);
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
  // to see its error text. A reason that means the file is truly gone
  // (not found, expired or undecryptable) gets its own sentence naming
  // the media's kind, such as "This video is no longer available.",
  // rather than the generic "something went wrong" every other helper
  // error shows. The photo viewer reuses this rather than asking the
  // helper itself, so there is one place that tracks an in-flight
  // download.
  function downloadMedia(timeline: var, id: string, report: bool, done: var): void {
    if (root._fetching[id]) return;

    root._fetching[id] = true;
    root.service.request("media.fetch", { messageId: id }, function(error, result) {
      delete root._fetching[id];
      if (error) {
        const reason = Rpc.errorReason(error);
        console.warn("media.fetch failed for " + id + ": code=" + error.code + " reason=" + (reason || "unknown"));
        timeline.setMediaFailed(id, true, reason);
        if (report) {
          const kind = Media.kindFor(timeline.media(id));
          timeline.lastError = Format.mediaUnavailableMessage(kind, reason) || Rpc.errorText(error);
        }
        return;
      }

      timeline.setMediaFailed(id, false, "");
      timeline.setMediaPath(id, result.path);
      timeline.lastError = "";
      done(result.path);
    });
  }
}
