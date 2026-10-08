pragma Singleton
import QtQuick

// The current time, refreshed every 30 seconds, for every leaf view that
// shows a relative time label (a conversation row's timestamp, a
// message's time and retry wording). Reading this singleton directly
// keeps the clock out of the plain nowMs property that used to carry it
// down through Panel.qml, MessengerLayout.qml and each view in between,
// none of which did anything with it but pass it on. A test overrides a
// leaf's own nowMs property instead of this singleton, so one test can
// pick an arbitrary time without affecting another.
//
// Item rather than QtObject: it holds the refresh timer below.
Item {
  id: root

  // nowMs is the current time in Unix milliseconds, as of the last tick.
  property real nowMs: Date.now()

  Timer {
    interval: 30000
    running: true
    repeat: true
    onTriggered: root.nowMs = Date.now()
  }
}
