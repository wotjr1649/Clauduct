import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { NativeError, OUTPUT_TOKEN_LIMIT_POLICY } from '../src/native-protocol.mjs';

// A usage ceiling and a limit on generation are different contracts. The
// subscription transport currently offers only the former. Fail before opening
// credentials or starting native when the caller requires the latter.
export function fixtureTokenBudget(options = {}) {
  if (!options || typeof options !== 'object' || Array.isArray(options)
    || Object.keys(options).some(key => !['maxInputTokens', 'maxOutputTokens', 'requirePreGenerationLimit'].includes(key)))
    throw new NativeError('VERIFICATION_TOKEN_BUDGET_INVALID');
  const { maxInputTokens = 131072, maxOutputTokens = 32768, requirePreGenerationLimit = false } = options;
  if (!Number.isSafeInteger(maxInputTokens) || maxInputTokens < 1 || maxInputTokens > 131072
    || !Number.isSafeInteger(maxOutputTokens) || maxOutputTokens < 1 || maxOutputTokens > 32768
    || typeof requirePreGenerationLimit !== 'boolean') throw new NativeError('VERIFICATION_TOKEN_BUDGET_INVALID');
  if (requirePreGenerationLimit) throw new NativeError('VERIFICATION_PREGENERATION_LIMIT_UNAVAILABLE');
  return Object.freeze({ maxInputTokens, maxOutputTokens, outputTokenLimitPolicy: OUTPUT_TOKEN_LIMIT_POLICY });
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const [input, output, hard] = process.argv.slice(2);
    if (process.argv.length !== 5 || !/^[1-9][0-9]*$/.test(input) || !/^[1-9][0-9]*$/.test(output)
      || !['true', 'false'].includes(hard)) throw new NativeError('VERIFICATION_TOKEN_BUDGET_INVALID');
    process.stdout.write(JSON.stringify(fixtureTokenBudget({ maxInputTokens: Number(input), maxOutputTokens: Number(output), requirePreGenerationLimit: hard === 'true' })) + '\n');
  } catch (error) { process.stderr.write(`${error instanceof NativeError ? error.code : 'VERIFICATION_TOKEN_BUDGET_INVALID'}\n`); process.exitCode = 1; }
}
