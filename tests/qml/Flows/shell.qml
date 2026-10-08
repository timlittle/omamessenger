// A single smoke suite driving the whole window through real key events
// against the test helper, journey by journey, checking each one reaches
// its outcome rather than re-verifying details lower layers already prove
// (the state machines behind escape, rail filters and message delivery
// are covered in ui/lib node tests, the controller tests and backend/ Go
// tests; this suite exists to prove the real keyboard, the real Panel and
// the real helper still fit together):
// - search for "ticket" finds Alex Chen, Enter opens it, its unread clears
//   and the bar widget's count follows
// - a draft survives the panel being destroyed and recreated
// - Ctrl+K jumps to Sam, whose first send fails, and t retries it
// - a message arriving in Sam's chat while the window is hidden stays unread
// - Ctrl+N, cycling every account, "Ben", Enter opens a new chat with Ben
// - Ctrl+K to Mum, whose short stored history makes the helper fetch older
//   messages: her newest stays at the bottom and the unread total holds,
//   and her newest, a photo, is downloaded into the media cache
// - a file from the test's own directory is attached (as the attach
//   button's file dialog would report it) and sent to Mum with a
//   caption, and is delivered as a file attachment
// - a photo from the test's own directory is attached the same way and
//   sent to Mum, and its bubble shows the real downloaded image, not an
//   empty box, once it is delivered
// - opening a conversation by moving the list cursor with j, j and
//   pressing Enter focuses the composer in its "writing" state (the only
//   place that checks the writing/not-writing chrome itself, since
//   Composer's own test drives the component directly, not through Panel)
// - Shift+Enter inserts a newline and grows the composer instead of
//   sending (its only coverage anywhere)
// - Ctrl+J, the "next unread conversation" shortcut, works while writing
//   without carrying the old draft over or sending it
// - three Escapes step back through compose, the open conversation and
//   finally ask to close the window, which Enter answers by keeping it in
//   the background, hiding it through the host's shell facade
// - reopening and narrowing the rail to Telegram with Ctrl+2 reaches the
//   filtered outcome through a real key, not just a direct controller call
// - the command palette runs "New message" and "Remove an account" by
//   name and by keyboard alone, the latter actually removing an account
//   and checking a shortcut still does something right afterward
// - every column still has width at the minimum window size
// - Ctrl+W then choosing Quit stops the helper, and reopening starts it
//   again; closing the window the way the compositor does always comes
//   back asking first
// XDG_DATA_HOME is set by the test runner, so this never touches real
// data, and the mock shell facade stands in for Omarchy's host.
import QtQuick
import QtTest
import Quickshell
import qs.Commons
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  property string expected: ""
  property int samUnread: -1
  readonly property string testRoot: String(Qt.resolvedUrl(".")).replace("file://", "")
  // expectedTitle, _priorTitle, _newlineHeightBefore, _hidesBeforeChain
  // and _hidesBefore are scratch state for the keyboard-chrome journeys
  // appended after the message flows above.
  property string expectedTitle: ""
  property string _priorTitle: ""
  property real _newlineHeightBefore: 0
  property int _hidesBeforeChain: 0
  property int _hidesBefore: 0
  property var steps: [
    root.waitForList,
    root.searchTicket,
    root.waitForSearch,
    root.openMatch,
    root.waitForAlexRead,
    root.typeDraft,
    root.recreatePanel,
    root.waitForDraftRestored,
    root.openSam,
    root.waitForSamFailed,
    root.retryWithT,
    root.waitForSamDelivered,
    root.hideWithSamOpen,
    root.messageWhileHidden,
    root.waitForSamUnread,
    root.reopen,
    root.newChatWithBen,
    root.waitForBen,
    root.openMum,
    root.waitForMumWithOlderHistory,
    root.waitForMumsPhoto,
    root.attachAndSendToMum,
    root.waitForAttachmentDelivered,
    root.attachImageToMum,
    root.waitForImageDelivered,
    root.leaveComposeForListReturn,
    root.waitComposeLeftForListReturn,
    root.closeConversationForListReturn,
    root.waitConversationClosedForListReturn,
    root.openThirdByCursor,
    root.waitThirdOpenAndWriting,
    root.typeMultiline,
    root.waitShiftNewlineInserted,
    root.typeThreeAndCtrlJ,
    root.waitCtrlJSwitched,
    root.leaveComposeForEscapeChain,
    root.waitComposeLeftNotWriting,
    root.closeConversationAndAskToClose,
    root.waitCloseQuestionAfterChain,
    root.chooseKeepInBackground,
    root.waitHiddenOnce,
    root.reopenAndFilterRail,
    root.waitRailFilter,
    root.checkCommandPaletteOpensNewChat,
    root.checkRemoveAccountKeyboard,
    root.waitAccountRemoved,
    root.checkMinimumSize,
    root.askToQuit,
    root.waitQuitQuestion,
    root.chooseQuit,
    root.waitReadyAgain,
    root.startCompositorClose,
    root.waitCompositorCloseQuestion
  ]

  // panel is the live Panel, which the Loader may have recreated.
  function panel(): var {
    return panelLoader.item;
  }

  // listModel is the conversation list the window shows.
  function listModel(): var {
    return Check.find(root.panel(), "conversationListView").model;
  }

  // rowIndex returns the visible row with the given title, or -1.
  function rowIndex(title: string): int {
    const model = root.listModel();
    for (let i = 0; i < model.count; i++) {
      if (model.get(i).title === title) return i;
    }
    return -1;
  }

  // messageStatus returns the status of the newest message with text, or "".
  function messageStatus(text: string): string {
    const model = Check.find(root.panel(), "messageListView").model;
    for (let i = 0; i < model.count; i++) {
      if (model.get(i).text === text) return model.get(i).status;
    }
    return "";
  }

  // title is the open conversation's header title.
  function title(): string {
    const header = Check.find(root.panel(), "conversationTitle");
    return header ? header.text : "";
  }

  // composerFocused reports whether the composer input has keyboard focus.
  function composerFocused(): bool {
    const composer = Check.find(root.panel(), "composerInput");
    return !!composer && composer.activeFocus;
  }

  // type sends each character of text as a key click.
  function type(text: string): void {
    for (const ch of text) t.keyClick(ch);
  }

  // waitForList holds until the demo helper has seeded all 11 chats.
  function waitForList(): var {
    // Each fake account stores its history before it reports connected, so
    // a search run any earlier could miss messages that are on their way.
    const accounts = helperService.accounts;
    return root.listModel().count === 11 && accounts.length === 3 && accounts.every((a) => a.status === "connected");
  }

  // searchTicket opens the unified palette with Ctrl+G and types a query,
  // the message search half of it: "ticket" matches no conversation
  // title, only a message in Alex Chen's history.
  function searchTicket(): var {
    t.keyClick(Qt.Key_G, Qt.ControlModifier);
    root.type("ticket");
    return true;
  }

  // waitForSearch holds until the palette's "Messages" section lists
  // Alex Chen's matching message, the only one "ticket" finds, once its
  // debounce and the round trip to the helper both settle.
  function waitForSearch(): var {
    const list = Check.find(root.panel(), "paletteList");
    const items = list ? list.model : [];
    return items.length === 1 && items[0].detail === "Alex Chen";
  }

  // openMatch opens the matched message with Enter, which also
  // highlights it in the conversation once it loads.
  function openMatch(): var {
    root.expected = String(helperService.unreadTotal);
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForAlexRead holds until Alex Chen is open, its unread message is
  // read and the bar widget shows the lower total.
  function waitForAlexRead(): var {
    if (root.title() !== "Alex Chen") return false;
    if (String(helperService.unreadTotal) === root.expected) return false;

    const badge = Check.find(barWidget, "unreadBadge");
    if (badge.count !== helperService.unreadTotal)
      return "bar shows " + badge.count + ", service has " + helperService.unreadTotal;
    return true;
  }

  // typeDraft leaves unsent text in the composer.
  function typeDraft(): var {
    root.type("draft kept");
    return true;
  }

  // recreatePanel destroys the panel and builds a new one, as Omarchy does
  // each time the window is hidden and summoned.
  function recreatePanel(): var {
    panelLoader.active = false;
    panelLoader.active = true;
    root.panel().open("{}");
    return true;
  }

  // waitForDraftRestored holds until the new panel reopens Alex Chen with
  // the draft still in the composer.
  function waitForDraftRestored(): var {
    const composer = Check.find(root.panel(), "composerInput");
    return root.title() === "Alex Chen" && composer && composer.text === "draft kept";
  }

  // openSam jumps to Sam's chat with Ctrl+K, the conversation switcher,
  // and sends a message there.
  function openSam(): var {
    t.keyClick(Qt.Key_Escape);
    t.keyClick(Qt.Key_K, Qt.ControlModifier);
    root.type("sam");
    t.keyClick(Qt.Key_Return);
    root.type("are you there");
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForSamFailed holds until Sam's spotty signal fails the send.
  function waitForSamFailed(): var {
    return root.title() === "Sam (spotty signal)" && root.messageStatus("are you there") === "failed";
  }

  // retryWithT leaves the composer, which resets the highlight to the
  // newest message (the one that just failed), and retries it with t.
  function retryWithT(): var {
    t.keyClick(Qt.Key_Escape);
    t.keyClick(Qt.Key_T);
    return true;
  }

  // waitForSamDelivered holds until the retried message is delivered.
  function waitForSamDelivered(): var {
    return root.messageStatus("are you there") === "delivered";
  }

  // hideWithSamOpen closes the window with Sam's chat still open and
  // keeps OmaMessenger running in the background.
  function hideWithSamOpen(): var {
    t.keyClick(Qt.Key_W, Qt.ControlModifier);
    const question = Check.find(root.panel(), "closeConfirm");
    if (!question || !question.visible) return false;

    t.keyClick(Qt.Key_Return);
    return !Check.find(root.panel(), "panelWindow").visible;
  }

  // messageWhileHidden has a demo message arrive in Sam's chat. The list
  // may still be filtered by the earlier search, so it asks the helper.
  function messageWhileHidden(): var {
    root.samUnread = -1;
    helperService.request("conversations.list", { query: "Sam (spotty" }, function(error, result) {
      helperService.request("fake.inject", { conversationId: result[0].id }, function() {});
    });
    return true;
  }

  // waitForSamUnread holds until that message counts as unread: nobody is
  // looking at a hidden window, so it must not be read on arrival.
  function waitForSamUnread(): var {
    helperService.request("conversations.list", { query: "Sam (spotty" }, function(error, result) {
      root.samUnread = result[0].unread;
    });
    return root.samUnread > 0;
  }

  // reopen shows the window again for the remaining steps.
  function reopen(): var {
    root.panel().open("{}");
    return true;
  }

  // newChatWithBen starts a chat with a WhatsApp contact. Ctrl+Tab once per
  // account cycles all the way round, back to the WhatsApp account the
  // dialog opens on.
  function newChatWithBen(): var {
    t.keyClick(Qt.Key_N, Qt.ControlModifier);
    for (let i = 0; i < helperService.accounts.length; i++) t.keyClick(Qt.Key_Tab, Qt.ControlModifier);
    root.type("Ben");
    root.expected = "";
    return true;
  }

  // waitForBen presses Enter once Ben is listed, then holds until his
  // conversation is open.
  function waitForBen(): var {
    if (root.title() === "Ben Okafor") return true;

    const dialog = Check.find(root.panel(), "newChatDialog");
    if (root.expected === "" && dialog && dialog.contacts.length === 1 && dialog.contacts[0].name === "Ben Okafor") {
      root.expected = "sent";
      t.keyClick(Qt.Key_Return);
    }
    return false;
  }
  // openMum jumps to Mum, whose few stored messages make opening her chat
  // fetch older history from the fake service.
  function openMum(): var {
    root.expected = String(helperService.unreadTotal);
    t.keyClick(Qt.Key_Escape);
    t.keyClick(Qt.Key_K, Qt.ControlModifier);
    root.type("mum");
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForMumWithOlderHistory holds until Mum's chat shows a full page,
  // older history included, with her newest message at the bottom and no
  // older message counted as unread.
  function waitForMumWithOlderHistory(): var {
    const model = Check.find(root.panel(), "messageListView").model;
    if (root.title() !== "Mum" || model.count < 50) return false;

    let newest = 0;
    for (let i = 0; i < model.count; i++) newest = Math.max(newest, model.get(i).created);
    if (model.get(0).created !== newest) return Check.fail("Mum's newest message is not at the bottom");
    if (String(helperService.unreadTotal) !== root.expected) {
      return Check.fail(`older history changed the unread total from ${root.expected} to ${helperService.unreadTotal}`);
    }
    return true;
  }
  // waitForMumsPhoto holds until her newest message, a photo, has been
  // downloaded through the helper into its media cache.
  function waitForMumsPhoto(): var {
    const newest = Check.find(root.panel(), "messageListView").model.get(0);
    return newest.mediaPath !== "" && newest.mediaPath.indexOf("/omamessenger/media/") >= 0;
  }

  // attachAndSendToMum attaches a file from the test's own directory, the
  // way the composer's file dialog would report it having been picked,
  // then sends it with a caption. Driving a real file dialog headlessly
  // is not possible, so this calls the signal the dialog's onAccepted
  // handler would have emitted.
  function attachAndSendToMum(): var {
    const composer = Check.find(root.panel(), "composer");
    composer.fileAttached(root.testRoot + "/attachment.txt");

    const chip = Check.find(root.panel(), "attachmentChip");
    if (!chip || !chip.visible) return Check.fail("attachment chip did not appear after fileAttached");

    const name = Check.find(root.panel(), "attachmentName");
    if (!name || name.text !== "attachment.txt")
      return Check.fail("attachment chip name = " + (name && name.text) + ", want attachment.txt");

    root.type("see attached");
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitForAttachmentDelivered holds until Mum's chat shows the
  // attachment delivered, and the chip is gone from the composer.
  function waitForAttachmentDelivered(): var {
    const chip = Check.find(root.panel(), "attachmentChip");
    if (chip && chip.visible) return false;

    return root.messageStatus("see attached") === "sent" || root.messageStatus("see attached") === "delivered";
  }
  // attachImageToMum attaches a photo from the test's own directory, the
  // way the composer's file dialog would report it having been picked,
  // then sends it with a caption.
  function attachImageToMum(): var {
    const composer = Check.find(root.panel(), "composer");
    composer.fileAttached(root.testRoot + "/photo.png");

    const chip = Check.find(root.panel(), "attachmentChip");
    if (!chip || !chip.visible) return Check.fail("attachment chip did not appear after attaching the photo");

    const name = Check.find(root.panel(), "attachmentName");
    if (!name || name.text !== "photo.png")
      return Check.fail("attachment chip name = " + (name && name.text) + ", want photo.png");

    root.type("see this photo");
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // messageIdFor returns the id of the newest loaded message with the
  // given text, or "" when none matches.
  function messageIdFor(text: string): var {
    const model = Check.find(root.panel(), "messageListView").model;
    for (let i = 0; i < model.count; i++) {
      if (model.get(i).text === text) return model.get(i).id;
    }
    return "";
  }

  // photoImageFor returns the "photoImage" Image inside the loaded
  // delegate for messageId, or null when that delegate is not built.
  function photoImageFor(messageId: string): var {
    const items = Check.find(root.panel(), "messageListView").contentItem.children;
    for (let i = 0; i < items.length; i++) {
      if (items[i].modelData && items[i].modelData.id === messageId) return Check.find(items[i], "photoImage");
    }
    return null;
  }

  // waitForImageDelivered holds until the attached photo is delivered and
  // its bubble shows the real downloaded image: a non-empty source that
  // has finished loading, not an empty box and not stuck waiting.
  function waitForImageDelivered(): var {
    const chip = Check.find(root.panel(), "attachmentChip");
    if (chip && chip.visible) return false;

    const status = root.messageStatus("see this photo");
    if (status !== "sent" && status !== "delivered") return false;

    const image = root.photoImageFor(root.messageIdFor("see this photo"));
    if (!image) return false;
    if (String(image.source) === "") return false;
    if (image.status === Image.Error) return Check.fail("the sent photo's image failed to load");
    if (image.status !== Image.Ready) return false;

    // The real file, not just the small inline thumb, must be what
    // loaded: that is the whole point of the local copy being used.
    if (String(image.source).indexOf("file://") !== 0) return Check.fail("the sent photo shows its thumb, not the real downloaded image: " + image.source);
    return true;
  }

  // leaveComposeForListReturn presses Escape to blur the composer, which
  // the message flows above left focused, before the keyboard-chrome
  // journeys below need the plain list (j/k and Enter only act there).
  function leaveComposeForListReturn(): var {
    t.keyClick(Qt.Key_Escape);
    return true;
  }

  // waitComposeLeftForListReturn holds until that Escape has taken effect.
  function waitComposeLeftForListReturn(): var {
    return !root.composerFocused();
  }

  // closeConversationForListReturn presses the second Escape, closing
  // Mum's conversation so the list is the only thing showing.
  function closeConversationForListReturn(): var {
    t.keyClick(Qt.Key_Escape);
    return true;
  }

  // waitConversationClosedForListReturn holds until the header clears.
  function waitConversationClosedForListReturn(): var {
    return root.title() === "";
  }

  // openThirdByCursor reads the row at index 2 (0-based) straight from the
  // list's own model, which exists whether or not that row's delegate
  // happens to be instantiated, then drives j, j, Enter: opening a
  // conversation by moving the list cursor and pressing Enter, rather than
  // jumping to it with Ctrl+K, is real keyboard delivery this suite would
  // otherwise never exercise.
  function openThirdByCursor(): var {
    root.expectedTitle = root.listModel().get(2).title;

    t.keyClick(Qt.Key_J);
    t.keyClick(Qt.Key_J);
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitThirdOpenAndWriting holds until the header names the row j, j
  // landed on and the composer has keyboard focus, then checks the
  // composer names its "writing" state in words and shows the input and
  // Send at full contrast: the only place that checks this chrome, since
  // Composer's own test drives the component directly rather than through
  // Panel. The report this exists for: scrolling and writing looked
  // identical except for the blinking text cursor.
  function waitThirdOpenAndWriting(): var {
    const titleEl = Check.find(root.panel(), "conversationTitle");
    const composer = Check.find(root.panel(), "composerInput");
    if (!titleEl || titleEl.text !== root.expectedTitle || !composer || !composer.activeFocus) return false;

    const hint = Check.find(root.panel(), "composerModeHint");
    const send = Check.find(root.panel(), "sendButton");
    if (!hint || hint.text !== "Writing · Esc to stop")
      return Check.fail("writing hint is \"" + (hint ? hint.text : "?") + "\", want \"Writing · Esc to stop\"");
    if (Check.find(root.panel(), "composerFrame"))
      return Check.fail("the composer still has a frame object");
    if (composer.opacity !== 1.0)
      return Check.fail("composer input opacity is " + composer.opacity + " while writing, want 1.0");
    if (!send || send.opacity !== 1.0)
      return Check.fail("Send opacity is " + (send ? send.opacity : "?") + " while writing, want 1.0");

    root._newlineHeightBefore = composer.implicitHeight;
    return true;
  }

  // typeMultiline types "one", Shift+Enter, then "two": Shift+Enter is the
  // composer's only coverage for inserting a newline instead of sending.
  function typeMultiline(): var {
    for (const ch of "one") t.keyClick(ch);
    t.keyClick(Qt.Key_Return, Qt.ShiftModifier);
    for (const ch of "two") t.keyClick(ch);
    return true;
  }

  // waitShiftNewlineInserted holds until the composer's text shows the
  // typed newline, then checks nothing was sent and the composer grew.
  function waitShiftNewlineInserted(): var {
    const composer = Check.find(root.panel(), "composerInput");
    if (!composer || composer.text !== "one\ntwo") return false;

    if (root.messageStatus("one") !== "" || root.messageStatus("one\ntwo") !== "")
      return Check.fail("Shift+Enter sent a message instead of inserting a newline");
    if (composer.implicitHeight <= root._newlineHeightBefore)
      return Check.fail("the composer did not grow for a second line: was "
        + root._newlineHeightBefore + ", now " + composer.implicitHeight);

    composer.text = "";
    return true;
  }

  // typeThreeAndCtrlJ types "three" then presses Ctrl+J: Ctrl+J must
  // always mean "next unread conversation", even while writing, rather
  // than inserting a newline.
  function typeThreeAndCtrlJ(): var {
    root._priorTitle = root.title();
    for (const ch of "three") t.keyClick(ch);
    t.keyClick(Qt.Key_J, Qt.ControlModifier);
    return true;
  }

  // waitCtrlJSwitched holds until Ctrl+J has switched the open
  // conversation away from the one "three" was typed into, then checks it
  // kept writing mode in the conversation it switched to, left no trace
  // of the old draft, and sent nothing.
  function waitCtrlJSwitched(): var {
    const newTitle = root.title();
    if (newTitle === "" || newTitle === root._priorTitle) return false;

    const composer = Check.find(root.panel(), "composerInput");
    if (!composer || !composer.activeFocus)
      return Check.fail("Ctrl+J left writing mode instead of keeping it in the next unread conversation");
    if (composer.text.indexOf("three") !== -1)
      return Check.fail("Ctrl+J carried the old draft over: " + JSON.stringify(composer.text));
    if (root.messageStatus("three") !== "" || root.messageStatus("three\nfour") !== "")
      return Check.fail("Ctrl+J sent a message instead of jumping to the next unread conversation");

    composer.text = "";
    return true;
  }

  // leaveComposeForEscapeChain presses the first of three Escapes: it
  // leaves the composer asynchronously, so the next step waits for that
  // before the chain continues. It also records how many times the
  // window has already been hidden (Sam's hide-while-open journey, above,
  // hid it once already), so the chain's own hide can be told apart from
  // that earlier one.
  function leaveComposeForEscapeChain(): var {
    root._hidesBeforeChain = fakeShell.hideCalls.length;
    t.keyClick(Qt.Key_Escape);
    return true;
  }

  // waitComposeLeftNotWriting holds until the first Escape has blurred the
  // composer, then checks the mode hint and frame reverted to the quiet,
  // not-writing state before the rest of the chain runs.
  function waitComposeLeftNotWriting(): var {
    const composer = Check.find(root.panel(), "composerInput");
    if (!composer || composer.activeFocus) return false;

    const hint = Check.find(root.panel(), "composerModeHint");
    const send = Check.find(root.panel(), "sendButton");
    if (!hint || hint.text !== "i to write")
      return Check.fail("not-writing hint is \"" + (hint ? hint.text : "?") + "\", want \"i to write\"");
    if (Check.find(root.panel(), "composerFrame"))
      return Check.fail("the composer still has a frame object");
    if (composer.opacity === 1.0)
      return Check.fail("composer input is still at full opacity while not writing");
    if (!send || send.opacity === 1.0)
      return Check.fail("Send is still at full opacity while not writing");
    return true;
  }

  // closeConversationAndAskToClose presses two more Escapes: one closes
  // the open conversation, the next asks before closing the window, since
  // nothing is left to undo.
  function closeConversationAndAskToClose(): var {
    t.keyClick(Qt.Key_Escape);
    t.keyClick(Qt.Key_Escape);
    return true;
  }

  // waitCloseQuestionAfterChain holds until the close question is showing.
  function waitCloseQuestionAfterChain(): var {
    const question = Check.find(root.panel(), "closeConfirm");
    return !!question && question.visible;
  }

  // chooseKeepInBackground checks the window has not already hidden
  // itself before asking, then presses Enter, which chooses the default,
  // Keep in background.
  function chooseKeepInBackground(): var {
    if (fakeShell.hideCalls.length !== root._hidesBeforeChain) return Check.fail("the window hid before asking");
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitHiddenOnce holds for the third Escape's hide to reach the shell
  // facade, since leaving the composer blurs it asynchronously, then
  // checks it was asked for exactly once more, with this plugin's own id.
  function waitHiddenOnce(): var {
    const want = root._hidesBeforeChain + 1;
    if (fakeShell.hideCalls.length < want) return false;
    if (fakeShell.hideCalls.length > want)
      return Check.fail("the escape chain called shell.hide " + (fakeShell.hideCalls.length - root._hidesBeforeChain) + " times, want 1");
    if (fakeShell.hideCalls[fakeShell.hideCalls.length - 1] !== "io.github.omamessenger")
      return Check.fail("shell.hide called with \"" + fakeShell.hideCalls[fakeShell.hideCalls.length - 1] + "\"");
    return true;
  }

  // reopenAndFilterRail reopens the window and narrows the rail to
  // Telegram with a real key: the "filtered" outcome, reached through the
  // keyboard rather than a direct controller call.
  function reopenAndFilterRail(): var {
    root.panel().open("{}");
    t.keyClick(Qt.Key_2, Qt.ControlModifier);
    return true;
  }

  // waitRailFilter holds for the Telegram-only row count the demo data
  // gives; which rows those are, and that rail.all restores the rest, is
  // ListController's own job, checked in tests/qml/Controllers.
  function waitRailFilter(): var {
    const listView = Check.find(root.panel(), "conversationListView");
    return !!listView && listView.count === 5;
  }

  // checkCommandPaletteOpensNewChat opens the command palette with
  // Ctrl+/, finds "New message" by typing part of it, and runs it with
  // Enter: fuzzy-matching and ranking the command is Palette.js's own job
  // (see tests/unit/palette.test.cjs), so this only checks the keystrokes
  // reach the command palette and running one opens the right dialog.
  function checkCommandPaletteOpensNewChat(): var {
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    const palette = Check.find(root.panel(), "commandPalette");
    if (!palette || !palette.visible) return Check.fail("Ctrl+/ did not open the command palette");

    for (const ch of "new mes") t.keyClick(ch);
    t.keyClick(Qt.Key_Return);

    const dialog = Check.find(root.panel(), "newChatDialog");
    if (palette.visible) return Check.fail("running a command left the palette open");
    if (!dialog || !dialog.visible) return Check.fail("running \"New message\" from the command palette did not open the new-chat dialog");

    t.keyClick(Qt.Key_Escape);
    if (dialog.visible) return Check.fail("Escape did not close the new-chat dialog");
    return true;
  }

  // checkRemoveAccountKeyboard runs "Remove an account" from the command
  // palette, cancels once with Escape before choosing anything, then
  // opens it again and removes the first account with Down then Enter,
  // entirely from the keyboard.
  function checkRemoveAccountKeyboard(): var {
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    for (const ch of "remove an acc") t.keyClick(ch);
    t.keyClick(Qt.Key_Return);

    const question = Check.find(root.panel(), "removeAccount");
    if (!question || !question.visible) return Check.fail("running \"Remove an account\" from the command palette did not open the question");

    t.keyClick(Qt.Key_Escape);
    if (question.visible) return Check.fail("Escape did not cancel the removal question");

    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    for (const ch of "remove an acc") t.keyClick(ch);
    t.keyClick(Qt.Key_Return);
    if (!question.visible) return Check.fail("reopening \"Remove an account\" after cancelling it with Escape did not work");

    t.keyClick(Qt.Key_Down);
    t.keyClick(Qt.Key_Return);
    return true;
  }

  // waitAccountRemoved holds until the removal question has closed, then
  // checks straight away that a shortcut still does something: the bug
  // this guards against left keyboard focus nowhere once an account was
  // actually removed, so Ctrl+/ did nothing until the window was closed
  // and reopened.
  function waitAccountRemoved(): var {
    const question = Check.find(root.panel(), "removeAccount");
    if (question && question.visible) return false;

    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    const commandPalette = Check.find(root.panel(), "commandPalette");
    if (!commandPalette || !commandPalette.visible)
      return Check.fail("Ctrl+/ did nothing right after removing an account: focus was left nowhere");

    t.keyClick(Qt.Key_Escape);
    return true;
  }

  // checkMinimumSize resizes the window to the documented minimum and
  // checks every column still has real width, rather than collapsing.
  function checkMinimumSize(): var {
    const win = Check.find(root.panel(), "panelWindow");
    win.width = Style.space(760);
    win.height = Style.space(540);

    const rail = Check.find(root.panel(), "serviceRail");
    const listColumn = Check.find(root.panel(), "listColumn");
    const conversationView = Check.find(root.panel(), "conversationView");

    if (!rail || rail.width <= 0) return Check.fail("service rail has no width at the minimum size");
    if (!listColumn || listColumn.width <= 0) return Check.fail("list column has no width at the minimum size");
    if (!conversationView || conversationView.width <= 0) return Check.fail("conversation view has no width at the minimum size");
    if (listColumn.width > Style.space(360) + 1)
      return Check.fail("list column is " + listColumn.width + " wide, more than its maximum of " + Style.space(360));
    if (conversationView.width < Style.space(300))
      return Check.fail("conversation view is only " + conversationView.width + " wide at the minimum size");
    return true;
  }

  // askToQuit presses Ctrl+W, asking to close again.
  function askToQuit(): var {
    t.keyClick(Qt.Key_W, Qt.ControlModifier);
    return true;
  }

  // waitQuitQuestion holds until the close question is showing.
  function waitQuitQuestion(): var {
    const question = Check.find(root.panel(), "closeConfirm");
    return !!question && question.visible;
  }

  // chooseQuit moves from Keep in background to Quit and checks the
  // helper actually stops, then reopens the window to check it starts
  // again.
  function chooseQuit(): var {
    t.keyClick(Qt.Key_Left);
    t.keyClick(Qt.Key_Return);
    if (helperService.status !== "stopped")
      return Check.fail("Left then Enter on the close question left the helper " + helperService.status);

    root.panel().open("{}");
    return true;
  }

  // waitReadyAgain holds until reopening has restarted the helper.
  function waitReadyAgain(): var {
    return helperService.status === "ready";
  }

  // startCompositorClose closes the window the way the compositor does,
  // which cannot be refused, and records how many times it has been
  // hidden so far for the next step to compare against.
  function startCompositorClose(): var {
    root._hidesBefore = fakeShell.hideCalls.length;
    Check.find(root.panel(), "panelWindow").visible = false;
    return true;
  }

  // waitCompositorCloseQuestion holds until the window comes back asking,
  // checks it did not report itself hidden before asking, then cancels
  // the question with Escape, ending the suite.
  function waitCompositorCloseQuestion(): var {
    const win = Check.find(root.panel(), "panelWindow");
    const question = Check.find(root.panel(), "closeConfirm");
    if (!win || !win.visible || !question || !question.visible) return false;

    if (fakeShell.hideCalls.length !== root._hidesBefore)
      return Check.fail("the window reported hidden before asking");

    t.keyClick(Qt.Key_Escape);
    if (Check.find(root.panel(), "closeConfirm").visible)
      return Check.fail("Escape did not cancel the close question");
    return true;
  }

  // Named so Panel's own service property, inside the Loader, does not
  // shadow it and bind to itself.
  Service {
    id: helperService
  }

  // fakeShell stands in for Omarchy's shell facade. Unlike the shared
  // FakeShell.qml, it records every hide call: the close-question journeys
  // below need to tell "asked, then hid" apart from "hid without asking".
  QtObject {
    id: fakeShell

    property var hideCalls: []

    function hide(id) { fakeShell.hideCalls.push(id); root.panel().close(); }
    function serviceFor(id) { return helperService; }
    function toggle(id, payloadJson) { root.panel().open(payloadJson); }
    function summon(id, payloadJson) { root.panel().open(payloadJson); }
  }

  Loader {
    id: panelLoader

    sourceComponent: Panel {
      service: helperService
      shell: fakeShell
    }
  }

  BarWidget {
    id: barWidget

    bar: ({ shell: fakeShell })
  }

  TestCase {
    id: t

    when: false
  }

  Stepper {
    id: stepper
    name: "Flows"
    steps: root.steps
    deadlineMs: 55000
    startFn: () => { root.panel().open("{}"); stepper.runStep(); }
  }
}
