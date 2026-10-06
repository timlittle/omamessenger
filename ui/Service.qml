import QtQuick
import "lib/Settings.js" as Settings
import "service"

// Owns the helper process and everything that must survive the panel
// being destroyed when Omarchy hides it: the JSON-RPC connection, the
// accounts and unread total it reports, and the UI state the panel
// restores when it reopens.
//
// Quickshell refuses to load a service plugin that declares a required
// property, so nothing here is required.
//
// Item rather than QtObject: only a type with a default property can hold
// the AppState, HelperProcess and RpcClient children below without naming
// a property for them.
Item {
  id: root

  // status mirrors the helper process: starting, ready, stopped, error
  // or missing.
  readonly property string status: helperProcess.status

  // detail is the last diagnostic line from the helper, shown beside the
  // status.
  readonly property string detail: helperProcess.detail


  // unreadTotal is the unread count across every conversation.
  readonly property int unreadTotal: appState.unreadTotal

  // accounts is the signed-in accounts the helper reported.
  readonly property var accounts: appState.accounts

  // uiState is the panel's durable state: rail filter, selection, open
  // pane, search text and drafts. It is an alias onto AppState's own
  // property so the panel can read and write it directly and still have
  // it survive the panel being destroyed on hide.
  property alias uiState: appState.uiState

  // event is emitted for every notification the helper sends, after it
  // has been applied to the state above.
  signal event(string name, var data)

  // request sends method with params and calls callback(error, result)
  // once the helper replies.
  function request(method: string, params: var, callback: var): void {
    rpcClient.request(method, params, callback);
  }

  // installHelper downloads the pinned helper release and starts it.
  function installHelper(): void {
    helperProcess.install();
  }

  // start runs the helper again after quit().
  function start(): void {
    helperProcess.start();
  }

  // quit stops the helper, and with it every notification, until start().
  function quit(): void {
    helperProcess.stop();
  }

  // applySettings forwards the plugin's settings to the helper.
  function applySettings(settings: var): void {
    root.request("settings.apply", Settings.withDefaults(settings), function() {});
  }

  // _sayHello greets the helper once it is ready and loads the accounts.
  function _sayHello(): void {
    root.request("hello", {}, function(error, result) {
      if (error) return;

      appState.applyHello(result);
      root.request("accounts.list", {}, function(listError, accounts) {
        if (listError) return;
        appState.accounts = accounts;
      });
    });
  }

  // _handleEvent applies a notification to the durable state before
  // forwarding it to the panel.
  function _handleEvent(name: string, data: var): void {
    if (name === "account.updated") appState.applyAccountUpdated(data);
    else if (name === "unread.changed") appState.applyUnreadChanged(data);

    root.event(name, data);
  }

  AppState {
    id: appState
  }

  HelperProcess {
    id: helperProcess

    onStatusChanged: {
      if (helperProcess.status === "ready") root._sayHello();
      else rpcClient.cancelPending();
    }
  }

  RpcClient {
    id: rpcClient
    transport: helperProcess

    onEvent: function(name, data) { root._handleEvent(name, data); }
  }
}
