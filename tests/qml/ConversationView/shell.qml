// Checks ConversationView: the empty state with no conversation, that an
// unverified title - self-chosen, like a bare push name - shows marked
// with a leading "~" and a direct chat's own phone number beside it,
// while a verified title shows unmarked with no phone number, loadOlder
// firing once scrolled to the oldest loaded message, a submitted composer
// message reaching the send signal with its reply id, replyTarget copied
// into the composer, a delegate's reply signals relayed outward, j/k and
// paging clamping at the real newest/oldest ends of the message list, and
// the "phone offline" note showing only while historyUnavailable is set,
// and a blocked conversation disabling the composer and reaching its own
// blocked flag.
import QtQuick
import Quickshell
import "ui/components"
import "ui/lib/Timeline.js" as Timeline
import "Check.js" as Check

ShellRoot {
  id: root

  property var sent: []
  property var sentReplyIds: []
  property int loadOlderCount: 0
  property int pollAttempts: 0

  // buildMessages returns count messages, newest first, one minute apart.
  function buildMessages(count) {
    const now = Date.now();
    const list = [];
    for (let i = 0; i < count; i++) {
      list.push({
        id: "m" + i, senderId: "s1", senderName: "Alex", text: "message " + i,
        outgoing: i % 2 === 0, status: "delivered", created: now - i * 60000
      });
    }
    return list;
  }

  FloatingWindow {
    implicitWidth: 400
    implicitHeight: 300
    visible: true

    ConversationView {
      id: view
      anchors.fill: parent
      onLoadOlder: root.loadOlderCount += 1
      onSend: (text, replyToId) => { root.sent.push(text); root.sentReplyIds.push(replyToId); }
    }
  }

  // pollTimer waits for the view's own content-height bindings to settle
  // after scrollToOldest(), since a 30-item ListView lays itself out over
  // several frames rather than in the one that calls it.
  Timer {
    id: pollTimer
    interval: 25
    repeat: true
    onTriggered: root.pollLoadOlder()
  }

  // scheduledPollTimer waits for a newly queued message's header row to
  // lay itself out before checkScheduledMessageStaysInView looks at it.
  Timer {
    id: scheduledPollTimer
    interval: 25
    repeat: true
    onTriggered: root.pollScheduledHeader()
  }

  // Checks run once Quickshell has finished loading; Qt.exit() is ignored
  // before then.
  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // run drives the view through each scenario and checks the outcome.
  function run(): void {
    if (!root.checkEmptyState()) return;
    if (!root.checkHeaderMarksAnUnverifiedTitle()) return;
    root.startLoadOlderCheck();
  }

  // checkHeaderMarksAnUnverifiedTitle verifies the header shows an
  // unverified title - self-chosen, like a bare WhatsApp push name -
  // marked with a leading "~", with a direct chat's own phone number
  // added to the subtitle line when the service gave one; a verified
  // title shows unmarked, with no phone number, even when one is
  // given (nothing to add once the title is already trusted); see
  // docs/decisions.md.
  function checkHeaderMarksAnUnverifiedTitle(): bool {
    view.subtitle = "";
    view.conversation = { id: "cu1", accountId: "a1", service: "whatsapp", remoteId: "ru1", kind: "direct", title: "Mum", titleVerified: false, phone: "+15551234567" };

    const title = Check.find(view, "conversationTitle");
    if (!title || title.text !== "~Mum") return Check.fail("unverified header title = " + (title && title.text) + ", want ~Mum");
    const subtitle = Check.find(view, "conversationSubtitle");
    if (!subtitle || subtitle.text !== "+15551234567") return Check.fail("unverified header subtitle = " + (subtitle && subtitle.text) + ", want the phone number");

    view.conversation = { id: "cu2", accountId: "a1", service: "whatsapp", remoteId: "ru2", kind: "direct", title: "Priya", titleVerified: true, phone: "+15559876543" };
    if (title.text !== "Priya") return Check.fail("verified header title = " + title.text + ", want unmarked Priya");
    if (subtitle.text !== "") return Check.fail("verified header subtitle = \"" + subtitle.text + "\", want empty: nothing to add once trusted");

    return true;
  }

  // checkEmptyState verifies the placeholder shows when no chat is open.
  function checkEmptyState(): bool {
    const nodes = Check.texts(view);
    const empty = nodes.find(node => node.text === "Pick a chat · j/k to move · Enter to open");
    if (!empty || !empty.visible) return Check.fail("empty state not shown for a null conversation");
    return true;
  }

  // startLoadOlderCheck opens a 30-message conversation, scrolls to the
  // oldest loaded message and starts polling for loadOlder().
  function startLoadOlderCheck(): void {
    const now = Date.now();
    const messages = root.buildMessages(30);

    view.conversation = { id: "c1", accountId: "a1", service: "whatsapp", remoteId: "r1", kind: "direct", title: "Alex", titleVerified: true, members: 0, preview: "", muted: false, unread: 0, lastActivity: now };
    view.messages = messages;
    view.annotations = Timeline.annotate(messages, false, now);

    root.loadOlderCount = 0;
    root.pollAttempts = 0;
    view.scrollToOldest();
    pollTimer.start();
  }

  // pollLoadOlder checks whether loadOlder() has fired yet, giving up
  // after four seconds.
  function pollLoadOlder(): void {
    root.pollAttempts += 1;
    if (root.loadOlderCount > 0) {
      pollTimer.stop();
      root.finishLoadOlderCheck();
      return;
    }
    if (root.pollAttempts < 160) return;

    pollTimer.stop();
    Check.fail("scrollToOldest() did not trigger loadOlder()");
  }

  // checkDraftRestore verifies a new draft still reaches the composer after
  // a message was sent, which clears the composer.
  function checkDraftRestore(): bool {
    view.draft = "first draft";
    view.composer.submit();
    view.draft = "second draft";
    if (view.composer.text !== "second draft")
      return Check.fail("composer shows \"" + view.composer.text + "\" after the draft changed, want \"second draft\"");
    return true;
  }

  // finishLoadOlderCheck runs the remaining checks once loadOlder() landed.
  function finishLoadOlderCheck(): void {
    if (!root.checkSend()) return;
    if (!root.checkDraftRestore()) return;
    if (!root.checkReplyTargetCopyIn()) return;
    if (!root.checkReplySignalRelay()) return;
    if (!root.checkScrollClampAtNewest()) return;
    if (!root.checkScrollClampAtOldest()) return;
    if (!root.checkScrollPageClamp()) return;
    if (!root.checkHistoryUnavailableNote()) return;
    if (!root.checkBlockedWiring()) return;
    root.startScheduledMessageCheck();
  }

  // checkBlockedWiring verifies that a blocked conversation reaches the
  // composer: disabled (composeEnabled alone does not say why) and its
  // own blocked flag set, so Composer shows the reason (see
  // tests/qml/Composer, which checks the banner and placeholder in
  // detail); this only checks the wiring reaches it.
  function checkBlockedWiring(): bool {
    view.conversation = Object.assign({}, view.conversation, { blocked: true });

    if (!view.composer.blocked) return Check.fail("a blocked conversation did not reach the composer's own blocked flag");
    if (view.composer.enabled) return Check.fail("the composer stayed enabled for a blocked conversation");

    view.conversation = Object.assign({}, view.conversation, { blocked: false });
    if (view.composer.blocked) return Check.fail("composer.blocked stayed true once the conversation was unblocked");
    if (!view.composer.enabled) return Check.fail("the composer stayed disabled once the conversation was unblocked");
    return true;
  }

  // checkHistoryUnavailableNote verifies the "phone offline" note only
  // shows once historyUnavailable is set, and that it clears again once
  // it is not: a real page that successfully reaches the service past
  // the earlier outage must not leave the note stuck showing.
  function checkHistoryUnavailableNote(): bool {
    view.historyUnavailable = false;
    let nodes = Check.texts(view);
    if (nodes.some(node => node.text.indexOf("Older messages need your phone online") !== -1 && node.visible))
      return Check.fail("the offline note showed with historyUnavailable false");

    view.historyUnavailable = true;
    nodes = Check.texts(view);
    const note = nodes.find(node => node.text.indexOf("Older messages need your phone online") !== -1);
    if (!note || !note.visible) return Check.fail("the offline note did not show with historyUnavailable true");

    view.historyUnavailable = false;
    nodes = Check.texts(view);
    if (nodes.some(node => node.text.indexOf("Older messages need your phone online") !== -1 && node.visible))
      return Check.fail("the offline note stayed visible after historyUnavailable cleared");
    return true;
  }

  // checkSend verifies a submitted composer message is trimmed and
  // reported with the id of the message it answers.
  function checkSend(): bool {
    root.sent = [];
    root.sentReplyIds = [];
    view.composer.replyTo = { id: "m9", senderName: "Alex", text: "quoted" };
    view.composer.text = "  hello there  ";
    view.composer.submit();
    if (JSON.stringify(root.sent) !== '["hello there"]') return Check.fail("sent " + JSON.stringify(root.sent) + ", want [\"hello there\"]");
    if (JSON.stringify(root.sentReplyIds) !== '["m9"]') return Check.fail("sent replyToId " + JSON.stringify(root.sentReplyIds) + ", want [\"m9\"]");
    return true;
  }

  // checkReplyTargetCopyIn verifies replyTarget reaches the composer, the
  // same way draft does, and clearing it clears the composer's reply too.
  function checkReplyTargetCopyIn(): bool {
    view.replyTarget = { id: "m5", senderName: "Alex", text: "quoted text" };
    if (!view.composer.replyTo || view.composer.replyTo.id !== "m5")
      return Check.fail("replyTarget was not copied into the composer");

    view.replyTarget = null;
    if (view.composer.replyTo !== null)
      return Check.fail("clearing replyTarget did not clear the composer's reply");
    return true;
  }

  // checkReplySignalRelay verifies a message delegate's replyRequested and
  // quoteOpened signals reach the view's own signals of the same name.
  function checkReplySignalRelay(): bool {
    view.scrollToNewest();
    const listView = Check.find(view, "messageListView");
    const delegateItem = listView.itemAtIndex(0);
    if (!delegateItem) return Check.fail("no message delegate realized at index 0");

    let replied = "";
    view.replyRequested.connect((id) => { replied = id; });
    delegateItem.replyRequested("m0");
    if (replied !== "m0") return Check.fail("replyRequested was not relayed to the view's signal");

    let quoted = "";
    view.quoteOpened.connect((remoteId) => { quoted = remoteId; });
    delegateItem.quoteOpened("r9");
    if (quoted !== "r9") return Check.fail("quoteOpened was not relayed to the view's signal");

    return root.checkScrollToMessage();
  }

  // checkScrollToMessage verifies scrolling to a loaded message brings it
  // into view, and that an unloaded id changes nothing.
  function checkScrollToMessage(): bool {
    view.scrollToNewest();
    view.scrollToMessage("m20");

    const listView = Check.find(view, "messageListView");
    if (!listView.itemAtIndex(20)) return Check.fail("scrollToMessage did not bring m20 into view");

    const before = listView.contentY;
    view.scrollToMessage("missing");
    if (listView.contentY !== before) return Check.fail("scrollToMessage moved the view for an unknown id");
    return true;
  }

  // scrollBounds returns the list's real scrollable range, read straight
  // from its geometry: a BottomToTop ListView's originY is not 0 and shifts
  // as delegates are created and estimated, so a correct clamp has to track
  // it rather than assume the content starts at 0.
  function scrollBounds(listView) {
    const oldest = listView.originY;
    const newest = oldest + Math.max(0, listView.contentHeight - listView.height);
    return { oldest: oldest, newest: newest };
  }

  // checkScrollClampAtNewest verifies that scrolling j past the newest
  // message stops exactly at it instead of continuing past the end.
  function checkScrollClampAtNewest(): bool {
    view.scrollToOldest();
    for (let i = 0; i < 200; i++) view.scrollBy(1);

    const listView = Check.find(view, "messageListView");
    const bounds = root.scrollBounds(listView);
    if (listView.contentY > bounds.newest + 0.5)
      return Check.fail("scrollBy(1) past the newest message left contentY " + listView.contentY + ", want at most " + bounds.newest);
    if (!listView.itemAtIndex(0))
      return Check.fail("the newest message's delegate is not visible after scrolling past it with j");
    return true;
  }

  // checkScrollClampAtOldest verifies that scrolling k past the oldest
  // loaded message stops exactly at it instead of continuing past the end.
  function checkScrollClampAtOldest(): bool {
    view.scrollToNewest();
    for (let i = 0; i < 200; i++) view.scrollBy(-1);

    const listView = Check.find(view, "messageListView");
    const bounds = root.scrollBounds(listView);
    if (listView.contentY < bounds.oldest - 0.5)
      return Check.fail("scrollBy(-1) past the oldest message left contentY " + listView.contentY + ", want at least " + bounds.oldest);
    if (!listView.itemAtIndex(listView.count - 1))
      return Check.fail("the oldest message's delegate is not visible after scrolling past it with k");
    return true;
  }

  // startScheduledMessageCheck queues a message while already at the
  // newest one, then polls until its header row has actually laid
  // itself out, to check it against the visible viewport: a ListView
  // header's own height growing needs a frame to settle, unlike the
  // synchronous scrolling the earlier checks poke directly.
  function startScheduledMessageCheck(): void {
    view.scrollToNewest();
    view.scheduledMessages = [{ id: "s1", text: "see you soon", sendAt: Date.now() + 60000 }];
    root.pollAttempts = 0;
    scheduledPollTimer.start();
  }

  // pollScheduledHeader waits for the header's delegate to be realized
  // (height > 0) before checking it, giving up after two seconds.
  function pollScheduledHeader(): void {
    root.pollAttempts += 1;
    const listView = Check.find(view, "messageListView");
    const headerItem = listView.headerItem;
    if (headerItem && headerItem.height > 0) {
      scheduledPollTimer.stop();
      root.finishScheduledMessageCheck(listView, headerItem);
      return;
    }
    if (root.pollAttempts < 80) return;

    scheduledPollTimer.stop();
    Check.fail("the scheduled message's header never laid itself out");
  }

  // finishScheduledMessageCheck checks the newly queued message's row
  // is inside the visible list at once (the report this covers found
  // it left below the clipped viewport, scrolled past it, until the
  // conversation was closed and reopened), then that a conversation
  // scrolled away into older history is not yanked back down just
  // because a message was scheduled elsewhere.
  function finishScheduledMessageCheck(listView, headerItem): void {
    const headerRect = Check.rect(headerItem, view);
    const listRect = Check.rect(listView, view);
    if (Check.overlapArea(headerRect, listRect, 0) <= 0) {
      Check.fail("the scheduled message's row is outside the visible list: header "
        + JSON.stringify(headerRect) + " list " + JSON.stringify(listRect));
      return;
    }

    view.scheduledMessages = [];
    view.scrollToOldest();
    const before = listView.contentY;
    view.scheduledMessages = [{ id: "s2", text: "see you soon", sendAt: Date.now() + 60000 }];
    if (listView.contentY !== before) {
      Check.fail("scheduling a message while scrolled into history moved contentY from " + before + " to " + listView.contentY);
      return;
    }

    view.scheduledMessages = [];
    console.log("PASS ConversationView");
    Qt.exit(0);
  }

  // checkScrollPageClamp verifies Ctrl+D/Ctrl+U paging clamps the same way
  // as j/k, at both ends of the loaded messages.
  function checkScrollPageClamp(): bool {
    view.scrollToOldest();
    for (let i = 0; i < 20; i++) view.scrollPage(1);
    let listView = Check.find(view, "messageListView");
    let bounds = root.scrollBounds(listView);
    if (listView.contentY > bounds.newest + 0.5)
      return Check.fail("scrollPage(1) past the newest message left contentY " + listView.contentY + ", want at most " + bounds.newest);

    view.scrollToNewest();
    for (let i = 0; i < 20; i++) view.scrollPage(-1);
    listView = Check.find(view, "messageListView");
    bounds = root.scrollBounds(listView);
    if (listView.contentY < bounds.oldest - 0.5)
      return Check.fail("scrollPage(-1) past the oldest message left contentY " + listView.contentY + ", want at least " + bounds.oldest);
    return true;
  }
}
