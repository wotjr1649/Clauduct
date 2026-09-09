// Synthetic client only; never starts Claude or any model. All HTTP stays on 127.0.0.1.
import { request } from 'node:http';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { resolve } from 'node:path';
import { ALIAS, FIXTURE_PATH } from './adapter.mjs';

export function syntheticClaudeRequest() {
  return { model: ALIAS, stream: true, max_tokens: 64000,
    system: [{ type: 'text', text: 'SYNTHETIC_SYSTEM', cache_control: { type: 'ephemeral' } }],
    messages: [{ role: 'user', content: [{ type: 'text', text: 'Read the public fixture once.' },
      { type: 'text', text: 'SYNTHETIC_CONTEXT' }] },
      { role: 'system', content: [{ type: 'text', text: 'SYNTHETIC_LATE_SYSTEM', cache_control: { type: 'ephemeral' } }] }],
    thinking: { type: 'adaptive', display: 'omitted' }, output_config: { effort: 'low' },
    metadata: { user_id: 'SYNTHETIC_PRIVATE_ID' },
    context_management: { edits: [{ type: 'clear_thinking_20251015', keep: 'all' }] },
    tools: [{ name: 'Read', description: 'Synthetic Read description',
      input_schema: { type: 'object', $schema: 'https://json-schema.org/draft/2020-12/schema', additionalProperties: false,
        required: ['file_path'], properties: { file_path: { type: 'string' }, pages: { type: 'string' },
          offset: { type: 'integer', minimum: 0, maximum: Number.MAX_SAFE_INTEGER },
          limit: { type: 'integer', exclusiveMinimum: 0, maximum: Number.MAX_SAFE_INTEGER } } } }] };
}
async function main() {
  const port = Number(process.argv[2]), mode = process.argv[3];
  if (!Number.isInteger(port) || port < 1 || port > 65535
    || !['normal', 'denied', 'wrong-result', 'bad-line', 'wrong-cli', 'late-cli', 'bad-beta', 'duplicate-beta', 'extra',
      'extra-system', 'changed-history', 'changed-options', 'changed-tool', 'changed-id', 'invalid-cache', 'invalid-tail',
      'duplicate-result', 'extra-message'].includes(mode)) process.exit(2);
  let reads = 0;
  async function send(body) {
    return new Promise((resolveResponse, reject) => {
      const raw = JSON.stringify(body);
      const req = request({ hostname: '127.0.0.1', port, method: 'POST', path: '/v1/messages?beta=true', agent: false,
        signal: AbortSignal.timeout(3000), headers: { Authorization: `Bearer ${process.env.ANTHROPIC_AUTH_TOKEN}`,
          'anthropic-version': '2023-06-01', 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(raw),
          'anthropic-beta': mode === 'bad-beta' ? 'SYNTHETIC_PRIVATE_BETA'
            : 'claude-code-20250219,effort-2025-11-24,thinking-token-count-2026-05-13'
              + (mode === 'duplicate-beta' ? ',thinking-token-count-2026-05-13' : ''),
          'x-claude-code-session-id': 'synthetic-read-session' } }, res => {
        let text = '';
        res.on('data', b => { text += b.toString(); if (text.length > 262144) req.destroy(); });
        res.on('error', reject);
        res.on('end', () => {
          if (res.statusCode !== 200) { reject(new Error('GATEWAY_REJECTED')); return; }
          const frames = text.trim().split('\n\n').map(frame => JSON.parse(frame.split('\ndata: ')[1]));
          const block = frames.find(frame => frame.type === 'content_block_start').content_block;
          const delta = frames.find(frame => frame.type === 'content_block_delta').delta;
          resolveResponse(block.type === 'tool_use' ? { ...block, input: JSON.parse(delta.partial_json) } : { type: 'text', text: delta.text });
        });
      });
      req.on('error', reject); req.end(raw);
    });
  }
  try {
    const first = syntheticClaudeRequest(), tool = await send(first);
    if (tool.type !== 'tool_use' || tool.name !== 'Read' || tool.input.file_path !== FIXTURE_PATH) throw new Error('BAD_TOOL');
    let content;
    if (mode === 'denied') content = 'Permission denied';
    else { reads++; content = '     1→' + readFileSync(FIXTURE_PATH, 'utf8'); }
    if (mode === 'wrong-result') content = '     1→CLAUDUCT_READ_' + '0'.repeat(32);
    if (mode === 'bad-line') content = '1' + '\t'.repeat(10000) + 'X' + content.slice(7).trim() + 'X';
    const next = structuredClone({ ...first, messages: [...first.messages, { role: 'assistant', content: [tool] },
      { role: 'user', content: [{ type: 'tool_result', tool_use_id: tool.id, is_error: mode === 'denied',
        content: [{ type: 'text', text: content }] }] }] });
    if (['extra-system', 'changed-history', 'changed-options', 'changed-tool', 'changed-id', 'invalid-cache',
      'invalid-tail', 'duplicate-result', 'extra-message'].includes(mode)) {
      next.messages[1].content = 'SYNTHETIC_LATE_SYSTEM';
      next.messages[0].content[1].cache_control = { type: 'ephemeral', ttl: '5m' };
      next.messages[2].content[0].cache_control = { type: 'ephemeral' };
      next.messages[3].content[0].cache_control = { type: 'ephemeral' };
      next.system = 'SYNTHETIC_SYSTEM';
      next.tools[0].cache_control = { type: 'ephemeral' };
      next.messages.push({ role: 'system', content: [{ type: 'text', text: 'SYNTHETIC_PRIVATE_SYSTEM',
        cache_control: { type: 'ephemeral' } }] });
    }
    if (mode === 'changed-history') next.messages[0].content[0].text = 'SYNTHETIC_CHANGED';
    if (mode === 'changed-options') next.max_tokens = 4096;
    if (mode === 'changed-tool') next.messages[2].content[0].input.file_path = 'SYNTHETIC_WRONG_PATH';
    if (mode === 'changed-id') next.messages[3].content[0].tool_use_id = 'wrong';
    if (mode === 'invalid-cache') next.messages[0].content[1].cache_control.ttl = 'forever';
    if (mode === 'invalid-tail') next.messages[4].content = [{ type: 'tool_use', id: 'wrong' }];
    if (mode === 'duplicate-result') next.messages[3].content.push(next.messages[3].content[0]);
    if (mode === 'extra-message') next.messages.push({ role: 'system', content: 'SYNTHETIC_EXTRA' });
    const last = await send(next);
    if (mode === 'extra') { try { await send(first); } catch { /* Expected closed gateway, tested by server request count. */ } }
    if (mode === 'late-cli') await new Promise(done => setTimeout(done, 150));
    process.stdout.write(JSON.stringify({ type: 'result', subtype: 'success', is_error: false,
      result: mode === 'wrong-cli' ? 'WRONG' : last.text, syntheticFixtureReads: reads }));
  } catch { process.stdout.write(JSON.stringify({ type: 'result', subtype: 'error', is_error: true, syntheticFixtureReads: reads })); process.exitCode = 1; }
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) await main();
