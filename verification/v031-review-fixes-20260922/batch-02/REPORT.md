# 보호 On native 전송 수리 — batch 02

2026-09-22, `fix/v031`, 기준 `149068edd693fb860a03244a2ea15764bcd68c34` 위 개발 작업.
Windows / Go 1.27.1 / 실제 Claude Code 2.1.278. 설치본·외부 프로그램·보호 설정은 변경하지 않았다.
On은 사용자의 마지막 확인을 유지한 조건이며, 이번에 보호 UI를 별도로 샘플링하지 않았다.

**이번 제품 수용 범위는 PASS다.** 최종 코드로 실제 native의 13단계 전송 대조, 실제 backend
TUI, 일반·race 전체 검사가 통과했다. v0.3.1 전체 리뷰의 다른 결함까지 해소하거나 배포를 승인한
판정은 아니다. 이전 Node·.NET·raw TCP FAIL을 제품 PASS로 바꾸지 않는다.

## 확인한 원인과 최종 수정

1. **native의 재전송이 백엔드를 중복 실행했다.** 백엔드 호출 뒤 HTTP 응답 전 socket을 한 번
   끊자 native가 같은 turn·step·body를 다시 보내 백엔드가 2회 실행됐다. 두 요청의
   `X-Stainless-Retry-Count`는 모두 `0`이었다. gateway 내부 retry가 0인 것만으로 부족했다.
   `native-baseline.txt`, `native-identity.txt`와 대응 trace에 수정 전 반례가 있다.
2. **큰 요청의 조기 거부가 종료 경로를 만들었다.** 실제 native의 약 352KB 요청은 keep-alive였다.
   body를 읽기 전 거부하면 Go의 256KiB 미소비 기준을 넘어 서버가 연결을 닫았다. 기존 보완은
   요청의 `Connection: close`만 표시하여 이 경로를 빠뜨렸다. 최종 수정은 오류 처리에서 남은
   body를 최대 1초·32MiB+1 byte까지 버린다. 정상적으로 끝난 업로드는 같은 연결을 유지하고,
   미완성·초과 입력은 닫는다. 오류 body를 실행하거나 파싱하는 변경은 아니다.
3. **100ms 종료 상한은 부하 중 큰 응답에 부족한 사례가 있었다.** 전체 검사에서 64KiB SSE의
   200개 중 1개가 121.2552ms에 reset으로 잘렸다. 수신 65,305 bytes, text 57,344 bytes,
   `message_stop=0`, JSON 파싱 오류와 원래 socket 오류를 함께 기록했다(`go-test-all.txt`).
   완료된 HTTP/1.1 연결의 drain을 서버 종료 경로에도 적용하고 상한을 500ms로 보완했다.
   완료 표시는 요청마다 초기화한다. gateway 종료는 진행 중인 drain도 즉시 중단한다.

재전송 예약은 선택·결과 변경 전에 수행한다. session·agent·turn·step·class와 원문 body의
SHA-256만 메모리에 보관하며, 요청 본문이나 지문은 로그·journal에 저장하지 않는다.
진행 중·성공·취소·불확실한 실패 뒤 같은 실행은 `NATIVE_REQUEST_REPLAY_BLOCKED`로 거부한다.
새 step과 새 turn은 구분하고, native를 재시작하거나 요청을 자동 재생성하지 않는다.
local budget/route/no-transport 거부처럼 미실행이 명확한 경우만 예약을 해제한다.
`--bare`의 식별 정보 부재도 보호를 끄지 않는다. 세션당 16,384개 한도에서 기록을 버리지 않고 거부한다.

수정 파일은 `go/internal/gateway/connection.go`, `errors.go`, `gateway.go`, `messages.go`,
`diagnostics.go`, `parent_wait.go`, 새 `request_replay.go`와 해당 회귀검사다.
설계 계약은 `docs/v2/ARCHITECTURE.md` 6.1·9절, 지원 판정은 `docs/v2/COMPATIBILITY.md`에 반영했다.

## 실제 native 대조

`native-category-verified.txt`, `native-category-verified-trace.jsonl`은 실제 native와 제품 loopback 전송을 사용한다.
backend 응답/중단만 공개 합성 입력으로 제어했다. 전송을 mock으로 대체하지 않았다.
계측은 시험 전용 Go overlay이고 제품 빌드에는 들어가지 않는다. 본문·인증 header는 기록하지 않는다.
최종 trace의 연결 ID는 증가하는 정수여서 메모리 주소 재사용과 혼동하지 않는다.

| 단계 | 주입/관측 | backend 실행 | 판정 |
|---|---|---:|---|
| 1 | 정상 응답 | 1 | 정상 marker |
| 2 → 3 | 큰 native body를 capability 오류로 거부 → 후속 요청 | 0 → 1 | 오류 전달, 같은 TCP 연결로 후속 성공 |
| 4 → 5 | 응답 header에서 서버 연결 종료 → 후속 요청 | 1 → 1 | 정상 framing과 새 연결의 후속 성공 |
| 6 → 7 | 실행 중 native interrupt → 후속 요청 | 1 → 1 | 취소 오류와 후속 성공 |
| 8 → 9 | 실행 뒤 응답 전 1회 연결 손실 → 후속 요청 | 1 → 1 | 재전송은 400으로 차단, 후속 성공 |
| 10 → 11 | HTTP header와 응답 일부 전달 후 1회 끊김 → 후속 요청 | 1 → 1 | 중복 실행 없이 오류·후속 성공 |
| 12 → 13 | upstream 실행 중 오류 반환 → 후속 요청 | 1 → 1 | 재전송 차단과 후속 성공 |

13단계 모두 **PID 2168, 시작 1회**, exit 0, cleanup 성공. main HTTP 요청은 재전송 3건을
포함한 16건이며 backend 실행은 총 12회다. 거부 단계 2는 backend에 도달하지 않는다.
실행 전 응답 손실과 부분 응답 손실을 섞지 않았고, 중단된 실행을 성공으로 세지 않았다.
손실/오류 단계 8·10·12 모두 실제 native 결과에 `NATIVE_REQUEST_REPLAY_BLOCKED`가 포함되는지도 검증했다.
단계 4는 서버가 HTTP 연결을 종료한 경우다. gateway 프로세스 자체가 종료된 뒤 같은 listener를
자동 재생성하는 기능을 주장하지 않는다. 실제 gateway shutdown의 취소·drain·정리는 별도 Go 검사로 검증한다.

## 실제 구독 backend TUI

최종 코드의 실제 PTY / native 2.1.278 / `gpt-5.6-luna`·`low` / `--tools ""` 조건이다.
승인된 `.tmp/integration-temp` 아래 공개 합성 프로젝트와 임시 profile만 신뢰 등록했다.
native 환경은 필요한 Windows 변수만 허용했고, MCP·사용자 설정을 불러오지 않았다.

`tui-accepted.txt`: **80.08초 PASS, PID 19376, 시작 1회, 실호출 5회, exit 0, cleanup 성공.**

1. 공개 사실 code/color/number를 입력하고 `PUBLIC_TUI_47`을 받았다.
2. `/compact` 뒤 `PUBLIC_TUI_47 blue 47 PUBLIC_COMPACT_DONE`을 받았다.
3. 실제 text delta 517 bytes를 관측한 긴 생성에 Esc를 보냈다.
4. 같은 프로세스에서 `PUBLIC_TUI_RECOVERED`를 받고 `/exit`로 정상 종료했다.

기존 검수기는 단일 native transcript의 사실·압축·중단·후속 응답 순서와 생성 완료 3회,
압축 완료 1회, 취소 1회를 그대로 검증했다. 보조 모델 요청 4건은 사전 예산/경로 거부로
외부 전송되지 않았다. 실제 backend 도구 부작용을 검증한 실행은 아니다.

## 회귀검사와 수정 제거

| 검사 | 최종 관측 |
|---|---|
| 제품 HTTP/SSE, keep-alive·매번 close | 800/800 PASS; 본문·terminal·실행 횟수 확인 |
| 큰 오류 body와 같은 연결의 후속 응답 | 인증 거부·capability 거부 모두 PASS |
| 미완성 body 거부·무응답 peer·강제 종료 | 기존 시간 제한 유지, PASS |
| 재전송·동시 실행·취소·새 step·`--bare`·독립 auxiliary | PASS |
| body drain 제거 | 같은 연결 유지 검사가 양쪽 거부 모두 FAIL |
| replay gate 제거 | status 200 / backend 2회로 FAIL |
| 종료 신호의 drain 중단 제거 | 50ms 이내 중단 검사 FAIL |
| native step 구분 제거 | 정상 다음 step이 400으로 거부되어 FAIL |
| `CGO_ENABLED=0 go test -count=1 -timeout=12m ./...` | 전체 18개 PASS; app 304.878초 / gateway 48.700초 |
| `CGO_ENABLED=1 go test -count=1 -race -timeout=12m ./...` | 전체 18개 PASS; app 325.294초 / gateway 51.163초 |
| gofmt·vet·build, runtime_evidence/policy_evidence vet | PASS |

최종 전체 일반·race 검사는 동시에 실행했고 실제 TUI와도 일부 겹쳤다. 검사 제한·본문 검증·완료
이벤트·실행 횟수 assertion을 낮추지 않았다. C 컴파일러는 기존 GCC를 검사 프로세스의 PATH/CC로만 지정했다.
수정 제거 검사와 모든 초기 실패를 각각의 로그에 보존했다. 문서·최종 diff 검증은 `evidence.json`에 기록한다.

## 실패 이력과 판단 범위

- `CloseWrite`를 그대로 전달한 초기안은 raw FIN 지연/timeout을 일관되게 해결하지 못했다.
  단순 full-close로 바꾼 대조에서도 본문 뒤 reset이 관측됐다. 최종 제품에는 두 안을 넣지 않았다.
  실제 오류 응답 뒤 연결을 유지하는 수정을 선택했다. 초기 FIN probe와 실패 로그는 보존한다.
- 초기 100ms 후보는 위 SSE 손실과 미완성 body 종료 대기 실패를 드러냈다. 종료 신호와 500ms
  상한을 반영했고, 후속 일반·race 검사는 통과했다. 모든 환경에 충분한 상한이라고 단정하지 않는다.
- `--bare`에서 receipt를 필수로 요구한 초기 guard는 기존 broken-stream 검사를 가로막았다.
  식별 정보가 없을 때도 입력 지문으로 추적하도록 수정했다. auxiliary가 root turn 게시에 잘못
  의존한 오류는 기존 독립 요청 판정 함수를 공유하고 명확한 사전 거부의 예약을 해제하여 수정했다.
- 수동 TUI 운전 2회는 입력·종료 제출 지연으로 각각 301.50초·302.07초에 FAIL했다.
  실제 호출은 3회·5회, 정리는 성공했으며 정상 종료 PASS로 계산하지 않았다. 공개 marker를 보고
  다음 입력을 보내는 bounded 자동 드라이버로 바꾼 뒤 74.82초에 통과했고, 최종 step 구분 반영
  후 80.08초에 다시 통과했다. 실호출은 각 5회로 이번 TUI 검증 전체 합계 18회다.

이 결과는 관측한 제품 경로의 신뢰성·복구·중복 실행 방지 판정이다. 외부 driver를 고쳤다는 뜻도,
모든 필터·버전·무제한 지연에서 TCP 종료가 정상임을 증명한 결과도 아니다. 독립 Node 31개·
.NET 35개·raw TCP 400개는 이번에 다시 실행하지 않았고, 이전 환경 진단 FAIL/HOLD를 유지한다.
전송 중 실제로 잃은 응답을 복원했다고 하지 않는다. 실패를 명시하고 같은 native 프로세스에서
다음 작업을 계속할 수 있게 하며, 이미 실행됐을 수 있는 요청을 자동으로 다시 실행하지 않는다.

`before.json`·`after.json`·`task-changes.patch`가 이번 묶음의 변경 범위를 고정한다.
이전 작업의 staged/unstaged/untracked 변경을 보존했으며 commit·push·설치·배포는 하지 않았다.
