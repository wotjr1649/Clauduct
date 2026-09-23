# v0.3.2 재실행 방지 묶음 (#61–#64)

2026-09-23. `main` `37ea733`에서 시작했다. 실행 원장(`request_replay.go`), turn 영수증 읽기
(`native_events.go`), native 이벤트 모듈(`native-events.mjs`)을 고쳤다. 모든 재현은 로컬 fixture
backend를 쓰며 모델 호출은 0회다. native는 Claude Code 2.1.280이다.

## 판정

| issue | 결과 | HEAD 재현 | 수정 후 |
|---|---|---|---|
| [#64](https://github.com/wotjr1649/Clauduct/issues/64) 요청당 영수증 3회 읽기 | 결함 수정 | claim과 선택 사이에 새 turn을 게시하면 `NATIVE_TURN_UNVERIFIED`, backend 0회 | 200, backend 1회 |
| [#61](https://github.com/wotjr1649/Clauduct/issues/61) 독립 auxiliary 키 | 결함 수정 | 두 번째 turn의 같은 auxiliary 요청이 400 | turn마다 1회 실행, 같은 turn의 재전송은 차단 |
| [#63](https://github.com/wotjr1649/Clauduct/issues/63) 실행 원장 16,384 한도 | 수정 | 이전 turn 기록 16,384개 뒤 새 turn 요청이 `NATIVE_REQUEST_CAPACITY` | 200 |
| [#62](https://github.com/wotjr1649/Clauduct/issues/62) 게시 순번 리셋 | 견고성 수정 | 다시 등록한 모듈의 게시가 이전 게시보다 작은 순번을 받음 | 새 게시가 가장 큰 순번 |

HEAD와 수정본의 실행 기록은 [repro-run.txt](repro-run.txt)에 있다. 두 경우 모두 같은 검사 파일을
overlay로 붙였다. [probe_test.go](probe_test.go)는 claim 바로 뒤에 hook을 두고 읽기 횟수를 센다.
[repro_test.go](repro_test.go)는 HEAD에서도 컴파일되는 동작 검사다. HEAD 실행은 바뀐 gateway 파일
6개를 `main`의 것(같은 hook 포함)으로 바꾼 overlay를 쓴다.

## #64 — 영수증은 요청당 한 번

HEAD는 읽기 단계의 취소 바인딩, `claimNativeExecution`, `agentSelection`이 각각 영수증 디렉터리를
읽었다. claim과 선택 사이에 게시된 turn은 두 값을 어긋나게 했고, handler는 요청을 거부했다.

수정 후 `pinNativeTurn`이 요청마다 한 번 읽고 record에 고정한다. 대화 요청(`main`·`subagent`·`workflow`)은
본문을 받기 전에 읽고, 나머지는 claim에서 읽는다. 선택과 취소 바인딩은 고정값을 쓴다. 두 값이
같아졌으므로 둘을 비교하던 거부는 지웠다. 이전 결과 포인터는 여전히 영수증보다 먼저 잡는다.

| 요청 | HEAD 읽기 | 수정 후 |
|---|---|---|
| main | 3 | 1 |
| 독립 auxiliary | 0 | 1 (#61의 root turn 키) |
| count_tokens | 1 | 1 |

본문 업로드 중에 새 turn이 게시되면, 그 요청은 도착할 때의 turn에 속한다. HEAD는 본문 뒤에 다시
읽어 그 요청을 새 turn으로 기록했고, 새 turn의 같은 입력을 재전송으로 거부했다.

## #61 — 독립 auxiliary는 root turn 범위

키에 현재 root turn을 넣는다. 같은 turn의 재전송은 계속 막고, 다음 turn의 같은 요청은 실행한다.
root 영수증을 읽을 수 없으면 이전처럼 세션 전체 키를 쓴다. auxiliary는 turn을 소유하지 않으므로
record에 turn을 고정하지 않는다(`TestIndependentRootAuxiliaryDoesNotOwnThePublishingConversationTurn`).

## #63 — 새 turn이 이전 turn의 기록을 지운다

원장은 session·agent마다 가장 최근에 예약된 turn과 그 게시 순번을 기억한다. 더 큰 순번의 다른
turn이 예약되면 그 agent의 이전 turn 키를 지운다. 새 요청은 도착할 때의 현재 turn으로 키를 정하므로
지운 키는 다시 맞지 않는다.

issue 본문에 없던 경우가 하나 있다. 새 turn보다 먼저 영수증을 읽은 요청이 새 turn의 예약 뒤에
claim에 오면, 지운 turn의 키로 예약되어 이미 실행된 입력을 다시 실행할 수 있다. 이 요청은
`NATIVE_TURN_UNVERIFIED`로 거부한다. 같은 agent의 turn은 native에서 차례로 진행하므로, 정상
요청이 이 거부를 받으려면 요청 도중 그 agent의 다음 turn이 시작되어야 한다(중단 후 새 입력 등).

남는 한도는 agent마다 마지막 turn의 기록과 turn 정보 없는 기록(`--bare`)의 합이다. 끝난 자식의
마지막 turn 기록은 남는다. 자식이 아주 많은 세션은 여전히 16,384에 닿을 수 있다.

issue 본문은 `--bare`와 독립 auxiliary 키를 세션 전체로 두라고 했다. 이 묶음은 #61을 따라 독립
auxiliary를 root turn 범위로 바꿨다. `--bare` 키는 세션 전체로 남는다.

## #62 — 모듈 순번을 시계에서 시작

plugin API 선언은 `session.start`가 plugin의 enable·worker respawn·reload 때 다시 실행된다고 적는다.
다시 등록된 모듈은 `turnSequence`를 1부터 셌고, gateway는 가장 큰 순번을 현재 turn으로 본다. 수정 후
모듈은 첫 게시 전에 `$.clock.now()`(epoch ms)를 한 번 읽어 순번의 시작으로 쓴다. 한 인스턴스 안에서는
1씩 올라가므로 gateway의 정리 기준(최근 32개 게시)은 그대로다. 시계가 이전 인스턴스의 시작 시각과
게시 수의 합보다 뒤로 가면 여전히 진다.

실제 native에서 `/reload-plugins`는 모듈을 다시 등록하지 않았다. HEAD 모듈로 입력 두 번,
`/reload-plugins`, 입력을 보냈고 마지막 입력도 성공했다([기록](reload-plugins.txt),
[검사](reload-probe_test.go)). 선언은 바뀐 모듈만 다시 읽는다고 적는다. 알려진 발생 경로는 여전히
없으므로 합성 재현으로 판정했다. 모듈 검사(`native_events_publication_test.mjs`)는 같은 디렉터리에
모듈을 다시 등록하고, 새 게시가 가장 큰 순번인지 확인한다.

## 판별력

수정을 하나씩 되돌리면 해당 검사가 실패한다([기록](mutations.txt)).

| 되돌린 것 | 실패한 검사 |
|---|---|
| auxiliary 키에서 root turn 제외 | `TestIndependentAuxiliaryIsSpentPerRootTurn` |
| 이전 turn 기록 삭제 | `TestANewTurnForgetsItsAgentsEarlierExecutions` (용량) |
| 이전 turn을 읽은 요청의 거부 | `TestANewTurnForgetsItsAgentsEarlierExecutions` (예약됨) |
| 읽기 단계가 다시 읽음 | `TestARequestReadsItsTurnReceiptOnce`, `TestARequestKeepsTheTurnItArrivedIn` |
| 선택이 다시 읽음 | `TestClaimAndSelectionUseThePinnedTurn` |
| claim이 다시 읽음 | `TestClaimAndSelectionUseThePinnedTurn`, `TestARequestKeepsTheTurnItArrivedIn` |
| 모듈의 시계 시작 (HEAD 모듈 포함) | `native_events_publication_test.mjs` |

## 검사

Go 1.27.1. gofmt·vet(기본, `runtime_evidence`, `policy_evidence`)·build, CI의 오프라인 증거 검사,
race 전체 19 package가 통과했다. 일반 전체의 첫 실행은 `TestAGrandchildEndsWithTheSessionThatStartedIt`
하나가 실패했다. 이 검사는 자식 `cmd`의 PID를 부모로 가진 프로세스를 손자로 보고, 800ms 뒤 그
PID가 살아 있는지 본다. 살아 있다고 나온 PID는 이 검사가 만들지 않은 `bun.exe`였고, 앞선 날짜
21:28:01에 생성됐으며 부모 PID 6688이 기록돼 있었다. 검사의 자식이 재사용된 PID 6688을 받아
무관한 프로세스를 손자로 센 것이다. 이번 변경은 프로세스 정리 코드를 건드리지 않는다. 그 검사만
10회 반복해 모두 통과했고, 일반 전체를 다시 돌려 19 package 모두 통과했다(최상위 829 통과, 3 SKIP).
전체 결과는 [go-test.txt](go-test.txt)에 있다. Node `src`는 130 통과·1 skip이고,
`verification/`·`poc/`의 `test-*.mjs`(문서 인용 실패 0 포함)와 PowerShell 검사 6개도 통과했다.
