# v0.3.2 정리 묶음 (#65–#68)

2026-09-23. `main` `37ea733`에서 시작했다. gateway의 기록 수명(#65), 거부 전 drain(#66),
읽기 취소 watcher와 연결 종료 결정(#67), 저장소의 `AGENTS.md` 무시(#68)를 다뤘다. 측정은 로컬
fixture backend만 쓰며 모델 호출은 0회다.

## 측정

HEAD와 수정본에 같은 [측정 검사](probe_test.go)를 overlay로 붙였다([기록](measure-run.txt)).
HEAD 실행은 바뀐 gateway 파일 8개(검사 파일 포함)를 `main`의 것으로 바꾼다.

| issue | 측정 | HEAD | 수정 후 |
|---|---|---|---|
| [#65](https://github.com/wotjr1649/Clauduct/issues/65) | 요청 20개가 끝난 뒤 최근 기록 16개가 붙잡은 `nativeTurn`·`execution` | 16·16 | 0·0 |
| [#66](https://github.com/wotjr1649/Clauduct/issues/66) | 동시 요청 한도에서 멈춘 업로드의 429까지 | 1s | 1ms |
| [#66](https://github.com/wotjr1649/Clauduct/issues/66) | 32MiB+1 byte 뒤 멈춘 업로드의 413까지(마지막 byte 기준) | 1.011s | 11ms |

측정의 root 요청에는 자식 결과가 없어 `nativeResult`는 양쪽 모두 0이다. 세 포인터를 모두 지우는지는
`TestFinishReleasesHandlerOwnedPointers`가 본다.

## 변경

- **#65** `record.finish`가 `nativeTurn`, `execution`, `nativeResult`를 지운다. `finish`는 요청을 연
  `handle`의 defer에서 handler가 돌아온 뒤에 실행되므로, 이 필드를 읽는 handler의 defer는 모두 끝난 뒤다.
- **#66** 한도 거부(`TOO_MANY_REQUESTS`)와 초과 본문(`INPUT_TOO_LARGE`)은 drain 없이 읽기 deadline을
  지금으로 두고 연결을 닫는다. 한도 거부는 이제 연결을 유지하지 않는다. 느린 업로드를 최대 1초
  기다리지 않는 대신 native는 다음 요청에 새 연결을 쓴다.
- **#67** `watchReadCancellation`을 지우고 세 호출부가 `httpguard.WatchReadCancellation`을 직접 부른다.
  그 함수를 검사하던 두 검사는 `httpguard`로 옮겼다. 취소된 읽기의 `Connection: close`는 `messages.go`에서
  지우고 `refuse`가 취소 거부마다 정한다.
- **#68** `.gitignore`의 `/docs/prompts/` 옆에 `/AGENTS.md`를 추가했다. 이 worktree에서
  `git check-ignore -v AGENTS.md`는 `.gitignore:16:/AGENTS.md`를 가리킨다. 원래 checkout의
  `.git/info/exclude` 항목은 그대로 둔다.

## 판별력

수정을 하나씩 되돌렸다([기록](mutations.txt)).

| 되돌린 것 | 결과 |
|---|---|
| `finish`의 포인터 해제 | `TestFinishReleasesHandlerOwnedPointers` 실패 |
| 한도 거부의 drain 생략 | `TestClosingRefusalsDoNotWaitForASlowUpload/busy` 실패(1.002s) |
| 초과 본문의 drain 생략 | `TestClosingRefusalsDoNotWaitForASlowUpload/too-large` 실패(1.013s) |
| `httpguard` watcher를 무동작으로 | `TestTheAbortWatcherStillStopsAReadThatTheClientAbandoned` 실패 |
| 취소 거부를 drain 경로로 | gateway 전체 통과 |

마지막 줄은 동작 차이가 없다는 뜻이다. 취소된 읽기는 읽기 deadline 오류로 끝나고, 그 뒤 drain은 같은
오류를 바로 돌려받아 연결을 닫는다. 이를 확인하려고 만든 HTTP 검사도 되돌린 코드에서 통과해서
저장소에 남기지 않았다. #67은 동작을 바꾸지 않는 정리이며, 기존 취소·거부 검사가 그대로 통과한다.

## 검사

Go 1.27.1. gofmt·vet(기본, `runtime_evidence`, `policy_evidence`)·build, CI의 오프라인 증거 검사,
일반(`CGO_ENABLED=0`)·race 전체 19 package가 통과했다. 일반 실행의 최상위 검사는 826 통과, 3 SKIP이다.
Node `src`는 130 통과·1 skip이고, `verification/`·`poc/`의 `test-*.mjs`(문서 인용 실패 0 포함)와
PowerShell 검사 6개도 통과했다. 전체 결과는 [go-test.txt](go-test.txt)에 있다.
