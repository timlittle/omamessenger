import QtQuick

// MiddleClickGuard swallows a middle-click's Wayland primary-selection
// paste, the one native paste path Ctrl+V's own routing cannot
// intercept: Qt's TextInput pastes the primary selection on a
// middle-button press, with no size or time limit of its own, the same
// risk the composer's own right-click handler already swallows this
// for (see docs/decisions.md). acceptedButtons excludes the left and
// right buttons, so they are never delivered here at all and reach the
// field underneath exactly as if this were not present; drop one of
// these as a direct child of the field it is meant to guard.
MouseArea {
  anchors.fill: parent
  acceptedButtons: Qt.MiddleButton
  onPressed: (mouse) => { mouse.accepted = true; }
}
