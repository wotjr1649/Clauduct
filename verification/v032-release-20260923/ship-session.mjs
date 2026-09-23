// Runs one real-backend session through an installed clauduct.exe: a prompt, /clear, and a
// delegation to a background child, over stream-json. Public workspace, profile and TEMP under
// the run directory; the account's own credentials are read by clauduct as in normal use.
//   node ship-session.mjs <installed clauduct.exe> <run directory>
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import readline from 'node:readline';

const [exe, runDir] = process.argv.slice(2);
if (!exe || !runDir) throw new Error('usage: ship-session.mjs <clauduct.exe> <run directory>');
const dirs = Object.fromEntries(['project', 'profile', 'temp'].map(n => [n, path.join(runDir, n)]));
for (const d of Object.values(dirs)) fs.mkdirSync(d, { recursive: true });
if (/^c:[\\/]users[\\/]/i.test(path.resolve(runDir))) throw new Error('run directory must be outside C:\\Users');

const first = 'SHIP_FIRST_OK', child = 'SHIP_CHILD_OK', second = 'SHIP_SECOND_OK';
const ask = `Use the Agent tool exactly once with subagent_type "general-purpose", model "gpt-5.6-luna", effort "low" and the prompt "Reply with exactly ${child}. Do not use tools.". When its report arrives, reply with exactly ${second}.`;
const env = { ...process.env, TEMP: dirs.temp, TMP: dirs.temp, CLAUDE_CONFIG_DIR: dirs.profile, CLAUDE_CODE_MAX_RETRIES: '0' };
const proc = spawn(exe, ['-p', '--input-format', 'stream-json', '--output-format', 'stream-json', '--verbose',
  '--model', 'gpt-5.6-luna', '--effort', 'low', '--allowedTools', 'Agent', '--strict-mcp-config'],
  { cwd: dirs.project, env, stdio: ['pipe', 'pipe', 'pipe'], windowsHide: true });
let stderrBytes = 0;
proc.stderr.on('data', b => { stderrBytes += b.length; });
const send = text => proc.stdin.write(JSON.stringify({ type: 'user', message: { role: 'user', content: text } }) + '\n');
const timer = setTimeout(() => { console.log('timeout: killing session'); proc.kill(); }, 240_000);

let phase = 0, firstSession = '', clearSession = '';
const seen = { first: false, clearChanged: false, childReported: false, second: false, errors: 0 };
send(`Reply with exactly ${first}. Do not use tools.`);
for await (const line of readline.createInterface({ input: proc.stdout })) {
  let e; try { e = JSON.parse(line); } catch { continue; }
  if (e.subtype === 'task_notification') {
    console.log(`task_notification status=${e.status} child_marker=${String(e.summary ?? '').includes(child)}`);
    seen.childReported ||= e.status === 'completed' && String(e.summary ?? '').includes(child);
  }
  if (e.type !== 'result') continue;
  console.log(`result phase=${phase} error=${e.is_error} session=${String(e.session_id).slice(0, 8)}`);
  if (e.is_error) { seen.errors++; break; }
  if (phase === 0) { seen.first = String(e.result).includes(first); firstSession = e.session_id; phase++; send('/clear'); continue; }
  if (phase === 1) { clearSession = e.session_id; seen.clearChanged = !!clearSession && clearSession !== firstSession; phase++; send(ask); continue; }
  seen.second ||= String(e.result).includes(second) && e.session_id === clearSession;
  if (seen.second && seen.childReported) break;
}
proc.stdin.end();
const code = await new Promise(resolve => proc.on('exit', c => resolve(c)));
clearTimeout(timer);
const statusFile = path.join(dirs.temp, 'clauduct', `status-${proc.pid}.json`);
const s = JSON.parse(fs.readFileSync(statusFile, 'utf8'));
const failures = s.gateway?.totals?.failures ?? {};
console.log(JSON.stringify({ ...seen, exit: code, stderrBytes, status: { category: s.category, exitCode: s.exitCode,
  attempts: s.attempts, inferences: s.inferences, refused: s.gateway?.requests?.refused, broken: s.gateway?.requests?.broken,
  failures, hookInstalled: s.session?.hookInstalled, client: s.gateway?.client?.version, reference: s.gateway?.client?.reference,
  verified: s.gateway?.client?.verified, apiFailures: s.completion?.apiFailures } }));
const pass = seen.first && seen.clearChanged && seen.childReported && seen.second && !seen.errors && code === 0 && s.exitCode === 0 &&
  s.session?.hookInstalled === true && Object.values(failures).every(n => !n);
console.log(pass ? 'SHIP_SESSION PASS' : 'SHIP_SESSION FAIL');
process.exit(pass ? 0 : 1);
