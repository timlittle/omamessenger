import QtQuick

// MiddleClickGuard swallows a middle-click's unbounded Wayland
// primary-selection paste, which Ctrl+V's own routing cannot
// intercept (see docs/decisions.md). Drop as a direct child of the
// field it guards; left and right clicks pass through untouched.
MouseArea {
  anchors.fill: parent
  acceptedButtons: Qt.MiddleButton
  onPressed: (mouse) => { mouse.accepted = true; }
}
