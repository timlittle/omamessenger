pragma ComponentBehavior: Bound
import QtQuick
import qs.Commons
import qs.Ui as Ui
import "../theme"

// The small toolbar that floats over a hovered message's top corner:
// buttons to add a reaction, reply and delete. The delegate places this
// item outside its message's own Column and fades it in with opacity, so
// showing or hiding it never changes any message's size or position.
// active is driven by the delegate from a hover state that covers both
// the bubble and this toolbar, so moving the pointer onto a button never
// hides it.
Item {
  id: root

  // active shows the toolbar, fading it in.
  property bool active: false
  // mediaActions shows the copy, save and open-externally buttons,
  // for a message whose photo or video this toolbar can act on.
  property bool mediaActions: false
  // hitSize is each button's clickable square: bigger than the glyph
  // itself, so the react and reply targets are easy to land a mouse on.
  readonly property real hitSize: Style.space(28)

  // react asks the caller to open the emoji picker for this message.
  signal react()
  // reply asks the caller to start replying to this message.
  signal reply()
  // deleteRequested asks the caller to open the delete question for
  // this message.
  signal deleteRequested()
  // copyRequested asks the caller to copy this message's photo or
  // video to the clipboard.
  signal copyRequested()
  // saveRequested asks the caller to save this message's photo or
  // video into the downloads directory.
  signal saveRequested()
  // openExternalRequested asks the caller to open this message's photo
  // or video in the user's own application, bypassing the in-app
  // viewer or player.
  signal openExternalRequested()

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
        size: root.hitSize
        onClicked: root.react()
      }

      Ui.PanelActionButton {
        objectName: "replyButton"
        iconText: "↩"
        tooltipText: "Reply"
        size: root.hitSize
        onClicked: root.reply()
      }

      Ui.PanelActionButton {
        objectName: "copyButton"
        visible: root.mediaActions
        iconText: "⧉"
        tooltipText: "Copy"
        size: root.hitSize
        onClicked: root.copyRequested()
      }

      Ui.PanelActionButton {
        objectName: "saveButton"
        visible: root.mediaActions
        iconText: "⇩"
        tooltipText: "Save"
        size: root.hitSize
        onClicked: root.saveRequested()
      }

      Ui.PanelActionButton {
        objectName: "openExternalButton"
        visible: root.mediaActions
        iconText: "↗"
        tooltipText: "Open externally"
        size: root.hitSize
        onClicked: root.openExternalRequested()
      }

      Ui.PanelActionButton {
        objectName: "deleteButton"
        iconText: "🗑"
        tooltipText: "Delete"
        size: root.hitSize
        onClicked: root.deleteRequested()
      }
    }
  }
}
