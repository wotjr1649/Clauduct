import { READ_BRIDGED_BETAS } from '../poc/gateway.mjs';

// Native client tool discovery and per-turn effort are validated by native-protocol.
// This does not enable server-side code execution or arbitrary beta features.
export const NATIVE_BETAS = Object.freeze([...READ_BRIDGED_BETAS,
  'advanced-tool-use-2025-11-20', 'tool-search-tool-2025-10-19',
  'per-turn-control-2026-07-01', 'mid-conversation-output-config-2026-07-01',
  'mid-conversation-tool-changes-2026-07-01',
  // Native Claude may retain this account-state hint with a custom gateway URL.
  // Local Bearer authentication is still required; neither the beta nor Claude OAuth goes upstream.
  'oauth-2025-04-20']);
const unsupported = Object.freeze({
  'extended-cache-ttl-2025-04-11': 'EXTENDED_CACHE_TTL',
  'cache-diagnosis-2026-04-07': 'CACHE_DIAGNOSIS',
  'thinking-binding-controls-2026-08-01': 'THINKING_BINDING',
  'thinking-display-updates-2026-08-18': 'THINKING_DISPLAY_UPDATES',
  'prompt-caching-evict-2026-05-12': 'CACHE_EVICT',
  'structured-outputs-2025-12-15': 'STRUCTURED_OUTPUTS',
  'fast-mode-2026-02-01': 'FAST_MODE',
  'task-budgets-2026-03-13': 'TASK_BUDGETS',
  'advisor-tool-2026-03-01': 'ADVISOR_TOOL',
  'agent-memory-2026-07-22': 'AGENT_MEMORY',
  'mcp-servers-2025-12-04': 'MCP_SERVERS',
  'skills-2025-10-02': 'SERVER_SKILLS',
  'files-api-2025-04-14': 'FILES_API',
  'server-side-fallback-2026-06-01': 'SERVER_FALLBACK_V1',
  'server-side-fallback-2026-07-01': 'SERVER_FALLBACK_V2',
  'fallback-credit-2026-06-01': 'FALLBACK_CREDIT',
  'afk-mode-2026-01-31': 'AFK_MODE',
  'dreaming-2026-04-21': 'DREAMING',
  'managed-agents-2026-04-01': 'MANAGED_AGENTS',
  'user-profiles-2026-03-24': 'USER_PROFILES',
  'web-search-2025-03-05': 'WEB_SEARCH',
  'token-counting-2024-11-01': 'TOKEN_COUNTING',
  'context-hint-2026-04-09': 'CONTEXT_HINT',
  'mid-conversation-system-clear-at-2026-08-21': 'SYSTEM_CLEAR_AT',
  'compact-2026-01-12': 'SERVER_COMPACT',
  'context-1m-2025-08-07': 'CONTEXT_1M',
  'auto-mode-classifier-2026-07-16': 'AUTO_CLASSIFIER',
  'dangerous-tool-use-2026-09-03': 'DANGEROUS_TOOL_USE'
});
export function betaFailure(value) {
  if (value === undefined) return null;
  const betas = value.split(',').map(item => item.trim());
  if (betas.some(item => !item) || new Set(betas).size !== betas.length) return 'INVALID_BETA_HEADER';
  const missing = betas.filter(item => !NATIVE_BETAS.includes(item));
  if (!missing.length) return null;
  const known = missing.filter(item => Object.hasOwn(unsupported, item)).map(item => unsupported[item]);
  return `UNSUPPORTED_BETA known=${known.join('|') || 'NONE'} unknown=${missing.length - known.length}`;
}
