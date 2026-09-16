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
| `GET /v1/models` | `MODELS` 4종을 `{id, object, owned_by:'openai'}`로 | **구현 완료 2026-09-16** | 동등 |
| `GET /clauduct/status` | `diagnostics()`, upstream 이름은 보류 | **404** | **격차** |
| `POST /clauduct/agents` | `linkTaskResult`/`linkWorkflow`/`linkResume`/`linkSkill` | **등록/해제 구현 완료.** 완료 연결은 보류 | 부분 |
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
| `image` | base64 png/jpeg/gif/webp → `input_image` 데이터 URL. user role 강제 | **구현 완료 2026-09-16** | 동등. 돌연변이 8/8 |
| `tool_addition` / `tool_removal` | 지원 (`:359,365`) | **구현 완료 2026-09-16** | 동등. 베타 게이트 포함 |
| `redacted_thinking` | 지원 (`:398`) | **완료 2026-09-16** (해독·생성 양쪽) | 동등 — 3.4절 |
| `tool_reference` (tool_result 안) | 지원 (`:387`) | **이미 지원됨** (`tools.go:304`) | 동등 — 최초 기재가 틀렸다 |
| `thinking` + `summary_text` | 지원 (`:234`) | 미확인 | **미조사** |

**텍스트 블록 묶음이 다르다(2026-09-16 발견).** 기준선은 텍스트 블록 **하나마다** input 항목을
따로 만든다(`native-protocol.mjs:356`). Go는 한 턴의 텍스트를 모아 항목 하나로 보낸다. 모델이 보는
내용은 같고 실세션이 통과했으므로 깨지지는 않지만, wire 모양은 다르다. 이미지는 기준선의 검증된
모양을 그대로 따랐다 — flush 후 자기 항목, role은 user.

### 3.4 추론 왕복 — 절반 닫았다 (2026-09-16)

`redacted_thinking`은 단순 블록이 아니라 **추론 왕복**이다. 기준선은 backend가 준
`encrypted_content`를 `clauduct-reasoning-v1:` 봉투에 base64url로 담아 클라이언트 전사에 넣고,
다음 턴에 그것을 되받아 backend 입력으로 돌려준다. 그래야 모델이 턴마다 사고를 처음부터 다시
시작하지 않는다.

Go는 `include:["reasoning.encrypted_content"]`를 **요청하면서 돌아온 것을 버리고 있었다.** 요청해
놓고 버리는 것은 앞뒤가 맞지 않는다.

- **해독(요청측) 완료.** 봉투·base64url·내부 JSON·`summary_text` 파트까지 전부 검사하고, assistant
  턴이 아니면 거부한다. 이것만으로도 **Node가 기록한 전사를 Go에서 이어받을 수 있다**(CAP09) —
  이전에는 `UNSUPPORTED_CONTENT`로 전사 전체가 거부됐다.
- **생성(응답측) 완료.** reasoning 항목을 봉투에 담아 `redacted_thinking` 블록으로 내보낸다.
  순서는 기준선과 같은 **text → reasoning → tools**이고, tool call과 같은 **delivery barrier 안**에
  있다 — 중간에 실패한 스트림이 모델의 사고 기록 일부를 전사에 남기면 다음 턴이 그것을 완전한 것처럼
  되돌려 보낸다.
- `encrypted_content`가 없는데 summary나 content가 있으면 `MISSING_ENCRYPTED_REASONING`으로 거부한다.
  보존할 수 없는 사고를 조용히 버리면 답은 평범해 보이고 다음 턴은 있어야 할 것보다 적게 시작한다 —
  거부보다 조용하고, 그래서 더 나쁘다.
- **쓴 것을 곧바로 읽어본다.** 쓰는 쪽과 읽는 쪽은 세션 하나만큼 떨어져 있어서, 눈으로 맞춰본 것은
  맞춰본 것이 아니다. 돌연변이에서 이 자체검사를 빼면 살아남았고, 디코더가 받지 않는 id를 주는
  테스트를 추가해 잡았다.
- **사고만 있고 답이 없는 응답은 빈 응답이다.** 불투명한 기록 하나만 든 메시지를 내보내면 사용자에게는
  빈 답이 보이고 클라이언트에게는 성공한 턴으로 보인다.

봉투 접두사는 **interop 계약**이다. 한쪽이 바꾸면 다른 쪽이 기록한 사고가 전부 읽히지 않는다.
리터럴을 직접 단언하는 테스트가 있다 — 모든 다른 테스트는 상수를 써서 만들기 때문에 상수가
드리프트해도 전부 초록으로 남는다.

### 3.2 스트리밍

기준선도 `stream !== true`를 `REQUEST_STREAM_FALSE`로 거부한다(`:254`). Go도 거부한다. **동등.**
WIRE16("non-streaming을 지원하면")은 전제가 성립하지 않으므로 구현 대상이 아니다.

### 3.3 모델 선택 — 여기가 생각보다 크다

**2026-09-16: alias 표를 고쳤고, 표 세 개를 하나로 합쳤다.**

기준선의 표는 Claude 4개 tier를 backend 3개 모델에 얹는다 — `sonnet`과 `haiku`가 **둘 다 luna**로
가고, **terra에는 Claude 이름이 하나도 없다.** 그래서 사용자 피커의 두 항목이 같은 경로로 돌고
네 번째 모델은 선택 자체가 불가능했다. 사용자 결정으로 `sonnet → terra`로 고쳤다. 기준선과의
**의도적 divergence**이며 `route_test.go` 주석에 사유를 적었다.

```
opus   → sol   (xhigh)      sonnet → terra (high)
haiku  → luna  (max)        fable  → astra (medium)
```

그리고 카탈로그·alias·family가 각각 다른 표에 있던 것을 `bridge.Models` **한 슬라이스**로 합쳤다.
라우팅 표·발행 순서·클라이언트 모델 목록·클라이언트 tier 기본값이 전부 거기서 파생된다. backend에
모델이 추가되면 **줄 하나만 늘리면 된다.** 표가 갈라져 있었다는 것이 애초에 terra가 고아가 된 이유다.

`TestEveryModelIsReachableByAClaudeName`이 그 불변식을 지킨다: 모든 모델은 Claude 이름으로 도달
가능해야 하고, 두 모델이 같은 이름을 공유해서는 안 된다. 돌연변이로 확인 — sonnet을 luna로
되돌리거나 이름 없는 모델을 추가하면 잡힌다.

**배경 tier의 effort는 기준선 그대로 둔다(사용자 결정).** `haiku → luna/max`. 실측상 클라이언트는
일반 턴에 effort를 직접 보내므로(`source=family+effort`) 카탈로그 기본값은 폴백이다.

#### 3.3.1 실측 — 클라이언트가 실제로 보내는 것 (2026-09-16)

실제 `claude -p`를 fixture backend에 붙여 잰 것이고 추론 비용은 0이다.

```
REQ 0 requested=claude-opus-5  -> gpt-5.6-sol/high  source=family+effort  tools=false  3.8 KB
REQ 1 requested=claude-opus-5  -> gpt-5.6-sol/high  source=family+effort  tools=true  78.6 KB
```

두 가지가 나왔다. **클라이언트는 effort를 직접 보낸다** — sol의 카탈로그 기본값은 `xhigh`인데
실제로 온 것은 `high`다. 그리고 **보조 요청(3.8 KB)도 주 모델로 간다.** 기준선은
`ANTHROPIC_DEFAULT_HAIKU_MODEL`을 luna로 두어 그것을 싸게 만드는데, Go는 `ANTHROPIC_*`를 전부
떨어뜨리므로 그 키를 설정할 수 없다. **제목 생성 같은 버리는 작업이 가장 비싼 모델에서 돈다.**
B1이 닫는다.



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

**Go: 구현 완료(2026-09-16).** `bridge/search.go`가 탐지·요청 생성·결과 검증·응답 합성을 하고,
`upstream/search.go`가 `/responses` → `/alpha/search`로 **유도한** 주소에 `originator: codex_exec`로
보낸다. 유도가 실패하면(엔드포인트가 그 경로로 끝나지 않으면) 추측하지 않고 `INVALID_ENDPOINT`로
거부한다 — 검색이 추론 엔드포인트로 가는 것이 돌연변이 중 **테스트 없이는 잡히지 않던 항목**이었다.

재시도는 정확히 1회, 그것도 통과할 수 있는 실패에만. 404/410은 alpha 엔드포인트가 사라진 것이므로
기능의 끝이고 재시도하지 않는다. **ledger를 건드리지 않는다** — 검색은 추론이 아니고, 추론 단위로
표현된 상한이 검색을 세면 그 상한은 더 이상 그 뜻이 아니다.

**의도적 divergence 하나.** 도구는 실렸는데 side query 모양이 아닌 요청을 기준선은 **도구를 떨어뜨리고
모델로 보내 검색 결과 없이 성공**시킨다(notice만 남긴다). Go는 `HOSTED_TOOL_UNSUPPORTED`로 거부한다.
조용히 아무것도 못 찾는 WebSearch보다 깨졌다고 말하는 쪽이 낫다 — 후자만 고쳐지기 때문이다.

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

**남은 차이였던 청킹은 닫았다(2026-09-16).** `gateway/messages.go`의 `chunkedWriter`가 16 KiB마다
쓰기 데드라인을 새로 걸고 그만큼씩 내보낸다. 실측: 164,645 바이트가 16 KiB짜리 bound 16번으로 나갔다.

이 수정 전에는 "전체 응답에 한 번만 건다" 돌연변이가 **0.5 ms 차이로** 겨우 잡히고 있었다.
이제는 결정적으로 잡힌다 — 프레임 하나가 열 청크를 넘으면 bound 수가 곧바로 달라진다.

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

## 6.8 실세션 검증 — A그룹 (2026-09-16)

`clauduct-dev probe parity --send`. luna/low, 추론 4회 + 검색 1회. 누적 34/100.

```
image          6 frames, reply 5 chars
tool change    8 frames, thought 2104 chars, reply 2 chars
reasoning out  7 frames, reply 6 chars
reasoning back 7 frames, reply 12 chars
search         31989 bytes back, 20 links over 14 hosts, 10149 chars of text, 11 frames out
reading all five reached the backend and came back in the shape this build expects
```

**`reasoning back`이 핵심이다.** backend가 **우리 봉투에 담긴 자기 암호화 기록을 되받았다.** 그건
fixture로 확인할 수 있는 종류의 것이 아니다.

### 6.8.1 라이브가 또 오프라인이 못 본 것을 찾았다

**검색이 HTTP 400이었다.** 404가 아니므로 주소와 자격증명은 맞았고 **본문이 틀렸다.** 두 가지가
빠져 있었다:

- 본문 맨 앞의 `id` — 프로세스당 세션 UUID. 기준선은 호출자가 넣은 것을 **덮어쓴다**
- `x-codex-turn-metadata` 헤더 — 17개 필드짜리 codex 형 turn 봉투

둘 다 넣자 통과했다. **오프라인 테스트 15개는 그동안 전부 초록이었다** — fixture는 건네받은 것을
그대로 받아들이기 때문이다. 이 프로젝트에서 세 번째다.

회귀 테스트를 달았고 돌연변이 6/6이 잡는다. 세션은 transport당 하나, turn은 요청마다 새로 만든다 —
매번 새 세션을 만들면 모든 검색이 첫 검색으로 보인다.

### 6.8.2 그리고 probe가 승인보다 비싸게 쓰고 있었다

`wire` probe의 요청에 effort가 없어서 `SelectRoute`가 luna의 카탈로그 기본값 **max**를 썼다. 승인된
예산은 luna **low**다. 추론 토큰이 호출 비용의 대부분이므로 이건 실제 과지출이다.

CAP03에서 넣은 **"본문과 대조해서 인가한다"** 검사가 이걸 드러냈다. 그 전에는 transport의 고정
필드(luna/low)로 예약하고 본문에는 max를 실어 보냈다. probe가 effort를 명시하도록 고쳤다.

## 7. `native-transport.mjs` — 재시도와 한계값

### 7.1 한계값 대조

| | 기준선 `NATIVE_TRANSPORT_LIMITS` | Go |
|---|---|---|
| `maxFrameBytes` | 8 MiB | 8 MiB — 동등 |
| `maxEvents` | 100,000 | 100,000 — 동등 |
| `maxResponseBytes` | `NATIVE_LIMITS.responseBytes` (16 MiB) | 16 MiB — 동등 |
| `timeoutMs` | 600,000 (전체) | **없음.** 대신 phase별(handshake 30s, 응답 헤더 120s) |
| `maxRetries` | **5** | **0** — 7.2절 |
| `retryBaseMs`/`retryMaxMs` | 100 / 2,000 | 해당 없음 |
| `retryAfterMaxMs` | 5,000 (in-process 대기 상한) | 자지 않고 **보고**한다 (Deferred) |
| `maxSockets`/`maxFreeSockets`/`idleSocketMs` | ∞ / 2 / 600,000 | `DefaultTransport` 기본값 |

Go에 **전체 timeout이 없다**는 점은 기록해 둔다. WIRE14는 phase별이 하나의 전체 deadline보다 낫다고
판정했고 그건 유효하지만, 기준선에는 그 위에 10분 상한이 또 있다. 둘은 배타적이지 않다.

### 7.2 재시도 — 설계 차이지 단순 격차가 아니다

기준선은 **연결 실패**를 최대 5회 재시도한다(100ms→2000ms). 재시도 대상은
`UPSTREAM_IDLE_TIMEOUT`·`UPSTREAM_DNS_ERROR`·`UPSTREAM_IO_ERROR`(단 `HPE_` 접두 제외)이고,
TLS 오류·권한 거부는 **재시도 루프에 들어가지 않는다**(`connectionFailure`의 `retryable`).

Go는 `MaxGatewayRetries = 0`이고 그 근거가 측정이다: 설치된 claude 2.1.272가 **5xx를 스스로
재시도한다(60초에 8회)**. 게이트웨이가 또 재시도하면 곱해진다.

두 근거 모두 유효하다. 결과 차이는 **비용의 위치**다. 일시적 DNS 장애에서 기준선은 한 요청 안에서
조용히 회복하고, Go는 503을 돌려주어 클라이언트가 8번 재시도한다. 회복은 양쪽 다 되지만 시도 수가
다르다. **누가 재시도를 소유하는가는 결정 사항이고, 이 원장은 그 결정이 미결임을 기록한다.**

### 7.3 실패 분류

기준선: `UPSTREAM_IDLE_TIMEOUT` / `UPSTREAM_TLS_ERROR` / `UPSTREAM_ACCESS_DENIED` /
`UPSTREAM_DNS_ERROR` / `UPSTREAM_IO_ERROR`. 인증서 코드 7종을 명시 집합으로 갖고, 그 밖에
`CERT_`·`ERR_TLS_`·`ERR_SSL_`·`ERR_OSSL_` 접두도 TLS로 본다. **원본 Node/OpenSSL 메시지와
미인식 코드는 진단에 넣지 않는다.**

Go: `Failure{Category, Disposition}`, LIFE13에서 11종 PASS. 대체로 대응하나 이름이 다르고
1:1 대조는 **미완**이다.

검색 전용 분류도 있다: `searchConnectionFailure`가 `UPSTREAM_IO_ERROR`를 `SEARCH_HTTP_ERROR`로
바꾼다. Go에는 검색 경로 자체가 없다(6.1절).

## 8. `native-gateway.mjs` — 요청 파이프라인

`REQUEST_STAGES` = `request` → `selection` → `prepare` → `review` → `upstream` →
`output-validation` → `delivery`. Go에는 stage 개념이 없다.

`POST /v1/messages`가 순서대로 하는 일(`:248-300`) 중 Go에 **없는** 것:

| 단계 | 기준선 | Go |
|---|---|---|
| `x-claude-code-{session,agent,parent-agent}-id` 모양 검사 | `INVALID_SESSION_ID` | **읽지 않음** |
| `anthropic-version === '2023-06-01'` | `UNSUPPORTED_VERSION` | **검사 없음** |
| `anthropic-beta` 형식 검사 + 미지/판정 베타 수집 | 있음 | **없음** (6.2절) |
| `content-encoding` 부재 또는 `identity` | `UNSUPPORTED_ENCODING` | **검사 없음** (content-type만) |
| 요청별 timing 10개 스탬프 + attempts[] + retryScheduledMs[] | 있음 | **없음** |
| `sessionRef`/`agentRef`/`parentRef` 해시 참조 | 있음 | **없음** |
| 최근 요청 16개 링 버퍼 | 있음 | **없음** |
| agent 등록 고정(LRU) 후 admission | 있음 | **없음** |
| `stage = 'selection'` → `agentSelection.resolve()` | 있음 | **없음** (4절) |
| `timing.requestedModel` 기록 | 있음 | CAP03로 **부분 대응** |

**헤더 검사 순서가 의도적이다.** 요청을 `recentRequests`에 먼저 넣고 나서 버전·베타·인코딩을 본다 —
"거부된 버전·베타·인코딩이 **기록되지 않은 400**이 아니라 진단된 실패가 되도록."

Go의 `checkBoundary`는 금지 헤더(Cookie·Proxy-Authorization·Origin·Sec-Fetch-Site·Forwarded)와
토큰만 본다. 버전·인코딩·세션 ID 모양은 보지 않는다.

### 8.1 실제 클라이언트가 보내는 헤더 (2026-09-16 실측)

fixture backend에 붙인 실제 `claude -p`, 추론 비용 0. 클라이언트는 claude-cli/2.1.273.

```
HEAD /api/hello   User-Agent=Bun/1.4.3, Authorization 없음
GET  /v1/models   Anthropic-Version=2023-06-01, Authorization 있음, Anthropic-Beta 없음
POST /v1/messages Anthropic-Version=2023-06-01 | Content-Encoding 없음
                  X-Claude-Code-Session-Id=<UUID> | X-App=cli | X-Stainless-* 7종
                  Anthropic-Beta=claude-code-20250219, interleaved-thinking-2025-05-14,
                    thinking-token-count-2026-05-13, context-management-2025-06-27,
                    prompt-caching-scope-2026-01-05, mid-conversation-system-2026-04-07,
                    mid-conversation-tool-changes-2026-07-01, effort-2025-11-24
```

세 가지가 나왔다.

**`mid-conversation-tool-changes-2026-07-01`을 기본으로 보낸다.** A4a의 베타 게이트는 실제 세션에서
항상 열려 있다 — 구현하지 않았다면 도구 변경이 매번 거부됐을 것이다.

**경계 검사를 넣어도 안전하다.** 버전은 정확히 `2023-06-01`, `Content-Encoding`은 아예 없고,
세션 id는 UUID다. 추측으로 넣었다면 모든 요청이 깨졌을 수 있다 — 그래서 먼저 쟀다.

**`/v1/models`에는 `Anthropic-Beta`가 없다.** 버전 검사를 그 경로에도 걸면 통과하지만, 기준선처럼
`/v1/messages` 안에서만 검사한다.

## 9. 응답 경로 — 진단 어휘

`EVENT_DIAGNOSTIC_TYPES` 약 60개. 여기에는 Codex `ThreadEvent` 태그(`thread.started`,
`turn.*`, `item.*`)도 들어 있는데 **라벨일 뿐이고 게이트웨이는 그것들을 여전히 거부한다** —
thread/turn/item 지원을 함의하지 않는다.

- `EVENT_TYPE_FORMATS` 6종: `missing`·`non-string`·`empty`·`oversized`·`identifier`·`other`
- `KEEPALIVE_SHAPES` 3종: `type-only`·`type-sequence`·`other`
- `capturableEventName`: 미매핑 이벤트 **이름만** 제한 캡처. 소문자 세그먼트 24자 이하, 점 최대 4개,
  전체 48자. **본문·해시·부분값·이미 라벨이 있는 이름은 절대 담지 않는다.**

주석에 실패 기록이 남아 있다: 점을 필수로 했더니 run-04에서 타입이 `identifier`로 분류되고
목록은 비어서, 이 캡처가 존재하는 이유인 그 이름을 놓쳤다. 이 프로토콜 어휘 자체가 점 없는 이름
(`error`·`ping`·`message_start`)을 쓰기 때문이다.

**Go**: `codex/events.go`에 이벤트 24개. 진단 어휘는 없고, 미지 이벤트는 `UNSUPPORTED_EVENT`로
요청을 실패시킨다. 캡처도 분류도 없다.

## 10. 옵션 정책과 종료

### 10.1 거부 옵션 — 여기서 "동등"은 후퇴다

| | 기준선 | Go |
|---|---|---|
| 거부 목록 | **30개** | **2개** |
| 근거 | gateway 경로를 벗어나거나 wrapper 계약을 깨는 것 | 세션 전체의 안전 장치를 제거하고 아래에서 되돌릴 수 없는 것 |

Go의 주석이 기준선의 이력을 근거로 든다: 목록이 30개로 자랐는데도 **`--name --model`을 혼동했다** —
어떤 옵션이 뒤따르는 값을 소비하는지 추적해야 했기 때문이다. Go가 고른 둘은 값을 소비하지 않으므로
그 추적이 필요 없고, "값이 옵션으로 오인되지 않는다"는 성질이 온전히 남는다.

그리고 기준선이 막는 `--mcp-config`·`--plugin-dir`·`--worktree`·`--permission-mode`·`--restricted`·
`--betas`는 **사용자 자신의 설정이고, 그게 동작하게 하는 것이 이 재설계의 목적**이다. `--bare`도
기준선은 막지만 Go는 통과시키고 CAP10이 실제로 연결됨을 측정했다.

**그러므로 이 항목은 기준선에 맞추지 않는다.** 단 조건이 하나 붙는다:

> 5절의 launcher 층을 이식하면 `--settings`·`--setting-sources`·`--agents`·`--system-prompt`
> **네 개는 반드시 막아야 한다.** wrapper가 직접 써서 주입하므로 사용자 값과 충돌한다.
> 지금은 주입하지 않으므로 통과시켜도 되지만, 주입을 켜는 순간 이 넷은 정책이 아니라 **필연**이다.

### 10.2 종료 코드 — 서로 다르다

| | 기준선 | Go |
|---|---|---|
| 종료 코드 | `SUCCESS ? 0 : 1` | **자식의 exit code 그대로** |

Go는 `cmd/clauduct-go/main.go`가 `result.NativeExitCode`를 반환하고 ARG08이 exit 7 왕복을 고정했다.
launcher로서는 Go 쪽이 맞다고 본다 — 스크립트가 자식 코드에 의존한다. 그래도 차이는 차이이므로
**결정 사항으로 남긴다.**

### 10.3 종료 출력 — Go에는 없다

```
Clauduct 종료: <category>                       # SUCCESS|CLIENT_FAILED|CLIENT_START_FAILED|REQUEST_BUDGET|USER_CANCELLED
CLAUDUCT_REQUEST_STATUS <json>                  # 6.6절
CLAUDUCT_REQUEST_STATUS_FILE <경로|none>         # launcher 자신의 무시 디렉터리
```

세션 중 모은 notice는 여기서 stderr로 나간다 — **native가 터미널을 소유하므로 세션 중에 쓰면
프롬프트 입력 상자 안에 떨어지기 때문이다.** 상태 파일 기록 실패는 치명적이지 않고, 그때는
stdout의 그 한 줄이 유일한 사본이라고 알린다.

**Go**: 종료 출력 없음. `CleanupErr`만 stderr로 나간다.

## 11. WP07 — 원장에서 다시 정의한 작업 목록

조사 완료. "C 등급 19건" 대신 이것이 작업 목록이다. 각 항목은 이 문서의 절을 근거로 갖는다.

### A. 사용자가 매일 닿는 것

| # | 작업 | 근거 | 라이브 필요 |
|---|---|---|---|
| A1 | ~~`image` 블록~~ **오프라인 완료 2026-09-16.** 실세션 검증은 남음 | 3.1 | 예 — 실제 이미지 왕복 |
| A2 | ~~hosted WebSearch~~ **오프라인 완료 2026-09-16.** 실세션 검증 남음. 돌연변이 15/15 | 6.1 | 예 |
| A3 | `GET /v1/models` **완료 2026-09-16**. `modelPicker`+`ENABLE_GATEWAY_MODEL_DISCOVERY`는 B1과 함께 | 2, 5.3 | 예 — 피커 확인 |
| A4 | ~~`tool_addition`/`tool_removal`, `redacted_thinking`~~ **오프라인 완료 2026-09-16.** 돌연변이 21/21 | 3.1, 3.4 | 예 |

### B. launcher 층 — 측정이 계획을 바꿨다

**2026-09-16: B1 완료. B2·B3는 필요 없어졌다.**

계획은 기준선처럼 `--model`·`--effort`·`--settings`를 argv로 주입하고, 그 결과로 네 옵션을 차단하는
것이었다(B2·B3). 그러려면 **어떤 native 옵션이 뒤따르는 값을 소비하는지 추적하는 파서**가 필요하다 —
기준선의 차단 목록이 30개로 자라고도 `--name --model`을 혼동한 그 추적이다.

실제 클라이언트로 재보니(fixture backend, 추론 비용 0) **전부 환경변수로 된다**:

| 잰 것 | 결과 |
|---|---|
| `ANTHROPIC_MODEL` | 클라이언트가 그 모델을 요청한다 |
| `CLAUDE_CODE_EFFORT_LEVEL` | effort가 그 값이 된다(`high` → `low`) |
| `CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY` | 요청이 2→3. **클라이언트가 `/v1/models`를 실제로 부른다** |
| `ANTHROPIC_DEFAULT_{HAIKU,SONNET,OPUS}_MODEL` | 클라이언트가 Claude 이름 대신 backend 모델을 직접 부른다 |
| **`--model` vs `ANTHROPIC_MODEL`** | **`--model`이 이긴다** |

마지막 줄이 결정적이다. 사용자의 `--model`이 이미 이기므로 **아무것도 파싱하지 않고도 사용자가 선택을
유지한다.** 그래서 argv 주입도, 파서도, 차단 목록도 필요 없다. ARG05(옵션 값 안의 모델명)도 그대로
성립한다 — 실제 클라이언트로 확인했다.

**세션 값은 명령이 아니라 기본값이다.** 사용자가 이미 설정한 이름이 이긴다. 기준선은 settings 블록을
환경에 무조건 덮어쓰고 그 대신 `--effort`를 제공하는데, 여기서는 `--effort`가 클라이언트 옵션이 아님이
측정으로 확인됐다(`--effort max`가 무시됐다). 사용자 환경이 이기게 하면 파서 없이 effort 선택이
돌아온다 — `CLAUDE_CODE_EFFORT_LEVEL=max`로 실제 확인했다.

예외 하나만 **요구사항**이다: `CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK`. 이 빌드는 non-streaming
요청을 거부하므로, fallback을 허용하면 느린 답이 아니라 **깨진 턴**이 된다.

시작 route는 기준선의 `DEFAULT_SELECTION`과 같은 **astra/low**다. 지금까지 Go는 아무것도 주지 않아
클라이언트 기본값(opus→sol/high)으로 돌았다.

| # | 작업 | 상태 |
|---|---|---|

| # | 작업 | 근거 |
|---|---|---|
| B1 | 세션 환경 14개 키 | **완료.** 돌연변이 9/9 |
| B2 | `--settings` 주입 (hook·picker만) | **완료 2026-09-16.** `--model`/`--effort` 소유는 여전히 불필요 |
| B3 | `--settings`·`--setting-sources` 차단 | **완료.** 두 개뿐 — `--agents`/`--system-prompt`는 주입하지 않으므로 그대로 통과 |
| B4 | `settings.hooks` + `clauduct-hook` | **완료 2026-09-16.** 실세션 subagent 검증은 남음 |
| B5 | `--agents` 정의 생성 | 남음. `--settings`/`--agents` 주입이 필요한 유일한 항목 |

남은 B4·B5는 argv 주입을 다시 요구하므로, 그때 B3의 차단이 필연이 된다. 그 시점에 다시 판단한다.

### B.1 측정이 또 두 가지를 바꿨다 (2026-09-16)

**`--settings` 두 개는 합쳐지지 않는다. 마지막 것만 적용된다.** 첫 번째의 `env`가 통째로 사라졌다.
그래서 이 빌드가 하나를 주입하는 순간, 사용자가 준 `--settings`가 **우리 것을 조용히 대체하고 hook이
설치되지 않는다.** B3의 차단은 정책이 아니라 주입의 **결과**다. `--setting-sources`도 어떤 소스가
로드되는지를 정하므로 함께 막는다.

`--agents`·`--system-prompt`는 **막지 않는다.** 주입하지 않으므로 충돌이 없다. B5(에이전트 정의)를
하면 그때 `--agents`가 필연이 된다.

**그리고 B1이 만든 회귀를 찾았다.** `ANTHROPIC_MODEL`이 Codex 모델을 가리키자 클라이언트가
stderr에 이렇게 말했다:

```
"gpt-6-astra" isn't described by this version's model catalog ...
Until then auto-compact keeps this session within 200k tokens (the context window it assumes)
```

**모든 세션이 실제의 절반에서 압축된다.** 기준선 `CONTEXT_POLICY`의 값(400000 / 320000 / 20000
reserve)을 `CLAUDE_CODE_MAX_CONTEXT_TOKENS`·`CLAUDE_CODE_AUTO_COMPACT_WINDOW`·
`CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`로 알려주자 경고가 사라졌다.

남은 `[claude-code:unrecognized_model]` 한 줄은 `modelPicker`의 `behavesAs`로 없앨 수 있다 —
**실측했다.** 다만 그것이 **effort까지 가져간다**(low로 고정한 세션이 medium으로 돌아왔다). 창 문제는
이미 해결됐으므로 쓰지 않는다. 사용자가 고르지 않은 비용 변화를 한 줄의 로그와 바꾸지 않는다.

### B.2 ARG01의 결론이 바뀌었다

`launch.Build`가 이제 `--settings <JSON>`을 **앞에** 붙인다. "자식의 argv가 전달 인자와 정확히
같다"는 더 이상 참이 아니다.

살아남는 성질을 다시 썼다: **사용자가 친 인자는 바뀌지 않고, 순서대로, 끊기지 않고, 맨 뒤에 도착한다.**
launcher가 더하는 것은 고정 접두사이지 인자 사이에 끼어드는 것이 아니다. 테스트가 그것을 확인한다.

### C. agent/workflow 선택 층

| # | 작업 | 근거 |
|---|---|---|
| C1 | `POST /clauduct/agents` **완료 2026-09-16** (SubagentStart/Stop). 완료 연결 바인딩 4종은 보류 | 2, 4 |
| C2 | 역할별 모델(`ROLE_MODELS`) **완료 2026-09-16**. 메타데이터 identity 검증은 보류 | 4, 3.3 |
| C3 | `x-claude-code-*` 모양 검사 **완료**. 바인딩 대조는 C1·C2와 함께 | 8, 4 |
| C4 | workflow-selection: 저널 검증·다이제스트·resume | 6.7 |
| C5 | `Route.Source = "role"` **완료 2026-09-16** | 3.3, CAP03 |

### C. agent/workflow 선택 층 — 범위를 좁혔다 (2026-09-16 결정)

사용자 결정: **역할별 라우팅까지만.** 기준선의 메타데이터 identity 검증(symlink 경계·재확인·실패 코드
12종)과 C4(workflow 저널 검증)는 보류한다.

결정의 근거가 된 사실 둘. **B1이 이미 티어별 라우팅을 가져왔다** — 서브에이전트는 이제
`ANTHROPIC_DEFAULT_*`를 따라 sonnet→terra, haiku→luna, opus→sol로 간다. C2의 *추가* 가치는 역할별
강제 재지정(`ROLE_MODELS`)과 모델을 고르는 에이전트 타입 노출이다. 그리고 **검증 실패가 서브에이전트
턴을 죽인다**(기준선의 `AGENT_SELECTION_UNVERIFIED_CALL`) — 완전한 메타데이터 검증 없이 그 동작을
가져오면 얻는 것 없이 실패 경로만 늘어난다.

그래서 이 빌드에서는 **선택 실패가 턴을 죽이지 않는다.** 등록을 못 찾거나 역할을 모르면 클라이언트가
요청한 모델로 간다 — 재지정을 못 했을 뿐이지 잘못된 것을 한 게 아니다.

**구현 2026-09-16.** `bridge.roleRoutes` 3개(`Explore`→luna/max, `Plan`→astra/low,
`general-purpose`→luna/max), `BuildRequest(request, override ...Route)`, gateway가
`X-Claude-Code-Agent-Id`로 등록을 찾아 override를 얹는다. 실패 3경로는 각각 카운터로 남는다
(`unregisteredAgents` / `unroutedRoles`) — "조용히 아무것도 안 했다"가 침묵이 아니라 숫자가 된다.

`begin(id)`가 요청 1건 동안 등록을 붙잡는다. **이게 있어야** "진행 중인 작업은 쓸지 않는다"가 의도가
아니라 사실이 된다 — 변이(`state.active++` 제거)는 lastUsed 갱신만으로 살아남았고, 그래서 테스트를
*요청이 idle 창보다 오래 사는* 경우로 고쳤다. 한 턴이 agentIdle보다 길어지는 서브에이전트가 바로
그 경우다.

변이 13건 전부 잡힘: 역할→모델/effort 오배정 2, `Source` 미기록 1, 미등록 역할의 zero-route 1,
`begin`/release/정지 판정 3, sweep·cap의 active 무시 2, 헤더 없는 요청을 카운트 1, override 미적용 1,
카운터 미증가 2.

### D. 진단과 관찰

| # | 작업 | 근거 |
|---|---|---|
| D1 | `GET /clauduct/status` + `diagnostics()` | 2, 6.6 |
| D2 | 종료 JSON `CLAUDUCT_REQUEST_STATUS` + 상태 파일 + notice 지연 출력 | 10.3, 6.6 |
| D3 | 요청별 timing 10 스탬프 + 최근 16개 링 + stage 7종 | 8 |
| D4 | `anthropic-beta` 형식 검사 + 판정/미지 베타 분류 (**이름은 거부하지 않는다**) | 6.2 |
| D5 | rate-limit **헤더** 관찰 (SSE 이벤트와 다른 출처) | 6.5 |
| D6 | 이벤트 진단 어휘 + `capturableEventName` 제한 캡처 | 9 |

### E. 프로토콜 경계

| # | 작업 | 근거 |
|---|---|---|
| E1 | ~~`anthropic-version` 검사~~ **완료 2026-09-16** | 8 |
| E2 | ~~`content-encoding` 검사~~ **완료 2026-09-16** | 8 |
| E3 | ~~`Frame.WriteTo` 16 KiB 청킹~~ **완료 2026-09-16.** `chunkedWriter`, 돌연변이 6/6 | 6.3 |
| E4 | compact 템플릿 식별 | 6.4 |
| E5 | 전체 요청 timeout 10분 (phase별 위에 추가) | 7.1 |

### F. 결정 완료 (2026-09-16)

| # | 항목 | 결정 |
|---|---|---|
| F1 | 재시도 소유권 | **응답 전 연결 실패만 재시도.** DNS·IO·idle timeout 3회(100→800ms). TLS·권한 거부 제외. 상태를 내보낸 뒤에는 재시도하지 않는다 — 측정된 곱셈 위험은 **클라이언트가 보는 상태**에만 성립하고, 이 분류는 클라이언트가 볼 일이 없다 |
| F2 | 종료 코드 | **자식 코드 전파 유지 + category 줄 추가.** 기준선의 0/1은 스크립트가 쓰는 정보를 버리고, Go에는 사람이 읽을 이름이 없다. 둘은 배타적이지 않다 |
| F3 | 거부 옵션 | **Go의 2개 유지.** 기준선 목록을 따라가면 그것이 가졌던 `--name --model` 혼동도 따라온다. 단 B2 착수 시 `--settings`·`--setting-sources`·`--agents`·`--system-prompt` 4개는 필연 |
| F4 | Go 1.27.2 게이트 | 실측 후 판단 |

### F(구)  — 원래 목록

| # | 항목 | 왜 결정인가 |
|---|---|---|
| F1 | 재시도를 누가 소유하는가 (기준선 5회 vs Go 0회) | 양쪽 근거가 모두 측정에 기반한다. 7.2 |
| F2 | 종료 코드 (자식 코드 전파 vs 0/1) | Go 쪽이 launcher로서 맞아 보이나 차이다. 10.2 |
| F3 | 거부 옵션 목록 (기준선 30 vs Go 2) | **기준선에 맞추면 후퇴다.** B3만 필연. 10.1 |
| F4 | Go 1.27.2 게이트 | 실측 후 판단으로 합의됨. 12절 |

### 순서 제안

B → C가 한 덩어리이므로 함께 간다. A1·A3·E3은 독립이고 사용자 체감이 커서 먼저 해도 된다.
D는 B·C가 만든 상태를 보고하는 층이라 뒤에 온다.

권장: **E3 → A1 → A3 → A4 → A2 → (F 결정) → B → C → D**

E3을 맨 앞에 두는 이유는 이미 측정된 결함을 닫기 때문이다(6.3). A2(검색)를 A 안에서 마지막에 두는
이유는 별도 엔드포인트·별도 실패 분류가 붙어 가장 크기 때문이다.

### 예산

라이브가 필요한 것은 A 전체와 C2(역할별 라우팅 실제 확인), D5(실제 rate-limit 헤더 관찰).
남은 73회로 **A는 충분하다**(항목당 2–4회 예상). B·C·D까지 포함한 산정은 A를 마친 뒤
실제 소모를 보고 다시 낸다. 지금 숫자를 부르면 근거 없는 숫자다.

## 12. Go 런타임 — 미해결 상류 결함

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
