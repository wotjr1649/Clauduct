# v0.3.1 무과금 보완 — 자동 판정과 로컬 거부 분류

2026-09-22. `fix/v031`, 기준 `149068e` 위의 기존 미커밋 작업을 보존했다.
추가 구독 backend 호출은 **0회**다. 앞선 [통합 원장](../v031-integration-20260921/evidence.json)의
40회 지출과 실패 기록을 초기화하지 않았다. 사용자가 활성화한 AdGuard 설정은 유지했다.

## 수정한 원인과 동작

### 전송 전 거부가 backend 장애로 분류됨

`ErrBudgetExhausted`와 `ErrRouteNotAuthorised`가 일반 오류로 빠져 `UPSTREAM_FAILURE`와
HTTP 502를 반환했다. 재시도로 해결되지 않는 로컬 제한을 재시도 가능한 서버 오류로
전달하는 결함이었다. [공통 분류](../../go/internal/gateway/messages.go)는 이제 각각
`REQUEST_BUDGET`, `ROUTE_NOT_AUTHORISED`와 HTTP 400을 반환한다. 계수 경로에도
`COUNT_TOKENS_FAILED_` 뒤에 원래 구분이 보존된다. 예약·전송 전 거부와 허용 라우트는 유지한다.

생성의 두 직접 오류·두 wrapped 오류와 계수의 두 오류를 먼저 검사했다. 수정 전 6개
대조군이 실제 assertion으로 실패했고, 수정 후 통과했다. wrapped 오류의 비공개 문맥 문자열이
응답에 노출되지 않는 조건도 유지한다. 실제 backend 장애의 기존 분류는 변경하지 않았다.

### 독립적인 루트 보조 요청과 대화 턴 게시가 경합함

로컬 실제 TUI 실행에서 보조 요청의 `AGENT_SELECTION_UNVERIFIED`를 관측했다.
현재 선택 경로는 도구 없는 루트 보조 요청도 현재 대화 턴 영수증을 읽었다. 새 영수증의
body만 있고 완료 표시가 아직 없는 상태를 만든 [독립 재현](../../go/internal/gateway/turn_receipt_test.go)에서
같은 거부를 확인했다. 실측 한 건의 파일 쓰기 시점을 별도 추적한 것은 아니며,
코드상 경합 조건의 재현과 실제 거부 관측을 구분한다.

자식·부모 ID가 모두 없고, 분류가 `auxiliary`이며, 도구·hosted search가 없는 요청만
대화 턴 선택에서 분리했다. 이 요청은 자식 선택·취소 영수증을 소유하지 않는다.
인증, 모델·effort 검증, 입력 검증, 비용 제한은 후속 경로에서 그대로 적용한다.
일반 대화·자식·Workflow·압축·부모 ID·도구·검색의 8개 대조군은 불완전 영수증을 계속 거부한다.
9개 대조군은 수정 후 통과했고, 수정 제거 overlay는 루트 보조 요청 assertion으로 다시 실패했다.

### TUI 성공을 횟수만으로 판정함

[공통 검수기](../../go/internal/app/integration_tui_test.go)는 최근 16건 대신 누적 feature
집계로 일반 생성 성공 3회, 압축 성공 1회, 생성 취소 1회를 따로 요구한다. 허용된 검증 예산의
거부도 로그에 남기며, 다른 오류는 성공으로 인정하지 않는다.

임시 profile의 native transcript를 읽어 같은 세션에서 다음 순서를 자동 검사한다.

1. assistant의 `PUBLIC_TUI_47` 출력.
2. native의 `compact_boundary`.
3. assistant의 `PUBLIC_TUI_47 blue 47 PUBLIC_COMPACT_DONE` 출력.
4. 생성 중단 기록.
5. assistant의 `PUBLIC_TUI_RECOVERED` 출력.

사용자 프롬프트에 표식이 있는 것만으로 통과하지 않는다. 파일은 `os.OpenRoot`로 임시 profile
안에 한정하고, transcript 2 MiB·행 1 MiB 상한과 고정 오류명을 사용한다. 원본 transcript,
프롬프트 전체, 오류 원문, 자격 정보는 검증 보고서에 저장하지 않는다.
누락·잘못된 사실·잘못된 순서·다른 세션·사용자 echo·손상·초과 크기·최근 기록 eviction·
압축을 일반 답변에 합산하는 오류 등을 포함한 15개 대조군이 통과했다.
transcript 검사를 제거하는 overlay는 10개 assertion으로 검출됐다.

## 실제 native TUI + 로컬 합성 응답

Claude Code 2.1.278, 실제 PTY, 제품 gateway·hook·native event 경로를 사용했다.
응답 공급자만 네트워크 연결과 credential provider가 없는 로컬 시험 구현이다.
이는 native 전달·취소·종료·검수기 검사이며 실제 모델의 요약 품질이나 계수 정확도 검사는 아니다.

| 실행 | 결과 |
|---|---|
| 최초 로컬 TUI, 83.50초 | 시험 응답 공급자가 developer 문자열 content를 배열로 해독하여 실패. 본문 형태 처리를 수정 |
| 두 번째 로컬 TUI, 197.23초 | 생성 전 대기 취소와 보조 요청의 선택 거부 관측. 엄격한 검수는 실패 유지 |
| 최종 로컬 TUI, 73.22초 | **PASS**. 부분 텍스트 전송 후 Esc, 같은 세션 후속 답변, transcript의 사실·순서, 정상 종료·정리 확인 |

최종 실행의 보조 요청 거부 4건은 `ROUTE_NOT_AUTHORISED`로 구분됐다. 모든 보조 모델 요청의
실제 성공을 의미하지 않는다. 검증용 Luna/low 제한을 제거하거나 강제로 라우팅하지 않았다.
승인된 `D:\AIDEV\clauduct-v031\.tmp\integration-temp` 아래 공개 합성 프로젝트만 임시 profile에
신뢰 등록했으며, 종료 후 해당 profile/project가 제거됐음을 확인했다.

## AdGuard 활성 상태에서 남은 반례

프로세스 `Adguard`, `AdguardSvc`가 존재했고 사용자가 필터 활성화를 확인했다.
[원래 raw TCP 검사](../../go/internal/gateway/socket_runtime_evidence_test.go)의 판정 조건을 유지했다.

| 실행 | 즉시 종료 200회 | 수신 ACK 후 종료 200회 |
|---|---:|---:|
| 최초 재현 | 실패 29 | 실패 17 |
| 오류 단계 계측 후 | 실패 19 | 실패 15 |

두 번째 실행의 즉시 종료 실패 19건은 수신 측 `errno_10054`와 payload 불일치였다.
ACK 방식 15건은 수신 본문이 모두 일치했지만 서버의 ACK 수신에서 `errno_10054`가 발생했다.
즉 ACK 방식 실패를 본문 유실 15건으로 해석하지 않는다. 이 검사는 Clauduct·native 없는
표준 라이브러리 loopback 반례다. 과거 on/off 대조와 함께 필터 활성 시 연결 종료와 reset의
관련성을 지지하지만, driver 내부 구현까지 특정한 근거는 없다.

이 실패를 지우거나 timeout 확대·재시도·linger 추가·필터 제외로 통과시키지 않았다.
최종 로컬 TUI PASS와 raw TCP FAIL을 각각 기록하며 **전체 무오류 판정은 HOLD**다.

## 승인 후 HTTP 검사 복구 실행

사용자가 두 standalone 명령의 복구 실행을 승인했다. `node --test` 대신 각 파일을
직접 실행했으며 runtime guard, timeout, 전송 방식과 통과 조건은 변경하지 않았다.
이전 [통합 검수](../v031-integration-20260921/REPORT.md)의 guard 거부 이력은 보존한다.
현재 상태는 승인 대기나 guard 차단이 아닌 **실행한 전송 검사 FAIL**이다.

| 실행 | 결과 |
|---|---|
| `node verification/test-http-transport.mjs` | exit 1, 0.428초. `read ECONNRESET`; 원래 출력에는 실패 사례·클라이언트가 없음 |
| `node verification/test-dotnet-http-transport.mjs` | exit 1, 3.775초. 첫 `sse-fixed` 사례가 `RESPONSE_TRUNCATED`, 사례 소요 883ms |
| Node 사례·클라이언트 진단 추가 후 실행 | exit 1, 0.425초. `sse`의 `node-fetch`가 `ECONNRESET`; 앞선 `node-http` 사례 1개 PASS |

Node 검사는 실패 시 고정 사례명·클라이언트·진행 횟수를 출력하도록 보완했다.
원래 오류를 다시 던지므로 실패 exit와 정리 경로는 유지한다. 새 진단은 helper 5개 통과,
loopback 1개 통과, 서버 수신 2건, 잘못된 요청 0건을 기록했다. 뒤의 미실행 사례를
PASS로 세지 않는다. .NET은 첫 loopback 전에 parser/self-test 검사를 통과했지만,
35개 loopback 전체의 완료 결과는 생성되지 않았다.

두 검사는 Clauduct와 실제 backend를 거치지 않고 `127.0.0.1`의 합성 SSE만 사용한다.
Node v24.19.0에서 실행했고 외부 요청·자격 정보 읽기는 0건이다. 검사 프로세스 및
PowerShell worker가 남아 있지 않음을 확인했다. 두 서버는 요청 본문 수신 완료 후
정상 사례의 길이를 명시하고 `Connection: close`로 응답한다. 이번 Node 반례는
유효한 요청을 서버가 받은 뒤 fetch가 응답을 확보하기 전에 reset된 사례다.

앞선 raw TCP 반례 및 [과거 필터 on/off 대조](../release-investigation-20260920/REPORT.md)와
함께 로컬 전송 환경의 문제를 지지한다. 이번 HTTP 실패 자체의 필터 on/off 대조나
packet/driver trace는 없으므로 특정 driver 결함으로 확정하지 않는다. AdGuard 설정,
global proxy, 설치 런타임을 바꾸지 않았고 성공할 때까지 반복 실행하지 않았다.

## 잔여 검증 경계

- tool-output 이미지의 warmup 2,828 / 생성 usage 3,525 불일치는 앞선 실측 그대로 남는다.
  제품은 해당 입력의 정확 계수를 거부하며 일반 생성을 독립 처리한다. 로컬 검사로 실제
  provider의 계산을 고쳤다고 주장하지 않는다. 추가 과금 금지에 따라 실호출 재검사는 하지 않았다.
- 승인된 Node HTTP 검사 2개는 위와 같이 전송 단계에서 실패했다. 전체 Success에는
  로컬 reset 원인을 해소한 환경에서 Node 31개·.NET 35개 loopback 검사를 끝까지 통과한
  근거가 필요하다. raw TCP 반례도 별도로 해결 여부를 확인해야 한다.
- Node V1 factory probe의 기존 skip, #50의 미관측 조건, #52 junction 경계,
  v0.3.0 새 선택 journal의 거부는 별도 범위이며 실패를 삭제해 지원으로 바꾸지 않았다.

## 최종 로컬 검사

| 검사 | 결과 |
|---|---|
| `CGO_ENABLED=1 go test -count=1 -race -timeout=12m ./...` | 전체 PASS. app 332.391초, gateway 32.446초 |
| TUI 판정 15개 대조군 `-tags=runtime_evidence -race` | PASS, 1.234초 |
| 보조 요청 수정 제거 overlay | 예상 assertion FAIL, 0.168초 |
| transcript 검사 제거 overlay | 예상 assertion 10개 FAIL, 0.180초 |
| `gofmt -l .`, tagged `go vet ./...`, `go build ./...` | PASS |
| HTTP 진단 코드 `node --check`, 기계 기록 JSON 파싱·상태 확인 | PASS |
| 문서 인용 | 문서 123개·인용 89개·로컬 링크 685개, 실패 0 |
| `git diff --check` | PASS |

기존 Node V1·PowerShell 전체 검사 근거는 앞선 통합 기록을 유지한다. 이번 제품 수정은
Go에 한정되며, Node 검사 변경은 실패 진단에 한정된다. 기존 skip·미실행 항목을 새 실행의
PASS로 바꾸지 않았다. 복구 실행의 부분 결과는 [기계 기록](evidence.json)에 보존한다.
HTTP 복구에서는 위 Go 검사를 반복하지 않고 변경한 JavaScript·기계 기록·문서·diff를 검사했다.

설치본·전역 profile·AdGuard 설정·remote·Release는 변경하지 않았다.
