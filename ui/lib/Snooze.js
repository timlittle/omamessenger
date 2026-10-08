.pragma library
.import "Format.js" as Format

// Parses when to snooze a conversation until: the three presets the
// command palette offers outright, and a custom time typed as a
// duration ("2h", "30m"), a clock time ("18:00"), or a weekday and
// clock time ("mon 9:00"). Every function takes "now" in milliseconds,
// so the parsing is exactly reproducible in a test.

// LATER_TODAY_MS is how far ahead "later today" snoozes: three hours.
var LATER_TODAY_MS = 3 * 60 * 60 * 1000;

// DEFAULT_HOUR is the hour "tomorrow" and "next week" snooze until.
var DEFAULT_HOUR = 9;

// WEEKDAYS maps a weekday's short name to Date's own Sunday-is-0
// numbering, for the "mon 9:00" form.
var WEEKDAYS = { sun: 0, mon: 1, tue: 2, wed: 3, thu: 4, fri: 5, sat: 6 };

// laterToday returns three hours from now, in milliseconds.
function laterToday(nowMs) {
  return nowMs + LATER_TODAY_MS;
}

// tomorrow returns nine in the morning the next calendar day.
function tomorrow(nowMs) {
  return atHour(addDays(nowMs, 1), DEFAULT_HOUR, 0);
}

// nextWeek returns nine in the morning next Monday: always at least two
// days away, so choosing it on a Sunday or Monday still means the
// following week's Monday, never today's or tomorrow's.
function nextWeek(nowMs) {
  const date = new Date(nowMs);
  let daysUntilMonday = (1 - date.getDay() + 7) % 7;
  if (daysUntilMonday < 2) {
    daysUntilMonday += 7;
  }

  return atHour(addDays(nowMs, daysUntilMonday), DEFAULT_HOUR, 0);
}

// parse reads a custom snooze time: a duration ("2h", "30m", "1h30m"), a
// clock time ("18:00", today if still ahead or else tomorrow), or a
// weekday and clock time ("mon 9:00", the next one to arrive). Returns
// milliseconds, or null when text matches none of them.
function parse(text, nowMs) {
  const trimmed = (text ?? '').trim().toLowerCase();
  if (trimmed === '') {
    return null;
  }

  return parseDuration(trimmed, nowMs) ?? parseWeekdayClock(trimmed, nowMs) ?? parseClock(trimmed, nowMs);
}

// preview returns parse's result as the one row the command palette
// shows while typing a custom time: [] while text does not parse yet,
// otherwise one row naming when it would wake the chat.
function preview(text, nowMs) {
  const at = parse(text, nowMs);
  if (at === null) {
    return [];
  }

  return [{ at, label: `Snooze until ${Format.snoozeUntilLabel(at, nowMs)}` }];
}

// parseDuration reads "2h", "30m" or "1h30m" as that long from now.
function parseDuration(text, nowMs) {
  const match = text.match(/^(?:(\d+)h)?(?:(\d+)m)?$/);
  if (!match || (!match[1] && !match[2])) {
    return null;
  }

  const hours = Number(match[1] ?? 0);
  const minutes = Number(match[2] ?? 0);

  return nowMs + (hours * 60 + minutes) * 60 * 1000;
}

// parseClock reads "18:00" or "9:00" as that time today if it has not
// passed yet, otherwise tomorrow.
function parseClock(text, nowMs) {
  const match = text.match(/^(\d{1,2}):(\d{2})$/);
  if (!match) {
    return null;
  }

  const hour = Number(match[1]);
  const minute = Number(match[2]);
  if (hour > 23 || minute > 59) {
    return null;
  }

  const today = atHour(nowMs, hour, minute);

  return today > nowMs ? today : atHour(addDays(nowMs, 1), hour, minute);
}

// parseWeekdayClock reads "mon 9:00" as that weekday's clock time, the
// next one to arrive (today counts only if its time has not passed).
function parseWeekdayClock(text, nowMs) {
  const match = text.match(/^(sun|mon|tue|wed|thu|fri|sat)\s+(\d{1,2}):(\d{2})$/);
  if (!match) {
    return null;
  }

  const hour = Number(match[2]);
  const minute = Number(match[3]);
  if (hour > 23 || minute > 59) {
    return null;
  }

  const target = WEEKDAYS[match[1]];
  const daysAhead = (target - new Date(nowMs).getDay() + 7) % 7;
  const candidate = atHour(addDays(nowMs, daysAhead), hour, minute);

  return candidate > nowMs ? candidate : atHour(addDays(nowMs, daysAhead + 7), hour, minute);
}

// addDays shifts a time by whole calendar days, in local time, so it
// still lands at midnight across a daylight-saving change.
function addDays(ms, days) {
  const d = new Date(ms);
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + days).getTime();
}

// atHour returns a time's own calendar day at the given local hour and
// minute.
function atHour(ms, hour, minute) {
  const d = new Date(ms);
  return new Date(d.getFullYear(), d.getMonth(), d.getDate(), hour, minute, 0, 0).getTime();
}
