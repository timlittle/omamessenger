#!/usr/bin/env node
// Prints the effective key bindings (Keymap.js's defaults merged with
// ~/.config/omamessenger/keys.conf's overrides, following XDG_CONFIG_HOME)
// for a bug report, without needing the running shell or a built helper.
// Run by `make keys`; reuses ui/lib the same way the node tests do, through
// tests/unit/load.cjs, since those files are QML library modules, not
// plain Node modules.
'use strict';

const fs = require('node:fs');
const { load } = require('../tests/unit/load.cjs');

const Keymap = load('lib/Keymap.js');
const KeyBindings = load('lib/KeyBindings.js');

main();

// main reads keys.conf (if it exists), merges it with the defaults, and
// prints the result as plain text.
function main() {
  const configPath = KeyBindings.configPath(process.env.HOME || '', process.env.XDG_CONFIG_HOME || '');
  const text = readConfig(configPath);
  const parsed = KeyBindings.parseConfig(text);
  const merged = KeyBindings.merge(Keymap.BINDINGS, parsed.overrides);

  console.log(`Reading ${configPath}`);
  console.log('');
  console.log(KeyBindings.reportText(merged.bindings, merged.conflicts, parsed.errors));
}

// readConfig reads configPath's text, or "" when the file does not
// exist yet, same as Service.qml treats a missing keys.conf.
function readConfig(configPath) {
  try {
    return fs.readFileSync(configPath, 'utf8');
  } catch (err) {
    if (err.code === 'ENOENT') return '';
    throw err;
  }
}
