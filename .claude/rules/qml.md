## QML

Baselines: the Qt [QML coding conventions](https://doc.qt.io/qt-6/qml-codingconventions.html) and [QML best practices](https://doc.qt.io/qt-6/qtquick-bestpractices.html), plus the patterns Omarchy's own shell uses (`/usr/share/omarchy/shell`). The rules below add to them.

### Structure

- `ui/Service.qml` owns the helper process and all durable state. Omarchy destroys the panel when it hides, so anything that must survive a hide lives in Service
- `ui/Panel.qml` composes the window and routes keys; it holds no protocol code
- `ui/service/`: helper process, RPC calls and the models they fill
- `ui/controllers/`: UI actions (open conversation, send, filter) that call the service
- `ui/components/`: views. A component takes data through properties and reports user intent through signals. It never calls the service or the helper
- `ui/lib/*.js`: pure logic (formatting, key mapping, list diffing), tested with node (see javascript.md)
- One component per file. Aim for ≤ 150 lines; move logic into `ui/lib` or split the view when it grows

### Code style

- 2-space indentation, matching Omarchy's shell
- Attribute order, one blank line between groups: `id`, property declarations, signal declarations, JS functions, object properties, child objects, states and transitions
- Every file starts with a comment saying what the component is for; every public property, signal and function has a one-line comment. Comments explain why, in plain English, with no plan IDs
- Typed properties (`string`, `int`, `bool`, `list<…>`, a component type); `var` only for JS objects that come from the helper
- Typed function signatures: `function select(id: string): void`
- `required property` for data a component cannot work without, except in `Service.qml`: Omarchy fails to load a service that declares one
- Prefer declarative bindings to imperative assignments; never break a binding by assigning to a bound property in JS
- Refer to other objects through their `id`, never through `parent` chains or unqualified names from an outer scope
- Group related properties: `font { pixelSize: …; bold: true }`, `anchors { fill: parent; margins: … }`
- In a Layout, size children with `Layout.*` properties, not `anchors` or `width`/`height`
- Keep state in models and Service, never only in a delegate: delegates are recycled
- Model rows are identified by id, never by index, so a reorder never moves the selection

### Theme

- Colours, spacing, fonts and radii come from Omarchy's `qs.Commons` (`Color`, `Style`, `Util`). No colour literals except `"transparent"`, and no hard-coded pixel sizes; use `Style.space(px)` so Omarchy's scaling applies
- Nested token groups (`Style.font`, `Style.spacing`, `Color.popups`, …) are invisible to qmllint, so read them through the typed adapter: `import "../theme"` then `Theme.font.body`, `Theme.spacing.md`. `ui/theme/Theme.qml` is the only file allowed to disable a qmllint warning
- Use `qs.Ui` controls (`Button`, `TextField`, `Dropdown`, `Toggle`, `Panel`, …) before writing your own
- Controls stay visually quiet until hovered, focused or selected. Use `Style.hoverFill` for hover and `Util.alpha(Color.accent, Style.selectedFillAlpha)` for selection
- Never rely on colour alone: pair it with a glyph, weight or label. Pair every icon glyph with a label or tooltip

### Keyboard and windows

- Every action is reachable from the keyboard and usable with the mouse. Shortcuts are defined once in `ui/lib/Keymap.js` and documented in the README
- Use a `FloatingWindow` so Hyprland manages the window like an app. No layer-shell overlay, no second Quickshell process
- `FloatingWindow` has no `requestActivate()`; calling it throws and aborts the function. Focus an open window with `Hyprland.dispatch("focuswindow …")`
- `open(payloadJson)` runs every time the panel is summoned, even when it is already open; it must be idempotent

### Gates

- `make lint` runs Qt 6 qmllint (`/usr/lib/qt6/bin/qmllint`, not the Qt 5 one in `/usr/bin`) with zero warnings allowed
- Visual or behaviour changes are checked with the offscreen harness (see ui-testing.md) and, when the user asks, in the installed plugin
