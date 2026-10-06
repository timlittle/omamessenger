// load() reads a QML library file the way the Quickshell/QML engine would:
// it strips the `.pragma library` line, resolves `.import "X.js" as X`
// statements recursively (each import is relative to the *importing*
// file's own directory, not the root), runs the result in a fresh `vm`
// context, and returns every top-level `function` and `var` declaration
// as a plain object. See docs/TASKS.md C8/C9 for the rules this is
// testing: `ui/lib/*.js` files are `.pragma library`, ES5-only, and
// declare only top-level `function`/`var` names.
'use strict';

const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

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

  // Each import's exports object is bound into the new module's global
  // object under its alias, exactly as `.import "X.js" as X` makes `X`
  // available at the top of the importing file.
  const sandbox = {};
  for (const imp of imports) {
    const importedAbsPath = path.resolve(path.dirname(absPath), imp.target);
    sandbox[imp.alias] = loadModule(importedAbsPath, imp.target, cache, nextChain);
  }
  const preseededKeys = new Set(Object.keys(sandbox));

  // Running the stripped source as a vm.Script against a fresh context
  // makes every top-level `var`/`function` declaration a real own
  // property of that context's global object — correct by construction,
  // unlike scanning the source text for declarations. `filename` keeps
  // runtime error stack traces pointing at the real file and line.
  const context = vm.createContext(sandbox);
  try {
    new vm.Script(stripped, { filename: absPath }).runInContext(context);
  } catch (err) {
    err.message = 'In ' + absPath + ': ' + err.message;
    throw err;
  }

  const exportsObj = {};
  for (const key of Object.keys(context)) {
    if (!preseededKeys.has(key)) exportsObj[key] = context[key];
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

module.exports = { load };
