# v0.3.1 전체 변경 코드 리뷰 — 통합 보고서

작성: 통합 검토자(Clauduct를 거치지 않는 Claude Code 세션). 2026-09-22.
출력 루트: `D:\AIDEV\clauduct-v031\verification\v031-code-review-20260922\run-01`

---

## 1. 판정

### `CHANGES_REQUIRED`

확정 결함 **18건**(high 3, medium 10, low 5)이 있다.
기원별로 **신규 회귀 15건 / 혼합 1건(A2-02) / 기존 결함 2건(A3-02, A5-03)** 이다.
동시에 근거 있는 보류 **5건**과 미실행 필수 검사 2종이 남아 `HOLD` 사유도 함께 성립한다.
인계 문서의 규칙대로 **둘 다 남긴다**. 확정 결함이 있으므로 최종 판정은 `CHANGES_REQUIRED`다.

### 리뷰 수행 완료 여부

| 항목 | 상태 |
|---|---|
| 리뷰 대상 118개 파일 커버리지 | **완료** — 미검토 0개 (5개 분야에 정확히 1회씩 배정, 종료 시점 `git diff --name-only`도 같은 118개) |
| 분야별 병렬 리뷰 + `/code-review xhigh` | **완료** — 5/5 분야, 전부 스킬 호출 성공 |
| 최소 재현 | **완료** — 확정 결함 대부분이 `go test -overlay` 기반 재현과 기준 commit 대조를 가진다 |
| 적대 검증 | **부분** — 분야당 상한 4건으로 16건만 검증(2관점씩 32 에이전트). 미검증 10 ID는 §2.3에 명시 |
| 통합 검토 | **완료** — 연결부 5개 직접 검토, 신규 지적 6건, 강등 7 ID, 승격 2건, 병합 5묶음 |
| 전체 회귀 | **2회 실행. 1회 실패 / 1회 성공 — 비결정적이다** (§2.4, §5.1a) |
| `-race ./...` 전체 | **NOT_RUN** |
| 태그 뒤 실호출 검사(`runtime_evidence`) 실행 | **NOT_RUN** (컴파일·vet은 실행, exit 0) |

**제품을 한 줄도 고치지 않았다.** 쓰기는 이 출력 루트 안에서만 했다.

---

## 2. 대상과 실행

### 2.1 대상

| 항목 | 값 |
|---|---|
| 리뷰 대상 root | `D:\AIDEV\clauduct-v031` (git worktree, branch `fix/v031`) |
| 기준 commit | `149068edd693fb860a03244a2ea15764bcd68c34` (v0.3.0) |
| HEAD | 기준 commit과 동일. v0.3.1 변경 전체가 **미커밋 작업트리**에 있다 |
| 변경 규모 | **118 files changed, +7895 / −636** |
| 새 파일 | intent-to-add (index blob = empty). 따라서 인자 없는 `git diff` 한 번이 리뷰 대상과 정확히 일치한다 |
| 삭제 파일 | 0개 |
| manifest | `manifest-start.json` / `manifest-end.json`. 파일별 상태·base blob·worktree SHA256·담당 분야 |
| 환경 | Windows 11 Pro 26200 / `go1.27.1 windows/amd64` / 관측 native `2.1.278` |
| 제품 버전 상수 | `go/internal/buildinfo/buildinfo.go:52` = `"0.3.0"` (미변경. 문서상 "v0.3.1 개발본·미출하"와 정합) |

`verification/v031-code-review-20260922/` 아래 이번 리뷰 산출물은 **대상 소스가 아니다.**
manifest는 이 경로를 제외한 118개만 기록한다.

### 2.2 리뷰 실행 환경

| 항목 | 확인한 값 |
|---|---|
| Claude Code | `2.1.278 (Claude Code)` (`claude --version`) |
| 모델 | 세션에 선택된 모델을 그대로 사용. 모델 override를 지정하지 않았으므로 상위 세션과 43개 하위 에이전트가 동일 모델을 상속했다 |
| effort | 요청 `xhigh`. **실제 적용 값을 기계적으로 확인할 수단이 세션에 없다.** `/code-review` 스킬도 반환 payload에 effort를 표기하지 않는다. 요청값만 기록하고 실제값은 미확인으로 남긴다 |
| `/code-review` | **CLI 내장 스킬**. 같은 이름의 marketplace 플러그인 `claude-plugins-official/code-review`와 `pr-review-toolkit`은 카탈로그(`~/.claude/plugins/marketplaces/`)에만 있고 `~/.claude/plugins/installed_plugins.json`에 **없다(미설치)**. 이번 리뷰가 호출한 것은 내장 스킬이다 |
| 호출 형태 | 각 분야가 `Skill(skill='code-review', args='xhigh <담당 제품 경로들>')`. `ultra`·`--fix`·`--comment` 미사용 |

### 2.3 Workflow 실행

| 항목 | 값 |
|---|---|
| Run ID | `wf_61ab1ba6-494` |
| Task ID | `w6rmdoesi` |
| transcript | `C:\Users\js\.claude\projects\D--AIDEV-clauduct-v031\a0616359-5dc7-45d4-9eb3-fd1b761c4af7\subagents\workflows\wf_61ab1ba6-494` |
| 에이전트 | **43개 완료 / 실패 0 / skip 0 / 빈 결과 0**, 약 6,932초, 하위 토큰 5,000,087 |
| 단계 | Review(5) → Verify(32) → Integrate(2) |

| 분야 | 결과 파일 | 완료 | `/code-review` | 담당 파일 | 미검토 |
|---|---|---|---|---|---|
| A1 역할·선택·인자 | `areas/A1-roles-selection-args.md` | 예 | 호출됨 | 22 | 0 |
| A2 세션·턴·복구 | `areas/A2-session-turn-recovery.md` | 예 | 호출됨 | 33 | 0 |
| A3 압축·사용량 | `areas/A3-compaction-usage.md` | 예 | 호출됨 | 23 | 0 |
| A4 전송·프로세스 | `areas/A4-transport-process.md` | 예 | 호출됨 | 26 | 0 |
| A5 검증·호환성 | `areas/A5-verification-compat.md` | 예 | 호출됨 | 14 | 0 |
| 통합 검토 | `areas/INTEGRATION.md` | 예 | (해당 없음) | 연결부 5 | — |
| 커버리지 비판 | `areas/COVERAGE-CRITIC.md` | 예 | (해당 없음) | manifest 전수 | — |

적대 검증 상한(분야당 4건)으로 **10 ID가 미검증**으로 남았다: A1-05, A1-06, A1-20, A1-21, A1-22, A1-23,
A2-13, A3-COMPACTION-USAGE-13, A5-VERIFICATION-COMPAT-05, A5-VERIFICATION-COMPAT-06.
이 절단은 Workflow 로그에도 기록돼 있다(조용한 truncation이 아니다).

### 2.4 실행한 검사 (통합 단계)

작업 디렉터리 `D:/AIDEV/clauduct-v031/go`, 환경 `CGO_ENABLED=0`.

| 명령 | 예상 | 실제 | exit |
|---|---|---|---|
| `go test ./... -count=1 -timeout 720s` — 통합 검토 에이전트 | 전 패키지 ok | `internal/gateway` **1건 FAIL**, 나머지 16 ok | **1** |
| `go test ./... -count=1 -timeout 900s` — 통합 보고자(본 문서) 재실행 | 재현 여부 확인 | **17개 패키지 전부 ok** (`internal/app` 263.674s, `internal/gateway` 38.499s) | **0** |
| `go vet ./...` | 출력 없음 | 출력 없음 | 0 |
| `go vet -tags runtime_evidence ./...` | 태그 파일도 컴파일 | 출력 없음 | 0 |
| `gofmt -l .` | 출력 없음 | 출력 없음 | 0 |
| `CGO_ENABLED=0 go build ./...` | 성공 | 성공 | 0 |
| `go test -list '.*' ./internal/app/ ./internal/gateway/` (태그 유무 대조) | 차이 확인 | 463 → 473. **태그로만 보이는 테스트 10개** | 0 |
| `go test -tags runtime_evidence -run 'TestTUIAcceptance…|TestIntegrationAudit…' ./internal/app/` | 환경 없이 통과하는지 | `ok 0.111s` | 0 |
| `go test ./internal/gateway/ -run 'TestGatewayResponsesSurviveConnectionTurnover' -count=1` ×3 | 단독 재현 | 3회 전부 ok | 0 |

통합 검토 에이전트가 관측한 실패 원문:

```
--- FAIL: TestGatewayResponsesSurviveConnectionTurnover (10.33s)
    --- FAIL: TestGatewayResponsesSurviveConnectionTurnover/closed/stream (5.47s)
        connection_test.go:75: invalid SSE JSON
FAIL	github.com/wotjr1649/Clauduct/go/internal/gateway	40.950s
```

**같은 명령이 한 번은 실패하고 한 번은 통과했다.** 이것이 INT-01의 본질이며, 뒤의 통과를
"앞의 실패가 오류였다"로 읽으면 안 된다. 검사가 실패 원인을 기록하지 않으므로
앞 실패가 제품 절단이었는지 검사 자신의 2초 client timeout이었는지 **지금 트리에서는 판정할 수 없다**.

### 2.5 무결성

| 시점 | 결과 |
|---|---|
| 시작 | 118개 파일 SHA256 기록 (`manifest-start.json`) |
| 중간 | 재계산 결과 drift **0** |
| 종료 | 재계산 결과 drift **0** (`manifest-end.json`, 118/118 일치) |

`git add|commit|stash|checkout|reset|restore|clean|rebase`를 한 번도 쓰지 않았다. 읽기 전용 git 명령만 썼다.
원본 수정이 필요한 재현은 전부 `go test -overlay`로 주입했다. 작업트리 루트의 미추적 `%SystemDrive%/`는 건드리지 않았다.

---

## 3. 확정 결함

신규 회귀(v0.3.1이 만든 것)와 기존 결함을 구분한다. 심각도는 통합 조정 후 값이다.

| ID | 심각도 | 기원 | 요약 | 파일 |
|---|---|---|---|---|
| A1-01 | **high** | 신규 | 읽지 못한 일반 역할 파일 1개가 세션의 **모든 미매칭 역할**(내장 역할 포함)을 거부시킨다 | `go/internal/app/roles.go:169`, `:68-74` |
| A1-03 | **high** | 신규 | pending 형제 하나가 실제 workflow 자식을 `workflowRoute`에서 떼어내 거부시킨다 | `go/internal/gateway/delegation.go:408-422` |
| A2-01 | **high** | 신규 | 고정된 옛 턴 영수증이 더 새로운 `awaiting_children` 항목을 파괴해 이어가기와 부모 보고를 잃는다 | `go/internal/gateway/native_events.go:245-272` |
| A1-02 | medium | 신규 | 결합 short 옵션(`-cp` 등) + `--settings`이면 세션이 `SETTINGS_INVALID`로 시작조차 못 한다 | `go/internal/app/native_args.go:34-36` |
| A2-02 | medium | 혼합 | `begin()`의 포인터 교체가 완료 closure를 고아로 만들어 새 턴이 옛 턴의 stop payload로 정산된다 | `go/internal/gateway/results.go:177-187` |
| A3-01 | medium | 신규 | `previewCompaction`이 `conversationRequest` 게이트를 빠뜨려 같은 요청을 생성 200 / 계수 400으로 처리한다 | `go/internal/gateway/context_compaction.go:60-77` |
| A3-02 | medium | **기존** | 압축 admission 중 journal 쓰기 1회 실패가 phase를 `compacting`에 래치해 세션을 영구 교착시킨다 | `go/internal/gateway/context.go:284-287` |
| A3-03 | medium | 신규 | `workflow_agent` 증거 카운터가 `entry.route()` 이전에 거부된 workflow 자식을 전부 놓친다 | `go/internal/gateway/features.go:98` |
| A4-01 | medium | 신규 | `responseConn`이 `net.Conn` **인터페이스**를 임베드해 `CloseWrite()`를 숨긴다 — net/http의 RST 회피 half-close가 사라졌다 | 검토 당시 `go/internal/gateway/connection.go` 33–39행 (현행 구현은 `httpguard`로 이동) |
| A5-01 | medium | 신규 | recheck 기계 기록의 SHA256이 출하 트리와 불일치 — 보호 On/Off 대조의 "동일 코드" 주장이 감사 불가 | `verification/v031-recheck-20260922/evidence.json` |
| A5-03 | medium | **기존** | `runtime_evidence` 태그 파일이 CI에서 컴파일조차 되지 않는다 | `.github/workflows/go.yml` (대조: 태그 파일 5개) |
| INT-01 | medium | 신규 | 전체 회귀가 비결정적이고, 실패가 제품 절단인지 검사 자신의 timeout인지 구분할 증거를 버린다 | `go/internal/gateway/connection_test.go:60-75` |
| INT-02 | medium | 신규 | A1-03이 만드는 거부 집합과 A3-03이 놓치는 집합이 **같다** — 회귀와 그 탐지기가 동시에 꺼졌다 | `delegation.go:416` × `features.go:98` |
| A1-05 | low | 신규 | plugin 정의의 frontmatter를 읽지 못하면 그 역할이 **오류 없이** "부재"로 답해진다 | `go/internal/app/roles.go:169` |
| A1-06 | low | 신규 | projects 트리 자체가 없을 때 `loadChoice`가 custom·미라우팅 역할까지 거부한다 | `go/internal/gateway/delegation.go:663-676` |
| A5-04 | low | 신규 | TUI 검수기 수정의 핵심 단언(`Totals.Failures["CANCELLED"]`)이 자기 대조군 15개에 전혀 걸리지 않는다 | `go/internal/app/integration_tui_test.go:40-42` |
| INT-04 | low(문서) | 신규 | `ARCHITECTURE.md` §6.1이 net/http에 귀속시킨 half-close가 실제로는 동작하지 않는다 | `docs/v2/ARCHITECTURE.md` §6.1 |
| INT-05 | low(문서) | 신규 | 결합 short 옵션은 `ARCHITECTURE.md` §4가 **이미 지원을 선언한** 계약이다 (A1-02 재분류) | `docs/v2/ARCHITECTURE.md` §4 |

### 3.1 A1-01 — 읽지 못한 역할 파일 1개가 모든 미매칭 역할을 거부시킨다 (high / 신규)

- **파일**: `go/internal/app/roles.go:169`(`claim()`), `:47-49`·`:68-74`(`resolve()`). 확대 지점 `go/internal/gateway/delegation.go:266-276`
- **발생 조건**: `.claude/agents` 계열 일반(비-plugin) 디렉터리에 frontmatter를 파싱할 수 없는 `.md` 파일이 **하나라도** 있을 때.
  대표 입력: 첫 줄이 `---`인 메모(닫는 fence 없음), 64 KiB 초과 frontmatter, 열 수 없는 파일.
- **원인**: `claim()`이 `incomplete = incomplete || dir.prefix == ""`로 그 디렉터리 전체를 "이름을 알 수 없음"으로 표시하고,
  `resolve()`가 `!found && unverified`면 `errRoleDefaults`를 돌려준다.
  v0.3.1은 `prepare()`가 **모든 Agent 호출의 맨 앞에서 무조건** `d.roleDefaults`를 부르므로(기준 commit은
  `source = "agent-call-role"` 분기 **안에서만** 불렀다) 그 오류가 곧 `AGENT_SELECTION_UNVERIFIED`가 된다.
- **영향**: 정의 파일이 있는 역할만 살아남는다. 정의가 없는 내장 역할 `Explore`·`Plan`·`general-purpose`·`fork`·
  `workflow-subagent`·`clauduct-inherit`, 그리고 **명시 모델을 준 호출, `subagent_type`을 생략한 호출,
  이 빌드 자신의 `clauduct-terra-high` 메뉴 역할까지 전부 거부**된다. v0.3.0에서는 같은 입력이 정상 동작했다.
- **재현** (cwd `D:/AIDEV/clauduct-v031/go`, `CGO_ENABLED=0`, `OVL=<run-01>/repro/A1-roles-selection-args`):
  ```
  go test ./internal/app/     -overlay=$OVL/overlay-app.json      -run 'TestProbeStrayNoteRefusesUnmatchedRoles'      -count=1 -timeout 300s -v
  go test ./internal/gateway/ -overlay=$OVL/overlay-gateway2.json -run 'TestProbeUnverifiedScanRefusesEveryAgentShape' -count=1 -timeout 300s -v
  ```
- **예상 / 실제**: 예상은 v0.3.0처럼 `found=false, err=nil`. 실제는
  `Explore / Plan / general-purpose / fork / workflow-subagent / clauduct-inherit` 전부 `err=ROLE_DEFAULTS_UNVERIFIED`이고
  정의가 있는 `reviewer`만 `found=true`. Agent 호출 4가지 형태 전부 `AGENT_SELECTION_UNVERIFIED`.
- **증거**: `repro/A1-roles-selection-args/probe_app_test.go`, `probe_gateway2_test.go`
- **통합 재확인(본 문서 작성자 직접)**: `parseRoleFile`이 `---\n`으로 시작하고 닫는 `\n---\n`이 없는 파일에
  `errRoleDefaults`를 돌려주는 것을 소스에서 확인했다. 삭제된 v0.3.0 주석이 예고한 "one stray markdown file" 입력과 정확히 같다.
  `git show 149068e:…/delegation.go`의 `prepare`에서 `roleDefaults` 호출이 분기 안에 있었던 것도 확인했다.
- **권장 수정**: ① `incomplete`는 **바이트를 읽지 못한 경우에만** 세운다(`os.Open`/`Stat` 실패, 64 KiB 경계 절단).
  내용을 전부 읽었는데 frontmatter가 잘못된 파일은 native도 건너뛴다 — 그런 파일은 다른 이름을 선언할 수 없으므로 부재를 증명할 수 있다.
  ② A1-07(필요할 때만 `roleDefaults` 호출)을 함께 적용해 남은 경우의 폭발 반경을 좁힌다.
- **미확인 / 정책 판단 필요**: 이 동작이 이슈 #42 채택안("손상된 정의 때문에 존재 여부를 알 수 없는 역할만 거부")의
  범위 안인지. 문자 그대로는 "모든 미매칭 이름이 알 수 없음"이 성립하지만,
  `verification/v031-roles-20260921/REPORT.md`는 **내장 역할 전부가 함께 거부된다는 사실을 적지 않았다.**
  결함 수정과 문서 정정 중 어느 쪽인지는 Codex에서 결정할 항목이다.

### 3.2 A1-03 — pending 형제가 workflow 자식을 떼어낸다 (high / 신규)

- **파일**: `go/internal/gateway/delegation.go:408-422` (게이트 `:416`)
- **발생 조건**: 요청의 `X-Claude-Code-Request-Class`가 `workflow`가 아니고, 같은 세션·같은 `scope.parent` 아래에
  pending 상태인 **무관한** Agent 선택이 하나라도 있을 때.
- **원인**: 기준 commit은 `if scope.workflow || binding.Role == "workflow-subagent"`를 `hasPending` 계산 **앞**에서 판정했다.
  v0.3.1은 이를 뒤로 옮기고 `!hasPending &&`를 붙였다. 게이트가 "이 binding에 대응하는 pending"이 아니라
  "같은 부모 아래 아무 pending"이 됐다. 그 뒤 경로는 `subagents/agent-<id>.meta.json`을 찾는데
  workflow 자식의 meta는 `run.directory` 아래에 있어 최대 1초 대기 후 거부된다.
- **영향**: 루트 턴이 Workflow와 Agent를 함께 내보낸 흔한 상황에서 workflow 자식의 비-`workflow` class 요청이
  `AGENT_SELECTION_UNVERIFIED` 400으로 떨어진다.
- **재현**: `go test ./internal/gateway/ -overlay=$OVL/overlay-gateway.json -run 'TestProbeWorkflowChildDivertedByPendingSibling' -count=1 -timeout 300s -v`
- **예상 / 실제**:
  ```
  sibling=false scope.workflow=false -> route={gpt-6-astra low workflow-parent} found=true  err=<nil>
  sibling=true  scope.workflow=false -> route={}                                found=false err=AGENT_SELECTION_UNVERIFIED
  ```
- **증거**: `repro/A1-roles-selection-args/probe_gateway_test.go`
- **도달성 근거(통합 승격 A1-22)**: ① 설치본 2.1.278 바이너리 문자열 추적 — workflow 클래스 승격은 `subagent` 클래스에서만
  일어나고 측면 질의는 `auxiliary`로 남는다. ② 실세션 감사
  `verification/parent-wait-20260920/s43-user-audit.json`에 `kind=generation, class=auxiliary, agentId 있음` 레코드가 **실재**한다.
  → 전제는 구조적으로 도달 가능하다. **잔여**: 그 요청이 해당 자식의 *첫* 요청이어야 `d.resolved` 미스로 이 분기에 들어간다(순서 의존).
- **권장 수정**: workflow 분기를 원래 자리로 되돌리고, `Agent(subagent_type:"workflow-subagent")` 호출만 구제하려면
  `d.pending`에 이 자식의 `meta.ToolUseID` 키가 실제로 존재할 때만 pending 경로로 보낸다.
  **INT-02 때문에 A3-03 수정과 같은 변경에 넣어야 한다.**

### 3.3 A2-01 — 고정된 옛 턴 영수증이 더 새로운 항목을 파괴한다 (high / 신규)

- **파일**: `go/internal/gateway/native_events.go:245-272` (파괴 지점 `:269`). 삭제된 보호막: 기준 commit의 `stillOnTurn`.
- **발생 조건**: 요청 A가 `agentSelection`에서 자식 X의 턴 T1을 `record.nativeTurn`에 고정한다.
  A가 `beginResult`를 타지 않는 요청(compaction 클래스 등)이거나 선택 이후 오래 사는 동안 X가 다음 턴 T2를 끝내
  `NativeTurn="T2", NativeEndObserved=true, EndReason="answer", State="awaiting_children"`이 된다.
  그 뒤 A가 `CANCELLED` 이외 category로 실패한다.
- **원인**: v0.3.1은 영수증을 요청 시작 시점에 고정했다(방향은 옳다). 그러나 같은 변경이 `begin()` 직전의 **재검증까지 삭제**했다.
  `recordFailedAgentRequest`의 검사 목록에 **턴이 한 번도 등장하지 않는다.** `applyNativeTurn`의
  `e.NativeTurn != receipt.Turn` 가드는 바로 앞 `begin()`이 `NativeTurn=""`인 새 항목을 깔아 무력화된다.
  턴 ID가 불투명 문자열이라 순서 비교도 불가능하다.
- **영향**: ① T2의 `awaiting_children` 항목이 archive 없이 사라지고 부모에게 전달되지 못한 보고 본문이 버려진다.
  ② 새 항목에 이미 끝난 T1이 찍혀 `FinalizeNativeResults`의 `session_ended_unverified`까지 미결로 남는다.
  ③ **이어가기가 끊긴다** — `continuation.go:39`가 요구하는 네 조건이 동시에 무너져 깨어난 coordinator 자식이
  `AGENT_SELECTION_UNVERIFIED`를 받는다.
- **재현**: `go test ./internal/gateway/ -overlay <run-01>/repro/A2-session-turn-recovery/overlay.json -run 'TestA2StalePinnedTurnDestroysNewerAwaitingChildren' -count=1 -timeout 300s -v` → **exit 1 (FAIL)**
- **실제**: 항목 포인터 교체, 턴이 `second` → `first`로 되돌아감, `NativeEndObserved`/`EndReason`/본문 소실.
  기준 commit `results.go`만 overlay로 되돌리면 항목이 유지되고 본문(`r.bytes=18`)도 남는다.
- **증거**: `repro/A2-session-turn-recovery/run-a2-01-02-current.txt`, `run-a2-01-02-baseline.txt`
- **삭제된 회귀 검사**: `absent_tree_test.go`의 `TestAReceiptThatMovedOnIsNotTakenAsCurrent`가 삭제됐고,
  대체로 소개된 `turn_receipt_test.go:153`은 **디스크가 앞으로 간 경우만** 다루고 **항목이 앞으로 간 경우는 다루지 않는다.**
  `verification/v031-events-20260921/REPORT.md`의 "대체했다"는 서술이 방향을 반대로 적고 있다.
- **권장 수정**: 고정 방식을 유지하되 **순서 증거를 함께 고정**한다. `readCurrentNativeTurn`은 이미 파일명에서 단조 증가
  `sequence`를 계산하면서 버린다 — 이 값을 영수증/항목에 싣고 파괴 분기 진입 조건에
  "고정 sequence > 항목의 현재 sequence"를 추가한다. 최소 조치: `e.NativeTurn != "" && e.NativeEndObserved`인 항목에
  대해서는 `begin()`/`applyNativeTurn`을 포기하고 category를 버린다(증거 보존 우선).
- **주의(통합)**: A3-02(phase 래치)가 이 결함이 필요로 하는 실패 category의 공급원이다. **한쪽만 고치면 다른 쪽이 남는다.**

### 3.4 나머지 확정 결함

각 항목의 발생 조건·원인·영향·재현 명령·예상/실제·증거 경로·권장 수정·미확인 전문은 분야 보고서에 있다.
아래는 판단에 필요한 핵심만 남긴다.

- **A1-02 (medium/신규)** — `nativeArgEnd`가 `-cp`·`-pv`·`-dapi` 같은 결합 short 옵션을 unknown으로 돌리고,
  `takeUserSettings`가 unknown 뒤의 설정 후보를 전부 거부한다. `clauduct -cp --settings=my.json "프롬프트"`가
  `SETTINGS_INVALID`로 막힌다. 같은 argv를 native는 정상 처리한다(직접 관측: `claude -cz` → `unknown option '-z'`,
  `claude -pv --settings={}` → exit 0). `roleCLI` 쪽 동일 증상은 base에도 있어 신규가 아니고,
  **신규인 것은 `takeUserSettings` 쪽뿐**이다.
  **INT-05**: `ARCHITECTURE.md` §4가 "2.1.278의 공개 옵션 형태를 사용"한다고 이미 선언했으므로
  "문서에 미지원 문장이 없다"가 아니라 **문서가 이미 지원을 약속한 상태**다.
  권장: 첫 글자가 알려진 boolean short일 때만 commander 규칙으로 전개.

- **A2-02 (medium/혼합)** — `begin()`이 `awaiting_children` 항목을 제자리 초기화 대신 **새 객체로 교체**하면서
  진행 중인 `beginAnswer` closure가 고아 객체를 잡는다. closure는 본문 기록을 건너뛰면서 `d.stopped(*binding)`은 계속 부르고,
  `stoppedTurn(binding, "")`이 `turn==""` 가드를 통과해 **map에 있는 새 항목**을 옛 턴의 payload로 정산한다.
  기준 대조에서 본문 `"NEW TURN STREAMED BODY"`가 남던 것이 v0.3.1에서는 `"PREVIOUS TURN STOP PAYLOAD"`로 바뀐다.
  "옛 stop이 새 항목을 정산" 자체는 기존 결함, **본문 교체는 신규 회귀**.

- **A3-01 (medium/신규)** — `previewCompaction`이 `beginContext`의 `conversationRequest` 게이트를 복제하지 않아
  같은 본문·같은 헤더가 `/v1/messages` 200 / `/v1/messages/count_tokens` 400 `CONTEXT_COMPACTION_UNVERIFIED`로 갈린다.
  v0.3.0 `count_tokens.go`로 되돌리면 재현 테스트가 FAIL → **신규 회귀 확정**.
  미확인: native 2.1.278이 실제로 `auxiliary` 클래스에 압축 템플릿 본문을 싣는지. 그래서 high가 아니라 medium이다.

- **A3-02 (medium/기존)** — `context.go:284-287`이 phase를 먼저 쓰고 `saveContext` 실패 시 되돌리지 않는다.
  `s.busy`는 `:292`에서야 true이므로 해제 경로가 없다. 일시적 파일 오류 1회로 해당 session/agent가
  프로세스 재시작 전까지 생성도 압축도 못 한다(`CONTEXT_COMPACTION_REQUIRED` ↔ `CONTEXT_COMPACTION_UNVERIFIED` 교착).
  `restoreContext`는 디스크의 `compacting`을 `failed`로 매핑해 구제할 줄 알지만 in-memory 경로에는 같은 구제가 없다.
  기준 commit에서도 동일 재현 → **기존 결함**이나, v0.3.1이 형제 래치(`persistenceError`)를 이미 고쳤으므로
  **남은 유일한 래치**다.

- **A3-03 (low→medium, 통합 승격/신규)** — `featureApplies("workflow_agent")`가
  `r.AgentRole == "workflow-subagent"`(거부 **전** 확정)에서 `strings.HasPrefix(r.Source, "workflow-")`(거부 **후** 확정)로 바뀌어,
  `entry.route()` 이전에 거부된 workflow 자식이 분모에서 사라진다(`Requests=0`, `Evidence=not_observed`).
  단독 영향은 low지만 INT-02 때문에 medium으로 올린다.

- **A4-01 (medium/신규)** — `responseConn`이 구체 타입이 아니라 `net.Conn` **인터페이스**를 임베드한다.
  `*responseConn`의 메서드 집합에 `CloseWrite()`가 없어 net/http `server.go`의 `c.rwc.(closeWriter)` 단언이 항상 실패한다.
  v0.3.0은 원본 `*net.TCPConn`을 그대로 넘겼으므로 성공했다. 결과적으로 `closeWriteAndWait()`가 FIN 없는
  500ms `rstAvoidanceDelay` sleep으로 전락하고, 상대는 FIN 대신 **RST**를 받는다.
  **제품 경로 관측 11회 전부 ≥499ms 후 RST, 응답 직후 FIN 0회.** 대조군(래핑 없는 stock net/http)은 6회 중 4회 0–3ms `io.EOF`.
  overlay로 `CloseWrite` 위임 1개만 추가하면 제품 경로 4회 전부 0–1ms `io.EOF`.
  하필 v0.3.1이 새로 넣은 `CONTEXT_REQUEST_CLASS_UNVERIFIED` 안내가 이 경로로 나간다(body를 읽기 전에 return).
  증거 `repro/A4-transport-process/evidence-current.txt`, `evidence-with-proposed-fix.txt`, `evidence-repeat-runs.txt`.
  **채택 정책과의 관계**: 이것은 "독립 Node/.NET/raw TCP의 이전 FAIL"이 아니라 **제품 listener + 제품 handler**의
  v0.3.0 대비 회귀이므로 `ARCHITECTURE.md` §6.1의 완료 기준(보호 On의 제품 실제 동작) **안쪽**이며 재보고 금지 대상이 아니다.

- **A5-01 (medium/신규)** — `verification/v031-recheck-20260922/evidence.json`의
  `adguard.same_test_code_sha256["go/internal/gateway/socket_runtime_evidence_test.go"]`가
  기록 `fdcb21ed37f17a3d0652c1360b02c080c9b9f9310c28740bf69667750fc362ce`,
  실측 `7dc3741c56061f3fac289a2a4c3e5f79d30c37b52213297529be19121db1252c`로 불일치.
  **통합 보고자가 직접 재확인했다.** 같은 블록의 `.mjs` 2개는 일치한다(`test-http-transport.mjs` = `304098b8cd9f…`).
  원인은 `evidence.json` 기록(09:30:36) 이후 같은 날 09:47:13에 `v031-close-20260922` 라운드가 같은 검사 파일을 수정하고
  해시를 갱신하지 않은 것. **제품 결함이 아니라 검증 기록 무결성 결함**이며, 보호 On/Off 대조의 감사 가능성을 떨어뜨린다.
  `fdcb21ed…` 시점의 파일 내용은 커밋되지 않아 복원 불가이므로 두 판본의 판정 로직 동일성은 확인할 수 없다.
  권장: 기록을 소급 수정하지 말고 해시 옆에 기록 시점과 무효화 사실을 남긴다.

- **A5-03 (medium/기존)** — `//go:build runtime_evidence` 태그 파일이 CI에서 컴파일조차 되지 않는다.
  **통합 보고자 직접 확인**: `.github/workflows/go.yml`의 4단계(`go vet ./...` 56행, `go build ./...` 59,
  `go test -count=1 ./...` 62, `-race` 69) 어디에도 `-tags`가 없다. `go test -list` 대조로 태그로만 보이는 테스트가
  **10개**임을 확인했다(463 → 473). 그중 `TestTUIAcceptanceRejectsMissingOrMisorderedEvidence`와
  `TestIntegrationAuditSignalsOnlyTextStart`는 자격증명·네트워크·PTY 없이 **`ok 0.111s`로 통과**한다 —
  환경이 필요 없는데 CI에서 빠져 있다.
  **기존 결함**: 태그 자체는 기준 commit에도 있다(`nonstream_evidence_test.go`,
  `upstream/parameter_evidence_test.go`, `upstream/runtime_evidence_test.go`). v0.3.1이 태그 파일을 4개 늘렸다.
  정확한 서술은 "지금 깨져 있다"가 아니라 **"깨져도 CI가 모른다"** 다 (현재 `go vet -tags runtime_evidence ./...` exit 0).

- **INT-01 (medium/신규)** — `connection_test.go:60`이 `io.ReadAll`의 **`readErr`를 보기 전에** 본문을 파싱하고,
  파싱 실패 시 `:75`에서 `t.Fatal("invalid SSE JSON")`으로 패키지 전체를 중단한다.
  12줄 아래 `:89-94`가 같은 종류의 실패를 `failures++`로 세면서 `readErr`를 로그에 싣는데 절단된 프레임은 거기 도달하지 못한다.
  그래서 실패 출력에 `status`·`bytes`·`read_error`가 하나도 남지 않는다.
  overlay probe로 **클라이언트 측 절단만으로 같은 `t.Fatal`이 재현됨**을 결정적으로 보였다
  (`truncate=true status=200 bytes=8179 read_error=context canceled parse_failed=true`).
  좁은 재현(5회 × 4,000요청), app+gateway 동시 부하, 통합 보고자의 전체 재실행에서는 재현되지 않았다.
  **같은 명령이 2회 중 1회 실패했다. CI가 같은 명령을 돌린다.**
  권장(검사 강도를 낮추지 않는 방향): `:60` 직후 `if readErr != nil { failures++; continue }`, `:74-75`의 `t.Fatal`을
  `failures++` + `t.Logf(status, len(raw), readErr)`로. 판정 지점은 `:97`로 그대로 둔다.
  **미확인**: 이번 실패의 진짜 원인. 제품 쪽 절단 가능성은 배제되지 않았으므로 A4-01/A4-02와 함께 다뤄야 한다.

- **INT-02 (medium/신규)** — A1-03이 거부하는 workflow 자식 집합과 A3-03이 계정에서 놓치는 집합이 **같다.**
  v0.3.0에서는 `Unconfirmed=1`로 운영자가 볼 수 있던 것이, 같은 릴리스가 만든 거부를 같은 릴리스가 못 보게 됐다.
  `native_agent`에는 `Unconfirmed=1`이 남으므로 완전 무증상은 아니다. **A1-03과 A3-03을 같은 변경에서 고쳐야 한다.**

- **A1-05 (low/신규)** — plugin 정의의 frontmatter를 읽지 못하면 `dir.prefix != ""` 예외로 `incomplete`가 서지 않아
  그 역할이 **오류 없이 "부재"**로 답해지고 호출자의 모델로 실행된다. A1-01과 한 수정으로 정리된다.

- **A1-06 (low/신규)** — projects 트리 자체가 없을 때 `loadChoice`가 `choiceAbsent`의 역할별 분기에 도달하지 못하고
  custom·미라우팅 역할까지 거부한다.

- **A5-04 (low/신규)** — `integration_tui_test.go:40-42`의 `Totals.Failures["CANCELLED"] != 1` 단언만 삭제한 사본(mutA)으로
  15개 대조군 전부가 **통과(exit 0)**한다. 반대로 수정 이전 방식으로 되돌린 사본(mutB)은 2개 대조군이 FAIL.
  즉 **전체 되돌리기는 검출되지만 해당 단언 단독 삭제는 검출되지 않는다.**
  `v031-offline-20260922/REPORT.md`가 이 수정의 근거로 "15개 대조군 통과"를 제시하므로 근거가 실제보다 강하게 읽힌다.
  권장: `totals_cancel_zero` 대조군 1건 추가. 증거 `repro/A5-verification-compat/mutation-run.txt`.

- **INT-04 (low, 문서/신규)** — `ARCHITECTURE.md` §6.1이 종료를 두 조각(net/http half-close + 제품 100ms drain)으로 설명하는데
  앞쪽이 A4-01 때문에 실제로는 없다. 뒤쪽 수치(100ms·64KiB)는 코드와 일치한다.
  이 문단이 이번 릴리스 판정의 완료 기준 문서라 읽는 사람이 남아 있는 보호를 실제보다 크게 본다.
  A4-01의 `CloseWrite` 위임을 채택하면 문서가 그대로 맞고, 채택하지 않으면 §6.1에서 (1)을 지운다.
  **두 수정을 같이 넣을 때 주의**: 순서가 FIN → net/http `rstAvoidanceDelay`(500ms) → 제품 drain(100ms)이 되어 합계 600ms다.

- **INT-05 (low, 문서/신규)** — 위 A1-02 항목 참조.

---

## 4. 반박·보류·개선 제안

후보를 삭제하지 않았다. 각 항목의 분류 근거는 분야 보고서 §4·§5·§7과 `areas/INTEGRATION.md` §4에 보존돼 있다.

### 4.1 반박된 지적 — 40건

| 분야 | 반박 건수 | 위치 |
|---|---|---|
| A1 | 7 (A1-13 ~ A1-19) | `areas/A1-roles-selection-args.md` §4 |
| A2 | 1 (R-01 = A2-14) | `areas/A2-session-turn-recovery.md` §4 |
| A3 | 5 (A3-07 ~ A3-11) | `areas/A3-compaction-usage.md` |
| A4 | 11 (A4-R1 ~ R11) | `areas/A4-transport-process.md` |
| A5 | 12 (R1 ~ R12) | `areas/A5-verification-compat.md` §4 |
| 통합 강등 → 반박 | 4 (A2-08, A2-09, A3-12, A5-06) | `areas/INTEGRATION.md` §4.1 |

통합 검토자는 반박 43건을 표본 점검했고 **뒤집을 근거를 하나도 찾지 못했다**. 특기할 반박:

- **A1-17** — "`run.go`가 `forward` 인덱스를 `o.Args`에 적용해 설정 슬롯이 어긋난다"는 반박됐다.
  `run.go:151`에서 `o.Args = forward`가 먼저 실행되므로 인덱스가 일치한다. **통합 보고자도 독립 확인했다.**
- **A3-08** — "`context.go`의 `s.route = route`가 세션 라우트를 압축 라우트로 덮어쓴다"는 반박됐다.
  `compactRoute`가 **값 복사본**을 반환하므로 세션 라우트는 상한 전 값을 유지한다. 채택 정책 #56과 정합.
- **A5-R1** — "`connection.go`의 drain이 절대 실행되지 않는다(`finished`를 `true`로 만드는 곳이 없다)"는 반박됐다.
  `gateway.go:167-168`이 핸들러 래퍼에서 설정한다. **통합 보고자가 grep으로 독립 확인했다.**
- **A5-R3 / A5-R8 / A5-R9 / A5-R10** — `ARCHITECTURE.md` §7.1의 "일반 생성·압축은 `InputCounter.Count`를 부르지 않는다",
  PARITY E4의 "자동 압축에만 medium 상한", `COMPATIBILITY.md`의 `requestClassRequired`/`requestClassMissing` 설명,
  `PACKAGING.md`의 "Clauduct도 같은 projects 트리에 metadata를 저장한다"는 **전부 코드와 일치**한다.
- **모순처럼 보이지만 아닌 것**: A4-R6(sha256 일치)과 A5-01(sha256 불일치)은 **서로 다른 파일**이다 —
  A4-R6은 `verification/test-http-transport.mjs`(일치), A5-01은 `go/internal/gateway/socket_runtime_evidence_test.go`(불일치).
  통합 보고자가 두 해시를 같은 명령으로 동시에 확인했다.

### 4.2 통합 단계 강등 — 7 ID

| ID | 분야 분류 | 통합 판정 | 근거 요약 |
|---|---|---|---|
| **A1-04** | confirmed/medium | **improvement/low (잠재)** | `prepare`가 `fields["subagent_type"]`을 canonical로 다시 써서 native에 보낸다(`delegation.go:277-282`). 분야 **자신의 검증 산출물이 code lens REFUTED**를 냈는데 보고서 본문이 반영하지 않았다. `results.go:302`가 gateway에 남은 유일한 정확 비교라는 사실은 맞아 수정 권고는 유지 |
| **A1-21** | hold/medium | **종결 (A1-04에 흡수)** | 같은 질문이고 위 근거로 답이 났다 |
| **A2-08** | hold/low | **반박** | `workflow.go:282` 분기는 `adapterBytes == 0`일 때만 실행되고, 그 경우는 gateway가 만든 스크립트뿐이며 `agent()` 호출이 없어 native가 journal 행을 쓸 수 없다 |
| **A2-09** | hold/low | **반박** | 대소문자 변형은 `workflow.go:269`의 `meta.AgentType != binding.Role`에서 **먼저** 걸린다(분야 자신의 probe가 관측) |
| **A3-12** | hold/low | **반박** | 2.1.278 바이너리 추적: web_search 측면 질의는 빈 클래스가 아니라 `auxiliary`를 달고 온다. 헤더 옵트인은 제품이 강제한다(`app/session.go`) |
| **A3-13** | hold/low | **improvement/low (문서화된 의도)** | `ARCHITECTURE.md` §7.1과 `COMPATIBILITY.md`가 `count_tokens`를 별도 capability로, 게이트를 **생성 경로**로 이미 좁혀 뒀다 → INT-03으로 병합 |
| **A5-02** | confirmed/medium | **improvement/low** | `buildinfo`의 프로덕션 도달점은 2곳뿐이고 진단의 `version`은 **native** 버전이다(`diagnostics.go:840`). 개발 빌드는 `+dirty`를 찍는 반면 릴리스는 clean 태그 worktree에서 빌드한다 → "출력이 같아진다"가 성립하지 않는다. **분야 자신의 verify가 code/repro 양쪽 REFUTED.** 남는 것은 릴리스 전 `Version` 상수 상향이라는 체크리스트 항목 |
| **A5-06** | hold/low | **반박** | `prepare`의 canonical 재작성 때문에 v0.3.0 reader의 정확 비교가 어긋날 입력이 없다. `PACKAGING.md` §6의 두 조건 목록은 완전하다 |

A1-04와 A5-02는 **분야 리뷰어가 자기 적대 검증 결과(REFUTED)를 본문에 반영하지 않은 경우**다.
이 보고서는 검증 결과를 따른다.

### 4.3 남은 보류 — 5건

| ID | 심각도 | 보류 내용 | 결론을 내리려면 |
|---|---|---|---|
| A1-20 | low | native **subcommand** argv가 top-level 옵션 표로 해석된다 | 2.1.278의 subcommand 목록과 각자의 옵션 arity 1회 수집 |
| A1-23 | medium | `native_args.go` 표가 2.1.278에 고정 — 버전 상향 시 설정 병합이 조용히 누락될 수 있다 | 새 native 버전에서 `--help` 대조를 자동화할지 정책 결정 |
| A2-13 | low | 완료 표시(`.ready`) 없는 최고 sequence가 남으면 그 agent(또는 root 전체)의 모든 조회가 거부된다 | 실제 Windows에서 `.json` 성공 / `.ready` 실패의 부분 쓰기를 유발 |
| A4-03 | low | 새 100ms drain이 **실제 native 세션**에서 한 번이라도 발동하는지 근거가 없다 | 실 TUI 1회에서 native가 `Connection: close`를 보내는 빈도 관측. INT-01의 실패는 이 hold를 풀지 못한다 — 그 `Connection: close`는 **검사 클라이언트가** 만든 것이다 |
| A5-05 | low | 실호출 검수기가 기본 `TEMP` 환경에서 자기 private-path guard로 모든 요청을 400 거부할 가능성 | 기본 `TEMP`로 `CLAUDUCT_EVIDENCE_TUI_LOCAL=1` 로컬 PTY 1회(backend 지출 0) |

추가로 **INT-01의 진짜 원인**이 미판정 상태다. 검사가 `readErr`를 버려 사후 판정이 불가능하다.

### 4.4 개선 제안 — 25건

전문은 분야 보고서에 있다. 통합 관점에서 **한 뿌리로 묶이는 5묶음**만 여기 남긴다.

| 묶음 | 포함 ID | 뿌리 | 권고 |
|---|---|---|---|
| **M1** fold 비교 이관 잔여 | A1-04, A2-08, A2-09 | 역할 비교를 `roleMatches`로 옮기면서 정확 비교 3곳을 남겼다 (`results.go:302`, `workflow.go:279`, `:282`) | 셋 다 오늘은 도달 불가. 고친다면 **한 변경에서 셋을 함께**. 반대로 `prepare`의 canonical 재작성(`delegation.go:279-281`)이 **이 셋의 유일한 안전망**이라는 사실을 그 줄의 주석으로 못 박는다 |
| **M2** 활성 턴 디렉터리 미정리 | A2-03, A2-05, A2-13 | `readCurrentNativeTurn`이 `active/<name>/`를 전수 스캔하는데 낮은 sequence 파일 쌍을 **아무도 지우지 않는다** (상한에서 130배 비용 증가 측정) | 발행 검증 후 낮은 sequence 파일 쌍 제거 하나로 셋이 같이 줄어든다 |
| **M3** `UserPromptSubmit` 차단 | A1-11, A2-10 | `app/settings.go:78`이 건 hook과 `cmd/clauduct-hook/main.go:189-197`의 단일 실패 분기 | 두 분야가 같은 채택 정책의 서로 다른 절반을 제안했다. 합치면 하나 — **전송 실패는 1회 재시도, 결정적 4xx는 상태코드/분류를 실은 다른 문구** |
| **M4** 두 진입점 admission 복제 (INT-03) | A3-01, A3-06, A3-13 | `previewCompaction`이 `beginContext` 전처리를 문장 단위로 복제 | 공유 헬퍼 하나로 A3-01이 **구조적으로** 사라진다. A3-13은 `COMPATIBILITY.md`에 엔드포인트 범위 한 줄 |
| **M5** 연결 종료 경로 | A4-01, A4-02, INT-04 | 같은 `Close`/`finished` 경로 | 두 수정이 상호작용한다(INT-04의 600ms 순서 주의). 같이 검토 |

묶이지 않는 개선 제안: A1-07 ~ A1-12, A2-04·A2-06·A2-07·A2-11·A2-12, A3-04·A3-05,
A4-02·A4-04 ~ A4-07, A5-07 ~ A5-09, INT-03·INT-06.

특히 **A4-02(medium)** — 새 drain 게이트가 `r.Close`(클라이언트가 `Connection: close`를 보낸 경우)에만 걸려
**서버 주도 종료를 전혀 덮지 않는다**. A4-01과 함께 보는 것이 맞다.
**INT-06** — `handleCountTokens`에 `entry.requestClass(...)` 한 줄이 없어 그 엔드포인트의 클래스 질문을
진단으로 영원히 답할 수 없다(실세션 감사의 `count_tokens` 레코드 31건 전부 클래스 미기록).

### 4.5 근거 부족으로 뺀 의심

삭제하지 않고 각 분야 보고서 §7에 보존했다. 대표 사례: `roleCLI`의 `hasCLI` 판정이 `--` 뒤 프롬프트로도 켜지는 문제,
`CanonicalRole`이 대문자 메뉴 이름을 정규화하지 않는 문제 — **둘 다 기준 commit과 동일**해서 신규 회귀가 아니고
실제 사용 사례를 구성하지 못했다. `launch.Refused`의 과잉 거부는 `ARCHITECTURE.md` §4 1번에
"의도된 정책"으로 명시된 채택 사항이라 지적에서 제외했다.

---

## 5. 커버리지와 인계

### 5.1 통합 보고자의 독립 검증

하위 에이전트 결과를 그대로 수용하지 않고 본 문서 작성자가 직접 확인한 항목:

| 확인 항목 | 방법 | 결과 |
|---|---|---|
| A5-01 sha256 불일치 | `sha256sum` 직접 실행 + `evidence.json` 대조 | **확인** — 기록 `fdcb21ed…` vs 실측 `7dc3741c…`. `.mjs` 2개는 일치 |
| A5-03 태그 파일 CI 누락 | `go test -list` 태그 유무 대조 + `.github/workflows/go.yml` 독해 | **확인** — 태그로만 보이는 테스트 10개, CI 4단계 전부 `-tags` 없음 |
| 태그 뒤 순수 로직 검사의 환경 비의존성 | `go test -tags runtime_evidence -run '...' ./internal/app/` | **확인** — `ok 0.111s` |
| 태그 관행의 기원 | `git grep -l "go:build runtime_evidence" 149068e` | **정정** — 기준 commit에 3개 존재. "신규 도입"이 아니라 **기존 관행의 확장** |
| A1-01의 `parseRoleFile` 오류 조건 | 소스 독해 | **확인** — `---\n` 시작 + 닫는 fence 없음 → `errRoleDefaults` |
| A1-01의 폭발 반경 확대 | `git show 149068e:…/delegation.go`의 `prepare` 호출 위치 대조 | **확인** — base는 분기 안, v0.3.1은 함수 맨 앞 무조건 |
| A1-17 반박(설정 슬롯 인덱스) | `run.go:151`의 `o.Args = forward` 확인 | **반박 타당** |
| A5-R1 반박(`finished` 설정 지점) | `grep -rn finished go/internal/gateway/` | **반박 타당** — `gateway.go:167-168` |
| A4-01의 메커니즘 | net/http `closeWriteAndWait`의 `c.rwc.(closeWriter)` 단언과 Go 인터페이스 임베딩 규칙 대조 | **확인** — 관측된 499–507ms가 `rstAvoidanceDelay`(500ms)와 정확히 일치한다 |
| `connection_test.go:60-75`의 구조 | 소스 독해 | **확인** — `readErr` 검사 전에 파싱하고 `t.Fatal` |
| INT-01 단독 재현 | 해당 테스트만 `-count=1` ×3 | 3회 전부 ok — **부하 의존 실패임을 뒷받침** |
| `gofmt -l .` / `CGO_ENABLED=0 go build ./...` | 직접 실행 | 둘 다 exit 0 |
| 커버리지 비판의 공백 3 (`messages.go:574-592`) | 소스 독해 (`messages.go:584`, `parent_wait.go:99`, `results.go:566`) | **결함 없음** — `ControlMode`는 `prepareParentWait`가 설정하고 `ParentReadiness`는 `deliverResults`가 채운다. `native_print_test.go`가 reasoning-after-text 회귀를 실제로 덮는다. **다만 어느 분야도 이 경로를 리뷰하지 않았고 A1은 해당 테스트를 "무관"으로 배제했다** — 검토 공백은 사실이며 통합 단계에서 닫았다 |
| 전체 회귀 재실행 | `go test ./... -count=1 -timeout 900s` | §5.1a |

#### 5.1a 전체 회귀 재실행 결과

명령: `cd D:/AIDEV/clauduct-v031/go && CGO_ENABLED=0 go test ./... -count=1 -timeout 900s`
출력: `repro/INTEGRATION/full-regression-verify-01.txt`

```
ok  github.com/wotjr1649/Clauduct/go/cmd/clauduct            0.448s
ok  github.com/wotjr1649/Clauduct/go/cmd/clauduct-dev        0.642s
ok  github.com/wotjr1649/Clauduct/go/cmd/clauduct-hook       4.390s
ok  github.com/wotjr1649/Clauduct/go/internal/app          263.674s
ok  github.com/wotjr1649/Clauduct/go/internal/auth           2.692s
ok  github.com/wotjr1649/Clauduct/go/internal/buildinfo      0.915s
ok  github.com/wotjr1649/Clauduct/go/internal/childprocess   2.148s
ok  github.com/wotjr1649/Clauduct/go/internal/gateway       38.499s
ok  github.com/wotjr1649/Clauduct/go/internal/launch         1.029s
ok  github.com/wotjr1649/Clauduct/go/internal/pdf            3.487s
ok  github.com/wotjr1649/Clauduct/go/internal/platform       1.730s
ok  github.com/wotjr1649/Clauduct/go/internal/protocol/anthropic 2.399s
ok  github.com/wotjr1649/Clauduct/go/internal/protocol/bridge    1.344s
ok  github.com/wotjr1649/Clauduct/go/internal/protocol/codex     0.976s
ok  github.com/wotjr1649/Clauduct/go/internal/stream         1.222s
ok  github.com/wotjr1649/Clauduct/go/internal/update         2.972s
ok  github.com/wotjr1649/Clauduct/go/internal/upstream       4.034s
ok  github.com/wotjr1649/Clauduct/go/internal/wire           0.993s
exit=0
```

**예상**: 통합 검토 에이전트의 실패 재현 여부 확인. **실제**: 17개 패키지 전부 ok, exit 0.
→ **같은 명령이 2회 중 1회 실패했다.** 이 통과는 앞의 실패를 무효화하지 않으며, INT-01(비결정성)의 직접 증거다.
릴리스 판정 전에 INT-01을 고쳐 실패가 원인을 남기도록 만든 뒤 재실행해야 한다.

### 5.2 읽지 못한 파일 / 주장만 있는 파일

- **미검토 파일 0개.** manifest 118개가 5개 분야 표에 정확히 한 번씩 배정됐고, 종료 시점 `git diff --name-only`도 같은 118개다.
- **"주장만 있는" 파일 10개** — 커버리지 표에 검토 방식만 적히고 내용 언급이 0인 것:
  - A3 담당 테스트 7개: `client_capability_test.go`, `compaction_runtime_evidence_test.go`, `context_journal_test.go`,
    `gateway/context_test.go`, `count_tokens_test.go`, `features_test.go`, `messages_test.go`
  - A4 담당 close 묶음 3개: `v031-close-20260922/component-snapshot.txt`, `shutdown-path.txt`, `UPSTREAM-ISSUE.md`
- **배정 공백 3건**: ① `messages.go:574-592`(통합 단계에서 닫음, §5.1), ② 태그 증거 사슬을 A4/A5가 반쪽씩만 검토,
  ③ 신규 `bridge.KnownRole`의 소비처 2곳 미검토.
- **근거 없는 PASS 7건** — 전문(傳聞) PASS 승계, 컴파일된 적 없는 파일에 대한 "정적 확인",
  표본 점검을 전수처럼 읽히게 쓴 문장 등. 상세는 `areas/COVERAGE-CRITIC.md`.
- **기존 검증 묶음의 변이 56건 중 재실행 1건(1.8%).** 기존 묶음의 PASS 주장 대부분은 이번 라운드가 재실행하지 않았다.

### 5.3 미실행 검사 (NOT_RUN)

분야·통합 합계 42건 기재 → 중복 제거 **21건**. 판정에 영향이 큰 것만 추린다.

| 검사 | 왜 실행하지 않았나 | 판정에 미치는 영향 | 추가 확인 방법 |
|---|---|---|---|
| `go test -race ./...` 전체 | 이 호스트에서 시간이 길고, 과제가 전체 회귀 1회를 권했다. 분야별 좁은 race 3건은 exit 0 | **중간.** v0.3.1의 유일한 잠금 경계 변경(`delegation.go route()`의 `Unlock` 이동 — A1-03이 바로 거기다)이 어떤 race 정규식에도 포함되지 않았다 | `CGO_ENABLED=1 CC=C:/msys64/ucrt64/bin/gcc.exe go test -race ./... -count=1 -timeout 1800s` |
| `-tags runtime_evidence` 테스트 **실행** | 다수가 실제 backend 호출 또는 PTY를 요구한다. 컴파일·vet은 실행(exit 0) | **중간.** `ARCHITECTURE.md` §6.1 채택 정책의 근거 수치를 만든 파일이 여기 있고, 그 파일의 기록 해시가 불일치한다(A5-01) | 승인된 예산·PTY에서 `CLAUDUCT_EVIDENCE_LIVE=1` / `CLAUDUCT_EVIDENCE_TUI_LOCAL=1` / `CLAUDUCT_SOCKET_EVIDENCE=1` 각 1회 |
| `-tags policy_evidence` 실행 | 아무도 요구하지 않았다. 통합 단계에서 **컴파일만** 최초 확인(exit 0) | 낮음 | 위와 동일 |
| INT-01의 원인 판정 | 검사가 증거를 버려 사후 판정 불가. 3회 시도 후 새 증거 없이 재시도하지 않았다 | **높음.** 제품 절단 가능성이 배제되지 않았다 | INT-01 권장 수정 적용 후 `go test ./...` 반복 실행 |
| A1-03/A1-22의 실제 native 종단 재현 | 실제 Workflow 세션 + 형제 pending 동시 성립 필요 | 중간(도달성 잔여) | 실제 Workflow 실행 1회에서 workflow 자식 **첫** 요청의 class 관측 |
| A3-01의 실사용 빈도 | native가 `auxiliary`에 압축 템플릿 본문을 싣는지 미관측 | 심각도 판단(medium ↔ high) | 실 TUI 1회에서 `auxiliary` 요청 본문 캡처 |
| Node 31 / .NET 35 / raw TCP 400 전송 검사, C 진단기 빌드·ETW | 호스트 상태 변경을 수반한다. 이번 라운드 재실행 없음 | 낮음(기존 HOLD 유지) | `areas/A4-transport-process.md` 8절 |
| 유료 backend·수동 TUI | 범위 밖 | A3-01·A4-03·A5-05의 도달성 | 승인된 예산에서 각 1회 |
| 기존 검증 묶음 변이 56건 | 재실행 1건(1.8%)만 했다 | 낮음 | 각 묶음 `mutations.json` |

### 5.4 시작/종료 소스 차이

| 항목 | 시작 | 종료 |
|---|---|---|
| 대상 파일 수 | 118 | 118 |
| SHA256 drift | — | **0** |
| `git diff --stat` | `118 files changed, 7895 insertions(+), 636 deletions(-)` | 동일 |
| 미추적 항목 | `%SystemDrive%/` | `%SystemDrive%/`, `verification/v031-code-review-20260922/` |

**대상 소스가 리뷰 중에 바뀌지 않았다.** 따라서 재확인이 필요한 결과도, 미해결 차이도 없다.
새로 생긴 미추적 항목은 이 리뷰의 출력 루트뿐이다.

### 5.5 남은 자원

| 자원 | 상태 |
|---|---|
| Workflow `wf_61ab1ba6-494` | 완료(43/43, 오류 0). 남은 백그라운드 자식 없음 |
| 배경 `go test` 프로세스 `b798wkhgd` | **완료(exit 0)**. 종료 확인함 |
| 작업트리 | 제품·문서·기존 `verification/` 파일 무변경. 신규는 `verification/v031-code-review-20260922/`뿐 |
| index / stash | 미변경 (intent-to-add 상태 그대로). stash 사용 안 함 |
| 보호 설정·훅·필터·설치본 | 미변경 |
| commit / push / PR / 태그 / Release | 수행하지 않음 |

### 5.6 Codex에서 다음에 확인할 항목

우선순위 순.

1. **A1-01의 정책 판단** — "모든 미매칭 역할 거부"가 이슈 #42 채택안의 범위 안인가.
   범위 밖이면 `roles.go:169` 수정, 범위 안이면 `v031-roles-20260921/REPORT.md`와 `COMPATIBILITY.md`에
   "내장 역할도 함께 거부된다"를 명시. **어느 쪽이든 코드나 문서 중 하나는 바뀌어야 한다.**
2. **A1-03 + A3-03을 같은 변경으로** (INT-02). 한쪽만 고치면 회귀나 그 탐지기 중 하나가 남는다.
3. **A2-01 + A3-02를 같은 변경으로**. A3-02가 A2-01이 필요로 하는 실패 category의 공급원이다.
4. **A4-01의 `CloseWrite` 위임 채택 여부**와 그에 따른 `ARCHITECTURE.md` §6.1 정정(INT-04).
   채택 시 FIN → 500ms → 100ms = 600ms 순서를 확인할 것. `KERNEL-ANALYSIS.md`가 기록한
   "이 호스트에서 `shutdown(SD_SEND)`가 가짜 수신 FIN을 주입당한다"와의 상호작용도 한 번 더 볼 것.
5. **INT-01 수정 후 `go test ./...` 반복 실행.** 지금 트리에서 릴리스 게이트는 **비결정적**이다(2회 중 1회 실패).
   그 뒤 `-race ./...`도 1회.
6. **A1-02 — 구현을 고칠지 `ARCHITECTURE.md` §4에 예외를 명시할지**(INT-05). 문서가 이미 지원을 약속했다.
7. **A5-01 — 검증 기록 해시 정정 방식 결정.** 소급 수정이 아니라 무효화 사실 기록을 권한다.
8. **A5-03 — CI에 `-tags runtime_evidence` 컴파일 단계를 추가할지.** 최소한 `go vet -tags runtime_evidence ./...` 1줄.
9. **릴리스 전 체크리스트**: `buildinfo.Version` 상수 상향(A5-02 강등 후 남은 항목).
10. **`native_args.go` 표의 버전 고정**(A1-23 보류) — 새 native 버전 대조 방식의 정책 결정.

**이 리뷰 통과를 출하 바이너리의 검증 완료로 확대하지 않는다.** 이번 작업의 범위는 리뷰 보고서 제출이며,
제품 수정 여부와 배포 판단은 Codex 채팅에서 결정한다.

### 5.7 산출물 경로

| 산출물 | 경로 |
|---|---|
| 이 보고서 | `D:\AIDEV\clauduct-v031\verification\v031-code-review-20260922\run-01\REPORT.md` |
| 시작 manifest | `…\run-01\manifest-start.json` |
| 종료 manifest | `…\run-01\manifest-end.json` |
| 분야별 결과 | `…\run-01\areas\A1-roles-selection-args.md`, `A2-session-turn-recovery.md`, `A3-compaction-usage.md`, `A4-transport-process.md`, `A5-verification-compat.md` |
| 통합 검토 | `…\run-01\areas\INTEGRATION.md` |
| 커버리지 비판 | `…\run-01\areas\COVERAGE-CRITIC.md` |
| 재현 코드·실행 기록 | `…\run-01\repro\` (316개 파일) |
| 전체 회귀 재실행 기록 | `…\run-01\repro\INTEGRATION\full-regression-verify-01.txt` |
| manifest 생성기 / Workflow 원본 결과 | `…\run-01\tools\manifest.mjs`, `…\run-01\tools\workflow-result.json` |

`repro/` 아래 `*_test.go`는 **대상 패키지 안이 아니라 별도 디렉터리에 있고 `go test -overlay`로 주입**하도록 만들어졌다.
에디터나 `gopls`가 그 파일만 열면 import 해석 오류를 보고하지만, 각 항목에 기재된 overlay 명령으로 실행하면 정상 컴파일된다.
