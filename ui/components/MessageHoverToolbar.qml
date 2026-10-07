pragma ComponentBehavior: Bound
import QtQuick
import qs.Commons
import qs.Ui as Ui
import "../theme"

// The small toolbar that floats over a hovered message's top corner: a
// button to add a reaction and one to reply. The delegate places this
// item outside its message's own Column and fades it in with opacity, so
// showing or hiding it never changes any message's size or position.
// active is driven by the delegate from a hover state that covers both
// the bubble and this toolbar, so moving the pointer onto a button never
// hides it.
Item {
  id: root

  // active shows the toolbar, fading it in.
  property bool active: false

  // react asks the caller to open the emoji picker for this message.
  signal react()
  // reply asks the caller to start replying to this message.
  signal reply()

  objectName: "hoverToolbar"
  implicitWidth: surface.implicitWidth
  implicitHeight: surface.implicitHeight
  width: root.implicitWidth
  height: root.implicitHeight
  opacity: root.active ? 1 : 0
  visible: root.opacity > 0

  Behavior on opacity {
    NumberAnimation { duration: 120 }
  }

  Ui.BorderSurface {
    id: surface

    anchors.fill: parent
    implicitWidth: row.implicitWidth + Theme.spacing.xxs * 2
    implicitHeight: row.implicitHeight + Theme.spacing.xxs * 2
    color: Theme.popups.background
    borderSpec: Border.flat(Theme.popups.border, Style.normalBorderWidth)
    radius: Style.cornerRadius

    Row {
      id: row
      anchors.centerIn: parent
      spacing: Theme.spacing.xxs

      Ui.PanelActionButton {
        objectName: "reactButton"
        iconText: "+"
        tooltipText: "Add reaction"
        onClicked: root.react()
      }

      Ui.PanelActionButton {
        objectName: "replyButton"
        iconText: "↩"
        tooltipText: "Reply"
        onClicked: root.reply()
      }
    }
  }
}
