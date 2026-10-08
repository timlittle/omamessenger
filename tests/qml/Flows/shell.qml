// Drives the whole window through the flows a person relies on, with real
// key events against the test helper:
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
// Each step polls until its condition holds, because every answer comes
// back from the helper asynchronously.
import QtQuick
import QtTest
import Quickshell
import "ui"
import "Check.js" as Check

ShellRoot {
  id: root

  property string expected: ""
  property int samUnread: -1
  readonly property string testRoot: String(Qt.resolvedUrl(".")).replace("file://", "")
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
    root.waitForImageDelivered
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

  // Named so Panel's own service property, inside the Loader, does not
  // shadow it and bind to itself.
  Service {
    id: helperService
  }

  FakeShell {
    id: fakeShell
    panel: root.panel()
    service: helperService
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
