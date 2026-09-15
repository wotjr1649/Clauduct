# V2 검증 계획 — 요구·테스트·게이트·예산

## 1. 현재 증거 상태

| 항목 | 값 |
|---|---|
| 실행한 V2 Go 테스트 | **81개 통과** (subtest 포함) |
| mutation 검증 | 16건 주입. 2건이 처음에 살아남았고 둘 다 테스트를 보강해 잡았다 |
| 실모델 호출 | **0회.** upstream 코드가 존재하지 않아 구조적으로 불가능하다 |
| 잔여 승인 예산 | **0. 그리고 별도로 BLOCKED다** — 3장 |

### 1.1 실행한 것

**Node 기준선 (변경 없음 확인용)**

| 명령 | 결과 | 관측 |
|---|---|---|
| `node verification/test-doc-citations.mjs` | PASS exit 0 | documents 86, citations 80, linksChecked 347, failures 0, externalRequests 0, credentialReads 0 |
| `./clauduct.cmd --dry-run -p` | PASS exit 0 | model gpt-6-astra, effort low, contextPolicy 400000/320000/20000, generalAgentModels 14행 |

두 번째 명령의 `credentialReads`·`childStarted`·`globalWrites`는 HANDOFF.md 3장이 "이 분기가 쓰는 고정값이라 아무것도 증명하지 않는다"고 못박은 값이다. 무접속의 증거로 인용하지 않는다.

**Go V2 (WP01)**

| 검사 | 결과 |
|---|---|
| `gofmt -l .` | 출력 없음 |
| `go vet ./...` | 통과 |
| `go build ./...` | 통과 |
| `go test -count=1 ./...` | **4 package 통과**, 81 테스트 |
| `go build -trimpath` 후 `version` | VCS stamp 확인: `bfbdf238…+dirty`, go1.27.0 windows/amd64 |
| `clauduct-dev doctor` | exit 0. claude.exe 해석 성공, 82 parent vars → 87 child vars, credential 읽기 0 |
| `clauduct-go --version` | exit 0. 실제 claude.exe가 `2.1.272 (Claude Code)` 출력 |

마지막 항목은 실제 사용자 프로필에서의 기회적 관측이지 통제된 `NATIVE_SYNTH` 실행이 아니다. ARG06·ARG07의 증거로 승격하지 않는다.

**실호출이 0인 근거는 관측이 아니라 구조다.** 이 빌드에는 upstream 클라이언트가 존재하지 않는다. `internal/gateway`는 `POST /v1/messages`를 구현하지 않고 아웃바운드 HTTP를 전혀 만들지 않으므로, 모델 요청은 "일어나지 않았다"가 아니라 "일어날 경로가 없다".

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
| Node 회귀 113개 파일 | `NOT_RUN` | 기준선 신원은 commit SHA이지 통과 개수가 아니다. HANDOFF.md 4장 4단계가 무검토 glob 실행을 금지한다. 비교 기준으로 필요해지는 시점은 G4 이후. 요청 시 파일 단위 offline 실행 가능 |
| `go test -race` | **`NOT_RUN`** | 이 머신에 cgo·C 툴체인이 없다(`-race requires cgo`, gcc 부재). CI 워크플로가 담당한다. **미실행은 통과가 아니다** |
| Go CI 워크플로 | **`NOT_RUN`** | `.github/workflows/go.yml`을 작성했으나 실행된 적이 없다. push는 별도 승인 사항이라 하지 않았다 |
| 대화형 세션 | `NOT_RUN` | `/v1/messages`는 인증·경계까지만 통과하고 501을 돌려준다. 본문 처리는 WP03 |
| native synthetic 통합 (NATIVE_SYNTH) | `NOT_RUN` | 격리 fixture profile을 아직 만들지 않았다. WP06 |
| 실모델 호출 | `NOT_AUTHORIZED` | 3장 |

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

핸드오프는 실호출 예산 기본값 0을 말한다. [현행 검증표](../remaining-verification.md) 3.2절은 그보다 강하다.

> 새 모델 실호출 예산 — **BLOCKED**. 사용자가 요구한 사전 출력 상한을 현재 구독 전송이 보장하지 못한다. 검증된 전송 계약 없이 상한 옵션을 제거하지 않는다.

세션 33은 잔여 0(최종 요청 cap 327)을 기록한다. 따라서 G6→G7 전이 조건에 **"사전 출력 상한을 보장하는 전송 계약"**을 명시적 선행 조건으로 넣는다. 예산을 더 달라고 요청하는 것만으로는 G7이 열리지 않는다.

## 4. 게이트

| Gate | 상태 | 산출물·증거 | 통과 후 허용 |
|---|---|---|---|
| G0 현황 | **완료** | 기준선 JSON, 전수 manifest 738/738, toolchain 실측 | 설계의 로컬 적합성 판단 |
| G1 설계 | **완료** | [DECISION.md](DECISION.md), [ARCHITECTURE.md](ARCHITECTURE.md), [MIGRATION.md](MIGRATION.md), 이 문서 | 위임 범위에 따른 구현 준비 |
| G2 격리 | **대기 — 사용자 권한 필요** | 새 worktree·Go module·기준선 hash·root 변경 allowlist | offline vertical slice |
| G3 최소 실행 | 미착수 | fake Claude argv/env/cwd·loopback lifecycle·cleanup | protocol 구현 |
| G4 기본 wire | 미착수 | text/tool/JSON/SSE/error/cancel offline P/S + limit registry 확정 | native synthetic 통합 |
| G5 host parity | 미착수 | 선택한 native 기능·환경·permissions·worktree 증거 | real backend 검증 계획 확정 |
| G6 transport 안전 | 미착수 | auth synthetic·attempt cap·retry·leak·process boundary | **+ 사전 출력 상한 전송 계약** 이 있어야 예산 요청 가능 |
| G7 live integration | 미착수 | 명시적 cap 안의 실제 버전 조합 검증 | release 후보 판단 |
| G8 package | 미착수 | build provenance·설치·반복 실행·rollback·문서 | 기본 전환 판단 요청 |
| G9 기본 전환 | 미착수 | 사용자 승인·정확한 artifact·target 확인 | 새 실행의 기본 binary 변경 |
| G10 선택적 archive | **DEFERRED** | [MIGRATION.md](MIGRATION.md) 6장 M3. 권고는 하지 않음 | 승인된 구조 정리 |

G7 통과가 G9 승인을 뜻하지 않는다. CI가 초록이라는 사실만으로 사용자 설치를 교체하지 않는다. `READY_FOR_USER_DECISION`과 `RELEASED`를 분리한다.

## 5. 작업 패키지와 테스트 ID 대응

테스트 ID 정의는 원본 핸드오프 23장에 있다. 게이트 종류: `P` core 제품 필수, `S` 안전 필수, `C` 해당 capability 선언 시 필수, `E` 별도 관리.

| WP | 범위 | 우선 테스트 |
|---|---|---|
| WP00 | 기준선·인벤토리·설계 정합성 | **완료** — REL01(738/738, 0/0/0/0) |
| WP01 | Go workspace, launcher skeleton, fake child | **완료** — 아래 5.1 |
| WP02 | ephemeral HTTP·생명주기 | **완료** — 아래 5.3 |
| **WP03** | 최소 text request/response protocol | WIRE01–WIRE10, WIRE12–WIRE15 |
| WP03 | 최소 text protocol | WIRE01–WIRE10, WIRE12–WIRE15 |
| WP04 | tool round-trip·delivery barrier | TOOL01–TOOL08, LIFE10, WIRE11 |
| WP05 | direct transport·read-only auth | AUTH01–AUTH08, LIFE08–LIFE10, LIFE13, REL12 |
| WP06 | native host compatibility | ENV04–ENV08, ENV10, TOOL09–TOOL11, CAP04–CAP10, ARG06–ARG07 |
| WP07 | capability 확장 | HTTP08–HTTP10, TOOL12–TOOL16, CAP01–CAP03, CAP12 |
| WP08 | Windows·자원 안정성 | ARG09–ARG10, LIFE04–LIFE05, LIFE11–LIFE17, REL06–REL07 |
| WP09 | live validation·패키징 | 해당 P/S의 live 연계, REL04–REL10. G7–G9 구분 |
| WP10 | 선택적 archive | REL01, REL09, REL11. **DEFERRED** |

한 번에 모두 착수하지 않는다. 다음 하나는 WP03다.

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
| ARG09 실제 native exe·shim quoting | `NOT_RUN` | 현재 fixture는 Go에서 Go로의 왕복이다. 아래 5.2 |
| REL02 Node source hash 대조 | `NOT_RUN` | manifest는 있으나 대조 harness를 아직 만들지 않았다 |

### 5.2 argv fixture의 알려진 한계

가짜 native client는 테스트 바이너리를 재실행한 것이다. 디스크의 실제 실행 파일이고 실제 Windows 커맨드라인을 받으므로 프로세스 생성 왕복은 진짜다. 다만 **양쪽 끝이 Go**라서 측정하는 것은 Go의 quoting 대 Go의 parsing이다.

다른 규칙으로 커맨드라인을 파싱하는 native 바이너리는 이 fixture가 닿지 못한다. 그것이 ARG09이며 `NATIVE_SYNTH` 수준의 질문이다. WP01의 통과를 "실제 claude.exe에서 argv가 보존된다"로 읽지 않는다.

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
| HTTP07 임의 redirect 거부 | `NOT_RUN` | upstream 클라이언트가 없다. WP05 |
| HTTP08 `/v1/models` query·cache·picker | `NOT_RUN` | discovery 미구현. WP07 |

### 5.4 WP02에서 고친 실제 결함 하나

`Close`가 30초 deadline을 다 쓰고 실패했다. **context 취소는 blocking body read를 풀지 못한다** — handler는 `io.Copy` 안에 있고 `Shutdown`은 그 handler를 기다린다. Node 기준선은 소켓을 destroy해서 이 지점을 통과한다(`readBody`의 `resetAndDestroy`). Go의 대응물은 read deadline이고, `http.ResponseController`로 취소 시 즉시 만료시킨다. 같은 메커니즘이 body 완료 상한(300s, 기준선과 동일)도 함께 맡는다.

`Shutdown`이 그래도 실패하면 `server.Close()`로 강제 해제하되, **원래 실패를 성공으로 덮지 않고 그대로 보고한다.**

## 6. 기존 Node 증거와의 대응

새 테스트 ID는 **새 요구**이며 기존 증거가 자동으로 그것을 만족시키지 않는다. 다만 기존 증거가 **회귀 사례의 출처**가 되는 자리는 명시한다.

| V2 요구 | 기준선의 대응 증거 | 어떻게 쓰는가 |
|---|---|---|
| TOOL05/TOOL06 (전달 전 부작용 0, 전달 후 replay 0) | [현행 검증표](../remaining-verification.md) "내용 전달 후 재시도 금지" 행, `test-native-gateway` 재시도 울타리 2건 | 울타리를 제거하면 실패한다는 관측이 있다. 같은 자극을 Go fixture로 옮긴다 |
| WIRE09/WIRE10 (terminal 누락·중복·trailing) | "빈 text done 처리·불일치 이벤트 진단" 행 — delta 없는 빈 `response.output_text.done` 실패를 실제·합성 재현 후 수정 | 그 재현 입력이 Go SSE parser의 첫 회귀 fixture다 |
| WIRE12 (reasoning 뒤 최종 text·JSON 순서) | "headless 최종 텍스트·도구 축소 후 이력 보존" 행 — 빈 JSON 결과를 실제 재현·수정 | 같은 |
| AUTH01–AUTH06 | `src/test-credential-recovery.mjs` 23개 검사, `secretFieldsAbsent` 전행 true, `actualCredentialReads` 0 | 오류 분류 이름(`UNAUTHENTICATED` `CREDENTIAL_ACCOUNT_CHANGED` `REQUEST_BUDGET`)을 계약으로 이관 |
| LIFE08/LIFE09 (429·Retry-After) | `src/retry-after.mjs` `src/retry-after-seconds.mjs` 런타임 closure 2개 | header parsing·defer 계약을 입력/출력 단위로 이관 |
| CAP04 (기본 주입 0) | `src/native-gateway.mjs:347`, `src/clauduct.mjs:343` | 등록 없는 경로가 이미 동작한다는 관측. V2의 기본 모드 정의 |
| REL09 (문서 인용) | `verification/test-doc-citations.mjs` PASS | **V2 문서도 이 게이트를 통과해야 한다.** 미래 경로 때문에 검사를 약화시키지 않는다 |

**기존 Node의 버그까지 정답으로 복제하지 않는다.** 위 표는 회귀 사례의 출처이지 정답의 정의가 아니다.

## 7. 비교 실행 계약

### 7.1 세 실행기

| 실행기 | 목적 | 주의 |
|---|---|---|
| Native Claude + synthetic Anthropic fixture | 원래 host의 argv/settings/tool/permission/UI 동작 | 실제 Claude 모델 정답 비교가 아니다 |
| 기존 Node Clauduct | 현재 protocol·오류·정리 계약 기준 | 기존 버그를 정답으로 고정하지 않는다 |
| 새 Go Clauduct | 새 경계의 결과 | 같은 scenario·oracle·격리 fixture |

Node baseline의 실행 명령은 추정하지 않는다. `.github/workflows/tests.yml`에서 확인한 실제 명령은 기준선 JSON의 `node_baseline_entrypoints`에 있다. 비교 실행은 PATH에서 우연히 발견한 `clauduct`를 쓰지 않는다.

각 실행기는 동일 seed와 fixture manifest를 쓰되 **별도의 복제된 임시 작업 디렉터리**를 쓴다. 앞선 실행의 파일 수정·cache·session·token이 뒤 실행에 영향을 주지 않게 한다.

### 7.2 oracle

argv/env/cwd · protocol · filesystem · process · network · budget · privacy · lifecycle 여덟 종류를 둔다. 자연어 답변 점수를 host parity의 유일한 근거로 쓰지 않는다. live 비교는 모델 비결정성을 감안해 exact text 일치 대신 계약·도구 결과·안전 조건을 평가한다.

정규화는 timestamp·임시 경로 prefix·난수 ID처럼 의미 없는 차이에만 적용한다. **permission 선택, tool arguments, 누락된 field, 실행 순서, route와 부작용은 정규화해서 지우지 않는다.**

### 7.3 결과 분류

```text
MATCH · EXPECTED_DELTA · REGRESSION · UNSUPPORTED_DECLARED · UNKNOWN · BLOCKED · HARNESS_DEFECT
```

역할별 effort 배정 상실은 `EXPECTED_DELTA`다([COMPATIBILITY.md](COMPATIBILITY.md) 2장). 반면 **보안 필터 축소에 따른 secret 상속 범위 확대는 별도 신뢰 경계 검토 없이 개선으로만 표기하지 않는다.**

### 7.4 Node를 offline으로 돌릴 수 없을 때

기존 exported fixture entry나 검증 harness를 먼저 찾는다. production entry가 무조건 실제 auth를 읽거나 외부 호출한다면 무작정 실행하지 않는다. **Node production 코드를 수정해 가짜 기준선을 만들지 않는다.** full end-to-end가 불가능한 부분은 `NOT_RUN`과 이유를 기록한다.

## 8. CI

기존 workflow의 trigger(`push: [main]`, `pull_request`)와 Windows-only runner, 40분 timeout, Node 24를 그대로 유지한다. 새 branch를 만들었으니 CI가 자동 실행됐다고 가정하지 않는다.

| Job | 내용 | 신규 |
|---|---|---|
| Node baseline | 기존 3단계 (test runner / standalone / PowerShell) | 유지 |
| Go static | gofmt diff, vet, build | 신규 |
| Go unit/contract | synthetic fixtures·JSON/SSE·auth·budget | 신규 |
| Go race | 지원 runner의 race tests | 신규 |
| Windows integration | argv/console/process/loopback | 신규 |
| Docs | `verification/test-doc-citations.mjs` + V2 문서 | 유지·확장 |

Go 실패를 Node 릴리즈 상태와 섞지 않되, V2 승격에는 Go gate가 필수다. CI에 사용자 real credential을 넣지 않는다. 모델 호출이 필요한 job은 명시적 승인과 cap 없이 활성화하지 않는다.

**기존 timeout을 건드리지 않는다.** `verification/DotnetHttpProbe.cs`에는 의도적으로 긴 대기가 있다. 모든 검증에 짧은 공통 timeout을 씌워 정상 테스트를 실패로 만들거나, 실패가 사라질 때까지 늘리지 않는다.

## 9. evidence record

각 실행은 다음을 남긴다.

```text
scenario ID · requirement ID · implementation/build commit · baseline commit
runtime versions + OS/arch · fixture hash/provenance · command + 비밀 없는 effective settings
start/end + timeout policy · exit status · real upstream attempts · assertion results
raw artifact local location · redaction status · verdict + limitations
```

원자료는 ignored run directory에 둔다. tracked 보고서에는 재현에 필요한 비밀 없는 metadata와 요약만 남긴다. 대형 로그·transcript를 통째로 Git에 넣지 않는다.

**어떤 테스트를 생략했는지와 이유를 기록한다.** "토큰 절약"을 이유로 실제 실행 없이 PASS를 쓰지 않는다. 미실행은 `NOT_RUN`이고, S가 막은 검사는 재시도가 아니라 시도·블로커·실행된 검사·잔여 위험을 보고한다.
