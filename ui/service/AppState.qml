import QtQuick

// Cross-panel state that must survive the panel being destroyed when
// Omarchy hides it: the accounts the helper reported, any sign-in in
// progress, the unread total, the helper's protocol version, and the UI
// state the panel restores when it reopens.
QtObject {
  id: root

  // accounts is the signed-in accounts the helper reported.
  property var accounts: []

  // unreadTotal is the unread count shown on the bar widget.
  property int unreadTotal: 0

  // unreadByService is the unread count for each service that has any,
  // keyed by service id ("whatsapp", "telegram"), for the bar tooltip's
  // per-service breakdown. Empty for a helper too old to report it.
  property var unreadByService: ({})

  // pendingAuth is the sign-in step an account is waiting on, from the
  // last auth.step, or null. It lives here so account setup can show it
  // again after the panel is recreated.
  property var pendingAuth: null

  // version is the helper's protocol version, from hello.
  property int version: 0

  // helperVersion is the helper's own release, such as "0.3.5", from
  // hello. Kept in step with manifest.json's version by release-please
  // (see release-please-config.json's extra-files), so it doubles as
  // the running plugin's own version for the "What's new" update hint.
  property string helperVersion: ""

  // services are the messaging services the helper can add an account
  // for, from hello. Empty for a helper too old to report them.
  property var services: []

  // showUpdateHint is true for the one run after an update where the
  // quiet "Updated to vX" hint should show, set once Service.qml has
  // asked the helper which version the user last saw (see
  // Service._checkWhatsNew). Stays false for a fresh install.
  property bool showUpdateHint: false

  // whatsNewReleases are the committed whats-new.json's releases,
  // newest first, parsed by Service.qml's own FileView; empty until
  // that first read finishes, or if the file is missing or malformed.
  property var whatsNewReleases: []

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
    root.version = result.protocol || 0;
    root.helperVersion = result.version || "";
    root.unreadTotal = result.unreadTotal || 0;
    root.unreadByService = result.unreadByService || {};
    root.services = result.services || [];
  }

  // applyAccountUpdated replaces one account in the list, or appends it
  // when it is new.
  function applyAccountUpdated(account: var): void {
    const next = root.accounts.filter((a) => a.id !== account.id);
    next.push(account);
    root.accounts = next;
    if (account.status === "connected") root._clearPendingAuth(account.id);
  }

  // applyAccountRemoved drops a removed account and any sign-in it was
  // waiting on.
  function applyAccountRemoved(data: var): void {
    root.accounts = root.accounts.filter((a) => a.id !== data.accountId);
    root._clearPendingAuth(data.accountId);
  }

  // applyAuthStep records the sign-in step an account is waiting on.
  function applyAuthStep(step: var): void {
    root.pendingAuth = step;
  }

  // _clearPendingAuth forgets accountId's sign-in step once it is over.
  function _clearPendingAuth(accountId: string): void {
    if (root.pendingAuth && root.pendingAuth.accountId === accountId) root.pendingAuth = null;
  }

  // applyUnreadChanged stores the new unread total and per-service
  // breakdown from an unread.changed event.
  function applyUnreadChanged(data: var): void {
    root.unreadTotal = data.total || 0;
    root.unreadByService = data.byService || {};
  }
}
