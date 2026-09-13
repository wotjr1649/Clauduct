import assert from 'node:assert/strict';
import { verifyNativeDevelopment, verifyDevelopmentOutputRecovery, verifyDevelopmentSequence } from '../verification/verify-native-development.mjs';
const base = { model: 'luna', powershell: 'C:\\PublicFixture\\pwsh.exe', localNative: true,
  priorAttempts: 0, priorElapsedMs: 0, priorInputTokens: 0, priorOutputTokens: 0 };
let checks = 0;
for (const extra of [{ model: 'toString' }, { model: ['luna'] }, { model: 'astra' }, { localNative: 'true' },
  { cutOutputAfterPass: 'true' }, { resumeRoot: 1 }, { resumeRoot: 'public', cutOutputAfterPass: true }, { powershell: 'cmd.exe' },
  { taskId: '../control' }, { taskId: ['retry-delay-window'] }, { taskId: 'toString' }, { taskId: null },
  { continueFrom: 1 }, { continueFrom: 'public', resumeRoot: 'public' }, { continueFrom: 'public', cutOutputAfterPass: true }]) {
  await assert.rejects(verifyNativeDevelopment({ ...base, ...extra }), { message: 'INVALID_ARGUMENTS' }); checks++;
}
await assert.rejects(verifyDevelopmentOutputRecovery({ ...base, localNative: undefined }), { message: 'INVALID_ARGUMENTS' }); checks++;
await assert.rejects(verifyDevelopmentSequence({ ...base, localNative: undefined }), { message: 'INVALID_ARGUMENTS' }); checks++;
for (const key of ['priorAttempts','priorElapsedMs','priorInputTokens','priorOutputTokens']) {
  await assert.rejects(verifyNativeDevelopment({ ...base, [key]: -1 }), { message: 'INVALID_PRIOR_USAGE' }); checks++;
}
await assert.rejects(verifyNativeDevelopment({ ...base, resumeRoot: 'C:\\PublicFixture\\outside-root' }), { message: 'RESUME_ROOT_INVALID' }); checks++;
await assert.rejects(verifyNativeDevelopment({ ...base, continueFrom: 'C:\\PublicFixture\\outside-root' }), { message: 'CONTINUATION_ROOT_INVALID' }); checks++;
console.log(JSON.stringify({ suite: 'development-arguments', checks, actualNativeExecutions: 0, actualCredentialReads: 0, externalRequests: 0 }));
