pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"

// The health check report from the helper's helper.doctor method: one
// line per check, a checkmark or a cross paired with its name and safe
// detail text, so a problem never relies on colour alone. Opened from
// the command palette's "Run health check"; Esc or the Close button
// dismiss it. A view only: it reports intent through a signal.
Item {
  id: root

  // open shows the report when true.
  property bool open: false
  // checks are the report's rows: [{name, ok, detail}], in the order the
  // helper ran them.
  property var checks: []

  // closed fires when the report should be dismissed.
  signal closed()

  objectName: "doctorReport"
  anchors.fill: parent
  visible: root.open

  ModalCard {
    id: modal

    cardWidth: Style.space(420)
    onOutsideClicked: root.closed()

    ColumnLayout {
      anchors.fill: parent
      spacing: Theme.spacing.md

      Text {
        text: "Health check"
        color: Color.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.subtitle; weight: Font.DemiBold }
      }

      ColumnLayout {
        Layout.fillWidth: true
        spacing: Theme.spacing.xs

        Repeater {
          objectName: "doctorChecks"
          model: root.checks

          RowLayout {
            id: row
            required property var modelData

            Layout.fillWidth: true
            spacing: Theme.spacing.sm

            Text {
              objectName: "doctorCheckGlyph"
              text: row.modelData.ok ? "✓" : "✗"
              color: row.modelData.ok ? Util.alpha(Color.foreground, 0.6) : Color.urgent
              font { family: Theme.font.family; pixelSize: Theme.font.body; bold: !row.modelData.ok }
            }

            Text {
              objectName: "doctorCheckText"
              Layout.fillWidth: true
              text: row.modelData.name + (row.modelData.detail ? ": " + row.modelData.detail : "")
              wrapMode: Text.WordWrap
              color: row.modelData.ok ? Color.foreground : Color.urgent
              font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
            }
          }
        }
      }

      Ui.Button {
        objectName: "doctorCloseButton"
        Layout.alignment: Qt.AlignRight
        text: "Close"
        focusable: true
        onClicked: root.closed()
      }
    }
  }
}
