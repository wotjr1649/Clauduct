# A2 — 세션·턴·복구 분야 리뷰

- 대상 루트: `D:\AIDEV\clauduct-v031` (git worktree)
- 기준 commit: `149068edd693fb860a03244a2ea15764bcd68c34` (v0.3.0), HEAD 동일. v0.3.1 변경 전체는 미커밋 작업트리.
- 환경: Windows 11 Pro 26200 / Go 1.27.1 / `CGO_ENABLED=0` (race 전용 명령만 `CGO_ENABLED=1 CC=C:/msys64/ucrt64/bin/gcc.exe`)
- 작업 디렉터리: Go 검사는 `/d/AIDEV/clauduct-v031/go`, JS 검사는 `/d/AIDEV/clauduct-v031/go/internal/app`
- 제품 소스·테스트·문서·기존 `verification/` 파일은 수정하지 않았다. 재현은 전부 `go test -overlay`로 주입했고
  기준 비교는 `git show <base>:<path>`로 꺼낸 사본을 overlay로 올려서 했다. 원본을 고쳤다가 되돌린 곳은 없다.
- `git add|commit|stash|checkout|reset|restore|clean|rebase`를 쓰지 않았다. 읽기 전용 git 명령만 사용했다.

---

## 1. 실행 근거 — `/code-review`

### 1.1 호출 원문

Skill 도구 호출:

- `skill` = `code-review`
- `args` = `xhigh go/internal/app/native-events.mjs go/cmd/clauduct-hook/ go/internal/gateway/native_events.go go/internal/gateway/selection_records.go go/internal/gateway/continuation.go go/internal/gateway/results.go go/internal/gateway/resume.go go/internal/gateway/projects.go go/internal/gateway/workflow.go go/internal/gateway/workflow_checkpoint.go go/internal/gateway/workflow_recovery.go`

`ultra`, `--fix`, `--comment`는 쓰지 않았다. 추가 서브에이전트도 띄우지 않았다.

### 1.2 로드 여부와 스킬 종류

- **로드됨.** 반환 첫 줄은 `Skill "code-review" completed (forked execution).` 이고, JSON 배열로 **13건**의 지적을 돌려줬다.
- **이 세션에서 실행된 것은 Claude Code CLI 내장 `code-review` 스킬이다.** 같은 이름의 marketplace 플러그인
  `claude-plugins-official/code-review` 와 `pr-review-toolkit` 은 **설치되어 있지 않다.**
  확인 방법과 결과: `C:\Users\js\.claude\plugins\installed_plugins.json` 의 `plugins` 키는 7개
  (`claude-mem@thedotmack`, `codex@openai-codex`, `gopls-lsp@claude-plugins-official`, `ponytail@ponytail`,
  `skill-creator@claude-plugins-official`, `superpowers@superpowers-marketplace`, `typescript-lsp@claude-plugins-official`)
  뿐이고 `code-review` / `pr-review` 로 시작하는 항목은 없다.

### 1.3 관측 가능한 effort / 모델 표기

**관측되지 않았다 (`NOT_OBSERVABLE`).** 인자로 `xhigh` 를 요청했지만 반환 payload 어디에도 effort 라벨이나
모델 ID 문자열이 없었다. 스킬은 대신 "full 10-angle pass plus sweep" 수행과 자체 검증 실행
(`go build ./...`, 3개 패키지 `go test`, `node native_events_publication_test.mjs`)을 산문으로 보고했다.
effort/모델이 실제 xhigh였다는 기계적 증거는 없다.

### 1.4 스킬 커버 범위 vs 직접 리뷰 범위

스킬이 실제로 지적을 낸 파일 (6개): `native_events.go`, `results.go`, `workflow.go`, `projects.go`,
`native-events.mjs`, `cmd/clauduct-hook/main.go`.

인자로 줬으나 **스킬이 한 건도 지적하지 않은 파일** (5개): `selection_records.go`, `continuation.go`, `resume.go`,
`workflow_checkpoint.go`, `workflow_recovery.go`. 읽었는지 확인할 수 없으므로 미커버로 간주하고 직접 읽었다.

인자에 없던 담당 파일 = **전부 직접 리뷰** (22개): 테스트 13개(`main_test.go`,
`native_events_publication_test.mjs`, app `native_events_test.go`, `session_registration_test.go`,
`subagent_test.go`, `absent_tree_test.go`, `continuation_test.go`, `late_receipt_test.go`,
gateway `native_events_test.go`, `projects_root_test.go`, `result_restart_test.go`, `turn_order_test.go`,
`turn_receipt_test.go`), 제품 3개(`progress.go`, `workflow_plan.go`, `workflow_results.go`), 검증 자료 6개.

스킬이 낸 13건은 전부 코드와 대조했다. 처리 결과: **재현으로 승격 2건**(A2-01, A2-02),
**개선으로 강등 6건**(A2-03, A2-05, A2-06, A2-07, A2-10, A2-12), **보류 3건**(A2-08, A2-09, A2-13),
**반박 1건**(R-01), **다른 지적과 병합 1건**(finding 8 → A2-04).
스킬이 놓쳤고 직접 찾은 신규 지적: **A2-11**(테스트 단언 삭제) 및 A2-01의 삭제된 회귀 테스트 추적.

---

## 2. 커버리지 표 (담당 33개 전부)

| # | 파일 | 검토 방식 |
|---|---|---|
| 1 | `go/cmd/clauduct-hook/main.go` | diff + 전체 읽음(1-230, 290-379). 스킬 지적 1건 검증 |
| 2 | `go/cmd/clauduct-hook/main_test.go` | diff 전체 읽음. 신규 `TestPromptSubmissionRegistersSessionWithoutForwardingPrompt` 단언 검토 + 실행 |
| 3 | `go/internal/app/native-events.mjs` | diff + 전체 읽음(215줄). 스킬 지적 2건 검증 |
| 4 | `go/internal/app/native_events_publication_test.mjs` | 전체 읽음(50줄) + 직접 실행 |
| 5 | `go/internal/app/native_events_test.go` | diff 전체 읽음 |
| 6 | `go/internal/app/session_registration_test.go` | 전체 읽음(127줄) + 직접 실행(PASS) |
| 7 | `go/internal/app/subagent_test.go` | diff + 주변 전체 읽음(61-185). 삭제 단언 재주입 실험 |
| 8 | `go/internal/gateway/absent_tree_test.go` | diff 전체 읽음. 삭제된 `TestAReceiptThatMovedOnIsNotTakenAsCurrent` 추적 |
| 9 | `go/internal/gateway/continuation.go` | diff + 전체 읽음(50줄) |
| 10 | `go/internal/gateway/continuation_test.go` | diff 전체 읽음 + 실행 |
| 11 | `go/internal/gateway/late_receipt_test.go` | diff 전체 읽음 + 실행 |
| 12 | `go/internal/gateway/native_events.go` | diff + 전체 읽음(390줄). 핵심 분석 대상 |
| 13 | `go/internal/gateway/native_events_test.go` | diff 전체 읽음 + 실행 |
| 14 | `go/internal/gateway/progress.go` | diff 전체 읽음 + `readCurrentNativeTurn` 연동 확인 |
| 15 | `go/internal/gateway/projects.go` | 신규 파일 전체 읽음(46줄) + 14개 호출 지점 전수 확인 |
| 16 | `go/internal/gateway/projects_root_test.go` | 전체 읽음(187줄) + 실행 |
| 17 | `go/internal/gateway/result_restart_test.go` | 전체 읽음(69줄) + 실행 |
| 18 | `go/internal/gateway/results.go` | diff + 전체 읽음(1-360, 430-569). 핵심 분석 대상 |
| 19 | `go/internal/gateway/resume.go` | diff 전체 읽음 |
| 20 | `go/internal/gateway/selection_records.go` | diff 전체 읽음 + `roleMatches` 의미 대조 |
| 21 | `go/internal/gateway/turn_order_test.go` | diff 전체 읽음 + 실행 |
| 22 | `go/internal/gateway/turn_receipt_test.go` | 신규 파일 전체 읽음(205줄) + 실행 |
| 23 | `go/internal/gateway/workflow.go` | diff + `findWorkflow` 240-325 읽음 |
| 24 | `go/internal/gateway/workflow_checkpoint.go` | diff 읽음(`openProjects` 치환 3곳) |
| 25 | `go/internal/gateway/workflow_plan.go` | diff 읽음(치환 1곳) |
| 26 | `go/internal/gateway/workflow_recovery.go` | diff 읽음(치환 1곳 + import 정리) |
| 27 | `go/internal/gateway/workflow_results.go` | diff 읽음(치환 1곳 + import 정리) |
| 28 | `verification/v031-events-20260921/REPORT.md` | 전문 읽음. 주장과 코드 대조 |
| 29 | `verification/v031-events-20260921/mutations.json` | 11개 mutation 이름·대상 테스트·검출 여부 전수 확인 |
| 30 | `verification/v031-projects-20260921/REPORT.md` | 전문 읽음 |
| 31 | `verification/v031-projects-20260921/mutations.json` | 8개 mutation 전수 확인 |
| 32 | `verification/v031-session-20260921/REPORT.md` | 전문 읽음 |
| 33 | `verification/v031-session-20260921/mutations.json` | 스키마(다른 형식) 확인 + 항목 확인 |

**미검토 없음.**

호출자·피호출자로 범위를 넓혀 함께 읽은 비담당 파일(지적 근거로만 사용):
`messages.go`, `delegation.go`, `agents.go`, `context.go`, `context_journal.go`, `context_display.go`,
`native_cancellation.go`, `gateway.go`, `diagnostics.go`, `app/settings.go`, `app/user_settings.go`,
`app/run.go`, `protocol/bridge/route.go`.

---

## 3. 지적 목록

### A2-01 (confirmed / high) — 고정된 옛 턴 영수증이 더 새로운 `awaiting_children` 항목을 파괴한다

- **파일·줄**: `go/internal/gateway/native_events.go:245-272` (파괴 지점 `:269`).
  삭제된 보호막: 기준 버전의 `stillOnTurn`(`git show 149068e:go/internal/gateway/native_events.go` 132-146)과
  `recordFailedAgentRequest` 안의 재조회.
- **발생 조건**: 요청 A가 `agentSelection`(`messages.go:285-295`)에서 자식 X의 턴 T1을 `record.nativeTurn`에 고정한다.
  A가 `beginResult`를 타지 않는 요청(`messages.go:159`의 조건에서 제외되는 compaction, 비-conversation 요청)이거나
  선택 이후 오래 살아 있는 동안, X가 다음 턴 T2를 돌고 비동기 자식을 남긴 채 끝나
  결과 항목이 `NativeTurn="T2", NativeEndObserved=true, EndReason="answer", State="awaiting_children"`이 된다.
  그 뒤 A가 `CANCELLED` 이외의 category로 실패한다.
- **원인**: v0.3.1은 실패 기록을 "요청이 접수된 턴"에 붙이려고 영수증을 요청 시작 시점에 고정했다. 방향은 옳다.
  그러나 같은 변경이 **`begin()` 직전의 재검증까지 함께 삭제**했다. `recordFailedAgentRequest`는
  `same := e.NativeTurn == active.Turn` 이 false일 때 `waiting`(= `awaiting_children`)만 확인하고
  `r.begin(id)`로 넘어간다. `:265`의 검사 목록(binding·metadata·role·model·parent·StoppedByUser)에는
  **턴이 한 번도 등장하지 않는다.** 그래서 고정된 턴이 항목의 현재 턴보다 **오래된** 경우를 걸러낼 수단이 없다.
  `applyNativeTurn`의 `e.NativeTurn != receipt.Turn` 가드(`:201`)는 바로 앞의 `begin()`이
  `NativeTurn=""`인 새 항목을 깔아버리기 때문에 무력화된다. 턴 ID는 불투명 문자열이라 순서 비교도 불가능하다.
- **영향**:
  1. T2의 `awaiting_children` 항목이 archive 없이 map에서 사라진다(`results.go:180-185`의 non-stopped 분기는
     archive하지 않는다). 부모에게 전달되지 못한 보고 본문이 버려진다.
  2. 새 항목에 이미 끝난 T1이 찍힌다. `reconcileNativeResults`는 `end-X-T1.json`을 찾는데 그 영수증은 T1 정산 때
     이미 소비·삭제됐으므로(`native_events.go:344-348`) 이 항목은 `FinalizeNativeResults`의
     `session_ended_unverified`까지 미결로 남는다.
  3. **이어가기가 끊긴다.** `continuation.go:39`는
     `State=="awaiting_children" && NativeEndObserved && EndReason=="answer" && NativeTurn!="" && NativeTurn!=active.Turn`
     를 요구한다. 위 파괴로 네 조건이 동시에 무너져 깨어난 coordinator 자식은 `AGENT_SELECTION_UNVERIFIED`를 받는다.
     삭제된 주석이 예고한 바로 그 실패다.
- **재현 명령** (작업 디렉터리 `/d/AIDEV/clauduct-v031/go`, 환경 `CGO_ENABLED=0`):
  ```
  CGO_ENABLED=0 go test ./internal/gateway/ \
    -overlay /d/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A2-session-turn-recovery/overlay.json \
    -run 'TestA2StalePinnedTurnDestroysNewerAwaitingChildren' -count=1 -timeout 300s -v
  ```
- **예상 결과**: 고정된 옛 턴은 더 새로운 `awaiting_children` 증거를 건드리지 못하고, 실패 category만 기록되거나 조용히 포기한다.
- **실제 결과** (exit 1, FAIL):
  ```
  after stale failure: entry=0x3dbd7bb0b60 live=0x3dbd7bb09c0 state="running" nativeTurn="first"
    endObserved=false endReason="" requestFailure="EMPTY_REPLY" body="" r.bytes=0
  stale pinned turn "first" replaced the live turn "second" and discarded the awaiting_children entry
  ```
  항목 포인터가 교체됐고, 턴이 `second` → `first`로 되돌아갔으며, `NativeEndObserved`/`EndReason`/본문이 전부 사라졌다.
- **증거 경로**:
  `/d/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A2-session-turn-recovery/run-a2-01-02-current.txt`
  (소스: 같은 디렉터리의 `a2_repro_test.go`, `overlay.json`)
- **신규회귀/기존결함**: **신규 회귀.** 기준 버전의 `recordFailedAgentRequest`는 deferred 실행 시점에
  `active-<id>.json`을 **새로 읽어서** `active`를 얻었으므로 이 시나리오에서 `active.Turn=="second"`가 되어
  `same=true`로 파괴 경로에 진입하지 않았고, 그마저도 `stillOnTurn`이 한 번 더 막았다.
  기준 `results.go`만 overlay로 되돌려 같은 재현을 돌리면 항목 포인터가 유지되고
  본문(`"second turn report"`, `r.bytes=18`)도 남는다(`run-a2-01-02-baseline.txt`) — 즉 파괴의 정도는 새 `begin()`이 키웠다.
- **삭제된 회귀 테스트**: `absent_tree_test.go`에서 `TestAReceiptThatMovedOnIsNotTakenAsCurrent`(정확히 이 케이스를
  검증하던 테스트)가 삭제됐다. 대체로 소개된 `turn_receipt_test.go:153` `TestAReceiptThatMovedOnDoesNotRetargetAnAdmittedFailure`는
  **디스크가 앞으로 간 경우(pinned=second, disk=third)만** 다루고, **항목이 앞으로 간 경우(pinned=first, entry=second)는
  다루지 않는다.** `verification/v031-events-20260921/REPORT.md`는 "기존 stillOnTurn 비교 전용 검사는 ... 검사로 대체했다"고
  적었지만 대체 검사의 방향이 반대다. mutation `failure_retargeted`도 "최신 턴 재조회"를 결함으로 재주입한 것이라
  이 방향을 덮지 않는다.
- **권장 수정**: 고정 방식을 유지하되 **순서 증거를 함께 고정**한다. `readCurrentNativeTurn`은 이미 파일명에서
  단조 증가 `sequence`를 계산하면서 버린다(`native_events.go:122-136`의 `latest`). 이 값을
  `nativeTurnReceipt`/entry에 함께 싣고, `recordFailedAgentRequest`의 파괴 분기 진입 조건에
  "고정 sequence > 항목의 현재 sequence"를 추가한다. 최소 조치로는 `if !same { ... }` 블록에서
  `e.NativeTurn != "" && e.NativeEndObserved` 인 항목에 대해 `begin()`/`applyNativeTurn` 호출을 포기하고
  category를 버린다(증거 보존 우선).
- **미확인**: 단위 수준 재현은 관측했다. 실제 native에서 "beginResult를 타지 않는 장시간 요청 + 같은 자식의 다음 턴 완료"가
  겹치는 빈도는 측정하지 못했다. 자식 agent 대상 compaction 요청의 실제 발생 빈도가 결정적인데
  실 TUI·과금 backend는 범위 밖이다.
- **출처**: 스킬(finding 1) + 직접(재현·기준 대조·영향 경로 `continuation.go:39` 특정·삭제 테스트 추적)

### A2-02 (confirmed / medium) — `begin()`의 포인터 교체가 진행 중인 완료 closure를 고아로 만들어, 새 턴을 옛 턴의 stop payload로 정산한다

- **파일·줄**: `go/internal/gateway/results.go:177-187` (교체), `:266-288` (`beginAnswer` closure), `:291-354` (`stoppedTurn`)
- **발생 조건**: 자식 X의 응답이 streaming 중(`beginAnswer`가 `e.streaming=true`)일 때 SubagentStop이 HTTP EOF보다 먼저
  도착하고, 그 시점에 대기 중인 손자가 없어 `stoppedTurn`이 `e.stopBinding=&binding`만 stash한다(`results.go:327-329`).
  이어서 항목이 `awaiting_children`으로 옮겨가고, 같은 agent의 다음 요청이 `beginResult → begin()`을 호출한다.
- **원인**: v0.3.0의 `begin()`은 `awaiting_children` 항목을 **제자리에서 초기화**했으므로 closure가 잡고 있던 `e`와
  `r.entries[id]`가 같은 객체였다. v0.3.1은 새 객체 `next`를 만들어 map에 넣고 옛 객체를 떼어낸다.
  closure는 떼어진 객체에 쓰고(`:271`의 `e.State != "awaiting_children"` 가드에 걸려 본문 기록조차 건너뛴다),
  그러면서 `d.stopped(*binding)`은 계속 호출한다(`:285-287`). `stoppedTurn(binding, "")`은 `turn==""`이라
  `:309`의 `e.NativeTurn != turn` 가드를 통과해 **map에 있는 새 항목**을 정산한다.
- **영향**: 새 턴의 항목이 즉시 `awaiting_parent`/`stopped`가 되고, 그 본문이 **이전 턴의 `binding.Result`**
  (또는 transcript 복구본)로 채워진다. 이번 턴이 실제로 streaming한 본문은 고아 객체에 남아 버려진다.
  부모는 새 턴의 결과라는 라벨로 옛 턴의 답을 받는다.
- **재현 명령** (작업 디렉터리 `/d/AIDEV/clauduct-v031/go`, 환경 `CGO_ENABLED=0`):
  ```
  CGO_ENABLED=0 go test ./internal/gateway/ \
    -overlay /d/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A2-session-turn-recovery/overlay.json \
    -run 'TestA2OrphanedCompletionSettlesTheNewTurn' -count=1 -timeout 300s -v
  ```
- **예상 결과**: 새 턴 항목은 이번 턴의 본문을 갖거나 미정산 상태로 남는다.
- **실제 결과** (exit 1, FAIL):
  ```
  orphan=0x3dbd7bb0d00 state="awaiting_children" body="" | live=0x3dbd7bb0ea0(next=0x3dbd7bb0ea0)
    state="awaiting_parent" body="PREVIOUS TURN STOP PAYLOAD" source="native_stop" endReason="answer"
    stopped=true r.bytes=26
  ```
  `"NEW TURN STREAMED BODY"`는 어디에도 없다.
- **기준 대조** (같은 재현 + `git show 149068e:go/internal/gateway/results.go` 만 overlay,
  `overlay-baseline.json`, exit 1):
  ```
  orphan=0x25b850fa4b60 state="awaiting_parent" body="NEW TURN STREAMED BODY"
    | live=0x25b850fa4b60(next=0x25b850fa4b60) state="awaiting_parent" body="NEW TURN STREAMED BODY"
      source="delivered_response" endReason="answer" stopped=true r.bytes=22
  ```
- **증거 경로**:
  `/d/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A2-session-turn-recovery/run-a2-01-02-current.txt`
  와 `.../run-a2-01-02-baseline.txt`
- **신규회귀/기존결함**: **혼합.** "옛 턴의 stop이 새 턴 항목을 정산한다"는 부분은 v0.3.0에도 있었다(기존결함).
  "새 턴이 스트리밍한 본문이 버려지고 옛 턴의 payload로 대체된다"는 부분은 **v0.3.1의 신규 회귀**다.
  두 실행 출력의 `body`/`source` 차이가 근거다.
- **권장 수정**: `begin()`이 non-stopped 항목을 떼어낼 때 `e.streaming || e.stopBinding != nil`이면
  stash된 binding을 처리한다. 최소 조치는 옛 객체의 `stopBinding`을 `nil`로 지워 closure가 `d.stopped`를
  호출하지 않게 하는 것이다(그 턴의 완료는 어차피 그 턴 항목과 함께 사라졌다).
- **미확인**: 실제 native에서 "stop 시점에 손자 없음 → 이후 `awaiting_children` 전이" 순서를 관측하지 못했다.
  `stoppedTurn`은 `childrenPending`을 `stopBinding` stash보다 **먼저** 검사하므로(`:312-330`),
  손자가 stop 이전에 등록되면 이 경로에 진입하지 않는다. 다만 `result_restart_test.go:20`이
  `streaming:true, stopBinding:&agentBinding{ID:"old"}, State:"awaiting_children"` 조합을 직접 fixture로 만들고 있어
  제품 팀도 도달 가능한 상태로 보고 있다.
- **출처**: 스킬(finding 4) + 직접(재현·기준 대조)

### A2-03 (improvement / low) — 현재 턴 조회 비용이 세션 길이에 비례해 증가한다 (측정치 확보)

- **파일·줄**: `go/internal/gateway/native_events.go:117-136`
- **발생 조건**: `readCurrentNativeTurn`은 `active/<name>/` 전체를 `ReadDir(16385)`한다. 이 디렉터리는 (agent, turn)당
  `.json`+`.ready` 2개씩 늘고 세션 종료 시 `run.go`의 `RemoveAll` 전까지 아무도 정리하지 않는다.
  `agentSelection`이 모든 `/v1/messages`·`/v1/messages/count_tokens` 마다, `nativeProgressReport`(진단 스냅샷)가
  root에 대해 한 번 더 호출한다.
- **원인**: v0.3.0의 `root.Open("active-<id>.json")` 단일 파일 열기가 디렉터리 전수 스캔으로 바뀌었다.
  `latest`만 필요한데 매번 전부 읽는다.
- **영향**: 요청당 고정 비용 증가. `verification/v031-events-20260921/REPORT.md` 한계 절이
  "턴 조회는 agent별 디렉터리의 제한된 스캔이다. **최대 용량의 지연·처리량은 측정하지 않았다**"고 적어 둔 공백을 메웠다.
  | 턴 수 | 파일 수 | 1회 읽기(warm, 기록본) | 1회 읽기(cold, 첫 실행) |
  |---|---|---|---|
  | 1 | 2 | 106.8µs | 421.3µs |
  | 256 | 512 | 382.5µs | 4.31ms |
  | 1024 | 2048 | 1.05ms | 5.60ms |
  | 4096 | 8192 | 4.11ms | 16.33ms |
  | 8192 (문서화된 상한) | 16384 | **14.11ms** | **58.53ms** |
- **재현 명령** (작업 디렉터리 `/d/AIDEV/clauduct-v031/go`, 환경 `CGO_ENABLED=0`):
  ```
  CGO_ENABLED=0 go test ./internal/gateway/ \
    -overlay /d/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A2-session-turn-recovery/overlay.json \
    -run 'TestA2ActiveTurnScanCostGrowsWithTurnCount' -count=1 -timeout 600s -v
  ```
- **예상 결과**: 조회 비용이 세션 길이와 무관하다(v0.3.0의 단일 파일 열기).
- **실제 결과** (exit 0, PASS — 측정 전용): 위 표. 1턴 대비 8192턴에서 약 **130배**.
- **증거 경로**:
  `/d/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A2-session-turn-recovery/run-a2-03-scan.txt`
- **신규회귀/기존결함**: 신규(비용). 실패가 아니라 비용 증가이므로 improvement.
- **권장 수정**: 발행이 성공적으로 검증된 뒤 그 agent 디렉터리의 더 낮은 sequence 파일 쌍을 gateway가
  `os.Root.Remove`로 정리한다. 이 한 가지가 A2-13의 sticky 상태도 함께 줄인다.
- **미확인**: 실제 세션이 8192턴에 도달하는 빈도. 수천 턴은 장기 위임 세션에서 도달 가능하다고 보지만 관측하지 않았다.
- **출처**: 스킬(finding 7, 미측정 추정) + 직접(측정)

### A2-04 (improvement / low) — `readActiveTurn`/`bindNativeTurn`이 제품 경로에서 죽었고, 남은 주석이 삭제된 동작을 설명한다

- **파일·줄**: `go/internal/gateway/native_events.go:158-194`
- **발생 조건**: 상시.
- **원인**: `messages.go`가 고정 영수증(`entry.nativeTurn`)으로 전환하면서 두 함수의 제품 호출자가 사라졌다.
- **영향**: `grep -rn "bindNativeTurn|readActiveTurn" go/` 결과 제품 호출자 0.
  `bindNativeTurn`은 `continuation_test.go:86`, `features_test.go:150`, `late_receipt_test.go:41`,
  `native_events_test.go:34/96/126`, `turn_order_test.go:35` 에서만,
  `readActiveTurn`은 `bindNativeTurn`과 `turn_order_test.go:46` 에서만 불린다.
  즉 5개 테스트 파일이 **제품이 더 이상 타지 않는 경로**를 검증한다. 특히 `late_receipt_test.go`와
  gateway `native_events_test.go`의 턴 바인딩 시나리오는 실제 `messages.go`가 쓰는
  "선택 시점 고정 → `applyNativeTurn`" 경로가 아니라 옛 "요청 중 재조회" 경로를 계속 증명한다.
  `:160-169`의 12줄 주석은 같은 diff가 지운 `messages.go`의 `beginResult` 순서를 설명한다.
- **재현 명령**: `NOT_RUN` (정적 확인).
  확인 방법: `cd /d/AIDEV/clauduct-v031 && grep -rn "bindNativeTurn\|readActiveTurn" go/` (exit 0, 위 목록).
- **예상 결과**: 담당 테스트가 제품 경로를 검증한다.
- **실제 결과**: 제품 경로 밖의 헬퍼를 검증한다.
- **증거 경로**: `NONE` (grep 결과는 위에 인용)
- **신규회귀/기존결함**: 신규(커버리지 품질).
- **권장 수정**: 테스트를 `agentSelection` + `applyNativeTurn` 경로로 옮기고 두 함수와 낡은 주석을 삭제하거나,
  테스트 전용임을 이름·위치로 드러낸다.
- **미확인**: 없음.
- **출처**: 스킬(finding 8) + 직접(grep 확인)

### A2-05 (improvement / low) — 새 거부 경로가 `nativeEvents.invalid`를 올리지 않아 진단이 0을 보고한다

- **파일·줄**: `go/internal/gateway/native_events.go:119-121`, `:130-132`, `:140-143`
- **발생 조건**: `ReadDir` 실패/항목 초과, 파일명 파싱 실패, `.ready` 부재·비정규·비영(非零) 크기.
- **원인**: 세 경로 모두 `errDelegationUnverified`를 직접 반환하면서 `readNativeReceipt`(`:46-54`)의
  `invalid++`를 거치지 않는다.
- **영향**: 모든 요청이 `AGENT_SELECTION_UNVERIFIED`로 거부되는 동안 `NativeEventReport.Invalid`가 0으로 보고된다.
  운영자가 이 조건을 이름 붙이라고 만든 진단이 침묵한다. 단 "발행이 아직 안 끝난 새 본문"을 invalid로
  세지 않는 것은 **의도**이며 `turn_receipt_test.go:58`이 명시적으로 단언한다(`Invalid != 0` 이면 실패).
  나머지 두 경로(파일명 오염, 완결된 시도의 마커 부재)는 의도가 확인되지 않는다.
- **재현 명령**: `NOT_RUN`. 확인 방법: 파일명 오염/마커 부재 케이스에 별도 카운터를 추가하고
  `CGO_ENABLED=0 go test ./internal/gateway/ -run 'TestIncompleteTurnPublication' -count=1` 로 의도 구간과 구분한다.
- **예상 결과**: 구조적 오염과 미완성 발행이 진단에서 구분된다.
- **실제 결과**: 둘 다 `Invalid=0`.
- **증거 경로**: `NONE`
- **신규회귀/기존결함**: 신규(진단 공백). v0.3.0은 단일 파일 읽기라 모든 실패가 `readNativeReceipt`를 거쳤다.
- **권장 수정**: "미완성 발행"과 "구조적 오염"을 다른 카운터로 분리하고 후자만 `invalid`에 넣는다.
- **미확인**: 구조적 오염이 제품 동작만으로 발생할 수 있는지(A2-13과 연결).
- **출처**: 스킬(finding 10) + 직접(의도 구간 식별)

### A2-06 (improvement / low) — `begin()`이 `r.bytes`만 빼고 `e.body`를 비우지 않아 파일 내 유일하게 불변식을 깬다

- **파일·줄**: `go/internal/gateway/results.go:183`
- **발생 조건**: `awaiting_children` 항목의 재시작.
- **원인**: `results.go:154-156`, `:220-222`, `:262-263`, `:321-322`, `:530-531`은 모두
  `r.bytes -= len(e.body)`와 `e.body = ""`를 짝지어 둔다. `:183`만 빼기만 한다.
- **영향**: **지금은 실제 이중 차감이 일어나지 않는다.** 전수 확인한 근거 — 떼어진 객체를 계속 잡는 유일한 곳은
  `beginAnswer` closure인데 그 안의 `r.bytes -=`(`:275`)는 `e.State != "awaiting_children"` 가드 아래라 실행되지 않고,
  `deliver`의 `receipts`(`:498`)는 `e.stopped && deliverable(State)` 만 담으므로 `awaiting_children` 고아를 담지 않는다.
  따라서 잠재 불변식 파손이며, A2-02 수정으로 고아의 수명이 바뀌면 곧바로 활성화될 수 있다.
- **재현 명령**: `NOT_RUN` (정적 확인). `result_restart_test.go:33-39`가 `r.bytes`만 단언하고
  고아 본문은 단언하지 않는 것도 확인했다.
- **예상 결과**: 모든 차감 지점이 같은 불변식을 지킨다.
- **실제 결과**: 한 곳만 어긴다.
- **증거 경로**: `NONE`
- **신규회귀/기존결함**: 신규(이 줄 자체가 v0.3.1 추가).
- **권장 수정**: `:183`에 `e.body, e.Bytes = "", 0`을 함께 둔다.
- **미확인**: 없음.
- **출처**: 스킬(finding 11) + 직접(이중 차감 경로 전수 확인 후 강등)

### A2-07 (improvement / low) — `openProjects(relative)`의 인자가 검증에만 쓰이고 반환 루트에 반영되지 않는다

- **파일·줄**: `go/internal/gateway/projects.go:17-22`
- **발생 조건**: 향후 호출자가 인자 의미를 오해할 때.
- **원인**: 반환 루트는 언제나 `d.projects`다. `relative`는 `filepath.IsLocal` 검사에만 쓰인다.
- **영향**: 14개 호출 지점 중 12개가 `"."`를 넘기고, 실제 경로를 넘기는
  `delegation.go:630/769`, `context_journal.go:65/148/153`, `context_display.go:81`은 전부
  `root.Open(filepath.Join(rel, ...))`로 경로를 다시 붙인다. **현재 오용은 없다(전수 확인).**
  다만 이름이 "이 하위 경로를 연다"로 읽혀, 향후 `root, _ := d.openProjects(rel); root.Open("agent.meta.json")`
  같은 호출이 조용히 `d.projects/agent.meta.json`을 읽게 된다.
- **재현 명령**: `NOT_RUN`. 확인 방법:
  `cd /d/AIDEV/clauduct-v031 && grep -rn "openProjects" go/internal/gateway/*.go | grep -v _test` (exit 0, 14 호출).
- **예상 결과**: 인자를 받는 함수가 인자를 쓴다.
- **실제 결과**: 쓰지 않는다.
- **증거 경로**: `NONE`
- **신규회귀/기존결함**: 신규(이 파일이 v0.3.1 신규).
- **권장 수정**: 부재 분류(경로 불필요)와 포함 검사를 분리하거나, 반환을 `(*os.Root, string, error)`로 바꿔
  정규화된 상대 경로를 함께 돌려준다.
- **미확인**: 없음.
- **출처**: 스킬(finding 9) + 직접(호출 지점 전수 확인 후 강등)

### A2-10 (improvement / medium) — 프롬프트 차단 안내가 "다시 제출하라"만 말해 결정적 실패에서 오도한다

- **파일·줄**: `go/cmd/clauduct-hook/main.go:189-197`
- **발생 조건**: `/clauduct/context`가 200/204 이외를 반환하면 `postReply`가 오류를 내고(`:361-363`),
  `UserPromptSubmit`이면 exit 2로 프롬프트를 차단하며
  `"...restore the Clauduct hook connection and submit the prompt again."` 하나만 출력한다.
  400 `CONTEXT_JOURNAL_UNVERIFIED`(`context.go:163-169` → `context_journal.go:29-42`)는 연결 문제가 아니다.
- **원인**: 전송 실패와 정책 거부가 같은 분기로 합쳐졌다.
- **영향**: 결정적 400(transcript가 projects 루트 밖, 같은 세션의 경로 변경, 세션 1024개 상한 등)에서
  사용자는 "연결 복구 후 재제출"만 안내받고 무한히 막힌다. 원인 분류가 stderr에 실리지 않는다.
  **차단 자체는 결함이 아니다**: `verification/v031-session-20260921/REPORT.md`가 Claude Code 공식 hook 계약
  (`exit-code-2-behavior-per-event`)을 인용하고 설치본 2.1.278에서 확인했다고 기록했으며,
  `session_registration_test.go`의 `persistent_failure` 하위 테스트가 "backend 호출 0 + 안내 출력"을 단언한다.
  같은 보고서가 "hook 미설치·비활성화·timeout 또는 지속적인 잘못된 경로/연결은 별도 원인 해결이 필요하다"고
  잔여 위험을 이미 적어두었다. 따라서 채택된 정책에 대한 improvement로 제출한다.
- **재현 명령** (작업 디렉터리 `/d/AIDEV/clauduct-v031/go`, 환경 `CGO_ENABLED=0`):
  ```
  CGO_ENABLED=0 go test ./cmd/clauduct-hook/ -count=1 -timeout 300s
  ```
  (제품 테스트 `TestPromptSubmissionRegistersSessionWithoutForwardingPrompt`의 `Bad Request` 하위 케이스가
  400 → exit 2 → `"submit the prompt again"` 을 이미 단언한다. 동작은 이미 관측돼 있고 다툼은 문구/분류다.)
- **예상 결과**: 400(정책 거부)과 5xx/전송 실패가 서로 다른 안내를 낸다.
- **실제 결과** (exit 0, `ok ...clauduct-hook 0.293s`): 두 경우 모두 동일 문구.
- **증거 경로**: `NONE` (기존 제품 테스트 출력으로 충분)
- **신규회귀/기존결함**: 신규 동작(v0.3.0에는 `UserPromptSubmit` 처리가 없었다). 채택된 설계이므로 improvement.
- **권장 수정**: 상태코드를 안내에 실어 400계열은 "세션 등록이 정책상 거부됨(설정/transcript 경로 확인)",
  5xx/전송 실패는 현행 문구로 나눈다. gateway가 낸 분류 문자열은 이미 400 응답 본문에 있으므로
  `postReply`가 그것을 함께 돌려주면 된다.
- **미확인**: 결정적 400을 유발하는 **실제 제품 입력**을 특정하지 못했다.
  `projects`가 `CLAUDE_CONFIG_DIR/projects`에서 파생되고 Claude Code의 transcript도 같은 곳이라
  cross-volume·경로 변경 시나리오를 재현하지 못했다(7절 4번 참조).
- **출처**: 스킬(finding 2, "배포 차단" 주장) → 직접 검증 후 improvement로 강등

### A2-11 (improvement / low) — `subagent_test.go`에서 `fellBack != 0` 단언이 근거 없이 제거됐다

- **파일·줄**: `go/internal/app/subagent_test.go:151`, `:158-161`
- **발생 조건**: 상시(커버리지).
- **원인**: v0.3.1 diff가 `session, sub, unregistered, unrouted, fellBack := subagentRun(t)` 를
  `..., _ := subagentRun(t)` 로 바꾸고 `fellBack != 0` 를 단언에서 뺐다.
  같은 헬퍼를 쓰는 `go/internal/app/workflow_test.go:473`은 그대로 유지한다.
  `subagentRun`은 여전히 `fellBack`을 계산해 반환한다(`:134`, `:137`).
- **영향**: `FellBackToCaller`(= `delegations.unroutedRoles`)가 Explore 위임에서 0이라는 보증이 사라졌다.
  v0.3.1은 바로 이 카운터가 올라가는 조건을 바꿨으므로(`delegation.go:481-503`,
  fallback source가 `parent-route` → `native-selection`) 이 테스트가 지켜야 할 값이다.
- **재현 명령** (작업 디렉터리 `/d/AIDEV/clauduct-v031/go`, 환경 `CGO_ENABLED=0`) — 삭제된 단언 재주입:
  ```
  CGO_ENABLED=0 go test ./internal/app/ \
    -overlay /d/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A2-session-turn-recovery/overlay-app.json \
    -run 'TestA2DroppedFellBackAssertion' -count=1 -timeout 300s -v
  ```
- **예상 결과**: 단언이 실패한다면 "통과시키려고 지운 것"이다.
- **실제 결과** (exit 0, PASS):
  `session=gpt-6-astra/low sub=gpt-5.6-luna/max unregistered=0 unrouted=0 fellBack=0`.
  **여전히 0이다.** 실패를 숨기려고 지운 것이 아니라 불필요하게 커버리지를 줄였다.
- **증거 경로**:
  `/d/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A2-session-turn-recovery/run-a2-11-fellback.txt`
- **신규회귀/기존결함**: 신규 회귀(테스트 커버리지).
- **권장 수정**: 단언을 되돌린다. `workflow_test.go:473`과 형태를 맞추면 된다.
- **미확인**: 제거 의도(기록 없음). events/projects/session 세 보고서 어디에도 이 제거에 대한 설명이 없다.
- **출처**: **직접** (스킬은 테스트 파일을 다루지 않았다)

### A2-12 (improvement / low) — plugin이 모델 허용목록을 `bridge.Models`와 이중으로 들고 있다

- **파일·줄**: `go/internal/app/native-events.mjs:68`
- **발생 조건**: `bridge.Models`에 모델을 추가하고 이 줄을 갱신하지 않을 때.
- **원인**: gateway 쪽 `validActiveReceipt`(`native_events.go:221-224`)는 `bridge.Models`를 순회하는데,
  plugin은 `['gpt-6-astra','gpt-5.6-sol','gpt-5.6-terra','gpt-5.6-luna']`를 하드코딩한다.
  이 파일은 이미 `__CLAUDUCT_EVENT_ROOT__` 치환 대상(`:5`)이므로 목록도 같은 방식으로 주입 가능하다.
- **영향**: 새 모델의 모든 턴이 `"unlisted"`로 기록된다. `continuationScope`(`continuation.go:19`)의
  `active.Model != choice.route.Model`이 걸려 이어가기가 거부되고,
  `delegation.go:491`의 `bridge.SelectRoute(active.Model, active.Effort)`도 실패한다.
- **재현 명령**: `NOT_RUN`. 확인 방법: `bridge.Models`에 항목을 추가한 overlay로 plugin은 그대로 두고
  `CGO_ENABLED=0 go test ./internal/app/ -run 'TestNativeEventModuleRunsWithNoNodeOnChildPATH' -count=1 -timeout 600s`.
- **예상 결과**: 모델 목록이 한 곳에서만 정의된다.
- **실제 결과**: 두 곳.
- **증거 경로**: `NONE`
- **신규회귀/기존결함**: 기존결함(v0.3.0의 같은 줄에도 동일 목록이 있었다).
- **권장 수정**: `__CLAUDUCT_EVENT_MODELS__` 치환으로 통일한다.
- **미확인**: 없음.
- **출처**: 스킬(finding 13) + 직접(영향 경로 특정)

---

## 4. 반박된 지적

### R-01 (refuted) — "root 턴이 공유 4096 상한을 쓰게 되어 이전에 없던 hard stop이 생겼다" (스킬 finding 3)

- **파일·줄**: `go/internal/app/native-events.mjs:62`
- **사실관계는 맞다.** v0.3.0의 root 분기는 `if (!agent && !turns.has('main:'+turn))`로 크기 검사 없이 썼고,
  `turns.size>=4096` 검사는 자식 분기에만 있었다. v0.3.1의 단일 가드는 root도 막는다.
- **그러나 결함이 아니다 — 의도이고 명시적으로 검사한다.**
  `go/internal/app/native_events_publication_test.mjs:44`가
  `await assert.rejects(step('overflow'),/CLAUDUCT_NATIVE_EVENT_LIMIT/)`로
  **agent 없는 root step**의 거부를 단언한다(`step(turn, agent)`에서 agent 생략 = root).
  `:41-43`은 마지막 슬롯을 두고 root와 child가 경쟁할 때 정확히 하나만 성공함을 단언한다.
  `verification/v031-events-20260921/REPORT.md`도 "완료·진행 중인 턴은 합계 4096개, ...
  파일 생성 시도는 8192회로 제한한다"고 적었다.
- **반박 실행** (작업 디렉터리 `/d/AIDEV/clauduct-v031/go/internal/app`):
  `node native_events_publication_test.mjs` → exit 0,
  `PASS: failed write retry, immutable earlier receipts, shared concurrent 4096-turn cap`.
- 스킬이 시사한 경쟁 조건도 없다: `turns.size>=4096` 검사와 `turns.set`은 `await` 없이 같은 tick에서 실행된다
  (`native-events.mjs:59-79`는 동기 구간).
- 남는 실질 과제는 A2-03의 개선(발행 정리)뿐이다.

---

## 5. 보류 항목과 결론을 내리려면 무엇이 필요한지

### A2-08 (hold / low) — `bridge.CanonicalRole(meta.AgentType)`가 자신의 문서화된 계약을 어긴다

- **파일·줄**: `go/internal/gateway/workflow.go:282`
- **근거**: `bridge.CanonicalRole`의 주석(`route.go:160-161`)은 "Callers must resolve custom definitions first:
  native allows a custom Fork distinct from fork"라고 못 박는다. 이 분기(`run.origin.adapterBytes == 0`)는
  custom 정의를 전혀 해석하지 않는다. 기존에는 `meta.AgentType != "workflow-subagent"` 완전일치였다.
  통과하면 `route.Source="workflow-parent"`로 부모 경로를 상속받는다(`:310-315`).
- **왜 확정하지 못했는가**: `"Workflow-Subagent"`처럼 대소문자만 다른 custom 역할이 이 분기에 도달하려면
  같은 이름이 `meta.AgentType`과 `binding.Role`에 동시에 들어와야 하고(`:269`가 완전일치 요구),
  adapter가 없는 workflow 실행이어야 한다. adapter 없는 실행에서 native는 고정 이름 `workflow-subagent`를 쓴다.
  그 조합을 만들어낼 제품 입력을 찾지 못했다.
- **결론 조건**: native 2.1.278에서 adapter 없는 Workflow 자식의 `agentType`이 사용자 정의 이름을 가질 수 있는지,
  또는 사용자가 `.claude/agents`에 `Workflow-Subagent`를 정의했을 때 실제로 기록되는 값이 무엇인지
  실제 native 실행 metadata로 확인.
- **권장 수정**: `roleMatches("workflow-subagent", meta.AgentType, custom)` 형태로 custom 여부를 함께 넘기거나
  기존 완전일치로 되돌린다.
- **출처**: 스킬(finding 5) + 직접(도달 경로 조사)

### A2-09 (hold / low) — 형제 비교를 전부 `roleMatches`로 옮기면서 `workflow.go:279`만 완전일치로 남았다

- **파일·줄**: `go/internal/gateway/workflow.go:279` (`receipt.Role != meta.AgentType`)
- **근거**: `continuation.go:15/23`, `resume.go:124`, `native_events.go:265`, `delegation.go:507`은 모두
  `roleMatches(expected, observed, custom)`로 이동했다. 근거는 `bridge.IsFork` 주석의
  "Native resolves these case-insensitively; this build did not, in five different places". 여기만 남았다.
- **왜 확정하지 못했는가**: `receipt.CustomRole`이 true면 `roleMatches`도 완전일치이므로 차이가 없다.
  차이는 built-in 역할 이름이 adapter 라벨과 native 기록에서 대소문자가 갈릴 때만 생기는데,
  `workflowLabelSelection`이 만드는 `receipt.Role`이 그럴 수 있는지 실제 입력으로 확인하지 못했다.
- **결론 조건**: adapter 라벨→역할 매핑이 만든 `SelectionRecord.Role`과 native가 쓴 `agentType`을
  같은 실행의 metadata에서 대조.
- **권장 수정**: `!roleMatches(receipt.Role, meta.AgentType, receipt.CustomRole)`로 통일하거나,
  여기만 완전일치인 이유를 주석으로 남긴다.
- **출처**: 스킬(finding 12) + 직접(영향 범위 축소)

### A2-13 (hold / low) — 완료 표시 없는 최고 sequence가 남으면 그 agent의 모든 조회가 거부된다

- **파일·줄**: `go/internal/gateway/native_events.go:130-132`, `:140-143`
- **근거**: `<n>-<turn>.json`은 썼는데 `<n>-<turn>.ready`를 못 쓴 채 끝나면, 그 `n`이 최고 sequence인 동안
  `root.Stat(... + ".ready")`가 실패해 `errDelegationUnverified`가 나온다. `id==""`면 root의 모든 요청이,
  자식이면 그 자식의 모든 요청이 `AGENT_SELECTION_UNVERIFIED`가 된다(`messages.go:285-288`).
  파일명 파싱 실패 시 `continue`가 아니라 `return`하는 `:130-132`도 같은 성격이다.
- **왜 확정하지 못했는가**: plugin의 `turn.step`은 `await publication` **이후에** `next(e)`를 호출하므로
  (`native-events.mjs:80-89`) 발행이 실패한 턴에는 gateway로 갈 요청 자체가 없다.
  즉 이 거부가 살아 있는 턴을 좌초시키는 경로를 만들지 못했다. `turns.delete(key)`(`:78`)로 같은 턴 재시도 시
  sequence가 올라가 자연 복구되며, `turn_receipt_test.go:40-74`가 이 복구를 명시적으로 검사한다.
  `active/<name>/`에 쓰는 주체는 plugin뿐이고 gateway는 그 디렉터리에 쓰지 않는다(확인함).
- **결론 조건**: (a) 발행 실패 후 native가 같은 세션에서 그 agent에 대해 **새 턴 없이** 요청을 보내는 경로가 있는지,
  (b) `.json`은 성공하고 `.ready`만 실패하는 부분 쓰기가 실제 Windows에서 관측되는지.
- **권장 수정**: 실패한 attempt의 `.json`을 발행 실패 시(또는 다음 성공 발행 시) 제거해 최고 sequence가 되지 않게 한다.
  A2-03의 정리와 같은 작업이다. 현재의 fail-closed 판단 자체는 옳으므로 유지.
- **출처**: 스킬(finding 6, "no recovery" 주장) → 직접 검증 후 hold로 조정

---

## 6. 개선 제안 (위 improvement 항목 외 추가)

1. **버려지는 순서 증거를 살려라.** `readCurrentNativeTurn`이 계산한 `latest`(단조 sequence, `:122-136`)를
   영수증과 함께 반환해 `record.nativeTurn`에 실으면, A2-01의 정식 수정 근거가 되고
   "턴 ID가 불투명 문자열이라 순서를 알 수 없다"는 현재의 구조적 한계가 그대로 해소된다. 파일명에 이미 있는 정보다.
2. `native_events.go:158-169`의 주석은 삭제된 `messages.go` 순서를 설명한다. A2-04 수정 시 함께 갱신.
3. `progress.go:62`는 이제 `readCurrentNativeTurn`을 거치므로 **진단 읽기 한 번이 `nativeEvents.invalid`를
   올릴 수 있게 됐다.** v0.3.0의 `readNativeJSON("active-main.json", ...)`은 올리지 않았다.
   진단이 자기 카운터를 움직이는 것은 피하는 편이 낫다.
4. **되돌리지 말 것 (올바른 수정으로 확인함):** `results.go:163`의 `e.stopped && len(r.entries) >= maxAgents`
   조건과 `:174`의 `r.bytes -= len(r.entries[oldest].body)` 추가.
   `awaiting_children` 경로는 map 키를 늘리지 않으므로 eviction이 불필요하고, 삭제 시 byte 회수 누락도 해소된다.
   `TestRestartEvictionReleasesOnlyTheEvictedBody` 실행으로 확인했다.
5. `bindNativeCancellation`(`native_cancellation.go:30-36`)이 고정 영수증을 쓰도록 바뀐 것은 **개선**이다.
   요청은 접수된 턴에 속하므로 그 턴의 abort 영수증에 묶이는 편이 재조회보다 정확하다.
   `p.identity`가 값 복사라 다른 goroutine과의 공유도 없다.

---

## 7. 근거 부족으로 후보에서 뺀 의심 (삭제하지 않고 기록)

1. **`record.nativeTurn`의 비동기화 접근** — `mu`로 보호되는 `data`와 달리 이 필드는 lock 없이 읽고 쓴다.
   전 독자를 추적했다: `messages.go:164/278/293`(핸들러 goroutine), `native_events.go:245`(같은 goroutine의 defer),
   `native_cancellation.go:30/33`(핸들러 goroutine이며 이후 **값 복사**를 binding에 저장하므로
   `ReconcileNativeCancellations`의 다른 goroutine은 `p.identity` 사본만 본다).
   직렬화 대상 `RequestRecord`에도 없다. 담당 테스트 `-race` 실행에서도 보고 없음. → 결함 아님.
2. **Windows 대소문자 무시로 인한 `active/child-<A>` ↔ `active/child-<a>` 충돌** —
   두 agent ID가 대소문자만 다르면 같은 디렉터리를 공유하고, `validActiveReceipt`의 `receipt.Agent != id`가 걸려
   한쪽이 거부된다. 그러나 v0.3.0의 `active-<agent>.json` 평면 파일명도 똑같이 충돌했으므로 신규 회귀가 아니고,
   native가 대소문자만 다른 상관 ID를 낸다는 증거도 없다.
3. **hook이 gateway 환경변수 없이 실행되어 모든 프롬프트를 막는 경우** — `postReply`는 `ANTHROPIC_BASE_URL`이
   loopback이 아니거나 토큰이 없으면 `errInvalidGateway`를 내고, `UserPromptSubmit`이면 exit 2가 된다.
   그러나 제품은 이 hook을 세션 `--settings`로만 주입하며(`app/settings.go:64-92`, `app/user_settings.go`),
   그 세션에는 항상 환경이 있다. 사용자가 직접 전역 설정에 넣지 않는 한 도달 불가 → 제품 결함 아님.
4. **`contextSession`의 결정적 400 유발 조건** — cross-volume transcript, 세션 1024개 상한(`maxAgents`),
   같은 세션의 transcript 경로 변경을 후보로 뒀으나, `projects`가 `CLAUDE_CONFIG_DIR/projects`에서 파생되고
   (`app/run.go:285-301`) Claude Code의 transcript도 같은 곳이라 실제 유발 입력을 특정하지 못했다.
   Go의 `filepath.Rel`은 Windows에서 대소문자를 접어 비교하므로 그 경로도 아니다. A2-10의 "미확인"으로 이관.
5. **`/clauduct/context` 처리 중 `contexts.mu` 경합으로 hook 3초 상한 초과** — `beginContext`는 lock을
   추론/전달 구간에 걸지 않는다(`context.go:214-215`에서 획득, 함수 반환 시 해제, 재획득은 `finishContext`에서만).
   lock 아래에 네트워크 호출이 없다. → 근거 없음.
6. **`begin()` eviction의 이중 차감** — `oldest`는 `resultReported(item.State)`만 고르는데, 그 상태로 만드는
   `deliver`의 finish closure(`results.go:516-533`)가 이미 `e.body=""`로 비운다. `len(body)==0`이라 무해.
7. **`selection_records.go`의 `roleMatches` 전환이 매칭을 넓힌 점** — 옛 코드는 `Plan/Explore/general-purpose`만
   대소문자를 접었고 새 코드는 `workflow-subagent`, `fork`, `clauduct-inherit`, menu 역할까지 접는다.
   `alias != r.NativeModel`과 `fields["effort"] != nil` 가드가 남아 있어 오매칭 위험이 낮고,
   대소문자만 다른 두 역할이 같은 세션에서 같은 alias를 쓰는 입력을 구성하지 못했다.
8. **v0.3.0 journal의 `CustomRole` 부재로 인한 오분류** — `choiceJournal`/`SelectionRecord`의 `CustomRole`은
   v0.3.1 신규 필드라 구버전 journal에서는 false로 읽힌다. 다만 v0.3.0 journal의 재개 불가는
   `docs/v2/COMPATIBILITY.md`에 문서화된 미지원이므로 새 회귀로 보지 않는다.

---

## 8. 실행한 명령 전체 목록과 NOT_RUN

작업 디렉터리를 명시하지 않은 것은 `/d/AIDEV/clauduct-v031`. Go 검사는 모두 `/d/AIDEV/clauduct-v031/go`.
`$R` = `/d/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A2-session-turn-recovery`.

| # | 명령 | exit |
|---|---|---|
| 1 | `git status --short`, `git rev-parse HEAD`, `git diff --stat` | 0 |
| 2 | `git diff --numstat -- <담당 33개 경로>` | 0 |
| 3 | `git diff -- go/internal/gateway/native_events.go` | 0 |
| 4 | `git diff -- go/internal/gateway/messages.go` | 0 |
| 5 | `git diff -- results.go continuation.go resume.go selection_records.go progress.go` | 0 |
| 6 | `git diff -- cmd/clauduct-hook/main.go projects.go workflow.go workflow_checkpoint.go workflow_plan.go workflow_recovery.go workflow_results.go` | 0 |
| 7 | `git diff -- <담당 테스트 8개>` | 0 |
| 8 | `git diff -- native_cancellation.go context_display.go progress.go` | 0 |
| 9 | `git diff -- go/internal/app/settings.go go/internal/app/user_settings.go` | 0 |
| 10 | `git diff -- go/internal/gateway/diagnostics.go`, `gateway.go`, `delegation.go` | 0 |
| 11 | `git show 149068edd693fb860a03244a2ea15764bcd68c34:go/internal/gateway/results.go > $R/baseline/results.go` | 0 |
| 12 | `grep -rn "recordFailedAgentRequest" go/` | 0 |
| 13 | `grep -rn "bindNativeTurn\|readActiveTurn" go/` | 0 |
| 14 | `grep -rn "openProjects" go/internal/gateway/*.go` | 0 |
| 15 | `grep -rn "os.OpenRoot" go/internal/gateway/*.go` | 0 |
| 16 | `grep -rn "unroutedRoles\|FellBackToCaller\|fellBack" go/` | 0 |
| 17 | `grep -rn "UserPromptSubmit" go/ docs/ verification/` | 0 |
| 18 | `grep -rn "continuationScope" go/` | 0 |
| 19 | `grep -rn "nativeTurn" go/internal/gateway/*.go` | 0 |
| 20 | `CGO_ENABLED=0 go vet -overlay $R/overlay-baseline.json ./internal/gateway/` | 0 |
| 21 | `CGO_ENABLED=0 go test ./internal/gateway/ -overlay $R/overlay.json -run 'TestA2StalePinnedTurnDestroysNewerAwaitingChildren\|TestA2OrphanedCompletionSettlesTheNewTurn' -count=1 -timeout 300s -v` | **1 (의도된 FAIL — A2-01/A2-02 재현)** |
| 22 | `CGO_ENABLED=0 go test ./internal/gateway/ -overlay $R/overlay-baseline.json -run '<동일>' -count=1 -timeout 300s -v` | **1 (기준 대조 출력 확보)** |
| 23 | `CGO_ENABLED=0 go test ./internal/gateway/ -overlay $R/overlay.json -run 'TestA2ActiveTurnScanCostGrowsWithTurnCount' -count=1 -timeout 600s -v` | 0 |
| 24 | `CGO_ENABLED=0 go test ./internal/app/ -run 'TestASubagentRunsWhereItsRoleSaysAndNotWhereTheClientAsked' -count=1 -timeout 300s -v` | 0 |
| 25 | `CGO_ENABLED=0 go test ./internal/app/ -overlay $R/overlay-app.json -run 'TestA2DroppedFellBackAssertion' -count=1 -timeout 300s -v` | 0 |
| 26 | `CGO_ENABLED=0 go test ./internal/gateway/ -run 'TestIncompleteTurnPublication\|TestMainTurnIdentity\|TestIndependentRootAuxiliary\|TestAReceiptThatMovedOn\|TestRestartKeepsOnly\|TestRestartEviction\|TestNativeContinuationRequires\|TestNativeTerminalReceipts\|TestMalformedActiveNativeReceipt\|TestRejectedContinuationRecords\|TestReadingTheActiveTurnChangesNothing\|TestALateAbortReceipt\|TestProjects\|TestMissingChoiceInAvailableProjects\|TestAnAbsentProjectsTree\|TestContextWriterDoesNot' -count=1 -timeout 300s` | 0 |
| 27 | `CGO_ENABLED=0 go test ./cmd/clauduct-hook/ -count=1 -timeout 300s` | 0 |
| 28 | `node native_events_publication_test.mjs` (cwd `/d/AIDEV/clauduct-v031/go/internal/app`) | 0 |
| 29 | `CGO_ENABLED=1 CC=C:/msys64/ucrt64/bin/gcc.exe go test ./internal/gateway/ -race -run 'TestIncompleteTurnPublication\|TestAReceiptThatMovedOn\|TestRestart\|TestNativeContinuation\|TestNativeTerminalReceipts\|TestIndependentRootAuxiliary\|TestProjects' -count=1 -timeout 300s` | 0 |
| 30 | `CGO_ENABLED=0 go test ./internal/app/ -run 'TestNativeSessionRegistrationRecoversBeforePrompt' -count=1 -timeout 600s -v` | 0 |
| 31 | `CGO_ENABLED=0 go build ./...` | 0 |
| 32 | `git status --porcelain -- go/ docs/` (83개 = v0.3.1 기존 변경 그대로), `git status --porcelain -- verification/v031-code-review-20260922/` (`?? ` 신규 작업공간만) | 0 |
| 33 | `python` 으로 `C:\Users\js\.claude\plugins\installed_plugins.json` 키 목록 확인 | 0 |
| 34 | `python` 으로 `verification/v031-{events,projects}-20260921/mutations.json` 항목 열거 | 0 |
| 35 | `python` 으로 `verification/v031-session-20260921/mutations.json` 열거 | 1 (스키마가 달라 `KeyError: 'name'` — 별도 raw 덤프로 확인 완료) |

### NOT_RUN

| 검사 | 이유 | 추가 확인 방법 |
|---|---|---|
| `go test ./...` 전체 회귀 | 과제에서 금지. 통합 검토자가 필요성 판단 | 통합 단계에서 `CGO_ENABLED=0 go test -count=1 -timeout=8m ./...` |
| A2-01의 실제 native end-to-end 재현 | `beginResult`를 타지 않는 장시간 요청과 같은 자식의 다음 턴 완료를 실 TUI에서 겹치게 해야 함. 수동 TUI·과금 backend는 범위 밖 | 자식 agent 대상 compaction 요청과 그 자식의 후속 턴을 동시에 유발하는 fixture 세션 |
| A2-02의 실제 native 순서 관측 | SubagentStop이 손자 등록보다 먼저 오는 실제 순서를 만들지 못함 | native의 SubagentStop / turn.complete 타이밍 로그 |
| A2-04·A2-05·A2-06·A2-07·A2-12의 재현 | 정적 확인으로 충분하거나(불변식·미사용 인자·중복 상수) 제품 입력을 만들 수 없음 | 각 항목에 기재 |
| A2-08·A2-09 (hold) | 실제 native metadata 필요 | 5절 각 항목의 "결론 조건" |
| A2-13 (hold) | 부분 쓰기(`.json` 성공 / `.ready` 실패)를 실제 OS에서 유발하지 못함 | 5절 A2-13의 "결론 조건" |
| `TestNativeEventPluginPassesInstalledValidator`, `TestNativeEventModuleRunsWithNoNodeOnChildPATH` | 설치본 native를 20~60초 구동. 담당 파일의 단언 검토는 정적으로 완료했고 events 묶음 보고서에 PASS 기록이 있음 | `CGO_ENABLED=0 go test ./internal/app/ -run 'TestNativeEvent' -count=1 -timeout 600s` |
| `/code-review` 의 effort/모델 기계 증거 | 반환 payload에 표기 없음 | 호스트가 skill 실행 메타데이터를 노출하면 확인 가능 |

---

## 부록 — 재현 자산

`/d/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A2-session-turn-recovery/`

| 파일 | 용도 |
|---|---|
| `a2_repro_test.go` | A2-01, A2-02 재현 (package gateway, overlay로 `zz_a2_repro_test.go` 주입) |
| `a2_scan_test.go` | A2-03 스캔 비용 측정 (package gateway) |
| `a2_fellback_test.go` | A2-11 삭제 단언 재주입 (package app) |
| `overlay.json` | 현행 트리 + gateway 테스트 2개 |
| `overlay-baseline.json` | 현행 트리 + gateway 재현 + **기준 `results.go`** |
| `overlay-app.json` | 현행 트리 + app 테스트 |
| `baseline/results.go` | `git show 149068e:go/internal/gateway/results.go` 사본 |
| `run-a2-01-02-current.txt` | A2-01/A2-02 현행 실행 출력 (exit 1) |
| `run-a2-01-02-baseline.txt` | 같은 재현의 기준 `results.go` 대조 출력 (exit 1) |
| `run-a2-03-scan.txt` | A2-03 측정 출력 (exit 0) |
| `run-a2-11-fellback.txt` | A2-11 단언 재주입 출력 (exit 0) |

제품 소스는 한 줄도 바뀌지 않았다. 확인:
`git status --porcelain -- go/ docs/` 가 리뷰 시작 시점과 동일한 83개이고,
`git diff --stat -- go/internal/gateway/native_events.go go/internal/gateway/results.go` 가
각각 117줄·26줄 변경으로 시작 시점의 numstat(77/40, 11/15)과 일치한다.
새로 만든 경로는 `verification/v031-code-review-20260922/` 아래(내 분야 작업공간)뿐이다.
