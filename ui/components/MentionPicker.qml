pragma ComponentBehavior: Bound
import QtQuick
import qs.Commons
import "../theme"

// The @-mention picker: a short list of a group's members matching what
// was typed after "@", shown above the composer's text field while it
// is active. Up/Down move the keyboard highlight (see Keymap.js's
// mentionPicker context); clicking a row reaches the same effect by
// mouse. It takes data through properties and reports the choice
// through a signal, same as any other component: Composer.qml owns the
// filtering and the key routing that drives highlightedIndex.
Rectangle {
  id: root

  // members are the filtered matches to show.
  property var members: []
  // highlightedIndex is which member the keyboard currently points at.
  property int highlightedIndex: 0

  // accepted reports the member at index was chosen.
  signal accepted(int index)

  objectName: "mentionPicker"
  implicitWidth: Style.space(220)
  implicitHeight: list.implicitHeight + Theme.spacing.xxs * 2
  radius: Style.cornerRadius
  color: Style.normalFill
  border.width: Theme.spacing.hairline
  border.color: Util.alpha(Color.foreground, 0.15)
  clip: true

  Column {
    id: list
    y: Theme.spacing.xxs
    width: parent.width

    Repeater {
      model: root.members

      Rectangle {
        id: row
        required property var modelData
        required property int index

        readonly property bool current: row.index === root.highlightedIndex

        objectName: "mentionRow-" + row.index
        width: list.width
        height: label.implicitHeight + Theme.spacing.xs * 2
        color: row.current ? Util.alpha(Color.accent, Style.selectedFillAlpha)
          : hover.containsMouse ? Style.hoverFill : "transparent"

        Text {
          id: label
          anchors.verticalCenter: parent.verticalCenter
          x: Theme.spacing.sm
          text: row.modelData.name
          color: Color.foreground
          font.family: Theme.font.family
          font.pixelSize: Theme.font.body
        }

        MouseArea {
          id: hover
          objectName: "mentionRowArea-" + row.index
          anchors.fill: parent
          hoverEnabled: true
          cursorShape: Qt.PointingHandCursor
          onClicked: root.accepted(row.index)
        }
      }
    }
  }
}
