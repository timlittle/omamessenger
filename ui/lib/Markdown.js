.pragma library

// Recognizes GitHub-style pipe tables inside a message's plain text, so
// the caller can render them as real HTML tables instead of letting the
// padded columns and dash separator smear across many wrapped lines.
// Everything else about the message (escaping, linkifying) stays the
// caller's job: this file only finds table shapes and lays out the HTML
// for one it has already found.

// SEPARATOR_CELL matches one cell of a table's separator row: a run of
// dashes with optional leading/trailing colons for alignment.
var SEPARATOR_CELL = /^:?-+:?$/;

// splitTables breaks text into an ordered list of parts, each either
// { kind: 'text', text } or { kind: 'table', table }. A table is only
// recognized when a header row is immediately followed by a matching
// separator row, so a stray "|" in ordinary prose is never mistaken for
// one.
function splitTables(text) {
  const lines = text.split('\n');
  const parts = [];
  let plain = [];

  for (let i = 0; i < lines.length;) {
    const found = tryParseTable(lines, i);
    if (!found) {
      plain.push(lines[i]);
      i++;
      continue;
    }
    if (plain.length > 0) {
      parts.push({ kind: 'text', text: plain.join('\n') });
      plain = [];
    }
    parts.push({ kind: 'table', table: found.table });
    i = found.next;
  }

  if (plain.length > 0) {
    parts.push({ kind: 'text', text: plain.join('\n') });
  }
  return parts;
}

// tryParseTable checks whether a table starts at lines[start], and if so
// returns its parsed rows and the index of the line after its last body
// row. It returns null when the header has no separator row beneath it.
function tryParseTable(lines, start) {
  if (start + 1 >= lines.length || !lines[start].includes('|')) {
    return null;
  }

  const header = splitRow(lines[start]);
  const separator = splitRow(lines[start + 1]);
  if (separator.length !== header.length || !separator.every((cell) => SEPARATOR_CELL.test(cell))) {
    return null;
  }

  const aligns = separator.map(cellAlign);
  const rows = [];
  let i = start + 2;
  while (i < lines.length && isBodyRow(lines[i])) {
    rows.push(padRow(splitRow(lines[i]), header.length));
    i++;
  }
  return { next: i, table: { header, aligns, rows } };
}

// isBodyRow reports whether line continues a table: a non-blank line
// that still delimits cells with "|". A blank line, or one without any
// pipe, ends the table.
function isBodyRow(line) {
  return line.trim() !== '' && line.includes('|');
}

// splitRow splits one table row into trimmed cells, dropping a leading
// and trailing "|" when the row has them: both are optional in GitHub's
// table syntax.
function splitRow(line) {
  let row = line.trim();
  if (row.startsWith('|')) {
    row = row.slice(1);
  }
  if (row.endsWith('|')) {
    row = row.slice(0, -1);
  }
  return row.split('|').map((cell) => cell.trim());
}

// padRow makes a body row exactly width cells, since a ragged row may
// have fewer (padded with empty cells) or more (extra ones dropped) than
// the header.
function padRow(cells, width) {
  const row = cells.slice(0, width);
  while (row.length < width) {
    row.push('');
  }
  return row;
}

// cellAlign reads one separator cell's colons into the alignment GitHub
// tables give that column: "left", "right", "center", or "" for none.
function cellAlign(cell) {
  const left = cell.startsWith(':');
  const right = cell.endsWith(':');
  if (left && right) {
    return 'center';
  }
  if (right) {
    return 'right';
  }
  return left ? 'left' : '';
}

// tableHtml lays out a parsed table as an HTML table that fits the width
// of whatever holds it, with a header row in bold and a border around
// each cell. escapeFn and linkifyFn are the caller's own text handling
// (escaping, then linkifying), so a cell's content goes through exactly
// the same rules as the rest of the message. borderColor comes from the
// caller's theme: this file never hard-codes a colour.
function tableHtml(table, escapeFn, linkifyFn, borderColor) {
  const cellStyle = `border:1px solid ${borderColor};padding:4px`;
  const headCells = table.header
    .map((cell, i) => cellHtml('th', cell, table.aligns[i], cellStyle, escapeFn, linkifyFn))
    .join('');
  const bodyRows = table.rows
    .map((row) => `<tr>${row.map((cell, i) => cellHtml('td', cell, table.aligns[i], cellStyle, escapeFn, linkifyFn)).join('')}</tr>`)
    .join('');

  // Qt's rich text only paints a cell's CSS border when the <table> tag
  // itself asks for one; border="1" turns that on, and cellspacing="0"
  // with border-collapse keeps cells from each drawing their own border
  // with a gap between them.
  return `<table border="1" cellspacing="0" cellpadding="0" width="100%" style="border-collapse:collapse">` +
    `<tr>${headCells}</tr>${bodyRows}</table>`;
}

// cellHtml renders one header or body cell, aligned as its column's
// separator colons asked, with its text escaped and linkified by the
// caller's own functions.
function cellHtml(tag, text, align, baseStyle, escapeFn, linkifyFn) {
  const style = align ? `${baseStyle};text-align:${align}` : baseStyle;
  return `<${tag} style="${style}">${linkifyFn(escapeFn(text))}</${tag}>`;
}
