// Regression guard against invisible and bidirectional-control characters
// written literally into source files. They render as nothing, or reorder
// the text around them, so code can read differently in an editor or a
// review from what actually runs (the "Trojan Source" class). A pattern
// that must match them, such as Format.js's HIDDEN_OR_UNUSUAL_CHAR, spells
// each one as a \u escape instead.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const REPO_ROOT = path.resolve(__dirname, '../..');

// SCANNED_DIRS are the trees whose source ships or runs: the UI, the
// helper, the launcher and the scripts, plus these tests themselves.
const SCANNED_DIRS = ['ui', 'backend', 'bin', 'scripts', 'tests'];

// SCANNED_EXTENSIONS are the text source files in those trees; a file with
// no extension is scanned too, since the launcher and scripts have none.
const SCANNED_EXTENSIONS = new Set(['.qml', '.js', '.cjs', '.go', '.sh', '']);

// SKIPPED_DIRS hold fixtures whose bytes are test input, not source.
const SKIPPED_DIRS = new Set(['testdata', 'fixtures', 'node_modules']);

// HIDDEN_CHARACTER matches zero-width characters, the bidirectional
// embedding, override and isolate controls, the invisible word joiners and
// the byte-order mark.
const HIDDEN_CHARACTER = /[\u200B-\u200F\u202A-\u202E\u2060-\u2064\u2066-\u2069\uFEFF]/;

// sourceFiles walks dir and returns every scanned source file under it.
function sourceFiles(dir) {
  const found = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (!SKIPPED_DIRS.has(entry.name)) found.push(...sourceFiles(full));
    } else if (entry.isFile() && SCANNED_EXTENSIONS.has(path.extname(entry.name))) {
      found.push(full);
    }
  }
  return found;
}

test('no source file contains a literal invisible or bidirectional-control character', () => {
  const offenders = [];
  for (const dir of SCANNED_DIRS) {
    const root = path.join(REPO_ROOT, dir);
    if (!fs.existsSync(root)) continue;
    for (const file of sourceFiles(root)) {
      const bytes = fs.readFileSync(file);
      // A built binary (bin/dev holds the test helper) is not source.
      if (bytes.includes(0)) continue;
      const lines = bytes.toString('utf8').split('\n');
      lines.forEach((line, index) => {
        const match = HIDDEN_CHARACTER.exec(line);
        if (match) {
          const code = match[0].codePointAt(0).toString(16).toUpperCase().padStart(4, '0');
          offenders.push(`${path.relative(REPO_ROOT, file)}:${index + 1} U+${code}`);
        }
      });
    }
  }
  assert.deepEqual(offenders, [], 'write these characters as \\u escapes instead');
});
