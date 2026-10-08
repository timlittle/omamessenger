import QtQuick
import "../lib/Actions.js" as Actions
import "../lib/Rpc.js" as Rpc

// Owns the new-chat dialog: which account's contacts are shown, the
// search text, the keyboard-selected contact, and opening the chosen
// conversation. The only controller that calls contacts.list and
// conversations.open.
//
// It holds its own copy of the dialog's navigation state (account,
// query, contacts, currentIndex) so a key action can run it without a
// view reference; NewChatDialog.qml's own moveCurrent/accept remain
// available for a view wired directly to mouse clicks.
//
// QtObject rather than Item: it holds no child objects, unlike the other
// controllers.
QtObject {
  id: root

  // service is the Service instance that owns the helper connection.
  property var service: null

  // open shows the dialog when true.
  property bool open: false

  // accountId is the account whose contacts are shown.
  property string accountId: ""

  // query is the active contact search text.
  property string query: ""

  // contacts are the helper's matches for accountId and query.
  property var contacts: []

  // currentIndex is the keyboard-selected row in contacts, or -1 for none.
  property int currentIndex: -1

  // lastError is the safe text of the most recent request failure.
  property string lastError: ""

  // opened hands off a conversation the dialog just opened. The panel
  // wires this to the conversation controller's open(conversation).
  signal opened(var conversation)

  // handles reports whether this controller owns action.
  function handles(action: string): bool {
    return Actions.owner(action) === "dialog";
  }

  // run performs action, the only entry point a key router needs.
  function run(action: string): void {
    const handlers = {
      "chat.new": () => root._openDialog(),
      "dialog.down": () => root._move(1),
      "dialog.up": () => root._move(-1),
      "dialog.accept": () => root._accept(),
      "dialog.nextAccount": () => root._nextAccount()
    };

    const handler = handlers[action];
    if (handler) handler();
  }

  // close dismisses the dialog without opening a conversation.
  function close(): void {
    root.open = false;
  }

  // setAccount switches which account's contacts are shown.
  function setAccount(id: string): void {
    root.accountId = id;
    root._loadContacts();
  }

  // setQuery updates the contact search text and re-runs it.
  function setQuery(text: string): void {
    root.query = text;
    root._loadContacts();
  }

  // openConversation opens accountId/contactId's conversation and hands
  // it off through opened(). Public so a view's own accept() (a mouse
  // click on a contact row) can reach the same helper call.
  function openConversation(accountId: string, contactId: string): void {
    root.service.request("conversations.open", { accountId: accountId, contactId: contactId }, function(error, result) {
      if (error) { root.lastError = Rpc.errorText(error); return; }
      root.open = false;
      root.opened(result);
    });
  }

  // _openDialog resets the dialog to the first account and loads its
  // contacts.
  function _openDialog(): void {
    root.open = true;
    root.query = "";
    const accounts = root.service ? root.service.accounts : [];
    root.setAccount(accounts.length > 0 ? accounts[0].id : "");
  }

  // _nextAccount switches to the account after the current one, wrapping.
  function _nextAccount(): void {
    const accounts = root.service ? root.service.accounts : [];
    if (accounts.length === 0) return;

    const ids = accounts.map((a) => a.id);
    const at = ids.indexOf(root.accountId);
    root.setAccount(ids[(at + 1) % ids.length]);
  }

  // _move shifts the keyboard-selected contact by delta, wrapping at the
  // ends.
  function _move(delta: int): void {
    const count = root.contacts.length;
    if (count === 0) { root.currentIndex = -1; return; }

    root.currentIndex = ((root.currentIndex + delta) % count + count) % count;
  }

  // _accept opens the keyboard-selected contact's conversation, if any.
  function _accept(): void {
    const contact = root.contacts[root.currentIndex];
    if (!contact) return;

    root.openConversation(contact.accountId, contact.remoteId);
  }

  // _loadContacts asks the helper for accountId's contacts matching query.
  function _loadContacts(): void {
    if (!root.service || !root.accountId) { root.contacts = []; root.currentIndex = -1; return; }

    // The helper answers requests concurrently, so a reply for an earlier
    // search can arrive after the latest one; only the latest counts.
    const params = { accountId: root.accountId, query: root.query };
    const accountCurrent = Rpc.guard(() => root.accountId, params.accountId);
    const queryCurrent = Rpc.guard(() => root.query, params.query);
    root.service.request("contacts.list", params, function(error, result) {
      if (!accountCurrent() || !queryCurrent()) return;
      if (error) { root.lastError = Rpc.errorText(error); return; }
      root.contacts = result ?? [];
      root.currentIndex = root.contacts.length > 0 ? 0 : -1;
    });
  }
}
