import { NativeError, need } from './native-protocol.mjs';

// Backpressure is awaited for every chunk; a slow consumer cannot retain an unbounded write queue.
export async function writeFrames(response, frames, signal, timeoutMs = 30000) {
  need(signal instanceof AbortSignal && Number.isInteger(timeoutMs) && timeoutMs > 0, 'INVALID_DELIVERY');
  for (const frame of frames) {
    const bytes = Buffer.from(`event: ${frame.type}\ndata: ${JSON.stringify(frame)}\n\n`);
    for (let offset = 0; offset < bytes.length; offset += 16384) {
      if (signal.aborted) throw new NativeError('CANCELLED');
      if (response.destroyed) throw new NativeError('CLIENT_DISCONNECTED');
      if (!response.write(bytes.subarray(offset, offset + 16384))) {
        await new Promise((resolve, reject) => {
          const fail = () => settle(new NativeError(signal.aborted ? 'CANCELLED' : 'CLIENT_DISCONNECTED'));
          const drained = () => settle();
          const timer = setTimeout(() => settle(new NativeError('DELIVERY_TIMEOUT')), timeoutMs);
          function settle(error) {
            clearTimeout(timer);
            response.removeListener('drain', drained); response.removeListener('close', fail); response.removeListener('error', fail);
            signal.removeEventListener('abort', fail);
            if (error) reject(error); else resolve();
          }
          response.once('drain', drained); response.once('close', fail); response.once('error', fail);
          signal.addEventListener('abort', fail, { once: true });
          if (signal.aborted || response.destroyed) fail();
        });
      }
    }
  }
}
