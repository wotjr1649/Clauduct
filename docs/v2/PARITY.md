# PARITY — 기준선 대비 Go의 실제 격차

2026-09-16 개시. **이 문서는 작업 목록이다.** 테스트 ID 매트릭스가 아니라 기준선
`D:/AIDEV/Clauduct/src`의 실제 제품 표면을 기준으로 삼는다.

## 0. 왜 이 문서가 생겼는가

G9(기본 전환)의 parity 기준을 정하다가 확인한 것: **ID 매트릭스가 작업량을 과소계상한다.**

`agent-selection.mjs`는 511줄에 실패 코드 12종(`SELECTION_FAILURES`), 완료 실패 20종
(`COMPLETION_FAILURES`), 완료 상태 7종(`COMPLETION_STATES`), IO 코드 8종을 갖는다. 대응하는
테스트 ID는 `CAP05`(custom agent)와 `CAP07`(overlay on/off) 둘뿐이고, 둘 다 C 등급이다.
그 둘을 통과시켜도 이 모듈이 하는 일은 검증되지 않는다.

그래서 "C 등급 19건"이라는 숫자로는 진행률도 완료도 말할 수 없다. 모듈과 동작 단위로 다시 센다.

한 번 더: **기준선이 거부하는 것을 Go도 거부하는 것은 동등이다.** 후퇴가 아니다. 예를 들어
non-streaming 요청은 기준선도 `REQUEST_STREAM_FALSE`로 거부한다(`native-protocol.mjs:254`).

## 1. 모듈 대조

`src/*.mjs` 중 제품 모듈만(테스트·fixture·review 제외). 줄 수는 규모의 지표일 뿐이다.

| 기준선 모듈 | 줄 | Go | 상태 |
|---|---|---|---|
| `native-protocol.mjs` | 873 | `protocol/{anthropic,bridge,codex}` | 부분 — 3절 |
| `native-transport.mjs` | 664 | `upstream` | 부분 — 미조사 |
| `native-gateway.mjs` | 653 | `gateway` | 부분 — 2절 |
| `agent-selection.mjs` | 511 | **없음** | 4절 |
| `clauduct.mjs` | 378 | `app`, `launch` | 부분 — 미조사 |
| `workflow-selection.mjs` | 310 | **없음** | 미조사 |
| `request-status.mjs` | 175 | **없음** | 미조사 |
| `rate-limit-observation.mjs` | 142 | 이벤트 이름만 | 미조사 |
| `native-search.mjs` | 107 | **없음** | 미조사 (TOOL12) |
| `agent-route.mjs` | 103 | **없음** | 4절 |
| `native-beta.mjs` | 82 | 부분 | 미조사 |
| `request-admission.mjs` | 68 | `gateway/registry.go` | 동등으로 보임 — 미확인 |
| `scan-native-features.mjs` | 53 | 없음 | 도구, 제품 아님 |
| `retry-after.mjs` + `retry-after-seconds.mjs` | 53 | `upstream/retry.go` | 동등 (LIFE09 PASS) |
| `runtime-paths.mjs` | 44 | `platform` | 부분 — 미조사 |
| `compact-policy.mjs` | 37 | **없음** | 미조사 |
| `http-close.mjs` | 34 | `gateway` | 미조사 |
| `native-delivery.mjs` | 29 | `bridge` barrier | 동등으로 보임 — 미확인 |
| `install-check.mjs` | 26 | `app/package` | 부분 — 미조사 |
| `models.mjs` | 21 | `bridge/route.go` | **부분 — 3.3절** |
| `client-version.mjs` | 16 | `upstream/version.go` | 동등 (WP05) |

**미조사는 미조사다.** 위에서 "동등으로 보임"이라 적은 것도 근거를 붙이기 전에는 동등이 아니다.

## 2. HTTP 라우트

`native-gateway.mjs:146-248` 대 `gateway/gateway.go:167-186`.

| 라우트 | 기준선 | Go | |
|---|---|---|---|
| `HEAD /api/hello` | 204, 인증 선택적 | 동일 | 동등 |
| `POST /v1/messages` | 본체 | 본체 | 3절 |
| `GET /v1/models` | `MODELS` 4종을 `{id, object, owned_by:'openai'}`로 | **404** | **격차** |
| `GET /clauduct/status` | `diagnostics()`, upstream 이름은 보류 | **404** | **격차** |
| `POST /clauduct/agents` | `linkTaskResult`/`linkWorkflow`/`linkResume`/`linkSkill` | **404** | **격차** — 4절 |
| 그 밖 | `UNSUPPORTED_ROUTE` | 동일 | 동등 |

`/clauduct/agents`를 "Go는 overlay를 주입하지 않으니 호출자가 없다"로 넘길 수 없다.
`agentSelection`은 `clauduct.mjs:338`에서 **조건 없이** 생성되어 게이트웨이에 물린다. CAP04(주입 0)는
*사용자 settings에 쓰지 않는다*는 뜻이고, 엔드포인트를 제공하는 것과 다른 문제다.

## 3. 요청 변환

### 3.1 content block

`native-protocol.mjs:355-398` 대 `anthropic/request.go:340-384`.

| 블록 | 기준선 | Go | |
|---|---|---|---|
| `text` | 지원 | 지원 | 동등 |
| `tool_use` / `tool_result` | 지원 | 지원 | 동등 (TOOL01–08) |
| `image` | base64 png/jpeg/gif/webp → `input_image` 데이터 URL. user role 강제 | `UNSUPPORTED_CONTENT` | **격차 — 사용자가 매일 닿는다** |
| `tool_addition` / `tool_removal` | 지원 (`:359,365`) | `UNSUPPORTED_CONTEXT_CHANGE` | **격차** |
| `redacted_thinking` | 지원 (`:398`) | `UNSUPPORTED_CONTENT` | **격차** |
| `tool_reference` (tool_result 안) | 지원 (`:387`) | `UNSUPPORTED_CONTENT` | **격차** |
| `thinking` + `summary_text` | 지원 (`:234`) | 미확인 | **미조사** |

### 3.2 스트리밍

기준선도 `stream !== true`를 `REQUEST_STREAM_FALSE`로 거부한다(`:254`). Go도 거부한다. **동등.**
WIRE16("non-streaming을 지원하면")은 전제가 성립하지 않으므로 구현 대상이 아니다.

### 3.3 모델 선택 — 여기가 생각보다 크다

`models.mjs`가 Go에 없는 것 셋:

| | 기준선 | Go |
|---|---|---|
| `MODELS` 4종 + alias + family | 있음 | **있음** (`bridge/route.go`, CAP01/02 PASS) |
| `DEFAULT_SELECTION` = astra/low (main startup) | 있음 | **없음** |
| `ROLE_MODELS` = Explore→luna, Plan→astra/low, general-purpose→luna | 있음 | **없음** |
| `CONTEXT_POLICY` = window 400000, compactAt 320000, outputReserve 20000 | 있음 | **없음** |

즉 **기준선은 요청된 모델을 그대로 쓰지 않는다.** 서브에이전트의 역할에 따라 경로를 바꾼다
(`native-gateway.mjs:294` — 요청마다 `agentSelection.resolve()`). Go는 클라이언트가 요청한 것을
alias만 거쳐 그대로 쓴다. Explore 서브에이전트가 기준선에서는 luna로 가고 Go에서는 클라이언트가
고른 모델로 간다. **비용이 달라진다.**

이것은 CAP03에 영향을 준다. 역할로 재지정된 경로는 `Route.Source`에 새 근거값이 필요하다.

## 4. agent-selection — 없는 층 전체

`createAgentSelection({ projectsRoot, agentDefinitions })`. 게이트웨이가 부르는 표면:

| API | 어디서 | 하는 일 |
|---|---|---|
| `resolve(binding)` | `:294`, **요청마다** | 서브에이전트의 모델을 결정 |
| `linkTaskResult` | `:167` | `TaskOutput` 결과를 세션에 연결 |
| `linkWorkflow` | `:180` | workflow 실행을 연결 |
| `linkResume` | `:188` | `SendMessage` 재개를 연결 |
| `linkSkill` | `:196` | 백그라운드 skill fork를 연결 |
| `begin` / `delivered` / `failed` | — | 요청 수명 기록 |

검증 방식: `~/.claude/projects/.../<session>/subagents/agent-<id>.meta.json`을 읽되
**symlink가 native project tree 밖으로 나가면 거부**하고(`within()` + `realpath` 대소문자 일치),
`x-claude-code-session-id`·`x-claude-code-parent-agent-id` 헤더와 대조한다(`:314-315`).

**CAP06 재검토 필요.** Go에서 correlation header 부재는 문제가 아니라고 방금 고정했다
(`TestAMissingCorrelationHeaderIsNotARoutingProblem`). 기준선에서는 그 헤더가 selection을 검증하는
**부하가 걸린 입력**이다. agent-selection을 이식하면 그 테스트의 의미가 바뀐다.

바인딩을 보내는 쪽은 `agent-route.mjs`(103줄)로, native hook에서 stdin으로 hook 이벤트를 받아
`POST /clauduct/agents`로 보낸다. 대상 hook: `PostToolUse`(TaskOutput·Workflow·SendMessage·Skill),
`SubagentStart`, `SubagentStop`.

## 5. 다음

- [ ] `native-transport.mjs` 664줄 대조
- [ ] `native-gateway.mjs` 나머지(진단·admission·http-close) 대조
- [ ] `native-protocol.mjs` 응답 경로 대조
- [ ] `clauduct.mjs` 378줄 — 종료 JSON·notice·옵션
- [ ] `workflow-selection.mjs`, `request-status.mjs`, `rate-limit-observation.mjs`, `native-search.mjs`, `native-beta.mjs`, `compact-policy.mjs`
- [ ] 위가 끝나면 WP07을 이 원장으로 다시 정의하고, 예산 필요량을 산정한다

## 6. Go 런타임 — 미해결 상류 결함

기록만 한다. 조치는 실측 후.

| 이슈 | 마일스톤 | 우리와의 관계 |
|---|---|---|
| #81411 concurrent Read/Close deadlock (HTTP/1 automatic draining) | **1.27.2, open** | upstream이 h2면 무관. ALPN 실측 필요 |
| #81515 readLoop이 Close 뒤 ContentLength 읽음 | 1.27.2, open | `-race` CI 간헐 실패의 후보 |
| #81516 `Body.Close`가 앞선 Read 오류 반환 | 1.27.2, open | Close 오류를 무시하므로 무관 |
| #81374 / #80979 상속 overlapped 파이프가 형제 `WriteFile`을 hang | **1.27.2, open** | Go 1.25 회귀. application-level workaround 없음 |

go1.27.1(2026-09-01)은 위 중 아무것도 포함하지 않는다. 그 안의 유일한 net/http 수정 #81027은
서버 쪽 `Request.Body.Close` 의미론이고 이 코드는 `r.Body.Close`를 호출하지 않는다.
**1.27.1 업그레이드가 이 프로젝트에 준 효과: 없음.**

#81374는 G9와 직접 관련이 있다. 오늘 기본값은 Node이므로 노출되지 않는다. 기본을 Go로 바꾸면
그 노출이 모든 세션에 생긴다.
