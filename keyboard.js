function escapeAction(state) {
    if (state.searchFocused) return "unfocus-search"
    if (state.composing) return "close-compose"
    if (state.conversationOpen) return "back"
    return "hide-window"
}

if (typeof module !== "undefined") module.exports = { escapeAction }
