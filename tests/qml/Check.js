.pragma library

// Shared by the offscreen QML tests: ending a test, and finding the items
// a test checks. make test-qml links this file into every test's root.

// fail ends the test with reason on stderr. It returns false, so a check
// can end with `return Check.fail(...)`.
function fail(reason) {
  console.error(`FAIL ${reason}`);
  Qt.exit(1);
  return false;
}

// find returns the descendant of item, or item itself, with objectName
// name, or null.
function find(item, name) {
  if (!item) return null;
  if (item.objectName === name) return item;

  // Some objects have a data property that is not a child list.
  const kids = item.data || item.children;
  if (!kids || typeof kids.length !== 'number') return null;

  for (let i = 0; i < kids.length; i++) {
    const found = find(kids[i], name);
    if (found) return found;
  }
  return null;
}

// texts returns every descendant of item that shows text.
function texts(item) {
  const out = [];
  for (const child of item.children) {
    if (typeof child.text === 'string') out.push(child);
    out.push(...texts(child));
  }
  return out;
}
