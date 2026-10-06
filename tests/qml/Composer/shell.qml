// Checks the composer: trimmed submit, whitespace ignored, cleared input
// and a placeholder naming the conversation.
import QtQuick
import Quickshell
import "ui/components"
import "Check.js" as Check

ShellRoot {
  id: root

  property var sent: []

  FloatingWindow {
    implicitWidth: 400
    implicitHeight: 200
    visible: true

    Composer {
      id: composer

      anchors.fill: parent
      title: "Mum"
      onSubmitted: text => root.sent.push(text)
    }
  }

  // Checks run once Quickshell has finished loading; Qt.exit() is ignored
  // before then.
  Timer {
    running: true
    interval: 0
    onTriggered: root.run()
  }

  // run drives the composer and checks the outcome.
  function run(): void {
    composer.text = "  hello  ";
    composer.submit();
    composer.text = "   ";
    composer.submit();

    if (JSON.stringify(root.sent) !== '["hello"]')
      return Check.fail("sent " + JSON.stringify(root.sent) + ", want [\"hello\"]");
    if (composer.text !== "   ")
      return Check.fail("whitespace-only text was cleared or sent");
    if (composer.input.placeholderText.indexOf("Mum") < 0)
      return Check.fail("placeholder \"" + composer.input.placeholderText + "\" does not name the conversation");

    console.log("PASS Composer");
    Qt.exit(0);
  }
}
