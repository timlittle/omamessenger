import QtQuick
import "../lib/Actions.js" as Actions
import "../lib/Rpc.js" as Rpc

// Owns the delete question and deleting a message: the only controller
// that calls messages.delete. conversation supplies the highlighted
// message id for the "delete the highlighted message" command and the
// loaded timeline this reads to tell whether the target is one of this
// account's own messages, which decides whether "delete for everyone"
// is on offer. The store removes the message and the UI drops its row
// through the same message.removed event a deletion the service reports
// on its own already uses (see ConversationController._removeMessage);
// this controller only ever asks the service, never touches the
// timeline itself.
//
// QtObject rather than Item: it holds no child objects.
QtObject {
  id: root

  // service is the Service instance that owns the helper connection.
  property var service: null

  // conversation is the ConversationController this reads the
  // highlighted message and its conversation id from.
  property var conversation: null

  // open shows the delete question.
  property bool open: false

  // targetId is the message the question is about.
  property string targetId: ""

  // targetIsOwn is true when target was sent by this account, which
  // offers "delete for everyone" as a choice; otherwise only "delete
  // for me" is offered, beside Cancel either way.
  property bool targetIsOwn: false

  // choices are the question's own choices, left to right, matching
  // targetIsOwn: Cancel is always last, its safe default highlight.
  readonly property var choices: root.targetIsOwn ? ["everyone", "forMe", "cancel"] : ["forMe", "cancel"]

  // highlightIndex is the highlighted choice, defaulting to Cancel.
  property int highlightIndex: 0

  // lastError is the safe text of the most recent request failure.
  property string lastError: ""

  // handles reports whether this controller owns action.
  function handles(action: string): bool {
    return Actions.owner(action) === "delete";
  }

  // run performs action, the only entry point a key router needs.
  function run(action: string): void {
    const handlers = {
      "message.delete": () => root.openConfirm(root.conversation.highlightedId),
      "delete.left": () => root._move(-1),
      "delete.right": () => root._move(1),
      "delete.accept": () => root._chooseHighlighted(),
      "delete.everyone": () => root._choose("everyone"),
      "delete.forMe": () => root._choose("forMe"),
      "delete.cancel": () => root.close()
    };

    const handler = handlers[action];
    if (handler) handler();
  }

  // openConfirm shows the question for a message, for the hover
  // toolbar's delete button or the "delete the highlighted message"
  // command. Nothing opens for an id this controller cannot find: there
  // is nothing to say "delete for everyone" or "for me" about.
  function openConfirm(id: string): void {
    if (!id || !root.conversation) return;

    const message = root.conversation.timeline.find(id);
    if (!message) return;

    root.targetId = id;
    root.targetIsOwn = !!message.outgoing;
    root.highlightIndex = root.choices.length - 1;
    root.open = true;
  }

  // close hides the question without deleting anything.
  function close(): void {
    root.open = false;
    root.targetId = "";
  }

  // _move shifts the highlight by delta, clamped to the question's own
  // choices rather than wrapping, the same as the close question's own
  // h/l.
  function _move(delta: int): void {
    const count = root.choices.length;
    root.highlightIndex = Math.max(0, Math.min(count - 1, root.highlightIndex + delta));
  }

  // _chooseHighlighted runs whichever choice is highlighted, for Enter.
  function _chooseHighlighted(): void {
    root._choose(root.choices[root.highlightIndex]);
  }

  // _choose answers the question: "everyone" and "forMe" delete the
  // target, "cancel" leaves it. Asking for "everyone" when it is not
  // on offer (the target is not this account's own message) does
  // nothing, since the mnemonic key for it is always bound.
  function _choose(choice: string): void {
    if (choice === "cancel") { root.close(); return; }
    if (choice === "everyone" && !root.targetIsOwn) return;

    root._delete(choice === "everyone");
  }

  // _delete asks the service to delete the question's target, then
  // closes the question; a failure is reported but the question is
  // closed either way, matching how React reports its own failures.
  function _delete(forEveryone: bool): void {
    const conversationId = root.conversation.activeId;
    const messageId = root.targetId;
    root.close();

    root.service.request("messages.delete",
      { conversationId: conversationId, messageIds: [messageId], forEveryone: forEveryone },
      function(error) {
        if (error) root.lastError = Rpc.errorText(error);
      });
  }
}
