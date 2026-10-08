.pragma library

// Maps every key action in Keymap.BINDINGS to the controller that owns it:
// the one whose run(action) performs the whole effect, from ui/controllers.

// OWNERS names the controller for each action: "list", "conversation",
// "composer", "photoViewer", "reactions", "dialog", "account" or "window".
// A key bound in more than one context (for example chat.mute in both
// "list" and "conversation") still has one owner, since the owning
// controller reads whatever extra state it needs itself.
var OWNERS = {
  // ListController: rail filter, search, the visible conversations,
  // selection and unread jump, mute.
  'search.focus': 'list',
  'list.showAll': 'list',
  'list.unread': 'list',
  'rail.all': 'list',
  'rail.whatsapp': 'list',
  'rail.telegram': 'list',
  'rail.next': 'list',
  'rail.prev': 'list',
  'cursor.down': 'list',
  'cursor.up': 'list',
  'cursor.top': 'list',
  'cursor.bottom': 'list',
  'chat.mute': 'list',
  'chat.pin': 'list',
  'chat.archive': 'list',
  'chat.hide': 'list',
  'chat.archiveRead': 'list',
  'list.archiveAllRead': 'list',
  'archiveAll.accept': 'list',
  'archiveAll.cancel': 'list',
  'chat.snoozeLaterToday': 'list',
  'chat.snoozeTomorrow': 'list',
  'chat.snoozeNextWeek': 'list',
  'chat.unsnooze': 'list',

  // ConversationController: the open conversation itself, paging,
  // send, retry, scrolling, and moving between chats while one is open.
  'chat.open': 'conversation',
  'search.accept': 'conversation',
  'pane.conversation': 'conversation',
  'pane.list': 'conversation',
  'message.highlightNewer': 'conversation',
  'message.highlightOlder': 'conversation',
  'message.open': 'conversation',
  'scroll.pageDown': 'conversation',
  'scroll.pageUp': 'conversation',
  'scroll.newest': 'conversation',
  'scroll.oldest': 'conversation',
  'chat.next': 'conversation',
  'chat.prev': 'conversation',
  'message.retry': 'conversation',
  'message.openLink': 'conversation',
  'message.goToQuote': 'conversation',
  // unread.next/prev open the conversation they land on when one is
  // already showing, which is why ConversationController (which already
  // knows how to open one) owns them rather than ListController, which
  // only ever moves the list cursor.
  'unread.next': 'conversation',
  'unread.prev': 'conversation',

  // ComposerController: the draft, replying, attaching and sending.
  'compose.focus': 'composer',
  'message.reply': 'composer',
  'message.send': 'composer',
  'compose.newline': 'composer',
  'compose.attach': 'composer',
  'compose.attachFile': 'composer',
  // The @-mention picker is part of composing: it opens over the
  // composer's own text field and never leaves it.
  'mention.down': 'composer',
  'mention.up': 'composer',
  'mention.accept': 'composer',
  'mention.cancel': 'composer',

  // PhotoViewerController: stepping through the in-app photo viewer.
  'viewer.next': 'photoViewer',
  'viewer.prev': 'photoViewer',
  'viewer.openExternal': 'photoViewer',

  // ReactionsController: the emoji picker and reacting to a message.
  'message.react': 'reactions',
  'reaction.left': 'reactions',
  'reaction.right': 'reactions',
  'reaction.accept': 'reactions',

  // DeleteController: the delete question and deleting a message.
  'message.delete': 'delete',
  'delete.left': 'delete',
  'delete.right': 'delete',
  'delete.accept': 'delete',
  'delete.everyone': 'delete',
  'delete.forMe': 'delete',
  'delete.cancel': 'delete',

  // DialogController: the new-chat dialog.
  'chat.new': 'dialog',
  'dialog.down': 'dialog',
  'dialog.up': 'dialog',
  'dialog.accept': 'dialog',
  'dialog.nextAccount': 'dialog',

  // AccountController: adding an account and signing it in, choosing a
  // service and a step's own navigation, and removing an account.
  'account.add': 'account',
  'account.addOwnKeys': 'account',
  'account.remove': 'account',
  'setup.down': 'account',
  'setup.up': 'account',
  'setup.accept': 'account',
  'setup.chooseTelegram': 'account',
  'setup.chooseWhatsapp': 'account',
  'qr.usePhone': 'account',
  'phone.useQr': 'account',
  'phone.back': 'account',
  'remove.down': 'account',
  'remove.up': 'account',
  'remove.pick1': 'account',
  'remove.pick2': 'account',
  'remove.pick3': 'account',
  'remove.pick4': 'account',
  'remove.pick5': 'account',
  'remove.pick6': 'account',
  'remove.pick7': 'account',
  'remove.pick8': 'account',
  'remove.pick9': 'account',
  'remove.accept': 'account',
  'remove.cancel': 'account',

  // WindowController: the command palette, closing and quitting, the
  // close question's own navigation, and the Escape chain.
  'palette.commands': 'window',
  'palette.conversations': 'window',
  // chat.snoozeCustom opens the palette's custom-snooze prompt, which
  // WindowController owns along with every other palette mode; it is the
  // one chat.* action not owned by ListController, since the chat it
  // applies to is resolved only once the prompt is accepted.
  'chat.snoozeCustom': 'window',
  'palette.down': 'window',
  'palette.up': 'window',
  'palette.accept': 'window',
  'window.hide': 'window',
  'app.quit': 'window',
  'helper.retryInstall': 'window',
  'helper.doctor': 'window',
  'keys.openConfig': 'window',
  'keys.showBindings': 'window',
  'close.left': 'window',
  'close.right': 'window',
  'close.accept': 'window',
  'close.cancel': 'window',
  'close.quit': 'window',
  'close.keep': 'window',
  'escape': 'window'
};

// owner returns the controller name that owns action, or "" if none does.
function owner(action) {
  return OWNERS[action] ?? '';
}
