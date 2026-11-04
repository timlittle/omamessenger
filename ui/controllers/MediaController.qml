import QtQuick
import Quickshell
import "../lib/Format.js" as Format
import "../lib/Media.js" as Media
import "../lib/MediaOpen.js" as MediaOpen
import "../lib/Rpc.js" as Rpc
import "../lib/Timeline.js" as Timeline

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

  // openExternally hands a local file to the user's own application via
  // the helper's files.open, which opens it through the desktop portal
  // rather than starting xdg-open with path as an argument (see
  // docs/decisions.md). A test overrides this with a recorder.
  property var openExternally: (path) => {
    if (!root.service) return;

    root.service.request("files.open", { path: path }, function(error) {
      if (error) console.warn("files.open failed: code=" + error.code);
    });
  }

  // cooldownMs is how long a message's video or file stays ignored after
  // it was last opened. Nothing tells this controller when, or whether,
  // the external player it handed a file to ever closes, so a click on
  // the same message again soon after reads as an impatient repeat of
  // the one that just opened it, not a second, deliberate one (see the
  // issue this fixes - repeatedly clicking a video kept opening more
  // players). A property, not a constant, so a test can shorten it.
  property int cooldownMs: 5000

  // _fetching holds, per message id whose media is downloading,
  // {play, waiters, rejoin}: play is whether the in-flight request
  // asked for a play copy; waiters are {report, done} pairs sharing
  // that request; rejoin are callers that wanted play while the
  // in-flight request did not, re-issued once it answers (see
  // downloadMedia).
  property var _fetching: ({})

  // _lastOpenedAt holds, per message id, when its video or file was last
  // handed to openExternally. MediaOpen.canOpen reads one message's own
  // entry to decide whether a new click opens it again or is ignored as
  // too soon; a different message's id is never consulted, so one
  // message's cooldown never blocks another's.
  property var _lastOpenedAt: ({})

  // _riskyVerdicts holds the helper's own risky verdict for each
  // message id this controller downloaded, kept here rather than only
  // on the message's row, which is gone once the list is cleared by a
  // chat switch or a reconnect while a download is still in flight.
  property var _riskyVerdicts: ({})

  // riskyConfirmOpen is true while a received file's "this can run
  // code" question is open, asking the user to confirm before it opens
  // externally, or to save it to Downloads instead (see
  // _askBeforeOpening and docs/decisions.md).
  property bool riskyConfirmOpen: false

  // _pendingRisky holds the timeline, message id and already-resolved
  // path riskyConfirmOpen's own question is about, until the user
  // answers it; an empty id means none is open.
  property var _pendingRisky: ({ timeline: null, id: "", path: "" })

  // fetchMedia downloads a message's photo, sticker or voice note and
  // shows it once it is here: the fetch a row's own visibility asks
  // for on its own, with no click from the user, so auto is always
  // true - the helper applies its small AutoFetchMaxBytes cap instead
  // of the far larger one a deliberate click is trusted with (see
  // media.fetch's auto param in docs/api.md). A row asks each time it
  // is shown, so one already on its way is not asked for twice; if
  // the download fails the preview stays, or, once it was refused
  // only for being over the auto cap, the view offers to load it on a
  // click instead (see retryMedia). A video, even one the sending
  // service marked to autoplay like a silent "GIF", never asks for
  // its file this way (see InlineVideo.qml) - only a click does,
  // through toggleVideo.
  function fetchMedia(timeline: var, id: string): void {
    root.downloadMedia(timeline, id, false, true, false, () => {});
  }

  // retryMedia downloads a message's photo, sticker or voice note
  // deliberately, bypassing the automatic-fetch cap fetchMedia applies:
  // the tap-to-load action offered once an automatic fetch was refused
  // only for being too large, never retried on its own.
  function retryMedia(timeline: var, id: string): void {
    root.downloadMedia(timeline, id, false, false, false, () => {});
  }

  // openMedia opens a message's photo, video, file, voice note or audio
  // message. A photo opens in the in-app viewer: Omarchy's window rule
  // floats the external image viewer small and keeps keyboard focus on
  // this window, so its close keys never reach it. A video or GIF
  // plays or pauses in the chat, the same as clicking it (see
  // toggleVideo); the full-size viewer is a click on one already
  // playing, and x opens it externally. A voice note or audio message
  // plays or pauses in place only once the helper has validated its
  // downloaded bytes (see _toggleVoice); otherwise it falls through to
  // the same "open in your own application" path a file gets.
  function openMedia(timeline: var, id: string): void {
    const media = timeline.media(id);
    const kind = Media.kindFor(media);
    if (kind === "photo") { if (root.photoViewer) root.photoViewer.show(id); return; }
    if (kind === "voice" || kind === "audio") { root._toggleVoice(timeline, id, media); return; }
    if (kind === "video" && root.videoController) { root.toggleVideo(timeline, id); return; }

    const path = timeline.mediaPath(id);
    if (path) { root._openIfAllowed(timeline, id, path); return; }

    root.downloadMedia(timeline, id, true, false, false, (downloaded) => root._openIfAllowed(timeline, id, downloaded));
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

    if (root._isRisky(timeline, id)) { root._askBeforeOpening(timeline, id, path); return; }

    root._lastOpenedAt[id] = now;
    root.openExternally(path);
  }

  // _isRisky reports whether id's media must be confirmed before it is
  // opened: the helper's verdict from this controller's own download
  // when there is one, otherwise the row's, and risky when neither is
  // known, since a verdict that could not be found is not one that
  // said safe.
  function _isRisky(timeline: var, id: string): bool {
    if (id in root._riskyVerdicts) return root._riskyVerdicts[id];

    return timeline.mediaRisky(id);
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

  // toggleVideo selects, deselects or pauses id as the current video,
  // asking for its in-window play copy first if not already known; a
  // video with no playPath opens externally instead.
  function toggleVideo(timeline: var, id: string): void {
    if (!root.videoController) return;

    if (timeline.mediaPlayKnown(id)) { root._selectOrOpenVideo(timeline, id); return; }

    root.downloadMedia(timeline, id, true, false, true, () => root._selectOrOpenVideo(timeline, id));
  }

  // _selectOrOpenVideo plays id in-window once a playPath exists,
  // otherwise opens the original externally.
  function _selectOrOpenVideo(timeline: var, id: string): void {
    if (timeline.mediaPlayPath(id)) { root.videoController.toggle(id); return; }

    const path = timeline.mediaPath(id);
    if (path) root._openIfAllowed(timeline, id, path);
  }

  // openExternal downloads id's media if need be and hands it straight
  // to the user's own application, bypassing the in-app photo viewer or
  // video player even when one is available: the explicit "open
  // externally" action, as opposed to openMedia's "do the obvious
  // thing" default.
  function openExternal(timeline: var, id: string): void {
    const path = timeline.mediaPath(id);
    if (path) { root._openIfAllowed(timeline, id, path); return; }

    root.downloadMedia(timeline, id, true, false, false, (downloaded) => root._openIfAllowed(timeline, id, downloaded));
  }

  // copyMedia asks the helper to copy a photo or video to the
  // clipboard (see media.copy in docs/api.md); does nothing for any
  // other kind (see Media.copyable). The helper runs wl-copy itself
  // with a fixed argv rather than this controller building a shell
  // command string, to keep sender-controlled text off a command line
  // (see docs/decisions.md).
  function copyMedia(timeline: var, id: string): void {
    if (!Media.copyable(timeline.media(id))) return;

    root.service.request("media.copy", { messageId: id }, function(error, result) {
      if (error) {
        const reason = Rpc.errorReason(error);
        console.warn("media.copy failed for " + id + ": code=" + error.code + " reason=" + (reason || "unknown"));
        timeline.lastError = Format.mediaUnavailableMessage(Media.kindFor(timeline.media(id)), reason) || Rpc.errorText(error);
        return;
      }

      timeline.lastError = "";
      timeline.notice = "Copied to clipboard";
    });
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

  // _toggleVoice plays or pauses a voice note or audio message, asking
  // for its in-window play copy first if not already known.
  function _toggleVoice(timeline: var, id: string, media: var): void {
    if (timeline.mediaPlayKnown(id)) { root._selectOrOpenVoice(timeline, id, media); return; }

    root.downloadMedia(timeline, id, true, false, true, () => root._selectOrOpenVoice(timeline, id, media));
  }

  // _selectOrOpenVoice plays id in place once a playPath exists and
  // QtMultimedia is loaded; otherwise falls back to the external-open
  // path. The decision rests on the helper's own transcoded output,
  // never the sender's claimed file name or kind (see docs/decisions.md).
  function _selectOrOpenVoice(timeline: var, id: string, media: var): void {
    const playPath = timeline.mediaPlayPath(id);
    if (root.voiceController && root.voiceController.available && playPath) {
      root.voiceController.toggle(id, playPath, (media.duration || 0) * 1000);
      return;
    }

    const path = timeline.mediaPath(id);
    if (path) root._openIfAllowed(timeline, id, path);
  }

  // downloadMedia asks the helper for a message's media once, records
  // where it is, then runs done with the path. A second call for the
  // same id while one is in flight joins it rather than sending
  // another; if the new call asks for play and the in-flight one did
  // not, it is queued in _rejoinAsPlay and re-asked once that request
  // answers, since a download-only answer never carries a playPath
  // (see docs/decisions.md). auto requests the helper's smaller
  // automatic-fetch size cap; play asks the helper to also prepare the
  // in-window play copy. An edit can swap id's media while the request
  // is in flight, so the answer is checked against current media
  // before being kept (see _answerIsStale).
  function downloadMedia(timeline: var, id: string, report: bool, auto: bool, play: bool, done: var): void {
    const inFlight = root._fetching[id];
    if (inFlight) {
      if (play && !inFlight.play) {
        timeline.setMediaPreparing(id, true);
        inFlight.rejoin.push({ report: report, auto: auto, done: done });
        return;
      }
      inFlight.waiters.push({ report: report, done: done });
      return;
    }

    root._fetching[id] = { play: play, waiters: [{ report: report, done: done }], rejoin: [] };
    if (play) timeline.setMediaPreparing(id, true);

    // Recorded before the request, not read back from timeline once
    // the answer arrives: an edit can swap id's media for a different
    // kind, or different bytes of the same kind, while media.fetch is
    // still in flight (see _answerIsStale and docs/decisions.md).
    const expected = Timeline.mediaSignature(timeline.media(id));
    const params = { messageId: id };
    if (auto) params.auto = true;
    if (play) params.play = true;

    root.service.request("media.fetch", params, (error, result) =>
      root._handleFetchAnswer(timeline, id, play, expected, error, result));
  }

  // _handleFetchAnswer applies the media.fetch answer for id: records
  // it, notifies waiters, and re-asks for play on behalf of anything
  // queued in _rejoinAsPlay. Runs exactly once per request.
  function _handleFetchAnswer(timeline: var, id: string, play: bool, expected: var, error: var, result: var): void {
    const entry = root._fetching[id];
    delete root._fetching[id];
    if (play) timeline.setMediaPreparing(id, false);

    if (error) {
      root._failDownload(timeline, id, error, entry.waiters);
      root._rejoinAsPlay(timeline, id, entry.rejoin);
      return;
    }
    if (root._answerIsStale(timeline, id, expected, play, result)) {
      root._rejoinAsPlay(timeline, id, entry.rejoin);
      return;
    }

    timeline.setMediaFailed(id, false, "");
    // Must be set before setMediaPath: PhotoView's onPathChanged reads
    // verifiedWidth the instant path changes.
    root._applyVerifiedSize(timeline, id, result);
    timeline.setMediaPath(id, result.path);
    if (play) timeline.setMediaPlayPath(id, result.playPath || "");
    root._riskyVerdicts[id] = !!result.risky;
    timeline.setMediaRisky(id, !!result.risky);
    timeline.lastError = "";
    for (const waiter of entry.waiters) waiter.done(result.path);
    root._rejoinAsPlay(timeline, id, entry.rejoin);
  }

  // _rejoinAsPlay re-issues downloadMedia with play requested, for
  // every waiter that was queued instead of joining a download-only
  // fetch.
  function _rejoinAsPlay(timeline: var, id: string, rejoin: var): void {
    for (const waiter of rejoin) {
      root.downloadMedia(timeline, id, waiter.report, waiter.auto, true, waiter.done);
    }
  }

  // _answerIsStale reports whether id's media no longer matches
  // expected (recorded before the request) or the kind/size the
  // answer actually served (see _answerMatchesCurrentMedia), meaning
  // an edit swapped the media while the request was in flight. A
  // stale answer is dropped and a fresh fetch started rather than
  // written back, since an unchecked file could reach a view that
  // trusts it (a photo's Image with no verified size).
  function _answerIsStale(timeline: var, id: string, expected: var, play: bool, result: var): bool {
    const current = timeline.media(id);
    if (!current) return false;

    const signatureMatches = Timeline.mediaSignaturesMatch(expected, Timeline.mediaSignature(current));
    if (signatureMatches && root._answerMatchesCurrentMedia(current, result)) return false;

    delete root._riskyVerdicts[id];
    console.warn("media.fetch answered after message " + id + " changed media; refetching");
    root.downloadMedia(timeline, id, false, true, play, () => {});
    return true;
  }

  // _answerMatchesCurrentMedia reports whether result was served for
  // the same media current names, by comparing result.kind and
  // result.mediaSize. Both fields are optional; an older helper that
  // omits them is treated as matching.
  function _answerMatchesCurrentMedia(current: var, result: var): bool {
    if (result.kind === undefined) return true;
    if (result.kind !== current.kind) return false;

    return result.mediaSize === undefined || current.size === undefined || result.mediaSize === current.size;
  }

  // _applyVerifiedSize records result's decoder-confirmed width and
  // height as verifiedWidth/verifiedHeight, kept separate from the
  // sender's own width/height claim so an unrelated edit never reads
  // this as a changed attachment (see ui/lib/Timeline.js and
  // docs/decisions.md).
  function _applyVerifiedSize(timeline: var, id: string, result: var): void {
    if (!(result.width > 0) || !(result.height > 0)) return;

    const media = timeline.media(id);
    if (!media) return;

    timeline.setMedia(id, Object.assign({}, media, { verifiedWidth: result.width, verifiedHeight: result.height }));
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
