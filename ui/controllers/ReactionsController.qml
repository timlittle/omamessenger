import QtQuick
import "../lib/Actions.js" as Actions
import "../lib/Reactions.js" as Reactions
import "../lib/Rpc.js" as Rpc

// Owns the open conversation's reaction chips and the emoji picker: the
// only controller that calls messages.react. conversation supplies the
// loaded timeline the chips and the "react to the newest message"
// command read and write; set it once, from whoever wires the
// controllers together.
//
// QtObject rather than Item: it holds no child objects.
QtObject {
  id: root

  // service is the Service instance that owns the helper connection.
  property var service: null

  // conversation is the ConversationController whose timeline this
  // controller reacts against.
  property var conversation: null

  // pickerOpen shows the emoji picker.
  property bool pickerOpen: false

  // pickerTarget is the id of the message the picker reacts to.
  property string pickerTarget: ""

  // pickerIndex is the highlighted emoji in the picker.
  property int pickerIndex: 0

  // pickerEmojis are the picker's choices, in order.
  readonly property var pickerEmojis: Reactions.COMMON_EMOJI

  // lastError is the safe text of the most recent request failure.
  property string lastError: ""

  // handles reports whether this controller owns action.
  function handles(action: string): bool {
    return Actions.owner(action) === "reactions";
  }

  // run performs action, the only entry point a key router needs.
  function run(action: string): void {
    const handlers = {
      "message.react": () => root.openPicker(root.conversation.timeline.newestId()),
      "reaction.left": () => root.movePicker(-1),
      "reaction.right": () => root.movePicker(1),
      "reaction.accept": () => root.acceptPicker()
    };

    const handler = handlers[action];
    if (handler) handler();
  }

  // react toggles the message's reaction with emoji: clicking a chip
  // already the user's own clears it, any other pick replaces it. The
  // chips update at once, from Reactions.applyLocal, before the helper's
  // reply confirms them.
  function react(id: string, emoji: string): void {
    if (!id) return;

    const timeline = root.conversation.timeline;
    const message = timeline.find(id);
    const toSend = Reactions.emojiToSend(message ? message.reactions : [], emoji);
    timeline.setReactions(id, Reactions.applyLocal(message ? message.reactions : [], toSend));

    root.service.request("messages.react", { messageId: id, emoji: toSend }, function(error, result) {
      if (error) { root.lastError = Rpc.errorText(error); return; }
      root.conversation.applyMessage(result);
    });
  }

  // openPicker shows the emoji picker for a message, for the "+" chip or
  // the "react to the newest message" command.
  function openPicker(id: string): void {
    if (!id) return;

    root.pickerTarget = id;
    root.pickerIndex = 0;
    root.pickerOpen = true;
  }

  // closePicker hides the emoji picker without reacting.
  function closePicker(): void {
    root.pickerOpen = false;
    root.pickerTarget = "";
  }

  // movePicker moves the picker's highlight, wrapping at the ends.
  function movePicker(delta: int): void {
    const count = root.pickerEmojis.length;
    if (count === 0) return;

    root.pickerIndex = ((root.pickerIndex + delta) % count + count) % count;
  }

  // acceptPicker reacts to the picker's target with the highlighted
  // emoji, then closes the picker.
  function acceptPicker(): void {
    root.pickAt(root.pickerIndex);
  }

  // pickAt reacts to the picker's target with the emoji at index, then
  // closes the picker; a click in the picker names its own index
  // directly, rather than going through the highlight.
  function pickAt(index: int): void {
    const id = root.pickerTarget;
    const emoji = root.pickerEmojis[index];
    root.closePicker();
    root.react(id, emoji);
  }
}
