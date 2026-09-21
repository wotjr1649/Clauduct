# Verified counting and delegation changes — 2026-09-18

## Result

The subscription backend has a usable count path. A WebSocket `response.create`
with `generate:false` returns completed `usage.input_tokens` without generated output.
The old HTTP `/input_tokens` and `/count_tokens` probes did not establish this path.
Their failure did not prove counting impossible.

The production `/v1/messages/count_tokens` handler now uses:

1. A bounded local o200k text counter for the validated plain-text shape.
2. The existing subscription backend's WebSocket warmup for function tools/results,
   images, PDF, structured output and encrypted reasoning.
3. An explicit unsupported response for shapes outside the supported converter/scope,
   including the native server-hosted search request shape.

No separate API key, external count service, generated counting prompt, retry loop,
Node runtime or PowerShell runtime is added to the product. Warmup requests still
consume the local backend-attempt budget; they are not recorded as generated inferences.
Zero generated output is verified. This does **not** establish provider billing or quota
exemption for warmup input.

Generation and counting share child model/effort selection. Counting also applies the
same Agent schema adapter as generation. Count failures do not poison later generation.
Expected unsupported counts remain short summaries; other count failures record route,
stage and a fixed diagnostic category. Bodies and credentials are not written to status.

## Observed evidence

Environment: Windows amd64; Go 1.27.1; Claude Code 2.1.275; four configured model IDs.
Provider behavior and native behavior may change after these observations.

| Check | Observed result | Artifact |
|---|---|---|
| Local text formula, initial calibration and edges | 64/64 equal to generation `input_tokens` | `local-count-comparison.json`, `go-local-count-live.jsonl`, `go-local-count-edge-live.jsonl` |
| Independent text/conversation holdout, prediction before dispatch | 8/8 equal | `go-local-count-holdout-live.jsonl` |
| Warmup versus identical generation input: tools, tool results, PNG, JSON schema | 16/16 equal | `go-websocket-formats-live.jsonl` |
| One-page public PDF, all four models | 4/4 equal | `go-backend-count-pdf-live.jsonl` |
| Encrypted reasoning, all four models | 4/4 equal | `go-backend-count-reasoning-live-2.jsonl`, `go-backend-count-reasoning-live-4.jsonl` |
| Actual count-only boundary requests | 238999, 239001, 449999, 450001 all equal to local counts | `go-backend-count-boundaries-live.jsonl` |
| Native `/context all` protocol integration | 14 backend-counter calls; successful counts consumed; 0 generation calls | `go-native-context-counts.jsonl` |
| Unit/regression suite excluding app | 1028 distinct leaf tests passed | `go-count-selection-unit-complete.jsonl` |
| Selected native application regressions | 25 distinct leaf tests passed | `go-count-selection-native-complete.jsonl` |
| Packaging, dependency, source and documentation checks | 9 distinct leaf tests passed | `go-count-package-checks.jsonl` |
| Nested native delegation after fixing asynchronous fixture ordering | 3 consecutive passes | `go-selection-nested-event-order.jsonl` |
| Static and module checks | `go vet ./...`, `go mod verify` passed | command output in development session |

Native protocol tests use a synthetic backend; they do not establish tokenizer accuracy.
Paired live tests establish the accuracy observations separately. The boundary tests
generate no large answer and do not test semantic compaction quality. Finite observations
are not a proof covering every possible image, PDF, schema, tokenizer update or input.

Failed observations remain in their original logs. In particular:

- The first PowerShell probe emitted a non-JSON void result; suppressing that output
  fixed the harness, after which the server returned 21 input tokens and zero output.
- The first Go probe rejected valid metadata events. The fixed allowlist uses the
  Responses protocol's metadata and rate-limit events; unknown data/output still fails.
- The first reasoning probe incorrectly expected completed response items in the terminal
  event. Codex delivers items through `response.output_item.done`. Some trivial prompts
  produced no encrypted reasoning; an overly large arithmetic prompt exceeded its bound.
  Final successful probes use actual generated response items, never fabricated reasoning.
- The old WebSearch fixture called an undiscovered deferred tool. It now exercises
  `ToolSearch -> WebSearch -> native tool result`.
- The nested fixture sometimes returned a final parent answer before its background child
  ran. Its bounded event coordination now keeps the fixture open until the nested request.
  This is a harness fix, not a claim that application result-recovery policy is finished.

## Delegation behavior in this change

- Explicit native aliases and full GPT IDs follow the same model-first precedence.
- Model-only calls use that model's catalogue effort; effort-only built-in roles keep
  their role model.
- A task-bound explicit choice propagates to descendants; a conflicting descendant
  override is refused before dispatch.
- Call ID, native child ID, role, parent, session and alias are cross-checked.
- A bounded selection journal allows a fresh gateway object to recover a choice after
  rechecking native metadata. Gateway restart recovery is unit-tested; an actual native
  cross-process child resume is not yet proven.
- Status recent records now include child/parent IDs, role, selection verification and
  counted input tokens, beside the effective model, effort and source.
- Menu workers include `ToolSearch` so they can use native deferred tools.

## Context and performance limits still open

The screenshot's 400K is real: `go/internal/app/session.go` still supplies a common
400000 maximum, 320000 auto window and an 84.2105% override. Native model switching
does not replace that process-wide environment. The catalogue's 500K/450K and
272K/239K entries remain targets, **not enforced model policies**. Status continues
to say `not_enforced` / `unverified`.

The exact counter removes the previous counting blocker. It does not by itself complete
the gateway's per-agent trigger, one-compaction-at-a-time state, summary validation,
old-model-before-switch transition, or policy-failure closure. Those must be implemented
and verified before changing the common native envelope or claiming those windows work.

Observed Go backend counting took roughly 0.760–1.996 seconds in the small probes and
1.042–1.221 seconds at the selected large boundaries. The local 6.5 KiB text microbenchmark
was approximately 0.186–0.194 ms per operation. These are different workloads.
General generation does not acquire a new count preflight in this change.
Adding an exact backend count before every generated request would introduce a network
round trip; no evidence establishes zero latency impact. Connection reuse and prepared
response continuation need a separate paired performance experiment before such a claim.

Other still-open items: full custom-role default discovery; universal failure before
dispatch for unknown selections; actual native child resume; event-driven result-body
acquisition including `SubagentHandback`; no-rerun handling of a genuinely missing result;
native threshold/compaction quality and downgrade-transition acceptance tests. Existing
policy acceptance tests for unverified legacy agents are retained and still expose that gap.
The race detector remains unrun with the present CGO-disabled Windows toolchain.

## Primary references

- [OpenAI WebSocket mode](https://developers.openai.com/api/docs/guides/websocket-mode): `generate:false` prepares request state without model output.
- [OpenAI Codex Responses event handling](https://github.com/openai/codex/blob/main/codex-rs/codex-api/src/sse/responses.rs): metadata and completed usage handling.
- [Claude Code model configuration](https://code.claude.com/docs/en/model-config): unknown-model context settings and model-picker behavior.
- [Claude Code gateway protocol](https://code.claude.com/docs/en/llm-gateway-protocol): optional counting and client fallback.
- [Claude Code hook reference](https://code.claude.com/docs/en/hooks): background Agent result behavior and `SubagentHandback` report semantics.
- [Dependency decision](DEPENDENCIES.md): exact pins, boundary review and licences.
