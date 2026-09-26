# Clauduct Go V2 — 현행 인덱스

V2(Go Native-Host-Preserving Bridge)의 **현재 상태를 읽는 단 하나의 자리**다.

## 1. 현재 상태 (2026-09-26, v0.4.4 출시)

| 항목 | 값 |
|---|---|
| 최신 출시 | **v0.4.4** (태그 `v0.4.4` → `90067da`). 변경과 출하 검사는 [RELEASE-v0.4.4.md](RELEASE-v0.4.4.md). 그 전은 [v0.4.3](RELEASE-v0.4.3.md) |
| 실행기 | `clauduct.exe` 하나 = Go 빌드. hook·PDF 렌더러·`--dev` 명령이 같은 파일이다(v0.4.0, #112). 이전 Node 구현은 v0.3.3에서 저장소에서 은퇴했다 — [분리 직전 커밋](https://github.com/wotjr1649/Clauduct/tree/1b1c5e19b3f33fda63254b2da7c9d0b372553481) |
| Go 모듈 | `github.com/wotjr1649/Clauduct`(`go.mod`은 저장소 루트, 패키지는 `go/` 아래), Go 1.27.1, **CGO_ENABLED=0**. 검토·고정한 의존성은 정확 계수용 3개와 역할 frontmatter용 YAML 1개. [계수 의존성 결정](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/policy-evidence-20260918/DEPENDENCIES.md) |
| 측정된 클라이언트 | Claude Code **2.1.283**(인자 표·native fixture, v0.5.0 개발 재측정), Codex CLI **0.157.0**. 아래 실제 backend 근거는 2.1.282에서의 것이다. v0.4.4 순수 출하 바이너리의 실제 WebSearch·SDK 위임·TUI 압축/취소/복구와 무료 peer SDK·background 재연결을 확인했다. v0.4.3의 실제 peer/background·idle 근거는 해당 릴리스 기록에 유지한다. 제품 요청 형식은 유지하며 Codex 자체 TUI·검색·계수의 재검증은 별도다. Windows, Go 1.27.1 |
| 테스트·증거 | v0.3.3부터 공개 저장소에 두지 않고 로컬에서 관리한다. 공개 CI는 gofmt·vet·build만 본다 |

기능별 현행은 [COMPATIBILITY.md](COMPATIBILITY.md)가 소유한다. v0.3.2까지의 판정·격차·증거는
[DECISION.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/DECISION.md),
[PARITY.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/PARITY.md),
[VALIDATION.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/VALIDATION.md)에
고정돼 있다.

v0.4.4는 #134 검증 예산을 보완해 명시적으로 허용한 검색도 같은 총상한 아래에서 검증한다.
전체 기능 감사와 수정 후 회귀, 순수 태그의 실제 backend 16회, 재현 빌드·설치·공개 업데이트 및
실제 설치본 확인을 [릴리스 노트](RELEASE-v0.4.4.md)에 기록했다. 전체 로컬 기능의 최종 인수는 아직 완료하지 않았다.

## 2. 문서 위치와 책임

| 문서 | 소유하는 것 |
|---|---|
| 이 파일 | 현재 상태. 다른 문서가 이 값을 복제하지 않는다 |
| [ARCHITECTURE.md](ARCHITECTURE.md) | 책임 경계·프로토콜·생명주기의 stable contract |
| [COMPATIBILITY.md](COMPATIBILITY.md) | 기능별 지원·제약·미검증 |
| [PACKAGING.md](PACKAGING.md) | 출하물·런타임 의존·빌드·되돌리기 |
| `RELEASE-v*.md` | 릴리스별 변경과 출하 검사 |
| `go/README.md` | Go 모듈의 빌드 명령과 지켜야 할 runtime 계약 |

## 3. 이 시점에 아직 사실이 아닌 것

- **forked Skill(`context: fork`)의 자식 재개는 native에 맡긴다.** 루트와 subagent 안의 fork(v0.5.0)는 받아들이고,
  fork 자식의 `SendMessage` 재개는 native 영수증과 대조해 실행하지만, Agent 자식과 달리 재개 연결과 사용자 중지를
  따로 검증하지 않는다([COMPATIBILITY.md](COMPATIBILITY.md) 3절).
- **커스텀 역할 기본값**은 v0.5.0에서 `--add-dir`·`--setting-sources`·settings의 `CLAUDE_CODE_SUBAGENT_MODEL`까지
  native와 같게 수집한다. managed 경로·세션 중 `/add-dir`·`/cd`·ZIP/URL plugin은 측정하지 않았다. 제품은 자식 선택을
  검증하지 못하면 실행 전에 거부한다.
- **auto 권한 모드의 분류기**는 지원하지 않는다. Claude Code 2.1.283의 기본값인 auto 모드에서 판정이 필요한 행동은
  `Classifier unavailable`로 거부된다(v0.5.0은 서버 분류기 요청 자체를 끈다, COMPATIBILITY.md 3절).
- **Workflow의 원 script 재실행 resume**은 하지 않는다. 저널 검증과 결과 회수, 독립 계획의 미실행 단계
  재개만 한다(COMPATIBILITY.md 3절).
- **WebSearch**는 native ToolSearch→WebSearch 시험이 추가됐다. 실제 검색 backend와 native의 조합 전체를 모든 역할에서 실측한 것은 아니다.
- **`POST /v1/messages/count_tokens`**는 2026-09-18 결정에 따라 검증한 로컬 텍스트 계수와 구독 backend의 출력 없는 정확 계수를 지원한다. 미지원 입력은 명시적으로 거부한다.
- **비Windows 대상은 없다.** `internal/platform`에 windows 태그 파일 하나뿐이다.
