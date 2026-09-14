// Node HTTP calls destroySoon after a Connection: close response. On the tested
// Windows runtime, destroying immediately after writable finish can reset the
// peer before it consumes the response. Wait for the client to consume the
// framed HTTP response and close, with a finite deadline for unresponsive peers.
// Explicit error/cancel/cleanup destroy calls retain their immediate semantics.
export function installHttpClose(socket, { timeoutMs = 1000, onPending = () => {}, onTimeout = () => {} } = {}) {
  if (!Number.isInteger(timeoutMs) || timeoutMs < 1 || timeoutMs > 1000) throw new Error('INVALID_CLOSE_TIMEOUT');
  let timer;
  const written = () => socket.destroy();
  const peerEnded = () => {
    socket.end();
    if (socket.writableFinished) written(); else socket.once('finish', written);
  };
  socket.destroySoon = () => {
    if (timer || socket.destroyed) return;
    onPending(1);
    // On this Windows loopback path, local shutdown can itself produce a read
    // EOF. It cannot serve as evidence that the peer consumed the response.
    // HTTP framing lets ordinary clients finish without waiting for server FIN.
    timer = setTimeout(() => {
      onTimeout(); socket.destroy();
    }, timeoutMs);
    timer.unref();
    // An EOF that preceded this response may only be a half-closed request;
    // give its reply the full deadline instead of treating it as an ACK.
    if (!socket.readableEnded) socket.once('end', peerEnded);
  };
  socket.once('close', () => {
    socket.removeListener('end', peerEnded);
    socket.removeListener('finish', written);
    if (timer) { clearTimeout(timer); timer = undefined; onPending(-1); }
  });
  return { get pending() { return timer !== undefined; } };
}
