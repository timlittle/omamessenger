.pragma library

var dayNames = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
var monthsShort = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
var monthsFull = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'];

function escapeHtml(text) {
  var result = text;
  result = result.split('&').join('&amp;');
  result = result.split('<').join('&lt;');
  result = result.split('>').join('&gt;');
  result = result.split('"').join('&quot;');
  return result;
}

function linkify(escaped) {
  // Match http(s) URLs, but not inside entities like &amp;
  var urlPattern = /(https?:\/\/[^\s<"&]*(?:&(?:amp|lt|quot|#\d+|#x[0-9a-fA-F]+);[^\s<"&]*)*)/g;
  return escaped.replace(urlPattern, function(url) {
    // Strip trailing punctuation that shouldn't be part of the URL
    var punctuation = '';
    while (url.length > 0) {
      var lastChar = url.charAt(url.length - 1);
      if (lastChar === '.' || lastChar === ',' || lastChar === ';' || lastChar === ':' || lastChar === '!' || lastChar === '?' || lastChar === ')') {
        punctuation = lastChar + punctuation;
        url = url.substring(0, url.length - 1);
      } else {
        break;
      }
    }
    return '<a href="' + url + '">' + url + '</a>' + punctuation;
  });
}

function highlight(escaped, query) {
  if (!query || query.length === 0) {
    return escaped;
  }
  // Split by entities and process text parts
  var parts = escaped.split(/(&[^;]*;)/);
  var result = '';
  for (var i = 0; i < parts.length; i++) {
    var part = parts[i];
    // If it looks like an entity, don't highlight inside it
    if (part.charAt(0) === '&' && part.charAt(part.length - 1) === ';') {
      result += part;
    } else {
      // Wrap case-insensitive matches in <b>
      var lowerPart = part.toLowerCase();
      var lowerQuery = query.toLowerCase();
      var highlighted = part;
      var idx = 0;
      var output = '';
      var searchIdx = lowerPart.indexOf(lowerQuery);
      while (searchIdx !== -1) {
        output += highlighted.substring(idx, searchIdx);
        output += '<b>' + highlighted.substring(searchIdx, searchIdx + query.length) + '</b>';
        idx = searchIdx + query.length;
        searchIdx = lowerPart.indexOf(lowerQuery, idx);
      }
      output += highlighted.substring(idx);
      result += output;
    }
  }
  return result;
}

function initials(name) {
  if (!name || name.length === 0) {
    return '?';
  }
  // Use Array.from to handle emoji and multi-byte characters
  var chars = Array.from(name);
  var result = '';
  var state = 'skip_leading';

  for (var i = 0; i < chars.length && result.length < 2; i++) {
    var ch = chars[i];
    var isSpace = /\s/.test(ch);

    if (state === 'skip_leading' && !isSpace) {
      // First non-space character
      result += ch;
      state = 'in_word';
    } else if (state === 'in_word' && isSpace) {
      // End of first word
      state = 'skip_whitespace';
    } else if (state === 'skip_whitespace' && !isSpace) {
      // First character of second word
      result += ch;
      state = 'done';
    }
  }

  if (result.length === 0) {
    return '?';
  }
  return result.toUpperCase();
}

function daysAgo(ms, nowMs) {
  var date = new Date(ms);
  var now = new Date(nowMs);
  var todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  var dateStart = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  var diff = todayStart - dateStart;
  return Math.round(diff / (24 * 60 * 60 * 1000));
}

function timeLabel(ms, nowMs) {
  var date = new Date(ms);
  var now = new Date(nowMs);
  var days = daysAgo(ms, nowMs);

  // Today
  if (days === 0) {
    return pad(date.getHours()) + ':' + pad(date.getMinutes());
  }

  // Yesterday
  if (days === 1) {
    return 'Yesterday';
  }

  // Within 6 days
  if (days >= 2 && days <= 6) {
    return dayNames[date.getDay()];
  }

  // Same year
  if (date.getFullYear() === now.getFullYear()) {
    return date.getDate() + ' ' + monthsShort[date.getMonth()];
  }

  // Different year
  return date.getDate() + ' ' + monthsShort[date.getMonth()] + ' ' + date.getFullYear();
}

function dayLabel(ms, nowMs) {
  var date = new Date(ms);
  var now = new Date(nowMs);
  var days = daysAgo(ms, nowMs);

  // Today
  if (days === 0) {
    return 'Today';
  }

  // Yesterday
  if (days === 1) {
    return 'Yesterday';
  }

  // Within 6 days (weekday)
  if (days >= 2 && days <= 6) {
    return dayNames[date.getDay()];
  }

  // Same year
  if (date.getFullYear() === now.getFullYear()) {
    return date.getDate() + ' ' + monthsFull[date.getMonth()];
  }

  // Different year
  return date.getDate() + ' ' + monthsFull[date.getMonth()] + ' ' + date.getFullYear();
}

function pad(n) {
  return n < 10 ? '0' + n : '' + n;
}

function statusGlyph(status) {
  if (status === 'pending') {
    return '○';
  }
  if (status === 'sent') {
    return '✓';
  }
  if (status === 'delivered') {
    return '✓✓';
  }
  if (status === 'read') {
    return '✓✓';
  }
  if (status === 'failed') {
    return '!';
  }
  return '';
}

function previewLine(conv) {
  if (!conv.preview) {
    return '';
  }

  if (conv.kind === 'group') {
    if (conv.previewOutgoing) {
      return 'You: ' + conv.preview;
    } else if (conv.previewSender) {
      return conv.previewSender + ': ' + conv.preview;
    } else {
      return conv.preview;
    }
  }

  // Direct message
  return conv.preview;
}
