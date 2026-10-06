import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Setup.js" as Setup

// Adds a Telegram account and signs it in: optionally the user's own API
// id and hash from my.telegram.org, then a QR code to scan, or a phone number, login code
// and two-step password. Enter continues, Escape cancels.
Item {
  id: root

  // stage is the step shown: credentials, waiting, qr, phone, code or password.
  property string stage: "credentials"
  // qr is the QR code to scan, as a base64 PNG.
  property string qr: ""
  // hint explains the current step.
  property string hint: ""
  // error is the last failure, shown under the step.
  property string error: ""
  // busy disables Continue while a request is in flight.
  property bool busy: false
  // routeKey intercepts key presses in the fields first; see Composer.qml.
  property var routeKey: null

  // credentialsSubmitted reports the API id and hash typed in.
  signal credentialsSubmitted(string apiId, string apiHash)
  // phoneRequested asks to sign in by phone number instead of QR code.
  signal phoneRequested()
  // answered reports what was typed for the phone, code or password step.
  signal answered(string value)
  // cancelled closes setup.
  signal cancelled()

  // _field describes the typed answer the stage asks for, or null.
  readonly property var _field: Setup.field(root.stage)

  // submit continues from the current step.
  function submit(): void {
    if (root.busy) return;
    if (root.stage === "credentials") root.credentialsSubmitted(idField.text, hashField.text);
    else if (root._field) root.answered(answerField.text);
  }

  // _focusStep focuses the stage's first control once it is visible.
  function _focusStep(): void {
    if (root.stage === "credentials") idField.forceActiveFocus();
    else if (root.stage === "qr") usePhoneButton.forceActiveFocus();
    else if (root._field) { answerField.text = ""; answerField.forceActiveFocus(); }
  }

  objectName: "accountSetup"
  anchors.fill: parent
  onStageChanged: Qt.callLater(root._focusStep)
  onVisibleChanged: if (root.visible) Qt.callLater(root._focusStep)

  // The scrim takes clicks so nothing behind it reacts, but does not
  // cancel: a stray click should not lose a sign-in half done.
  Rectangle {
    anchors.fill: parent
    color: Theme.menu.scrim

    MouseArea {
      anchors.fill: parent
    }
  }

  Ui.BorderSurface {
    id: card

    width: Math.min(parent.width - Theme.spacing.xxl * 2, Style.space(440))
    height: content.implicitHeight + card.contentTopInset + card.contentBottomInset
    color: Theme.popups.background
    borderSpec: Border.flat(Theme.popups.border, Style.normalBorderWidth)
    radius: Style.cornerRadius
    padding: Theme.spacing.panelPadding
    anchors.centerIn: parent

    ColumnLayout {
      id: content

      spacing: Theme.spacing.md
      anchors {
        fill: parent
        topMargin: card.contentTopInset
        rightMargin: card.contentRightInset
        bottomMargin: card.contentBottomInset
        leftMargin: card.contentLeftInset
      }

      Text {
        text: root.stage === "credentials" ? "Add a Telegram account" : "Sign in to Telegram"
        color: Color.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.subtitle; weight: Font.DemiBold }
      }

      Text {
        Layout.fillWidth: true
        text: root.stage === "credentials"
          ? "To use your own Telegram app instead of OmaMessenger's, sign in at <a href=\"https://my.telegram.org/apps\">my.telegram.org</a>, open API development tools, and copy the app's api_id and api_hash here. They stay on this computer."
          : root.stage === "waiting" ? "Connecting to Telegram…" : root.hint
        visible: text !== ""
        textFormat: Text.StyledText
        wrapMode: Text.WordWrap
        linkColor: Color.accent
        color: Util.alpha(Color.foreground, 0.7)
        font { family: Theme.font.family; pixelSize: Theme.font.body }
        onLinkActivated: link => Qt.openUrlExternally(link)
      }

      Ui.TextField {
        id: idField
        objectName: "apiIdField"
        Layout.fillWidth: true
        visible: root.stage === "credentials"
        placeholderText: "API id"
        KeyNavigation.tab: hashField
        Keys.priority: Keys.BeforeItem
        Keys.onPressed: event => { if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true }
        onAccepted: hashField.forceActiveFocus()
      }

      Ui.TextField {
        id: hashField
        objectName: "apiHashField"
        Layout.fillWidth: true
        visible: root.stage === "credentials"
        placeholderText: "API hash"
        Keys.priority: Keys.BeforeItem
        Keys.onPressed: event => { if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true }
        onAccepted: root.submit()
      }

      Image {
        objectName: "qrImage"
        Layout.alignment: Qt.AlignHCenter
        Layout.preferredWidth: Style.space(220)
        Layout.preferredHeight: Style.space(220)
        visible: root.stage === "qr"
        source: Setup.qrSource(root.qr)
        smooth: false
        fillMode: Image.PreserveAspectFit
      }

      Ui.TextField {
        id: answerField
        objectName: "answerField"
        Layout.fillWidth: true
        visible: root._field !== null
        placeholderText: root._field ? root._field.placeholder : ""
        password: root._field ? root._field.password : false
        Keys.priority: Keys.BeforeItem
        Keys.onPressed: event => { if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true }
        onAccepted: root.submit()
      }

      Text {
        Layout.fillWidth: true
        visible: root.error !== ""
        text: root.error
        wrapMode: Text.WordWrap
        color: Color.urgent
        font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
      }

      RowLayout {
        Layout.alignment: Qt.AlignRight
        spacing: Theme.spacing.controlGap

        Ui.Button {
          text: "Cancel"
          focusable: true
          onClicked: root.cancelled()
        }

        Ui.Button {
          id: usePhoneButton
          objectName: "usePhoneButton"
          visible: root.stage === "qr"
          text: "Use phone number instead"
          focusable: true
          onClicked: root.phoneRequested()
        }

        Ui.Button {
          objectName: "continueButton"
          visible: root.stage === "credentials" || root._field !== null
          enabled: !root.busy
          text: "Continue"
          focusable: true
          selected: true
          onClicked: root.submit()
        }
      }
    }
  }
}
