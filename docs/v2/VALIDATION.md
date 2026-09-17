# V2 검증 계획 — 요구·테스트·게이트·예산

## 1. 현재 증거 상태

| 항목 | 값 |
|---|---|
| 실행한 V2 Go 테스트 | **1119개 통과 / 0 실패 / 3 skip** (subtest 포함), 테스트가 있는 15 package(`internal/protocol/codex`는 테스트 파일 없음). `CGO_ENABLED=0 go test ./... -count=1` 2026-09-17 실행 |
| mutation 검증 | **230건 주입** (battery 15개) + 2026-09-17 수동 4건. 현재 전부 잡힌다. 처음 주입 때 살아남은 것은 각 절에 기록했다 |
| 실모델 호출 | **추론 55회.** probe 원장 35회(`gpt-5.6-luna`/low: 1.3절 상한 · 1.4절 wire · 1.6절 G7 실세션 · 1.9절 LIFE17 · 6.8절 A그룹 · D5 헤더) **+ 2026-09-17 제품 빌드 실세션 20회**(`gpt-6-astra`/low, 9세션). 마지막 2회는 **의도하지 않은 소비**다 — `--uninstall`이 프롬프트 안에 있을 때 삭제되지 않는지를 실제 바이너리에 `-p`로 확인했는데, 그 경로는 백엔드까지 간다. 같은 것을 단위 테스트가 이미 증명하고 있었다(`TestUninstallRequestedTakesTheFirstArgumentOnly`). 옵션 파싱은 실호출로 확인할 것이 아니다 |
| 잔여 승인 예산 | **45회** (2026-09-15 사용자가 누적 100회로 상향) |
| 그 20회의 출처 | `%TEMP%\clauduct\status-*.json` 실측(`clauduct-dev usage`). status 기록은 2026-09-17 06:16에 들어왔으므로 이 파일들은 09-15·09-16 실세션과 겹치지 않는다. 전부 에이전트가 검증으로 띄운 세션이다 — **사용자가 스스로 띄운 세션은 이 예산에 넣지 않는다.** 다음에 셀 때 status 파일을 그대로 합치면 사용자 사용량까지 예산으로 청구하게 된다 |
| 검색 요청 | ledger를 쓰지 않는다(설계). 2026-09-17에 1회 더 실행했고 통과했다 — 1.11절 |
| skip 3건 | live 1(예산 opt-in), 수동 2(콘솔 종료·런처 강제 종료) |

### 1.1 실행한 것

**Node 기준선 (변경 없음 확인용)**

| 명령 | 결과 | 관측 |
|---|---|---|
| `node verification/test-doc-citations.mjs` | PASS exit 0 | documents 86, citations 80, linksChecked 347, failures 0, externalRequests 0, credentialReads 0 |
| `./clauduct.cmd --dry-run -p` | PASS exit 0 | model gpt-6-astra, effort low, contextPolicy 400000/320000/20000, generalAgentModels 14행 |

두 번째 명령의 `credentialReads`·`childStarted`·`globalWrites`는 HANDOFF.md 3장이 "이 분기가 쓰는 고정값이라 아무것도 증명하지 않는다"고 못박은 값이다. 무접속의 증거로 인용하지 않는다.

**Node 기준선 — 패키지별 직전 검증 (2026-09-15 확정 정책)**

하나의 큰 스냅샷 대신, 각 WP는 자기가 계약을 가져올 파일들의 Node 테스트를 착수 직전에 돌린다. 소스는 동결돼 있으므로(D02, tracked 변경 0) 시점은 환경에만 의존하고, 이 방식은 스냅샷이 낡을 일이 구조적으로 없다.

| 대상 | 결과 |
|---|---|
| WP03 관련 30개 (`native-protocol`·`native-delivery`·`native-transport` 의존) | **44 pass / 0 fail / 1 skip, 11.6초** |

각 suite가 자체 보고한 값: `externalRequests: 0`, `credentialReads: 0`, `actualClaudeExecutions: 0`. 실행 전 범위를 검토했다 — `https://` 참조는 전부 fixture 문자열(`example.com` 등)이고 `interactiveLaunch`는 spec을 만들 뿐 spawn하지 않는다.

나머지 83개는 WP06·WP07이 그 계약을 가져올 때 그때의 환경에서 돌린다.

**Go V2 (WP01)**

| 검사 | 결과 |
|---|---|
| `gofmt -l .` | 출력 없음 |
| `go vet ./...` | 통과 |
| `go build ./...` | 통과 |
| `go test -count=1 ./...` | **8 package 통과**, 349 테스트 |
| `go build -trimpath` 후 `version` | VCS stamp 확인: `bfbdf238…+dirty`, go1.27.0 windows/amd64 |
| `clauduct-dev doctor` | exit 0. claude.exe 해석 성공, 82 parent vars → 87 child vars, credential 읽기 0 |
| `clauduct-go --version` | exit 0. 실제 claude.exe가 `2.1.272 (Claude Code)` 출력 |

마지막 항목은 실제 사용자 프로필에서의 기회적 관측이지 통제된 `NATIVE_SYNTH` 실행이 아니다. ARG06·ARG07의 증거로 승격하지 않는다.

**WP05 전까지 실호출 0의 근거는 구조였다.** upstream 클라이언트가 존재하지 않았으므로 모델 요청은 "일어나지 않았다"가 아니라 "일어날 경로가 없다"였다.

**WP05부터는 그 문장을 쓸 수 없다.** 실제 HTTPS 전송이 `internal/upstream.Direct`로 존재한다. 대신 더 약하지만 정확한 주장이 남는다.

| 근거 | 확인 방법 |
|---|---|
| 제품 빌드는 실제 전송에 닿지 않는다 | `internal/app/run.go`가 `gateway.Start(nil)`을 호출하고, gateway는 nil을 `upstream.None`으로 바꾼다. `Direct`를 참조하는 곳이 `cmd/clauduct-dev` 밖에 없다 |
| `Direct`는 예산 없이 소켓을 열지 않는다 | `Reserve`가 credential 읽기보다 먼저다. 테스트는 클라이언트 반환값이 아니라 **listener가 센 요청 수**로 확인한다 |
| 유일한 도달 경로가 명시적 동의를 요구한다 | `clauduct-dev probe`는 `--send` 없이는 아무것도 보내지 않는다. 오타·접두사·인자 추가 9종을 테스트가 덮는다 |
| probe는 아직 실행하지 않았다 | 이 세션에서 `--send`를 한 번도 붙이지 않았다. 원장 누적 0 |

### 1.1.2 실측 — claude 2.1.272가 실제로 보내는 것

WP02의 인증·경계 규칙을 Node 기준선의 규칙과 핸드오프 경고만 보고 설계할 뻔했다. 그것은 근거가 아니라 회상이다. upstream이 없는 일회용 listener에 실제 `claude.exe`를 붙여 측정했다. **모델 호출 0회** — 이 probe에도 upstream이 없어 경로 자체가 없다.

| 관측 | 값 | 설계에 미친 영향 |
|---|---|---|
| readiness | `HEAD /api/hello`, **Authorization 없음** (`User-Agent: Bun/1.4.3`) | 무인증 readiness가 옳다. 단, 인증을 제시하면 검증한다 |
| 인증 header | **`Authorization: Bearer` 하나뿐.** `x-api-key` 없음, 중복 header 없음 | HTTP06의 "복수 auth header"는 현재 native가 만들지 않는다. 그래도 전방 호환으로 구현하고 **측정된 미발생**을 기록한다 |
| `GET /v1/models` | **호출되지 않음** | discovery는 `CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY` opt-in이다. 이 키는 WP01 overlay에 없다. `/v1/models` 구현은 WP07 |
| query | `?beta=true` | HTTP05 fixture의 근거 |
| `-p "ping"` 한 번의 본문 | **119,312 bytes**, 최상위 키: `context_management max_tokens messages metadata model output_config stream system thinking tools` | 32 MiB 상한의 근거이자 WP03 envelope의 실물 |
| 그 밖의 header | `Anthropic-Beta`(다건), `Anthropic-Version: 2023-06-01`, `X-Claude-Code-Session-Id`, `X-Stainless-Retry-Count`, `X-Stainless-Timeout: 600` | session id는 CAP06 correlation 후보. retry-count는 R07 입력 |
| **400 응답에 대한 반응** | **동일 POST 2회** | 기준선의 `CLAUDE_CODE_MAX_RETRIES: '0'`가 왜 있는지 설명된다. gateway가 오류를 내면 클라이언트가 시도를 늘린다 — LIVE 예산 산정 시 **cap은 socket을 열기 전 우리 쪽에서** 걸어야 한다는 직접 증거 |

마지막 행은 예산 항목이다. G6→G7 전이 검토 시 이 관측을 근거로 쓴다.

### 1.1.1 mutation 검증 — 초록이 진짜인지

통과하는 suite는 실패할 수 있어야 의미가 있다. 코드를 일부러 깨뜨려 확인했다.

| 주입한 결함 | 결과 |
|---|---|
| env denylist 무력화 | 1차 시도에서 **통과해버렸다.** overlay가 그 5개 키를 무조건 덮어써서, 하필 그 5개만 leak 대상으로 쓴 테스트가 결함을 가리고 있었다. overlay 밖의 이름을 추가해 보강한 뒤 `launch`·`app` 두 계층 모두 실패 |
| gateway를 모든 인터페이스에 bind | gateway 테스트 3개 실패 |
| 실행 파일 해석보다 gateway bind를 먼저 수행 | `TestMissingExecutableBindsNothing` 실패 |

첫 행이 이 절을 쓰는 이유다. 보강 전 그 테스트는 초록이었지만 아무것도 지키지 못하고 있었다.

WP02의 8건(Host 고정·두 번째 credential 검사·중복 header·종료 취소·read deadline·멱등 release·close 순서·동시 실행 상한)은 전부 즉시 잡혔다.

옵션 거부 도입 시 5건을 더 주입했고 **1건이 살아남았다** — `app.Run`에서 거부 호출을 통째로 지워도 초록이었다. `internal/launch`의 테스트가 `Refused()`를 직접 호출하기 때문이다. 규칙을 아는 곳에만 테스트가 있고 **그것을 강제하는 곳**에는 없었다. 같은 실패 유형이 두 번째다. 강제 지점 테스트를 추가한 뒤 잡힌다.

### 1.2 실행하지 않은 것

| 대상 | 상태 | 이유 |
|---|---|---|
| Node 회귀 나머지 83개 | `NOT_RUN` | 패키지별 직전 검증 정책. WP06·WP07이 그 계약을 가져올 때 돌린다. WP03 관련 30개는 위 1.1에서 통과했다 |
| `go test -race` | **PASS** | 2026-09-15 CI에서 `CGO_ENABLED=1`로 실행, 12 package 통과. 이 머신에는 C 툴체인이 없어 로컬에서는 여전히 불가하다 |
| Go CI 워크플로 | **PASS** | 2026-09-15 사용자 승인 후 push. 3회 실행, 첫 두 번의 실패가 각각 진짜 결함이었다 — 1.8.4절·1.8.6절 |
| 대화형 세션 | `NOT_RUN` | 제품 빌드에 실제 전송이 연결돼 있지 않다. 연결은 G7 사항이다 |
| native synthetic 통합 (NATIVE_SYNTH) | `NOT_RUN` | 격리 fixture profile을 아직 만들지 않았다. WP06 |
| 실모델 호출 | `NOT_AUTHORIZED` | 3장 |

### 1.3 실호출 — 5회, 그리고 그것이 답한 것

사용자 승인(2026-09-15, *"돌린다. gpt 모델은 묻지 말고 돌려라"*) 하에 `clauduct-dev probe --send`를 두 번 돌렸다. 전부 `gpt-5.6-luna` / effort `low`.

| 실행 | 요청 | 결과 |
|---|---|---|
| 1차 (2회) | 대조군 / `max_output_tokens: 16` | `response.completed` 83 토큰 / **HTTP 400** |
| 2차 (3회) | 대조군 / `16` / `48` | `response.completed` 122 토큰 / **HTTP 400** / **HTTP 400** |

**1차만으로는 결론을 낼 수 없었다.** 400이 파라미터를 거부한 것인지, 값 16이 최솟값 아래라서인지 구분되지 않는다. 후자라면 사전 상한이 가능하다는 뜻이고 3장의 BLOCKED가 풀린다 — 정반대 결론이다. 그래서 어떤 최솟값보다도 확실히 크고 답의 길이(83·122 토큰)보다는 작은 48로 한 번 더 돌렸다.

**둘 다 400이다.** 그 키만 뺀 동일한 본문은 두 번 다 통과했다. 최솟값 문제가 아니라 파라미터 자체가 거부된다.

#### 이 5회가 증명한 것 둘

**1. WP05의 `max_output_tokens` 제거는 기준선 parity가 아니라 출시 차단 결함의 수정이었다.** WP03·WP04가 만든 본문은 그 키를 항상 보내고 있었다. 즉 **이 bridge를 통한 모든 추론 요청이 400으로 끝났을 것이다.** 기준선 대조가 아니었으면 실호출을 붙이는 순간에야 발견됐다.

**2. 응답 경로가 실제 바이트에서 처음 동작했다.** `stream.Parser`가 실제 스트림을 오류 없이 파싱했고 `DecodeUsage`가 카운트를 읽었다.

**다만 이때 "wire 형식이 검증됐다"고 쓴 것은 과장이었다.** 이 probe는 본문을 **손으로 만든다.** `bridge.BuildRequest`는 한 번도 실행되지 않았다. 검증된 것은 endpoint가 그 모양을 받는다는 사실이지 이 모듈이 그 모양을 만든다는 사실이 아니다. 요청 경로 검증은 1.4절이다.

### 1.4 wire probe — 제품 경로를 실제 backend에 걸었고, WP04가 무너졌다

1.3절의 probe는 본문을 손으로 만들었으므로 이 모듈의 인코딩을 검증하지 않았다. `probe wire`는 **제품 경로 그대로** 돈다 — `anthropic.DecodeRequest` → `bridge.BuildRequest` → 전송 → `stream.Parser` → `bridge.Translator`. 손으로 쓴 본문도 손으로 읽은 이벤트도 없다.

| 실행 | 결과 |
|---|---|
| 1차 | text **6 프레임 정상** / tool `UNSUPPORTED_EVENT` |
| 2차 (어휘 수정 후) | text 정상 / tool `EMPTY_REPLY` |
| 3차 (`tool_choice` 강제 + 계측) | text 정상 / tool `EMPTY_REPLY`, **원인 확정** |

#### 결함 1 — 도구 요청이 통째로 죽었다

```
tool call  REFUSED TRANSLATE: UNSUPPORTED_EVENT
```

`response.function_call_arguments.delta`/`.done`이다. **모든 도구 요청에 반드시 오는 이벤트인데 V2가 모른다.** barrier 설계는 "호출은 `completed`에서만 만든다"였고 그것은 맞았지만, **그 이벤트들이 여전히 도착한다는 것을 잊었다.** 모르는 이벤트는 거부하는 것이 V2의 의도된 기본값이므로 요청 전체가 죽는다.

**오프라인 테스트가 이 결함을 명시적으로 주장하고 있었다.** `TestUnknownEventIsRefused`가 `response.function_call_arguments.delta`를 "거부되어야 하는 이벤트" 목록에 넣고 초록이었다. 테스트가 자기를 쓴 사람과 합의한 것이다.

고치면서 기준선이 이 이벤트로 무엇을 하는지 봤다. 무시하지 않는다 — 스트리밍된 arguments를 누적해 `.done` 스냅샷과 대조한다(`SNAPSHOT_MISMATCH`). V2가 텍스트에 하는 것과 같은 검사다. 도구 호출은 클라이언트가 **실행**하므로, 두 진술이 어긋나면 하나를 고르는 것이 아니라 멈춘다. `ARGUMENTS_MISMATCH`로 포팅했다.

이벤트를 추가할 때 기준선 목록을 통째로 베끼지 않고 원칙을 세웠다.

| 처리 | 대상 | 이유 |
|---|---|---|
| 알고 누적·검증 | `function_call_arguments.delta`/`.done` | 실측으로 도착이 확인됐고 대조할 내용이 있다 |
| 알고 무시 | `rate_limits.updated`, `codex.rate_limits`, `codex.response.metadata`, `responsesapi.websocket_timing`, `reasoning_text.delta`/`.done` | 답의 일부를 싣지 않으므로 무시해도 응답이 짧아질 수 없다 |
| **계속 거부** | `refusal.*`, `output_text.annotation.added`, `custom_tool_call_input.*`, `web_search_call.*`, `item.*` | 전부 V2가 지원하지 않는 **내용**을 싣는다. 받아서 버리면 정확히 그만큼 짧은 응답이 된다 |

기준선이 이름을 안다는 것은 **도착할 수 있다는 증거**이지 무엇을 해야 하는지의 증거가 아니다. 내용에 대해 처리를 추측하는 것이 응답이 조용히 한 부분을 잃는 방식이다.

#### 결함 2 — barrier가 채워지지 않는 배열을 읽고 있다

어휘를 고치자 `EMPTY_REPLY`가 나왔다. `tool_choice`로 호출을 강제하고 계측을 붙였다.

```
saw [response.created response.in_progress response.output_item.added
     response.function_call_arguments.delta response.function_call_arguments.done
     response.output_item.done response.completed]
output items [empty output array]
```

**도구 호출은 분명히 일어났다.** 그런데 `response.completed`의 `output` 배열이 **비어 있다** — 키는 있고 배열이 빈 것이다. 텍스트 요청에서도 똑같이 비어 있다.

WP04의 barrier는 이렇게 구현돼 있다.

> 호출은 스트리밍 이벤트가 아니라 `response.completed`의 `output` 배열에서만 나온다.

**이 backend는 그 배열을 채우지 않는다. 따라서 V2는 도구 호출을 영원히 만들 수 없다.**

기준선은 `output_item.added`의 스냅샷으로 item을 만들고 `output_item.done`으로 대조한다(`native-protocol.mjs:596`). `completed`는 종결 회계를 한다. V2는 `output_item.*`를 **구조적 이벤트로 분류해 통째로 무시**하고 있었다.

같은 이유로 조용히 죽어 있던 것이 하나 더 있다. `checkStreamedText`는 completed output의 `message` item에 대해서만 돈다. 그 item이 오지 않으므로 **V2의 텍스트 스냅샷 검사도 이 backend에서 한 번도 실행된 적이 없다.**

#### 왜 offline으로는 절대 못 찾는가

fixture를 쓴 사람이 `completed.output`에 item을 넣었고, 디코더가 그것을 읽었고, 테스트가 통과했다. **fixture는 그것을 쓴 사람과 합의한다.** 세 층(fixture·디코더·테스트)이 전부 같은 잘못된 전제를 공유하면 오프라인에서 초록은 정보가 아니다.

`max_output_tokens`와 정확히 같은 모양의 결함이고, 실호출 1회에 드러났다. 이것이 WP06 착수 전에 도구 wire를 실호출로 검증하자고 한 이유이며, 판단은 맞았다.

#### 수정 완료 — barrier 원칙은 살아 있다

barrier가 보장하는 것은 **"완료 전에는 클라이언트에 아무것도 전달되지 않는다"**이고 그것은 유지된다. 바뀌는 것은 데이터의 **출처**다.

| | 현재 | 수정 |
|---|---|---|
| item 생성 | `completed.output` (비어 있음) | `output_item.added` 스냅샷 |
| item 확정 | — | `output_item.done` 스냅샷과 대조 |
| arguments | `completed.output` | 누적 + `.done` 대조 (이미 구현) |
| **전달 시점** | `completed` | **`completed` — 바뀌지 않는다** |

즉 barrier는 구조적(만들어지는 자리가 뒤)에서 시간적(만들어지되 붙잡아 둠)으로 바뀐다. 기준선이 하는 방식이다.

#### 1.4.1 재작업 결과 — 4차 실행에서 SOUND

사용자 승인(2026-09-15) 후 재작업했다.

```
text         6 frames, reply 2 chars
tool call    6 frames, call_id 29 chars, arguments {}
tool result  13 frames, reply 16 chars, tool result reached the model
reading      SOUND — request, tool call and recorded result all survive the real backend
```

**세 번째 줄이 결정적이다.** 도구가 돌려준 값 `BUILD-TOKEN-7Q4M`(정확히 16자)를 모델이 되풀이했다. 모델이 **다른 방법으로는 알 수 없는 값**이므로, `function_call` + `function_call_output` 인코딩이 실제로 모델에 도달했다는 뜻이다. 인코딩은 `native-protocol.mjs:378`·`397`과 대조해 동일함을 확인했다.

재작업의 내용은 다음과 같다.

| 항목 | 내용 |
|---|---|
| item 생성 | `output_item.added` 스냅샷에서. index는 조밀·오름차순이어야 하고 id는 중복될 수 없다 |
| item 확정 | `output_item.done`이 권위 있는 최종본이되, **같은 item이어야 한다** — id·kind가 바뀌면 둘 중 하나가 다른 것을 말하고 있다 |
| 여는 스냅샷의 관대함 | 열리는 item은 신원만 있어도 된다(arguments는 뒤에 흐른다). 닫히는 것은 완전해야 한다. 기준선의 `functionSnapshot(first, true)` 구분과 같다 |
| arguments | 누적 후 `.done` 이벤트와, 그리고 item의 최종 스냅샷과 **둘 다** 대조. 흐른 적이 없으면 불일치가 아니다 — backend는 통째로 줄 권리가 있다 |
| **전달 시점** | `response.completed`. **바뀌지 않았다** |
| 미완료 item | 응답이 끝났는데 item이 열려 있으면 거부한다. 부분만 전달하면 backend가 썼다고 말한 적 없는 것을 건네는 것이다 |
| completion 교차검사 | 빈 배열은 모순이 아니다(아무것도 말하지 않은 것). 비어 있지 않은데 스트림과 다르면 거부 |
| 보류 상한 | `maxOutputItems = 1024`. Builder는 **방출된 것**을 묶지만 보류는 그 앞이다 |

#### 1.4.2 fixture가 세 층에서 같은 거짓말을 하고 있었다

재작업하면서 fixture를 실제 wire로 바꿨다. 그러자 기존 테스트가 무더기로 깨졌다 — **그것이 요점이다.**

| 깨진 것 | 무엇을 담고 있었나 |
|---|---|
| `completedWith(...)` | `completed.output`에 item을 넣었다. 실제로는 빈 배열이고 item은 `output_item.*`로 온다 |
| `event(codex.OutputItemAdd, '{"type":"response.output_item.added"}')` | `output_index`도 `item`도 없는 **자리표시자**다. 이벤트가 무시되고 있었으므로 아무것도 주장하지 않았다 |
| `functionCall(...)` | item `id`가 없었다. 실제 item은 client가 결과를 보내는 `call_id`와 **별개의 id**를 갖고, arguments 스트림은 그 id로 묶인다 |
| `TestUnknownEventIsRefused` | `function_call_arguments.delta`를 "거부되어야 함"으로 명시했다 |
| `TestRepeatedCallIdentifierIsRefused` | 같은 call_id에 같은 item id를 써서, item 중복 방어와 call_id 방어를 구분하지 못했다 |

마지막 것은 고치면서 **테스트가 둘로 갈렸다.** item 중복(backend 자체 장부)과 call_id 중복(client가 주소를 지정할 수 있는가)은 다른 방어다.

#### 1.4.3 mutation — 21건, 4건이 살아남았다

| 살아남은 결함 | 왜 | 조치 |
|---|---|---|
| completion이 다른 **개수**를 말해도 통과 | 교차검사 테스트가 전부 1:1이라 개수가 어긋나는 경우가 없었다 | 2개 vs 1개 사례 추가 |
| 닫히는 item이 arguments를 빠뜨려도 통과 | 여는 스냅샷의 관대함만 테스트했고 **닫는 쪽의 엄격함**은 아무도 안 봤다 | call_id·name·arguments·content 누락 4종 추가 |
| 음수 output_index 허용 | **중복 검사였다.** `openItem`의 `Index != len(t.order)`가 음수를 전부 잡으므로 디코더의 검사는 결과를 바꿀 수 없다 | 테스트가 아니라 **중복을 삭제했다.** 닿을 수 없는 guard는 guard가 아니다 |
| `item`이 없는 이벤트 허용 | 빈 item을 보류하면 그 index가 무엇이었든 응답이 그만큼 짧아진다 | item 없음·null·kind 없음·index 없음 4종 추가 |

compiler-only 1건도 있었다 — 교차검사 mutation이 컴파일되지 않았다. 컴파일되는 두 형태로 나눴다. 재실행: **21건 주입, 미검출 0, compiler-only 0.**

### 1.5 NATIVE_SYNTH — 실제 클라이언트가 처음으로 V2를 통과했다

WP06은 **실제 `claude.exe`를 fixture backend에 붙인다.** 지금까지 실제 클라이언트를 붙이면 언제나 `400 NO_UPSTREAM_TRANSPORT`로 끝났다. 이번에 처음으로 세션이 성립했고, 모델 호출은 **0회**다.

```
text  reply reaches stdout   1초
```

fixture가 replay하는 것은 **1.4절에서 실측한 wire 모양**이다 — `output_item.added` → delta → `output_item.done` → `completed`(빈 output 배열). 그럴듯한 모양을 쓰면 WP04에서 저지른 실수를 그대로 반복한다.

#### 1.5.1 격리는 가정이 아니라 측정이다

실제 클라이언트는 `~/.claude/`를 읽고 쓰며, 사용자 설정에 MCP 서버가 있으면 **그것을 띄운다.** 테스트가 사람의 Slack이나 DB 서버를 시작하는 것은 어떤 정의로도 격리가 아니다.

| 장치 | 확인 |
|---|---|
| `CLAUDE_CONFIG_DIR`을 임시 디렉터리로 | **실측**: 클라이언트가 존중한다. `.claude.json`·`backups/`·`projects/`·`sessions/`가 전부 그쪽에 생겼다 |
| `--strict-mcp-config` 상시 | "Only use MCP servers from --mcp-config" — 사용자 MCP가 도달할 경로를 구조적으로 없앤다 |
| 임시 작업 디렉터리 | project 수준 설정 파일이 끼어들 수 없다 |

**`--bare`는 일부러 기본값이 아니다.** hooks·plugin·CLAUDE.md 탐색을 건너뛰는데, 그것들이야말로 몇몇 테스트가 관찰하려는 대상이다. 기본으로 켜면 모든 결과가 보통 세션을 대표하지 못한다.

**남는 위험을 적어둔다**: 시스템 전역 managed settings 파일은 여전히 적용되며 이 harness는 그것을 격리하지 않는다.

#### 1.5.2 leak 테스트는 overlay가 덮는 이름으로는 성립하지 않는다

WP01이 이미 한 번 걸린 함정이다. overlay는 5개 `ANTHROPIC_*` 이름을 **무조건 덮어쓰므로**, 그 5개만 leak 표지로 쓰는 테스트는 denylist를 통째로 지워도 초록이다.

그래서 canary는 **overlay가 설정하지 않는 이름**을 쓴다 — `ANTHROPIC_MODEL`. 새면 클라이언트가 그 값으로 동작하고, 동작한 결과는 **backend 요청 본문에 나타난다.** fixture가 그 본문을 기록하므로 읽을 수 있다.

mutation이 확인한다: denylist를 지워도, **overlay 이름만 남기도록 좁혀도** 둘 다 잡힌다.

#### 1.5.3 덮은 테스트 ID

| ID | 상태 | 어디서 |
|---|---|---|
| ARG06 unknown 옵션 전달·native 오류 보존 | PASS | 실제 클라이언트가 자기 오류를 낸다. launcher는 옵션을 소유하지 않으므로 의견이 없다 |
| ARG07 help/version이 real auth 없이 | PASS | exit 0, **backend 호출 0회**. 버전 질문이 추론 비용을 내면 안 된다 |
| ENV04 사용자 `CLAUDE_CONFIG_DIR` 보존 | PASS | 합성 디렉터리에 클라이언트 자체 상태가 실제로 쌓인다 |
| ENV06 사용자 hook·agent 덮어쓰기 없음 | PASS | 미리 놓은 agent와 settings가 **바이트 단위로** 그대로다 |
| ENV07 BASE_URL 충돌 | **부분 — 기록된 gap** | 덮어쓰기는 동작한다. **진단은 하지 않는다.** 테스트가 그 부재를 고정해, 구현되면 실패하고 다시 쓰이게 했다 |
| ENV08/ENV02 다른 credential 재주입 | PASS | 1.5.2절의 canary |
| ENV10 native 쓰기와 wrapper 쓰기 구분 | PASS | 프로젝트 **옆**에 아무것도 생기지 않는다. 안쪽은 native의 몫이므로 제외한다 |
| CAP04 기본 실행에 agent·hook 주입 0 | PASS | 합성 config 전체와 `.claude.json` 본문에 이 wrapper의 흔적이 없다 |
| CAP03 requested/effective route·근거 기록 | PASS | 1.6.6절. ledger가 `(requested, model, effort, source)`로 센다. 고정 route 두 필드가 제품 경로에서 **빈 문자열**이던 결함을 고쳤다 |
| CAP06 correlation header 부재와 route 모호성 구분 | PASS | 1.6.7절. header 부재는 정상 요청, 라우팅 불가는 400 `UNSUPPORTED_MODEL_OR_EFFORT`, 경로 없음은 404 `UNSUPPORTED_ROUTE` |
| HTTP12 보조 서비스 통신과 모델 route 구분 | PASS | 1.6.8절. `POST /clauduct/agents`는 404, backend 호출 0, attempt 0 |
| CAP10 `--bare`에서도 기본 연결 | PASS | 아래 |

#### 1.5.4 `--bare` — 예측이 틀렸고 측정이 맞았다

`--bare`의 도움말은 이렇게 말한다: *"Anthropic auth is strictly ANTHROPIC_API_KEY or apiKeyHelper (OAuth and keychain are never read)."*

이 overlay는 `ANTHROPIC_AUTH_TOKEN`을 넣고 `ANTHROPIC_API_KEY`를 **빈 문자열로 만든다.** 그 문장만 읽으면 실패를 예측하게 된다. 나도 그렇게 예측했다.

**측정: 연결된다.** 그래서 이것이 문단이 아니라 테스트다.

#### 1.5.5 mutation — 10건, 전부 잡혔다

launcher를 깨뜨려 이 9개 테스트가 실패할 수 있는지 확인했다.

| 주입 | 잡은 테스트 |
|---|---|
| `CLAUDE_CONFIG_DIR` 드롭 | ENV04 |
| ANTHROPIC denylist 삭제 | ENV08 canary |
| denylist를 overlay 이름으로만 좁힘 | ENV08 canary — **WP01의 거짓 초록 시나리오** |
| 프로젝트 옆에 status 파일 쓰기 | ENV10 |
| 사용자 settings 덮어쓰기 / agents 삭제 | ENV06 |
| endpoint·token을 자식에게 안 넘김 | CAP10·세션 |
| launcher가 모르는 옵션을 거부 | ARG06 |
| argv를 전달하지 않음 | ARG06·ARG07 |

앞선 시도에서 "전체 부모 환경 드롭" 주입은 **suite를 멈추게 했다.** 처음에 그것을 `caught`로 적었는데 틀렸다 — **timeout은 테스트 실패가 아니다.** 아무 단언도 발화하지 않았다. harness를 고쳐 `HUNG`으로 따로 세고 실패로 계산하게 했고, 그 주입은 아무것도 말해주지 않으므로 뺐다. 대신 같은 성질을 단언으로 잡는 denylist 주입 둘을 넣었다.

세션 timeout도 25초로 줄였다. fixture 세션은 1초에 끝나므로 넉넉하고, launcher가 깨지면 실제 클라이언트가 재시도에 들어가므로 긴 상한은 mutation 실행을 **실패가 아니라 대기**로 만든다.

#### 1.5.6 아직 안 한 것 — 전부 C 등급

| ID | 왜 |
|---|---|
| ENV05 user/project/managed 설정 우선순위 | **부분 처리됨 — 1.5.7절.** wrapper 몫은 덮었고, managed 층 자체는 시스템 전역이라 `NOT_RUN`으로 남는다 |
| TOOL09 MCP config·schema·service env | MCP 서버를 실제로 띄워야 한다. `--strict-mcp-config`로 막아둔 것을 의도적으로 여는 작업이고 별도 설계가 필요하다 |

#### 1.5.7 ENV05 — wrapper의 몫과 남는 것 (2026-09-16)

세 층을 이 wrapper가 결정하지 않는다. 결정하는 것은 native client다. 그러므로 검증할 명제는 "우선순위가 맞는가"가 아니라 **"wrapper가 그 결정에 끼어들지 않는가"**이다. 그건 잴 수 있다.

| 층 | 어디서 찾는가 | 증거 |
|---|---|---|
| user | `CLAUDE_CONFIG_DIR`·`USERPROFILE`·`HOME` | `TestTheUsersConfigDirChoiceIsPreserved` — 합성 config dir에 **실제 클라이언트가 자기 상태를 썼다**. `TestNothingHereSelectsASettingsLayer` — 세 이름이 사용자가 정한 값 그대로 도착한다 |
| project | 작업 디렉터리 | `TestStdioCwdAndExitCodeAreCarried` + `TestNothingHereSelectsASettingsLayer` — `Dir`가 바뀌면 어떤 `.claude/settings.json`이 적용되는지가 조용히 달라진다 |
| managed | 시스템 전역 경로 | **`NOT_RUN`** |

managed 층을 만들려면 이 머신의 시스템 전역 상태를 바꿔야 하고, 그러면 **이 머신에서 도는 사용자 자신의 모든 claude 세션에 영향이 간다.** 돌리지 않았다. 미실행은 통과가 아니다.

인자 쪽도 같은 명제다: 인자 파서가 없으므로 사용자가 친 `--settings`는 그대로 도착하고, wrapper가 **인자를 추가할 수 없으므로 권한을 넓히는 인자도 추가할 수 없다**(H06). mutation으로 확인했다 — `--settings` 주입과 `Dir` 교체 둘 다 잡힌다.
| TOOL10 plugin·skill·hook discovery | 합성 plugin 디렉터리가 필요하다 |
| TOOL11 worktree 생성·사용·cleanup | git worktree를 만드는 세션이며 cleanup 의미가 별도 판정 대상이다 |
| CAP05·CAP07·CAP08·CAP09 | custom agent·overlay on/off·resume·Node 세션 호환. resume 두 건은 세션을 남긴 뒤 두 번째 실행이 필요하다 |

전부 `C`(capability) 등급이고 `P`/`S`는 남기지 않았다. **미실행은 통과가 아니다.**

#### 1.5.7 이번 WP에서 실행한 Node 기준선

`test-launcher-native` · `test-native-diagnostics` · `test-development-arguments` · `test-development-change-arguments` · `test-request-diagnostics` · `test-unsupported-event-diagnostics` · `test-agent-selection`

**7개 전부 exit 0.** 기준선 worktree는 tracked 변경 0으로 남아 있다.

### 1.6 G7 — 제품 빌드를 실제로 연결했고, 첫 시도는 실패했다

사용자 승인(2026-09-15). `internal/app`이 `upstream.None` 대신 `upstream.Direct`를 쓴다. **이제 이 바이너리가 시작한 모든 추론이 사용자의 Codex 구독에 도달한다.**

#### 1.6.1 연결 전에 정해야 했던 것 셋

| 문제 | 왜 그냥 연결하면 안 되는가 | 결정 |
|---|---|---|
| 예산 | `ApprovedBudget()`은 **내가 검증에 쓸 수 있는 양**이지 사용자 세션의 요청 수가 아니다. 100으로 묶으면 긴 세션이 중간에 멈춘다 | `Budget.Unrestricted` — 제품 세션 전용. 별도 필드라서 산술로 도달할 수 없고, 0값 Budget은 여전히 아무것도 허가하지 않는다 |
| 경로 고정 | `Direct`는 생성 시 model/effort에 고정된다. **제품은 클라이언트가 요청한 모델을 보내야 한다** | Unrestricted는 경로를 검사하지 않는다 |
| 지연 해석 | credential 읽기나 `codex --version`을 시작 시 하면 **ARG07이 깨진다** — `--version`이 자식 프로세스와 credential 읽기를 유발한다 | 둘 다 첫 요청에서만. `InstalledVersionFunc`가 once로 감싼다 |

#### 1.6.2 상태 코드는 Disposition을 본다 — 실측 때문이다

5.7.1절의 실측: **클라이언트는 모든 5xx를 재시도한다** (60초에 8회, 계속). 실전송이 붙은 지금 그것은 **같은 거절을 여덟 번 사는 것**이다.

| Disposition | 상태 | 이유 |
|---|---|---|
| Deferred | **429** | 서버가 시각을 말했다. 클라이언트가 두드리지 않고 기다리는 유일한 상태다 |
| Retryable | 502 | 재시도가 정말 성공할 수 있다 |
| Terminal | **400** | 재시도가 같은 답을 실제 돈 주고 다시 산다 |
| credential 계열 | 503 | 기준선과 동일(`native-gateway.mjs:557`). 사용자가 다시 로그인하면 회복되고, 소켓 이전에 실패하므로 재시도가 upstream 비용 0이다 |
| runtime·store 거부 | 400 | 이 머신의 설정이고 다시 물어도 바뀌지 않는다 |

**Terminal→400은 기준선에서 의도적으로 벗어난 것이다.** 기준선은 모든 upstream 실패를 502로 답한다. 실측이 근거이고, 범주는 메시지에 그대로 실려 무엇이 일어났는지 말한다.

#### 1.6.3 첫 실호출이 실패했고, 그것이 CAP01을 찾아냈다

```
live session: 3 attempts, 3 inferences, 0 refused
stdout: "API Error: 400 UPSTREAM_HTTP_ERROR"
```

**3 attempts.** 400으로 매핑한 덕에 8회 폭주 대신 3회에서 멈췄다 — 매핑이 설계대로 동작한 첫 증거다.

원인은 **무료로** 확정했다. fixture 세션에서 V2가 실제로 보내는 본문을 읽었다.

```
{"model":"claude-opus-5","instructions":"Follow the developer instructions...
```

**V2가 Anthropic 모델 이름을 Codex backend에 그대로 넘기고 있었다.** 그런 모델이 없으니 400이다. 이것이 `CAP01`(Claude alias → Codex model ID mapping, **P 등급**)이고 구현되어 있지 않았다.

오프라인으로는 영원히 못 찾는다. **모든 fixture가 Codex 모델 이름을 직접 적었다** — fixture를 쓴 사람이 어느 이름을 적어야 하는지 알고 있었기 때문이다. `"model":"m"`이라고 적은 것도 여럿 있었는데, 모델이 전달만 되던 때는 아무거나 되었다.

#### 1.6.4 CAP01·CAP02 — 매핑은 발명하지 않았다

`src/models.mjs`와 `src/agent-selection.mjs:23-27`에서 읽었다. **어느 모델로 도느냐가 곧 청구액이므로 추측할 자리가 아니다.**

| Claude | Codex | 기본 effort |
|---|---|---|
| `haiku`·`sonnet`·`claude-haiku-*`·`claude-sonnet-*` | `gpt-5.6-luna` | max |
| `opus`·`claude-opus-*` | `gpt-5.6-sol` | xhigh |
| `fable`·`claude-fable-*` | `gpt-6-astra` | medium |
| `terra` | `gpt-5.6-terra` | high |

가족 접두사로 맞추므로 **버전 접미사를 고정하지 않는다** — `claude-opus-5`와 `claude-opus-4-1`이 같은 경로이고, 새 릴리스에 코드 변경이 필요 없다.

**effort는 모델마다 다르고 평준화하면 안 된다.** 같게 만들면 모든 요청의 비용이 바뀐다. 테스트가 그것을 고정한다.

**CAP02**: 모르는 모델·effort는 **거부하지 기본값으로 대체하지 않는다.** 대체하면 사용자가 요청하지 않은 모델로 돌리고 그 값을 청구한다. 사용자가 보고 고칠 수 있는 실패가 청구서에서 발견하는 실패보다 낫다.

연쇄로 하나가 더 드러났다. 기준선은 `reasoning.effort`를 **무조건** 보낸다(`native-protocol.mjs:429`) — 카탈로그가 기본값을 채우므로 backend가 고를 일이 없다. V2는 없으면 생략하고 있었고, `TestAbsentEffortSendsNoReasoningParameter`가 **그 잘못된 동작을 주장**하고 있었다. fixture가 존재하지 않는 모델을 적었기 때문에 이 질문이 제기되지 않았다.

#### 1.6.6 CAP03 — 기록이 비어 있었다 (2026-09-16)

route는 요청마다 다르다. 실측: 한 번의 `claude -p`가 대화와 세션 제목을 서로 다른 모델로 보낸다. 그런데 route가 **transport의 고정 필드 두 개**였고, 제품 transport는 그 둘을 빈 문자열로 만들고 있었다(`NewDirect(..., "", "")`). 그래서 실제 세션의 모든 attempt가 **빈 route로 예약**됐다. 횟수는 셌지만 무엇에 썼는지는 남지 않았다.

route를 요청에 실었다(`upstream.Call`). gateway만이 양쪽을 안다 — 클라이언트가 부른 이름과 실제로 돌아갈 backend 모델 — 그래서 gateway가 둘을 **따로** 넘기고, ledger가 `(requested, model, effort, source)` 단위로 센다. `claude-opus-5 -> gpt-5.6-sol/xhigh (family)`. 하나로 합치면 "요청한 것으로 돌았는가"라는 질문 자체가 답할 수 없게 된다.

예산 검사는 **실제로 보낼 본문**과 대조한다. 옆에 붙은 선언만 믿으면, 그 선언이 틀린 바로 그 경우에 검사가 통과한다.

#### 1.6.7 CAP06 — 라우팅 불가를 500으로 보고하고 있었다 (2026-09-16)

`bridge.BuildRequest` 실패가 전부 **500 `REQUEST_CONVERSION_FAILED`**였다. 두 가지가 틀렸다. 사용자가 할 수 있는 일이 없고, **측정된 이 클라이언트는 모든 5xx를 재시도한다**(1분에 8회). 성공할 수 없는 요청이 여덟 번 나간다. 라우팅할 수 없는 모델은 400 `UNSUPPORTED_MODEL_OR_EFFORT`다.

CAP06이 요구하는 구분은 이것이다: correlation header(`X-Claude-Code-Session-Id`)의 **부재는 문제가 아니고**, 라우팅 불가는 호출자가 고칠 문제다. 둘이 같은 답으로 도착하면 안 된다. 경로 거부(`UNSUPPORTED_ROUTE`, 404)와도 이름이 갈린다.

#### 1.6.8 HTTP12 — 보조 서비스는 모델 요청이 아니다

기준선의 `agent-route.mjs`는 subagent binding을 **같은 gateway의** `POST /clauduct/agents`로 보낸다(`ANTHROPIC_BASE_URL` + 세션 토큰). 이 빌드는 그 엔드포인트를 구현하지 않으며, 요구사항도 구현이 아니다 — **모델 요청으로 착각되지 않는 것**이다. 실측: 404로 거부되고, backend 호출 0, attempt 0.

#### 1.6.5 실세션 — 성공

```
live session: 2 attempts, 2 inferences, 0 refused
stdout: "pineapple"
```

실제 `claude.exe` → 제품 빌드 → 실제 Codex backend → 모델의 답이 클라이언트에 도달했다. 요청한 그대로다.

**출하 바이너리로도 확인했다.** `clauduct-go -p "Reply with exactly the word: marmalade"` → `marmalade`, exit 0. 테스트가 검증한 것과 같은 코드 경로지만, 실제로 나가는 산출물을 한 번은 돌려봐야 한다.

두 실행 모두 `CLAUDE_CONFIG_DIR`를 임시로 돌렸다. 검증이 사용자의 실제 Claude 상태에 쓰는 것은 검증의 몫이 아니다.

#### 1.6.6 실세션이 내 버그를 하나 잡았다

두 번째 실행은 답을 받고도 실패했다.

```
stdout: "pineapple"
Result reported 0/0 against the ledger's 2/2
```

`Result.Attempts`가 언제나 0이었다. **이름 없는 반환값에 대한 `defer` 쓰기는 버려진다** — Go의 고전적 함정이다. 테스트가 `Result`를 ledger와 **대조**했기 때문에 잡혔다. 한쪽만 로그로 찍었다면 언제나 0을 보고하는 계수기를 출하했을 것이다.

그리고 mutation이 더 깊은 구멍을 드러냈다. ledger 읽기를 통째로 지워도 초록이었다 — **그것을 검사하는 유일한 테스트가 live였고 live는 기본 skip이다.** 오프라인 테스트를 추가했다.

#### 1.6.7 mutation — 21건, 2건이 살아남았다

| 살아남은 것 | 왜 | 조치 |
|---|---|---|
| 세션 지출이 기록되지 않음 | 검사하는 테스트가 live 하나뿐이고 기본 skip | ledger를 실제로 예약하는 fixture transport로 오프라인 테스트 추가 |
| 버전이 매 요청 해석됨 | **compiler-only였다.** `once`가 미사용이 되어 컴파일 실패 | `once.Do(func() {})`를 남겨 컴파일되게 고쳤다. 컴파일러가 거부한 것은 테스트 증거가 아니다 |

재실행: **21건 주입, 미검출 0, compiler-only 0.**

#### 1.6.8 이번에 실행한 Node 기준선

`test-agent-selection` · `test-compact-policy` · `test-completion-relay-target` · `test-fixture-tool-policy` · `test-launcher-native` · `test-native` · `test-verification-route`

**7개 전부 exit 0.** CAP01의 계약(`models.mjs`·`agent-selection.mjs`)을 가져온 파일들이다. 기준선 worktree는 tracked 변경 0으로 남아 있다.

### 1.7 G8 — 출하되는 것을 검사한다

컴파일되는 것이 아니라 **나가는 것**을 본다. 산출물은 [PACKAGING.md](PACKAGING.md)이고, 아래는 그것을 뒷받침하는 측정이다.

| ID | 상태 | 어디서 |
|---|---|---|
| REL05 재현 빌드·checksum·OS/arch | PASS | 같은 소스를 **두 번 빌드해 SHA256이 같다.** 신원은 toolchain VCS stamp에서 나오고 HEAD와 대조한다 |
| REL04 package에 secret 없음 | PASS | **두 바이너리 모두** JWT 서명(`eyJ…`)과 명시 표지로 스캔 |
| REL03 런타임 의존 | PASS + **새 사실** | Node·.NET 의존 0은 그대로. 다만 G7이 `codex.exe` 의존을 추가했다 — 아래 |
| REL06 독립 디렉터리·설치 경로 | PASS | 설치 디렉터리에 **아무것도 쓰지 않는다.** 실행 전후를 비교한다 |
| REL07 Node와 Go 동시 실행 | PASS | 아래 |
| REL09 문서 인용 검증 | PASS | Go 테스트로 옮겼다. 18 citation · 35 link · 7 문서 |
| LIFE14 연속 실행 자원 증가 | PASS | 5세션 후 goroutine 증가가 상한 내 |
| REL10 CI 분리 보고 | **PASS** | Node workflow와 별도 파일로 실행된다. Go 실패가 Node 회귀로 읽히지 않는다 |
| REL08 기본 전환·rollback | **PASS 2026-09-17** | G9 완료. `clauduct`=Go, `clauduct-node`=Node. 되돌리기는 두 파일 이름 바꾸기이고 PACKAGING.md 6장이 `.EXE`가 `.CMD`보다 먼저 풀리는 것까지 적는다. 실측: `where`가 셋을 각각 풀고, `clauduct --version`이 `2.1.273 (Claude Code)`+exit 0, `clauduct-node`는 Node 자신의 `USER_TERMINAL_REQUIRED`로 응답한다(살아 있다) |

#### 1.7.1 REL03을 정직하게 다시 적는다

REL03은 "Node adapter·.NET probe에 runtime 의존하지 않음"을 묻는다. 그 둘은 실제로 없고 스캐너가 막는다. **하지만 그것이 런타임 의존 0이라는 뜻은 아니다.**

G7이 하나를 추가했다: 제품이 `codex.exe`를 exec한다. 그 버전이 모든 요청의 header에 들어가므로 **없으면 보낼 것을 만들 수 없다.**

이 머신에는 설치돼 있으므로 그 경로를 타본 적이 없었다. resolver를 주입 가능하게 만들어 테스트했다 — **아무도 실행해본 적 없는 요구사항은 아무도 확인해본 적 없는 요구사항이다.** PACKAGING.md 2장이 셋을 전부 적는다.

#### 1.7.2 재현성이 checksum을 의미 있게 만든다

"이 commit에서 빌드했다"는 **바이너리가 자기에 대해 하는 주장**이다. 두 빌드의 해시가 같다는 것은 **누구나 확인할 수 있는 주장**이다. 후자가 없으면 공개된 checksum은 아무것도 보장하지 않는다.

같은 소스·같은 Go 버전에서 `-trimpath`로 두 번 빌드해 바이트가 같음을 확인했다.

#### 1.7.3 Node와 Go — 그리고 뜻밖의 교차 확인

```
node exit=0   go exit=0   (동시 실행)
```

Node 기준선(`clauduct.cmd --dry-run -p`)과 Go 바이너리를 동시에 돌려 둘 다 exit 0이다. 모델 호출 0회(`credentialReads: 0`, `childStarted: false`).

Go 세션끼리의 격리는 별도 테스트다 — 3개를 동시에 돌려 **각자 다른 listener를 얻고** 각자의 답을 받는다.

그리고 Node dry-run이 자기 카탈로그를 출력했다.

```
"models":{"astra":{"model":"gpt-6-astra","effort":"medium"},"sol":{"model":"gpt-5.6-sol","effort":"xhigh"},
          "terra":{"model":"gpt-5.6-terra","effort":"high"},"luna":{"model":"gpt-5.6-luna","effort":"max"}}
```

**1.6.4절에서 구현한 것과 정확히 일치한다.** 소스를 읽어서가 아니라 **돌고 있는 기준선이** 확인해준 것이다.

#### 1.7.4 mutation — 8건, 3건이 살아남았고 셋 다 다른 문제였다

| 살아남은 것 | 진짜 원인 | 조치 |
|---|---|---|
| credential이 제품에 컴파일됨 | 두 가지가 겹쳤다. **(1)** 참조되지 않은 상수는 링커가 버린다 — 주입이 바이너리를 바꾸지 않았다. **(2)** 고쳐서 실제 사용되는 상수를 바꾸자, 이번엔 **테스트가 `clauduct-go`만 스캔**하고 있었다. 그 바이너리는 `buildinfo`를 import하지도 않는다 | 주입을 현실적으로 바꾸고, **두 바이너리 모두** 스캔하게 했다. 패키지는 둘을 내보낸다 |
| commit stamp를 지어냄 | `buildinfo`에 **단위 테스트가 하나도 없었다.** 순수 함수인데도 | 직접 테스트 추가 |
| 수정된 worktree가 그 사실을 숨김 | 더 나빴다. 신원 테스트가 **`+dirty`일 때 skip**하고 있었다 — 즉 그 선언이 필요한 바로 그 상황에서 검사가 물러섰다. 주입이 `+dirty`를 지우자 테스트가 skip을 멈추고 **통과**했다 | skip을 없애고 **양방향으로 단언**한다. 바이너리가 말하는 dirty 여부와 git이 말하는 것이 일치해야 한다 |

세 번째가 이 배터리에서 가장 값진 것이다. **가장 필요한 순간에 건너뛰는 검사는 검사가 아니다.**

넓힌 스캔이 실제 적중도 하나 냈다 — `clauduct-dev`에 `BUILD-TOKEN`. probe의 fixture 값이 product 파일에 있어 정말로 출하된다. 다만 그것은 모델에게 되풀이하게 시키는 지어낸 값이고 **비밀성이 0이다.** 표지가 틀렸지 바이너리가 틀린 게 아니다. **방금 발화한 어설션을 지우는 것은 검사를 멈추는 방법이므로** 지운 이유를 코드에 적었다.

#### 1.7.5 REL09 — 검사를 스크래치 밖으로

문서 인용 검증이 재설계 내내 `.tmp`의 Python 스크립트로만 있었다. 즉 **누가 기억할 때만 돌았다.** Go 테스트로 옮겼고 이제 `go test ./...`에 포함되며 Python이 필요 없다.

### 1.8 도구 실행 — "답한다"와 "일을 한다"의 차이

여기까지의 실세션은 단어 하나를 요청하고 받았다. 그것은 텍스트 경로의 증거이지 **이 bridge로 일을 할 수 있다는 증거가 아니다.** 도구 wire는 probe로 backend에 대조했고 barrier는 fixture로 검증했지만, **클라이언트가 호출을 실행하고 결과를 돌려보내는 것은 한 번도 일어난 적이 없었다.**

모델 호출 0회로 했다. backend를 스크립트로 두면 각 테스트가 필요한 정확한 호출을 만들 수 있고, **사용자가 거부한 호출**처럼 실제 모델이 요청해주지 않을 것도 만들 수 있다.

| ID | 상태 | 증거 |
|---|---|---|
| TOOL01 실제 왕복과 fixture 실제 변화 | **PASS** | 실제 클라이언트가 Write를 실행했고 **파일이 디스크에 생겼다.** 결과가 3번째 요청(79KB)으로 돌아갔다 |
| TOOL02 permission 거부가 실행으로 안 바뀜 | **PASS** | backend가 요청해도 허용 목록 밖 도구는 파일을 만들지 않는다 |
| TOOL06 전달 후 실패 시 자동 재실행 0 | **PASS** | 아래 |

#### 1.8.1 fixture가 순서로 답하면 안 된다

첫 시도가 실패했다. 도구가 안 돌고 요청이 3번 왔다. **추측 대신 무엇이 오갔는지 봤다.**

```
request 1:  3,780 bytes  tools=0   ← "Return JSON with a single title field"
request 2: 78,568 bytes  tools=24  ← 진짜 대화
request 3:  3,780 bytes  tools=0
```

**요청 1은 사용자의 턴이 아니다** — 클라이언트가 세션 제목을 따로 생성한다. 순서대로 답하는 스크립트가 **도구 턴을 제목 요청에 줘버렸다.**

`upstream.Script`를 내용으로 짝짓게 고쳤다. **클라이언트가 요청을 다중화하므로 backend를 대신하는 fixture도 그래야 한다.**

#### 1.8.2 세션은 사용자가 치지 않은 요청을 보낸다

도구 세션 한 번이 **3 요청**이다 — 대화 2, 사용자가 치지 않은 것 1. 그 side 요청도 같은 모델·같은 effort·같은 구독으로 간다.

단어 하나짜리 `-p`는 side 요청이 **0개**였다. 즉 세션당 고정 추가 비용이 아니고 **턴 수로 예측되지 않는다.** 그것이 요점이다 — **사용자의 턴만 세는 비용 주장은 틀렸다.**

클라이언트의 행동이지 이 bridge의 것이 아니며 기준선도 같게 라우팅한다. 이 bridge의 몫은 하나다: side 요청도 **라우팅되지 그대로 전달되지 않는다.** 전달했다면 G7이 찾은 바로 그 결함이다.

#### 1.8.3 TOOL06은 요청 재시도가 아니라 도구 재실행에 관한 것이다

결과를 실은 턴을 backend가 `response.failed`로 답하게 했더니 요청이 4번 왔고 **3번과 4번이 바이트 단위로 동일했다.** 클라이언트가 실패한 요청을 재시도한 것이지 도구를 다시 실행한 게 아니다.

처음엔 요청 수로 단언했는데 **틀린 것을 재고 있었다.** 파일도 답이 될 수 없다 — Write는 덮어쓰므로 두 번 실행해도 바이트가 같다. 구분하는 것은 **대화**다: 재실행이었다면 두 번째 `function_call`과 두 번째 output이 생긴다. 결과를 실은 요청들이 서로 동일한지로 단언한다.

#### 1.8.4 CI 첫 실행이 로컬에서 절대 못 찾을 것을 잡았다

`gofmt -l .`이 **모든 파일**을 플래그했다. 포맷이 아니라 **줄바꿈**이다.

`core.autocrlf=true`이고 `.gitattributes`가 없어, Windows 체크아웃마다 `.go`가 CRLF가 된다. **gofmt는 CRLF를 미포맷으로 본다.** 로컬 파일은 LF라서 통과했고, git이 다시 체크아웃하기 전까지는 영원히 통과했을 것이다.

`go/.gitattributes`에 `*.go text eol=lf`를 넣었다. **저장소 루트가 아니라 `go/` 하위에만** — Node 기준선의 체크아웃 동작은 이 브랜치가 바꿀 것이 아니다.

#### 1.8.6 CI 2차 — 이번엔 내 테스트의 가정이 틀렸다

`gofmt`는 통과했다. `TestTheInstallDirectoryIsNotWrittenTo`가 실패했다 — runner에 `claude.exe`가 없어 `CLAUDE_NOT_FOUND`로 exit 1인데 테스트가 exit 0을 요구했다.

**exit 코드는 애초에 질문이 아니었다.** 검사 대상은 "바이너리를 실행하면 설치 디렉터리에 아무것도 쓰지 않는다"이고, 그것은 클라이언트 유무와 무관하다 — 오히려 **없을 때 더 강한 검사다.** 출력이 전혀 없을 때만 실패하도록 고쳤다. 그것이 "실행되지 않았다"의 실제 모습이다.

3차에서 **전부 통과**했고, 그것이 `-race`와 REL10을 동시에 닫았다.

#### 1.8.7 `go test -race` — 처음으로 실행됐다

```
Run go test -count=1 -race ./...   CGO_ENABLED: 1
12 package 통과
```

registry·ledger·gateway의 동시성이 **한 번도 race 검사를 받은 적이 없었다.** 이 머신에는 C 툴체인이 없어 로컬에서는 여전히 불가능하고, 앞으로도 CI가 담당한다.

#### 1.8.5 `Run`이 context를 받아놓고 쓰지 않고 있었다

가장 무거운 결함이고, mutation이 스크립트를 무한 반복으로 바꾸자 드러났다. **suite가 25초 상한에 대해 7분을 넘겼다.**

`Run(ctx, ...)`이 `ctx`를 **한 번도 보지 않았다.** `process.Wait()`가 무조건 막혔다. 즉 **호출자가 포기해도 세션을 끝낼 수 없었고**, 이 package의 모든 테스트 timeout이 장식이었다. 제품에서는 `main`이 `context.Background`를 넘기므로 아무도 눈치채지 못했다.

고친 뒤 측정: 6초 상한에 **242 요청**이 나갔다. 실제 backend였다면 6초에 242회 과금이다.

| 항목 | 내용 |
|---|---|
| 중단 범위 | **내가 시작한 프로세스 핸들로만.** 이름을 찾지도 열거하지도 않으므로 다른 사람의 Claude·MCP 프로세스에 닿을 수 없다 |
| 명시한 한계 | Windows에서 **손자 프로세스는 같이 죽지 않는다.** 트리를 묶으려면 Job Object가 필요하고 그것이 LIFE11이며 **아직 안 했다** |
| 오류 구분 | 죽인 프로세스의 `Wait()`는 `ExitError`를 낸다. 그것을 반환하면 **호출자가 끝낸 세션이 자식이 끝낸 세션처럼 보인다.** 버리고 `ctx.Err()`를 반환한다 |

부수 효과가 하나 있다. context가 자식에 닿으니 **무한 루프가 멈춤이 아니라 실패가 된다** — 앞서 `HUNG`으로 셌던 mutation이 이제 단언으로 잡힌다.

### 1.9 프로세스 경계 — 소유한 것과 남는 것

핸드오프가 이 셋을 어떻게 다루라고 했는지가 먼저다(1085–1087, 1101행): **Job Object 등을 검토해 이 실행이 소유한 트리만 정리하고, 지원이 불가능한 조합은 정확히 보고하되 정책 우회로 해결하지 않으며, child `Wait`만으로 손자가 종료됐다고 단정하지 않는다.**

그래서 측정하고 보고했다. 이름에 Job Object가 들어 있다고 넣은 것이 아니다.

**모든 테스트가 자기가 띄운 프로세스만, PID로만 다룬다.** 이 머신에는 작성 중에도 사용자의 claude.exe가 셋 떠 있었고 그중 하나가 작성하던 세션이다. **이름으로 죽이는 테스트였다면 셋 다 죽였다.**

| ID | 상태 | 측정 |
|---|---|---|
| LIFE12 다른 프로세스 불간섭 | **PASS** | 미끼 2개가 세션 종료 후에도 살아 있다. 자기 자식은 끝난다 |
| LIFE11 손자 정리 | **PASS(한계 기록)** | 손자 **1/1이 살아남았다.** 아래 |
| LIFE17 부모 비정상 종료 | **PASS(한계 기록)** | launcher를 죽이면 claude.exe 자식이 **1/1 살아남는다** |

#### 1.9.1 한계를 단언으로 적었다

LIFE11 테스트는 **정리가 실패하는 것을 단언한다.** 일부러다. Windows에서 프로세스를 죽이면 그 프로세스만 죽고 자식은 핸들에 없다. 기준선도 같다 — `clauduct.mjs:261`이 맨 `child.kill()` 하나다. **그렇지 않은 척하는 테스트는 검사가 아니라 주장이다.**

트리가 함께 죽게 되면 이 테스트가 **실패한다.** 그때 다시 쓰는 것이 작업이다.

#### 1.9.2 Job Object를 넣지 않기로 한 이유

검토는 했다. 넣지 않는다.

| 근거 | 내용 |
|---|---|
| 제품이 그 경로를 타지 않는다 | `Stop()`은 **context가 취소될 때만** 실행되고, `main`은 `context.Background`를 넘긴다. 오늘 제품에서 호출되는 일이 없다 |
| 새 의존과 새 실패 양식 | Job Object는 syscall wrapper를, `taskkill /T`는 외부 프로세스를 끌어들인다. **아무도 타지 않는 경로를 위해** |
| 명세가 그렇게 말한다 | "지원이 불가능한 조합은 정확히 보고하고 **정책 우회로 해결하지 않는다**" |

**다시 볼 조건**: `main`이 신호 처리를 갖게 되어(Ctrl+C → cancel) `Stop()`이 실제로 도는 경로가 생기면, 그때 트리 문제가 살아난다.

`--bg`도 고려했다. kill-on-close job이면 `clauduct-go --bg`가 시작한 백그라운드 세션을 launcher 종료와 함께 죽인다. 다만 `--bg`는 claude.exe가 곧바로 끝나므로 `Stop()`이 아예 호출되지 않는다 — 오늘은 무관하고, Job Object를 넣는다면 그때 실재하는 비용이다.

#### 1.9.3 mutation이 셋을 드러냈고 둘은 내 테스트가 아니라 제품 문제였다

**(1) 테스트가 제품의 정리 경로를 타지 않고 있었다.** LIFE11·LIFE12가 `Process` 스텁을 주입했는데 **스텁은 자기 `Stop`을 가져온다.** 그래서 `osProcess.Stop`을 "핸들 밖으로 손을 뻗도록" 바꾼 변이와 "트리 전체를 죽이도록" 바꾼 변이가 **둘 다 살아남았고 테스트는 통과했다.** 실제 spawn 경로를 타도록 고쳤다 — 무해한 sleeper를 자식으로 띄운다.

**(2) `waitFor`가 Stop 실패에 영원히 막혔다.** `Stop()`을 no-op으로 바꾸는 변이가 suite를 **멈추게** 했다. 원인은 Stop 뒤의 무조건 `<-done`이다. **Stop이 듣지 않는 자식이 호출자의 마감을 무력화한다** — 권한 거부, 보안 제품 개입, 핸들이 더 이상 그것을 통제하지 못하는 경우. `stopGrace` 5초를 두고 넘기면 **기다리는 대신 말한다.**

**(3) 멈춤은 실패가 아니다.** "context를 다시 무시" 변이가 여전히 HUNG이었다. `runOwning`에 watchdog을 넣어 **단언으로 바꿨다** — 이제 `Run did not return within 33s. The child never exits on its own, so the context is not reaching it.`로 실패한다.

재실행: **4건 주입, 미검출 0, HUNG 0.**

### 1.10 취소 — 세션을 끝내는 것이 모델이 아닐 때

#### 1.10.1 Ctrl+C를 쏘지 않는다

이 절의 어떤 테스트도 console control event를 만들지 않는다. Windows에서 Ctrl+C와 close event는 **콘솔에 붙은 모든 프로세스**로 가지 선택한 하나로 가지 않는다. 이 머신에는 사용자의 다른 세션이 돌고 있었고, 신호를 쏘는 테스트는 **LIFE12가 막으려는 실패를 다른 문으로** 일으킨다.

| ID | 상태 | 어떻게 |
|---|---|---|
| LIFE04 Ctrl+C가 자식에 닿음 | **PASS(구조)** | `CREATE_NEW_PROCESS_GROUP` **부재**를 실제로 만들어진 command에서 단언한다. 그 flag가 붙는 순간 console Ctrl+C가 자식에 닿지 않고, 이 launcher는 신호를 전달할 자체 처리가 없다 |
| LIFE05 stdin 종료 | **PASS** | 입력도 프롬프트도 없으면 클라이언트가 그렇게 말하고 끝낸다. **Run이 오류를 내지 않는다** — 클라이언트가 끝낸 세션이지 bridge가 실패한 것이 아니다 |
| LIFE05 취소와의 구분 | **PASS** | 아래 |
| LIFE05 console 종료 | **`NOT_RUN`** | 한 프로세스에 보낼 방법이 없다. **미실행은 통과가 아니다.** 돌릴 조건: 다른 세션이 없는 머신, 또는 자체 콘솔을 가진 자식 — 후자는 LIFE04가 의존하는 Ctrl+C 동작 자체를 바꾼다 |

#### 1.10.2 테스트가 다른 이유로 통과하고 있었다

취소 구분 테스트가 통과하고 있었는데, **`ctx.Err()`를 `nil`로 바꾸는 변이에도 통과했다.**

손으로 재현했더니 7초가 걸렸다 — 마감 2초 + `stopGrace` 5초. `ctx.Err()` 경로가 아니라 **"자식이 안 나갔다" 경로**를 타고 있었고, 그 메시지가 `%w`로 마감을 감싸고 있어 `Contains` 단언이 그대로 통과했다.

원인은 제품 사실이다. **`cmd.Wait()`는 손자가 물려받은 파이프를 닫을 때까지 돌아오지 않는다.** `cmd /c ping`에서 `cmd`를 죽여도 `ping`이 stdout을 쥐고 있다. **1.9절이 프로세스 표에서 기록한 한계가 wait를 통해 다시 나타난 것이다.**

둘로 갈랐다.

| 경우 | 결과 |
|---|---|
| 자식에게 자식이 없음(`ping` 직접) | **2.09초**, 오류가 **정확히** `context deadline exceeded` |
| 자식에게 자식이 있음(`cmd /c ping`) | **7.001초**, `did not exit ... after` — 취소된 세션이 grace를 꽉 채운다 |

단언도 `Contains`에서 **정확한 일치**로 바꿨다. 감싼 오류를 포함으로 재면 두 경로를 구분하지 못한다.

#### 1.10.3 `-race`가 제품 race를 잡았다

CI에서 처음으로 무언가를 잡았다. `TestAChildThatWillNotStopIsReportedRatherThanWaitedOn`에서 **DATA RACE 셋**이고 둘은 제품이다.

```
Read  at ... osProcess.ExitCode()  run.go:275   (Run)
Write at ... os/exec.(*Cmd).Wait()              (buried goroutine)
```

grace 경로로 빠져나오면 **`Wait()`가 아직 돌고 있다.** 그 상태에서 `Run`이 `ExitCode()`를 읽으면 `ProcessState`를 쓰는 중인 goroutine과 경쟁한다. 로컬에서는 보이지 않는다 — 이 머신에 C 툴체인이 없어 `-race`를 돌릴 수 없다.

고쳤다. `waitFor`가 **reap 여부를 함께 반환**하고, reap하지 못했으면 `Run`이 exit 코드를 **읽지 않는다.** 값은 `ExitCodeUnknown`(-1)이다 — **0은 성공한 세션처럼 읽힌다.**

셋째는 테스트가 같은 `Cmd`에 `Wait()`를 중복 호출한 것이다. 제거했다.

그리고 `-race` 없이도 서는 단언을 남겼다: reap 못 한 세션의 `NativeExitCode`는 `ExitCodeUnknown`이어야 한다.

#### 1.10.4 mutation — 4건, 전부 잡힌다

`CREATE_NEW_PROCESS_GROUP` 주입 · 클라이언트 exit 코드를 launcher 오류로 바꾸기 · exit 코드를 보고하지 않기 · 취소가 오류를 내지 않기. **console event를 쏘는 변이는 넣지 않았다.**

exit-코드 변이는 처음에 compiler-only였다 — `errors.As`를 지우면 `exitErr`가 미사용이 된다. 컴파일되는 형태로 고쳤다. **컴파일러가 거부한 것은 테스트 증거가 아니다.**

### 1.11 2026-09-17 — 재확인과, 확인이 찾아낸 것

세 가지를 물었다. 클라이언트가 올라갔는데 계약이 아직 맞는가, 원장이 말하는 것이 코드에도 있는가,
성능은 얼마인가. 전부 추론 0회로 답했고 **그 과정에서 조용한 결함 하나를 찾았다.**

#### 클라이언트 2.1.274 — 계약은 그대로다

원장의 wire 측정은 전부 2.1.272/273에 대한 것이었고, 설치된 클라이언트는 **2.1.274**다.
`internal/app` 98개 테스트가 실제 `claude.exe`를 띄워 게이트웨이를 통과시키며, 3건만 skip됐다
(live 1, 수동 2). 요청 top-level 필드 집합은 **닫혀 있으므로**, 새 필드가 하나라도 늘었다면
`REQUEST_FIELDS` 400으로 전부 실패했을 것이다. 통과는 곧 필드·베타·헤더 계약이 2.1.274에서
유지된다는 뜻이다. 재측정 비용 0원.

**미실행은 통과가 아니다.** 이 실행이 말하지 않는 것: 실백엔드 추론 경로(offline fixture다),
그리고 backend가 내보내는 SSE 이벤트 이름(그것은 클라이언트 버전과 무관하다).

#### 구조화 출력이 검증만 되고 버려지고 있었다 — 고쳤다

`decodeOutputConfig`는 `output_config.format`의 모양·`json_schema` 타입·schema·name을 전부
검사하고 **아무 데도 싣지 않았다.** `anthropic.Request`에 담을 자리가 없었고 `BuildRequest`는
`text.format`을 만들지 않았다. 기준선은 `native-protocol.mjs:429`에서
`text: { format: { type, name, schema, strict: true } }`로 **보낸다.**

이것이 최악의 실패 모양이다. 요청은 200으로 성공하고, 클라이언트는 제약이 걸렸다고 믿고,
모델은 그 제약을 들은 적이 없다. 돌아오는 것은 스키마를 만족하지 않는 산문이고, 그것을 파싱하는
쪽(workflow agent, 결과를 구조로 받는 서브에이전트)에서 **원인에서 한참 떨어진 자리에** 오류가 난다.

기준선과 같은 계약으로 연결했다. 이름 없는 스키마의 기본값도 기준선의 `structured_output`이다 —
다른 기본값을 쓰면 같은 클라이언트의 같은 요청이 어느 빌드가 받았느냐에 따라 다른 스키마로 보인다.

돌연변이 3건 중 **1건이 처음에 살아남았다.** 기본 이름 검사를 `anthropic.DefaultSchemaName`으로
썼더니 상수를 바꿔도 테스트가 따라 움직였다 — 테스트가 자기 자신과 동의한 세 번째 사례다.
리터럴로 고정한 뒤 잡힌다.

#### CGO — 지금도 꺼져 있지만, 아무도 그것을 강제하지 않고 있었다

출하 바이너리는 `CGO_ENABLED=0`으로 기록돼 있다(`go version -m`). 그러나 그것은 **이 머신에 C
툴체인이 없어서**이지 결정이 아니었다. `CGO_ENABLED`는 툴체인이 있는 곳에서 1로 기본값을 잡고,
CI의 `windows-latest`에는 gcc가 있다. 즉 CI는 **출하되는 것과 다른 바이너리를 빌드·테스트**하고
있었고, 재현성 테스트는 같은 환경에서 두 번 빌드해 비교하므로 이 차이를 구조적으로 볼 수 없다.

`TestTheShippedBinaryIsBuiltWithoutCgo`가 산출물의 build info를 읽어 고정한다. 테스트는 값을
직접 설정하지 않고 **환경을 상속한다** — 스스로 핀을 박고 그 핀을 검사하는 테스트는 자기 자신과만
동의하기 때문이다. `CGO_ENABLED=1`로 돌리면 실패한다(확인함). CI와 PACKAGING의 빌드 명령에
핀을 넣었다.

이 모듈에는 `import "C"`가 없고 Windows의 `net`·`os/user`는 어느 쪽이든 syscall을 쓰므로
**동작은 바뀌지 않는다.** 바뀌는 것은 산출물이 빌드 머신에 의존하지 않는다는 점이다.

#### 성능 — 시작 비용과 메모리

| 측정 | 값 |
|---|---|
| `clauduct.exe --version` 전체 | 150–159 ms (5회) |
| 그중 클라이언트 몫 | `claude --version` 단독 44–46 ms |
| 옵션이 붙으면 클라이언트가 느려진다 | `--agents` 또는 `--append-system-prompt`가 하나라도 붙으면 134–141 ms |
| **런처 자신의 몫** | **≈ 17 ms** (153 − 136) |
| 런처 peak working set | **12.3 MB** (30회 샘플링) |
| Node 기준선 런처 | **44.6 MB** (node 자체 바닥값이 ~45 MB) |
| 클라이언트 | 680 MB |

**+90 ms는 위임 메뉴 탓이 아니다.** 에이전트 1개와 17개가 같고, 같은 크기의
`--append-system-prompt`도 같다 — 옵션이 붙는 순간 클라이언트가 `--version` 지름길에서 벗어나는
비용이다. 실제 세션은 어차피 전체 초기화를 하므로 이 90 ms는 `--version`·`--help`류에만 보인다.
메뉴를 줄여도 돌아오지 않는다.

**v1과의 같은 경로 비교는 성립하지 않았다.** Node 기준선은 TTY가 없으면 자식을 띄우지 않고
`USER_TERMINAL_REQUIRED`로 끝난다(측정함). 즉 헤드리스에서 v1은 런처 비용을 낼 기회조차 없다.
요청당 지연·처리량 비교는 두 구현이 같은 fixture backend를 보게 하는 harness가 있어야 하고,
그것은 아직 없다.

#### PDF — 물어보고 나서 구현했다

`document` 블록은 이 빌드에서 `UNSUPPORTED_CONTENT`였고, 그것은 Read가 PDF를 열 때마다 턴이
죽는다는 뜻이었다. 구현 여부를 결정하기 전에 **백엔드가 파일을 읽는지**를 직접 물었다:
`clauduct-dev probe file --send`가 생성한 1페이지 PDF를 `input_file` 데이터 URL로 보내고,
모델이 **PDF 안에만 있던 토큰**을 돌려줬다. 그 뒤 제품 경로(document 블록 → 디코더 → bridge)로
같은 확인을 한 번 더 했다.

**probe의 첫 두 실행은 틀린 답을 출력했다.** 토큰을 `translate()`가 돌려주는 exchange에서 찾았는데
그 버퍼는 EOF에서 비워진다 — 성공할 수 없는 검사였고, 그 결과를 백엔드에 대한 사실로 적었다.
스트림에서 직접 찾도록 고치고, 읽기 경계에 걸친 토큰을 잡는 단위 테스트를 먼저 통과시킨 뒤 다시
쐈다. 실호출 3회 중 2회가 이 결함의 값이다.

media_type은 `application/pdf` 하나만 받는다. 참조 Codex 클라이언트는 `input_file`을 보내지
않으므로 다른 타입에는 근거가 없다. 돌연변이 3건 전부 잡힌다.

#### count_tokens — 부르지 않았다

실세션 한 번(`clauduct -p`)의 요청 전수: `HEAD /api/hello` · `GET /v1/models` ·
`POST /v1/messages` 2건. **count_tokens는 없다.** 클라이언트 바이너리에는 그 경로가 있고
(`source:"count_tokens"`, `maxRetries:1`) 실패 경로도 있다(`count_tokens_unreachable`, "estimates
and may differ from actual usage"). Bedrock upstream에는 클라이언트가 스스로 501을 만들어 로컬
추정기로 넘긴다.

**구현하지 않는다.** 정직한 답에는 Codex 모델용 토크나이저가 필요하고, 추정치를 API 응답으로
돌려주면 클라이언트는 그것을 측정값으로 취급한다 — 지금은 스스로 추정하고 있다는 것을 안다.
남은 미측정: 대화형 세션. `-p`만 관측했다.

같은 실행이 세 가지를 덤으로 줬다. **클라이언트가 `STRUCTURED_OUTPUTS` 베타를 보낸다**(계정의
`betas.judged`) — 오늘 고친 구조화 출력이 가설이 아니라는 뜻이다. rate limit 관측이 실제로 채워진다
(`partial`, activeLimit `premium`, primary 46%/10080분). 그리고 요청별 지연이 기록된다: 첫 바이트
1,449 ms와 3,669 ms, 세션 8.7초.

#### WebSearch — 실백엔드 재확인

`clauduct-dev probe search --send`: 32,060 bytes, 20 links over 14 hosts, 10,149 chars, 11 frames.
추론 ledger 0. 클라이언트가 보낼 수 있는 hosted 도구는 `web_search` 하나뿐이다 — 2.1.274
바이너리에 `web_fetch`·`code_execution`·`computer`·`text_editor`·`memory` 타입 이름이 없다.

**남은 구멍 하나**: 실제 클라이언트 세션이 WebSearch를 일으키는 NATIVE_SYNTH 테스트가 없다.
검증은 bridge 단위와 실백엔드 probe까지이고, 그 사이의 클라이언트 경로는 비어 있다.

### 1.12 NATIVE_SYNTH — 비어 있던 세 칸을 채웠다 (2026-09-17)

"한 층씩은 봤지만 사용자가 쓰는 경로는 아무도 안 돌렸다"가 세 군데 있었다. 전부 실제
`claude.exe` 2.1.274 + 스크립트 백엔드이고, **모델 호출 0원**이다.

| 테스트 | 무엇을 처음으로 측정했나 | 결과 |
|---|---|---|
| `TestReadingAPDFCarriesItToTheBackend` | 실제 Read가 PDF를 **document 블록으로 돌려주고**, 이 빌드가 그것을 `input_file`로 싣는다 | PASS. tool_result를 나르는 요청이 78,800 → 80,231 바이트로 커진다 |
| `TestAWebSearchRoundTripsThroughTheBridge` | 클라이언트가 side query를 내고, 게이트웨이가 가로채고, **합성한 블록이 다음 턴에 돌아온다** | PASS. 링크 제목과 URL이 tool_result 요청 안에 있다 |
| `TestAWorkflowsAgentsReachTheBridge` | Workflow 호출이 실행되고, 그 에이전트가 **요청으로 여기 돌아온다** | PASS. spawner=`gpt-6-astra/low`, agent=`gpt-6-astra/low` (부모 상속), `unregistered=0 unrouted=0` |

세 가지가 새로 확인됐다.

**workflow 에이전트는 툴 수로 식별된다.** 세션은 24개를 들고 오고 workflow 에이전트는 20개를
들고 온다(Workflow 자신이 빠진다). 순서로 고르면 side request 하나에 어긋난다 — 서브에이전트
테스트가 같은 이유로 같은 규칙을 쓴다.

**부모 상속이 파일을 안 읽고도 성립한다.** 기준선은 저널을 읽고 검증해서 `selectModel(parentRoute)`에
도달한다. 이 빌드는 "역할에 route가 없으면 클라이언트가 고른 모델을 유지한다"는 기본값으로 같은
결과에 이른다. C4를 구현하지 않기로 한 판단이 이제 추론이 아니라 테스트다.

**계정이 조용하다는 것도 단언한다.** `unrouted=0`이 없으면 workflow를 쓴 모든 세션이 "보고할 게
있는 세션"이 된다. 돌연변이로 확인: `buildHook`을 빼면 `unregistered=1`로 실패한다 — 즉 이 테스트는
hook 경로를 실제로 재고 있다. PDF 쪽도 `case "document"`를 지우면 실패한다.

**아직 미측정**: 대화형(TUI) 세션. 이 환경에서 클라이언트에 pty를 줄 수 없어(샌드박스가 거부)
`-p`만 관측했다. count_tokens가 대화형에서 불리는지는 그래서 여전히 열려 있다.

### 1.13 버전을 고정하지 않으면서 드리프트를 보는 법 (2026-09-17)

클라이언트와 Codex CLI는 계속 갱신된다. 이 빌드는 **어느 쪽 버전도 거부하지 않고 앞으로도 그럴
것이다** — 한 버전에서만 도는 브리지는 결함이 아니라 설계로 깨진 것이다. 대신 두 가지를 더한다.

**관측한 버전을 계정에 적는다.** 클라이언트가 매 요청 User-Agent에 자기 이름을 싣으므로 프로세스를
하나 더 띄우지 않고도 알 수 있다. 다만 그 모양은 **한 가지가 아니다** — 실측:

```
/api/hello    Bun/1.4.3
/v1/models    claude-code/2.1.274
/v1/messages  claude-cli/2.1.274 (external, sdk-cli)
```

첫 판본은 `^claude-cli/<버전>$`로 썼고, 그래서 **아무것과도 맞지 않아 계정의 version이 빈 채로**
나왔다. 실세션 계정을 읽다가 찾았다 — 이 필드가 존재하는 이유가 바로 그것이라는 점에서, 기능이
자기 자신을 한 번 증명한 셈이다. 지금은 두 제품명과 접미사를 모두 받고, 실측 문자열 세 개가
테스트에 리터럴로 박혀 있다. 게이트웨이가 처음 한 번만 기록하고(세션에 클라이언트는
하나다), 계정의 `gateway.client`가 관측값·기준값(`ReferenceClient`)·일치 여부를 낸다. 준비 probe는
`Bun/1.4.3`로 오므로 그것은 기록하지 않는다 — 이 계정은 파일로 남고 세션보다 오래 산다.

**일치하지 않아도 세션을 막지 않는다.** `verified: false`는 고장이 아니라 "클라이언트가 움직였고
아직 아무도 다시 재지 않았다"는 뜻이고, 업데이트 직후 깨진 세션이 넘겨줄 수 있는 유일한 사실이다.

**미지의 입력은 계속 크게 실패한다(사용자 결정).** 새 top-level 필드는 `REQUEST_FIELDS <이름>`으로
거부되고 이름이 계정에 남는다. 오늘 찾은 최악의 결함이 "조용히 무시"였으므로 fail-open은 택하지
않았다.

### 1.14 `clauduct --update` — 태그를 기준으로, 확인을 받고 (2026-09-17)

사용자 결정 둘: 기준은 **태그 릴리스 자산**, 적용은 **보여주고 확인**.

`--update`는 이 런처가 소유하는 **유일한** 옵션이고, **첫 인자일 때만** 인식한다. 전체 argv를
훑으면 `clauduct -p "how do I --update this"`가 바이너리 교체가 된다 — 프롬프트도 인자이기
때문이다. 거부 목록은 과잉 매칭을 감당할 수 있지만(거부하고 사용자가 고쳐 쓴다) 이쪽은 **파일을
쓴다.** 이름 자체는 아무것도 가리지 않는다: 클라이언트는 `update`를 bare 서브커맨드로 쓰고
`--update` 옵션은 없다(2.1.274 실측). 그래서 `clauduct update`는 여전히 클라이언트를 갱신한다.

검증 순서가 안전의 전부다 — 메타데이터 → `SHA256SUMS` → **태그와 digest 3개를 출력하고 확인** →
전부 내려받아 검증 → 그 다음 교체. 테스트가 고정하는 것: digest 불일치는 아무것도 쓰지 않고
"손대지 않았다"고 말한다, 릴리스 API digest와 `SHA256SUMS`가 **어긋나면 멈춘다**, 동의 없이는
내려받지도 않는다, **EOF는 동의가 아니다**, 셋 중 하나라도 실패하면 옮긴 것을 되돌린다,
바이너리가 없는 릴리스는 무엇이 없는지 이름으로 말한다.

**실제 엔드포인트에서 하나 배웠다.** 인증 없는 GitHub API는 주소당 시간당 60회이고, 실행해 보니
바로 `403 rate limit exceeded`였다. 첫 판본은 그것을 "no release to update from"이라고 말했다 —
멀쩡히 있는 릴리스를 찾아 헤매게 만드는 문장이다. `RATE_LIMITED`로 이름 붙이고 재시도 시각을 함께
낸다. private 저장소의 403과 구분하기 위해 `x-ratelimit-remaining: 0`을 함께 본다.

**아직 못 한 것**: 실제 태그에 대고 끝까지 돌려본 적이 없다. 공개된 `v0.1.0`은 Node zip을 싣고
있어 Go 바이너리 3개와 그 `SHA256SUMS`가 없기 때문이다. 전 경로는 stand-in 서버로 검증했고, 실제
API 경로는 레이트 리밋까지만 닿았다. 닫으려면 **태그를 만들고 릴리스를 발행**해야 하며 그것은
별도 승인 사항이다.

### 1.15 사용량 — 클라이언트가 못 보여주는 것을, 이미 읽고 있었다 (2026-09-17)

**질문**: `/usage`·`/cost`를 쓰면 GPT 사용량과 주간 한도가 보이는가.

**측정 1 — 클라이언트는 우리에게 묻지 않는다.** 내가 통제하는 리스너에 클라이언트를 붙이고
`/usage`를 두 자격증명 모양으로 돌렸다(모델 호출 0원).

| 조건 | 요청한 경로 | `/api/oauth/usage` |
|---|---|---|
| Clauduct와 같은 모양(`ANTHROPIC_AUTH_TOKEN`, OAuth 비움) | `/api/hello`, `/v1/messages` | **없음** |
| OAuth 모양(`CLAUDE_CODE_OAUTH_TOKEN` 설정) | 같음 | **없음** |

바이너리의 해당 코드가 `if(!St()||!Jd())return{}`로 시작한다. 커스텀 base URL에는 계정 데이터를
묻지 않는 것으로 보이고, 클라이언트 입장에서 옳은 설계다. **따라서 게이트웨이에
`/api/oauth/usage`를 구현하면 아무도 부르지 않는 죽은 코드가 된다.** 구현하기 전에 물어봐서
알았다.

**측정 2 — 토큰 수는 진짜, 금액은 무의미.** 브리지는 백엔드가 센 `input_tokens`/`output_tokens`를
`message_delta`에 싣고, 백엔드가 말하지 않은 값은 0으로 쓰지 않고 **생략**한다. 달러는 클라이언트가
자기 가격표에서 모델 이름으로 찾는데 `gpt-*`가 없다. 유일한 손잡이인 `modelPicker.behavesAs`는
Opus 요금으로 **확신에 찬 틀린 금액**을 만들므로 쓰지 않는다(그리고 이전 측정상 effort까지 끌고 간다).

**그래서 만든 것.** 숫자는 이미 있었다 — 매 응답 헤더의 `x-codex-primary-*`를 D5가 읽어 모든 세션
계정에 적고 있었다. 없던 것은 볼 자리뿐이다.

- `clauduct --usage` = `clauduct-dev usage`: 주간/보조 창의 사용률·리셋까지 남은 시간·in force
  family·읽지 못한 다른 family, 그리고 **그 읽은 값이 얼마나 오래됐는지**. 요청 0회 — 세션이 남긴
  계정 파일에서 읽는다. 낡은 값을 현재로 제시하는 것이 값이 없는 것보다 나쁘므로 나이를 함께 적는다.
- 모든 세션 **종료 줄의 `quota=47%/7d`**. 한 조각만, 묻지 않아도 보이게.

**옵션은 첫 인자일 때만 인식한다.** `--update`와 같은 규율이고 이유도 같다 — 프롬프트도 인자라서
`clauduct -p "what does --usage show"`가 세션 대신 계정을 찍으면 안 된다. 실세션으로 확인했다:
그 프롬프트는 요청 4건짜리 정상 세션으로 돌았다. 클라이언트에는 `--usage` 옵션이 없다(실측).

돌연변이 4건 전부 잡힘 — 한도 없는 계정을 집기, 주 단위를 분으로 출력, 종료 줄에서 quota 제거,
옵션을 argv 어디서나 매칭.

**주의로 남기는 것**: 이 수치는 **계정 단위**다. 다른 머신과 Codex CLI 자신이 같은 한도를 쓴다.
출력에 그 문장을 넣었다.

## 2. 네 단계 실행 강도

| Level | 내용 | 실모델 호출 |
|---|---|---|
| `OFFLINE` | Go unit, fake child, fake upstream, golden·property tests | 0 |
| `NATIVE_SYNTH` | 실제 native Claude + 임시 synthetic profile + 가짜 backend | 0으로 계측·보장 |
| `LIVE_BUDGETED` | 승인된 실제 backend, 제한된 scenario | 명시된 cap 이내 |
| `USER_OBSERVED` | 사용자 운영 중 제보 | 개발 suite PASS와 구분 |

`NATIVE_SYNTH`는 외부 연결을 만드는 hook·plugin·auto-update·기본 auth 사용 여부를 확인한 **격리 fixture profile**에서 실행한다. 사용자의 실제 프로필을 통째로 복사하지 않는다. 정상 사용자 프로필 호환은 후속 승인 범위로 남긴다.

실호출 0 단계에서는 DNS/socket·HTTP instrumentation으로 **실제 provider 호출이 0임을 입증**한다. "mock을 썼으니 아마 호출되지 않았을 것"으로 끝내지 않는다(REL12).

## 3. LIVE_VALIDATION은 예산 부족이 아니라 BLOCKED다

핸드오프는 실호출 예산 기본값 0을 말한다. [현행 검증표](../v1/remaining-verification.md) 3.2절은 그보다 강하다.

> 새 모델 실호출 예산 — **BLOCKED**. 사용자가 요구한 사전 출력 상한을 현재 구독 전송이 보장하지 못한다. 검증된 전송 계약 없이 상한 옵션을 제거하지 않는다.

세션 33은 잔여 0(최종 요청 cap 327)을 기록한다. 따라서 G6→G7 전이 조건에 **"사전 출력 상한을 보장하는 전송 계약"**을 명시적 선행 조건으로 넣는다. 예산을 더 달라고 요청하는 것만으로는 G7이 열리지 않는다.

### 3.1 그 선행 조건은 이제 미검증이 아니라 **충족 불가**로 측정됐다

1.3절의 5회가 그것을 물었다. `max_output_tokens`는 값을 가리지 않고 **HTTP 400**이다.

| | 전 | 후 |
|---|---|---|
| 근거 | PoC가 한 번 거부를 기록. 문서상 "미검증" | 2026-09-15 실측. 두 값, 두 번의 대조군 |
| 상태 | 전송 계약이 상한을 보장하는지 **모른다** | 이 backend는 상한 파라미터를 **받지 않는다** |

이 구독 전송으로는 생성을 미리 자를 방법이 없다. 사용할 수 있는 것은 기준선과 동일한 `usage-enforced-completion` 하나뿐이다 — 완료 후 보고된 usage로 검사하고 초과면 거부한다. 토큰은 이미 쓰였고, 거부가 막는 것은 "요청한 것과 다른 답을 건네는 것"뿐이다.

#### 이 실측이 내 것이 아니라 확인이라는 점

`poc/codex-transport.mjs:121`은 backend의 오류를 `code: unsupported_parameter` / `param: max_output_tokens`로 파싱한다. **"모르는 키" 거부가 아니라 이름을 알면서 지원하지 않는다는 응답이다.** 설치된 Codex CLI(0.154.0)에도 출력 상한 옵션이 없다. 즉 이 결론은 기존 증거가 이미 지지하고 있었고, 5회는 그것을 현재 시점에서 **확인**했다. 새로 발견한 것이 아니다.

#### 그래서 무엇이 바뀌는가 — 게이트 조건이다, 제품이 아니다

4.1절을 보라. "사전 출력 상한"을 G6→G7 전이 조건으로 적은 것은 이 문서의 오독이었고, 정정했다. 제품 동작은 바뀌지 않는다 — WP05의 구현이 이미 2026-09-08 처방과 같다.

#### 죽은 대안 하나를 기록한다

"스트리밍 중 상한에 닿으면 중단"을 검토했다. 기준선보다 나은 답처럼 보였다. **usage는 `response.completed`와 `response.incomplete`에만 실린다** — 스트리밍 중에는 토큰 수를 알 방법이 없으므로, 문자 수 추정으로 자르면 정상 응답을 자를 수 있다. 그리고 폭주 방어는 이미 있다(`stream.Limits`의 frame·event 상한, `ErrResponseTooLarge`). **정밀한 상한은 불가능하고 폭주 방어는 이미 존재하므로 만들 것이 남아 있지 않다.**

### 1.10 G5 C 범위 — argv 도달은 동작이 아니다 (2026-09-17)

기준선이 막던 옵션들이 이 재설계의 **정당화**인데, 증거는 `TestConfigurationOptionsStillReachTheChild`
하나뿐이었다 — 자식 argv에 그대로 도착한다는 것. 그건 "동작한다"와 다른 주장이다. 근거가
**"파서가 없으니 당연히 된다"는 구성 논증**이었고, 이 프로젝트에서 구성 논증은 다섯 번 뒤집혔다.

전부 추론 0회, 스크립트 백엔드 + 실제 클라이언트로 측정했다.

| 옵션 | 무엇을 확인했나 | 결과 |
|---|---|---|
| `--mcp-config` | stub MCP 서버가 뜨고, 툴이 모델에게 제공되고, 호출이 브리지를 왕복하고, **서버의 상속 환경이 살아남는가** | 통과 |
| `--resume` | 세션 1의 단어가 **세션 2의 요청 본문에** 실려 오는가 | 통과 |
| `--permission-mode plan` | 세션 내용이 실제로 달라지고 plan mode를 명시하는가 | 통과 |
| `--worktree` | `git worktree list`에 실제로 생겼는가 | 통과 |
| `--plugin-dir` | 플러그인의 skill 이름이 세션이 보내는 것에 들어 있는가 | 통과 |

**다섯 개 전부 옵션을 빼면 실패한다.** 확인했다 — 빼도 통과하는 테스트는 아무것도 측정하지 않는다.

**MCP가 가장 값진 측정이다.** 기준선은 자식 환경에서 `TOKEN`·`SECRET`을 포함한 **모든** 이름을
지웠고, 그래서 MCP 서버가 동작하지 않았다. 이 빌드는 `ANTHROPIC_*`과 OAuth 토큰만 지운다.
stub 서버의 툴이 `MCP_STUB_SECRET_TOKEN`(기준선 규칙의 두 문자열을 모두 포함)을 보고한다:

- 이 빌드: `PRESENT:the-servers-own-credential`
- `denied()`에 기준선 규칙을 넣은 변이: **`ABSENT`**

재설계가 고쳤다고 주장하던 것이 이제 논증이 아니라 측정이다.


## 4. 게이트

| Gate | 상태 | 산출물·증거 | 통과 후 허용 |
|---|---|---|---|
| G0 현황 | **완료** | 기준선 JSON, 전수 manifest 738/738, toolchain 실측 | 설계의 로컬 적합성 판단 |
| G1 설계 | **완료** | [DECISION.md](DECISION.md), [ARCHITECTURE.md](ARCHITECTURE.md), [MIGRATION.md](MIGRATION.md), 이 문서 | 위임 범위에 따른 구현 준비 |
| G2 격리 | **완료** | 2026-09-15 사용자 승인. worktree `Clauduct-go-v2`, 의존 0 Go module, 기준선 tracked 변경 0 | offline vertical slice |
| G3 최소 실행 | **산출물 완료** | WP01·WP02. argv/env/cwd 사양, loopback lifecycle, cleanup. 알려진 한계는 5.2절 | protocol 구현 |
| G4 기본 wire | **산출물 완료** | WP03·WP04. text·tool·JSON·SSE·error·cancel offline P/S, limit registry 확정 | native synthetic 통합 |
| G5 host parity | **완료 2026-09-17** | WP06이 P/S 범위를 덮었고, C 범위(MCP·plugin·worktree·resume·permission-mode)가 **기능으로** 닫혔다 — argv 도달이 아니라 동작. 아래 1.10절 | real backend 검증 계획 확정 |
| G6 transport 안전 | **완료** | WP05가 auth·attempt cap·retry·leak을, 1.9절이 process boundary(LIFE11·LIFE12·LIFE17)를 덮었다 | 아래 G7 조건 |
| G7 live integration | **완료** | 1.6절. 실제 claude.exe → 제품 빌드 → 실제 backend 왕복. 출하 바이너리로도 확인 | release 후보 판단 |
| G8 package | **완료** | 1.7절. 재현 빌드·신원·설치·동시 실행·자원 증가·문서. [PACKAGING.md](PACKAGING.md) | 기본 전환 판단 요청 |
| G9 기본 전환 | **완료 2026-09-17** | 사용자 승인("지금 전환"). 설치 실측: `clauduct`→Go, `clauduct-node`→Node, `clauduct-hook` 동거. 상태 파일이 `hookInstalled: true` | 새 실행의 기본 binary 변경 |
| G10 선택적 archive | **DEFERRED** | [MIGRATION.md](MIGRATION.md) 6장 M3. 권고는 하지 않음. **여는 조건 3개가 거기 적혀 있다** (2026-09-17) | 승인된 구조 정리 |

**산출물 완료**와 **게이트 통과**를 구분한다. 앞의 것은 "그 게이트가 요구한 증거가 만들어졌다"는 사실이고, 뒤의 것은 사용자 판단이다. 이 표는 앞의 것만 기록한다.

### 4.1 G6→G7 조건 — 2026-09-15 수정

이 전이 조건에 "사전 출력 상한을 보장하는 전송 계약"을 넣었던 것은 **G1에서 내가 출처를 과하게 읽은 것**이다. 원문은 그렇게 말하지 않는다.

| 출처 | 실제 범위 |
|---|---|
| `docs/remaining-verification.md` 3.2 | BLOCKED 대상은 **"새 모델 실호출 예산"** — 검증용 예산이다 |
| `docs/v1/release/session-29-release-verdict.md:77` | "상한 옵션을 제거하거나 **관측 후 판정으로 대체하지 않는다**" — 주장 위생 규칙이다 |
| `docs/v1/audit/audit-2026-09-08.md:180` | 처방은 이미 있었다: 완료 usage 검사 유지, 미지원 필드 추가 안 함, **한계 명시** |
| `docs/v1/README.md:221` | Node 제품은 그 한계를 명시한 채 **이미 출하돼 있다** |

즉 WP05가 구현한 것이 2026-09-08 처방과 같다. 게이트에 "충족 불가로 측정된 조건"을 걸어두면 Node 제품까지 소급해 출하 불가가 되므로, 기존 판정과 정면으로 충돌한다.

**수정된 조건 (2026-09-15 사용자 결정).**

| # | 조건 |
|---|---|
| 1 | **명시적 한정 예산이 있을 것** — 경로(model+effort)와 횟수를 함께 정한다. 사전 토큰 상한이 불가능하므로 이것이 실재하는 유일한 통제다 |
| 2 | **사전 상한을 주장하는 검증은 거부한다** — 기준선의 `VERIFICATION_PREGENERATION_LIMIT_UNAVAILABLE`과 같은 자리. "짧은 응답을 관측했으니 출력이 묶인다"는 추론을 금지한다 |
| 3 | `OUTPUT_TOKEN_LIMIT_EXCEEDED` 유지 — 상한 옵션을 제거하지 않는다 |
| 4 | 한계를 사용자 문서에 명시 — `docs/v1/README.md:221`가 Node에서 하는 것과 같게 |

조건 2는 **미래 코드에 대한 규칙이므로 지금 코드로 만들지 않았다.** V2에는 아직 검증 harness가 없어 호출자가 없고, 호출자 없는 상수는 유지보수할 죽은 코드다. G7에서 harness를 만들 때 이 표가 구현 대상이다. 지금 존재하는 방어는 `TestTheOutputLimitIsNotSentUpstream`과 mutation battery의 `max_output_tokens sent again`이다.

G7 통과가 G9 승인을 뜻하지 않는다. CI가 초록이라는 사실만으로 사용자 설치를 교체하지 않는다. `READY_FOR_USER_DECISION`과 `RELEASED`를 분리한다.

## 5. 작업 패키지와 테스트 ID 대응

테스트 ID 정의는 원본 핸드오프 23장에 있다. 게이트 종류: `P` core 제품 필수, `S` 안전 필수, `C` 해당 capability 선언 시 필수, `E` 별도 관리.

| WP | 범위 | 우선 테스트 |
|---|---|---|
| WP00 | 기준선·인벤토리·설계 정합성 | **완료** — REL01(738/738, 0/0/0/0) |
| WP01 | Go workspace, launcher skeleton, fake child | **완료** — 아래 5.1 |
| WP02 | ephemeral HTTP·생명주기 | **완료** — 아래 5.3 |
| WP03 | 최소 text request/response protocol | **완료** — 아래 5.5 |
| WP04 | tool round-trip과 delivery barrier | **완료** — 아래 5.7 |
| **WP05** | direct transport와 read-only auth | AUTH01–AUTH08, LIFE08–LIFE10, LIFE13, REL12 |
| WP03 | 최소 text protocol | WIRE01–WIRE10, WIRE12–WIRE15 |
| WP04 | tool round-trip·delivery barrier | TOOL01–TOOL08, LIFE10, WIRE11 |
| WP05 | direct transport·read-only auth | AUTH01–AUTH08, LIFE08–LIFE10, LIFE13, REL12 |
| WP06 | native host compatibility | ENV04–ENV08, ENV10, TOOL09–TOOL11, CAP04–CAP10, ARG06–ARG07 |
| WP07 | capability 확장 | HTTP08–HTTP10, TOOL12–TOOL16, CAP01–CAP03, CAP12 |
| WP08 | Windows·자원 안정성 | ARG09–ARG10, LIFE04–LIFE05, LIFE11–LIFE17, REL06–REL07 |
| WP09 | live validation·패키징 | 해당 P/S의 live 연계, REL04–REL10. G7–G9 구분 |
| WP10 | 선택적 archive | REL01, REL09, REL11. **DEFERRED** |

한 번에 모두 착수하지 않는다. 다음 하나는 WP06이다.

### 5.1 WP01이 실제로 덮은 테스트 ID

각 ID에 대응하는 Go 테스트가 존재하고 통과한다. 덮지 못한 것은 덮지 못했다고 적는다.

| ID | 상태 | 어디서 |
|---|---|---|
| ARG01 argv 순서·개수·값 | PASS | `launch` 단위 + `app`의 실제 spawn 왕복 |
| ARG02 빈 인자·공백·따옴표·trailing backslash | PASS | 동일. 빈 인자는 별도 테스트 |
| ARG03 한국어·Unicode·이모지 | PASS | 동일 |
| ARG04 `--` 이후 positional | PASS | `launch` 단위 + spawn 왕복 |
| ARG05 옵션 값 내부 모델명 미가로채기 | PASS | parser가 없어 구조적으로 성립. 그래도 assert한다 |
| ARG08 stdin/stdout/stderr·cwd·exit code | PASS | 실제 spawn, exit 7 왕복 |
| ARG09 실제 native exe·shim quoting | **PASS(한계 기록)** | 5.1.1절. 17개 hostile shape가 **node.exe**를 왕복해 그대로 돌아온다. shim은 해석기가 `claude.exe`만 받으므로 경로에 없다 |
| ARG10 shell metacharacter가 명령이 되지 않음 | PASS | 앰퍼샌드·파이프·리다이렉트·캐럿·퍼센트·느낌표·세미콜론·명령치환 fixture |
| ENV01 일반 MCP/service 환경변수 보존 | PASS | 사용자 결정 반영 |
| ENV02 Anthropic credential 유출 방지 | PASS | mutation으로 보강 후 |
| ENV03 Windows env key 대소문자 중복 | PASS | 규칙을 이 모듈이 소유하고 pin |
| ENV09 secret이 stdio·오류에 노출되지 않음 | PASS | 세션 token 기준 |
| HTTP01 `127.0.0.1:0` bind 후 native 시작 | PASS | 자식이 실제로 dial해 200 확인 |
| LIFE01 gateway 준비 실패 시 native 미실행 | PASS | 주입한 bind 실패 |
| LIFE02 spawn 실패 후 listener 정리 | PASS | 포트 해제를 dial로 확인 |
| LIFE03 정상 종료 후 owned socket 0 | PASS | 동일 + 5회 연속 세션 |
| REL03 Node·.NET runtime 의존 없음 | PASS | AST 문자열 리터럴 스캔 + 의존성 0 검사 |
| ARG06 unknown native 옵션 전달 | `NOT_RUN` | NATIVE_SYNTH 필요. WP06 |
| ARG07 native help/version이 auth 없이 실행 | `NOT_RUN` | 기회적 관측은 있으나 통제된 실행이 아니다 |
| ARG09 실제 native exe·shim quoting | **재판정: PASS(한계 기록)** | 5.1.1절 |
| REL02 Node source hash 대조 | `NOT_RUN` | manifest는 있으나 대조 harness를 아직 만들지 않았다 |

### 5.2 argv fixture의 알려진 한계

가짜 native client는 테스트 바이너리를 재실행한 것이다. 디스크의 실제 실행 파일이고 실제 Windows 커맨드라인을 받으므로 프로세스 생성 왕복은 진짜다. 다만 **양쪽 끝이 Go**라서 측정하는 것은 Go의 quoting 대 Go의 parsing이다.

다른 규칙으로 커맨드라인을 파싱하는 native 바이너리는 이 fixture가 닿지 못한다. 그것이 ARG09이며 `NATIVE_SYNTH` 수준의 질문이다. WP01의 통과를 "실제 claude.exe에서 argv가 보존된다"로 읽지 않는다.

#### 5.2.1 ARG09 — 제3자에게 물었다 (2026-09-16)

한계를 닫았다. `node.exe`로 17개 hostile shape를 왕복시킨다. 편해서 고른 것이 아니다 — **이 launcher가 띄우는 클라이언트가 Node 바이너리**이므로, node가 커맨드라인에 적용하는 규칙이 실제 자식이 적용하는 규칙이다. 빈 문자열·앞뒤 공백·중첩 따옴표·trailing backslash·`&|<>^`·`%PATH%`·`!DELAYED!`·한국어·이모지·탭이 전부 그대로 돌아온다.

**처음 고른 제3자는 틀렸고, 그것이 발견이다.** `cscript.exe`를 먼저 썼는데 모든 따옴표를 뭉갰다 — `he said "hi"`가 `he said \hi\`로 도착한다. Windows Script Host는 C 런타임 방식으로 커맨드라인을 해체하지 않는다. 즉 **CRT 규칙을 쓰지 않는 native host는 실제로 인자를 망친다.**

그래서 shim 쪽 답은 "구현했다"가 아니라 **"경로에 없다"**이다. 해석기는 `claude.exe`만 찾는다. `.cmd`·`.bat`·`.ps1`은 cmd.exe나 PowerShell이 한 번 더 파싱하며, 그건 Go가 quoting한 규칙이 아니다. 그 정책을 테스트가 고정한다: shim만 존재하는 PATH에서 해석기는 **아무것도 찾지 못한다**.

### 5.3 WP02가 실제로 덮은 테스트 ID

| ID | 상태 | 어디서 |
|---|---|---|
| HTTP02 동시 두 세션의 port/token/request 격리 | PASS | 주소·token 상이 + registry 분리 확인 |
| HTTP03 잘못된 token·재사용 token 거부 | PASS | 8개 케이스(부재·빈 Bearer·오타·scheme 누락·소문자 scheme·이중 공백·접미사·**타 세션 token**) |
| HTTP04 Host/method/content type/payload 경계 | PASS | 405·415·413(실제 32 MiB 초과 전송)·중복 header 400 |
| HTTP05 `?beta=true` 등 query 처리 | PASS | 측정된 query 포함 4종이 라우팅을 바꾸지 않음 |
| HTTP06 동일 token 복수 auth header | PASS | 전방 호환. **현재 native는 이 상황을 만들지 않는다**(1.1.2) |
| HTTP07 서로 다른 auth header 값 거부 | PASS | 타 세션 token·외부 키·Bearer 접두 3종 전부 403 |
| HTTP09 warmup 응답에 secret 없음 | PASS | 본문 공백, header에 token 부재 |
| HTTP11 실제 provider 접속 0 | PASS(구조) | upstream 클라이언트 코드가 존재하지 않는다 |
| LIFE06 한 요청 취소가 형제에 전파되지 않음 | PASS | registry 단위에서 결정적으로. HTTP 층은 등록·해제 배선만 확인 |
| LIFE07 중복 close·취소 race·닫힌 channel | PASS | 멱등 release, 미등록 cancel=false, 50-goroutine 동시 race |
| HTTP07 임의 redirect 거부 | PASS | WP05. `CheckRedirect`가 항상 거부한다. 301·302·303·307·308 각각에 대해 **redirect 대상 서버가 요청을 0건 받았음**을 확인 |
| HTTP08 `/v1/models` query·cache·picker | `NOT_RUN` | discovery 미구현. WP07 |

### 5.4 WP02에서 고친 실제 결함 하나

`Close`가 30초 deadline을 다 쓰고 실패했다. **context 취소는 blocking body read를 풀지 못한다** — handler는 `io.Copy` 안에 있고 `Shutdown`은 그 handler를 기다린다. Node 기준선은 소켓을 destroy해서 이 지점을 통과한다(`readBody`의 `resetAndDestroy`). Go의 대응물은 read deadline이고, `http.ResponseController`로 취소 시 즉시 만료시킨다. 같은 메커니즘이 body 완료 상한(300s, 기준선과 동일)도 함께 맡는다.

`Shutdown`이 그래도 실패하면 `server.Close()`로 강제 해제하되, **원래 실패를 성공으로 덮지 않고 그대로 보고한다.**

**2026-09-16, 그 메커니즘이 실어 나른 결함 하나.** gateway suite가 절반쯤 `"An existing connection was forcibly closed by the remote host"`로 깨졌다. 매번 다른 테스트였고, 빠른 테스트만 걸렸다. 포트 재사용도 커넥션 풀링도 아니었다 — trace는 전부 `reused=false`였고 포트 이력은 단조 증가했다.

범인은 취소 감시 goroutine이었다. `select`가 `ctx.Done()`과 handler의 done channel을 함께 기다리는데, 빠른 요청에서는 goroutine이 처음 스케줄될 때 **둘 다 이미 닫혀 있다**(net/http는 handler가 리턴하는 즉시 요청 context를 취소한다). Go는 준비된 case 둘 중 하나를 무작위로 고른다. 그 절반은 handler가 이미 손을 뗀 커넥션에 만료된 read deadline을 걸었고, 응답은 그때 아직 서버 쓰기 버퍼에 있었다. client는 답 대신 reset을 받았다. **테스트만의 문제가 아니다.**

추론이 아니라 측정으로 좁혔다. 감시 goroutine만 빼고 300초 deadline은 그대로 둔 6회 실행에서 reset 0건, deadline을 통째로 뺀 6회에서도 0건. done channel은 defer로 닫히므로 `ctx.Done()`이 오기 **전에** 반드시 닫힌다 — 다시 확인하는 것은 또 하나의 추측이 아니라 확정이다. 수정 후 gateway 22회·모듈 전체 6회 통과.

이 결함이 드러난 이유 자체가 기록할 만하다. **suite를 처음으로 연속해서 돌렸기 때문이다.** 한 번 초록인 것은 초록이라는 증거가 아니다.

### 5.5 WP03 — 완료

text 경로가 끝에서 끝까지 동작한다. `POST /v1/messages`는 501을 돌려주지 않는다: 요청을 해독하고, backend 요청으로 변환하고, transport로 실행하고, 돌아온 SSE를 파싱해 Anthropic 프레임으로 내보낸다.

**2026-09-15 G7에서 연결했다.** WP05가 만든 실제 전송이 제품 빌드에 들어가 있고, 이 바이너리가 시작한 모든 추론은 사용자의 Codex 구독에 도달한다. 1.6절.

| ID | 상태 | 어디서 |
|---|---|---|
| WIRE01 JSON missing/null/empty 구분 | PASS | `wire.Of`가 absent·null·present를 세 답으로 돌려준다. `{}` / `{"isolation":null}` / `{"isolation":"worktree"}`가 절대 합쳐지지 않음을 고정 |
| WIRE02 큰 정수·ID·정밀도 보존 | PASS | 페이로드를 재직렬화하지 않는다. `max_tokens`는 리터럴 텍스트로 파싱 |
| WIRE03 duplicate key·trailing JSON·비정상 UTF-8 | PASS | 요청과 이벤트 양쪽에서 `internal/wire` 공유 규칙 |
| WIRE04 SSE 한 byte 단위 fragmentation | PASS | parser 단위 + HTTP 경로 end-to-end |
| WIRE05 CRLF/LF·멀티라인 data·comment/ping | PASS | parser 단위 |
| WIRE06 64 KiB 초과 합법 frame과 최대 경계 | PASS | 256 KiB 보존 + 상한 초과 거부 + 미완결 frame 계상 |
| WIRE07 event 수·총 bytes·응답 상한 | PASS | `TOO_MANY_EVENTS`·`RESPONSE_TOO_LARGE`, emitter 쪽 16 MiB·1024 block 상한 포함 |
| WIRE08 text delta와 completion snapshot 일관성 | PASS | backend snapshot과 누적 delta 불일치는 `TEXT_MISMATCH`. delta 없는 **빈** snapshot만 허용 — 기준선이 실제로 실패했던 사례 |
| WIRE09 terminal 누락·조기 EOF·중복 terminal | PASS | 6종 순서 위반 + 전송 조기 종료 + **EOF 아닌 read 실패** |
| WIRE10 `[DONE]`·완료 후 trailing data | PASS | parser 단위 |
| WIRE12 reasoning 뒤 최종 text 순서 | PASS | reasoning 선행이 client가 보는 순서를 바꾸지 않음. reasoning 내용은 전달되지 않음 |
| WIRE13 느린 downstream backpressure | PASS | 쓰기마다 갱신되는 30초 deadline(`writeStall`). 읽기를 멈춘 client는 **3.01초**(테스트용 3초 bound)만에 놓여났다 — 그 전에는 goroutine·backend 연결·과금 중인 요청을 무한정 붙잡았다. 전체 응답 timeout이 아니라는 것은 `SetWriteDeadline`/`Flush` 순서를 직접 기록해 확정했다. 경계: bound는 **쓰기 하나**에 걸린다. 수신 버퍼를 8 KB씩 비우는 client는 정상 코드에서도 잘리는 것이 실측됐는데, 원인은 gateway가 아니라 TCP다(수신측이 8 KB마다 window 재통지를 하지 않는다). 30초 기준으로는 초당 10 KB 미만으로 소비하는 client에 해당한다 |
| WIRE14 ping과 upstream idle timeout 구분 | PASS | keepalive를 진전으로 읽지 않는 것에 더해, WP05가 phase별 timeout을 붙였다(handshake 30초, 응답 헤더 120초). 하나의 전체 deadline이면 "오래 생각하는 응답"과 "멈춘 연결"을 같은 순간에 자른다 |
| WIRE15 gzip/encoding 지원 여부와 크기 상한 | PASS | WP05. 압축을 **요청하지 않는다**(`Accept-Encoding: identity` + `DisableCompression`). 요청하지 않은 압축은 풀 일이 없고, 풀 일이 없으면 상한을 정할 크기도 압축 폭탄도 없다 |
| TOOL01–TOOL08 | `NOT_RUN` | 도구는 명시적으로 거부된다. WP04 |

### 5.5.1 실측이 계약을 두 번 고쳤다

**첫 번째.** `max_tokens`를 `json.Number`로 unmarshal하면 JSON *문자열* `"1024"`가 숫자 1024로 받아들여진다. 인용부호를 붙여 보낸 클라이언트가 숫자를 보낸 것처럼 읽힌다. 리터럴 텍스트 파싱으로 바꿨고, 그 김에 `1.5`·`1e100`·safe range 초과도 거부된다. 핸드오프 R05가 경고한 "Go JSON의 묵시적 변환"이다.

**두 번째.** 파이프라인을 붙인 뒤 실제 `claude.exe`로 `-p "ping"`을 돌렸더니 `400 MESSAGE_ROLE`이 나왔다. 역할 허용목록을 `user`·`assistant` 둘로 잡았는데 **실제 클라이언트는 `system` turn을 보낸다.** 기준선을 다시 읽어 네 가지를 고쳤다.

- 역할은 셋이다: `user` `assistant` `system`. 오류 이름도 기준선의 `UNSUPPORTED_MESSAGES`로 맞췄다
- 메시지에 `output_config`가 올 수 있다 — **turn별 effort override**이며 `system` turn에만 허용된다
- text block의 허용 키는 `type` `text` `cache_control`이다. 그 전에는 아무 키나 통과했다
- backend에서 `system`은 `developer`다

고친 뒤 같은 명령이 **`400 TOOL_USE_UNSUPPORTED`**를 낸다. 측정된 119 KB envelope 전체가 통과하고, 실제 세션과 이 빌드 사이에 남은 것은 도구 지원뿐이라는 뜻이다.

두 번 다 단위 테스트가 아니라 **실물과 맞대 본 것**이 찾아냈다. 모델 호출은 0회다 — probe에도 제품 빌드에도 transport가 없다.

### 5.5.2 기준선보다 엄격하게 한 것

기준선은 `keepalive` 한 건에만 원문 정규식으로 중복 key를 막는다. V2는 **요청과 이벤트 양쪽에서 top-level 중복 key를 일반 규칙으로 거부**한다. 그 결과 기준선의 정규식은 도달 불가능한 분기가 되어 이식하지 않았다 — 규칙 하나가 자기 특수 사례를 흡수한다.

### 5.6 WP03이 남긴 것

`/v1/messages`는 구현됐고 **G7에서 실제 전송에 연결됐다.** 1.6절.

도구는 WP04다. 측정된 실제 요청은 `tools`를 **항상** 포함하므로, 도구 지원 전까지 실제 세션은 성립하지 않는다. 그 사실이 침묵이 아니라 명시된 오류로 나타나는 것이 WP03이 보장하는 것이다.

### 5.7 WP04 — 완료

도구 왕복이 끝났다. 요청은 정의·`tool_choice`·기록된 호출과 결과를 받아들이고, 응답은 backend의 호출을 검증해 `tool_use` 블록으로 내보낸다.

**barrier는 설계의 결과이지 덧댄 검사가 아니다.** 호출은 스트리밍 이벤트가 아니라 `response.completed`의 `output` 배열에서만 나온다. 따라서 중간에 실패한 스트림은 클라이언트에게 실행할 것을 건넨 적이 없다 — 막는 코드가 있어서가 아니라 만들어지는 자리가 그 뒤이기 때문이다.

| ID | 상태 | 어디서 |
|---|---|---|
| TOOL03 name·ID·arguments·result 연결 보존 | PASS | id·name이 그대로 도달, arguments는 재직렬화 없음 |
| TOOL04 복수 tool call의 순서·결과 대응 | PASS | 3개 호출의 순서와 blockindex 단조 증가 |
| TOOL05 completion 전 tool side effect 0 | PASS | 스트리밍 이벤트에서 프레임 0, 실패한 스트림에서 호출 0 |
| TOOL07 optional enum 생략·null·값 구분 | PASS | `{}` / `{"isolation":null}` / `{"isolation":"worktree"}`가 셋으로 도달 |
| TOOL08 inactive historical tool과 신규 inactive call 구분 | PASS | 철회된 도구를 이름으로 가진 기록은 해독되고, 그 이름의 **새 호출**은 거부 |
| WIRE11 malformed tool arguments 미전달 | PASS | 8종(비JSON·잘림·배열·문자열·숫자·trailing·중복 key·빈 값) |
| LIFE10 semantic delivery 이후 자동 replay 0 | PASS | WP05에서 재판정했다. `MaxGatewayRetries = 0`이고 `Direct.Execute`에 재시도 루프가 없다 |
| TOOL03·TOOL04·WIRE11 | **재판정 완료** | 1.4.1절. fixture를 실제 wire로 바꿔 재작성했고, 도구 왕복이 실제 backend에서 통과했다 |
| TOOL01 Read/Edit/Write/Bash 실제 왕복 | `NOT_RUN` | NATIVE_SYNTH. WP06 |
| TOOL02 permission 거부가 실행으로 바뀌지 않음 | `NOT_RUN` | NATIVE_SYNTH. WP06 |
| TOOL06 전달 후 실패 시 자동 재실행 0 | `NOT_RUN` | NATIVE_SYNTH. WP06 |

### 5.7.1 측정이 상태 코드를 바꿨다

도구 지원 후 실제 `claude.exe`를 다시 붙였더니 **150초를 매달렸다.** transport가 없어 `NO_UPSTREAM_TRANSPORT`(당시 503)를 돌려주는데, 클라이언트가 재시도를 반복하고 있었다.

일회용 listener로 상태 코드별 재시도를 측정했다. 모델 호출 0회다.

| 응답 | 60초 동안의 요청 수 | 자식 종료 |
|---|---|---|
| 400 | **3** (readiness 1 + POST 2) | 즉시 |
| 501 | **8**, 계속 | 종료 안 함 |
| 502 | **8**, 계속 | 종료 안 함 |
| 503 | **8**, 계속 | 종료 안 함 |

**5xx는 종류를 가리지 않고 재시도된다.** 상태 코드 계열은 책임 소재이기 전에 **재시도 지시**다. "transport가 설정되지 않음"은 프로세스 수명 내내 영구적이므로, 5xx로 답하면 절대 바뀌지 않을 조건을 향해 클라이언트가 무한히 backoff한다. 400으로 바꾸니 같은 명령이 **4.5초**에 `400 NO_UPSTREAM_TRANSPORT`로 끝난다.

진짜 upstream 실패는 502로 남겼다 — 재시도가 성공할 수 있는 조건이다. 다만 클라이언트 재시도와 이 bridge의 재시도가 곱해지는지는 실제 전송이 생기는 WP05에서 판정한다. 위 수치가 그 판정의 입력이다.

### 5.7.2 WP04에서 고친 것 하나

WP03은 아무것도 만들지 않은 응답에 빈 assistant 메시지를 내보내고 있었다. 기준선은 `EMPTY_REPLY`로 거부한다. 빈 메시지는 "모델이 아무 말도 안 했다"는 **그럴듯한 답**처럼 읽히므로 실패를 답으로 위장한다. 거부로 바꿨고, 도구 호출만 있는 응답은 무언가를 만들었으므로 비어 있지 않다.

### 5.8 WP05 — 완료

실제 HTTPS 전송, 읽기 전용 credential provider, attempt 원장, 실패 분류가 들어왔다. **제품 빌드에는 연결하지 않았다.**

#### 5.8.1 예산 정책은 개수가 아니라 경로다

사용자 승인(2026-09-15): **`gpt-5.6-luna` / effort `low` / 누적 20회.**

처음에 나는 astra를 가장 싼 모델로 가정하고 제안했다. 사용자가 정정했다 — *"astra모델은 최상위 모델로 fable 급이다 제일 비싸다."* 이 정정이 정책의 모양을 바꿨다. **개수만으로는 아무것도 승인되지 않는다.** 가장 싼 모델의 가장 낮은 effort 20회와 최상위 모델의 최고 effort 20회는 같은 숫자로 전혀 다른 금액이다. reasoning token도 비용에 들어가므로 effort까지 고정한다.

그래서 `Budget`은 `{Model, Effort, Limit}` 셋이 모두 있어야 무엇이든 허가한다. 셋 중 하나라도 비면 `Reserve`도 `Remaining()`도 0을 답한다 — 이 둘이 서로 다른 조건을 쓰고 있던 것이 이 절을 쓰다가 테스트로 잡힌 결함이다.

| 결정 | 이유 |
|---|---|
| 예약이 소켓보다 먼저 | 사후 집계는 상한이 아니라 보고서다. 상한 대상이 이미 일어난 뒤에 보고서가 써진다 |
| credential 읽기보다도 먼저 | 아무도 승인하지 않은 요청이 credential 파일을 읽을 이유가 없다 |
| attempt와 inference를 따로 셈 | 재시도 2회를 곁들인 1 추론은 1 추론이고 3 attempt다. 한 단위로 정하고 다른 단위로 재면 상한이 아니다 |
| 완료된 attempt를 돌려주지 않음 | 요청은 이미 나갔다. 끝난 일을 환불하는 상한은 아무것도 막지 못한다 |
| `MaxGatewayRetries = 0` | 5.7.1의 실측이다. 설치된 클라이언트가 모든 5xx를 스스로 재시도하므로 여기서 또 재시도하면 곱해진다. 그러면 "attempt 단위 상한"이 사용자가 청구받는 금액을 묶지 못한다 |

#### 5.8.2 기준선과 어긋난 요청 본문 셋을 고쳤다

WP03·WP04가 만든 upstream 본문을 `src/native-protocol.mjs:427-430`과 대조했다.

| 항목 | V2가 보내던 것 | 기준선 | 조치 |
|---|---|---|---|
| `max_output_tokens` | 클라이언트의 `max_tokens`를 그대로 전달 | **보내지 않음** | 제거 |
| `instructions` | 클라이언트 system prompt를 승격 | 고정 문자열 + system은 `input`의 `developer` 턴 | 기준선과 동일하게 |
| `include` / `store` | 둘 다 없음 | `['reasoning.encrypted_content']` / `false` | 추가 |

**`max_output_tokens` 제거는 혼자 올 수 없었다.** 그냥 빼면 클라이언트의 `max_tokens`를 강제하는 것이 아무것도 남지 않는다 — 검증을 약화시켜 통과하는 쪽이다. 기준선이 어떻게 하는지 찾았다: `native-protocol.mjs:800`이 완료 시점에 `usage.output_tokens <= outputLimit`을 검사하고 `OUTPUT_TOKEN_LIMIT_EXCEEDED`로 거부한다. `OUTPUT_TOKEN_LIMIT_POLICY = 'usage-enforced-completion'`이 그 이름이다.

**이것은 생성 상한이 아니라 사후 검사다.** 토큰은 이미 쓰였고, 거부는 "요청한 것과 다른 답을 건네지 않는다"는 의미밖에 없다.

이 절을 쓸 때는 "backend가 받는지 미검증"이 근거였다. 그 뒤 5회를 실제로 돌렸고(1.3절) **받지 않는 것으로 측정됐다.** 제거는 parity가 아니라 **출시 차단 결함의 수정**이었다 — 그 키를 보내는 한 이 bridge의 모든 추론 요청이 400으로 끝난다.

연쇄가 하나 더 있었다. 상한을 usage로 검사하려면 usage가 있어야 한다. 기준선의 `nativeUsage`는 세 카운트가 모두 없으면 `INVALID_USAGE`로 거부한다. Go 쪽은 "없으면 모르는 것"으로 두고 있었으므로, usage를 생략하는 backend가 상한을 그냥 통과하게 된다. 거부로 바꿨다. fixture 다수가 usage 없는 `response.completed`를 쓰고 있었고 전부 고쳤다.

검사 **순서**도 측정해서 맞췄다. 처음에는 완료 이벤트를 받자마자 상한을 봤는데, 그러면 malformed tool call이 `INVALID_TOOL_CALL` 대신 `OUTPUT_TOKEN_LIMIT_EXCEEDED`로 보고된다. 기준선은 응답 자체를 먼저 검증하고 상한은 마지막이다. 도착한 것의 결함과 정상 응답에 대한 정책 질문은 다른 것이다.

#### 5.8.3 테스트 ID

| ID | 상태 | 어디서 |
|---|---|---|
| AUTH01–AUTH08 | PASS | WP05a. `internal/auth`. mutation 18건 전수 |
| LIFE08 429/5xx retry와 총 attempt cap | PASS | 상한 도달 후 **listener가 센 요청 수**가 멈춘다. 재시도는 attempt를 쓰되 inference를 쓰지 않는다 |
| LIFE09 긴 Retry-After deferred·시각 계산 | PASS | delta-seconds 3형식 + HTTP-date 3형식. 마감은 분류 시점의 시계 **한 번**에서 계산한다 |
| LIFE10 semantic delivery 이후 자동 replay 0 | PASS | `MaxGatewayRetries = 0`. `Execute`에 재시도 루프 없음 |
| LIFE13 네트워크/DNS/TLS 오류 분류와 retry 경계 | PASS | 11종. 인증서 실패는 terminal이고 terminal로 남는다 |
| REL12 예산 0에서 real attempts 0 | PASS | 증거가 반환값이 아니라 **서버가 센 수**다. 예산 0 / ledger 없음 / probe 무동의 세 경로 |
| HTTP07 임의 redirect 거부 | PASS | 5개 상태 코드, redirect 대상 서버 요청 0건 |
| WIRE15 encoding 상한 | PASS | 압축을 요청하지 않는다 |

#### 5.8.4 mutation — 39건 주입, 3건이 살아남았다

| 살아남은 결함 | 왜 초록이었나 | 조치 |
|---|---|---|
| 403 arm 무력화 | 403 분기가 아래 일반 4xx 분기와 **행동이 완전히 같았다.** 구분할 수 없는 두 분기는 한 분기와 주석이다 | 분기를 지우고 이유를 일반 분기 주석에 합쳤다. 이제 그 분기를 바꾸면 잡힌다 |
| `max_output_tokens` 재도입 | 주입이 필드만 추가하고 채우지 않아 wire에 나타나지 않았다. **무의미한 주입이 살아남은 것이지 커버리지 구멍이 아니다** | battery가 한 mutation에 여러 편집을 허용하도록 고쳤다. 필드 추가와 대입을 함께 넣으니 잡힌다 |
| 버전 문자열 검사 제거 | `installedCodexVersion`이 subprocess와 붙어 있어 아무 테스트도 닿지 못했다. 순수한 파싱만 따로 테스트되고 있었다 | 파싱을 `parseCodexVersion`으로 분리하고 이 머신이 내지 않는 출력 11종으로 테스트 |

compiler-only 1건도 있었다 — redirect 주입이 존재하지 않는 변수를 썼다. 컴파일되는 형태로 고쳤고 잡힌다. **컴파일러가 거부한 것은 테스트 증거가 아니다.**

재실행 결과: **39건 주입, 미검출 0, compiler-only 0.**

#### 5.8.5 probe — 만들었고 돌리지 않았다

3장의 BLOCKED를 푸는 데 필요한 사실은 하나다: **backend가 `max_output_tokens`를 받아들이는가.** 기준선은 보내지 않고 PoC는 거부를 기록했지만, "한 번 거부됐다"와 "오늘 거부된다"는 다른 주장이다.

`clauduct-dev probe --send`가 그것을 측정한다. luna/low로 두 요청 — 하나는 기준선과 같은 본문(대조군), 하나는 `max_output_tokens: 16`. `response.incomplete`에 `reason: max_output_tokens`가 오면 backend가 지킨 것이고, `response.completed`가 오면 무시한 것이고, 400이면 거부한 것이다.

| 안전장치 | 내용 |
|---|---|
| `--send` 없이는 아무것도 보내지 않음 | 정확히 `["--send"]` 하나일 때만. 오타·접두사·중복·앞뒤 인자 9종 테스트 |
| 무동의 시 가격을 화면에 출력 | 모델·effort·이 실행의 상한·누적 승인량·endpoint. 가격을 말하지 않는 동의는 동의가 아니다 |
| 한 실행의 상한 3회 < 누적 20회 | 반복 실행이 눈에 보이는 결정이 되게 한다 |
| 모델 출력을 읽지도 출력하지도 않음 | 이벤트 타입과 카운트만. 출력되는 `reason`은 고정 집합 밖이면 `unrecognised` |

**두 번 돌렸고 답이 나왔다 — 1.3절.** 누적 5회, 잔여 15회.

첫 실행 뒤 설계 결함 하나를 고쳤다. `max_output_tokens: 16` 하나만으로는 **파라미터 거부와 최솟값 미달을 구분할 수 없다.** 둘은 정반대 결론으로 이어진다. 48을 추가해 갈랐다. 그리고 원래 프롬프트("reply ok")는 어차피 상한 안에서 끝나므로, backend가 상한을 지키든 무시하든 **똑같이 `response.completed`가 온다** — 두 세계에서 관측이 같으면 측정이 아니다. 40까지 세는 프롬프트로 바꿨다.

#### 누적 상한은 기록이지 기구(機構)가 아니다

`probeAttempts = 3`은 **한 프로세스 안에서** 강제된다. 누적 20회는 이 문서가 기록하는 숫자이고, 프로세스 사이에 남는 상태가 없으므로 반복 실행을 막는 것은 없다. 파일에 원장을 두면 막을 수 있지만, 지우면 그만인 파일은 상한이 아니라 상한의 외양이다. **현재 상태를 정확히 적는 쪽을 골랐다.**

#### 5.8.6 이번 WP에서 지운 것

`SyntheticOnly` wrapper를 썼다가 지웠다. fixture transport는 credential을 읽지 않으므로 실제 credential이 거기 도달할 경로가 **없다.** 도달할 수 없는 경로를 지키는 wrapper는 일어날 수 없는 경우를 위해 유지보수할 코드다. 반대 방향 — 합성 credential이 실제 소켓에 가는 것 — 은 경로가 있으므로 `Direct`가 검사하고 테스트가 덮는다.

`Ledger.Reserve`가 돌려주던 `release` 클로저도 지웠다. 본문이 비어 있었다. 아무것도 하지 않는 것을 호출자가 반드시 호출해야 하는 구조는 의식(儀式)이다.

#### 5.8.7 이번 WP에서 실행한 Node 기준선

패키지별 직전 검증 정책에 따라, WP05가 계약을 가져온 파일들의 Node 테스트 17개를 돌렸다.

`test-additional-rate-limits` · `test-auth-owner-pipes` · `test-auth-owner-protocol` · `test-client-version` · `test-connection-fault` · `test-credential-recovery` · `test-credential-store-selection` · `test-fixture-token-budget` · `test-fixture-transport-progress` · `test-fixture-usage` · `test-http-close` · `test-http-retry-status` · `test-keepalive-transport` · `test-native-transport` · `test-rate-limit-headers` · `test-rate-limit-observation` · `test-transport-rejections`

**17개 전부 exit 0.** 기준선 worktree는 tracked 변경 0으로 남아 있다.
