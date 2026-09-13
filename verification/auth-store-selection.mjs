// Extract only the root credential-store setting. Other values are skipped,
// never evaluated or returned. This is a bounded TOML structural reader, not a
// general config validator or a source of model/tool/permission configuration.
export function readAuthStoreSetting(config) {
  const invalid = () => { throw new Error('CONFIG_UNSUPPORTED'); };
  if (typeof config !== 'string' || config.length > 65536) invalid();
  let offset = 0, rootTable = true, found = false, store;
  const ows = () => { while (config[offset] === ' ' || config[offset] === '\t') offset++; };
  const comment = () => { while (offset < config.length && config[offset] !== '\n') offset++; };
  function quoted(key = false, decode = false) {
    const quote = config[offset++];
    const multiline = config.slice(offset, offset + 2) === quote.repeat(2);
    if (multiline) { if (key) invalid(); offset += 2; }
    let value = '';
    if (multiline && config[offset] === '\r' && config[offset + 1] === '\n') offset += 2;
    else if (multiline && config[offset] === '\n') offset++;
    while (offset < config.length) {
      const char = config[offset++];
      if (char === quote) {
        if (!multiline) return value;
        let count = 1;
        while (config[offset] === quote) { count++; offset++; }
        if (count > 5) invalid();
        if (count >= 3) { if (decode) value += quote.repeat(count - 3); return value; }
        if (decode) value += quote.repeat(count);
      } else if (char === '\\' && quote === '"') {
        if (offset >= config.length) invalid();
        const escaped = config[offset++];
        if (multiline && /[ \t\r\n]/.test(escaped)) {
          let newline = /[\r\n]/.test(escaped);
          while (offset < config.length && /[ \t\r\n]/.test(config[offset])) {
            const whitespace = config[offset++];
            newline = newline || /[\r\n]/.test(whitespace);
          }
          if (!newline) invalid();
        } else if (escaped === 'u' || escaped === 'U') {
          const width = escaped === 'u' ? 4 : 8, hex = config.slice(offset, offset + width);
          if (hex.length !== width || !/^[0-9a-fA-F]+$/.test(hex)) invalid();
          offset += width;
          const point = Number.parseInt(hex, 16);
          if (point > 0x10ffff || point >= 0xd800 && point <= 0xdfff) invalid();
          if (decode) value += String.fromCodePoint(point);
        } else {
          const escapes = { b: '\b', t: '\t', n: '\n', f: '\f', r: '\r', '"': '"', '\\': '\\' };
          if (!Object.hasOwn(escapes, escaped)) invalid();
          if (decode) value += escapes[escaped];
        }
      } else {
        if (!multiline && (char === '\r' || char === '\n') || /[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(char)) invalid();
        if (decode) value += char;
      }
      if (decode && value.length > 1024) invalid();
    }
    invalid();
  }
  function keys() {
    const parts = [];
    for (;;) {
      ows();
      if (config[offset] === '"' || config[offset] === "'") parts.push(quoted(true, true));
      else {
        const start = offset;
        while (offset < config.length && /[A-Za-z0-9_-]/.test(config[offset])) offset++;
        if (offset === start) invalid();
        parts.push(config.slice(start, offset));
      }
      if (parts.length > 64) invalid();
      ows(); if (config[offset] !== '.') return parts;
      offset++;
    }
  }
  function skipValue() {
    const stack = [];
    while (offset < config.length) {
      const char = config[offset];
      if (char === '"' || char === "'") quoted();
      else if (char === '#') { comment(); if (!stack.length) return; }
      else if (char === '\n' && !stack.length) { offset++; return; }
      else if (char === '[' || char === '{') {
        if (stack.length >= 64) invalid(); stack.push(char === '[' ? ']' : '}'); offset++;
      } else if (char === ']' || char === '}') {
        if (stack.pop() !== char) invalid(); offset++;
      } else offset++;
    }
    if (stack.length) invalid();
  }
  function lineEnd() {
    ows(); if (config[offset] === '#') comment();
    if (config[offset] === '\r') offset++;
    if (offset < config.length && config[offset] !== '\n') invalid();
    if (config[offset] === '\n') offset++;
  }
  while (offset < config.length) {
    ows();
    if (config[offset] === '#') { comment(); continue; }
    if (config[offset] === '\r' || config[offset] === '\n') { offset++; continue; }
    if (offset === config.length) break;
    if (config[offset] === '[') {
      offset++; const array = config[offset] === '['; if (array) offset++;
      const path = keys();
      if (path[0] === 'cli_auth_credentials_store') invalid();
      if (config[offset++] !== ']' || array && config[offset++] !== ']') invalid();
      lineEnd(); rootTable = false;
    } else {
      const path = keys();
      if (config[offset++] !== '=') invalid();
      ows();
      if (rootTable && path[0] === 'cli_auth_credentials_store') {
        if (found || path.length !== 1 || !['"', "'"].includes(config[offset])) invalid();
        found = true; store = quoted(false, true); lineEnd();
      } else skipValue();
    }
  }
  if (found && !['file', 'keyring', 'auto', 'ephemeral'].includes(store)) invalid();
  return store;
}
