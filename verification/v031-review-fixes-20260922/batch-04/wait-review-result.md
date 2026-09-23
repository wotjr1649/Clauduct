## Findings

### 1. `prepareParentWait` unconditionally downgrades a live `hold:true` decision for the *same* turn/index
**File:** `go/internal/gateway/parent_wait.go` — `prepareParentWait`, the `g.writeParentDecision(&step, false)` call (before `entry.checked("parent_input_snapshot")`).

**Trigger:** a second conversation-class request for the same session/agent admitted while the first request is inside its wait window — i.e. after `relay` (`messages.go`, the `translator.Builder().WaitingForChildren()` branch) has written `hold:true`, and before the plugin resolves the control at its first semantic chunk. Reachable cases: an `auxiliary`-class request that satisfies `conversationRequest` (only `compaction` is excluded here, and `relay`'s `beginAnswer` guard shows `auxiliary` requests do flow with a parent scope), or any second request that passes the replay ledger because its body hash differs (`request_replay.go` keys on `body`, so a different body at the same `step` is admitted).

**Mechanism:** the plugin rewrites `step-<name>.json` only at the next `turn.step`, so the second request re-reads the *unchanged* step (`Turn=T`, `Index=N`) and writes `decision-<name>.json` = `{"turn":T,"index":N,"hold":false}`. The plugin's staleness guard (`decision.turn!==turn || decision.index!==e.index`) therefore does *not* reject it; it accepts `hold:false`, yields the prefix, and returns the terminal empty result. The SDK parent ends the turn on the EMPTY_REPLY with children still live, and the native retry is then refused `NATIVE_REQUEST_REPLAY_BLOCKED` — the exact failure this change targets. Outcome is timing-dependent (if the reader wins the race the hold survives), which is why it will present as a flaky regression rather than a deterministic one.

Note the prepare-time `hold:false` write has no other effect: stale decisions from earlier turns/indexes are already rejected by the plugin's turn/index check, so the only behaviour unique to this write is the clobber. Suggested fix: don't write at prepare time when a decision for the same `(turn,index)` already exists, or carry a per-request nonce that both `relay` and the plugin agree on so a non-owning request cannot lower an owner's hold.

### 2. New `PARENT_WAIT_UNVERIFIED` refusal returns inside the window where the results-delivery reservation is unowned
**File:** `go/internal/gateway/messages.go`, the block

```
finishResults := g.deliverResults(r, request, entry)
parentWait, waitErr := g.prepareParentWait(r, request, entry)
if waitErr != nil { g.refuseCategory(w, http.StatusBadRequest, "PARENT_WAIT_UNVERIFIED"); return }
```
vs. `resultsDelivered := false; defer func() { finishResults(resultsDelivered) }()` ~20 lines later.

**Trigger:** `prepareParentWait` returns `errDelegationUnverified` — from `readNativeStep` (step session/agent mismatch, non-`correlationShape` turn, `Index` out of range, unknown `Mode`, or `Eligible` with `Mode=="unclassified"`, the case the new `"unclassified wait"` subtest exercises), or from the `!found && len(readiness.Pending) > 0` branch (no `step-<name>.json` yet for an identity that never emitted `turn.step`, e.g. a workflow-class conversation request, while a child is pending).

**Mechanism:** `deliverResults`/`deliverWithReadiness` has already run `d.results.deliver(request, session, parent, out)`, injecting the completed child report into `request.Messages` and handing back the commit/rollback callback. Returning here means `finishResults(false)` is never invoked, so the delivery is neither acknowledged nor rolled back for the refused request. Same-window returns already exist (`NATIVE_TURN_UNVERIFIED`, `AGENT_RESULT_CAPACITY`), but this path is new and is reachable from an ordinary missing-step race, not only from tampering. Move the `defer` immediately after `deliverResults`.

## Evidence gaps (explicit)

I ran nothing — no tools, no execution, no filesystem access. Nothing below is a claim of a defect, only scope I could not check:

- `bridge` builder semantics are outside the supplied scope: whether `WaitingForChildren()` becomes true at the first accepted event or only once the builder knows the reply is empty, and whether it suppresses frames until then. If it is a mode flag that is true from the first event while frames are emitted normally, a *non-empty* parent answer with pending children would get `hold:true` on disk and the plugin would throw `CLAUDUCT_PARENT_WAIT_CONTROL_INVALID` (native-events.mjs, `result.answer!==''` check). Neither supplied test covers pending-children + non-empty answer.
- `categoryFor`/`statusForUpstream` are not supplied, so the comment on `errParentWaitUnverified` ("returns 400 by name") is unverified; if there is no mapping, `fail(errParentWaitUnverified)` in `relay` still yields the 502/`UPSTREAM_FAILURE` classification the comment says it is avoiding.
- `results.deliver`'s rollback contract (what `finish(false)` restores) is not supplied; finding 2's severity depends on it.
- `record.snapshot()` copy depth and whether `ParentReadiness` is non-nil for SDK requests without delegations — if it is nil, `relay`'s `ControlMode == "sdk"` gate makes `DeferTextUntilComplete()` apply only to SDK turns that happen to have a readiness snapshot, so SDK output shape would differ between turns.