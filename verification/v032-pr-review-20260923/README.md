# v0.3.2 후보 PR 리뷰 (#71)

2026-09-23. PR [#71](https://github.com/wotjr1649/Clauduct/pull/71)의 head `16b02f2`에 `/code-review high`를
1회 실행했다. 리뷰는 코드를 읽고 추론한 것이며 검사를 돌리지 않았다. 지적마다 코드로 다시 판정했다.

| # | 지적 | 판정 | 처리 |
|---|---|---|---|
| 1 | 모듈 순번의 시계 시작이 이전 인스턴스보다 큰 순번을 보장하지 않는데, 문서는 조건 없이 단정한다 | 문서 과장. 한계는 코드 주석에 있고 사용자가 수용했다 | 문서 3곳에 조건을 적었다 |
| 2 | 도착 시 읽기가 일시적으로 실패하면 요청 전체가 거부로 굳는다 | 회귀 아님 | 없음 |
| 3 | 원장의 agent별 최근 turn 표시가 세션 동안 줄지 않는다 | 설계 | 없음 |
| 4 | 동시 요청 한도 거부가 항상 연결을 닫는다 | 설계, 사용자가 수용했다 | 없음 |
| 5 | 이전 결과 포인터를 본문 수신 전에 잡는다 | 결함 아님 | 없음 |
| 6 | turn이 바뀔 때마다 전역 lock 안에서 원장 전체를 훑는다 | 효율, 영향 미미 | 없음 |
| 7 | 독립 auxiliary가 요청마다 root 영수증 디렉터리를 읽는다 | 효율, 영향 미미 | 없음 |
| 8 | 읽기 단계에서 영수증 검증을 두 번 한다 | 중복이지만 같은 함수 | 없음 |
| 9 | `agentSelection`의 auxiliary 분기가 비어 있는 필드를 지운다 | 기존 검사 계약 유지용 | 없음 |
| 10 | `pinNativeTurn`의 nil record 대체가 도달하지 않는다 | 도달 불가 | 없음 |

2번: 이전 코드는 한 요청에서 거부로 이어지는 읽기가 두 번(claim, 선택)이었고, 지금은 한 번이다.
native는 영수증 본문과 완료 표시를 모두 쓴 뒤 요청을 보내므로, 도착 시점에 완료 표시가 없는 경합은 없다.
백신 등의 일시 잠금은 어느 시점의 읽기에도 같은 확률로 생긴다. 이 PR은 그런 읽기의 횟수를 줄였다.

3번: 표시는 agent마다 하나(세션·agent·turn·순번)다. 표시를 지우면 새 turn이 지운 이전 turn의 키로
늦은 요청이 다시 실행될 수 있으므로 세션 동안 유지한다. 크기는 세션에서 turn을 예약한 agent 수에 비례한다.

5번: 결과 항목을 교체하는 곳은 `start`(route 안)와 `beginLocked`뿐이고, 둘 다 포인터를 잡은 뒤에 실행된다.
HEAD의 순서도 같다. handler가 선택 전에 부르는 `reconcileNativeResults`, `reconcileWorkflowResults`,
`observeMessageFailures`, `restoreSelectionHistory`, `toolFailures`는 요청 agent의 항목 포인터를 바꾸지 않는다.
다른 handler가 그사이 새 turn을 들였다면, 도착 시점의 포인터와 달라 그 항목을 교체하지 않는다. 이는
"그사이 들인 turn은 교체하지 않는다"는 원래 의도와 맞는다. 같은 turn이면 `same` 분기가 실패를 기록한다.

6·7번: turn 전환은 agent마다 prompt·자식 단위로 일어나고, 원장은 최대 16,384개다. auxiliary의 읽기는
main 요청과 같은 비용이며 backend 요청 한 번에 비해 작다. 측정하지 않았다.

9번: 이 PR에서 auxiliary는 turn을 고정하지 않는다. `TestIndependentRootAuxiliaryDoesNotOwnThePublishingConversationTurn`은
record에 남은 값이 auxiliary 요청 뒤에 남지 않아야 한다고 검사하므로 그 대입을 둔다.
