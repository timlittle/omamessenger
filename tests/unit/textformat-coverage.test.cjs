// Regression guard against showing someone else's text as rich text by
// accident. QtQuick's Text element defaults to AutoText, which sniffs the
// string and renders anything that looks like HTML as rich text — including
// an <img src="https://attacker/..."> that fetches a remote image just by
// being displayed. A poll question once reached the screen this way (see
// ui/components/PollView.qml), so every Text, Label, TextEdit and TextArea
// in ui/ must say explicitly what it renders, rather than leaning on a
// default that behaves differently depending on what the other person sent.
//
// This cannot check the chosen value is the *right* one — that needs a
// human judgement call about where the text came from, made inline as a
// comment next to each textFormat: line — only that a choice was made
// instead of silently inherited. See textFormatCoverage.test.cjs's sibling
// offscreen test, tests/qml/PlainTextRendering, for proof that untrusted
// text actually renders literally rather than as markup.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const UI_ROOT = path.resolve(__dirname, '../../ui');

// ELEMENT_PATTERN matches the start of a Text, Label, TextEdit or TextArea
// object declaration: the element name immediately followed by "{", not
// preceded by a letter (so "AccessibleText {" or "qs.Ui.TextField {" is
// never mistaken for one) and not part of a longer identifier such as
// "TextField" or "TextMetrics".
const ELEMENT_PATTERN = /(?<![\w.])(Text|Label|TextEdit|TextArea)\s*\{/g;

// qmlFiles walks dir and returns every ".qml" file under it.
function qmlFiles(dir) {
  const found = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      found.push(...qmlFiles(full));
    } else if (entry.name.endsWith('.qml')) {
      found.push(full);
    }
  }
  return found;
}

// blockFor returns the brace-balanced text of the object declaration whose
// "{" starts at openIndex in src, braces included.
function blockFor(src, openIndex) {
  let depth = 0;
  for (let i = openIndex; i < src.length; i++) {
    if (src[i] === '{') depth++;
    else if (src[i] === '}' && --depth === 0) return src.slice(openIndex, i + 1);
  }
  throw new Error('unbalanced braces');
}

// findMissing returns one description per Text/Label/TextEdit/TextArea
// declaration in src that has no textFormat: property anywhere in its own
// block, as "<file>:<line>: <Element>".
function findMissing(file, src) {
  const missing = [];
  for (const match of src.matchAll(ELEMENT_PATTERN)) {
    const openIndex = match.index + match[0].length - 1;
    const block = blockFor(src, openIndex);
    if (!/\btextFormat\s*:/.test(block)) {
      const line = src.slice(0, match.index).split('\n').length;
      missing.push(`${file}:${line}: ${match[1]}`);
    }
  }
  return missing;
}

test('every Text, Label, TextEdit and TextArea in ui/ declares textFormat explicitly', () => {
  const missing = qmlFiles(UI_ROOT).flatMap((file) => findMissing(path.relative(UI_ROOT, file), fs.readFileSync(file, 'utf8')));

  assert.deepEqual(missing, [],
    `${missing.length} element(s) leave textFormat to default to AutoText, which can render sender-controlled ` +
    `text as rich text and fetch a remote image; set textFormat explicitly (PlainText for plain data, RichText ` +
    `only once the input is Format.escapeHtml'd first):\n  ${missing.join('\n  ')}`);
});

test('a deliberately broken expectation is caught', () => {
  const missing = findMissing('fixture.qml', 'Item {\n  Text {\n    text: "hi"\n  }\n}\n');

  assert.deepEqual(missing, ['fixture.qml:2: Text']);
});
