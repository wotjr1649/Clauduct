# 미지원 이벤트 안전 진단

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
