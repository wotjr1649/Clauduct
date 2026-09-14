import { runNativeDevelopmentEntry } from '../native-development-entry.mjs';

const [root] = process.argv.slice(2);
let transportStarts = 0, guardCode = null;
try {
  await runNativeDevelopmentEntry({ entryArgs: [root, 'development', '-p', '--model', 'sol', '--effort', 'low',
    '--verify-request-limit', '6', '--', 'PUBLIC_UNUSED_TASK'],
  openTransport: () => { transportStarts++; throw new Error('PUBLIC_TRANSPORT_DISABLED'); } });
} catch (error) {
  guardCode = error.message === 'INVALID_DEVELOPMENT_RESERVATION' ? error.message : 'UNEXPECTED_ENTRY_FAILURE';
} finally {
  console.log(JSON.stringify({ guardCode, transportStarts }));
  if (process.connected) process.disconnect();
}
