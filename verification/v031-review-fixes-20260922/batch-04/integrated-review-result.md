## Findings

### F1 — `hold:true` is overloaded: the TUI hook can only read it as "await children", including when the gateway knows nothing is outstanding

**Location:** `gateway/parent_wait.go:prepareParentWait` → `messages.go:relay` (mode selection) → `native-events.mjs` `turn.step`, the `if (state.mode==='sdk')` branch and the `held=true` fallthrough.

**Trigger:** TUI session (`state.mode==='native_tui'`); the last outstanding background task finishes; native wakes the root with a `task-notification` turn, so `turn.step` fires with `agent===''`, `e.index===0`, `eligible===true`. `readiness.Pending` is empty (that child was just delivered), so `prepareParentWait` returns the step via the `id == "" && step.Index == 0` arm, and `relay` takes the `else` branch → `ConsumeEmptyNotification()`. The model has nothing to add — exactly the case that control exists for — so `ReplyEmpty()` is true, `WaitingForChildren()` is true, and the relay writes `hold:true`.

The mjs then has only one behaviour for TUI: `held=true; p.phase='awaiting_children'; return {...result,answer:'',stopReason:null}`. The SDK branch above it ends the turn with an attributed status; TUI has no equivalent. The turn is parked in `awaiting_children` with no pending child, no workflow run and no notification left to produce another `turn.step`. The decision record carries `{turn,index,hold}` only, so the gateway cannot distinguish "wait" from "consume and finish" even though it computed `len(Pending)==0` to choose `ConsumeEmptyNotification` in the first place.

**Minimal falsifying check:** drive `relay` with `ParentReadiness{Pending: nil, ControlMode: "native_tui"}` and `scopes[0].parentWait = &parentStep{Agent:"", Turn:"t1", Index:0, Mode:"native_tui", Eligible:true}` over a backend stream that completes with no text and no tool calls; assert `decision-root.json` contains `"hold":true`. Then trace `turn.step` in the mjs with `state.mode==='native_tui'` and that file: it reaches `held=true`, never the `end_turn` return. The same input with `state.mode==='sdk'` returns `stopReason:'end_turn'` — the asymmetry is the defect.

---

### F2 — the 429/back-off status and the per-step execution ledger cancel each other out

**Location:** `gateway/request_replay.go:claimNativeExecution` / `release` / `rejectedBeforeDispatch`, against `messages.go:statusForUpstream`.

**Trigger:** any upstream failure that occurs *after* the transport call — most commonly `upstream.Failure{Category: RateLimited}` or `Disposition: Deferred`. `statusForUpstream` deliberately answers 429 "so a client that keeps asking recovers" / "the one status this client backs off from properly". The client backs off and resends the same conversation step. `rejectedBeforeDispatch` only un-dispatches `ErrNoTransport`, `ErrBudgetExhausted`, `ErrRouteNotAuthorised` ("prove no network request was made"), so a 429 leaves `dispatched==true`, `release()` does nothing, and the key stays in `seen`. The retry is answered `400 NATIVE_REQUEST_REPLAY_BLOCKED` and the turn ends.

The final change makes this unconditional rather than body-dependent: for a conversation request with a step present, `key.class, key.body = "conversation", [32]byte{}`, so a retry that re-serialises the body (refreshed system reminders, context hints) no longer differs in any field. Every retry of `(session, agent, turn, step)` is now blocked, which is precisely the client behaviour the 429 was chosen to provoke.

**Minimal falsifying check:** call `claimNativeExecution` twice with identical headers/body for a conversation request whose `step-*.json` is present; on the first, call `dispatch()` then `release()` (an upstream 429 after dispatch). Assert the second call returns `"NATIVE_REQUEST_REPLAY_BLOCKED"`, and pair it with `statusForUpstream(upstream.Failure{Category: upstream.RateLimited}) == 429`. Either the rate-limit class should not be 429 here, or `rejectedBeforeDispatch` needs an arm for upstream refusals that produced no assistant output.

---

### F3 — a withheld child reply is recorded as a *completed empty* answer

**Location:** `messages.go:relay`, the `scopes[0].parent != ""` deferred `finishAnswer(answer, relayCompleted)`, against `anthropic.(*Builder).Answer` and `results.go:deliver`.

**Trigger:** a child agent with its own pending grandchildren, TUI mode. `prepareParentWait` returns a step (`step.Eligible && len(readiness.Pending) > 0` with `id != ""`), `relay` selects `WaitForChildren()`, which withholds the reply *regardless of content* (`WaitingForChildren()` does not consult `ReplyEmpty()` when `waitChildren` is set). The relay still commits, so `relayCompleted == true`, and the deferred closure runs `finishAnswer(translator.Answer(), true)` — but `Builder.Answer()` short-circuits to `""` on `WaitingForChildren()`. So the child's real text exists in the builder and is recorded as an empty answer with the completed flag set, while the mjs has held the child's turn open (`stopReason:null`).

Downstream in `results.deliver`, an entry with `e.body == ""` and no cancellation/end evidence produces the `"child … has no acquired result body. Its completed result is unverified. Report 결과 미확보"` supplement to the parent — for a child that is merely waiting. The SDK variant diverges differently: native's transcript holds `"[Clauduct] Waiting for background task notification."` while the gateway recorded `""`.

**Minimal falsifying check:** relay a non-empty backend text stream with `scopes[0].parent != ""`, `ControlMode == "native_tui"`, `Pending` non-empty; assert `translator.Answer() == ""` while `translator.Builder().Text() != ""`, and that `finishAnswer` receives `("", true)`. The fix is to skip or pass `completed=false` when `Builder().WaitingForChildren()` is true, the same way the withheld state is already recorded on `ParentReadiness.Withheld`.

---

## Evidence gaps (not counted as findings)

- `dispatch()`'s call site is not in scope. F2 assumes it is invoked before the transport attempt, which is what `rejectedBeforeDispatch`'s "prove no network request was made" comment implies; if `dispatch()` is instead called only on a fully relayed response, F2 does not hold.
- `beginAnswer`/`finishAnswer`, `resultReported`, `deliverable`, and `agentResults.change` bodies are not supplied; F3's downstream consequence is inferred from the `e.body == ""` branch in the supplied `deliver`.
- `conversationRequest`, `readCurrentNativeTurn`, `validActiveReceipt`, the `delegationScope.parentWait` assignment, and the `resultsDelivered = relay(...)` assignment are not supplied. The separate concern that a one-shot cancellation/unavailable notice is cleared by `finish(true)` on a *withheld* (not failed) turn depends on those and on `resultReported("cancellation_reported")`; it is distinct from the refuted rollback point, but I could not close it from this scope.
- Native's engine semantics for `stopReason:null`, and the chunk vocabulary the SDK branch yields (`{kind:'text'}` then `{kind:'stop'}` with the entire prefix dropped, including whatever opens a stream), are not supplied. F1 rests on the plugin's own `awaiting_children` bookkeeping, not on external engine behaviour.
- `writeParentDecision` is reached by any conversation-class request that shares `step-root.json`; whether `conversationRequest` admits auxiliary traffic (the explicit `compaction` header test alongside it suggests it is class-agnostic) determines whether a concurrent auxiliary request can reset `hold` between the relay's write and the hook's read. Not resolvable here.

Tools were disabled for this review; I ran nothing and am reporting only what is derivable from the supplied text.