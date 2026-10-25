import QtQuick

// Owns every "send later" message the helper currently has queued,
// kept in step with the helper's scheduledMessage.* events, and the
// calls that queue, edit, cancel or send one right away.
// ConversationView reads forConversation() for the open chat's own
// pending list; WindowController reads all for the "Scheduled
// messages" command. Not driven by the key router: ComposerController
// and WindowController call its functions directly, so handles always
// answers false.
//
// Item rather than QtObject: it holds the Connections child below.
Item {
  id: root

  // service is the Service instance that owns the helper connection.
  property var service: null

  // all is every scheduled message the helper has queued, soonest due
  // first, kept in step with scheduledMessage.* events.
  property var all: []

  // handles reports whether this controller owns action - it owns
  // none, since nothing in Keymap.BINDINGS names it directly.
  function handles(action: string): bool {
    return false;
  }

  // run does nothing: see handles.
  function run(action: string): void {}

  // load fetches every scheduled message from the helper, replacing
  // whatever was loaded before.
  function load(): void {
    if (!root.service) return;

    root.service.request("messages.listScheduled", {}, function(error, result) {
      if (error) return;
      root.all = (result && result.messages) || [];
    });
  }

  // forConversation returns conversationId's own scheduled messages,
  // soonest due first, for the open chat's own pending list.
  function forConversation(conversationId: string): var {
    return root.all.filter((m) => m.conversationId === conversationId);
  }

  // schedule queues text to send to conversationId at sendAt (Unix
  // milliseconds), calling callback(error) once the helper answers.
  function schedule(conversationId: string, text: string, sendAt: real, callback: var): void {
    if (!root.service) return;

    root.service.request("messages.schedule", { conversationId: conversationId, text: text, sendAt: sendAt }, function(error, result) {
      if (!error) root._upsert(result);
      if (callback) callback(error);
    });
  }

  // edit changes a queued message's text and send time, calling
  // callback(error) once the helper answers.
  function edit(id: string, text: string, sendAt: real, callback: var): void {
    if (!root.service) return;

    root.service.request("messages.editScheduled", { scheduledId: id, text: text, sendAt: sendAt }, function(error, result) {
      if (!error) root._upsert(result);
      if (callback) callback(error);
    });
  }

  // cancel removes a queued message before it sends.
  function cancel(id: string): void {
    if (!root.service) return;

    root.service.request("messages.cancelScheduled", { scheduledId: id }, function(error) {
      if (!error) root._remove(id);
    });
  }

  // sendNow sends a queued message right away, instead of waiting for
  // its own time.
  function sendNow(id: string): void {
    if (!root.service) return;

    root.service.request("messages.sendScheduledNow", { scheduledId: id }, function(error) {
      if (!error) root._remove(id);
    });
  }

  // _upsert replaces m in all, or appends it when it is new, keeping
  // the list ordered soonest due first.
  function _upsert(m: var): void {
    const next = root.all.filter((x) => x.id !== m.id);
    next.push(m);
    next.sort((a, b) => a.sendAt - b.sendAt);
    root.all = next;
  }

  // _remove drops id from all, once it is no longer queued.
  function _remove(id: string): void {
    root.all = root.all.filter((m) => m.id !== id);
  }

  Connections {
    target: root.service

    function onEvent(name, data) {
      if (name === "scheduledMessage.added" || name === "scheduledMessage.updated") root._upsert(data);
      else if (name === "scheduledMessage.removed") root._remove(data.scheduledId);
    }
    function onStatusChanged() {
      if (root.service && root.service.status === "ready") root.load();
    }
  }
}
