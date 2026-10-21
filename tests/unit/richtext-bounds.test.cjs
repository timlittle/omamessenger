// Regression guard against a QML Text or TextEdit rendering sender- or
// service-controlled content as StyledText or RichText with nothing
// bounding how much of it there can be: Qt's rich-text layout does real
// work (parsing tags, laying out a table) proportional to the string's
// own length, so an unbounded one could make the UI do an unbounded
// amount of work for a single message. Every component using one of
// these text formats is on the allow-list below, each with the one-line
// reason its own input is already bounded - by a fixed, short string
// this UI wrote itself, or by a cap applied before the text ever
// reaches it (see ui/lib/Format.js's MAX_RICH_TEXT_INPUT_LENGTH and
// backend's own domain.MaxIncomingTextLength, and docs/decisions.md).
// A new use of either format needs the same review, not just an entry
// added here to silence this test.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const UI_ROOT = path.resolve(__dirname, '../../ui');

// ALLOWLIST names every file allowed to set textFormat to StyledText or
// RichText, each with the one-line reason its input is already bounded.
const ALLOWLIST = {
  'components/MessageBubbleContent.qml': 'Format.messageHtml caps its own input to MAX_RICH_TEXT_INPUT_LENGTH before building any rich text',
  'components/ConversationRow.qml': 'the preview text it shows is the stored message text, already capped at ingest by domain.MaxIncomingTextLength, and shown on one elided line',
  'components/AccountSetup.qml': 'every branch of the sign-in hint it shows is a fixed, short, connector-written sentence, never raw service or sender text (see docs/decisions.md)',
};

// RICH_TEXT_PATTERN matches a textFormat binding set to StyledText or
// RichText, however it is spelled (bare Text.* or TextEdit.*).
const RICH_TEXT_PATTERN = /textFormat\s*:\s*(?:Text|TextEdit)\.(?:StyledText|RichText)\b/;

// qmlFiles walks dir and returns every ".qml" file under it, as paths
// relative to UI_ROOT.
function qmlFiles(dir) {
  const found = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      found.push(...qmlFiles(full));
    } else if (entry.name.endsWith('.qml')) {
      found.push(path.relative(UI_ROOT, full));
    }
  }
  return found;
}

test('every StyledText or RichText use is on the allow-list, with its input already bounded', () => {
  const offenders = qmlFiles(UI_ROOT).filter((file) =>
    !(file in ALLOWLIST) && RICH_TEXT_PATTERN.test(fs.readFileSync(path.join(UI_ROOT, file), 'utf8')));

  assert.deepEqual(offenders, [],
    `${offenders.length} file(s) use StyledText or RichText with no reviewed input bound: ` +
    `${offenders.join(', ')}. Add an entry to this test's ALLOWLIST once its input is bounded, with a one-line reason.`);
});

test('every allow-listed file still actually uses StyledText or RichText', () => {
  const stale = Object.keys(ALLOWLIST).filter((file) =>
    !RICH_TEXT_PATTERN.test(fs.readFileSync(path.join(UI_ROOT, file), 'utf8')));

  assert.deepEqual(stale, [],
    `${stale.join(', ')} no longer use StyledText or RichText; remove from ALLOWLIST`);
});

test('a deliberately broken expectation is caught', () => {
  const src = 'Text {\n  textFormat: Text.StyledText\n}\n';
  assert.equal(RICH_TEXT_PATTERN.test(src), true);
});
