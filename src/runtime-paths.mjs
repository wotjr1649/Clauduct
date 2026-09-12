import { userInfo } from 'node:os';
import { join } from 'node:path';

// Bind the credential home and installed executables to the OS user, not a caller's
// USERPROFILE override or one developer's username. No configuration or credential is read.
const userRoot = userInfo().homedir;
export const CODEX_ROOT = join(userRoot, '.codex');
export const CODEX_EXE = join(userRoot, 'AppData', 'Local', 'Programs', 'OpenAI', 'Codex', 'bin', 'codex.exe');
export const CLAUDE_EXE = join(userRoot, '.local', 'bin', 'claude.exe');
