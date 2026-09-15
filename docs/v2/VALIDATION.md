# V2 검증 계획 — 요구·테스트·게이트·예산

## 1. 현재 증거 상태

| 항목 | 값 |
|---|---|
| 실행한 V2 Go 테스트 | **585개 통과** (subtest 포함), 11 package |
| mutation 검증 | **153건 주입** (battery 8개). 현재 전부 잡힌다. 처음 주입 때 살아남은 것은 각 WP 절에 기록했다 |
| 실모델 호출 | **0회.** 다만 WP05부터는 구조가 아니라 **경로 제한**이다 — 아래 |
| 잔여 승인 예산 | **0. 그리고 별도로 BLOCKED다** — 3장 |

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
| `go test -race` | **`NOT_RUN`** | 이 머신에 cgo·C 툴체인이 없다(`-race requires cgo`, gcc 부재). CI 워크플로가 담당한다. **미실행은 통과가 아니다** |
| Go CI 워크플로 | **`NOT_RUN`** | `.github/workflows/go.yml`을 작성했으나 실행된 적이 없다. push는 별도 승인 사항이라 하지 않았다 |
| 대화형 세션 | `NOT_RUN` | 제품 빌드에 실제 전송이 연결돼 있지 않다. 연결은 G7 사항이다 |
| `clauduct-dev probe --send` | **`NOT_RUN`** | 구현·테스트 완료. 실행하면 luna/low로 **실제 요청 2회**를 보낸다. 사용자 재확인 전까지 돌리지 않았다 |
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
| HTTP07 임의 redirect 거부 | PASS | WP05. `CheckRedirect`가 항상 거부한다. 301·302·303·307·308 각각에 대해 **redirect 대상 서버가 요청을 0건 받았음**을 확인 |
| HTTP08 `/v1/models` query·cache·picker | `NOT_RUN` | discovery 미구현. WP07 |

### 5.4 WP02에서 고친 실제 결함 하나

`Close`가 30초 deadline을 다 쓰고 실패했다. **context 취소는 blocking body read를 풀지 못한다** — handler는 `io.Copy` 안에 있고 `Shutdown`은 그 handler를 기다린다. Node 기준선은 소켓을 destroy해서 이 지점을 통과한다(`readBody`의 `resetAndDestroy`). Go의 대응물은 read deadline이고, `http.ResponseController`로 취소 시 즉시 만료시킨다. 같은 메커니즘이 body 완료 상한(300s, 기준선과 동일)도 함께 맡는다.

`Shutdown`이 그래도 실패하면 `server.Close()`로 강제 해제하되, **원래 실패를 성공으로 덮지 않고 그대로 보고한다.**

### 5.5 WP03 — 완료

text 경로가 끝에서 끝까지 동작한다. `POST /v1/messages`는 501을 돌려주지 않는다: 요청을 해독하고, backend 요청으로 변환하고, transport로 실행하고, 돌아온 SSE를 파싱해 Anthropic 프레임으로 내보낸다.

**다만 제품 빌드에 transport가 연결돼 있지 않다.** `upstream.None`이 들어가 있어 모든 추론 요청이 `NO_UPSTREAM_TRANSPORT`(400)로 끝난다. WP05가 실제 전송을 만들었지만 **연결하지는 않았다** — 연결은 G7 사항이고, 지금 연결하면 모든 세션이 실호출 세션이 된다.

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
| WIRE13 느린 downstream backpressure | 부분 | 프레임 단위 flush는 있으나 느린 client 압력 실측은 없다. WP08 |
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

`/v1/messages`는 구현됐지만 **보낼 곳이 연결돼 있지 않다.** WP05가 실제 전송을 만들었고, 제품 빌드는 여전히 `upstream.None`을 쓴다. 연결은 G7이다.

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

**이것은 생성 상한이 아니라 사후 검사다.** 토큰은 이미 쓰였고, 거부는 "요청한 것과 다른 답을 건네지 않는다"는 의미밖에 없다. 3장의 BLOCKED가 말하는 "사전 출력 상한을 보장하는 전송 계약"이 없다는 것은 여전히 사실이고, WP05는 그것을 해결하지 않았다 — **정확히 기술했을 뿐이다.**

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

**아직 한 번도 `--send`를 붙이지 않았다.** 원장 누적 0회다. 실행은 사용자 결정 사항이다.

#### 5.8.6 이번 WP에서 지운 것

`SyntheticOnly` wrapper를 썼다가 지웠다. fixture transport는 credential을 읽지 않으므로 실제 credential이 거기 도달할 경로가 **없다.** 도달할 수 없는 경로를 지키는 wrapper는 일어날 수 없는 경우를 위해 유지보수할 코드다. 반대 방향 — 합성 credential이 실제 소켓에 가는 것 — 은 경로가 있으므로 `Direct`가 검사하고 테스트가 덮는다.

`Ledger.Reserve`가 돌려주던 `release` 클로저도 지웠다. 본문이 비어 있었다. 아무것도 하지 않는 것을 호출자가 반드시 호출해야 하는 구조는 의식(儀式)이다.

#### 5.8.7 이번 WP에서 실행한 Node 기준선

패키지별 직전 검증 정책에 따라, WP05가 계약을 가져온 파일들의 Node 테스트 17개를 돌렸다.

`test-additional-rate-limits` · `test-auth-owner-pipes` · `test-auth-owner-protocol` · `test-client-version` · `test-connection-fault` · `test-credential-recovery` · `test-credential-store-selection` · `test-fixture-token-budget` · `test-fixture-transport-progress` · `test-fixture-usage` · `test-http-close` · `test-http-retry-status` · `test-keepalive-transport` · `test-native-transport` · `test-rate-limit-headers` · `test-rate-limit-observation` · `test-transport-rejections`

**17개 전부 exit 0.** 기준선 worktree는 tracked 변경 0으로 남아 있다.
