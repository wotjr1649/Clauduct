
const fail = () => { throw new Error('DEVELOPMENT_SOURCE_REJECTED'); };

// A deliberately narrow language for validated development tasks, not a general
// JavaScript sandbox. No arbitrary strings, imports, loops, dynamic evaluation,
// property writes or ambient names may reach native execution or its transcript.
// Passing this lexical boundary still requires the outer developer's review
// and the fixed oracle before proposed code executes.
export function checkDevelopmentSourceAgainstTask(source, task) {
  const variables = new Set(task.variables), numbers = new Set(task.numbers), regexes = task.regexes;
  const identifiers = new Set([...variables, ...task.globals, ...Object.keys(task.members),
    'export', 'function', task.functionName, 'const', 'if', 'else', 'return', 'typeof', 'null']);
  if (typeof source !== 'string' || Buffer.byteLength(source) > 8192 || /[^\x09\x0a\x0d\x20-\x7e]/.test(source)) fail();
  const tokens = [];
  let offset = 0;
  while (offset < source.length) {
    const rest = source.slice(offset), whitespace = /^[ \t\r\n]+/.exec(rest);
    if (whitespace) { offset += whitespace[0].length; continue; }
    const regex = regexes.find(value => rest.startsWith(value));
    if (regex) { tokens.push('REGEX'); offset += regex.length; continue; }
    const match = /^(?:===|!==|>=|<=|\|\||&&|[A-Za-z_][A-Za-z_0-9]*|[0-9]+|'[^'\r\n]*'|"[^"\r\n]*"|[!><*?():;{}.,=+-])/.exec(rest);
    if (!match) fail();
    const token = match[0];
    if (/^[A-Za-z_]/.test(token) && !identifiers.has(token) || /^[0-9]/.test(token) && !numbers.has(token)) fail();
    if (['\'', '"'].includes(token[0]) && !task.strings.includes(token.slice(1, -1))) fail();
    tokens.push(token); offset += token.length;
    if (tokens.length > 2048) fail();
  }
  if (tokens.slice(0, 7).join(' ') !== `export function ${task.functionName} ( value ) {` || tokens.at(-1) !== '}') fail();
  let depth = 0;
  for (let index = 0; index < tokens.length; index++) {
    const token = tokens[index], previous = tokens[index - 1];
    if (token === '{') depth++;
    if (token === '}' && (--depth < 0 || depth === 0 && index !== tokens.length - 1)) fail();
    if (index > 1 && ['export', 'function'].includes(token)) fail();
    if (token === 'const' && !variables.has(tokens[index + 1])) fail();
    if (['+', '-'].includes(token) && previous === token) fail();
    if (token === '=' && (tokens[index - 2] !== 'const' || !variables.has(previous))) fail();
    if (token === '.' && (!Object.hasOwn(task.members, tokens[index + 1]) || !task.members[tokens[index + 1]].includes(previous))) fail();
    if (token === '(' && /^[A-Za-z_]/.test(previous ?? '')
      && ![task.functionName, 'if', 'return', 'typeof', ...task.calls].includes(previous)) fail();
    if (token === '(' && previous === task.functionName && index !== 3) fail();
    if (token === '(' && [')', '}', 'REGEX'].includes(previous)) fail();
    if (Object.hasOwn(task.members, token) && token !== 'length'
      && (previous !== '.' || !task.members[token].includes(tokens[index - 2]))) fail();
  }
  if (depth !== 0) fail();
  return true;
}
