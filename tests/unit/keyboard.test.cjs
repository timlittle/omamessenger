const assert = require("node:assert/strict")
const { escapeAction } = require("../../keyboard.js")

assert.equal(escapeAction({ searchFocused: true, composing: true, conversationOpen: true }), "unfocus-search")
assert.equal(escapeAction({ searchFocused: false, composing: true, conversationOpen: true }), "close-compose")
assert.equal(escapeAction({ searchFocused: false, composing: false, conversationOpen: true }), "back")
assert.equal(escapeAction({ searchFocused: false, composing: false, conversationOpen: false }), "hide-window")

console.log("keyboard Escape routing: 4 checks passed")
