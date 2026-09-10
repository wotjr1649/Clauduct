# 미지원 이벤트 안전 진단

## 진단 보완 후 최소 읽기 시험: PASS

사용자가 제공한 session 878f5eda-0661-4151-8203-ccb1bb3c5ac3의 종료 JSON으로 판정했다. 이번 기록에서 transcript 원문을 별도 확인하거나 실제 세션을 실행하지 않았다.

- 실제 model/requestedModel gpt-5.6-luna, effort/requestedEffort max. 요청 4/5/6 총 3건 모두 HTTP 200/completed, 실패·재시도 0건.
- UNSUPPORTED_EVENT 및 요청 형식 오류 미관측. 새 unsupportedEventTypeFormat 필드는 출력됐으며 오류가 없어 null이다.
- Clauduct 종료 SUCCESS, cleanup 9개 항목 모두 true.
- clientVersion 0.154.0 / reference 0.153.4 / unverified. context window/autoCompactWindow 400000, compactPercent 84.21052631578947은 설정 증거이며 실제 압축 발동 증거가 아니다.

판정은 luna/max 최소 읽기·정상 종료 PASS다. 과거 sol/low 실패의 동일 조건 재현, 오류 원인 해결, 새 진단의 실제 실패 분기, 계정 신원, SDD 완주는 미검증이다. 같은 최소 시험을 반복하지 않고 기존 session-22의 제한된 독립 리뷰 시험으로 진행한다. 첫 오류에서 중단하고 정상 종료 JSON을 수집하는 규칙을 유지한다.

재개 준비 시 run-02 HEAD 298a5332e8f4f698002e236b020adf0d803f1562 및 tracked 수정 네 파일/untracked test/untag.test.mjs 상태를 재확인했다. 개발 파일 수정이나 테스트 재실행은 하지 않았다. 기존 session-22 프롬프트는 변경·커밋하지 않고 그대로 제공한다.

## 실제 실패 순서

사용자가 제공한 session 1433a5dd-3a0a-4840-ab13-61abb4d410eb 종료 JSON에서 요청 4는 성공, 요청 5는 약 57.6초 뒤 UNSUPPORTED_EVENT/other, 요청 6은 prepare 단계 REQUEST_SHAPE였다. 요청 5는 HTTP 200이나 completed=false, 재시도 없음이다. 요청 6은 upstream 시도 없음이다. 총 3건 중 1건 성공/2건 실패이며 종료 SUCCESS와 cleanup 9개 true는 작업 성공을 의미하지 않는다.

후속 요청이 native fallback인지, 정확한 미지원 이벤트명과 계정 변경의 영향은 미확인이다. 기존 other를 소급 해석하지 않는다.

## 신뢰 경계와 수정

upstream event.type도 신뢰할 수 없는 문자열이다. 이름처럼 보이는 문자열에 인증·개인정보·지시문이 포함될 수 있어 정규식만 통과한 원문을 공개하거나 해시로 남기지 않는다. 알려진 고정 라벨만 출력한다.

- 기존 진단 전용 allowlist에 프로젝트의 downstream 프로토콜에서 사용하는 message_start/message_delta/message_stop/content_block_start/content_block_delta/content_block_stop을 추가한다. upstream 지원을 추가한 것이 아니다. 해당 이벤트는 여전히 UNSUPPORTED_EVENT로 거부된다.
- 알 수 없는 codex.*와 responsesapi.*는 각각 unknown-codex-event/unknown-websocket-event로 분류한다. 후자는 이름 prefix 분류일 뿐 실제 WebSocket transport 증명이 아니다. 기존 response.* 분류도 유지한다.
- unsupportedEventTypeFormat을 추가한다: missing/non-string/empty/oversized/identifier/other. oversized는 문자열 길이 128 초과, identifier는 소문자 ASCII 영숫자·underscore의 dot 구분 형식이다. 원문 이름과 본문은 보존하지 않는다.
- parser → gateway 502 오류/메모리 진단 → request-status projection → 기존 launcher 종료 JSON 경로를 재사용한다. 알 수 없는 진단 라벨은 projection에서 null로 제한한다.

이 변경으로 namespace와 malformed type, 알려진 다른 프로토콜의 이벤트를 구분할 수 있다. 임의의 새 이벤트 정확한 이름까지 복구할 수는 없다. 여전히 other/identifier이면 추가 근거 없이 수용하거나 정상 처리로 무시하지 않는다.

## 검증

Invoke-ClauductNodeTests(60초 제한)로 src 아래 test-unsupported-event-diagnostics.mjs, test-native-protocol.mjs, test-native-gateway.mjs, test-native-transport.mjs, test-native.mjs, test-request-diagnostics.mjs, test-upstream-failures.mjs, test-cancel-snapshot.mjs의 8개 파일이 모두 통과했다.

새 suite는 29개 검사/13개 실제 loopback HTTP·SSE 왕복으로 고정 진단 보존, 최초 실패 유지, 후속 completion에 의한 성공 덮어쓰기 방지, 재시도 0, 소켓 정리, 임의 이름·본문 미노출을 확인했다. null/누락 type은 parser 단위에서 검사하고 transport의 기존 조기 거부 기준은 변경하지 않았다. 기존 정상 응답 및 upstream terminal 실패 회귀도 통과했다.

외부 추론 요청·실제 인증 조회·Claude 실행은 하지 않았다. 실제 원인 해결이나 native 호환성 PASS로 표시하지 않는다. 제품 런타임 진단만 수정했으며 계정 정책, 요청 수용 기준, retry 정책, run-02 산출물은 변경하지 않았다. 다음 실제 관측은 새 프로세스에서 최소 읽기 후 정상 종료 JSON을 수집하며 SDD 전체 재시험과 분리한다.

## 설치 Codex 바이너리의 ThreadEvent 후보 — 2026-09-11

`other`/`identifier`는 소문자 dot 식별자이면서 `response.`/`codex.`/`responsesapi.` 어느 namespace도 아닌 이름이라는 뜻이다. 이 조건에 맞는 실제 어휘를 찾기 위해 설치된 `C:/Users/JS/AppData/Local/Programs/OpenAI/Codex/bin/codex.exe`를 읽기 전용으로 문자열 검색했다. 실행·수정·인증 조회는 하지 않았고 `auth.json`도 읽지 않았다.

고정 serde 태그 테이블에서 다음 연속 문자열을 확인했다: `ThreadEvent` `ThreadStarted` `type` `thread.started` `TurnStarted` `turn.started` `TurnCompleted` `turn.completed` `TurnFailed` `turn.failed` `ItemStarted` `item.started` `ItemUpdated` `item.updated` `ItemCompleted` `item.completed`. 일곱 태그 모두 위 조건을 만족하므로 관측된 진단 서명과 일치한다.

한계: 이 enum이 설치 바이너리에 존재한다는 사실만 확인했다. ChatGPT `backend-api/codex/responses` 응답이 실제로 이 이벤트를 보냈는지, 과거 요청 57의 이름이 이 중 하나였는지는 확인하지 못했다. 바이너리에 `backend-api/codex/responses` 리터럴은 없었고 경로는 조합된다. 따라서 이것은 좁혀진 후보이지 원인 확정이 아니다. `codex.thread.started`는 같은 영역의 telemetry span 이름이며 SSE 이벤트 증거로 쓰지 않았다.

## 진단 보완 2

- 위 일곱 태그를 진단 전용 allowlist에 추가한다. upstream 지원을 추가한 것이 아니며 여전히 `UNSUPPORTED_EVENT`로 거부한다. 재현 시 정확한 고정 이름이 그대로 기록된다.
- 알 수 없는 `thread.`/`turn.`/`item.` 이름은 `unknown-thread-event`로 분류한다. namespace 분류일 뿐 원문 이름·본문은 보존하지 않는다.
- 접두사가 없는 `threadprivate_event` 같은 이름은 계속 `other`다.

검증: `Invoke-ClauductNodeTests`(60초 제한, Node.js v24.19.0)로 `src/test-unsupported-event-diagnostics.mjs`가 29 → 51개 검사, loopback 왕복 13 → 24회로 통과했다. 새 검사는 일곱 고정 이름의 자기 이름 보고, 세 namespace fallback, 접두사 없는 이름의 `other` 유지, 임의 이름·본문 미노출을 확인한다. `test-native-protocol`, `test-native-transport`, `test-native-gateway`도 함께 통과했다. 외부 추론 요청·실제 인증 조회·실제 Claude 실행은 0이다.
