# v0.3.1 — native 프로세스를 유지하는 연결 종료 보완

2026-09-22, `fix/v031`, 기준 commit `149068e` 위 개발 변경. Go 1.27.1 / Windows,
Claude Code 2.1.278. 설치본·릴리즈는 변경하지 않았다.

**관측한 제품 수용 범위는 PASS다.** 보호 On 상태에서 제품 HTTP/SSE 800건과 실제 backend
TUI의 생성·압축·취소 후 후속 작업이 통과했다. 독립 Node·.NET·raw TCP 검사의 이전 FAIL은
그대로 남는다. 외부 driver를 고쳤거나 모든 필터·버전에서 동작함을 증명한 결과가 아니다.

## 수용 범위와 원인

사용자는 필터가 활성화된 상태의 제품 실제 동작을 완료 기준으로 정했고, native 프로세스를
유지하는 복구를 먼저 구현하도록 지시했다. 특정 프로그램을 직접 수정·배포하거나 그 공식
수정 빌드에 의존하지 않는다. 보호 설정은 바꾸지 않았다. On 상태는 사용자의 마지막 확인을
유지한 것이며 이번에는 보호 UI를 별도로 샘플링하지 않았다.

[이전 커널 계측](../v031-close-20260922/KERNEL-ANALYSIS.md)은 송신 종료 이후 필터 경로가
상대의 수신 종료를 주입하는 위치를 보여준다. 이번에는 **제품 listener**의 응답 종료에서도
같은 환경의 실패를 재현했다. 기존 검사에서 통과하던 keep-alive 200건과 달리,
매 응답 뒤 연결을 닫으면 모델 목록 200건 중 42건이 응답 header 전에 reset으로 실패했다.
응답 write 성공 직후 socket을 닫는 순서가 필터가 전달 중인 응답의 손실을 유발할 수 있다.
과거 `CloseWrite` 후 drain이나 linger 실험과 달리, 이번 처리는 FIN을 보내기 **전**에 적용한다.

## 구현

- [connection.go의 현행 위치](../../go/internal/httpguard/connection.go): 표준 `net.Listener`/`net.Conn` wrapper. 당시 gateway 구현은 이후 공통 `httpguard`로 이동했다.
  완료된 HTTP/1.1 `Connection: close` 응답의 framing을 `net/http`가 flush한 뒤 상대가 먼저
  닫을 기회를 준다. 최대 100ms·64KiB로 제한된 socket drain 후 실제 close를 수행한다.
- [gateway.go](../../go/internal/gateway/gateway.go): 연결을 request context에 연결하고 정상
  handler 완료를 표시한다. write 오류·취소·강제 shutdown은 일반 완료 대기와 구분한다.
  강제 종료는 진행 중인 drain도 끊는다.
- [connection_test.go](../../go/internal/gateway/connection_test.go): 실제 loopback과 제품
  listener/writer를 사용한다. 합성 upstream은 공개 응답 입력만 공급한다. 검사 대상 전송을
  mock으로 대체하지 않는다. backend 호출 수도 확인하여 자동 재생성을 숨기지 않는다.

응답 전송을 고정 sleep으로 늦추지 않으며 request retry·native 재시작·필터별 분기·새 의존성을
추가하지 않았다. 전달 여부가 불명확한 생성과 도구 실행을 replay하지 않는 기존 정책을 유지한다.
서버는 HTTP/1.1 framing으로 완료를 알 수 있는 경로만 기다리므로 EOF로만 끝을 알리는
HTTP/1.0 응답을 불필요하게 지연시키지 않는다.

## 기계 검증

| 검사 | 관측 결과 |
|---|---|
| 수정 전 제품 모델 목록 / keep-alive | 200/200 PASS |
| 수정 전 제품 모델 목록 / 매번 close | **42/200 FAIL** |
| 수정 후 모델 목록 / keep-alive·close | 400/400 PASS |
| 수정 후 64KiB SSE / keep-alive·close | 400/400 PASS, 본문 길이·terminal 1개·backend 400회 확인 |
| wrapper 제거 mutation / 매번 close | **7/200 FAIL**, 검사 후 원복 |
| 응답 후 상대 무응답 | 완전한 응답과 제한 시간 내 EOF 확인 |
| drain 상한·강제 종료 | 정상 100ms 상한, 강제 종료로 대기 중단 확인 |
| 기존 요청 중단·gateway 종료 검사 | PASS |
| `gofmt -l .`, `go vet ./...` | PASS |
| `CGO_ENABLED=0 go build ./...` | PASS |
| 일반 Go 검사 | 모든 패키지 PASS 확인. 아래 초기 실행 오류와 app 복구 실행 참조 |
| `CGO_ENABLED=1 go test -count=1 -race -timeout=12m ./...` | 전체 PASS, app 284.449초 / gateway 53.095초 |
| 실제 backend TUI | PASS, 246.76초 / 실제 호출 5회 / native PID 21768 유지 |
| 문서 인용·링크 / diff 검사 | PASS, 문서 128개·로컬 링크 724개·오류 0 / 공백 오류 0 |

초기 전체 일반 검사는 runner의 `CGO_ENABLED=1` 때문에 출하 설정 검사가 실패했고,
app 전체에 준 3분 제한도 소진했다. 다른 패키지는 모두 통과했다. 검사 조건을 수정하지 않고
프로세스 환경을 `CGO_ENABLED=0`으로 설정해 app 전체를 12분 suite 상한에서 다시 실행하여
224.190초에 통과했다. 개별 native 작업 제한은 그대로다. 영향받는 gateway·upstream·platform·
protocol도 `CGO_ENABLED=0`에서 통과했다(gateway 29.295초). race는 설치된 GCC를 해당
검사 프로세스의 PATH/CC에만 지정했고 전역 설정은 바꾸지 않았다.

수정 후 800건 검사와 mutation이 같은 제품 구현을 사용한다. mutation은 listener wrapper
사용 한 줄만 제거하여 실패를 관측한 뒤 복원했다. 실패율은 확률적 환경 관측이며 성능 SLA가 아니다.

## 실제 native/backend 수용

승인된 임시 검증 루트의 공개 합성 프로젝트·임시 profile만 사용했다. `gpt-5.6-luna`/`low`,
실제 PTY, 제품 gateway·hook, 실제 구독 backend로 다음을 수행했다.

1. `PUBLIC_TUI_47` 응답과 공개 사실 blue / 47 저장.
2. `/compact` 후 `PUBLIC_TUI_47 blue 47 PUBLIC_COMPACT_DONE` 응답.
3. 긴 생성의 실제 text delta 관측 후 Esc. 취소 전 text delta 5,791 bytes를 관측했다.
4. 같은 프로세스에서 `PUBLIC_TUI_RECOVERED` 응답, `/exit` 정상 종료와 정리 성공.

[기존 엄격한 검수기](../../go/internal/app/integration_tui_test.go)가 단일 native transcript의
사실·압축·중단·후속 응답 순서와 누적 생성 완료 3회 / 압축 완료 1회 / 생성 취소 1회를 확인했다.
PID 21768과 생성 시각은 초기·압축 후·중단 후·복구 응답 후 동일했다. 검증 범위를 벗어난
auxiliary 요청 4건은 기존 `ROUTE_NOT_AUTHORISED`로 거부했고 실제 호출에 포함하지 않았다.
이 TUI는 `--tools ""` 조건이므로 실제 backend 도구 실행을 검증했다고 주장하지 않는다.

첫 TUI 실행은 터미널 Enter 입력 제출 지연으로 301.04초 deadline에서 FAIL했다(실제 호출 1회).
입력 정리 과정의 `/exit`도 프롬프트에 붙어 제출됐다. 종료 후 해당 프로세스 트리가 없음을
확인하고, 이전 검수에서 확인한 CSI-u Enter로 새 수용 검수를 실행했다. 시간 제한·요청 상한·
검수 assertion은 변경하지 않았다. 성공 실행 내에서는 native를 재시작하지 않았다.
검수 바이너리의 최초 PowerShell 인자 분리 오류는 테스트 시작 전 실패이며 backend 호출 0회다.
두 TUI 임시 profile의 제거와 테스트가 소유한 native·app 프로세스 트리의 종료를 확인했다.

## 남는 경계

독립 Node 31개·.NET 35개·raw TCP 400개는 이번에 재실행하지 않았다. 해당 검사들은 제품
listener를 쓰지 않으므로 이번 제품 수정의 적용 대상도 아니다. 마지막 보호 On 결과는
[종료 순서 보고서](../v031-close-20260922/REPORT.md)의 FAIL이며 그대로 보존한다.
다른 필터·다른 버전·100ms를 넘는 전달 지연과 스트림 중간의 임의 차단까지 보장하지 않는다.
그런 오류가 새로 관측되면 실패 단계와 전달 여부를 기준으로 추가 복구를 검토한다.

채택 계약은 [ARCHITECTURE 6.1](../../docs/v2/ARCHITECTURE.md#61-필터가-활성화된-환경의-연결-종료),
현재 지원 판정은 [COMPATIBILITY](../../docs/v2/COMPATIBILITY.md), 수치는
[evidence.json](evidence.json)에 기록한다.
