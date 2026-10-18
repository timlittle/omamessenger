import QtQuick
import "../lib/Format.js" as Format
import "../lib/Media.js" as Media
import "../lib/MediaOpen.js" as MediaOpen
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

  // cooldownMs is how long a message's video or file stays ignored after
  // it was last opened. Nothing tells this controller when, or whether,
  // the external player it handed a file to ever closes, so a click on
  // the same message again soon after reads as an impatient repeat of
  // the one that just opened it, not a second, deliberate one (see the
  // issue this fixes - repeatedly clicking a video kept opening more
  // players). A property, not a constant, so a test can shorten it.
  property int cooldownMs: 5000

  // _fetching holds, per message id whose media is downloading, every
  // caller still waiting on it: {report, done} pairs, so a photo's own
  // silent prefetch and a deliberate open that follows before it answers
  // share the one request instead of sending a second, and both still
  // hear back once it answers, rather than the second caller's own
  // report flag and done callback being dropped silently (see
  // tests/qml/MediaController's prefetch-race check).
  property var _fetching: ({})

  // _lastOpenedAt holds, per message id, when its video or file was last
  // handed to openExternally. MediaOpen.canOpen reads one message's own
  // entry to decide whether a new click opens it again or is ignored as
  // too soon; a different message's id is never consulted, so one
  // message's cooldown never blocks another's.
  property var _lastOpenedAt: ({})

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
    if (path) { root._openIfAllowed(id, path); return; }

    root.downloadMedia(timeline, id, true, (downloaded) => root._openIfAllowed(id, downloaded));
  }

  // _openIfAllowed hands path to openExternally unless this same
  // message was already opened within cooldownMs, in which case the
  // click is ignored as a repeat rather than a second, deliberate open.
  function _openIfAllowed(id: string, path: string): void {
    const now = Date.now();
    if (!MediaOpen.canOpen(root._lastOpenedAt[id], now, root.cooldownMs)) return;

    root._lastOpenedAt[id] = now;
    root.openExternally(path);
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
  // where it is, then runs done with the path. A second call for the
  // same id while the first is still waiting on media.fetch joins that
  // one request instead of sending another, and is still answered once
  // it comes back: this is what lets a photo's or voice note's own
  // silent prefetch (report false, done a no-op) and a deliberate open
  // that follows right after it (report true, done something that
  // matters) both get what they asked for from the one round trip. A
  // failure is reported to the user only for a caller that asked for it,
  // but is always recorded on the message, so a photo with no preview
  // shows "Photo unavailable" instead of an empty box that a failed,
  // silent auto-fetch would otherwise leave forever, and always logged
  // to the console with its code and safe reason category, so a failed
  // fetch is never silent even when nothing asked to see its error
  // text. A reason that means the file is truly gone (not found,
  // expired or undecryptable) gets its own sentence naming the media's
  // kind, such as "This video is no longer available.", rather than the
  // generic "something went wrong" every other helper error shows. The
  // photo viewer reuses this rather than asking the helper itself, so
  // there is one place that tracks an in-flight download.
  function downloadMedia(timeline: var, id: string, report: bool, done: var): void {
    if (root._fetching[id]) { root._fetching[id].push({ report: report, done: done }); return; }

    root._fetching[id] = [{ report: report, done: done }];
    root.service.request("media.fetch", { messageId: id }, function(error, result) {
      const waiting = root._fetching[id];
      delete root._fetching[id];

      if (error) { root._failDownload(timeline, id, error, waiting); return; }

      timeline.setMediaFailed(id, false, "");
      timeline.setMediaPath(id, result.path);
      timeline.lastError = "";
      for (const waiter of waiting) waiter.done(result.path);
    });
  }

  // _failDownload records a failed media.fetch on the message and tells
  // every waiter that asked to be told (report true) a user-facing
  // reason, once for the whole shared request rather than once per
  // waiter, so two callers failing together do not overwrite each
  // other's sentence with the same text twice.
  function _failDownload(timeline: var, id: string, error: var, waiting: var): void {
    const reason = Rpc.errorReason(error);
    console.warn("media.fetch failed for " + id + ": code=" + error.code + " reason=" + (reason || "unknown"));
    timeline.setMediaFailed(id, true, reason);

    if (!waiting.some(waiter => waiter.report)) return;

    const kind = Media.kindFor(timeline.media(id));
    timeline.lastError = Format.mediaUnavailableMessage(kind, reason) || Rpc.errorText(error);
  }
}
