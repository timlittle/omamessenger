// Regression guard against a shell command string built by concatenating
// an unquoted value into it. `Util.execDetached("<string>")` runs
// <string> as a single command line under `bash -lc` (see Omarchy's own
// Commons/Util.qml), which splits on spaces, quotes and operators the
// string happens to contain — the same way an attacker who controls a
// chat name or a file path (from a photo or video someone else sent, see
// ui/controllers/MediaController.qml's copyToClipboard) could break out
// of the intended `wl-copy ... < path` and run something else entirely.
// The known-good shape wraps every interpolated value in
// `Util.shellQuote(...)` before it joins the string; this check fails on
// any concatenation that skips it. It also catches the opposite mistake
// that once shipped here: wrapping the whole string in an argv array
// (`Util.execDetached(["sh", "-c", "<string>"])`) rather than passing it
// directly - Util.execDetached takes one command string, not an argv
// list, so a list was silently coerced to one unexecutable, comma-joined
// word and copying anything failed every time with no error at all.
//
// This cannot judge whether a *literal* string segment is safe on its own
// merit — only that every non-literal segment went through shellQuote
// rather than being joined in raw. See tests/qml/MediaController for
// proof that copyToClipboard dispatches exactly once; a real wl-copy
// invocation receiving the quoted form is checked by hand (see
// docs/decisions.md), since no automated test here touches the real,
// user-visible clipboard.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const UI_ROOT = path.resolve(__dirname, '../../ui');

// SHELL_STRING_PATTERN finds a call to Util.execDetached, whose single
// argument becomes the command string a shell interprets.
const SHELL_STRING_PATTERN = /Util\.execDetached\(/g;

// ARGV_ARRAY_PATTERN matches the broken shape this file also rejects:
// wrapping the command string in an argv-style array, which
// Util.execDetached does not take.
const ARGV_ARRAY_PATTERN = /^\s*\[\s*"sh"\s*,\s*"-c"\s*,[\s\S]*\]\s*$/;

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

// matchingParen returns the index, in src, of the ")" that closes the
// "(" already consumed up to startIndex (exclusive), skipping over
// parenthesis characters that appear inside a double-quoted string so a
// literal ")" in the command text never ends the scan early.
function matchingParen(src, startIndex) {
  let depth = 1;
  let inString = false;
  for (let i = startIndex; i < src.length; i++) {
    const c = src[i];
    if (inString) {
      if (c === '\\') i++;
      else if (c === '"') inString = false;
      continue;
    }
    if (c === '"') inString = true;
    else if (c === '(') depth++;
    else if (c === ')' && --depth === 0) return i;
  }
  throw new Error('unbalanced parentheses');
}

// splitTerms splits expr on every top-level "+" (ignoring one nested
// inside parentheses or a string literal) and returns each term trimmed.
function splitTerms(expr) {
  const terms = [];
  let depth = 0;
  let inString = false;
  let start = 0;
  for (let i = 0; i < expr.length; i++) {
    const c = expr[i];
    if (inString) {
      if (c === '\\') i++;
      else if (c === '"') inString = false;
      continue;
    }
    if (c === '"') inString = true;
    else if (c === '(') depth++;
    else if (c === ')') depth--;
    else if (c === '+' && depth === 0) {
      terms.push(expr.slice(start, i).trim());
      start = i + 1;
    }
  }
  terms.push(expr.slice(start).trim());
  return terms;
}

// isSafeTerm reports whether term is either a plain string literal with
// no interpolation, or a Util.shellQuote(...) call: the two shapes that
// cannot carry an unescaped value into the shell string.
function isSafeTerm(term) {
  if (/^"([^"\\]|\\.)*"$/.test(term)) return true;
  return /^Util\.shellQuote\(.*\)$/.test(term);
}

// findUnquoted returns one description per Util.execDetached call in src
// whose argument either wraps the command string in the broken argv-array
// shape, or joins in a term that is not a literal or Util.shellQuote(...),
// as "<file>:<line>".
function findUnquoted(file, src) {
  const found = [];
  for (const match of src.matchAll(SHELL_STRING_PATTERN)) {
    const exprStart = match.index + match[0].length;
    const closeIndex = matchingParen(src, exprStart);
    const expr = src.slice(exprStart, closeIndex);
    const unsafe = ARGV_ARRAY_PATTERN.test(expr) || splitTerms(expr).some((term) => term !== '' && !isSafeTerm(term));
    if (unsafe) {
      const line = src.slice(0, match.index).split('\n').length;
      found.push(`${file}:${line}`);
    }
  }
  return found;
}

test('every shell command string built for sh -c quotes its interpolated values with Util.shellQuote', () => {
  const offenders = qmlFiles(UI_ROOT)
    .flatMap((file) => findUnquoted(path.relative(UI_ROOT, file), fs.readFileSync(file, 'utf8')));

  assert.deepEqual(offenders, [],
    `${offenders.length} shell command string(s) join in a value that is not wrapped in Util.shellQuote: a chat ` +
    `name, file path or other value another person or service controls could break out of the intended command:\n  ${offenders.join('\n  ')}`);
});

test('a deliberately broken expectation is caught: an unquoted interpolated value', () => {
  const offenders = findUnquoted('fixture.qml', 'property var f: () => Util.execDetached("wl-copy < " + path)');

  assert.deepEqual(offenders, ['fixture.qml:1']);
});

test('a deliberately broken expectation is caught: the argv-array shape Util.execDetached does not take', () => {
  const offenders = findUnquoted('fixture.qml', 'property var f: () => Util.execDetached(["sh", "-c", "wl-copy < " + Util.shellQuote(path)])');

  assert.deepEqual(offenders, ['fixture.qml:1']);
});

test('the known-good wl-copy shape in MediaController.qml and ConversationController.qml is not flagged', () => {
  for (const file of ['controllers/MediaController.qml', 'controllers/ConversationController.qml']) {
    const src = fs.readFileSync(path.join(UI_ROOT, file), 'utf8');
    assert.deepEqual(findUnquoted(file, src), []);
  }
});
