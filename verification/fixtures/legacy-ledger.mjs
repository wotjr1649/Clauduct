import { mkdirSync, mkdtempSync, writeFileSync, readFileSync, existsSync, lstatSync, realpathSync } from 'node:fs';
import { join, dirname, basename } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
const project = dirname(dirname(dirname(fileURLToPath(import.meta.url))));
export function createPublicLegacyLedger({ ledgerPatch = {}, manifestPatch = {} } = {}) {
  const temporary = join(project, '.tmp'); if (!existsSync(temporary)) mkdirSync(temporary);
  const info = lstatSync(temporary);
  if (!info.isDirectory() || info.isSymbolicLink() || realpathSync(temporary).toLowerCase() !== temporary.toLowerCase()) throw new Error('PUBLIC_LEDGER_PATH');
  const sourceDirectory = mkdtempSync(join(temporary, 'legacy-ledger-public-'));
  const ledger = { priorLedger: `PUBLIC_LEDGER_${basename(sourceDirectory)}`,
    attemptLower: 35, attemptUpper: 37, knownPreConnectionFaultAttempts: 2, knownPreRequestServiceSignals: 1,
    knownInputTokens: 7000, knownOutputTokens: 1200, nativePhaseElapsedMs: 4000, nativePhaseElapsedMsIsPartial: true,
    unobservedNativeDurationRuns: 1, unobservedNativeDurationReservationMs: 60000,
    inFlightUsageUnobservedRuns: 1, inFlightInputReservation: 131072, inFlightOutputReservation: 32768,
    earlierTimeoutAttemptRangeUnresolved: true, firstFailureUsageUnobserved: true, searchTokenUsageNotReported: true,
    earlierFullInputReservation: 262144, earlierFullOutputReservation: 65536, ...ledgerPatch };
  const manifest = { version: 1, cumulativeAttemptUpper: 80, cumulativeKnownAndReservedInputTokens: 1048576,
    cumulativeKnownAndReservedOutputTokens: 262144, cumulativeKnownAndReservedNativeMs: 600000, ...manifestPatch };
  writeFileSync(join(sourceDirectory, 'result.json'), JSON.stringify(ledger, null, 2) + '\n', { flag: 'wx' });
  writeFileSync(join(sourceDirectory, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n', { flag: 'wx' });
  const digest = name => createHash('sha256').update(readFileSync(join(sourceDirectory, name))).digest('hex');
  return { sourceDirectory, ledgerHash: digest('result.json'), manifestHash: digest('manifest.json'), ledger, manifest };
}
