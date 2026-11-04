.pragma library

// Limits.js names every single-line editable field's maximumLength, so
// a paste can never hand a field far more text than it was meant to
// hold (see docs/decisions.md). The composer has its own larger cap
// instead (SpellCheck.MAX_COMPOSER_TEXT_LENGTH).

// PALETTE_QUERY bounds the command palette's search field: a command
// label or a conversation title, never anywhere near this long.
var PALETTE_QUERY = 200;

// SEARCH_QUERY bounds the conversation list's search field, the same
// reasoning as PALETTE_QUERY.
var SEARCH_QUERY = 200;

// CONTACT_QUERY bounds the new-chat dialog's contact search field.
var CONTACT_QUERY = 200;

// API_ID bounds Telegram's own api_id field: a decimal integer, never
// anywhere near this many digits.
var API_ID = 32;

// API_HASH bounds Telegram's own api_hash field: a fixed 32-character
// hex string from my.telegram.org, with room to spare.
var API_HASH = 64;

// SETUP_ANSWER bounds whichever of the phone number, login code or
// two-step password fields the current sign-in step is showing.
var SETUP_ANSWER = 128;

// ACCOUNT_ALIAS bounds an account's own display name.
var ACCOUNT_ALIAS = 64;
