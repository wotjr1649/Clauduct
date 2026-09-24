# Clauduct Go V2 — 현행 인덱스

V2(Go Native-Host-Preserving Bridge)의 **현재 상태를 읽는 단 하나의 자리**다.

## 1. 현재 상태 (2026-09-24, v0.4.0 출시)

| 항목 | 값 |
|---|---|
| 최신 출시 | **v0.4.0** (태그 `v0.4.0` → `8f0ffd7`). 변경과 출하 검사는 [RELEASE-v0.4.0.md](RELEASE-v0.4.0.md). 그 전은 [v0.3.5](RELEASE-v0.3.5.md) |
| 실행기 | `clauduct.exe` 하나 = Go 빌드. hook·PDF 렌더러·`--dev` 명령이 같은 파일이다(v0.4.0, #112). 이전 Node 구현은 v0.3.3에서 저장소에서 은퇴했다 — [분리 직전 커밋](https://github.com/wotjr1649/Clauduct/tree/1b1c5e19b3f33fda63254b2da7c9d0b372553481) |
| Go 모듈 | `github.com/wotjr1649/Clauduct`(`go.mod`은 저장소 루트, 패키지는 `go/` 아래), go 1.27.1, **CGO_ENABLED=0**. 정확 계수를 위해 검토·고정한 의존성 3개: [결정](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/policy-evidence-20260918/DEPENDENCIES.md) |
| 측정된 클라이언트 | claude **2.1.281**(기준 상수·인자 표·native fixture, v0.3.4 재측정. 실제 backend TUI는 v0.4.0 개발본에서도 통과). Codex CLI 기준은 0.156.1(v0.4.1 재측정: 설치본 요청과의 차이를 기록하고 이 빌드의 요청 모양은 유지, #121), 설치본은 계속 바뀐다. 이전 기준은 2.1.280. Windows, go 1.27.1 |
| 테스트·증거 | v0.3.3부터 공개 저장소에 두지 않고 로컬에서 관리한다. 공개 CI는 gofmt·vet·build만 본다 |

기능별 현행은 [COMPATIBILITY.md](COMPATIBILITY.md)가 소유한다. v0.3.2까지의 판정·격차·증거는
[DECISION.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/DECISION.md),
[PARITY.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/PARITY.md),
[VALIDATION.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/VALIDATION.md)에
고정돼 있다.

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

- **forked Skill(`context: fork`)은 루트 대화에서 부른 fork만 받아들인다.** subagent 안에서 부른 fork와
  fork 자식의 `SendMessage` 재개는 거부한다. 루트 fork는 파일 skill과 내장 `/code-review` 모두 실제 backend에서 확인했다.
  Agent 호출로 만든 자식, Workflow 자식, `SendMessage` 재개는 검증된다.
- **커스텀 역할 기본값의 완전한 수집**은 아직 없다. 제품은 자식 선택을 검증하지 못하면 실행 전에 거부한다.
- **Workflow의 원 script 재실행 resume**은 하지 않는다. 저널 검증과 결과 회수, 독립 계획의 미실행 단계
  재개만 한다(COMPATIBILITY.md 3절).
- **WebSearch**는 native ToolSearch→WebSearch 시험이 추가됐다. 실제 검색 backend와 native의 조합 전체를 모든 역할에서 실측한 것은 아니다.
- **`POST /v1/messages/count_tokens`**는 2026-09-18 결정에 따라 검증한 로컬 텍스트 계수와 구독 backend의 출력 없는 정확 계수를 지원한다. 미지원 입력은 명시적으로 거부한다.
- **비Windows 대상은 없다.** `internal/platform`에 windows 태그 파일 하나뿐이다.
