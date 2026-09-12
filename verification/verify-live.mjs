import { request } from 'node:http';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { openUserTransport, safeEntryCategory } from '../poc/user-session.mjs';
import { createNativeTransport } from '../src/native-transport.mjs';
import { startNativeGateway } from '../src/native-gateway.mjs';
import { requestStatusSnapshot } from '../src/request-status.mjs';

// One public, fixed request. No repository, transcript, model-produced command, or native
// configuration enters its payload. The ordinary credential/TLS/account checks still run.
export async function verifyLive(transport, { timeoutMs = 60000 } = {}) {
  if (!Number.isInteger(timeoutMs) || timeoutMs < 1 || timeoutMs > 60000) throw new Error('INVALID_ARGUMENTS');
  const controller = new AbortController();
  const marker = 'CLAUDUCT_PUBLIC_CHECK_OK';
  const gateway = await startNativeGateway({ transport: { ...transport,
    send: (body, signal, callbacks) => transport.send(body, signal, { ...callbacks, canRetry: async () => false }) } });
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  let text = '', httpStatus = null, complete = false;
  try {
    const body = JSON.stringify({ model: 'luna', stream: true, max_tokens: 1024,
      output_config: { effort: 'low' }, tools: [], tool_choice: { type: 'none' },
      system: 'This is a public connectivity check. Do not use tools.',
      messages: [{ role: 'user', content: `Reply with exactly ${marker}.` }] });
    await new Promise((done, fail) => {
      const req = request({ hostname: '127.0.0.1', port: gateway.port, path: '/v1/messages', method: 'POST',
        signal: controller.signal, agent: false, headers: { ...gateway.clientHeaders(),
          'anthropic-version': '2023-06-01', 'content-type': 'application/json',
          'content-length': Buffer.byteLength(body) } }, response => {
        httpStatus = response.statusCode;
        let bytes = 0, raw = '';
        response.setEncoding('utf8');
        response.on('data', chunk => {
          bytes += Buffer.byteLength(chunk);
          if (bytes > 256 * 1024) { req.destroy(); fail(new Error('RESPONSE_TOO_LARGE')); return; }
          raw += chunk;
        });
        response.on('error', fail);
        response.on('end', () => {
          if (httpStatus === 200 && response.complete) {
            try {
              for (const frame of raw.split('\n\n')) {
                const data = frame.split('\n').find(line => line.startsWith('data: '));
                if (!data) continue;
                const event = JSON.parse(data.slice(6));
                if (event.type === 'content_block_delta' && event.delta?.type === 'text_delta') text += event.delta.text;
                if (event.type === 'message_stop') complete = true;
              }
            } catch { complete = false; }
          }
          done();
        });
      });
      req.on('error', fail); req.end(body);
    });
  } catch { complete = false; }
  finally { clearTimeout(timer); await gateway.close(); }
  const diagnostics = gateway.diagnostics(), status = requestStatusSnapshot(diagnostics);
  const resourcesClosed = diagnostics.activeSockets === 0 && diagnostics.activeJobs === 0
    && diagnostics.activeTimers === 0 && diagnostics.activeDeliveries === 0 && !diagnostics.busy
    && !diagnostics.cleanupFailed && diagnostics.transport.activeSockets === 0 && diagnostics.transport.activeRequests === 0;
  const exactReply = text.trim() === marker;
  return { suite: 'live-public', passed: complete && exactReply && resourcesClosed
      && status.requestOutcome === 'all-succeeded' && status.lifetime.succeeded === 1,
    httpStatus, complete, exactReply, resourcesClosed, requestOutcome: status.requestOutcome,
    started: status.lifetime.started, succeeded: status.lifetime.succeeded, failed: status.lifetime.failed,
    category: status.recentRequests.at(-1)?.failureCategory ?? null,
    attempts: diagnostics.transport.requestAttempts, clientVersion: status.clientVersion,
    model: 'gpt-5.6-luna', effort: 'low', actualClaudeExecutions: 0, payload: 'fixed-public-text-only' };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  let transport;
  try {
    if (process.argv.length !== 3 || process.argv[2] !== '--live') throw new Error('INVALID_ARGUMENTS');
    transport = openUserTransport({ nonInteractive: true, transportFactory: createNativeTransport });
    const result = await verifyLive(transport);
    console.log(JSON.stringify(result)); process.exitCode = result.passed ? 0 : 1;
  } catch (error) {
    console.log(JSON.stringify({ suite: 'live-public', passed: false, category: safeEntryCategory(error) }));
    process.exitCode = 1;
  } finally { await transport?.close(); }
}
