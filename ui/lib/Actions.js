.pragma library

// Maps every key action in Keymap.BINDINGS to the controller that owns it:
// the one whose run(action) performs the whole effect, from ui/controllers.

// OWNERS names the controller for each action: "list", "conversation",
// "dialog", "account" or "window". A key bound in more than one context (for example
// chat.mute in both "list" and "conversation") still has one owner, since
// the owning controller reads whatever extra state it needs itself.
var OWNERS = {
  // ListController: rail filter, search, the visible conversations,
  // selection and unread jump, mute.
  'search.focus': 'list',
  'rail.all': 'list',
  'rail.whatsapp': 'list',
  'rail.telegram': 'list',
  'rail.next': 'list',
  'rail.prev': 'list',
  'cursor.down': 'list',
  'cursor.up': 'list',
  'cursor.top': 'list',
  'cursor.bottom': 'list',
  'unread.next': 'list',
  'unread.prev': 'list',
  'chat.mute': 'list',

  // ConversationController: the open conversation, paging, send, retry,
  // scrolling and composing, and moving between chats while one is open.
  'chat.open': 'conversation',
  'search.accept': 'conversation',
  'pane.conversation': 'conversation',
  'pane.list': 'conversation',
  'scroll.down': 'conversation',
  'scroll.up': 'conversation',
  'scroll.pageDown': 'conversation',
  'scroll.pageUp': 'conversation',
  'scroll.newest': 'conversation',
  'scroll.oldest': 'conversation',
  'compose.focus': 'conversation',
  'chat.next': 'conversation',
  'chat.prev': 'conversation',
  'message.retry': 'conversation',
  'message.send': 'conversation',

  // DialogController: the new-chat dialog.
  'chat.new': 'dialog',
  'dialog.down': 'dialog',
  'dialog.up': 'dialog',
  'dialog.accept': 'dialog',
  'dialog.nextAccount': 'dialog',

  // AccountController: adding an account and signing it in.
  'account.add': 'account',
  'account.addOwnKeys': 'account',

  // WindowController: the command palette, closing and quitting, and the
  // Escape chain.
  'palette.commands': 'window',
  'palette.conversations': 'window',
  'palette.down': 'window',
  'palette.up': 'window',
  'palette.accept': 'window',
  'window.hide': 'window',
  'app.quit': 'window',
  'escape': 'window'
};

// owner returns the controller name that owns action, or "" if none does.
function owner(action) {
  return OWNERS[action] ?? '';
}
