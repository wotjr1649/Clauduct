# 남은 실제 검증 — 현행 기준표

## 1. 이 문서를 읽는 법

이 문서의 **2~5장이 현재 기준**이다. 6장은 이력 자료이며 그 안의 수치·프롬프트는 당시 값이므로 현재 설정으로 사용하지 않는다. 감사 원문(`docs/audit-*.md`)은 지우지 않고 증거 링크로만 참조한다. 아래 상태 값의 뜻은 다음과 같다.

| 상태 | 뜻 |
|---|---|
| 완료 | 해당 범위의 수용 조건을 만족하는 증거가 있고, 변경 영향이 없으면 재검증하지 않는다 |
| 조건부 | 로컬·합성 증거는 있으나 실제 실행 증거가 없거나 일부 조건이 남아 있다 |
| 미검증 | 증거가 없다. 통과로 표시하지 않는다 |
| 복구 불가 | 당시 필요한 증거가 보존되지 않아 과거 원인을 확정할 수 없다. 수정 완료나 정상 동작을 뜻하지 않는다 |
| 차단 | guard·권한·정책으로 검사를 실행할 수 없다. 우회하지 않고 미검증으로 남긴다 |
| 범위밖·이관 | 사용자가 이번 목표의 자동 시험에서 뺐거나 사용자 실사용 확인으로 넘겼다. PASS로 바꾸지 않고 제외·이관으로 기록한다 |

감독하 판정은 2026-09-11 **PASS**다. 조건이던 "실제 실패 1회와 그 복구 관측"이 세션 46b6af24에서 충족됐다. [실사용 검증 기록](audit-2026-09-11-live-session-verification.md).

| 조건 | 관측 |
|---|---|
| 안전 중단 | request 3이 고정 라벨 `UNSUPPORTED_BETA`, upstream 시도 0, 재시도 0, `clientDisconnected` false. 종료 시 `cleanup` 9개 모두 true |
| 데이터 보존 | 실패가 `failureHistory`에 보존되고 `omitted` 0. 실패 직후 대화가 이어졌고 이후 요청 7건이 모두 성공 |
| 도구 미중복 | upstream 시도가 0회라 중복 실행이 성립하지 않음 |

이 과거 PASS의 범위는 **감독하 사용**이다. 2026-09-12에는 별도로 bounded 비대화형 실행·실패 복구·로컬 릴리즈를 검증했다(4.0절). 무제한 무인 연속 개발은 여전히 HOLD이며, 아래 표의 미검증·조건부 항목이 PASS로 바뀌는 것도 아니다.

원래의 승격 조건은 다음과 같았다.

| 조건 | 종료 JSON에서 볼 값 |
|---|---|
| 안전 중단 | 해당 요청의 `failureCategory`가 고정 라벨이고 `attempts` 수가 재시도 정책 안이며, `cleanup` 9개가 모두 true |
| 데이터 보존 | 실패 이후 요청이 이어져 성공하거나(`lifetime.succeeded` 증가) 세션이 정상 종료했고, `failureHistory`에 그 실패가 남아 있음 |
| 도구 미중복 | 실패 요청의 `retryScheduledMs`가 비어 있거나 downstream 전달 전이며, 같은 요청에서 `firstDownstreamWriteMs` 이후 재시도가 없음 |

무오류 누적만으로는 승격하지 않으며, 실패가 끝내 발생하지 않으면 CONDITIONAL이 최종 상태로 남는다.

2026-09-12 릴리즈 작업에서는 사용자가 비대화형 실제 실행을 명시적으로 요청해 새 작업 전용 프로필과 공개 fixture로 검증했다. 기존 credential supplier를 사용하고 전역 설정·hook 신뢰·권한·인증 파일은 변경하지 않았으며 원문 인증 값을 출력하지 않았다. 당시의 실행 요청을 향후 임의 작업의 상시 권한으로 사용하지 않는다. 이미 통과한 항목은 변경 영향이나 새 증거가 있을 때만 다시 확인한다.

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
| 비대화형 진입 | 명시적 `-p` / `--print` | native 결과는 stdout, wrapper 안내·종료 진단은 stderr. 기본 대화형은 TTY 요구 유지 |

## 3. 현행 상태표

| 항목 | 상태 | 증거 | 남은 위험 | 다음 행동 |
|---|---|---|---|---|
| 과거 `UNSUPPORTED_EVENT other/identifier` 두 건의 원인 | 복구 불가 | 세션 c4c222f8 종료 JSON(요청 57), [진단 감사](audit-2026-09-11-unsupported-event-diagnostics.md). run-04에서도 이름을 못 잡았다 — [분석과 수정](audit-2026-09-11-run-04-exit-diagnostics.md) | 이름 두 건 모두 소실됐다. 원인은 여전히 미확정이며 포착 수정은 지난 두 건을 밝히지 않는다 | 과거 원인 복원은 종료한다. 새 재발은 종료 JSON의 `unsupportedEventNames`와 `unsupportedEventNamesWithheld`로 별도 판정한다 |
| 미지원 이벤트 분류와 제한 캡처 | 완료 | 2026-09-12 재실행: `src/test-unsupported-event-diagnostics.mjs` 63개 검사 / loopback 37회. 형태 검사·4개 상한·상태 API 비노출 포함 | ThreadEvent는 SDK/app-server 어휘라 원인 후보에서 격하했다. 형태를 흉내 낸 문자열은 통과할 수 있다 | 없음 |
| 경계 거부가 진단에 남지 않던 공백 | 완료 | [경계 거부 감사](audit-2026-09-11-boundary-failure-diagnostics.md), 2026-09-12 `test-native-gateway` 61개 검사 | 요청 기록 전 거부는 `rejectedBeforeStart`, 서버 수준 clientError·CONNECT·upgrade·Expect 거부는 별도 `transportRejections` 집계다 | 없음 |
| `OTHER` 분류 축소 | 완료 | 같은 감사. 요청 기록에 도달 가능한 고정 코드 추가와 접미사 정규화 | 새 오류 코드를 추가하면 목록도 함께 갱신해야 한다 | 코드 추가 시 목록 동기화 |
| native fallback 차단 | 완료 | 통제 실험 — 같은 주입 자극에 설정만 다른 두 세션에서 대조군만 `REQUEST_STREAM_FALSE`를 냈다(prepare 실패 1 대 0). [실험 기록](audit-2026-09-12-fallback-arms.md). 자식 env·settings 동시 적용은 `test-launcher-native`, 주입 경로는 `test-fallback-verification` 19개 | 여전히 효과의 관측이지 바이너리 안 분기의 직접 관측은 아니다. 이 클라이언트 버전(0.154.0)에 대한 판정이며 조건식이 바뀌면 다시 돌려야 한다 | `--verify-fallback blocked\|allowed`로 재현한다. 클라이언트가 올라가면 다시 본다 |
| 내용 전달 후 재시도 금지(도구 중복 실행 방지) | 완료 | `test-native-gateway` 재시도 울타리 2건. 울타리를 제거하면 실패하는 것을 확인 | downstream 전달 전 재시도는 유지되므로 upstream 계산은 중복될 수 있다 | 없음 |
| 요청 형식 거부에서 upstream 미시도 | 완료 | `test-request-diagnostics` 69개 검사 / loopback 34회, `sends=0` | 없음 | 없음 |
| 취소·등록 교체·형제 격리 | 조건부 — **유지**(2026-09-14 범위 변경이 건드리지 않음) | [취소 감사](audit-2026-09-10-active-agent-cancellation.md), `test-agent-selection`, `test-completion-selection` 46개. run-04 취소 3건 중 자식 2건은 `clientDisconnected: false`, 1건은 클라이언트 선이탈. 2026-09-12 인수인계에 보존된 사용자 보고: UI에서 직접 취소했고 오류가 없었다 | 사용자 UI 보고의 세션·요청 ID·취소 시각은 미기록이다. 이 관측을 등록 교체·형제 격리 전체의 실제 검증으로 확대하지 않는다. 원격 계산 중단과 전송 데이터 회수는 보장하지 않는다 | 취소와 형제 격리는 F12 후속의 최소 수용 사례에서 함께 판정한다(3.1절). 등록 교체와 UI 관측은 별도로 남는다 |
| 정상 종료 자원 정리 | 완료 | [정리 경합 감사](audit-2026-09-11-cleanup-close-race.md), `test-cancel-snapshot` 6개, 세션 0d5d6174의 cleanup 9개 true | 창을 강제 종료하면 종료 JSON이 남는다고 보장하지 않는다 | 없음 |
| 비대화형 작업·재개·오류 복구 | 완료 | [릴리즈 검증](release-readiness.md): 실제 새 프로세스의 session ID 일치·공개 코드 회상, MCP 1회 실행 후 응답 중 오류·명시적 재개·도구 미중복·정리 9개 true | 모든 서버 장애의 무중단 처리를 보장하지 않는다. 전달 후 오류는 명시적 재개가 필요하다 | 최종 릴리즈 구성으로 회귀·배포 검증 |
| native background 작업·TaskStop | 완료 | 같은 릴리즈 검증: 실제 Bash background 1회·TaskOutput 1회, 별도 TaskStop 뒤 worker PID 종료 확인 | 사용자 UI 키 입력 시각·Agent 등록 교체·형제 격리 전체의 실제 검증은 아님 | 기존 취소·형제 격리 조건부 판정 유지 |
| headless 최종 텍스트·도구 축소 후 이력 보존 | 완료 | 빈 JSON 결과와 과거 도구의 `INVALID_TOOL_CALL`을 실제 재현·수정. 수정 전 실패 회귀와 실제 resume/failure-resume 통과 | 비활성 도구의 새 호출은 계속 거부한다 | 없음 |
| 빈 text done 처리·불일치 이벤트 진단 | 완료 | delta 없는 빈 `response.output_text.done` 실패를 실제·합성 재현 후 수정. 비어 있지 않은 누락 delta는 계속 거부. `snapshotMismatchEvent` 고정 라벨 검사 | 이전 실패의 원문 복원 기능은 아님 | 새 불일치의 이벤트 종류로 위치를 구분 |
| 종료 판정과 exit code 계약 | 완료 | [native.md](native.md) 종료 진단 절, `requestOutcome`과 `cleanup` 분리 | exit 0은 프로세스 종료·자원 정리 판정이며 요청 성공 판정이 아니다 | 호환성 검토 없이 exit code를 바꾸지 않는다 |
| 단일 완료 알림 기반 복귀 | 완료 | c19c8b14의 [실제 성공](audit-2026-09-09-completion-success.md), 2026-09-12 `test-completion-selection` 46개 통과 | 같은 gateway의 검증 이력과 단일 `completed` 알림을 연결한 경로에 한정한다. [지원 한계](audit-2026-09-09-completion-resume.md)는 유지한다 | 없음 |
| 다중 알림·실패/취소 알림에 의한 자동 복귀 | 범위밖 | 설계상 미지원. 실패·취소 알림의 거부는 로컬 회귀로 확인했고, [run-04](audit-2026-09-11-three-cycle-run-04.md)에서는 결과가 이미 전달된 실패 알림 뒤 올바른 비재실행을 관측했다 | run-04는 실패 알림 자동 복귀의 성공이나 gateway 거부 분기의 실측이 아니다 | 측정 대기 항목으로 두지 않는다 |
| 과거 완료 복귀 실패(35985327)의 원인 | 복구 불가 | [당시 진단](audit-2026-09-09-completion-diagnostics.md): `CALL`만 남았고 당시 gateway 메모리·파일 가용 시점 증거가 없다 | 보존 기록의 재구성 성공으로 과거 원인을 소급 확정하지 않는다 | 과거 원인 복원은 종료한다. 새 실패는 `completionFailure`·`completionParentState`·`completionChildState`로 별도 판정한다 |
| 직접 부모 모델·effort 상속 | 완료 | db34be24, a2d50ff0, [수용 조건](audit-2026-09-10-agent-acceptance.md), [계약](gpt-agent-selection-contract.md) | 생성 후 모델 변경과 손자 전 조합은 미검증 | 회귀 통과만 유지한다 |
| Claude 별칭·전체 모델 ID 매핑 | 완료 | [감사](audit-2026-09-11-unmapped-agent-model.md), `test-agent-selection`의 8개 route 확인과 블록 단위 생략 검사 | 사용자가 구형 모델을 쓰지 않기로 해 구형 명명 위험은 닫혔다. 새 계열이 나오면 그 자식만 fail-closed 되고 턴은 보존된다 | 없음 |
| 신규 beta 헤더 내성 | 완료 | `test-native.mjs`의 통과·기록 검사. 이름만으로는 거부하지 않고 형식 오류만 거부한다. 판정 27개는 `judgedBetaLabels`로 기록 | 알 수 없는 beta가 실제로 계약을 바꾸면 더 뒤 단계에서 거부된다. 그 beta가 켜졌다고 가정한 클라이언트 동작은 보장하지 않는다 | 종료 JSON의 `unknownBetaNames`를 보고 allowlist를 보완한다 |
| Workflow 자식 선택 | 완료 | 992c0794 병렬 성공, `test-workflow-selection` 36개, 그리고 실제 실행 관측 — 자식 둘에 명시한 `luna`/`terra`가 각각 `gpt-5.6-luna`/`gpt-5.6-terra`로 도달했다(자식 metadata와 transcript 직접 대조). [라우팅 관측](audit-2026-09-12-workflow-routing.md) | 중첩은 경로가 없다 — Workflow 자식에게 Agent 도구가 없어 `StructuredOutput`만 쓴다. 커스텀 agentType은 적용 지점이 그 중첩뿐이라 미검증으로 남고, Workflow 자식 자체는 `workflow-subagent`로 고정이라 타입을 받지 않는다. resume은 캐시 적중 경로만 확인했다. effort는 스크립트가 주지 않고 부모에게서 상속된다(`effort: high` 실측) | 커스텀 agentType 라우팅과 캐시 미적중 resume은 정상 사용 중 관측되면 기록한다. 새 시험을 만들지 않는다 |
| 자동 압축 실제 발동(400K / 320K) | **이관** — 사용자 실사용 검증 | 축소 창(100000)에서의 발동은 메인 3건·자식 1건이 실측됐다(아래 관측 기록). 현재 기본값에서의 발동 증거는 없다 | 현재 기본값(400K/320K)에서의 발동과 압축 후 기억·도구 이력 보존은 여전히 미확인이며 이관은 PASS가 아니다. 2장의 설정값과 변환·라우팅 검사는 유지한다 | 출하 전 자동 시험에서 제외한다. 정상 개발 중 `compact_boundary`가 관측되면 기록한다. 채우기용 반복 생성은 하지 않는다 |
| 웹 검색 경로 | 완료 | [브리지 감사](audit-2026-09-11-web-search-bridge.md). 실사용 세션 a6e7f1ad 요청 13: `webSearchAnswered=true`, `webSearchCalls=1`, `webSearchLinks=15`, `firstContentBlock=server_tool_use`, 모델이 출처를 인용 | 게이트웨이가 side query를 직접 답한다. `alpha/`는 알파 경로라 사라지거나 모양이 바뀔 수 있다. 그때는 `SEARCH_UNAVAILABLE`·`SEARCH_HTTP_ERROR`·`SEARCH_RESPONSE_SHAPE`로 이름이 붙어 실패하며 조용한 빈 결과가 되지 않는다 | 탐지가 빗나가면 세션당 1회 stderr 통지가 뜬다. 그때 실제 요청 모양을 확보한다 |
| WebFetch | 완료 | 세션 4851a91a에서 `https://example.com` 정상 성공. 요청 3개(결정·apply·후속) 중 `effort: high`인 apply 호출이 텍스트를 반환 | 이전 한 번의 `No response from model`은 대상 URL 미기록으로 재현 불가. 게이트웨이는 거부한 적이 없다 | 재발 시 `effort: high` 요청의 타이밍으로 귀속한다 |
| 신규 기능 감지 | 완료 | `src/scan-native-features.mjs`, 현재 관측 50 / 미분류 0 | 바이너리 문자열 기반이라 동적 기능은 잡지 못한다 | Claude 업데이트 후 한 번 실행 |
| 장기 자원 안정성 | 완료 | run-04(2c4ceab9) 종료 JSON 실측 — 2시간 37분·408요청·자식 9명 뒤 `cleanup` 9개 전부 true, `gatewayIdle` true, `agentRegistrationsEvicted` 0, `agentRegistrationsExpired` 1. [종료 진단 분석](audit-2026-09-11-run-04-exit-diagnostics.md). 합성 근거는 `test-native` 47개, `test-request-admission` | admission이 메모리 기반이라 같은 부하에서 요청 4건이 6~7분 대기 끝에 클라이언트 이탈로 끝났다. 죽지는 않았으나 지연은 실재한다 | socket·timer·listener를 OS 수준에서 별도 실측하지는 않았다 |
| HTTP 서버 수준 거부 집계 | 완료 | `lifetime.transportRejections`, `test-native-gateway`의 Expect 검사 | 연결 단계에서 끊긴 바이트의 원인까지는 남기지 않는다 | 없음 |
| native 기능 지원 범위 | 완료 | [전수 대조](native-feature-support.md), [릴리즈 검증](release-readiness.md)의 실제 Read/Edit·Bash·PowerShell·stdio MCP·PNG·WebFetch·WebSearch | 모든 사용자 plugin·hook·UI 조합을 실측한 것은 아님 | 남은 개별 범위를 완료와 구분 |
| 인증·계정 경계 | 갈림 — 정상 갱신 판정은 **제외**, 401/403·재시도·비밀·계정 혼입 경계는 **유지**(조건부) | 2026-09-15 실행한 `src/test-credential-recovery.mjs` 23개 검사 exit 0: `401-stable`→`UNAUTHENTICATED`, `403`→`UPSTREAM_HTTP_ERROR`/`SEARCH_HTTP_ERROR`, `limited-401`→`REQUEST_BUDGET`, `401-account-change`·`changed-between-requests`→`CREDENTIAL_ACCOUNT_CHANGED`, 23행 전부 `secretFieldsAbsent` true, `actualCredentialReads` 0. 같은 결과가 제외 범위를 `normalOAuthRefresh: NOT_RUN`으로 스스로 보고한다. 그 밖에 `test-client-version` loopback 12회, [요청 형식 감사](audit-2026-09-11-request-shape.md)의 계정 경계 절 | 정상 갱신(실계정 회전)은 사용자 Codex CLI 로그인 확인으로 제외됐다. 유지 범위인 프로세스 내 계정 변경 거부는 합성 loopback 검사만 통과했다 | 인증 파일을 조회하지 않는다. 제외된 정상 갱신을 PASS로 적지 않는다 |
| 보안 경계(위조·재사용·중단·경로) | 조건부 — **유지**(사유 교체) | `test-agent-selection`, `test-completion-selection`, `test-workflow-selection`, [중단 metadata 수정](audit-2026-09-10-stopped-agent-selection.md). 차단된 것은 이 행 전체가 아니라 동적 링크 한 갈래다 — `src/test-completion-selection.mjs`의 `--symlink` 블록은 단일 assertion이고 위조·재사용·중단 검사는 기본 실행에서 돈다 | 위조·재사용·중단은 로컬 합성 검사로만 판정했다. 동적 링크 우회 방어는 출하 선행 조건에서 빠졌을 뿐 미검증으로 남는다(아래 행) | 없음. 차단 항목이 범위밖이 됐다는 이유로 완료로 올리지 않는다 |
| 동적 symlink·junction 검사 | **범위밖** — 2026-09-14 사용자 지시로 제외 | `test-completion-selection --symlink`와 `test-workflow-selection`이 `notRun`으로 보고. 제외 근거는 [필수성 판단](release-priority-assessment-2026-09-14.md) | 실제 링크 우회 방어는 미검증으로 남는다. 차단 이력을 PASS로 바꾸지 않으며 제품의 경로·신원·기록 재사용 보호 코드는 유지한다 | 다른 셸·경로로 재현하지 않는다 |
| SDD 무인 3주기 | 완료 | run-04(2c4ceab9)이 THREE-CYCLE-PASS — [run-04 감사](audit-2026-09-11-three-cycle-run-04.md). 커밋 3개, 독립 재실행 63/63 pass, 자식 9명, 개입 0. 종료 JSON도 확보해 [분석](audit-2026-09-11-run-04-exit-diagnostics.md)했고, 보존된 자식 행은 전부 `model: gpt-5.6-luna` / `effort: max`이며 `requestedModel`·`requestedEffort`와 일치한다 | 게이트웨이가 보낸 라우팅까지가 관측이다. upstream이 그 effort로 실제 추론했는지는 이쪽에서 관측할 수 없다. 자식 9명이 개별로 다 보존된 것도 아니다(보존은 first-8·last-8과 최근 요청) | 없음 |
| Claude 모델 전체 지원·app-server 전환·버전 pin | 범위밖 | 사용자 지정. 별칭·전체 ID 매핑은 2026-09-11에 사용자가 별도 승인했다 | — | — |
| 실제 인증 갱신·수시간 연속 실행 | 범위밖 | 사용자 지정 | — | — |
| 코드 리팩토링(파일 분리·추상화) | 범위밖 | 재현 결함이나 측정 근거가 없어 수행하지 않았다 | 큰 함수의 결합도는 남아 있다 | 결함이나 측정 근거가 생기면 그때 착수한다 |

### 3.1 2026-09-14 범위 변경 대조 (2026-09-15 정리)

권위 있는 출처는 [필수성 판단](release-priority-assessment-2026-09-14.md)의 판단표다. 아래는 그 판단을 위 표에 반영한 결과다. **HOLD 해제도 출하 PASS 표기도 아니다** — 근거만 모으고 선언은 사용자가 한다.

**먼저 시간 순서.** [진행 기록](unattended-release-progress.md)은 최신 문단이 위에 쌓이는 구조라 파일 순서가 시간 순서가 아니다. `docs/unattended-release-progress.md:11`의 "F12·F14·F22로 전체 HOLD"는 그보다 **나중**인 `docs/unattended-release-progress.md:7`의 범위 변경 이전 문단이다. 같은 파일 `docs/unattended-release-progress.md:5`가 "아래 이전 마감의 F22 포함 '세 공백'은 범위 변경 전 이력"이라고 직접 표시하고, 뒤쪽 문단이 스스로를 "후속 사용자 변경"이라 부른다. 두 표시가 일치한다. 문단 하나만 읽고 HOLD 사유를 세면 틀린다.

| 행 | 판정 | 근거 |
|---|---|---|
| 자동 압축 실제 발동(400K/320K) | **이관** | `docs/unattended-release-progress.md:17` — 사용자 실사용 검증으로 이관하고 기본 압축 발동을 요구하는 F10·F18 및 다른 행의 해당 부분을 출하 전 자동 시험에서 제외한다. 설정값·변환·라우팅 검사는 유지하므로 2장의 계약 값은 그대로다 |
| 동적 symlink·junction 검사 | **제외(범위밖)** | `docs/unattended-release-progress.md:7`, `docs/release-completion-2026-09-14.md:11`. 판단표는 "F22 전체의 신원·경로·기록 재사용 검증을 없애는 뜻은 아니다"라고 못박으므로 차단 이력과 제품 경로 보호는 보존한다 |
| 인증·계정 경계 | **갈림 — 제외 + 유지** | `docs/unattended-release-progress.md:17`이 정상 갱신 판정과 갱신 주체 구현·검증을 제외하고, `docs/unattended-release-progress.md:11`이 401/403·인증 부재의 정확한 보고, 무한 재시도 금지, 비밀 미노출, 다른 계정 혼입 차단을 유지로 못박는다. 2026-09-15 `src/test-credential-recovery.mjs` 실행이 유지 범위를 23개 검사로 확인했고, 같은 결과가 제외 범위를 `normalOAuthRefresh: NOT_RUN`으로 보고한다 |
| 보안 경계(위조·재사용·중단·경로) | **유지 — 사유 교체** | 조건부 사유였던 "아래 차단 항목 참조"는 그 항목이 범위밖이 되면서 **출하 선행 조건으로서는** 사라졌다. 그래도 완료로 올리지 않는다: 판단표가 차단 이력을 PASS로 바꾸지 말라고 적고, 이 행의 "경로" 하위 범위에는 동적 링크 증거가 여전히 없다. 확인한 사실은 차단 범위가 좁다는 것이다 — `--symlink` 블록은 단일 assertion이고 나머지 위조·재사용·중단 검사는 기본 실행에서 돈다 |
| 취소·등록 교체·형제 격리 | **유지** | 범위 변경이 건드리지 않았다. 다만 판단표의 F12 최소 수용 사례가 "형제 혼입"과 "사용자 취소"를 명시하므로 이 두 하위 범위는 F12 후속에서 함께 판정하고 따로 추적하지 않는다. 등록 교체는 "오래된 요청 알림"과 겹칠 뿐 같지 않아 별도로 남는다 |

사용자 UI 취소의 세션·요청 ID와 취소 시각은 클라이언트 측 정보라 gateway 기록으로 복원되지 않는다. F12의 명시적 수집(`TaskOutput`)·메인 중계로 옮기면 취소가 gateway 자기 기록에 남아 UI 보고 의존이 줄어들 것으로 **추정**한다 — 측정하지 않았다. 원격 계산 중단과 전송 데이터 회수의 미보장은 F12와 무관하게 남는다.

### 3.2 지금 남은 필수

2026-09-14 변경을 모두 반영하면 출하 선행 조건으로 남는 것은 **둘**이다. F 정의는 [심층 리서치](research-2026-09-12-unattended-release-gates.md)의 F01~F23 표를 기준으로 하며 요약을 기준으로 삼지 않는다.

| 남은 필수 | 왜 남았나 | 무엇이면 닫히나 |
|---|---|---|
| F12 중 자식 결과·실패를 올바른 작업에 연결 | 판단표는 **전달 방식**(native가 중간 부모에게 보내는 직접 알림)은 필수가 아니라고 정했지만 결과 회수·실패 보존·중복 처리 방지는 필수로 유지했다. 기존 메인 중계 성공은 고정 공개 성공 fixture라 일반 실패·취소까지 완료로 확대하지 않는다 | 정상 성공, 자식 실패, 중복 알림, 오래된 요청 알림, 형제 혼입, 사용자 취소의 여섯 사례. 고정 공개 응답과 native로 먼저 본다 |
| F14 중 저장 기록이 남은 Workflow 정상 재개 | 일반 편집·Agent 사용에 필수는 아니나 기존 Workflow 지원과 장애 후 개발 지속 약속을 유지하려면 해결 대상으로 남는다 | 정상 접근 조건 확정과 실행 세대별 신원 연결. `nothing to resume`을 기록 전체 소실과 같은 뜻으로 쓰지 않는다 |

출하 선행 조건이 **아닌** 것은 다음과 같다. 전부 PASS가 아니라 제외·이관으로 기록한다.

| 항목 | 기록 |
|---|---|
| F22 동적 링크 실행 검사 | 범위밖(사용자 지시). 차단 이력 보존, 제품 경로 보호 유지 |
| 알려진 HTTP/TCP 안정성 | 범위밖(사용자 지시). FAIL 이력과 실사용 위험 보존 |
| 정상 인증 갱신 | 제외 — 사용자가 Codex CLI 로그인 기준으로 확인 |
| 기본 400K/320K 압축 발동 | 이관 — 사용자 실사용 검증 |
| 4h → 24h×3 → 72h 단계와 장기 시험 전용 실행 | 범위밖 |
| Workflow 기록 전체 소실 뒤 자동 복원 | 필수 아님 — native도 자동으로 다시 시작하지 않는다 |

## 4. 최신 증거

### 4.0 릴리즈 보완의 새 실행

2026-09-12 개발 작업 트리, Node 24.19.0 / native Claude 2.1.269 / Codex 0.154.0에서 실행했다. 최신 `src/test-*.mjs`는 23/23, exit 0, 17.87초다. `test-review-diff`는 기본 러너에서는 PATH 부재로 notRun이며, 검토한 Git 디렉터리만 PATH에 둔 별도 최소 환경 실행에서는 실제 통과했다. symlink 차단은 그대로다. `verification/test-manual-http-probe.mjs`는 88/88 통과했다.

실제 정상·실패·취소 왕복은 [릴리즈 검증](release-readiness.md)의 케이스 표를 기준으로 한다. 모델 4종의 low 왕복을 확인했지만 astra 최초 검사에서는 전달 전 `UPSTREAM_ERROR_EVENT`가 한 번 관측됐다. 이후 단독 성공을 그 원인의 수정 증거로 보지 않으며 최초 원인은 미확정이다. 원래 감독하 PASS·과거 3주기 관측과 이 새 실행을 섞지 않는다.

같은 구성의 새 `-p` Workflow 검사도 통과했다. 정확한 inline script로 Workflow 1회·TaskOutput 1회, 한 자식의 journal started/result 각 1개·`sum=5`, `workflow-result`·luna/low 라우팅과 부모 복귀·정리 9개 true를 확인했다. 일반 allow rule을 사용했고 human origin을 위장하거나 native disable 설정을 변경하지 않았다.

최종 로컬 빌드는 `e216ad8`이다. 원래 `D:/AIDEV/Clauduct`에 fast-forward로 반영한 뒤 `src` 회귀 23/23(18.15초)이 통과했다. ZIP의 68개 파일 무결성·재현성·공백 경로 설치·실제 stream-json 정상 종료, 배포본 러너 자기 검사와 HTTP 검사 88/88도 확인했다. 빌드 SHA256과 앞선 실제 Workflow·실패 복구 결과는 [릴리즈 검증](release-readiness.md)에 있다. 전체 사용자 hook/plugin·UI 및 기본 320K 압축의 미검증 범위는 유지한다.

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

2026-09-11 후반, 웹 검색 경로 구현 이후 기준선은 다음과 같다: `test-native-gateway`(61), `test-native`(47), `test-native-search`(8, 신규), `test-native-transport`(루프백 검색 왕복 포함), `test-client-version`(loopback 12), `verification/test-manual-http-probe`(88). 2026-09-12에 `src/test-*.mjs` 20개가 **20/20으로 통과**한다(러너 종료 코드 0).

그 전까지 `test-chat`과 `test-review-diff`를 "이 셸에서 자식 프로세스를 띄우지 못한다"로 적어 둔 것은 **오진이었다.** 실패 이름만 남기고 이유를 버리는 하네스 때문에 두 개의 다른 원인이 한 라벨로 묶여 있었다.

- `test-chat` 7건은 셸과 무관했다. 6건은 `runInteractive`가 상태 projection 실패에 그대로 무너지던 것이고 — 그중 하나는 자식을 아예 띄우지도 않는다 — 1건은 기본 effort를 `medium`으로 적은 낡은 기대값이었다. 원인을 고치자 27건 전부 통과한다.
- `test-review-diff`는 러너가 자식 env를 `SystemRoot`/`WINDIR`/`TEMP`/`TMP`로 비우면서 **PATH가 없어** git을 찾지 못하는 것이다. PATH가 있으면 통과하고, 러너 아래에서는 `notRun`으로 보고한다. 매번 빨간 채로 두면 아무도 읽지 않는 실패가 되고, 그것이 이 오진이 살아남은 방식이었다.

`verification/test-http-transport`와 `test-dotnet-http-transport`는 러너의 `--test-concurrency=1`이 프로브의 런타임 검사에 걸리는 별개 건이며 exec 인수 없이 직접 실행하면 통과한다.

아래는 그 이전 기준선 기록이다. 기준 11개 파일이 함께 통과했다: `test-native-gateway`(54), `test-launcher-native`, `test-native-protocol`, `test-native-transport`, `test-native`(47), `test-request-diagnostics`(69 / loopback 34), `test-upstream-failures`(84 / loopback 62), `test-unsupported-event-diagnostics`(61 / loopback 36), `test-cancel-snapshot`(6), `test-client-version`(loopback 12), `test-compact-policy`. 모두 `src/`의 `.mjs`다.

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

native 세션을 정상 종료하면 launcher가 `Clauduct 종료: <분류>`와 `CLAUDUCT_REQUEST_STATUS <JSON>`을 출력한다. 같은 JSON이 `/.clauduct-status/request-status.jsonl`에도 한 줄씩 덧붙으므로(무시 대상 디렉터리) 스크롤백을 뒤질 필요가 없다 — run-04 이전에는 stdout이 유일한 사본이었다. 셋째 줄 `CLAUDUCT_REQUEST_STATUS_FILE`이 그 경로이며 `none`이면 기록에 실패한 것이다. 이 출력을 그대로 수집한다. 세션 안에서 `node src/request-status.mjs`를 부르는 방식은 뒤이어 assistant 처리가 붙어 모델 요청 없는 수집이 아니므로 사용하지 않는다.

먼저 볼 값: `requestOutcome`, `lifetime.failed`, `lifetime.failuresByStage`, `lifetime.rejectedBeforeStart`, `lifetime.firstRejectedCategory`, `failureHistory.records[].failureCategory`, `unsupportedEventNames`, `unknownBetaNames`, `lifetime.unmappedAgentModels`, `cleanup`의 9개 항목, `clientExecutionPolicy.nonStreamingFallbackDisabled`.

`unsupportedEventNames`에 값이 있으면 그것이 P0의 실제 이름이다. 세션 안에서 `request-status.mjs`로 조회하면 이 필드는 제거된 `null`로 나오므로, 반드시 종료 후 launcher 출력에서 확인한다.

첫 오류에서 중단하고 정상 종료 JSON을 수집한다. 실패한 작업을 자동으로 반복하지 않는다.

### 5.3 자동 압축 저비용 확인

정상 세션과 구분되는 새 실행에서만 사용한다.

```powershell
D:\AIDEV\Clauduct\clauduct.cmd --model luna --effort max --verify-auto-compact
```

이 절차의 대상 메커니즘은 이미 실측됐다 — 위 관측 기록의 축소 창 자동 압축 4건이다. 새 실행이 더하는 것은 **현재 400K 설정에서의 재확인뿐**이며 `CLAUDE_CODE_AUTO_COMPACT_WINDOW`를 세우는 코드 경로(`src/clauduct.mjs`)는 그때와 같다. 돌릴지는 그 값어치로 판단한다.

`/context`에서 400K 모델 창을, 진단에서 `autoCompactWindow=100000`을 확인한다. 목표는 기본 출력 예약량에서 약 67.4K 발동이다. `/autocompact` 값 지정으로 전역 설정을 바꾸지 않는다. 자동 압축이 비활성화돼 있으면 덮어쓰지 않고 그 조건을 보고한다. transcript의 `compact_boundary`에서 `trigger=auto`, 압축 후 토큰 감소, 후속 요청 성공으로 판정한다. 메인의 성공을 모든 자식의 압축 성공이나 backend 용량 수락으로 확대하지 않는다.

### 5.4 무인 3주기 — 선행 조건과 실행 조건

선행 조건 3개는 2026-09-11에 사용자 결정으로 모두 닫혔다.

| 조건 | 처리 |
|---|---|
| 1. 최초 `UNSUPPORTED_EVENT`의 원인 확정 **또는** 재발 시 4속성 확인 | **닫음.** 세션 46b6af24에서 다른 실패(`UNSUPPORTED_BETA`)로 고정 이름·안전 중단·데이터 보존·도구 미중복이 실제 관측됐고, 재발 시 이름을 잡는 `unsupportedEventNames`가 들어가 있다. 원인 자체는 여전히 미확정이며 이 판정이 그것을 확정하지 않는다 |
| 2. 시간·비용·동시성·중단 조건과 실행 주체 | **닫음.** 시간 상한 없음. guard·권한·사용량·인증 거부 중 하나라도 나오면 즉시 SAFE-STOP. 사용자가 새 세션을 시작하고 프롬프트가 1회 전달되며 그 뒤 회신하지 않는다 |
| 3. 기능 3개가 의존성·외부 서비스 없이 의미 있는 로컬 작업 | **닫음.** JSONL 로그 집계 CLI 3플랜, `verification/dev-sandbox/run-03` |

실행 프롬프트는 `docs/prompts/2026-09-11-session-25-sdd-three-cycle-run-03.md`다. 사용자가 세션을 시작하고 그 프롬프트를 **한 번만** 전달한다. 이후 그 세션이 질문을 보내도 답하지 않는다. 답하면 그 실행은 회복 시험이며 무인 완주가 아니다.

run-01과 run-02가 깨진 지점은 같다. 테스트 러너가 guard에 거부된 뒤 중단하지 않고 계속했다. 새 프롬프트는 `src/run-node-tests.ps1`을 처음부터 지정해 그 거부가 발생할 상황 자체를 없앤다. 이것은 guard 우회가 아니라 위반하지 않는 방법이다.

run-04(2026-09-11)가 **THREE-CYCLE-PASS**다. 3개 플랜이 실제 테스트·로컬 커밋까지 완료됐고, 회차마다 구현 자식 1명과 순차 검토 자식 2명이 실제로 실행됐으며(총 9명), 최초 전달 이후 개입이 없었다. 검토 자식이 구체적 결함을 반환해 수정 회차로 이어진 기록이 남아 있어 형식적 검토가 아니었다. 판정은 보고가 아니라 커밋·작업 트리·독립 러너 재실행(63/63 pass)·transcript 대조로 확인했다.

run-03(2026-09-11)에서 그 의도는 확인됐다. `test-node-runner-cap`은 등장하지 않았고 세션은 거부 직후 다른 셸로 재현하지 않고 멈췄다 — 절차 규칙이 처음 지켜졌다. 다만 최초 확인 1단계가 reparse point 판별의 수단을 열어 둔 탓에 세션이 `pwsh -Command`를 골랐고 `bash-nested-shell`에 거부됐다. 공유 guard에는 `pwsh -File` 예외가 있어 지정된 러너 줄 자체는 허용되는 형태였으나 거기 도달하지 못했다. run-04 프롬프트는 최초 확인 명령을 직접 적고 허용되는 pwsh 형태를 `-File` 하나로 고정했다. 함께 러너의 빈 루트 오분류(`TEST_RUNNER_FAILED` → `TEST_FILE_NOT_FOUND`)도 고쳤다. run-04의 `VERIFICATION.md`는 회차마다 `TEST_RUNNER_FAILED`나 guard 거부가 없었음을 명시한다.

SDD 범위는 사용자 결정으로 **자식 3종 필수**다. 회차마다 구현 자식 1명, 독립 명세 검토 자식 1명, 독립 품질 검토 자식 1명이 실제로 실행되어야 한다. 자식 없이 메인이 전부 수행하면 3주기를 마쳐도 THREE-CYCLE-PASS가 아니다. session-14 프롬프트의 "서브에이전트는 필수가 아니다"는 그 세션의 조건이며 현재 기준이 아니다.

각 주기에 계획, 새 구현(TDD면 RED→GREEN 증거), 실제 독립 리뷰 완료, 전체 회귀, 로컬 커밋을 남긴다. 외부 장애·권한 거부·사용자 개입 뒤 이어서 완료한 것은 회복 시험이며 무개입 PASS가 아니다. 계획된 RED assertion 실패는 예상된 개발 증거이며 API·환경 오류와 분리해 기록한다. 3주기 통과는 무제한·수시간 운영 안정성의 보증이 아니다.

## 6. 이력 자료

아래는 과거 검증 기록이다. **수치와 프롬프트는 당시 값이며 현재 설정이 아니다.** 재실행 지시가 아니다.

- 이관 기준과 호출 경로 조사: [호출 경로](native-call-paths-2026-09-09.md), [초기 감사](audit-2026-09-08.md). 당시 기준 실행 파일은 Claude 2.1.266이었고 이후 2.1.267 실행이 관측됐다. 동적 전체 기능 목록은 미완료로 남아 있다.
- 진단 확장 경과: [완료 이후 SSE](audit-2026-09-09-completion-diagnostics.md), [요청 상관](audit-2026-09-09-request-correlation.md), [lifetime](audit-2026-09-09-lifetime-diagnostics.md), [upstream 오류 상세](audit-2026-09-10-upstream-error-details.md), [실패 보존](audit-2026-09-10-upstream-failure-preservation.md), [요청 형식](audit-2026-09-11-request-shape.md), [fallback과 실패 이력](audit-2026-09-11-native-fallback-failure-history.md), [테스트 실행기](audit-2026-09-10-request-diagnostics-test-runner.md).
- 모델·역할·Workflow 실제 성공: [Plan 역할](audit-2026-09-09-plan-lifetime-success.md), [완료 성공](audit-2026-09-09-completion-success.md), [직접 부모](audit-2026-09-10-direct-parent-success.md), [병렬 Workflow](audit-2026-09-10-workflow-parallel-success.md), [/btw 경로](audit-2026-09-10-native-btw-path.md), [away 요약](audit-2026-09-10-away-summary.md), [세션20 취소 snapshot](audit-2026-09-10-session20-cancel-snapshot.md).
- 압축 실측(과거 500K / 83.33% 설정): f236f867 request 11에서 admission 0.15ms, 첫 이벤트 2212.41ms, 첫 텍스트 10776.14ms, 완료 64177.91ms, 재시도 없음. 922e6ba0에서 수동 압축 경계 뒤 새 Read 성공. b6d81841에서 검증 모드 `autoCompactWindow=100000` 자동 압축 68608ms(98097→39330)와 후속 요청 성공.
- 축소 창 자동 압축 전수(2026-09-11 transcript 재조사, 2026-09-08 기록): 같은 검증 모드에서 `trigger=auto` 경계가 **메인 3건·자식 1건** 있다. 6b4e6224(`gpt-5.6-luna`) 101740→56090과 69249→38457, b6d81841 98097→39330, 그리고 6b4e6224의 sidechain 자식 `a22a7821c3ca7295c` 86170→44712. 네 경계 모두 뒤에 assistant 턴이 이어졌고 `is_error` tool_result는 0건이다. 69249는 목표 발동점 약 67.4K에 가장 근접한 관측이며, 나머지가 더 높은 것은 한 턴이 임계를 크게 넘겨 채운 경우다. 창 값은 당시 설정인 `window=500000`이고 `autoCompactWindow=100000`은 `request-status.mjs`의 `clientContextPolicy` tool_result로 확인했다 — 문서 텍스트가 아니라 실행 진단이다. 이 관측은 축소 창의 발동이며 현재 기본값 400K/320K 발동을 증명하지 않는다.
- 리뷰·병렬 실행: [native high-02](audit-2026-09-09-native-high-02.md), [native high-03](audit-2026-09-09-native-high-03.md). low와 high 최종 반환은 확인했고 모든 review 수준은 확인하지 않았다.
- 세션별 실행 프롬프트는 `docs/prompts/`에 uncommitted 이력으로 남아 있다. session-13~23은 절차 기록이며 그 전제(미커밋 상태 등)를 현재 상태에 적용하지 않는다.
