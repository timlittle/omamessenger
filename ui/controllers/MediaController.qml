import QtQuick
import Quickshell
import qs.Commons
import "../lib/Format.js" as Format
import "../lib/Links.js" as Links
import "../lib/Media.js" as Media
import "../lib/MediaOpen.js" as MediaOpen
import "../lib/Rpc.js" as Rpc

// Downloads and opens a message's photo, video, file or voice note, for
// the open conversation, asking first when the helper named a file
// risky (see riskyConfirmOpen) rather than handing it to the user's own
// application unconfirmed. Split out of ConversationController to keep
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

  // videoController is the VideoController a video selects, deselects
  // or mutes through, when it is there.
  property var videoController: null

  // copyToClipboard puts path's content on the system clipboard as
  // mimeType, by shelling out to wl-copy the same way Omarchy's own
  // shell copies a value: Quickshell's own clipboard property only
  // carries plain text, with nothing for an image or a video.
  // Util.execDetached runs its argument as a single command string
  // under "bash -lc", not an argv list (see Omarchy's own
  // Commons/Util.qml); passing it a list silently failed to copy
  // anything at all, since the whole list was coerced to one
  // comma-joined, unexecutable word. The default calls the real
  // wl-copy; a test overrides it with a recorder, so copying a photo
  // or video can be proven to dispatch exactly once without ever
  // touching the real, user-visible clipboard.
  property var copyToClipboard: (path, mimeType) => Util.execDetached("wl-copy --type " + Util.shellQuote(mimeType) + " < " + Util.shellQuote(path))

  // openExternally hands a local file to the user's own application.
  // The default calls the real Qt.openUrlExternally; a test overrides
  // it with a recorder, so opening a video or file can be proven to
  // dispatch exactly once without ever calling the real, user-visible
  // function.
  property var openExternally: (path) => Qt.openUrlExternally(Links.fileURL(path))

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

  // riskyConfirmOpen is true while a received file's "this can run
  // code" question is open, asking the user to confirm before it opens
  // externally, or to save it to Downloads instead (see
  // _askBeforeOpening and docs/decisions.md).
  property bool riskyConfirmOpen: false

  // _pendingRisky holds the timeline, message id and already-resolved
  // path riskyConfirmOpen's own question is about, until the user
  // answers it; an empty id means none is open.
  property var _pendingRisky: ({ timeline: null, id: "", path: "" })

  // fetchMedia downloads a message's photo and shows it once it is here.
  // A photo asks each time its row is shown, so one already on its way is
  // not asked for twice; if the download fails the preview stays. auto
  // is true only for a video the sending service marked to autoplay
  // like a silent "GIF" (see Media.isAnimated): the one case this
  // silent, no-click fetch is for a kind large enough that the helper's
  // own auto cap matters (see media.fetch's auto param in
  // docs/api.md). A photo or sticker's own silent fetch is already
  // bounded by the helper's image profile regardless, so this is never
  // true for those.
  function fetchMedia(timeline: var, id: string): void {
    root.downloadMedia(timeline, id, false, Media.isAnimated(timeline.media(id)), () => {});
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
    if (kind === "video" && root._playableInViewer(timeline, id) && root.photoViewer) { root.photoViewer.show(id); return; }

    const path = timeline.mediaPath(id);
    if (path) { root._openIfAllowed(timeline, id, path); return; }

    root.downloadMedia(timeline, id, true, false, (downloaded) => root._openIfAllowed(timeline, id, downloaded));
  }

  // _playableInViewer reports whether the in-app viewer (see
  // photoViewer, which shows a video or GIF too) can actually play
  // media: a real GIF file, confirmed only by the helper's own
  // decodeAs verdict once downloaded (see Media.decodesAsGif), always
  // can, through AnimatedImage; an ordinary video, or one a service
  // marked to play back like one, needs videoController's own
  // QtMultimedia probe to have succeeded. Without either, opening
  // falls through to the plain download-and-open-externally path
  // below instead, exactly as it did before the viewer could show a
  // video at all.
  function _playableInViewer(timeline: var, id: string): bool {
    return Media.decodesAsGif(timeline.mediaDecodeAs(id)) || (!!root.videoController && root.videoController.available);
  }

  // _openIfAllowed hands path to openExternally unless this same
  // message was already opened within cooldownMs, in which case the
  // click is ignored as a repeat rather than a second, deliberate open,
  // or unless the helper named id's own media risky (see
  // MessageTimeline.mediaRisky and docs/decisions.md), in which case it
  // asks first rather than opening anything.
  function _openIfAllowed(timeline: var, id: string, path: string): void {
    const now = Date.now();
    if (!MediaOpen.canOpen(root._lastOpenedAt[id], now, root.cooldownMs)) return;

    if (timeline.mediaRisky(id)) { root._askBeforeOpening(timeline, id, path); return; }

    root._lastOpenedAt[id] = now;
    root.openExternally(path);
  }

  // _askBeforeOpening opens the risky-open question for id, remembering
  // what confirming, cancelling or saving to Downloads instead must act
  // on once the user answers.
  function _askBeforeOpening(timeline: var, id: string, path: string): void {
    root._pendingRisky = { timeline: timeline, id: id, path: path };
    root.riskyConfirmOpen = true;
  }

  // confirmRiskyOpen opens the file the risky-open question is about,
  // the deliberate "open anyway" answer, bypassing the cooldown check
  // _openIfAllowed already applied before asking: this is the same
  // click still being answered, not a new one.
  function confirmRiskyOpen(): void {
    const pending = root._pendingRisky;
    root._closeRiskyConfirm();
    if (!pending.id) return;

    root._lastOpenedAt[pending.id] = Date.now();
    root.openExternally(pending.path);
  }

  // cancelRiskyOpen leaves the risky-open question's file unopened.
  function cancelRiskyOpen(): void {
    root._closeRiskyConfirm();
  }

  // saveRiskyToDownloads saves the risky-open question's file into the
  // downloads directory instead of opening it, the safe default.
  function saveRiskyToDownloads(): void {
    const pending = root._pendingRisky;
    root._closeRiskyConfirm();
    if (pending.id) root.saveMedia(pending.timeline, pending.id);
  }

  // _closeRiskyConfirm closes the risky-open question and forgets which
  // file it was about.
  function _closeRiskyConfirm(): void {
    root.riskyConfirmOpen = false;
    root._pendingRisky = { timeline: null, id: "", path: "" };
  }

  // toggleVideo selects, deselects or pauses id as the one video
  // playing right now, downloading it first if it is not on this
  // machine yet; does nothing with no videoController wired (an older
  // caller that never set one up).
  function toggleVideo(timeline: var, id: string): void {
    if (!root.videoController) return;

    const path = timeline.mediaPath(id);
    if (path) { root.videoController.toggle(id); return; }

    root.downloadMedia(timeline, id, true, false, () => root.videoController.toggle(id));
  }

  // openExternal downloads id's media if need be and hands it straight
  // to the user's own application, bypassing the in-app photo viewer or
  // video player even when one is available: the explicit "open
  // externally" action, as opposed to openMedia's "do the obvious
  // thing" default.
  function openExternal(timeline: var, id: string): void {
    const path = timeline.mediaPath(id);
    if (path) { root._openIfAllowed(timeline, id, path); return; }

    root.downloadMedia(timeline, id, true, false, (downloaded) => root._openIfAllowed(timeline, id, downloaded));
  }

  // copyMedia downloads id's photo or video if need be and copies it to
  // the clipboard; does nothing for any other media kind, which has
  // nothing sensible to copy as (see Media.mimeFor).
  function copyMedia(timeline: var, id: string): void {
    const mimeType = Media.mimeFor(timeline.media(id));
    if (!mimeType) return;

    const path = timeline.mediaPath(id);
    if (path) { root._copyAndNotify(timeline, path, mimeType); return; }

    root.downloadMedia(timeline, id, true, false, (downloaded) => root._copyAndNotify(timeline, downloaded, mimeType));
  }

  // _copyAndNotify copies path to the clipboard and shows a brief
  // confirmation, so Copy is never silent about having done anything.
  function _copyAndNotify(timeline: var, path: string, mimeType: string): void {
    root.copyToClipboard(path, mimeType);
    timeline.notice = "Copied to clipboard";
  }

  // saveMedia asks the helper to save id's media into the downloads
  // directory, fetching it first if need be; the helper does both in
  // one round trip (see media.save in docs/api.md), so this never asks
  // downloadMedia itself. A failure is reported the same way a failed
  // media.fetch already is; success shows a brief confirmation naming
  // where the file ended up, with the user's own home directory folded
  // to "~" (Format.js is pure and never reads the environment itself,
  // so this reads it through Quickshell.env, the same way Service.qml
  // already does for keys.conf's own path).
  function saveMedia(timeline: var, id: string): void {
    root.service.request("media.save", { messageId: id }, function(error, result) {
      if (error) {
        const reason = Rpc.errorReason(error);
        console.warn("media.save failed for " + id + ": code=" + error.code + " reason=" + (reason || "unknown"));
        timeline.lastError = Format.mediaUnavailableMessage(Media.kindFor(timeline.media(id)), reason) || Rpc.errorText(error);
        return;
      }

      timeline.lastError = "";
      timeline.notice = Format.savedLocationLabel(result.path, Quickshell.env("HOME") || "");
    });
  }

  // _toggleVoice plays or pauses a voice note, downloading it first if it
  // has not been fetched yet; voice notes are small, so this is quick and
  // happens the same way a photo's own missing full image would.
  function _toggleVoice(timeline: var, id: string, media: var): void {
    const path = timeline.mediaPath(id);
    if (path) { root.voiceController.toggle(id, path, (media.duration || 0) * 1000); return; }

    root.downloadMedia(timeline, id, true, false, (downloaded) => root.voiceController.toggle(id, downloaded, (media.duration || 0) * 1000));
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
  // there is one place that tracks an in-flight download. auto is sent
  // to the helper only for the call that actually starts the request
  // (a joiner's own auto is never consulted): true means the helper
  // applies its small, automatic-fetch size cap instead of the far
  // larger one a deliberate click is trusted with (see media.fetch's
  // auto param in docs/api.md).
  function downloadMedia(timeline: var, id: string, report: bool, auto: bool, done: var): void {
    if (root._fetching[id]) { root._fetching[id].push({ report: report, done: done }); return; }

    root._fetching[id] = [{ report: report, done: done }];
    const params = auto ? { messageId: id, auto: true } : { messageId: id };
    root.service.request("media.fetch", params, function(error, result) {
      const waiting = root._fetching[id];
      delete root._fetching[id];

      if (error) { root._failDownload(timeline, id, error, waiting); return; }

      timeline.setMediaFailed(id, false, "");
      timeline.setMediaPath(id, result.path);
      timeline.setMediaDecodeAs(id, result.decodeAs || "");
      timeline.setMediaRisky(id, !!result.risky);
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
