# 등록표 상한과 서버 수준 거부 집계

## 관측

`native-gateway`의 서브에이전트 등록표 `agents`는 SubagentStop hook이 도착해야 비워진다. hook이 누락되거나 자식이 비정상 종료하면 항목이 남는다. 상한이 없어 한 프로세스 수명 동안 무한히 커질 수 있었다. 선택기 쪽 `pending`과 `verified`는 이미 1024로 묶여 있었으므로 이 표만 예외였다.

별개로 HTTP 서버가 요청 처리기에 도달하기 전에 직접 거부하는 경로(`clientError`, `connect`, `upgrade`, `checkContinue`, `checkExpectation`)는 내부 `rejected`만 올릴 뿐 진단으로 나오지 않았다. 요청 카운터와도, `rejectedBeforeStart`와도 분리돼 있어 종료 JSON에서 흔적을 볼 수 없었다.

## 변경

- 등록표 상한을 1024로 둔다. 선택기의 기존 상한과 같은 값이다.
- 상한에서 새 등록이 오면 **가장 오래 쓰이지 않은 유휴 항목**을 제거한다. 진행 중인 요청이 있는 항목은 후보에서 제외한다. 요청이 등록을 고정할 때 그 항목을 최근 위치로 옮겨 사용 시각을 반영한다.
- 유휴 항목이 하나도 없으면 새 등록을 `AGENT_BINDING_LIMIT`으로 거부한다. 새 고정 분류를 추가했다.
- 제거 횟수는 `lifetime.agentRegistrationsEvicted`, 현재 상태는 `registeredAgents`/`maxAgents`로 노출한다.
- `lifetime.transportRejections`를 추가해 서버 수준 거부를 센다. 요청 성패 카운터, `rejectedBeforeStart`와 섞지 않는다.

제거된 등록의 자식은 다음 요청에서 `AGENT_SELECTION_UNVERIFIED_UNREGISTERED`로 fail-closed 된다. 동시 활성 서브에이전트가 1024개인 상황 자체가 비정상이며, 무한 증가를 방치하는 쪽이 더 나쁘다고 판단했다. 상한은 테스트를 위해 `maxAgents` 옵션으로 주입할 수 있고 1024를 넘길 수는 없다.

## 검증

2026-09-11, Node.js v24.19.0, `Invoke-ClauductNodeTests`, 60초 제한. `test-native-gateway`는 52 → 53개 검사다. 새 검사는 `maxAgents: 4`에서 진행 중 요청을 가진 등록 하나와 유휴 등록 셋을 만든 뒤 다섯 번째 등록이 유휴 항목만 제거하는지, 제거 후에도 진행 중 요청이 정상 완료되는지, `Expect: 100-continue`가 417로 거부되며 `transportRejections`만 증가하고 `rejectedBeforeStart`는 그대로인지 확인한다.

기준 11개 파일과 선택·완료 표면 4개가 모두 통과했다. 외부 추론 요청·실제 인증 조회·실제 Claude 실행은 0이다.

## 남은 위험

연결 단계에서 끊긴 바이트의 구체적 원인은 기록하지 않는다. 3주기 종료 후 socket·timer·listener 실측은 여전히 없다.
