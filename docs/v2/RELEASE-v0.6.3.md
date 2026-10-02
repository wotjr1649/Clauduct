# v0.6.3 — GPT-6.1 Sol, 분류기 모델 설정, bypass 시작

2026-10-02. GPT-6.1 Sol을 기본 Sol 단계로 추가하고, auto 권한 모드 분류기의 모델을 설정으로 고르게 했다.
사용자가 bypass로 시작한 세션에는 Clauduct의 필수 확인 목록을 넣지 않는다. 모델 목록과 측정 기준 버전은
코드에서 내장 제품 자료로 옮겼고, Claude Code 2.1.287의 기록 형식 변화에 맞췄다
([PR #236](https://github.com/wotjr1649/Clauduct/pull/236)).

## 바뀐 것

| 범위 | 내용 |
|---|---|
| GPT-6.1 Sol | `sol` 키·`opus` 별칭·공장 시작 모델이 `gpt-6.1-sol`이다. 등록 측정(probe accept)에서 low~max가 수락됐고 ultra는 HTTP 400으로 거부됐다. 도구·reasoning·이미지는 정상이었고 로컬 계수는 5/5 일치했다. `gpt-6-sol`은 전체 ID·`sol6` 키·`clauduct-sol6` 메뉴로 계속 쓴다. Agent 위임 때 native 인자에는 `opus` 단계로 적지만, 실제 경로는 native 생성 이벤트가 전체 ID로 고정한다. 사용자 설정에 적어 둔 모델은 그대로 따른다. |
| `classifier_model` | `~/.clauduct/settings.json`에서 auto 분류기 모델만 고른다. 분류기 지원 범위(Terra 이상) 밖이나 모르는 값은 시작할 때 거부한다. 기본은 `sonnet` 매핑(Terra)이다. 그 모델로 route를 만들 수 없으면 거부하고 Sonnet 경로로 넘기지 않는다. [SETTINGS.md](SETTINGS.md) |
| bypass 시작 | `--permission-mode bypassPermissions`나 native `permissions.defaultMode`로 bypass를 시작하면 필수 ask 목록을 넣지 않는다. 다음 경우에는 목록을 유지한다: 관리 정책(`managed-settings.json`·`managed-settings.d`·`HKLM`/`HKCU` 정책 키)이 있을 때, 어느 설정이든 `disableBypassPermissionsMode`가 있을 때, 해석하지 못한 인자가 있을 때, 설정을 읽을 수 없을 때. 상태 파일 `requiredAsk`에 결정을 기록한다. step 출처 확인은 그대로 유지한다. |
| 하드코딩 제거 | 모델 목록·키·별칭·effort·계수 허용·은퇴 매핑(`models.json`)과 측정 기준 클라이언트 버전(`measured-clients.json`)을 Go 상수에서 내장 제품 자료로 옮기고, 시작할 때 검증한다. 설치된 버전은 실행할 때마다 읽고, 재측정 `--accept`가 기준 파일을 갱신한다. |
| Claude Code 2.1.287 | 측정 기준을 Claude Code 2.1.287·Codex CLI 0.159.3으로 옮겼다. 무과금 재측정 결과, Codex는 기존과 같았다. native는 `--desktop`이 추가됐고(이미 거부한다) `--client-data-url`이 제거됐으며, plugin API 변경은 추가뿐이었다. `/plugin-types`가 없어져 재측정 도구는 바이너리에 압축된 선언을 정적으로 읽는다. 2.1.287은 로컬 명령 caveat와 `/model` 결과 문구도 바꿨다. 두 형식을 모두 읽어 세션 중 `/model`·`/effort` 변경을 UUID 재개 snapshot에 다시 반영한다. |
| 위임·복원 | 별칭 없는 모델의 Agent 위임, Workflow 카탈로그 빈 키, v0.6.2 시절 `opus`+`gpt-6-sol` 자식 기록 복원을 고쳤다. |

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 / commit | `v0.6.3` / `19c638e9d592612696dd9924c41d2cc29987a6fc` |
| 바이너리 SHA256 | `0a575d32a1670d991a246639229db07d4a0a5a956bfecd75b1d6167bd24f9d63` |
| 빌드 | Go 1.27.1, Windows amd64, CGO=0, trimpath, clean tag, 독립 캐시 재현 빌드 일치 |
| 측정한 환경 | Claude Code 2.1.287, Codex CLI 0.159.3, PowerShell 7.6.6 |
| 자산 | clauduct.exe · install.ps1 · uninstall.ps1 · SHA256SUMS |
| Release | [v0.6.3 정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.3) |

## 검증

| 범위 | 관측 결과 |
|---|---|
| 출하 소스 | 같은 tree에서 일반·race 각 22패키지, gofmt·vet·build를 통과했다. 새 동작마다 회귀 테스트를 두고, 수정을 빼면 실패하는지(mutation 검출)도 확인했다. 독립 정적 리뷰는 3회였다. 1회차 6건과 2회차 1건을 반영했고, 2.1.287 기록 수정 리뷰는 NO_FINDINGS였다. PR CI도 통과했다. |
| 최종 후보와 출시 bytes | 실제 backend(Sol 6.1/Luna 6) 표본 25개가 최종 후보와 출시 bytes 각각에서 모두 통과했다. 표본 내용: Read, 확인 필수 도구 거부, SDK, SendMessage, auto 모드 무승인 push 차단, background(Luna 역할의 자식은 별칭 없는 Sol 6), 30초 넘는 background, 출력 전·후 취소, `/context`·`/agents`·`/list-agents`·`/mcp`, `/effort`·`/model`(snapshot 반영), bypass 시작, 다른 세션 SendMessage 승인, `/compact`. |
| 검색 | 검색을 따로 허용한 원장(상한 40)에서 gateway `WebSearch`와 native `WebFetch`를 정확한 입력 1회 승인으로 확인했다. 공유 원장의 검색 금지 정책은 바꾸지 않았다. |
| 분류기 | 같은 빌드에서 Terra와 Sol 6.1을 비교했다. Terra low는 오허용·오차단 0, medium은 오허용 0·오차단 1이었고 중앙 응답은 약 4초였다. Sol 6.1 low는 중앙 응답이 약 16초였고, 보안 판정 표본 1건을 backend가 `cyber_policy`로 두 번 연속 거부했다. 그래서 기본값은 Terra로 유지한다. auto 모드에서 단순 Read·Write(작업 폴더 안)·Bash는 분류기를 거치지 않았다. |
| #234 대조 | 별도 원장으로 같은 native 2.1.287과 같은 단계를 썼을 때, attach 직후 반복 응답은 v0.6.1과 v0.6.2 모두 4회 중 2회였다. v0.6.2 회귀가 아니다. |
| 설치·공개 업데이트 | PowerShell 7에서 새 설치, v0.6.2에서의 업데이트·되돌림, 설정·PATH 보존을 확인했다. 다운로드 bytes와 API digest를 대조했고, 공개 updater, `.old` 정리, 최신 상태 no-op, 0.3.x updater의 안전 거부, 설치본 세션(normal/repeat)도 확인했다. |
| 실제 설치 | 표준 사용자 설치 경로를 v0.6.2에서 v0.6.3으로 바꿨다. doctor에서 두 클라이언트 모두 측정 기준 버전으로 나왔고, 업데이트 no-op을 확인했다. 사용자 설정은 바이트 단위로 그대로였다. |

준비 단계의 실패는 모두 원본을 보존했고, 최종 PASS에 합산하지 않았다. 실패 원인은 다음과 같다.
- backend 스트림 절단 1회(`TRUNCATED_STREAM`으로 올바르게 처리)
- 상태 파일 쓰기 경합 1회(최종 기록은 표준 오류로 출력됨)
- 모델의 잘못된 도구 호출 2회(입력이 빠진 Agent 호출, 같은 SendMessage 반복)
- 하네스 조건 문제

## 알려진 제한

- 세션이 끝날 때 다른 프로세스가 상태 파일을 읽고 있으면 파일 갱신이 실패할 수 있다. 이때 최종 기록은 표준 오류에만 남는다.
- 분류기가 판정을 받지 못했을 때(예: backend 정책 거부) native가 어떻게 처리하는지는 측정하지 않았다.
- `/model`에서 Enter로 고르면 native가 그 모델을 자기 설정의 기본값으로 저장한다. 이번 세션에만 쓰려면 `s`를 쓴다.
- 새 `/model` 문구는 native 버전과 무관하게 읽는다. effort가 생략된 모델 변경은 기존 effort를 유지한다고 보는데, 근거는 실측 1건이다.
- bypass 세션 도중 다른 모드로 바꿔도 필수 확인 목록은 시작 때의 결정을 따른다.

기존 v0.6.2 태그와 자산은 보존한다. 설치와 되돌리기는 [PACKAGING.md](PACKAGING.md), 설정은 [SETTINGS.md](SETTINGS.md)를 따른다.
