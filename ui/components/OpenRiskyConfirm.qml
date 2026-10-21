import QtQuick

// Asks whether to open a received file the helper found to be active or
// executable content - a web page, a vector image carrying a script, or
// anything it could not confirm safe - rather than handing it straight
// to the user's own application. Save to Downloads is the visually
// highlighted, Enter-reachable default, so a stray Enter never runs
// anything; Open anyway is a separate, deliberate choice.
ChoiceDialog {
  id: root

  // confirmedOpen opens the file anyway, bypassing the question.
  signal confirmedOpen()
  // savedToDownloads saves the file into the downloads directory
  // instead of opening it.
  signal savedToDownloads()
  // cancelled leaves the file untouched.
  signal cancelled()

  objectName: "openRiskyConfirm"
  titleObjectName: "openRiskyQuestion"
  title: "This file can run code or load a web page"
  description: "Opening it could run a script or load a web page, not just show you its contents. Save it to Downloads to look at it safely instead."
  choices: [
    { objectName: "cancelButton", text: "n  Cancel", selected: false, value: "cancel" },
    { objectName: "saveButton", text: "s  Save to Downloads", selected: true, value: "save" },
    { objectName: "openButton", text: "y  Open anyway", selected: false, value: "open" }
  ]

  onPicked: value => {
    if (value === "save") root.savedToDownloads();
    else if (value === "open") root.confirmedOpen();
    else root.cancelled();
  }
}
