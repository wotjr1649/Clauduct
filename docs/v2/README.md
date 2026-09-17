# Clauduct Go V2 — 현행 인덱스

V2(Go Native-Host-Preserving Bridge) 작업의 **현재 상태를 읽는 단 하나의 자리**다. 기존 Node
제품의 현행 상태는 여기에 복제하지 않는다 — 그것은 [HANDOFF.md](../../HANDOFF.md)와
[현행 검증표](../remaining-verification.md)가 소유한다.

## 1. 현재 상태 (2026-09-17)

| 항목 | 값 |
|---|---|
| 세대 | V2 — Go Native-Host-Preserving Bridge |
| 게이트 | **G0–G9 완료.** G10(archive)만 DEFERRED |
| 작업 패키지 | **WP01–WP07 완료.** C4·F4는 "조사 후 안 함"으로 닫힘 |
| 기본 실행기 | **Go.** `clauduct` = Go 빌드, `clauduct-node` = Node 구현 (G9, 2026-09-17) |
| 기준선 | `node-bfbdf23-g0` (commit `bfbdf2385175…`, tree `9cd96d660959…`) |
| 개발 branch | `redesign/go-v2-native-host` (main에 머지됨). 작업은 `D:/AIDEV/Clauduct` main에서 한다 — worktree `Clauduct-go-v2`는 2026-09-17에 제거했다 |
| Go 모듈 | `github.com/wotjr1649/Clauduct/go`, go 1.27.1, 제3자 의존성 0, **CGO_ENABLED=0** |
| 기존 Node 변경 | **0건.** baseline tracked 파일 생성·수정·이동·삭제 없음 |
| 테스트 | **1111 pass / 0 fail / 3 skip** (subtest 포함, 15 package). CI green |
| 실모델 호출 | **추론 53/100** (probe 35 + 2026-09-17 제품 실세션 18). 검색 probe는 ledger를 쓰지 않는다(별도 기록) |
| 측정된 클라이언트 | claude **2.1.274** (2026-09-17 재확인), codex-cli 0.154.0 |

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

원본 설계 요구는 `docs/prompts/2026-09-16-session-34-go-v2-native-host-redesign-handoff.md`다.
그 문서는 세션 프롬프트이며 현재 상태 문서가 아니다 — 갱신하지 않는다.

## 3. 이 시점에 아직 사실이 아닌 것

- **완료 연결 바인딩 4종**(`linkTaskResult`·`linkWorkflow`·`linkResume`·`linkSkill`)은 없다.
  등록/해제(`POST /clauduct/agents`)만 있다.
- **agent 메타데이터 identity 검증**(symlink 경계·실패 코드 12종)은 사용자 결정으로 보류다.
  이 빌드에서 선택 실패는 턴을 죽이지 않고 클라이언트가 고른 모델로 진행한다.
- **workflow 저널 검증(C4)**은 조사 후 구현하지 않기로 했다 — 이 빌드는 라우팅에 디스크를 읽지 않는다.
- **WebSearch는 bridge 단위와 실백엔드 probe로만 검증됐다.** 실제 클라이언트 세션이 WebSearch를
  일으키는 NATIVE_SYNTH 테스트는 없다.
- **성능 비교는 시작 비용과 메모리만 측정했다.** 요청당 지연·처리량의 v1/v2 비교는 공용 fixture
  harness가 없어 미측정이다.
- **`POST /v1/messages/count_tokens`는 구현하지 않기로 했다** (2026-09-17). PDF는 구현했다.
- **비Windows 대상은 없다.** `internal/platform`에 windows 태그 파일 하나뿐이다.
