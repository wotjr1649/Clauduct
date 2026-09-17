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


## 설치 Codex 바이너리 조사와 후보 격하 — 2026-09-11

`other`/`identifier`는 소문자 dot 식별자이면서 `response.`/`codex.`/`responsesapi.` 어느 namespace도 아닌 이름이라는 뜻이다. 설치된 `Codex/bin/codex.exe`를 읽기 전용으로 문자열 검색했다. 실행·수정·인증 조회는 하지 않았고 인증 저장 파일도 읽지 않았다.

고정 serde 태그 테이블에서 `ThreadEvent`의 일곱 태그(`thread.started`, `turn.started`, `turn.completed`, `turn.failed`, `item.started`, `item.updated`, `item.completed`)를 확인했다. 형태는 관측된 서명과 맞는다.

**그러나 후속 공개 문서 조사에서 이 어휘의 계층이 다르다는 것이 확인됐다.** 이 태그들은 Codex SDK의 `runStreamed()`와 Codex app-server가 내보내는 이벤트다([SDK README](https://github.com/openai/codex/blob/main/sdk/typescript/README.md), [app-server 문서](https://learn.chatgpt.com/docs/app-server)). Clauduct는 app-server를 쓰지 않고 직접 HTTPS로 responses 엔드포인트에 붙으므로 이 어휘가 해당 와이어에 나타날 근거는 없다. 따라서 **원인 후보로는 격하한다.** 진단 라벨은 비용이 없고 오분류를 막아 주므로 유지한다.

같은 조사에서 더 중요한 사실을 확인했다. 설치된 codex의 standalone 패키지는 `0.154.0-x86_64-pc-windows-msvc`이며 이는 Clauduct가 전송하는 clientVersion과 같다. 즉 이 바이너리가 해당 와이어의 기준 클라이언트다. 그 responses SSE 이름표에서 확인된 것은 `response.*` 계열, `responsesapi.websocket_timing`, `codex.` namespace 항목뿐이며 Clauduct의 기존 allowlist와 사실상 같다.

결론: 문제의 이벤트는 **기준 클라이언트 0.154.0도 모르는 이름**이다. 오프라인 분석으로는 더 좁힐 수 없고, 재발 시 이름을 실제로 확보하지 않으면 원인은 확정되지 않는다.

## 진단 보완 2

- ThreadEvent 일곱 태그를 진단 전용 allowlist에 추가한다. upstream 지원 추가가 아니며 여전히 `UNSUPPORTED_EVENT`로 거부한다.
- 알 수 없는 `thread.`/`turn.`/`item.` 이름은 `unknown-thread-event`로 분류한다. 접두사가 없는 `threadprivate_event` 같은 이름은 계속 `other`다.

## 제한 캡처 정책 — 사용자 결정 2026-09-11

사용자가 grilling에서 "제한적 캡처 허용"과 "종료 JSON 전용"을 선택했다. 임의 원문 기록 금지 원칙은 유지하되 다음 형태 검사를 통과한 값만 예외로 둔다.

- 소문자로 시작하는 `[a-z][a-z0-9_]{0,23}` 세그먼트, 점 1개 이상 4개 이하, 전체 48자 이하.
- 이미 고정 라벨을 가진 이름은 캡처하지 않는다. 중복 없이 서로 다른 이름 최대 4개까지만 보관한다.
- 본문·해시·부분 값은 여전히 기록하지 않는다. 클라이언트에 돌려주는 오류 메시지에도 이름을 넣지 않는다.
- 노출면: launcher 종료 JSON의 `unsupportedEventNames`에만 나온다. 세션 내 `/clauduct/status`는 이 필드를 제거해 응답하므로 upstream 문자열이 모델 컨텍스트로 들어가는 경로가 생기지 않는다. 투영 결과의 `null`은 "이 경로가 캡처를 제공하지 않음", `[]`는 "캡처 가능하나 관측 없음"이다.

잔여 위험: 이 형태를 정확히 흉내 낸 문자열은 통과할 수 있다. 사용자가 이 위험을 인지하고 선택했다.

## 검증 2

`Invoke-ClauductNodeTests`(60초 제한, Node.js v24.19.0)로 `src/test-unsupported-event-diagnostics.mjs`가 29 → 61개 검사, loopback 왕복 13 → 36회로 통과했다. 새 검사는 일곱 고정 이름의 자기 이름 보고, 세 namespace fallback, 접두사 없는 이름의 `other` 유지, 형태 검사 경계(세그먼트 23자 통과 / 25자 거부, 세그먼트 6개 거부, 대문자 거부, 점 없음 거부), 중복 제거와 4개 상한, 상태 API의 필드 제거, 재투영 보존, 임의 본문 미노출을 확인한다.

기준 11개 파일과 선택·완료 표면 4개도 함께 통과했다. 외부 추론 요청·실제 인증 조회·실제 Claude 실행은 0이다.
