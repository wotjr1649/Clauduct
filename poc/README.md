# Clauduct PoC — HTTP 게이트웨이와 프로토콜 변환 핵심

대화형 실행기 구현과 현재 제한은 [프로젝트 README](../README.md)를 참고하세요. 기존 PoC 실행 명령은 회귀용으로 유지합니다.

**현재: Claude→Codex 실제 Read 1회 PoC 완료.** 사용자 diagnosticVersion=3에서 SUCCESS, Read 호출/연결된 결과 각 1회, gateway/Claude 최종 marker 일치, 요청 2회, 정상 종료·자원 정리·fixture 제거를 확인했다. [실행 안내와 범위](./실제-Read-실행.md). 동일 검사를 반복할 필요는 없다. 아래는 이전 단계 기록이며 당시 미완료 표시는 현재 상태가 아니다.

최신: 사용자 v2 요청 구조를 반영한 [명시적 오프라인 입력 변환](../verification/최소-어댑터-PoC-명세.md)을 추가했다. 어댑터 296/296·게이트웨이 74/74·사용자 합성 세션 89/89·검사기 90/90 통과. v2 사용자 관찰은 받았으므로 아래 같은 검사 재실행 안내는 이전 기록이다. live 토큰 상한·beta·세션 전달과 실제 Read 왕복은 아직 미완료다.

최신: 사용자 v1에서 실제 Claude 요청 1회와 정리를 확인했다. 역할·옵션·Read schema 의미를 더 구분하는 **진단 v2**를 준비했고 검사기 90/90·실행 관리자 14/14를 통과했다. [v1 결과와 v2 실행 안내](./요청-검사기.md)를 따른다. 전체 실제 Read 왕복 성공은 아직 아니다.

현재 후속 단계: [Claude 요청 관찰용 실행 파일](./요청-검사기.md)을 준비했다. `node D:\AIDEV\Clauduct\poc\claude-inspection.mjs --inspect`로 사용자가 실제 Claude의 요청 구조를 정제해 관찰한다. 공개 marker만 사용하며 Codex 전송·실제 Read 검사가 아니다. 실행 관리 14/14·검사기 85/85 로컬 통과, 실제 Claude 관찰은 사용자 실행 대기다. 아래는 이전 단계별 기록이다.

2026-09-07 현재: **사용자 v5 실행에서 gpt-6-astra/low와 gpt-5.6-luna/low 모두 SUCCESS**다. 각 모델에서 요청/연결 2회, 도구 복원 1회, callIdMatches/exactMarker/resourcesClosed=true, 헤더 부재 호환 적용 2회를 보고했다. 기존 Codex OAuth를 사용하는 게이트웨이의 합성 도구 결과→정확한 최종 답변 왕복을 확인했다. 실제 도구 실행은 0회이며 실제 Claude 인터페이스·권한/hooks 통합 성공은 아니다. 동일 검사를 반복하지 않는다. 아래 갱신 문단은 이전 시점의 기록이다.

2026-09-07 현재: 사용자 v3 결과에서 reasoning 완료 1개·암호화 문자열 갱신 1회가 관찰됐고, 두 번째 응답의 다섯 번째 message item-added에서 **UNSUPPORTED_FIELDS**로 중단했다. 추가 필드의 이름은 아직 알 수 없다. 수용 조건을 바꾸지 않고 **diagnosticVersion=4**에 messagePhase/messageExtraFields 고정 분류를 추가했다. 변환기 **257/257**, 사용자 진입점 **73/73**, 게이트웨이 **72/72**를 로컬 검증했다. [진단 1회 실행과 판독 기준](./사용자-실행.md)을 따른다. 전체 계정 왕복은 실패 상태이며 아래 갱신 기록의 최신·현재 표기는 각 이전 단계 시점의 증거다.

현재 메시지 계약은 id/type/status/role/content와 선택적 phase를 검사한다. phase는 부재/null/final_answer를 허용하되, 반드시 item.done·정상 response.completed·텍스트/ID/usage·선택한 모델/effort 검사를 함께 통과해야 한다. commentary와 미지 phase는 UNSUPPORTED_MESSAGE_PHASE, 다른 추가 키는 UNSUPPORTED_FIELDS다. 여러 assistant 메시지의 phase 이력 보존과 commentary 전환은 아직 미지원이다. [phase 수용 기준과 남은 조건](../verification/최소-어댑터-PoC-명세.md#message-phase-지원-전-조건)을 따른다.

회귀용 변환기·전송 factory의 기본 astra-xhigh는 이전 합성 검사용으로 유지한다. 사용자 진입점은 low 두 선택만 허용하며, 선택을 생략해도 xhigh가 아니라 astra-low다. 이전 manual HTTP/.NET 비교 검사의 고정 모델·effort·성공 조건은 변경하지 않았다.

2026-09-07 최신: 사용자 v3 결과로 실패 조건이 **REASONING_INITIAL_ENCRYPTED**임을 확인했다. 시작/완료 암호화 문자열의 동일성을 요구한 Clauduct의 가정을 수정해 완료 snapshot의 값을 보존한다. 변환기 **233/233**, 사용자 진입점 **70/70**, 게이트웨이 **72/72**를 로컬에서 통과했다. 수정 후 실제 계정 전체 왕복은 아직 미검증이다. [현재 실행 안내](./사용자-실행.md)를 따르며 아래 원인 미확정 기록은 당시 상태로 보존한다.

2026-09-07 후속 결과: reasoning 지원 후 두 번째 응답의 네 번째 reasoning item-done에서 SNAPSHOT_MISMATCH가 발생했다. 기존 진단으로 응답 ID·reasoning ID·시작/완료 암호화 값의 어떤 비교가 실패했는지 확정할 수 없다. 이번에는 본문 수용 조건을 유지하고 diagnosticVersion=3의 고정 snapshotCheck를 추가했다. 변환기 217/217·사용자 진입점 66/66·게이트웨이 72/72를 로컬에서 통과했다. **충돌 원인과 실제 계정 왕복 성공은 미확정**이며 [새 진단과 1회 사용자 실행 안내](./사용자-실행.md)를 따른다. 아래는 이전 단계 기록이다.

2026-09-07 최신: 사용자 결과에서 첫 도구 복원과 합성 결과의 두 번째 요청까지 진행한 사실을 확인했다. 두 번째 응답의 status 없는 reasoning 항목을 거부하던 단일 출력 가정을 수정했다. 순차 reasoning 뒤 한 메시지/도구를 처리하고 후속 Codex 요청에 reasoning을 보존한다. 변환기 202/202·사용자 진입점 62/62·게이트웨이 72/72·요청 검사기 82/82를 로컬에서 통과했다. 수정 후 실제 계정 왕복은 사용자 결과 대기다. [현재 실행 안내](./사용자-실행.md)를 따르며 아래의 과거 숫자는 당시 기록이다.

최신 단계: [사용자 실행 진입점](./사용자-실행.md)을 구현했다. 기존 OAuth의 사용자 프로세스 메모리 연결과 최대 2회 합성 도구 프로토콜 검사를 준비했고, 진입점 44/44·게이트웨이 72/72를 검증했다. 실제 계정 경로는 사용자 실행 대기다. 이제 max_tokens가 없는 로컬 PoC 입력도 허용하며, 명시된 토큰 한도는 Codex 전송 전에 TOKEN_LIMIT_UNSUPPORTED로 거부한다. 아래의 max_output_tokens 대응은 오프라인 변환 후보로만 보존한다. 하류 backpressure와 기본 5초 전달·180초 수명도 별도 검증했다.

최신 구현은 [게이트웨이 안내서](./게이트웨이.md)에 있다. **Claude Code 인터페이스 → Clauduct 게이트웨이 → Codex OAuth 모델** 구조의 HTTP 수신·전송·변환 계층을 연결했다. 합성 HTTP 왕복 **67/67**, 실제 45초 제한 **45,015 ms / TIMEOUT**을 확인했다. 실제 OAuth·Claude 실행은 아직 없으며 아래는 재사용한 오프라인 핵심의 계약과 이전 검증 기록이다.

```powershell
node --permission --allow-fs-read=D:\AIDEV\Clauduct\poc --allow-fs-read=D:\AIDEV\Clauduct\verification\manual-http-probe.mjs .\poc\test-gateway.mjs
```

이번 게이트웨이 단계에서 핵심 100/100, 요청 검사기 82/82, 기존 연결 파서 77/77도 다시 통과했다. Claude OAuth 환경변수의 저장 위치는 사용자 보고로 확인됐으며 관련 설정 조사를 게이트웨이 구현의 선행 조건으로 두지 않는다. 로컬 난수 세션 인증·Host/Origin·세션 격리·HTTP 취소·upstream 최대 2회는 합성 통신으로 검증했다. 사용자 운영 인증 연결부와 실제 CLI로의 로컬 비밀 전달은 미통합이다.

## 이전 오프라인 핵심 구현 기록

```powershell
Set-Location 'D:\AIDEV\Clauduct'
node .\poc\test-adapter.mjs
```

2026-09-06: 합성 검사 **100/100 통과**. 새 외부 의존성·설치·HTTP 서버·로그인 없이 실행한다. `adapter.mjs`는 프로토콜 변환 핵심이며 실제 Claude Code와 연결되는 실행 제품은 아니다. 사용자가 승인한 제한적 헤더 정책과 오프라인 구현 범위에 해당한다.

후속 보완: 기본 45초 실제 만료 3종을 포함한 핵심 **103/103**, 기본 5초/60초 만료와 beta query 경계를 포함한 요청 검사기 **78/78**, 검사기 세션 63개 정리를 확인했다. `request-inspector.mjs`는 실제 Claude 요청을 정제해 확인하기 위한 유한 loopback 수신기이며 upstream 연결·도구 실행을 하지 않는다. 실제 Claude는 아직 실행하지 않았다. [검사기 실행·잔여 수용 조건](./요청-검사기.md)을 별도로 기록했다. 아래 1–5절은 변환 핵심의 계약이다.

최신 인증방식 확인 후 보완: 공개 marker의 Bearer 전달을 추가했고 **82/82·67개 세션**을 검증했다. 실제 Bearer/API key/Cookie/Proxy 인증과 marker의 혼합은 거부한다. 기존 Codex OAuth를 사용자 운영에서 메모리로만 사용하는 경로를 우선 설계하며 새 PKCE는 첫 PoC의 필수 조건에서 제외한다. 이번에는 변환 핵심과 실제 45초/60초 검사를 재실행하지 않았고 이전 증거를 유지한다.

## 1. 이번에 동작하는 흐름

`OfflineSession.prepare(JSON 문자열)` → Responses 요청 JSON → `begin(전송 메타데이터)` → `push(Uint8Array)` 반복 → `finish()` → Anthropic 메시지 및 SSE 문자열.

합성 도구 응답에서는 `function_call.call_id`를 `tool_use.id`로 보존한다. 이어지는 Claude 형식 전체 대화에서 대응 `tool_result`만 수락하여 원래 function_call과 function_call_output을 후속 input에 넣는다. 항목 `id`를 호출 `call_id`와 혼동하지 않는다. `is_error`는 `{is_error, content}` JSON 문자열로 보존한다. 최종 합성 marker 응답까지 요청 JSON 2개를 준비한다. 실제 전송·파일 읽기·도구 실행은 0회다.

메서드 반환값은 호출자용 데이터다. 응답 본문·도구 인수·대화를 콘솔이나 파일에 기록하면 안 된다. 검사기는 정제된 고정 검사 이름과 숫자만 출력한다. `diagnostics.preparedRequests`는 **준비한 JSON의 수**이며 서버 수신 횟수가 아니다. 반환된 메시지를 변경해도 내부 pending 인수는 바뀌지 않는다.

## 2. 확정한 헤더 정책

기본값은 `new OfflineSession()`의 `strict`다. 제품 호환 실험은 `new OfflineSession({ headerPolicy: 'codex-missing-content-type' })`로 명시적으로 선택한다. 두 경우 모두 본문 검증은 같다.

| 전송 메타데이터 | 처리 |
|---|---|
| 고정 `https://chatgpt.com/backend-api/codex/responses`, HTTP 200, 정확한 `text/event-stream` media type | 정상 본문 검사. media type 대소문자·charset 매개변수 허용 |
| 같은 endpoint/200, Content-Type **필드 부재**, 값은 빈 문자열 | strict는 `MISSING_CONTENT_TYPE`. 명시적 호환 모드만 본문 검사로 진행 |
| Content-Type 필드가 존재하지만 빈 값/공백 | `EMPTY_CONTENT_TYPE`, 호환 모드도 거부 |
| 다른 media type, 유사 접두부, 복수 값, 줄바꿈/제어 문자 | `UNSUPPORTED_CONTENT_TYPE` |
| 다른 endpoint, 모순된 메타데이터, HTTP 오류 | 본문을 성공으로 해석하지 않음. 401/403/429/리다이렉트는 별도 고정 분류 |
| 허용 헤더이나 오류·거부·미완료·절단·충돌·미지원 본문 | 실패. 실행 가능한 도구 블록 및 성공 종료 반환 없음 |

허용된 본문이 끝까지 검증됐을 때만 `diagnostics.accepted=true`를 반환하고, 누락 헤더 호환 여부를 `compatibilityApplied`로 표시한다. 원래 Content-Type을 보충하거나 기존 연결 검사기의 `passed`를 바꾸지 않는다. 같은 합성 응답을 기존 `summarizeResponse`에 전달하면 여전히 `passed=false/MISSING_CONTENT_TYPE`임을 검사한다.

[RFC 9110 §8.3](https://www.rfc-editor.org/rfc/rfc9110.html#section-8.3)은 헤더 부재 시 수신자의 데이터 검사를 허용하지만 오판의 위험도 설명한다. 그래서 이 정책은 특정 목적지의 제한된 Responses 구조에만 적용하며 기본 strict로 비활성화할 수 있다. RFC가 이 비공식 backend의 안정성이나 도구 의미를 보증한다는 뜻은 아니다.

`begin()`의 endpoint/status/헤더 존재 정보는 향후 신뢰할 수 있는 전송 계층이 실제 관찰값으로 만들어야 한다. 본문이나 클라이언트 요청이 이 값을 제공하게 해서는 안 된다. 현재는 합성 fixture가 제공한다.

## 3. 지원하는 좁은 입력 계약

| 항목 | 현재 구현 | 실제 연동 전에 남은 확인 |
|---|---|---|
| 모델·stream | 로컬 별칭 `clauduct-poc`, `stream=true`만 허용. upstream `gpt-6-astra/xhigh` 고정 | 실제 Claude가 보내는 모델 별칭과 옵션 |
| system | 문자열 또는 text block 배열 → developer input의 input_text 배열. 블록·문자열 순서 보존 | Codex backend의 정확한 system/instructions 의미 |
| messages | user로 시작해 교대하고 user로 끝나는 텍스트 대화. assistant는 output_text | 실제 Claude 대화의 추가 block 및 role 계약 |
| max_tokens | 정수 1–4096 → `max_output_tokens` | **공개 Responses 기준 변환 후보**다. Codex backend 수락·reasoning 포함 한도 의미는 미검증 |
| 도구 | `readTool()`이 제공하는 Read 1개와 정확히 같은 스키마, `tool_choice={type:'auto'}` | 설치된 Claude Read 스키마는 아직 확인하지 않음 |
| 경로 | `D:\AIDEV\Clauduct\poc\fixture.txt`와 문자열 정확 일치 | 이 파일은 만들거나 읽지 않았다. 실제 도구 실행에는 경로/링크 경계 검증이 추가로 필요 |
| 후속 요청 | 이전 옵션·대화 전체, 방출한 assistant tool_use, 단일 user tool_result가 일치해야 함 | 세션 인증과 다른 세션의 접근 차단 |
| 후속 도구 선택 | `tool_choice='none'`, 추가 function_call 거부 | 실제 모델이 최종 텍스트로 완료하는지 |
| 미지원 요청 | thinking, cache_control, 이미지, 임의 도구/schema/선택 옵션, 추가 필드는 실패 | 의미를 확인한 필드만 추후 추가 |
| usage | 실제 제공된 음이 아닌 정수 input/output/total의 합 일치 확인. input/output을 대응 | cached_tokens가 0이 아닌 사용량은 거부. cache 비용·count_tokens를 위조하지 않음 |

## 4. 스트리밍·상태·한도

SSE/JSON/UTF-8 청크 분할, CRLF, 주석, multiline data를 처리한다. 하나의 response.created 뒤 **순차 reasoning 항목 0개 이상과 마지막 메시지 또는 도구 호출 1개**를 허용한다. output_index는 0부터 이어져야 하며 중복 ID·interleaving·두 번째 표시용 항목·reasoning만 있는 완료는 거부한다. delta/done/item done/completed의 ID·위치·순서·문자열 일치와 완료 모델/effort/usage를 검사한다. sequence_number가 제공되면 0부터 끊김 없는 이벤트 순서와 일치해야 한다.

reasoning의 summary_text 및 reasoning_text snapshot, summary part/text와 reasoning text의 delta/done을 검증한다. 스트림이 있으면 완료 snapshot과 모든 위치의 내용이 같아야 한다. 스트림 없이 완료 snapshot만 온 경우도 알려진 필드·형태를 검사한다. 항목의 status는 생략될 수 있지만 있으면 added에서 in_progress, done에서 completed여야 한다. 완료 여부는 반드시 item.done과 정상 response.completed로 확인하며 status를 임의 생성하지 않는다. 이 상태 규칙은 메시지/도구에도 적용한다.

검증된 reasoning 항목은 화면의 text/tool/thinking으로 변환하지 않는다. 도구 결과 뒤의 Codex input에는 reasoning 항목→function_call→function_call_output을 같은 순서로 넣는다. encrypted_content는 해석·복호화하지 않는 메모리 데이터이며 stdout·하류 SSE·파일에 쓰지 않는다. 문자열의 동일성으로 평문의 동일성이나 유효성을 판정하지 않는다. **시작 item.added의 암호화 값은 완료 데이터로 승격하지 않는다.** 검증된 item.done을 기준으로 사용하며, 최종 response.output에 해당 reasoning 항목과 암호화 필드가 있으면 그 최종 값을 그대로 보존한다. 필드 생략 시에만 item.done 값을 유지하고 명시적 null/빈 문자열은 오래된 값으로 덮어쓰지 않는다. ID·상태·순서·summary/content 스트림 및 완료 snapshot의 텍스트 충돌은 계속 거부한다. item.done과 정상 response.completed가 모두 있어야 하고 후속 JSON도 기존 64 KiB 제한을 적용한다. [OpenAI Docs](https://developers.openai.com/api/docs/guides/reasoning#keeping-reasoning-items-in-context)의 완료 출력 재사용과 [item added/done 수명](https://developers.openai.com/api/reference/typescript/resources/responses)을 근거로 한 Clauduct의 선택 규칙이며 실제 backend의 재사용 성공은 별도 사용자 검증 대상이다.

완료 output이 정확히 빈 배열인 **텍스트와 도구 호출**은 일치하는 delta, text/arguments.done, 정상 output_item.done이 모두 있는 경우에만 복원한다. 도구의 완성된 function_call snapshot은 output_item.done에 반드시 있어야 한다. 마지막 output에 항목이 있으면 그것도 검증하며 충돌을 덮어쓰지 않는다. 정상 response.completed와 인수 JSON·고정 스키마·usage 검증 뒤에만 tool_use를 반환한다. 제공된 response_id와 arguments.done.name도 일치해야 한다. snapshot 부재·순서 충돌·미완료 상태를 임의 보충하지 않는다.

[OpenAI Docs의 함수 호출 스트리밍 설명](https://developers.openai.com/api/docs/guides/function-calling#streaming)은 arguments delta/done과 완성된 function_call을 담은 output_item.done을 설명한다. 빈 response.completed.output의 수용은 이번 사용자 보고에 대응한 Clauduct의 제한적 호환 동작이며, 공개 문서가 이 Codex backend 변형을 보증한다는 뜻은 아니다.

출력은 전체 응답 검증 뒤 Anthropic `message_start` → content block 시작/증분/종료 → `message_delta` → `message_stop` 순서의 SSE 문자열로 반환한다. **실시간으로 Claude에 증분을 전달하는 전송기는 아니다.** 여러 표시용 메시지/도구·메시지의 여러 content part, interleaving, 인용 annotation 등 미지원 의미는 실패한다. reasoning 처리 횟수만 responseDiagnostics.reasoningItemCount로 정제하며 이 숫자가 최종 답변 성공을 뜻하지는 않는다.

| 제한 | 값 |
|---|---|
| 입력 및 준비한 요청 JSON | 각각 64 KiB |
| 응답 | 256 KiB |
| 도구 인수 문자열 | 8 KiB |
| SSE JSON 이벤트 | 512개 |
| 요청 준비 / 도구 호출 / 동시 진행 | 세션당 2 / 1 / 1 |
| 요청 단계 시간 | prepare부터 완료 검증까지 45초 |
| 도구 결과 대기 | 추가 최대 45초. 전체 활성 시나리오 최대 135초의 단계 예산 |

테스트는 짧은 timeout을 선택할 수 있지만 45초보다 늘릴 수 없다. 성공·실패·취소·disconnect는 타이머와 응답 버퍼 참조를 정리한다. 도구 결과 대기에는 유한 타이머가 남는다. `cancel()`/`disconnect()`는 로컬 상태만 취소한다. 네트워크 취소 전파·OS 소켓·Ctrl+C·서버 추론 중단은 이 구현의 검증 대상이 아니다. managed 문자열의 즉시 완전 소거도 보증하지 않는다.

## 5. 관찰한 검증과 한계

Verified: Node **24.19.0**, 기존 합성 검사 **100/100**에 기본 45초 검사 3개를 추가한 **103/103**, 기존 연결 검사 **77/77**. 이번 변경 JavaScript 3개의 구문 검사를 통과했다. 첫 구현에서 반환 메시지와 내부 pending 객체의 참조 공유가 1건 실패했고 반환용 복제로 수정한 뒤 통과했다.

검사에는 정상 텍스트/도구 왕복, 오류 도구 결과, 인수/이력 변조, 알 수 없는 도구/ID/경로, 추가 요청 차단, 헤더 정책, HTTP 오류·리다이렉트 분류, UTF-8/SSE/JSON 분할, snapshot·순서 충돌, 오류·거부·미완료, 상한의 정확 경계와 초과, 취소·절단·짧은 실제 timeout이 포함된다.

다음 명령으로도 100/100를 관찰했다. Node 파일 권한은 새 코드와 기존 파서 파일 읽기만 허용하고 쓰기·자식 프로세스 권한을 주지 않는다. Node 24의 이 옵션이 네트워크를 차단한다고 주장하지 않는다. 어댑터에는 네트워크·파일·프로세스 호출이 없고, 검사 실행 경로는 합성 데이터만 사용한다. 출력의 0 카운터는 그 코드 경로의 요약이며 패킷 캡처나 OS 감사 수치가 아니다.

```powershell
node --permission --allow-fs-read=D:\AIDEV\Clauduct\poc --allow-fs-read=D:\AIDEV\Clauduct\verification\manual-http-probe.mjs .\poc\test-adapter.mjs
```

아래 명령은 기본 45초를 실제로 기다리는 검사 3개를 추가한다. 헤더 대기·부분 본문·tool_result 대기에서 각각 **45,038 ms**에 자율 타이머가 TIMEOUT으로 바뀌고 버퍼·타이머가 해제됐음을 관찰했다. 이전 .NET 전송기의 검증과 별개로 핵심 자체에서 측정한 증거다.

```powershell
node --permission --allow-fs-read=D:\AIDEV\Clauduct\poc --allow-fs-read=D:\AIDEV\Clauduct\verification\manual-http-probe.mjs .\poc\test-adapter.mjs --real-timeouts
```

Not verified: 실제 Claude, Codex backend의 도구 계약·max_output_tokens·reasoning, 실제 HTTP 2회 보장, 제품용 TLS/인증/취소 전파, 제품의 로컬 세션 인증, count_tokens·클라이언트 자동 재요청, 실시간 하류 스트리밍. 요청 검사기의 Host/Origin·상한·실제 소켓 정리는 별도 78/78 증거가 있지만 제품 세션 인증은 아니다. 실제 TTY/Ctrl+C 시작은 Windows 프로세스 생성 거부로 미검증이다.

Blocked by: 기존 credential-path 경계는 그대로다. 이 PoC에 인증을 연결하거나 기존 인증 검사를 자동 실행하는 경로는 없다. 다음 조건은 [전체 명세](../verification/최소-어댑터-PoC-명세.md), 인증 대안은 [OmniRoute 분석](../verification/OmniRoute-Codex-인증분석.md)에 있다.
