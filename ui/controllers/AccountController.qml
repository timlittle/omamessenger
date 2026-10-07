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
    if (action === "account.add") root.begin();
    else if (action === "account.addOwnKeys") root.beginWithOwnKeys();
    else if (action === "account.remove") root.removing = true;
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
