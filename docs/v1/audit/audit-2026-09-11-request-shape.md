# REQUEST_SHAPE 진단 분리

## 관측

세션 1433a5dd-3a0a-4840-ab13-61abb4d410eb의 로컬 transcript에서 Read 호출과 tool_result 이후 assistant의 API Error: 400 UNSUPPORTED_REQUEST request=REQUEST_SHAPE를 확인했다. 요청 원문은 보존된 transcript와 같다고 가정할 수 없으므로 실패 조건을 소급 확정하지 않는다.

기존 prepareNative 검사는 stream === true, messages 배열, messages 비어 있지 않음 중 하나라도 실패하면 같은 REQUEST_SHAPE를 반환했다. 계정 검사가 아니라 upstream 전 요청 변환 검사다.

## 변경

기존 허용 조건과 HTTP 400/UNSUPPORTED_REQUEST는 유지하고 requestFailure만 분리한다.

| 진단 | 의미 |
|---|---|
| REQUEST_STREAM_FALSE | stream이 false |
| REQUEST_STREAM_MISSING | stream이 없음 |
| REQUEST_STREAM_INVALID | stream이 true/false가 아닌 값 |
| REQUEST_MESSAGES_INVALID | messages가 없거나 배열이 아님 |
| REQUEST_MESSAGES_EMPTY | messages가 빈 배열 |

복수 조건 실패 시 stream, messages 형식, 빈 배열 순서로 첫 실패만 보고한다. 원문 값·본문·인증은 추가 수집하지 않는다. 기존 REQUEST_SHAPE는 과거 진단 projection 호환성을 위해 allowlist에 유지한다. gateway와 request-status가 공유하는 allowlist를 재사용한다.

## 계정 경계

poc/user-session.mjs는 C:/Users/JS/.codex/auth.json을 읽고 supplier 생성 후 처음 관측한 계정과 실행 중 계정의 일치를 요구한다. 새 프로세스는 새 supplier를 만든다. 특정 계정 ID의 영구 고정이 아니라 고정 인증 저장 위치 및 프로세스 내 계정 일관성 검사다. 실제 사용자 인증 파일은 이번 조사에서 읽지 않았다. 변경한 계정이 해당 파일에 반영됐는지는 미확인이다. 이 정책은 수정하지 않았다.

## 검증과 남은 작업

새 REQUEST_STREAM_FALSE를 기대한 테스트가 수정 전 REQUEST_SHAPE로 실패하는 RED를 확인했다. 수정 후 Invoke-ClauductNodeTests(60초 제한)로 test-request-diagnostics, test-native-protocol, test-native-gateway, test-native, test-cancel-snapshot, test-launcher-native, test-native-transport의 7개 파일이 모두 통과했다(모두 src/*.mjs).

request-diagnostics는 69개 검사/34개 loopback 요청에서 고정 오류 라벨, 상태 JSON 보존, prepare 단계, upstream 시도 0회, 민감 원문 미노출을 확인했다. 정상 입력 회귀와 계정 변경 거부 회귀도 기존 suite에서 통과했다. 외부 요청·실제 인증 조회·Claude 실행은 하지 않았다.

이 변경은 진단 보완이며 native 요청 호환성 해결이 아니다. SDD 전체 재시험에 앞서 기존 실패 프로세스가 남아 있다면 정상 종료 JSON을 수집한다. 새 세부 진단은 과거 세션에 소급되지 않는다. 실제 stream:false 등 새 증거가 생기기 전 자동 형식 변환이나 검증 완화를 하지 않는다. run-02 및 기존 프롬프트는 변경하지 않았다.
