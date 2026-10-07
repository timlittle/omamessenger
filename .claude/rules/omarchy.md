## Omarchy and Quickshell Environment

Verified against Omarchy 4.0.4, Quickshell 0.3.1 and Qt 6.11. When a newer version disproves one of these, fix the rule.

### Plugin lifecycle

- The plugin is installed in `~/.config/omarchy/plugins/io.github.omamessenger/`. Never edit Omarchy's packaged source in `/usr/share/omarchy`
- Omarchy runs the shell with Quickshell's file watcher off (`QS_DISABLE_FILE_WATCHER=1` in `omarchy-launch-shell`). Copying plugin files makes Omarchy reload the plugin, but that reload keeps Quickshell's loaded QML, so new UI code only runs after `omarchy-restart-shell`. `make install-local` copies, waits for Omarchy's reload to finish, then restarts: never restart on top of a running reload, which has crashed Quickshell
- Omarchy watches `~/.config/omarchy/plugins` itself (`inotifywait` in `PluginRegistry.qml`) and reloads a plugin when any file in its folder changes, as does `omarchy-shell shell rescanPlugins`. A reload destroys the plugin's service, which stops the helper, and creates a new one, so never write runtime files into the plugin folder
- A reload while the shell is still starting has crashed Quickshell. To wait for a restarted shell, call the read-only `omarchy-shell shell listPlugins`, never `rescanPlugins`
- The panel may receive its `service` after it is created, or a replacement later. Controllers start up whenever `service` changes, not only in `Component.onCompleted`
- `manifest.json` declares three entry points: the service, the panel and the bar widget
- Omarchy loads the panel only while it is shown and destroys it on hide (`close()`); durable state belongs in `Service.qml`
- The panel receives `property var service` (our Service instance) and `property var shell`, a facade with `serviceFor(id)`, `hide(id)`, `toggle(id, json)` and `summon(id, json)`. `summon` calls `panel.open(payloadJson)` every time, even when the panel is already open
- The bar widget finds the service with `bar.shell.serviceFor("io.github.omamessenger")`, opens the window with `bar.shell.toggle("io.github.omamessenger", "{}")`, and receives plugin settings on its `settings` property; it forwards them with `service.applySettings(settings)`
- `Service.qml` must not declare `required` properties, or the plugin fails to load

### Quickshell

- Start the helper with `Process` (`stdinEnabled: true`, `write(line)`) and read its stdout with `stdout: SplitParser { onRead: … }`
- Resolve the plugin's own files with `Qt.resolvedUrl("../bin/oma-messenger-service")` and strip the `file://` prefix; this works for installed copies and symlinked dev checkouts
- `FloatingWindow` has no `requestActivate()`
- Hyprland 0.56 moved dispatch strings to a Lua call syntax (`hl.dsp.focus({ window = "title:…" })`); even syntactically valid calls can fail at runtime ("hl.focus: window not found") and the exact form is tied to the Hyprland version. To focus this plugin's own already-open window, do what Omarchy's own bar widget does to refocus any window: `import Quickshell.Wayland`, find the matching entry in `ToplevelManager.toplevels.values` (by title) and call `.activate()`. That goes through the Wayland foreign-toplevel protocol, which does not change with the compositor version

### Tools

- `/usr/bin/qmllint` and `/usr/bin/qmltestrunner` are Qt 5. Use the Qt 6 tools in `/usr/lib/qt6/bin/`
- `make validate` checks the manifest and entry points on a staged copy of the plugin. Do not validate the repository itself: the validator rejects the symlinks that lint and tests leave under `build/`
