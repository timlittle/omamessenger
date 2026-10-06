## JavaScript (`ui/lib`)

QML's JS engine supports modern ECMAScript (ES2016+), so write current JS: `const`/`let`, arrow functions, template literals, destructuring, spread, `Array.prototype.includes`, optional chaining. It lacks some newer built-ins that node has, such as `Object.fromEntries`: the node tests pass while the UI throws, so check the offscreen QML tests too

- Every file starts with `.pragma library` so QML shares one instance and the file cannot reach into QML scope
- Pure functions only: input in, value out. No QML objects, no timers, no I/O. Anything stateful belongs in `ui/service` or a controller
- Top level holds only `var` and `function` declarations: `tests/unit/load.cjs` runs the file in a `vm` context and exports what lands on its global object, which top-level `const`/`let` do not. Inside functions use `const`/`let`, never `var`. Share code between lib files with `.import "Other.js" as Other`; no `import`/`require`/`export`
- Each file has a header comment saying what it is for; each exported function has a one-line comment
- Name files by domain in PascalCase (`Format.js`, `Keymap.js`), and import them in QML with a capitalised qualifier: `import "../lib/Format.js" as Format`
- Same size limits as Go: ~30-line functions, 50 hard; at most 3 levels of nesting
- Never build HTML from untrusted text without `Format.escapeHtml` first

### Tests

- One test file per lib file: `ui/lib/Format.js` → `tests/unit/format.test.cjs`, using `node:test` and `node:assert`
- Load the file with `tests/unit/load.cjs`. Its values come from another realm, so compare objects with `assert.deepEqual`, not `deepStrictEqual`
- Run with a quoted glob (`node --test 'tests/unit/**/*.test.cjs'`); passing a directory fails on Node ≥ 22
- Coverage gate: 95 % lines and 90 % branches over `ui/lib` (`make test-js`)
