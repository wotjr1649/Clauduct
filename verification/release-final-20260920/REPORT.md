# S47 필터 활성·취소·Workflow 수리 근거

AdGuard는 사용자가 활성화한 상태 그대로 유지했다. 설정 변경, localhost 제외,
native 실행 파일 패치, 외부 배포는 하지 않았다. 최종 후보의 판정은
[후보 검수 보고서](../release-candidate-20260920/REPORT.md)에 별도로 기록한다.

## 필터가 소켓 취소를 늦출 때

`29ed902`는 native `turn.complete(reason=aborted)`의 session/agent/turn을 요청에
연결한다. 정확히 일치하는 task 전용 취소 영수증만 기존 2초 checkpoint에서 읽는다.
소켓 EOF가 지연돼도 해당 backend context를 취소한다. 활동 없음이나 쓰기 실패를
취소로 추정하지 않으며, 재개된 다른 turn은 취소하지 않는다. 완료 이벤트 기록은
4096개, 동시에 등록되는 취소 대상은 기존 요청 admission 범위로 제한된다.

실제 native TUI에 HTTP 취소를 의도적으로 전달하지 않는 loopback proxy와 공개
upstream fixture를 연결했다. 부분 도구 인자 125개/본문 delta 0개에서 Esc 후
`native_abort_receipt`로 취소했고, proxy의 client 종료 관측부터 upstream 취소까지
358ms였다. 미완성 도구 실행·대상 파일 생성은 없었으며 같은 TUI의 다음 답변이
정상적으로 표시됐다. 이 검사는 **실제 native TUI + 고정 backend**이며 실제 구독
backend의 장애 재현으로 바꾸어 부르지 않는다. [기계 근거](evidence.json)

필터 활성 상태에서 실제 native TUI deadline 두 경우도 확인했다. 유예 내 완료는
`graceExpired=false`, 응답을 멈춘 경우는 유예 만료 후 종료됐다. 둘 다
`nativeReaped=true`, `CleanupErr=null`이다. deadline에 따른 native exit 1은
예정된 강제 중단이며 성공 응답으로 세지 않는다.

## 남겨 둔 소켓 반례

S46의 즉시 서버 종료는 200회 중 41회 WSAECONNRESET이었고, 클라이언트 본문 수신 후
종료는 200/200 성공했다. 이번 `CloseWrite` 후 bounded drain 실험은 처음 3/200,
중복 Close를 직렬화한 뒤에도 1/200 실패했다. 실패 로그는 그대로 보존했고 이 방식을
제품에 추가하지 않았다. SetLinger(1)도 이전 반례를 해결하지 못해 채택하지 않았다.

이는 필터 활성과 종료 순서의 상호작용을 입증하지만 RST를 만드는 driver 내부까지
특정한 근거는 아니다. OS/Go 자체 버그로 단정하지 않는다. TCP close는 상대
application의 수신 완료 증명이 아니며, 임의 필터가 패킷을 버리는 환경에서 모든
전달 성공을 보장할 수 없다.
[Microsoft 종료 설명](https://learn.microsoft.com/en-us/windows/win32/winsock/graceful-shutdown-linger-options-and-socket-closure-2),
[closesocket](https://learn.microsoft.com/en-us/windows/win32/api/winsock/nf-winsock-closesocket).
제품은 native 회수 후 gateway 종료 순서를 유지하고 명시적 취소를 독립 처리한다.

## 화면 첫 텍스트

Clauduct gateway도 hook도 없는 native 2.1.278에 공개 SSE 텍스트를 1초 간격으로
16회 전송했다. 토큰 카운터는 증가했으나 본문은 완료 후 나타났다. native event
module을 넣은 대조군도 같았다. Clauduct/hook이 있어야만 생기는 현상은 아니다.
서버 첫 text부터 terminal까지 약 15초였지만 **실제 첫 screen paint의 정밀 시각을
측정한 값은 아니다**. native 내부 함수 추적만으로 정확한 렌더링 원인을 확정하지
않는다. 클라이언트 바이너리를 고치거나 미완성 도구를 조기 전달하는 우회는 하지 않았다.

## 실패와 수정의 연결

1. 첫 작업자 역할 설명 수정(`29ed902`)은 실제 Workflow에서 ToolSearch 3회와
   Skill 1회를 막지 못했다. 결과 42가 맞아도 무도구 조건은 실패다.
2. `f9b1955`는 독립 계획의 `tools:[]`를 명시적으로 지원한다. 검증된 step/child에만
   backend 도구 목록과 native로 전달할 callable set을 모두 비운다. 생략하면 기존
   native 도구를 유지하며 null/비어 있지 않은 목록/강제 tool choice는 거부한다.
   제한은 원래 계획에 저장되어 재개에도 유지된다. 원시 `agent({tools})` 지원과 다르다.
3. 실제 A/B/C 시험에서 A와 재개된 C는 각각 43/180, 도구 0회를 확인했다. 부모가
   TaskStop 뒤 추가 이벤트를 기다린다는 답만 한 경우는 실패로 남겼다. 명시적 재개
   후에는 A 재사용/B 미재실행/C만 실행됐고 중복 거부 후 답변도 정상이다.
4. `442c366`은 TaskStop 성공 결과가 중단 확인임을 도구 설명에 명확히 하고,
   새 도구 제한의 `verifiedChecks`/기능 누계 등록 누락을 수정했다. 최종 TUI 결과는
   위 후보 보고서를 따른다. 자연어 지침의 모든 준수를 결정적으로 보장하지 않는다.

## 검사

- 전체 회귀: 17 packages, 1622 test/subtest 통과, 3 skip.
- 전체 race: 17 packages, 1621 통과, 4 skip. GCC 16.2.0 사용. race 실행에서는
  CGO=0 배포 바이너리 검사를 추가로 skip했고 일반 검사에서는 통과했다.
- 마지막 status/설명 보완 뒤 gateway 일반/race 각각 548 통과, vet exit 0.
- 콘솔 창 닫기와 opt-in live 검사 2개는 실행하지 않았다. 별도 실제 TUI 및 Windows
  Job의 정상/Stop/launcher-kill + 무관 프로세스 생존 검사를 대체 근거로 구분한다.
- 예전 opt-in launcher kill 검사가 잔여 자식을 로그만 남기던 결함을 발견해 실제
  실패 assertion으로 강화했다. 그 opt-in 검사 자체를 이번에 실행했다고 하지 않는다.
- 새 시험의 첫 build 실패(잘못된 시험 필드/상수명)를 수정했다. 소켓 반례와 최초
  Workflow 실패를 재실행 성공으로 덮지 않았다. [검사 수·해시·실패 목록](evidence.json)
