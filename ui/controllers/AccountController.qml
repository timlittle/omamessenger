import QtQuick
import "../lib/Actions.js" as Actions
import "../lib/Rail.js" as Rail
import "../lib/Rpc.js" as Rpc
import "../lib/Setup.js" as Setup

// Owns account setup: choosing a service when the helper offers more than
// one, adding an account with OmaMessenger's own app keys (or, for
// Telegram, the user's own from my.telegram.org), then answering the
// sign-in steps its connector asks for until it connects. The only
// controller that calls accounts.add, accounts.remove and auth.submit. A
// sign-in step for a saved account whose session ran out opens setup too,
// at that step. It also removes accounts.
//
// Item rather than QtObject: it holds a Connections child.
Item {
  id: root

  // service is the Service instance that owns the helper connection.
  property var service: null

  // open shows account setup.
  property bool open: false

  // stage is what setup shows: chooseService, credentials, waiting, qr,
  // phone, code or password.
  property string stage: "credentials"

  // chosenService is the service the account being set up belongs to,
  // "telegram" until the user picks another from the chooser.
  property string chosenService: "telegram"

  // accountId is the account being signed in, once the helper has added it.
  property string accountId: ""

  // qr is the QR code to scan, as a base64 PNG.
  property string qr: ""

  // hint explains the current step, from the helper.
  property string hint: ""

  // busy is true while a request is in flight, so it is not sent twice.
  property bool busy: false

  // lastError is the safe text of the most recent failure.
  property string lastError: ""

  // removing shows the question asking which account to remove.
  property bool removing: false

  // chooseIndex is the highlighted row in the service chooser: -1 for
  // Cancel, else an index into service.services.
  property int chooseIndex: -1

  // removeIndex is the highlighted row in the removal question: -1 for
  // Cancel, else an index into service.accounts.
  property int removeIndex: -1

  // navContext names the key-router context for whichever step setup or
  // removal is showing, so each one answers to its own keys instead of
  // one catch-all "setup" context that could not tell them apart; "" when
  // neither is open.
  readonly property string navContext: root.removing ? "removeAccount" : (root.open ? root.stage : "")

  // _added is true when this setup created the account, so cancelling may
  // remove it. A saved account asking to sign in again is never removed.
  property bool _added: false

  // serviceName names the account's service, for AccountSetup's wording.
  readonly property string serviceName: Rail.serviceLabel(root.chosenService, root.service ? root.service.services : [])

  // handles reports whether this controller owns action.
  function handles(action: string): bool {
    return Actions.owner(action) === "account";
  }

  // run performs action, the only entry point a key router needs.
  function run(action: string): void {
    const handlers = {
      "account.add": () => root.begin(),
      "account.addOwnKeys": () => root.beginWithOwnKeys(),
      "account.remove": () => { root.removing = true; root.removeIndex = -1; },
      "setup.down": () => root._moveChoose(1),
      "setup.up": () => root._moveChoose(-1),
      "setup.accept": () => root._acceptChoose(),
      "setup.chooseTelegram": () => root._chooseById("telegram"),
      "setup.chooseWhatsapp": () => root._chooseById("whatsapp"),
      "qr.usePhone": () => root.usePhone(),
      "phone.useQr": () => root.useQr(),
      "phone.back": () => (root.qr ? root.useQr() : root.cancel()),
      "remove.down": () => root._moveRemove(1),
      "remove.up": () => root._moveRemove(-1),
      "remove.accept": () => root._acceptRemove(),
      "remove.cancel": () => root.cancel(),
      "remove.pick1": () => root._pickRemove(0),
      "remove.pick2": () => root._pickRemove(1),
      "remove.pick3": () => root._pickRemove(2),
      "remove.pick4": () => root._pickRemove(3),
      "remove.pick5": () => root._pickRemove(4),
      "remove.pick6": () => root._pickRemove(5),
      "remove.pick7": () => root._pickRemove(6),
      "remove.pick8": () => root._pickRemove(7),
      "remove.pick9": () => root._pickRemove(8)
    };

    const handler = handlers[action];
    if (handler) handler();
  }

  // _moveChoose shifts the service chooser's highlight by delta, wrapping
  // through Cancel the same way j/k already wrap the removal list.
  function _moveChoose(delta: int): void {
    const services = root.service && root.service.services ? root.service.services : [];
    root.chooseIndex = Setup.wrapIndex(root.chooseIndex, delta, services.length);
  }

  // _acceptChoose adds the account for the highlighted service, or
  // cancels setup when nothing is highlighted (Cancel).
  function _acceptChoose(): void {
    const services = root.service && root.service.services ? root.service.services : [];
    const service = services[root.chooseIndex];
    if (service) root.chooseService(service.id);
    else root.cancel();
  }

  // _chooseById adds the account for serviceId straight away, for the
  // chooser's own mnemonic letters; it does nothing when the helper did
  // not offer that service.
  function _chooseById(serviceId: string): void {
    const services = root.service && root.service.services ? root.service.services : [];
    if (services.some((s) => s.id === serviceId)) root.chooseService(serviceId);
  }

  // _moveRemove shifts the removal question's highlight by delta,
  // wrapping through Cancel.
  function _moveRemove(delta: int): void {
    const accounts = root.service && root.service.accounts ? root.service.accounts : [];
    root.removeIndex = Setup.wrapIndex(root.removeIndex, delta, accounts.length);
  }

  // _pickRemove highlights the account at index directly, for the
  // removal question's number mnemonics; out of range does nothing.
  function _pickRemove(index: int): void {
    const accounts = root.service && root.service.accounts ? root.service.accounts : [];
    if (index >= 0 && index < accounts.length) root.removeIndex = index;
  }

  // _acceptRemove removes the highlighted account, or cancels the
  // question when nothing is highlighted (Cancel), for Enter and y.
  function _acceptRemove(): void {
    const accounts = root.service && root.service.accounts ? root.service.accounts : [];
    const account = accounts[root.removeIndex];
    if (account) root.remove(account.id);
    else root.cancel();
  }

  // remove signs an account out and deletes it from this computer; the
  // question closes when the helper reports it removed.
  function remove(accountId: string): void {
    root._call("accounts.remove", { accountId: accountId }, () => {});
  }

  // begin starts adding an account: straight to its own setup when the
  // helper offers only one service, or a chooser when it offers several.
  // A helper too old to report its services defaults to Telegram, as
  // before that list existed.
  function begin(): void {
    const services = root.service && root.service.services ? root.service.services : [];
    if (services.length > 1) {
      root._reset();
      root.open = true;
      root.stage = "chooseService";
      return;
    }

    root._beginService(services.length === 1 ? services[0].id : "telegram");
  }

  // chooseService adds an account for the service picked at the chooser.
  function chooseService(serviceId: string): void {
    root._beginService(serviceId);
  }

  // _beginService adds an account for serviceId with OmaMessenger's own
  // keys and waits for its first sign-in step.
  function _beginService(serviceId: string): void {
    root._reset();
    root.chosenService = serviceId;
    root.open = true;
    root.stage = "waiting";
    root._add({ service: serviceId });
  }

  // beginWithOwnKeys opens setup at the credentials step, for someone who
  // would rather use their own Telegram app. Telegram only: it is the
  // only service whose own API keys OmaMessenger's helper accepts.
  function beginWithOwnKeys(): void {
    root._reset();
    root.chosenService = "telegram";
    root.open = true;
  }

  // submitCredentials adds the account; its connector then reports the
  // first sign-in step.
  function submitCredentials(apiId: string, apiHash: string): void {
    if (!Setup.credentialsReady(apiId, apiHash)) {
      root.lastError = "Enter the API id (a number) and the API hash from my.telegram.org.";
      return;
    }

    root._add({ service: "telegram", apiId: Setup.apiID(apiId), apiHash: apiHash.trim() });
  }

  // _add asks the helper to add the account, then waits for its first
  // sign-in step unless one has already arrived.
  function _add(params: var): void {
    root._call("accounts.add", params, (account) => {
      root.accountId = account.id;
      root._added = true;
      if (root.stage === "credentials") root.stage = "waiting";
    });
  }

  // usePhone switches from the QR code to signing in by phone number.
  // What happens next differs by service (Telegram texts a login code
  // back; WhatsApp shows a code to type on the phone instead), so the
  // hint stays general until that reply names the actual next step.
  function usePhone(): void {
    root.stage = "phone";
    root.hint = "Enter the phone number, with its country code.";
    root.lastError = "";
  }

  // useQr switches back to the QR code already shown, for someone who
  // started typing a phone number by mistake or changed their mind; it
  // does nothing without a QR code cached to go back to.
  function useQr(): void {
    if (!root.qr) return;

    root.stage = "qr";
    root.hint = "";
    root.lastError = "";
  }

  // answer sends what the user typed for the current step.
  function answer(value: string): void {
    const field = Setup.field(root.stage);
    if (!field || !root.accountId) return;

    const step = { accountId: root.accountId, step: field.step, value: value };
    root._call("auth.submit", step, () => { root.stage = "waiting"; root.hint = ""; });
  }

  // cancel closes setup. An account added here that never finished
  // signing in is removed, so it does not linger half added.
  function cancel(): void {
    if (root.removing) {
      root.removing = false;
      root.removeIndex = -1;
      root.lastError = "";
      return;
    }

    const account = (root.service ? root.service.accounts : []).find((a) => a.id === root.accountId);
    if (root._added && (!account || account.status !== "connected")) {
      root.service.request("accounts.remove", { accountId: root.accountId }, function() {});
    }

    root._close();
  }

  // _showStep opens setup at a step the helper asked for.
  function _showStep(step: var): void {
    if (!step || (root.open && root.accountId && step.accountId !== root.accountId)) return;

    root.open = true;
    root.accountId = step.accountId;
    root.stage = step.kind;
    root.qr = step.qr || "";
    root.hint = step.hint || "";
    root.busy = false;
  }

  // _accountUpdated closes setup once the account connects, and shows why
  // when it fails.
  function _accountUpdated(account: var): void {
    if (!root.open || account.id !== root.accountId) return;

    if (account.status === "connected") {
      root._close();
    } else if (account.status === "error") {
      root.lastError = account.detail || "Telegram could not connect.";
      root.busy = false;
    }
  }

  // _call sends a request, marking setup busy until it returns, and runs
  // done with the result if it succeeded.
  function _call(method: string, params: var, done: var): void {
    root.busy = true;
    root.lastError = "";
    root.service.request(method, params, function(error, result) {
      root.busy = false;
      if (error) { root.lastError = Rpc.errorText(error); return; }
      done(result);
    });
  }

  // _accountRemoved closes the removal question, and setup for that account.
  function _accountRemoved(accountId: string): void {
    root.removing = false;
    root.removeIndex = -1;
    if (accountId === root.accountId) root._close();
  }

  // _close hides setup and clears it.
  function _close(): void {
    root.open = false;
    root._reset();
  }

  // _reset clears setup for a new account.
  function _reset(): void {
    root.stage = "credentials";
    root.accountId = "";
    root._added = false;
    root.qr = "";
    root.hint = "";
    root.busy = false;
    root.lastError = "";
    root.chooseIndex = -1;
  }

  // A sign-in waiting for the user is shown again once the service is
  // there, which may be after the panel is created.
  onServiceChanged: if (root.service) root._showStep(root.service.pendingAuth)
  Component.onCompleted: if (root.service) root._showStep(root.service.pendingAuth)

  Connections {
    target: root.service

    function onEvent(name, data) {
      if (name === "auth.step") root._showStep(data);
      else if (name === "account.updated") root._accountUpdated(data);
      else if (name === "account.removed") root._accountRemoved(data.accountId);
    }
  }
}
