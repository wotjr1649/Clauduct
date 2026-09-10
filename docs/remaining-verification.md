# 남은 실제 검증 — 현행 기준표

## 1. 이 문서를 읽는 법

이 문서의 **2~5장이 현재 기준**이다. 6장은 이력 자료이며 그 안의 수치·프롬프트는 당시 값이므로 현재 설정으로 사용하지 않는다. 감사 원문(`docs/audit-*.md`)은 지우지 않고 증거 링크로만 참조한다. 아래 상태 값의 뜻은 다음과 같다.

| 상태 | 뜻 |
|---|---|
| 완료 | 해당 범위의 수용 조건을 만족하는 증거가 있고, 변경 영향이 없으면 재검증하지 않는다 |
| 조건부 | 로컬·합성 증거는 있으나 실제 실행 증거가 없거나 일부 조건이 남아 있다 |
| 미검증 | 증거가 없다. 통과로 표시하지 않는다 |
| 차단 | guard·권한·정책으로 검사를 실행할 수 없다. 우회하지 않고 미검증으로 남긴다 |
| 범위밖 | 사용자가 이번 목표에서 제외했다 |

감독하 판정을 CONDITIONAL에서 PASS로 올리는 조건은 **실제 실패가 한 번 발생하고 그 복구가 관측되는 것**이다(2026-09-11 사용자 결정). 실패의 종류는 가리지 않으며 종료 JSON에서 다음 세 가지를 모두 확인한다.

| 조건 | 종료 JSON에서 볼 값 |
|---|---|
| 안전 중단 | 해당 요청의 `failureCategory`가 고정 라벨이고 `attempts` 수가 재시도 정책 안이며, `cleanup` 9개가 모두 true |
| 데이터 보존 | 실패 이후 요청이 이어져 성공하거나(`lifetime.succeeded` 증가) 세션이 정상 종료했고, `failureHistory`에 그 실패가 남아 있음 |
| 도구 미중복 | 실패 요청의 `retryScheduledMs`가 비어 있거나 downstream 전달 전이며, 같은 요청에서 `firstDownstreamWriteMs` 이후 재시도가 없음 |

무오류 누적만으로는 승격하지 않으며, 실패가 끝내 발생하지 않으면 CONDITIONAL이 최종 상태로 남는다.

실제 인증 실행은 사용자가 수행한다. 에이전트는 실제 Claude를 대신 실행하지 않고, 전역 설정·hook 신뢰·권한·인증 파일을 변경하거나 조회하지 않는다. 이미 통과한 항목은 변경 영향이나 새 증거가 있을 때만 다시 확인한다. 같은 smoke 시험을 반복 요청하지 않는다.

## 2. 현재 계약 값

`src/models.mjs`, `src/clauduct.mjs`, `src/native-gateway.mjs` 기준이다.

| 항목 | 현재 값 | 주의 |
|---|---|---|
| 메인 무옵션 시작값 | `gpt-6-astra` / `low` | 모델별 기본 effort와 다르다 |
| 명시 모델 기본 effort | astra=medium, sol=xhigh, terra=high, luna=max | `--effort` 명시값이 우선한다 |
| 역할 기본값 | Explore=luna/max, Plan=**sol/xhigh**, general-purpose=luna/max | 과거 문서의 `Plan=sol/high`는 오기다 |
| context 창 | window 400000, 자동 압축 목표 320000 | 설정 전달이며 backend 용량 증명이 아니다 |
| 압축 계산 | outputReserve 20000, compactPercent 84.21052631578947 | 과거 500000 / 83.33333333333334는 폐기됐다 |
| `--verify-auto-compact` | 계산 창 100000, 기본 예약량에서 약 67.4K 발동 목표 | 모델 창 400000은 유지한다. 전역 설정을 바꾸지 않는다 |
| 자식 실행 환경 | `CLAUDE_CODE_MAX_RETRIES=0`, `CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK=1` | 부모 환경은 바꾸지 않는다 |
| clauduct-inherit | 생성 시점 직접 부모의 실제 모델·effort | 메인 값이나 고정 역할값을 하드코딩하지 않는다 |
| Codex clientVersion | 관측 0.154.0 / reference 0.153.4 → `unverified` | Claude CLI 버전(2.1.26x)과 구분한다 |
| 차단 옵션 | `--settings`, `--setting-sources`, `--agents`, `--system-prompt` | 임의 agent 정의 주입을 막는다 |

## 3. 현행 상태표

| 항목 | 상태 | 증거 | 남은 위험 | 다음 행동 |
|---|---|---|---|---|
| 최초 `UNSUPPORTED_EVENT other/identifier`의 원인 | 미검증 | 세션 c4c222f8 종료 JSON(요청 57), [진단 감사](audit-2026-09-11-unsupported-event-diagnostics.md) | 기준 클라이언트 0.154.0도 모르는 이름이라 오프라인 분석은 소진됐다. 미재발을 해결로 처리하지 않는다 | 재발 시 종료 JSON의 `unsupportedEventNames`로 이름을 확보한다. 이를 위해 새 실사용 시험을 따로 만들지 않는다 |
| 미지원 이벤트 분류와 제한 캡처 | 완료 | `src/test-unsupported-event-diagnostics.mjs` 61개 검사 / loopback 36회. 형태 검사·4개 상한·상태 API 비노출 포함 | ThreadEvent는 SDK/app-server 어휘라 원인 후보에서 격하했다. 형태를 흉내 낸 문자열은 통과할 수 있다 | 없음 |
| 경계 거부가 진단에 남지 않던 공백 | 완료 | [경계 거부 감사](audit-2026-09-11-boundary-failure-diagnostics.md), `test-native-gateway` 53개 검사 | 서버 수준 clientError·CONNECT·upgrade는 여전히 카운터 밖이다 | 없음 |
| `OTHER` 분류 축소 | 완료 | 같은 감사. 요청 기록에 도달 가능한 고정 코드 추가와 접미사 정규화 | 새 오류 코드를 추가하면 목록도 함께 갱신해야 한다 | 코드 추가 시 목록 동기화 |
| native fallback 차단 | 조건부 | 자식 env·settings 동시 적용은 `test-launcher-native` 통과. 공식 [환경 변수 문서](https://code.claude.com/docs/en/env-vars)와 설치 코드 경로 확인 | 실제 native의 비스트리밍 전환 차단 분기는 실행되지 않았다 | 실제 스트리밍 오류가 난 세션의 종료 JSON을 수집한다 |
| 내용 전달 후 재시도 금지(도구 중복 실행 방지) | 완료 | `test-native-gateway` 재시도 울타리 2건. 울타리를 제거하면 실패하는 것을 확인 | downstream 전달 전 재시도는 유지되므로 upstream 계산은 중복될 수 있다 | 없음 |
| 요청 형식 거부에서 upstream 미시도 | 완료 | `test-request-diagnostics` 69개 검사 / loopback 34회, `sends=0` | 없음 | 없음 |
| 취소·등록 교체·형제 격리 | 조건부 | [취소 감사](audit-2026-09-10-active-agent-cancellation.md), `test-agent-selection`, `test-completion-selection` 46개 | 실제 UI 취소 시점, 원격 계산 중단, 전송 데이터 회수는 보장하지 않는다 | 실제 취소가 발생한 세션의 진단만 수집한다 |
| 정상 종료 자원 정리 | 완료 | [정리 경합 감사](audit-2026-09-11-cleanup-close-race.md), `test-cancel-snapshot` 6개, 세션 0d5d6174의 cleanup 9개 true | 창을 강제 종료하면 종료 JSON이 남는다고 보장하지 않는다 | 없음 |
| 종료 판정과 exit code 계약 | 완료 | [native.md](native.md) 종료 진단 절, `requestOutcome`과 `cleanup` 분리 | exit 0은 프로세스 종료·자원 정리 판정이며 요청 성공 판정이 아니다 | 호환성 검토 없이 exit code를 바꾸지 않는다 |
| 완료 알림 기반 복귀 | 조건부 | c19c8b14 실제 성공, [감사](audit-2026-09-09-completion-resume.md), 로컬 회귀 | 과거 실패(35985327)의 원인 미확정. 다중 알림·실패 알림 복귀는 미지원 | 없음 |
| 직접 부모 모델·effort 상속 | 완료 | db34be24, a2d50ff0, [수용 조건](audit-2026-09-10-agent-acceptance.md), [계약](gpt-agent-selection-contract.md) | 생성 후 모델 변경과 손자 전 조합은 미검증 | 회귀 통과만 유지한다 |
| Claude 별칭·전체 모델 ID 매핑 | 완료 | [감사](audit-2026-09-11-unmapped-agent-model.md), `test-agent-selection`의 8개 route 확인과 블록 단위 생략 검사 | 사용자가 구형 모델을 쓰지 않기로 해 구형 명명 위험은 닫혔다. 새 계열이 나오면 그 자식만 fail-closed 되고 턴은 보존된다 | 없음 |
| 신규 beta 헤더 내성 | 완료 | `test-native.mjs`의 통과·거부·기록 검사. 비호환 판정 목록 29개는 계속 거부 | 알 수 없는 beta가 실제로 계약을 바꾸면 더 뒤 단계에서 거부된다. 그 beta가 켜졌다고 가정한 클라이언트 동작은 보장하지 않는다 | 종료 JSON의 `unknownBetaNames`를 보고 allowlist를 보완한다 |
| Workflow 자식 선택 | 조건부 | 992c0794 병렬 성공, `test-workflow-selection` 36개 | custom agentType·중첩·resume은 미검증 | 없음 |
| 자동 압축 실제 발동(400K / 320K) | 미검증 | 과거 축소 창(100000)에서의 발동 증거만 있다 | 현재 기본값에서의 발동, 자식별 압축, 압축 후 기억·도구 이력 보존이 미확인 | 정상 개발 중 `compact_boundary`가 관측되면 기록한다. 채우기용 반복 생성은 하지 않는다 |
| 장기 자원 안정성 | 조건부 | `test-native` 45개(1000요청 / 20동시 포함), `test-request-admission`, `test-native-gateway`의 등록표 상한 검사 | 등록표는 1024로 묶였고 유휴 LRU만 제거한다. 3주기 종료 후 socket·timer·listener 실측은 여전히 없다 | 실제 장시간 실행 시 종료 JSON의 `cleanup`·`admission`·`agentRegistrationsEvicted`를 함께 본다 |
| HTTP 서버 수준 거부 집계 | 완료 | `lifetime.transportRejections`, `test-native-gateway`의 Expect 검사 | 연결 단계에서 끊긴 바이트의 원인까지는 남기지 않는다 | 없음 |
| native 기능 지원 범위 | 완료 | [전수 대조](native-feature-support.md) | 로컬 MCP·이미지·plan mode 등 개별 실제 왕복은 미검증으로 명시 | 정상 사용 중 관측되면 기록 |
| 인증·계정 경계 | 조건부 | `test-client-version` loopback 12회, [요청 형식 감사](audit-2026-09-11-request-shape.md)의 계정 경계 절 | 실계정 회전과 프로세스 내 계정 변경 거부는 합성 검사만 통과했다 | 인증 파일을 조회하지 않는다 |
| 보안 경계(위조·재사용·중단·경로) | 조건부 | `test-agent-selection`, `test-completion-selection`, `test-workflow-selection`, [중단 metadata 수정](audit-2026-09-10-stopped-agent-selection.md) | 아래 차단 항목 참조 | 없음 |
| 동적 symlink·junction 검사 | 차단 | `test-completion-selection --symlink`와 `test-workflow-selection`이 `notRun`으로 보고 | 실제 링크 우회 방어는 미검증으로 남는다 | 다른 셸·경로로 재현하지 않는다 |
| SDD 무인 3주기 | 미검증 | run-02(38e9c28)의 보존 결과 통합만 완료 | 하나의 실행에서 무개입 3주기를 마친 증거가 없다 | 5.4의 선행 조건이 닫히기 전에는 시험하지 않는다 |
| Claude 모델 전체 지원·app-server 전환·버전 pin | 범위밖 | 사용자 지정. 별칭·전체 ID 매핑은 2026-09-11에 사용자가 별도 승인했다 | — | — |
| 실제 인증 갱신·수시간 연속 실행 | 범위밖 | 사용자 지정 | — | — |
| 코드 리팩토링(파일 분리·추상화) | 범위밖 | 재현 결함이나 측정 근거가 없어 수행하지 않았다 | 큰 함수의 결합도는 남아 있다 | 결함이나 측정 근거가 생기면 그때 착수한다 |

## 4. 최신 증거

### 4.1 사용자가 제공한 실제 실행 종료 JSON

| 세션 | 관측 | 확대 해석 금지 |
|---|---|---|
| 5f27e1c8 | 요청 37건 전부 성공, `requestOutcome=all-succeeded`, `failureHistory` 빈 배열 / `omitted=0`, `cleanup` 9개 true, 자식 fallback 설정 true, 최근 16개 sol/low | 실패가 없었으므로 fallback 차단 분기와 실패 이력 실발생은 미검증이다 |
| 0d5d6174 | 요청 3건 성공, sol/low, cleanup 9개 true, 종료 SUCCESS | 취소 경로나 과거 정리 실패 원인의 소급 확정이 아니다 |
| 878f5eda | 요청 3건 성공, luna/max, `unsupportedEventTypeFormat` 출력됨(오류가 없어 null) | 새 진단의 실패 분기는 미검증이다 |
| 1433a5dd | 요청 4 성공 → 5 `UNSUPPORTED_EVENT/other` → 6 prepare 단계 `REQUEST_SHAPE` | 후속 요청이 native fallback인지는 미확인이다 |
| c4c222f8 | 총 49 / 성공 44 / 실패 5. 요청 57 `UNSUPPORTED_EVENT other/identifier`, 58 `REQUEST_STREAM_FALSE` | 앞선 실패 3건의 상세는 복원할 수 없다 |

이 표의 출처는 대화로 제공된 종료 JSON이다. 원문 JSON을 저장하지 않으며 누락된 요청별 값을 추정해 채우지 않는다.

### 4.2 이 저장소의 로컬 검사

2026-09-11, Node.js v24.19.0, 프로젝트 `Invoke-ClauductNodeTests`, 60초 제한, test concurrency 1.

기준 11개 파일이 함께 통과했다: `test-native-gateway`(53), `test-launcher-native`, `test-native-protocol`, `test-native-transport`, `test-native`(45), `test-request-diagnostics`(69 / loopback 34), `test-upstream-failures`(84 / loopback 62), `test-unsupported-event-diagnostics`(61 / loopback 36), `test-cancel-snapshot`(6), `test-client-version`(loopback 12), `test-compact-policy`. 모두 `src/`의 `.mjs`다.

선택·완료 표면도 함께 통과했다: `test-agent-selection`, `test-completion-selection`(46, symlink notRun), `test-workflow-selection`(36, native Workflow·symlink notRun), `test-request-admission`.

모든 실행에서 외부 추론 요청 0, 실제 인증 조회 0, 실제 Claude 실행 0이다. 과거 실행의 검사 수를 새 실행 결과로 보고하지 않는다.

## 5. 실행 가능한 절차

실행 지시는 이 장에만 둔다. 6장 이력의 프롬프트는 재실행 대상이 아니다.

### 5.1 로컬 회귀

PowerShell 7에서 프로젝트 실행기를 직접 사용한다.

```powershell
. D:/AIDEV/Clauduct/src/run-node-tests.ps1
Invoke-ClauductNodeTests -Root D:/AIDEV/Clauduct -TimeoutSeconds 60 -TestFiles @(
  'src/test-native-gateway.mjs', 'src/test-launcher-native.mjs', 'src/test-native-protocol.mjs',
  'src/test-native-transport.mjs', 'src/test-native.mjs', 'src/test-request-diagnostics.mjs',
  'src/test-upstream-failures.mjs', 'src/test-unsupported-event-diagnostics.mjs',
  'src/test-cancel-snapshot.mjs', 'src/test-client-version.mjs', 'src/test-compact-policy.mjs')
```

모든 `src/test-*.mjs`를 무검토 glob으로 실행하지 않는다. fixture·자식 프로세스·네트워크·쓰기 대상을 먼저 확인하고 필요한 파일만 나열한다. 선택·완료·Workflow·보안 표면을 수정했으면 `test-agent-selection`, `test-completion-selection`, `test-workflow-selection`, `test-request-admission`을 포함한다. 이 실행기는 환경 allowlist·시간 제한·단일 concurrency를 제공하지만 OS 보안 sandbox가 아니다.

### 5.2 실제 실행 후 증거 수집

native 세션을 정상 종료하면 launcher가 `Clauduct 종료: <분류>`와 `CLAUDUCT_REQUEST_STATUS <JSON>`을 출력한다. 이 출력을 그대로 수집한다. 세션 안에서 `node src/request-status.mjs`를 부르는 방식은 뒤이어 assistant 처리가 붙어 모델 요청 없는 수집이 아니므로 사용하지 않는다.

먼저 볼 값: `requestOutcome`, `lifetime.failed`, `lifetime.failuresByStage`, `lifetime.rejectedBeforeStart`, `lifetime.firstRejectedCategory`, `failureHistory.records[].failureCategory`, `unsupportedEventNames`, `unknownBetaNames`, `lifetime.unmappedAgentModels`, `cleanup`의 9개 항목, `clientExecutionPolicy.nonStreamingFallbackDisabled`.

`unsupportedEventNames`에 값이 있으면 그것이 P0의 실제 이름이다. 세션 안에서 `request-status.mjs`로 조회하면 이 필드는 제거된 `null`로 나오므로, 반드시 종료 후 launcher 출력에서 확인한다.

첫 오류에서 중단하고 정상 종료 JSON을 수집한다. 실패한 작업을 자동으로 반복하지 않는다.

### 5.3 자동 압축 저비용 확인

정상 세션과 구분되는 새 실행에서만 사용한다.

```powershell
D:\AIDEV\Clauduct\clauduct.cmd --model luna --effort max --verify-auto-compact
```

`/context`에서 400K 모델 창을, 진단에서 `autoCompactWindow=100000`을 확인한다. 목표는 기본 출력 예약량에서 약 67.4K 발동이다. `/autocompact` 값 지정으로 전역 설정을 바꾸지 않는다. 자동 압축이 비활성화돼 있으면 덮어쓰지 않고 그 조건을 보고한다. transcript의 `compact_boundary`에서 `trigger=auto`, 압축 후 토큰 감소, 후속 요청 성공으로 판정한다. 메인의 성공을 모든 자식의 압축 성공이나 backend 용량 수락으로 확대하지 않는다.

### 5.4 무인 3주기 선행 조건

다음이 모두 닫히기 전에는 무인 판정을 보류한다.

1. 최초 `UNSUPPORTED_EVENT`의 원인이 확정되거나, 재발 시 고정 이름·안전 중단·데이터 보존·도구 중복 방지가 실제 실행으로 확인될 것.
2. 실행 시간·비용·동시성·중단 조건과 실행 주체를 시험 전에 별도로 확인할 것.
3. 기능 3개가 의존성·외부 서비스 없이 의미 있는 로컬 작업일 것.

각 주기에 계획, 새 구현(TDD면 RED→GREEN 증거), 실제 독립 리뷰 완료, 전체 회귀, 로컬 커밋을 남긴다. 외부 장애·권한 거부·사용자 개입 뒤 이어서 완료한 것은 회복 시험이며 무개입 PASS가 아니다. 계획된 RED assertion 실패는 예상된 개발 증거이며 API·환경 오류와 분리해 기록한다. 3주기 통과는 무제한·수시간 운영 안정성의 보증이 아니다.

## 6. 이력 자료

아래는 과거 검증 기록이다. **수치와 프롬프트는 당시 값이며 현재 설정이 아니다.** 재실행 지시가 아니다.

- 이관 기준과 호출 경로 조사: [호출 경로](native-call-paths-2026-09-09.md), [초기 감사](audit-2026-09-08.md). 당시 기준 실행 파일은 Claude 2.1.266이었고 이후 2.1.267 실행이 관측됐다. 동적 전체 기능 목록은 미완료로 남아 있다.
- 진단 확장 경과: [완료 이후 SSE](audit-2026-09-09-completion-diagnostics.md), [요청 상관](audit-2026-09-09-request-correlation.md), [lifetime](audit-2026-09-09-lifetime-diagnostics.md), [upstream 오류 상세](audit-2026-09-10-upstream-error-details.md), [실패 보존](audit-2026-09-10-upstream-failure-preservation.md), [요청 형식](audit-2026-09-11-request-shape.md), [fallback과 실패 이력](audit-2026-09-11-native-fallback-failure-history.md), [테스트 실행기](audit-2026-09-10-request-diagnostics-test-runner.md).
- 모델·역할·Workflow 실제 성공: [Plan 역할](audit-2026-09-09-plan-lifetime-success.md), [완료 성공](audit-2026-09-09-completion-success.md), [직접 부모](audit-2026-09-10-direct-parent-success.md), [병렬 Workflow](audit-2026-09-10-workflow-parallel-success.md), [/btw 경로](audit-2026-09-10-native-btw-path.md), [away 요약](audit-2026-09-10-away-summary.md), [세션20 취소 snapshot](audit-2026-09-10-session20-cancel-snapshot.md).
- 압축 실측(과거 500K / 83.33% 설정): f236f867 request 11에서 admission 0.15ms, 첫 이벤트 2212.41ms, 첫 텍스트 10776.14ms, 완료 64177.91ms, 재시도 없음. 922e6ba0에서 수동 압축 경계 뒤 새 Read 성공. b6d81841에서 검증 모드 `autoCompactWindow=100000` 자동 압축 68608ms(98097→39330)와 후속 요청 성공.
- 리뷰·병렬 실행: [native high-02](audit-2026-09-09-native-high-02.md), [native high-03](audit-2026-09-09-native-high-03.md). low와 high 최종 반환은 확인했고 모든 review 수준은 확인하지 않았다.
- 세션별 실행 프롬프트는 `docs/prompts/`에 uncommitted 이력으로 남아 있다. session-13~23은 절차 기록이며 그 전제(미커밋 상태 등)를 현재 상태에 적용하지 않는다.
