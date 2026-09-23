# A3 — 압축·사용량 분야 리뷰 (v0.3.1 전체 변경)

- 대상 루트: `D:\AIDEV\clauduct-v031` (worktree). 기준 commit `149068edd693fb860a03244a2ea15764bcd68c34`, HEAD 동일, v0.3.1 전체가 미커밋 작업트리.
- 담당 초점: 컨텍스트 정책과 압축 티켓, 압축 effort 상한, 토큰 계수, usage 집계, 표시값, 미디어 추정, 기능 플래그.
- 담당 diff 규모: 23개 파일 `+987 / -115`.
- 제품 소스·테스트·문서·기존 `verification/`를 일절 수정하지 않았다. 재현은 전부 `go test -overlay`로만 했고, 산출물은 이 분야 디렉터리 안에만 썼다. index와 작업트리는 그대로다 (`git status` 기준 tracked 변경 64건 = 리뷰 시작 시점 스냅샷과 동일, 신규는 untracked `verification/v031-code-review-20260922/`뿐).

---

## 1. 실행 근거 — /code-review

### 호출 원문
```
Skill(skill='code-review',
      args='xhigh go/internal/gateway/context.go go/internal/gateway/context_compaction.go go/internal/gateway/context_journal.go go/internal/gateway/context_display.go go/internal/gateway/context_estimate.go go/internal/gateway/count_tokens.go go/internal/gateway/messages.go go/internal/gateway/features.go')
```
`ultra`, `--fix`, `--comment`는 쓰지 않았다. 추가 서브에이전트도 띄우지 않았다.

### 로드 여부
로드·실행됐다. 도구 반환은 `Skill "code-review" completed (forked execution).`이었고 findings JSON 배열 **11건**을 돌려줬다.

### 내장 스킬과 미설치 플러그인 구분
이 세션이 쓴 것은 **CLI 내장 `code-review` 스킬**이다. 같은 이름의 marketplace 플러그인
`claude-plugins-official/code-review`와 `pr-review-toolkit`은 **설치되어 있지 않다**. 직접 확인했다:
`~/.claude/plugins/installed_plugins.json`의 설치 목록은
`claude-mem@thedotmack`, `codex@openai-codex`, `gopls-lsp@claude-plugins-official`,
`ponytail@ponytail`, `skill-creator@claude-plugins-official`,
`superpowers@superpowers-marketplace`, `typescript-lsp@claude-plugins-official` 7개뿐이고
`code-review` / `pr-review-toolkit` 문자열은 0회 등장한다.

### 관측된 effort / 모델 표기
**관측 근거 없음.** 스킬은 findings JSON 배열만 반환했고 level/effort/model 필드나 진행 로그를 남기지
않았다. 내가 `xhigh`를 인자로 넘겼다는 사실 외에 실제 적용된 effort나 모델을 이 세션에서 관측할
수단이 없었다. 반환 건수(11)와 지적의 깊이는 고effort와 모순되지 않지만, 그것은 추정이지 관측이
아니므로 근거로 세지 않는다.

### 스킬이 실제로 훑은 범위 vs 직접 리뷰 범위

| 스킬 인자로 준 제품 파일 | 스킬 지적 | 비고 |
|---|---|---|
| `context.go` | 2건 | 압축 journal 래치, checkContext 죽은 분기 |
| `context_compaction.go` | 2건 | conversationRequest 누락, 전처리 중복 |
| `context_estimate.go` | 1건 | `input_audio` 도달 불가 |
| `count_tokens.go` | 0건 (context_compaction 지적에 간접 포함) | |
| `messages.go` | 4건 | 전부 native turn 영수증 쪽 — A3 초점 밖(타 분야와 중복 가능)이라 본문에서 제외 |
| `features.go` | 1건 | workflow_agent applies 조건 |
| `context_journal.go` | **0건 — 미커버** | 직접 리뷰로 채움 |
| `context_display.go` | **0건 — 미커버** | 직접 리뷰로 채움 |

- 스킬은 인자에 없던 `go/internal/gateway/native_events.go`에 대한 지적 1건을 추가로 냈다(담당 범위 밖, 채택하지 않았다).
- 담당 파일 23개 중 **테스트 13개 + verification 3개는 스킬 인자에 없었고 전부 직접 리뷰**했다.
- 스킬 지적 11건 중 담당 초점에 해당하는 6건을 전부 코드로 재검증했다. 3건은 재현으로 확정(A3-01/02/03), 2건은 죽은 코드·중복으로 격하(A3-05/06), 1건은 그대로 채택(A3-04).
- 스킬이 내지 않았고 내가 직접 찾은 것: `estimate_media_test.go`의 테스트-구현 추종(A3-04에 병합), `diagnostics.go:121` 주석 표류(A3-04), ledger 인가 의심의 반박(A3-COMPACTION-USAGE-07), `s.route` 덮어쓰기 의심의 반박(A3-COMPACTION-USAGE-08), 계수/생성 payload 동일성 확인(A3-COMPACTION-USAGE-09), 영수증 소비 확인(A3-COMPACTION-USAGE-10), `+auto-compact` source 파급 반박(A3-COMPACTION-USAGE-11), web_search 클래스 게이트 확대(A3-COMPACTION-USAGE-12), count_tokens 클래스 게이트 부재(A3-COMPACTION-USAGE-13), `verification/v031-compaction-20260921/*` 3개 파일 대조와 mutation 1건 재현.

---

## 2. 커버리지 표 (담당 23개 파일)

| # | 파일 | 검토 방식 |
|---|---|---|
| 1 | `go/internal/app/context_resume_test.go` | diff 읽음 + 해당 native 테스트 2개 실행 |
| 2 | `go/internal/app/context_test.go` | diff 읽음 + fixture 구조 확인 + native 테스트 3개 실행 |
| 3 | `go/internal/gateway/client_capability_test.go` | 신규, 전체 읽음 + 실행 |
| 4 | `go/internal/gateway/compaction_effort_test.go` | 신규, 전체 읽음 + 실행 + mutation 1건 재현 |
| 5 | `go/internal/gateway/compaction_runtime_evidence_test.go` | 신규, 전체 읽음. 실행은 `NOT_RUN`(실제 backend 호출) |
| 6 | `go/internal/gateway/context.go` | 전체 읽음 + v0.3.0 대조 + panic probe |
| 7 | `go/internal/gateway/context_compaction.go` | 신규, 전체 읽음 + 재현 |
| 8 | `go/internal/gateway/context_display.go` | diff + 함수 전체(60–140행) 읽음 |
| 9 | `go/internal/gateway/context_estimate.go` | 전체 읽음 + part 생산자 전수 grep |
| 10 | `go/internal/gateway/context_journal.go` | 전체 읽음 + `projects.go` 대조 |
| 11 | `go/internal/gateway/context_journal_test.go` | diff 읽음 + 실행 |
| 12 | `go/internal/gateway/context_test.go` | diff + helper 전체 읽음 + 실행 |
| 13 | `go/internal/gateway/count_tokens.go` | 전체 읽음 + v0.3.0 대조 |
| 14 | `go/internal/gateway/count_tokens_test.go` | diff 읽음 + 실행 |
| 15 | `go/internal/gateway/estimate_media_test.go` | diff 읽음 + 실행 |
| 16 | `go/internal/gateway/features.go` | 전체 읽음 + v0.3.0 대조 + 재현 |
| 17 | `go/internal/gateway/features_test.go` | diff 읽음 + 실행 |
| 18 | `go/internal/gateway/messages.go` | diff 전체 + 1–260 / 270–345 / 800–930행 읽음 |
| 19 | `go/internal/gateway/messages_test.go` | diff 읽음 + 실행 |
| 20 | `go/internal/upstream/mixed_pdf_evidence_test.go` | diff 읽음. 실행은 `NOT_RUN`(`CLAUDUCT_MIXED_PDF_EVIDENCE=1` 실제 backend) |
| 21 | `verification/v031-compaction-20260921/REPORT.md` | 전체 읽음 + 주장 대조 |
| 22 | `verification/v031-compaction-20260921/live.json` | 전체 읽음 + 보고서 표와 대조 |
| 23 | `verification/v031-compaction-20260921/mutations.json` | 전체 읽음 + `.tmp/v031-compaction-mutations/` 10개 아티팩트 존재 확인 + 1건 재실행 |

미검토 담당 파일: **없음**.

### 호출자·피호출자 확인
변경된 계약의 호출처를 전수 grep했다.
`estimateTextInput`(context.go 1곳), `conversationRequest`(context.go 4곳, messages.go 1곳, parent_wait.go 1곳, results.go 1곳),
`beginContext`(messages.go 1곳), `previewCompaction`(count_tokens.go 1곳),
`compactRoute`·`stripCompactReceipts`·`compactReceipt`(context.go + context_compaction.go),
`addCompactGuidance`(messages.go, count_tokens.go), `featureApplies`(features.go + 테스트),
`saveContext`(context.go 6곳), `restoreContext`(context.go, context_compaction.go), `checkContext`(messages.go 1곳).

삭제된 보증 확인: v0.3.0 `beginContext`의 `CONTEXT_REQUEST_CLASS_UNVERIFIED` 반환은 사라진 것이
아니라 `handleMessages` 최상단으로 **이동**했다. 다만 그 이동 때문에 v0.3.0에서는 검사 대상이
아니던 `request.HostedSearch != nil` 경로까지 새로 덮게 됐다(A3-COMPACTION-USAGE-12).

---

## 3. 지적 목록

이하 본문에서 `A3-0n`은 `A3-COMPACTION-USAGE-0n`의 약칭이다. 전체 ID 목록: 01–03 confirmed, 04–06 improvement, 07–11 refuted(4절), 12–13 hold(5절).

### A3-COMPACTION-USAGE-01 — `previewCompaction`이 `conversationRequest` 게이트를 빠뜨려 계수와 생성이 갈린다
- **분류** confirmed / **심각도** medium / **출처** 스킬+직접 / **신규회귀**
- **파일** `go/internal/gateway/context_compaction.go:60-77` (61행 진입 조건, 66·72·76행 거부)
- **발생 조건** context policy On, `X-Claude-Code-Request-Class: auxiliary`(즉 `conversationRequest`가 false인 클래스)인데 마지막 user 텍스트가 native 압축 템플릿과 일치하는 본문.
- **원인** `beginContext`는 `conversationRequest(r, request)`가 false면 압축 판정을 아예 하지 않고 통과시킨다(`context.go:206-208`). `previewCompaction`은 그 게이트를 복제하지 않고 `nativeCompact || bridge.IsCompaction(request)`만으로 압축 경로에 들어간다. 그래서 `auxiliary` 요청이 영수증 검증(76행), 세션 신원(66행), 세션 등록(72행) 검사를 새로 받는다.
- **영향** 같은 본문·같은 헤더인데 `/v1/messages`는 200, `/v1/messages/count_tokens`는 400 `CONTEXT_COMPACTION_UNVERIFIED`. preflight 계수에 의존하는 클라이언트 동작이 생성과 어긋난다. 구조적으로는 두 진입점의 압축 판정 기준이 이제 서로 다르며, `conversationRequest`가 바뀌면 간격이 더 벌어진다.
- **재현 명령** 작업 디렉터리 `/d/AIDEV/clauduct-v031/go`, 환경 `CGO_ENABLED=0`
  ```
  go test -overlay=D:/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A3-compaction-usage/overlay.json \
    ./internal/gateway/ -run 'TestA3CountRefusesAuxiliaryCompactionTextThatGenerationRuns' -count=1 -timeout 300s -v
  ```
- **예상** (결함이 없다면) 생성 200 / 계수 200.
- **실제** `generation: status=200` / `count: status=400 body={"error":{"message":"CONTEXT_COMPACTION_UNVERIFIED",...}}`. 재현 테스트는 불일치를 통과 조건으로 쓰므로 exit 0.
- **기존결함 여부 판정** v0.3.0 `count_tokens.go`를 overlay로 되돌려 같은 테스트를 돌리면 `count: status=200 body={"input_tokens":1000}`이 나오고 재현 테스트는 FAIL(exit 1). → **v0.3.1 신규 회귀**.
- **증거 경로** `D:\AIDEV\clauduct-v031\verification\v031-code-review-20260922\run-01\repro\A3-compaction-usage\run-01-product.log`, 같은 폴더 `run-01-baseline.log`
- **권장 수정** `previewCompaction` 진입 조건에 `beginContext`와 같은 `conversationRequest(r, request)` 게이트를 넣는다. 더 나은 쪽은 A3-06대로 전처리를 공유 헬퍼로 빼서 양쪽이 같은 게이트를 쓰게 하는 것이다.
- **미확인** native 2.1.278이 실제로 `auxiliary` 클래스 요청에 압축 템플릿 본문을 담아 보내는 경우가 있는지 관측하지 못했다. 그래서 high가 아니라 medium이다. 결론에 필요한 것: 실제 TUI 세션에서 `auxiliary` 요청 본문 1회 캡처.

### A3-COMPACTION-USAGE-02 — 압축 admission 중 journal 쓰기 1회 실패가 세션을 영구 교착시킨다
- **분류** confirmed / **심각도** medium / **출처** 스킬+직접 / **기존결함(pre-existing)**
- **파일** `go/internal/gateway/context.go:284-287`
- **발생 조건** 압축 요청이 phase 검증을 통과해 `s.phase = "compacting"`을 설정한 직후 `g.saveContext(s)`가 한 번 실패(백신이 temp 파일을 rename 동안 붙들고 있음, 일시적 ACL 오류 등).
- **원인** 284행에서 phase를 먼저 바꾸고 285행 실패 시 `func(){}`를 release closure로 반환한다. `s.busy`는 292행에서야 true가 되므로 해제 경로가 없고, in-memory `s.phase`가 `"compacting"`에 래치된다. 이후 일반 요청은 289행에서 `CONTEXT_COMPACTION_REQUIRED`, 압축 요청은 265행의 수동 재시도 구제가 `failed`/`recount`만 인식하므로 268행에서 `CONTEXT_COMPACTION_UNVERIFIED`. 클라이언트는 "압축하라"는 지시를 받고 그 압축은 거부된다. `restoreContext`(`context_journal.go:110-111`)는 디스크의 `compacting`을 `failed`로 매핑해 구제할 줄 알지만 in-memory 경로에는 같은 구제가 없다.
- **영향** 프로세스 재시작 전까지 해당 session/agent가 생성도 압축도 못 한다. 일시적 파일 오류 1회로 세션 사망.
- **재현 명령** 작업 디렉터리 `/d/AIDEV/clauduct-v031/go`, 환경 `CGO_ENABLED=0`
  ```
  go test -overlay=D:/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A3-compaction-usage/overlay.json \
    ./internal/gateway/ -run 'TestA3JournalFailureDuringCompactionWedgesTheSession' -count=1 -timeout 300s -v
  ```
  재현은 journal 목적지 이름을 잠깐 디렉터리로 만들어 `root.Rename`을 실패시키고, 그 조건을 곧바로 완전히 제거한다.
- **예상** 일시 조건이 사라지면 다음 일반 요청 또는 수동 `/compact`로 복구된다.
- **실제**
  - 압축(쓰기 실패 중): `400 CONTEXT_JOURNAL_FAILED`, `phase="compacting"`
  - 조건 제거 후 일반 요청: `400 CONTEXT_COMPACTION_REQUIRED`
  - 조건 제거 후 새 영수증으로 수동 압축: `400 CONTEXT_COMPACTION_UNVERIFIED`, `phase="compacting"` 유지
  - 재시작 시뮬레이션(`restoreContext`): `phase="failed"` → 수동 압축 200으로 복구
- **기존결함 여부 판정** v0.3.0 `context.go`를 overlay로 되돌려 같은 테스트를 돌리면 **완전히 같은 4줄 로그와 PASS(exit 0)**. → **기존 결함**. 다만 v0.3.1이 형제 래치(`persistenceError`)에는 이미 손을 댔으므로(250행 재시도) 이제 남은 유일한 래치가 이것이다.
- **증거 경로** `...\repro\A3-compaction-usage\run-01-product.log`, `...\run-01-baseline.log`
- **권장 수정** 284–287행 순서를 뒤집어 `saveContext`가 성공한 뒤에만 `s.phase = "compacting"`을 쓰거나, 실패 시 진입 시 phase(또는 `failed`)로 되돌린다. 후자는 `restoreContext`의 `compacting → failed` 규칙을 in-memory에도 한 번만 적는 쪽이라 더 작다.
- **미확인** 실제 Windows 환경에서 백신·ACL로 인한 `Rename` 실패 빈도는 측정하지 않았다.

### A3-COMPACTION-USAGE-03 — `workflow_agent` 증거 카운터가 거부된 workflow 자식을 전부 놓친다
- **분류** confirmed / **심각도** low / **출처** 스킬+직접 / **신규회귀**
- **파일** `go/internal/gateway/features.go:98`
- **발생 조건** `X-Claude-Code-Request-Class`가 `workflow`가 아닌 workflow 자식 요청(예: `subagent`)이 `entry.route()` 이전 단계에서 거부될 때. 그 경로는 실재한다 — `delegation.go:416`은 `scope.workflow` 없이도 `bridge.CanonicalRole(binding.Role) == "workflow-subagent"`이면 workflow 라우트를 만든다.
- **원인** applies 조건이 `r.AgentRole == "workflow-subagent"`에서 `strings.HasPrefix(r.Source, "workflow-")`로 바뀌었다. `RequestRecord.Source`는 `record.route()`(`diagnostics.go:304-312`)에서만 채워지고 그 호출은 `messages.go:220` — `agentSelection`, `restrictWorkflowTools`, native turn 검사, `beginResult`, `beginContext` **뒤**다. 반면 `AgentRole`은 `agentSelection` 안에서 일찍 채워졌다.
- **영향** `AGENT_SELECTION_UNVERIFIED`, `WORKFLOW_TOOL_POLICY`, `NATIVE_TURN_UNVERIFIED`, `AGENT_RESULT_CAPACITY`, `CONTEXT_*`로 거부된 workflow 자식이 `workflow_agent`의 분모에서 완전히 빠진다(`Requests=0`, `Evidence=not_observed`). 예전에는 `Unconfirmed`로 잡혔다. "필수 검사 없이 실행된 workflow 자식"을 보고하라고 있는 카운터가 실패를 보지 못한다. 같은 요청이 `native_agent`에서는 여전히 `Unconfirmed=1`로 남으므로 완전 무증상은 아니다 → low.
- **재현 명령** 작업 디렉터리 `/d/AIDEV/clauduct-v031/go`, 환경 `CGO_ENABLED=0`
  ```
  go test -overlay=D:/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A3-compaction-usage/overlay.json \
    ./internal/gateway/ -run 'TestA3RefusedWorkflowChildDropsOutOfWorkflowFeatureEvidence' -count=1 -timeout 300s -v
  ```
- **예상** 거부된 workflow 자식이 `workflow_agent.Requests=1, Unconfirmed=1`로 잡힌다(v0.3.0 동작).
- **실제** `workflow_agent: requests=0 unconfirmed=0 evidence=not_observed` / `native_agent: requests=1 unconfirmed=1 evidence=conditions_not_confirmed_for_all_requests`
- **기존결함 여부 판정** `git show 149068e…:go/internal/gateway/features.go`의 같은 줄은 `r.AgentRole == "workflow-subagent" || r.RequestClass == "workflow"`이고 `AgentRole`은 거부 전에 설정된다. → **v0.3.1 신규 회귀**.
- **증거 경로** `...\repro\A3-compaction-usage\run-01-product.log`
- **권장 수정** 변경 의도(사용자 정의 role 이름이 `workflow-subagent`인 가짜 배제)는 타당하다. 의도를 지키면서 분모를 잃지 않으려면 거부 이전에 이미 확정되는 사실을 쓴다 — 예: `slices.Contains(r.VerifiedChecks, "workflow_selection") || r.RequestClass == "workflow"`, 또는 `agentSelection`에서 해결된 route의 Source를 record에 즉시 기록.
- **미확인** 실제 native가 workflow 자식에 `subagent` 클래스를 붙이는 빈도. `delegation.go:416` 분기의 존재가 근거지 실측은 아니다.

### A3-COMPACTION-USAGE-04 — `input_audio` 미디어 분기는 도달 불가이고, 테스트가 구현을 따라 바뀌었다
- **분류** improvement / **심각도** low / **출처** 스킬+직접 / **신규회귀**
- **파일** `go/internal/gateway/context_estimate.go:37`, `go/internal/gateway/estimate_media_test.go:18,37`, `go/internal/gateway/diagnostics.go:121`
- **발생 조건** 없음(도달 불가).
- **원인** `bridge.InputPart.Type`에 값을 쓰는 곳은 전수 grep 기준 `input_text` / `input_image` / `input_file`뿐이다(`bridge.go:376,401,409,461,466,469,473,476`, `search.go:121`, `documents.go:147`). `bridge/token_count.go:115`는 그 넷 밖의 타입을 계수 불가로 거부한다. `input_audio` part는 `estimateTextInput`에 도달할 수 없다.
- **영향** 기능 영향 없음. 문제는 둘이다. (a) 바로 위 주석이 "나중에 추가될 종류가 기본값으로 media가 되는 것"을 막으려고 이름을 열거한다고 해 놓고 존재하지 않는 종류를 추가해 그 규칙을 흐린다. (b) `estimate_media_test.go`가 원래 "이름 없는 종류" 반례로 쓰던 `input_audio`를 빼앗기고 `__clauduct_test_unknown__`이라는 합성 문자열로 교체됐다 — 테스트가 실제 동작 대신 구현을 따라 쓴 형태다. 덧붙여 `diagnostics.go:121` 주석은 여전히 "Media narrows Opaque to image and file parts"로 코드와 어긋난다.
- **재현 명령** `NOT_RUN` — 도달 불가 분기라는 주장 자체라 실패 재현이 성립하지 않는다. 근거는 전수 grep: 작업 디렉터리 `/d/AIDEV/clauduct-v031`, `grep -rn '"input_image"\|"input_file"\|"input_audio"\|"input_text"' go/internal/ --include=*.go`, exit 0.
- **예상 / 실제** 예상: 어딘가 `input_audio` 생산자가 있다. 실제: 생산자 0곳, `token_count.go:115`가 명시적으로 거부.
- **증거 경로** NONE (grep 결과를 본문에 인용)
- **권장 수정** 분기와 `diagnostics.go:121` 주석 중 하나를 맞춘다. 생산자 없이 둘 거면 `context_estimate.go` 분기를 되돌리고 테스트 반례도 `input_audio`로 복원하는 쪽이 작다. audio를 지원할 거면 생산자와 `token_count.go` 계수 분기를 같은 변경에 넣어야 한다.
- **미확인** native/backend가 가까운 릴리스에서 audio part를 실제로 보낼 계획이 있는지.

### A3-COMPACTION-USAGE-05 — `checkContext`의 압축 분기가 죽은 코드이고, 되살아나면 상한을 세션 라우트로 굳힌다
- **분류** improvement / **심각도** low / **출처** 스킬+직접 / **신규회귀**
- **파일** `go/internal/gateway/context.go:363-366`
- **발생 조건** 없음(도달 불가).
- **원인** `entry.snapshot().Kind == "compaction"`이 되려면 `beginContext`의 압축 분기가 288행 `entry.kind("compaction")`까지 갔다는 뜻이고, 그 분기는 278행에서 `s.route = route`를 무조건 실행한다. 따라서 364행 `s.route.Model == ""`는 성립할 수 없다. v0.3.0에는 `beginContext`에 `s.route` 대입이 없어 이 분기가 초기화 역할을 했다.
- **영향** 지금은 없다. 다만 되살아나면 365행이 `built.Effort.Effort`(자동 압축이면 상한 적용된 `medium`)와 `built.Source`(`…+auto-compact`)를 세션 라우트로 저장해 이슈 #56의 "이후 생성은 원래 effort" 정책을 정면으로 깬다. 남겨 두면 위험한 죽은 경로다.
- **재현 명령** 작업 디렉터리 `/d/AIDEV/clauduct-v031/go`, 환경 `CGO_ENABLED=0`. 해당 분기 본문에 `panic(...)`을 넣은 overlay로 gateway 패키지 전체 실행:
  ```
  go test -overlay=D:/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A3-compaction-usage/overlay-probe-context.json \
    ./internal/gateway/ -count=1 -timeout 420s
  ```
- **예상** 압축 관련 테스트 어딘가에서 panic.
- **실제** `ok github.com/wotjr1649/Clauduct/go/internal/gateway 35.732s`, exit 0 — 패키지 전체에서 한 번도 도달하지 않았다.
- **증거 경로** `...\repro\A3-compaction-usage\run-01-baseline.log` (마지막 절), overlay `...\overlay-probe-context.json`, 변형본 `...\probe-context.go`
- **권장 수정** 363–366행의 lazy 초기화를 삭제하고 `return checkpoint()`만 남긴다.
- **미확인** 패키지 테스트가 도달시키지 못했다는 것이 모든 실행 경로에서 도달 불가라는 증명은 아니다. 위의 코드 논증이 이를 보강한다.

### A3-COMPACTION-USAGE-06 — `previewCompaction`이 `beginContext` 전처리를 통째로 복제한다
- **분류** improvement / **심각도** low / **출처** 스킬+직접 / **신규회귀**
- **파일** `go/internal/gateway/context_compaction.go:63-91` vs `go/internal/gateway/context.go:209-279`
- **발생 조건** 상시(코드 구조).
- **원인** 신원 검사, `c.sessions[session]` 검사, `c.states[contextKey(...)]` 조회, `restoreContext` 폴백, `if s.route.Model != "" { override = … }`, `bridge.ResolveRoute`, `compactRoute`가 문장 단위로 두 번 적혀 있다. 이미 한 군데(`conversationRequest`)에서 어긋났고 그 어긋남이 A3-01이다.
- **영향** (a) 전처리를 바꿀 때마다 두 곳을 손으로 맞춰야 하고 어긋남이 조용히 통과한다. (b) 압축 계수마다 `contexts.mu`를 잡은 채 디스크에서 journal을 다시 읽고 파싱한 뒤 그 상태를 버린다(캐시 안 함). 계수 1회당 open/stat/read/JSON 파싱 1세트가 admission 직렬화 락 안에서 일어난다.
- **재현 명령** `NOT_RUN` — 구조 지적이라 실패 재현이 없다. 근거는 두 함수의 나란한 비교(위 행 범위).
- **예상 / 실제** 해당 없음.
- **증거 경로** NONE
- **권장 수정** `resolveCompactionRoute(session, agent, request, override)` 같은 공유 헬퍼로 공통부를 뽑고, `beginContext`만 상태 변경·영수증 소비·journal 저장을 이어서 한다.
- **미확인** 실제 부하에서 락 안 디스크 I/O가 유의미한 지연을 만드는지 측정하지 않았다.

---

## 4. 반박된 지적과 반박 근거

### A3-COMPACTION-USAGE-07 — "자동 압축의 medium 상한이 ledger의 route 인가를 어겨 `ROUTE_NOT_AUTHORISED`를 낸다"
**반박.** `upstream.Ledger.Reserve`의 모델/effort 대조는 `!l.budget.Unrestricted`일 때만 돈다(`budget.go:128-140`). 제품 경로는 `go/internal/app/run.go:446`에서 `upstream.NewLedger(upstream.Unlimited())`를 쓰고 `Unlimited()`는 `Budget{Unrestricted: true}`다(`budget.go:52`). 고정 route ledger를 쓰는 곳은 테스트와 `cmd/clauduct-dev/probe.go:138`뿐이며, live 증거 테스트는 high/medium 두 ledger를 따로 두어 이미 대비돼 있다.

### A3-COMPACTION-USAGE-08 — "`context.go:278`의 `s.route = route`가 세션 라우트를 압축 요청 라우트로 덮어써 이후 생성 effort가 바뀐다"
**반박.** `bridge.ResolveRoute`는 override가 있으면 `override[0]`을 그대로 돌려준다(`bridge.go:288-291`). 271–273행이 `s.route`가 비지 않았을 때 override를 `s.route`로 놓으므로 278행은 항등 대입이고, 비어 있을 때만 새로 채운다. 상한은 279행에서 `compactRoute`가 만든 **복사본**에만 걸린다(`context_compaction.go:31-39`, 값 수신자). `TestAutomaticCompactionCapsOnlyItsRequest`가 5개 effort × auto/manual/미지정 전부에서 `state.route.Effort != effort`를 잡는다. `mutations.json`의 `stored_route_overwritten`이 이를 검출하며, 나는 표본으로 `automatic_cap_removed` 변이를 재실행해 기록된 failure 3줄과 동일한 출력(exit 1)을 확인했다.

### A3-COMPACTION-USAGE-09 — "압축 계수 payload와 생성 payload가 어긋나 `prior-count-cache`가 붙지 않는다"
**반박.** 두 경로의 순서가 같다 — override 결정 → `BuildRequest` → (compact면) `describeWorkflowStep` 생략 → `describe` → `addCompactGuidance` → `prepareDocuments` → `json.Marshal`. `TestCompactionCountMatchesGenerationWithoutConsumingReceipt`가 `f.LastRequest() != counted`로 바이트 동일성을 직접 잡고 `CountSource=="prior-count-cache" && CountAgreement=="matched"`를 확인한다. 실행 PASS. `live.json`의 `count_source: prior-count-cache / count_agreement: matched`와도 일치한다.

### A3-COMPACTION-USAGE-10 — "영수증이 검증 실패해도 소비되거나 만료 영수증이 다른 요청에 먹힌다"(v0.3.0 결함의 잔존 의심)
**반박.** `context.go:280-282`가 `if validReceipt`일 때만 `delete`한다. `previewCompaction`은 아예 삭제하지 않는다. `TestCompactionRequiresVerifiedAutomaticReceiptForCap`이 missing/expired/other-session/other-agent 4케이스 × count/generation 2회에서 `len(g.contexts.tickets) != 1`을 잡는다. `mutations.json`의 `invalid_receipt_consumed`가 그 단언으로 검출된다. 실행 PASS.

### A3-COMPACTION-USAGE-11 — "`+auto-compact` source 접미사가 하류 소비자를 깨뜨린다"
**반박.** source를 정확히 비교하는 곳은 `delegation.go:712`의 선택 journal 허용 목록, `messages.go:312`의 `HasPrefix("workflow-") || == "native-selection"`, `workflow_checkpoint.go:277`뿐이고 모두 자식 선택 경로라 압축 override가 지나가지 않는다. `saveContext`는 Source를 저장하지 않고(`contextJournal`에 필드 없음) 복원 시 `bridge.SelectRoute`가 새로 만든다.

---

## 5. 보류 항목 (결론에 필요한 것)

### A3-COMPACTION-USAGE-12 — 새 클래스 게이트가 web_search 경로까지 덮는다
`go/internal/gateway/messages.go:46-50`. v0.3.0에서는 `CONTEXT_REQUEST_CLASS_UNVERIFIED` 판정이 `beginContext` 안에 있었고 `beginContext`는 `request.HostedSearch != nil` 조기 반환(`messages.go:147-156`) **뒤**였다. 즉 검색 요청은 클래스 헤더를 요구받지 않았다. 이제는 핸들러 최상단이라 검색도 요구받는다.
**필요한 것**: native 2.1.278이 hosted `web_search` 요청에 `X-Claude-Code-Request-Class`를 붙이는지 실제 세션 1회 캡처. 붙이지 않으면 context policy On에서 web_search가 전면 400이 된다. 기존 테스트 helper `messages()`가 기본으로 클래스를 붙이므로 이 경우를 잡는 테스트가 없다.

### A3-COMPACTION-USAGE-13 — `count_tokens`에는 클래스 게이트가 없다
`handleCountTokens`에는 `CONTEXT_REQUEST_CLASS_UNVERIFIED` 검사가 없다. 헤더를 못 보내는 구형 클라이언트는 preflight 계수는 200으로 통과하고 생성만 400으로 죽는다. `docs/v2/COMPATIBILITY.md:132`는 "context policy의 헤더 요구"라고만 하고 엔드포인트를 좁히지 않는다.
**필요한 것**: 이 비대칭이 의도인지(계수는 무해하므로 막지 않는다) 누락인지 설계 확인. 의도라면 COMPATIBILITY.md에 엔드포인트 범위를 한 줄 적으면 끝난다.

---

## 6. 개선 제안

1. **A3-06의 공유 헬퍼**가 A3-01을 구조적으로 없앤다. 게이트를 한 번만 적으면 두 진입점이 어긋날 수 없다.
2. **A3-02는 `saveContext` 성공 후 phase 커밋**으로 잡는 편이 작다. 같은 패턴이 `recoverContextOverflow`(435-443행)와 release closure(312-315행)에도 있지만 그 둘은 실패를 진단으로 남기거나 `brokeAfterCommitting`으로 표시해 이미 일관적이다. 압축 admission만 예외다.
3. **`estimate_media_test.go`의 반례 복원.** `__clauduct_test_unknown__`은 계약을 지키긴 하지만, 실제 프로토콜에 나타날 법한 이름을 반례로 두는 편이 규칙의 의도에 맞는다(A3-04와 함께 처리).
4. **`diagnostics.go:121` 주석**을 코드에 맞춘다(또는 A3-04대로 코드를 되돌린다). 지금은 둘이 어긋나 있다.
5. **`previewCompaction`이 복원한 상태를 버리는 대신** 짧게 캐시하면 압축 계수마다의 디스크 재파싱을 없앨 수 있다. 단 "계수는 상태를 만들지 않는다"는 현 불변식(테스트가 `len(states) != 0`으로 강제)을 깨지 않는 형태여야 한다.
6. **`verification/v031-compaction-20260921/REPORT.md`** 는 검증 범위와 한계를 정확히 적고 있고 mutation 아티팩트 10개도 `.tmp/v031-compaction-mutations/`에 실재한다. 다만 보고서 본문 bullet은 변이를 "계수 지침 제거"로 부르고 `mutations.json`은 `count_guidance_omitted`로 부른다 — 이름을 맞추면 대조가 쉬워진다. (결함 아님.)

---

## 7. 근거 부족으로 뺀 의심

- **`previewCompaction`이 `contexts.mu`를 잡은 채 디스크 I/O를 해서 교착이 가능하다.** 락 순서는 `contexts.mu → delegations.mu` 단방향으로만 확인됐고(`context_journal.go:44-49`) 역순 경로를 찾지 못했다. 성능 측면은 A3-06에 포함했다.
- **`stripCompactReceipts`가 정규식 전역 치환으로 바뀌어 사용자 본문의 우연한 토큰을 지운다.** 패턴 `\[clauduct-compact:([A-Za-z0-9_-]{43})\]`가 매우 좁아 현실적인 충돌 입력을 만들지 못했다. 오히려 v0.3.0의 리터럴 치환보다 전사 표식 제거 측면에서 낫다.
- **압축 티켓 128개 상한 소진.** `handleContextEvent`가 매 이벤트마다 5분 초과분을 먼저 지우므로(`context.go:175-179`) 누적 시나리오를 만들지 못했다.
- **`trigger` 미지정 PreCompact가 상한을 회피한다.** `handleContextEvent:151`이 빈 trigger를 허용하고 `compactRoute`는 `auto`일 때만 상한을 건다. `TestAutomaticCompactionCapsOnlyItsRequest`가 `""` 케이스를 의도된 무상한으로 명시 검사하고 있고, 훅이 trigger를 항상 채우는지 확인할 실측이 없었다.
- **native 압축 요청에 영수증이 없으면(훅 미동작) 자동 압축도 상한을 받지 않는다.** `nativeCompact`만으로 압축은 허용되지만 상한은 검증된 `auto` 영수증을 요구한다. REPORT.md가 "수동 압축, trigger 미지정, 영수증 부재·만료…는 effort를 낮추지 않는다"로 명시한 설계 구분이라 결함으로 올리지 않았다.
- **`REQUEST_BUDGET` / `ROUTE_NOT_AUTHORISED`의 400 매핑이 부적절하다.** 제품 경로는 Unrestricted ledger라 발생하지 않고(A3-COMPACTION-USAGE-07), 400은 5xx 재시도 폭주를 막는 기존 `statusForUpstream` 원칙과 일치한다. 새 테스트는 오히려 wrapping 문맥("private context") 누출까지 막아 단언이 강해졌다.

---

## 8. 실행한 명령 전체 목록 (exit code 포함)

| # | 명령 (요약) | 작업 디렉터리 | 환경 | exit |
|---|---|---|---|---|
| 1 | `git status --porcelain=v1` | `/d/AIDEV/clauduct-v031` | — | 0 |
| 2 | `git rev-parse HEAD` | 루트 | — | 0 |
| 3 | `git diff --stat -- <담당 23개 경로>` | 루트 | — | 0 |
| 4 | `git diff -- context.go context_compaction.go` | 루트 | — | 0 |
| 5 | `git diff -- context_journal.go context_display.go context_estimate.go count_tokens.go features.go` | 루트 | — | 0 |
| 6 | `git diff -- messages.go` | 루트 | — | 0 |
| 7 | `git diff -- <담당 테스트 9개>` | 루트 | — | 0 |
| 8 | `git show 149068e…:go/internal/gateway/features.go` / `:context.go` / `:count_tokens.go` | 루트 | — | 0 |
| 9 | `go test ./internal/gateway/ -run 'TestCompaction\|TestAutomaticCompaction\|TestContextPolicy\|TestWorkflowEvidence\|TestReasoningIsOpaque\|TestMissingRequestClass\|TestRequestClassCapability\|TestRequestClassRequirement\|TestRepeatedSessionRegistration' -count=1 -timeout 300s` | `go` | `CGO_ENABLED=0` | 0 |
| 10 | `go test -overlay=…/overlay.json ./internal/gateway/ -run 'TestA3' -count=1 -timeout 300s -v` | `go` | `CGO_ENABLED=0` | 0 |
| 11 | `go test -overlay=…/overlay-base-count.json ./internal/gateway/ -run 'TestA3CountRefuses…' -count=1 -timeout 300s -v` | `go` | `CGO_ENABLED=0` | **1 (의도된 FAIL: 기준 버전에는 결함 없음)** |
| 12 | `go test -overlay=…/overlay-base-context.json ./internal/gateway/ -run 'TestA3JournalFailure…' -count=1 -timeout 300s -v` | `go` | `CGO_ENABLED=0` | 0 (기준 버전에서도 교착 → 기존 결함) |
| 13 | `go test -overlay=.tmp/v031-compaction-mutations/automatic_cap_removed.json ./internal/gateway/ -run '^TestAutomaticCompactionCapsOnlyItsRequest/auto/' -count=1 -timeout 300s` | `go` | `CGO_ENABLED=0` | **1 (의도된 FAIL: mutations.json 기록과 동일)** |
| 14 | `go test -overlay=…/overlay-probe-context.json ./internal/gateway/ -count=1 -timeout 420s` (죽은 분기 panic probe, 패키지 전체) | `go` | `CGO_ENABLED=0` | 0 (panic 없음, 35.732s) |
| 15 | `go test ./internal/app/ -run 'TestNativeMeasuredUsageTriggersPreventiveCompaction' -count=1 -timeout 420s` | `go` | `CGO_ENABLED=0` | 0 (21.998s) |
| 16 | `go test ./internal/app/ -run 'TestNativeExactCompactionFailureDoesNotResume\|TestNativeExactCompactionKeepsParallelAgentsSeparate\|TestNativeResumeSwitchCompactsOnPersistedOldModelAtDestinationThreshold\|TestNativeResumeSwitchWithOneShortTurn' -count=1 -timeout 600s` | `go` | `CGO_ENABLED=0` | 0 (29.412s) |
| 17 | `go test ./internal/gateway/ -run 'Compact\|Context\|Count\|Feature\|Estimate\|Media\|Journal\|Workflow\|RequestClass\|Usage' -count=1 -timeout 300s` | `go` | `CGO_ENABLED=0` | 0 (9.855s) |
| 18 | `go test -race ./internal/gateway/ -run 'Compact\|Context\|Count\|Estimate\|Journal' -count=1 -timeout 420s` | `go` | `CGO_ENABLED=1 CC=C:/msys64/ucrt64/bin/gcc.exe` | 0 (8.775s) |
| 19 | `grep -rn '"input_image"\|"input_file"\|"input_audio"\|"input_text"' go/internal/ --include=*.go` (part 생산자 전수) | 루트 | — | 0 |
| 20 | 변경 함수 12개 호출처 전수 grep | 루트 | — | 0 |
| 21 | `cat ~/.claude/plugins/installed_plugins.json` + 설치 플러그인 이름 추출 | 홈 | — | 0 |
| 22 | `git status --porcelain=v1` (종료 시 무변경 확인) | 루트 | — | 0 |

`go`는 `/d/AIDEV/clauduct-v031/go`, `루트`는 `/d/AIDEV/clauduct-v031`. overlay 경로 생략 표기 `…`는 전부 `D:/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A3-compaction-usage/`.

### NOT_RUN

| 검사 | 이유 | 추가 확인 방법 |
|---|---|---|
| `TestRuntimeEvidenceAutomaticCompactionEffort` / `…LongCompactionEffort` (build tag `runtime_evidence` + `CLAUDUCT_EVIDENCE_LIVE=1`) | 실제 제품 backend에 과금 요청을 보낸다. 리뷰 권한으로 새 외부 호출을 하지 않는다. | 별도 승인 후 `CGO_ENABLED=0 go test -tags runtime_evidence ./internal/gateway/ -run TestRuntimeEvidenceAutomaticCompactionEffort -count=1`을 1회. 기존 근거는 `verification/v031-compaction-20260921/live.json`. |
| `TestPublicMixedPDFCount` / `TestPublicMixedPDFCountLuna` (`CLAUDUCT_MIXED_PDF_EVIDENCE=1`) | 동일 — 실제 backend 호출 + ledger 2회. | 동일 조건 1회 실행. 이번 변경은 기존 함수를 `publicMixedPDFCount(t, model)`로 뽑고 luna 케이스를 추가한 것뿐이고 단언 약화는 없다(정적 확인 완료). |
| 전체 `go test ./...` 회귀 | 지시상 통합 검토자 판단 사항. | 통합 단계에서 1회. |
| native web_search 요청 헤더 관측 (A3-COMPACTION-USAGE-12) | 실제 TUI/SDK 세션과 네트워크가 필요. | 실제 세션 1회에서 gateway 진단 `Recent[]` 중 `Kind=="web_search"` 레코드의 `RequestClass` 확인. |
| `mutations.json` 나머지 9건 재실행 | 표본 1건(`automatic_cap_removed`)만 재현했다. 아티팩트 10개 존재는 확인. | 각 `.json` overlay로 기록된 `-run` 정규식을 동일 실행. |

---

## 재현 자산 목록

`D:\AIDEV\clauduct-v031\verification\v031-code-review-20260922\run-01\repro\A3-compaction-usage\`

| 파일 | 용도 |
|---|---|
| `a3_repro_test.go` | A3-01/02/03 재현 테스트 3개 (overlay로만 패키지에 주입) |
| `overlay.json` | 제품 트리 + 재현 테스트 |
| `base-context.go`, `overlay-base-context.json` | v0.3.0 `context.go` 대조용 (A3-02 origin 판정) |
| `base-count_tokens.go`, `overlay-base-count.json` | v0.3.0 `count_tokens.go` 대조용 (A3-01 origin 판정) |
| `probe-context.go`, `overlay-probe-context.json` | A3-05 죽은 분기 panic probe |
| `run-01-product.log` | 제품 트리 재현 3건 전체 출력 |
| `run-01-baseline.log` | v0.3.0 대조 2건 + 죽은 분기 probe 출력 |
