pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"

// The in-app "What's new" list from whats-new.json: one section per
// release, newest first, with its version, date and short change lines.
// Opened from the command palette's "What's new" command; j/k or the
// arrow keys scroll (see Keymap.js's "whatsNew" context), Esc or the
// Close button dismiss it. A view only: it takes the parsed releases
// and the owning controller's scroll position through properties and
// reports intent through a signal.
Item {
  id: root

  // open shows the overlay when true.
  property bool open: false
  // releases are whats-new.json's own entries, newest first:
  // [{version, date, changes}].
  property var releases: []
  // highlightedIndex is the release currently scrolled to, moved by
  // the owning controller's j/k or arrow handling.
  property int highlightedIndex: 0

  // closed fires when the overlay should be dismissed.
  signal closed()

  objectName: "whatsNewOverlay"
  anchors.fill: parent
  visible: root.open
  onHighlightedIndexChanged: list.positionViewAtIndex(root.highlightedIndex, ListView.Contain)

  ModalCard {
    id: modal

    cardWidth: Style.space(440)
    cardHeight: Style.space(420)
    atTop: true
    onOutsideClicked: root.closed()

    ColumnLayout {
      anchors.fill: parent
      spacing: Theme.spacing.md

      Text {
        text: "What's new"
        textFormat: Text.PlainText
        color: Theme.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.subtitle; weight: Font.DemiBold }
      }

      ListView {
        id: list
        objectName: "whatsNewReleases"
        Layout.fillWidth: true
        Layout.fillHeight: true
        clip: true
        spacing: Theme.spacing.md
        model: root.releases

        delegate: ColumnLayout {
          id: releaseDelegate
          required property var modelData

          width: list.width
          spacing: Theme.spacing.xs

          RowLayout {
            Layout.fillWidth: true
            spacing: Theme.spacing.sm

            Text {
              objectName: "whatsNewVersion"
              text: "v" + releaseDelegate.modelData.version
              textFormat: Text.PlainText
              color: Theme.foreground
              font { family: Theme.font.family; pixelSize: Theme.font.body; bold: true }
            }

            Text {
              objectName: "whatsNewDate"
              Layout.fillWidth: true
              text: releaseDelegate.modelData.date
              textFormat: Text.PlainText
              horizontalAlignment: Text.AlignRight
              color: Util.alpha(Theme.foreground, 0.6)
              font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
            }
          }

          Repeater {
            objectName: "whatsNewChanges"
            model: releaseDelegate.modelData.changes

            Text {
              required property string modelData

              Layout.fillWidth: true
              text: "• " + modelData
              textFormat: Text.PlainText
              wrapMode: Text.WordWrap
              color: Theme.foreground
              font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
            }
          }
        }
      }

      Ui.Button {
        objectName: "whatsNewCloseButton"
        Layout.alignment: Qt.AlignRight
        text: "Close"
        focusable: true
        onClicked: root.closed()
      }
    }
  }
}
