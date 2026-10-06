import QtQuick
import "../lib/Actions.js" as Actions
import "../lib/Rpc.js" as Rpc
import "../lib/Setup.js" as Setup

// Owns account setup: adding a Telegram account, with OmaMessenger's own
// app keys or the user's from my.telegram.org, then answering the sign-in steps its connector asks for until it
// connects. The only controller that calls accounts.add, accounts.remove
// and auth.submit. A sign-in step for a saved account whose session ran
// out opens setup too, at that step.
//
// Item rather than QtObject: it holds a Connections child.
Item {
  id: root

  // service is the Service instance that owns the helper connection.
  property var service: null

  // open shows account setup.
  property bool open: false

  // stage is what setup shows: credentials, waiting, qr, phone, code or
  // password.
  property string stage: "credentials"

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

  // _added is true when this setup created the account, so cancelling may
  // remove it. A saved account asking to sign in again is never removed.
  property bool _added: false

  // handles reports whether this controller owns action.
  function handles(action: string): bool {
    return Actions.owner(action) === "account";
  }

  // run performs action, the only entry point a key router needs.
  function run(action: string): void {
    if (action === "account.add") root.begin();
    else if (action === "account.addOwnKeys") root.beginWithOwnKeys();
  }

  // begin adds an account with OmaMessenger's keys and waits for its first
  // sign-in step.
  function begin(): void {
    root._reset();
    root.open = true;
    root.stage = "waiting";
    root._add({ service: "telegram" });
  }

  // beginWithOwnKeys opens setup at the credentials step, for someone who
  // would rather use their own Telegram app.
  function beginWithOwnKeys(): void {
    root._reset();
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
  function usePhone(): void {
    root.stage = "phone";
    root.hint = "Telegram will send a login code to this number.";
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

  Component.onCompleted: {
    if (root.service) root._showStep(root.service.pendingAuth);
  }

  Connections {
    target: root.service

    function onEvent(name, data) {
      if (name === "auth.step") root._showStep(data);
      else if (name === "account.updated") root._accountUpdated(data);
      else if (name === "account.removed" && data.accountId === root.accountId) root._close();
    }
  }
}
