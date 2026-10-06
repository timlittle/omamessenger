import QtQuick

// Cross-panel state that must survive the panel being destroyed when
// Omarchy hides it: the accounts the helper reported, the unread total,
// demo mode, the helper's protocol version, and the UI state the panel
// restores when it reopens.
QtObject {
  id: root

  // accounts is the signed-in accounts the helper reported.
  property var accounts: []

  // unreadTotal is the unread count shown on the bar widget.
  property int unreadTotal: 0

  // demo is true while the helper serves seeded demo data.
  property bool demo: false

  // version is the helper's protocol version, from hello.
  property int version: 0

  // uiState is what the panel restores after Omarchy destroys and
  // recreates it: the rail filter, selection, open pane, search text and
  // unsent drafts keyed by conversation id.
  property var uiState: ({
    railKey: "all",
    selectedId: "",
    activeId: "",
    pane: "list",
    query: "",
    drafts: {}
  })

  // applyHello stores the fields the hello response carries.
  function applyHello(result: var): void {
    root.demo = !!result.demo;
    root.version = result.protocol || 0;
    root.unreadTotal = result.unreadTotal || 0;
  }

  // applyAccountUpdated replaces one account in the list, or appends it
  // when it is new.
  function applyAccountUpdated(account: var): void {
    const next = root.accounts.filter((a) => a.id !== account.id);
    next.push(account);
    root.accounts = next;
  }

  // applyUnreadChanged stores the new unread total from an unread.changed
  // event.
  function applyUnreadChanged(data: var): void {
    root.unreadTotal = data.total || 0;
  }
}
