// Synthetic subprocess fixture. No Claude, credentials, files, tools or external host.
import { request } from 'node:http';
import { setTimeout as delay } from 'node:timers/promises';
import { ALIAS, readTool } from './adapter.mjs';
import { FIXTURE_MARKER } from './request-inspector.mjs';

const port = Number(process.argv[2]), mode = process.argv[3];
if (!Number.isInteger(port) || port < 1 || port > 65535
  || !['capture', 'repeat', 'idle', 'noise', 'reject', 'early'].includes(mode)) process.exit(2);
if (mode === 'idle') await delay(5000);
else if (mode === 'early') process.exitCode = 7;
else if (mode === 'noise') process.stdout.write('SYNTHETIC_PRIVATE_VALUE'.repeat(20000));
else {
  for (let index = 0; index < (mode === 'repeat' ? 3 : 1); index++) {
    const body = JSON.stringify({ model: ALIAS, stream: true, max_tokens: 1024,
      messages: [{ role: 'user', content: 'SYNTHETIC_PRIVATE_VALUE' }],
      tools: [readTool()], tool_choice: { type: 'auto' } });
    await new Promise(resolveDone => {
      const req = request({ hostname: '127.0.0.1', port, path: '/v1/messages?beta=true',
        method: 'POST', agent: false, signal: AbortSignal.timeout(2000),
        headers: { Authorization: `Bearer ${mode === 'reject' ? 'SYNTHETIC_PRIVATE_VALUE' : FIXTURE_MARKER}`,
          'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(body),
          'anthropic-version': '2023-06-01' } }, res => {
        res.resume(); res.once('end', resolveDone); res.once('error', resolveDone);
      });
      req.once('error', resolveDone); req.end(body);
    });
  }
  process.stdout.write('SYNTHETIC_PRIVATE_VALUE'); process.stderr.write('PRIVATE_HEADER');
}
