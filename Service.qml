import QtQuick
import Quickshell
import Quickshell.Io

// The shell owns this helper for the lifetime of the loaded plugin. Build is
// explicit and user initiated; plugin installation never compiles or runs it.
Item {
    id: root

    property var shell: null
    readonly property string pluginDir: Quickshell.env("HOME") + "/.config/omarchy/plugins/io.github.omamessenger"
    readonly property string helperPath: pluginDir + "/oma-messenger-service"
    property string status: "checking"
    property string detail: ""

    function buildHelper() {
        if (buildProcess.running || helper.running) return
        detail = ""
        status = "building"
        buildProcess.running = true
    }

    function startHelper() {
        if (helper.running) return
        detail = ""
        status = "starting"
        helper.running = true
    }

    function stopHelper() {
        helper.running = false
        status = "stopped"
    }

    Process {
        id: binaryCheck
        command: ["test", "-x", root.helperPath]
        onExited: function(exitCode) {
            if (exitCode === 0) root.startHelper()
            else root.status = "helper-missing"
        }
    }

    Process {
        id: buildProcess
        command: ["go", "build", "-mod=vendor", "-buildvcs=false", "-o", root.helperPath, "./backend"]
        workingDirectory: root.pluginDir
        stderr: StdioCollector {
            waitForEnd: true
            onStreamFinished: root.detail = text.trim()
        }
        onExited: function(exitCode) {
            if (exitCode !== 0) {
                root.status = "build-error"
                if (!root.detail) root.detail = "Build failed. Check that Go is installed and try again."
                return
            }
            root.startHelper()
        }
    }

    Process {
        id: helper
        command: [root.helperPath]
        onRunningChanged: {
            if (running) root.status = "running"
            else if (root.status === "running" || root.status === "starting") root.status = "stopped"
        }
        stderr: StdioCollector {
            waitForEnd: false
            onTextChanged: if (text.trim()) root.detail = text.trim()
        }
        onExited: function(exitCode) {
            if (exitCode !== 0) {
                root.status = "runtime-error"
                if (!root.detail) root.detail = "The helper exited with code " + exitCode + "."
            }
        }
    }

    Component.onCompleted: binaryCheck.running = true
}
