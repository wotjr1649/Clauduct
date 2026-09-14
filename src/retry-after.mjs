import { parseRetryAfterSeconds } from './retry-after-seconds.mjs';

const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
const weekday = '(?:Mon|Tue|Wed|Thu|Fri|Sat|Sun)';
const month = '(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)';
const time = '(\\d{2}):(\\d{2}):(\\d{2})';
const imf = new RegExp(`^${weekday}, (\\d{2}) ${month} (\\d{4}) ${time} GMT$`);
const obsolete = new RegExp(`^(?:Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Sunday), (\\d{2})-${month}-(\\d{2}) ${time} GMT$`);
const asctime = new RegExp(`^${weekday} ${month} ( \\d|\\d{2}) ${time} (\\d{4})$`);

// RFC 9110 §§5.6.7,10.2.3. Parsing never shortens a server delay or starts a timer.
export function parseRetryAfter(value, nowMs = Date.now()) {
  // Match the transport's bounded HTTP header surface. A valid integer outside
  // our deadline range is different from invalid syntax: it forbids a retry.
  if (typeof value !== 'string' || value.length > 16384 || !Number.isSafeInteger(nowMs) || nowMs < 0) return null;
  value = value.replace(/^[ \t]+|[ \t]+$/g, '');
  if (value.length > 0 && !/[^0-9]/.test(value)) {
    const delay = parseRetryAfterSeconds(value.replace(/^0+(?=[0-9])/, ''));
    return delay !== null && Number.isSafeInteger(nowMs + delay)
      ? { delayMs: delay, retryAtMs: nowMs + delay } : { unrepresentable: true };
  }
  if (value.length > 128) return null;
  let fields = imf.exec(value), shortYear = false;
  if (!fields) { fields = obsolete.exec(value); shortYear = fields !== null; }
  if (!fields) {
    const old = asctime.exec(value);
    if (old) fields = [old[0], old[2].trim(), old[1], old[6], old[3], old[4], old[5]];
  }
  if (!fields) return null;
  const day = Number(fields[1]), monthIndex = months.indexOf(fields[2]);
  let year = Number(fields[3]);
  const hour = Number(fields[4]), minute = Number(fields[5]), second = Number(fields[6]);
  if (shortYear) year += Math.floor(new Date(nowMs).getUTCFullYear() / 100) * 100;
  if (day < 1 || day > 31 || hour > 23 || minute > 59 || second > 60) return null;
  const date = new Date(0);
  date.setUTCFullYear(year, monthIndex, day); date.setUTCHours(hour, minute, Math.min(second, 59), 0);
  if (shortYear) {
    const fiftyYears = new Date(nowMs); fiftyYears.setUTCFullYear(fiftyYears.getUTCFullYear() + 50);
    if (date.getTime() > fiftyYears.getTime()) { year -= 100; date.setUTCFullYear(year); }
  }
  if (date.getUTCFullYear() !== year || date.getUTCMonth() !== monthIndex || date.getUTCDate() !== day) return null;
  const target = date.getTime() + (second === 60 ? 1000 : 0);
  if (!Number.isSafeInteger(target)) return null;
  return { delayMs: Math.max(0, target - nowMs), retryAtMs: Math.max(nowMs, target) };
}
