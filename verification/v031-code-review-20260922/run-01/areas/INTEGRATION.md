# INTEGRATION — v0.3.1 전체 변경 통합 검토

- 대상 루트: `D:\AIDEV\clauduct-v031` (git worktree, branch `fix/v031`)
- 기준 commit: `149068edd693fb860a03244a2ea15764bcd68c34` (v0.3.0). HEAD 동일, v0.3.1 전체가 미커밋 작업트리
- 환경: Windows 11 Pro 26200 / `go1.27.1 windows/amd64` / 관측 native 2.1.278
- 제품 소스·테스트·문서·기존 `verification/` 파일을 하나도 수정·생성·삭제하지 않았다.
  `git add|commit|stash|checkout|reset|restore|clean|rebase`를 쓰지 않았고 읽기 전용 git 명령만 썼다.
  쓰기는 `verification/v031-code-review-20260922/run-01/areas/INTEGRATION.md`와
  `.../repro/INTEGRATION/` 안에만 했다. 재현은 전부 `go test -overlay`로 주입했다.
- 종료 시점 대조: `git diff --stat` = **118 files changed, +7895 / -636** (시작 스냅숏과 동일),
  untracked 신규는 `verification/v031-code-review-20260922/`뿐. 작업트리 루트의 `%SystemDrive%/`는 건드리지 않았다.

---

## 0. 판정 요약

| 항목 | 값 |
|---|---|
| 전체 회귀 `go test ./...` | **exit 1 — 실패 1건** (`internal/gateway`) |
| `go vet ./...` / `go vet -tags runtime_evidence ./...` | exit 0 / exit 0 |
| 통합 단계 신규 지적 | 6건 (INT-01 ~ INT-06) |
| 분야 지적 강등 | 7 ID (A1-04, A1-21, A2-08, A2-09, A3-12, A3-13, A5-02, A5-06) |
| 분야 지적 승격 | 2건 (A1-22 근거 확보, A3-03 low → medium) |
| 중복 병합 | 5묶음 (M1~M5) |

**배포 판단에 가장 무거운 넷**: A1-01(high), A1-03(high — INT-02로 계정에서 탐지 불가), A2-01(high),
INT-01(전체 회귀가 결정적이지 않다. CI가 같은 명령을 돌린다).

---

## 1. 실행한 검사와 결과

Go 검사 작업 디렉터리는 전부 `/d/AIDEV/clauduct-v031/go`, 환경은 `CGO_ENABLED=0`.
`$R` = `/d/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/INTEGRATION`.

| # | 명령 | 예상 | 실제 | exit | 출력 |
|---|---|---|---|---|---|
| 1 | `CGO_ENABLED=0 go test ./... -count=1 -timeout 720s` | 17 패키지 전부 ok | `gateway` 1건 FAIL, 나머지 16 ok | **1** | `$R/full-regression.txt` |
| 2 | `CGO_ENABLED=0 go vet ./...` | 출력 없음 | 출력 없음 | 0 | `$R/go-vet.txt` |
| 3 | `CGO_ENABLED=0 go vet -tags runtime_evidence ./...` | 태그 파일도 컴파일 통과 | 출력 없음 | 0 | `$R/go-vet-tagged.txt` |
| 4 | `CGO_ENABLED=0 go test ./internal/gateway/ -run 'TestGatewayResponsesSurviveConnectionTurnover' -count=5 -timeout 600s -v` | #1의 실패 재현 | 20개 서브테스트 전부 PASS (`failures=0` ×20, 4,000 요청) | 0 | `$R/turnover-narrow-x5.txt` |
| 5 | `CGO_ENABLED=0 go test ./internal/app/ ./internal/gateway/ -count=1 -timeout 720s` | 동시 부하로 재현 | app 244.1s ok / gateway 31.4s ok | 0 | `$R/turnover-under-load.txt` |
| 6 | `CGO_ENABLED=0 go test ./internal/gateway/ -overlay=$R/overlay.json -run 'TestIntegrationTurnoverParseMisattributesClientTruncation' -count=1 -timeout 300s -v` | 클라이언트 측 절단이 `invalid SSE JSON`과 같은 모양인지 판정 | `truncate=true status=200 bytes=8179 read_error=context canceled parse_failed=true` | 0 | `$R/int-probe.txt` |
| 7 | 읽기 전용 `git diff/show/status`, `grep`, `python`(evidence JSON 집계) 다수 | — | — | 0 | 본문 인용 |

### 전체 회귀를 실행한 근거

과제가 판단을 통합 검토자에게 맡겼다. 실행했다. 이유는 셋이다.

1. 5개 분야가 전부 좁은 `-run` 정규식으로만 돌았고, 누구도 패키지 경계를 넘는 조합을 돌리지 않았다.
2. 변경 규모가 118 파일 / +7895 라인이고 `results.go`·`native_events.go`·`messages.go`처럼
   여러 분야가 동시에 손댄 파일이 있다.
3. CI(`.github/workflows/go.yml:62`)가 정확히 같은 명령을 돌린다. 여기서 녹색이 아니면 CI도 녹색이 아니다.

**결과적으로 이 판단이 유일하게 새 실패를 드러냈다(INT-01).**

### 실패 원문 (`$R/full-regression.txt`)

```
ok  	github.com/wotjr1649/Clauduct/go/internal/app	263.071s
--- FAIL: TestGatewayResponsesSurviveConnectionTurnover (10.33s)
    --- FAIL: TestGatewayResponsesSurviveConnectionTurnover/closed/stream (5.47s)
        connection_test.go:75: invalid SSE JSON
FAIL	github.com/wotjr1649/Clauduct/go/internal/gateway	40.950s
EXIT=1
```

---

## 2. 연결부별 검토

### 2.1 native 인자 조립 → launch → bridge 라우팅 → 위임

읽은 경로: `app/native_args.go:10-39` → `app/user_settings.go:23-120` → `app/run.go:144-151,232-236,255-262`
→ `launch/launch.go:99-120` → `app/roles.go:267-354` → `protocol/bridge/route.go:152-195`
→ `gateway/delegation.go:210-392,394-537`.

- `nativeArgEnd`는 `takeUserSettings`(`user_settings.go:32`)와 `roleCLI`(`roles.go:280`) **두 소비자가 공유**하는데
  실패 규약이 다르다. `takeUserSettings`는 알 수 없는 옵션 뒤에 `--settings` 후보가 있을 때만 거부하고,
  `roleCLI`는 `end > len(args)`이면 `hasCLI`와 무관하게 `ROLE_DEFAULTS_UNVERIFIED`를 낸다.
  기준 commit의 `roleCLI` 인라인 스캐너(`git show 149068e:go/internal/app/roles.go`)도 같은 모양이므로
  **이 비대칭 자체는 신규 회귀가 아니다.** A1-02가 신규인 것은 `takeUserSettings` 쪽뿐이라는 A1 판정이 맞다.
- v0.3.1의 표는 base의 `valueOptions`/`flags` 집합을 **넓혔다**(`--environment`, `--file`, `--autocompact`,
  `--bg`, `--restricted`, `--tmux` 등). 넓어진 만큼 `roleCLI`의 과잉 거부는 줄었다. 회귀 없음.
- **argv에 병합 blob이 들어간 것이 역할 스캔을 바꾸지 않는다(부정 결과).** v0.3.0은
  `takeUserSettings`가 `--settings` 토큰을 forward에서 제거하고 `launch.Build`가 `--settings <blob>`을
  앞에 붙였다. v0.3.1은 원래 자리(`run.go:232-234`)에 `--settings=<blob>` 한 토큰으로 넣는다.
  `roleCLI`의 `hasCLI` 판정(`roles.go:270`)은 `--agents` / `--plugin-dir` 접두사만 보고,
  `nativeArgEnd("--settings=…")`는 attached라 `end=i+1, known=true`다.
  판정이 달라지는 입력을 만들지 못했다. 비용만 늘어난다(A1-07/A1-08 유지).
- `spec.Args`에 붙는 `--plugin-dir <nativePlugin>`(`run.go:256`)은 `o.Args`가 아니라 `spec.Args`에 붙으므로
  `roleCLI`의 `hasCLI`를 켜지 않는다. 확인함.
- **선택 값의 정규화 지점이 경로마다 다르다.** Agent 호출은 `prepare`(`delegation.go:277-282`)와
  `route`(`:482-485`)에서 `bridge.CanonicalRole`로 접히고, workflow 자식은 `workflowRoute`가
  `role: binding.Role`(`workflow.go:315`)로 **원형 그대로** 저장한다. 두 경로가 같은 `d.resolved` 맵에
  다른 규약의 값을 넣는다. 지금은 각자 비교 상대와 일치하므로 관측 실패가 없다(§4.1의 M1 강등 근거).

### 2.2 native 이벤트 → gateway 턴 바인딩 → results / continuation

읽은 경로: `app/native-events.mjs` → `gateway/native_events.go:100-278`
→ `gateway/messages.go:116,121,159-180,285-295` → `gateway/results.go:148-188,255-354`
→ `gateway/continuation.go:6-50`.

- `recordFailedAgentRequest`의 제품 호출자는 **`messages.go:121`의 defer 하나뿐**이다
  (`handleCountTokens`에는 없다). 따라서 A2-01의 파괴 경로는 `/v1/messages`에만 존재한다. A2 보고서를 좁히는 사실.
- 그 defer는 `messages.go:48`의 클래스 게이트 **뒤**(121행)에 등록되므로
  `CONTEXT_REQUEST_CLASS_UNVERIFIED` 거부는 A2-01을 촉발하지 않는다. 반면
  `beginContext`가 내는 `CONTEXT_COMPACTION_*`(`messages.go:185-189`)은 defer 등록 뒤라 촉발한다.
- **A2-01 × A3-02 조합.** A2-01은 "`beginResult`를 타지 않으면서 오래 사는 요청"을 필요로 한다.
  `messages.go:159`가 `Request-Class != "compaction"`을 요구하므로 **compaction 클래스 요청이 정확히 그 집합**이다.
  A3-02(`context.go:284-287`의 phase 래치)는 그 집합의 요청을 `CANCELLED`가 아닌 category로 반복 실패시킨다.
  즉 A3-02는 A2-01이 필요로 하는 실패 category의 공급원이다. 한쪽만 고치면 다른 쪽이 남는다.
- `agents.go:85`의 `existing.role != binding.Role` 정확 비교는 **hook이 보고한 두 binding끼리의 비교**라
  fold 이관 대상이 아니다. 결함 아님(전수 grep으로 확인).
- `continuation.go:39`가 요구하는 네 필드가 A2-01의 파괴로 동시에 무너지는 것을 코드로 확인했다. A2-01 유지.

### 2.3 압축 → messages → count_tokens 의 usage 집계와 연결 수명주기

읽은 경로: `gateway/context.go:106-115,200-300` → `gateway/context_compaction.go:13-94`
→ `gateway/count_tokens.go:16-130` → `gateway/messages.go:33-60,143-152`
→ `gateway/connection.go:12-69` → `gateway/gateway.go:164-186,537-559`.

- **두 진입점이 서로 반대 방향으로 어긋난다(INT-03).**
  `/v1/messages`에는 클래스 게이트(`messages.go:48-50`)가 있고 `beginContext`에
  `conversationRequest` 게이트(`context.go:206`)가 있다. `/v1/messages/count_tokens`에는 **둘 다 없다**.

  | 입력 | `/v1/messages` | `/v1/messages/count_tokens` |
  |---|---|---|
  | 클래스 헤더 없음 | 400 `CONTEXT_REQUEST_CLASS_UNVERIFIED` | 200 |
  | `auxiliary` + 압축 템플릿 본문 | 200 | 400 `CONTEXT_COMPACTION_UNVERIFIED` |

- `messages.go:48`의 새 게이트는 `request.HostedSearch` 조기 반환(`:143-152`)보다 **앞**이다(A3-12의 사실관계는 성립).
  실제 도달성은 §4.1에서 반박된다.
- 클래스 헤더는 native가 **옵트인**으로만 보낸다. 제품이 `CLAUDE_CODE_GATEWAY_HINT_HEADERS=1`을
  강제하므로(`app/session.go:74`, `:112`의 `sessionRequirements`) 프로덕션에서는 항상 켜진다.
  이 강제가 사라지면 **모든 생성 요청이 400**이 된다. 두 파일이 한 줄로 묶여 있다는 사실을 기록해 둔다.
- **연결 종료와의 상호작용**: A3가 새로 넣은 `CONTEXT_REQUEST_CLASS_UNVERIFIED` 거부는 body를 읽기 전에
  return하는 경로라, A4-01이 "half-close를 잃은 조기 거부 경로"로 지목한 목록에 v0.3.1이 **새로 추가한** 항목이다.
  A3의 새 게이트와 A4의 wrapper는 같은 요청에서 만난다. A4-01 본문이 이 연결을 이미 적었다. 유지.
- `Gateway.Close`는 `g.closing.Store(true)`를 **먼저** 하고(`gateway.go:539`) `Shutdown`을 부르므로,
  진행 중인 100ms drain은 두 번째 `Close()`의 조기 반환 경로(`connection.go:51-53`)로 중단된다.
  drain이 종료를 붙잡는 누수 경로를 만들지 못했다. 결함 아님.

### 2.4 app/run.go 정리 경로가 gateway 자원을 모두 회수하는가 — 누수 경로 없음

읽은 경로: `app/run.go:194-207,325-341,343-387` → `gateway/gateway.go:537-559`
→ `gateway/connection.go:49-65` → `gateway/native_cancellation.go:17-83`.

- 순서: `closeGateway`(`run.go:362`, handler drain 완료) → `FinalizeNativeResults`(`:364`)
  → `Diagnose`(`:366`) → 함수 반환 시 `nativePlugin`의 `RemoveAll`(`:194-207` defer).
  gateway가 `receipts` 디렉터리를 읽는 두 지점이 모두 삭제보다 앞이다. 순서 정확.
- `nativeCancellation` 항목은 handler의 defer(`native_cancellation.go:46-51`)가 지운다.
  `Shutdown`이 handler를 기다리므로 맵에 남는 항목이 없다. 누수 없음.
- A4-06(`run.go:344-350`의 `==` 비교와 수제 `Unwrap() []error` 분해)을 코드로 재확인했다.
  같은 함수 `:371-375`가 `errors.Is`를 쓰라고 주석까지 달아 둔 바로 아래다. improvement 유지.

### 2.5 문서(docs/v2/*)와 실제 코드 경로

- **ARCHITECTURE §6.1 ↔ `connection.go`(INT-04).** 문서는 "`net/http`가 framing을 flush한 후
  상대가 먼저 닫을 기회를 준다"고 적는다. `responseConn`이 `net.Conn` **인터페이스**를 임베드하므로
  (`connection.go:33-39`) `*responseConn`의 메서드 집합에 `CloseWrite()`가 없고,
  net/http의 `closeWriteAndWait` half-close는 실행되지 않는다(A4-01).
  A5는 수치(100ms/64KiB)만 대조했고(A5-R7), A4는 이 문장을 대조하지 않았다. 경계에서 빠진 항목.
- **ARCHITECTURE §4 ↔ `native_args.go`(INT-05).** §4는 argv 계약을
  "Claude Code 2.1.278의 공개 옵션 형태와 기존 hidden 옵션 목록을 사용"한다고 못 박는다.
  결합 short 옵션은 2.1.278의 공개 형태다. 표에 그 형태가 없다.
- **ARCHITECTURE §7.1 / COMPATIBILITY ↔ `count_tokens.go`.** §7.1은
  "`/v1/messages/count_tokens`는 별도 capability다"라고 적고, COMPATIBILITY는
  "헤더를 보내지 않는 client는 제품 **생성 경로**를 사용할 수 없다"로 엔드포인트를 좁힌다.
  A3-13의 "게이트 부재"는 **문서화된 의도**에 해당한다(강등).
- **PACKAGING §6 ↔ `delegation.go` 정규화.** A5-06의 "세 번째 거부 조건" 가설은
  `prepare`가 `fields["subagent_type"]`을 canonical로 다시 써서 native에 보내므로
  (`delegation.go:277-282`) 성립하지 않는다(강등).
- **COMPATIBILITY "최근 검사: S49 전체 회귀 17 packages/1,668 통과/3 skip"** — 이 트리에서 같은 명령의
  결과는 **exit 1**이다(INT-01). 문서의 회귀 근거가 현재 트리 상태와 어긋난다.

---

## 3. 새로 찾은 지적

### INT-01 — 전체 회귀가 결정적이지 않고, 그 실패가 제품 절단인지 검사 자신의 timeout인지 구분되지 않는다

- **분류/심각도/기원**: confirmed / **medium** / 신규 회귀 (`connection_test.go`는 v0.3.1 신규 파일)
- **파일**: `go/internal/gateway/connection_test.go:60-94` (특히 `:74-75`)
- **발생 조건**: `go test ./...`처럼 17개 패키지가 동시에 도는 부하에서
  `TestGatewayResponsesSurviveConnectionTurnover/closed/stream`의 200회 루프 중 한 번이
  `http.Client{Timeout: 2 * time.Second}`(`:37`)에 걸릴 때.
- **원인**: 루프가 `raw, readErr := io.ReadAll(response.Body)`(`:60`)의 **`readErr`를 보기 전에** `raw`를 파싱하고,
  파싱 실패 시 `:75`에서 `t.Fatal("invalid SSE JSON")`으로 **패키지 전체를 중단**한다.
  12줄 아래 `:89-94`가 같은 종류의 실패를 `failures++`로 세면서 `readErr`를 로그에 싣는데,
  절단된 프레임은 그 지점에 도달하지 못한다. 그래서 출력에 `status`·`bytes`·`read_error`가 하나도 남지 않는다.
- **영향**:
  1. `go test ./...`가 결정적이지 않다. CI(`.github/workflows/go.yml:62`, `:69`)가 같은 명령을 돌린다.
  2. 실패가 나도 **원인을 특정할 수 없다.** 제품이 스트림을 끊었는지 검사 클라이언트가 2초에 끊었는지,
     남는 증거가 `invalid SSE JSON` 한 줄뿐이다. 하필 `closed/*`는 `DisableKeepAlives: closeEach`(`:35`),
     즉 v0.3.1이 새로 넣은 `r.Close` drain 경로(`gateway.go:167-170`)를 타는 서브테스트다.
- **재현 명령과 결과**:
  - 실패 관측: §1 #1 (exit 1). `$R/full-regression.txt`
  - 좁은 재현 시도: §1 #4 — 5회 × 4 서브테스트 = 4,000 요청, `failures=0`, exit 0. **재현 안 됨.**
  - 부하 재현 시도: §1 #5 — `app`(244s)와 `gateway`(31s) 동시 실행, exit 0. **재현 안 됨.**
  - 판별 probe: §1 #6 — 클라이언트가 본문을 중간에 포기하면 어떤 모양이 되는지 관측.
    ```
    truncate=false status=200 bytes=67146 read_error=<nil>            parse_failed=false
    truncate=true  status=200 bytes=8179  read_error=context canceled parse_failed=true
                   bad_prefix="{\"delta\":{\"text\":\"xxxxxxxxxx…"
    ```
    **클라이언트 측 절단만으로 `:75`의 `t.Fatal`이 정확히 재현된다.**
- **증거 경로**: `$R/full-regression.txt`, `$R/turnover-narrow-x5.txt`, `$R/turnover-under-load.txt`,
  `$R/int-probe.txt`, probe 소스 `$R/int_probe_test.go`, overlay `$R/overlay.json`
- **권장 수정**(글로만): `:60` 직후 `if readErr != nil { failures++; continue }`를 넣고,
  `:74-75`의 `t.Fatal`을 `failures++` + `t.Logf(... status, len(raw), readErr ...)`로 바꾼다.
  그러면 (a) 부하로 인한 클라이언트 timeout이 200회 중 몇 번인지 수치로 남고,
  (b) 그래도 `failures != 0`이면 `:97-99`가 여전히 실패시키며,
  (c) 실패 출력이 제품 절단과 클라이언트 timeout을 구분해 준다.
  **검사 강도를 낮추는 변경이 아니다.** 판정 지점은 `:97`로 그대로다.
- **미확인**: 이번 실패의 진짜 원인. probe는 "클라이언트 절단이면 이 모양이 된다"를 보였을 뿐
  "이번 실패가 클라이언트 절단이었다"를 보이지 않았다. 검사가 그 증거를 버렸기 때문에
  **현재 트리에서는 사후 판정이 불가능하다.** 위 수정 후 재관측이 필요하다.
  제품 쪽 절단 가능성은 배제되지 않았으므로 A4-01/A4-02와 함께 다뤄야 한다.

### INT-02 — A1-03의 회귀와 A3-03의 진단 공백이 같은 요청 집합을 덮어, 회귀가 계정에서 보이지 않는다

- **분류/심각도/기원**: confirmed / **medium**(단독) — 실질 효과는 A1-03(high)의 탐지 불가 / 신규 회귀
- **파일**: `go/internal/gateway/delegation.go:408-422` × `go/internal/gateway/features.go:98`
  (경유 `go/internal/gateway/messages.go:299`, `:229`)
- **근거**:
  - A1-03의 거부는 `route()`가 `errDelegationUnverified`를 내고 `agentSelection`이
    `messages.go:305`에서 반환하는 지점에서 끝난다. `entry.route(...)`는 `messages.go:229` — 한참 뒤다.
    따라서 `RequestRecord.Source`는 비어 있다.
  - v0.3.1의 `featureApplies("workflow_agent")`는 `strings.HasPrefix(r.Source, "workflow-")`를 쓴다(`features.go:98`).
    기준 commit은 `r.AgentRole == "workflow-subagent"`였고(`git show 149068e:…/features.go` 95행),
    `AgentRole`은 거부 **이전**인 `messages.go:299`의 `entry.agent(...)`에서 채워진다.
  - 즉 **A1-03이 떼어낸 workflow 자식은 `workflow_agent` 분모에서 사라진다.**
    v0.3.0에서는 `Unconfirmed=1`로 남아 운영자가 볼 수 있었다.
- **영향**: "필수 검사 없이 실행된 workflow 자식"을 보고하라고 만든 계정이,
  같은 릴리스가 새로 만든 workflow 자식 거부를 **정확히 그 종류만** 놓친다.
  `native_agent`에는 `Unconfirmed=1`이 남으므로 완전 무증상은 아니다.
- **재현 명령**: `NOT_RUN`. 두 분야가 각각 재현했다
  (A1 `TestProbeWorkflowChildDivertedByPendingSibling`, A3 `TestA3RefusedWorkflowChildDropsOutOfWorkflowFeatureEvidence`).
  이 항목은 두 재현의 **집합 동일성** 주장이며 근거는 위 행 번호 대조다.
- **권장 수정**: A3-03의 권장안(거부 이전에 확정되는 사실로 게이트)을 A1-03 수정과 **같은 변경에** 넣는다.
  A1-03만 고치면 계정 공백이 남고, A3-03만 고치면 회귀가 남는다.

### INT-03 — 두 요청 진입점의 admission 로직이 복제돼 양방향으로 어긋난다 (A3-01 + A3-06 + A3-13 병합)

- **분류/심각도/기원**: improvement / low / 신규 회귀
- **파일**: `go/internal/gateway/messages.go:48-50` + `go/internal/gateway/context.go:106-115,206`
  vs `go/internal/gateway/count_tokens.go:16-91` + `go/internal/gateway/context_compaction.go:59-63`
- **근거**: §2.3의 표. 같은 본문·같은 헤더가 두 엔드포인트에서 **반대로** 갈린다.
  첫 줄은 문서화된 의도(§2.5)이고 둘째 줄은 A3-01이다.
  뿌리는 하나 — `previewCompaction`이 `beginContext` 전처리를 문장 단위로 복제했다(A3-06).
- **권장 수정**: A3-06의 공유 헬퍼 하나로 A3-01이 구조적으로 사라진다.
  A3-13은 고칠 대상이 아니라 COMPATIBILITY에 엔드포인트 범위를 한 줄 더 명시하면 끝난다.

### INT-04 — ARCHITECTURE §6.1이 net/http에 귀속시킨 half-close가 실제로는 동작하지 않는다

- **분류/심각도/기원**: confirmed / low(문서) — 근거 결함은 A4-01(medium) / 신규 회귀
- **파일**: `docs/v2/ARCHITECTURE.md` §6.1 (`HTTP/1.1 Connection: close …` 문단)
  vs 검토 당시 `go/internal/gateway/connection.go` 33–39행
- **근거**: 문서는 종료를 두 조각으로 설명한다. (1) net/http가 framing flush 후 상대에게 먼저 닫을 기회를 준다,
  (2) 제품이 최대 100ms·64KiB drain한다. (2)는 코드와 일치한다(A5-R7이 수치를 대조했다).
  (1)은 `responseConn`이 `net.Conn` 인터페이스를 임베드해 `CloseWrite()`를 감추는 순간 사라진다 —
  net/http `server.go`의 `if tcp, ok := c.rwc.(closeWriter)` 단언이 항상 실패한다.
  A4-01이 제품 경로 관측 11회 전부에서 FIN 0회 / 499–507ms 후 RST를 기록했다.
- **영향**: 문서가 구현보다 한 겹 두껍다. 이 문단은 이번 릴리스 판정의 완료 기준 문서라,
  읽는 사람이 남아 있는 보호를 실제보다 크게 본다.
- **권장 수정**: A4-01의 `CloseWrite` 위임을 채택하면 문서가 그대로 맞는다.
  채택하지 않으면 §6.1에서 (1)을 지우고 "제품 drain만이 이 보호를 한다"로 적는다.
  **두 수정을 같이 넣을 때 주의**: `CloseWrite` 복원 시 순서가
  FIN → net/http `rstAvoidanceDelay`(500ms) → `responseConn.Close`의 100ms drain이 된다. 무해하나 합계 600ms다.

### INT-05 — 결합 short 옵션은 ARCHITECTURE §4가 이미 선언한 계약의 일부다 (A1-02 재분류)

- **분류/심각도/기원**: confirmed / low(재분류) — 근거 결함은 A1-02(medium) / 신규 회귀
- **파일**: `docs/v2/ARCHITECTURE.md` §4 항목 2와 "값 경계" 문단 vs `go/internal/app/native_args.go:11-38`
- **근거**: §4는 v0.3.1이 새로 쓴 절이고 "Claude Code **2.1.278의 공개 옵션 형태**와
  기존 hidden 옵션 목록을 사용"한다고 못 박는다.
  `-cp`·`-pv`·`-dapi`는 commander가 분해하는 2.1.278의 공개 형태다
  (A1 직접 관측: `claude -cz` → `error: unknown option '-z'`, `claude -pv --settings={}` → exit 0).
  `nativeArgEnd`의 `default` 분기(`:35-36`)는 이를 unknown으로 돌린다.
- **영향**: A1-02의 "미확인" 칸("문서에 미지원이라는 문장이 없다 → 고치거나 문서화하거나")이 잘못됐다.
  문서는 이미 지원을 선언했다. 남은 선택지는 구현을 맞추거나 §4에 예외를 명시하는 것이다.
- **권장 수정**: A1-02의 권장안(첫 글자가 알려진 boolean short일 때만 전개) 그대로.

### INT-06 — `count_tokens`는 진단에 요청 클래스를 전혀 남기지 않아, 클래스 관련 질문을 계정으로 답할 수 없다

- **분류/심각도/기원**: improvement / low / 기존결함 (v0.3.1이 클래스 게이트를 넣으면서 중요해졌다)
- **파일**: `go/internal/gateway/diagnostics.go:193-200`(`record.requestClass`).
  제품 호출자는 `go/internal/gateway/messages.go:116` **하나뿐**이다(전수 grep, 기준 commit도 동일).
- **근거**: 실제 세션 감사(`verification/parent-wait-20260920/s43-user-audit.json`, native 2.1.278)의
  `count_tokens` 레코드 32건 전부 `nativeRequestClass`가 없다. 이는 native가 헤더를 안 보냈다는 뜻이 아니라
  **게이트웨이가 그 엔드포인트에서 기록하지 않는다**는 뜻이다.
- **영향**: A3-13("구형 클라이언트의 preflight만 통과한다")과 A3-01("auxiliary 계수가 갈린다")의
  실사용 빈도를 기존 계정으로는 영원히 답할 수 없다.
- **재현 명령**: `NOT_RUN`(정적). 확인 방법:
  `grep -rn "requestClass(" go/internal/gateway/*.go | grep -v "func (r \*record)"` → 1건(exit 0).
- **권장 수정**: `handleCountTokens`에도
  `entry.requestClass(r.Header.Get("X-Claude-Code-Request-Class"))` 한 줄. 거부하지 않고 기록만 한다 —
  §2.5의 문서화된 의도를 바꾸지 않는다.

---

## 4. 분야 지적 근거 대조

### 4.1 강등 (근거 부족 / 도달 불가)

| ID | 분야 분류 | 통합 판정 | 근거 |
|---|---|---|---|
| **A1-04** | confirmed / medium | **improvement / low (잠재)** | `prepare`가 `fields["subagent_type"]`을 canonical로 다시 써서 native에 보낸다(`delegation.go:277-282`). 그 바이트가 실제로 클라이언트에 전달되는 것을 `PrepareToolCall`(`messages.go:609-618`)까지 추적해 확인했다. 분야 자신의 검증 산출물 `repro/A1-roles-selection-args/verify-A1-04-code/run-a104-probes.log`가 `typed="EXPLORE" → sent to native="Explore" pending.role="Explore" sentEqualsPending=true`를 7개 철자에서 관측했고, 같은 로그의 `TestA104_ControlHandInjectedNonCanonicalSpelling`은 **철자를 손으로 주입했을 때만** 재현된다. 실제 세션 감사에서도 관측된 역할은 `general-purpose`·`workflow-subagent` 등 canonical뿐이다. **분야 보고서의 최종 분류가 자기 verify 결과(code lens REFUTED)와 어긋난다.** `results.go:302`가 gateway에 남은 유일한 정확 비교라는 사실은 맞으므로 수정 권고는 유지한다. |
| **A1-21** | hold / medium | **종결 (A1-04에 흡수)** | 같은 질문이고 위 근거로 답이 났다. |
| **A2-08** | hold / low | **refuted** | `workflow.go:282`의 분기는 `run.origin.adapterBytes == 0`일 때만 실행된다. 분야 자신의 `verify-A2-08-code/NOTES.md`가 `adapterBytes`의 전 쓰기 지점을 열거해 0이 되는 것은 gateway가 만든 `rejectWorkflow`/`recoverWorkflow` 스크립트뿐이고 둘 다 `agent()` 호출이 없어 native가 `started` journal 행을 쓸 수 없음을 보였다(측정: `shape=user-script adapterBytes=2793` / `shape=recovery 0, canCallAgent=false`). 사용자 정의 역할이 이 분기에 도달할 입력이 없다. |
| **A2-09** | hold / low | **refuted** | 분야 자신의 probe(`verify-A2-09-code/run-probe-asis.log`)가 `TestA209CaseVariantWorkflowSubagent → found=false err=AGENT_SELECTION_UNVERIFIED`를 관측했다. 대소문자 변형은 `workflow.go:269`의 `meta.AgentType != binding.Role`에서 **먼저** 걸린다. `:279`가 fold였어도 결과가 같다. |
| **A3-12** | hold / low | **refuted** | 설치본 2.1.278 바이너리 문자열 추적(`repro/A3-compaction-usage/verify-A3-COMPACTION-USAGE-12-code/native-2.1.278-class-header-trace.txt`)의 `Xs()`: `source`가 `repl_main_thread*`/`sdk`면 `main`, `agent:*`/`hook_agent`면 `subagent`, **그 밖은 전부 `auxiliary`**. `hI()` 측면 질의 헬퍼는 `source:"side_query"`를 고정으로 넘긴다. 즉 web_search 요청은 빈 클래스가 아니라 `auxiliary`를 달고 온다. 헤더의 옵트인 스위치 `CLAUDE_CODE_GATEWAY_HINT_HEADERS`는 제품이 강제한다(`app/session.go:74`, `:112`). 전면 400 시나리오 성립 안 함. |
| **A3-13** | hold / low | **improvement / low (문서화된 의도)** | `docs/v2/ARCHITECTURE.md` §7.1 "`/v1/messages/count_tokens`는 별도 capability다", `docs/v2/COMPATIBILITY.md` "헤더를 보내지 않는 client는 제품 **생성 경로**를 사용할 수 없다". 엔드포인트가 이미 좁혀져 있다. 남는 것은 preflight/생성 비대칭의 혼란뿐 → INT-03으로 병합. |
| **A5-02** | confirmed / medium | **improvement / low** | 분야 자신의 `verify-A5-VERIFICATION-COMPAT-02-code/EVIDENCE.txt`가 (a) `buildinfo`의 프로덕션 도달점은 `cmd/clauduct-dev/main.go:49`와 `internal/update/run.go:75` 둘뿐이고 진단/상태 writer는 도달하지 않으며(진단의 `version`은 `g.ClientVersion()`, 즉 **native** 버전이다 — `diagnostics.go:840`), (b) 이 개발 빌드는 `…+dirty`를 찍는 반면 릴리스 절차는 clean 태그 worktree에서 빌드하고 `+dirty` 부재를 확인하도록 PACKAGING이 규정한다는 것을 보였다. **"출력 문자열이 완전히 같아진다"는 성립하지 않는다.** 남는 것은 릴리스 전 `Version` 상수 상향이라는 체크리스트 항목이다. |
| **A5-06** | hold / low | **refuted** | `prepare`의 `subagent_type` 재작성(`delegation.go:277-282`) 때문에 native가 보고하는 `binding.Role`이 canonical이 되고 journal의 `Role`도 같은 값이다. v0.3.0 reader의 정확 비교(`git show 149068e:…/delegation.go` 704행)가 어긋날 입력이 없다. workflow 경로는 `role: binding.Role` 원형 저장(`workflow.go:315`)이라 애초에 정규화되지 않는다. PACKAGING §6의 두 조건 목록은 완전하다. |

### 4.2 승격

| ID | 분야 분류 | 통합 판정 | 근거 |
|---|---|---|---|
| **A1-22** | hold / high (A1-03 도달성) | **근거 확보 — hold 해제 방향** | 두 갈래. (1) 바이너리 추적의 `x8r()`: `if(r==="subagent" && n.agentType==="subagent" && n.workflowRunId) return "workflow"` — workflow 승격은 `subagent` 클래스에서만 일어나므로, workflow 자식이 내는 **측면 질의는 `Xs("side_query") = "auxiliary"`** 로 남는다. (2) 실제 세션 감사에 `kind=generation, class=auxiliary, agentId 있음, role=general-purpose` 레코드가 1건 실재한다 — **agent 범위 요청에 비-workflow 클래스가 붙는 것이 실측된다.** 따라서 A1-03의 전제(`scope.workflow == false`인 workflow 자식 요청)는 구조적으로 도달 가능하다. 잔여: 그 요청이 해당 자식의 **첫** 요청이어야 `d.resolved` 미스로 분기에 들어간다(순서 의존). |
| **A3-03** | improvement / low | **improvement / medium** | 단독 영향은 low가 맞지만, 이 계정이 놓치는 집합이 A1-03(high)이 만드는 집합과 같다(INT-02). 회귀와 그 탐지기가 동시에 꺼진다. |

### 4.3 유지

| ID | 통합 재확인 내용 |
|---|---|
| **A1-01** (confirmed/high) | `roles.go:190-193`이 `parseRoleFile`의 **오류**에도 `claim()`을 부르고, `claim()`이 `incomplete = incomplete \|\| dir.prefix == ""`(`:169`)로 일반 디렉터리 전체를 미검증으로 만든다. `resolve`(`:68-79`)가 그 상태에서 미매칭 역할 전부를 `errRoleDefaults`로 돌리고, `prepare`(`delegation.go:268`)가 모든 Agent 호출 앞에서 무조건 이를 부른다. 전 경로 코드로 재확인. 유지. |
| **A1-02** (confirmed/medium) | `nativeArgEnd("-cp")` → `default` → `known=false`(`native_args.go:35-36`), `takeUserSettings`(`user_settings.go:33-43`)가 뒤의 `--settings`를 보고 `errUserSettings`. 유지. 문서 대조는 INT-05로 강화. |
| **A1-03** (confirmed/high) | 기준 commit의 `route()`는 `if scope.workflow \|\| binding.Role == "workflow-subagent"`를 `hasPending` 계산 **앞**에서 판정했다. v0.3.1은 `:416`에서 `!hasPending &&`를 붙여 뒤로 옮겼다. 게이트가 "이 binding에 대응하는 pending"이 아니라 "같은 부모 아래 아무 pending"이다. 유지. 도달성은 A1-22 승격으로 보강. |
| **A1-05, A1-06** | 분야 probe 근거 채택. A1-05는 `roles.go:169`의 `dir.prefix == ""` 예외와 `:194-203`의 plugin fallback을 코드로 확인(A1-01과 한 수정으로 정리된다). A1-06은 통합 단계에서 재실행하지 않았다. |
| **A2-01** (confirmed/high) | `native_events.go:259`의 `same := e.NativeTurn == active.Turn`, `:265` 검사 목록에 턴 없음, `:269`의 `r.begin(id)` + `applyNativeTurn`을 코드로 재확인. `continuation.go:39`의 네 조건이 동시에 무너지는 것도 확인. 유지. §2.2의 A3-02 조합 주의. |
| **A2-02** (confirmed/medium) | `results.go:177-187`의 포인터 교체, `:266-288` closure의 `e.State != "awaiting_children"` 가드, `:285-287`의 `d.stopped(*binding)`, `:309`의 `turn != "" &&` 조건을 코드로 재확인. 유지. |
| **A2-03 / A2-05 / A2-13** | 셋 다 `readCurrentNativeTurn`(`native_events.go:117-148`)이 `active/<name>/`를 전수 스캔하면서 **아무도 정리하지 않는** 한 뿌리에서 나온다. M2로 병합. 유지. |
| **A2-04, A2-06, A2-07, A2-11, A2-12** | improvement 유지. 통합 관점에서 추가할 것 없음. |
| **A2-14 / R-01** | 반박 유지. `native_events_publication_test.mjs:44`가 root step 거부를 명시 단언한다는 근거가 충분하다. |
| **A3-01** (confirmed/medium) | `context_compaction.go:61`에 `conversationRequest` 게이트가 없고 `context.go:206`에는 있다. 코드로 재확인. 유지. INT-03으로 병합. |
| **A3-02** (confirmed/medium, 기존결함) | `context.go:284-287`에서 phase를 먼저 쓰고 실패 시 되돌리지 않는다. `s.busy`는 `:292`에서야 true다. 코드로 재확인. 유지. **A2-01의 실패 category 공급원이라는 점 추가**(§2.2). |
| **A3-04 ~ A3-06, A3-11** | 유지. A3-06의 공유 헬퍼가 INT-03의 권장 수정이다. |
| **A3-07 ~ A3-10** | 반박 유지. 특히 A3-08(`s.route = route`가 항등 대입)은 `context.go:271-279`를 직접 읽어 확인했다. |
| **A4-01** (confirmed/medium) | `connection.go:33-39`가 구체 타입이 아닌 `net.Conn` **인터페이스**를 임베드하는 것을 확인했다. `*responseConn`의 메서드 집합에 `CloseWrite`가 없다. 유지. INT-04로 문서 대조 추가. INT-01의 실패가 같은 서브테스트에서 났다는 점도 함께 본다. |
| **A4-02** (improvement/medium) | `gateway.go:167`의 `r.Close && ProtoMajor==1 && ProtoMinor>=1`은 "요청이 close를 요구했는가"이고 제품 응답은 `keep-alive`를 붙인다. 유지. |
| **A4-03** (hold/low) | 유지. INT-01이 이 hold를 풀지 못한다 — 실패한 `closed/*`의 `Connection: close`는 **검사 클라이언트가** 만든 것이다(`connection_test.go:35`). 실제 native 관측은 여전히 없다. |
| **A4-04 ~ A4-07** | improvement 유지. A4-06은 `run.go:344-350`을 직접 읽어 재확인. |
| **A4-R1 ~ R11** | 반박 유지. R7(`n.mu`를 쥔 채 `p.cancel()`)은 `native_cancellation.go:70-81`을 읽어 재확인 — `context.CancelFunc`는 done 채널을 닫고 자식을 취소할 뿐 `nativeEvents.mu`를 다시 잡지 않는다. |
| **A5-01** (confirmed/medium) | 유지. 검증 기록 무결성 결함이고 제품 결함이 아니라는 분류도 맞다. |
| **A5-03** (confirmed/medium) | 통합 단계에서 재확인: `.github/workflows/go.yml`의 4개 단계(`go vet ./...` 56행, `go build ./...` 59, `go test -count=1 ./...` 62, `-race` 69) **어디에도 `-tags`가 없다.** 유지. 다만 이 트리의 태그 컴파일은 지금 정상이다 — `go vet -tags runtime_evidence ./...` exit 0(§1 #3). 즉 "지금 깨져 있다"가 아니라 "깨져도 CI가 모른다"가 정확한 서술이다. |
| **A5-04**(confirmed/low), **A5-05**(hold), **A5-07 ~ A5-09** | 유지. |
| **A5-R1 ~ R12** | 반박 유지. |

### 4.4 중복 병합

| 묶음 | 포함 ID | 하나의 뿌리 | 권고 |
|---|---|---|---|
| **M1** fold 비교 이관 잔여 | A1-04, A2-08, A2-09 | v0.3.1이 역할 비교를 `roleMatches`로 옮기면서 정확 비교 3곳을 남겼다 (`results.go:302`, `workflow.go:279`, `:282`) | 세 곳 모두 오늘은 도달 불가(§4.1). 고친다면 **한 변경에서 셋을 함께** 정리하고, 반대로 `prepare`의 canonical 재작성(`delegation.go:279-281`)이 이 안전망이라는 사실을 그 줄의 주석으로 못 박는다. 그 한 줄이 사라지면 세 결함이 동시에 살아난다. |
| **M2** 활성 턴 디렉터리 미정리 | A2-03, A2-05, A2-13 | `readCurrentNativeTurn`이 `active/<name>/`를 전수 스캔하고 낮은 sequence 쌍을 아무도 지우지 않는다 | 발행 검증 후 낮은 sequence 파일 쌍 제거 하나로 셋이 같이 줄어든다(A2 권고와 동일). |
| **M3** UserPromptSubmit 차단 | A1-11, A2-10 | `app/settings.go:78`이 건 hook과 `cmd/clauduct-hook/main.go:189-197`의 단일 실패 분기 | 두 분야가 같은 채택 정책(이슈 #53)에 대해 서로 다른 절반을 제안했다. 합치면 하나다 — **전송 실패는 1회 재시도, 결정적 4xx는 상태코드/분류를 실은 다른 문구.** 둘 다 improvement. |
| **M4** 두 진입점 admission 복제 | A3-01, A3-06, A3-13 | `previewCompaction`이 `beginContext` 전처리를 복제 | INT-03. 공유 헬퍼 하나. |
| **M5** 연결 종료 경로 | A4-01, A4-02, INT-04 | 같은 `Close`/`finished` 경로 | 두 수정이 상호작용한다(INT-04의 순서 주의). 같이 검토. |

### 4.5 근거가 약한데 confirmed로 나온 것 / 반박이 부실한 것

- **근거 부족 → 강등**: A1-04, A5-02. 두 건 모두 **분야 리뷰어 자신의 verify 산출물이 이미 REFUTED를
  말하고 있는데 보고서 본문이 반영하지 않았다.** 최종 보고서 작성 시 반드시 반영해야 한다.
  (입력 요약 JSON의 `verdicts`에도 `A1-04 code=REFUTED`, `A5-02 code/repro=REFUTED`로 남아 있다.)
- **반박이 부실했던 것**: 없음. 반박 43건을 표본 점검했고(A1-R1~R7, A2-R01, A3-07~11, A4-R1~R11, A5-R1~R12)
  통합 단계에서 뒤집을 근거를 찾지 못했다.
- **모순처럼 보이지만 아닌 것**: A4-R6(sha256 일치)과 A5-01(sha256 불일치)은 **서로 다른 파일**을 가리킨다 —
  A4-R6은 `verification/test-http-transport.mjs`, A5-01은 `go/internal/gateway/socket_runtime_evidence_test.go`다.
  두 보고서를 나란히 읽으면 모순처럼 보이므로 최종 보고서에서 파일명을 붙여 구분할 것.
- **통합 단계에서도 풀리지 않은 hold**: A4-03(실제 native의 `Connection: close` 빈도),
  A5-05(기본 `TEMP`에서 실호출 검수기), A2-13(부분 쓰기), A3-01의 실사용 빈도.
  전부 실제 세션/PTY가 필요하다.

---

## 5. NOT_RUN

| 검사 | 이유 | 추가 확인 방법 |
|---|---|---|
| `go test -race ./...` 전체 | 과제가 전체 회귀 1회만 권했고 전체 race는 이 호스트에서 훨씬 길다. 분야별 좁은 race는 A2 #29·A3 #18·A4 #20이 exit 0으로 기록했다. INT-01은 race가 아니라 부하 의존 실패다 | `CGO_ENABLED=1 CC=C:/msys64/ucrt64/bin/gcc.exe go test -race ./... -count=1 -timeout 1800s` |
| `go test -tags runtime_evidence ./...` 실행 | 태그 파일 다수가 실제 backend 호출 또는 PTY를 요구한다. **컴파일·vet은 실행했다**(§1 #3, exit 0) | 승인된 예산·PTY에서 `CLAUDUCT_EVIDENCE_LIVE=1` / `CLAUDUCT_EVIDENCE_TUI_LOCAL=1` / `CLAUDUCT_SOCKET_EVIDENCE=1` 각 1회 |
| INT-01의 진짜 원인 판정 | 검사가 `readErr`를 버려 사후 판정이 불가능하다. 3회 시도(좁은 5회, app+gateway 동시, 판별 probe) 후 새 증거 없이 재시도하지 않았다 | INT-01 권장 수정 적용 후 `go test ./...` 재실행, 또는 `-count=N` 반복으로 부하 부여 |
| A1-03 / A1-22의 실제 native 종단 재현 | 실제 Workflow 세션 + 형제 pending 동시 성립 필요. 바이너리 추적과 실세션 감사로 **전제**는 확보했고 **순서 의존 잔여**만 남았다 | 실제 Workflow 실행 1회에서 gateway 진단 `Recent[]`의 workflow 자식 첫 요청 클래스 관측 |
| A3-12 / A3-13의 실제 헤더 관측 | A3-12는 바이너리 추적으로 반박됐다. A3-13은 `count_tokens`에 기록 자체가 없다(INT-06) | INT-06 수정 후 실제 세션 1회 |
| Node 31 / .NET 35 / raw TCP 400 전송 검사, C 진단기 빌드·ETW | A4 범위이자 호스트 상태 변경을 수반한다. 이번 라운드 재실행 없음 | A4 보고서 8절 |
| A5-06 round-trip probe | 강등으로 불필요해졌다(§4.1). 필요하면 별도 task worktree에서 | 기준 commit worktree + v0.3.1이 실제로 쓴 journal |
| A1-06의 재실행, 분야 probe 전수 재실행 | 통합 검토자는 대조가 역할이고 같은 단위를 두 번 돌리지 않았다. 분야 로그를 근거로 채택 | 각 분야 repro 디렉터리의 기록된 명령 |
| `/code-review` 스킬 호출 | 같은 단위를 다시 돌리지 않는다. A4·A5에서 결과가 도달하지 않은 실행은 **재실행 근거로 삼지 않았다** | 스킬 transcript 직접 열람 |

---

## 6. 통합 관점의 커버리지 공백

1. **전체 회귀가 이번 라운드 이전에 한 번도 실행되지 않았다.** 5개 분야 전부 `NOT_RUN`으로 넘겼고,
   통합 단계의 단 1회 실행이 바로 실패를 냈다(INT-01). 릴리스 판정 전 최소 1회는 녹색이어야 한다.
2. **CI가 태그 파일을 컴파일하지 않는다**(A5-03). v0.3.1이 태그 파일을 4개 늘렸고 그중 둘은
   환경이 필요 없는 순수 로직 검사다. 지금은 통과하지만(§1 #3) CI가 그 사실을 모른다.
3. **`count_tokens` 엔드포인트가 진단에서 반쯤 투명하다**(INT-06). 클래스 미기록 + 압축 판정이
   `/v1/messages`와 다른 규약 → 이 엔드포인트에 대한 질문은 계정으로 답할 수 없다.
4. **회귀와 그 탐지기가 같이 꺼진 사례가 있다**(INT-02). feature evidence가
   "필수 검사 없이 실행된 자식"을 보고하도록 설계돼 있는데, 그 정의를
   `AgentRole`(거부 전 확정)에서 `Source`(거부 후 확정)로 바꾸면서 거부된 요청 전체가 분모에서 빠졌다.
   **같은 릴리스가 만든 거부를 같은 릴리스가 못 보게 됐다.**
   게이트 조건을 바꿀 때 "이 값이 실패 경로에서도 채워지는가"를 확인하는 규칙이 없다.
5. **새 문서 절과 그 코드가 다른 분야로 쪼개졌다.** ARCHITECTURE §4·§6.1·§7.1은 전부 v0.3.1 신규이고
   각각 A1·A4·A3의 코드 영역을 서술하는데, 문서는 A5가 코드는 다른 분야가 봤다.
   INT-04와 INT-05가 그 틈에서 나왔다. 새 문서 절은 해당 코드 분야가 함께 읽어야 한다.
6. **`prepare`의 canonical 재작성 한 줄(`delegation.go:279-281`)이 세 결함의 유일한 안전망이다**(M1).
   그 사실이 코드 어디에도 적혀 있지 않다. 안전망과 그것이 막는 것이 다른 파일에 있고 서로를 언급하지 않는다.
7. **native 동작 근거가 흩어져 있다.** 이번에 A1-22/A3-12/A5-06을 푼 결정적 근거는
   `verification/parent-wait-20260920/s43-user-audit.json`(실세션 레코드 110건)과
   A3의 바이너리 문자열 추적이었다. 다섯 분야 중 아무도 전자를 열지 않았다.
   "native가 이렇게 한다"를 hold로 넘기기 전에 기존 실세션 감사를 먼저 볼 것.

### 참고 — 실세션 감사 집계 (native 2.1.278, `verification/parent-wait-20260920/s43-user-audit.json`, 레코드 110건)

| kind / path | class | agentId | role | source | 건수 |
|---|---|---|---|---|---|
| count_tokens `/v1/messages/count_tokens` | (미기록) | 없음 | — | backend-count-warmup | 31 |
| generation `/v1/messages` | main | 없음 | — | direct+effort | 24 |
| agent_binding `/clauduct/agents` | — | — | — | — | 19 |
| generation `/v1/messages` | auxiliary | 없음 | — | direct+effort | 13 |
| generation `/v1/messages` | subagent | 있음 | general-purpose | agent-call-model+effort | 6 |
| generation `/v1/messages` | subagent | 있음 | general-purpose | delegation-inherited | 4 |
| context_event `/clauduct/context` | — | — | — | — | 4 |
| generation `/v1/messages` | workflow | 있음 | workflow-subagent | workflow-selection | 3 |
| compaction `/v1/messages` | compaction | 없음 | — | direct+effort | 2 |
| generation `/v1/messages` | **auxiliary** | **있음** | general-purpose | delegation-inherited | **1** |
| count_tokens `/v1/messages/count_tokens` | (미기록) | 없음 | — | exact-count-cache | 1 |
| other `/clauduct/workflows` | — | — | — | — | 2 |

마지막에서 두 번째 행이 A1-22의 핵심 근거다 — **agent 범위 요청에 `auxiliary` 클래스가 붙는 것이 실측된다.**
관측된 역할 철자는 전부 canonical이며 이것이 A1-04/A1-21/A5-06 강등의 보조 근거다.

---

## 7. 재현 자산

`D:\AIDEV\clauduct-v031\verification\v031-code-review-20260922\run-01\repro\INTEGRATION\`

| 파일 | 용도 |
|---|---|
| `full-regression.txt` | `go test ./...` 전체 출력 + `EXIT=1` |
| `go-vet.txt` | `go vet ./...` + `EXIT=0` |
| `go-vet-tagged.txt` | `go vet -tags runtime_evidence ./...` + `EXIT=0` |
| `turnover-narrow-x5.txt` | 실패 테스트 좁은 5회 반복, 전부 PASS |
| `turnover-under-load.txt` | `app`+`gateway` 동시 실행, PASS |
| `int_probe_test.go` | INT-01 판별 probe (overlay로만 `zz_int_probe_test.go`로 주입) |
| `overlay.json` | 현행 트리 + 위 probe |
| `int-probe.txt` | probe 출력 (`truncate=true … parse_failed=true`) |

제품 파일은 한 줄도 바뀌지 않았다. 확인: `git diff --stat` = 118 files / +7895 / -636 (시작과 동일),
`git status --porcelain=v1 | grep '^??'` = `%SystemDrive%/`(원래 있던 것)와
`verification/v031-code-review-20260922/`뿐.
