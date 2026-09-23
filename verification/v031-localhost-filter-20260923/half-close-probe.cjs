const net = require('node:net');
async function trial() {
  return await new Promise(resolve => {
    let peer, client, data = '', settled = false;
    const server = net.createServer({allowHalfOpen: true}, socket => {
      peer = socket;
      socket.on('error', () => {});
      socket.on('data', () => {});
      socket.on('end', () => setTimeout(() => {
        if (!socket.destroyed) socket.write('X');
        setTimeout(() => { if (!socket.destroyed) socket.end(); }, 300);
      }, 100));
    });
    function finish(result) {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      client?.destroy();
      peer?.destroy();
      server.close();
      resolve(result);
    }
    const timer = setTimeout(() => finish('timeout'), 1800);
    server.on('error', e => finish(e.code || 'server_error'));
    server.listen(0, '127.0.0.1', () => {
      client = net.createConnection({host: '127.0.0.1', port: server.address().port, allowHalfOpen: true});
      client.on('connect', () => client.end('Q'));
      client.on('data', bytes => data += bytes.toString());
      client.on('end', () => finish(data === 'X' ? 'ok' : `eof_${data.length}`));
      client.on('error', e => finish(e.code || 'client_error'));
    });
  });
}
(async () => {
  const counts = {};
  for (let i = 0; i < 20; i++) { const outcome = await trial(); counts[outcome] = (counts[outcome] || 0) + 1; }
  process.stdout.write(JSON.stringify(counts) + '\n');
})().catch(e => { process.stderr.write(e.message + '\n'); process.exitCode = 1; });
