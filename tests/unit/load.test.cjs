// Tests for tests/unit/load.cjs, the loader `ui/lib/*.js` tests use to pull
// in QML `.pragma library` files under plain Node. `ui/` doesn't exist yet
// (Phase B), so every case here loads its own fixture under
// tests/unit/fixtures/ui/ via the `root` option.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { load } = require('./load.cjs');

const FIXTURES = path.join(__dirname, 'fixtures', 'ui');

function escapeRegExp(s) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

test('exports top-level function and var declarations', () => {
  const basic = load('lib/Basic.js', { root: FIXTURES });
  assert.deepEqual(Object.keys(basic).sort(), ['GREETING', 'add']);
  assert.equal(typeof basic.add, 'function');
  assert.equal(basic.add(2, 3), 5);
  assert.equal(basic.GREETING, 'hi');
});

test('the .pragma library line is stripped, not left as invalid JS', () => {
  const source = fs.readFileSync(path.join(FIXTURES, 'lib/Basic.js'), 'utf8');
  assert.match(source, /^\.pragma library$/m);
  // new Function() would throw a SyntaxError on ".pragma library" if load()
  // failed to strip it; reaching this line proves it did.
  assert.ok(load('lib/Basic.js', { root: FIXTURES }));
});

test('resolves a nested .import chain, each module under its alias', () => {
  const top = load('lib/chain/Top.js', { root: FIXTURES });
  const mid = load('lib/chain/Mid.js', { root: FIXTURES });
  // Leaf.helper(1) = 2; Mid.fromMid() = 2; Top.fromTop() = 3. Getting the
  // right numbers back proves Top -> Mid -> Leaf resolved correctly and
  // each file saw its import under the alias it named.
  assert.equal(mid.fromMid(), 2);
  assert.equal(top.fromTop(), 3);
});

test('imports are relative to the importing file, not the load() root', () => {
  // lib/sub/Deep.js imports "../Basic.js", which only resolves correctly
  // when resolution is relative to lib/sub/, not to FIXTURES itself.
  const deep = load('lib/sub/Deep.js', { root: FIXTURES });
  assert.equal(deep.deep(), 3);
});

test('an import cycle is detected with a clear error', () => {
  assert.throws(
    () => load('lib/cycle/One.js', { root: FIXTURES }),
    (err) => {
      assert.match(err.message, /cycle/i);
      assert.match(err.message, /One\.js/);
      assert.match(err.message, /Two\.js/);
      return true;
    }
  );
});

test('a missing file gives a clear error naming it', () => {
  assert.throws(
    () => load('lib/NoSuchFile.js', { root: FIXTURES }),
    (err) => {
      assert.match(err.message, /NoSuchFile\.js/);
      return true;
    }
  );
});

test('runtime errors keep a useful file name and line number', () => {
  const file = path.join(FIXTURES, 'lib/Thrower.js');
  const lines = fs.readFileSync(file, 'utf8').split('\n');
  const throwLine = lines.findIndex((l) => l.includes('throw new Error')) + 1; // 1-based

  const thrower = load('lib/Thrower.js', { root: FIXTURES });
  assert.throws(
    () => thrower.boom(),
    (err) => {
      // new Function wraps the body in a 2-line header, so the reported
      // line is the original line number plus that fixed offset.
      const expected = new RegExp(escapeRegExp(file) + ':' + (throwLine + 2) + ':');
      assert.match(err.stack, expected);
      return true;
    }
  );
});
