# 경계 거부와 OTHER 분류 축소

## 관측한 공백

2026-09-11 로컬 재현에서 gateway는 요청 기록을 만들기 전에 거부한 요청을 어느 진단에도 남기지 않았다. 합성 loopback에서 성공 1건, `anthropic-beta` 미지원 400 1건, 미지원 경로 400 1건을 보낸 뒤 상태는 `requestOutcome=all-succeeded`, `lifetime.started=1/succeeded=1/failed=0`, `failureHistory.records=[]`였다. 즉 native가 새 beta나 다른 버전 헤더를 보내 모든 요청이 400이 되어도 종료 JSON은 실패 없는 실행처럼 보인다. 이는 진단 공백이며 과거 세션의 실패 원인을 소급 확정하는 근거가 아니다.

두 번째 공백은 분류 어휘다. `FAILURE_DIAGNOSTIC_CATEGORIES`에 없는 로컬 고정 코드는 gateway에서 `OTHER`가 된다. 소스 대조에서 요청 기록에 도달할 수 있는데 목록에 없던 코드는 `REQUEST_TIMEOUT`, `INPUT_TOO_LARGE`, `INVALID_JSON`, `MEMORY_QUEUE_FULL`, prepare 단계의 `UNSUPPORTED_TOOLS`/`UNSUPPORTED_MESSAGES`/`UNSUPPORTED_IMAGE`/`UNSUPPORTED_SAMPLING`/`INVALID_OUTPUT_LIMIT`/`INVALID_TOOL_RESULT`/`MISSING_TOOL_RESULT`/`INVALID_TOOL_REFERENCE`/`UNSUPPORTED_CONTEXT_EDIT`/`UNSUPPORTED_DEFERRED_TOOLS`/`UNSUPPORTED_TOOL_CHANGE`, review 단계의 `REVIEW_DIFF_UNAVAILABLE`, upstream 단계의 `RETRY_AFTER_OUTPUT`, selection 단계의 `AGENT_SELECTION_UNVERIFIED_<이유>`였다. 실패가 `OTHER`로 보이는 것이 곧 원인 미상은 아니었다.

## 변경

1. `/v1/messages` 요청의 진단 기록을 `anthropic-version`/`anthropic-beta`/`content-type` 검사보다 먼저 만든다. 거부 조건·HTTP 상태·upstream 미시도는 그대로다. 세션/agent 식별자 형식 검사는 기록보다 앞에 남겨 참조값을 검증된 헤더에서만 만든다.
2. `lifetime.rejectedBeforeStart`와 `lifetime.firstRejectedCategory`를 추가한다. 요청 기록이 없는 HTTP 경계 거부만 센다. 모델 요청 성패 카운터, `failureHistory`, `requestOutcome`의 의미는 바꾸지 않았다. 서버 수준 clientError/CONNECT/upgrade는 포함하지 않는다.
3. `FAILURE_DIAGNOSTIC_CATEGORIES`에 위 고정 코드와 HTTP 경계 코드(`UNSUPPORTED_VERSION`, `UNSUPPORTED_BETA`, `INVALID_BETA_HEADER`, `UNSUPPORTED_ROUTE`, `INVALID_HEADER`, `LOCAL_BOUNDARY_REJECTED`, `UNEXPECTED_CREDENTIAL_SOURCE`, `INVALID_SESSION_ID`, `INVALID_AGENT_BINDING`, `AGENT_BINDING_CONFLICT`)를 추가한다. 모두 이 저장소가 만든 고정 라벨이며 upstream 문자열을 복사하지 않는다.
4. 접미사가 붙는 로컬 코드는 고정 접두사만 기록한다. `AGENT_SELECTION_UNVERIFIED_<이유>` → `AGENT_SELECTION_UNVERIFIED`, `UNSUPPORTED_BETA known=... unknown=N` → `UNSUPPORTED_BETA`. 접두사가 일치하지 않으면 계속 `OTHER`다. HTTP 오류 메시지의 상세 문구와 `selectionFailure`는 바꾸지 않았다.

## 재시도 울타리 회귀

downstream에 내용이 전달된 뒤 재시도 가능한 truncation이 와도 upstream에 재전송하지 않는지 검사한다. 같은 loopback upstream에서 `response.created`만 보내고 끊으면 1회 재시도로 성공하고 `message_start`와 `tool_use`가 각각 1회만 전달된다. 텍스트 delta까지 보내고 끊으면 시도 1회로 끝나고 원래 실패(`TRUNCATED_STREAM`/`UPSTREAM_IO_ERROR`)가 보존되며 `message_stop`과 `tool_use`는 전달되지 않는다. gateway의 `canRetry`를 제거하면 이 검사가 실패한다(원래 실패 대신 `RETRY_AFTER_OUTPUT`)는 것을 확인했고 즉시 복원했다. 이는 중복 도구 실행 방지의 로컬 증거이며 native 클라이언트 내부의 비스트리밍 전환을 실증한 것은 아니다.

## 검증

2026-09-11, Node.js v24.19.0, 프로젝트 `Invoke-ClauductNodeTests`(60초 제한, test concurrency 1)로 다음을 실행해 모두 통과했다.

- 기준 11개: `test-native-gateway`, `test-launcher-native`, `test-native-protocol`, `test-native-transport`, `test-native`, `test-request-diagnostics`, `test-upstream-failures`, `test-unsupported-event-diagnostics`, `test-cancel-snapshot`, `test-client-version`, `test-compact-policy` (모두 src/, `.mjs`).
- 선택·완료 표면 4개: `test-agent-selection`, `test-completion-selection`, `test-workflow-selection`, `test-request-admission`.

gateway suite는 46 → 49개 검사다. 새 검사는 (a) 고정 분류 전수 왕복에 접미사 코드 4종을 더해 정규화와 접미사 미노출을 확인하고, (b) 버전/beta/중복 beta/content-type 거부가 `failureStage=request`, 고정 `failureCategory`, `failureHistory` 기록, `requestOutcome=has-failures`, upstream 시도 0으로 남는지 확인하며, (c) 미지원 경로와 잘못된 agent 식별자가 `rejectedBeforeStart=2`, `firstRejectedCategory=UNSUPPORTED_ROUTE`로만 집계되고 요청 카운터를 건드리지 않는지 확인하고, (d) 위 재시도 울타리 두 경우를 확인한다.

외부 추론 요청·실제 인증 조회·실제 Claude 실행은 0이다. Verified는 로컬 합성/loopback 범위다. 최초 `UNSUPPORTED_EVENT other/identifier`의 원인, 실제 native의 비스트리밍 전환 차단, 실제 세션에서의 새 필드 출력은 Not verified다.
