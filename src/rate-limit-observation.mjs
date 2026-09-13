// Header names and units: OpenAI codex/codex-rs/codex-api/src/rate_limits.rs,
// reviewed 2026-09-13. This observes numeric windows without family names; it does not
// authorize spending, infer remaining tokens, or change any request budget.
const windows = ['primary', 'secondary'];
const fields = ['usedPercent', 'windowMinutes', 'resetAtSeconds'];
const suffixes = ['used-percent', 'window-minutes', 'reset-at'];
const maximumOtherLimits = 8;
const states = ['missing', 'partial', 'observed', 'invalid'];
const reasons = ['header-shape', 'duplicate', 'numeric-format', 'numeric-range'];
const fieldLabels = windows.flatMap(window => fields.map(field => `${window}.${field}`));
const integer = value => Number.isSafeInteger(value) && value >= 0;
const validNumber = (value, index) => index === 0 ? Number.isFinite(value) && value >= 0 && value <= 100
  : index === 1 ? integer(value) && value <= Math.floor(Number.MAX_SAFE_INTEGER / 60000)
    : Number.isSafeInteger(value) && Math.abs(value) <= Math.floor(Number.MAX_SAFE_INTEGER / 1000);
const freeze = value => {
  for (const key of windows) if (value[key]) Object.freeze(value[key]);
  if (value.otherLimits) { value.otherLimits.forEach(freeze); Object.freeze(value.otherLimits); }
  return Object.freeze(value);
};
const invalid = (otherLimitFamilies = 0, invalidReason = 'header-shape', invalidField = null) => freeze({
  state: 'invalid', primary: null, secondary: null, otherLimitFamilies, invalidReason, invalidField });

function observeWindows(rawHeaders, prefix) {
  const names = windows.flatMap(window => suffixes.map(suffix => `${prefix}-${window}-${suffix}`));
  const values = new Map(), seen = new Set();
  let malformed = null;
  for (let index = 0; index < rawHeaders.length; index += 2) {
    const name = rawHeaders[index].toLowerCase();
    if (!names.includes(name)) continue;
    const raw = rawHeaders[index + 1], nameIndex = names.indexOf(name), fieldIndex = nameIndex % 3;
    if (seen.has(name)) { values.delete(name); malformed ??= ['duplicate', fieldLabels[nameIndex]]; continue; }
    seen.add(name);
    if (typeof raw !== 'string' || raw.length > 64 || /[\r\n]/.test(raw)
      || !(fieldIndex === 0 ? /^[ \t]*[0-9]+(?:\.[0-9]+)?[ \t]*$/ : fieldIndex === 1
        ? /^[ \t]*[0-9]+[ \t]*$/ : /^[ \t]*-?[0-9]+[ \t]*$/).test(raw)) {
      malformed ??= ['numeric-format', fieldLabels[nameIndex]]; continue;
    }
    const number = Number(raw);
    if (!validNumber(number, fieldIndex)) { malformed ??= ['numeric-range', fieldLabels[nameIndex]]; continue; }
    values.set(name, number);
  }
  // An invalid optional reset must not erase independent observed percentages.
  // It stays null, and the overall invalid state remains explicit. Signed reset
  // seconds match the upstream i64 schema; negative is not a future reset.
  const result = { state: malformed ? 'invalid' : values.size === 0 ? 'missing' : values.size === 6 ? 'observed' : 'partial',
    primary: null, secondary: null,
    ...(malformed ? { invalidReason: malformed[0], invalidField: malformed[1] } : {}) };
  for (const window of windows) {
    const selected = suffixes.map(suffix => values.get(`${prefix}-${window}-${suffix}`) ?? null);
    if (selected.some(value => value !== null)) result[window] = Object.fromEntries(fields.map((field, index) => [field, selected[index]]));
  }
  return freeze(result);
}

export function observeRateLimitHeaders(rawHeaders) {
  if (!Array.isArray(rawHeaders) || rawHeaders.length > 1024 || rawHeaders.length % 2 !== 0) return invalid();
  const other = new Set(), familySuffix = '-primary-used-percent';
  for (let index = 0; index < rawHeaders.length; index += 2) {
    const rawName = rawHeaders[index];
    if (typeof rawName !== 'string' || rawName.length > 256) return invalid();
    const name = rawName.toLowerCase();
    if (name.startsWith('x-') && name.endsWith(familySuffix) && name !== `x-codex${familySuffix}`) {
      other.add(name.slice(0, -familySuffix.length));
    }
  }
  // Identifiers are used only for selecting the same six numeric fields. No
  // family name, plan, balance, promotional text or arbitrary value is retained.
  const result = { ...observeWindows(rawHeaders, 'x-codex'), otherLimitFamilies: other.size };
  if (other.size > 0) {
    result.otherLimits = [...other].sort().slice(0, maximumOtherLimits).map(prefix => observeWindows(rawHeaders, prefix));
    result.otherLimitsTruncated = other.size > maximumOtherLimits;
  }
  return freeze(result);
}

// Result files are another trust boundary. Reconstruct only the fixed numeric
// shape; a malformed observation cannot become an apparently complete quota.
export function rateLimitEvidence(rows) {
  const fail = () => { throw new Error('RATE_LIMIT_EVIDENCE_INVALID'); };
  const shape = (value, keys) => value && typeof value === 'object' && !Array.isArray(value)
    && Object.keys(value).length === keys.length && keys.every(key => Object.hasOwn(value, key));
  const cleanWindows = (value, metadata = []) => {
    const hasDiagnostic = value?.state === 'invalid' && Object.hasOwn(value, 'invalidReason');
    if (!shape(value, ['state', ...windows, ...metadata, ...(hasDiagnostic ? ['invalidReason', 'invalidField'] : [])]) || !states.includes(value.state)) fail();
    const clean = { state: value.state, primary: null, secondary: null };
    if (value.state === 'invalid') {
      if (hasDiagnostic && (!reasons.includes(value.invalidReason)
        || (value.invalidReason === 'header-shape' ? value.invalidField !== null : !fieldLabels.includes(value.invalidField)))) fail();
      // Earlier invalid observations lacked fixed diagnostics; keep that gap.
      clean.invalidReason = hasDiagnostic ? value.invalidReason : null;
      clean.invalidField = hasDiagnostic ? value.invalidField : null;
    }
    let present = 0;
    for (const window of windows) {
      if (value[window] === null) continue;
      if (!shape(value[window], fields)) fail();
      clean[window] = {};
      for (const [index, field] of fields.entries()) {
        const number = value[window][field];
        if (number !== null && !validNumber(number, index)) fail();
        if (number !== null) present++;
        clean[window][field] = number;
      }
      if (fields.every(field => clean[window][field] === null)) fail();
    }
    if ((value.state === 'observed' && present !== 6) || (value.state === 'partial' && (present === 0 || present === 6))
      || (value.state === 'missing' && present !== 0)) fail();
    if (value.state === 'invalid') {
      if (!hasDiagnostic || value.invalidReason === 'header-shape') { if (present !== 0) fail(); }
      else {
        const [window, field] = value.invalidField.split('.');
        if (present === 6 || clean[window]?.[field] != null) fail();
      }
    }
    return clean;
  };
  if (!Array.isArray(rows) || rows.length > 4096) fail();
  const counts = Object.fromEntries(states.map(state => [state, 0]));
  const seen = new Set();
  let latest = null;
  for (const row of rows) {
    if (!shape(row, ['at', 'event', 'requestAttempt', 'kind', 'httpStatus', 'observation']) || row.event !== 'RESPONSE_LIMITS'
      || !integer(row.at) || !Number.isSafeInteger(row.requestAttempt) || row.requestAttempt < 1 || row.requestAttempt > 4096
      || seen.has(row.requestAttempt) || !['responses', 'search'].includes(row.kind)
      || !Number.isInteger(row.httpStatus) || row.httpStatus < 100 || row.httpStatus > 599) fail();
    seen.add(row.requestAttempt);
    const value = row.observation, hasOther = value && Object.hasOwn(value, 'otherLimits');
    const clean = cleanWindows(value, ['otherLimitFamilies', ...(hasOther ? ['otherLimits', 'otherLimitsTruncated'] : [])]);
    if (!integer(value.otherLimitFamilies) || value.otherLimitFamilies > 512) fail();
    clean.otherLimitFamilies = value.otherLimitFamilies;
    if (hasOther) {
      if (value.otherLimitFamilies === 0 || !Array.isArray(value.otherLimits)
        || value.otherLimits.length !== Math.min(value.otherLimitFamilies, maximumOtherLimits)
        || value.otherLimitsTruncated !== (value.otherLimitFamilies > maximumOtherLimits)) fail();
      clean.otherLimits = value.otherLimits.map(limit => cleanWindows(limit));
      clean.otherLimitsTruncated = value.otherLimitsTruncated;
    }
    counts[value.state]++;
    latest = { at: row.at, requestAttempt: row.requestAttempt, kind: row.kind, httpStatus: row.httpStatus, observation: clean };
  }
  return { responseCount: rows.length, counts, latest, authorizesAdditionalRequests: false };
}
