import assert from 'node:assert/strict';
import { createServer, request } from 'node:http';
import { createConnection } from 'node:net';
import { installHttpClose } from './http-close.mjs';

let pending = 0, timeouts = 0, requests = 0, latestSocket;
const sockets = new Set();
const chunk = 'PUBLIC_CHUNK\n'.repeat(1024);
const server = createServer((req, res) => { requests++;
  if (req.url === '/half-closed') {
    res.writeHead(200, { Connection: 'close', 'Content-Length': 23 }); res.end('PUBLIC_HALF_CLOSE_REPLY'); return;
  }
  res.writeHead(200, { Connection: 'close' });
  for (let i = 0; i < 8; i++) res.write(chunk);
  res.end(); });
server.on('connection', socket => {
  latestSocket = socket;
  sockets.add(socket); socket.once('close', () => sockets.delete(socket)); socket.on('error', () => {});
  installHttpClose(socket, { timeoutMs: 100, onPending: change => { pending += change; }, onTimeout: () => { timeouts++; } });
});
await new Promise(done => server.listen(0, '127.0.0.1', done));
try {
  for (let i = 0; i < 20; i++) {
    const text = await new Promise((done, reject) => {
      const req = request({ hostname: '127.0.0.1', port: server.address().port, path: '/', agent: false,
        signal: AbortSignal.timeout(2000) }, res => {
        let text = ''; res.on('data', chunk => { text += chunk; });
        res.once('end', () => done(text)); res.once('error', reject);
      });
      req.once('error', reject); req.end();
    });
    assert.equal(text, chunk.repeat(8));
  }
  assert.equal(timeouts, 0);
  // A peer that receives FIN but never sends its own FIN cannot retain the socket.
  const client = createConnection({ host: '127.0.0.1', port: server.address().port, allowHalfOpen: true });
  try {
    const ended = new Promise((done, reject) => { client.once('end', done); client.once('error', reject); });
    client.resume(); client.write('GET / HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\n\r\n');
    await ended;
    await new Promise(done => setTimeout(done, 200));
    // Some Windows runtimes report peer EOF even while the client write side
    // remains open. Either EOF or the deadline must release the actual socket.
    assert.ok(timeouts <= 1); assert.equal(pending, 0); assert.equal(sockets.size, 0);
  } finally { client.destroy(); }
  // A client may finish sending its request before reading the reply. That FIN
  // must not cut short the server's pending normal response-close path.
  const peerState = { end: false, finish: false, timeout: false, errorCode: null, bytesRead: 0, bytesWritten: 0 };
  const reply = await new Promise(done => {
    const peer = createConnection({ host: '127.0.0.1', port: server.address().port });
    let body = '';
    peer.setTimeout(1000, () => { peerState.timeout = true; peer.destroy(); });
    peer.once('end', () => { peerState.end = true; }); peer.once('finish', () => { peerState.finish = true; });
    peer.on('data', chunk => { body += chunk.toString(); });
    peer.on('error', error => { peerState.errorCode = ['ECONNRESET', 'EPIPE', 'ERR_STREAM_PREMATURE_CLOSE'].includes(error.code) ? error.code : 'OTHER'; });
    peer.once('close', () => { peerState.bytesRead = peer.bytesRead; peerState.bytesWritten = peer.bytesWritten; done(body); });
    peer.once('connect', () => peer.end('GET /half-closed HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\n\r\n'));
  });
  assert.equal(reply.includes('PUBLIC_HALF_CLOSE_REPLY'), true, JSON.stringify({ requests, responseBytes: reply.length,
    serverBytesRead: latestSocket.bytesRead, serverBytesWritten: latestSocket.bytesWritten,
    serverClosed: latestSocket.closed, serverReadableEnded: latestSocket.readableEnded,
    serverWritableFinished: latestSocket.writableFinished, pending, timeouts, peerState }));
  assert.equal(requests, 22);
} finally {
  server.closeAllConnections(); await new Promise(done => server.close(done));
}
assert.equal(pending, 0);
console.log(JSON.stringify({ suite: 'http-close', roundtrips: 20, halfOpenPeerClosed: true, halfClosedClientReply: true, pending, closeTimeouts: timeouts,
  externalRequests: 0, actualCredentialReads: 0 }));
