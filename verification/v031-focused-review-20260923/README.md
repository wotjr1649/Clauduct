# v0.3.1 전송·취소 경로 집중 리뷰

2026-09-23. 대상은 후보 `99b681f`의 전송·취소 경로 8개 파일이다: `httpguard/connection.go`와
gateway의 `request_replay.go`, `native_cancellation.go`, `messages.go`, `gateway.go`,
`native_events.go`, `diagnostics.go`, `errors.go`. `/code-review high`를 1회 실행했고, 그 결과는
코드를 읽고 추론한 것이다. 지적마다 코드·설계 문서·실제 native 실행으로 다시 판정했다.
native는 Claude Code 2.1.280이고, 아래 재현은 모두 로컬 합성 backend를 사용해 과금이 없다.
처음에는 2.1.278로 잘못 적었다. 검사가 실행하는 `~/.local/bin/claude.exe`는 2026-09-23 01:58(KST)부터
2.1.280과 같은 파일이고, 리뷰 대상 `99b681f`는 같은 날 09:33(KST)에 커밋됐다
([정정 근거](../v032-client-2.1.280-20260923/README.md)).

## 판정

| # | 지적 | 판정 | 처리 |
|---|---|---|---|
| 1 | backend가 거절한 요청의 재시도가 같은 step 키에 막힌다 | 컨텍스트 초과는 오탐. 429는 설계 | 없음 |
| 2 | 요청 실행 기록이 세션당 16,384개에서 멈춘다 | v0.3.1 설계, 문서화됨 | [#63](https://github.com/wotjr1649/Clauduct/issues/63) |
| 3 | 본문이 같은 독립 auxiliary 요청은 세션 동안 한 번만 실행된다 | low, v0.3.1 | [#61](https://github.com/wotjr1649/Clauduct/issues/61) |
| 4 | 같은 turn의 다른 요청 업로드가 취소된다 | 오탐 | 없음 |
| 5 | 세션 헤더가 없는 root 요청이 거부된다 | fail-closed. 실제 native는 헤더를 보낸다 | 없음 |
| 6 | turn 순번이 리셋되거나 한도에 걸린다 | 리셋은 low, v0.3.1. 4096 한도는 v0.3.0 결함 | 리셋은 [#62](https://github.com/wotjr1649/Clauduct/issues/62), 한도는 수정 |
| 7 | 거부 전에 본문을 비운다 | 의도된 동작, 효율 문제 | [#66](https://github.com/wotjr1649/Clauduct/issues/66) |
| 8 | 요청마다 영수증 디렉터리를 세 번 읽는다 | low, v0.3.1 | [#64](https://github.com/wotjr1649/Clauduct/issues/64) |
| 9 | 최근 요청 기록이 핸들러 소유 포인터를 붙잡는다 | low, v0.3.1. 기록 16개로 제한 | [#65](https://github.com/wotjr1649/Clauduct/issues/65) |
| 10 | `watchReadCancellation`이 httpguard 함수를 그대로 넘긴다 | low, v0.3.1 | [#67](https://github.com/wotjr1649/Clauduct/issues/67) |
| 추가 | `/clear` 뒤의 모든 요청이 거부된다 | high, v0.3.0부터 | 수정 |

1번의 컨텍스트 초과: native가 step 1에서 backend 초과를 받고 압축한 뒤, 재시도를 step 2로 보냈다.
실행 키가 달라 차단되지 않았고 응답을 이어받았다([기록](overflow-keys.txt)). 429 뒤 자동 재시도를
막는 것은 `docs/v2/ARCHITECTURE.md`에 적힌 정책이다.

4번: `native_cancellation.go`는 `main`·`subagent`·`workflow` 요청에만 취소 바인딩을 만든다.
auxiliary 요청은 다른 요청의 교체로 업로드가 취소되지 않는다.

6번의 리셋: `/clear`에서 plugin은 다시 등록되지 않았다. 순번이 1에서 2로 이어졌다
([기록](clear-receipt-trace.txt)).

## `/clear` 결함

native는 `/clear`에서 같은 프로세스로 새 세션을 시작한다. plugin의 `session()`은 처음 읽은 세션 ID를
보관했다. 그래서 이후 turn 영수증은 옛 세션 ID를 적었고, 새 세션 헤더와 대조한 gateway가 요청을
거부했다. 계측 기록에서 `/clear` 뒤 요청의 헤더와 영수증 세션이 달랐다([기록](clear-receipt-trace.txt)).

| 대상 | `/clear` 뒤 요청 |
|---|---|
| v0.3.0 `149068e` | `PARENT_WAIT_UNVERIFIED` ([기록](clear-v0.3.0.txt)) |
| 후보 `99b681f` | `AGENT_SELECTION_UNVERIFIED` ([기록](clear-before.txt)) |
| 수정 후 | 응답 정상 ([기록](clear-after.txt)) |

수정은 영수증마다 현재 세션 ID를 읽는다. 회귀 검사는 `TestNativeClearSignsLaterReceiptsWithTheNewSession`이며,
실제 native에 stream-json으로 입력, `/clear`, 입력을 보낸다. `/clear`가 세션 ID를 바꿨는지도 확인한다.

## 장기 세션 한도

v0.3.0의 plugin은 게시한 turn, 진행 기록을 둔 agent, 취소 turn을 각각 4096개까지만 기억하고
줄이지 않았다. 한 native 프로세스가 이를 넘기면 이후 요청이 `CLAUDUCT_NATIVE_EVENT_LIMIT`로 멈췄다.
후보는 여기에 전역 게시 순번 8192와 gateway의 순번 상한 8192를 더했다.

수정 뒤 plugin은 agent마다 현재 turn의 게시 하나만 기억한다. turn이 끝나면 게시 자리와 자식의 진행
기록을 반환하고, 취소 turn은 오래된 것부터 잊는다. 4096은 동시에 진행 중인 agent 수의 한도로만 남는다.
gateway는 JavaScript 정수 범위까지 순번을 받는다. 읽을 때 항목이 256개를 넘으면 최신 32개 게시만
남기고 지난 영수증 파일을 지운다. 남는 한도는 동시 진행 agent 4096, 요청 실행 기록 16,384(#63),
정리가 계속 실패할 때의 디렉터리 항목 16,384다.

검사는 plugin의 `native_events_publication_test.mjs`와
`TestLongSessionReceiptsPassTheFormerCapAndStayPruned`다. 수정 여섯 가지를 하나씩 되돌리면 모두 실패한다
([기록](capacity-mutations.txt)).

## 실제 backend 재검증

2026-09-23T02:01Z, 이 기록과 같은 커밋의 소스로 [live-clear_test.go](live-clear_test.go)를 실행했다.
`gpt-5.6-luna`/`low`, ledger 한도 6회, `CLAUDE_CODE_MAX_RETRIES=0`이다. 입력 → `/clear` → 백그라운드
자식 하나에 위임하는 입력을 stream-json으로 보냈다. backend 호출 5회, 거부 0, `/clear` 뒤 세션 변경,
자식 완료 알림에 자식 표식, 자식 생성 요청 200 1건, 최종 결과에 표식, native exit 0, 정리 오류와
gateway 실패 0이다([기록](live-clear.txt)). 기록에는 표식·세션 앞 8자·횟수만 남긴다.

같은 흐름의 fixture 사전 검사는 수정 전 모듈에서 `AGENT_SELECTION_UNVERIFIED`로 실패하고
([기록](live-dryrun-before.txt)) 수정 후 통과했다([기록](live-dryrun-after.txt)). 사전 검사가 처음
만든 흐름의 오류(자식이 백그라운드로 돈다는 점)를 과금 전에 잡았다.

## 검사

Go 1.27.1. gofmt·vet(기본, `runtime_evidence`, `policy_evidence`)·build와 일반 전체 19 package가
통과했다([기록](go-test.txt)). race 전체 19 package도 통과했다([기록](go-race.txt)).

race의 첫 전체 실행에서 gateway의 `TestCloseCancelsEventBodyReads`가 종료 1초 예산을 넘겨 실패했다.
이 검사는 영수증 디렉터리를 설정하지 않아 바뀐 gateway 코드에 도달하지 않고, 단일 실행 순서에서
새 정리 검사보다 앞선다. 같은 실패는 이전 기록(`v031-review-fixes-20260922`의 batch-02, batch-04)에도
있다. gateway만 race 3회 반복하면 새 검사가 파일 600개를 만들 때 두 번 실패했고, 새 검사를 빼거나
HEAD 파일로는 통과했다. 새 검사의 게시 수를 정리 기준을 막 넘는 129개로 줄인 뒤 gateway race 3회
반복 두 번과 위 전체 race가 통과했다. 줄인 검사도 두 수정의 제거를 잡는다([기록](capacity-mutations.txt)).
이 검사는 v0.3.0에도 같은 1초 예산으로 있었다. 업로드가 스스로 끝나지 않으므로 어떤 상한이든 취소되지
않은 읽기를 잡는다. 그래서 예산을 제품의 종료 예산(`defaultShutdownTimeout`, 5초)에 맞췄다. 종료 시 요청
취소를 제거하면 여전히 `context deadline exceeded`로 실패한다.

## 재현

- [clear-probe_test.go](clear-probe_test.go): `go/internal/app`에 overlay로 추가해
  `go test -overlay <json> -run '^TestReviewProbeClearThenPrompt$' ./internal/app`으로 실행한다.
- [clear-trace-probe_test.go](clear-trace-probe_test.go)와 [receipt-trace-probe.go](receipt-trace-probe.go):
  gateway의 `request_replay.go`에서 `readCurrentNativeTurn` 호출 다음 줄에
  `g.reviewProbeNote(key.session, turn.Session, turn.Turn, found)`를 넣은 overlay와 함께 실행한다.
- [overflow-probe_test.go](overflow-probe_test.go)와 [ledger-key-probe.go](ledger-key-probe.go):
  같은 파일에서 `ledger := &g.executions` 앞에 `reviewProbeRecord(key)`를 넣은 overlay와 함께 실행한다.

계측 overlay는 제품 소스에 들어가지 않는다.
