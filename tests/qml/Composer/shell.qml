// Checks the composer: trimmed submit, whitespace ignored, cleared input,
// a fixed placeholder that never names the conversation, the blocked banner and
// placeholder shown while blocked, the reply banner and sending the
// id it answers, cancelling a reply, the editing banner for a sent
// message and the separate one for a queued "send later" message being
// edited (never shown together), the attachment chip, the chip's
// preview showing only for a reported kind of "photo" and never
// guessed from the path's own extension, both the
// banner and the chip showing together, the @-mention picker (shown
// while typing "@" in a group, keyboard and mouse insertion, Escape
// closing it without inserting, and the resolved mention reaching
// submitted()), and that a key the picker does not own (such as the
// command palette's Ctrl+/) still reaches routeKey while the picker is
// open, rather than being silently swallowed.
import QtQuick
import QtTest
import Quickshell
import "ui/components"
import "ui/lib/Keymap.js" as Keymap
import "ui/lib/SpellCheck.js" as SpellCheck
import "Check.js" as Check

ShellRoot {
  id: root

  property var sent: []
  property var sentReplyIds: []
  property var sentMentions: []
  property var routeKeyCalls: []
  property int cancelCount: 0
  property int editCancelCount: 0
  property int scheduledEditCancelCount: 0
  property var attached: []
  property int removeRequests: 0
  property var testMisspellings: []
  property var testSuggestions: []
  property var spellSuggestionRequests: []
  property var spellWordAdditions: []
  property int sendLaterRequests: 0
  property var truncatedLimits: []

  FloatingWindow {
    implicitWidth: 400
    implicitHeight: 300
    visible: true

    Composer {
      id: composer

      anchors.fill: parent
      misspellings: root.testMisspellings
      suggestions: root.testSuggestions
      onSubmitted: (text, replyToId, mentions) => {
        root.sent.push(text); root.sentReplyIds.push(replyToId); root.sentMentions.push(mentions);
      }
      onReplyCanceled: root.cancelCount += 1
      onEditCanceled: root.editCancelCount += 1
      onScheduledEditCanceled: root.scheduledEditCancelCount += 1
      onFileAttached: path => root.attached.push(path)
      onAttachmentRemoveRequested: root.removeRequests++
      onSpellSuggestionsRequested: word => root.spellSuggestionRequests.push(word)
      onSpellWordAdded: word => root.spellWordAdditions.push(word)
      onSendLaterRequested: root.sendLaterRequests++
      onTextTruncated: limit => root.truncatedLimits.push(limit)
    }
  }

  TestCase {
    id: t
    when: false
  }

  // Checks run once Quickshell has finished loading; Qt.exit() is ignored
  // before then.
  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // mentionFocusTimer gives forceActiveFocus() one turn of the event
  // loop to actually take hold before the first real key event is sent;
  // calling keyClick in the same synchronous turn as forceActiveFocus
  // fails, since the platform has not yet finished activating the
  // window.
  Timer {
    id: mentionFocusTimer
    interval: 20
    onTriggered: root.runFocusedChecks()
  }

  // run drives the composer and checks the outcome. focusInput() starts
  // writing first: the real text input now exists only from that call
  // until it loses focus (see Composer.qml's own _writingRequested), so
  // every check below that reaches composer.input needs this first,
  // the same way a real caller reaches it through compose.focus (i/a)
  // or a click, never for free just by the composer being on screen.
  function run(): void {
    composer.focusInput();
    if (!root.checkSubmit()) return;
    if (!root.checkBlocked()) return;
    if (!root.checkReplyBanner()) return;
    if (!root.checkEditBanner()) return;
    if (!root.checkScheduledEditBanner()) return;
    if (!root.checkAttachment()) return;
    if (!root.checkAttachmentPreview()) return;
    if (!root.checkAttachmentSizeAndHint()) return;
    if (!root.checkBannerAndChipTogether()) return;
    if (!root.checkSendLaterButton()) return;
    if (!root.checkOversizedTextIsCapped()) return;

    composer.attachmentPath = "";
    composer.replyTo = null;
    composer.members = [{ id: "u1", name: "Nadia" }, { id: "u2", name: "Ben" }];
    composer.input.forceActiveFocus();
    mentionFocusTimer.start();
  }

  // checkOversizedTextIsCapped covers the input's own paste guard: a
  // pasted clipboard string has no length limit of its own, so once it
  // lands in the TextArea it must be cut to the same length
  // messages.send would accept, exactly as if it had been typed that
  // short to begin with. It also covers textTruncated: a paste over
  // the limit reports it exactly once, typing further while still
  // pinned at the cap reports nothing more, and deleting below the cap
  // then pasting over it again reports it once more.
  function checkOversizedTextIsCapped(): bool {
    root.truncatedLimits = [];
    composer.input.text = "a".repeat(SpellCheck.MAX_COMPOSER_TEXT_LENGTH + 500);

    if (composer.input.text.length !== SpellCheck.MAX_COMPOSER_TEXT_LENGTH) {
      return Check.fail("composer input held " + composer.input.text.length + " characters, want capped to " +
        SpellCheck.MAX_COMPOSER_TEXT_LENGTH);
    }
    if (composer.text.length !== SpellCheck.MAX_COMPOSER_TEXT_LENGTH) {
      return Check.fail("composer.text held " + composer.text.length + " characters, want capped to " +
        SpellCheck.MAX_COMPOSER_TEXT_LENGTH);
    }
    if (JSON.stringify(root.truncatedLimits) !== "[" + SpellCheck.MAX_COMPOSER_TEXT_LENGTH + "]") {
      return Check.fail("textTruncated fired " + JSON.stringify(root.truncatedLimits) + " for the paste, want exactly one [" +
        SpellCheck.MAX_COMPOSER_TEXT_LENGTH + "]");
    }

    // Typing more while already pinned at the cap must not fire a
    // second toast for every rejected keystroke.
    composer.input.text = composer.input.text + "more text past the limit"
    if (JSON.stringify(root.truncatedLimits) !== "[" + SpellCheck.MAX_COMPOSER_TEXT_LENGTH + "]") {
      return Check.fail("textTruncated fired again while still pinned at the cap: " + JSON.stringify(root.truncatedLimits));
    }

    // Deleting back under the cap, then pasting over it again, is a
    // fresh cut event and reports once more.
    composer.input.text = composer.input.text.slice(0, composer.input.text.length - 10)
    composer.input.text = composer.input.text + "a".repeat(20)
    if (JSON.stringify(root.truncatedLimits) !== "[" + SpellCheck.MAX_COMPOSER_TEXT_LENGTH + "," + SpellCheck.MAX_COMPOSER_TEXT_LENGTH + "]") {
      return Check.fail("a fresh cut after shrinking below the cap did not report again: " + JSON.stringify(root.truncatedLimits));
    }

    composer.input.text = "";
    composer.text = "";
    root.truncatedLimits = [];
    return true;
  }

  // checkSubmit verifies trimming, whitespace-only text being ignored,
  // and the placeholder staying a fixed string.
  function checkSubmit(): bool {
    composer.text = "  hello  ";
    composer.submit();
    composer.text = "   ";
    composer.submit();

    if (JSON.stringify(root.sent) !== '["hello"]')
      return Check.fail("sent " + JSON.stringify(root.sent) + ", want [\"hello\"]");
    if (composer.text !== "   ")
      return Check.fail("whitespace-only text was cleared or sent");
    // The style draws the hint as AutoText, so it must never carry the
    // chat's own title, which the other side controls.
    if (composer.input.placeholderText !== "Message")
      return Check.fail("placeholder \"" + composer.input.placeholderText + "\", want the fixed \"Message\"");
    return true;
  }

  // checkBlocked verifies the banner naming why the contact is blocked,
  // and that the placeholder says so too, then resets blocked before the
  // checks that follow.
  function checkBlocked(): bool {
    const banner = Check.find(composer, "blockedBanner");
    if (banner.visible) return Check.fail("blocked banner shown while not blocked");

    composer.blocked = true;

    if (!banner.visible) return Check.fail("blocked banner not shown while blocked");
    if (composer.input.placeholderText !== "Blocked")
      return Check.fail("placeholder \"" + composer.input.placeholderText + "\" does not say Blocked");

    composer.blocked = false;
    return true;
  }

  // checkReplyBanner verifies the banner names the reply's sender and
  // text, that submitting carries the reply's id, that cancelling it
  // reports it without sending, and that it is hidden with no reply.
  function checkReplyBanner(): bool {
    const banner = Check.find(composer, "replyBanner");
    if (banner.visible) return Check.fail("reply banner shown with no reply in progress");

    composer.replyTo = { id: "m1", senderName: "Alex", text: "original message" };
    if (!banner.visible) return Check.fail("reply banner not shown while replying");

    const bannerText = Check.find(composer, "replyBannerText");
    if (bannerText.text.indexOf("Alex") < 0 || bannerText.text.indexOf("original message") < 0)
      return Check.fail("banner text \"" + bannerText.text + "\" does not name the sender and text");

    root.sent = [];
    root.sentReplyIds = [];
    composer.text = "sure";
    composer.submit();
    if (JSON.stringify(root.sentReplyIds) !== '["m1"]')
      return Check.fail("submitted replyToId " + JSON.stringify(root.sentReplyIds) + ", want [\"m1\"]");

    composer.replyTo = { id: "m2", senderName: "Alex", text: "another one" };
    const cancelButton = Check.find(composer, "replyCancel");
    root.cancelCount = 0;
    cancelButton.clicked();
    if (root.cancelCount !== 1) return Check.fail("replyCanceled fired " + root.cancelCount + " times, want 1");

    composer.replyTo = null;
    return true;
  }

  // checkEditBanner verifies the editing banner shows its fixed text
  // while editTarget names a message, that submitting still reports the
  // text through submitted() exactly like a normal send (the caller
  // decides whether that means saving an edit or sending something new,
  // see ComposerController and MessengerLayout.onSend), and that
  // cancelling it reports editCanceled.
  function checkEditBanner(): bool {
    const banner = Check.find(composer, "editBanner");
    if (banner.visible) return Check.fail("editing banner shown with no edit in progress");

    composer.editTarget = { id: "m1" };
    if (!banner.visible) return Check.fail("editing banner not shown while editing");

    const bannerText = Check.find(composer, "editBannerText");
    if (bannerText.text.indexOf("Esc to cancel") < 0)
      return Check.fail("editing banner text \"" + bannerText.text + "\" does not mention Esc to cancel");

    const sentBefore = root.sent;
    root.sent = [];
    composer.text = "edited";
    composer.submit();
    const sentWhileEditing = root.sent;
    root.sent = sentBefore;
    if (JSON.stringify(sentWhileEditing) !== '["edited"]')
      return Check.fail("submit while editing did not report the text: " + JSON.stringify(sentWhileEditing));

    composer.editTarget = { id: "m2" };
    const cancelButton = Check.find(composer, "editCancel");
    root.editCancelCount = 0;
    cancelButton.clicked();
    if (root.editCancelCount !== 1) return Check.fail("editCanceled fired " + root.editCancelCount + " times, want 1");

    composer.editTarget = null;
    return true;
  }

  // checkScheduledEditBanner verifies the queued message's own editing
  // banner shows its fixed text while scheduledEditing is true, never
  // at the same time as the sent-message editBanner, and that
  // cancelling it reports scheduledEditCanceled.
  function checkScheduledEditBanner(): bool {
    const banner = Check.find(composer, "scheduledEditBanner");
    if (banner.visible) return Check.fail("scheduled editing banner shown with no scheduled edit in progress");

    composer.scheduledEditing = true;
    if (!banner.visible) return Check.fail("scheduled editing banner not shown while editing a queued message");
    if (Check.find(composer, "editBanner").visible)
      return Check.fail("the sent-message editing banner showed at the same time as the scheduled one");

    const bannerText = Check.find(composer, "scheduledEditBannerText");
    if (bannerText.text.indexOf("Esc to cancel") < 0)
      return Check.fail("scheduled editing banner text \"" + bannerText.text + "\" does not mention Esc to cancel");

    const cancelButton = Check.find(composer, "scheduledEditCancel");
    root.scheduledEditCancelCount = 0;
    cancelButton.clicked();
    if (root.scheduledEditCancelCount !== 1)
      return Check.fail("scheduledEditCanceled fired " + root.scheduledEditCancelCount + " times, want 1");

    composer.scheduledEditing = false;
    return true;
  }

  // checkAttachment drives the attachment chip: it appears with a name,
  // lets a captionless message submit, and a click on its ✕ asks to
  // remove it.
  function checkAttachment(): bool {
    const chip = Check.find(composer, "attachmentChip");
    if (!chip) return Check.fail("no attachmentChip found");
    if (chip.visible) return Check.fail("chip visible before an attachment was set");

    composer.attachmentPath = "/tmp/photo.png";
    if (!chip.visible) return Check.fail("chip not visible with an attachment set");

    const name = Check.find(composer, "attachmentName");
    if (!name || name.text !== "photo.png")
      return Check.fail("attachment name = " + (name && name.text) + ", want photo.png");

    composer.text = "";
    composer.submit();
    if (JSON.stringify(root.sent) !== '["sure",""]')
      return Check.fail("a captionless attachment did not submit: sent " + JSON.stringify(root.sent));

    const removeButton = Check.find(composer, "removeAttachmentButton");
    if (!removeButton) return Check.fail("no removeAttachmentButton found");
    removeButton.clicked();
    if (root.removeRequests !== 1)
      return Check.fail("attachmentRemoveRequested fired " + root.removeRequests + " times, want 1");

    composer.attachmentPath = "";
    return true;
  }

  // checkAttachmentPreview verifies the chip decides its thumbnail from
  // attachmentKind alone, never from attachmentPath's own extension: a
  // ".png" path with no reported kind shows the file glyph, not a
  // decoded image, since the helper has not actually sanitized it as
  // one; a reported kind of "photo" shows the preview, "video" shows
  // the video glyph instead. It also checks attachmentName, not
  // attachmentPath's own base name, is what the chip names once set,
  // since a prepared attachment's path carries an internal id.
  function checkAttachmentPreview(): bool {
    const preview = Check.find(composer, "attachmentPreview");
    if (!preview) return Check.fail("no attachmentPreview found");

    composer.attachmentPath = "/tmp/evil.png";
    composer.attachmentKind = "";
    if (preview.visible) return Check.fail("a .png path with no reported kind showed a decoded preview");

    composer.attachmentKind = "photo";
    if (!preview.visible) return Check.fail("attachmentKind photo did not show the preview");

    composer.attachmentPath = "/outgoing/m1-vacation.png";
    composer.attachmentName = "vacation.png";
    const name = Check.find(composer, "attachmentName");
    if (!name || name.text !== "vacation.png")
      return Check.fail("attachment name = " + (name && name.text) + ", want vacation.png, not the stored path's own base name");

    composer.attachmentKind = "video";
    if (preview.visible) return Check.fail("attachmentKind video still showed a decoded preview");

    composer.attachmentPath = "";
    composer.attachmentName = "";
    composer.attachmentKind = "";
    return true;
  }

  // checkAttachmentSizeAndHint verifies the chip's size label: hidden
  // for an attachment well under its limit, shown naming the limit
  // once it is over it, and the attach button's tooltip carrying
  // attachmentLimitHint when the open conversation's account has one,
  // falling back to the plain tooltip when it does not.
  function checkAttachmentSizeAndHint(): bool {
    const sizeLabel = Check.find(composer, "attachmentSizeLabel");
    if (!sizeLabel) return Check.fail("no attachmentSizeLabel found");

    composer.attachmentPath = "/tmp/notes.txt";
    composer.attachmentKind = "file";
    composer.attachmentSize = 1024;
    composer.attachmentLimit = 2 * 1024 * 1024 * 1024;
    if (sizeLabel.visible) return Check.fail("size label shown for an attachment well under its limit: " + sizeLabel.text);

    composer.attachmentSize = 3 * 1024 * 1024 * 1024;
    if (!sizeLabel.visible) return Check.fail("size label hidden for an attachment over its limit");
    if (sizeLabel.text.indexOf("over") < 0 || sizeLabel.text.indexOf("2.0 GB") < 0)
      return Check.fail("size label = \"" + sizeLabel.text + "\", want it to name the 2.0 GB limit as exceeded");

    composer.attachmentPath = "";
    composer.attachmentKind = "";
    composer.attachmentSize = 0;
    composer.attachmentLimit = 0;

    const attachButton = Check.find(composer, "attachButton");
    if (!attachButton) return Check.fail("no attachButton found");
    if (attachButton.tooltipText !== "Attach a file")
      return Check.fail("attachButton tooltip = \"" + attachButton.tooltipText + "\" with no hint, want the plain \"Attach a file\"");

    composer.attachmentLimitHint = "Up to 2.0 GB";
    if (attachButton.tooltipText.indexOf("Up to 2.0 GB") < 0)
      return Check.fail("attachButton tooltip = \"" + attachButton.tooltipText + "\", want it to carry the hint");

    composer.attachmentLimitHint = "";
    return true;
  }

  // checkBannerAndChipTogether verifies replying to a message while an
  // attachment is already picked shows both the banner and the chip, and
  // that sending carries the reply id alongside the attachment.
  function checkBannerAndChipTogether(): bool {
    composer.attachmentPath = "/tmp/report.pdf";
    composer.replyTo = { id: "m3", senderName: "Alex", text: "see attached" };

    if (!Check.find(composer, "replyBanner").visible) return Check.fail("reply banner hidden while an attachment is pending");
    if (!Check.find(composer, "attachmentChip").visible) return Check.fail("attachment chip hidden while replying");

    root.sent = [];
    root.sentReplyIds = [];
    composer.text = "";
    composer.submit();
    if (JSON.stringify(root.sentReplyIds) !== '["m3"]')
      return Check.fail("submitted replyToId " + JSON.stringify(root.sentReplyIds) + " alongside an attachment, want [\"m3\"]");

    return true;
  }

  // checkSendLaterButton verifies the clock button beside Send: its
  // tooltip names what it does and the actual bound key for
  // compose.sendLater, read live off bindings rather than a hardcoded
  // "Ctrl+L" (keys.conf lets a person remap it), stays disabled with
  // nothing typed (an attachment alone is not enough, unlike Send),
  // and clicking it with text typed reports sendLaterRequested
  // exactly once.
  function checkSendLaterButton(): bool {
    const button = Check.find(composer, "sendLaterButton");
    if (!button) return Check.fail("no sendLaterButton found");
    if (button.tooltipText !== "Send later (Ctrl+L)")
      return Check.fail("sendLaterButton tooltip = \"" + button.tooltipText + "\", want \"Send later (Ctrl+L)\"");

    const rebound = composer.bindings.map((b) => b.action === "compose.sendLater" ? Object.assign({}, b, { keys: ["Ctrl+Alt+L"] }) : b);
    composer.bindings = rebound;
    if (button.tooltipText !== "Send later (Ctrl+Alt+L)")
      return Check.fail("sendLaterButton tooltip did not follow a remapped key, got \"" + button.tooltipText + "\"");
    composer.bindings = Keymap.BINDINGS;

    composer.text = "";
    composer.attachmentPath = "/tmp/report.pdf";
    if (button.enabled) return Check.fail("sendLaterButton enabled with an attachment but no text");
    composer.attachmentPath = "";

    composer.text = "later please";
    if (!button.enabled) return Check.fail("sendLaterButton not enabled with text typed");

    root.sendLaterRequests = 0;
    button.clicked();
    if (root.sendLaterRequests !== 1)
      return Check.fail("sendLaterRequested fired " + root.sendLaterRequests + " times, want 1");

    composer.text = "";
    return true;
  }

  // runFocusedChecks chains the checks that need real keyboard focus:
  // the @-mention picker, then the spelling picker, since both compete
  // for the same input and its focus.
  function runFocusedChecks(): void {
    if (!root.checkMentionPicker()) return;
    if (!root.checkSpellCheck()) return;
    if (!root.checkOversizedMisspellingListStaysBounded()) return;

    console.log("PASS Composer");
    Qt.exit(0);
  }

  // checkMentionPicker drives the @-mention picker end to end: it opens
  // while typing "@query" in a group, filters to matching members,
  // Escape closes it without changing the text, Tab inserts the
  // highlighted member by keyboard, a click inserts one by mouse, and
  // the text submitted carries the resolved mention at its real
  // position in the final, trimmed text.
  function checkMentionPicker(): bool {
    const input = composer.input;
    if (!input.activeFocus) return Check.fail("composer input did not get active focus");
    input.text = "hi @nad";
    input.cursorPosition = input.text.length;

    if (!composer.pickerOpen) return Check.fail("picker not open while typing an @query");

    const picker = Check.find(composer, "mentionPicker");
    if (!picker || !picker.visible) return Check.fail("mentionPicker not visible while the picker is open");

    const names = Check.texts(picker).map((item) => item.text);
    if (!names.includes("Nadia")) return Check.fail("picker did not show Nadia: " + names);
    if (names.includes("Ben")) return Check.fail("picker showed Ben, which does not match \"nad\": " + names);

    // A key the mention picker does not own, such as the command
    // palette's Ctrl+/, must still reach routeKey while the picker is
    // open: Keymap.match("mentionPicker", …) also matches every global
    // binding, so without checking the action is actually one of the
    // picker's own, it would be swallowed here and never reach the key
    // router at all.
    // keyClick synthesizes the Control press itself as its own key event
    // too, which also reaches routeKey regardless of this fix (nothing
    // in Keymap.BINDINGS matches a bare Control press), so only a call
    // for the Slash key itself counts as reaching routeKey.
    root.routeKeyCalls = [];
    composer.routeKey = (key, modifiers, text) => { root.routeKeyCalls.push(key); return true; };
    t.keyClick(Qt.Key_Slash, Qt.ControlModifier);
    if (!root.routeKeyCalls.includes(Qt.Key_Slash)) {
      return Check.fail("Ctrl+/ while the mention picker was open did not reach routeKey: " + JSON.stringify(root.routeKeyCalls));
    }
    composer.routeKey = null;

    t.keyClick(Qt.Key_Escape);
    if (composer.pickerOpen) return Check.fail("Escape did not close the picker");
    if (input.text !== "hi @nad") return Check.fail("Escape changed the text: " + input.text);

    input.text = "";
    input.text = "hi @nad";
    input.cursorPosition = input.text.length;
    if (!composer.pickerOpen) return Check.fail("picker did not reopen for a fresh query");

    t.keyClick(Qt.Key_Tab);
    if (input.text !== "hi @Nadia ") return Check.fail("Tab did not insert the mention: " + input.text);
    if (composer.pickerOpen) return Check.fail("picker stayed open after accepting a mention");

    root.sent = [];
    root.sentReplyIds = [];
    root.sentMentions = [];
    composer.submit();
    if (JSON.stringify(root.sent) !== '["hi @Nadia"]') {
      return Check.fail("sent " + JSON.stringify(root.sent) + ", want [\"hi @Nadia\"]");
    }

    const mentions = root.sentMentions[0];
    if (!mentions || mentions.length !== 1 || mentions[0].userId !== "u1" || mentions[0].offset !== 3 || mentions[0].length !== 6) {
      return Check.fail("resolved mentions = " + JSON.stringify(mentions) + ", want one mention of u1 at offset 3, length 6");
    }

    input.text = "";
    input.text = "hi @b";
    input.cursorPosition = input.text.length;
    if (!composer.pickerOpen) return Check.fail("picker did not open for the second query");

    const picker2 = Check.find(composer, "mentionPicker");
    if (!picker2) return Check.fail("no mentionPicker found for the second query");
    // The real click goes through MentionPicker's own MouseArea straight
    // to this same accepted signal (see MentionPicker.qml's onClicked);
    // emitting it here is how the test harness reaches the mouse path
    // without depending on synthetic pointer coordinates landing inside
    // an offscreen, zero-geometry window.
    picker2.accepted(0);
    if (input.text !== "hi @Ben ") return Check.fail("clicking the row did not insert the mention: " + input.text);

    return true;
  }

  // checkSpellCheck drives the spelling picker end to end: an underline
  // is drawn under a misspelled range given in misspellings, the
  // keyboard shortcut and a right-click both open the picker for the
  // word under the caret or the click and ask the caller for
  // suggestions, choosing one replaces the word, "Add to dictionary"
  // reports the word without changing the text, and "Ignore" clears the
  // underline locally without reporting anything to the caller.
  function checkSpellCheck(): bool {
    const input = composer.input;
    input.text = "";
    input.text = "see wrongg now"; // "wrongg" spans [4, 10)
    input.cursorPosition = input.text.length;
    root.testMisspellings = [{ start: 4, end: 10, word: "wrongg" }];

    if (!root.spellUnderlineDrawn()) return Check.fail("no underline drawn under the misspelled word");

    input.cursorPosition = 6;
    root.spellSuggestionRequests = [];
    t.keyClick(Qt.Key_Period, Qt.ControlModifier);
    if (!composer.spellPickerOpen) return Check.fail("Ctrl+. did not open the spelling picker");
    if (JSON.stringify(root.spellSuggestionRequests) !== '["wrongg"]') {
      return Check.fail("spellSuggestionsRequested = " + JSON.stringify(root.spellSuggestionRequests) + ", want [\"wrongg\"]");
    }

    root.testSuggestions = ["wrong", "wrongs"];
    const picker = Check.find(composer, "spellPicker");
    if (!picker || !picker.visible) return Check.fail("spellPicker not visible while the picker is open");

    const rowNames = Check.texts(picker).map((item) => item.text);
    if (!rowNames.includes("wrong") || !rowNames.includes("Add to dictionary") || !rowNames.includes("Ignore")) {
      return Check.fail("spellPicker rows = " + JSON.stringify(rowNames) + ", want wrong, Add to dictionary and Ignore");
    }

    // Enter with the first suggestion highlighted (the picker always
    // opens with highlightedIndex 0) replaces the word.
    t.keyClick(Qt.Key_Return);
    if (input.text !== "see wrong now") return Check.fail("choosing a suggestion did not replace the word: " + input.text);
    if (composer.spellPickerOpen) return Check.fail("picker stayed open after choosing a suggestion");

    // Reopen for a fresh word and add it to the dictionary instead.
    input.text = "";
    input.text = "see wrongg now";
    root.testMisspellings = [{ start: 4, end: 10, word: "wrongg" }];
    input.cursorPosition = 6;
    t.keyClick(Qt.Key_Period, Qt.ControlModifier);
    if (!composer.spellPickerOpen) return Check.fail("Ctrl+. did not reopen the spelling picker");

    root.spellWordAdditions = [];
    t.keyClick(Qt.Key_A);
    if (composer.spellPickerOpen) return Check.fail("picker stayed open after adding to the dictionary");
    if (JSON.stringify(root.spellWordAdditions) !== '["wrongg"]') {
      return Check.fail("spellWordAdded = " + JSON.stringify(root.spellWordAdditions) + ", want [\"wrongg\"]");
    }
    if (input.text !== "see wrongg now") return Check.fail("adding to the dictionary changed the text: " + input.text);

    // Ignore clears the underline locally, without reporting anything.
    input.cursorPosition = 6;
    t.keyClick(Qt.Key_Period, Qt.ControlModifier);
    if (!composer.spellPickerOpen) return Check.fail("Ctrl+. did not reopen the spelling picker a third time");

    root.spellWordAdditions = [];
    t.keyClick(Qt.Key_I);
    if (composer.spellPickerOpen) return Check.fail("picker stayed open after ignoring the word");
    if (root.spellWordAdditions.length !== 0) return Check.fail("ignoring the word reported it to the caller, want nothing sent");
    if (root.spellUnderlineDrawn()) return Check.fail("underline still drawn under an ignored word");

    // A right-click on a flagged word opens the picker the same way the
    // shortcut does, for whichever word the click landed on rather than
    // the caret. A fresh word, never "wrongg": the step above already
    // put that one in _ignoredWords, which would filter it out here.
    input.text = "";
    input.text = "mispelt";
    root.testMisspellings = [{ start: 0, end: 7, word: "mispelt" }];
    root.spellSuggestionRequests = [];
    const clickPos = input.positionToRectangle(2);
    t.mouseClick(input, clickPos.x + 1, clickPos.y + 1, Qt.RightButton);
    if (!composer.spellPickerOpen) return Check.fail("a right-click on a flagged word did not open the spelling picker");
    if (JSON.stringify(root.spellSuggestionRequests) !== '["mispelt"]') {
      return Check.fail("right-click spellSuggestionsRequested = " + JSON.stringify(root.spellSuggestionRequests) + ", want [\"mispelt\"]");
    }
    t.keyClick(Qt.Key_Escape);

    return true;
  }

  // spellUnderlineDrawn reports whether the overlay currently draws at
  // least one underline. _recomputeOverlay defers with Qt.callLater, so
  // this waits one real turn of the event loop first.
  function spellUnderlineDrawn(): bool {
    t.wait(50);
    return composer._overlayRects.length > 0;
  }

  // checkOversizedMisspellingListStaysBounded covers _recomputeOverlay's
  // own defence in depth: the helper already caps spell.check's result
  // (app.MaxSpellMisspellings), but a connector bug or an older helper
  // could still hand this component more, and it must never turn that
  // into an unbounded number of underline rectangles or delegates.
  function checkOversizedMisspellingListStaysBounded(): bool {
    const input = composer.input;
    input.text = "";
    input.text = "a".repeat(1000);

    const huge = [];
    for (let i = 0; i < SpellCheck.MAX_RENDERED_MISSPELLINGS + 300; i++) {
      huge.push({ start: i, end: i + 1, word: "a" });
    }
    root.testMisspellings = huge;

    t.wait(50);
    // Each misspelling draws one rect, or two when its own range wraps
    // across a line break (see _rectsForRange), so the bound is twice
    // MAX_RENDERED_MISSPELLINGS, not exactly it.
    if (composer._overlayRects.length > SpellCheck.MAX_RENDERED_MISSPELLINGS * 2) {
      return Check.fail("overlay drew " + composer._overlayRects.length + " rects for " + huge.length +
        " misspellings, want at most " + (SpellCheck.MAX_RENDERED_MISSPELLINGS * 2));
    }

    root.testMisspellings = [];
    return true;
  }
}
