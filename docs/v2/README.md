# Clauduct Go V2 — 현행 인덱스

V2(Go Native-Host-Preserving Bridge)의 **현재 상태를 읽는 단 하나의 자리**다.

## 1. 현재 상태 (2026-10-04, v0.6.10 출시)

| 항목 | 값 |
|---|---|
| 최신 출시 | **v0.6.10** (태그 `v0.6.10` → `b47d7f8`). `showThinkingSummaries`를 켠 TUI 세션의 main turn에서 backend 추론 요약을 화면에만 보여 주고(모델 요청·대화 메시지에는 싣지 않음), auto 모드에서 형제 agent의 같은 동작에 대한 분류 질문이 replay로 오인 차단되던 결함(v0.6.5부터)을 고쳤다. replay 차단 진단 기록도 더했다. [변경·출하 검사와 알려진 제한](RELEASE-v0.6.10.md), [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.10). 직전 출시 **v0.6.9** (태그 `v0.6.9` → `b780a15`). native Claude Code 2.1.289를 측정 기준으로 올리고, 2.1.289에서 모두 거부되던 Agent teams의 teammate를 지원하며, TUI subagent가 응답 도중 실패할 때 부분 보고가 넘어가던 것을 명시적 오류로 바꿨다. `max_uses` 값 검증, 공개 contract test, 알려진 제한 원장도 더했다. [변경·출하 검사와 알려진 제한](RELEASE-v0.6.9.md), [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.9). 그 이전 **v0.6.8** (태그 `v0.6.8` → `00f2020`). 빈 tool 결과가 요청 전체를 실패시키던 결함을 고치고, auto 모드 분류기 transcript가 너무 길 때 native가 압축하게 하며, 요청 값(thinking·metadata·cache scope)을 native 실측 기준으로 검증한다. native 2.1.288의 이어 쓰기·빈 응답 재시도·첫 요청 대기를 측정해 판정했다. [변경·출하 검사와 알려진 제한](RELEASE-v0.6.8.md), [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.8). 그 이전 **v0.6.7** (태그 `v0.6.7` → `62b84f2`): 측정에서 통과하지 못한 분류기 pair를 시작할 때 알리고(Luna/low 재측정 불합격), `/context` 직후 종료로 버려진 계수를 거부와 분리해 보여 준다. [변경·출하 검사와 알려진 제한](RELEASE-v0.6.7.md), [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.7). 그 이전 **v0.6.6**(태그 `v0.6.6` → `82fcc76`): 종료 상태 파일 저장을 재시도하고, 2026-10-14에 은퇴하는 `gpt-5.5`를 고르면 목록에서 빠진 뒤 `gpt-6.1-sol`을 안내한다. [변경·출하 검사와 알려진 제한](RELEASE-v0.6.6.md), [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.6). 그 이전 **v0.6.5**(태그 `v0.6.5` → `1786d30`): 계정이 숨긴 모델을 위임 메뉴와 Agent 선택지에서 빼고(전체 ID 지정은 유지), native 명령행이 Windows 한도를 넘으면 자르지 않고 시작 전에 거부한다. [변경·출하 검사와 알려진 제한](RELEASE-v0.6.5.md), [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.5). 그 이전 **v0.6.4**(태그 `v0.6.4` → `3273786`): 모델 목록을 세션 시작마다 Codex 계정에서 받아 새 모델을 전체 ID로 고르고, Clauduct가 더하던 native 권한 규칙을 없애 권한 판단을 native에 맡겼다. `classifier_model`은 `{"model","effort"}` 객체, 두 effort 상한은 은퇴, 설정 파일은 빠진 최상위 키만 동기화한다. [변경·출하 검사와 알려진 제한](RELEASE-v0.6.4.md), [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.4). 그 이전 [v0.6.3](RELEASE-v0.6.3.md), [v0.6.2](RELEASE-v0.6.2.md)와 [v0.6.1](RELEASE-v0.6.1.md), Bundle C와 전체 목록 [v0.6.0](RELEASE-v0.6.0.md), 설정 계약 [v0.5.4](RELEASE-v0.5.4.md), phase 호환 범위 [v0.5.5](RELEASE-v0.5.5.md), 권한 분류·media 보완 [v0.5.6](RELEASE-v0.5.6.md)의 기록은 보존한다 |
| 출하 후 후속 | 알려진 제한과 미검증 항목은 [알려진 제한 원장](COMPATIBILITY.md#알려진-제한-원장-v0610)에 분류와 다시 볼 조건을 붙여 한곳에 모았다(v0.6.9, #273). 릴리스별 기록의 "알려진 제한" 절은 그 릴리스 당시의 기록이다 |
| 실행기 | `clauduct.exe` 하나 = Go 빌드. hook·PDF 렌더러·`--dev` 명령이 같은 파일이다(v0.4.0, #112). 이전 Node 구현은 v0.3.3에서 저장소에서 은퇴했다 — [분리 직전 커밋](https://github.com/wotjr1649/Clauduct/tree/1b1c5e19b3f33fda63254b2da7c9d0b372553481) |
| Go 모듈 | `github.com/wotjr1649/Clauduct`(`go.mod`은 저장소 루트, 패키지는 `go/` 아래), Go 1.27.1, **CGO_ENABLED=0**. 검토·고정한 의존성은 정확 계수용 3개와 역할 frontmatter용 YAML 1개. [계수 의존성 결정](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/policy-evidence-20260918/DEPENDENCIES.md) |
| 측정된 클라이언트 | Claude Code **2.1.289**, Codex CLI **0.160.0**(`measured-clients.json`의 측정 기준). 2026-10-04 무과금 재측정(#267): 2.1.289 바이너리를 지정해 도움말은 같고, 세션 plugin이 쓰는 hook 타입에서 teammate 관련 차이(`isTeammate`, `teammateId`, `AgentTeammateRecord`)를 보고했으며, 설치본 native로 도는 fixture 시험이 통과했다. 같은 날 요청 형태 전수 수집(비대화형 11경로, TUI 4경로)이 2.1.288과 같았고, 실제 backend 회귀 18개가 통과했다. 이전 기준은 2.1.288(v0.6.8), 2.1.287·0.159.3(v0.6.3). Windows, Go 1.27.1, PowerShell 7.6.6. [v0.6.3 실행 결과](RELEASE-v0.6.3.md), Worktree·media·phase·Bundle C와 식별자별 NOT_RUN은 [v0.6.0 기록](RELEASE-v0.6.0.md)에서 구분한다 |
| 테스트·증거 | v0.3.3부터 공개 저장소에 두지 않고 로컬에서 관리한다. 예외는 v0.6.9부터 공개하는 합성 protocol contract test다(#271). 공개 CI는 그 테스트와 gofmt·vet·build, PowerShell 7 AST/최소 버전을 검사한다. 전체 회귀·race는 로컬 관문이다 |

기능별 현행은 [COMPATIBILITY.md](COMPATIBILITY.md)가 소유한다. v0.3.2까지의 판정·격차·증거는
[DECISION.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/DECISION.md),
[PARITY.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/PARITY.md),
[VALIDATION.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/VALIDATION.md)에
고정돼 있다.

v0.6.x 후속 작업은 현행 Windows 지원 범위를 검증한다. [우선순위·성능·자원·직접 인수 범위](QUALITY-v0.6.x.md)에
출하 대기와 개발 검증을 구분했다. 다음 태그·Release·설치는 별도 출하 작업이며 현재 설치본은 위 출시 버전이다.

v0.4.4는 #134 검증 예산을 보완해 명시적으로 허용한 검색도 같은 총상한 아래에서 검증한다.
전체 기능 감사와 수정 후 회귀, 순수 태그의 실제 backend 16회, 재현 빌드·설치·공개 업데이트 및
실제 설치본 확인을 [릴리스 노트](RELEASE-v0.4.4.md)에 기록했다. v0.6.0에서는 최종 바이너리의 필수
대표·경계 인수를 완료했다. 열거한 모든 native UI·옵션의 개별 실행을 완료했다는 뜻은 아니다.

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
  fork 자식의 `SendMessage` 재개는 native 영수증과 대조해 실행하지만, Agent 자식과 달리 재개 연결과 사용자 중지를
  따로 검증하지 않는다([COMPATIBILITY.md](COMPATIBILITY.md) 3절).
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
