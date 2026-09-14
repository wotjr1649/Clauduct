import { mkdirSync, mkdtempSync } from 'node:fs';
import { join } from 'node:path';

// Fixtures create their scratch directories inside the project's .tmp working
// directory. Create it here rather than assuming it exists: on a fresh checkout
// only the runner made it, so running a single test file directly failed with
// ENOENT, and in a full run the outcome depended on which test got there first.
export function temporaryDir(base, prefix) {
  const root = join(base, '.tmp');
  mkdirSync(root, { recursive: true });
  return mkdtempSync(join(root, prefix));
}
