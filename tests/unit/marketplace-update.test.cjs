// Tests for scripts/marketplace-update.sh, run with `gh` stubbed on PATH
// so no real GitHub API, and certainly no issue create, is ever reached.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const repo = path.resolve(__dirname, '../..');

// A trimmed copy of the marketplace repository's own verify-plugin.yml
// issue form: the field ids, headings and checkbox wording the script
// must read back verbatim, without the markdown intro and descriptions
// the script never uses.
const TEMPLATE_YAML = `name: Verify or update a listed plugin
body:
  - type: dropdown
    id: verification-action
    attributes:
      label: Verification action
      options:
        - Verify the currently listed snapshot
        - Verify the listed snapshot and enable standard installation
        - Verify and publish a newer upstream commit
    validations:
      required: true

  - type: input
    id: plugin-id
    attributes:
      label: Plugin ID
    validations:
      required: true

  - type: input
    id: repository
    attributes:
      label: Repository URL
    validations:
      required: true

  - type: input
    id: target-commit
    attributes:
      label: Target commit
    validations:
      required: true

  - type: checkboxes
    id: acknowledgment
    attributes:
      label: Verification acknowledgment
      options:
        - label: I understand that only the exact target commit can become a verified marketplace snapshot and that verification is not a security audit.
          required: true

  - type: checkboxes
    id: standard-installation-acknowledgment
    attributes:
      label: Standard installation acknowledgment
      options:
        - label: I confirm that this listed root plugin supports the standard Omarchy installation path and does not require manual setup.
          required: false
`;

const TAG = 'v0.3.2';
const SHA = 'a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2';

const EXPECTED_TITLE = `[Verify]: OmaMessenger ${TAG}`;
const EXPECTED_BODY = `### Verification action

Verify and publish a newer upstream commit

### Plugin ID

io.github.omamessenger

### Repository URL

https://github.com/timlittle/omamessenger

### Target commit

${SHA}

### Verification acknowledgment

- [x] I understand that only the exact target commit can become a verified marketplace snapshot and that verification is not a security audit.

### Standard installation acknowledgment

- [x] I confirm that this listed root plugin supports the standard Omarchy installation path and does not require manual setup.
`;

// stubGh writes a `gh` standing in for the real CLI: it answers the exact
// release, api and issue create calls the script makes, and records any
// issue create into recordDir/title and recordDir/body so a test can
// check what would have been filed, without ever reaching GitHub.
function stubGh(dir, { tag, assets, sha, templateYaml, recordDir }) {
  const b64 = Buffer.from(templateYaml, 'utf8').toString('base64');
  const file = path.join(dir, 'gh');
  fs.writeFileSync(file, `#!/bin/sh
set -eu
case "$1" in
  release)
    if [ "$2" != "view" ]; then echo "gh-stub: unexpected release subcommand: $2" >&2; exit 1; fi
    shift 2
    if [ "$1" = "--json" ]; then
      printf '%s\\n' '${tag}'
      exit 0
    fi
    shift
    case "$2" in
      assets) printf '%s\\n' '${assets}' ;;
      targetCommitish) printf '%s\\n' '${sha}' ;;
      *) echo "gh-stub: unexpected release view field: $2" >&2; exit 1 ;;
    esac
    ;;
  api)
    printf '%s\\n' '${b64}'
    ;;
  issue)
    if [ "$2" != "create" ]; then echo "gh-stub: unexpected issue subcommand: $2" >&2; exit 1; fi
    shift 2
    title=""
    bodyfile=""
    while [ $# -gt 0 ]; do
      case "$1" in
        --title) title=$2; shift 2 ;;
        --body-file) bodyfile=$2; shift 2 ;;
        --repo) shift 2 ;;
        *) shift ;;
      esac
    done
    printf '%s\\n' "$title" > '${recordDir}'/title
    cp "$bodyfile" '${recordDir}'/body
    printf 'https://github.com/omacom/omarchy-plugin-marketplace/issues/1\\n'
    ;;
  *)
    echo "gh-stub: unexpected command: $*" >&2
    exit 1
    ;;
esac
`);
  fs.chmodSync(file, 0o755);
}

// fixture builds a throwaway checkout holding only what the script reads
// (manifest.json, helper-version, its own copy of the script) plus a
// stubbed gh on PATH, and returns how to run it and where a filed issue
// would have been recorded.
function fixture(t, { tag = TAG, assets = 2, sha = SHA, manifestVersion, helperVersion, templateYaml = TEMPLATE_YAML } = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'oma-marketplace-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const version = tag.replace(/^v/, '');

  fs.writeFileSync(path.join(root, 'manifest.json'), JSON.stringify({ version: manifestVersion ?? version }));
  fs.writeFileSync(path.join(root, 'helper-version'), `${helperVersion ?? version} # x-release-please-version\n`);

  fs.mkdirSync(path.join(root, 'scripts'));
  fs.copyFileSync(path.join(repo, 'scripts/marketplace-update.sh'), path.join(root, 'scripts/marketplace-update.sh'));
  fs.chmodSync(path.join(root, 'scripts/marketplace-update.sh'), 0o755);

  const stubBin = path.join(root, 'stub-bin');
  fs.mkdirSync(stubBin);
  const recordDir = path.join(root, 'gh-record');
  fs.mkdirSync(recordDir);
  stubGh(stubBin, { tag, assets, sha, templateYaml, recordDir });

  const env = { ...process.env, PATH: `${stubBin}:${process.env.PATH}` };
  const run = (extraEnv, input) => spawnSync(path.join(root, 'scripts/marketplace-update.sh'), [], {
    encoding: 'utf8', env: { ...env, ...extraEnv }, input,
  });
  return { run, recordDir };
}

test('builds the right title and body from a stubbed release and template', (t) => {
  const { run, recordDir } = fixture(t);
  const result = run({ DRY_RUN: '1' });

  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stdout, `${EXPECTED_TITLE}\n\n${EXPECTED_BODY}`);
  assert.ok(!fs.existsSync(path.join(recordDir, 'title')), 'DRY_RUN must never file the issue');
});

test('refuses a release with no assets yet', (t) => {
  const { run, recordDir } = fixture(t, { assets: 0 });
  const result = run({ DRY_RUN: '1' });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /no assets yet/);
  assert.ok(!fs.existsSync(path.join(recordDir, 'title')));
});

test('refuses when manifest.json disagrees with the release tag', (t) => {
  const { run, recordDir } = fixture(t, { manifestVersion: '0.0.1' });
  const result = run({ DRY_RUN: '1' });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /version mismatch/);
  assert.ok(!fs.existsSync(path.join(recordDir, 'title')));
});

test('refuses when helper-version disagrees with the release tag', (t) => {
  const { run, recordDir } = fixture(t, { helperVersion: '0.0.1' });
  const result = run({ DRY_RUN: '1' });

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /version mismatch/);
  assert.ok(!fs.existsSync(path.join(recordDir, 'title')));
});

test('does not file the issue without a "y" answer', (t) => {
  const { run, recordDir } = fixture(t);
  const result = run({}, 'n\n');

  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /Not filed\.\n$/);
  assert.ok(!fs.existsSync(path.join(recordDir, 'title')));
});

test('files the issue once answered "y"', (t) => {
  const { run, recordDir } = fixture(t);
  const result = run({}, 'y\n');

  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /https:\/\/github\.com\/omacom\/omarchy-plugin-marketplace\/issues\/1/);
  assert.equal(fs.readFileSync(path.join(recordDir, 'title'), 'utf8'), `${EXPECTED_TITLE}\n`);
  assert.equal(fs.readFileSync(path.join(recordDir, 'body'), 'utf8'), EXPECTED_BODY);
});
