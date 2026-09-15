// Offline only: reads tracked markdown and the files it cites. No network, no credential read,
// no write. A failure carries a fixed label and the citation as written; file contents are never
// echoed, because the point is to name the wrong citation, not to quote the line it landed on.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { readFileSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join, relative, resolve as resolvePath, isAbsolute } from 'node:path';

// A citation is a backticked repo-relative path with a line, or a line range. The extension is
// required: without it a branch name like `docs/handoff-citation-exception` reads as a citation.
const CITATION = /`((?:src|verification|poc|bin|docs|\.github)\/[A-Za-z0-9._/-]+\.[A-Za-z0-9]+):(\d+)(?:-(\d+))?`/g;
// A fenced block holds sample output and stack traces. Those name positions as they were when
// captured and are not claims about the tree now, so they are not citations.
const stripFences = text => text.replace(/```[\s\S]*?```/g, '');

// What this can and cannot decide: that a cited file exists and the cited line holds something.
// Whether the line says what the sentence beside it claims is not mechanical and is not checked.
function citationFailures(text, locate) {
  const failures = [];
  let citations = 0;
  for (const [cited, file, from, to] of stripFences(text).matchAll(CITATION)) {
    citations++;
    const fail = reason => failures.push({ citation: cited.replaceAll('`', ''), reason });
    const found = locate(file);
    if (found.reason !== undefined) { fail(found.reason); continue; }
    const start = Number(from), end = to === undefined ? start : Number(to);
    if (end < start) { fail('RANGE_INVERTED'); continue; }
    if (start < 1 || end > found.lines.length) { fail('LINE_OUT_OF_RANGE'); continue; }
    // Both ends of a range are read. A range whose last line is blank is as wrong as one whose
    // first line is, and checking only the start hides it.
    if (!found.lines[start - 1].trim() || !found.lines[end - 1].trim()) { fail('BLANK_LINE'); continue; }
  }
  return { citations, failures };
}

// Synthetic first, so a green run below means the checks ran rather than that nothing matched.
const sample = { lines: ['first', '', 'third'] };
const fixture = file => file === 'src/sample.mjs' ? sample : { reason: 'MISSING_FILE' };
let selfTests = 0;
const expect = (text, reasons, citations) => {
  const result = citationFailures(text, fixture);
  assert.deepEqual(result.failures.map(item => item.reason), reasons, text);
  assert.equal(result.citations, citations, text);
  selfTests++;
};
expect('`src/sample.mjs:1`', [], 1);
expect('`src/sample.mjs:1-3`', [], 1);
expect('`src/sample.mjs:2`', ['BLANK_LINE'], 1);
expect('`src/sample.mjs:1-2`', ['BLANK_LINE'], 1);
expect('`src/sample.mjs:4`', ['LINE_OUT_OF_RANGE'], 1);
expect('`src/sample.mjs:1-4`', ['LINE_OUT_OF_RANGE'], 1);
expect('`src/sample.mjs:3-1`', ['RANGE_INVERTED'], 1);
expect('`src/absent.mjs:1`', ['MISSING_FILE'], 1);
expect('`src/sample.mjs:1` and `src/absent.mjs:2`', ['MISSING_FILE'], 2);
// Not citations: fenced output, a path without an extension, and an unbackticked stack frame.
expect('```\n`src/absent.mjs:1`\n```', [], 0);
expect('`docs/handoff-citation-exception:1`', [], 0);
expect('at src/absent.mjs:1:9', [], 0);

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const listed = spawnSync('git', ['-C', root, 'ls-files', '-z', '*.md'], {
  encoding: 'utf8', timeout: 60000, maxBuffer: 4 * 1024 * 1024, windowsHide: true });
assert.equal(listed.error, undefined); assert.equal(listed.status, 0); assert.equal(listed.stderr, '');
const documents = listed.stdout.split('\0').filter(Boolean);
assert.ok(documents.length > 0, 'NO_TRACKED_MARKDOWN');

const cache = new Map();
function locate(file) {
  if (cache.has(file)) return cache.get(file);
  const target = resolvePath(root, file);
  // A citation that climbs out of the repository is refused rather than read, whatever it names.
  const found = isAbsolute(file) || relative(root, target).startsWith('..') ? { reason: 'OUTSIDE_REPOSITORY' }
    : !safeIsFile(target) ? { reason: 'MISSING_FILE' }
      : { lines: readFileSync(target, 'utf8').split(/\r?\n/) };
  cache.set(file, found);
  return found;
}
function safeIsFile(target) { try { return statSync(target).isFile(); } catch { return false; } }

const failures = [];
let citations = 0;
for (const document of documents) {
  const result = citationFailures(readFileSync(join(root, document), 'utf8'), locate);
  citations += result.citations;
  for (const item of result.failures) failures.push({ document, ...item });
}
for (const { document, citation, reason } of failures) console.error(`${reason} ${document} -> ${citation}`);
console.log(JSON.stringify({ suite: 'doc-citations', selfTests, documents: documents.length, citations,
  filesResolved: [...cache.values()].filter(item => item.lines !== undefined).length,
  failures: failures.length, externalRequests: 0, credentialReads: 0 }));
if (failures.length) process.exitCode = 1;
