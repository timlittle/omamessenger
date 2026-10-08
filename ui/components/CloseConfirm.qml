import QtQuick

// Asks what closing the window should do: keep OmaMessenger running in
// the background, so messages still notify, or quit until it is opened
// again. h/l (or Left/Right) move a highlight across Cancel, Quit and
// Keep, Enter chooses it, a mnemonic letter jumps straight to one, and
// Escape cancels. Every button stays clickable by mouse too.
ChoiceDialog {
  id: root

  // highlightIndex is the caller's highlighted choice: 0 Cancel, 1 Quit,
  // 2 Keep in background, left to right.
  property int highlightIndex: 2

  // keep hides the window and keeps notifying.
  signal keep()
  // quit stops OmaMessenger until the window is opened again.
  signal quit()
  // cancelled leaves the window open.
  signal cancelled()

  objectName: "closeConfirm"
  title: "Close OmaMessenger?"
  description: "Keep it running to get notifications for new messages, or quit until you open it again."
  choices: [
    { objectName: "cancelButton", text: "c  Cancel", selected: root.highlightIndex === 0, value: "cancel" },
    { objectName: "quitButton", text: "q  Quit", selected: root.highlightIndex === 1, value: "quit" },
    { objectName: "keepButton", text: "k  Keep in background", selected: root.highlightIndex === 2, value: "keep" }
  ]

  onPicked: value => {
    if (value === "quit") root.quit();
    else if (value === "keep") root.keep();
    else root.cancelled();
  }
}
