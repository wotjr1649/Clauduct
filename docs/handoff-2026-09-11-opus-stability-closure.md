# Clauduct 안정성 종결 인계 — Claude Opus 개발 세션

## 1. 사용자와 grilling으로 확정한 결정

- 다음 개발자는 일반 Claude Code의 Opus이며 작업 루트는 D:/AIDEV/Clauduct다. Clauduct를 거쳐 Opus를 실행하는 것이 아니다. Clauduct의 Claude 모델 지원이나 전체 모델 목록 확장은 제외한다.
- 감독하 실제 저장소 개발과 무인 연속 개발이라는 두 목표를 유지하되 별도 판정한다.
- 과거 UNSUPPORTED_EVENT가 재현되지 않아 원인을 끝내 특정하지 못하면, 데이터 보존·도구 중복 실행 방지·안전한 중단을 입증한 경우에만 감독하 사용을 조건부 허용한다. 무인 판정은 보류한다. 단순 미재발을 원인 해결로 처리하지 않는다.
- 무인 수용 기준은 하나의 실행에서 새 기능 3개의 계획→구현→독립 리뷰→테스트→로컬 커밋을 연속 완료하는 것이다. 중간 사용자 개입 또는 예기치 않은 오류가 발생한 시도는 무인 PASS가 아니다. TDD에서 계획된 RED assertion 실패는 예상된 개발 증거이며 API/환경 오류와 분리해 기록한다.
- 관련 개선을 묶어 구현·통합 회귀·커밋한다. 한 줄 수정마다 사용자에게 실사용 시험을 반복시키지 않는다. 이미 통과한 영역은 변경 영향이나 새 증거가 있을 때만 재검증한다.
- 비용 예산·실제 인증 실행 권한은 이번 수용 기준 결정으로 확대되지 않는다. 무인 시험의 실행 시간/비용/동시성/중단 조건과 실행 주체는 실제 시험 전에 별도로 확인한다.

## 2. 현재 상태와 보존할 것

작성 시 제품 branch는 fix/native-completion-resume, HEAD는 339fc11이다. tracked 변경은 없다. 다수의 기존 untracked 프롬프트, %SystemDrive% 디렉터리, clauduct-check.txt, clauduct-agent-validation-7b4a674.txt, src/agent-selection.review-fixture.mjs, verification/dev-sandbox/는 사용자 상태다. 일괄 stage/삭제/정리하지 않는다. 시작 시 실제 status와 guidance를 다시 읽고 이후 변경은 보존한다.

독립 sandbox D:/AIDEV/Clauduct/verification/dev-sandbox/run-02는 HEAD 38e9c285508f78a162a68c7b06eeb4fe67f4c2a0, clean이다. untag 구현·테스트·SDD_VERIFICATION.md를 포함한 여섯 파일이 커밋됐다. 제품 repo와 별개의 Git 저장소이며 제품 커밋에 흡수하지 않는다. session-18~23 프롬프트는 역사적 절차다. 특히 session-22/23의 미커밋 전제를 현재 상태에 적용하거나 그대로 재실행하지 않는다.

이번 인계 작성에서는 제품 수정이나 테스트 재실행을 하지 않았다. 아래 통과 결과의 시점과 출처를 유지한다. 인계문서와 session-24 프롬프트는 새 uncommitted 문서다.

### 제품 계약

- 구조: Claude Code → loopback Clauduct gateway → 직접 HTTPS 추론 backend. Codex app-server 경로가 아니다. 아키텍처 전환은 이 인계로 승인되지 않는다.
- 메인 기본값은 gpt-6-astra/low, context 400000, 자동 압축 목표 320000이다. 현재 계산은 output reserve 20000과 compactPercent 84.21052631578947을 사용한다. 설정 전달은 실제 backend 용량/압축 발동 증명이 아니다.
- 명시 모델 기본 effort는 astra medium / sol xhigh / terra high / luna max다. 메인 무옵션 기본 low와 혼동하지 않는다.
- clauduct-inherit는 생성 시점 직접 부모의 실제 모델과 effort를 상속한다. 과거 astra/max나 메인 값을 모든 자식에 하드코딩하지 않는다. 기존 고정 정의와 역할 기본값을 유지한다.
- 계정 ID를 영구 고정하지 않지만 현재 인증 저장 위치는 C:/Users/JS/.codex/auth.json이며 실행 중 최초 계정과 일관성을 검사한다. 인증 파일을 인계 확인 목적으로 읽지 않는다. 계정 변경 원인으로 API 오류를 추정하지 않는다.

## 3. 핵심 증거와 판정

| 근거 | 관측된 사실 | 확대 해석 금지 |
|---|---|---|
| 제품 59687cd | destroyed 상태 대신 실제 socket close 대기; cleanup boolean 진단 | 과거 모든 CLEANUP_FAILED 원인의 소급 확정 아님 |
| 제품 5ae8fbc | REQUEST_SHAPE를 stream false/누락/비정상, messages 형식/빈 배열로 분리 | 비스트리밍 지원 구현 아님 |
| 제품 76d1cd0 | 미지원 이벤트의 고정 이름/namespace/typeFormat 진단 | 임의 원문 이름을 복원하지 않음 |
| 제품 339fc11 | child-only nonstreaming fallback 차단, 실패 최초8/최근8, omitted, requestOutcome | 최초 UNSUPPORTED_EVENT 해결 아님 |
| c4c222f8 세션 원문 | 독립 spec completed/PASS 다음 quality completed/PASS, 테스트 41개 통과, 기록 작성 후 커밋 전 오류 | 한 세션 무중단 SDD 성공 아님 |
| c4c222f8 종료 JSON | 요청 49/성공44/실패5; 후반57 UNSUPPORTED_EVENT other/identifier→58 REQUEST_STREAM_FALSE | 누락된 앞선 실패3건의 상세는 복원 불가 |
| 5f27e1c8 세션 원문·Git | 테스트 1회 41/41/0, 기록 보완, sandbox 38e9c28 커밋, clean, 새 자식 없음 | 이전 리뷰를 이번 세션에서 실행한 것으로 기록 금지 |
| 5f27e1c8 사용자 제공 종료 JSON | 요청37 전부 성공, requestOutcome all-succeeded, failureHistory 빈 배열/omitted0, cleanup9개 true, child fallback 설정 true; 최근16개 sol/low | 실제 오류가 없었으므로 fallback 차단 분기와 실패 이력 실발생은 미검증 |

5f27e1c8의 마지막 종료 JSON은 대화로 제공된 증거다. 이 인계에서 원문 JSON을 저장하거나 누락된 이전 요청별 model/effort를 지어내지 않는다. 제품의 기존 감사 문서에는 이 최신 37건 PASS가 아직 반영되지 않은 곳이 있다. 첫 문서 정리 묶음에서 출처를 명시해 반영한다.

## 4. 잔여 위험·검증·개선 행렬

아래는 수행 대상 후보와 증거 공백이다. 모두 현재 결함이라는 뜻이 아니다. 해당 소스·테스트·호출자를 확인해 완료/조건부/미검증/차단/범위밖으로 분류한다.

| 우선 | 항목 | 현재 공백 및 필요한 증거 | 관련 위치 |
|---|---|---|---|
| P0 | 최초 미지원 이벤트 | other/identifier의 실제 종류·발생 조건 미확정. 미지원 성공 처리 금지. 공개 공식 프로토콜/설치 코드의 근거, 비밀 없는 재현 fixture로 원인 좁히기. 임의 이름·본문·reasoning·토큰 로깅/해시 수집 금지 | native-protocol.mjs, native-transport.mjs, test-unsupported-event-diagnostics.mjs |
| P0 | native fallback 경계 | 공식 flag와 설치 코드 경로 확인, 설정 전달 실증 있음. 오류 주입 시 원래 실패 유지/비스트리밍 재전송 없음/중복 tool 실행 없음의 실제 native 검증은 없음. 승인 범위 내 synthetic 경로 우선; native 자동 실행 제한 우회 금지 | clauduct.mjs, test-launcher-native.mjs, audit-2026-09-11-native-fallback-failure-history.md |
| P0 | 실패·완료·취소 순서 | 로컬 cancel-first/mismatch-first, stop/재등록, 형제 격리 검사는 있음. UI 취소·native 알림 순서·resume 중복 side effect는 별도. remote 계산 취소나 전송 데이터 회수를 보장하지 않음 | native-gateway.mjs, native-delivery.mjs, agent-selection.mjs, test-cancel-snapshot.mjs, test-completion-selection.mjs |
| P0 | TDD/SDD 실제 종결 | 보존된 결과 통합은 완료. fresh 기능 전체 cycle 및 3-cycle 무개입 증거 없음. 기존 untag 재개발 금지. 독립 리뷰의 실제 completed 결과와 각 cycle 커밋/테스트 필요 | run-02/SDD_VERIFICATION.md, 기존 session-13~23은 이력만 참조 |
| P1 | 실패 진단 신뢰성 | first8/last8/omitted와 snapshot isolation 로컬 통과. 장기 요청, 상태 정리 timeout, 시작 전 거부가 lifetime에 포함되는 범위, 중복 projection 및 오류 메타데이터 소실을 검토. 카운터 이름의 뜻과 집계 경계를 명시 | request-status.mjs, native-gateway.mjs, test-request-diagnostics.mjs |
| P1 | 종료 판정 | requestOutcome과 cleanup은 분리됨. 프로세스 exit0이 모든 작업 성공으로 소비되지 않도록 사용자 문서/자동화 계약 대조. 호환성 고려 없이 exit code 변경 금지 | clauduct.mjs, docs/native.md |
| P1 | 컨텍스트·압축 | 과거 축소 창 자동 compact와 manual 후속 요청 증거 있음. 현재400K/320K의 실제 발동, 자식별 압축, 압축 후 관계/기억/도구 이력 보존 미완료. 채우기용 반복 생성으로 비용 낭비 금지 | models.mjs, compact-policy.mjs, test-compact-policy.mjs |
| P1 | 모델·Agent·Workflow | 직접 GPT/비기본 effort/직접 부모 상속의 기존 실제 증거 보존. 생성 후 모델 변경·병렬 손자·모든 review 수준·동적 native 경로를 일괄 PASS로 확대하지 않음. 공통 회귀와 변경 분기만 재검증 | gpt-agent-selection-contract.md, native-call-paths-2026-09-09.md, test-agent-selection.mjs, test-workflow-selection.mjs |
| P1 | 경계·보안 | session/parent/metadata 위조·재사용·중단·path traversal·symlink/junction·잘못된 인코딩·상한 검사. 동적 symlink 시험에는 과거 거부가 있으므로 다른 셸/경로로 재현하지 않음. 거부된 검사는 미검증으로 남김 | agent-selection.mjs, completion/workflow 테스트, 보안 감사 문서 |
| P1 | 장기 자원 안정성 | 제한된 fixture에서 메모리 admission/1000요청/20동시 검사 있음. 3-cycle 종료 후 registry/socket/timer/listener, 큐, retry 및 실패기록 상한을 측정. 단순 heap snapshot 차이를 누수로 확정하지 않음 | request-admission.mjs, native-transport.mjs, test-native.mjs |
| P1 | 인증·버전 호환성 | 실제 버전 전달 및 비차단 unverified 정책 유지. auth cache 회전·동일 프로세스 계정 전환 거부·새 프로세스 새 계정 선택의 합성 검사와 실계정 검증을 구분. Claude 버전과 Codex clientVersion을 혼동하지 않음 | poc/user-session.mjs, client-version.mjs, test-client-version.mjs |
| P1 | 현황 문서 노후화 | remaining-verification.md에 과거500K, 과거 미완료, 현재 비권장 진단 프롬프트 혼재. 최신 기준표와 역사 자료를 분리하고 실행 가능 지시는 현재 정책 한곳에만 둠 | docs/remaining-verification.md, docs/native.md, docs/audit-* |
| P2 | 실행 낭비 | session23에 Task계열21회/Skill3회. 필수 지침을 유지하며 계획 조회·갱신 반복을 줄이는 좁은 프롬프트 개선. 전역 plugin/skill/guard 변경으로 줄이지 않음 | 후속 task 프롬프트, 실행 호출 수 지표 |
| P2 | 코드 리팩토링 | 진단 allowlist/row projection/오류 계층/테스트 fixture 중복과 거대 함수 결합도를 검토. 재현 결함·명확한 중복 제거·측정 개선에 연결될 때만 변경. 파일 분리/클래스/설정 추상화 자체를 목표로 삼지 않음 | native-protocol.mjs, native-gateway.mjs, request-status.mjs 및 scoped callers |

## 5. 다음 세션의 작업 묶음

### A. 사실 정리와 검증 설계

guidance/status/diff 확인 후 현재 계약과 증거를 재대조한다. remaining-verification.md를 현재 표와 이력 링크로 정리하되 감사 원문을 지우지 않는다. 원인 미확정 항목, 이미 완료된 항목, 기존 guard 차단을 명시한다. 위험별 테스트를 읽고 실제 미커버 분기를 목록화한다. 과거 테스트 수를 새 실행 결과로 보고하지 않는다.

### B. 로컬 안정성·회귀 묶음

P0/P1에서 안전하게 재현 가능한 공백을 먼저 고친다. 같은 error/diagnostic/lifecycle 표면을 건드리는 수정과 fixture를 한 묶음으로 리뷰·검증한다. synthetic fail→false fallback 연쇄, 취소/완료 경합, 실패기록 bounded retention, 악성 입력/민감 데이터 비노출, 정상 tool 전달이 한 계약을 유지하는지 대조한다. 외부 모델 호출 없이 가능한 테스트를 먼저 끝낸다. 검증을 약화하는 수정은 하지 않는다.

### C. 감독하 판정

변경된 경로에만 필요한 native 실증을 설계한다. 인증된 native 실행은 사용자가 수행하던 기존 경계를 유지한다. 실제 쓰기 시험은 격리 저장소·로컬 커밋만 대상으로 하고 정확한 경로를 확정한다. P0 원인 미확정이면 안전 중단/데이터 보존/중복 방지 증거가 있을 때 CONDITIONAL, 그렇지 않으면 HOLD다. 모든 조건이 충족돼야 PASS다. 사용자에게 같은 Read smoke를 반복시켜 검증을 대신하지 않는다.

### D. 무인 3-cycle 판정

P0 원인 미확정이 남으면 HOLD이며 무인 PASS 시험으로 넘어가지 않는다. 선행 조건이 닫히면 실행 전 예산·상한·범위·개입 규칙을 확인하고 새 기능3개를 하나의 실행에서 연속 수행하는 계획을 제시한다. 기능은 의존성/외부 서비스 없이 의미 있는 로컬 작업을 선택한다. 각 cycle에 계획, 새 구현(TDD이면 RED→GREEN 증거), 실제 독립 리뷰 완료, 전체 회귀, 커밋을 남긴다. 외부 장애/권한 거부/사용자 개입 후 이어서 완료한 것은 회복 시험이며 무개입 PASS가 아니다. 3-cycle은 무제한·수시간 운영 안정성의 보증이 아니다.

## 6. 실행·검증·커밋 원칙

호스트 guidance와 현재 사용자 권한이 우선이며 이 파일은 권한 확장 수단이 아니다. 외부 쓰기·push·배포·인증/전역 설정·hook/guard/plugin 변경을 하지 않는다. guard 거부/timeout은 다른 도구나 하위 에이전트로 우회하지 않는다. 일반 제품 작업은 bounded/recoverable 범위에서 자율 진행한다. 별도 에이전트는 호스트가 허용하고 독립 읽기 검토 등 구체적 이득이 있을 때만 사용하며 같은 파일 동시 편집은 피한다.

기존 실행기를 먼저 읽고 프로젝트에 필요한 reviewed 테스트만 선택한다. src/run-node-tests.ps1의 Invoke-ClauductNodeTests는 환경 allowlist, 60초 제한, 단일 test concurrency를 제공하지만 OS 보안 sandbox가 아니다. 모든 src/test-*.mjs를 무검토 glob 실행하지 않는다. fixture·자식프로세스·네트워크·쓰기 대상부터 확인한다.

339fc11에서 함께 통과한 기준 묶음은 다음 11개다: test-native-gateway.mjs, test-launcher-native.mjs, test-native-protocol.mjs, test-native-transport.mjs, test-native.mjs, test-request-diagnostics.mjs, test-upstream-failures.mjs, test-unsupported-event-diagnostics.mjs, test-cancel-snapshot.mjs, test-client-version.mjs, test-compact-policy.mjs (모두 src/). 선택/완료/Workflow/보안 표면을 수정하면 관련 test-agent-selection/test-completion-selection/test-workflow-selection/test-request-admission 등도 검토 후 포함한다.

로컬 결함은 원인별로 수정하면서 작은 판별 검사와 묶음 회귀를 활용한다. 모든 변경마다 실제 인증된 시험을 요구하지 않는다. 실패의 새로운 근거 없이 같은 목표를 반복하지 않으며, 세 번 정체되면 다른 계층을 조사하거나 정확한 차단을 보고한다. task-only diff를 검수하고 의미 있는 묶음별로 제품에 로컬 커밋한다. unrelated staged/untracked를 포함하지 않는다. 인계 프롬프트는 기본적으로 uncommitted로 유지한다.

## 7. 종료 산출물

최신 검증 표의 각 행에 상태, source/test/실행 증거, 남은 위험, 다음 행동을 둔다. 최종 보고는 감독하 PASS/CONDITIONAL/HOLD와 무인 PASS/HOLD를 별도로 제시하고 전체 완료라는 모호한 표현을 쓰지 않는다. 변경·커밋·실행한 테스트·미실행/차단 검사·비용 한계·잔여 위험을 빠짐없이 보고한다. 다음 시험이 실제 필요한 경우에만 사용자가 실행할 명령과 프롬프트를 제공한다.

관련 증거는 docs/audit-2026-09-11-native-fallback-failure-history.md, docs/audit-2026-09-11-unsupported-event-diagnostics.md, docs/audit-2026-09-11-request-shape.md, docs/audit-2026-09-11-cleanup-close-race.md를 우선 읽는다. 모델 상속은 docs/audit-2026-09-10-agent-acceptance.md와 gpt-agent-selection-contract.md, 취소는 audit-2026-09-10-active-agent-cancellation.md, 보조 경로는 audit-2026-09-10-away-summary.md에 근거가 있다. 오래된 수치를 현재 설정으로 사용하지 않는다.
