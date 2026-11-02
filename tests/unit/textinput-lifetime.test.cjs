// Regression guard against a text input field that exists even while its
// overlay is closed. A palette or account-setup TextField that is merely
// hidden, not destroyed, still exists as a live Qt object: on supported
// Qt versions it keeps reading a foreign Wayland clipboard offer (whoever
// else on the compositor last copied something) with no size or time
// limit, just by being focusable, long before anyone opens that overlay
// or pastes into it. The fix is a Loader whose `active` is bound to the
// overlay's own open state, so the field is created fresh each time the
// overlay opens and destroyed when it closes.
//
// This does not re-check that a Loader's `active` binding reaches the
// right boolean — that is the controller's job, proven in
// tests/qml/Controllers and friends — only that every component holding
// one of these fields is either on the short allow-list of fields that
// are supposed to always be on screen, or is never instantiated directly
// outside a Loader whose `active` is bound to something other than a
// bare `true`.
'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const UI_ROOT = path.resolve(__dirname, '../../ui');

// ALLOWLIST names every component allowed to hold a text input field
// with no Loader gating it at all, anywhere, each with the one-line
// reason it is safe. Empty: ListColumn's search field and Composer's
// own text input used to be listed here as "always visible, so nothing
// to bound" - true of the component itself, but not of the field inside
// it, which is hidden along with everything else in a narrow window or
// with no conversation open while staying alive underneath. Each now
// gates its own field with an inner Loader instead (see
// fieldSelfGated), so a caller may still instantiate ListColumn or
// Composer freely, uninvolved with this fix.
const ALLOWLIST = {};

// FIELD_PATTERN matches the start of a TextField, TextArea or TextInput
// object declaration, whether bare or namespaced (qs.Ui's "Ui.TextField").
const FIELD_PATTERN = /(?<![\w.])(?:[A-Za-z][\w.]*\.)?(TextField|TextArea|TextInput)\s*\{/g;

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
    else if (src[i] === '}' && --depth === 0) return { text: src.slice(openIndex, i + 1), end: i + 1 };
  }
  throw new Error('unbalanced braces');
}

// hasField reports whether src declares a TextField, TextArea or
// TextInput anywhere in it. FIELD_PATTERN is global and therefore
// stateful between calls, so lastIndex is reset first.
function hasField(src) {
  FIELD_PATTERN.lastIndex = 0;
  return FIELD_PATTERN.test(src);
}

// loaderBlocks returns every "Loader { ... }" block in src, as
// { start, end, text } brace-balanced spans.
function loaderBlocks(src) {
  const blocks = [];
  for (const match of src.matchAll(/Loader\s*\{/g)) {
    const openIndex = match.index + match[0].length - 1;
    const { text, end } = blockFor(src, openIndex);
    blocks.push({ start: match.index, end, text });
  }
  return blocks;
}

// isGated reports whether the component usage at index in src sits inside
// a Loader block that both loads a sourceComponent and binds active to
// something other than a bare "true" (a Loader with active: true is
// always active, which defeats the point of gating).
function isGated(src, index) {
  return loaderBlocks(src).some((block) =>
    index >= block.start && index < block.end &&
    /\bsourceComponent\s*:/.test(block.text) &&
    /\bactive\s*:/.test(block.text) &&
    !/\bactive\s*:\s*true\b/.test(block.text));
}

// findUngatedUsages returns one description per occurrence of
// componentName used as an object type anywhere in src that is not
// inside a gated Loader block, as "<file>:<line>". componentName may
// appear bare or namespaced ("Dialogs.FileDialog {"), the same as
// FIELD_PATTERN allows for TextField and friends, since a type that
// comes from a module import is as likely to be qualified as not.
function findUngatedUsages(file, src, componentName) {
  const pattern = new RegExp(`(?<![\\w.])(?:[A-Za-z][\\w.]*\\.)?${componentName}\\s*\\{`, 'g');
  const found = [];
  for (const match of src.matchAll(pattern)) {
    if (isGated(src, match.index)) continue;
    const line = src.slice(0, match.index).split('\n').length;
    found.push(`${file}:${line}`);
  }
  return found;
}

// KNOWN_CONTAINER_TYPES names component types that never declare a
// TextField, TextArea or TextInput in this codebase's own sources -
// either because they come from Qt itself or from Omarchy's shell - but
// are known, by reading their own implementation, to hold one alive
// for as long as the container object exists, not only while it is
// shown:
//
//   - QtQuick.Dialogs' non-native ColorDialog has ColorInputs' hex
//     field (/usr/lib/qt6/qml/QtQuick/Dialogs/quickimpl/qml/ColorInputs.qml),
//     FontDialog has FontDialogContent's family field
//     (.../FontDialogContent.qml), and MessageDialog has its own
//     detailedTextArea (.../MessageDialog.qml)
//   - Omarchy's Ui.SearchableDropdown, Ui.NumberField and
//     Ui.MultiSelect each wrap a TextField or TextInput internally
//     (/usr/share/omarchy/shell/Ui)
//
// Each is checked exactly like a direct field declaration: a usage must
// sit inside a gated Loader, because the field inside is created when
// the container itself is, never deferred until the container opens.
// FileDialog and FolderDialog are deliberately NOT here: see the
// dedicated, zero-tolerance test below instead of gating them.
const KNOWN_CONTAINER_TYPES = [
  'ColorDialog',
  'FontDialog',
  'MessageDialog',
  'SearchableDropdown',
  'NumberField',
  'MultiSelect',
];

// fieldSelfGated reports whether every TextField, TextArea and
// TextInput declaration in src sits inside a gated Loader block within
// that same file: a component such as ListColumn or Composer that must
// always exist itself, but whose one field inside it must not, gates
// that field with its own inner Loader rather than asking every caller
// to wrap the whole component in one (see each field's own comment).
// A component this is true for needs no entry in ALLOWLIST and no
// caller-side check at all: nothing about how it is instantiated
// matters to this fix, since the field's own lifetime no longer
// follows the component's.
function fieldSelfGated(src) {
  FIELD_PATTERN.lastIndex = 0;
  for (const match of src.matchAll(FIELD_PATTERN)) {
    if (!isGated(src, match.index)) return false;
  }
  return true;
}

test('every TextField, TextArea and TextInput outside the allow-list is only ever instantiated behind a gated Loader', () => {
  const files = qmlFiles(UI_ROOT).map((file) => path.relative(UI_ROOT, file));
  const fieldFiles = files.filter((file) => hasField(fs.readFileSync(path.join(UI_ROOT, file), 'utf8')));
  const toCheck = fieldFiles.filter((file) =>
    !(file in ALLOWLIST) && !fieldSelfGated(fs.readFileSync(path.join(UI_ROOT, file), 'utf8')));

  const offenders = [];
  for (const fieldFile of toCheck) {
    const componentName = path.basename(fieldFile, '.qml');
    for (const file of files) {
      if (file === fieldFile) continue; // a file's own definition never names itself as an object type
      const src = fs.readFileSync(path.join(UI_ROOT, file), 'utf8');
      offenders.push(...findUngatedUsages(file, src, componentName));
    }
  }

  assert.deepEqual(offenders, [],
    `${offenders.length} usage(s) of a text-input-holding component outside a Loader gated on its own open ` +
    `state: a field that exists while its overlay is closed can read a foreign Wayland clipboard offer with no ` +
    `size or time limit. Either add the component to this test's ALLOWLIST with a one-line reason it is always ` +
    `on screen, or load it through a Loader whose active binds to the overlay's own open state:\n  ${offenders.join('\n  ')}`);
});

test('every Qt dialog and known third-party text-input container is only ever instantiated behind a gated Loader', () => {
  const files = qmlFiles(UI_ROOT).map((file) => path.relative(UI_ROOT, file));

  const offenders = [];
  for (const file of files) {
    const src = fs.readFileSync(path.join(UI_ROOT, file), 'utf8');
    for (const containerType of KNOWN_CONTAINER_TYPES) {
      offenders.push(...findUngatedUsages(file, src, containerType));
    }
  }

  assert.deepEqual(offenders, [],
    `${offenders.length} usage(s) of a Qt dialog or other known text-input-holding container outside a Loader ` +
    `gated on its own open state: the field inside is created when the container is, not when it opens, so it ` +
    `keeps reading a foreign Wayland clipboard offer after the dialog closes unless the whole container is torn ` +
    `down with it:\n  ${offenders.join('\n  ')}`);
});

// FileDialog and FolderDialog get no gated-Loader allowance at all,
// unlike every other container above: a security review found Qt's
// own non-native FileDialog (options: FileDialog.DontUseNativeDialog)
// renders a sender-controlled file name as rich text through its own
// delegate's default AutoText, so listing a folder that merely
// contains a markup-named file - already on disk from an earlier
// release, or dropped there by any other means - ran it as HTML with
// no click on anything (see docs/decisions.md). The fix removed the
// composer's and the notification-sound picker's own in-shell
// fallback dialogs outright, in favor of a fixed notice when the
// portal cannot be reached, so this checks that neither type is
// declared anywhere in ui/ at all, gated or not: there is no safe way
// to gate this one back in, only to not have it.
test('FileDialog and FolderDialog are never declared anywhere in the UI, gated or not', () => {
  const files = qmlFiles(UI_ROOT).map((file) => path.relative(UI_ROOT, file));
  const pattern = /(?<![\w.])(?:[A-Za-z][\w.]*\.)?(FileDialog|FolderDialog)\s*\{/;

  const offenders = files.filter((file) => pattern.test(fs.readFileSync(path.join(UI_ROOT, file), 'utf8')));

  assert.deepEqual(offenders, [],
    `${offenders.length} file(s) declare a FileDialog or FolderDialog, which must never exist in this UI again ` +
    `(see docs/decisions.md): ${offenders.join(', ')}`);
});

test('a deliberately broken expectation is caught for a known container type', () => {
  const offenders = findUngatedUsages('fixture.qml', 'Item {\n  FileDialog {\n    title: "x"\n  }\n}\n', 'FileDialog');

  assert.deepEqual(offenders, ['fixture.qml:2']);
});

test('a namespaced known container type is matched the same as a bare one', () => {
  const offenders = findUngatedUsages('fixture.qml', 'Item {\n  Dialogs.FileDialog {\n    title: "x"\n  }\n}\n', 'FileDialog');

  assert.deepEqual(offenders, ['fixture.qml:2']);
});

test('a gated known container type is not flagged', () => {
  const src = 'Item {\n  Loader {\n    active: root.picking\n    sourceComponent: FileDialog {\n      title: "x"\n    }\n  }\n}\n';

  assert.deepEqual(findUngatedUsages('fixture.qml', src, 'FileDialog'), []);
});

test('a deliberately broken expectation is caught', () => {
  const offenders = findUngatedUsages('fixture.qml', 'Item {\n  Dummy {\n    open: true\n  }\n}\n', 'Dummy');

  assert.deepEqual(offenders, ['fixture.qml:2']);
});

test('a Loader with active bound to an overlay state gates its usage', () => {
  const src = 'Item {\n  Loader {\n    active: controller.open\n    sourceComponent: Dummy {\n      open: true\n    }\n  }\n}\n';

  assert.deepEqual(findUngatedUsages('fixture.qml', src, 'Dummy'), []);
});

test('a Loader with active hard-coded to true does not gate its usage', () => {
  const src = 'Item {\n  Loader {\n    active: true\n    sourceComponent: Dummy {\n      open: true\n    }\n  }\n}\n';

  assert.deepEqual(findUngatedUsages('fixture.qml', src, 'Dummy'), ['fixture.qml:4']);
});

test('fieldSelfGated is true when every field in the file sits behind its own gated Loader', () => {
  const src = 'Item {\n  Loader {\n    active: root.shown\n    sourceComponent: Component {\n      TextField {\n        id: f\n      }\n    }\n  }\n}\n';

  assert.equal(fieldSelfGated(src), true);
});

test('fieldSelfGated is false when any field in the file is not gated at all', () => {
  const src = 'Item {\n  Loader {\n    active: root.shown\n    sourceComponent: Component {\n      TextField {\n        id: f\n      }\n    }\n  }\n  TextArea {\n    id: g\n  }\n}\n';

  assert.equal(fieldSelfGated(src), false);
});

// SELF_GATED_FILES names every component whose own field(s) must sit
// behind their own inner Loader, gated on the exact condition that
// shows that field, rather than relying on a caller wrapping the whole
// component in a Loader of its own. AccountSetup and RenameAccount were
// added after a security review found each one's fields merely hidden
// with `visible:` per step inside an overlay that is itself gated at
// the whole-dialog level (Overlays.qml) - WhatsApp can open
// AccountSetup's own overlay unasked (a QR re-pairing after the linked
// device is logged out remotely), so a hidden field left alive for as
// long as that overlay stays open was still reading every foreign
// Wayland clipboard offer the whole time, with no size or time limit,
// through every step the overlay's own Loader has no reason to
// re-create it for (see docs/decisions.md).
const SELF_GATED_FILES = [
  'components/ListColumn.qml',
  'components/Composer.qml',
  'components/AccountSetup.qml',
  'components/RenameAccount.qml',
];

test('every field in SELF_GATED_FILES is gated by its own inner Loader, needing no caller-side Loader', () => {
  for (const file of SELF_GATED_FILES) {
    const src = fs.readFileSync(path.join(UI_ROOT, file), 'utf8');
    assert.equal(fieldSelfGated(src), true, `${file} has a field not gated by its own inner Loader`);
  }
});

// STEP_GATED_ACTIVE_SUBSTRINGS names, for AccountSetup and RenameAccount,
// a substring every one of the dialog's own step conditions must appear
// in at least one Loader's `active` binding somewhere in the file: proof
// that each field is gated on the specific step that shows it, the same
// condition the field used to carry on its own now-removed `visible:`,
// not merely on the whole overlay being open (which fieldSelfGated alone
// cannot tell apart from a single step, since both read as "not a bare
// true" to it). A field toggled only by `visible` has no such binding at
// all, so this also catches a regression back to that shape.
const STEP_GATED_ACTIVE_SUBSTRINGS = {
  'components/AccountSetup.qml': ['stage === "credentials"', 'root._field !== null'],
  'components/RenameAccount.qml': ['stage === "alias"'],
};

test("AccountSetup's and RenameAccount's fields gate on their own step, not merely on the dialog being open", () => {
  for (const [file, substrings] of Object.entries(STEP_GATED_ACTIVE_SUBSTRINGS)) {
    const src = fs.readFileSync(path.join(UI_ROOT, file), 'utf8');
    const activeBindings = [...src.matchAll(/\bactive\s*:\s*([^\n;]+)/g)].map((m) => m[1].trim());

    for (const substring of substrings) {
      assert.ok(activeBindings.some((expr) => expr.includes(substring)),
        `${file}: no field Loader's active binding contains ${JSON.stringify(substring)}; found ${JSON.stringify(activeBindings)}`);
    }
  }
});

// EDIT_INTENT_FLAGS names, for each self-gated field, the private flag
// its own Loader's `active` binding must read: true only from the
// moment the user actually asks to type (a click on its placeholder,
// i/a, Up into an empty draft) until the real field loses focus, never
// merely because the surrounding component is shown. A security review
// found both fields gated on "merely visible" instead - ListColumn's
// own root.visible, Composer's own root.active, itself only ever
// "is the pane open" - which still left each one a clipboard sink
// the whole time its column was on screen, focused or not, exactly
// the gap tests/unit/textinput-lifetime otherwise exists to close (see
// docs/decisions.md). fieldSelfGated above cannot see this: a Loader
// gated on root.visible and one gated on an edit-intent flag both read
// as "not a bare true" to it. This checks the one thing that test
// cannot: that the flag named here, not just any non-literal
// expression, is what each field's own Loader actually reads.
const EDIT_INTENT_FLAGS = {
  'components/Composer.qml': '_writingRequested',
  'components/ListColumn.qml': '_editingSearch'
};

test('the search field and the composer input gate on actually being edited, not merely on being shown', () => {
  for (const [file, flag] of Object.entries(EDIT_INTENT_FLAGS)) {
    const src = fs.readFileSync(path.join(UI_ROOT, file), 'utf8');
    const activeBindings = [...src.matchAll(/\bactive\s*:\s*([^\n;]+)/g)].map((m) => m[1].trim());

    assert.ok(activeBindings.some((expr) => expr.includes(flag)),
      `${file}'s field Loader does not gate on ${flag} (an edit-intent flag that only turns true once the user ` +
      `actually asks to type, and false again the moment the field loses focus); found active: ${JSON.stringify(activeBindings)}`);
  }
});
