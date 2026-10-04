# v0.6.9 — native 2.1.289 기준, Agent teams, TUI subagent 부분 보고 제거

2026-10-04. 범위는 [#275](https://github.com/wotjr1649/Clauduct/issues/275)이다. 남은 제한과 다시 볼 조건은
[COMPATIBILITY.md](COMPATIBILITY.md)의 3절 "알려진 제한 원장 (v0.6.9)"이 관리한다.

## 바뀐 것

- **Agent teams(teammate) 지원**([#269](https://github.com/wotjr1649/Clauduct/issues/269), PR #279)
  - native 2.1.289의 teammate는 HTTP 요청에는 주소(`이름@팀`)를, hook 이벤트에는 루프 ID를 싣는다. v0.6.8은 그 요청을 모두
    `INVALID_SESSION_ID`로 거부했다.
  - 세션 plugin이 spawn 때 주소와 루프 ID, Agent 호출·역할의 대응을 기록하고, gateway는 그 기록이 있을 때만 주소를 받아들인다.
    요청마다 teammate 신원과 중지 여부를 확인하고, 첫 요청은 그 호출의 선택·모델과 대조한다.
  - teammate의 보고는 native가 idle 알림으로 lead에게 전달하므로 gateway가 결과를 따로 추적하지 않는다.
  - 대상은 Windows in-process 모드다. split pane, `-p`, `/resume` 뒤 teammate 복원은 native 제한이다.
- **TUI subagent의 부분 보고 제거**([#264](https://github.com/wotjr1649/Clauduct/issues/264), PR #276)
  - subagent의 text를 TUI에서도 완료까지 보류한다. 응답 도중 backend가 실패하면 부분 보고 대신 자식의 명시적 API 오류가 된다.
    root TUI는 계속 스트리밍한다.
- **측정 기준 Claude Code 2.1.289**([#267](https://github.com/wotjr1649/Clauduct/issues/267), PR #280)
- **hosted web search `max_uses`**([#272](https://github.com/wotjr1649/Clauduct/issues/272), PR #278): 1 이상의 정수만 받는다.
  검색은 side query마다 한 번이라 상한 의미가 지켜진다.
- **공개 contract test**([#271](https://github.com/wotjr1649/Clauduct/issues/271), PR #281): credential 없는 합성 protocol
  테스트 31개를 공개하고 공개 CI에서 돌린다. 결정은 [#256](https://github.com/wotjr1649/Clauduct/issues/256).
- **알려진 제한 원장**([#273](https://github.com/wotjr1649/Clauduct/issues/273), PR #282)
- 재측정 도구가 세션 plugin이 쓰는 hook 타입 전부를 비교한다([#268](https://github.com/wotjr1649/Clauduct/issues/268), 비공개
  검증 도구).
- 조사: backend 추론 요약을 화면에만 보여 줄 경로를 확인했다([#270](https://github.com/wotjr1649/Clauduct/issues/270)). 구현은
  [#277](https://github.com/wotjr1649/Clauduct/issues/277).

## native 2.1.289 판정

| 변경점 | 판정 |
|---|---|
| `--help`, Clauduct가 쓰는 hook 이벤트 | 도움말은 같다. `agent.spawn`에 `isTeammate`·`teammateId`, Agent 결과에 `AgentTeammateRecord`가 더해졌다(재측정이 보고) |
| teammate의 `agent.spawn` 경유와 agent ID 통일 | hook은 루프 ID, HTTP 헤더는 주소로 서로 다르다. 위 #269로 대응 |
| 요청 형태 | 비대화형 11경로와 TUI 4경로 모두 2.1.288과 같다 |

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 | `v0.6.9` → `b780a158526058b6e0bddc0e30736c0c8a98691f` |
| `clauduct.exe` | `ff36ef7bd8dde9edb63ed9dee60722ef44254f79c9763a456782551aaeb4b0f5` |
| `install.ps1` | `ed4f70af63d46ae5972396c4d868166bc41202e165967f0fc1000693cf19ed68` |
| `uninstall.ps1` | `eb060a28e1f7d4ed30438774871798cb562bfb4dff4c9b53465b243a4ce06613` |
| `SHA256SUMS` | `8a3f086131a48e48d2315416ae6bc83099b21daa29217b0e00301f5425f3efc4` |
| 빌드 | 깨끗한 태그 worktree, Go 1.27.1, `CGO_ENABLED=0`, `-trimpath`, 독립 캐시로 두 번 빌드해 바이트 일치 |

## 검증 (native 2.1.289 고정)

- **로컬**: gofmt·vet·build, 근거 태그 vet 2종, `go test -timeout=120m`과 `-race`(각 22개 패키지) 통과. 문서 인용 검사 통과.
  작업마다 독립 리뷰를 받고 지적을 반영했다(#264 0건, #268 3건, #269 7건, #271 5건, #272 1건, #273 12건).
- **실제 backend(이 릴리스 바이트)**: 기존 회귀 18개가 첫 실행에서 모두 통과했다. 검증 하네스는 실행마다 설치본 native를
  2.1.289와 sha256으로 대조했다.
- **실제 backend(신규 영향)**
  - TUI subagent를 응답 도중 실패시킨 유효 7회: 부분 보고 0, 모두 명시적 실패.
  - teammate 전체 흐름(spawn, 도구 호출, 보고, idle, `SendMessage`로 다시 시작, 두 번째 보고) 3회 통과.
  - 추론 요약 요청 8회(모델 4종): 모두 받는다. 요약은 짧고 추론이 짧으면 오지 않는다.
- **설치**: PowerShell 7.6.6에서 새 설치, v0.6.8 설치 후 교체·되돌리기, 설치본 세션(7회, 실패 0), 발행 후 검증(대상은 태그의
  commit), v0.6.8의 `--update --yes`, `.old` 정리, 두 번째 `--update` 무변경이 모두 통과했다.
- **이 머신의 실제 설치본**: v0.6.8에서 `--update --yes`로 v0.6.9가 됐다. digest 일치, `.old` 정리, 두 번째 `--update` 무변경,
  설정 변경 없음.

## 알려진 제한

- 남은 제한과 다시 볼 조건은 [COMPATIBILITY.md](COMPATIBILITY.md) 3절 "알려진 제한 원장 (v0.6.9)"에 있다. v0.6.9에서 새로
  생긴 항목은 다음과 같다.
  - native가 teammate 모델을 고르는 경로는 단위 시험으로만 확인했다.
  - 첫 출력 전 240초 뒤 keepalive가 응답을 연 다음의 subagent 실패는 실제 backend로 재지 않았다. 이때도 부분 text는 0이다.
- `gpt-5.5`의 실제 은퇴 뒤 동작은 2026-10-14T19:00Z 이후에 확인한다([#274](https://github.com/wotjr1649/Clauduct/issues/274)).
