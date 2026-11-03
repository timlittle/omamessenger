.pragma library

// Verify: marks a self-chosen display name so it reads differently
// from a saved contact's name or the service's own verification, the
// way the official WhatsApp app marks a stranger's own push name with
// a leading "~" and shows their phone number instead of trusting it.

// markedName returns name exactly as given when verified is true, or
// with a leading "~" when it is not (including when verified is
// missing altogether, such as from an older helper: a name is never
// trusted by default). This never looks at name's own text - a
// self-chosen name of "You" still renders "~You", since only the
// helper's own outgoing flag or account id ever means a message is
// the user's own, never a name matching that word.
function markedName(name, verified) {
  return verified ? name : '~' + name;
}

// titleSubtitle is a direct chat's own phone number or id, shown
// beside an unverified title the way the official app shows a
// stranger's number instead of trusting their self-chosen name; ""
// once the title is verified (nothing to add) or the service gave no
// phone number to show at all.
function titleSubtitle(titleVerified, phone) {
  if (titleVerified || !phone) return '';

  return phone;
}
