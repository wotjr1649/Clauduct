# v0.3.1 공통 내장 HTTP 필터 검증

2026-09-23. 상태: 최종 일반/race 검증 완료, 출하물·실 backend·원격 CI는 별도 판정.

사용자는 Node V1 개발을 종료하고 Go V2에서 실패 조건을 개선하도록 지정했다.
내장 필터는 v0.3.1의 Windows 경로에 적용하되, 향후 macOS·Linux 제품 지원을 위해
OS 전용 API에 의존하지 않도록 했다. 외부 이슈 댓글은 게시하지 않았다.
AdGuard 설정·제외 목록·서비스·driver는 변경하지 않았다. 읽기 전용 확인에서
`Adguard Service`와 `adgnetworkwfpdrv`가 Running이었다.

## 구현과 검토

- [httpguard](../../go/internal/httpguard/connection.go)는 표준 Go 패키지만 사용한다.
  기존 gateway의 연결 종료 처리를 이동하고, HTTP 파서의 완성된 오류 응답에
  `Content-Length`를 추가한다. 상태와 본문은 보존한다. 정상 응답·SSE는 재작성하지 않는다.
- 완료 응답의 drain은 최대 500ms, gateway가 전달하는 크기 상한은 32MiB다.
  종료 context가 이미 진행 중인 drain을 중단한다. 취소된 handler의 buffered write가
  완료 표시를 되살리지 않도록 parser 응답과 handler 응답을 구분한다.
- [실행 예약](../../go/internal/gateway/request_replay.go)은 transport 직전에 취소를 확인한다.
  이미 취소된 추론·검색은 transport에 진입하거나 실행 예약을 소비하지 않는다.
  전송 이후의 불확실한 실패는 여전히 같은 실행의 재전송을 차단한다.
- [native 종료 처리](../../go/internal/gateway/native_cancellation.go)는 session·agent·turn과
  허용된 종료 사유를 모두 대조한다. 기존 per-turn 취소 기록에 error·refusal을 추가하여
  다음 turn의 progress 갱신에도 원인이 남는다. 검색도 동일한 취소 경로를 사용한다.
  성공·다른 turn·다른 session·미확인 필드는 취소 권한이 없다. 무응답 시간으로 취소하지 않는다.
  기록의 JSON은 기존 2KiB 상한과 중복/알 수 없는 key 거부를 유지한다.
- 본문 수신 단계도 별도로 추적한다. 검증된 전체 요청이 같은 session·agent·turn으로
  재연결하면, 아직 해독하지 못한 이전 본문 읽기를 취소한다. 이미 해독·실행한 요청은
  이 경로로 취소하지 않으며 실행 원장이 중복을 차단한다. 다른 session·agent·turn과
  auxiliary·compaction은 이 교체 권한이 없다. 취소된 본문 연결은 재사용하지 않는다.
- [실제 연결 검사](../../go/internal/httpguard/connection_test.go)는 parser 오류의 본문 길이,
  새 연결·재사용 연결, 기존 server callback 보존, 조용한 상대와 강제 종료를 확인한다.
  431 입력은 Go의 read 여유와 keep-alive에 미리 읽힌 bytes를 넘는 32KiB다.
  `MaxHeaderBytes`의 정확한 바이트 경계를 검증했다고 주장하지 않는다.

## 관측 결과

| 검사 | 결과 | 근거 |
|---|---|---|
| 공통 필터의 실제 socket·종료 검사, race | PASS | [실행 기록](httpguard-race-focused-final.txt) |
| parser framing 제거 | 기대한 FAIL | [mutation](httpguard-mutation.txt) |
| 전송 전 취소, 수정 전 | FAIL: 추론·검색 transport 각각 1회 진입 | [수정 전](pre-dispatch-before.txt) |
| 전송 전 취소·후속 연결·중복 방지 | PASS | [수정 후](pre-dispatch-after.txt) |
| 전송 전 취소 guard 제거 | 기대한 FAIL | [mutation](dispatch-mutation.txt) |
| parser 거부 4종 × 20회 | PASS | [반복 기록](parser-framed-expanded.txt) |
| 종료 기록 수신·출력, 수정 전 | FAIL | `terminal-receipt-before.txt`, `terminal-publication-before.txt` |
| 정확한 turn의 종료·추론/검색·재연결 20회·중복 차단, race | PASS | `terminal-reconnect-after.txt` |
| native 종료 기록 출력과 정상 완료 비취소 | PASS | `terminal-publication-after.txt` |
| 종료 사유 처리 제거 | 기대한 FAIL | `terminal-mutation.txt` |
| 부분 본문 socket을 열린 채 재연결, 수정 전 | FAIL: active=1 잔류 | `partial-reconnect-before.txt` |
| 부분 본문 교체·다른 요청 보존·FIN 재연결, race | PASS | `partial-reconnect-accepted.txt` |
| 부분 본문 교체 제거 | 기대한 FAIL: active=1 잔류 | `partial-mutation.txt` |
| 최초 전체 Go 검사 | FAIL: 19 package 중 gateway 1개 | [전체 기록](go-test-httpguard.txt) |
| 최초 전체 Go race 검사 | FAIL: 같은 gateway half-close 조건 | [race 기록](go-race-httpguard.txt) |
| 부분 본문 교체 전 전체 일반 / race | 일반 19 PASS / race partial-body FAIL | `go-test-recovery-final.jsonl`, `go-race-recovery-final.jsonl` |
| 최종 전체 Go 일반 | 19 package PASS, failures=0 | [일반](go-test-reconnect-final.jsonl) |
| 최종 전체 Go race | 19 package PASS, failures=0 | [race](go-race-reconnect-final.jsonl) |
| 공통 패키지 Linux/amd64·macOS/arm64 | 컴파일 PASS, 해당 OS 실행 NOT_RUN | 로컬 `go test -c` |
| 최종 commit의 원격 CI·출하 바이너리 실 backend | NOT_RUN | 후보 확정 이전 |

최초 전체 Go 검사의 실패는 당시
`TestProtocolRefusalsAndHalfClosePreserveResponses/half-close`다.
18개 package는 통과했으며, 전체 검사와 race 검사 전체를 PASS로 기록하지 않는다.
`httpguard`의 431 추가 검사는 전체 검사 이후 별도로 race까지 수행했다.
Go 1.27.1, 일반 검사 `CGO_ENABLED=0`, race만 `CGO_ENABLED=1`과 설치된 gcc를 사용했다.
최종 실행의 skip은 일반 3개, race 4개다. console close, 명시적 live opt-in, parent kill
탐색 검사는 실행하지 않았다. race에서는 출하 CGO=0 검사도 해당되지 않아 skip이다.
목록과 package별 실행 시간은 [검사 요약](final-tests.json)에 보존한다.

사용자가 복구 범위를 안전한 재연결·중복 방지·후속 작업으로 확정한 뒤 검사를 분리했다.
기존 단일 raw TCP 연결의 즉시 FIN 응답 assertion은 `runtime_evidence`의
`TestRuntimeEvidenceImmediateFINReply`로 보존하고 다시 실행했다. 20/20 FAIL이며
`raw-fin-diagnostic-retained.txt`에 남긴다. 기본 제품 검사는 parser 거부와 실행 전/후
재연결의 실행 횟수·후속 turn을 판정한다. 테스트 대상이나 의미를 바꾼 이력을 숨기거나
raw FIN 문제가 해결됐다고 판정하지 않는다.

첫 재연결 전체 검사에서 일반 19개 package는 통과했으나 race의 실행 전 partial-body
1/10이 active=1로 남았다. 스택은 `io.ReadAll`에서 대기하고 있었다. 이 결과로 본문 읽기까지
종료 소유권을 확장했다. 수정 중에는 만료된 연결 context의 재사용으로 후속 turn이 499가
되는 실패와 취소 응답의 reset도 확인했다. `partial-reconnect-after.txt`,
`partial-reconnect-fixed.txt`, `partial-reconnect-final.txt`는 이름과 무관하게 FAIL 기록이다.
새 읽기 바인딩 직후 중복으로 종료 기록을 적용하지 않고, 기존 요청 시작/2초 checkpoint와
검증된 본문 이후의 종료 확인을 사용한다. 취소된 본문은 Connection: close로 처리한다.
완료된 실패 turn의 재입장은 실행 원장에서 400 또는 본문 취소에서 499로 거부될 수 있다.
어느 경우에도 transport 횟수가 증가하지 않고 명시적 다음 turn은 정상 처리되어야 한다.

## 즉시 TCP FIN 조사

같은 현상을 여러 계층에서 분리했다. 모든 데이터는 공개 합성 문자열이며 credential이나
backend를 사용하지 않았다. 아래 진단 실행의 exit code 0은 관측기 종료를 뜻하며,
각 행의 `marker=false`를 전송 성공으로 바꾸지 않는다.

- [IPv4·IPv6](tcp-probe.txt), [다른 loopback 주소](tcp-address-probe.txt),
  [WSASendDisconnect](tcp-wsa-probe.txt): 요청 직후 송신 종료에서 응답 유실이 재현됐다.
- [선행 receive와 두 번째 receive](tcp-pending-read-probe.txt): receive를 먼저 걸거나
  EOF 뒤 다시 읽어도 응답이 복원되지 않았다. 서버에서 요청 전체를 받거나 응답 쓰기가
  성공한 경우에도 클라이언트가 0 bytes를 받을 수 있었다.
- [TLS 대조](tls-half-close-probe.txt): `close_notify`는 3/3 전달됐으나 underlying TCP의
  송신 종료는 3/3 유실됐다. TLS 결과를 raw TCP FIN 수정으로 판정하지 않는다.

재현 소스는 각 기록과 같은 이름의 `.go` 파일이다. 개별 파일을 `go run 파일명.go`로 실행한다.
TLS probe의 키와 신뢰 대상은 프로세스 메모리에만 있으며 시스템 인증서 저장소를 바꾸지 않는다.

이 조사 결과를 모든 해결 경로의 불가능 판정으로 사용하지 않는다. 현재 공통 필터가 해결한
HTTP framing·실행 전 취소와, 아직 남아 있는 raw TCP half-close 실패를 구분한다.
완료했지만 전달되지 못한 응답의 보관·자동 복구는 사용자가 이번 범위에서 제외했다.
최종 commit 확정·깨끗한 출하 빌드·실제 backend·설치/업데이트/되돌리기·원격 CI의
완료를 이 보고서로 대신하지 않는다.
