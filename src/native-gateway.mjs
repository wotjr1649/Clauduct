import { createServer } from 'node:http';
import { randomBytes, timingSafeEqual, createHmac } from 'node:crypto';
import { REQUEST_STAGES, FAILURE_DIAGNOSTIC_CATEGORIES, REQUEST_FAILURES, UPSTREAM_ERROR_CODES, UPSTREAM_ERROR_TYPES, UPSTREAM_INCOMPLETE_REASONS, KEEPALIVE_SHAPES } from './native-protocol.mjs';
import { prepareNative, createNativeResponse, prepareFileReview, prepareReviewContext, verifyFileReviewStep, NativeError, need, NATIVE_LIMITS, EVENT_DIAGNOSTIC_TYPES, EVENT_TYPE_FORMATS, UPSTREAM_FAILURES, capturableEventName } from './native-protocol.mjs';
import { MODELS, ROLE_MODELS, CONTEXT_POLICY, selectModel } from './models.mjs';
import { writeFrames } from './native-delivery.mjs';
import { createAdmission } from './request-admission.mjs';
import { betaFailure, unknownBetas, judgedBetas } from './native-beta.mjs';
import { SELECTION_FAILURES, SELECTION_IO_CODES, COMPLETION_FAILURES, COMPLETION_STATES } from './agent-selection.mjs';
import { searchSideQuery, searchRequestBody, searchReply } from './native-search.mjs';
import { installHttpClose } from './http-close.mjs';
import { transportClientErrorCode } from './native-protocol.mjs';

// Fixed-label diagnostics. A composed local code carries a variable suffix built from
// this project's own allowlists, never upstream text; keep only its fixed prefix.
function diagnosticCategory(code) {
  const fixed = typeof code !== 'string' ? 'OTHER'
    : code.startsWith('AGENT_SELECTION_UNVERIFIED') ? 'AGENT_SELECTION_UNVERIFIED'
    : code.startsWith('UNSUPPORTED_BETA ') ? 'UNSUPPORTED_BETA' : code;
  return FAILURE_DIAGNOSTIC_CATEGORIES.includes(fixed) ? fixed : 'OTHER';
}

// Matches the bounded pending/verified tables in agent selection.
const MAX_AGENTS = 1024;

export async function startNativeGateway({ transport, onUnregisteredAgent, onUnmappedAgentModel, onUnsupportedEventCapture, onWebSearchUnused,
  admissionOptions, agentSelection, cleanupMs = 2000, heartbeatMs = 15000, maxAgents = MAX_AGENTS, agentIdleMs = 1800000,
  injectStreamError = false, verificationSelection } = {}) {
  if (verificationSelection !== undefined) {
    need(verificationSelection?.model && verificationSelection?.effort, 'INVALID_GATEWAY_OPTIONS');
    verificationSelection = Object.freeze(selectModel(verificationSelection.model, verificationSelection.effort));
  }
  need(typeof transport?.send === 'function' && typeof transport?.close === 'function'
    && typeof transport?.diagnostics === 'function' && Number.isInteger(cleanupMs) && cleanupMs > 0 && cleanupMs <= 10000
    && Number.isInteger(heartbeatMs) && heartbeatMs >= 5 && heartbeatMs <= 60000
    && Number.isInteger(maxAgents) && maxAgents > 0 && maxAgents <= MAX_AGENTS
    && Number.isInteger(agentIdleMs) && agentIdleMs > 0, 'INVALID_GATEWAY_OPTIONS');
  const admission = createAdmission(admissionOptions);
  const secret = Buffer.from(`Bearer ${randomBytes(32).toString('base64url')}`), jobs = new Set(), sockets = new Set();
  const agents = new Map();
  const correlationKey = randomBytes(32), correlationScope = randomBytes(16).toString('hex');
  const reference = (kind, session, id) => session && (kind === 'session' || id)
    ? createHmac('sha256', correlationKey).update(JSON.stringify([kind, session, id ?? null])).digest('hex').slice(0, 32) : null;
  let closing = false, reason = 'NONE', requests = 0, rejected = 0, unregisteredAgentRequests = 0, port, finish, closingWork;
  let activeBodies = 0, activeDeliveries = 0, activeHeartbeats = 0, cleanupFailed = false;
  let pendingConnectionCloses = 0, connectionCloseTimeouts = 0;
  const connectionClosures = new WeakMap();
  const maxObservedInputTokens = { main: 0, subagent: 0 };
  // Fixed metadata only; bounded memory, no transcript, headers or credential material.
  const recentRequests = [], failureRequests = [];
  // At most four distinct unmapped upstream event names and eight unrecognised client beta
  // names, both kept for the exit diagnostic only.
  const unsupportedEventNames = [], unknownBetaNames = [], judgedBetaLabels = [];
  const copyRecord = record => ({ ...record, retryScheduledMs: [...record.retryScheduledMs],
    attempts: record.attempts.map(attempt => ({ ...attempt })),
    ...(record.agentContextPolicy && { agentContextPolicy: { ...record.agentContextPolicy } }),
    ...(record.compactShape && { compactShape: { ...record.compactShape } }) });
  // rejectedBeforeStart counts HTTP-boundary rejections that never became a request record:
  // another route, a local boundary or authorization rejection, a malformed identifier header
  // or an agent binding failure. Inference requests are counted by started/succeeded/failed.
  const lifetime = { started: 0, succeeded: 0, failed: 0, auxiliaryMetadataEvents: 0, keepaliveEvents: 0, unsupportedEvents: 0, unsupportedEventNamesWithheld: 0, injectedStreamErrors: 0,
    rejectedBeforeStart: 0, firstRejectedCategory: null, unmappedAgentModels: 0,
    transportRejections: 0, transportRejectedConnections: 0, agentRegistrationsEvicted: 0, agentRegistrationsExpired: 0,
    webSearchRequests: 0, webSearchCalls: 0, webSearchLinks: 0 };
  let notifiedUnmappedModel = false, notifiedEventCapture = false, notifiedWebSearchUnused = false;
  const failuresByStage = Object.fromEntries(REQUEST_STAGES.map(stage => [stage, 0]));
  // rejectedBeforeStart counts every pre-timing refusal but firstRejectedCategory keeps only
  // one label, so a long run reports a number nothing can be read out of. Keys come from
  // diagnosticCategory, which answers inside a fixed vocabulary, so this cannot grow unbounded.
  const rejectedCategories = {};
  const transportRejectionsByEvent = {}, transportClientErrorsByCode = {};
  // Count distinct sockets without retaining closed connections or their identifying data.
  const transportRejectedSockets = new WeakSet();
  // Verification only, and off unless the launcher was told which fallback arm is running.
  // One synthetic upstream error per gateway, after content has already reached the client,
  // because that is the shape the client's stream-failure path actually sees. Never an idle
  // timeout: the client disables fallback for those on a separate term, which would decide
  // the experiment before the setting under test got a say. Hooked to onEvent only, so a
  // transport that returns its events as an array instead of streaming them is not injected.
  let streamErrorInjected = false;
  const done = new Promise(resolve => { finish = resolve; });
  const diagnostics = () => ({ closing, reason, activeSockets: sockets.size, activeJobs: jobs.size,
    activeTimers: activeBodies + activeHeartbeats + pendingConnectionCloses + Number(admission.diagnostics().timerActive), activeBodies,
    pendingConnectionCloses, connectionCloseTimeouts,
    activeDeliveries, cleanupFailed, admission: admission.diagnostics(), maxObservedInputTokens: { ...maxObservedInputTokens },
    contextPolicy: CONTEXT_POLICY, contextPolicyRuntimeVerified: false,
    correlationScope, lifetime: { ...lifetime, failuresByStage: { ...failuresByStage },
      rejectedCategories: { ...rejectedCategories }, transportRejectionsByEvent: { ...transportRejectionsByEvent },
      transportClientErrorsByCode: { ...transportClientErrorsByCode } },
    recentRequests: recentRequests.map(copyRecord),
    failureRequests: failureRequests.map(copyRecord),
    unsupportedEventNames: [...unsupportedEventNames], unknownBetaNames: [...unknownBetaNames],
    judgedBetaLabels: [...judgedBetaLabels],
    busy: jobs.size > 0, transport: transport.diagnostics(), requests, rejected,
    persistedBodies: 0, registeredAgents: agents.size, maxAgents, unregisteredAgentRequests, sessionLifetime: null, requestBudget: null });
  function authorized(value) {
    return typeof value === 'string' && Buffer.byteLength(value) === secret.length && timingSafeEqual(Buffer.from(value), secret);
  }
  function reply(res, status, value) {
    res.writeHead(status, { 'Content-Type': 'application/json', 'Cache-Control': 'no-store', Connection: 'close' });
    res.end(JSON.stringify(value));
  }
  async function readBody(req, limit, controller) {
    need(!controller.signal.aborted, 'CANCELLED');
    activeBodies++;
    const abort = () => {
      // An incomplete/cancelled request must reset promptly, rather than use the
      // graceful response-close path and leave a half-written client waiting.
      if (req.socket && !req.socket.destroyed) req.socket.resetAndDestroy();
      req.destroy(controller.signal.reason instanceof NativeError
        ? controller.signal.reason : new NativeError('CANCELLED'));
    };
    controller.signal.addEventListener('abort', abort, { once: true });
    const timer = setTimeout(() => controller.abort(new NativeError('REQUEST_TIMEOUT')), 300000);
    try {
      let bytes = 0, raw = ''; const decoder = new TextDecoder('utf-8', { fatal: true });
      for await (const chunk of req) {
        bytes += chunk.length; need(bytes <= limit, 'INPUT_TOO_LARGE'); raw += decoder.decode(chunk, { stream: true });
      }
      raw += decoder.decode();
      try { return JSON.parse(raw); } catch { throw new NativeError('INVALID_JSON'); }
    } finally { clearTimeout(timer); controller.signal.removeEventListener('abort', abort); activeBodies--; }
  }
  async function handle(req, res, controller) {
    requests++;
    let release, activeAgent, selectionRequest, upstream = false, timing, heartbeat, heartbeatPending = false, responseStarted = false;
    let writeTail = Promise.resolve(), deliveryError, stage = 'request';
    const stopHeartbeat = () => { if (heartbeat) { clearInterval(heartbeat); heartbeat = undefined; activeHeartbeats--; } };
    const started = performance.now();
    const elapsed = () => Math.round((performance.now() - started) * 100) / 100;
    const abort = () => { if (!res.writableFinished) {
      if (timing && !closing && !controller.signal.aborted) {
        timing.clientDisconnected = true; timing.clientDisconnectedMs = elapsed();
      }
      controller.abort();
    } };
    res.once('close', abort); req.on('error', abort); req.once('aborted', abort); res.on('error', abort);
    try {
      const names = req.rawHeaders.filter((_, i) => i % 2 === 0).map(name => name.toLowerCase());
      need(!closing && new Set(names).size === names.length, 'INVALID_HEADER');
      need(req.socket.remoteAddress === '127.0.0.1' && req.headers.host === `127.0.0.1:${port}`
        && req.headers.origin === undefined && req.headers['sec-fetch-site'] === undefined
        && req.headers.forwarded === undefined && !names.some(name => name.startsWith('x-forwarded-')), 'LOCAL_BOUNDARY_REJECTED');
      need(!['cookie', 'x-api-key', 'proxy-authorization'].some(name => names.includes(name)), 'UNEXPECTED_CREDENTIAL_SOURCE');
      const path = req.url.split('?')[0];
      if (req.method === 'HEAD' && path === '/api/hello') {
        need(req.headers.authorization === undefined || authorized(req.headers.authorization), 'LOCAL_SESSION_REQUIRED');
        res.writeHead(204, { Connection: 'close' }); res.end(); return;
      }
      need(authorized(req.headers.authorization), 'LOCAL_SESSION_REQUIRED');
      if (req.method === 'GET' && path === '/v1/models') {
        reply(res, 200, { object: 'list', data: Object.values(MODELS).map(item => ({ id: item.model, object: 'model', owned_by: 'openai' })) }); return;
      }
      if (req.method === 'GET' && path === '/clauduct/status') {
        // The captured upstream name is withheld here so it never enters model context.
        const { unsupportedEventNames: withheldEvents, unknownBetaNames: withheldBetas, ...state } = diagnostics();
        void withheldEvents; void withheldBetas; reply(res, 200, state); return;
      }
      if (req.method === 'POST' && path === '/clauduct/agents') {
        const binding = await readBody(req, 4096, controller);
        if (binding?.kind === 'task-result') {
          need(agentSelection && Object.keys(binding).every(key => ['kind', 'sessionId', 'toolUseId', 'id', 'status', 'parent'].includes(key))
            && ['sessionId', 'toolUseId', 'id'].every(key => typeof binding[key] === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding[key]))
            && ['completed', 'failed'].includes(binding.status)
            && (binding.parent === undefined || (typeof binding.parent === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding.parent))), 'INVALID_AGENT_BINDING');
          let linked;
          try { linked = await agentSelection.linkTaskResult(binding, controller.signal); }
          catch { throw new NativeError('AGENT_SELECTION_UNVERIFIED_CALL'); }
          reply(res, 200, { linked }); return;
        }
        if (binding?.kind === 'workflow-result') {
          need(agentSelection && Object.keys(binding).every(key => ['kind', 'sessionId', 'toolUseId', 'taskId', 'runId',
            'workflowName', 'transcriptPath', 'transcriptDir', 'scriptPath', 'scriptDigest', 'resumeFromRunId', 'resumedScriptDigest', 'parent'].includes(key))
            && ['sessionId', 'toolUseId', 'taskId', 'runId', 'workflowName'].every(key => typeof binding[key] === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding[key]))
            && ['transcriptPath', 'transcriptDir', 'scriptPath'].every(key => typeof binding[key] === 'string' && binding[key].length <= 4096)
            && (binding.resumeFromRunId === undefined ? binding.resumedScriptDigest === undefined && typeof binding.scriptDigest === 'string' && /^[a-f0-9]{64}$/.test(binding.scriptDigest)
              : binding.scriptDigest === undefined && typeof binding.resumeFromRunId === 'string' && binding.resumeFromRunId === binding.runId
                && typeof binding.resumedScriptDigest === 'string' && /^[a-f0-9]{64}$/.test(binding.resumedScriptDigest))
            && (binding.parent === undefined || (typeof binding.parent === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding.parent))), 'INVALID_AGENT_BINDING');
          try { await agentSelection.linkWorkflow(binding, controller.signal); } catch { throw new NativeError('AGENT_SELECTION_UNVERIFIED_CALL'); }
          reply(res, 200, { linked: true }); return;
        }
        if (binding?.kind === 'resume-result') {
          need(agentSelection && Object.keys(binding).every(key => ['kind', 'sessionId', 'toolUseId', 'id', 'parent'].includes(key))
            && ['sessionId', 'toolUseId', 'id'].every(key => typeof binding[key] === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding[key]))
            && (binding.parent === undefined || (typeof binding.parent === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding.parent))), 'INVALID_AGENT_BINDING');
          let linked;
          try { linked = agentSelection.linkResume(binding); } catch { throw new NativeError('AGENT_SELECTION_UNVERIFIED_CALL'); }
          reply(res, 200, { linked }); return;
        }
        if (binding?.kind === 'skill-result') {
          need(agentSelection && Object.keys(binding).every(key => ['kind', 'sessionId', 'toolUseId', 'id', 'skill', 'parent'].includes(key))
            && ['sessionId', 'toolUseId', 'id'].every(key => typeof binding[key] === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding[key]))
            && (binding.parent === undefined || (typeof binding.parent === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding.parent)))
            && typeof binding.skill === 'string' && binding.skill.length > 0 && binding.skill.length <= 200, 'INVALID_AGENT_BINDING');
          try { agentSelection.linkSkill(binding); } catch { throw new NativeError('AGENT_SELECTION_UNVERIFIED_CALL'); }
          reply(res, 200, { linked: true }); return;
        }
        need(binding && Object.keys(binding).every(key => ['id', 'role', 'stop', 'contextPolicy', 'sessionId', 'transcriptPath'].includes(key))
          && typeof binding.id === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding.id)
          && typeof binding.role === 'string' && binding.role.length > 0 && binding.role.length <= 200
          && typeof binding.stop === 'boolean'
          && (binding.sessionId === undefined || (typeof binding.sessionId === 'string' && /^[A-Za-z0-9_-]{1,200}$/.test(binding.sessionId)))
          && (binding.transcriptPath === undefined || (typeof binding.transcriptPath === 'string' && binding.transcriptPath.length <= 4096)), 'INVALID_AGENT_BINDING');
        const context = binding.contextPolicy;
        need(context === undefined || (context && typeof context === 'object' && !Array.isArray(context)
          && Object.keys(context).length === 3 && Object.keys(context).every(key => ['window', 'autoCompactWindow', 'compactPercent'].includes(key))
          && Number.isSafeInteger(context.window) && context.window > 0
          && Number.isSafeInteger(context.autoCompactWindow) && context.autoCompactWindow > 0
          && Number.isFinite(context.compactPercent) && context.compactPercent > 0 && context.compactPercent <= 100), 'INVALID_AGENT_BINDING');
        need(!agents.has(binding.id) || agents.get(binding.id).role === binding.role, 'AGENT_BINDING_CONFLICT');
        // Registrations are removed by SubagentStop. For a session where that never arrives,
        // drop idle entries that have gone unused past the window. No timer: the sweep runs
        // here, and a registration with live requests is never a candidate.
        if (!binding.stop) {
          const stale = Date.now() - agentIdleMs;
          for (const [id, state] of agents) {
            if (id === binding.id || state.requests.size > 0 || state.lastUsedMs > stale) continue;
            state.selectionController?.abort();
            agents.delete(id);
            lifetime.agentRegistrationsExpired++;
          }
        }
        // The cap is the backstop when even the window has not released enough entries.
        if (!binding.stop && !agents.has(binding.id) && agents.size >= maxAgents) {
          const idle = [...agents].find(([, state]) => state.requests.size === 0);
          need(idle !== undefined, 'AGENT_BINDING_LIMIT');
          idle[1].selectionController?.abort();
          agents.delete(idle[0]);
          lifetime.agentRegistrationsEvicted++;
        }
        agents.get(binding.id)?.selectionController?.abort();
        // Stop or replacement invalidates active requests of this registration only.
        for (const active of agents.get(binding.id)?.requests ?? []) active.abort(new NativeError('CANCELLED'));
        if (binding.stop) {
          agents.delete(binding.id);
        }
        else {
          const state = { role: binding.role, requests: new Set(), lastUsedMs: Date.now(),
            contextPolicy: context ?? null, selectionPending: Boolean(agentSelection),
            selectionBinding: agentSelection ? { ...binding, nativeRegistered: true } : undefined, selectionController: agentSelection ? new AbortController() : undefined };
          agents.set(binding.id, state);
          // Native can persist the sidecar only after this hook returns. Resolve
          // on the first child request, never while blocking SubagentStart.
        }
        reply(res, 200, { registered: !binding.stop }); return;
      }
      need(req.method === 'POST' && path === '/v1/messages', 'UNSUPPORTED_ROUTE');
      for (const key of ['x-claude-code-session-id', 'x-claude-code-agent-id', 'x-claude-code-parent-agent-id']) {
        need(req.headers[key] === undefined || /^[A-Za-z0-9_-]{1,200}$/.test(req.headers[key]), 'INVALID_SESSION_ID');
      }
      // Do not decode or retain a large request body while waiting for memory.
      timing = { request: requests, startedAt: new Date().toISOString(), model: null, effort: null,
        sessionRef: reference('session', req.headers['x-claude-code-session-id']),
        agentRef: reference('agent', req.headers['x-claude-code-session-id'], req.headers['x-claude-code-agent-id']),
        parentRef: reference('agent', req.headers['x-claude-code-session-id'], req.headers['x-claude-code-parent-agent-id']),
        subagent: req.headers['x-claude-code-agent-id'] !== undefined,
        admissionStartedMs: elapsed(), admittedMs: null, preparedMs: null, transportStartedMs: null,
        firstEventMs: null, firstTextDeltaMs: null, firstDownstreamWriteMs: null,
        transportFinishedMs: null, finishedMs: null, retryScheduledMs: [], attempts: [], success: false };
      recentRequests.push(timing);
      lifetime.started++;
      if (recentRequests.length > 16) recentRequests.shift();
      // Record the inference request before header compatibility checks so a rejected
      // version, beta or encoding is a diagnosed failure instead of an unrecorded 400.
      need(req.headers['anthropic-version'] === '2023-06-01', 'UNSUPPORTED_VERSION');
      const betaError = betaFailure(req.headers['anthropic-beta']);
      if (betaError) throw new NativeError(betaError);
      for (const name of unknownBetas(req.headers['anthropic-beta'])) {
        if (unknownBetaNames.length < 8 && !unknownBetaNames.includes(name)) unknownBetaNames.push(name);
      }
      // Fixed labels, so unlike the raw names these stay available inside the session too.
      timing.judgedBetaLabels = judgedBetas(req.headers['anthropic-beta']);
      for (const label of timing.judgedBetaLabels) {
        if (!judgedBetaLabels.includes(label)) judgedBetaLabels.push(label);
      }
      need((req.headers['content-type'] ?? '').split(';')[0].trim().toLowerCase() === 'application/json'
        && (!req.headers['content-encoding'] || req.headers['content-encoding'] === 'identity'), 'UNSUPPORTED_ENCODING');
      // Pin the registration before either admission or body reads can yield.
      const agent = req.headers['x-claude-code-agent-id'];
      const agentBinding = agents.get(agent), role = agentBinding?.role;
      if (agentBinding) { agentBinding.lastUsedMs = Date.now(); agents.delete(agent); agents.set(agent, agentBinding); }
      activeAgent = agentBinding;
      activeAgent?.requests.add(controller);
      release = await admission.acquire(controller.signal);
      timing.admittedMs = elapsed();
      let doc = await readBody(req, NATIVE_LIMITS.requestBytes, controller);
      need(!controller.signal.aborted, 'CANCELLED');
      timing.requestedModel = Object.values(MODELS).find(item => item.model === doc?.model)?.model
        ?? (typeof doc?.model === 'string' && Object.hasOwn(MODELS, doc.model) ? MODELS[doc.model].model : null);
      stage = 'selection';
      if (agentSelection && agent !== undefined) {
        if (agentBinding?.selectionPending) {
          agentBinding.selectionWork ??= agentSelection.resolve({ ...agentBinding.selectionBinding,
            requestedModel: doc.model, requestedEffort: doc.output_config?.effort }, agentBinding.selectionController.signal).then(selection => {
            agentBinding.selection = selection;
            agentBinding.selectionPending = false;
            agentBinding.selectionBinding = undefined;
          }).catch(error => {
            agentBinding.selectionFailure = SELECTION_FAILURES.includes(error.selectionReason) ? error.selectionReason : 'UNKNOWN';
            throw Object.assign(new NativeError(`AGENT_SELECTION_UNVERIFIED_${agentBinding.selectionFailure}`), {
              selectionReason: agentBinding.selectionFailure,
              selectionIoCode: SELECTION_IO_CODES.includes(error.selectionIoCode) ? error.selectionIoCode : null,
              completionFailure: COMPLETION_FAILURES.includes(error.completionFailure) ? error.completionFailure : null,
              completionParentState: COMPLETION_STATES.includes(error.completionParentState) ? error.completionParentState : null,
              completionChildState: COMPLETION_STATES.includes(error.completionChildState) ? error.completionChildState : null
            });
          });
          await agentBinding.selectionWork;
          need(!controller.signal.aborted, 'CANCELLED');
        }
        need(agentBinding && !agentBinding.selectionPending,
          `AGENT_SELECTION_UNVERIFIED_${agentBinding?.selectionFailure ?? (agentBinding ? 'PENDING' : 'UNREGISTERED')}`);
        need(agentBinding.selection.sessionId === req.headers['x-claude-code-session-id']
          && agentBinding.selection.parent === req.headers['x-claude-code-parent-agent-id'], 'AGENT_SELECTION_UNVERIFIED');
      }
      selectionRequest = agentSelection?.begin(req.headers['x-claude-code-session-id'], agent);
      const route = agentSelection ? agentBinding?.selection?.route : Object.hasOwn(ROLE_MODELS, role) ? ROLE_MODELS[role] : undefined;
      stage = 'prepare';
      // The client's search side query is a server tool call addressed to this gateway, not a
      // model request. Detected before preparation so the model path stays untouched; anything
      // that is not exactly the side query the client sends falls through to it.
      const searchRequest = searchSideQuery(doc);
      const prepared = prepareNative(doc, { subagent: agent !== undefined, route, search: searchRequest !== null,
        verificationSelection,
        turnToolChanges: req.headers['anthropic-beta']?.split(',').some(value => value.trim() === 'mid-conversation-tool-changes-2026-07-01') });
      stage = 'review';
      prepareReviewContext(prepared, agentBinding?.selection);
      if (agentBinding?.selection?.review) prepareFileReview(prepared, doc);
      doc = undefined;
      timing.selectionSource = agentBinding?.selection?.source ?? null;
      timing.model = prepared.selected.model; timing.effort = prepared.selected.effort; timing.preparedMs = elapsed();
      timing.purpose = prepared.purpose; timing.requestedEffort = prepared.requestedEffort;
      timing.compactShape = prepared.compactShape;
      timing.role = Object.hasOwn(ROLE_MODELS, role) || ['claude', 'workflow-subagent'].includes(role) ? role : null;
      timing.roleRegistered = agent !== undefined && agents.has(agent);
      timing.agentContextPolicy = agentBinding?.contextPolicy ? { ...agentBinding.contextPolicy } : null;
      // Fixed booleans/counts only: whether the client asked for backend search and how many
      // searches the backend actually ran. Neither carries a query or a result.
      timing.webSearchRequested = prepared.webSearch === true;
      timing.webSearchAnswered = searchRequest !== null;
      // Counted where the request is shaped, not where it succeeds: a search request that
      // fails upstream is exactly the one the lifetime totals have to show.
      if (timing.webSearchRequested) lifetime.webSearchRequests++;
      // Registration is routing metadata, not authorization. Never bypass native tool/permission hooks.
      if (agent !== undefined && !agents.has(agent)) {
        unregisteredAgentRequests++;
        if (unregisteredAgentRequests === 1) onUnregisteredAgent?.();
      }
      upstream = true;
      const responseOptions = { deferText: agentBinding?.selection?.source === 'workflow-result' };
      let output;
      let response = createNativeResponse(prepared, responseOptions);
      const validateResponse = (phase, action) => {
        need(!controller.signal.aborted, 'CANCELLED');
        try { return action(); }
        catch (error) {
          if (error instanceof NativeError && error.code === 'SNAPSHOT_MISMATCH' && timing.snapshotMismatchMs === undefined) {
            timing.snapshotMismatchMs = elapsed(); timing.snapshotMismatchPhase = phase;
            timing.snapshotMismatchEvent = EVENT_DIAGNOSTIC_TYPES.includes(error.snapshotMismatchEvent) ? error.snapshotMismatchEvent : null;
          }
          throw error;
        }
      };
      const emit = (frames, ping = false) => {
        if (!frames.length) return Promise.resolve();
        const work = writeTail.then(async () => {
        if (controller.signal.aborted) throw new NativeError('CANCELLED');
        if (!ping) { responseStarted = true; timing.firstDownstreamWriteMs ??= elapsed(); }
        if (!res.headersSent) res.writeHead(200, { 'Content-Type': 'text/event-stream; charset=utf-8', 'Cache-Control': 'no-store', Connection: 'close' });
        activeDeliveries++;
        try {
          await writeFrames(res, frames, controller.signal);
          if (ping) { timing.pingCount = (timing.pingCount ?? 0) + 1; timing.lastPingMs = elapsed(); }
        }
        finally { activeDeliveries--; }
        });
        writeTail = work.catch(error => { deliveryError = error; controller.abort(error); });
        return work;
      };
      timing.transportStartedMs = elapsed();
      stage = 'upstream';
      if (searchRequest) {
        // No model turn: one JSON round trip to the backend's own search endpoint, then the
        // blocks the client reduces. Nothing from this conversation travels with the query.
        const found = await transport.search(
          searchRequestBody(null, prepared.selected.model, searchRequest), controller.signal);
        timing.transportFinishedMs = elapsed();
        need(!controller.signal.aborted, 'CANCELLED');
        stage = 'output-validation';
        output = searchReply(prepared.selected.model, searchRequest, found);
        timing.webSearchCalls = output.searchCalls;
        timing.webSearchLinks = output.links;
      } else {
        const pushEvent = async event => {
          timing.firstEventMs ??= elapsed();
          timing.lastUpstreamEventMs = elapsed();
          if (event.type === 'response.output_text.delta') timing.firstTextDeltaMs ??= elapsed();
          await emit(validateResponse('stream', () => response.push(event)));
          if (event.type === 'codex.response.metadata') {
            timing.auxiliaryMetadataEvents = (timing.auxiliaryMetadataEvents ?? 0) + 1;
            lifetime.auxiliaryMetadataEvents++;
          }
          if (event.type === 'keepalive') {
            timing.keepaliveEvents = (timing.keepaliveEvents ?? 0) + 1;
            lifetime.keepaliveEvents++;
          }
        };
        const legacyEvents = await transport.send(prepared.body, controller.signal, {
          attemptTimings: timing.attempts,
          onEvent: async event => {
            await pushEvent(event);
            if (injectStreamError && !streamErrorInjected && responseStarted) {
              streamErrorInjected = true;
              lifetime.injectedStreamErrors++;
              await pushEvent({ type: 'error', error: { type: 'api_error', code: 'server_error' } });
            }
            if (!heartbeat) {
              activeHeartbeats++;
              heartbeat = setInterval(() => {
                if (heartbeatPending || controller.signal.aborted || res.writableEnded) return;
                heartbeatPending = true;
                void emit([{ type: 'ping' }], true).then(() => { heartbeatPending = false; }, error => {
                  heartbeatPending = false; deliveryError = error; controller.abort(error);
                });
              }, heartbeatMs);
            }
          },
          canRetry: () => !responseStarted && !controller.signal.aborted,
          onRetry: () => {
            need(!responseStarted, 'RETRY_AFTER_OUTPUT');
            if (timing.retryScheduledMs.length < 5) timing.retryScheduledMs.push(elapsed());
            response = createNativeResponse(prepared, responseOptions);
          }
        });
        timing.transportFinishedMs = elapsed();
        stopHeartbeat(); await writeTail;
        need(!controller.signal.aborted, 'CANCELLED');
        // Existing injected offline transports can still return the old event-array contract.
        if (Array.isArray(legacyEvents)) for (const event of legacyEvents) await pushEvent(event);
        output = validateResponse('final', () => response.finish());
        stage = 'output-validation';
      }
      // Fixed block kinds and counts, never their content. The client reads only the first
      // block in some paths, so record which kind that was.
      const blocks = output.message.content;
      const blockCount = kind => blocks.filter(block => block.type === kind).length;
      timing.contentBlocks = { text: blockCount('text'), toolUse: blockCount('tool_use'),
        thinking: blockCount('redacted_thinking'),
        // A search reply is three blocks; counting only the first three kinds reported one.
        serverToolUse: blockCount('server_tool_use'), searchResult: blockCount('web_search_tool_result') };
      timing.firstContentBlock = blocks[0]?.type ?? null;
      verifyFileReviewStep(output.message, prepared);
      // Recording routing evidence is atomic; report why it was refused instead of a
      // generic protocol rejection, so the failed turn names the unverified selection.
      let unmappedModels = 0;
      try { unmappedModels = agentSelection?.remember(output.message, req.headers['x-claude-code-session-id'], agent, prepared.selected) ?? 0; }
      catch (error) {
        const reason = SELECTION_FAILURES.includes(error?.selectionReason) ? error.selectionReason : 'UNKNOWN';
        throw Object.assign(new NativeError(`AGENT_SELECTION_UNVERIFIED_${reason}`), { selectionReason: reason });
      }
      if (unmappedModels > 0) {
        lifetime.unmappedAgentModels += unmappedModels;
        if (!notifiedUnmappedModel) { notifiedUnmappedModel = true; onUnmappedAgentModel?.(); }
      }
      const kind = agent === undefined ? 'main' : 'subagent';
      maxObservedInputTokens[kind] = Math.max(maxObservedInputTokens[kind],
        output.message.usage.input_tokens + (output.message.usage.cache_read_input_tokens ?? 0));
      stage = 'delivery';
      await emit(output.frames);
      res.end();
      timing.webSearchCalls ??= response.webSearchCalls?.() ?? 0;
      timing.success = true;
      if (timing.webSearchRequested) {
        lifetime.webSearchCalls += timing.webSearchCalls;
        lifetime.webSearchLinks += timing.webSearchLinks ?? 0;
        // The gateway answers a search side query itself, so a request that carried the search
        // tool and was not answered here is one detection did not recognise: it went to the
        // model with the tool dropped and came back without results. Nothing failed, yet the
        // feature did nothing. Say so once, and say which case it is.
        if (timing.webSearchCalls === 0 && !notifiedWebSearchUnused) {
          notifiedWebSearchUnused = true; onWebSearchUnused?.();
        }
      }
      agentSelection?.delivered(req.headers['x-claude-code-session-id'], agent, selectionRequest, output.message);
    } catch (error) {
      stopHeartbeat(); await writeTail;
      error = deliveryError ?? error;
      rejected++;
      // Only locally constructed fixed categories cross this diagnostic boundary.
      const category = error instanceof NativeError ? error.code : upstream ? 'PROTOCOL_REJECTED' : 'INVALID_REQUEST';
      if (!controller.signal.aborted && !closing && !['CANCELLED', 'CLIENT_DISCONNECTED'].includes(category)) {
        agentSelection?.failed?.(req.headers['x-claude-code-session-id'], req.headers['x-claude-code-agent-id'], selectionRequest);
      }
      if (!timing) {
        lifetime.rejectedBeforeStart++;
        const rejectedLabel = diagnosticCategory(category);
        lifetime.firstRejectedCategory ??= rejectedLabel;
        rejectedCategories[rejectedLabel] = (rejectedCategories[rejectedLabel] ?? 0) + 1;
      }
      const requestFailure = stage === 'prepare' && category === 'UNSUPPORTED_REQUEST'
        && REQUEST_FAILURES.includes(error.requestFailure) ? error.requestFailure : null;
      const eventName = category === 'UNSUPPORTED_EVENT' ? capturableEventName(error.eventTypeName) : null;
      // An empty capture list must not read the same as "no unsupported event happened".
      // Without this counter a withheld name is indistinguishable from nothing to withhold.
      if (timing && category === 'UNSUPPORTED_EVENT') {
        lifetime.unsupportedEvents++;
        if (!eventName) lifetime.unsupportedEventNamesWithheld++;
      }
      if (eventName && unsupportedEventNames.length < 4 && !unsupportedEventNames.includes(eventName)) {
        unsupportedEventNames.push(eventName);
        if (!notifiedEventCapture) { notifiedEventCapture = true; onUnsupportedEventCapture?.(); }
      }
      const eventKind = category === 'UNSUPPORTED_EVENT' && EVENT_DIAGNOSTIC_TYPES.includes(error.eventKind) ? error.eventKind : null;
      const eventTypeFormat = category === 'UNSUPPORTED_EVENT' && EVENT_TYPE_FORMATS.includes(error.eventTypeFormat) ? error.eventTypeFormat : null;
      const upstreamFailureEvent = Object.keys(UPSTREAM_FAILURES).find(type => UPSTREAM_FAILURES[type] === category) ?? null;
      const upstreamErrorCode = upstreamFailureEvent && UPSTREAM_ERROR_CODES.includes(error.upstreamErrorCode) ? error.upstreamErrorCode : null;
      const upstreamErrorType = upstreamFailureEvent && UPSTREAM_ERROR_TYPES.includes(error.upstreamErrorType) ? error.upstreamErrorType : null;
      const upstreamIncompleteReason = upstreamFailureEvent === 'response.incomplete'
        && UPSTREAM_INCOMPLETE_REASONS.includes(error.upstreamIncompleteReason) ? error.upstreamIncompleteReason : null;
      const completionFailure = COMPLETION_FAILURES.includes(error.completionFailure) ? error.completionFailure : null;
      const parentState = COMPLETION_STATES.includes(error.completionParentState) ? error.completionParentState : null;
      const childState = COMPLETION_STATES.includes(error.completionChildState) ? error.completionChildState : null;
      if (timing) timing.unsupportedEvent = eventKind;
      if (timing) timing.unsupportedEventTypeFormat = eventTypeFormat;
      if (timing) timing.keepaliveShape = KEEPALIVE_SHAPES.includes(error.keepaliveShape) ? error.keepaliveShape : null;
      if (timing) {
        timing.upstreamFailureEvent = upstreamFailureEvent;
        timing.upstreamErrorCode = upstreamErrorCode;
        timing.upstreamErrorType = upstreamErrorType;
        timing.upstreamIncompleteReason = upstreamIncompleteReason;
        timing.failureStage = stage;
        timing.requestFailure = requestFailure;
        timing.selectionFailure = SELECTION_FAILURES.includes(error.selectionReason) ? error.selectionReason : null;
        timing.selectionIoCode = SELECTION_IO_CODES.includes(error.selectionIoCode) ? error.selectionIoCode : null;
        timing.completionFailure = completionFailure;
        timing.completionParentState = parentState;
        timing.completionChildState = childState;
      }
      if (timing) timing.reviewDiffMismatch = category === 'REVIEW_DIFF_REQUIRED'
        && ['call-count', 'tool-name', 'command', 'background'].includes(error.reviewDiffMismatch) ? error.reviewDiffMismatch : null;
      if (timing) timing.failureCategory = diagnosticCategory(category);
      const retryAtMs = category === 'UPSTREAM_RETRY_DEFERRED' && Number.isSafeInteger(error.retryAtMs)
        && error.retryAtMs >= 0 ? error.retryAtMs : null;
      if (timing) timing.retryAtMs = retryAtMs;
      const relogin = ['UNAUTHENTICATED', 'CREDENTIAL_UNAVAILABLE_OR_EXPIRED', 'CREDENTIAL_ACCOUNT_CHANGED', 'CREDENTIAL_ACCOUNT_MISMATCH', 'CODEX_RELOGIN_REQUIRED'].includes(category);
      const status = category === 'LOCAL_SESSION_REQUIRED' ? 401 : category === 'RATE_LIMITED' || category === 'UPSTREAM_RETRY_DEFERRED' ? 429
        : category === 'MEMORY_QUEUE_FULL' || category === 'MEMORY_ADMISSION_TIMEOUT' || relogin ? 503 : upstream ? 502 : 400;
      const failure = { type: 'error', error: { type: status === 429 ? 'rate_limit_error' : status >= 500 ? 'api_error' : 'invalid_request_error',
        message: category + (eventKind ? ` event=${eventKind}` : '')
          + (eventTypeFormat ? ` event_type_format=${eventTypeFormat}` : '')
          + (upstreamFailureEvent ? ` event=${upstreamFailureEvent}` : '')
          + (upstreamErrorCode ? ` upstream_code=${upstreamErrorCode}` : '')
          + (upstreamErrorType ? ` upstream_type=${upstreamErrorType}` : '')
          + (upstreamIncompleteReason ? ` incomplete_reason=${upstreamIncompleteReason}` : '')
          + (requestFailure ? ` request=${requestFailure}` : '')
          + (retryAtMs !== null ? ` retry_at_ms=${retryAtMs}; resume no earlier than this time.` : '')
          + (category === 'UPSTREAM_RETRY_UNREPRESENTABLE' ? ': Server retry deadline exceeds the supported numeric range; automatic retry is blocked.' : '')
          + (category === 'MEMORY_ADMISSION_TIMEOUT' ? ': Memory admission timed out before upstream execution; retry when capacity recovers.' : '')
          + (completionFailure ? ` completion=${completionFailure} parent=${parentState ?? 'NONE'} child=${childState ?? 'NONE'}` : '')
          + (relogin ? ': Codex login required; resume after logging in.' : res.headersSent ? ': Partial response; explicit resume required.' : '') } };
      if (!res.destroyed && !res.headersSent) reply(res, status, failure);
      else if (!res.destroyed && !controller.signal.aborted) {
        try { await writeFrames(res, [failure], controller.signal); res.end(); }
        catch { res.destroy(); }
      } else {
        if (res.socket && !res.socket.destroyed) res.socket.resetAndDestroy();
        res.destroy();
      }
    } finally {
      stopHeartbeat();
      activeAgent?.requests.delete(controller);
      if (timing) {
        timing.finishedMs = elapsed(); lifetime[timing.success ? 'succeeded' : 'failed']++;
        if (!timing.success) {
          failuresByStage[stage]++;
          failureRequests.push(copyRecord(timing));
          // Pin the first eight completed failures; retain the latest eight thereafter.
          if (failureRequests.length > 16) failureRequests.splice(8, 1);
        }
      }
      release?.(); res.removeListener('close', abort);
    }
  }
  // Body watchdog begins on admission, so queued requests do not expire while waiting for memory.
  const server = createServer({ maxHeaderSize: 16384, requestTimeout: 0, headersTimeout: 60000 }, (req, res) => {
    const controller = new AbortController(); const job = { controller };
    job.finished = handle(req, res, controller).finally(() => jobs.delete(job)); jobs.add(job);
  });
  server.on('connection', socket => {
    sockets.add(socket); socket.once('close', () => sockets.delete(socket)); socket.on('error', () => {});
    connectionClosures.set(socket, installHttpClose(socket, { onPending: change => { pendingConnectionCloses += change; },
      onTimeout: () => { connectionCloseTimeouts++; } }));
    if (closing) socket.resetAndDestroy();
  });
  // Event count, not failed inference requests: clientError can also follow a partial request.
  // Only fixed call-site labels and allowlisted codes survive. Never retain the error/request.
  const transportReject = (event, socket, errorCode) => {
    rejected++; lifetime.transportRejections++;
    transportRejectionsByEvent[event] = (transportRejectionsByEvent[event] ?? 0) + 1;
    if (!transportRejectedSockets.has(socket)) {
      transportRejectedSockets.add(socket); lifetime.transportRejectedConnections++;
    }
    if (event === 'clientError') {
      const code = transportClientErrorCode(errorCode);
      transportClientErrorsByCode[code] = (transportClientErrorsByCode[code] ?? 0) + 1;
    }
  };
  server.on('clientError', (error, socket) => { transportReject('clientError', socket, error.code); socket.destroy(); });
  for (const event of ['connect', 'upgrade']) server.on(event, (_req, socket) => { transportReject(event, socket); socket.destroy(); });
  for (const event of ['checkContinue', 'checkExpectation']) server.on(event, (_req, res) => {
    transportReject(event, _req.socket); reply(res, 417, { type: 'error', error: { type: 'invalid_request_error', message: 'EXPECT_REJECTED' } });
  });
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
  port = server.address().port;
  function close(why = 'CLIENT_CLOSED') {
    if (closingWork) return closingWork;
    closing = true; reason = why; secret.fill(0); correlationKey.fill(0);
    for (const state of agents.values()) state.selectionController?.abort();
    agents.clear(); admission.close();
    closingWork = (async () => {
      for (const job of jobs) job.controller.abort();
      const socketsDone = [...sockets].map(socket => new Promise(resolve => {
        socket.once('close', resolve);
        if (!connectionClosures.get(socket)?.pending) socket.resetAndDestroy();
      }));
      // server.close() itself destroys idle HTTP sockets. Let already-finished
      // responses drain first; the closing flag rejects newly accepted peers.
      const serverDone = Promise.all(socketsDone).then(() => new Promise(resolve => server.close(resolve)));
      let timer;
      const work = Promise.allSettled([Promise.resolve().then(() => transport.close()),
        ...[...jobs].map(job => job.finished), ...socketsDone, serverDone]);
      const results = await Promise.race([work, new Promise(resolve => { timer = setTimeout(() => resolve(null), cleanupMs); })]);
      clearTimeout(timer);
      if (results === null || results.some(result => result.status === 'rejected')) { cleanupFailed = true; reason = 'CLEANUP_FAILED'; }
      const state = diagnostics(); finish(state); return state;
    })();
    return closingWork;
  }
  server.on('error', () => { void close('SERVER_ERROR'); });
  return { port, close, done, diagnostics, clientHeaders() { need(!closing, 'GATEWAY_CLOSED'); return { Authorization: secret.toString() }; } };
}
