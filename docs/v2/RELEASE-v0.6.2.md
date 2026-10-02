# v0.6.2 — native 확인 helper와 도구 호출 출처 검증

2026-10-02. backend 요청마다 native step의 확인을 받는 경로를 파일 우편함에서 내장 helper의 인증된
loopback 연결로 바꿨다. 모델 도구 호출에는 그 step의 표지를 붙여 실행 직전에 대조한다. v0.6.1 이후 추적한
결함과 원인 조사를 같은 출하에 묶었다([PR #232](https://github.com/wotjr1649/Clauduct/pull/232),
추적 #198).

## 바뀐 것

| 범위 | 내용 |
|---|---|
| native 확인 helper | 확인 응답은 현재 Clauduct 바이너리의 내장 helper가 인증된 loopback 연결로 전달하고, 연결 수명을 그 HTTP 요청에 묶는다. 늦게 도착한 helper는 410·종료 코드 3으로 조용히 끝나며 세션을 잠그지 않는다. 백그라운드(`--bg`) 작업의 helper는 세션 연결로 gateway에 접속한다. 응답을 준비하는 중에 확인 구간이 사라지면 명시적으로 거부한다. |
| 보조 요청 확인 | 같은 step이 아직 추론 중일 때 native가 보내는 `auxiliary` 요청을 그 step의 확인으로 처리한다(예: 30초가 지난 자식 Agent의 진행 확인). step이나 도구 구간이 정상으로 끝나도 이미 받아들인 요청은 그 요청이 끝날 때 닫힌다. turn이 중단·오류·거부로 끝나거나 세션이 끝나면 즉시 닫는다. |
| 도구 호출 출처(#214) | 모델 도구 호출 ID에 해당 native step의 표지(`__cdt` + 12 hex)를 붙이고 실행 시점에 대조한다. 지난 turn·step의 늦은 호출은 거부한다. backend에는 원래 `call_id`를 보낸다. plugin hook 모듈이 직접 실행한 도구(표지 없음)는 native 권한 규칙대로 실행하지만 turn 상태를 쓰지 못하며, 직접 실행한 `Agent`·`SendMessage`·`Workflow`·`Skill`은 거부한다. |
| 권한(#218) | 실행·외부 통신 도구는 native `permissions.ask`에 들어간다. 대상은 `Bash`·`PowerShell`·`Monitor`·`Workflow`·`Skill`·`SendMessage`·`WebFetch`·`WebSearch`·MCP 등이다. 이 도구들은 native 승인 화면이나 `PermissionRequest` hook으로 승인해야 하고, `dontAsk`에서는 거부된다. 보조 요청 effort 상한은 `auxiliary_effort_cap`으로 정한다([SETTINGS.md](SETTINGS.md)). |
| 안정성 | native가 Workflow journal·자식 meta·transcript·Agent meta를 쓰는 중에 읽으면 판정을 미루고 기존 대기 시간 안에서 다시 읽는다(#211/#215). 요청을 보낸 뒤 응답 헤더를 기다리다 timeout이 나면 HTTP/1.1·HTTP/2 모두 `REQUEST_TIMEOUT`으로 분류해 native가 다시 보내지 않게 했다(#213). 같은 turn의 다음 step으로 지연된 요청이 재전송되는 것을 막는다(#226). 종료된 Agent의 이력은 메모리에서 회수한다(#208). turn 밖에서 오는 루트 `auxiliary` 토큰 계수 요청(`/context`)은 step 증명 없이 처리한다(#209). 취소 영수증을 쓰는 중에 읽으면 판정을 미룬다. |
| 진단 | 응답 출력 형태(`outputItems`·`answerChars`, #212/#220), 선택 거부 분기(`selectionRefusal`), 자식 결과 시간표(`timeline`, #206), 세션 중 native 교체(`native_replaced=1`, #229)를 기록한다. |

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 / commit | `v0.6.2` / `cf57beddabc9db316ce5aa5f3fff3e042a567b00` |
| 바이너리 SHA256 | `20fa0e2255ca32b0b43c73570c38ef4d84ff50d2c065ae30a346e979f0a0a6c2` |
| 빌드 | Go 1.27.1, Windows amd64, CGO=0, trimpath, clean tag, 독립 캐시 재현 빌드 일치 |
| 측정한 환경 | Claude Code 2.1.287, Codex CLI 0.159.3, PowerShell 7.6.6 |
| 자산 | clauduct.exe · install.ps1 · uninstall.ps1 · SHA256SUMS |
| Release | [v0.6.2 정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.2) |

## 검증

| 범위 | 관측 결과 |
|---|---|
| 출하 소스 | 같은 tree에서 일반·race 각 22패키지, gofmt·vet·build를 통과했다. 새 동작마다 고장 주입 테스트를 두고, 수정을 제거하면 실패하는지(mutation 검출)도 확인했다. 독립 정적 리뷰는 5회였다. 1회차 6건과 4회차 1건을 반영했고, 마지막 결과는 NO_FINDINGS다. PR CI도 통과했다. |
| 최종 후보와 출시 bytes | 실제 backend(Sol/Luna) 표본 20개가 최종 후보와 출시 bytes 각각에서 모두 통과했다. 표본은 Read 표지 왕복, 확인 필수 도구 거부, SDK, SendMessage, auto 모드 무승인 push 차단, background, 30초 넘는 background 자식, 출력 전·후 취소, `/context`·`/agents`·`/list-agents`·`/mcp` 화면이다. 예상 밖 API·도구·정리 실패는 0이다. |
| 자원(#208) | Sol/Luna 동시 세션을 30턴씩 12분 동안 돌렸고 한도 초과·실패·잔류 프로세스는 0이었다. gateway의 최고 working set은 68MB, handle은 796으로 이전 출하 측정과 같은 수준이다. 하위 프로세스까지 합친 최고치는 1.23GB로, 이전 측정 0.58GB보다 높다. 최고점에는 단명 하위 프로세스 24개가 있었는데, 요청마다 뜨는 확인 helper로 추정한다(프로세스 이름은 기록하지 않았다). 끝날 때는 native 2개와 작은 프로세스 2개로 돌아왔다. |
| 식별자(#209) | 다음 식별자를 실제로 실행했다: `--print`·`--continue`·`--resume`·`--no-session-persistence`·`/compact`, fork Skill과 fork 자식 SendMessage 재개, Workflow 실행·`resumeFromRunId` 재개, `--bare` 거부(backend 0회). 정보 명령과 예상 거부 검사, dev probe도 통과했다. |
| 설치·공개 업데이트 | PowerShell 7에서 새 설치, v0.6.1에서의 업데이트와 되돌림, 설정·PATH 보존을 확인했다. 내려받은 bytes를 API digest·`SHA256SUMS`와 대조했다. 공개 updater의 교체, `.old` 정리, 최신 상태에서의 no-op을 확인했다. 0.3.x updater는 안전하게 거부하고, 새 설치 스크립트로 넘어가는 것도 확인했다. 격리된 설치본 세션(normal/repeat)도 통과했다. |
| 실제 설치 | 표준 사용자 설치 경로를 v0.6.1에서 v0.6.2로 바꿨다. 신원·SHA256·doctor·업데이트 no-op을 확인했고, 사용자 설정은 바이트 단위로 그대로였다. |

준비 단계의 실패는 모두 보존했고 최종 PASS에 합산하지 않았다. 그중 실제 결함은 3건이었고, 모두 고친 뒤 같은
표본으로 다시 통과했다: background helper의 토큰, `/context` 계수, 30초 넘는 background 자식. 나머지는 검증
도구 쪽 문제였다. v0.6.2 권한 정책 이전에 만든 하네스의 기대, native 2.1.287의 형식 변화
(`SubagentHandback`, 거부된 요청의 재전송), 하네스 조건 오류가 여기에 속한다. 이것들은 원본 결과를 남긴 채
파생 도구로 다시 실행했다.

## 알려진 제한

- 출하 전 검증에서 background 작업을 stop/attach한 직후 첫 입력에 모델이 이전 완료 보고를 되풀이한 사례가 4회 중
  3회 있었다. 확인한 사실은 세 가지다. 새 입력은 backend 요청에 들어 있었다. native가 대기 중이던 완료 알림을 같은
  입력 앞에 붙였다. 응답은 backend가 새로 생성한 것이었다. v0.6.1과 같은 조건으로 대조하지 못해 원인은 확정하지
  않았다.
- `/model`·`/effort` TUI, gateway 경유 `WebSearch`·`WebFetch`, 다른 세션으로 가는 `SendMessage` 승인은 이
  바이너리에서 실제로 실행해 보지 않았다. 해당 동작은 Go 테스트로만 확인했다.
- 권한 분류기의 모델·effort 조합별 품질은 출하 후에 측정한다. 분류기가 허용하더라도 필수 ask 도구는 native 확인
  없이 실행되지 않는다. 분류 대상 모델에서 Luna는 제외한다.
- 원인을 확정하지 못한 과거 간헐 실패(#206 #211 #212 #213 #215 #220 #229)는 결정적 고장 주입으로 안전한 처리를
  확인했고, 다음에 원인을 가를 진단도 넣었다. 원인 미확정이라는 기록은 그대로 둔다.
- doctor는 측정 기준(Claude Code 2.1.283, Codex CLI 0.157.1)보다 새 클라이언트에서 `re-measure due`를 표시한다.
  세션은 그대로 실행된다.

기존 v0.6.1 태그와 자산은 보존한다. 설치와 되돌리기는 [PACKAGING.md](PACKAGING.md), 설정과 권한은
[SETTINGS.md](SETTINGS.md)를 따른다.
