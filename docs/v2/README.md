# Clauduct Go V2 — 현행 인덱스

V2(Go Native-Host-Preserving Bridge) 작업의 **현재 상태를 읽는 단 하나의 자리**다. 기존 Node 제품의 현행 상태는 여기에 복제하지 않는다 — 그것은 [HANDOFF.md](../../HANDOFF.md)와 [현행 검증표](../remaining-verification.md)가 소유한다. 두 축은 서로 다르며 한쪽의 판정이 다른 쪽을 바꾸지 않는다.

## 1. 현재 상태

| 항목 | 값 |
|---|---|
| 세대 | V2 — Go Native-Host-Preserving Bridge |
| 완료한 게이트 | **G0(현황) · G1(설계) · G2(격리)** |
| 완료한 작업 패키지 | **WP01** — Go workspace와 독립 launcher skeleton |
| 다음 게이트 | G3 최소 실행 → G4 기본 wire. 다음 패키지는 WP02 |
| 기준선 | `node-bfbdf23-g0` (commit `bfbdf2385175…`, tree `9cd96d660959…`) |
| 개발 branch | `redesign/go-v2-native-host`, worktree `D:/AIDEV/Clauduct-go-v2` |
| Go 모듈 | `github.com/wotjr1649/Clauduct/go`, go 1.27.0, 제3자 의존성 0 |
| 기존 Node 변경 | **0건.** baseline tracked 파일 생성·수정·이동·삭제 없음 |
| 실모델 호출 | **0회.** 잔여 승인 예산 0 |
| 실행 언어 | Go 확정 (D01). 재투표 대상 아님 |

판정 전문은 [DECISION.md](DECISION.md)에 있다. 요약:

```text
DESIGN:             GO (수정 9건 반영)
OFFLINE_BUILD:      GO  — G2 승인됨, WP01 구현·검증 완료
LIVE_VALIDATION:    NOT_AUTHORIZED  (예산 0 + 전송 계약 BLOCKED)
DEFAULT_SWITCH:     NOT_REQUESTED
RELEASE:            USER_DECISION_REQUIRED
ARCHIVAL_MOVE:      DEFERRED
```

## 2. 문서 위치와 책임

| 문서 | 소유하는 것 |
|---|---|
| 이 파일 | 현재 상태·다음 게이트·예산. 다른 문서가 이 값을 복제하지 않는다 |
| [DECISION.md](DECISION.md) | 착수 판정, 판정 근거, 확정·보류·블로커 |
| [ARCHITECTURE.md](ARCHITECTURE.md) | 책임 경계·프로토콜·생명주기의 stable contract |
| [MIGRATION.md](MIGRATION.md) | 파일 처분의 요약과 이동 단계. 기계 판본이 source of truth |
| [COMPATIBILITY.md](COMPATIBILITY.md) | 기능별 지원·제약·미검증 |
| [VALIDATION.md](VALIDATION.md) | 요구 ID·test ID·게이트·예산·비교 실행 계약·실행 증거 |
| `go/README.md` | Go 모듈의 빌드 명령과 지켜야 할 runtime 계약 |
| `comparison/baselines/node-bfbdf23-g0.json` | 기준선 SHA·환경·검증 상태 |
| `comparison/baselines/node-bfbdf23-g0-migration.json` | **전수 파일 처분의 source of truth** (738 entries) |

원본 설계 요구는 `docs/prompts/2026-09-16-session-34-go-v2-native-host-redesign-handoff.md`다. 그 문서는 세션 프롬프트이며 현재 상태 문서가 아니다 — 갱신하지 않는다.

## 3. 다음 세션이 읽을 것

전체 핸드오프를 매번 다시 읽지 않는다. 이 파일 → [DECISION.md](DECISION.md) → 착수할 작업 패키지의 해당 절만 읽는다.

다음 하나의 bounded work package는 **WP02 — ephemeral HTTP·생명주기**다. WP01이 bind·token·readiness·정리까지 끝냈으므로 WP02는 token 검증, 동시 세션 격리, 요청 단위 취소, 등록 교체 race를 맡는다. 우선 테스트는 HTTP02–HTTP07, HTTP09, HTTP11, LIFE06–LIFE07.

## 4. 이 시점에 아직 사실이 아닌 것

- **대화형 세션이 성립하지 않는다.** `POST /v1/messages`가 없어 첫 모델 요청은 404 `UNSUPPORTED_ROUTE`를 받는다. 모델을 호출하지 않는 native 명령(`--version`·`--help`)만 통과한다.
- upstream·인증·프로토콜 변환·SSE·도구 왕복·모델 라우팅은 코드가 없다.
- 성능·지연·메모리 비교 수치는 측정한 적이 없다.
- host parity는 미검증이다. 통제된 synthetic profile 실행(NATIVE_SYNTH)은 아직 하지 않았다.
- `-race`는 이 머신에서 **NOT_RUN**이다 — cgo와 C 툴체인이 없다. CI가 담당하며 미실행은 통과가 아니다.
- CI(`.github/workflows/go.yml`)는 작성만 했고 **실행된 적이 없다.** push 권한은 별도 승인 사항이다.
