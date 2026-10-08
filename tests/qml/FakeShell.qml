import QtQuick

// FakeShell stands in for Omarchy's shell facade (the "shell" property
// Panel receives) in tests that drive a real Panel directly, without a
// real host: hide and summon/toggle call straight through to the
// panel's own close/open, the way Omarchy's shell would. make test-qml
// links this file into every test's root, the same way it links
// Check.js.
//
// tests/qml/Flows and tests/qml/BarWidget each need their own, slightly
// different fake shell (Flows' own records every hide call; BarWidget has
// no panel to drive at all), so they keep their own rather than using
// this one.
QtObject {
  id: root

  // panel is the Panel instance this fake shell controls.
  property var panel: null
  // service is what serviceFor hands back.
  property var service: null

  // hide closes the panel, as Omarchy's shell does when asked to.
  function hide(id) { root.panel.close(); }
  // serviceFor returns the one service every caller here already has.
  function serviceFor(id) { return root.service; }
  // toggle opens the panel with the given payload.
  function toggle(id, payloadJson) { root.panel.open(payloadJson); }
  // summon opens the panel with the given payload, same as toggle.
  function summon(id, payloadJson) { root.panel.open(payloadJson); }
}
