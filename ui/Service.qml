import QtQuick
import Quickshell
import Quickshell.Io
import "lib/Settings.js" as Settings
import "lib/Keymap.js" as Keymap
import "lib/KeyBindings.js" as KeyBindings
import "lib/WhatsNew.js" as WhatsNew
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

  // status mirrors the helper process: starting, ready, stopped, error,
  // missing, installing or installFailed. See ui/service/HelperProcess.qml.
  readonly property string status: helperProcess.status

  // detail is the last diagnostic line from the helper, shown beside the
  // status.
  readonly property string detail: helperProcess.detail


  // unreadTotal is the unread count across every conversation.
  readonly property int unreadTotal: appState.unreadTotal

  // unreadByService is the unread count for each service that has any,
  // keyed by service id, for the bar tooltip's per-service breakdown.
  readonly property var unreadByService: appState.unreadByService

  // accounts is the signed-in accounts the helper reported.
  readonly property var accounts: appState.accounts

  // services are the messaging services the helper can add an account
  // for, from hello. Empty for a helper too old to report them.
  readonly property var services: appState.services

  // appVersion is the running helper's own release, used as this
  // plugin's version for the "What's new" overlay and update hint; see
  // AppState.helperVersion for why the helper's version stands in for
  // the plugin's own.
  readonly property string appVersion: appState.helperVersion

  // showUpdateHint is true for the one run after an update where the
  // quiet "Updated to vX" hint should show in the window.
  readonly property bool showUpdateHint: appState.showUpdateHint

  // whatsNewReleases are the committed whats-new.json's releases,
  // newest first, for the "What's new" palette command's overlay.
  readonly property var whatsNewReleases: appState.whatsNewReleases

  // pendingAuth is the sign-in step an account is waiting on, or null.
  readonly property var pendingAuth: appState.pendingAuth

  // readReceipts mirrors the last readReceipts value sent to the helper,
  // true until applySettings or toggleReadReceipts says otherwise: off
  // means incognito mode, so the footer can show it quietly and the
  // palette's "Toggle read receipts" command knows which way to flip.
  property bool readReceipts: true

  // _lastSettings is the plugin settings object last given to
  // applySettings, kept so toggleReadReceipts can resend every other
  // setting unchanged alongside the one it flips.
  property var _lastSettings: ({})

  // shell is the host facade Omarchy injects after creating this service,
  // used to summon the panel open on a conversation when a notification is
  // clicked. Never `required`: Quickshell refuses to load a service that
  // declares one, and the host sets it after construction anyway.
  property var shell: null

  // uiState is the panel's durable state: rail filter, selection, open
  // pane, search text and drafts. It is an alias onto AppState's own
  // property so the panel can read and write it directly and still have
  // it survive the panel being destroyed on hide.
  property alias uiState: appState.uiState

  // effectiveBindings are Keymap.BINDINGS with ~/.config/omamessenger/
  // keys.conf's overrides merged in; every key-aware view reads this
  // instead of the defaults directly. Starts as the plain defaults and
  // updates once the file's first read, or any later change, finishes.
  property var effectiveBindings: Keymap.BINDINGS

  // keyBindingConflicts/keyBindingErrors are keys.conf's own report: a
  // conflict is two actions that ended up wanting the same key, where
  // the default won; an error is a line that named an unknown action or
  // gave no valid key. Both are empty with no file, or while it is
  // still loading.
  property var keyBindingConflicts: []
  property var keyBindingErrors: []

  // _keyConfigPath is where keys.conf lives, following XDG_CONFIG_HOME.
  readonly property string _keyConfigPath: KeyBindings.configPath(Quickshell.env("HOME"), Quickshell.env("XDG_CONFIG_HOME"))

  // _whatsNewPath is the committed whats-new.json shipped alongside this
  // plugin's own files, resolved the same way HelperProcess resolves
  // the launcher and installer scripts.
  readonly property string _whatsNewPath: String(Qt.resolvedUrl("../whats-new.json")).replace("file://", "")

  // _keyConfigMissing is true once the first read has confirmed the
  // file does not exist yet, so openKeyConfigFile knows to seed it.
  property bool _keyConfigMissing: false

  // event is emitted for every notification the helper sends, after it
  // has been applied to the state above.
  signal event(string name, var data)

  // request sends method with params and calls callback(error, result)
  // once the helper replies.
  function request(method: string, params: var, callback: var): void {
    rpcClient.request(method, params, callback);
  }

  // installHelper downloads the pinned helper release and starts it. The
  // panel calls this itself the first time it opens with the helper
  // missing or out of date; it is also the Retry button's action after a
  // failed install.
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

  // applySettings forwards the plugin's settings to the helper. The
  // fallback passed to Settings.withDefaults is root.readReceipts, its
  // own current value, not the manifest default: Omarchy only sends a
  // bar widget the settings the user has changed there, so an unrelated
  // change (a mute edit, a plugin reload) would otherwise re-forward
  // {} and silently undo a readReceipts flip the palette's own command
  // just made. An explicit readReceipts from Omarchy itself still wins.
  function applySettings(settings: var): void {
    root._lastSettings = settings ?? {};
    const merged = Settings.withDefaults(settings, root.readReceipts);
    root.readReceipts = merged.readReceipts;
    root.request("settings.apply", merged, function() {});
  }

  // toggleReadReceipts flips incognito read receipts and tells the
  // helper at once, for the command palette's "Toggle read receipts".
  function toggleReadReceipts(): void {
    root.applySettings(Object.assign({}, root._lastSettings, { readReceipts: !root.readReceipts }));
  }

  // openKeyConfigFile is the palette's "Open key bindings file" command:
  // it seeds keys.conf with a commented template the first time it is
  // asked for (never overwriting one that already exists), then opens
  // it in the user's own editor through xdg-open, the same way every
  // other externally-opened file in this UI does.
  function openKeyConfigFile(): void {
    if (root._keyConfigMissing) keysFile.setText(KeyBindings.template(Keymap.BINDINGS));

    Qt.openUrlExternally("file://" + root._keyConfigPath);
  }

  // _applyKeyConfig re-merges keys.conf's text into effectiveBindings,
  // called on every load, whether the first one or a later reload.
  function _applyKeyConfig(text: string): void {
    const parsed = KeyBindings.parseConfig(text);
    const merged = KeyBindings.merge(Keymap.BINDINGS, parsed.overrides);

    root.effectiveBindings = merged.bindings;
    root.keyBindingConflicts = merged.conflicts;
    root.keyBindingErrors = parsed.errors;
  }

  // _sayHello greets the helper once it is ready and loads the accounts.
  function _sayHello(): void {
    root.request("hello", {}, function(error, result) {
      if (error) return;

      appState.applyHello(result);
      root._checkWhatsNew();
      root.request("accounts.list", {}, function(listError, accounts) {
        if (listError) return;
        appState.accounts = accounts;
      });
    });
  }

  // _checkWhatsNew asks the helper which "What's new" version the user
  // has already seen, decides whether the quiet update hint should show
  // this run, then records the running version as seen so it never
  // shows again for it. Asking the helper again after a reconnect within
  // the same Service lifetime is harmless: by then it already holds the
  // version this call itself just recorded, so shouldShowHint says no
  // the second time.
  function _checkWhatsNew(): void {
    root.request("whatsNew.getSeenVersion", {}, function(error, result) {
      if (error) return;

      appState.showUpdateHint = WhatsNew.shouldShowHint(appState.helperVersion, result.version);
      root.request("whatsNew.setSeenVersion", { version: appState.helperVersion }, function() {});
    });
  }

  // _handleEvent applies a notification to the durable state before
  // forwarding it to the panel.
  function _handleEvent(name: string, data: var): void {
    if (name === "account.updated") appState.applyAccountUpdated(data);
    else if (name === "account.removed") appState.applyAccountRemoved(data);
    else if (name === "auth.step") appState.applyAuthStep(data);
    else if (name === "unread.changed") appState.applyUnreadChanged(data);
    else if (name === "notification.clicked") root._openOnNotificationClick(data);

    root.event(name, data);
  }

  // _openOnNotificationClick summons the panel open on the conversation a
  // clicked notification carried, the same way the bar widget summons it.
  // Without a shell facade (a host too old to inject one, or a test) this
  // does nothing; the event still reaches the panel through root.event.
  function _openOnNotificationClick(data: var): void {
    if (!root.shell || typeof root.shell.summon !== "function") return;
    if (!data || typeof data.conversationId !== "string" || !data.conversationId) return;

    root.shell.summon("io.github.omamessenger", JSON.stringify({ conversationId: data.conversationId }));
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

  // keysFile holds ~/.config/omamessenger/keys.conf's overrides. Watched
  // so editing it takes effect live, the same way Omarchy's own shell
  // watches a user config file; QS_DISABLE_FILE_WATCHER (set when
  // Omarchy launches the shell) only turns off reloading this plugin's
  // own QML on a source change, not a FileView's own file watch.
  FileView {
    id: keysFile
    path: root._keyConfigPath
    watchChanges: true
    printErrors: false

    onLoaded: {
      root._keyConfigMissing = false;
      root._applyKeyConfig(text());
    }
    onLoadFailed: function(error) {
      root._keyConfigMissing = error === FileViewError.FileNotFound;
      root._applyKeyConfig("");
    }
    // text() is stale inside fileChanged itself; reload() re-reads the
    // file fresh and reports back through onLoaded/onLoadFailed.
    onFileChanged: reload()
  }

  // whatsNewFile holds the committed whats-new.json shipped with the
  // plugin. WhatsNew.parse never throws, so malformed JSON just means
  // an empty "What's new" list rather than a broken overlay; no watch,
  // since it only ever changes with the plugin's own files, which is
  // already a full reload.
  FileView {
    id: whatsNewFile
    path: root._whatsNewPath
    printErrors: false

    onLoaded: appState.whatsNewReleases = WhatsNew.parse(text())
    onLoadFailed: appState.whatsNewReleases = []
  }
}
