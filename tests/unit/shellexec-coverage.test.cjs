// Regression guard against a shell command string built by concatenating
// an unquoted value into it. `Util.execDetached(["sh", "-c", "<string>"])`
// hands that whole string to a shell, which splits on spaces, quotes and
// operators the string happens to contain — the same way an attacker who
// controls a chat name or a file path (from a photo or video someone
// else sent, see ui/controllers/MediaController.qml's copyToClipboard)
// could break out of the intended `wl-copy ... < path` and run something
// else entirely. The known-good shape wraps every interpolated value in
// `Util.shellQuote(...)` before it joins the string; this check fails on
// any concatenation that skips it.
//
// This cannot judge whether a *literal* string segment is safe on its own
// merit — only that every non-literal segment went through shellQuote
// rather than being joined in raw. See tests/qml/MediaController for
// proof that a real wl-copy invocation actually receives the quoted form.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const UI_ROOT = path.resolve(__dirname, '../../ui');

// SHELL_STRING_PATTERN finds an array literal that opens a shell with
// `"sh", "-c",` followed by the expression that becomes the command
// string: the one place a concatenated value is handed to a shell for
// interpretation, rather than passed as its own separate argv word.
const SHELL_STRING_PATTERN = /\[\s*"sh"\s*,\s*"-c"\s*,/g;

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

// matchingBracket returns the index, in src, of the "]" that closes the
// "[" already consumed up to startIndex (exclusive), skipping over
// bracket characters that appear inside a double-quoted string so a
// literal "]" in the command text never ends the scan early.
function matchingBracket(src, startIndex) {
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
    else if (c === '[') depth++;
    else if (c === ']' && --depth === 0) return i;
  }
  throw new Error('unbalanced brackets');
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

// findUnquoted returns one description per shell-string concatenation in
// src that joins in a term that is not a literal or Util.shellQuote(...),
// as "<file>:<line>".
function findUnquoted(file, src) {
  const found = [];
  for (const match of src.matchAll(SHELL_STRING_PATTERN)) {
    const exprStart = match.index + match[0].length;
    const closeIndex = matchingBracket(src, exprStart);
    const expr = src.slice(exprStart, closeIndex);
    const unsafe = splitTerms(expr).some((term) => term !== '' && !isSafeTerm(term));
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

test('a deliberately broken expectation is caught', () => {
  const offenders = findUnquoted('fixture.qml', 'property var f: () => Util.execDetached(["sh", "-c", "wl-copy < " + path])');

  assert.deepEqual(offenders, ['fixture.qml:1']);
});

test('the known-good wl-copy shape in MediaController.qml is not flagged', () => {
  const src = fs.readFileSync(path.join(UI_ROOT, 'controllers/MediaController.qml'), 'utf8');

  assert.deepEqual(findUnquoted('controllers/MediaController.qml', src), []);
});
