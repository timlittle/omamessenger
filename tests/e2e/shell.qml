import QtQuick
import Quickshell
import Quickshell.Io
import "../.." as OmaMessenger

ShellRoot {
    QtObject {
        id: mockService
        readonly property string status: "running"
        readonly property string detail: ""
        function startHelper() {}
    }

    QtObject {
        id: mockShell
        function serviceFor(_id) { return mockService }
    }

    OmaMessenger.Panel {
        id: app
        shell: mockShell
    }

    FloatingWindow {
        id: focusProbe
        title: "OmaMessenger Focus Probe"
        visible: false
        implicitWidth: 420
        implicitHeight: 240
        color: "#202020"
        Text { anchors.centerIn: parent; text: "Focus probe"; color: "white" }
    }

    IpcHandler {
        target: "oma-messenger-e2e"
        function showFocusProbe() { focusProbe.visible = true }
        function hideFocusProbe() { focusProbe.visible = false }
    }
}
