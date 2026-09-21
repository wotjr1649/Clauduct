# 기능별 판정·진행 관측·Workflow 결과 회수

2026-09-19 실행, 2026-09-20 기록 마감. 제품 commit은
`34ebcb1608e0dc712a9ae9f887427fdc48a3bcfd`이다. 아래 범위의 검수와 개발 바이너리 반영을 완료했다.
실행별 신원은 [build.json](build.json), 목적지 해시는 [delivery.json](delivery.json)에 있다.
빈 응답 대기 제어는 별도 실험이며 제품에 활성화하지 않았다.

## 1. 구현 범위

- `gateway.features`: 동일 요청에서 실제로 통과한 조건만 집계한다. 최근 16건을
  넘어도 세션 누계를 유지한다. 과거 성공은 다음 요청의 실행 허가가 아니다.
  `acceptance:not_assessed`를 유지하며 `client.verifiedMeaning:version_match_only`로 구분한다.
- native Agent의 회차 영수증이 설정되어 있는데 없으면 실행 전 거부한다.
  `/context` 출처 검증은 고정 기준 버전 비교 대신 해당 세션의 관측 버전·기록 구조·계보를 확인한다.
- native 진행 영수증에 요청/도구/승인 요청/종료의 종류, 시각, 순번, 대기 도구 수를 기록한다.
  본문·도구 인자·명령은 복사하지 않는다. 병렬 이벤트의 파일 쓰기를 직렬화하고 늦은 이전 회차를 제외한다.
  승인 요청 횟수는 그 요청이 관측됐다는 뜻이며 현재 승인 화면이 열려 있다는 보장은 아니다.
- `lastObservedMs`와 고정된 backend text/tool-argument/reasoning delta 수를 기록한다.
  도구 인자는 기존과 같이 응답 완료 전 실행기에 전달하지 않는다.
- HTTP 요청이 없어도 미종료 도구/자식/진행 기록 누락이 있으면 drain 완료로 보지 않는다.
  사용자가 지정한 기존 deadline/grace만 적용하며, 관측이 오래됐다는 이유의 새 강제 종료는 없다.

## 2. 실제 native TUI

모두 이전에 승인된 공개 fixture 프로젝트와 격리 profile에서 수행했다.

| 실행 | 제품 | 확인 내용 |
|---|---|---|
| `bf4a458d-5056-4592-a7f5-2b8ef9d135ba` | `8f66a1d` | Terra/medium 자식, Read, Bash 승인 질문과 8초 실행, 결과 수신, 동일 자식 SendMessage 재개. API/도구 실패·결과 미확보 0. 보조 요청을 native_agent로 집계하는 결함 발견 |
| `07ab4b75-6eb5-4143-a938-f641fd2e9343` | `e36c7f3` | 집계 수정 후 Read 자식과 결과 수신. 일반 생성 6/6, 실제 자식 요청 2/2 필수 조건 관측. API/도구 실패·결과 미확보 0 |
| `e8a5e4a9-6fdb-4f90-87b3-0d71c910c334` | `0614513` | 원본 Workflow 자식이 value 42 반환. 이후 완료 결과 회수 Workflow에서 새 자식 0. 정상 경로 API/도구 실패·결과 미확보 0 |
| `cceb2ca8-8213-472e-92bb-d88de3077d8a` | `34ebcb1` | value 59 회수와 새 자식 0. 잘못된 run ID의 의도한 400 거부 1회 후 같은 세션에서 `PUBLIC_RECOVERY_AFTER_REFUSAL_OK`. 도구 실패·결과 미확보 0 |

[승인 질문의 관측](permission-proof.json), `run-<UUID>.json`의 최종 status,
[Workflow 감사](workflow-audit.mjs)와 [결과](workflow-audit.json)가 기계 근거다.
최종 세션의 `apiFailures:1`은 의도한 거부를 포함한 값이다. 0으로 바꾸거나 정상 요청 실패로 오인하지 않는다.
native 회수 run의 journal에는 새 `started` 자식이 0개이며, source run의 보고서와 값이 일치했다.
프로세스 exit 0만으로 작업 성공을 판정하지 않았다.

## 3. 빈 응답 대기: 실증과 제품 적용을 구분

[실험 코드](native-wait.mjs), [test-only hook](wait-probe.mjs),
[감사 결과](wait-audit.json). 실제 Claude Code 2.1.278 TUI와 로컬 fixture 응답이다.
구독 backend나 Clauduct gateway의 빈 응답 경로를 통과한 실험은 아니다.

- `4cfac413-30b8-4ca4-b5bb-2a020be00629`: MIDDLE의 빈 응답 뒤 내부 추가 요청 한 번을
  `turn.step`에서 전송하지 않았다. LEAF 완료 후 동일 MIDDLE ID의 새 회차가 실행되고 ROOT가 결과를 받았다.
- `e7cd9b3d-ec18-4a34-b155-16503e57ef34`: LEAF에 의도한 HTTP 400을 반환했다.
  부모가 다시 실행되어 실패 내용을 받았다. 실패한 작업을 성공으로 기록하지 않는다.
- `2452d8f3-fe24-4b77-9097-1e298ea3e26d`: Esc와 화면 이동을 했지만 작업 취소를 입증하지 못했다.
  45초 동안 응답을 보내지 않은 fixture에서 약 29,953ms 뒤 같은 LEAF의 두 번째 HTTP 요청도 관측했다.
  첫 응답 완료 후 두 번째 연결이 미완료로 닫혔다. 이것을 Esc 취소 성공으로 오인하지 않는다.
  [45초 실험 원본](native-wait-45s.mjs)은 기록된 harness SHA256과 일치한다.
- `4d65ca16-164f-4fe0-96cc-f9fefc5be7f8`: native 작업 관리에서 MIDDLE을 `x`로 명시적으로 중단했다.
  하위 보류 연결이 닫히고 같은 세션에서 `PUBLIC_AFTER_ESC_OK` 응답을 받았다.
  이 근거는 작업 관리 중단이며, Esc 자체가 중단했다는 근거가 아니다.

**제품의 EMPTY_REPLY 정책은 유지했다.** 실험의 직전 빈 응답/같은 회차/다음 index/실행 중 자식 조건만으로는
내부 보정과 그 순간 들어온 정상 사용자 입력을 모든 경우 구분할 수 없다. 출처 판정과 gateway 통합,
자식 종료 경합·늦은 알림·취소를 검증하기 전에는 요청을 억제하지 않는다.
가짜 완료 본문이나 가짜 usage를 만들어 보내지 않았다.

## 4. Workflow 확대 범위

`Workflow({"resumeFromRunId":"wf_..."})`만 전달하면 같은 launcher가 연결한 원본 run의
검증된 자식 결과를 회수하는 별도 데이터 반환 Workflow를 만든다. 이것은 원본 JavaScript 재실행이나
미완료 단계의 자동 재시작이 아니다. native의 승인 화면과 결과 전달을 사용한다.

- script digest, 원본 run/call/session, child 선택, native 정상 종료, journal key/result를 대조한다.
- 검증된 결과는 `completed_result_reused`, 없는 결과는 `result_unavailable`이다.
- `originalRunState:not_assessed`, `newAgentExecutions:0`을 명시한다. 원본 스크립트의 최종 집계값을 재현했다고 주장하지 않는다.
- `script`/`scriptPath`/`args` 등 추가 필드, 다른 세션, 알 수 없는 run, 변경된 script는 거부한다.
- 16 child, 최초 journal 2 MiB, 반환 데이터 합계 256 KiB로 제한한다. 제한 초과를 잘라 성공시키지 않는다.
- 보고서는 JSON 문자열 데이터로만 넣어 반환한다. 본문이 JavaScript 조각이어도 실행 코드가 되지 않는 검사를 포함한다.
- launcher 재시작을 건너뛰는 복원, 미완료 작업 재실행, 임의 쓰기의 exactly-once는 지원하지 않는다.

공식 native Workflow 재개는 실패한 단계와 이후 단계를 재실행할 수 있다.
[공식 재개 동작](https://code.claude.com/docs/en/workflows#resume-after-a-pause)
따라서 이번 지원표는 `workflow_result_reuse`와 범용 `workflow_resume`를 구분한다.

## 5. 실패 이력과 검증 범위

- 첫 전체 검사 `../session41-repair-20260919/all-1789826394578.jsonl`은 실패했다.
  native validator가 중첩 observe helper에 `$` 전달을 거부했다. top-level 선언으로 수정한 뒤 정상 validator를 통과했다.
  검사를 실행하면서 embedded 원본을 수정한 byte 비교 실패와 harness의 CGO 설정 누락도 구분해 기록한다.
- 수정 후 `all-1789827193700.jsonl`은 통과했다. 이후 진행 기록 누락, auxiliary 집계,
  Workflow 회수와 거부 요청 집계 변경은 각각 관련 검사와 실제 TUI로 추가 확인했다.
- 완료 결과 회수의 unit 검사는 완료/미완료/누락/다른 세션/변조/새 script/경로/알 수 없는 run/본문 코드 주입을 검사한다.
- 승인 요청 관측은 전체 native 승인 UI의 상태 추적이 아니다. 오래된 이벤트는 생존 증명이 아니다.
- backend argument delta 계측은 구현했다. 이 회차의 실제 TUI에서 argument delta 중 Esc를 정밀 실측한 것은 아니다.
- 모든 입력·환경의 무결점, 성능 무저하, 모든 멀티모달·압축 경계의 재검증은 주장하지 않는다.

## 6. 최종 검사와 개발 바이너리 반영

- 최종 제품 commit에서 17 packages, 1,545 test/subtest 통과 기록, 실패 0.
  [검사 요약](test-summary.json), [전체 로그](all-1789829498792.jsonl),
  [실행 신원](all-1789829498792.jsonl.meta.json), [vet 실행 신원](vet-1789829729592.jsonl.meta.json).
  `go vet`도 exit 0이다.
- 3개 검사는 skip이다: `TestConsoleCloseIsNotRunHere`, `TestALiveSessionCompletes`,
  `TestWhatSurvivesTheLauncherBeingKilled`. 위 TUI 증거를 별도로 사용하며, skip을 통과로 세지 않는다.
  콘솔 창 닫기·OS 종료·모든 강제 종료 경로를 이번 변경에서 재검증했다고 주장하지 않는다.
- 실제 구독 backend TUI 4개와 native/로컬 응답 실험 4개를 분리해 기록했다.
  최종 제품 TUI는 21 HTTP 요청, 생성 요청 14개였고, 의도한 잘못된 run ID 거부 1개 후 같은 세션에서 정상 응답했다.
  원치 않은 실패가 없었던 이 실행을 모든 환경의 무결점이나 반복 실행 안정성 보장으로 확대하지 않는다.
- `D:\AIDEV\clauduct-s36-build`의 `clauduct.exe`, `clauduct-hook.exe`, `clauduct-dev.exe`를
  검증한 세 바이너리로 반영하고 목적지 해시를 대조했다. `go version -m`의 제품 revision은 위 commit,
  `vcs.modified=false`, Go 1.27.1/Windows amd64/CGO_ENABLED=0이다.
- `clauduct.exe` SHA256: `bbc498a694fbfa39c74c9d6f2339d289b34ace6a51660247e0485c58cb94633b`.
  이전 세 바이너리와 사람용 실행기는 `D:\AIDEV\clauduct-s36-build\before-34ebcb1-20260919`에 보존했다.
  사람용 `interactive.mjs`의 빌드 검증 참조를 이번 근거로 갱신했고 문법 검사를 통과했다.
  승인된 fixture/profile·사람용 2시간 제한은 유지했다. 사용자 전역 설치 릴리즈는 변경하지 않았다.
- [산출물 확인](artifact-check.json)에서 문서의 로컬 링크 41개와 실행 도구의 문법·기록 JSON·반영 해시를 대조했다.
  [프로세스 확인](process-audit.json)에서 이번 8개 실행의 PID는 모두 사라졌고,
  해당 session ID나 후보 바이너리 경로가 일치하는 프로세스는 없었다. 전체 시스템의 모든 orphan 검증과 구분한다.
