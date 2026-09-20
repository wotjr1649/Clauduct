# Clauduct Go V2 — 현행 인덱스

V2(Go Native-Host-Preserving Bridge) 작업의 **현재 상태를 읽는 단 하나의 자리**다. 기존 Node
제품의 현행 상태는 여기에 복제하지 않는다 — 그것은 [HANDOFF.md](../../HANDOFF.md)와
[현행 검증표](../v1/remaining-verification.md)가 소유한다.

## 1. 현재 상태 (2026-09-18)

| 항목 | 값 |
|---|---|
| 세대 | V2 — Go Native-Host-Preserving Bridge |
| 게이트 | **G0–G9 완료.** G10(archive)만 DEFERRED |
| 작업 패키지 | **WP01–WP07 완료.** C4·F4는 "조사 후 안 함"으로 닫힘 |
| 기본 실행기 | **Go.** `clauduct` = Go 빌드, `clauduct-node` = Node 구현 (G9, 2026-09-17) |
| 기준선 | `node-bfbdf23-g0` (commit `bfbdf2385175…`, tree `9cd96d660959…`) |
| 개발 branch | `fix/verified-agent-policies`, `D:/AIDEV/Clauduct`. 검증 바이너리: `D:/AIDEV/clauduct-s36-build/clauduct.exe` |
| Go 모듈 | `github.com/wotjr1649/Clauduct/go`, go 1.27.1, **CGO_ENABLED=0**. 정확 계수를 위해 검토·고정한 의존성 3개: [결정](../../verification/policy-evidence-20260918/DEPENDENCIES.md) |
| 기존 Node 변경 | **0건.** baseline tracked 파일 생성·수정·이동·삭제 없음 |
| 테스트 | 2026-09-17 기준선은 1124 pass / 0 fail / 3 skip. 2026-09-18 변경의 별도 증거: [계수](../../verification/policy-evidence-20260918/COUNT-TOKENS.md), [압축 제어](../../verification/policy-evidence-20260918/CONTEXT-ENFORCEMENT.md) |
| 실모델 호출 | 2026-09-17 누계는 추론 55/100. 이후 검증은 각 evidence 테스트의 유한 ledger와 실행 기록으로 별도 집계 |
| 측정된 클라이언트 | claude **2.1.275**, Windows, go 1.27.1 |

판정 전문은 [DECISION.md](DECISION.md), 기능별 현행은 [COMPATIBILITY.md](COMPATIBILITY.md),
격차 원장은 [PARITY.md](PARITY.md), 증거는 [VALIDATION.md](VALIDATION.md)가 소유한다.

## 2. 문서 위치와 책임

| 문서 | 소유하는 것 |
|---|---|
| 이 파일 | 현재 상태·다음 작업·예산. 다른 문서가 이 값을 복제하지 않는다 |
| [DECISION.md](DECISION.md) | 착수 판정과 그 근거. **G1 시점 문서이며 이후 상태를 말하지 않는다** |
| [ARCHITECTURE.md](ARCHITECTURE.md) | 책임 경계·프로토콜·생명주기의 stable contract |
| [MIGRATION.md](MIGRATION.md) | 파일 처분의 요약과 이동 단계. 기계 판본이 source of truth |
| [COMPATIBILITY.md](COMPATIBILITY.md) | 기능별 지원·제약·미검증 |
| [PARITY.md](PARITY.md) | 기준선 대비 실제 격차와 작업 목록 |
| [VALIDATION.md](VALIDATION.md) | 요구 ID·test ID·게이트·예산·실행 증거 |
| [PACKAGING.md](PACKAGING.md) | 출하물·런타임 의존·빌드·되돌리기 |
| `go/README.md` | Go 모듈의 빌드 명령과 지켜야 할 runtime 계약 |

원본 설계 요구는 세션 프롬프트였고 저장소에 없다 — 프롬프트와 인계 문서는 `docs/journal/`에
남으며 올라가지 않는다(2026-09-17 결정: 증거는 공개, 지시는 비공개). 그 프롬프트가 요구한 것
가운데 지금도 사실인 것은 전부 위 표의 문서들이 소유한다. **여기에 파일 이름을 적어두는 것은
클론한 사람이 열 수 없는 파일을 가리키는 것이었다** — 그 이름은 한 번도 커밋된 적이 없다.

## 3. 이 시점에 아직 사실이 아닌 것

- **완료 연결 바인딩 4종**(`linkTaskResult`·`linkWorkflow`·`linkResume`·`linkSkill`)은 없다.
  등록/해제(`POST /clauduct/agents`)만 있다.
- **커스텀 역할 기본값의 완전한 수집**은 아직 없다. 제품은 자식 선택을 검증하지 못하면 실행 전에 거부한다.
- **일반 workflow 저널(C4)**은 없다. 모델 선택 전용 저널과 native 메타데이터 대조는 구현됐으며 디스크를 읽는다. 실제 native 자식 resume은 별도 미검증이다.
- **WebSearch**는 native ToolSearch→WebSearch 시험이 추가됐다. 실제 검색 backend와 native의 조합 전체를 모든 역할에서 실측한 것은 아니다.
- **성능 비교는 시작 비용과 메모리만 측정했다.** 요청당 지연·처리량의 v1/v2 비교는 공용 fixture
  harness가 없어 미측정이다.
- **`POST /v1/messages/count_tokens`**는 2026-09-18 결정에 따라 검증한 로컬 텍스트 계수와 구독 backend의 출력 없는 정확 계수를 지원한다. 미지원 입력은 명시적으로 거부한다.
- **비Windows 대상은 없다.** `internal/platform`에 windows 태그 파일 하나뿐이다.
