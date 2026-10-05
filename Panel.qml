import QtQuick
import QtQuick.Controls
import Quickshell
import Quickshell.Io
import Quickshell.Wayland
import qs.Commons
import qs.Ui

Panel {
    id: root
    moduleName: "io.github.omamessenger"
    ipcTarget: "io.github.omamessenger"
    property var shell: null
    readonly property var backendService: shell ? shell.serviceFor("io.github.omamessenger") : null

    property string api: "http://127.0.0.1:43821/api/v1"
    readonly property string configHome: Quickshell.env("XDG_CONFIG_HOME") || (Quickshell.env("HOME") + "/.config")
    property string apiToken: ""
    property var conversations: []
    property var messages: []
    property string query: ""
    property string serviceFilter: "all"
    property string activeConversationId: ""
    property int cursor: 0
    property string errorText: ""
    property bool composing: false
    property bool focusPrimed: false

    function request(method, path, body, done) {
        var xhr = new XMLHttpRequest()
        xhr.open(method, api + path)
        xhr.setRequestHeader("Content-Type", "application/json")
        if (apiToken.length) xhr.setRequestHeader("Authorization", "Bearer " + apiToken)
        xhr.onreadystatechange = function() {
            if (xhr.readyState !== XMLHttpRequest.DONE) return
            if (xhr.status >= 200 && xhr.status < 300) {
                errorText = ""
                if (done) done(xhr.responseText.length ? JSON.parse(xhr.responseText) : null)
            } else {
                errorText = xhr.status === 0
                    ? "OmaMessenger helper is not responding. Try restarting omarchy-shell."
                    : "Request failed (" + xhr.status + ")"
            }
        }
        xhr.send(body === undefined ? null : JSON.stringify(body))
    }

    function refresh() {
        var q = encodeURIComponent(query)
        request("GET", "/conversations?q=" + q, undefined, function(rows) {
            if (serviceFilter !== "all")
                rows = rows.filter(function(row) { return row.service === serviceFilter })
            conversations = rows
            if (cursor >= conversations.length) cursor = Math.max(0, conversations.length - 1)
            if (activeConversationId !== "") loadMessages(activeConversationId)
        })
    }

    function loadMessages(id) {
        activeConversationId = id
        request("GET", "/conversations/" + encodeURIComponent(id) + "/messages", undefined,
                function(rows) { messages = rows })
        request("POST", "/conversations/" + encodeURIComponent(id) + "/read", {})
    }

    function openConversation() {
        if (conversations.length) loadMessages(conversations[cursor].id)
    }

    function goBack() {
        activeConversationId = ""
        messages = []
        composeField.focus = false
        search.focus = false
        keyCatcher.forceActiveFocus()
    }

    function sendCurrent() {
        if (!activeConversationId || composeField.text.trim().length === 0) return
        var text = composeField.text.trim()
        composeField.text = ""
        request("POST", "/conversations/" + encodeURIComponent(activeConversationId) + "/messages",
                { text: text }, function() { loadMessages(activeConversationId); refresh() })
    }

    function helperStatusText() {
        if (!backendService) return "OmaMessenger helper is unavailable. Rescan or restart omarchy-shell."
        if (backendService.status === "checking") return "Checking the OmaMessenger helper…"
        if (backendService.status === "helper-missing") return "Build the local Go helper to start the messaging service."
        if (backendService.status === "building") return "Building the local Go helper…"
        if (backendService.status === "build-error") return backendService.detail || "Build failed. Install Go with `omarchy pkg add go` and try again."
        if (backendService.status === "runtime-error") return backendService.detail || "The helper stopped unexpectedly. Try restarting it."
        if (backendService.status === "stopped") return "The helper is stopped. Start it again to continue."
        if (backendService.status === "starting") return "Starting the local messaging service…"
        if (backendService.status === "running") return "Waiting for the local messaging API…"
        return ""
    }

    function shortcut(event) {
        if ((event.modifiers & Qt.ControlModifier) && event.key === Qt.Key_K) {
            search.forceActiveFocus(); search.selectAll(); event.accepted = true
        } else if ((event.modifiers & Qt.ControlModifier) && event.key === Qt.Key_N) {
            composing = true; composeTitle.forceActiveFocus(); event.accepted = true
        } else if ((event.modifiers & Qt.ControlModifier) && event.key === Qt.Key_1) {
            serviceFilter = "whatsapp"; cursor = 0; activeConversationId = ""; refresh(); event.accepted = true
        } else if ((event.modifiers & Qt.ControlModifier) && event.key === Qt.Key_2) {
            serviceFilter = "telegram"; cursor = 0; activeConversationId = ""; refresh(); event.accepted = true
        } else if ((event.modifiers & Qt.ControlModifier) && event.key === Qt.Key_0) {
            serviceFilter = "all"; cursor = 0; activeConversationId = ""; refresh(); event.accepted = true
        } else if ((search.activeFocus || composeField.activeFocus || composing) && event.key !== Qt.Key_Escape) {
            return
        } else if (event.key === Qt.Key_Escape) {
            if (search.activeFocus) { search.focus = false; keyCatcher.forceActiveFocus() }
            else if (composing) composing = false
            else if (activeConversationId) goBack()
            else root.controller.hide()
            event.accepted = true
        } else if (activeConversationId === "" && event.key === Qt.Key_J) {
            cursor = Math.min(cursor + 1, conversations.length - 1); event.accepted = true
        } else if (activeConversationId === "" && event.key === Qt.Key_K) {
            cursor = Math.max(cursor - 1, 0); event.accepted = true
        } else if (activeConversationId === "" && (event.key === Qt.Key_Return || event.key === Qt.Key_Enter)) {
            openConversation(); event.accepted = true
        }
    }

    FileView {
        id: apiTokenFile
        path: root.configHome + "/omamessenger/api.token"
        watchChanges: true
        printErrors: false
        onLoaded: {
            root.apiToken = text().trim()
            if (root.apiToken.length) root.refresh()
        }
        onLoadFailed: root.apiToken = ""
    }
    Timer { interval: 4000; repeat: true; running: root.opened && root.apiToken.length > 0; onTriggered: root.refresh() }
    Timer {
        interval: 1000
        repeat: true
        running: root.opened && root.backendService && root.backendService.status === "running" && root.apiToken.length === 0
        onTriggered: apiTokenFile.reload()
    }
    Timer { id: focusSettle; interval: 100; onTriggered: root.focusPrimed = true }
    onOpenedChanged: {
        if (opened) {
            focusPrimed = false
            focusSettle.restart()
            if (apiToken.length) refresh()
            Qt.callLater(function() { keyCatcher.forceActiveFocus() })
        } else {
            focusPrimed = false
            focusSettle.stop()
        }
    }

    PanelWindow {
        id: panel
        visible: root.opened
        anchors { top: true; bottom: true; left: true; right: true }
        color: "transparent"
        exclusionMode: ExclusionMode.Ignore
        WlrLayershell.namespace: "omarchy-omamessenger"
        WlrLayershell.layer: WlrLayer.Overlay
        WlrLayershell.keyboardFocus: root.opened
            ? (root.focusPrimed ? WlrKeyboardFocus.OnDemand : WlrKeyboardFocus.Exclusive)
            : WlrKeyboardFocus.None

        Rectangle {
            id: card
            width: Math.min(Style.space(1120), panel.width - Style.space(48))
            height: Math.min(Style.space(760), panel.height - Style.space(48))
            anchors.centerIn: parent
            radius: Style.cornerRadius
            color: Util.alpha(Color.background, 0.98)
            border.color: Color.popups.border
            border.width: Math.max(1, Style.space(1))

            Item {
            id: keyCatcher
            anchors.fill: parent
            focus: true
            Keys.priority: Keys.BeforeItem
            Keys.onPressed: function(event) { root.shortcut(event) }

                Column {
                    anchors.fill: parent
                    anchors.margins: Style.space(20)
                    spacing: Style.space(14)

                    Row {
                        width: parent.width
                        spacing: Style.space(10)
                        Text { text: "OMA"; color: Color.accent; font.pixelSize: Style.font.heading; font.bold: true }
                        Text { text: "MESSENGER"; color: Color.foreground; font.pixelSize: Style.font.heading; font.bold: true }
                        Item { width: parent.width - 420; height: 1 }
                        Text { text: "Ctrl K  Search     Ctrl N  Compose     Esc  Back"; color: Color.foreground; opacity: 0.62; font.pixelSize: Style.font.caption }
                    }

                    Row {
                        width: parent.width
                        height: parent.height - Style.space(40)
                        spacing: Style.space(14)

                        Rectangle {
                            width: Math.min(Style.space(340), parent.width * 0.35)
                            height: parent.height
                            radius: Style.space(12)
                            color: Color.background
                            Column {
                                anchors.fill: parent
                                anchors.margins: Style.space(12)
                                spacing: Style.space(10)
                                Row {
                                    spacing: Style.space(8)
                                    Repeater {
                                        model: [{id:"all",label:"All"},{id:"whatsapp",label:"WhatsApp"},{id:"telegram",label:"Telegram"}]
                                        delegate: Rectangle {
                                            required property var modelData
                                            width: tabLabel.implicitWidth + Style.space(18); height: Style.space(32)
                                            radius: Style.space(8)
                                            color: root.serviceFilter === modelData.id ? Color.accent : Color.background
                                            Text { id: tabLabel; anchors.centerIn: parent; text: modelData.label; color: root.serviceFilter === modelData.id ? Color.background : Color.foreground; font.pixelSize: Style.font.bodySmall }
                                            MouseArea { anchors.fill: parent; onClicked: { root.serviceFilter = modelData.id; root.cursor = 0; root.activeConversationId = ""; root.refresh() } }
                                        }
                                    }
                                }
                                TextField {
                                    id: search
                                    width: parent.width
                                    placeholderText: "Search conversations"
                                    text: root.query
                                    palette.text: Color.foreground
                                    palette.base: Color.background
                                    palette.placeholderText: Color.foreground
                                    onTextChanged: { root.query = text; debounce.restart() }
                                    Keys.onPressed: function(event) { if (event.key === Qt.Key_Escape) { focus = false; keyCatcher.forceActiveFocus(); event.accepted = true } }
                                }
                                Timer { id: debounce; interval: 180; onTriggered: root.refresh() }
                                ListView {
                                    id: chatList
                                    width: parent.width
                                    height: parent.height - Style.space(92)
                                    clip: true
                                    model: root.conversations
                                    currentIndex: root.cursor
                                    delegate: Rectangle {
                                        required property var modelData
                                        required property int index
                                        width: chatList.width; height: Style.space(74); radius: Style.space(9)
                                        color: index === root.cursor ? Color.accent : "transparent"
                                        Column {
                                            anchors.left: parent.left; anchors.right: parent.right
                                            anchors.leftMargin: Style.space(10); anchors.rightMargin: Style.space(10)
                                            anchors.verticalCenter: parent.verticalCenter; spacing: Style.space(4)
                                            Row {
                                                width: parent.width
                                                Text { width: parent.width - unread.width - Style.space(10); text: modelData.title; color: index === root.cursor ? Color.background : Color.foreground; font.pixelSize: Style.font.body; font.bold: true; elide: Text.ElideRight }
                                                Rectangle {
                                                    id: unread
                                                    visible: modelData.unread > 0
                                                    width: unreadLabel.implicitWidth + Style.space(10)
                                                    height: unreadLabel.implicitHeight + Style.space(6)
                                                    radius: height / 2
                                                    color: Color.accent
                                                    Text {
                                                        id: unreadLabel
                                                        anchors.centerIn: parent
                                                        text: modelData.unread
                                                        color: Color.background
                                                        font.pixelSize: Style.font.caption
                                                    }
                                                }
                                            }
                                            Text { width: parent.width; text: (modelData.service === "whatsapp" ? "WA" : "TG") + "   " + modelData.preview; color: Color.foreground; opacity: 0.66; font.pixelSize: Style.font.bodySmall; elide: Text.ElideRight }
                                        }
                                        MouseArea { anchors.fill: parent; onClicked: { root.cursor = index; root.openConversation() } }
                                    }
                                    ScrollBar.vertical: ScrollBar { }
                                }
                            }
                        }

                        Rectangle {
                            width: parent.width - Style.space(354)
                            height: parent.height
                            radius: Style.space(12)
                            color: Color.background
                            Column {
                                anchors.fill: parent
                                anchors.margins: Style.space(16)
                                spacing: Style.space(12)
                                Text {
                                    text: root.activeConversationId ? "Conversation" : "Your messages, together"
                                    color: Color.foreground; font.pixelSize: Style.font.title; font.bold: true
                                }
                                Text {
                                    visible: !root.activeConversationId
                                    width: parent.width
                                    text: root.errorText || (root.conversations.length ? "Choose a conversation with j/k and press Enter. Use Ctrl+N to compose." : root.apiToken ? "No conversations yet. Remote accounts are not connected in this build." : root.helperStatusText())
                                    color: Color.foreground; opacity: 0.72; font.pixelSize: Style.font.body; wrapMode: Text.WordWrap
                                }
                                Button {
                                    visible: !root.apiToken && root.backendService
                                        && ["helper-missing", "build-error", "runtime-error", "stopped"].indexOf(root.backendService.status) !== -1
                                    text: root.backendService && root.backendService.status === "stopped" ? "Start helper" : "Build helper"
                                    onClicked: {
                                        if (root.backendService.status === "stopped") root.backendService.startHelper()
                                        else root.backendService.buildHelper()
                                    }
                                }
                                ListView {
                                    id: messageList
                                    visible: root.activeConversationId !== ""
                                    width: parent.width; height: parent.height - composer.height - Style.space(54)
                                    clip: true; spacing: Style.space(8); model: root.messages
                                    delegate: Rectangle {
                                        required property var modelData
                                        width: Math.min(messageText.implicitWidth + Style.space(24), messageList.width * 0.78)
                                        height: messageText.implicitHeight + Style.space(24)
                                        anchors.right: modelData.outgoing ? parent.right : undefined
                                        radius: Style.space(10); color: modelData.outgoing ? Color.accent : Color.popups.background
                                        Text { id: messageText; anchors.fill: parent; anchors.margins: Style.space(12); text: modelData.text; color: modelData.outgoing ? Color.background : Color.foreground; font.pixelSize: Style.font.body; wrapMode: Text.Wrap; width: messageList.width * 0.75 }
                                    }
                                }
                                Row {
                                    id: composer
                                    visible: root.activeConversationId !== ""
                                    width: parent.width; spacing: Style.space(8)
                                    TextField {
                                        id: composeField
                                        objectName: "composer"
                                        width: parent.width - sendButton.width - Style.space(8)
                                        placeholderText: "Write a message"
                                        palette.text: Color.foreground
                                        palette.base: Color.background
                                        palette.placeholderText: Color.foreground
                                        onAccepted: root.sendCurrent()
                                    }
                                    Button { id: sendButton; text: "Send"; onClicked: root.sendCurrent() }
                                }
                            }
                        }
                    }
                }
            }
        }
    Rectangle {
        id: composeDialog
        visible: root.composing
        z: 3
        anchors.centerIn: card
        width: Style.space(420)
        height: contentColumn.implicitHeight + Style.space(32)
        radius: Style.cornerRadius
        color: Color.background
        border.color: Color.popups.border
        border.width: Math.max(1, Style.space(1))
        Column {
            id: contentColumn
            width: parent.width
            anchors.centerIn: parent
            anchors.margins: Style.space(16)
            spacing: Style.space(10)
            Text { text: "New conversation"; color: Color.foreground; font.pixelSize: Style.font.title; font.bold: true }
            ComboBox { id: composeService; width: parent.width; model: ["whatsapp", "telegram"] }
            TextField { id: composeTitle; width: parent.width; placeholderText: "Contact or chat name"; palette.text: Color.foreground; palette.base: Color.background }
            TextField { id: composeAccount; width: parent.width; placeholderText: "Account ID"; palette.text: Color.foreground; palette.base: Color.background }
            Row {
                width: parent.width
                spacing: Style.space(8)
                Button { text: "Cancel"; onClicked: root.composing = false }
                Button {
                    text: "Create"
                    onClicked: {
                        if (!composeTitle.text.trim() || !composeAccount.text.trim()) return
                        var service = composeService.currentText
                        var account = composeAccount.text.trim()
                        var title = composeTitle.text.trim()
                        root.request("POST", "/conversations", {
                            "id": service + ":" + account + ":" + title,
                            "accountId": account, "service": service, "title": title
                        }, function() {
                            root.composing = false
                            composeTitle.text = ""
                            root.refresh()
                        })
                    }
                }
            }
        }
    }
    }
}
