# session-19 API 오류와 직접 HTTPS 경로 검수

## 실제 확인한 범위

18f826b4-f00e-40fd-8fbb-f994cb87d585 세션은 session-19 문서와 실행기를 읽고 초기 Git 상태·파일 해시 확인까지 실행했다. 2026-09-10T08:43:52Z에 UPSTREAM_ERROR_EVENT event=error가 표시됐다. 이번 세션의 전체 테스트·검토 자식 생성·커밋은 실행되지 않았다. run-02의 이전 구현과 테스트 파일은 보존했다.

이는 transport가 수신한 error 이벤트를 고정 분류로 거부하는 경로다. 화면의 502는 Clauduct가 downstream에 생성하는 HTTP 오류이며 upstream HTTP 502를 받았다는 증거가 아니다. 현재 원본 세션에는 upstream error.code/message가 없다. 이전 버전은 그 값을 폐기했으므로 지금 소스를 고쳐도 과거 실패의 세부 코드를 복원할 수 없다. 실제 원인을 rate limit, 모델, 컨텍스트, 암호화 reasoning, 서버 장애 중 하나로 단정하지 않는다.

## 커밋·검증 감사

- fb13dc8: 버전 pin을 reference/unverified 정책으로 분리하고 실제 버전 전달. backend 전반 호환성 보장은 아님.
- dcdb3fa: 최초 실패 이벤트 보존 코드, test-upstream-failures.mjs, 검증 기록이 커밋돼 있다. 테스트 누락/미커밋으로 해당 코드가 배포되지 않았다는 주장은 사실과 다르다. 이번 고정 오류 이름도 수정된 경로와 일치한다.
- dcdb3fa의 51개 검사는 실패 이벤트 뒤 다른 이벤트가 와도 오류 보존·중단·비노출이 유지되는지 검증했다. 실제 backend의 실패 원인이나 정상 요청 완주를 검증하지 않았다. detail code를 기록하지 않은 진단 누락은 남아 있었다.
- 3903ad2: astra/low 메인 시작과 400K/320K 목표 설정. 당시 launcher/protocol/compact 검증을 수행했고 전체 native 통합 검사는 미실행으로 보고됐다. 이번에는 전체 native 합성 통합 검사를 실행했다.

## 구조 확인

src/clauduct.mjs의 main → openUserTransport → createNativeTransport → node:https 요청으로 이어진다. 목적지는 poc/adapter.mjs의 https://chatgpt.com/backend-api/codex/responses다. verification/manual-http-probe.mjs의 buildHeaders는 Codex 인증 정보와 CLI 버전 헤더를 구성한다. codex.exe는 이 진입점에서 --version 조회에 사용되며 codex app-server의 thread/start 또는 turn/start에 위임하지 않는다.

따라서 현재 구조는 Codex app-server 통합이 아니라 Claude Code 메시지와 backend Responses SSE 사이의 직접 변환기다. 인증 정보를 공유해도 Codex 앱의 요청 구성·세션 상태·오류 복구를 공유하는 것은 아니다.

공식 [Codex App Server 문서](https://learn.chatgpt.com/docs/app-server)는 별도의 양방향 JSON-RPC 인터페이스, initialize/thread/start/turn/start, stdio 등의 transport를 설명한다. dynamicTools는 experimental이다. app-server 전환은 단순 endpoint 교체가 아니며 Claude 도구의 실행 소유권, 승인/취소, transcript, 모델 상속 연결을 새로 검증해야 한다. 문서가 app-server 명령과 WebSocket의 실험적 상태를 명시하므로 이를 무조건적인 안정성 해결책으로 주장하지 않는다.

## 이번 수정

최초 실패를 즉시 거부하는 정책은 유지하면서 구조화된 고정 진단을 추가했다.

- error.code 또는 error 이벤트의 nested error.code, response.failed/response.incomplete의 response.error.code를 제한된 UPSTREAM_ERROR_CODES 목록으로 분류한다.
- nested error.type은 별도 UPSTREAM_ERROR_TYPES 목록으로 분류한다. top-level code가 있으면 null인 경우도 이를 우선하며 nested code로 바꾸지 않는다.
- response.incomplete의 incomplete_details.reason을 제한된 UPSTREAM_INCOMPLETE_REASONS 목록으로 분류한다.
- upstreamErrorCode/upstreamErrorType/upstreamIncompleteReason을 프로토콜 sticky 오류와 gateway/status에 보존한다. 화면에도 upstream_code/upstream_type/incomplete_reason 고정 라벨을 표시한다.
- 필드 부재는 null, 모르는 값/잘못된 타입은 OTHER다. 임의 message/param/type/code 문자열과 response 원문은 기록하지 않는다. code allowlist는 진단 어휘이며 전체 backend error enum 또는 retry 가능성 목록이 아니다.
- 알려지지 않은 오류를 재시도하거나 성공으로 바꾸지 않는다. 기존 sequence·완료 후 프레임 거부·취소·정리·retry 제한을 유지한다.

공식 [Responses streaming events](https://developers.openai.com/api/reference/resources/responses/streaming-events)는 error.code와 response.failed의 response.error.code, response.incomplete의 incomplete_details를 구분한다. 공개 Responses 문서와 ChatGPT backend의 완전한 동일성은 검증하지 않았다. 이 때문에 top-level/nested 형태를 합성으로 검증하고 미지의 값은 미확인으로 남긴다.

## 이번 실행 증거

- test-upstream-failures.mjs: 84 checks / 62 loopback requests 통과. top-level/nested 코드와 precedence, nested type, failed/incomplete 세부 분류, unknown/missing/악성 객체, 부분 출력 전후 HTTP/SSE 오류, 후속 오류가 최초 분류를 덮지 않는지 검사.
- test-native-gateway.mjs: 43 passed. category와 맞지 않는 진단 및 악성 값 제거 포함.
- test-native.mjs: 45 passed / 0 failed. 기존 PowerShell 실행기로 제한된 환경에서 실행했다. 실제 hook 프로세스, 병렬 synthetic 요청, 1000회 누적 요청 검사 포함. 실제 Claude 실행/인증 읽기/외부 요청은 0.
- test-native-protocol.mjs, test-native-transport.mjs, test-launcher-native.mjs, test-compact-policy.mjs: 통과.
- test-request-diagnostics.mjs: 24 passed.
- test-client-version.mjs: 통과, loopback 12 requests.

검증은 오류 처리와 로컬 회귀에 한정된다. 이번 실제 세션 오류의 세부 원인, 실제 backend 성공, app-server 전환, 장기 무인 개발은 미검증이다. 실제 authenticated/native 실행은 하지 않았다. 개발 시험을 성공으로 기록하거나 run-02의 미커밋 산출물을 변경하지 않았다.

독립 서브에이전트가 기존 커밋과 테스트 범위를 읽기 전용 검토했다. 제안된 nested type 누락과 top-level/nested precedence 검증을 보완했다. 공개 오류에 허용한 것은 닫힌 목록의 고정 진단 라벨뿐이며, arbitrary raw code/type 노출을 허용하지 않는다. 이는 외부 서비스가 제공한 메시지 전체를 그대로 전달하는 것과 다르다.

## 남은 결정과 중단 기준

실제 원인 해결 완료는 최소한 새 실패의 allowlisted code 확인 → 그 원인을 겨냥한 수정 또는 외부 조건 확인 → 동일 경로 실제 성공 확인이 있어야 선언한다. 새 진단도 OTHER/null이면 원인 미확인이다. 원문 전체 수집·인증 노출·무제한 재시도는 대안이 아니다. 현재 프로세스에 새 코드는 소급 적용되지 않는다.

사용자가 기대한 구조가 Codex app-server라면 먼저 전환 범위를 확정하고 별도 호환성 설계를 해야 한다. 직접 HTTPS 구조에서 테스트를 더 통과시켰다는 이유로 app-server 통합 검증이 끝났다고 보고하지 않는다. 외부 오류가 절대 발생하지 않는다는 보장은 할 수 없으며, 실패를 감추는 방식으로 그 요구를 충족하지 않는다.
