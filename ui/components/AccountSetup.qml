pragma ComponentBehavior: Bound
import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import qs.Commons
import qs.Ui as Ui
import "../theme"
import "../lib/Setup.js" as Setup
import "../lib/Limits.js" as Limits
import "../lib/Paste.js" as Paste

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
  // passwordHint is the two-step verification hint the user set for
  // their account, shown under the password field; "" shows nothing.
  property string passwordHint: ""
  // error is the last failure, shown under the step.
  property string error: ""
  // busy disables Continue while a request is in flight.
  property bool busy: false
  // routeKey intercepts key presses before a field or this step's own
  // control handles them; see Composer.qml for why this is a function
  // property, not a signal.
  property var routeKey: null
  // windowActive is whether the user can see the window at all: shown
  // and focused. Every typed field - credentialsLoader's and
  // answerLoader's own, below - exists only while this is also true: a
  // field merely visible while the window sits unfocused in the
  // background - alt-tabbed away - is still a live sink for a foreign
  // Wayland clipboard offer, with no size or time limit (see
  // docs/decisions.md). Defaults true so a caller or test that never
  // sets it (tests/qml/FieldPaste, say) keeps every field exactly as it
  // was before this existed.
  property bool windowActive: true

  // _apiIdDraft and _apiHashDraft hold the credentials step's own typed
  // text outside the field itself: credentialsLoader below destroys and
  // recreates it across a window-focus blip, so something outside it has
  // to be the value's real home, the same reasoning Composer.qml's own
  // text documents. Both always start "" since the credentials step is
  // only ever shown once per setup session.
  property string _apiIdDraft: ""
  property string _apiHashDraft: ""
  // _answerDraft is the same, for whichever of the phone, code or
  // password steps is showing; _focusStep below resets it to "" on
  // every fresh entry to a new one of those steps, since - unlike the
  // credentials step - this one step is reused for several different
  // questions in turn, each of which must start empty.
  property string _answerDraft: ""
  // _focusedField names whichever field last actually gained
  // activeFocus - "apiId", "apiHash" or "answer" - until it loses that
  // focus for a reason other than the window itself going inactive (a
  // real blur: Tab, a click elsewhere); see each field's own
  // onActiveFocusChanged below. windowActive turning true again only
  // restores focus to the field this names, rather than stealing it
  // from whatever the user had instead moved it to (Cancel, say)
  // before switching away.
  property string _focusedField: ""

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
  // linkOpenRequested asks the caller to open url, the hint text's own
  // link: always the same as its own visible text, since root.hint is
  // always a fixed, connector-written string rather than a sender's
  // own formatting (see the comment on the Text below). Reported
  // rather than opened here directly, so the one allow-list check
  // happens where every such request ends up (see
  // WindowController.openLink).
  signal linkOpenRequested(string url, string displayText)

  // _field describes the typed answer the stage asks for, or null.
  readonly property var _field: Setup.field(root.stage)

  // _credentials exposes credentialsLoader's own item, or null while the
  // credentials step is not showing (see the Loader's own comment
  // below). A plain var, not an alias, since Loader.item has no
  // declared type for qmllint to check members against (the same
  // reasoning Composer.qml's own `input` property documents).
  property var _credentials: credentialsLoader.item
  // _answer exposes answerLoader's own item, or null while none of the
  // phone, code or password steps is showing; see _credentials above.
  property var _answer: answerLoader.item

  // submit continues from the current step, reading whatever was typed
  // from _apiIdDraft/_apiHashDraft/_answerDraft rather than the live
  // field: those hold the truth even for the instant right after a
  // window-focus blip destroys it (see windowActive above). _credentials
  // and _answer only exist while their own step is the one showing (see
  // each Loader's own comment below), which is exactly when submit can
  // be called for that step, so a missing one here would itself be a bug
  // - but the guard costs nothing and keeps this from reading the wrong
  // step's answer.
  function submit(): void {
    if (root.busy) return;
    if (root.stage === "credentials" && root._credentials) {
      root.credentialsSubmitted(root._apiIdDraft, root._apiHashDraft);
    } else if (root._field && root._answer) {
      root.answered(root._answerDraft);
    }
  }

  // _focusStep moves real keyboard focus to the stage's own control: a
  // text field for credentials, a phone number, a code or a password, and
  // this Item itself for every other step, so j/k, the mnemonic letters
  // and p/q reach it through routeKey rather than needing a focused
  // button of their own. Deferred with Qt.callLater by every caller
  // below, so by the time this runs, credentialsLoader's and
  // answerLoader's own active bindings (bound to the same stage this
  // reads) have already created or destroyed their item.
  function _focusStep(): void {
    if (root.stage === "credentials") { if (root._credentials) root._credentials.idField.forceActiveFocus(); return; }
    if (root._field) {
      root._answerDraft = "";
      if (root._answer) { root._answer.text = ""; root._answer.forceActiveFocus(); }
      return;
    }
    root.forceActiveFocus();
  }

  // _restoreFocus puts keyboard focus back in whichever field actually
  // had it when the window last went inactive (see _focusedField
  // above), once windowActive turns true again: a user alt-tabbing away
  // to copy a login code or an api_hash, then back, must not also lose
  // their place. Deferred with Qt.callLater, for the same reason
  // _focusStep already is.
  function _restoreFocus(): void {
    if (root._focusedField === "apiId" && root._credentials) root._credentials.idField.forceActiveFocus();
    else if (root._focusedField === "apiHash" && root._credentials) root._credentials.hashField.forceActiveFocus();
    else if (root._focusedField === "answer" && root._answer) root._answer.forceActiveFocus();
  }

  objectName: "accountSetup"
  anchors.fill: parent
  onStageChanged: Qt.callLater(root._focusStep)
  onVisibleChanged: if (root.visible) Qt.callLater(root._focusStep)
  // Setup is only ever instantiated while open (see Overlays.qml, which
  // destroys it on close instead of just hiding it, so its credential,
  // phone, code and password fields cannot read the clipboard while
  // hidden): onVisibleChanged above only fires on a later toggle, never
  // for the value a binding already had when this object was created,
  // so this is what actually focuses the first step's field.
  Component.onCompleted: Qt.callLater(root._focusStep)
  // See _restoreFocus above.
  onWindowActiveChanged: if (root.windowActive) Qt.callLater(root._restoreFocus)

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
        color: Theme.foreground
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
        // StyledText is safe here because every branch, including
        // root.hint, is always a fixed string the connector itself
        // wrote (QRHint, a fixed per-step sentence, or whatsapp's
        // pairByPhoneFailedHint): never a raw error or any other text
        // a service returns, which is exactly what let a crafted
        // IQError turn into a clickable link here before. root.serviceName
        // comes from the helper's own fixed service list, never a
        // contact or message. onLinkActivated still reports through
        // linkOpenRequested rather than opening anything itself, so
        // even a future hint that turned out not to be fixed could
        // only ever open an allowed scheme (see
        // WindowController.openExternalLink), never load a remote
        // "<img>" or open anything else.
        textFormat: Text.StyledText
        wrapMode: Text.WordWrap
        linkColor: Theme.accent
        color: Util.alpha(Theme.foreground, 0.7)
        font { family: Theme.font.family; pixelSize: Theme.font.body }
        onLinkActivated: link => root.linkOpenRequested(link, link)
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

      // credentialsLoader holds the API id and hash fields, active only
      // for the credentials step and only while the window is active
      // (see windowActive above): a field merely hidden with `visible:`,
      // or merely visible while the window sits unfocused in the
      // background, is still a live Qt object that keeps reading every
      // foreign Wayland clipboard offer, with no size or time limit, and
      // WhatsApp's own connector can open this whole overlay unasked (a
      // QR re-pairing after the linked device is logged out remotely),
      // so the fields must actually be destroyed once this is not the
      // step shown, not just hidden behind the waiting, qr, phone, code
      // or password steps that can follow (see docs/decisions.md).
      // _apiIdDraft/_apiHashDraft are what each field is seeded from and
      // mirrors every keystroke into, so recreating them on return from
      // an alt-tab loses nothing the user typed.
      Loader {
        id: credentialsLoader
        Layout.fillWidth: true
        active: root.stage === "credentials" && root.windowActive

        sourceComponent: ColumnLayout {
          // idField and hashField expose the two fields so root.submit
          // and root._focusStep can reach them through this Loader's
          // own item, without either field needing to be known outside
          // this Component.
          property alias idField: idFieldInner
          property alias hashField: hashFieldInner

          spacing: Theme.spacing.md

          Ui.TextField {
            id: idFieldInner
            objectName: "apiIdField"
            Layout.fillWidth: true
            text: root._apiIdDraft
            placeholderText: "API id"
            maximumLength: Limits.API_ID
            // Qt's own right-click "Paste" menu would read the
            // clipboard in-process, with no size or time limit of its
            // own; routeKey below already routes Ctrl+V and
            // Shift+Insert through the helper's bounded
            // clipboard.readText instead (see docs/decisions.md), so
            // this menu is removed outright - this field invites
            // exactly that paste, from my.telegram.org.
            ContextMenu.menu: null
            KeyNavigation.tab: hashFieldInner
            Keys.priority: Keys.BeforeItem
            // Catches a native paste key (ui/lib/Paste.js's own
            // isPasteKey, which covers Ctrl+Shift+Insert and the
            // dedicated Paste key on top of Ctrl+V and Shift+Insert)
            // ahead of the binding lookup below, so a rebound
            // field.paste in keys.conf can never bring back this
            // field's own unbounded native paste() (see
            // docs/decisions.md).
            Keys.onPressed: event => {
              if (Paste.isPasteKey(event.key, event.modifiers)) {
                if (root.routeKey) root.routeKey(event.key, event.modifiers, event.text, true);
                event.accepted = true;
                return;
              }
              if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
            }
            onAccepted: hashFieldInner.forceActiveFocus()
            onTextChanged: root._apiIdDraft = text
            // Losing focus because the window itself went inactive is
            // not a real blur: root.windowActive is already false by
            // the time this runs, since that is what emptied
            // credentialsLoader.active in the first place, so
            // _focusedField is left alone for that case and only
            // cleared for an actual Tab or a click elsewhere.
            onActiveFocusChanged: {
              if (activeFocus) { root._focusedField = "apiId"; return; }
              if (root.windowActive) root._focusedField = "";
            }

            MiddleClickGuard {}
          }

          Ui.TextField {
            id: hashFieldInner
            objectName: "apiHashField"
            Layout.fillWidth: true
            text: root._apiHashDraft
            placeholderText: "API hash"
            maximumLength: Limits.API_HASH
            ContextMenu.menu: null
            Keys.priority: Keys.BeforeItem
            // Catches a native paste key (ui/lib/Paste.js's own
            // isPasteKey, which covers Ctrl+Shift+Insert and the
            // dedicated Paste key on top of Ctrl+V and Shift+Insert)
            // ahead of the binding lookup below, so a rebound
            // field.paste in keys.conf can never bring back this
            // field's own unbounded native paste() (see
            // docs/decisions.md).
            Keys.onPressed: event => {
              if (Paste.isPasteKey(event.key, event.modifiers)) {
                if (root.routeKey) root.routeKey(event.key, event.modifiers, event.text, true);
                event.accepted = true;
                return;
              }
              if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
            }
            onAccepted: root.submit()
            onTextChanged: root._apiHashDraft = text
            // See idFieldInner's own onActiveFocusChanged above.
            onActiveFocusChanged: {
              if (activeFocus) { root._focusedField = "apiHash"; return; }
              if (root.windowActive) root._focusedField = "";
            }

            MiddleClickGuard {}
          }
        }
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

      // answerLoader holds the phone, code and password field, active
      // only while this step asks for one (root._field is non-null for
      // exactly those three steps) and only while the window is active
      // (see windowActive above): the same reasoning as credentialsLoader
      // above, including WhatsApp's own ability to reopen this overlay at
      // the qr or waiting step with nothing typed. _answerDraft is what
      // the field is seeded from and mirrors every keystroke into, so
      // recreating it on return from an alt-tab loses nothing typed so
      // far for this step.
      Loader {
        id: answerLoader
        Layout.fillWidth: true
        active: root._field !== null && root.windowActive

        sourceComponent: Ui.TextField {
          id: answerFieldInner
          objectName: "answerField"
          text: root._answerDraft
          placeholderText: root._field ? root._field.placeholder : ""
          password: root._field ? root._field.password : false
          maximumLength: Limits.SETUP_ANSWER
          ContextMenu.menu: null
          Keys.priority: Keys.BeforeItem
          // Catches a native paste key (ui/lib/Paste.js's own
          // isPasteKey, which covers Ctrl+Shift+Insert and the
          // dedicated Paste key on top of Ctrl+V and Shift+Insert)
          // ahead of the binding lookup below, so a rebound
          // field.paste in keys.conf can never bring back this
          // field's own unbounded native paste() (see
          // docs/decisions.md).
          Keys.onPressed: event => {
            if (Paste.isPasteKey(event.key, event.modifiers)) {
              if (root.routeKey) root.routeKey(event.key, event.modifiers, event.text, true);
              event.accepted = true;
              return;
            }
            if (root.routeKey && root.routeKey(event.key, event.modifiers, event.text)) event.accepted = true
          }
          onAccepted: root.submit()
          onTextChanged: root._answerDraft = text
          // Losing focus because the window itself went inactive is not
          // a real blur: root.windowActive is already false by the time
          // this runs, since that is what emptied answerLoader.active in
          // the first place, so _focusedField is left alone for that
          // case and only cleared for an actual Tab or a click
          // elsewhere.
          onActiveFocusChanged: {
            if (activeFocus) { root._focusedField = "answer"; return; }
            if (root.windowActive) root._focusedField = "";
          }

          MiddleClickGuard {}
        }
      }

      Text {
        objectName: "passwordHint"
        Layout.fillWidth: true
        visible: root.stage === "password" && root.passwordHint !== ""
        text: "Hint: " + root.passwordHint
        // PlainText: the hint is whatever the user typed into Telegram
        // when they set their password, so it is untrusted text.
        textFormat: Text.PlainText
        wrapMode: Text.WordWrap
        color: Util.alpha(Theme.foreground, 0.7)
        font { family: Theme.font.family; pixelSize: Theme.font.bodySmall }
      }

      Text {
        Layout.fillWidth: true
        visible: root.error !== ""
        text: root.error
        textFormat: Text.PlainText
        wrapMode: Text.WordWrap
        color: Theme.urgent
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
