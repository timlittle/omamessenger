import QtQuick
import QtQuick.Controls
import qs.Commons
import "theme"
import "components"

// The OmaMessenger bar icon: the unread count across every conversation,
// dimmed until the helper is ready, and a left click that opens or closes
// the conversation window. Settings edited in Omarchy's bar editor are
// forwarded to the helper as soon as they change, not just when the panel
// next opens.
Item {
  id: root

  // bar is the scoped facade the host bar injects for a third-party
  // widget: bar.shell exposes serviceFor, summon, hide and toggle, each
  // already scoped to this plugin's own id.
  property var bar: null
  // settings holds this widget's bar-editor overrides: notifications,
  // notificationPreview and demoChatter, read from shell.json.
  property var settings: ({})

  // pluginId names this plugin to bar.shell, matching manifest.json.
  readonly property string pluginId: "io.github.omamessenger"
  // service is the plugin's long-lived Service instance, looked up
  // through the bar facade rather than injected directly: a bar widget
  // only ever gets bar, moduleName and settings from the host.
  readonly property var service: root.bar && root.bar.shell
    ? root.bar.shell.serviceFor(root.pluginId) : null
  // ready mirrors the helper connection; the icon dims until it is.
  readonly property bool ready: root.service ? root.service.status === "ready" : false
  // unreadTotal is the unread count across every conversation.
  readonly property int unreadTotal: root.service ? root.service.unreadTotal : 0
  // demo is true while the helper serves seeded demo data.
  readonly property bool demo: root.service ? root.service.demo : false
  // tooltipText names the plugin, its unread count and, in demo mode, says so.
  readonly property string tooltipText: "OmaMessenger · " + root.unreadTotal + " unread"
    + (root.demo ? " · demo" : "")

  implicitWidth: Theme.bar.iconSlot
  implicitHeight: Theme.bar.iconSlot
  opacity: root.ready ? 1 : 0.4

  // _forwardSettings sends the bar-editor settings to the helper so a
  // change there takes effect immediately rather than waiting for the
  // panel to be summoned next. Runs on both triggers because the host
  // sets bar and settings independently, and either can arrive first.
  function _forwardSettings(): void {
    if (root.service && typeof root.service.applySettings === "function")
      root.service.applySettings(root.settings);
  }

  onBarChanged: root._forwardSettings()
  onSettingsChanged: root._forwardSettings()

  Rectangle {
    anchors.fill: parent
    radius: Theme.spacing.xs
    color: hover.containsMouse ? Style.hoverFill : "transparent"
  }

  ServiceGlyph {
    id: glyph
    anchors.centerIn: parent
    service: ""
  }

  UnreadBadge {
    id: badge
    objectName: "unreadBadge"
    count: root.unreadTotal
    anchors { top: parent.top; right: parent.right }
  }

  MouseArea {
    id: hover
    anchors.fill: parent
    hoverEnabled: true
    onClicked: {
      if (root.bar && root.bar.shell) root.bar.shell.toggle(root.pluginId, "{}");
    }
  }

  ToolTip.visible: hover.containsMouse
  ToolTip.text: root.tooltipText
  ToolTip.delay: 500
}
