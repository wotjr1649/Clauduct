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

## 5. launcher — 가장 큰 격차이고, Go가 의도적으로 미뤄둔 것

`clauduct.mjs:180-218` 대 `launch/launch.go`.

Go는 argv를 **그대로** 넘기고 환경변수 5개만 덮는다. 기준선은 그렇지 않다.

### 5.1 자식 argv에 주입하는 것

```
--model <codex 모델>  --effort <effort>
--settings <JSON>     --agents <JSON>
[--append-system-prompt DOCUMENT_FIRST_PROMPT]
...사용자 인자
```

`--settings`·`--agents`는 **argv JSON이지 파일이 아니다.** 그래서 기준선도 CAP04(사용자 설정 파일에
쓰지 않음)를 만족한다. Go의 "주입 0"은 CAP04가 요구한 것보다 더 나아간 상태다.

`--model`/`--effort`는 launcher가 자기 옵션으로 소비해서(`ownedOptions`) **해석된 Codex 모델로 바꿔**
자식에게 다시 준다. `clauduct --model sol` → 자식은 `--model gpt-5.6-sol --effort xhigh`.

### 5.2 `settings.env` — Go의 5개 대 기준선의 16개

| 키 | 기준선 | Go | 효과 |
|---|---|---|---|
| `ANTHROPIC_BASE_URL` | 설정 | **설정** | 동등 |
| `ANTHROPIC_AUTH_TOKEN` | 설정 | **설정** | 동등 |
| `ANTHROPIC_API_KEY` / `CLAUDE_CODE_OAUTH_TOKEN` / `ANTHROPIC_CUSTOM_HEADERS` | 빈 문자열 | **동일** | 동등 (ENV02) |
| `CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK=1` | 설정 | 없음 | **격차 — 3.2절 재검토** |
| `CLAUDE_CODE_RETRY_WATCHDOG=0` | 설정 | 없음 | 격차 |
| `DISABLE_TELEMETRY=1`, `DISABLE_ERROR_REPORTING=1` | 설정 | 없음 | **격차 — Anthropic 쪽 보고는 여기서 갈 곳이 없다** |
| `CLAUDE_CODE_RESUME_INTERRUPTED_TURN=0` | 설정 | 없음 | 격차 |
| `ANTHROPIC_DEFAULT_HAIKU_MODEL` = luna | 설정 | 없음 | 격차 |
| `ANTHROPIC_DEFAULT_SONNET_MODEL` = luna | 설정 | 없음 | 격차 |
| `ANTHROPIC_DEFAULT_OPUS_MODEL` = sol | 설정 | 없음 | 격차 |
| `ANTHROPIC_CUSTOM_MODEL_OPTION(+_NAME,_DESCRIPTION)` | 설정 | 없음 | 격차 — 모델 피커 |
| `CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1` | 설정 | 없음 | **격차 — `GET /v1/models`가 불리는 이유** |
| `CLAUDE_CODE_DISABLE_ADVISOR_TOOL=1` | 설정 | 없음 | **격차 — advisor는 Codex backend에서 실행될 수 없다** |

**3.2절 정정.** "기준선도 non-streaming을 거부하니 동등"이라고 적었는데 절반만 맞다. 기준선은
거부하는 데 더해 **클라이언트의 non-streaming fallback 자체를 꺼둔다.** Go는 끄지 않으므로,
클라이언트가 fallback을 시도하면 Go 세션은 거부를 만나고 기준선 세션은 애초에 시도하지 않는다.
같은 거부가 아니다.

### 5.3 `settings.modelPicker`

`replaceBuiltInOptions: true`에 4개 Codex 모델을 채운다. 없으면 사용자의 `/model` 목록은 이 backend에
존재하지 않는 Anthropic 모델들이다. **HTTP08(`/v1/models`)은 여기에 묶여 있다.**

### 5.4 `settings.hooks`

`SubagentStart`·`SubagentStop`·`PostToolUse(Skill|SendMessage|Workflow|TaskOutput)` →
`"<node>" "<agent-route.mjs>"`, timeout 5. 이것이 4절의 바인딩을 보내는 주체다.

### 5.5 `--agents` 정의 생성

`sessionAgentDefinitions()`가 모델×effort 조합마다 `clauduct-<family>-<effort>` 에이전트를 만들고
`clauduct-inherit`도 만든다. 각각 description·prompt·tools를 갖는다. 즉 **사용자에게 모델을 고르는
에이전트 타입이 노출된다.**

### 5.6 이것이 WP01의 결론을 바꾸는가

바꾼다. `launch.Build`의 주석은 "제품 launcher는 어떤 옵션도 소유하지 않는다"이고, 그 밑에
"context window·model defaults·retry·telemetry·compaction은 게이트웨이가 아직 구현하지 않은 정책이라
지금 키를 넣으면 코드가 뒷받침하지 않는 동작을 주장하게 된다"라고 적혀 있다.

**그건 유예였지 최종 설계가 아니었다.** 이제 게이트웨이가 그만큼 왔으므로 유예를 푸는 것이 맞다.

살아남는 것: ARG05(옵션 값 안의 모델명을 가로채지 않음)는 파서가 생겨도 유지된다 — 기준선도
파서를 갖고 그 구분을 한다(`nativeValueOptions`·`optionalNativeValueOptions`). 전달 인자의
argv 충실성(ARG01–03, ARG09–10)도 그대로다.

바뀌는 것: "파서가 없으므로 구조적으로 성립"이라는 **근거**는 더 이상 쓸 수 없다. 파서가 생기면
그 성질들은 파서가 지켜야 하고, 테스트가 다시 그것을 증명해야 한다.

## 6. 기능 모듈

### 6.1 `native-search.mjs` — WebSearch가 Go에서 동작하지 않는다

기준선은 클라이언트의 WebSearch **side query를 가로채 직접 답한다.** 그 요청은 추론이 아니라
backend의 **별도 standalone search 엔드포인트**로 가는 검색 왕복이다.

탐지 조건(`searchSideQuery`)은 전부 클라이언트가 스스로 만드는 모양이라 근접 오탐이 일반 경로로
빠진다: `tools` 길이 1 · `type: web_search_20250305` · `name: web_search` · `tool_choice`가
undefined/auto/해당 tool · 메시지 1개 user · 본문이 `"Perform a web search for the query: "`로 시작.

응답은 `server_tool_use` + `web_search_tool_result` + 텍스트 블록으로 **합성**한다. 검색 결과는
정의상 공격자 영향 아래 있는 웹 콘텐츠이므로 `SEARCH_LIMITS`(query 2048, results 20, title 512,
url 2048, output 100000)로 묶고 제어문자를 걸러내며 http/https만 통과시킨다. 링크도 텍스트도 없는
응답은 **빈 검색이 아니라 실패한 검색**으로 처리한다(`SEARCH_RESULTS_EMPTY`).

쿼리만 올라간다 — 클라이언트가 함께 보낼 수 있는 대화 꼬리는 **의도적으로 뺀다.**

**Go**: `anthropic/tools.go:76-78`이 `^web_search_20\d{6}$`를 만나면 `HOSTED_TOOL_UNSUPPORTED`로
거부한다. 즉 사용자가 WebSearch를 쓰면 **요청이 실패한다.** (`codex/events.go:83`에
`response.web_search_call.*`를 "deferred to WP07"로 적어둔 그 항목이다.)

### 6.2 `native-beta.mjs` — 베타 이름은 절대 거부하지 않는다

측정된 규칙이다: **베타 이름 하나를 거부하면 기능이 꺼지는 게 아니라 요청 전체가 죽는다 — 실세션에서
WebFetch가 깨지는 것을 관찰했다.** 그래서 거부하는 것은 **형식이 깨진 헤더뿐**(`INVALID_BETA_HEADER`:
빈 항목 또는 중복). 대신 세 목록으로 분류만 한다.

- `NATIVE_BETAS` — 통과로 아는 것(+ `poc/gateway.mjs`의 `READ_BRIDGED_BETAS`)
- `UNSUPPORTED_BETAS` — 27개, 이름 → 고정 라벨(`STRUCTURED_OUTPUTS`, `MCP_SERVERS`, `FILES_API`,
  `ADVISOR_TOOL`, `TOKEN_COUNTING` …). 진단용이며 거부하지 않는다
- `SERVER_DEPENDENT_BETAS` — 7개, Anthropic 서버가 있어야 동작하므로 이 backend에서는 이미 무력.
  feature scan이 신규로 보고하지 않도록 목록에만 올린다

**Go**: `anthropic-beta` 헤더를 **읽지 않는다.** 금지 헤더 목록에도 없으므로 통과한다. 결과적으로
"거부하지 않는다"는 안전 규칙은 **누락으로 만족**하지만, `INVALID_BETA_HEADER` 검사도 없고
판정/미지 베타 진단도 없다. 동등이 아니라 우연한 일치다.

### 6.3 `native-delivery.mjs` — WIRE13의 기준선 대응물이고, 값이 일치한다

```
청크 16 KiB → 쓰기 → false면 drain 대기 → 타임아웃 30000 ms → DELIVERY_TIMEOUT
매 청크마다 signal.aborted(CANCELLED)·response.destroyed(CLIENT_DISCONNECTED) 확인
```

**Go의 `writeStall = 30 * time.Second`는 기준선의 `timeoutMs = 30000`과 같은 값이다.** WIRE13에서
고른 값이 기준선과 독립적으로 일치했다. 그 값의 근거가 하나 늘었다.

**남은 차이 하나: 청킹.** `anthropic/response.go:49` `Frame.WriteTo`는 프레임 전체를 `w.Write` **한 번**에
쓴다. 기준선은 16 KiB로 쪼개고 각 조각마다 backpressure를 기다린다. 이것이 WIRE13 실측에서 만난
"배치 하나가 메가바이트가 되면 단일 쓰기가 bound를 넘는다"는 문제의 기준선 쪽 해답이다.
**16 KiB 청킹을 넣으면 bound의 의미가 기준선과 같아진다.**

### 6.4 `compact-policy.mjs` — 압축 요청 식별

Claude 2.1.263이 압축 요청을 감싸는 정확한 접두/접미 문구를 공백 정규화 후 대조해
`{lastRole, textBlocks, mixedBlocks, prefixMatches, suffixMatches, matches}`를 낸다.
`<system-reminder>`로만 이루어진 텍스트 블록은 제외하고, **과거 요약·assistant 텍스트·도구 출력은
라우팅 목적으로 들여다보지 않는다.** 원본 프롬프트는 재작성하지 않는다.

**Go**: 없음.

### 6.5 `rate-limit-observation.mjs` — 관찰이지 예산이 아니다

upstream 응답 **헤더**에서 `*-{primary,secondary}-{used-percent,window-minutes,reset-at}`을 읽어
`missing`/`partial`/`observed`/`invalid` 상태와 무효 사유(`header-shape`·`duplicate`·
`numeric-format`·`numeric-range`)를 낸다. 다른 family는 최대 8개까지 이름만 센다.
**지출을 승인하지도, 남은 토큰을 추정하지도, 요청 예산을 바꾸지도 않는다.**

**Go**: `codex` 이벤트에 `rate_limits.updated`/`CodexRateLimits`가 있다. 그건 **SSE 이벤트**이고
기준선이 읽는 것은 **HTTP 응답 헤더**다. 출처가 다르다. 헤더 관찰은 없음.

### 6.6 `request-status.mjs` — 종료 JSON

`CLAUDUCT_REQUEST_STATUS` 한 줄로 나가는 세션 요약. 담는 것:

clientVersion/referenceClientVersion/status · clientContextPolicy(window·autoCompactWindow·
compactPercent, 환경에서 상속) · clientExecutionPolicy(nonStreamingFallbackDisabled) ·
correlationScope · admission(active·queued·queuedTotal·timedOutTotal·maxWaitMs·oldestWaitMs) ·
unsupportedEventNames · unknownBetaNames · judgedBetaLabels · requestOutcome(not-observed/
has-failures/in-progress/no-requests/all-succeeded) · lifetime(transportRejectionsByEvent,
transportClientErrorsByCode …).

세션 중에는 `readRequestStatus(env)`가 `GET /clauduct/status`로 읽는다. 단 **캡처한 upstream 이벤트
이름은 in-session API가 의도적으로 보류**하므로 stdout의 종료 줄이 그 유일한 사본이다.

**Go**: `app.Result`에 Attempts·Inferences·NativeExitCode·ExitCodeUnknown뿐. 종료 JSON 없음.

### 6.7 `workflow-selection.mjs` — workflow 실행 검증

`projectsRoot` 아래 저널을 읽어 workflow 실행의 출처·저장된 결과·자식 경로를 검증한다.
스크립트는 sha256 다이제스트로 고정하고(`workflowDigest`), resume은 `resumeFromRunId`와
`scriptPath` 일치를 요구한다. `agent-selection`의 `linkWorkflow`가 이것을 부른다.

**Go**: 없음.

## 7. 다음

- [ ] `native-transport.mjs` 664줄 대조
- [ ] `native-gateway.mjs` 나머지(진단·admission·http-close) 대조
- [ ] `native-protocol.mjs` 응답 경로 대조
- [ ] `clauduct.mjs` 378줄 — 종료 JSON·notice·옵션
- [ ] `workflow-selection.mjs`, `request-status.mjs`, `rate-limit-observation.mjs`, `native-search.mjs`, `native-beta.mjs`, `compact-policy.mjs`
- [ ] 위가 끝나면 WP07을 이 원장으로 다시 정의하고, 예산 필요량을 산정한다

## 8. Go 런타임 — 미해결 상류 결함

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
