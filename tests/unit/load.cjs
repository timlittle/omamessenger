// load() reads a QML library file the way the Quickshell/QML engine would:
// it strips the `.pragma library` line, resolves `.import "X.js" as X`
// statements recursively (each import is relative to the *importing*
// file's own directory, not the root), evaluates the result with
// `new Function`, and returns every top-level `function` and `var`
// declaration as a plain object. See docs/TASKS.md C8/C9 for the rules
// this is testing: `ui/lib/*.js` files are `.pragma library`, ES5-only,
// and declare only top-level `function`/`var` names.
'use strict';

const fs = require('node:fs');
const path = require('node:path');

const DEFAULT_ROOT = path.join(__dirname, '..', '..', 'ui');

const PRAGMA_RE = /^\s*\.pragma\s+library\s*$/;
const IMPORT_RE = /^\s*\.import\s+"([^"]+)"\s+as\s+([A-Za-z_$][\w$]*)\s*$/;

// load(relPath, { root }) reads `<root>/relPath` (default root: `<repo>/ui`,
// so `load("lib/Keymap.js")` reads `ui/lib/Keymap.js`) and returns its
// exports. `root` lets tests point at fixtures before `ui/` exists.
function load(relPath, options) {
  const root = (options && options.root) || DEFAULT_ROOT;
  const absPath = path.resolve(root, relPath);
  return loadModule(absPath, relPath, new Map(), []);
}

// loadModule evaluates one file and recurses into its `.import`s.
// `cache` memoizes by absolute path, so a module imported from two places
// (or twice) is only read and evaluated once. `chain` is the list of
// absolute paths currently being loaded, used to detect import cycles.
function loadModule(absPath, label, cache, chain) {
  const cached = cache.get(absPath);
  if (cached) return cached;

  if (chain.includes(absPath)) {
    const cycle = chain.concat(absPath).join('\n  -> ');
    throw new Error('Import cycle detected:\n  ' + cycle);
  }

  const source = readSource(absPath, label);
  const nextChain = chain.concat(absPath);
  const { stripped, imports } = stripDirectives(source);

  const paramNames = imports.map((imp) => imp.alias);
  const paramValues = imports.map((imp) => {
    const importedAbsPath = path.resolve(path.dirname(absPath), imp.target);
    return loadModule(importedAbsPath, imp.target, cache, nextChain);
  });

  const names = topLevelNames(stripped);
  const returnExpr = '{' + names.map((n) => JSON.stringify(n) + ': ' + n).join(', ') + '}';
  // `//# sourceURL=` makes V8 report `absPath` (not `<anonymous>`) in stack
  // traces. `new Function` always prepends a 2-line header ("function
  // anonymous(params\n) {\n") before the body, so a runtime error on body
  // line N is reported at line N + 2 — a fixed, predictable offset.
  const body = stripped + '\nreturn ' + returnExpr + ';\n//# sourceURL=' + absPath;

  let exportsObj;
  try {
    const fn = new Function(...paramNames, body);
    exportsObj = fn(...paramValues);
  } catch (err) {
    err.message = 'In ' + absPath + ': ' + err.message;
    throw err;
  }

  cache.set(absPath, exportsObj);
  return exportsObj;
}

// readSource reads `absPath`, raising a clear error (naming the import
// text and the path it resolved to) when the file does not exist.
function readSource(absPath, label) {
  try {
    return fs.readFileSync(absPath, 'utf8');
  } catch (err) {
    if (err.code === 'ENOENT') {
      throw new Error('Cannot find module "' + label + '" (resolved to ' + absPath + ')');
    }
    throw err;
  }
}

// stripDirectives removes the `.pragma library` line and every
// `.import "X.js" as X` line, replacing each with a blank line so every
// other line keeps its original line number. It returns the rewritten
// source plus the list of `{alias, target}` imports it found, in order.
function stripDirectives(source) {
  const lines = source.split('\n');
  const imports = [];

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (PRAGMA_RE.test(line)) {
      lines[i] = '';
      continue;
    }
    const m = IMPORT_RE.exec(line);
    if (m) {
      imports.push({ target: m[1], alias: m[2] });
      lines[i] = '';
    }
  }

  return { stripped: lines.join('\n'), imports };
}

// topLevelNames scans `source` for `function name(...)` and `var name`
// (including comma lists, `var a, b`) declarations that sit outside any
// `{ ... }` block — the only two kinds of top-level declaration C8 allows
// in `ui/lib/*.js`. String and comment contents are skipped so braces or
// the words "function"/"var" inside them are never mistaken for code.
function topLevelNames(source) {
  const names = [];
  let depth = 0;
  let i = 0;
  const n = source.length;

  while (i < n) {
    const ch = source[i];

    if (ch === '"' || ch === "'") {
      i = skipString(source, i);
      continue;
    }
    if (ch === '/' && source[i + 1] === '/') {
      i = indexOfOr(source, '\n', i, n);
      continue;
    }
    if (ch === '/' && source[i + 1] === '*') {
      i = indexOfOr(source, '*/', i + 2, n) + 2;
      continue;
    }
    if (ch === '{') {
      depth++;
      i++;
      continue;
    }
    if (ch === '}') {
      depth--;
      i++;
      continue;
    }

    if (depth === 0 && keywordAt(source, i, 'function')) {
      const m = /^\s+([A-Za-z_$][\w$]*)/.exec(source.slice(i + 8, i + 208));
      if (m) names.push(m[1]);
      i += 8;
      continue;
    }
    if (depth === 0 && keywordAt(source, i, 'var')) {
      const end = indexOfOr(source, ';', i + 3, n);
      for (const part of source.slice(i + 3, end).split(',')) {
        const m = /^\s*([A-Za-z_$][\w$]*)/.exec(part);
        if (m) names.push(m[1]);
      }
      i = end + 1;
      continue;
    }
    i++;
  }

  return names;
}

// indexOfOr is `source.indexOf(needle, from)`, defaulting to `end` (the
// string length) when the needle never appears, so callers never have to
// special-case an unterminated comment, string or statement.
function indexOfOr(source, needle, from, end) {
  const at = source.indexOf(needle, from);
  return at === -1 ? end : at;
}

// skipString returns the index just past the string literal that starts
// at `start` (which must point at the opening quote).
function skipString(source, start) {
  const quote = source[start];
  let i = start + 1;
  while (i < source.length && source[i] !== quote) {
    i += source[i] === '\\' ? 2 : 1;
  }
  return i + 1;
}

// keywordAt reports whether `word` starts at index `i` on a word
// boundary, so `var` doesn't match inside `variable` or `myvar`.
function keywordAt(source, i, word) {
  if (!source.startsWith(word, i)) return false;
  const before = source[i - 1];
  const after = source[i + word.length];
  return !/[\w$]/.test(before || '') && !/[\w$]/.test(after || '');
}

module.exports = { load };
