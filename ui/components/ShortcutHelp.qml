pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import "../theme"
import "../lib/Keymap.js" as Keymap

// Shortcut help overlay: every key binding, grouped by context, laid out
// in columns. Toggled by the caller through the open property.
Item {
  id: root

  // open shows the overlay when true.
  property bool open: false

  // closed reports that the overlay should be dismissed.
  signal closed()

  anchors.fill: parent
  visible: root.open

  Rectangle {
    anchors.fill: parent
    color: Theme.menu.scrim

    MouseArea {
      anchors.fill: parent
      onClicked: root.closed()
    }

    Rectangle {
      anchors.centerIn: parent
      width: Math.min(parent.width - Theme.spacing.xxl * 2, Style.space(640))
      height: Math.min(parent.height - Theme.spacing.xxl * 2, Style.space(420))
      radius: Style.cornerRadius
      color: Theme.popups.background
      border { color: Theme.popups.border; width: Style.normalBorderWidth }

      MouseArea {
        anchors.fill: parent
        onClicked: () => {}
      }

      Flow {
        anchors.fill: parent
        anchors.margins: Theme.spacing.panelPadding
        spacing: Theme.spacing.xxl

        Repeater {
          model: Keymap.helpSections()

          ColumnLayout {
            id: section
            required property var modelData

            spacing: Theme.spacing.xs

            Text {
              text: section.modelData.title
              color: Color.accent
              font.family: Theme.font.family
              font.pixelSize: Theme.font.subtitle
              font.weight: Font.DemiBold
            }

            Repeater {
              model: section.modelData.rows

              RowLayout {
                id: row
                required property var modelData

                spacing: Theme.spacing.md

                Text {
                  Layout.preferredWidth: Style.space(120)
                  text: row.modelData.keys
                  color: Util.alpha(Color.foreground, 0.6)
                  font.family: Theme.font.family
                  font.pixelSize: Theme.font.bodySmall
                }

                Text {
                  text: row.modelData.label
                  color: Color.foreground
                  font.family: Theme.font.family
                  font.pixelSize: Theme.font.bodySmall
                }
              }
            }
          }
        }
      }
    }
  }
}
