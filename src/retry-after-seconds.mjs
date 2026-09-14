export function parseRetryAfterSeconds(value) {
  if (typeof value !== 'string' || value.length > 128 || !/^[ \t]*[0-9]+[ \t]*$/.test(value)) {
    return null;
  }

  const milliseconds = Number(value) * 1000;
  return Number.isSafeInteger(milliseconds) ? milliseconds : null;
}
