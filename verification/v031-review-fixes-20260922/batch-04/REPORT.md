# v0.3.1 네 번째 수리 — 최초 502와 native 빈 응답 경계

2026-09-22, `D:\AIDEV\clauduct-v031`, branch `fix/v031`, base
`149068edd693fb860a03244a2ea15764bcd68c34`. Windows, Go 1.27.1,
Claude Code 2.1.278. 기존 mixed/staged/untracked 작업을 보존했다.
보호 On은 사용자의 마지막 확인 상태이며 이번 작업에서 설정을 변경하지 않았다.

**최종 판정: 현재 검수 범위 SUCCESS. 미해결 확정 결함과 판정 차단 항목 0. 미출하.**
최종 코드의 일반·race 전체 18개 패키지와 아래 검사들이 통과했다. commit/push/PR/태그/Release/
설치본 변경은 수행하지 않았다. 사용량은 실제 backend usage이며 생성·압축의 원격 사전 계수를
다시 도입하지 않았다.

## 최초 원인과 최소 재현

보완된 기록을 적용한 실제 구독 backend 실행 `live-silent-02.txt`에서 의도적인 손실 주입 전
첫 HTTP 502를 확보했다. native PID 26248, backend 7회, 34.90초, 종료 1이다.

| 항목 | 관측 |
|---|---|
| 최초 category / stage | `EMPTY_REPLY` / `delivery` |
| stream 마지막 read 오류 | `none` |
| client context | `none` |
| terminal 관측 | `true` |
| event / 읽은 bytes | 17 / 115611 |
| 후속 오류 | 같은 native 실행의 `NATIVE_REQUEST_REPLAY_BLOCKED` |

정상 terminal을 포함한 빈 모델 응답을 bridge가 거부했고, native가 재전송한 순서다.
이 재현의 502를 socket reset으로 분류할 근거는 없다. batch-03 최초 실패의 유실된 category를
소급 복원한 것은 아니다. 같은 실행 경로에서 원인과 종료 정보를 새로 관측하고, 해당 기전을
결정적인 최소 재현으로 고정했다.

최소 재현은 `go/internal/app/native_wait_test.go`의
`TestNativeSDKEmptyReplyWaitsForBackgroundChild`다. 실제 native·gateway·background child·
영수증을 실행하고 upstream SSE만 공개 합성 입력으로 고정한다. 부모의 빈 terminal response를
닫을 때 자식을 해제하여 대기 중 응답 조건을 만든다. Agent, Workflow, 결과를 받은 뒤 빈
task-notification을 각각 검사한다. sleep, 모델의 임의 판단, 재실행으로 결과를 맞추지 않는다.
새 명시적 입력·다른 session·다른 child·실패한 Workflow·지난 tool result의 경계도 검사한다.

## 수정

1. `messages.go`: relay의 모든 반환에서 `StreamEnd`를 기록한다. 파싱·변환 단계가 EOF 전에
   끝나도 최초 category와 terminal/event/byte 관측이 남는다.
2. `parent_wait.go`, `response.go`, `native-events.mjs`: 확인된 SDK 빈 응답을 제어 응답으로
   처리한다. SDK는 빈 assistant를 재요청하고 task-notification 뒤 응답을 통째로 없애면
   `error_during_execution`을 반환하므로 다음 고정 상태를 출처와 함께 전달한다.
   `[Clauduct] Waiting for background task notification.` 또는
   `[Clauduct] Background task notification received; no additional response.`
   상태는 자식 보고서·모델 답변·업무 완료가 아니다. SDK의 실제 텍스트·도구는 보존한다.
   TUI의 기존 무출력 대기는 유지한다. 별도 모델 polling과 프로세스 재시작은 없다.
3. Workflow는 tool 반환 뒤 자식 등록이 늦을 수 있다. 최신 assistant Workflow call, 성공한
   tool result, gateway가 준비하거나 연결한 같은 session의 run을 대조한다. native가 tool
   result 뒤 붙인 system reminder 때문에 이 연결을 놓치지 않는다. pending 자식이나 완료
   결과를 새로 만들어 넣지 않는다.
4. 같은 native conversation step은 body/class를 바꾸어도 실행 소유권이 하나다. 두 번째
   요청이 현재 `hold:true`를 덮어쓰거나 backend를 중복 실행하지 못한다. compaction·독립
   auxiliary·step 없는 fingerprint 경로는 별도 의미를 보존한다.
5. 독립 리뷰가 확인한 TUI 완료 알림의 진행 상태 잔류를 수정했다. 빈 제어 응답을 소비하는
   `hold`와 실제 자식 대기의 `wait`를 분리했다. 자식 없는 Workflow의 시작 기록만으로
   pending을 만들지 않는다. 실제 pending이 없으면 알림 처리 후 `turn_ended`가 된다.

## 검증 근거

| 검사 | 결과 / 파일 |
|---|---|
| 최초 StreamEnd 누락의 수정 전/후 | `stream-before.txt` FAIL → `stream-after.txt` PASS |
| SDK Agent·Workflow·빈 완료 알림, 중첩 역할·상속·fork | `sdk-status-after.txt` PASS. 중간 SDK `is_error`도 거부 |
| 최종 제어/진행 상태 경계 | `final-boundaries.txt` PASS. pending·완료 알림·자식 없는 Workflow 구분 |
| 같은 step의 다른 body/class | `owner-before.txt` FAIL → `owner-after.txt` PASS |
| 수정 제거 검증 | 초기 5건, 후속 4건, Workflow의 가짜 pending 1건: 총 10건의 기대 assertion FAIL. `mutations.json`, `mutations-final.json`, `mutation-launch-pending-result.json`. 빌드 실패를 근거로 사용하지 않음 |
| 단독 gateway race 전송/종료 재검사 | `gateway-race-isolated.txt` PASS, HTTP/SSE 800/800, event body 4종 종료 |
| 실제 backend SDK | `live-release.txt` PASS. PID 21712, starts=1, 12회 호출, 52.97초, 자식 결과 2개 수신, Workflow 완료 1, Write 2/성공 2, 손실 주입 1, exit=0, cleanup=true |
| 실제 backend TUI | `tui-release.txt` PASS. PID 8200, starts=1, 5회 호출, 97.11초, 생성 3·압축 1·취소 1, 보존값과 순서 검사, exit=0, cleanup=true |
| 리뷰 F1 수정 후 실제 backend SDK | `live-reviewed.txt` PASS. PID 28224, starts=1, 12회 호출, 40.82초. 자식 결과 2개·Workflow 완료 1·Write 성공 2·의도적 손실 1·정상 종료 |
| 리뷰 F1 수정 후 실제 backend TUI | `tui-reviewed.txt` PASS. PID 26160, starts=1, 5회 호출, 85.25초. 생성 3·압축 1·취소 1·보존값·복구·정상 종료 |
| fmt/vet/build·태그 vet·오프라인 검수기 | `static-final.json` PASS, `CGO_ENABLED=0` |
| 마지막 수정 후 정적/태그 검사 | `static-accepted.json` PASS, `CGO_ENABLED=0` |
| 최종 일반 전체 | `go-test-accepted.txt`, 18개 패키지 PASS, `CGO_ENABLED=0` |
| 최종 race 전체 | `go-race-accepted.txt`, 18개 패키지 PASS, `CGO_ENABLED=1`, 기존 gcc, `-p 1` |
| 문서 인용 | `doc-citations.txt`: 128개 문서, failures=0 |

SDK 실검증의 빈 대기 응답은 status 200, terminal=true, EOF, 24 events/120324 bytes로
확인했다. 이후 자식 본문 2개를 실제 부모 입력에 넣은 뒤 작업 결과를 받았다. 의도한 손실
1건의 `CANCELLED`와 재전송 거부 1건만 허용하고, 그 밖의 모든 failure 누계는 실패로 판정했다.
TUI의 제한된 실호출 예산 밖 auxiliary 거부는 로컬 진단이며 실제 backend 호출 수에 넣지 않는다.
마지막 Workflow-only `wait` 조건은 그 뒤 직접 경계 검사와 전체 일반/race로 검증했다.
실제 TUI 실호출은 기본 생성·압축·취소·복구 경로이며, 빈 완료 알림의 상태 분기는
실제 module과 filesystem을 사용하는 결정적 검사 근거로 구분한다.

## 실패 시도와 남기는 한계

- `live-accepted.txt`는 파일명과 달리 FAIL이다. 대기 수정 후 추가 완료 알림의 빈 응답이
  새 `EMPTY_REPLY`를 만들었다. 이 실패를 후속 PASS로 덮지 않고 알림 경계 검사를 추가했다.
- 초기 전체 일반·race에서 SDK 중첩 위임 실패가 있었다. 모든 SDK 텍스트를 TUI처럼 억제한
  것이 원인이었다. 빈 응답만 제어하고 실제 텍스트를 보존하여 기존 세 회귀를 복구했다.
- SDK의 빈 frame 통과, 응답 제거, 줄바꿈 frame 실험은 native 재요청 또는 실행 오류를
  만들었다. 관련 `sdk-envelope-*`, `sdk-control-after.txt`, `sdk-notification-boundary.txt`는
  실패 근거다. 최종 구현은 출처를 표시한 제어 상태로 이 경계를 처리한다.
- 최초 병렬 race에서 1초 shutdown/dial deadline 실패 2건이 있었다. 독립 전송 재검사에서는
  800/800과 종료 4종이 통과했다. 제한을 늘리지 않았고 최종 전체 race 결과를 별도로 기록한다.
- `go-test-final.txt`는 `CGO_ENABLED=1`이 상속되어 출하 조건 1건이 실패했다. 나머지 제품
  검사는 통과했다. 조건을 약화하지 않고 일반 검사 실행 환경을 `CGO_ENABLED=0`으로 바로잡았다.
- 과거 raw TCP source snapshot과 batch-03의 유실된 최초 category는 복구하지 못했다.
  현재 제품 검증과 역사적 증거 한계를 분리한다. 임의의 미래 native/필터 버전 전체를 보장하지 않는다.
- 이전에 자동 승인 검토가 거부한 batch-03 임시 폴더 삭제는 재시도하지 않았다. 선택적 정리
  제한이며 이번 제품 검증을 생략하는 이유로 사용하지 않았다.

독립 리뷰의 발견 사항·반박·반환 여부는 같은 디렉터리의 `REVIEW.md`에 기록한다.
전체 상태·실행 로그·소스 및 patch hash는 `evidence.json`으로 고정한다.

이번 변경은 제품 6개·검사 6개·설계 문서 2개, 합계 14개 파일이다. `candidate-source.json`은
Go 트리 268개 파일의 hash를 기록한다. `batch.patch`는 이번 수정만 담고 reverse-check는
검사만 수행했다. 원복을 적용하거나 pre-existing 변경을 stage/commit하지 않았다.
보존된 원본과 이번 수정은 `.tmp/v031-review-batch4-20260922/before` 및 `changed-files.json`으로
구분한다. 실제 backend와 합성 fixture, 정적 리뷰와 실행 근거를 서로 대체하지 않았다.

출하·remote CI·설치본 변경은 이번 검수의 실행 항목이 아니다. Node V1/.NET/raw TCP 원본
검사를 이번 Go 변경의 전체 판정 기준으로 다시 도입하지 않았다. 채택한 제품 동작 기준의
전송·동일 프로세스 회복 검증을 사용했으며, 과거 외부 전송 구성요소의 제한은 보존했다.
