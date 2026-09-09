// Private stdin from the local .NET probe; stdout contains only the shared sanitizer's result.
// Input never selects code, files, commands, a destination, or a transport.
import { summarizeResponse, checkRuntime } from './manual-http-probe.mjs';

const timer = setTimeout(() => process.exit(1), 5000);
try {
  checkRuntime(process.env, process.execArgv);
  const chunks = [];
  let length = 0;
  for await (const chunk of process.stdin) {
    if ((length += chunk.length) > 400000) throw new Error();
    chunks.push(chunk);
  }
  const doc = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(Buffer.concat(chunks)));
  if (!Number.isInteger(doc.status) || doc.status < 100 || doc.status > 599
    || typeof doc.contentType !== 'string' || doc.contentType.length > 16384
    || typeof doc.contentTypePresent !== 'boolean' || typeof doc.body !== 'string'
    || !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(doc.body)) throw new Error();
  const bytes = Buffer.from(doc.body, 'base64');
  if (bytes.length > 256 * 1024 || (!doc.contentTypePresent && doc.contentType !== '')) throw new Error();
  const result = summarizeResponse(doc.status, doc.contentType, bytes);
  result.transportDiagnostics = { contentTypePresent: doc.contentTypePresent };
  // NonValidated is still a .NET header collection, not a raw wire-header API.
  console.log(JSON.stringify(result));
} catch {
  console.log('{"passed":false,"category":"LOCAL_CHECK_FAILED"}');
  process.exitCode = 1;
} finally { clearTimeout(timer); }
