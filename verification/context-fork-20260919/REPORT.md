# Context 입력 누적·fork 재개 보완 결과 — 2026-09-19

후속 수정·실제 TUI 재검증·최신 설치 기록은 [요청 수명 및 native 표시 한계 보고서](LIFECYCLE-REPORT.md)에 있다. 이 문서는 9ebb722 당시의 증거와 판정을 보존한다.

개발 바이너리에 실제 입력 누적 방지, 완료된 fork의 새 작업 전달, 자식 ID 전달 근거, SendMessage의 native 호환 처리를 반영했다. **native `/context`의 모든 표시 문제가 해결됐거나, 간헐 실패가 완전히 제거됐다는 판정은 하지 않는다.** 아래 두 공백은 남아 있다.

- 응답을 한 번 받은 이후 native의 `Messages`·`Free space` 항목은 로컬 이력을 다시 추정한다. 진단 출력이 실제 backend 입력에서 제외돼도 이 표시에는 포함될 수 있다.
- 기존 취소 요청 정리 테스트가 전체 gateway 검사에서 한 번 실패했다. 이후 TCP 종료 관측과 테스트 클라이언트 격리를 추가한 검사는 통과했지만, 그 한 번의 근본 원인까지 입증한 것은 아니다.

## 반영 대상과 식별

- 소스: `D:\AIDEV\clauduct-s36-build\context-fork-repair-20260919`, branch `fix/context-fork-acceptance`.
- 제품 코드 commit: `9ebb72258838d190da9c9a565ce7bc3ae7f4c223`.
- 그 앞의 구현 commit: `1dade48`(조회 기록의 출처 확인·입력 제외), `c8f8a80`(빈 입력 배열·재개 지시·위임 receipt).
- 빌드: 깨끗한 별도 checkout, Go 1.27.1, `vcs.modified=false`.
- 교체한 개발용 실행 파일: `D:\AIDEV\clauduct-s36-build\clauduct.exe` 및 같은 디렉터리의 hook/dev 실행 파일.
- `clauduct.exe` SHA256: `e651ff8c0f4a6a4a2805b722a8d7897b45a4fed0c26b93eb8e524ce3001d6b3e`.
- 이전 3개 바이너리 보관: `D:\AIDEV\clauduct-s36-build\before-9ebb722-20260919`.
- 사람용 검증 launcher의 manifest 참조도 이번 빌드로 변경했다. 기존 launcher 사본은 `interactive-before-context.mjs`에 보관했다.
- 설치된 release, native Claude 실행 파일, 전역 설정·신뢰 설정은 편집하지 않았다. 실제 TUI는 앞서 승인된 공개 fixture 프로젝트/profile을 사용했다.

## 확인된 원인과 수정

### 1. `/context` 출력이 다음 요청에 들어감

Claude Code 2.1.278의 대화형 명령은 명령 행, ANSI 화면 출력, 숨은 Markdown 보고서를 native 이력에 남긴다. `-p`에서는 사용자 명령과 출력 쌍을 남긴다. 종전 Clauduct는 이들을 일반 사용자 입력과 함께 변환했다. 이전 native 단독 loopback 실험에서도 같은 전송 형태를 관측했다. [공식 저장소의 관련 증상 보고](https://github.com/anthropics/claude-code/issues/61907)도 있으나, 그 이슈를 현재 버전의 수정 보증으로 사용하지 않는다.

`context_display.go`는 인증된 SessionStart가 등록한 transcript 경로만 `os.Root` 안에서 읽는다. native 버전, session ID, 행 종류, 부모 UUID 연결, 실제 context 명령의 output/meta 조합을 확인한 뒤 해당 문자열의 SHA256을 이용해 요청에서 정확한 기록 묶음만 제외한다. 일반 생성과 `count_tokens`에 같은 처리를 적용했다.

문자열이 비슷하다는 이유로 삭제하지 않는다. 사용자 인용, 부분 일치, 출처 불명, 다른 버전, 끊어진 부모 연결, 같은 내용의 중복으로 출처가 모호한 경우는 보존한다. 파일은 추가된 부분만 읽으며 보고서 원문을 별도 파일에 저장하지 않는다. 압축 경계에서는 기록을 비우고, 재시작 시 native 이력에서 근거를 재구성한다. 4096개의 서로 다른 기록 한계에 도달하면 미확인 입력은 보존하고 `capacityExceeded`를 표시한다.

첫 TUI 후보에서는 제외 후 빈 입력을 `null`로 보냈다. backend 계수 실패 후 native가 대체 추정치를 표시했다. 이를 `input: []`로 수정한 뒤 실제 backend에서 빈 입력 14토큰을 받았다. 19→14 차이는 초기 native의 빈 입력용 문구와 빈 배열의 차이이며, 수치를 임의로 0으로 설정하지 않았다.

native 명령 handler, 화면 renderer, transcript는 바꾸지 않았다. 명령 hook의 context 보존 검사를 우회하지도 않았다. `status.contextDisplay`에는 제외 횟수·출처·읽기 실패·한계와 `nativeDisplay=unchanged_local_history_estimates`를 기록한다.

### 2. 완료된 fork의 새 지시

기존 검증 fixture는 새 지시를 확인하지 않고 정해진 답을 반환할 수 있었다. 이를 마지막 사용자 입력의 새 지시, 새 completion 본문, 부모 전달까지 검사하도록 강화했다.

native가 완료된 fork 재개에도 “작업 중 coordinator 메시지” 형식을 쓰므로, 같은 세션의 완료된 fork로 확인된 경우에만 새 작업이라는 설명을 기존 메시지 앞에 붙인다. 실제 새 지시, 모델·effort, 역할 제한, 자식 생성 신원은 보존한다. 진행 중 메시지나 알 수 없는 수신자에는 이 설명을 붙이지 않는다. 이는 지시 전달을 명확하게 하는 보완이며 임의의 LLM 지시 준수를 수학적으로 보장하는 장치는 아니다.

### 3. 자식/손자 ID 보고 누락

native launch 결과에는 내부 ID를 사용자에게 인용하지 말라는 문구가 있다. 해당 결과 문구를 다시 쓰는 대신 gateway가 확인한 `agentId`, `parentAgentId`, 모델·effort receipt를 기존 부모 요청에 함께 제공한다. receipt는 신원·라우팅 근거이며 자식의 조사 내용이 맞다는 보증은 아니다. 결과 본문을 중복 첨부하지 않으며 부모에게 전달된 receipt는 다시 보내지 않는다.

추가 상태 확인이나 모델 호출은 만들지 않는다. 다만 receipt와 재개 설명 자체의 입력 토큰은 추가되며, 그 토큰도 정확 계수에 포함된다.

### 4. TUI에서 추가 발견한 SendMessage 오류

`notify_when_idle=true`는 native의 다른 Claude 세션용 기능이다. 자식에게 이를 보내면 native가 전달하지 않고 `success:false`를 반환한다. 자식은 원래 완료 이벤트를 보내므로, 확인된 같은 세션의 자식에 대해서만 중복 flag를 제거해 기존 완료 이벤트 경로를 사용한다. 알 수 없는 수신자 및 peer 세션의 인자는 보존한다.

또한 실제 SendMessage tool result의 `success:false`를 실패로 기록하고 해당 재개 예약을 해제한다. `is_error`가 없어서 실패가 집계에서 빠지던 공백을 보완했다. 다른 도구 출력이나 사용자 인용 JSON은 이 판정의 대상이 아니다. 실패 본문은 status에 저장하지 않는다.

## 실제 대화형 검증

최종 기능 세션: **`5f83e058-739d-42cb-8c26-95aa323c8cd3`**. Native 2.1.278, 실제 구독 backend, 실제 TUI, `-p` 사용 없음.

| 확인 항목 | 관측 결과 |
|---|---|
| 초기 연속 `/context all` | `Messages` 19→14, 전체 9.9K→9.8K; 계수 실패 없음 |
| fork 새 결과 | 동일 ID `ab242fe34261befe4`가 `FORK_FIRST`, `FORK_SECOND`, `FORK_THIRD`를 순서대로 반환 |
| 모델 전환 후 fork | 부모 Sol/high, fork Terra/medium 유지; 두 재개 모두 새 결과를 부모가 수신 |
| 중첩 Plan | 일반 자식 `ae195b94d1e154969` → Plan `a85002bc12bc8469a`, Terra/medium 상속 |
| Plan 결과 보고 | 부모가 두 ID, `COPPER-RIVER-739`, 파일·행 근거, 미검증 사항을 수신·보고 |
| 정확 계수 | 완료된 생성 29건 모두 backend 입력 사용량 일치, 불일치 0 |
| 실패와 종료 | API 실패 0, 취소 0, native 도구 실패 0, 결과 미확보 0, 불필요한 압축 제어 0, native 정상 회수 |
| 제외 관측 | 14개 요청에서 진단 블록 81개 제외; 읽기 실패·용량 한계 0 |

보조 세션 **`5dfdb47b-5b9b-41fe-b8e0-913b3a011c68`**에서는 3회 조회가 19→14→14였고, 사용자가 직접 붙인 `<command-name>`, `<local-command-stdout>`, `## Context Usage` 인용문을 실제 모델이 그대로 돌려주었다. 단, 그 후보는 이후 SendMessage flag 오류를 발견한 세션이므로 전체 합격으로 취급하지 않는다.

강제 종료 세션 **`11f34964-a2cb-4f29-9979-b6a539d8a099`**에서는 native TUI에서 90초 대기 명령을 실행한 상태로 launcher 하나만 종료했다. launcher와 자손 6개가 관측 기준 36ms 이내 종료됐고, 별도로 만든 대조 프로세스는 살아 있었다. 이 검사는 의도된 강제 종료이므로 exit 57005를 정상 대화 종료로 표시하지 않는다.

원문 대신 필요한 공개 fixture 답변·호출과 status를 모은 `audit-*.json`, `hard-kill-*.json`, `delivery.json`, `test-summary.json`을 이 디렉터리에 보관했다. `inspect.mjs`는 이번 테스트 모드와 승인된 profile을 확인한 세션만 읽는다.

## 자동 검사와 실패 기록

- 제품 commit 9ebb722의 전체 검사: **1477개 테스트/하위 테스트 통과, 17개 package 통과**, 실패 0. `all-1789804112184.jsonl`.
- `go vet ./...`: exit 0. `vet-1789804386066.jsonl`.
- 전체 검사에서 console close, 별도 live 실행, launcher kill 항목 3개는 기존 조건에 따라 skip됐다. live와 kill은 위 별도 TUI 증거가 있으며 console close는 이번에 직접 실행하지 않았다.
- 취소 검사 강화 뒤 gateway/bridge 검사도 통과했다. `gateway-1789804631928.jsonl`.
- 입력 제외 함수의 약 2MiB 합성 benchmark: **894825 ns/op (0.895ms)**, 약 2.2MB allocation/op. 파일 읽기·네트워크·전체 TUI 지연을 포함한 측정은 아니다. `bench-1789804536061.jsonl`.
- 최초 조회의 정확 backend 계수에는 여전히 네트워크 왕복이 있다. native 로컬 추정과 같은 즉시 응답 속도를 달성했다고 주장하지 않는다.

실패 기록을 삭제하거나 최종 통과로 덮지 않았다. 개발 중 native print/TUI 기록 차이, fixture의 합성 계수·usage 불일치, 빈 입력 null, notify_when_idle 오류를 각각 발견하고 보완했다. 검사 코드의 타입·문법 오류도 수정 후 재실행했다.

`TestTheReadingKeepsNothingButTheQuota`는 새 `provenReports` 필드의 `pro`를 요금제 누출로 오인했다. 실제 요금제 값을 고유한 공개 canary로 넣고 그 값이 status 어디에도 없는지 검사하도록 수정했다. 측정된 `pro` header를 해석하는 별도 baseline 검사는 유지했다.

`gateway-1789803907427.jsonl`에서 `TestAbandonedRequestIsCancelledAndDrains`가 5초 후 active=1로 한 번 실패했다. 이후 기존 구현의 단독 30회 검사도 통과했으므로 단순 재실행 성공을 원인 해결의 근거로 쓰지 않는다. 현재 검사는 독립 HTTP transport, 실제 TCP close 확인, 실패 시 goroutine stack, 확실한 fixture 정리를 추가했다. 강화 후 30회 및 전체 gateway/bridge 검사는 통과했지만, **그 간헐 실패의 원인은 아직 확정하지 못했다.**

추가로 문제가 발생한 구성인 깨끗한 9ebb722 checkout에서 gateway 전체를 5회 반복했다. 84.4초 동안 모두 통과해 원래 실패를 재현하지 못했다. 이 결과 역시 간헐 실패가 제거됐다는 판정에 사용하지 않는다. 검증 보강·benchmark·보고서 이후에는 제품 소스가 9ebb722와 같은지 diff로 확인했으며, 개발 바이너리의 식별 commit은 계속 9ebb722이다.

## 남은 범위와 검수 판정

1. **native 항목별 표시:** native 2.1.278의 `VBt`는 마지막 backend usage가 있으면 `QFt`로 그 뒤의 로컬 이력을 추정한다. 이 경로는 `count_tokens`를 호출하지 않는다. 보조 TUI에서 전체 12.6K는 그대로였지만 `Messages`가 2.8K→4.9K로 증가했다. Gateway 응답을 고치는 것으로 이 로컬 계산까지 바꿀 수 없다. 현재 native API의 session.measure는 관찰용이고 turn.step 메시지 수는 고정이다. native를 패치하거나 별도 표시를 만드는 방식은 이번 변경에 포함하지 않았다. 매우 많은 조회 이후 native 자체 압축 휴리스틱에 미치는 영향도 실측하지 않았으므로 완전 해결로 판정하지 않는다.
2. **간헐 취소 검사:** 위 1회 실패의 root cause는 미확정이다. 관측을 강화했으며 재현 시 TCP 종료 여부와 서버 stack으로 먼저 구분해야 한다. 시간 제한을 늘리거나 실패를 무시해서 통과시키지 않았다.
3. **유한 검증:** 완료 본문을 전달하는 기능과 특정 fixture의 지시 준수를 검증했다. 모든 모델의 모든 자연어 지시, 모든 입력 형식, 무제한 세션, 모든 native 버전의 무결함을 보증하지 않는다. 모델별 경계·멀티모달의 기존 자동 회귀 검사는 실행했지만 이번 TUI에서 239K/450K까지 채우거나 모든 멀티모달을 다시 실측한 것은 아니다.

따라서 이번 판정은 **개발 바이너리의 확인된 개선을 반영했고, 지정한 실제 TUI 사례가 통과했다**는 것이다. `process=SUCCESS`나 `parent_received`만으로 전체 기능 승인 또는 조사 내용의 의미적 완성을 선언하지 않는다.
