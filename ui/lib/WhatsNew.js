.pragma library

// Parses the committed whats-new.json and decides whether the quiet,
// one-time "updated" hint should show. Both are pure: the overlay and
// Service.qml hand this whatever text or versions they already have, and
// never see a thrown error even for a malformed file.

// parse turns whats-new.json's raw text into its releases, newest first,
// as [{version, date, changes}]. Malformed JSON, a missing "releases"
// array, or a release with no version string is dropped rather than
// thrown: a broken file should never crash the overlay, just show less.
function parse(jsonText) {
  let data;
  try {
    data = JSON.parse(jsonText);
  } catch (err) {
    return [];
  }

  if (!data || !Array.isArray(data.releases)) return [];

  return data.releases
    .map(sanitizeRelease)
    .filter((release) => release !== null);
}

// sanitizeRelease validates one release entry, or returns null for one
// with no version to show it under.
function sanitizeRelease(release) {
  if (!release || typeof release !== 'object') return null;

  const version = typeof release.version === 'string' ? release.version : '';
  if (!version) return null;

  const date = typeof release.date === 'string' ? release.date : '';
  const changes = Array.isArray(release.changes)
    ? release.changes.filter((line) => typeof line === 'string' && line.trim() !== '')
    : [];

  return { version, date, changes };
}

// compareVersions orders two "x.y.z" version strings: negative when a is
// older than b, positive when newer, 0 when equal. A missing or
// non-numeric part counts as 0, so "0.3" and "0.3.0" compare equal.
function compareVersions(a, b) {
  const partsA = String(a).split('.');
  const partsB = String(b).split('.');
  const length = Math.max(partsA.length, partsB.length);

  for (let i = 0; i < length; i++) {
    const diff = toInt(partsA[i]) - toInt(partsB[i]);
    if (diff !== 0) return diff > 0 ? 1 : -1;
  }

  return 0;
}

// toInt reads one version segment as a number, or 0 for anything that
// is not a plain integer.
function toInt(part) {
  const n = parseInt(part, 10);
  return Number.isNaN(n) ? 0 : n;
}

// shouldShowHint decides whether the quiet update hint should show:
// true only when seenVersion is a real, older version than
// currentVersion. A fresh install (seenVersion empty) never shows it,
// and neither does seeing the same version again or a seenVersion that
// is not actually older (for example after downgrading a dev build).
function shouldShowHint(currentVersion, seenVersion) {
  if (!currentVersion || !seenVersion) return false;

  return compareVersions(currentVersion, seenVersion) > 0;
}

// hintText is the one-time update hint's own words, shown in the
// window's notice banner.
function hintText(currentVersion) {
  return `Updated to v${currentVersion} · What's new is in the command palette`;
}

// nextPatchVersion returns version with its patch component incremented
// by one: the bump release-please gives a "feat" or "fix" commit while
// this project stays pre-1.0 (bump-patch-for-minor-pre-major in
// release-please-config.json). whats-new.json's top entry may name
// that next version while it is being prepared; see
// tests/unit/whats-new-json.test.cjs.
function nextPatchVersion(version) {
  const parts = String(version).split('.').map(toInt);
  while (parts.length < 3) parts.push(0);
  parts[2] += 1;

  return parts.join('.');
}
