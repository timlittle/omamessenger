import QtQuick
import Quickshell.Io

Item {
    id: root
    property string moduleName: ""
    property string ipcTarget: ""
    property var shell: null
    property bool manageIpc: true
    property bool opened: false
    property alias controller: panelController

    function open() { opened = true }
    function close() { opened = false }
    function toggle() { opened = !opened }

    QtObject {
        id: panelController
        function hide() { root.close() }
        function show() { root.open() }
    }

    IpcHandler {
        enabled: root.manageIpc && root.ipcTarget !== ""
        target: root.ipcTarget
        function open() { root.open() }
        function close() { root.close() }
        function show() { root.open() }
        function hide() { root.close() }
        function toggle() { root.toggle() }
    }
}
