# v0.5.0 — 요청 세션 식별, 역할·fork 범위, 자산 넷

2026-09-26 출하·실제 설치 확인을 완료했다. 태그 `v0.5.0`은 `d665e18ce32e621c0bcc16942e423c76a869d328`이며
[GitHub Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.5.0)는 latest·정식 릴리스다. Claude Code 2.1.283·Codex CLI 0.157.0 기준이다.
개발 후보의 무료 회귀와 실제 backend 검증은 [호환성 문서](COMPATIBILITY.md#v050--요청-세션-식별-역할fork-범위-자산-넷)에 있다.
[PR #141](https://github.com/wotjr1649/Clauduct/pull/141), [PR #142](https://github.com/wotjr1649/Clauduct/pull/142).

## 바뀐 것

- **Claude Code 2.1.283 인자.** 새 필수값 옵션 `--client-data-url <url>`을 인자 표에 더했다. 이전 빌드는 이 옵션을
  모르는 옵션으로 읽어, 뒤에 온 `--settings`를 `SETTINGS_INVALID`로 거부하고 `--model`·`-p`의 경계를 잃었다.
  서명 구성 문서의 적재·검사는 native가 처리한다(native는 `https://downloads.claude.ai/` 외의 값을 모델 요청 전에 거부).
- **요청 세션 식별(#135).** 모든 생성 요청에 native 세션별 불투명 키를 `prompt_cache_key`와 `session-id` 헤더로
  보낸다. 전송은 HTTP SSE, 본문은 기존 형식 그대로다. 결정 근거와 재검토 조건은
  [호환성 문서](COMPATIBILITY.md#v050--요청-세션-식별-역할fork-범위-자산-넷)에 있다.
- **커스텀 역할 기본값.** `--add-dir`의 역할 정의, `--setting-sources`의 출처 제외, settings의
  `CLAUDE_CODE_SUBAGENT_MODEL`과 그 effort를 native와 같게 읽는다. 이전 빌드는 네 경우에 native와 다른 모델·effort로
  자식을 실행했다(예: 부모 low에서 native sol/low, 이전 빌드 sol/xhigh).
- **subagent 안의 forked Skill.** 검증된 subagent가 부른 `context: fork` skill의 자식을 받아들인다. 이전 빌드는
  `AGENT_SELECTION_UNVERIFIED`로 거부했다. fork 자식의 `SendMessage` 재개를 "거부한다"고 적은 이전 문서도 바로잡았다.
  2.1.283에서 native는 이 재개를 받아들이고, 제품은 같은 자식을 native 영수증과 대조해 실행한다.
- **auto 권한 모드.** Claude Code 2.1.283의 기본값인 auto 모드는 메인 요청마다 Anthropic 서버의 분류기를
  요청해, 이 gateway에서는 턴마다 거부 1건이 생겼다. native의 공식 스위치 `CLAUDE_CODE_AUTO_MODE_SERVER=0`을 기본으로
  둬 이를 없앴다(사용자가 정한 값이 이긴다). native 자체 분류기는 여전히 지원하지 않아, 판정이 필요한 행동은 auto
  모드에서 `Classifier unavailable`로 거부된다. 필요하면 `Shift+Tab`으로 다른 권한 모드를 쓴다.
- **릴리스 자산 넷(#136).** `clauduct.exe`·`install.ps1`·`uninstall.ps1`·`SHA256SUMS`. v0.4.x가 0.3.x updater를 위해
  함께 올리던 `clauduct-hook.exe`·`clauduct-dev.exe` 사본은 더 이상 올리지 않는다.

## 0.3.x 설치에서 올라오는 경우

0.3.x의 `clauduct --update`는 사본이 없는 v0.5.0을 받지 못하고 다음처럼 멈춘다. 설치는 바뀌지 않는다.

```text
clauduct: release v0.5.0 does not carry clauduct-hook.exe, clauduct-dev.exe
```

[README](../../README.md)의 설치 스크립트로 다시 설치한다. 설치 스크립트가 세 파일을 `clauduct.exe` 하나로 바꾼다.
v0.4.x 설치의 `clauduct --update`는 그대로 동작한다.

## 출하 검사 (2026-09-26)

첫 태그 후보(`74d96a7`)는 발행 전에 철회했다. 그 실제 TUI에서 2.1.283 auto 모드의 `safeguards` 거부 2건이
나와 [PR #142](https://github.com/wotjr1649/Clauduct/pull/142)로 고친 뒤, 새 태그 `d665e18`에서 아래 검사를 처음부터 다시 했다.
같은 후보의 TUI 첫 실행은 2.1.283의 화면 문구 변화로 첫 입력 전에 멈췄고(과금 0), 역할 첫 실행은 모델이 Agent를
반복 실행해 판정 기록이 밀려 FAIL했다. 두 실패는 하네스를 보완해 재실행했고 기록을 보존했다.

순수 태그 바이너리의 실제 backend 검증은 다음과 같다. 각 실행은 전송 전에 만든 영속 총상한 안에서 돌았다.

| 실행 | 예약 / 상한 | 결과 |
|---|---|---|
| 실제 TUI 입력·압축·취소·복구·종료 | 8 / 10 | PASS. 의도한 취소 1회, API 실패 0, `safeguards` 거부 0 |
| SDK 입력·`/clear`·자식 위임 | 5 / 6 | PASS. 부모 luna/max, 자식 luna/low, 기준 클라이언트 2.1.283 확인 |
| `--add-dir` 역할·settings env 모델 | 6 / 10 | PASS. 정의는 luna/medium, 모델 없는 정의는 sol/low |
| subagent 안 forked Skill | 6 / 10 | PASS. 손자 `native-fork` luna/low, 부모 subagent 연결 |
| fork 자식의 `--resume` 뒤 `SendMessage` 재개 | 10 / 16 | PASS. 같은 자식이 `native-fork`로 실행, 자식만 아는 재개 코드 확인 |

모두 API 오류·cleanup 실패 0, 종료 시 활성 요청·메모리 예약 0이다. 순수 태그 출하 검증은 **35회**다.
이번 세션 누계는 영속 예산 예약 231회(#135 형식 비교 110회, 개발 후보 40회, 철회한 후보 46회, 발행 태그 35회)와
예산 밖에서 잘못 실행한 추론 2회를 합쳐 233회다.

깨끗한 태그를 Go 1.27.1 windows/amd64·CGO_ENABLED=0·trimpath로 두 번 빌드했으며 독립 빌드 캐시에서도 동일 바이트였다.

```text
935079edcc7603aaa9e592a6e6fb113196c848daf5f6dd34f3376c9b32a3e1d1  clauduct.exe
```

격리 새 설치·v0.4.4에서 교체·v0.4.4 설치 스크립트로 되돌리기를 통과했다. 발행 뒤 자산 넷을 다시 내려받아 후보·SHA256SUMS·
GitHub API digest를 모두 대조했다. 공개 태그 고정 설치, v0.4.4의 실제 updater, 다음 실행의 `.old` 정리와 두 번째
업데이트의 no-op을 통과했다. v0.3.5 설치는 `--update`가 "does not carry"로 멈추고 파일을 그대로 두었고, v0.5.0 설치
스크립트가 세 파일을 `clauduct.exe` 하나로 바꿨다. 격리 검사마다 사용자 PATH는 그대로였다.

실제 사용자 설치본도 v0.4.4에서 `clauduct --update`로 v0.5.0에 올려 버전·commit·바이트·native 실행을 확인했다.
잔여 `.old`는 다음 실행에서 정리됐고 두 번째 업데이트는 no-op이었다. 버전 확인 실행의 backend 시도는 0회다.
