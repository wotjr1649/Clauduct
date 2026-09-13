const fail = () => { throw new Error('DEVELOPMENT_SOURCE_REJECTED'); };
const variables = new Set(['value', 'seconds', 'milliseconds', 'result', 'ms', 'digits', 'trimmed']);
const identifiers = new Set([...variables, 'export', 'function', 'parseRetryAfterSeconds', 'const', 'if', 'else',
  'return', 'typeof', 'null', 'Number', 'Math', 'isFinite', 'isSafeInteger', 'min', 'length', 'test', 'trim']);
const numbers = new Set(['0', '1', '128', '1000', '5000']);
const regexes = ['/^[ \\t]*[0-9]+[ \\t]*$/', '/[\\r\\n]/'];

// A deliberately narrow language for this public parser fixture, not a general
// JavaScript sandbox. No arbitrary strings, imports, loops, dynamic evaluation,
// property writes or ambient names may reach native execution or its transcript.
// Passing this lexical boundary still requires the outer developer's review
// and the fixed oracle before proposed code executes.
export function checkDevelopmentSource(source) {
  if (typeof source !== 'string' || Buffer.byteLength(source) > 8192 || /[^\x09\x0a\x0d\x20-\x7e]/.test(source)) fail();
  const tokens = [];
  let offset = 0;
  while (offset < source.length) {
    const rest = source.slice(offset), whitespace = /^[ \t\r\n]+/.exec(rest);
    if (whitespace) { offset += whitespace[0].length; continue; }
    const regex = regexes.find(value => rest.startsWith(value));
    if (regex) { tokens.push('REGEX'); offset += regex.length; continue; }
    const match = /^(?:===|!==|>=|<=|\|\||&&|[A-Za-z_][A-Za-z_0-9]*|[0-9]+|'string'|"string"|[!><*?():;{}.,=+-])/.exec(rest);
    if (!match) fail();
    const token = match[0];
    if (/^[A-Za-z_]/.test(token) && !identifiers.has(token) || /^[0-9]/.test(token) && !numbers.has(token)) fail();
    tokens.push(token); offset += token.length;
    if (tokens.length > 2048) fail();
  }
  if (tokens.slice(0, 7).join(' ') !== 'export function parseRetryAfterSeconds ( value ) {' || tokens.at(-1) !== '}') fail();
  let depth = 0;
  for (let index = 0; index < tokens.length; index++) {
    const token = tokens[index], previous = tokens[index - 1];
    if (token === '{') depth++;
    if (token === '}' && (--depth < 0 || depth === 0 && index !== tokens.length - 1)) fail();
    if (index > 1 && ['export', 'function'].includes(token)) fail();
    if (token === 'const' && !variables.has(tokens[index + 1])) fail();
    if (['+', '-'].includes(token) && previous === token) fail();
    if (token === '=' && (tokens[index - 2] !== 'const' || !variables.has(previous))) fail();
    if (token === '.' && !['length', 'isFinite', 'isSafeInteger', 'min', 'test', 'trim'].includes(tokens[index + 1])) fail();
    if (token === '(' && /^[A-Za-z_]/.test(previous ?? '')
      && !['parseRetryAfterSeconds', 'if', 'Number', 'isFinite', 'isSafeInteger', 'min', 'test', 'trim', 'return', 'typeof'].includes(previous)) fail();
    if (token === '(' && previous === 'parseRetryAfterSeconds' && index !== 3) fail();
    if (['isFinite', 'isSafeInteger'].includes(token) && (previous !== '.' || tokens[index - 2] !== 'Number')) fail();
    if (token === 'min' && (previous !== '.' || tokens[index - 2] !== 'Math')) fail();
    if (token === 'test' && (previous !== '.' || tokens[index - 2] !== 'REGEX')) fail();
    if (token === 'trim' && (previous !== '.' || !variables.has(tokens[index - 2]))) fail();
  }
  if (depth !== 0) fail();
  return true;
}
