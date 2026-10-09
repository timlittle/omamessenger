pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Setup.js" as Setup

// Adds an account and signs it in: a chooser first when the helper offers
// more than one service, then optionally the user's own API id and hash
// from my.telegram.org for Telegram, then a QR code to scan, or a phone
// number, login code and two-step password. The chooser takes j/k,
// Up/Down or a mnemonic letter; Enter chooses the highlight, Escape
// cancels. The QR and phone steps can switch to each other with p or q.
Item {
  id: root

  // stage is the step shown: chooseService, credentials, waiting, qr,
  // phone, code or password.
  property string stage: "credentials"
  // services lists the services to pick from at the chooseService step.
  property var services: []
  // serviceName names the service being set up, for every step after the
  // chooser.
  property string serviceName: "Telegram"
  // chooseIndex is the chooser's highlighted service: -1 for Cancel.
  property int chooseIndex: -1
  // qr is the QR code to scan, as a base64 PNG.
  property string qr: ""
  // hint explains the current step.
  property string hint: ""
  // error is the last failure, shown under the step.
  property string error: ""
  // busy disables Continue while a request is in flight.
  property bool busy: false
  // routeKey intercepts key presses before a field or this step's own
  // control handles them; see Composer.qml for why this is a function
  // property, not a signal.
  property var routeKey: null

  // serviceChosen reports the service picked at the chooseService step.
  signal serviceChosen(string serviceId)
  // credentialsSubmitted reports the API id and hash typed in.
  signal credentialsSubmitted(string apiId, string apiHash)
  // phoneRequested asks to sign in by phone number instead of QR code.
  signal phoneRequested()
  // qrRequested asks to go back to the QR code already shown, from the
  // phone step.
  signal qrRequested()
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

  // _focusStep moves real keyboard focus to the stage's own control: a
  // text field for credentials, a phone number, a code or a password, and
  // this Item itself for every other step, so j/k, the mnemonic letters
  // and p/q reach it through routeKey rather than needing a focused
  // button of their own.
  function _focusStep(): void {
    if (root.stage === "credentials") { idField.forceActiveFocus(); return; }
    if (root._field) { answerField.text = ""; answerField.forceActiveFocus(); return; }
    root.forceActiveFocus();
  }

  objectName: "accountSetup"
  anchors.fill: parent
  onStageChanged: Qt.callLater(root._focusStep)
  onVisibleChanged: if (root.visible) Qt.callLater(root._focusStep)

  Keys.onPressed: event => {
    if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
  }

  // Clicks outside the card do not cancel: a stray click should not lose a
  // sign-in half done.
  ModalCard {
    cardWidth: Style.space(440)

    ColumnLayout {
      id: content

      anchors.fill: parent
      spacing: Theme.spacing.md

      Text {
        text: root.stage === "chooseService" ? "Add an account"
          : root.stage === "credentials" ? "Add a " + root.serviceName + " account"
          : "Sign in to " + root.serviceName
        textFormat: Text.PlainText
        color: Color.foreground
        font { family: Theme.font.family; pixelSize: Theme.font.subtitle; weight: Font.DemiBold }
      }

      Text {
        objectName: "stepHint"
        Layout.fillWidth: true
        text: root.stage === "chooseService" ? "Choose which service to add an account for: j/k or a letter, Enter to choose."
          : root.stage === "credentials"
          ? "To use your own Telegram app instead of OmaMessenger's, sign in at <a href=\"https://my.telegram.org/apps\">my.telegram.org</a>, open API development tools, and copy the app's api_id and api_hash here. They stay on this computer."
          : root.stage === "waiting" ? "Connecting to " + root.serviceName + "…" : root.hint
        visible: text !== ""
        // StyledText is safe here only because every branch is a fixed
        // string we wrote ourselves (root.serviceName comes from the
        // helper's own fixed service list, never a contact or message):
        // nothing here is sender-controlled, so there is no way for a
        // remote "<img>" to reach this label.
        textFormat: Text.StyledText
        wrapMode: Text.WordWrap
        linkColor: Color.accent
        color: Util.alpha(Color.foreground, 0.7)
        font { family: Theme.font.family; pixelSize: Theme.font.body }
        onLinkActivated: link => Qt.openUrlExternally(link)
      }

      ColumnLayout {
        id: serviceChoices
        Layout.fillWidth: true
        visible: root.stage === "chooseService"
        spacing: Theme.spacing.controlGap

        Repeater {
          id: serviceRepeater
          model: root.services

          Ui.Button {
            required property var modelData
            required property int index

            objectName: "serviceButton-" + modelData.id
            Layout.fillWidth: true
            leftAlign: true
            text: Setup.mnemonicLabel(Setup.serviceMnemonic(modelData.id), modelData.name)
            selected: index === root.chooseIndex
            onClicked: root.serviceChosen(modelData.id)
          }
        }
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
        Layout.preferredWidth: Style.space(280)
        Layout.preferredHeight: Style.space(280)
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
        textFormat: Text.PlainText
        wrapMode: Text.WordWrap
        color: Color.urgent
        font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
      }

      RowLayout {
        Layout.alignment: Qt.AlignRight
        spacing: Theme.spacing.controlGap

        Ui.Button {
          id: cancelButton

          objectName: "cancelButton"
          text: "Cancel"
          onClicked: root.cancelled()
        }

        Ui.Button {
          id: usePhoneButton
          objectName: "usePhoneButton"
          visible: root.stage === "qr"
          text: "p  Use phone number instead"
          onClicked: root.phoneRequested()
        }

        Ui.Button {
          id: useQrButton
          objectName: "useQrButton"
          visible: root.stage === "phone" && root.qr !== ""
          text: "q  Use QR code instead"
          onClicked: root.qrRequested()
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
