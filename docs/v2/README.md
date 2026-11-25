# Clauduct Go V2 — 현행 인덱스

V2(Go Native-Host-Preserving Bridge)의 **현재 상태를 읽는 단 하나의 자리**다.

## 1. 현재 상태 (v0.6.19 출하 준비)

| 항목 | 값 |
|---|---|
| 출하 준비 | **v0.6.19**. Clauduct 설정 파일이 v2 형식으로 바뀌어 v1 파일은 native 시작 전에 거부된다(v2로 옮긴다. 기본값만 쓰려면 백업 뒤 파일 전체를 `{"version":2}`로 바꾼다). 새 세션의 시작 모델은 native의 선택을 먼저 따르고, 비어 있으면 계정 모델 목록의 단독 1순위를 쓴다. 새 세션의 모르는 명시 effort는 인증 확인 전에 거부하고, 끊긴 `--update`는 다음 실행이 기록과 파일 해시를 대조해 복구하거나 멈춘다. 응답의 첫 terminal 이벤트 종류를 진단에 남긴다. [변경·업그레이드 안내·알려진 제한](RELEASE-v0.6.19.md) |
| 최신 출시 | **v0.6.18** (태그 `v0.6.18` → `3f2000f`). Claude Code 2.1.332 auto 모드의 분류 요청을 처리하도록 고쳤다. v0.6.17 이하는 native 2.1.332에서 auto 모드의 분류 요청이 거부된다. [변경·출하 검사와 알려진 제한](RELEASE-v0.6.18.md), [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.18). 그 이전 **v0.6.17** (태그 `v0.6.17` → `cb1edf2`). 선택형 격리 모드를 추가했다. 기본 모드에서 background 작업을 `stop`·`rm`한 직후 세션을 다시 열 수 있고, 쓸 수 없는 모델 목록을 받으면 마지막 정상 캐시를 보존한다. 설치·업데이트·일반 실행은 Clauduct 설정 파일을 자동으로 쓰지 않는다. [변경·출하 검사와 알려진 제한](RELEASE-v0.6.17.md), [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.17). 그 이전 **v0.6.16** (태그 `v0.6.16` → `a450202`). 모든 native에서 같은 규칙으로 background·관리 명령을 판정하고 native 2.1.330 전용 판정 표를 제거했다. 긴 PDF 페이지 Read에 필요한 Poppler 설치 방법을 안내한다. [변경·출하 검사와 알려진 제한](RELEASE-v0.6.16.md), [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.16). |
| 출하 후 후속 | 알려진 제한과 미검증 항목은 [알려진 제한 원장](COMPATIBILITY.md#알려진-제한-원장-v0618)에 분류와 다시 볼 조건을 붙여 한곳에 모았다(v0.6.9, #273). 릴리스별 기록의 "알려진 제한" 절은 그 릴리스 당시의 기록이다 |
| 실행기 | `clauduct.exe` 하나 = Go 빌드. hook과 `--dev` 명령이 같은 파일이다(v0.4.0, #112). v0.6.11부터 진단 전용 `clauduct-probe.exe`가 별도 자산으로 붙는다. launch에는 필요 없고, 설치·업데이트는 둘을 한 쌍으로 다룬다([PACKAGING](PACKAGING.md) 1절·5.1절). 이전 Node 구현은 v0.3.3에서 저장소에서 은퇴했다 — [분리 직전 커밋](https://github.com/wotjr1649/Clauduct/tree/1b1c5e19b3f33fda63254b2da7c9d0b372553481) |
| Go 모듈 | `github.com/wotjr1649/Clauduct`(`go.mod`은 저장소 루트, 패키지는 `go/` 아래), Go(필요 버전은 `go.mod`가 유일한 출처, [PACKAGING](PACKAGING.md#툴체인-버전-370)), **CGO_ENABLED=0**. 검토·고정한 의존성은 정확 계수용 3개와 역할 frontmatter용 YAML 1개. [계수 의존성 결정](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/policy-evidence-20260918/DEPENDENCIES.md) |
| 측정된 도구 표면과 검증한 클라이언트 | 도구 이름과 모드별 core는 내장 자료(`measured-clients.json`)로 비교한다. 세션의 [`verified` 판정](COMPATIBILITY.md#0-기준과-판정-방법--2026-09-20-실행-2026-09-19)은 도구 이름과 모드별 core에 한정한다. v0.6.14 출하는 native 2.1.330에서 표면을 재측정하고 실제 backend 표본 3종을 확인했다. v0.6.13의 신규 영향 실측은 2.1.321에서 했다. [v0.6.14 출하 검사](RELEASE-v0.6.14.md#출하-검사), [v0.6.13 출하 검사](RELEASE-v0.6.13.md#출하-검사) |
| 테스트·증거 | v0.3.3부터 공개 저장소에 두지 않고 로컬에서 관리한다. 예외는 v0.6.9부터 공개하는 합성 protocol contract test다(#271). 공개 CI는 그 테스트와 gofmt·vet·build, PowerShell 7 AST/최소 버전을 검사한다. 전체 회귀·race는 로컬 관문이다 |

### 이전 문서와 검증 기록

기능별 현행은 [COMPATIBILITY.md](COMPATIBILITY.md)가 소유한다. v0.3.2까지의 판정·격차·증거는
[DECISION.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/DECISION.md),
[PARITY.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/PARITY.md),
[VALIDATION.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/VALIDATION.md)에
고정돼 있다.

[v0.6.x 후속 품질 기록](QUALITY-v0.6.x.md)은 당시의 성능·자원·직접 인수를 담는다. 검증 예산·설치와 대표·경계
인수의 당시 범위는 [v0.4.4 기록](RELEASE-v0.4.4.md)과 [v0.6.0 기록](RELEASE-v0.6.0.md)에 있다.

<details>
<summary>v0.6.11까지의 클라이언트 재측정 이력</summary>

- 2026-11-03, native 2.1.317·Codex CLI 0.166.0. 도움말과 도구 이름·core 집합은 2.1.316과 같았고 plugin API에 desktop용 `Backdrop`·`$.ui.backdrop`가 추가됐다. fresh print/TUI × reference/clauduct/strict 표면 검사만 통과했다. 이 재측정에서 전체 fixture·전체 관문·실제 backend는 실행하지 않았다.
- 2026-10-31의 native 2.1.316 fixture·표면 PASS는 당시 입력에 한정한다. 이전 기준은 2.1.314·2.1.312·2.1.311·2.1.310·2.1.309(Codex CLI 0.164.0), 2.1.305·2.1.304·2.1.303(0.163.0), 그 전은 2.1.299·0.162.1이다. #319의 2.1.303 관문은 당시 후보의 기록이다.
- 2026-10-16, native 2.1.301의 6경로 무과금 표면 재측정이 통과했다. 도움말과 선택 UI API 선언에 차이가 있었고 측정 스냅샷은 갱신하지 않았다. 소스 `599da41b` 관문은 launcher 종료 합성 행에서 실패했다. `4813b7ae`의 콘솔 종료 경합 보정 뒤 관문 상태는 [v0.6.11 기록](RELEASE-v0.6.11.md)에 있다.
- 2026-10-14, 2.1.289 대비 `attach`·`logs`가 이름을 받았고 선택 hook 필드(workflow·resume·command·requestMeta 등)가 늘었다. fixture와 도구 표면 검사가 통과했다. 2.1.299에서 Agent View 기본 `claude` agent는 `CLAUDE_CODE_DISABLE_AGENT_VIEW`를 주었을 때만 목록에서 빠졌으며 v0.6.11은 이 변수를 넣지 않는다.
- 2026-10-04, native 2.1.289·Codex CLI 0.160.0. 도움말은 같았고 teammate hook 타입이 달라졌다. fixture 및 실제 backend 회귀 18개가 통과했고 요청 형태(비대화형 11경로, TUI 4경로)는 2.1.288과 같았다. 이전 기준은 2.1.288과 2.1.287·0.159.3이다. [v0.6.9 기록](RELEASE-v0.6.9.md), [v0.6.3 기록](RELEASE-v0.6.3.md)

v0.6.11의 출하 관문은 실행기 13단계가 종료 코드 0이었으나 closure coverage가 FAIL이었다. 실제 backend 18묶음은 별도로 실행하지 않았다. [v0.6.11 검증한 클라이언트와 출하 검사](RELEASE-v0.6.11.md#검증한-클라이언트)

</details>

## 2. 문서 위치와 책임

| 문서 | 소유하는 것 |
|---|---|
| 이 파일 | 현재 상태. 다른 문서가 이 값을 복제하지 않는다 |
| [ARCHITECTURE.md](ARCHITECTURE.md) | 책임 경계·프로토콜·생명주기의 stable contract |
| [COMPATIBILITY.md](COMPATIBILITY.md) | 기능별 지원·제약·미검증 |
| [PACKAGING.md](PACKAGING.md) | 출하물·런타임 의존·빌드·되돌리기 |
| [SETTINGS.md](SETTINGS.md) | Clauduct 전용 설정·생성 규칙·우선순위·세션 선택과 재개 지원 범위 |
| `RELEASE-v*.md` | 릴리스별 변경과 출하 검사 |
| `go/README.md` | Go 모듈의 빌드 명령과 지켜야 할 runtime 계약 |

## 3. 이 시점에 아직 사실이 아닌 것

- **forked Skill(`context: fork`)의 자식 재개는 native에 맡긴다.** 루트와 subagent 안의 fork(v0.5.0)는 받아들이고,
  `SendMessage`로 재개한 자식은 native 영수증과 대조한다. 재개 연결은 따로 검증하지 않으며, metadata에
  `stoppedByUser`가 있으면 거부한다. [fork 관측과 제한](COMPATIBILITY.md#알려진-제한-원장-v0618)
- **커스텀 역할 기본값**은 `--add-dir`·`--setting-sources`·settings의 `CLAUDE_CODE_SUBAGENT_MODEL`까지
  native와 같게 수집한다. v0.5.1에서 ZIP plugin 역할과 세션 중 `/cd`를 확인했고, v0.5.4에서는 설정 우선순위와
  managed 출처를 검증할 수 없는 경우의 거부 경계를 검사했다. 모든 managed 배포·세션 중 `/add-dir`·임의 URL
  plugin을 실측한 것은 아니다. 제품은 자식 선택을 검증하지 못하면 실행 전에 거부한다.
- **auto 권한 분류기의 모든 행동을 검증한 것은 아니다.** v0.5.6은 native 지침·두 추가 규칙을 유지한
  Terra/high로 기존 70개와 새 독립 표본을 통과했다. 기존 정상 프로세스 종료·로컬 설치 및 승인 PR/Release
  오차단을 재검사했다. 초기 실패 기록과 버전별 관측 범위는 COMPATIBILITY.md 3절에 보존한다.
  v0.6.4는 두 추가 규칙을 더하지 않는다. 분류기가 판정을 내지 못한 여섯 실패 경우 native가 호출을
  실행하지 않고 turn을 마치는 것을 합성 backend로 확인했고, 실제 backend 동작은 아직 측정하지 않았다.
- **media 용량을 모두 사전에 예측하지는 못한다.** 추가 사전 계수 없이 native 사용 범위를 유지하며,
  압축 직후에도 backend가 길이를 거부하면 자동 재시도 없이 중단한다. 진단 누락은 v0.5.6에서 고쳤다.
- **Workflow 재개는 exactly-once를 보장하지 않는다.** v0.6.0에서 검증된 원 run에 source를 함께
  전달하는 명시적 native 재개를 구현했다. 실패·중단·prompt 변경 지점 이후 효과가 반복될 수 있으며,
  결과 회수와 독립 계획의 미실행 단계 재개는 기존 의미를 유지한다(COMPATIBILITY.md 3절).
- **WebSearch**는 native ToolSearch→WebSearch 시험이 추가됐다. 실제 검색 backend와 native의 조합 전체를 모든 역할에서 실측한 것은 아니다.
- **`POST /v1/messages/count_tokens`**는 2026-09-18 결정에 따라 검증한 로컬 텍스트 계수와 구독 backend의 출력 없는 정확 계수를 지원한다. 미지원 입력은 명시적으로 거부한다.
- **비Windows 대상은 없다.** `internal/platform`의 구현은 Windows 전용이다.
