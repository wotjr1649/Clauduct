// Read-only survey of the installed Claude binary: reports beta feature names this project
// has not classified yet. Never executes the binary and never reads credential files.
// This is a report, not a gate: it does not change routing and no test depends on its output.
import { createReadStream } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { CLAUDE_EXE } from '../poc/claude-inspection.mjs';
import { NATIVE_BETAS, UNSUPPORTED_BETAS, SERVER_DEPENDENT_BETAS } from './native-beta.mjs';

const NAME = /[a-z][a-z0-9]*(?:-[a-z0-9]+)*-20(?:2[4-9]|3[0-9])-[01][0-9]-[0-3][0-9]/g;
// Strings inside the binary that match the shape but are not feature names: a test fixture
// and a phrase from an MCP handshake error ("no pre-2026-07-28 protocol version").
const IGNORED = new Set(['foo-2025-01-01', 'pre-2026-07-28']);

export async function scanBetaNames(path = CLAUDE_EXE, { chunkBytes = 1 << 20 } = {}) {
  const found = new Set();
  let tail = '';
  for await (const chunk of createReadStream(path, { highWaterMark: chunkBytes })) {
    // latin1 keeps one byte per character, so an offset never splits a character.
    const text = tail + chunk.toString('latin1');
    for (const match of text.matchAll(NAME)) found.add(match[0]);
    tail = text.slice(-96);
  }
  return [...found].sort();
}

export function classifyBetaNames(names) {
  const known = new Set(NATIVE_BETAS), judged = new Set(Object.keys(UNSUPPORTED_BETAS));
  const serverDependent = new Set(SERVER_DEPENDENT_BETAS);
  const report = { observed: names.length, allowed: [], refused: [], serverDependent: [], unclassified: [] };
  for (const name of names) {
    if (IGNORED.has(name)) continue;
    if (known.has(name)) report.allowed.push(name);
    else if (judged.has(name)) report.refused.push(name);
    else if (serverDependent.has(name)) report.serverDependent.push(name);
    else report.unclassified.push(name);
  }
  return report;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const target = process.argv[2] ?? CLAUDE_EXE;
  try {
    const report = classifyBetaNames(await scanBetaNames(target));
    process.stdout.write(JSON.stringify(report, null, 1) + '\n');
    if (report.unclassified.length) {
      process.stderr.write(`CLAUDUCT_UNCLASSIFIED_BETAS ${report.unclassified.length}: classify each in src/native-beta.mjs.\n`);
    }
  } catch {
    process.stderr.write('SCAN_UNAVAILABLE: the installed Claude binary could not be read.\n');
    process.exitCode = 1;
  }
}
