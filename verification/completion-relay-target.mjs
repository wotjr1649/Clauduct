import { lstatSync, readdirSync, realpathSync, readFileSync } from 'node:fs';
import { join, resolve, basename } from 'node:path';
import { NativeError } from '../src/native-protocol.mjs';

const need = value => { if (!value) throw new NativeError('VERIFICATION_TOOL_INPUT_REJECTED'); };
const validId = value => typeof value === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(value);
function inspect(path, directory, limit = 16384) {
  const stat = lstatSync(path);
  need(!stat.isSymbolicLink() && realpathSync(path).toLowerCase() === resolve(path).toLowerCase()
    && (directory ? stat.isDirectory() : stat.isFile() && stat.size <= limit));
}
function directories(path) {
  inspect(path, true);
  const names = readdirSync(path, { withFileTypes: true });
  need(names.length <= 16 && names.every(entry => !entry.isSymbolicLink()));
  return names.filter(entry => entry.isDirectory()).map(entry => join(path, entry.name));
}
function read(path, limit) {
  inspect(path, false, limit);
  return new TextDecoder('utf-8', { fatal: true }).decode(readFileSync(path));
}

// This fixture can message only its single newly created parent. Never resolve a
// name, another session, or a destination supplied by model text or tool output.
export function verifyCompletionRelayTarget({ workingRoot, target, parentCall, childCalls }) {
  try {
    need(validId(target) && validId(parentCall) && Array.isArray(childCalls) && childCalls.length === 2
      && childCalls.every(validId) && new Set([parentCall, ...childCalls]).size === 3);
    inspect(resolve(workingRoot), true);
    const projects = resolve(workingRoot, '..', 'config', 'projects');
    const matches = [];
    for (const project of directories(projects)) for (const session of directories(project)) {
      const agents = join(session, 'subagents');
      if (!readdirSync(session).includes('subagents')) continue;
      inspect(agents, true);
      const names = readdirSync(agents);
      need(names.length <= 16);
      const metadata = [];
      for (const name of names.filter(value => /^agent-[A-Za-z0-9_-]{1,200}\.meta\.json$/.test(value))) {
        const value = JSON.parse(read(join(agents, name), 16384));
        metadata.push({ ...value, fixtureId: name.slice(6, -10) });
      }
      const parent = metadata.find(value => value.fixtureId === target);
      if (!parent) continue;
      need(parent.agentType === 'clauduct-inherit' && parent.toolUseId === parentCall
        && parent.parentAgentId == null && parent.stoppedByUser !== true);
      const children = metadata.filter(value => value.parentAgentId === target);
      need(metadata.length === 3 && children.length === 2
        && new Set(children.map(value => value.toolUseId)).size === 2);
      for (const child of children) {
        need(child.agentType === 'clauduct-probe-inherit' && child.stoppedByUser !== true && childCalls.includes(child.toolUseId));
        const raw = read(join(agents, `agent-${child.fixtureId}.jsonl`), 1048576);
        need(raw.endsWith('\n'));
        const rows = raw.trimEnd().split('\n');
        need(rows.length <= 256);
        const last = rows.map(line => JSON.parse(line)).findLast(row => ['user', 'assistant'].includes(row.type));
        need(last?.type === 'assistant' && last.isApiErrorMessage !== true && last.agentId === child.fixtureId
          && last.sessionId === basename(session) && last.message?.stop_reason === 'end_turn'
          && Array.isArray(last.message.content)
          && last.message.content.filter(block => block.type === 'text').map(block => block.text).join('').trim() === 'MODEL-PROBE-COMPLETED');
      }
      matches.push(target);
    }
    need(matches.length === 1);
    return target;
  } catch { throw new NativeError('VERIFICATION_TOOL_INPUT_REJECTED'); }
}
