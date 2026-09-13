import { createServer } from 'node:http';
import { runNativeDevelopmentEntry } from './native-development-entry.mjs';
import { createNativeLoopbackTransport } from '../src/native-transport.mjs';
import { publicDevelopmentEvents } from './fixtures/development-responses.mjs';

let requests = 0, failed = false;
const server = createServer((req, res) => {
  void (async () => {
    if (++requests > 5 || req.method !== 'POST' || req.url !== '/backend-api/codex/responses') throw new Error('DEVELOPMENT_LOCAL_REQUEST');
    const chunks = []; let bytes = 0;
    for await (const chunk of req) { if ((bytes += chunk.length) > 1048576) throw new Error('DEVELOPMENT_LOCAL_REQUEST'); chunks.push(chunk); }
    const body = JSON.parse(Buffer.concat(chunks).toString('utf8'));
    const events = publicDevelopmentEvents(body.model, body.reasoning?.effort, requests);
    res.writeHead(200, { 'Content-Type': 'text/event-stream' });
    res.end(events.map(event => `data: ${JSON.stringify(event)}\n\n`).join('') + 'data: [DONE]\n\n');
  })().catch(() => { failed = true; res.writeHead(500); res.end(); });
});
server.requestTimeout = 10000;
await new Promise((done, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', done); });
try {
  await runNativeDevelopmentEntry({
    openTransport: options => options.transportFactory({ requestBudget: options.requestBudget }),
    transportFactory: configuration => createNativeLoopbackTransport(server.address().port, { ...configuration, retryDelayMs: 0 })
  });
} finally {
  server.closeAllConnections(); await new Promise(done => server.close(done));
  if (failed || requests !== 5) process.exitCode = 1;
}
