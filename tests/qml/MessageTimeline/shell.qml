// Checks MessageTimeline against a scripted service: a message that
// arrives as an event takes its place by time, older history at the bottom
// and new messages at the top; a page that arrives after older history
// already came in as events still puts its newer messages at the top; a
// page that repeats a message already shown does not show it twice; media()
// reads a loaded message's photo or video back; photoNeighbor() steps
// between the photos in the loaded history, skipping messages without one;
// ids() lists the loaded messages for moving a highlight; initialLoaded()
// fires once loadInitial's page has landed; loadOlder()'s reply is
// dropped, not applied, once its guard says the conversation it was for
// is no longer the one open; a flood of live messages past
// Timeline.MAX_LOADED_ROWS windows out the oldest loaded rows rather than
// growing without bound; and a protected id (the highlighted message)
// stays loaded through that same windowing - the wiring this test checks,
// not windowNewest's own decision, already proven in tests/unit/timeline.test.cjs.
import QtQuick
import Quickshell
import "ui/controllers"
import "ui/lib/Timeline.js" as Timeline
import "Check.js" as Check

ShellRoot {
  id: root

  QtObject {
    id: service

    property var firstPage: null

    // request holds the first page until the test answers it, and answers
    // the older page at once.
    function request(method: string, params: var, callback: var): void {
      if (!params.before) {
        service.firstPage = callback;
        return;
      }
      callback(null, { hasMore: false, messages: [root.message("m10", 10), root.message("m20", 20)] });
    }
  }

  property int initialLoadedCount: 0

  MessageTimeline {
    id: timeline
    onInitialLoaded: root.initialLoadedCount++
  }

  // staleService defers every request until the test fires it by hand,
  // to reproduce a loadOlder reply arriving after its conversation is no
  // longer the one open: scrolling up in one conversation, then switching
  // to another before the older page answers.
  QtObject {
    id: staleService

    property var pending: []

    function request(method: string, params: var, callback: var): void {
      staleService.pending.push(callback);
    }
  }

  // staleActiveId is whichever conversation is "open" for staleTimeline's
  // guards to check against, moved by hand to simulate switching.
  property string staleActiveId: "a"

  // windowTimeline is its own, empty MessageTimeline for the windowing
  // and id-stability checks below, so flooding it with thousands of
  // messages never disturbs timeline's own, already-asserted state.
  MessageTimeline {
    id: windowTimeline
  }

  MessageTimeline {
    id: staleTimeline
  }

  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // message is a minimal message with id and created time.
  function message(id: string, created: int): var {
    return { id: id, conversationId: "chat", senderId: "s", senderName: "S", text: id, outgoing: false, status: "received", created: created };
  }

  // photoMessage is a minimal message carrying a photo.
  function photoMessage(id: string, created: int): var {
    return Object.assign(root.message(id, created), { media: { kind: "photo", width: 400, height: 300, thumb: "" } });
  }

  // ids lists the timeline's message ids, newest first.
  function ids(): string {
    return root.idsOf(timeline);
  }

  // idsOf lists tl's message ids, newest first, for a test that needs a
  // second MessageTimeline of its own.
  function idsOf(tl: var): string {
    const out = [];
    for (let i = 0; i < tl.model.count; i++) out.push(tl.model.get(i).id);
    return out.join(",");
  }

  // run loads a page, receives an old and a new message, then the older page.
  function run(): void {
    timeline.loadInitial(service, "chat", () => true, false);
    timeline.upsert(root.message("m20", 20), false);
    service.firstPage(null, { hasMore: true, messages: [root.message("m50", 50), root.message("m60", 60)] });
    if (root.initialLoadedCount !== 1) {
      Check.fail("initialLoaded fired " + root.initialLoadedCount + " times, want 1");
      return;
    }
    timeline.upsert(root.message("m70", 70), false);
    if (root.ids() !== "m70,m60,m50,m20") {
      Check.fail("messages not placed by time: " + root.ids());
      return;
    }
    if (timeline.ids().join(",") !== "m70,m60,m50,m20") {
      Check.fail("ids() did not match the loaded messages: " + timeline.ids().join(","));
      return;
    }

    timeline.loadOlder(service, "chat", () => true, false);
    if (root.ids() !== "m70,m60,m50,m20,m10") {
      Check.fail("older page duplicated or misplaced a message: " + root.ids());
      return;
    }
    if (root.initialLoadedCount !== 1) {
      Check.fail("loadOlder fired initialLoaded, want only loadInitial to");
      return;
    }

    timeline.remove("m50", false);
    if (root.ids() !== "m70,m60,m20,m10") {
      Check.fail("remove did not drop the message: " + root.ids());
      return;
    }

    timeline.remove("m50", false);
    if (root.ids() !== "m70,m60,m20,m10") {
      Check.fail("removing an id twice changed the model: " + root.ids());
      return;
    }

    if (timeline.newestId() !== "m70") {
      Check.fail("newestId = " + timeline.newestId() + ", want m70");
      return;
    }

    const got = timeline.messageById("m60");
    if (!got || got.text !== "m60") {
      Check.fail("messageById did not return the loaded message");
      return;
    }
    if (timeline.messageById("missing") !== null) {
      Check.fail("messageById did not return null for an unloaded id");
      return;
    }

    timeline.upsert(Object.assign(root.message("m80", 80), { remoteId: "r80" }), false);
    if (timeline.localIdForRemote("r80") !== "m80") {
      Check.fail("localIdForRemote did not find the message by its remote id");
      return;
    }
    if (timeline.localIdForRemote("missing") !== "") {
      Check.fail("localIdForRemote did not return \"\" for an unknown remote id");
      return;
    }

    timeline.upsert(root.photoMessage("m65", 65), false);
    timeline.upsert(root.photoMessage("m15", 15), false);
    if (root.ids() !== "m80,m70,m65,m60,m20,m15,m10") {
      Check.fail("photo messages were not placed by time: " + root.ids());
      return;
    }

    if (timeline.media("m70") !== null) {
      Check.fail("media() returned a photo for a message that has none");
      return;
    }
    if (!timeline.media("m65") || timeline.media("m65").kind !== "photo") {
      Check.fail("media() did not read back m65's photo");
      return;
    }

    if (timeline.photoNeighbor("m65", 1) !== "") {
      Check.fail("photoNeighbor found a newer photo that does not exist: " + timeline.photoNeighbor("m65", 1));
      return;
    }
    if (timeline.photoNeighbor("m65", -1) !== "m15") {
      Check.fail("photoNeighbor(-1) from m65 gave " + timeline.photoNeighbor("m65", -1) + ", want m15");
      return;
    }
    if (timeline.photoNeighbor("m15", 1) !== "m65") {
      Check.fail("photoNeighbor(1) from m15 gave " + timeline.photoNeighbor("m15", 1) + ", want m65");
      return;
    }
    if (timeline.photoNeighbor("unknown", 1) !== "") {
      Check.fail("photoNeighbor for an id that is not loaded should be \"\"");
      return;
    }

    // setReactions updates an existing row, not a freshly inserted one,
    // which is exactly the case a QML ListModel role holding an array of
    // objects loses silently; row() and reactions() keep it as JSON to
    // avoid that.
    timeline.setReactions("m60", [{ emoji: "👍", count: 1, mine: true }]);
    const reactions = Timeline.reactions(timeline.find("m60"));
    if (reactions.length !== 1 || reactions[0].emoji !== "👍") {
      Check.fail("setReactions did not stick: " + JSON.stringify(reactions));
      return;
    }

    if (!root.checkUpsertDropsStaleMediaPathOnRetire()) return;
    if (!root.checkUpsertDropsMediaOnIdentityChange()) return;
    if (!root.checkUpsertKeepsVerifiedDimensionsAcrossUnrelatedUpdate()) return;
    if (!root.checkWindowingCapsLoadedRows()) return;
    if (!root.checkWindowingProtectsHighlightedId()) return;
    root.checkLoadOlderDropsStaleReply();
  }

  // checkWindowingCapsLoadedRows floods windowTimeline with more live
  // messages than Timeline.MAX_LOADED_ROWS, one at a time through
  // upsert() exactly as a real message.added event would, and confirms
  // the model never grows past the cap, keeps the newest message on
  // top, and comes back asking for more history: the oldest rows were
  // windowed out, not lost, so scrolling back can still re-page them
  // from the helper (see MessageTimeline.upsert). Returns false, having
  // already called Check.fail, on the first thing that does not hold.
  function checkWindowingCapsLoadedRows(): bool {
    const cap = Timeline.MAX_LOADED_ROWS;
    for (let i = 0; i < cap + 50; i++) {
      windowTimeline.upsert(root.message("w" + i, i), false, "");
    }

    if (windowTimeline.model.count !== cap) {
      return Check.fail("flooding past the cap left " + windowTimeline.model.count + " rows loaded, want " + cap);
    }
    if (windowTimeline.newestId() !== "w" + (cap + 49)) {
      return Check.fail("windowing lost the newest message: newestId() = " + windowTimeline.newestId());
    }
    if (!windowTimeline.hasMore) {
      return Check.fail("windowing out loaded rows must leave hasMore true, so scrolling back re-pages them");
    }
    if (windowTimeline.find("w0") !== null) {
      return Check.fail("the oldest message should have been windowed out of the model");
    }

    return true;
  }

  // checkWindowingProtectsHighlightedId reproduces a user's highlight
  // sitting on an old message while the chat keeps receiving live
  // messages in the background: windowing must never evict the
  // protected id, so the highlight never silently points at a message
  // that is no longer loaded (qml.md: rows identified by id, never an
  // index). Returns false, having already called Check.fail, on the
  // first thing that does not hold.
  function checkWindowingProtectsHighlightedId(): bool {
    const loadedIds = windowTimeline.ids();
    const protect = loadedIds[loadedIds.length - 1]; // the oldest row still loaded, and so the first windowing would otherwise drop
    const cap = Timeline.MAX_LOADED_ROWS;
    const before = windowTimeline.model.count;

    for (let i = 0; i < 50; i++) {
      windowTimeline.upsert(root.message("p" + i, cap + 1000 + i), false, protect);
    }

    if (windowTimeline.find(protect) === null) {
      return Check.fail("windowing evicted the protected id " + protect + " it was told to keep");
    }
    if (windowTimeline.model.count !== before + 50) {
      return Check.fail("protecting the oldest loaded row should keep every one of the 50 new messages too: count = " + windowTimeline.model.count + ", want " + (before + 50));
    }

    return true;
  }

  // checkUpsertDropsStaleMediaPathOnRetire reproduces the helper
  // retiring a sent photo's attachment: upsert must drop the cached
  // mediaPath (resolved while the message was still pending, to its
  // now-removed outgoing copy) the moment the message's own update
  // first confirms delivery, so the bubble resolves it again instead of
  // pointing at a file that is gone forever. A later update once
  // already confirmed, or one for a message without media, must leave
  // whatever mediaPath is already there alone. Returns false, having
  // already called Check.fail, on the first thing that does not hold.
  function checkUpsertDropsStaleMediaPathOnRetire(): bool {
    timeline.upsert(Object.assign(root.photoMessage("mp1", 90), { outgoing: true, status: "pending" }), false);
    timeline.setMediaPath("mp1", "/data/omamessenger/media/outgoing/mp1-photo.jpg");

    timeline.upsert(Object.assign(root.photoMessage("mp1", 90), { outgoing: true, status: "sent" }), false);
    if (timeline.mediaPath("mp1") !== "") {
      return Check.fail("upsert kept a sent photo's stale outgoing mediaPath instead of dropping it: " + timeline.mediaPath("mp1"));
    }

    timeline.setMediaPath("mp1", "/data/omamessenger/media/mp1.jpg");
    timeline.upsert(Object.assign(root.photoMessage("mp1", 90), { outgoing: true, status: "delivered" }), false);
    if (timeline.mediaPath("mp1") !== "/data/omamessenger/media/mp1.jpg") {
      return Check.fail("upsert dropped an already-confirmed photo's resolved mediaPath on a later status update: " + timeline.mediaPath("mp1"));
    }

    timeline.upsert(Object.assign(root.message("mt1", 91), { outgoing: true, status: "pending" }), false);
    timeline.setMediaPath("mt1", "/data/omamessenger/media/outgoing/mt1.txt");
    timeline.upsert(Object.assign(root.message("mt1", 91), { outgoing: true, status: "sent" }), false);
    if (timeline.mediaPath("mt1") !== "/data/omamessenger/media/outgoing/mt1.txt") {
      return Check.fail("upsert dropped mediaPath for a message with no media: " + timeline.mediaPath("mt1"));
    }

    return true;
  }

  // checkUpsertDropsMediaOnIdentityChange reproduces a service's own
  // end-to-end encrypted edit swapping a message's media for a
  // different kind, or for different bytes of the same kind: upsert
  // must drop the cached mediaPath and every verdict either way, since
  // they describe the bytes the edit just replaced, never the ones now
  // shown. A caption-only edit that leaves the same media alone must
  // keep whatever mediaPath is already there. Returns false, having
  // already called Check.fail, on the first thing that does not hold.
  function checkUpsertDropsMediaOnIdentityChange(): bool {
    timeline.upsert(Object.assign(root.message("me1", 95), { media: { kind: "voice", fileName: "note.ogg", duration: 5 } }), false);
    timeline.setMediaPath("me1", "/data/omamessenger/media/me1.ogg");
    timeline.setMediaPlayable("me1", true);

    timeline.upsert(Object.assign(root.message("me1", 95), { edited: true, media: { kind: "photo", fileName: "note.ogg", width: 800, height: 600 } }), false);
    if (timeline.mediaPath("me1") !== "") {
      return Check.fail("upsert kept a stale mediaPath across an edit that changed the media kind: " + timeline.mediaPath("me1"));
    }
    if (timeline.mediaVerdictsKnown("me1")) {
      return Check.fail("upsert kept stale media verdicts across an edit that changed the media kind");
    }

    timeline.upsert(root.photoMessage("me2", 96), false);
    timeline.setMediaPath("me2", "/data/omamessenger/media/me2.jpg");

    timeline.upsert(Object.assign(root.photoMessage("me2", 96), { edited: true, text: "new caption" }), false);
    if (timeline.mediaPath("me2") !== "/data/omamessenger/media/me2.jpg") {
      return Check.fail("upsert dropped mediaPath for a caption-only edit that left the same media alone: " + timeline.mediaPath("me2"));
    }

    return true;
  }

  // checkUpsertKeepsVerifiedDimensionsAcrossUnrelatedUpdate reproduces
  // the gap docs/decisions.md described: MediaController.setMedia
  // records media.fetch's own verified size as verifiedWidth/
  // verifiedHeight on a loaded photo (see ui/lib/ImageBounds.js); an
  // unrelated message.updated echo - a reaction, in this test - must
  // not lose that verified size, since the attachment itself has not
  // changed, even though the service's own echo never carries these
  // UI-only fields at all. An edit that swaps in different media must
  // still clear them, exactly as it already clears mediaPath. Returns
  // false, having already called Check.fail, on the first thing that
  // does not hold.
  function checkUpsertKeepsVerifiedDimensionsAcrossUnrelatedUpdate(): bool {
    timeline.upsert(root.photoMessage("mv1", 97), false);
    timeline.setMedia("mv1", Object.assign({}, timeline.media("mv1"), { verifiedWidth: 390, verifiedHeight: 290 }));

    timeline.upsert(Object.assign(root.photoMessage("mv1", 97), { reactions: [{ emoji: "👍", count: 1, mine: true }] }), false);
    const afterReaction = timeline.media("mv1");
    if (!afterReaction || afterReaction.verifiedWidth !== 390 || afterReaction.verifiedHeight !== 290) {
      return Check.fail("upsert lost verified dimensions across an unrelated reaction update: " + JSON.stringify(afterReaction));
    }

    timeline.upsert(Object.assign(root.message("mv1", 97), { edited: true, media: { kind: "photo", width: 10, height: 10, fileName: "different.jpg" } }), false);
    const afterEdit = timeline.media("mv1");
    if (!afterEdit || afterEdit.verifiedWidth || afterEdit.verifiedHeight) {
      return Check.fail("upsert kept stale verified dimensions across an edit that changed the media: " + JSON.stringify(afterEdit));
    }

    return true;
  }

  // checkLoadOlderDropsStaleReply reproduces scrolling up in conversation
  // "a", switching to conversation "b" before the older page answers, and
  // then that stale page finally arriving: it must not splice "a"'s
  // messages into "b"'s timeline, or overwrite hasMore/historyUnavailable
  // with "a"'s values, the same staleness guard loadInitial already has.
  function checkLoadOlderDropsStaleReply(): void {
    staleTimeline.loadInitial(staleService, "a", () => "a" === root.staleActiveId, false);
    staleService.pending.shift()(null, { hasMore: true, messages: [root.message("a1", 1)] });

    staleTimeline.loadOlder(staleService, "a", () => "a" === root.staleActiveId, false);
    const staleReply = staleService.pending.shift();

    root.staleActiveId = "b";
    staleTimeline.loadInitial(staleService, "b", () => "b" === root.staleActiveId, false);
    staleService.pending.shift()(null, { hasMore: false, messages: [root.message("b1", 1)] });

    const idsBeforeStaleReply = root.idsOf(staleTimeline);
    staleReply(null, { hasMore: true, historyUnavailable: true, messages: [root.message("a0", 0)] });

    if (root.idsOf(staleTimeline) !== idsBeforeStaleReply) {
      Check.fail("a stale loadOlder reply for a closed conversation changed the messages of the one now open: " + root.idsOf(staleTimeline));
      return;
    }
    if (staleTimeline.hasMore !== false) {
      Check.fail("a stale loadOlder reply overwrote hasMore for the conversation actually open");
      return;
    }
    if (staleTimeline.historyUnavailable !== false) {
      Check.fail("a stale loadOlder reply overwrote historyUnavailable for the conversation actually open");
      return;
    }

    console.log("PASS MessageTimeline");
    Qt.exit(0);
  }
}
