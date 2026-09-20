# S46 accepted accounting / compaction policy — 2026-09-20

Status: accepted by the user: replace exact pre-execution threshold blocking with
measured usage recording and preventive compaction. Implementation and verification
are recorded separately; a policy decision alone is not a released binary.

## Recommendation

Separate three contracts: completed backend usage, predictive context management,
and explicit exact-count requests. Ordinary generation must not depend on an
undocumented warmup response being an exact tokenizer for every future input shape.

- Record backend input/output usage as reported, per request and agent. Missing usage
  after cancellation remains unknown, never zero. Do not add cached input to its parent
  input count or reasoning output to its parent output count a second time.
- Keep cumulative consumption separate from current retained context. Summing all
  request input counts double-counts repeatedly sent history. A previous response's
  total does not exactly measure a new prompt, new files, pruning or changed tools.
- Use the last measured usage as an anchor and cheap bounded estimation of retained
  changes for preventive compaction. Estimate validity is explicit. An estimate is
  neither an exact-count API answer nor proof of a hard admission cap.
- Keep the configured 500K/450K and 272K/239K values as context-management targets.
  This policy does NOT guarantee every unseen input is stopped just before the
  target threshold or a configured window. A large new input can cross it once.
  The backend's actual request limit is a separate limit, not necessarily these values.
- Do not compact for a changed model name alone. Reassess the destination policy,
  retain history, and use the preceding resolved model/effort when compaction is needed.
  A model change also invalidates any claim that the old measured count is exact for
  the new model. Supported per-agent route and capability checks remain mandatory.
- On a confirmed context-length rejection before any completed operation, allow only
  the validated native compaction/recovery path with a finite retry bound. Never infer
  that generic 400, timeout or disconnect means an oversized context. Never silently
  truncate history or replay an operation whose completion is uncertain.
- `/v1/messages/count_tokens` remains independent: return an exact count only for
  a verified model/input/method combination. Expected unsupported count requests do
  not cancel generation. The known tool-output image mismatch is not supported.
- Future official subscription count/compaction capabilities may replace the adapter
  after conformance tests. Public API documentation is not proof that the subscription
  backend or native Claude Code supports that path.
- Keep one reviewed model catalogue in source, as previously requested. New model
  admission verifies route, effort, usage schema, context targets and the capabilities
  being enabled. Do not copy a previous tokenizer's constants based on a family name.
  Do not require an exact-count capability to claim independently verified generation.

The user explicitly withdrew the old exact pre-execution threshold guarantee.
It cannot be promised at the same time as ordinary post-generation-only measurement
when a new unmeasured payload is accepted. The accepted predictive
management can compact early or occasionally encounter a backend overflow; it must
not be advertised as an exact bound.

## Implemented algorithm and limits

Ordinary generation and compaction do not call `InputCounter.Count`. The text-only
preventive estimate is UTF-8 bytes plus small framing allowances, divided by three.
Images, files and opaque reasoning are marked unestimated; base64 size is not a token
count. With a previous usage anchor, the signal is the greater of the current text
estimate and `previous input + previous output + current text estimate - previous
text estimate`. This reserves output conservatively and can compact early. It is
not a calibrated tokenizer or a strict upper/lower bound.

The anchor belongs to a session/agent, persists only numeric usage and routing, and
does not apply to auxiliary title/classifier calls. A successful compact discards the
old full-history anchor; the next completed generation establishes the new one.
Version-1 journals migrate with unknown usage rather than invented counts.

Only structured `context_length_exceeded` before delivery authorizes bounded native
overflow recovery. A second overflow after compaction fails explicitly. Generic HTTP
errors and uncertain tool completion never authorize replay. A new model's catalogue
entry must explicitly enable independently verified counting; routing alone does not.

An exact count already cached for identical request bytes may be compared with usage.
A mismatch invalidates the optional counter but preserves the valid generation.
Actual usage can answer an identical explicit count request from the bounded two-minute
cache. A concurrent older warmup cannot overwrite this measurement. `/context` may
still spend time on its own explicit count requests; this policy removes generation
preflight latency, not every backend or native UI delay.

## Source basis

- [Responses usage and truncation](https://developers.openai.com/api/reference/cli/resources/responses/methods/retrieve):
  usage reports input/output; disabled truncation rejects oversized requests rather
  than removing the oldest input. Subscription behavior still needs local validation.
- [Token counting](https://developers.openai.com/api/docs/guides/token-counting): local
  text tokenization does not cover arbitrary image/file/tool framing; output counts
  include non-visible tokens. Exact input_tokens is a separate endpoint.
- [Codex configuration](https://learn.chatgpt.com/docs/config-file/config-reference):
  context window and auto-compaction threshold are separate model configuration fields.
  This does not prove Codex implementation uses this proposed Clauduct algorithm.
- [Claude Code context management](https://code.claude.com/docs/en/how-claude-code-works):
  compaction is preventive and bounded against thrashing on excessive individual inputs.
- [OpenAI compaction](https://developers.openai.com/api/docs/guides/compaction): a server
  can compact at rendered token thresholds and return opaque compaction items. That
  would also require preserving those items across the native client, not only adding
  a request flag. It is not enabled or claimed supported by this policy.

## Independent filter-on investigation

The user explicitly reported re-enabling AdGuard. The existing same-script Node test
reproduced raw TCP errors 9/40 and ordinary HTTP error 1/40; Expect changed 3/3.
Its public counters are preserved in
`../release-investigation-20260920/socket-1789883256791.json`.

A stricter new Go-only public loopback probe checks both the exact response body and
completion while shutting down immediately after the handler is released. Previous
experiments sometimes checked only whether the client returned. Neither content
length nor positive linger fixes all cases in this stronger trial:

| Variant | Failed / 200 | Delayed over 200 ms | Total ms |
|---|---:|---:|---:|
| chunked | 40 | 2 | 2671 |
| Content-Length | 18 | 0 | 1677 |
| chunked + linger 1s | 25 | 1 | 19476 |
| Content-Length + linger 1s | 49 | 3 | 11530 |

Raw evidence: `socket-1789883422010.jsonl`. A follow-up with fixed response-shape
diagnostics is preserved in `socket-1789883571751.jsonl`; it is not a retry to replace
the failure. Request-level errors occurred before a usable HTTP response. Exact OS
error classification and an actual product-lifecycle remedy remain unfinished.
No linger, filter exception, global setting or product-source change was applied.
These counterexamples are not a final real-TUI verdict, and do not justify a new
claim that AdGuard-enabled operation is fully verified. v0.3.0 remains HOLD.

S46 follow-up: immediate close failed 41/200 with Winsock errno 10054; a control
that consumed the response before closing succeeded 200/200. The actual candidate
TUI also completed under the enabled filter. Scope, remaining limits and final
development-binary delivery are recorded in [REPORT.md](REPORT.md).
