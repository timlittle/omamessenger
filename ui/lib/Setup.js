.pragma library

// Account setup: checks what the user typed for a new Telegram account,
// and describes the field each sign-in step asks for.

// FIELDS describes the typed answer for each sign-in step.
var FIELDS = {
  phone: { placeholder: 'Phone number, with country code', password: false },
  code: { placeholder: 'Login code', password: false },
  password: { placeholder: 'Two-step verification password', password: true }
};

// apiID reads the API id from my.telegram.org, or 0 when it is not a
// positive whole number.
function apiID(text) {
  const trimmed = String(text).trim();

  return /^[1-9][0-9]*$/.test(trimmed) ? Number(trimmed) : 0;
}

// credentialsReady reports whether an API id and hash have been entered.
function credentialsReady(id, hash) {
  return apiID(id) > 0 && String(hash).trim() !== '';
}

// field returns {step, placeholder, password} for a stage the user
// answers by typing, or null.
function field(stage) {
  const f = FIELDS[stage];

  return f ? { step: stage, placeholder: f.placeholder, password: f.password } : null;
}

// qrSource turns the helper's base64 PNG into an image URL.
function qrSource(png) {
  return png ? `data:image/png;base64,${png}` : '';
}
