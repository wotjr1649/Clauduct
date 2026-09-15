# Go V2 아키텍처 — 로컬 검증 후 확정본

핸드오프 5·7·12–21장의 설계를 기준선 `node-bfbdf23-g0`의 실제 코드와 대조한 결과다. 핸드오프와 다른 곳에는 이유를 붙였다. 상태·판정은 여기에 두지 않는다 — [README.md](README.md)와 [DECISION.md](DECISION.md)가 소유한다.

## 1. 실행 경로

```text
clauduct
  → cwd·argv·env 수집
  → claude.exe 해석 (platform)
  → 127.0.0.1:0 bind + session token 생성 (gateway)
  → readiness 확인
  → native Claude 실행 (stdio 상속, shell 없음)
  → native 종료 관찰
  → owned 요청 취소 → HTTP shutdown → owned child/socket 정리
  → native exit code + cleanup 결과를 분리해 반환
```

포트는 OS가 고른다. 빈 포트를 찾아 닫았다 다시 bind하는 race를 만들지 않는다. listener를 확보한 뒤 실제 bound address를 child에게 넘긴다.

## 2. 책임 경계

Clauduct가 소유하는 것은 **모델 이름/effort 호환, API envelope·SSE 변환, loopback 생명주기** 셋뿐이다. TUI·도구 실행·MCP·플러그인·스킬·사용자 훅·permissions·worktree·session/resume은 native Claude와 사용자가 소유한다. Codex 로그인과 credential 갱신은 사용자와 Codex 도구가 소유하며 Clauduct는 읽기 전용으로만 접근한다.

Go나 Codex가 Bash/Edit를 중복 실행하는 단계는 없다(D09). 예외는 backend 측 hosted search 하나이며, 일반 도구와 구분해 별도 capability로 설계한다.

## 3. 패키지 구조

```text
go/
├── go.mod                    module github.com/wotjr1649/Clauduct/go, go 1.27.0
├── cmd/clauduct-go/          제품 launcher (승격 전 이름)
├── cmd/clauduct-dev/         doctor / plan / compare
└── internal/
    ├── app/                  구성·생명주기 조립
    ├── launch/               native 실행 사양: argv/env/cwd
    ├── gateway/              loopback HTTP endpoints
    ├── protocol/{anthropic,codex,bridge}/
    ├── upstream/             interface + direct Codex transport
    ├── auth/                 읽기 전용 credential provider
    ├── routing/              모델·effort·capability registry
    ├── stream/               SSE parsing·emission·delivery state
    ├── platform/             *_windows.go 등 OS 경계
    ├── observability/        redacted diagnostics·bounded run record
    ├── buildinfo/
    └── testkit/              fake Claude/upstream·fault injection
```

**먼저 다 만들지 않는다.** 빈 package·빈 인터페이스 scaffolding은 하지 않는다. 첫 slice는 `launch` `gateway` `platform` `testkit` `buildinfo`만 만들고 나머지는 실제 책임이 생길 때 분리한다. 만능 `utils`/`manager`로 다시 합치지도 않는다.

`internal/` 아래를 뜻하며, module path에 `/v2` semantic-major suffix를 붙이지 않는다 — 제품 아키텍처 V2와 Go module major는 별개다. subdirectory module을 배포한다면 태그에 `go/` prefix가 필요하다(예 `go/v0.1.0`). 초기 배포는 검증된 binary artifact 우선이며 `go install` 지원은 별도 검증 후에만 선언한다.

## 4. CLI 계약

제품 런처는 native 인자를 그대로 넘긴다. Claude CLI shadow parser를 만들지 않는다(D05). `--help`·`--version`은 native 의미를 유지하며 Go 제품 정보는 `clauduct-dev version`에서 본다(D06).

옵션 값 안의 문자열을 재파싱하지 않는다. `--append-system-prompt "--model은 설명용 문자열이다"`에서 `--model`을 가로채면 안 된다. `--` 뒤 positional 영역도 원형 그대로 전달한다.

기준선의 `blockedOptions` 30개와 그 인과 분해는 [DECISION.md](DECISION.md) 2.6절에 있고, 정책 범주 10개의 개방 여부는 사용자 결정으로 남아 있다.

프로세스 시작마다 credential을 읽지 않는다. 실제 inference 요청 시점의 lazy loading을 쓴다. 그래야 native help/version이 로그인 부재로 막히지 않는다.

## 5. 환경·설정 overlay

최소 후보는 session-local `ANTHROPIC_BASE_URL`, loopback 인증 token, 필요한 gateway discovery 설정이다. 실제 native 버전에서 endpoint·credential 우선순위를 검증해 필요한 것만 더한다.

| 변수군 | 처리 |
|---|---|
| Anthropic endpoint·인증 선택 | gateway 연결로 일관되게 정리. 원래 Anthropic credential이 gateway·Codex로 새지 않게 한다 |
| `GITHUB_TOKEN`·AWS/service key·MCP secret | 무차별 삭제하지 않는다. native child 환경에서만 보존 |
| `CLAUDE_CONFIG_DIR` | 사용자의 명시적 선택을 보존. 기본 별도 프로필을 강제하지 않는다 |
| proxy·CA 설정 | 신뢰된 사용자 네트워크 구성으로 취급. TLS 검증 완화는 하지 않는다 |
| bridge 내부 run-id·port·token | 자기 프로세스와 child에 필요한 범위만 |

기존의 광범위 secret 제거를 축소하면 MCP 호환은 개선되지만 **child에 보이는 secret 범위가 넓어진다.** 이를 보안상 동일한 동작으로 표현하지 않는다.

기본 모드에서 system prompt 교체, agent 목록 주입, picker 교체, permission 우회, telemetry 변경, context window 확대, resume policy 변경을 하지 않는다. 기준선이 지금 하고 있는 주입 3키·agent 14개·hook 3이벤트는 V2 기본 경로에 없다([DECISION.md](DECISION.md) 2.5·2.7).

## 6. HTTP façade

| Endpoint | 처리 | 성공 주장 조건 |
|---|---|---|
| `POST /v1/messages` | 핵심 지원 | stream·tool·error·cancel 계약 검증 |
| `/v1/messages`의 query | 지원 native query 보존·검사 | `?beta=true` 등 실제 요청 fixture |
| `GET /v1/models` | 로컬 검증 catalog | query·paging·auth·timeout·picker 상호작용 |
| `HEAD /api/hello` | 최소 readiness 응답 | 인증 없이도 비밀·상태 노출 없음 |
| `/v1/messages/count_tokens` | 별도 capability | 기본 지원 선언 금지 |
| agent registration | **기본 실행의 필수 조건 아님** | optional overlay에서만 |
| 기타 | 명확한 unsupported 응답 | 침묵 성공·임의 upstream forwarding 금지 |

기준선의 4번째 endpoint `POST /clauduct/agents`는 V2에서 overlay 전용이다.

listener는 `127.0.0.1:0`에만 bind한다. 세션마다 충분히 긴 난수 token을 만들고 비교는 timing leakage를 줄인다. token을 커맨드라인·일반 로그·오류에 표시하지 않는다. Host·Origin·method·content type·payload size를 검증하고, browser-origin·잘못된 인증·임의 target URL·cross-session token 재사용의 거부 테스트를 만든다. 같은 OS 사용자에게 process 환경을 숨기는 sandbox라고 주장하지 않는다.

native 버전별로 `/v1/models` 요청에 인증 header가 둘 이상 실릴 수 있다. 모든 auth 값을 검증하되 **같은 loopback token이 두 header에 실렸다는 이유로 정상 discovery를 깨뜨리지 않는다.** 서로 다른 credential을 허용하거나 upstream으로 전달해서는 안 된다.

`ANTHROPIC_BASE_URL`을 바꿨다는 사실만으로 Claude의 모든 통신이 gateway를 통과한다고 선언하지 않는다. 지원하는 모델 추론 요청만 route trace로 확인한다. native 서비스 점검·WebFetch domain safety·플러그인·MCP의 별도 네트워크는 별도 범주다. Go V2는 OS 수준 egress sandbox가 아니다.

## 7. 프로토콜 변환

거대한 범용 canonical framework를 만들지 않는다. Messages 입력 ↔ Codex wire ↔ Codex events ↔ Claude 출력 사이의 명시적 변환만 둔다. protocol DTO와 domain state를 섞지 않는다.

외부 JSON을 `map[string]any`로 끝없이 넘기지도, 모르는 필드를 전부 버리는 struct decode를 쓰지도 않는다. known envelope는 타입으로, schema·tool arguments·opaque content는 검토된 `json.RawMessage`와 presence 정보로 다룬다.

| JSON 구분 | 요구 |
|---|---|
| 없음 vs `null` | 동치로 처리하지 않음 |
| 빈 배열/객체 vs 없음 | canonicalization으로 합치지 않음 |
| 숫자 | 식별자·큰 정수의 정밀도 손실 금지. 임의 float64 변환 금지 |
| 중복 key | 의미에 영향을 주는 ambiguous 입력은 명확히 거부 |
| UTF-8 | 깨진 byte를 자동 대체해 다른 입력으로 실행하지 않음 |
| trailing JSON | 두 번째 객체·추가 payload를 무시하지 않음 |
| schema defaults | validator가 원문 arguments를 자동 변경하지 않음 |
| remote `$ref` | 임의 네트워크 조회 금지 |
| 대소문자 | tool 이름·field를 case-insensitive로 정규화하지 않음 |

Go `encoding/json`의 기본 동작이 위 요구와 같다고 가정하지 않는다. 선택한 Go 버전과 JSON API의 차이를 테스트로 고정한다.

text·system instruction·role·image/document·tool 정의·tool_use/tool_result·tool 오류·content block 순서·structured output·reasoning opaque·cache hint·compaction 요청을 분류한다. 전부 초기 지원할 필요는 없지만 **silent drop은 금지**다. 의미를 바꾸는 필드를 지원 못 하면 명확한 capability 오류를 반환한다.

도구 arguments를 임의로 보정하지 않는다. 다음 셋은 자동으로 같은 뜻이 되지 않는다.

```json
{}
{"isolation": null}
{"isolation": "worktree"}
```

optional enum을 강제로 채우거나 의미 있는 사용자 선택을 "default 정리"로 지우지 않는다. upstream의 strict schema에 맞춘다는 이유로 optional field를 required로 만들거나 임의 enum/default를 추가하지 않는다.

## 8. SSE·전달 barrier

```text
RECEIVED → INPUT_VALIDATED → ROUTE_RESOLVED → ATTEMPT_RESERVED → UPSTREAM_ACTIVE
  → TEXT_STREAMING / TOOL_BUFFERING → COMPLETION_VALIDATED → RESPONSE_DELIVERED → CLOSED

어느 단계에서든: CANCELLED | FAILED_BEFORE_DELIVERY | FAILED_AFTER_COMMIT | CLEANUP_FAILED
```

`COMPLETION_VALIDATED`와 `RESPONSE_DELIVERED`, `tool delivered`와 `tool executed`는 별개다. 서버는 response write 성공만으로 Claude가 실제 도구 부작용을 완료했는지 알 수 없다.

text는 검증된 event 범위에서 streaming할 수 있다. **부작용을 유발할 수 있는 `tool_use`는 completion 검증 전에 내보내지 않는다.** 초기 Go는 요구되는 terminal 조건과 해당 HTTP 응답 본문의 정상 종료까지 확인한 뒤 도구를 전달하는 보수적 안을 쓴다. keep-alive TCP 연결 자체의 종료를 기다리라는 뜻은 아니다. 이 선택의 지연과 취소 동작을 fixture로 검증한다.

이 설계가 보장하는 것은 **검증 전 tool 전달 방지와 bridge의 중복 전달 억제**다. 외부 도구의 exactly-once 실행을 보장한다고 쓰지 않는다.

SSE parser는 byte 경계가 UTF-8 문자·JSON token·CRLF·빈 줄 가운데에서 끊겨도 동작해야 한다. 64 KiB를 넘는 합법 event를 기본 scanner limit 때문에 자르지 않되 무제한 buffer도 허용하지 않는다. frame·event count·총 bytes·최대 depth·idle·총 요청 budget은 명시적 configuration schema로 관리하고 단위를 구분하며 overflow를 검사한다.

terminal 이벤트·`[DONE]`·중복 완료·완료 이후 trailing data·비정상 EOF의 의미를 backend 계약으로 고정한다. terminal을 봤다는 이유로 뒤의 프로토콜 위반을 정상 처리하지 않는다.

다운스트림 ping은 연결 유지용일 뿐 모델 진전의 증거가 아니다. connect/header/idle/overall/user-cancel timeout을 분리한다. HTTP global `WriteTimeout` 하나로 긴 SSE를 자르지 않는다. `ResponseWriter`는 한 소유자가 관리하고, 느린 client를 위해 무제한 event queue를 만들지 않는다.

## 9. retry

첫 slice에서 gateway 내부 자동 retry는 0이다. 단일 시도 계약을 먼저 검증한다. 제품 후보에서는 native retry와 gateway retry가 곱해지지 않도록 실제 trace로 소유권을 명시한다.

| 상태 | 정책 |
|---|---|
| upstream socket 전 명확한 사전 실패 | 전체 예산 안에서 재시도 검토 |
| 시도 시작, downstream 미전달 | 상태·실패 종류를 확인한 제한 retry만 |
| 의미 있는 응답 전달 이후 | **자동 replay 금지** |
| 전달 여부 불명확 | 성공으로 간주도, 자동 재실행도 하지 않음 |
| 401 | 같은 account의 읽기 전용 재확인. 무한 반복 금지 |
| 403·정책 거부·TLS 검증 실패 | 우회·자동 credential 교체 금지 |
| 429·일시적 5xx | Retry-After·총 예산·시도 제한·취소를 함께 적용 |
| 긴 Retry-After | delay를 보존해 deferred 보고. 임의 조기 retry 금지 |
| user cancel | retry 금지 |

"text가 아직 없다"만으로 재시도 안전성을 추정하지 않는다.

## 10. upstream·인증

초기 backend는 기준선의 direct Codex 경로를 Go로 재구현한 것 하나다. 제품 runtime에서 Node adapter를 실행하지 않는다. `codex exec`나 app-server를 동시에 넣지 않으며, 아직 쓰지 않는 다중 backend framework를 만들지 않는다.

```text
CredentialProvider  필요 시점에 읽기 전용 조회 / account 일관성 검사 / 고정 오류 분류만 반환
Upstream            검증된 요청 실행 / 검증 가능한 event stream / context 취소 수용 / owned connection 정리
AttemptBudget       socket 열기 전 시도 예약 / main·search·retry 공통 cap / 미승인 실호출 차단
RouteRegistry       requested와 effective를 분리 기록 / capability 판정
```

credential을 일반 JSON 로그 구조에 넣지 않는다. `String()`·error wrapping·디버거 dump가 비밀을 출력하지 않게 한다. Go GC 환경에서 비밀 메모리가 즉시 완전히 지워진다고 주장하지 않는다.

backend target은 신뢰된 제품 설정으로 고정한다. 프로젝트 파일·모델 출력·임의 HTTP header가 credential 전달 목적지를 바꿀 수 없게 한다. redirect 시 credential이 다른 origin으로 가지 않게 제한한다. TLS 검증을 끄거나 보호 proxy를 우회하지 않는다.

fake upstream은 synthetic credential만 받는다. production credential이 loopback fixture나 테스트 로그에 들어오면 실패해야 한다. 제품의 일반 native 인자로 실제 backend URL을 바꿀 수 없게 한다.

실호출 cap이 HTTP attempt 기준인지 logical inference 기준인지 명시하고 둘 다 기록한다. 검색·retry·보조 모델 요청을 누락하지 않는다. 허용되지 않은 추가 요청은 socket을 열기 전에 거부한다. 토큰·비용·캐시 절감은 관측값만 보고하며, 값이 없으면 `unknown`이지 0이 아니다.

## 11. Windows 프로세스

가능하면 실제 `.exe`를 직접 실행한다. shell 문자열 조립으로 user argument를 연결하지 않는다. npm `.cmd`만 발견되면 설치 방식을 식별해 검증된 adapter를 쓰거나 지원 한계를 알린다. 어떤 `.cmd`든 내용을 대충 파싱하거나 `cmd /c`에 인자를 이어붙이는 fallback은 금지한다. cwd에서 우연히 발견한 동명 프로그램을 신뢰된 설치로 취급하지 않는다.

기준선의 실행 파일 해석 계약(`src/runtime-paths.mjs`)은 Windows 전용이다 — PATH를 `;`로 나누고, PATH 항목 중 cwd와 같은 것을 제외하고, 절대경로만 받고, 64개로 제한하며, npm shim은 `package.json`의 `name`/`bin` 일치까지 확인한 뒤에만 Node 경유로 실행한다. 이 다섯 가지 방어는 Go에서도 유지한다.

기본 interactive 실행은 native console과 stdin/stdout/stderr를 상속한다. 새 PTY를 만들어 native UI를 재구현하지 않는다. `Esc`·`Ctrl+C`·prompt 편집·취소가 baseline과 맞아야 한다. `Ctrl+C`를 무조건 parent 종료로 해석하는 구현도, 모든 signal을 무시하는 구현도 금지한다. 같은 console event를 중복 전달해 도구·세션을 두 번 취소하지 않는다.

Job Object 등 OS 수단으로 **이 실행에서 소유한 process tree만** 정리한다. 이름이 `claude`나 `codex`인 모든 프로세스를 종료하지 않는다. 자식이 손자를 만들기 전 소유권 확보 race, 기존 job 안에서 실행되는 경우, nested job 제약, IDE terminal·보안 제품의 권한 거부를 검증한다. 지원 불가 조합은 정확히 보고하고 정책 우회로 해결하지 않는다.

```text
새 요청 admission 중단 → owned in-flight 취소 → native 종료 상태 수집
  → 제한된 HTTP shutdown → 남은 owned socket/child 정리 → run metadata flush
  → native 결과 + cleanup 결과 확정
```

child `Wait`만으로 모든 손자가 종료됐다고 단정하지 않는다. native exit code를 가능한 한 보존하고, 정리 실패가 원래 실패를 덮어쓰지 않게 한다. headless stdout에 bridge 진단을 섞지 않는다.

## 12. 자원·상태·로그

활성 요청·연결·대기열·개별 frame·총 응답·로그·종료 대기에 명시적 상한을 둔다. goroutine이 가볍다는 이유로 무제한 작업을 만들지 않는다. 각 자원에 생성자·취소 원인·해제 조건·테스트가 있어야 한다. G4 이전에 limit registry의 단위·값·초과 동작·테스트를 모두 확정하며, 필수 제한을 `unknown`이나 무제한으로 둔 채 제품 후보로 승격하지 않는다. 높은 동시성 × 큰 요청 크기가 프로세스 memory budget을 넘지 않도록 admission을 설계한다.

로그에는 timestamp·run/request ID·고정 오류 분류·byte 수·소요 시간·지원 버전·비밀이 아닌 모델 식별자를 남긴다. prompt 원문·tool arguments·파일 내용·credential·cookie·전체 URL query·환경 dump는 남기지 않는다. 서버에서 온 문자열을 무제한 metric label로 쓰지 않는다. rotation과 run별 총 크기 제한을 두고, 만료·삭제는 자기가 만든 run 자료에만 적용한다.

**제품 상태는 설치 디렉터리에 두지 않는다.** OS가 제공하는 사용자별 state/cache 경로 아래 독립 namespace에 둔다. Go API가 반환하는 경로와 사용자 권한을 확인하고 OS 이름만으로 문자열 조합하지 않는다. 기준선의 `.clauduct-status`와 V2 자료를 자동으로 합치지 않는다. 동시 실행한 두 세션은 서로의 token·port·로그·cleanup을 공유하지 않는다. crash 후 정리도 run 소유권을 검증하고 진행한다. (요구 V2-01)

## 13. 모델·agent·resume

`requested_model`·`effective_model`·`requested_effort`·`effective_effort`·`resolution_source`를 분리한다. 불명확한 요청을 저렴한 모델로 몰래 바꾸거나 effort를 silent clamp하지 않는다. unsupported이면 선택 가능한 범위와 이유를 보여준다. Claude alias mapping과 직접 Codex ID 사용을 구분한다.

기준선 catalog는 출발점이며 V2가 실제로 지원하는 조합을 별도 기록한다. hardcoded 최신 모델명을 추측하지 않는다.

기본 실행에서 Clauduct 전용 agent와 routing hook을 필수 주입하지 않는다(V2-03). 세션·agent·parent header가 있으면 correlation evidence로 쓰되, 신뢰된 권한 증명이나 모든 native 버전에 반드시 존재하는 ID로 취급하지 않는다.

```text
route sufficient + lineage unavailable
  → 일반 inference 가능, lineage 검증은 unavailable

route ambiguous for an explicitly requested managed feature
  → 해당 기능에 한정된 오류/제약
```

overlay에는 ID·목적·주입 키/훅·host 충돌 검사·제거 방법·테스트 ID가 있어야 한다. overlay는 기존 user hook을 치환하지 않으며, hook 실패가 기본 연결 자체를 깨뜨리는지 명시한다. **overlay를 끈 상태가 정상적인 제품 모드다.**

V1에서 만든 transcript를 V2가 무조건 재개할 수 있다고 선언하지 않는다. 완료 여부가 불명확한 tool 부작용을 resume 과정에서 재실행하지 않는다. context window·auto compact 수치를 backend capacity·native 처리 검증 없이 확대하지 않는다.

## 14. 의존성 정책

`net/http`·`os/exec`·`context`·`encoding/json`을 우선한다. 표준 라이브러리가 모든 의미 검증을 대신하지는 않는다 — Windows shell quoting·프로세스 트리 종료·JSON 중복 키는 별도 계약이 필요하다.

| 후보 | 허용 기준 |
|---|---|
| Windows syscall wrapper | Job Object·console 처리에 필요하면 유지보수되는 최소 의존성 허용 |
| TOML parser | 실제 지원 config 읽기에 필요할 때 검증된 parser. 정규식 자작 금지 |
| JSON Schema validator | 지원 dialect·범위를 명시하고 충분한 테스트가 있을 때 |
| logging framework | 기본 구조화 로그로 부족하다는 근거 없으면 추가하지 않음 |
| CLI framework | 제품 pass-through에는 쓰지 않음. dev CLI에 필요할 때만 |
| HTTP router | 표준 mux로 충분하면 추가하지 않음 |
| Codex/LLM SDK | 초기 direct wire contract에 불필요하면 도입하지 않음 |

"표준 라이브러리만"이라는 목표 때문에 credential parser나 schema validator를 불완전하게 자작하지 않는다. 라이선스·전이 의존성·보안 업데이트·버전 고정·공급망 비용을 기록한다.

기준선의 제3자 npm 의존성은 0이므로 재현해야 할 의존성 표면이 없다. 새 의존성은 전부 Go V2가 새로 만드는 비용이다.

모든 Go 코드는 `gofmt` 적용, `go vet`·타입 검사 통과. CGO 없는 production binary를 우선하되 필요한 native 기능은 검증한다. race 검사는 지원 runner에서 수행하고 미실행을 PASS로 표기하지 않는다. Windows 지원은 Windows 실행 증거가 있어야 한다. artifact에 commit·Go version·target OS/arch·빌드 명령·dependency 정보를 연결한다.

`.NET`·PowerShell·Python을 Go 제품의 필수 runtime으로 새로 요구하지 않는다. **Node도 마찬가지다** — 기준선이 `process.execPath`를 wire로 내보내는 두 곳([DECISION.md](DECISION.md) 2.3)은 포팅하지 않고 재설계한다(요구 V2-02). 사용자가 설치한 Claude/Codex가 npm shim이면 그 도구의 Node 의존은 별개다.

두 독립 build의 hash가 같다고 주장하려면 동일 toolchain·dependency·build flags·VCS metadata·입력 상태로 실제 재빌드 비교를 한다. `-trimpath`를 썼다는 사실만으로 재현 가능성을 선언하지 않는다. 서명·자동 업데이트는 별도 권한과 설계가 필요하며 기본 범위에 몰래 추가하지 않는다.
