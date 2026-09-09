import { request } from 'node:http';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { MODELS, EFFORTS } from './models.mjs';
import { contextFromEnvironment } from './agent-route.mjs';
import { EVENT_DIAGNOSTIC_TYPES } from './native-protocol.mjs';
import { SELECTION_FAILURES, SELECTION_IO_CODES } from './agent-selection.mjs';

const times = ['admissionStartedMs', 'admittedMs', 'preparedMs', 'transportStartedMs', 'firstEventMs',
  'firstTextDeltaMs', 'firstDownstreamWriteMs', 'transportFinishedMs', 'finishedMs'];
const number = value => typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : null;

// User-operated within the native child. Credentials go only to its fixed loopback endpoint.
export async function readRequestStatus(env) {
  const base = env.ANTHROPIC_BASE_URL, token = env.ANTHROPIC_AUTH_TOKEN;
  if (typeof base !== 'string' || !/^http:\/\/127\.0\.0\.1:[0-9]{1,5}$/.test(base)
    || typeof token !== 'string' || !/^[A-Za-z0-9_-]{43}$/.test(token)) throw new Error('STATUS_UNAVAILABLE');
  const rows = await new Promise((done, fail) => {
    const req = request(`${base}/clauduct/status`, { agent: false, signal: AbortSignal.timeout(5000),
      headers: { Authorization: `Bearer ${token}` } }, res => {
      if (res.statusCode !== 200) { res.destroy(); fail(new Error('STATUS_UNAVAILABLE')); return; }
      let raw = '', bytes = 0;
      res.setEncoding('utf8');
      res.on('data', chunk => {
        bytes += Buffer.byteLength(chunk);
        if (bytes > 128 * 1024) { res.destroy(); fail(new Error('STATUS_UNAVAILABLE')); return; }
        raw += chunk;
      });
      res.on('error', fail);
      res.on('end', () => {
        try {
          const value = JSON.parse(raw);
          if (!res.complete || !Array.isArray(value.recentRequests)) throw new Error('STATUS_UNAVAILABLE');
          done(value.recentRequests.slice(-16));
        } catch { fail(new Error('STATUS_UNAVAILABLE')); }
      });
    });
    req.on('error', fail); req.end();
  });
  return { clientContextPolicy: { evidence: 'inherited-environment',
    ...(contextFromEnvironment(env) ?? { window: null, autoCompactWindow: null, compactPercent: null }) },
    recentRequests: rows.map(row => ({ request: number(row?.request),
    startedAt: typeof row?.startedAt === 'string' && /^\d{4}-\d{2}-\d{2}T[\d:.]+Z$/.test(row.startedAt) ? row.startedAt : null,
    model: Object.values(MODELS).some(model => model.model === row?.model) ? row.model : null,
    requestedModel: Object.values(MODELS).some(model => model.model === row?.requestedModel) ? row.requestedModel : null,
    effort: EFFORTS.includes(row?.effort) ? row.effort : null,
    requestedEffort: EFFORTS.includes(row?.requestedEffort) ? row.requestedEffort : null,
    selectionSource: ['explicit-metadata', 'role-default', 'native-inherit', 'skill-result', 'verified-resume', 'verified-peer-resume', 'native-fork'].includes(row?.selectionSource) ? row.selectionSource : null,
    purpose: ['compact-template', 'conversation'].includes(row?.purpose) ? row.purpose : null,
    compactShape: row?.compactShape ? {
      lastRole: ['user', 'assistant', 'other'].includes(row.compactShape.lastRole) ? row.compactShape.lastRole : 'other',
      textBlocks: number(row.compactShape.textBlocks), mixedBlocks: row.compactShape.mixedBlocks === true,
      prefixMatches: row.compactShape.prefixMatches === true, suffixMatches: row.compactShape.suffixMatches === true,
      matches: row.compactShape.matches === true } : null,
    role: ['Explore', 'Plan', 'general-purpose'].includes(row?.role) ? row.role : null,
    roleRegistered: row?.roleRegistered === true,
    agentContextPolicy: row?.agentContextPolicy ? { evidence: 'subagent-start-hook-environment',
      window: number(row.agentContextPolicy.window), autoCompactWindow: number(row.agentContextPolicy.autoCompactWindow),
      compactPercent: number(row.agentContextPolicy.compactPercent) } : null,
    subagent: row?.subagent === true, success: row?.success === true,
    unsupportedEvent: EVENT_DIAGNOSTIC_TYPES.includes(row?.unsupportedEvent) ? row.unsupportedEvent : null,
    failureStage: ['request', 'selection', 'prepare', 'review', 'upstream', 'output-validation', 'delivery'].includes(row?.failureStage) ? row.failureStage : null,
    selectionFailure: SELECTION_FAILURES.includes(row?.selectionFailure) ? row.selectionFailure : null,
    selectionIoCode: SELECTION_IO_CODES.includes(row?.selectionIoCode) ? row.selectionIoCode : null,
    reviewDiffMismatch: ['call-count', 'tool-name', 'command', 'background'].includes(row?.reviewDiffMismatch) ? row.reviewDiffMismatch : null,
    failureCategory: ['CANCELLED', 'CLIENT_DISCONNECTED', 'UPSTREAM_IDLE_TIMEOUT', 'UPSTREAM_IO_ERROR',
      'DELIVERY_TIMEOUT', 'UNSUPPORTED_EVENT', 'REVIEW_DIFF_FAILED', 'REVIEW_DIFF_REQUIRED', 'OTHER'].includes(row?.failureCategory) ? row.failureCategory : null,
    clientDisconnected: row?.clientDisconnected === true,
    lastUpstreamEventMs: number(row?.lastUpstreamEventMs),
    pingCount: number(row?.pingCount) ?? 0, lastPingMs: number(row?.lastPingMs),
    auxiliaryMetadataEvents: number(row?.auxiliaryMetadataEvents) ?? 0,
    ...Object.fromEntries(times.map(key => [key, number(row?.[key])])),
    retryScheduledMs: Array.isArray(row?.retryScheduledMs) ? row.retryScheduledMs.slice(0, 5).map(number) : [],
    attempts: Array.isArray(row?.attempts) ? row.attempts.slice(0, 6).map(attempt => ({
      ...Object.fromEntries(['attempt', 'startedMs', 'requestFlushedMs', 'headersMs', 'firstBodyMs', 'endedMs', 'status']
        .map(key => [key, number(attempt?.[key])])), completed: attempt?.completed === true })) : [] })) };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try { process.stdout.write(JSON.stringify(await readRequestStatus(process.env)) + '\n'); }
  catch { process.stderr.write('STATUS_UNAVAILABLE: Run inside the updated Clauduct session.\n'); process.exitCode = 1; }
}
