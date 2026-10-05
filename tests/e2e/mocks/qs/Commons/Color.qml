pragma Singleton
import QtQuick
QtObject {
    readonly property color accent: "#7aa2f7"
    readonly property color background: "#1a1b26"
    readonly property color foreground: "#c0caf5"
    readonly property QtObject popups: QtObject {
        readonly property color border: "#414868"
        readonly property color background: "#24283b"
    }
}
