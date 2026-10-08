.pragma library

// Pure logic for poll bubbles: each option's share of the vote to draw
// as a bar, toggling the highlighted option while voting, and an
// optimistic local update for instant feedback before the helper's
// reply confirms it, shared by the poll bubble and its controller.

// percentages returns one 0-100 share per option in poll.options, of
// however many people voted at all: Telegram and WhatsApp both show a
// poll's bars this way, as a share of voters, not of the raw vote count
// a multiple-choice poll could otherwise push over 100%. A poll with no
// votes yet shows every bar empty rather than dividing by zero.
function percentages(poll) {
  const options = poll && poll.options ? poll.options : [];
  const total = poll && poll.totalVoters > 0 ? poll.totalVoters : 0;
  if (total === 0) {
    return options.map(() => 0);
  }

  return options.map((o) => Math.round((100 * o.votes) / total));
}

// toggleOption returns the selection a vote-mode toggle on optionId
// produces: in a multiple-choice poll, optionId's own membership flips;
// otherwise the new selection always replaces whatever was picked
// before, since a single-choice poll can only ever cast one.
function toggleOption(selected, optionId, multipleChoice) {
  const current = selected || [];

  if (!multipleChoice) {
    return [optionId];
  }

  return current.includes(optionId)
    ? current.filter((id) => id !== optionId)
    : current.concat([optionId]);
}

// voterLabel names a poll's total voters in words, pluralized, for its
// footer.
function voterLabel(totalVoters) {
  const count = totalVoters || 0;
  return count === 1 ? '1 vote' : `${count} votes`;
}

// applyLocalVote returns poll updated as if the service had already
// accepted a vote for optionIds, replacing the signed-in user's own
// previous choice, if any: each option's Chosen flag and vote count
// move to match optionIds, and TotalVoters only grows the first time
// this user votes at all.
function applyLocalVote(poll, optionIds) {
  const options = (poll.options || []).map((o) => {
    const wasChosen = !!o.chosen;
    const nowChosen = optionIds.includes(o.id);
    let votes = o.votes;
    if (wasChosen && !nowChosen) votes -= 1;
    if (!wasChosen && nowChosen) votes += 1;

    return Object.assign({}, o, { votes: Math.max(0, votes), chosen: nowChosen });
  });

  const votedBefore = (poll.options || []).some((o) => o.chosen);
  const totalVoters = votedBefore ? poll.totalVoters : (poll.totalVoters || 0) + 1;

  return Object.assign({}, poll, { options: options, totalVoters: totalVoters });
}
