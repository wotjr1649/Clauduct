# v0.6.4 — 계정 모델 목록, native 권한 위임, classifier pair, 설정 동기화

2026-10-03. 모델 목록을 Codex 계정에서 받아 새 모델을 Clauduct 릴리스 없이 고를 수 있게 했다.
권한 판단은 Claude Code(native)에 맡긴다. 분류기 모델·effort는 독립 pair로 정하고, 두 effort 상한은
은퇴시켰다. 업데이트 후 첫 실행에서는 빠진 최상위 설정만 채운다
([#238](https://github.com/wotjr1649/Clauduct/issues/238), [PR #239](https://github.com/wotjr1649/Clauduct/pull/239)).
설정 계약은 [SETTINGS.md](SETTINGS.md), 지원 범위와 실측 표는 [COMPATIBILITY.md](COMPATIBILITY.md)의 v0.6.4 절을 따른다.

## 바뀐 것

- **계정 모델 목록**
  - 일반 세션 시작마다 Codex 계정 목록을 세션의 credential provider로 받는다(`GET models?client_version=`).
  - 그 세션의 표시·라우팅·Agent/Workflow·native hook·재개가 같은 목록을 쓴다.
  - 조회에 실패하면 같은 계정의 마지막 정상 목록(`~/.clauduct/account-models.json`)을 쓴다. 그것도 없으면 시작하지 않는다.
  - 내장 `models.json`은 기존 이름과 은퇴 표만 담고, 선택 범위를 제한하지 않는다.
- **모델·effort**
  - effort는 명시값 → `modelDefaults` → 계정 기본값 순서로 정한다. low~max만 보내고, 지원하지 않는 값은 낮추지 않고 오류로 처리한다.
  - 내장 이름이 없는 모델은 전체 ID로 고르고, 위임 메뉴는 `clauduct-<ID>`다.
  - 계정에 없는 설정 항목은 보존하고 경고한다. 실제로 선택할 때만 오류가 난다.
- **권한**
  - Clauduct가 더하던 `permissions.ask` 17개, `autoMode.hard_deny` 2개, bypass 분기를 제거했다. 요청 출처 증명은 유지한다.
  - auto 모드의 도구 실행은 native 규칙과 native 분류기만으로 결정된다.
- **`classifier_model`**
  - `{"model","effort"}` 객체다(공장값 `gpt-5.6-terra`/`low`). 대화·Agent와 독립이고, 낮추는 상한이 없다.
  - 허용 목록과 tier 제한을 없앴다. v0.6.3의 문자열은 고치는 방법을 안내하는 설정 오류다.
- **effort 상한 제거**
  - `auxiliary_effort_cap`·`auto_compact_effort_cap`을 적용하지 않는다. 파일에 남은 두 키는 은퇴 키로 보존하고 안내한다.
  - 자동·수동 압축은 현재 선택(Agent route 또는 압축 요청의 모델·effort)을 쓴다.
- **설정 동기화**
  - 일반 시작, `--dev --sync-settings`, `--update`(설치된 새 바이너리로 실행), `install.ps1`에서 새 기본 문서의 최상위 키 중 없는 것만 끝에 덧붙인다.
  - 덧붙이기 전에 원본을 바이트 그대로 백업하고, 완전한 파일은 쓰지 않는다.
- **공장값과 측정 기준**
  - 공장 설정은 시작 Sol 6.1/high, Plan·general-purpose Sol 6.1/high, Terra medium이다. `gpt-6-sol` 기본 effort 항목은 없다.
  - 측정 기준은 Claude Code 2.1.288, Codex CLI 0.160.0이다.

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 | `v0.6.4` → `3273786f0c0031d53880b27380aed93ce80a0ed8` |
| `clauduct.exe` | `d8039d3315bb23691ed02279dee256b005cd7cfa0b1da79b0658d975f33ecb25` |
| `install.ps1` | `5a983cd794497d57c484e15542613cb6e861d133038523cd54942789e9051460` |
| `uninstall.ps1` | `ff339b7674a309e5add69d80b45aad7ad7980d00d5e1db04e348aa0a96e1d087` |
| `SHA256SUMS` | `ba59f50e1786470c1fbedd058b9398e3c1af1fd9ef80974483108cd07f49638c` |
| 빌드 | 깨끗한 태그 worktree, Go 1.27.1, `CGO_ENABLED=0`, `-trimpath`, 독립 캐시로 두 번 빌드해 바이트 일치 |

## 검증

- **로컬**
  - gofmt·vet·build를 통과했고, `go test`·`-race`가 각 22패키지에서 통과했다. 문서 인용 검사도 통과했다.
  - 독립 리뷰 결함 2건을 고쳤다: classifier pair가 sonnet 매핑에 묶이던 문제와, `--model`만 줄 때 시작 effort로 대체하던 문제다.
  - 핵심 분기 변이 18개 중 16개를 검출했다. 나머지 2개는 동작이 같은 변이다.
- **native 2.1.288 + 합성 backend(과금 없음)**
  - 내장 이름이 없는 모델로 세션·메뉴·Agent 인자 경로가 실행됐다.
  - 압축이 현재 선택을 썼다.
  - classifier 판정 실패 6종에서 도구가 실행되지 않고 turn이 끝났다.
  - 설정 파일이 다른 프로세스에 잡혀 있을 때 원본이 보존됐다.
- **실제 backend(사용자 승인)**: 최종 후보 `bf984f2`와 이 릴리스 바이트에서 각각 다음 14개 시나리오가 통과했다.
  - 계정 목록·동기화
  - Sol 6.1
  - `gpt-5.5` 계정 기본값
  - Agent 메뉴·인자
  - 은퇴 표에 있지만 계정이 제공하는 `gpt-5.6-luna`
  - classifier Terra·Luna·미제공 pair
  - 모델 전환·자동 압축
  - 권한 4종
  - TUI `/model`·`/context`·snapshot
- **v0.6.3 updater 경로**: v0.6.3 updater가 설치한 v0.6.4는 첫 정상 실행에서 v0.6.3이 만든 설정에 `classifier_model`만 덧붙이고 원본을 백업했다(실제 backend).
- **설치**: PowerShell 7.6.6에서 다음을 모두 통과했다.
  - 새 설치
  - v0.6.3 설치 후 새 설치 스크립트로 교체·되돌리기
  - 설치본 세션(`ship-session`, 7회)
  - 발행 후 검증
  - `install.ps1 -Tag`
  - v0.6.3의 `--update --yes`
  - `.old` 정리
  - 두 번째 `--update` 무변경
- **이 머신의 실제 설치본**: v0.6.3에서 `--update`로 v0.6.4가 됐고(digest 일치), `--dev --sync-settings`가 빠진 3개 키를 백업과 함께 추가했다.

## 알려진 제한

- 처음 쓰는 계정은 시작할 때 목록을 받아야 한다. 오프라인이고 저장된 목록도 없으면 시작하지 않는다.
- (2026-10-03 정정) `/context` 직후 몇 초 안에 세션을 끝내면, 끝나지 않은 계수 요청이 HTTP 499 `CANCELLED`로 종료 줄과 실패 집계에 남는다. 모델과는 무관하다. 출시 때 적은 "로컬 계산식이 없는 모델이라 native가 이전 계수를 끊는다"는 설명은 틀렸다. Esc로 화면을 닫는 것만으로는 끊기지 않으며, 계수가 끝난 뒤 종료한 실제 backend 재검사에서는 실패가 0건이었다. [호환성 문서](COMPATIBILITY.md#v064--계정-모델-목록-권한-규칙-제거-설정-동기화)에 정정 근거가 있다.
- native가 effort를 명시한 자체 보조 요청은 그 effort 그대로 실행된다. 압축도 high·max 세션이면 그 effort로 실행된다. 출하 뒤 1회씩 잰 압축 요청 시간은 medium 12.2초, high 21.7초, max 26.1초였다(같은 입력, 추론 137·171·354토큰).
- 분류 품질은 v0.6.3의 Terra·Sol 표본에서만 측정했다. 계정 목록에 있다는 것은 품질 보증이 아니다.
- v0.6.4보다 오래된 버전은 객체형 `classifier_model`을 읽지 못한다. 되돌리기 전에 남겨 둔 백업으로 설정을 복원한다.
- 세션 중 계정 전환은 기존 거부 계약을 유지하며, 이번 릴리스에서 다시 측정하지 않았다.
- 계정 목록에 따르면 `gpt-5.5`는 2026-10-14 은퇴 예정이다(후속 `gpt-6.1-sol`). Clauduct는 목록에서 빠진 모델을 자동으로 대체하지 않는다.
