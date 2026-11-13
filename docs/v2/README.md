# Clauduct Go V2 — 현행 인덱스

V2(Go Native-Host-Preserving Bridge)의 **현재 상태를 읽는 단 하나의 자리**다.

## 1. 현재 상태 (2026-11-13, v0.6.13 출시)

| 항목 | 값 |
|---|---|
| 최신 출시 | **v0.6.13** (태그 `v0.6.13` → `725ac5c`). streaming 완료 신호를 본문 정상 종료 뒤 전달하고, resident 재생성 중 응답 보류와 `/model` pair 유지, background stop 뒤 worker 종료 확인, native fast mode 확인 끔을 더했다. 분류기 공장값은 `gpt-6.1-terra/low`이고 기존 설정 파일은 바뀌지 않는다. 전체 관문은 같은 후보의 리허설 결과로 갈음했고 실제 backend 18묶음은 실행하지 않았다(소유자 예외). [변경·출하 검사와 알려진 제한](RELEASE-v0.6.13.md), [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.13). 그 이전 **v0.6.12** (태그 `v0.6.12` → `234e13b`). `sonnet` 별칭과 `terra` 키가 `gpt-6.1-terra`를 가리키고 `gpt-5.6-terra`는 `terra56`으로 남는다. 기존 설정 파일은 바뀌지 않으며 분류기 공장값은 `gpt-5.6-terra/low`다. 전체 관문은 #371의 소유자 예외로 실행하지 않았다. [변경·출하 검사와 알려진 제한](RELEASE-v0.6.12.md), [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.12). |
| 출하 후 후속 | 알려진 제한과 미검증 항목은 [알려진 제한 원장](COMPATIBILITY.md#알려진-제한-원장-v0613)에 분류와 다시 볼 조건을 붙여 한곳에 모았다(v0.6.9, #273). 릴리스별 기록의 "알려진 제한" 절은 그 릴리스 당시의 기록이다 |
| 실행기 | `clauduct.exe` 하나 = Go 빌드. hook·PDF 렌더러·`--dev` 명령이 같은 파일이다(v0.4.0, #112). v0.6.11부터 진단 전용 `clauduct-probe.exe`가 별도 자산으로 붙는다. launch에는 필요 없고, 설치·업데이트는 둘을 한 쌍으로 다룬다([PACKAGING](PACKAGING.md) 1절·5.1절). 이전 Node 구현은 v0.3.3에서 저장소에서 은퇴했다 — [분리 직전 커밋](https://github.com/wotjr1649/Clauduct/tree/1b1c5e19b3f33fda63254b2da7c9d0b372553481) |
| Go 모듈 | `github.com/wotjr1649/Clauduct`(`go.mod`은 저장소 루트, 패키지는 `go/` 아래), Go(필요 버전은 `go.mod`가 유일한 출처, [PACKAGING](PACKAGING.md#툴체인-버전-370)), **CGO_ENABLED=0**. 검토·고정한 의존성은 정확 계수용 3개와 역할 frontmatter용 YAML 1개. [계수 의존성 결정](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/policy-evidence-20260918/DEPENDENCIES.md) |
| 측정된 도구 표면과 검증한 클라이언트 | 측정된 도구 표면(이름과 모드별 core)은 내장 자료(`measured-clients.json`)이다. 세션의 `verified`는 부분 검사다: 비교 대상이며 비어 있지 않은 모드 core 기대 목록이 있는 세션에서, 세션 중 한 요청의 실제 도구 정의(tools 배열)에 모드의 core 도구가 모두 있고, 세션의 어느 요청에도 측정 목록 밖 비-MCP 도구 정의가 없으며, 추가 도구 이름 진단의 16개 보관 한도 초과가 한 번도 없을 때 true다. 비교하지 않는 세션(기대 core 목록이 비어 있는 경우: 예를 들어 도구를 좁힌 `--tools`, background)은 도구 정의가 있어도 항상 false다. 서로 다른 요청의 부분 core는 합치지 않고, 프롬프트의 deferred 도구 안내 텍스트는 true를 입증하는 근거로 쓰지 않는다. core가 deferred로만 전달되면 false이며, deferred 진단에서만 보관 한도를 초과해도 세션은 false를 유지한다. 측정된 비핵심 도구의 부재, schema, MCP는 검증하지 않는다. false는 차이 또는 미비교일 뿐 거부가 아니며, 버전은 기록만 한다. Claude Code·Codex CLI 버전은 고정·내장하지 않고 기록만 한다(v0.6.11, #333). v0.6.11 출하 검증에 쓴 native·Codex CLI 버전과 근거는 [검증한 클라이언트 표](RELEASE-v0.6.11.md#검증한-클라이언트)에 있다. 아래는 버전별 재측정 이력이다. 2026-11-03 무과금 재측정(native 2.1.317, Codex CLI 설치본 0.166.0): 도움말과 도구 이름·core 집합은 2.1.316과 같고, plugin API에 desktop용 `Backdrop`·`$.ui.backdrop`가 추가됐다. fresh print/TUI × reference/clauduct/strict 표면 관문은 통과했다. 이번 평가에서 전체 fixture·전체 관문·실제 backend는 실행하지 않았다. 당시 Codex CLI 설치본은 0.166.0이었고 도움말이 같았다. 전체 관문 실행기는 13단계가 종료 코드 0(`ALLDONE failed=0`)이었으나 검증 범위 판정기는 FAIL이라 전체 관문 PASS가 아니며(알려진 제한), 별도 실제 backend 18묶음은 실행하지 않았다(NOT_RUN, 소유자 결정 2026-11-10)이다([v0.6.11](RELEASE-v0.6.11.md)). #319 마감 관문(2.1.303)의 결과는 그 과거 후보에만 해당한다. 이전 기준 2.1.316의 2026-10-31 fixture·표면 PASS는 그 당시 입력에만 유효하다. 이전 기준은 2.1.314·2.1.312·2.1.311·2.1.310·2.1.309(Codex CLI 0.164.0), 2.1.305·2.1.304·2.1.303(0.163.0), 그 전은 2.1.299·0.162.1이다. 2026-10-14 무과금 재측정: 2.1.289 기준 대비 `attach`·`logs`가 이름도 받고, hook 타입에 선택 필드(workflow·resume·command·requestMeta 등)가 늘었으며, fixture 시험과 도구 표면 gate가 통과했다. print 목록의 native 전용 `claude` agent는 Agent View 기본 agent로, 2.1.299 측정에서 `CLAUDE_CODE_DISABLE_AGENT_VIEW`가 있을 때만 빠졌다. v0.6.11은 이 변수를 넣지 않는다(공개 빌드 Agent View). 2026-10-16에는 자동 갱신된 2.1.301에서 6경로 무과금 도구 표면 재측정이 통과했다. 도움말과 선택 UI API 선언의 차이를 확인했으며, 제품의 측정 기준과 기준 스냅샷은 갱신하지 않았다. 같은 native를 고정한 제품 소스 599da41b의 관문은 launcher 종료 합성 행에서 실패했고, 검증 driver의 콘솔 종료 경합을 보정한 4813b7ae의 후속 관문 상태를 기록한다([v0.6.11](RELEASE-v0.6.11.md)). Go 1.27.2. 이전 기준은 2.1.289·0.160.0. 2026-10-04 무과금 재측정(#267): 2.1.289 바이너리를 지정해 도움말은 같고, 세션 plugin이 쓰는 hook 타입에서 teammate 관련 차이(`isTeammate`, `teammateId`, `AgentTeammateRecord`)를 보고했으며, 설치본 native로 도는 fixture 시험이 통과했다. 같은 날 요청 형태 전수 수집(비대화형 11경로, TUI 4경로)이 2.1.288과 같았고, 실제 backend 회귀 18개가 통과했다. 이전 기준은 2.1.288(v0.6.8), 2.1.287·0.159.3(v0.6.3). Windows, Go 1.27.1, PowerShell 7.6.6. [v0.6.3 실행 결과](RELEASE-v0.6.3.md), Worktree·media·phase·Bundle C와 식별자별 NOT_RUN은 [v0.6.0 기록](RELEASE-v0.6.0.md)에서 구분한다 |
| 테스트·증거 | v0.3.3부터 공개 저장소에 두지 않고 로컬에서 관리한다. 예외는 v0.6.9부터 공개하는 합성 protocol contract test다(#271). 공개 CI는 그 테스트와 gofmt·vet·build, PowerShell 7 AST/최소 버전을 검사한다. 전체 회귀·race는 로컬 관문이다 |

기능별 현행은 [COMPATIBILITY.md](COMPATIBILITY.md)가 소유한다. v0.3.2까지의 판정·격차·증거는
[DECISION.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/DECISION.md),
[PARITY.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/PARITY.md),
[VALIDATION.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/VALIDATION.md)에
고정돼 있다.

v0.6.x 후속 작업은 현행 Windows 지원 범위를 검증한다. [우선순위·성능·자원·직접 인수 범위](QUALITY-v0.6.x.md)에
출하 검사와 개발 검증을 구분한다. v0.6.11은 2026-11-10에 발행했지만 전체 관문의 closure coverage와 실제 backend 18묶음은 통과하지 않았다. 알려진 제한과 발행 전 발견 사항은 [RELEASE-v0.6.11.md](RELEASE-v0.6.11.md)와 알려진 제한 원장이 소유하며, 발행 사실을 그 항목들의 해소로 읽지 않는다.

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
