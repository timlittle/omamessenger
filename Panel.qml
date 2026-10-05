import QtQuick
import QtQuick.Controls
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui
import qs.Ui as OmarchyUi
import "keyboard.js" as Keyboard

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
    property string composeServiceId: "whatsapp"
    property bool closingFromHost: false

    function close() {
        closingFromHost = true
        composing = false
        controller.hide()
        Qt.callLater(function() { root.closingFromHost = false })
    }

    function dismiss() {
        if (shell && typeof shell.hide === "function") shell.hide("io.github.omamessenger")
        else close()
    }

    function openCompose() {
        composeServiceId = serviceFilter === "telegram" ? "telegram" : "whatsapp"
        composing = true
        Qt.callLater(function() { composeTitle.forceActiveFocus() })
    }

    function createConversation() {
        if (!composeTitle.text.trim() || !composeAccount.text.trim()) return
        var service = composeServiceId
        var account = composeAccount.text.trim()
        var title = composeTitle.text.trim()
        request("POST", "/conversations", {
            "id": service + ":" + account + ":" + title,
            "accountId": account, "service": service, "title": title
        }, function() {
            composing = false
            composeTitle.text = ""
            composeAccount.text = ""
            refresh()
        })
    }

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

    function activeConversationTitle() {
        for (var i = 0; i < conversations.length; i++) {
            if (conversations[i].id === activeConversationId) return conversations[i].title
        }
        return "Conversation"
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

    function handleEscape() {
        if (composing && composeService.popupOpen) {
            composeService.close()
            return
        }
        var action = Keyboard.escapeAction({
            searchFocused: search.activeFocus,
            composing: composing,
            conversationOpen: activeConversationId !== ""
        })
        if (action === "unfocus-search") {
            search.focus = false
            keyCatcher.forceActiveFocus()
        } else if (action === "close-compose") {
            composing = false
            Qt.callLater(function() { keyCatcher.forceActiveFocus() })
        } else if (action === "back") {
            goBack()
        } else {
            root.dismiss()
        }
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
        if (backendService.status === "helper-missing") return backendService.detail || "The bundled helper is missing. Update or reinstall the OmaMessenger plugin."
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
            openCompose(); event.accepted = true
        } else if ((event.modifiers & Qt.ControlModifier) && event.key === Qt.Key_1) {
            serviceFilter = "whatsapp"; cursor = 0; activeConversationId = ""; refresh(); event.accepted = true
        } else if ((event.modifiers & Qt.ControlModifier) && event.key === Qt.Key_2) {
            serviceFilter = "telegram"; cursor = 0; activeConversationId = ""; refresh(); event.accepted = true
        } else if ((event.modifiers & Qt.ControlModifier) && event.key === Qt.Key_0) {
            serviceFilter = "all"; cursor = 0; activeConversationId = ""; refresh(); event.accepted = true
        } else if ((search.activeFocus || composeField.activeFocus || composing) && event.key !== Qt.Key_Escape) {
            return
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
    onOpenedChanged: {
        if (opened) {
            if (apiToken.length) refresh()
            Qt.callLater(function() {
                panel.requestActivate()
                keyCatcher.forceActiveFocus()
            })
        }
    }

    FloatingWindow {
        id: panel
        visible: root.opened
        title: "OmaMessenger"
        flags: Qt.Window
        modality: Qt.NonModal
        implicitWidth: Style.space(1120)
        implicitHeight: Style.space(760)
        minimumSize: Qt.size(Style.space(760), Style.space(540))
        color: Color.background
        onClosing: root.dismiss()
        onVisibleChanged: {
            if (!visible && root.opened && !root.closingFromHost) root.dismiss()
        }
        Shortcut {
            sequence: "Escape"
            context: Qt.WindowShortcut
            enabled: root.opened
            onActivated: root.handleEscape()
        }

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
                        Text { text: "Ctrl K  Search     Ctrl N  Compose     Esc  Back/Close"; color: Color.foreground; opacity: 0.62; font.pixelSize: Style.font.caption }
                        Button { text: "Close"; onClicked: root.dismiss() }
                    }

                    Row {
                        width: parent.width
                        height: parent.height - Style.space(40)
                        spacing: Style.space(14)

                        Rectangle {
                            id: serviceRail
                            width: Style.space(88)
                            height: parent.height
                            radius: Style.space(12)
                            color: Color.background

                            Column {
                                anchors.top: parent.top
                                anchors.left: parent.left
                                anchors.right: parent.right
                                anchors.margins: Style.space(8)
                                spacing: Style.space(8)

                                Text {
                                    width: parent.width
                                    height: Style.space(38)
                                    text: "OMA"
                                    color: Color.accent
                                    font.pixelSize: Style.font.body
                                    font.bold: true
                                    horizontalAlignment: Text.AlignHCenter
                                    verticalAlignment: Text.AlignVCenter
                                }

                                Repeater {
                                    model: [
                                        { id: "all", short: "ALL", name: "All" },
                                        { id: "whatsapp", short: "WA", name: "WhatsApp" },
                                        { id: "telegram", short: "TG", name: "Telegram" }
                                    ]
                                    delegate: Rectangle {
                                        required property var modelData
                                        width: parent.width
                                        height: Style.space(58)
                                        radius: Style.space(9)
                                        color: root.serviceFilter === modelData.id ? Color.accent : "transparent"

                                        Column {
                                            anchors.centerIn: parent
                                            spacing: Style.space(2)
                                            Text {
                                                anchors.horizontalCenter: parent.horizontalCenter
                                                text: modelData.short
                                                color: root.serviceFilter === modelData.id ? Color.background : Color.foreground
                                                font.pixelSize: Style.font.body
                                                font.bold: true
                                            }
                                            Text {
                                                anchors.horizontalCenter: parent.horizontalCenter
                                                text: modelData.name
                                                color: root.serviceFilter === modelData.id ? Color.background : Color.foreground
                                                opacity: 0.76
                                                font.pixelSize: Style.font.caption
                                            }
                                        }

                                        MouseArea {
                                            anchors.fill: parent
                                            onClicked: {
                                                root.serviceFilter = modelData.id
                                                root.cursor = 0
                                                root.activeConversationId = ""
                                                root.refresh()
                                            }
                                        }
                                    }
                                }

                            }

                            Button {
                                anchors.horizontalCenter: parent.horizontalCenter
                                anchors.bottom: parent.bottom
                                anchors.bottomMargin: Style.space(8)
                                width: parent.width - Style.space(16)
                                text: "+"
                                font.pixelSize: Style.font.title
                                onClicked: {
                                    root.openCompose()
                                }
                                ToolTip.visible: hovered
                                ToolTip.text: "New conversation (Ctrl+N)"
                            }
                        }

                        Rectangle {
                            id: conversationPane
                            width: Math.min(Style.space(340), Math.max(Style.space(220), (parent.width - serviceRail.width - Style.space(28)) * 0.34))
                            height: parent.height
                            radius: Style.space(12)
                            color: Color.background
                            Column {
                                anchors.fill: parent
                                anchors.margins: Style.space(12)
                                spacing: Style.space(10)
                                Row {
                                    width: parent.width
                                    Text {
                                        text: "CONVERSATIONS"
                                        color: Color.foreground
                                        opacity: 0.68
                                        font.pixelSize: Style.font.caption
                                        font.bold: true
                                    }
                                    Item { width: parent.width - Style.space(120); height: 1 }
                                    Text {
                                        text: root.conversations.length
                                        color: Color.foreground
                                        opacity: 0.58
                                        font.pixelSize: Style.font.caption
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
                            width: parent.width - serviceRail.width - conversationPane.width - Style.space(28)
                            height: parent.height
                            radius: Style.space(12)
                            color: Color.background
                            Column {
                                anchors.fill: parent
                                anchors.margins: Style.space(16)
                                spacing: Style.space(12)
                                Text {
                                    text: root.activeConversationId ? root.activeConversationTitle() : "Your messages, together"
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
                                        && ["runtime-error", "stopped"].indexOf(root.backendService.status) !== -1
                                    text: "Start helper"
                                    onClicked: root.backendService.startHelper()
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
        id: composeScrim
        anchors.fill: card
        visible: root.composing
        z: 2
        color: Util.alpha(Color.background, 0.56)
        MouseArea { anchors.fill: parent; onClicked: {} }
    }
    OmarchyUi.BorderSurface {
        id: composeDialog
        visible: root.composing
        z: 3
        anchors.centerIn: card
        width: Math.min(Style.space(440), card.width - Style.space(40))
        height: contentColumn.implicitHeight + Style.space(36)
        radius: Style.cornerRadius
        color: Color.popups.background
        borderSpec: Border.localOrSurfaceSpec("popups", "border", Color.popups.border, Color.popups.border, Math.max(1, Style.space(1)))
        padding: Style.space(18)
        Column {
            id: contentColumn
            anchors.fill: parent
            anchors.topMargin: composeDialog.contentTopInset
            anchors.rightMargin: composeDialog.contentRightInset
            anchors.bottomMargin: composeDialog.contentBottomInset
            anchors.leftMargin: composeDialog.contentLeftInset
            spacing: Style.space(10)
            Text { text: "New conversation"; color: Color.popups.text; font.pixelSize: Style.font.title; font.bold: true }
            Text { text: "Choose a service and enter the account and contact details."; color: Color.popups.text; opacity: 0.72; font.pixelSize: Style.font.bodySmall; wrapMode: Text.WordWrap; width: parent.width }
            OmarchyUi.Dropdown {
                id: composeService
                width: parent.width
                showLabel: false
                value: root.composeServiceId
                options: [{ value: "whatsapp", label: "WhatsApp" }, { value: "telegram", label: "Telegram" }]
                onChanged: function(value) { root.composeServiceId = value }
            }
            OmarchyUi.TextField {
                id: composeTitle
                width: parent.width
                placeholderText: "Contact or chat name"
                onAccepted: composeAccount.forceActiveFocus()
            }
            OmarchyUi.TextField {
                id: composeAccount
                width: parent.width
                placeholderText: "Account ID"
                onAccepted: root.createConversation()
            }
            Row {
                width: parent.width
                anchors.right: parent.right
                spacing: Style.space(8)
                Item { width: parent.width - cancelButton.implicitWidth - createButton.implicitWidth - Style.space(8); height: 1 }
                OmarchyUi.Button { id: cancelButton; text: "Cancel"; onClicked: root.handleEscape() }
                OmarchyUi.Button { id: createButton; text: "Create"; selected: true; onClicked: root.createConversation() }
            }
        }
    }
    }
}
