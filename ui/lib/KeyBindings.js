.pragma library

// Merging ~/.config/omamessenger/keys.conf's overrides into Keymap.js's
// defaults. Keymap.js stays the single definition of what each action's
// default keys are; everything here is pure (text in, data out), so
// Service.qml can load the file, call this, and hold the result as the
// effective bindings every key-aware view reads instead of the defaults.

.import "Keymap.js" as Keymap

// configPath returns where the override file lives, following the same
// XDG rule every other user-scoped OmaMessenger path does: XDG_CONFIG_HOME
// when set, otherwise home's own .config.
function configPath(home, xdgConfigHome) {
  const base = xdgConfigHome && xdgConfigHome.length > 0 ? xdgConfigHome : `${home}/.config`;
  return `${base}/omamessenger/keys.conf`;
}

// parseConfig reads keys.conf's text into {overrides, errors}. overrides
// maps an action to the key specs that replace its defaults; errors are
// {line, message} for a line that named an unknown action, gave no valid
// key, or mixed a malformed key in with valid ones. Parsing is forgiving:
// one bad line never stops the rest of the file from taking effect.
function parseConfig(text) {
  const knownActions = new Set(Keymap.BINDINGS.map((b) => b.action));
  const overrides = {};
  const errors = [];

  (text || '').split('\n').forEach((raw, index) => parseLine(raw, index + 1, knownActions, overrides, errors));

  return { overrides, errors };
}

// parseLine applies one line of keys.conf to overrides, or appends an
// error. Blank lines and whole-line comments ("#...") are silently
// skipped, same as any other line a person has not written yet.
function parseLine(rawLine, lineNumber, knownActions, overrides, errors) {
  const line = rawLine.trim();
  if (line === '' || line.startsWith('#')) return;

  const eq = line.indexOf('=');
  if (eq < 0) {
    errors.push({ line: lineNumber, message: `missing "=" in "${line}"` });
    return;
  }

  const action = line.slice(0, eq).trim();
  if (!knownActions.has(action)) {
    errors.push({ line: lineNumber, message: `unknown action "${action}"` });
    return;
  }

  applyKeys(line.slice(eq + 1).trim(), action, lineNumber, overrides, errors);
}

// applyKeys validates action's comma-separated key list, recording a
// malformed one by name and keeping whichever keys on the line did
// parse; an action left with none at all is an error too, since an
// override with no keys would silently unbind it.
function applyKeys(rhs, action, lineNumber, overrides, errors) {
  const tokens = rhs.split(',').map((t) => t.trim()).filter((t) => t !== '');
  const keys = tokens.filter((t) => isValidSpec(t));

  for (const bad of tokens.filter((t) => !isValidSpec(t))) {
    errors.push({ line: lineNumber, message: `malformed key "${bad}" for ${action}` });
  }
  if (keys.length === 0) {
    errors.push({ line: lineNumber, message: `no valid keys for ${action}` });
    return;
  }

  overrides[action] = keys;
}

// isValidSpec reports whether token parses to a real key press, reusing
// Keymap's own parser rather than a second copy of its key-name table.
function isValidSpec(token) {
  const parsed = Keymap.parseSpec(token);
  return Boolean(parsed.text) || (Array.isArray(parsed.keys) && parsed.keys.length > 0);
}

// merge returns {bindings, conflicts}: bindings is Keymap.BINDINGS-shaped,
// each entry carrying its effective keys and an `overridden` flag,
// conflicts lists every case where two actions ended up wanting the same
// key in the same context. An override that causes a conflict loses it:
// the default wins, and the action it belongs to reverts, which can only
// ever remove a conflict (defaults never conflict with each other), so
// one pass per still-active override is always enough to settle.
function merge(bindings, overrides) {
  const reverted = new Set();
  const conflicts = [];
  const seen = new Set();
  const maxPasses = Object.keys(overrides).length + 1;

  for (let pass = 0; pass < maxPasses; pass++) {
    const effective = applyOverrides(bindings, overrides, reverted);
    const found = findConflicts(effective);
    recordConflicts(found, seen, conflicts);
    if (found.length === 0 || !revertOverridden(found, overrides, reverted)) {
      return { bindings: effective, conflicts };
    }
  }

  return { bindings: applyOverrides(bindings, overrides, reverted), conflicts };
}

// applyOverrides clones bindings with each action's override keys
// substituted in, skipping any action already reverted for a conflict.
function applyOverrides(bindings, overrides, reverted) {
  return bindings.map((b) => {
    const override = reverted.has(b.action) ? undefined : overrides[b.action];
    return override
      ? Object.assign({}, b, { keys: override, overridden: true })
      : Object.assign({}, b, { overridden: false });
  });
}

// findConflicts groups bindings by every context string they literally
// list (including "global" as one such group) and checks each group on
// its own. A context-specific binding always wins over a global one for
// the same key (Keymap.match resolves the context's own bindings before
// its global ones), so that pairing is a deliberate, deterministic
// shadow, not a conflict; only two bindings that both claim the very
// same context, specific or both global, can truly never both fire.
function findConflicts(bindings) {
  const groups = new Map();
  for (const b of bindings) {
    for (const context of b.contexts) {
      if (!groups.has(context)) groups.set(context, []);
      groups.get(context).push(b);
    }
  }

  let found = [];
  for (const [context, group] of groups) found = found.concat(conflictsInGroup(context, group));
  return found;
}

// conflictsInGroup reports every key two or more distinct actions in
// group both claim, naming one of the colliding specs for display.
function conflictsInGroup(context, group) {
  const byCanonical = new Map();
  for (const b of group) {
    for (const spec of b.keys) groupByCanonicalSpec(byCanonical, spec, b.action);
  }

  return [...byCanonical.values()]
    .filter((entry) => entry.actions.size > 1)
    .map((entry) => ({ context, key: entry.sample, actions: [...entry.actions].sort() }));
}

// groupByCanonicalSpec records that action claims spec, bucketed by
// spec's canonical identity so "Ctrl+K" and an equivalent respelling of
// the same press land in the same bucket.
function groupByCanonicalSpec(byCanonical, spec, action) {
  const canonical = canonicalSpec(spec);
  const entry = byCanonical.get(canonical) || { actions: new Set(), sample: spec };
  entry.actions.add(action);
  byCanonical.set(canonical, entry);
}

// canonicalSpec turns a key spec into a string that is equal for every
// spec Keymap.specMatches would treat as the same key press.
function canonicalSpec(spec) {
  const parsed = Keymap.parseSpec(spec);
  if (parsed.text) return `text:${parsed.text}`;

  const keys = parsed.keys.slice().sort((a, b) => a - b).join(',');
  return `keys:${keys}|shift:${parsed.shift ? 1 : 0}|ctrl:${parsed.ctrl ? 1 : 0}|alt:${parsed.alt ? 1 : 0}`;
}

// recordConflicts appends each not-yet-seen conflict in found to
// conflicts, deduplicated across merge's passes by action pair, context
// and key, so a conflict found more than once is only reported once.
function recordConflicts(found, seen, conflicts) {
  for (const c of found) {
    const key = `${c.actions.join('|')}@${c.context}@${canonicalSpec(c.key)}`;
    if (seen.has(key)) continue;
    seen.add(key);
    conflicts.push(c);
  }
}

// revertOverridden reverts every overridden action involved in found,
// returning whether it reverted anything; merge stops once a pass
// reverts nothing, since nothing left to try can still be at fault.
function revertOverridden(found, overrides, reverted) {
  let progressed = false;
  for (const c of found) {
    for (const action of c.actions) {
      if (overrides[action] && !reverted.has(action)) {
        reverted.add(action);
        progressed = true;
      }
    }
  }
  return progressed;
}

// groupByContext buckets bindings by every context they apply in
// (including "global" as its own bucket), sorted by context name, for a
// listing that shows what applies where.
function groupByContext(bindings) {
  const groups = new Map();
  for (const b of bindings) {
    for (const context of b.contexts) {
      if (!groups.has(context)) groups.set(context, []);
      groups.get(context).push(b);
    }
  }

  return [...groups.entries()].sort((a, b) => a[0].localeCompare(b[0]));
}

// rows returns the command palette's "Show key bindings" listing: any
// parse errors first, then conflicts, then every effective binding
// grouped by context with a heading row, overrides marked.
function rows(bindings, conflicts, errors) {
  const list = [];
  for (const err of errors) list.push({ label: `Error (line ${err.line})`, detail: err.message, keys: '' });
  for (const c of conflicts) {
    list.push({ label: `Conflict: ${c.actions.join(', ')}`, detail: `${c.context} · the default wins`, keys: Keymap.display(c.key) });
  }
  for (const [context, group] of groupByContext(bindings)) {
    list.push({ label: `— ${context} —`, detail: '', keys: '' });
    for (const b of group) list.push(bindingRow(b));
  }
  return list;
}

// bindingRow is one rows() entry for a single binding.
function bindingRow(b) {
  const keys = b.keys.length > 0 ? b.keys.map((k) => Keymap.display(k)).join(', ') : '(none)';
  return { label: b.label, detail: b.overridden ? 'custom' : 'default', keys };
}

// reportText renders the same information as rows(), as plain text for
// a bug report: `make keys` and the node script behind it print this
// without needing the running shell.
function reportText(bindings, conflicts, errors) {
  const lines = [];
  appendSection(lines, 'Errors', errors.map((e) => `line ${e.line}: ${e.message}`));
  appendSection(lines, 'Conflicts (default wins)', conflicts.map((c) => `${c.context}: ${c.actions.join(', ')} both want ${Keymap.display(c.key)}`));

  for (const [context, group] of groupByContext(bindings)) {
    lines.push(`[${context}]`);
    for (const b of group) lines.push(`  ${b.action}${b.overridden ? ' *' : ''}: ${bindingRow(b).keys}`);
    lines.push('');
  }

  return lines.join('\n');
}

// appendSection adds a titled block of lines to lines, when there is
// anything to show; reportText uses it for its two optional sections.
function appendSection(lines, title, entries) {
  if (entries.length === 0) return;

  lines.push(`${title}:`);
  for (const entry of entries) lines.push(`  ${entry}`);
  lines.push('');
}

// template renders the commented file "Open key bindings file" writes
// when keys.conf does not exist yet: every action, commented out, with
// its default keys, so uncommenting and editing one line is all an
// override takes.
function template(bindings) {
  const lines = [
    '# OmaMessenger key bindings.',
    '#',
    '# One "action = Key[, Key...]" line overrides that action\'s keys,',
    '# replacing its defaults entirely; "#" starts a whole-line comment.',
    '# Modifiers are Ctrl, Shift and Alt, joined with "+", e.g. "Ctrl+Shift+A".',
    '# This file is re-read automatically whenever it changes.',
    '#',
    '# Uncomment a line below and edit its keys to change it.',
    ''
  ];
  for (const b of bindings) lines.push(`# ${b.action} = ${b.keys.join(', ')}  -- ${b.label}`);
  return `${lines.join('\n')}\n`;
}
