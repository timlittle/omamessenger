import QtQuick
import "../lib/Actions.js" as Actions
import "../lib/Poll.js" as Poll
import "../lib/Rpc.js" as Rpc
import "../lib/Timeline.js" as Timeline

// Owns vote mode for the open conversation's polls: the only controller
// that calls messages.vote. conversation supplies the loaded timeline
// vote mode reads and writes, and the highlighted message id the "vote
// in the highlighted poll" command reads; set it once, from whoever
// wires the controllers together. Unlike ReactionsController's picker,
// which is a popup over the message, vote mode highlights options
// inside the poll bubble itself: PollView reads voteTarget, voteIndex
// and selected straight off this controller to show the same thing a
// mouse click would.
//
// QtObject rather than Item: it holds no child objects.
QtObject {
  id: root

  // service is the Service instance that owns the helper connection.
  property var service: null

  // conversation is the ConversationController whose timeline this
  // controller votes against.
  property var conversation: null

  // voteTarget is the id of the message in vote mode, or "" when none is.
  property string voteTarget: ""

  // voteIndex is the highlighted option while voting.
  property int voteIndex: 0

  // selected are the option ids checked so far, for a multiple-choice
  // poll building up a selection before pollVote.accept casts it.
  property var selected: []

  // lastError is the safe text of the most recent request failure.
  property string lastError: ""

  // handles reports whether this controller owns action.
  function handles(action: string): bool {
    return Actions.owner(action) === "polls";
  }

  // run performs action, the only entry point a key router needs.
  function run(action: string): void {
    const handlers = {
      "message.vote": () => root.openVoting(root.conversation.highlightedId),
      "pollVote.down": () => root.move(1),
      "pollVote.up": () => root.move(-1),
      "pollVote.toggle": () => root.toggleHighlighted(),
      "pollVote.accept": () => root.acceptVote(),
      "pollVote.cancel": () => root.cancelVoting()
    };

    const handler = handlers[action];
    if (handler) handler();
  }

  // pollFor reads the poll a message's media carries, or null when it
  // has none or is not open to voting.
  function pollFor(id: string): var {
    const item = id ? root.conversation.timeline.find(id) : null;
    const media = item ? Timeline.media(item) : null;
    return media && media.kind === "poll" && media.poll && !media.poll.closed ? media.poll : null;
  }

  // openVoting enters vote mode on a message's poll, highlighting its
  // first option, for the "vote in the highlighted poll" command. It
  // does nothing for a message with no open poll.
  function openVoting(id: string): void {
    const poll = root.pollFor(id);
    if (!poll) return;

    root.voteTarget = id;
    root.voteIndex = 0;
    root.selected = poll.options.filter((o) => o.chosen).map((o) => o.id);
  }

  // cancelVoting leaves vote mode without casting anything.
  function cancelVoting(): void {
    root.voteTarget = "";
    root.selected = [];
  }

  // move moves the highlighted option, wrapping at the ends.
  function move(delta: int): void {
    const poll = root.pollFor(root.voteTarget);
    if (!poll || poll.options.length === 0) return;

    const count = poll.options.length;
    root.voteIndex = ((root.voteIndex + delta) % count + count) % count;
  }

  // toggleHighlighted checks or unchecks the highlighted option.
  function toggleHighlighted(): void {
    const poll = root.pollFor(root.voteTarget);
    if (!poll) return;

    const option = poll.options[root.voteIndex];
    root.selected = Poll.toggleOption(root.selected, option.id, poll.multipleChoice);
  }

  // acceptVote casts whatever is checked, or, if nothing was checked
  // yet, the highlighted option alone, so a bare "v" then Enter votes
  // without needing Space first.
  function acceptVote(): void {
    const poll = root.pollFor(root.voteTarget);
    if (!poll) return;

    const optionIds = root.selected.length > 0 ? root.selected : [poll.options[root.voteIndex].id];
    root.castVote(root.voteTarget, optionIds);
  }

  // castVote casts optionIds for message id directly, for a mouse click
  // on an option, which names it without going through the highlight.
  // The bubble updates at once, from Poll.applyLocalVote, before the
  // helper's reply confirms it.
  function castVote(id: string, optionIds: var): void {
    const poll = root.pollFor(id);
    if (!poll || optionIds.length === 0) return;

    root.cancelVoting();

    const item = root.conversation.timeline.find(id);
    const media = Timeline.media(item);
    root.conversation.timeline.setMedia(id, Object.assign({}, media, { poll: Poll.applyLocalVote(poll, optionIds) }));

    root.service.request("messages.vote", { messageId: id, optionIds: optionIds }, function(error, result) {
      if (error) { root.lastError = Rpc.errorText(error); return; }
      root.conversation.applyMessage(result);
    });
  }
}
