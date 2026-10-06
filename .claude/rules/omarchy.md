## Omarchy and Quickshell Environment

Verified against Omarchy 4.0.4, Quickshell 0.3.1 and Qt 6.11. When a newer version disproves one of these, fix the rule.

### Plugin lifecycle

- The plugin is installed in `~/.config/omarchy/plugins/io.github.omamessenger/`. Never edit Omarchy's packaged source in `/usr/share/omarchy`
- Omarchy hot-reloads plugin code whenever a file under `~/.config/omarchy/plugins/` changes. Keep databases and runtime files outside it
- `manifest.json` declares three entry points: the service, the panel and the bar widget
- Omarchy loads the panel only while it is shown and destroys it on hide (`close()`); durable state belongs in `Service.qml`
- The panel receives `property var service` (our Service instance) and `property var shell`, a facade with `serviceFor(id)`, `hide(id)`, `toggle(id, json)` and `summon(id, json)`. `summon` calls `panel.open(payloadJson)` every time, even when the panel is already open
- The bar widget finds the service with `bar.shell.serviceFor("io.github.omamessenger")`, opens the window with `bar.shell.toggle("io.github.omamessenger", "{}")`, and receives plugin settings on its `settings` property; it forwards them with `service.applySettings(settings)`
- `Service.qml` must not declare `required` properties, or the plugin fails to load

### Quickshell

- Start the helper with `Process` (`stdinEnabled: true`, `write(line)`) and read its stdout with `stdout: SplitParser { onRead: … }`
- Resolve the plugin's own files with `Qt.resolvedUrl("../bin/oma-messenger-service")` and strip the `file://` prefix; this works for installed copies and symlinked dev checkouts
- `FloatingWindow` has no `requestActivate()`

### Tools

- `/usr/bin/qmllint` and `/usr/bin/qmltestrunner` are Qt 5. Use the Qt 6 tools in `/usr/lib/qt6/bin/`
- `make validate` checks the manifest and entry points on a staged copy of the plugin. Do not validate the repository itself: the validator rejects the symlinks that lint and tests leave under `build/`
