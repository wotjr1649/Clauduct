# v0.6.5 — 숨긴 모델 위임 제외, native 명령행 길이 검사

2026-10-03. v0.6.4 출하 뒤 확인한 두 공백을 고쳤다
([PR #242](https://github.com/wotjr1649/Clauduct/pull/242)). 같은 날 v0.6.4 `/context` HTTP 499의 원인 설명도
정정했다([PR #241](https://github.com/wotjr1649/Clauduct/pull/241)). 지원 범위는 [COMPATIBILITY.md](COMPATIBILITY.md)의
v0.6.5 절을 따른다.

## 바뀐 것

- **숨긴 모델**
  - 계정이 picker에서 숨긴 모델(visibility가 `list`가 아님)은 위임 메뉴, Agent `model` 선택지, 그 모델만 받는
    `effort` 값에서 빠진다. `/model`은 v0.6.4부터 이미 뺐다.
  - 전체 ID를 지정하면 Agent `model` 인자와 Workflow는 그대로 실행한다.
- **native 명령행 길이**
  - 위임 메뉴와 `/model` 목록은 native 시작 인자로 넘어간다.
  - 시작하기 전에 Windows `CreateProcessW` 한도(NUL 포함 32,767자)와 비교한다. 넘으면 job·프로세스를 만들기 전에
    `NATIVE_COMMAND_LINE_TOO_LONG`과 위임 메뉴 항목 수를 알리고 멈춘다. 목록은 자르지 않는다.
  - 모델 하나당 약 416자가 든다. 현재 형태에서는 표시 모델이 약 73개를 넘을 때 해당한다.
- **로컬 race 검사 명령**: `internal/app`은 race에서 10분을 넘는다(641.7초). 그래서 `-timeout=120m`으로 실행한다.

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 | `v0.6.5` → `1786d30ff89b7fbc79a837685c129c7ba9b756a7` |
| `clauduct.exe` | `1dc2422db55d4630deb79f927c1a982bb735e20ec9ae10cefb86bceccf5c31fd` |
| `install.ps1` | `5a983cd794497d57c484e15542613cb6e861d133038523cd54942789e9051460` |
| `uninstall.ps1` | `ff339b7674a309e5add69d80b45aad7ad7980d00d5e1db04e348aa0a96e1d087` |
| `SHA256SUMS` | `3937ca63b958a155d007d5ef91843c24035c111a35d6ea25cd14754cfc18343d` |
| 빌드 | 깨끗한 태그 worktree, Go 1.27.1, `CGO_ENABLED=0`, `-trimpath`, 독립 캐시로 두 번 빌드해 바이트 일치 |

## 검증

- **로컬**
  - gofmt·vet·build 통과, `go test` 22개 패키지 통과, 문서 인용 검사 통과.
  - `-race`: `internal/app`을 뺀 패키지는 그대로 통과했다. `internal/app`은 기본 10분 제한에 걸려 `-timeout=60m`으로 다시
    돌렸고 통과했다. DATA RACE나 실패는 없었다.
  - 독립 리뷰 2회. 지적 1건(숨긴 모델에만 있는 effort가 enum에 남는 문제)을 고쳤다.
  - Windows 명령행 경계는 이 검사가 들어가기 전에 실측했다: 32,766자는 시작, 32,767자는 OS가 "The filename or
    extension is too long."으로 거부. 지금은 그 전에 Clauduct가 거부한다. 한글처럼 여러 바이트인 문자도 UTF-16
    단위로 센다.
- **native 2.1.288 + 합성 backend(과금 없음)**
  - `/context` 계수는 Esc로 화면을 닫아도 계속 진행되고, 세션 종료에서만 끊긴다.
  - v0.6.4 출하 검사 순서(Esc 직후 `/exit`)는 같은 499를 재현했다.
- **실제 backend(사용자 승인, 이 릴리스 바이트)**: 17개 시나리오 통과.
  - 이번 변경 4개:
    - 위임 메뉴 9개(표시 8 + `inherit`, v0.6.4는 11)
    - 표시 모델 Agent 인자
    - 숨긴 `gpt-reserve`를 전체 ID로 지정해 실행
    - TUI `/model`·`/context`(Esc 뒤 20초 대기): 계수 20건 성공, 세션 실패 0
  - v0.6.4 회귀 13개:
    - 계정 목록·동기화
    - Sol 6.1
    - `gpt-5.5` 계정 기본값
    - Agent 메뉴·인자
    - 은퇴 표의 계정 제공 모델
    - classifier Terra·Luna·미제공 pair
    - 모델 전환·자동 압축
    - 권한 4종
    - v0.6.3 설정의 첫 시작
- **설치**: PowerShell 7.6.6에서 다음을 모두 통과했다.
  - 새 설치
  - v0.6.4 설치 후 새 설치 스크립트로 교체·되돌리기
  - 설치본 세션(`ship-session`, 7회, 실패 0)
  - 발행 후 검증(API digest·내려받은 바이트)
  - `install.ps1 -Tag`
  - v0.6.4의 `--update --yes`
  - `.old` 정리
  - 두 번째 `--update` 무변경
- **이 머신의 실제 설치본**: v0.6.4에서 `--update --yes`로 v0.6.5가 됐다. digest 일치, `.old` 정리, 설정 변경 없음.

## 알려진 제한

- 명령행 한도 초과 경로는 합성 목록(151개 모델, 64,195자)으로만 확인했다. 실제 계정에서는 아직 재현할 수 없다.
- 선택지 목록은 권고일 뿐 강제가 아니다. 모델이 숨긴 모델의 ID를 직접 적으면 실행된다.
- `/context` 직후 몇 초 안에 세션을 끝내면, 끝나지 않은 계수 요청이 HTTP 499 `CANCELLED`로 종료 줄과 실패 집계에
  남을 수 있다. 계수 오류는 아니다.
- 분류기 판정을 읽을 수 없을 때 native가 같은 바이트로 다시 보낸 요청은 replay 차단으로 거부된다(v0.6.3부터 같다).
  그 도구는 실행되지 않는다.
- v0.6.4의 다른 알려진 제한은 그대로다([v0.6.4 기록](RELEASE-v0.6.4.md#알려진-제한)).
