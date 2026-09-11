import { request } from 'node:http';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { MODELS, EFFORTS } from './models.mjs';
import { clientVersionPolicy, isClientVersion } from './client-version.mjs';
import { contextFromEnvironment } from './agent-route.mjs';
import { EVENT_DIAGNOSTIC_TYPES, EVENT_TYPE_FORMATS, REQUEST_STAGES, FAILURE_DIAGNOSTIC_CATEGORIES, REQUEST_FAILURES, UPSTREAM_FAILURES, capturableEventName,
  UPSTREAM_ERROR_CODES, UPSTREAM_ERROR_TYPES, UPSTREAM_INCOMPLETE_REASONS } from './native-protocol.mjs';
import { SELECTION_FAILURES, SELECTION_IO_CODES, COMPLETION_FAILURES, COMPLETION_STATES } from './agent-selection.mjs';

const times = ['admissionStartedMs', 'admittedMs', 'preparedMs', 'transportStartedMs', 'firstEventMs',
  'firstTextDeltaMs', 'firstDownstreamWriteMs', 'transportFinishedMs', 'finishedMs'];
const number = value => typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : null;
const counter = value => Number.isSafeInteger(value) && value >= 0 ? value : null;
const reference = value => typeof value === 'string' && /^[a-f0-9]{32}$/.test(value) ? value : null;
const betaName = value => typeof value === 'string' && /^[a-z][a-z0-9-]{0,63}$/.test(value);
const betaLabels = value => Array.isArray(value)
  ? value.filter(label => typeof label === 'string' && /^[A-Z][A-Z0-9_]{0,31}$/.test(label)).slice(0, 8) : [];

// User-operated within the native child. Credentials go only to its fixed loopback endpoint.
export async function readRequestStatus(env) {
  const base = env.ANTHROPIC_BASE_URL, token = env.ANTHROPIC_AUTH_TOKEN;
  if (typeof base !== 'string' || !/^http:\/\/127\.0\.0\.1:[0-9]{1,5}$/.test(base)
    || typeof token !== 'string' || !/^[A-Za-z0-9_-]{43}$/.test(token)) throw new Error('STATUS_UNAVAILABLE');
  const snapshot = await new Promise((done, fail) => {
    const req = request(`${base}/clauduct/status`, { agent: false, signal: AbortSignal.timeout(5000),
      headers: { Authorization: `Bearer ${token}` } }, res => {
      if (res.statusCode !== 200) { res.destroy(); fail(new Error('STATUS_UNAVAILABLE')); return; }
      let raw = '', bytes = 0;
      res.setEncoding('utf8');
      res.on('data', chunk => {
        bytes += Buffer.byteLength(chunk);
        if (bytes > 256 * 1024) { res.destroy(); fail(new Error('STATUS_UNAVAILABLE')); return; }
        raw += chunk;
      });
      res.on('error', fail);
      res.on('end', () => {
        try {
          const value = JSON.parse(raw);
          if (!res.complete || !Array.isArray(value.recentRequests)) throw new Error('STATUS_UNAVAILABLE');
          done(value);
        } catch { fail(new Error('STATUS_UNAVAILABLE')); }
      });
    });
    req.on('error', fail); req.end();
  });
  return requestStatusSnapshot(snapshot, env);
}

// Pure projection, also usable after gateway shutdown: no credential or HTTP access.
export function requestStatusSnapshot(value, env = {}) {
  if (!Array.isArray(value?.recentRequests)) throw new Error('STATUS_UNAVAILABLE');
  const rows = value.recentRequests.slice(-16), { lifetime, correlationScope } = value;
  // Re-projecting an already projected snapshot keeps the same version evidence.
  const clientVersion = value.transport?.clientVersion ?? value.clientVersion;
  const projectRow = row => ({ request: number(row?.request),
    sessionRef: reference(row?.sessionRef), agentRef: reference(row?.agentRef), parentRef: reference(row?.parentRef),
    startedAt: typeof row?.startedAt === 'string' && /^\d{4}-\d{2}-\d{2}T[\d:.]+Z$/.test(row.startedAt) ? row.startedAt : null,
    model: Object.values(MODELS).some(model => model.model === row?.model) ? row.model : null,
    requestedModel: Object.values(MODELS).some(model => model.model === row?.requestedModel) ? row.requestedModel : null,
    effort: EFFORTS.includes(row?.effort) ? row.effort : null,
    requestedEffort: EFFORTS.includes(row?.requestedEffort) ? row.requestedEffort : null,
    selectionSource: ['explicit-metadata', 'role-default', 'native-inherit', 'definition-inherit', 'definition-model', 'skill-result', 'workflow-result', 'verified-resume', 'verified-peer-resume', 'verified-completion-resume', 'native-fork'].includes(row?.selectionSource) ? row.selectionSource : null,
    purpose: ['compact-template', 'conversation'].includes(row?.purpose) ? row.purpose : null,
    compactShape: row?.compactShape ? {
      lastRole: ['user', 'assistant', 'other'].includes(row.compactShape.lastRole) ? row.compactShape.lastRole : 'other',
      textBlocks: number(row.compactShape.textBlocks), mixedBlocks: row.compactShape.mixedBlocks === true,
      prefixMatches: row.compactShape.prefixMatches === true, suffixMatches: row.compactShape.suffixMatches === true,
      matches: row.compactShape.matches === true } : null,
    role: ['Explore', 'Plan', 'general-purpose', 'claude', 'workflow-subagent'].includes(row?.role) ? row.role : null,
    roleRegistered: row?.roleRegistered === true,
    agentContextPolicy: row?.agentContextPolicy ? { evidence: 'subagent-start-hook-environment',
      window: number(row.agentContextPolicy.window), autoCompactWindow: number(row.agentContextPolicy.autoCompactWindow),
      compactPercent: number(row.agentContextPolicy.compactPercent) } : null,
    subagent: row?.subagent === true, success: row?.success === true,
    webSearchRequested: row?.webSearchRequested === true, webSearchCalls: counter(row?.webSearchCalls) ?? 0,
    // Whether this gateway answered the search itself, and how many links it returned. Counts
    // only: no query and no result ever reaches a status response.
    webSearchAnswered: row?.webSearchAnswered === true, webSearchLinks: counter(row?.webSearchLinks) ?? 0,
    judgedBetaLabels: betaLabels(row?.judgedBetaLabels),
    contentBlocks: row?.contentBlocks ? { text: counter(row.contentBlocks.text) ?? 0,
      toolUse: counter(row.contentBlocks.toolUse) ?? 0, thinking: counter(row.contentBlocks.thinking) ?? 0 } : null,
    firstContentBlock: ['text', 'tool_use', 'redacted_thinking', 'server_tool_use', 'web_search_tool_result'].includes(row?.firstContentBlock) ? row.firstContentBlock : null,
    unsupportedEvent: EVENT_DIAGNOSTIC_TYPES.includes(row?.unsupportedEvent) ? row.unsupportedEvent : null,
    unsupportedEventTypeFormat: EVENT_TYPE_FORMATS.includes(row?.unsupportedEventTypeFormat) ? row.unsupportedEventTypeFormat : null,
    failureStage: REQUEST_STAGES.includes(row?.failureStage) ? row.failureStage : null,
    requestFailure: REQUEST_FAILURES.includes(row?.requestFailure) ? row.requestFailure : null,
    selectionFailure: SELECTION_FAILURES.includes(row?.selectionFailure) ? row.selectionFailure : null,
    selectionIoCode: SELECTION_IO_CODES.includes(row?.selectionIoCode) ? row.selectionIoCode : null,
    completionFailure: COMPLETION_FAILURES.includes(row?.completionFailure) ? row.completionFailure : null,
    completionParentState: COMPLETION_STATES.includes(row?.completionParentState) ? row.completionParentState : null,
    completionChildState: COMPLETION_STATES.includes(row?.completionChildState) ? row.completionChildState : null,
    reviewDiffMismatch: ['call-count', 'tool-name', 'command', 'background'].includes(row?.reviewDiffMismatch) ? row.reviewDiffMismatch : null,
    failureCategory: FAILURE_DIAGNOSTIC_CATEGORIES.includes(row?.failureCategory) ? row.failureCategory : null,
    upstreamFailureEvent: typeof row?.upstreamFailureEvent === 'string' && Object.hasOwn(UPSTREAM_FAILURES, row.upstreamFailureEvent)
      && UPSTREAM_FAILURES[row.upstreamFailureEvent] === row?.failureCategory ? row.upstreamFailureEvent : null,
    upstreamErrorCode: Object.values(UPSTREAM_FAILURES).includes(row?.failureCategory)
      && UPSTREAM_ERROR_CODES.includes(row?.upstreamErrorCode) ? row.upstreamErrorCode : null,
    upstreamErrorType: Object.values(UPSTREAM_FAILURES).includes(row?.failureCategory)
      && UPSTREAM_ERROR_TYPES.includes(row?.upstreamErrorType) ? row.upstreamErrorType : null,
    upstreamIncompleteReason: row?.failureCategory === UPSTREAM_FAILURES['response.incomplete']
      && UPSTREAM_INCOMPLETE_REASONS.includes(row?.upstreamIncompleteReason) ? row.upstreamIncompleteReason : null,
    clientDisconnected: row?.clientDisconnected === true,
    clientDisconnectedMs: number(row?.clientDisconnectedMs),
    snapshotMismatchMs: number(row?.snapshotMismatchMs),
    snapshotMismatchPhase: ['stream', 'final'].includes(row?.snapshotMismatchPhase) ? row.snapshotMismatchPhase : null,
    lastUpstreamEventMs: number(row?.lastUpstreamEventMs),
    pingCount: number(row?.pingCount) ?? 0, lastPingMs: number(row?.lastPingMs),
    auxiliaryMetadataEvents: number(row?.auxiliaryMetadataEvents) ?? 0,
    ...Object.fromEntries(times.map(key => [key, number(row?.[key])])),
    retryScheduledMs: Array.isArray(row?.retryScheduledMs) ? row.retryScheduledMs.slice(0, 5).map(number) : [],
    attempts: Array.isArray(row?.attempts) ? row.attempts.slice(0, 6).map(attempt => ({
      ...Object.fromEntries(['attempt', 'startedMs', 'requestFlushedMs', 'headersMs', 'firstBodyMs', 'endedMs', 'status']
        .map(key => [key, number(attempt?.[key])])), completed: attempt?.completed === true,
      terminalState: ['open', 'completed', 'done', ...Object.keys(UPSTREAM_FAILURES)].includes(attempt?.terminalState) ? attempt.terminalState : null,
      postCompletionFrame: [...EVENT_DIAGNOSTIC_TYPES, 'done', 'invalid-json', 'oversized'].includes(attempt?.postCompletionFrame) ? attempt.postCompletionFrame : null,
      postCompletionSequence: ['missing', 'unsequenced', 'invalid', 'expected', 'unexpected'].includes(attempt?.postCompletionSequence) ? attempt.postCompletionSequence : null })) : [] });
  const failures = Array.isArray(value.failureRequests) ? value.failureRequests
    : value.failureHistory?.records;
  const retained = Array.isArray(failures) ? (failures.length > 16 ? [...failures.slice(0, 8), ...failures.slice(-8)] : failures) : null;
  const failed = counter(lifetime?.failed), started = counter(lifetime?.started), succeeded = counter(lifetime?.succeeded);
  return { ...(isClientVersion(clientVersion) ? clientVersionPolicy(clientVersion)
    : { clientVersion: null, referenceClientVersion: null, clientVersionStatus: 'not-observed' }),
    clientContextPolicy: { evidence: 'inherited-environment',
    ...(contextFromEnvironment(env) ?? { window: null, autoCompactWindow: null, compactPercent: null }) },
    clientExecutionPolicy: { evidence: 'inherited-environment', nonStreamingFallbackDisabled:
      env.CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK === '1' ? true : null },
    correlationScope: reference(correlationScope),
    // null means this channel does not carry the capture (the in-session status API);
    // an empty array means the capture is available and no unmapped name was observed.
    unsupportedEventNames: Array.isArray(value.unsupportedEventNames)
      ? value.unsupportedEventNames.map(capturableEventName).filter(Boolean).slice(0, 4) : null,
    unknownBetaNames: Array.isArray(value.unknownBetaNames)
      ? value.unknownBetaNames.filter(betaName).slice(0, 8) : null,
    judgedBetaLabels: betaLabels(value.judgedBetaLabels),
    requestOutcome: failed === null || started === null || succeeded === null ? 'not-observed'
      : failed > 0 ? 'has-failures' : started > succeeded ? 'in-progress' : started === 0 ? 'no-requests' : 'all-succeeded',
    lifetime: lifetime ? { scope: 'gateway-lifetime', failuresByStage: lifetime.failuresByStage
      ? Object.fromEntries(REQUEST_STAGES.map(stage => [stage, counter(lifetime.failuresByStage[stage])])) : null, ...Object.fromEntries(
      ['started', 'succeeded', 'failed', 'auxiliaryMetadataEvents', 'unsupportedEvents', 'rejectedBeforeStart',
        'unmappedAgentModels', 'transportRejections', 'agentRegistrationsEvicted',
        'agentRegistrationsExpired', 'webSearchRequests', 'webSearchCalls', 'webSearchLinks'].map(key =>
        [key, counter(lifetime[key])])),
      firstRejectedCategory: FAILURE_DIAGNOSTIC_CATEGORIES.includes(lifetime.firstRejectedCategory)
        ? lifetime.firstRejectedCategory : null } : null,
    failureHistory: retained === null ? null : { retention: 'first-8-last-8-completed',
      omitted: failed === null ? null : Math.max(0, failed - retained.length), records: retained.map(projectRow) },
    recentRequests: rows.map(projectRow) };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { process.stdout.write(JSON.stringify(await readRequestStatus(process.env)) + '\n'); }
  catch { process.stderr.write('STATUS_UNAVAILABLE: Run inside the updated Clauduct session.\n'); process.exitCode = 1; }
}
