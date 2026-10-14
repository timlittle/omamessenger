import QtQuick
import qs.Commons
import "theme"
import "components"
import "lib/Tooltip.js" as Tooltip

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
  // settings holds this widget's bar-editor overrides: notifications and
  // notificationDetail, read from shell.json.
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
  // tooltipText names the plugin, its total unread count, unread per
  // service, and any account that is not connected.
  readonly property string tooltipText: Tooltip.format({
    total: root.unreadTotal,
    byService: root.service ? root.service.unreadByService : ({}),
    services: root.service ? root.service.services : [],
    accounts: root.service ? root.service.accounts : []
  })

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

  // _showTooltip asks the host bar to show its own shared tooltip bubble
  // over this widget. Omarchy's bar widgets never draw a tooltip
  // themselves (a plain QtQuick.Controls ToolTip does not size itself to
  // this widget's small bar slot); they ask the bar to show its own, the
  // same way Ui.WidgetButton does on hover.
  function _showTooltip(): void {
    if (root.bar) root.bar.showTooltip(root, root.tooltipText);
  }

  onBarChanged: root._forwardSettings()
  onSettingsChanged: root._forwardSettings()
  // The unread count changes while the pointer stays put (a message
  // arrives, settings are forwarded); re-show the tooltip so its text
  // stays current instead of the one shown when the pointer first
  // entered, since Omarchy's bar sets its tooltip text once per show.
  onTooltipTextChanged: if (hover.containsMouse) root._showTooltip()

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
    onEntered: root._showTooltip()
    onExited: if (root.bar) root.bar.hideTooltip(root)
    onClicked: {
      if (root.bar && root.bar.shell) root.bar.shell.toggle(root.pluginId, "{}");
    }
  }
}
