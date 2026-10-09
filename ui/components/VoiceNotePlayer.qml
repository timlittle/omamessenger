import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Format.js" as Format

// A voice note in a message: a "Voice message" label (never just the
// play glyph, which names nothing on its own), a play/pause button, a
// progress bar and an elapsed/total time label. Reads
// playback state (playing, positionMs, durationMs) as plain data, the
// same way PhotoView reads a download's path, rather than calling a
// controller itself; the caller owns starting, pausing and downloading.
// When available is false (the optional QtMultimedia module this needs
// could not be loaded), this shows the same plain "open externally" row
// FileView uses instead, so the rest of the timeline never breaks over a
// dependency not every Omarchy install has.
Item {
  id: root

  // media is the message's voice media: duration and fileName.
  required property var media
  // path is where the voice note was downloaded, or "" until then.
  property string path: ""
  // failed is true once a fetch for this voice note has failed outright.
  property bool failed: false
  // failedReason is that fetch's safe reason category (see
  // server/errors.go), "" when failed is false or the helper gave none.
  property string failedReason: ""
  // available is false when in-window playback cannot work at all.
  property bool available: true
  // playing is true while this particular voice note is the one making
  // sound; a different note playing, or this one paused, is false.
  property bool playing: false
  // positionMs/durationMs are this note's playback position and length,
  // read from the controller while it is the one loaded, 0 otherwise.
  property real positionMs: 0
  property real durationMs: 0

  // wanted asks for the full voice note to be downloaded.
  signal wanted()
  // opened asks for this voice note to be played, paused, or (with no
  // in-window player available) opened in the user's own application.
  signal opened()

  // _knownDurationMs is the file's real length once the controller
  // reports one, else the duration the message's own metadata already
  // carried, so the total shown never starts blank.
  readonly property real _knownDurationMs: root.durationMs > 0 ? root.durationMs : (root.media.duration || 0) * 1000
  // _fraction is how far through playback is, 0 when nothing is known yet.
  readonly property real _fraction: root._knownDurationMs > 0 ? Math.min(1, root.positionMs / root._knownDurationMs) : 0
  // _unplayable is true once a fetch is known to have failed and there is
  // still no downloaded copy to play, so the button and label say so
  // instead of offering to play nothing.
  readonly property bool _unplayable: root.failed && !root.path
  // _unavailableHint is a short, safe phrase for failedReason's category
  // ("connection problem", "took too long", …), or "" when there is none
  // to show, so the play button's tooltip can say why without ever
  // showing the error itself.
  readonly property string _unavailableHint: Format.mediaFailureReason(root.failedReason)

  implicitWidth: Style.space(280)
  implicitHeight: available ? playerRow.implicitHeight : fallbackRow.implicitHeight

  // A voice note is small, so it is fetched as soon as it is shown,
  // exactly like a photo's own thumbnail-to-full-image fetch; rows are
  // recycled as the list scrolls, so this follows what is on screen.
  Component.onCompleted: if (root.visible && !root.path) root.wanted()
  onVisibleChanged: if (root.visible && !root.path) root.wanted()

  RowLayout {
    id: playerRow
    objectName: "voicePlayerRow"
    visible: root.available
    anchors { left: parent.left; right: parent.right }
    spacing: Theme.spacing.sm

    Ui.Button {
      objectName: "voicePlayButton"
      text: root.playing ? "⏸" : "▶"
      tooltipText: root._unplayable
        ? ("Unavailable" + (root._unavailableHint ? " — " + root._unavailableHint : ""))
        : (root.playing ? "Pause voice message" : "Play voice message")
      enabled: !root._unplayable
      focusable: true
      onClicked: root.opened()
    }

    ColumnLayout {
      Layout.fillWidth: true
      spacing: Theme.spacing.xxs

      Text {
        objectName: "voiceAccessibleLabel"
        Layout.fillWidth: true
        text: "Voice message"
        textFormat: Text.PlainText
        elide: Text.ElideRight
        color: Color.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.bodySmall; weight: Font.DemiBold }
      }

      Item {
        id: track
        Layout.fillWidth: true
        implicitHeight: Style.space(4)

        Rectangle {
          anchors.fill: parent
          radius: height / 2
          color: Util.alpha(Color.foreground, 0.15)
        }

        Rectangle {
          objectName: "voiceProgressFill"
          anchors { left: parent.left; top: parent.top; bottom: parent.bottom }
          width: track.width * root._fraction
          radius: height / 2
          color: Color.accent
        }
      }

      Text {
        objectName: "voiceTimeLabel"
        text: root._unplayable
          ? "Unavailable"
          : Format.elapsed(Math.floor(root.positionMs / 1000)) + " / " + Format.elapsed(Math.floor(root._knownDurationMs / 1000))
        textFormat: Text.PlainText
        color: Util.alpha(Color.foreground, 0.6)
        font { family: Theme.font.family; pixelSize: Theme.font.caption }
      }
    }
  }

  RowLayout {
    id: fallbackRow
    objectName: "voiceFallbackRow"
    visible: !root.available
    anchors { left: parent.left; right: parent.right }
    spacing: Theme.spacing.sm

    Rectangle {
      Layout.preferredWidth: Style.space(40)
      Layout.preferredHeight: Style.space(40)
      radius: Style.cornerRadius
      color: Util.alpha(Color.accent, 0.22)

      Text {
        anchors.centerIn: parent
        text: "▶"
        textFormat: Text.PlainText
        color: Color.accent
        font { family: Theme.font.family; pixelSize: Theme.font.body }
      }
    }

    ColumnLayout {
      Layout.fillWidth: true
      spacing: 0

      Text {
        Layout.fillWidth: true
        text: "Voice message · open"
        textFormat: Text.PlainText
        color: Color.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.bodySmall; weight: Font.DemiBold }
      }

      Text {
        Layout.fillWidth: true
        visible: text !== ""
        text: Format.duration(root.media.duration)
        textFormat: Text.PlainText
        color: Util.alpha(Color.foreground, 0.6)
        font { family: Theme.font.family; pixelSize: Theme.font.caption }
      }
    }
  }

  MouseArea {
    anchors.fill: fallbackRow
    visible: !root.available
    cursorShape: Qt.PointingHandCursor
    onClicked: root.opened()
  }
}
