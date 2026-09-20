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
| `thinking` + `summary_text` | 지원 (`:234`) | 지원 (봉투 안의 summary 파트) | 동등 — 3.4절 |
| `output_config.format` (구조화 출력) | `text.format`으로 **전달** (`:429`) | **전달 — 2026-09-17 수정** | 동등. 그 전까지는 검증만 하고 버렸다 — 3.5절 |
| `document` (PDF) | **없음** | **`input_file`로 전달 — 2026-09-17** | **기준선을 앞선다** — 3.6절 |

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

### 3.5 구조화 출력 — 검증하고 버리고 있었다 (2026-09-17)

`decodeOutputConfig`는 `format`의 모양, `json_schema` 타입, schema 객체, name 식별자를 전부
검사했다. 그리고 **아무 데도 싣지 않았다.** `anthropic.Request`에 담을 필드가 없었고
`BuildRequest`는 `text`를 만들지 않았다. 기준선은 `native-protocol.mjs:429`에서
`text: { format: { type, name, schema, strict: true } }`로 보낸다.

검증하고 버리는 것은 거부보다 나쁘다. 요청은 200이고, 클라이언트는 제약이 걸렸다고 믿고, 모델은
그 제약을 들은 적이 없다. 실패는 **스키마를 기대하고 파싱하는 쪽**에서 난다 — workflow의
agent 결과, 구조를 받기로 한 서브에이전트. 원인에서 가장 멀리 떨어진 자리다.

기준선과 같은 계약으로 연결했다. 이름 없는 스키마의 기본값도 기준선의 `structured_output`이다.
돌연변이 3건 중 하나가 처음에 살아남았다 — 기본 이름을 상수로 단언해 테스트가 자기 자신과
동의하고 있었다. 리터럴로 고정한 뒤 잡힌다.

**실백엔드에서는 아직 확인하지 않았다.** 계약의 근거는 기준선이 그 모양을 보내고 출하돼 있다는
것이고, 이 빌드가 그것을 실제로 보내는 것은 오프라인으로만 고정돼 있다.

### 3.6 PDF — 기준선에 없는 것을 먼저 갖게 됐다 (2026-09-17)

`document` 블록은 양쪽 다 미지원이었고, 이 빌드에서는 `UNSUPPORTED_CONTENT`로 **턴을 죽였다.**
드문 경로가 아니다 — 클라이언트가 Read로 PDF를 열면 tool_result 안에 document 블록을 담아 보낸다.

구현 전에 두 가지를 쟀다. **클라이언트가 보내는 모양**은 바이너리에서 읽었다(추측 아님):
`{type:"document",source:{type:"base64",media_type:"application/pdf",data}}`, 그리고 user 첨부와
tool_result 양쪽. **백엔드가 읽는지**는 직접 물었다 — `clauduct-dev probe file --send`로 생성한
1페이지 PDF를 `input_file` 데이터 URL로 보냈고, 모델이 **PDF 안에만 있던 토큰을 돌려줬다.**
지원 여부를 문서에서 추정하지 않았다.

`media_type`은 `application/pdf` 하나만 받는다. 참조 Codex 클라이언트는 `input_file`을 아예 보내지
않으므로(바이너리에 문자열 0건) 다른 타입에 대한 근거가 없고, 근거 없이 전달하면 사용자가 붙인
파일이 "아무것도 아닌 것에 대한 답"이 된다. `url`·`file` source도 거부한다 — 가져오거나 조회해야
하는 것이고 이 빌드는 둘 다 하지 않는다.

**probe가 처음에 거짓말을 했다.** 첫 두 번의 실행은 "accepted, but the reply does not carry the
token"이라고 보고했는데, 토큰을 `translate()`가 반환하는 exchange에서 찾고 있었고 그 버퍼는
EOF에서 `nil`로 비워진다. 즉 성공할 수 없는 검사였고, 그것을 **백엔드에 대한 사실로** 출력했다.
스트림을 지나가는 바이트에서 찾도록 바꾸고, 토큰이 읽기 경계에 걸쳐 도착하는 경우를 테스트로
고정했다. 답은 처음부터 "읽는다"였다.

돌연변이 3건 전부 잡힘 — 파일 이름, 미측정 media_type 통과, document를 text로 떨어뜨리기.

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

**실측 2026-09-17 (`probe headers --send`, 1회, 누적 35/100).** 응답 헤더 39개. 결과 다섯:

1. **여섯 필드가 전부 온다.** `x-codex-{primary,secondary}-{used-percent,window-minutes,reset-at}`.
   기준선이 남의 소스 트리에서 읽어온 이름이 실제로 맞다.
2. **`x-codex-secondary-reset-at`의 값이 비어 있다.** 그런데 기준선의 정규식은 빈 값을
   `numeric-format`으로 판정하고 관측 전체를 `invalid`로 만든다 — **실제 응답마다 매번 invalid가
   된다.** 빈 값은 망가진 게 아니라 없는 것이다. 그래서 여기서는 빈 값을 부재로 다룬다.
   실측 기준 결과는 `partial`(6개 중 5개)이다.
3. **두 번째 family가 실제로 있다**: `x-codex-bengalfox-*` 여섯 필드 전부. "다른 family" 개념은
   추측이 아니었다.
4. **`x-codex-active-limit`이 있다** — 어느 쪽이 실제로 걸려 있는지 말해준다. 기준선은 안 읽는다.
   두 family를 나란히 보고하고 독자에게 추측을 맡기는 것보다 이걸 읽는 쪽이 모호하지 않다.
5. **`set-cookie`가 온다.** 값을 통째로 찍지 않기로 한 결정이 추측이 아니라 필요였다.

기준선이 안 읽는 것: `-primary-over-secondary-limit-percent`, `-primary-reset-after-seconds`,
`x-codex-plan-type`, `x-codex-credits-*`, `x-codex-turn-state`.

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
| `CLAUDE_CODE_EFFORT_LEVEL` | effort가 그 값이 된다(`high` → `low`). **`--effort`까지 이긴다 — 아래 2026-09-18 정정** |
| `CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY` | 요청이 2→3. **클라이언트가 `/v1/models`를 실제로 부른다** |
| `ANTHROPIC_DEFAULT_{HAIKU,SONNET,OPUS}_MODEL` | 클라이언트가 Claude 이름 대신 backend 모델을 직접 부른다 |
| **`--model` vs `ANTHROPIC_MODEL`** | **`--model`이 이긴다** |

마지막 줄이 결정적이다. 사용자의 `--model`이 이미 이기므로 **아무것도 파싱하지 않고도 사용자가 선택을
유지한다.** 그래서 argv 주입도, 파서도, 차단 목록도 필요 없다. ARG05(옵션 값 안의 모델명)도 그대로
성립한다 — 실제 클라이언트로 확인했다.

**세션 값은 명령이 아니라 기본값이다.** 사용자가 이미 설정한 이름이 이긴다.

> **2026-09-18 정정 — 이 문단의 effort 부분은 틀렸다.**
>
> 원문은 *"`--effort`가 클라이언트 옵션이 아님이 측정으로 확인됐다(`--effort max`가 무시됐다)"*였다.
> 다시 재보니 `--effort`는 동작한다. 환경변수를 지우고 주면 `--effort medium` → `effort=medium`이
> 그대로 나간다.
>
> 2026-09-16에 무시되어 보인 이유는 **같은 빌드가 `CLAUDE_CODE_EFFORT_LEVEL`을 함께 세우고 있었고,
> 그 이름이 `--effort`를 이기기 때문**이다. 측정은 교란됐고, 결론은 "옵션이 아니다"가 아니라
> "환경변수가 옵션을 이긴다"였어야 했다.
>
> | 2026-09-18 실측 (fixture backend, 추론 0) | 결과 |
> |---|---|
> | 환경변수 없음, `--effort low` | `effort=low` |
> | 환경변수 없음, `--effort low` 뒤에 사용자의 `--effort high` | `effort=high` — 뒤의 것이 이긴다 |
> | `CLAUDE_CODE_EFFORT_LEVEL=high` + `--effort low` | **`high`** — 환경변수가 이긴다 |
> | `CLAUDE_CODE_EFFORT_LEVEL=low` + `--effort high` | **`low`** — 같은 방향 |
>
> 이것이 `--model`과 정반대다. `--model`은 `ANTHROPIC_MODEL`을 이기지만, `CLAUDE_CODE_EFFORT_LEVEL`은
> `--effort`를 이긴다. 명시적 플래그를 이기는 값은 **클라이언트의 `/model` 피커도 이긴다** — 이것이
> 2026-09-17 첫 실사용에서 사용자가 high를 고르고도 182건 전부 `low`로 나간 원인이다.
>
> 그래서 시작 effort는 `--effort`로 옮겼다(`launch.Overlay.Effort`). 기준선이 원래 그렇게 했고
> (`src/clauduct.mjs:213`), 기준선이 `CLAUDE_CODE_EFFORT_LEVEL`을 세우는 자리는
> `--verify-model-route` 하나뿐이다(`src/clauduct.mjs:192`) — 검증 실행이 경로에서 벗어나지 못하게
> **일부러 못 박는** 모드다. v1의 잠금장치를 v2가 기본값으로 만들었던 것이다.
>
> 이 문단의 나머지 — 세션 값은 명령이 아니라 기본값이고 사용자 환경이 이긴다 — 는 그대로 맞다.
> 사용자가 `CLAUDE_CODE_EFFORT_LEVEL`을 직접 세우면 주입된 `--effort`를 이기므로, 그 성질은
> 옮긴 뒤에도 공짜로 보존된다.

기준선은 settings 블록을 환경에 무조건 덮어쓰고 그 대신 `--effort`를 제공한다. 사용자 환경이 이기게
하면 파서 없이 effort 선택이 돌아온다 — `CLAUDE_CODE_EFFORT_LEVEL=max`로 실제 확인했다.

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
| B5 | `--agents` 정의 생성 | **완료 2026-09-16.** 14종 + 게이트웨이가 effort를 맡는다 |

### B.5 위임 메뉴 — 측정이 설계를 절반 바꿨다 (2026-09-16)

`clauduct-<model>-<effort>` 13종 + `clauduct-inherit`. 전부 `bridge.Models`/`bridge.Efforts`에서
파생되므로 모델이 늘면 메뉴도 늘고 아무도 이 파일을 안 고친다. max는 max가 기본인 모델의 것이다
(`(정의.effort == max) == (모델.effort == max)`) — 양방향으로 테스트한다.

> **2026-09-18 실측: 14개 중 `clauduct-inherit` 하나만 비결정적이다.**
>
> Agent 도구에는 자체 `model` 인자가 있고, **호출한 모델이 묻지도 않고 채운다** — 실세션 위임 4회
> 중 4회, 사용자가 요청한 적 없다. 이름 붙은 13종은 gateway의 role override가 그 인자를 이기므로
> 영향이 없다(`clauduct-luna-max` + 호출자 `model=fable` → luna/max로 갔다). `inherit`은 설계상
> override가 없어서 — 상속이란 고르지 않는다는 뜻이므로 — 호출자 인자가 마지막 말이 된다.
>
> | 호출 | 자식이 요청한 것 | 실제 경로 |
> |---|---|---|
> | `inherit`, `model` 없음 | 부모와 같음 | **부모 route** ✓ |
> | `inherit`, `model=opus` | `gpt-5.6-sol` | sol ✗ |
> | `luna-max`, `model=fable` | `claude-fable-5-1` | **luna/max (role)** ✓ |
>
> 그래서 이 항목의 **description이 유일한 기구**다: 호출자에게 `model` 인자를 넘기지 말라고
> 명시한다. 나머지 13종은 description이 무엇을 말하든 경로가 바뀌지 않으므로 그럴 필요가 없다.
> description을 테스트로 검사하는 이유가 이것이다(`TestTheInheritEntrySaysWhatItNeedsFromTheCaller`).
>
> 고치지 않은 이유: gateway가 부모 모델을 기억해 강제하는 길은 반례 셋을 휴리스틱으로 덮어야 한다 —
> 헤더 없는 요청이 대화만은 아니고(배경 작업도 헤더가 없다), 중첩 자식은 직계가 아닌 최상위 부모를
> 물려받고, 병렬 요청에서 "마지막"이 타이밍에 좌우된다.

**잰 것 넷:**

| 질문 | 답 |
|---|---|
| settings 블록의 `agents` 키 | **안 먹는다.** 타입이 정의되지 않고 호출이 버려진다 |
| `--agents` | **먹는다.** `agent:custom:clauduct-terra-high` → terra |
| `--agents` 두 개 | **합쳐지지 않는다.** 마지막 것만 — `--settings`와 같다 |
| 주입 vs 사용자의 `.claude/agents/*.md` | **합쳐진다.** 사용자의 reviewer가 자기 파일이 정한 모델로 그대로 돈다 |

마지막 줄이 B5를 감당 가능하게 만든다. 그래서 **`--agents`는 막지 않는다.** 우리 것을 앞에 두면
사용자가 직접 준 `--agents`가 이긴다 — 세션 환경과 같은 규칙이다. `--settings`와 다른 이유는 잃는
것이 다르기 때문이다: settings를 잃으면 hook이 조용히 설치되지 않지만, 메뉴를 잃으면 사용자가 방금
원하지 않는다고 말한 메뉴를 잃을 뿐이고 클라이언트 자신의 subagent는 여전히 게이트웨이에서
역할로 라우팅된다.

**그리고 절반이 안 됐다. agent 정의로는 effort를 정할 수 없다.** `effort`, `effortLevel`,
`reasoningEffort`, `reasoning_effort` 넷 다 무시되고(모델만 옮겨간다), `{"level":"high"}` 객체는
정의 자체를 무효로 만든다. `model: "gpt-5.6-terra:high"` 같은 접미사도 안 된다. 자식은 세션의
effort로 돈다.

그래서 **effort는 게이트웨이가 맡는다.** 이름이 이미 그것을 싣고(`clauduct-terra-high`), hook이 이미
그 이름을 보고, 요청이 이미 그 id를 들고 온다. `bridge.RoleRoute`가 `clauduct-` 접두사를 파싱한다.
정의는 model을 그대로 유지한다 — 클라이언트 자신의 회계가 맞아야 하므로. 실측: `terra/high`로 나간다.

**이 연결의 대가**: 메뉴의 effort 절반은 hook이 설치돼야 동작한다. hook이 없으면 모델만 옮겨가고
effort는 세션 값으로 남는다. 테스트가 hook을 실제로 빌드해서 그 경로로만 확인한다.

돌연변이 11건 전부 잡힘. 그중 하나(`max`를 모든 모델에 제공)는 **처음에 살아남았다** — `want`를
`agentEfforts`로 만들어서 테스트가 자기 자신과 동의했기 때문이다. 규칙을 테스트 안에 따로 쓰고 나서야
잡혔다.

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
| C4 | workflow-selection: 저널 검증·다이제스트·resume | **조사 완료 2026-09-17 — 안 한다.** 아래 참조 |
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

**구현 2026-09-16.** `bridge.roleRoutes` 3개(`Explore`→luna/max, `Plan`→astra/**medium**,
`general-purpose`→luna/max), `BuildRequest(request, override ...Route)`, gateway가
`X-Claude-Code-Agent-Id`로 등록을 찾아 override를 얹는다. 실패 3경로는 각각 카운터로 남는다
(`unregisteredAgents` / `unroutedRoles`) — "조용히 아무것도 안 했다"가 침묵이 아니라 숫자가 된다.

> **2026-09-18: `Plan`을 low → medium으로 올렸다 (사용자 결정).** 기준선은 astra/low이고
> (`src/models.mjs:20-21`) 여기도 그랬다. sonnet에 이은 **두 번째 기록된 갈라짐**이다. 근거는
> 계획이 유일하게 그 위에 쌓이는 모든 것이 실수를 대신 갚는 작업이라는 것 — 아낄 자리가 아니었다.
> medium이 astra의 카탈로그 기본과 같은 것은 우연이고 근거가 아니다. **항목은 명시로 남는다**:
> override는 effort뿐 아니라 모델도 못박으므로, 카탈로그로 떨어뜨리면 Plan이 대화가 요청한 아무
> 모델에서나 돌게 된다.

> **2026-09-18: 응답이 어느 모델 이름을 싣는지 뒤집었다 (기록된 결정 번복).**
>
> v2는 `message_start.model`에 **클라이언트가 요청한** 모델을 실었고, `route_test.go`에 그 불변식과
> 이유가 적혀 있었다 — *"the client is told it talked to something it never requested"*.
> **기준선은 반대다.** v1의 `frameStart`는 `prepared.selected.model`을 쓰고
> (`src/native-protocol.mjs:509-511`), 역할 route가 있으면 `doc.model`은 읽히지도 않는다(`:258`).
> v1 테스트가 그걸 못 박는다 — 클라이언트가 terra를 보내도 응답에 `ROLE_MODELS.Plan.model`이
> 들어있는지 단언한다(`src/test-native.mjs:505-511`).
>
> v1 쪽으로 맞췄다. 이유의 honesty가 거꾸로였다: 역할 라우팅된 서브에이전트는 요청한 것과 **대화하지
> 않는다.** 요청 이름을 싣는 쪽이 거짓 진술이고, 그것도 모델 교체를 사용자에게서 **감추는** 방향이다 —
> ARCHITECTURE.md 6장이 하지 않겠다고 적은 바로 그것이다.
>
> **클라이언트가 이 값으로 무엇을 하는지는 실측했다 — 눈에 보이는 것은 없다.**
>
> | 무엇 | 언제 정해지나 | 근거 |
> |---|---|---|
> | TUI 에이전트 헤더 | **spawn 시점**, 응답 존재 이전 | 에이전트 실행 중 헤더가 이미 그려져 있음 |
> | 모델별 토큰·비용 장부 | **클라이언트가 요청한 모델** 기준 | 변경 후 서브에이전트 10건이 전부 `gpt-6-astra`에서 돌았는데 장부는 `gpt-5.6-terra`에 계속 쌓였고 astra는 나타나지 않았다 |
>
> 교차 증거: 같은 머신의 `code-map-memo` 프로젝트 장부에는 astra가 **있다.** 거기선 클라이언트가
> astra를 직접 요청했기 때문이다. 즉 장부는 요청을 따라간다.
>
> **그러므로 이 변경은 표시나 장부의 수정이 아니다.** 게이트웨이가 돌려보내는 바이트는 셋 모두의
> 하류에 있어서 닿지 못한다. 필드가 사실이어야 한다는 것, 그리고 기준선이 그렇게 한다는 것이 근거의
> 전부다.
>
> 번복 과정에서 두 가지를 잘못 적었다가 고쳤다. `route_test.go`의 기존 결정을 못 보고
> "기록이 없다"고 단정했고(grep 출력을 자른 채 전수로 취급), 클라이언트 장부가 이 값에서 온다고
> 적었다(반증됨). 그리고 그 테스트는 변경 후에도 통과했다 — `runFor`가 게이트웨이 배선을 우회해
> **실패할 수 없는 테스트**였다. 지금은 게이트웨이가 넘기는 값과 빈 값을 모두 통과시킨다.

> **미해결(세션 37): WebSearch는 역할 라우팅을 건너뛴다.** `messages.go`의 hosted search 분기가
> 역할 오버라이드 블록보다 **앞에서 반환한다.** 그래서 역할 라우팅된 서브에이전트의 웹검색은
> `SelectRoute(request.Model, ...)`로 만들어져 역할의 모델이 아니라 **요청 모델**로 나가고,
> 그 경로의 `message_start`도 같은 값을 싣는다. 근거 주석이 없어 의도인지 누락인지 확인되지 않았다.

`begin(id)`가 요청 1건 동안 등록을 붙잡는다. **이게 있어야** "진행 중인 작업은 쓸지 않는다"가 의도가
아니라 사실이 된다 — 변이(`state.active++` 제거)는 lastUsed 갱신만으로 살아남았고, 그래서 테스트를
*요청이 idle 창보다 오래 사는* 경우로 고쳤다. 한 턴이 agentIdle보다 길어지는 서브에이전트가 바로
그 경우다.

**실세션 확인 2026-09-16 (추론 0회).** 스크립트 백엔드 + 실제 클라이언트로 서브에이전트를 실제로
띄워 끝에서 끝까지 봤다. A/B가 갈린다:

| | 서브에이전트가 실제로 간 곳 | `unregistered` |
|---|---|---|
| hook 설치됨 | `gpt-5.6-luna/max` | 0 |
| hook 없음 | `gpt-5.6-sol/low` | 1 |

sol→luna는 티어 매핑이 아니다. 역할이다. 그리고 이 A/B가 없으면 "luna로 갔다"는 "서브에이전트는
원래 luna로 간다"로도 읽힌다 — 그래서 두 번째 절반이 첫 번째를 의미 있게 만든다.

이 과정에서 나온 사실 넷. 전부 오프라인 합의만으로는 볼 수 없었던 것들이다.

1. **툴 이름은 `Task`가 아니라 `Agent`다.** 기준선 주석은 아직 Task라고 쓴다. `Task`로 스크립트한
   호출은 툴 결과조차 없이 조용히 버려지고 클라이언트가 같은 대화를 다시 보낸다 — 기준선 이름으로 쓴
   테스트는 *통과하면서 아무것도 측정하지 않는다.*
2. **`X-Claude-Code-Agent-Id`는 실제로 온다.** 서브에이전트 요청에만 붙는다(측정된 헤더 덤프).
3. **`X-Claude-Code-Agent-Type`은 오지 않는다.** 클라이언트 바이너리에 이름은 있지만 요청에는 없다.
   그래서 hook이 없으면 역할을 알 길이 없다 — hook은 편의가 아니라 유일한 경로다.
4. **`SubagentStart`/`SubagentStop`은 존재하고 필요한 필드를 전부 싣는다** — `agent_id`(17자리 hex),
   `agent_type`, `session_id`, `transcript_path`, Stop에는 `agent_transcript_path`까지.

변이 13건 전부 잡힘: 역할→모델/effort 오배정 2, `Source` 미기록 1, 미등록 역할의 zero-route 1,
`begin`/release/정지 판정 3, sweep·cap의 active 무시 2, 헤더 없는 요청을 카운트 1, override 미적용 1,
카운터 미증가 2.

### D. 진단과 관찰

| # | 작업 | 근거 |
|---|---|---|
| D1 | `GET /clauduct/status` + `Diagnose()` | **완료 2026-09-17** |
| D2 | 종료 요약 + 상태 파일 + 조건부 JSON | **완료 2026-09-17.** stdout에는 안 쓴다 |
| D3 | 요청 링 16개 + stage 5종 + 스탬프 3개 | **완료 2026-09-17.** 아래 참조 |
| D4 | `anthropic-beta` 분류 4종 (**이름도 형식도 거부하지 않는다**) | **완료 2026-09-17** |
| D5 | rate-limit **헤더** 관찰 | **완료 2026-09-17.** 실측이 검증기를 바꿨다 |
| D6 | 미지 이벤트 이름 제한 캡처 + 형식 3종 | **완료 2026-09-17** |

### D.2 종료 출력 — stdout에는 절대 안 쓴다 (2026-09-17 결정)

기준선은 `CLAUDUCT_REQUEST_STATUS <json>`을 **stdout**에 찍는다. 여기서는 **안 찍는다.**

`claude -p "..."`는 답을 stdout에 쓰고 그게 그 플래그의 존재 이유다. 거기에 JSON 한 줄을 붙이면
`> out.txt`가 깨지고 `| jq`가 깨진다 — 그걸 못 견디는 바로 그 용법이. 전부 stderr와 파일로 간다.
main.go가 `os.Stderr`를 넘기는지 **소스를 파싱해서** 고정한다. 나중에 stdout으로 바꿔도 위 테스트는
전부 통과하기 때문에, 바뀌면 안 되는 것은 쓰이는 곳이 아니라 **쓰는 곳**에서 확인한다.

**그리고 할 말이 없으면 JSON을 안 찍는다.** 깨끗하게 끝난 세션은 보고할 게 없고, 매번 나오는 JSON
한 줄은 읽는 사람이 건너뛰는 법을 배우는 노이즈다 — 그게 정작 중요한 날 그 줄이 안 읽히는 경로다.
파일은 **항상** 쓰므로 전체 계정은 언제나 있고, `CLAUDUCT_STATUS=1`이면 무조건 찍는다.
찍을 조건: 거부·미지 이벤트·라우팅 실패·판정/미지 베타·형식 불량·`SUCCESS`가 아닌 종료 중 하나라도
있을 때, 또는 **파일 쓰기가 실패했을 때**(그때는 찍힌 줄이 유일한 사본이다).

상태 파일은 **임시 디렉터리**에 쓴다. 작업 디렉터리에 쓰면 남의 저장소에 이 빌드의 쓰레기를
남기고, `~/.claude`에 쓰면 사용자 것을 건드린다. 프로세스당 한 파일이고, 끝난 세션 파일은 OS가
임시 디렉터리를 비울 때까지 쌓인다 — 우리 쪽에서 패턴으로 지우는 것보다 나은 거래다.

계정에는 게이트웨이가 볼 수 없는 절반도 담는다: 시작 route, context 정책(window·autoCompactWindow·
compactPercent), non-streaming fallback 차단 여부, 위임 메뉴 항목 수. "왜 여기서 압축했나"에
답할 다른 곳이 없다.

종료 분류는 **읽는 사람에게 유용한 순서**다. 취소 > 예산 > 클라이언트 실패 > 성공. 취소된 세션은
보통 non-zero로도 끝나는데 "네가 멈췄다"가 더 나은 답이고, 예산이 떨어진 세션은 보통 클라이언트
실패도 같이 나는데 예산이 그 **원인**이다.

변이 10건 전부 잡힘.

### D.6 실패의 원인이 이름일 때 (2026-09-17)

미지 이벤트는 **여전히 거부한다.** 읽지 못한 이벤트가 결과를 싣고 있을 수 있고, 지나치면 아무도 안
읽은 바로 그만큼 짧은 답이 된다. 잘못된 건 거부가 아니라 **거부가 `UNSUPPORTED_EVENT`만 말했다는
것**이다. 백엔드가 새 이벤트 이름을 내보내면 세션의 모든 턴이 깨지는데 고치는 건 상수 하나이고,
그 상수를 아무도 지목할 수 없다.

**이미 한 번 일어났다.** 도구 정의를 실은 요청이 호출이 돌아오기도 전에 거부됐다 —
`response.function_call_arguments.*`가 목록에 없어서. 그걸 찾는 데 실세션이 들었다. 이름이 계정에
있었으면 출력 한 줄이면 됐다.

**형식은 6종이 아니라 3종이다.** `identifier`·`oversized`·`other`. 기준선의 `missing`·`non-string`·
`empty` 셋은 **여기서는 발생할 수 없다** — SSE 파서가 쓸 수 없는 type을 가진 프레임을 translator에
닿기 전에 자기 오류(`INVALID_SSE`)로 거부한다. 일어날 수 없는 범주를 이름만 가져오면 계정에 0이
영원히 박힌다. D3에서 stage에 대해 내린 판단과 같다.

**`KEEPALIVE_SHAPES` 3종은 안 가져온다.** keepalive 페이로드의 모양을 분류하는 것이고, 깨진 세션을
진단하는 데 주는 것이 없다.

캡처 규칙은 기준선 그대로다: 소문자 세그먼트 24자 이하, 점 최대 4개, 전체 48자. **점은 필수가
아니다** — 기준선 주석에 남은 실패 기록이 그것이다. 점을 요구했더니 이 프로토콜이 실제로 쓰는
점 없는 이름(`error`·`ping`·`message_start`)을 놓쳤다. 불변식 하나를 테스트가 고정한다:
**이름이 기록되는 것과 형식이 `identifier`인 것은 정확히 같다.**

변이 9건 전부 잡힘.

### D.5 빈 값은 망가진 게 아니라 없는 것이다 (2026-09-17)

기준선을 그대로 가져왔다면 **실제 응답마다 매번 `invalid`**이 나왔다. `x-codex-secondary-reset-at`이
빈 값으로 오고, 기준선 정규식은 그것을 `numeric-format`으로 판정해 관측 전체를 무효로 만든다.
매번 틀리는 진단은 아무도 안 읽는다. 여기서 빈 값은 부재이고, 실측 결과는 `partial`(6개 중 5개)다.

기준선에 없는 것 둘을 더 읽는다. **`x-codex-active-limit`** — 두 family를 나란히 놓고 독자에게
어느 쪽이 자기한테 걸리는지 추측하게 하는 것보다 낫다. 그리고 **다른 family는 이름만** 센다
(`x-codex-bengalfox`). 기준선은 family별 전체 관측을 8개까지 하는데, 실측된 것은 하나뿐이고
"내가 못 보는 다른 한도가 있나"에 답하는 데는 이름이면 충분하다.

테스트 fixture는 **실측 헤더 22개를 그대로** 쓴다 — 값을 아무도 고르지 않았다는 게 요점이다.
`set-cookie`·`x-codex-plan-type`·`x-codex-credits-*`·요청 id가 계정에 안 들어가는지 함께 고정한다.

**관찰이지 예산이 아니다.** 지출을 승인하지도, 남은 토큰을 추정하지도, 요청 예산을 바꾸지도 않는다.

변이 11건 전부 잡힘. 그중 하나는 **패턴이 안 맞아 실행되지 않았는데 SURVIVED로 보고됐다** —
미실행은 통과가 아니다. 다시 돌려서 잡았다. 또 하나는 진짜로 살아남았다: 헤더가 아예 없는 경우만
테스트하고 **헤더는 있는데 한도 얘기가 없는 경우**를 안 봤다. 후자가 실제 백엔드가 나쁜 날에 내는
모양이다.

### D.4 `INVALID_BETA_HEADER`는 가져오지 않는다 (2026-09-17 결정)

기준선은 **이름은 절대 거부하지 않되 형식이 깨진 헤더는 거부한다**(빈 항목·중복). 이 빌드는
**형식도 거부하지 않는다.** 같은 논리를 그 논리의 전제에 적용한 결과다.

근거는 코드에 있다. 이 헤더가 닿는 판단은 `negotiated` 하나뿐이고, 그것은 쉼표로 자른 뒤 **정확히
일치**하는지만 본다. 그러니 빈 항목도 중복도 *무엇이 켜지는지를 바꿀 수 없다.* 거부하면 아무 효과도
없는 클라이언트 쪽 이상값이 죽은 턴이 된다 — 그리고 이 헤더는 클라이언트가 만든다. 사용자 잘못이
아닌 것으로 사용자의 턴을 죽이는 셈이다. 대신 센다. 읽는 사람에게 같은 것을 말해주고 비용이 없다.

분류는 4종이다. `judged`(27개 → 고정 라벨), `serverDependent`(7개 → 이름), `unknown`(모양 검사를
통과한 이름, 8개 한도), `malformed`(횟수). 기준선은 `serverDependent`를 어느 맵에도 넣지 않아
`unknown`으로 떨어지는데, **아무도 안 본 이름과 보고 나서 무력하다고 판단한 이름은 다른 답이다.**

모양을 통과 못 한 텍스트는 **아예 기록하지 않는다.** 헤더는 클라이언트의 텍스트이고 이 계정은
파일로 남아 세션보다 오래 산다. 실측 8개 이름이 전부 `nativeBetas`에 있는지도 테스트가 고정한다 —
목록이 클라이언트가 실제로 보내는 것과 어긋나면 매 요청마다 `unknown`이 뜨고, 매번 우는 진단은
아무도 안 읽게 된다.

변이 9건 전부 잡힘. 그중 하나는 **동등 변이**였다: 빈 항목 검사가 `default` 분기와 같은 말을 두 번
하고 있었다. 지우고 나서야 변이 공간이 정직해졌다.

### D.1/D3 세션이 스스로를 말할 수 있게 됐다 (2026-09-17)

`GET /clauduct/status`가 `Diagnose()`를 낸다. 카운터 넷으로는 "뭔가 거부됐다"까지만 말할 수 있었고
어느 요청이, 어느 단계에서, 어느 모델로 갔는지는 말할 수 없었다.

**stage는 7종이 아니라 5종이다.** `request`·`selection`·`prepare`·`upstream`·`delivery`.
기준선의 `review`·`output-validation`은 이 빌드에서 별도 단계가 아니다 — 없는 단계를 이름만
가져오면 모든 기록이 지나가지 않은 곳을 지나갔다고 주장하게 된다.

**스탬프는 10개가 아니라 3개다.** `startedMs`·`firstByteMs`·`endedMs`. 세 번째 질문까지가 실제로
답을 요구하는 전부다 — 첫 바이트가 **느린 백엔드와 느린 브리지를 가르는 유일한 경계**이고, 그
앞은 이 빌드의 것, 그 뒤는 스트림의 것이다. 거부 응답 본문은 첫 바이트로 세지 않는다(변이로 확인).

**기록은 검사보다 먼저 열린다.** 기준선이 의도적으로 그렇게 배치했고 이유가 살아남았다 — 거부된
버전·경계·인코딩이 *기록되지 않은 400*이 아니라 진단된 실패가 되도록. 변이로 확인했다.

**상태를 읽는 것은 트래픽이 아니다.** 16번 읽으면 세션이 한 일의 기록이 전부 지워진다 — 읽는 사람이
보러 온 바로 그것이. `/clauduct/status`만 링에서 제외한다.

**요청이 실은 것은 아무것도 안 담는다.** 프롬프트·시스템·응답·세션 id 넷을 넣고 상태 본문에 없는지
확인한다. 이 계정은 파일로 쓰이고 세션 끝에 출력되므로, 담는 것은 세션보다 오래 산다.

변이 9건 전부 잡힘.

### E. 프로토콜 경계

| # | 작업 | 근거 |
|---|---|---|
| E1 | ~~`anthropic-version` 검사~~ **완료 2026-09-16** | 8 |
| E2 | ~~`content-encoding` 검사~~ **완료 2026-09-16** | 8 |
| E3 | ~~`Frame.WriteTo` 16 KiB 청킹~~ **완료 2026-09-16.** `chunkedWriter`, 돌연변이 6/6 | 6.3 |
| E4 | compact 템플릿 식별 + **effort medium 상한** | **완료 2026-09-17** |
| E5 | 전체 요청 timeout 10분 | **완료 2026-09-17** |

### E.4/E5 (2026-09-17)

**E4는 진단이 아니라 돈이다.** 기준선의 `compact-policy`가 하는 일은 분류가 아니라
`purpose === 'compact-template'`일 때 **effort를 medium으로 낮추는 것**이다. 압축 요청은
아무도 시키지 않고, 세션이 보내는 **가장 큰 입력**을 싣고, max로 고정된 세션에서는 자동으로
일어나는 가장 비싼 단일 요청이다. 전사(轉寫) 요약은 max가 존재하는 이유가 아니다.

규칙은 기준선 그대로다. 마지막 non-system 턴이 `user`여야 하고, 텍스트 블록만 보고,
`<system-reminder>`로만 된 블록은 빼고, 공백을 접은 뒤 접두/접미를 **둘 다** 대조하고,
사이에 내용이 있어야 한다. 모델은 안 옮기고 effort만 내린다. `Source`가 `compact`가 된다 —
세션이 고른 적 없는 effort를 보는 독자에게 설명이 필요하다.

**E5.** phase별 deadline은 더 나은 도구이고 그대로 둔다. 그런데 한 경우가 열려 있었다:
**계속 보내는 백엔드**는 어떤 phase deadline에도 안 걸리고 끝나지 않는다. 끝나지 않는 요청은
goroutine·연결·사용자 구독을 원하는 만큼 붙잡는다. 10분은 기준선 값이다.

구현에서 걸린 곳: deadline이 **본문까지 덮어야 하므로 `Execute` 반환보다 오래 살아야 한다.**
`defer cancel()`로 쓰면 모든 스트림이 첫 바이트에서 잘린다. body를 닫을 때 놓는다.

변이 19건 중 **4건이 처음에 살아남았다.** 전부 진짜 빈틈이었다:
- 접두만 있는 경우를 길이 검사가 대신 걸러서, 접미 검사가 테스트되지 않았다
- **어시스턴트가 템플릿을 말한 경우**를 안 봤다 — 모델 출력이 라우팅 신호가 되면 안 된다
- 텍스트 아닌 블록의 `Text` 필드를 직접 만들어 보지 않아, 가드가 테스트 밖에 있었다
- 전체 deadline을 **호출자 deadline으로만** 재고 있었다. 호출자가 아무것도 안 걸면 무엇이
  막는지 아무도 안 보고 있었다

### G5 C 범위 — 재설계의 정당화를 처음으로 측정했다 (2026-09-17)

`--mcp-config` · `--resume` · `--permission-mode` · `--worktree` · `--plugin-dir`.
기준선이 전부 막던 것이고, 그걸 여는 게 이 재설계의 이유였다. 그런데 증거는 **자식 argv에
도착한다**는 것 하나였다. 도달은 동작이 아니다. 전부 기능으로 닫았다 — 추론 0회.

가장 값진 것은 MCP다. 기준선은 자식 환경에서 `TOKEN`·`SECRET`을 포함한 이름을 **전부** 지웠고
그래서 MCP 서버가 동작하지 않았다. stub 서버를 띄워 `MCP_STUB_SECRET_TOKEN`을 물었다:
이 빌드는 `PRESENT:...`, `denied()`에 기준선 규칙을 넣으면 **`ABSENT`**.

다섯 개 전부 **옵션을 빼면 실패한다**는 것까지 확인했다. 빼도 통과하는 테스트는 아무것도
측정하지 않는다. 상세는 VALIDATION.md 1.10절.

### C4 조사 — 이 빌드에는 방어할 대상이 없다 (2026-09-17)

사용자 결정: **먼저 조사만.** 결과는 **구현하지 않는다**이고, 근거는 추론이 아니라 실측이다.

**C4가 실제로 무엇인가.** `linkWorkflow`는 Workflow 도구 호출과 디스크의 실행 저널을 대조한다 —
경로가 세션 transcript에서 유도한 바로 그것인지, 스크립트의 sha256이 도구 호출의 것과 같은지,
resume이 `resumeFromRunId`와 같은 스크립트인지. 모듈 주석이 이유를 말한다:
*"임의 폴더를 스캔해서 자식을 발견하지 말 것."*

**즉 C4는 파일을 읽어서 라우팅하는 층을 안전하게 만드는 장치다. 이 빌드는 라우팅을 위해 디스크를
전혀 읽지 않는다.** 역할은 클라이언트가 직접 실행한 hook이 세션 토큰을 달고 loopback으로 보고한다.
C4를 가져오려면 먼저 **사용자가 보류하기로 한 그 파일 읽기 검증부터** 만들어야 한다. 방어할 대상을
먼저 만들고 나서 방어하는 셈이다.

**실측 — workflow 에이전트가 이 빌드에서 어떻게 도는가** (추론 0회, 스크립트 백엔드 + 실제 클라이언트):

| 관찰 | 값 |
|---|---|
| hook이 붙는가 | **붙는다.** `unregistered=0` — 등록이 도착하고 요청이 그 id를 들고 온다 |
| `agent_type` | **`workflow-subagent`** 하나로 고정 |
| 실제로 간 곳 | `gpt-6-astra/low` = **세션 자신의 route** |

기준선이 저널 검증으로 얻는 것은 `selectModel(parentRoute)` — **부모 route 상속**이다.
이 빌드는 "선택 실패가 턴을 죽이지 않는다"는 안전 기본값으로 **같은 결과**에 이미 도달한다.
클라이언트가 부모 모델을 보내고, 우리가 그걸 유지하기 때문이다. 파일을 한 번도 안 읽고.

**조사가 진짜 결함 하나를 찾았다.** workflow 에이전트마다 `unrouted`가 올라가고 있었다. 그러면
workflow를 쓴 모든 세션이 "보고할 게 있는 세션"이 되어 전체 JSON이 찍힌다 — **D 내내 막으려던
'매번 우는 진단'을 내가 방금 만들어 놓은 것**이다. `workflow-subagent`를 **의도적 상속 역할**로
이름 붙여 고쳤다. 베타 보고에서 `serverDependent`와 `unknown`을 가른 것과 같은 구분이다:
아무도 안 본 이름과, 보고 나서 그대로 두기로 한 이름은 다른 답이다.

변이 3건 전부 잡힘.

### G9 기본 전환 — 완료 (2026-09-17)

사용자 결정: **지금 전환.** `clauduct`는 Go 빌드이고 Node는 `clauduct-node`로 남는다.

설치된 것은 세 개다. `clauduct.exe` · `clauduct-hook.exe` · `clauduct-dev.exe`.
PACKAGING.md는 **두 개**라고 적고 있었다 — `clauduct-hook`이 빠져 있었고, 그건 선택 사항이 아니다.
`findHook()`은 실행 파일 **옆만** 보므로, 없으면 hook이 설치되지 않고 역할별 라우팅과 위임 메뉴의
effort가 **조용히** 동작하지 않는다.

그래서 **계정이 그걸 말하게 했다**: `session.hookInstalled`. 아무도 찾아볼 생각을 안 할 실패를
매 세션이 스스로 보고한다. hook이 없으면 "보고할 게 있는 세션"이 되어 전체 JSON이 찍힌다.
실설치 확인: `hookInstalled: true`.

**그리고 전환이 한 셸에서 먹지 않고 있었다 (2026-09-17).** `~/.local/bin`에는 v1 설치 프로그램이
만든 **`clauduct`라는 디렉터리**(버전 저장소)가 있고, MSYS/Git Bash의 PATH 탐색은 같은 폴더의
`clauduct.exe`에 도달하기 전에 그 디렉터리에서 멈춘다 — `type -a clauduct` → not found. cmd는
PATHEXT로 정상 해석하므로 **셸에 따라 G9가 적용되기도 하고 안 되기도 했다.** 재현으로 확정했다:
같은 이름의 디렉터리를 지우면 즉시 해석된다. v1 시절에도 같은 이유로 bash에서는 `clauduct`가
동작한 적이 없다 — G9의 회귀가 아니라 G9가 고칠 수 있었던 기존 결함이다.

저장소를 `clauduct-node-store`로 개명하고 `clauduct-node.cmd`가 그곳을 가리키게 했다. bash에서
`clauduct`가 Go 빌드로 해석되고, v1 `--dry-run`도 그대로 돈다(둘 다 확인). 남는 조건:
`install.ps1`은 `$bin/clauduct`를 하드코딩하므로 **v1을 재설치하면 충돌이 돌아온다.**

되돌리기는 **두 파일 이름 바꾸기**다. 그리고 쉽게 틀리는 부분을 문서에 적었다 — Windows `PATHEXT`는
`.EXE`를 `.CMD`보다 먼저 보므로, `clauduct-node.cmd`를 `clauduct.cmd`로 되살리는 것만으로는
부족하고 **`clauduct.exe`를 치워야** 한다.

### F. 결정 완료 (2026-09-16)

| # | 항목 | 결정 |
|---|---|---|
| F1 | 재시도 소유권 | **철회했다 (2026-09-17).** 결정은 원장에만 있었고 코드는 `MaxGatewayRetries = 0` 그대로였다. 실측 후 코드가 맞다고 판정 — 12.2절 |
| F2 | 종료 코드 | **자식 코드 전파 유지 + category 줄 추가.** 기준선의 0/1은 스크립트가 쓰는 정보를 버리고, Go에는 사람이 읽을 이름이 없다. 둘은 배타적이지 않다 |
| F3 | 거부 옵션 | **Go의 2개 유지.** 기준선 목록을 따라가면 그것이 가졌던 `--name --model` 혼동도 따라온다. B2·B5 완료 후 실제 목록은 **4개**다: 권한 2개 + `--settings`·`--setting-sources`. `--agents`는 **막지 않는다** — 실측상 사용자 파일 에이전트와 합쳐지고, 사용자가 직접 준 것이 이기는 게 의도한 규칙이다. `--system-prompt`는 주입하지 않으므로 충돌이 없다 |
| F4 | Go 1.27.2 게이트 | **G9를 막지 않는다 (2026-09-17).** 12절 참조 — 우리가 할 수 있는 조치가 없다 |

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

라이브가 필요한 것은 A 전체와 D5(실제 rate-limit 헤더 관찰). C2는 **추론 0회로 끝났다** — 스크립트
백엔드에 실제 클라이언트를 물리면 서브에이전트가 진짜로 뜨고, 그게 라이브와 같은 것을 증명한다.
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

### 12.1 F4 실측 — 전제가 반대였다 (2026-09-17)

**"1.27.2가 아직 안 나왔으니 미리 대응할 필요가 있나"는 마일스톤을 위협으로 읽은 것이다.** 반대다.
#81374는 **백포트 티켓**(`CherryPickApproved`)이고 마일스톤 1.27.2는 **수정이 들어갈 곳**이다.
결함은 지금 쓰는 1.27.0/1.27.1에 **이미 살아 있다.** 1.27.2는 위협이 아니라 고침이다.

그래서 F4는 "미출시 버전 대응"이 아니라 **"지금 쓰는 런타임의 알려진 결함에 우리가 노출되는가"**다.

**측정 1 — ALPN은 `h2`다.** `chatgpt.com:443` TLS 핸드셰이크 실측(`NegotiatedProtocol="h2"`,
TLS 1.3). #81411·#81515·#81516은 전부 **HTTP/1 automatic draining** 결함이므로 이 경로에 없다.
12절 표의 "upstream이 h2면 무관"이 확정됐다.

**측정 2 — #80979 발동 조건.** 부모가 **overlapped** 파이프를 만들고, 그것을 stdout으로 물려받은
Go 프로세스의 `os` init이 완료 포트에 연결하면, 같은 핸들을 쥔 쪽의 동기 `WriteFile`이 멈춘다.
원 보고의 부모는 **OpenSSH Server**다.

**측정 3 — 그 재현 시도는 공허했다.** `os.Pipe()`는 `syscall.Pipe` → **`CreatePipe`**,
즉 **non-overlapped**다. 그래서 Go의 `os/exec`도, 일반 셸 파이프(`|`)도, 콘솔도 overlapped 파이프를
만들지 않는다. 처음 돌린 400,000줄 무정지 결과는 **overlapped 핸들이 아예 없었으므로 아무것도
증명하지 않는다.** 확인 안 했으면 "재현 안 됨"을 근거로 쓸 뻔했다. 미실행은 통과가 아니고,
**잘못된 조건에서의 통과도 통과가 아니다.**

**결론: F4는 G9를 막지 않는다.** 근거 셋.

1. 노출 경로가 **OpenSSH 류의 overlapped 파이프 부모**로 한정된다. 콘솔·셸 파이프·Go의 exec은 아니다.
2. **application-level workaround가 없다**(상류가 명시). 기다려도 우리가 손댈 것이 생기지 않는다.
3. 이것은 **모든 Go 바이너리**의 런타임 결함이다. Clauduct가 특별히 노출되는 게 아니다.

우리가 쥔 레버는 **툴체인 버전 한 줄**뿐이다. 1.27.2가 나오면 올린다. 그때까지 게이트로 두는 것은
행동 가능한 것이 없는 항목으로 출시를 막는 것이다.

**함께 고친 것**: CI가 1.27.0에 고정돼 있었고 개발·테스트는 1.27.1에서 했다. 배포하는 것과 다른
패치 릴리스를 검증하고 있었다. CI를 1.27.1로 맞췄다.

### 12.2 F1 재시도 — 원장이 코드를 앞질러 있었다 (2026-09-17)

F 표의 F1은 "응답 전 연결 실패만 3회 재시도"로 **결정됐다고 적혀 있었고**, 코드는
`MaxGatewayRetries = 0`에 재시도 루프가 없었다. 주석은 여전히 "누가 재시도를 소유하는지 정해야
한다"고 말한다. 둘 중 하나가 틀렸으므로 구현하기 전에 전제를 쟀다.

**측정 1 — 표준 라이브러리가 이미 안전한 경우를 처리한다.** go1.27.1의
`persistConn.shouldRetryRequest`는 POST를 replayable로 보지 않는다(`isReplayable`은 GET·HEAD·
OPTIONS·TRACE 또는 `Idempotency-Key`만). 남는 재시도 경로는 **재사용된 연결에서 아무것도 쓰지
않은 경우**(`nothingWrittenError`) 하나뿐이고, 그것은 "보낸 것이 없음이 증명된" 경우다.
우리가 더할 것이 없다.

**측정 2 — 현재 분류로는 안전한 재시도를 고를 수 없다.** `ClassifyTransport`의
`CONNECTION_FAILED`는 `*net.OpError`를 `Op` 구분 없이 받고, `CONNECTION_TIMEOUT`은
`netErr.Timeout()`이라 **dial 타임아웃과 응답 헤더 타임아웃을 같은 값으로 만든다.** 후자는 요청이
이미 전송된 뒤이므로, 그것을 재시도하면 백엔드가 시작한 생성을 한 번 더 시킨다 — 토큰을 두 번
쓰는 결함이지 복구가 아니다.

**측정 3 — 남는 경우(fresh dial 실패)에는 곱셈 논증이 그대로 성립한다.** dial 실패는 클라이언트가
보는 503이 되고, 클라이언트는 5xx를 60초에 8회 재시도한다(기존 실측). 우리가 3회를 더하면 지속
장애에서 24회가 된다. `MaxGatewayRetries = 0`이 막으려던 바로 그 곱셈이다.

**판정: 코드가 맞고 원장이 틀렸다.** F1을 철회한다. 되살리려면 선행 조건이 하나 있다 —
**전송 전/후를 분류가 구분할 수 있어야 한다**(`OpError.Op == "dial"` 수준). 그 구분 없이 붙이는
재시도는 복구가 아니라 중복 과금이다.
