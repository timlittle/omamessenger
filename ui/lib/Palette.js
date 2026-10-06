.pragma library

// Fuzzy matching for the command palette, in the style of Telescope or
// Alfred: the letters typed must appear in order, and matches at word
// starts or in unbroken runs rank first.

// search returns the items whose text matches query, best first. textOf
// picks the text to match from an item. An empty query keeps every item
// in its original order.
function search(items, query, textOf) {
  const wanted = (query ?? '').trim().toLowerCase();
  if (!wanted) {
    return items.slice();
  }

  return items
    .map((item, index) => ({ item, index, score: score(textOf(item).toLowerCase(), wanted) }))
    .filter((entry) => entry.score > 0)
    .sort((a, b) => b.score - a.score || a.index - b.index)
    .map((entry) => entry.item);
}

// score rates how well text matches wanted, or returns 0 when the letters
// do not all appear in order. Each matched letter scores 1, plus 3 at the
// start of a word and 2 when it follows the previous match directly. Text
// containing the whole query scores extra, most of all at a word start.
function score(text, wanted) {
  const whole = text.indexOf(wanted);
  let total = whole < 0 ? 0 : 10 + (whole === 0 || text[whole - 1] === ' ' ? 10 : 0);
  let from = 0;
  let previous = -2;

  for (const ch of wanted) {
    const at = text.indexOf(ch, from);
    if (at < 0) {
      return 0;
    }

    total += 1 + (at === 0 || text[at - 1] === ' ' ? 3 : 0) + (at === previous + 1 ? 2 : 0);
    previous = at;
    from = at + 1;
  }

  return total;
}
