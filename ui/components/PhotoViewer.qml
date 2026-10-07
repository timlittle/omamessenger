import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Format.js" as Format

// Shows a conversation photo over the whole window. Omarchy's Hyprland
// window rule floats the external image viewer too small to reach with
// the keyboard, and focus stays on this window, so a photo opened there
// gets stuck with no way to close it; this shows it here instead, where
// Escape, the window's own close button and every other shortcut still
// work. While the full image is still downloading it shows the same
// blurred preview the message bubble already has.
Item {
  id: root

  // open shows the viewer when true.
  property bool open: false
  // photo is the open message's media: kind, width, height and thumb, or
  // null while nothing is open.
  property var photo: null
  // path is where the full image was downloaded, or "" until then.
  property string path: ""
  // routeKey is the panel's key router, so Escape and the photo-stepping
  // keys work while this holds keyboard focus, the same as every other
  // full-window dialog here.
  property var routeKey: null

  // closed asks the caller to leave the viewer: the close button, or a
  // click on the backdrop outside the photo.
  signal closed()
  // openExternally asks the caller to open the photo in the user's own
  // image viewer and leave the in-app view.
  signal openExternally()

  objectName: "photoViewer"
  anchors.fill: parent
  visible: root.open
  focus: true

  Keys.priority: Keys.BeforeItem
  Keys.onPressed: event => {
    if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
  }

  // Focus waits a turn of the event loop: an item still hidden when open
  // changes cannot take focus.
  onOpenChanged: if (root.open) Qt.callLater(root.forceActiveFocus)

  Rectangle {
    id: backdrop
    objectName: "backdrop"
    anchors.fill: parent
    color: Theme.menu.scrim

    MouseArea {
      anchors.fill: parent
      onClicked: root.closed()
    }
  }

  Item {
    id: stage
    anchors.fill: parent
    anchors.margins: Theme.spacing.xl

    Image {
      id: image
      objectName: "photoImage"
      anchors.fill: parent
      source: root.path ? "file://" + root.path : (root.photo && root.photo.thumb ? "data:image/jpeg;base64," + root.photo.thumb : "")
      fillMode: Image.PreserveAspectFit
      asynchronous: true
      // Scaling a tiny preview up smoothly keeps it a soft blur, same as
      // the bubble's own photo view.
      smooth: true

      // Clicks on the photo itself stay on the photo; only the backdrop
      // around it, sized to the painted image rather than this item's
      // whole letterboxed bounds, closes the viewer.
      MouseArea {
        anchors.centerIn: parent
        width: image.paintedWidth
        height: image.paintedHeight
      }
    }

    Text {
      objectName: "loadingLabel"
      anchors.centerIn: parent
      visible: root.path === ""
      text: "Loading…"
      color: Util.alpha(Color.foreground, 0.7)
      font { family: Theme.font.family; pixelSize: Theme.font.body }
    }
  }

  RowLayout {
    anchors { top: parent.top; left: parent.left; right: parent.right; margins: Theme.spacing.md }
    spacing: Theme.spacing.sm

    Text {
      objectName: "sizeLabel"
      Layout.fillWidth: true
      text: root.photo ? Format.photoSize(root.photo.width, root.photo.height) : ""
      color: Util.alpha(Color.foreground, 0.7)
      font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
    }

    Ui.Button {
      objectName: "openExternalButton"
      text: "Open in image viewer"
      tooltipText: "Open in image viewer"
      focusable: true
      onClicked: root.openExternally()
    }

    Ui.Button {
      objectName: "closeButton"
      text: "✕"
      tooltipText: "Close"
      focusable: true
      onClicked: root.closed()
    }
  }
}
