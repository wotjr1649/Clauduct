import assert from 'node:assert/strict';
import { createServer, get } from 'node:http';
import { summarizeResponse, model, effort, buildBody, buildHeaders, buildFetchOptions,
  inspectFetchResponse, selectTransport, checkRuntime } from './manual-http-probe.mjs';

// Synthetic loopback HTTP only. Does not exercise upstream TLS, login, or sendOnce.
// No runtime preload or environment proxy may alter this direct local comparison.
checkRuntime(process.env, process.execArgv);
let helperTests = 0;
function check(action) { action(); helperTests++; }
check(() => {
  assert.equal(selectTransport(['--live'], true, true), 'node-https');
  assert.equal(selectTransport(['--live-fetch'], true, true), 'node-fetch');
});
check(() => {
  for (const args of [[], ['--live', '--live-fetch'], ['--live-fetch', 'https://example.invalid'], ['--unknown']]) {
    assert.throws(() => selectTransport(args, true, true), /USER_TERMINAL_REQUIRED/);
  }
  for (const flag of ['--live', '--live-fetch']) {
    assert.throws(() => selectTransport([flag], false, true), /USER_TERMINAL_REQUIRED/);
    assert.throws(() => selectTransport([flag], true, false), /USER_TERMINAL_REQUIRED/);
  }
});
check(() => {
  checkRuntime({}, []);
  for (const name of ['NODE_OPTIONS', 'NODE_DEBUG']) {
    assert.throws(() => checkRuntime({ [name]: 'synthetic' }, []), /DEBUG_RUNTIME_UNSUPPORTED/);
  }
  assert.throws(() => checkRuntime({}, ['--import=synthetic']), /DEBUG_RUNTIME_UNSUPPORTED/);
  for (const name of ['NODE_USE_ENV_PROXY', 'NODE_TLS_REJECT_UNAUTHORIZED']) {
    assert.throws(() => checkRuntime({ [name]: '0' }, []), /TRANSPORT_RUNTIME_UNSUPPORTED/);
  }
});
const requestBody = JSON.stringify(buildBody());
const localHeaders = buildHeaders({ accessToken: 'synthetic', account: 'synthetic' }, '0.153.4', requestBody);
check(() => assert.deepEqual(localHeaders, {
  Authorization: 'Bearer synthetic', 'chatgpt-account-id': 'synthetic',
  'Content-Type': 'application/json', Accept: 'text/event-stream', 'Accept-Encoding': 'identity',
  Version: '0.153.4', 'User-Agent': 'codex-cli/0.153.4 (Windows; x64)', originator: 'codex_cli_rs',
  'Openai-Beta': 'responses=experimental', 'Content-Length': Buffer.byteLength(requestBody)
}));
delete localHeaders.Authorization;
delete localHeaders['chatgpt-account-id'];
check(() => {
  const signal = new AbortController().signal;
  const options = buildFetchOptions(requestBody, localHeaders, signal);
  assert.equal(options.redirect, 'manual'); assert.equal(options.credentials, 'omit');
  assert.equal(options.method, 'POST'); assert.equal(options.duplex, 'half');
  assert.equal(options.signal, signal); assert(options.body instanceof ReadableStream);
});

const body = Buffer.from([
  { type: 'response.output_text.delta', output_index: 1, content_index: 0, delta: 'OK' },
  { type: 'response.output_text.done', output_index: 1, content_index: 0, text: 'OK' },
  { type: 'response.completed', response: { model, status: 'completed', reasoning: { effort },
    output: [{ type: 'reasoning' }] } }
].map(event => `event: ${event.type}\r\ndata: ${JSON.stringify(event)}\r\n\r\n`).join(''));
const cases = [
  { name: 'sse', header: ['Content-Type', 'text/event-stream'], passed: true },
  { name: 'mixed-case', header: ['cOnTeNt-TyPe', 'Text/Event-Stream; charset=utf-8'], passed: true },
  { name: 'missing-fixed', passed: false, category: 'MISSING_CONTENT_TYPE' },
  { name: 'missing-chunked', chunked: true, passed: false, category: 'MISSING_CONTENT_TYPE' },
  { name: 'empty', header: ['Content-Type', ''], passed: false, category: 'MISSING_CONTENT_TYPE' },
  { name: 'wrong-type', header: ['Content-Type', 'application/json'], passed: false, category: 'UNEXPECTED_RESPONSE' },
  { name: 'http-error', status: 401, header: ['Content-Type', 'text/event-stream'], passed: false, category: 'AUTH_REJECTED' },
  ...[301, 302, 303, 307, 308].map(status => ({ name: `redirect-${status}`, status,
    passed: false, category: 'REDIRECT_REFUSED' })),
  { name: 'no-421-retry', status: 421, passed: false, category: 'HTTP_ERROR' },
  { name: 'no-429-retry', status: 429, passed: false, category: 'RATE_LIMITED' }
];
let activeCase, received = 0, passed = 0, invalidRequests = 0;
const server = createServer((req, res) => {
  received++;
  if (!activeCase || !['GET', 'POST'].includes(req.method) || req.url !== '/probe'
    || req.headers.authorization !== undefined || req.headers.cookie !== undefined) {
    invalidRequests++;
    res.writeHead(400); res.end(); return;
  }
  const incoming = [];
  let length = 0;
  req.on('error', () => { invalidRequests++; });
  req.on('data', chunk => {
    if ((length += chunk.length) > 4096) { invalidRequests++; req.destroy(); return; }
    incoming.push(chunk);
  });
  req.on('end', () => {
    if (Buffer.concat(incoming).toString('utf8') !== (req.method === 'POST' ? requestBody : '')
      || (req.method === 'POST' && Object.entries(localHeaders).some(([key, value]) => req.headers[key.toLowerCase()] !== String(value)))) {
      invalidRequests++; res.writeHead(400); res.end(); return;
    }
    res.statusCode = activeCase.status ?? 200;
    if (activeCase.header) res.setHeader(...activeCase.header);
    res.setHeader('Connection', 'close');
    if (res.statusCode >= 300 && res.statusCode < 400) res.setHeader('Location', '/must-not-follow');
    if (activeCase.name === 'oversize') { res.end(Buffer.alloc(256 * 1024 + 1, 65)); return; }
    if (activeCase.name === 'truncated') {
      res.setHeader('Content-Length', body.length + 10); res.end(body); return;
    }
    if (activeCase.name === 'abort') { res.write(body.subarray(0, 17)); return; }
    if (!activeCase.chunked) res.setHeader('Content-Length', body.length);
    res.write(body.subarray(0, 17));
    res.end(body.subarray(17));
  });
});
server.maxConnections = 4;
server.requestTimeout = 5000;
server.headersTimeout = 5000;
server.setTimeout(5000, socket => socket.destroy());

function nativeRequest(port) {
  return new Promise((resolve, reject) => {
    const req = get({ hostname: '127.0.0.1', port, path: '/probe', agent: false,
      signal: AbortSignal.timeout(5000) }, res => {
      const contentType = String(res.headers['content-type'] ?? '');
      const rawHeaders = [...res.rawHeaders];
      const chunks = [];
      let length = 0;
      res.on('data', chunk => {
        if ((length += chunk.length) > 4096) { req.destroy(new Error('TEST_RESPONSE_LIMIT')); return; }
        chunks.push(chunk);
      });
      res.on('error', reject);
      res.on('end', () => {
        try {
          assert.equal(String(res.headers['content-type'] ?? ''), contentType);
          assert.deepEqual(res.rawHeaders, rawHeaders);
          assert.deepEqual(Buffer.concat(chunks), body);
          resolve(summarizeResponse(res.statusCode, contentType, Buffer.concat(chunks), rawHeaders));
        } catch (error) { reject(error); }
      });
    });
    req.on('error', reject);
  });
}

async function fetchRequest(port) {
  const response = await fetch(`http://127.0.0.1:${port}/probe`,
    buildFetchOptions(requestBody, localHeaders, AbortSignal.timeout(5000)));
  return inspectFetchResponse(response);
}

const deadline = setTimeout(() => {
  server.closeAllConnections(); server.close();
  console.error('LOOPBACK_TEST_TIMEOUT'); process.exit(1);
}, 20000);
try {
  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', resolve);
  });
  const port = server.address().port;
  for (const testCase of cases) {
    activeCase = testCase;
    for (const client of [nativeRequest, fetchRequest]) {
      const before = received;
      const result = await client(port);
      assert.equal(received, before + 1, testCase.name);
      assert.equal(result.passed, testCase.passed, testCase.name);
      assert.equal(result.category, testCase.category ?? 'SUCCESS', testCase.name);
      assert.equal(result.headerDiagnostics.normalizedContentTypeNonEmpty, Boolean(testCase.header?.[1]));
      assert.equal(result.headerDiagnostics.rawHeadersAvailable, client === nativeRequest);
      assert.equal(result.headerDiagnostics.rawContentTypePresent,
        client === nativeRequest ? Boolean(testCase.header) : null);
      if (client === fetchRequest) assert.equal(result.transportDiagnostics.contentTypePresent, Boolean(testCase.header));
      assert.equal(result.responseBytes, body.length);
      const sseResult = result.sseDiagnostics ?? result;
      if (testCase.passed || testCase.category === 'MISSING_CONTENT_TYPE') {
        assert.equal(sseResult.passed, true);
        assert.equal(sseResult.replySource, 'stream');
        assert.equal(sseResult.exactOK, true);
      }
      passed++;
    }
  }
  for (const name of ['oversize', 'truncated', 'abort']) {
    activeCase = { name, header: ['Content-Type', 'text/event-stream'] };
    const before = received;
    if (name === 'abort') {
      const controller = new AbortController();
      const response = await fetch(`http://127.0.0.1:${port}/probe`, buildFetchOptions(requestBody, localHeaders, controller.signal));
      controller.abort();
      await assert.rejects(inspectFetchResponse(response));
    } else {
      await assert.rejects(fetchRequest(port), name === 'oversize' ? /RESPONSE_TOO_LARGE/ : undefined);
    }
    assert.equal(received, before + 1);
    passed++;
  }
  assert.equal(invalidRequests, 0);
  assert.equal(received, cases.length * 2 + 3);
  console.log(JSON.stringify({ helperTests, loopbackTests: passed, passed, localRequests: received,
    externalRequests: 0, credentialReads: 0, cases: cases.map(item => item.name) }));
} finally {
  server.closeAllConnections();
  await new Promise(resolve => server.close(resolve));
  clearTimeout(deadline);
}
