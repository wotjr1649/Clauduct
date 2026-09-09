# 최소 어댑터와 Claude Code 도구 왕복 PoC — final_answer 처리와 두 low 모델 검사

**현재 단계: 실제 Claude→Codex Read 1회 PoC 완료.** [사용자 실행](../poc/실제-Read-실행.md)의 diagnosticVersion=3 SUCCESS로 호출/연결된 결과 각 1회, gateway/Claude marker 일치, 요청 2회와 정리를 확인했다. 실제 실행은 backend-default이며 미지원 출력 한도를 생략하고 응답 usage를 전달 전에 검사한다. backend 생성량·과금 상한은 보장하지 않는다. 최신 로컬 검사는 Read 33/33, adapter 318/318, gateway 74/74, user-session 89/89다. 일반 대화·여러 도구 지원 완료를 뜻하지 않는다. 아래는 이전 단계 기록이다.

## 최신 — v2 관찰을 기반으로 한 명시적 오프라인 입력 변환

사용자 v2로 초기 messages 순서 user→system, output_config.effort=xhigh, context_management의 clear_thinking_20251015/keep=all, Read의 숫자 제약을 확인했다. 같은 shape 검사 재실행을 요구하지 않는다. **실제 Claude→Codex Read 왕복은 아직 미완료**이며 아래 구현은 인증 없는 변환·프로토콜 단계다.

`new OfflineSession({ inputPolicy: 'claude-code-v2-offline', profile: 'astra-xhigh' })`로 명시적으로 선택할 수 있다. 기본 fixture 계약, 실제 gateway 생성 경로, 사용자 live 진입점의 astra-low 기본값은 바꾸지 않았다. profile은 세션 소유자가 선택하며 요청이 모델/effort를 바꾸지 못한다. xhigh 요청을 low 세션에 넣으면 MODEL_EFFORT_MISMATCH다. 공개 사용자 실행 명령으로 이 모드를 활성화하지 않았다.

| 입력/의미 | 새 오프라인 정책 |
|---|---|
| top-level system과 messages의 user→system | top-level developer→user→developer 순서로 보존. system 텍스트를 user에 합치거나 앞으로 재배치하지 않음. 첫 두 role의 다른 순서·추가 초기 메시지는 거부 |
| max_tokens | 1–64000 정수 필수, 동일한 max_output_tokens로 보존. live transport의 TOKEN_LIMIT_UNSUPPORTED는 유지 |
| thinking/output_config | adaptive + display=omitted만 지원, effort는 선택 profile과 정확 일치. 예산·summary 표시·추가 옵션은 거부. Codex reasoning은 기존 메모리 이력으로 보존하고 Claude thinking signature를 위조하지 않음 |
| context_management | clear_thinking_20251015/keep=all 한 항목만 허용. 이 PoC의 완료 reasoning은 도구 결과 요청에 유지. 삭제/압축 전략은 거부 |
| metadata.user_id | 문자열 512자 이하만 허용하고 세션 원래 요청의 이력 비교에만 보관. OpenAI 요청이나 진단에 전송하지 않음. clientMetadataPolicy=local-only로 명시 |
| cache_control | ephemeral, 선택적 ttl=5m/1h를 검증하되 캐싱은 모사하지 않음. 본문을 보존하고 요청 측 캐시 힌트는 upstream에 넣지 않는 명시적 오프라인 정책. cachePolicy=not-emulated를 보고하며 cache 성능/요금 동등성을 보증하지 않음 |
| 실제 Read schema | file_path/pages string, offset integer 0..MAX_SAFE_INTEGER, limit integer 0 초과..MAX_SAFE_INTEGER, required=[file_path], additionalProperties=false를 검증. 설명은 문자열만, dialect는 부재 또는 알려진 draft-07/2020-12만 허용. 임의 ref/키/제약은 거부 |
| 모델에 제공할 Read | 기존 readTool()의 고정 fixture 경로·file_path 한 인수·strict=true로 제한. 실제 Read의 범위/페이지 기능은 모델에 제공하지 않음. readPolicy=fixed-fixture-only로 명시 |
| tool_choice 부재 | 해당 모드에서만 auto로 해석. 명시적 auto도 허용, 강제/임의 선택 거부 |
| tool_result | 정확한 기존 ID/assistant/옵션/전체 선행 이력 검증. 문자열 또는 text block 배열을 JSON 출력에 보존. is_error를 유지하며 결과 내용을 system/developer로 승격하지 않음 |

**Verified:** 어댑터 **296/296**. 새 계약 선택·기존 기본 거부 유지·순서/권한·메타데이터 비전송·effort 불일치·한도 보존·schema/캐시/옵션의 악성/미지원 입력 거부·reasoning→function_call→function_call_output·정확한 합성 marker·도구 거부 내용·ID/이력/옵션 변조를 검사했다. 생성된 64000 한도 요청을 실제 loopback transport에 넣으면 TOKEN_LIMIT_UNSUPPORTED이고 요청/연결 시도와 합성 HTTP 수신이 모두 0이다.

공유 핵심 변경 영향으로 gateway **74/74**(68개 정리·합성 upstream 37회), user-session **89/89**(45개 정리·합성 upstream 70회), request-inspector **90/90**(68개 정리)을 실행했다. 변경 JS 2개 구문 검사 통과. 전달 timeout은 5006ms에 DELIVERY_TIMEOUT. 실제 인증 읽기·외부 모델 요청·Claude/도구 실행은 0회. 45/180초 만료·실제 TTY 및 변경하지 않은 Claude 실행 관리자 회귀는 이번에 재실행하지 않았다.

**남은 정확한 조건:** v2는 thinking.display와 $schema의 문자열 값, cache_control의 구체적 형태, 미분류 beta 5개의 이름을 제공하지 않았다. 테스트의 display=omitted·draft-2020-12·ephemeral은 지원 후보를 검사하는 합성 선택이며 관찰값으로 주장하지 않는다. 원문 요청이 새 모드에서 수락됐다고 표시하지 않는다. 또한 첫 실제 tool_result의 형식/이력 변화, live 토큰 상한 지원, live 세션 비밀 전달과 beta 정책은 미검증이다. gateway는 여전히 beta 헤더를 거부하고 새 입력 모드를 사용하지 않는다.

후속은 같은 연결/shape 검사를 반복하는 대신 토큰 상한 및 beta 변환 계약을 해결하는 작업이다. 단순히 64000을 4096으로 낮춰도 현재 live transport는 명시된 한도를 모두 거부하므로 해결이 아니다. 시간·응답 바이트 제한을 토큰 상한과 같은 의미라고 주장하거나 한도를 삭제하지 않는다. 기본 strict·기존 헤더 누락 FAIL·두 low 모델의 이전 합성 OAuth SUCCESS는 유지한다.

공개 의미 검토: [Messages metadata/cache](https://platform.claude.com/docs/en/api/beta/messages/create), [thinking 표시와 암호화 이력](https://platform.claude.com/docs/en/build-with-claude/thinking), [context editing](https://platform.claude.com/docs/en/build-with-claude/context-editing). Claude의 서명/캐시/추론 의미와 Codex의 의미가 완전히 같다고 입증한 것은 아니며 새 모드를 오프라인으로 제한한 이유다.

## 이전 단계 기록

현재 후속: [Claude 요청 관찰 실행](../poc/요청-검사기.md)을 구현했다. 공개 marker로 실제 Claude 요청 shape를 먼저 얻으며 Codex 전송과 실제 Read를 실행하지 않는 단계다. 실행 관리 14/14·기존 검사기 85/85 로컬 통과, 실제 Claude 관찰은 사용자 실행 대기. 전체 Read PoC와 live 인증 전달·토큰 한도 정책은 아직 미완료다. 아래 갱신은 이전 상태로 보존한다.

2026-09-07 실제 Claude 후속: **요청 진단 보완 85/85, 실제 launcher 구현은 미완료**다. 명시적 max_tokens의 전송 미지원과 EndConversation 개수를 정제 진단에 추가했다. 공식 helper의 이중 헤더·메모리 전달, env 금지, 실제 도구 집합과 토큰 한도 의미가 남아 있다. [연결 조건과 미적용 제안](../poc/요청-검사기.md)에 근거와 로컬 검사·미검증을 구분했다. 기존 두 low 합성 SUCCESS와 strict 헤더 FAIL은 유지한다.

2026-09-07 현재: **사용자 v5 실행에서 gpt-6-astra/low와 gpt-5.6-luna/low 모두 SUCCESS**다. 각 모델에서 요청/연결 2회, 도구 복원 1회, callIdMatches/exactMarker/resourcesClosed=true, 헤더 부재 호환 적용 2회를 보고했다. 기존 Codex OAuth를 사용하는 게이트웨이의 합성 도구 결과→정확한 최종 답변 왕복을 확인했다. 실제 도구 실행은 0회이며 실제 Claude 인터페이스·권한/hooks 통합 성공은 아니다. 동일 검사를 반복하지 않는다. 아래 갱신 문단은 이전 시점의 기록이다.

2026-09-07 현재: 사용자 v3 보고는 **UNSUPPORTED_FIELDS**, 두 번째 HTTP 200 응답의 다섯 번째 message item-added/in_progress다. reasoning 완료 1개·암호화 갱신 1회를 통과했지만 추가 필드의 이름은 미확정이다. 진단을 v4로 보완했고 본문 계약은 유지했다. 변환기 **257/257**, 사용자 진입점 **73/73**, 게이트웨이 **72/72**를 로컬에서 통과했다. 전체 계정 왕복과 phase 지원은 완료되지 않았다. 아래 이전 갱신 문단은 해당 시점의 기록이다.

2026-09-07 최신: 사용자 v3 결과에서 SNAPSHOT_MISMATCH / REASONING_INITIAL_ENCRYPTED로 실패한 비교가 식별됐다. 시작/완료 암호화 문자열의 동일성 가정을 수정했다. item.added의 암호화 값을 완료 데이터로 승격하지 않고, 검증된 item.done 및 최종 response.output의 값을 생명주기에 따라 보존한다. 변환기 233/233·사용자 진입점 70/70·게이트웨이 72/72를 로컬에서 통과했다. **수정 후 실제 계정 전체 왕복과 backend의 완료 reasoning 재사용은 미검증**이다. 아래 원인 식별 전 기록은 당시 증거로 보존한다.

2026-09-07 후속 갱신: reasoning 지원 후 사용자 결과는 SNAPSHOT_MISMATCH, HTTP 200·13,357바이트·18개 이벤트 중 네 번째 reasoning item-done, status 부재/sequenceNumber=3/outputIndex=0이다. 첫 도구 복원과 두 번째 요청 진행은 유지됐지만 완료 reasoning 수는 0이며 전체 성공은 확인되지 않았다. 실제 충돌 필드는 미확정이다. **본문 계약을 변경하지 않고 diagnosticVersion=3의 snapshotCheck를 추가했으며 변환기 217/217·사용자 진입점 66/66·게이트웨이 72/72로 로컬 검증했다.** 다음 수용 작업은 고정 비교 이름을 확인하고 실제 프로토콜 계약과 대조하는 것이다. 특히 시작·완료 encrypted_content의 동일성 가정을 검증해야 하며 근거 없이 비교를 생략하거나 서로 다른 값을 덮어쓰지 않는다. 아래 UNSUPPORTED_OUTPUT과 v2 기록은 이전 단계의 증거다.

갱신일: 2026-09-07. 사용자 상세 결과에서 첫 도구 복원 1회와 두 번째 요청까지 진행했고, 두 번째 HTTP 200 응답의 status 없는 reasoning 항목에서 UNSUPPORTED_OUTPUT으로 중단했다. 단일 output_index=0·필수 status 가정을 수정해 순차 reasoning 뒤 한 메시지/도구를 처리한다. 사용자 진입점 62/62, 기본 게이트웨이 72/72, 변환기 202/202, 요청 검사기 82/82를 로컬에서 통과했다. 실제 계정 왕복의 최종 성공은 수정 후 사용자 결과 대기다.

사용자 보고로 Claude는 환경변수 CLAUDE_CODE_OAUTH_TOKEN + MAX 20 PLAN, Codex는 기존 로그인 OAuth를 사용한다. 새 PKCE·device login은 첫 PoC 필수가 아니다. OmniRoute처럼 라우터가 Codex Bearer/account 헤더를 소유하며 Claude는 도구 실행과 권한 판단을 소유한다. 실제 인증 값과 전역 설정을 읽거나 변경하지 않았다.

현재 상태는 **두 low 모델의 사용자 운영 OAuth·합성 도구 프로토콜 왕복 검증 완료, 실제 Claude 인터페이스 통합 전**이다. 모델별 사용자 결과 대기는 해소됐으며 동일 검사를 반복하지 않는다. 기본 strict와 기존 세 전송의 Content-Type 부재 FAIL은 유지한다. 이번 SUCCESS는 명시적 헤더 부재 호환 정책 아래의 별도 판정이다.

## 1. 근거와 적용 범위

| 구분 | 현재 근거 | 확정할 수 없는 부분 |
|---|---|---|
| 사용자 실행 증거 | Node https/fetch와 .NET 각각 HTTP 200, 모델 gpt-6-astra/effort xhigh 필드 일치, 정확한 OK 스트림 복원, 본문 SUCCESS | 세 방식 모두 Content-Type 누락으로 전체 FAIL. 내부 추론 강도·누락 주체·도구 사용 능력은 이 검사로 입증되지 않음 |
| 에이전트 로컬 검증 | 기존 Node 검사와 새 .NET의 합성 데이터·loopback 검사 | 실제 OpenAI HTTPS/TLS·인증 접근·Claude 클라이언트 실행은 포함하지 않음 |
| .NET 사용자 비교 | 정제 JSON 수신. Content-Type 필드 부재, 전체 MISSING_CONTENT_TYPE, 본문 SUCCESS, 요청/연결 호출 각 1회·재시도/인증 쓰기 0회 | 클라이언트 카운터이며 서버 수신 로그는 아님. 헤더를 보존하는 성공 전송은 확인되지 않음. 후속 호환 정책은 별도 사용자 승인으로 결정 |
| 사용자 gateway 실행 | v5에서 Astra low·Luna low 각각 SUCCESS, 요청/연결 2회·도구 복원 1회·호환 적용 2회·callIdMatches/exactMarker/resourcesClosed=true. 마지막 응답은 각각 9,695바이트/16이벤트와 13,142바이트/18이벤트. 모두 HTTP 200·Content-Type 부재·messagePhase=final_answer | 실제 도구 실행은 0회. 실제 Claude 요청/권한/hooks, 장기 대화·임의 도구와 지속적 계정 접근, 이전 reasoning 재사용 여부는 별도 조건 |
| 문서 계약 | 공개 Responses의 함수 호출/SSE 구조, Claude Messages 스트리밍과 gateway 요구사항 | Codex backend의 공개 API 호환성이나 계정의 지속적 사용 가능성을 보증하지 않음 |

비교 결론은 [검증 기록](./검증결과.md)에 있다. 세 전송 모두 헤더 부재·본문 성공이므로 동일 목적의 반복 라이브 검사를 종료한다. 원인 규명을 위한 조사 확대·헤더 보충·기존 검사 성공 조건 완화는 수행하지 않는다. 별도 승인한 제품 정책은 고정 endpoint/200에서 Content-Type 필드가 없을 때만 제한된 본문 검증을 허용한다. 빈 값·다른 media type·오류/불완전한 본문은 거부하며 기본 strict로 끌 수 있다. 원래 연결 FAIL을 SUCCESS로 바꾸지 않는다.

Anthropic의 현재 gateway 안내는 Claude Code에서 non-Claude 모델로 라우팅하는 구성을 공식 지원하지 않는다고 명시한다. 따라서 이 작업은 호환성 PoC이며 공식 지원 제품으로 표현하지 않는다. [Claude Code gateway 안내](https://code.claude.com/docs/en/llm-gateway)

## 2. 확정 요구사항 — 책임과 제외

1. Claude Code가 UI, 파일·shell 등 도구 실행, 권한 판단, 지침·hooks·plugins·skills를 소유한다. 어댑터가 승인을 대신하거나 Codex CLI/app-server를 도구 실행기로 띄우지 않는다.
2. Clauduct는 한 사용자·한 계정에 대해 실행 전에 선택한 astra-low(gpt-6-astra/low) 또는 luna-low(gpt-5.6-luna/low)를 세션 동안 고정한다. 메시지/도구 스키마/이벤트와 제한된 upstream 요청·취소를 중계하며 모델이나 URL을 응답 내용에서 선택하지 않는다. 서로 다른 모델은 독립 세션이고 자동 fallback하지 않는다.
3. 최초 PoC는 한 세션, 한 진행 중 요청, 한 읽기 도구 호출을 지원한다. 중복 요청·동시 요청·알 수 없는 도구/내용은 명시적 실패로 처리한다. Claude의 subagent 요청이 들어오면 섞지 않고 거부한다.
4. 범용 API 플랫폼, 대시보드, 다중 계정/provider, 설치·업데이트 패키지, 영구 대화 저장, 자동 인증 갱신, 모델 대체, 재시도, 실제 쓰기 도구 작업은 제외한다.
5. 로컬 HTTP 수신기는 127.0.0.1에만 바인딩한다. Host/Origin/세션 인증을 검사하고 URL에 비밀을 넣지 않는다. 난수 세션 비밀의 메모리 API·수명·종료 후 거부는 구현했으며 실제 CLI로의 비밀 전달은 별도 통합 조건이다.

## 3. 요청과 응답 변환 계약

아래 표는 전체 최소 어댑터의 요구사항이다. 이번 구현은 [PoC 입력 계약](../poc/README.md)의 좁은 합성 부분집합이다. public Responses 문서는 참고 계약이며 실제 Codex backend의 수락은 합성 검사로 대신 확인할 수 없다. 실제 backend 수락은 별도의 승인된 PoC가 필요하다.

| 입력 또는 경계 | 최소 변환 요구사항 |
|---|---|
| `POST /v1/messages?beta=true` | 라우트는 pathname으로 판정한다. `stream=true`의 메시지 추론 경로를 우선 지원하고 다른 method/path는 로컬에서 거부한다. 비스트리밍을 실제 클라이언트가 요구하면 구현 전 범위를 명시적으로 확정한다. |
| `system`, user/assistant text blocks | 순서, 역할, 원문 문자열, Unicode·줄바꿈을 보존한다. 단순 문자열 합치기로 system 경계/메시지 순서를 잃지 않는다. Responses의 `instructions`/`input`에 옮길 정확한 형태는 해당 CLI/backend 버전의 fixture로 고정한다. |
| `tools[].name/description/input_schema` | 검증된 client function 선언으로 변환한다. 이름·JSON Schema의 의미를 보존하며 변환할 수 없는 스키마를 조용히 축소하지 않는다. backend built-in shell/web/MCP 도구는 추가하지 않는다. |
| `tool_choice` | PoC의 허용 도구와 요청 선택 의미를 보존한다. 이 연결 검사기의 `tools=[]/none`을 제품에 그대로 복사하지 않는다. 지원하지 않는 선택 옵션은 upstream 전송 전에 실패한다. |
| 모델/effort/옵션 | 모델과 effort는 고정한다. Claude 모델 별칭을 어떻게 받을지는 실제 클라이언트 요청으로 확정한다. `thinking`, `cache_control`, `max_tokens`, 이미지, 서명/암호화된 reasoning, context management, tool reference 등 의미가 다른 필드는 임의 삭제·위조하지 않는다. 각 필드의 지원·거부·무손실 전달 여부를 착수 전에 표로 고정한다. |
| `response.output_text.delta` | output/content index별로 추적하고 Anthropic text block의 `text_delta`로 변환한다. UTF-8 code point·SSE 프레임·JSON 문자열이 네트워크 청크 사이에서 나뉘어도 보존한다. |
| reasoning 항목 | 표시용 출력 앞의 순차 reasoning을 ID/index로 추적한다. summary/text의 delta/done/part/snapshot을 대조하고 상태 생략은 item.done 및 정상 response.completed로 판정한다. 명시된 실패 상태·ID/텍스트 충돌·알 수 없는 필드는 거부한다. 암호화 문자열을 불변 식별자로 비교하지 않는다. 시작 값은 완료 데이터로 보충하지 않고, 검증된 item.done과 최종 response.output의 해당 값을 메모리에 보존한다. 최종 필드가 있으면 null/빈 문자열도 그대로 선택하고, 생략한 경우만 item.done 값을 유지한다. 후속 Codex 요청에만 전달하며 Claude thinking/text/tool로 위조하지 않는다. 응답 256 KiB·512 이벤트·후속 입력 64 KiB 안에서만 유지한다. |
| `function_call` 및 arguments delta/done | response item ID와 `call_id`를 구별한다. `call_id` ↔ `tool_use.id`를 세션 안에서 일대일 연결하고 이름/위치/인수 완료의 충돌을 거부한다. 인수 문자열은 JSON 데이터로만 처리한다. |
| 도구 블록 방출 | 최소 PoC는 인수 delta를 제한된 메모리에 모은다. 정상 response 완료와 스키마 검증까지 통과한 뒤 `tool_use` + `input_json_delta` + block stop을 보낸다. 완성되지 않았거나 충돌한 도구 인수가 Claude에서 실행 가능한 완료 블록이 되지 않게 한다. |
| Claude `tool_result` | 대응하는 pending `tool_use_id`만 수락해 Responses `function_call_output.call_id/output`으로 바꾼다. 해당 호출의 이전 function_call과 필요한 conversation items를 후속 요청에 함께 보존한다. 알 수 없는 ID, 중복 결과, 다른 세션의 ID는 전송하지 않는다. |
| 도구 거부·오류 | Claude가 반환한 `is_error` 결과는 성공으로 바꾸지 않고 모델이 실패임을 구별할 수 있는 명시적 출력으로 변환한다. 어댑터가 재실행하거나 다른 도구로 대신 실행하지 않는다. 정확한 오류 출력 포맷은 fixture로 고정한다. |
| 완료 output이 빈 배열 | 텍스트 및 단일 function_call은 delta·text/arguments.done·정상 output_item.done이 일치하고 정상 response.completed가 있을 때만 복원한다. 도구는 item/call ID·이름·위치·상태·인수 JSON/고정 경로·usage를 전부 검증한다. 제공된 response_id·arguments.done.name도 일치해야 한다. 존재하는 최종 항목은 별도 snapshot 검사하며 충돌을 덮어쓰지 않는다. 항목 snapshot·전체 완료가 없으면 실패한다. 이 경로는 별도 성공/실패·HTTP 왕복 검사로 검증했다. |
| usage | upstream이 실제 제공한 허용된 숫자만 대응시킨다. cache/reasoning 사용량을 다른 의미의 필드로 위조하거나 token count를 비용으로 확정하지 않는다. Anthropic 클라이언트 필수 usage 필드가 없을 때의 정책은 착수 조건이다. |

`function_call`의 JSON 인수와 `call_id`가 `function_call_output`의 연결 키라는 근거는 [OpenAI Function calling](https://developers.openai.com/api/docs/guides/function-calling)이다. Anthropic의 text/tool block 시작·증분·종료 형식은 [Messages streaming](https://platform.claude.com/docs/en/build-with-claude/streaming)을 따른다. 이 대응 표 자체는 Clauduct의 설계 요구사항이며 두 서비스가 자동 호환된다는 공식 설명이 아니다.

## 4. 종료 상태와 전송 제한

| 상태 | 필수 처리 |
|---|---|
| 정상 텍스트 완료 | 유일한 `response.completed`, 정상 status, 오류/미완료 없음, 모델/effort·스트림 일관성을 검사한다. 열린 블록을 닫고 `message_delta(stop_reason=end_turn)`와 `message_stop`을 한 번 보낸다. 단순 TCP EOF나 `[DONE]`만으로 완료하지 않는다. |
| 정상 도구 요청 완료 | 완성된 허용 도구 호출만 방출하고 `stop_reason=tool_use`로 해당 응답을 끝낸다. 도구 실행·권한 판단은 Claude에 남기고 어댑터 상태는 결과 대기다. |
| HTTP/인증 오류 | 401/403/429/기타 HTTP 실패를 구별한다. refresh·재시도·모델 교체 없이 고정된 로컬 오류를 반환한다. provider의 임의 오류 원문·헤더를 노출하지 않는다. |
| 스트림 중 오류 | HTTP 헤더 전이면 JSON 오류, 스트림 시작 후면 계약에 맞는 SSE 오류 후 연결 종료. 성공 종료 이벤트를 붙이지 않는다. 이미 표시한 text delta는 임시 출력이며 성공으로 확정하지 않는다. |
| 미완료/한도 초과 | `response.incomplete`, 완료 이벤트 부재, 중복/순서 충돌, 인수·본문·이벤트 수 상한 초과는 실패다. 실제 upstream 원인이 확인된 경우만 분류하고 정상 완료로 추정하지 않는다. |
| 사용자 취소/클라이언트 연결 종료 | upstream read/send와 자식 파서를 취소하고 소켓·타이머·pending 상태를 정리한다. 대기하던 도구를 실행시키지 않는다. 서버 추론/과금의 즉시 중단은 보증하지 않는다. |
| upstream 연결 중단 | 본문 절단/연결 오류로 실패하며 자동 재전송하지 않는다. 같은 입력이 다시 와도 이전 상태를 정상 완료로 복구하지 않는다. |
| 중복/동시성 | 한 세션의 진행 중 요청과 pending call ID를 보존한다. 중복 tool_result가 후속 추론을 중복 생성하지 않게 거부한다. 일반 요청 재전송을 구별할 수 있는 키와 메모리 수명은 착수 전에 정한다. |

연결 비교 검사의 45초/256 KiB를 제품의 모든 도구 결과 상한으로 확정하지 않는다. 최소 실제 PoC에는 요청 수 최대 2회(도구 선택·결과 뒤 최종 응답), 실행 도구 1회, 동시 추론 1개를 별도 예산으로 둔다. 현재 오프라인 핵심은 요청 JSON 각각 64 KiB, 인수 8 KiB, 응답 256 KiB, 이벤트 512개, 요청 단계 각 45초와 도구 결과 대기 45초의 예산을 적용한다. 핵심의 요청 준비 수와 실제 전송 수는 구별한다. 새 게이트웨이의 합성 도구 왕복에서는 upstream 서버 수신 2회를 별도로 관찰했다. 무제한 기본값은 허용하지 않으며 실제 제품 한도는 별도 확인한다.

## 5. 무해한 실제 도구 왕복 시나리오 — 미실행

1. 다음 단계 승인 후 작업 루트 안의 전용 합성 폴더에 `probe.txt`를 만들고 고정 문자열 `CLAUDUCT_PROBE_7`만 넣는다. 기존 사용자 파일·네트워크·외부 서비스를 사용하지 않는다.
2. 실제 설치된 Claude Code의 읽기 도구 이름/스키마를 확인하고 해당 도구만 PoC에 허용한다. 프롬프트는 지정 파일을 그 도구로 읽어 정확한 marker를 답하도록 한다. 도구를 쓰지 않고 맞힌 텍스트는 왕복 성공으로 계산하지 않는다.
3. 모델이 생성한 call ID·완성된 인수와 Claude에 전달된 tool_use를 대응시킨다. 경로가 합성 파일을 벗어나면 실행 전에 실패한다. 어댑터에 도구 작업을 수행하는 파일 읽기·shell 실행 경로를 넣지 않는다.
4. Claude가 기존 권한 및 hooks를 적용해 파일을 읽고 반환한 tool_result를 같은 call ID의 function_call_output으로 보낸다. 모델의 두 번째 응답에서 정확한 marker와 정상 종료를 확인한다.
5. 동일한 합성 fixture로 권한 거부, 잘못된 ID, 중복 결과, 분할 JSON 인수, 취소·절단을 로컬에서 주입한다. 거부된 경우 실행 0회·대체 실행 0회·성공 종료 없음이 통과 조건이다. 실제 계정 PoC 재실행 횟수는 별도 승인 범위를 넘기지 않는다.

첫 실제 성공 시나리오의 증거는 Claude 도구 실행 1회, upstream 요청 2회, ID 대응 일치, marker 정확 일치, 정상 완료, 어댑터 직접 도구 실행 0회다. 원문 대화/경로/토큰 로그 대신 합성 전용 assertion과 정제 카운터로 확인한다. 이 시나리오는 지금 실행한 검사가 아니다.

## 6. 구현 상태와 남은 수용 테스트

| ID | 조건/상태 | 닫는 증거/수용 기준 |
|---|---|---|
| G1 | 완료 — .NET 사용자 결과 분석 | 정제 JSON 1건을 기존 Node 보고와 비교했다. .NET도 HTTP 200·Content-Type 필드 부재·본문 SUCCESS·전체 FAIL이다. 원인 미확정으로 기록하고 동일 목적의 반복 라이브 검사를 종료한다. 에이전트가 실행한 라이브 증거로 바꾸지 않는다. |
| G2 | 완료 — 제한적 헤더 정책 | opt-in을 정확한 endpoint/200/헤더 부재에 적용한다. 최신 사용자 보고에서는 첫 응답 본문이 통과해 compatibilityApplied=1, 두 번째는 헤더와 reasoning을 통과하고 message에서 실패했다. 본문 성공 수와 헤더 단계 통과를 구별한다. 기본 strict·기존 연결 FAIL 및 빈 값/다른 type 거부를 유지한다. |
| G3 | 제한된 사용자 OAuth 왕복 완료 — Claude 전달은 미통합 | 기존 캐시를 메모리로 사용하는 사용자 진입점에서 두 low 모델의 SUCCESS를 보고받았다. 각각 요청 2회·정확한 marker·자원 정리를 확인했다. 에이전트의 직접 인증 검증은 아니며 실제 Claude CLI의 세션 비밀 전달은 남아 있다. credential-path guard를 유지한다. |
| G4 | 부분 완료 — 인증방식·저장 위치 보고 | Claude는 환경변수 CLAUDE_CODE_OAUTH_TOKEN, Codex는 기존 로그인 OAuth라는 사용자 보고를 기록했다. 추가 Claude 설정 조사를 게이트웨이 구현 조건에서 제외했다. 이전 파일 버전 2.1.263.0과 실제 실행/요청 shape는 구별하며 후자는 미검증이다. |
| G5 | 부분 완료 — HTTP 경로·요청 예산 | HEAD hello 204, Messages pathname 처리·SSE 반환, count_tokens 404, HTTP 8회·연결 16개·upstream POST 2회 상한을 구현했다. 합성 서버에서 301/302/303/307/308/421/429 추가 요청 없음과 도구 왕복 2회를 관찰했다. 실제 Claude fallback/재요청·정확한 token count는 미검증이다. |
| G6 | 부분 완료 — 도구 복원·reasoning | 검증된 item.done 도구 복원에 이어 순차 reasoning과 표시용 출력 분리, 상태 생략의 lifecycle 검사, 완료 reasoning/암호화 문맥 보존을 구현했다. 사용자 v3의 시작/완료 암호화 동일성 실패를 확인하고 완료 snapshot 선택 규칙으로 수정·로컬 검증했다. 자체 요청의 토큰 한도·instructions 정책은 유지한다. 실제 Claude max_tokens/Read, cache usage, 여러 표시용 출력·interleaving과 실제 backend의 reasoning 연속성은 미해결이다. |
| G7 | 부분 완료 — 검증 후 SSE와 backpressure | 전체 upstream 검증 뒤 16 KiB 하류 청크·write/drain 대기·5초 한도를 적용한다. 실제 Node stream에서 Unicode 보존·정체/재개·취소·close·5초 만료를 검증했다. 실제 느린 TCP 하류와 backend 생성 중 실시간 delta는 미검증/미구현이다. 기본 strict·이전 FAIL을 유지한다. |
| G8 | 실제 모델 + 합성 결과 왕복 완료 — 실제 Claude 도구 미실행 | 사용자 v5에서 두 모델 모두 도구 호출 요청→tool_use→합성 tool_result→정확한 최종 답변을 요청 2회로 완료했다. call ID·marker·정리 수용 기준 통과를 확인했다. 실제 Read 실행·Claude 권한/hooks는 포함되지 않으며 toolExecutions=0이다. |
| G9 | 부분 완료 — 시간·전달·자원 정리 | 09-07 기본 게이트웨이 72/72·66개 검증 세션 정리. 09-06 별도 시간 검사에서 upstream 45초는 45,016 ms, 전달 함수 5초는 5,010 ms, 전체 수명 180초는 180,014 ms에 만료했다. 이번 진단 수정에서 45초/180초는 반복 측정하지 않았다. 실제 OpenAI TLS·느린 TCP 하류·OS 오류 전체는 에이전트 미검증이다. TTY 거부는 우회하지 않았다. |
| G10 | 공식 지원의 한계 | 비공식 backend/비-Claude 모델 구성이라는 전제, 계정·클라이언트 버전 의존성, 업데이트 시 재검토 필요성을 유지한다. 공개 Responses 문서만으로 backend 도구 계약을 통과 처리하지 않는다. |

G5의 fallback·재요청·경로 규칙은 현재 [Claude Code gateway compatibility guide](https://code.claude.com/docs/en/llm-gateway-protocol)에 명시돼 있다. 이 문서의 Anthropic 오류 원문 전달 권고는 Clauduct의 비노출 요구를 바꾸지 않는다. 정제 오류를 받았을 때 실제 Claude의 동작을 별도 검사해야 한다.

## 7. 완료한 범위와 다음 단계 경계

비교 종료와 제한적 헤더 정책, 변환 핵심, 요청 검사기에 이어 HTTP 게이트웨이와 Codex 전송 계층의 구현·합성 통신 검증까지 완료했다. G1/G2의 비교·정책 결정과 G3–G9의 로컬 수용 증거는 실제 계정·인터페이스의 성공과 구별한다.

사용자 운영 인증 연결부와 같은 프로세스의 메모리 검사 클라이언트는 구현했고 두 low 모델의 사용자 실행 수용 기준을 통과했다. 이제 실제 Claude로의 로컬 세션 비밀 전달과 요청 계약 지원 확장이 남아 있다. 기존 OAuth의 메모리 사용·만료/401 중단·refresh/쓰기 없음은 유지하며 새 로그인은 필수가 아니다.

**Verified — 이전 에이전트 로컬 실행:** 변환기 269/269, 사용자 진입점 89/89, 기본 게이트웨이 74/74. 이번 결과 분석은 문서만 변경했으므로 회귀 검사를 재실행하지 않았다. **Verified — 이번 사용자 실행:** Astra low·Luna low 모두 제한된 실제 모델 왕복 SUCCESS. 두 증거의 출처를 구별하며 상세 수치는 [검증 기록](./검증결과.md)에 있다.

**이전 reasoning 지원 단계의 로컬 실행:** 사용자 진입점 62/62, 기본 게이트웨이 72/72, 변환기 202/202, 요청 검사기 82/82. reasoning 포함 합성 HTTP 왕복·후속 보존·비노출·snapshot 충돌/누락·추가 요청 없음·5초 전달·자원 정리를 확인했다. 45초/180초·기존 연결 파서는 이전 기록이며 그 단계에서는 재실행하지 않았다. 첫 도구 복원·두 번째 요청·reasoning 실패는 당시의 별도 사용자 보고다.

**Not verified:** 실제 Claude 요청/Read/권한/hooks/재요청·CLI 세션 비밀 전달·토큰 한도 정책, 여러 assistant 메시지의 provider phase 이력 보존과 commentary 전환, backend 생성 중 실시간 delta, 느린 실제 TCP 정체·OS 오류 전체. 이번 두 성공은 각각 한 번의 고정 합성 도구 시나리오에 한정되며 임의 도구·장기 대화·지속적 계정 접근을 보증하지 않는다. backend의 이전 reasoning 재사용 여부와 내부 추론 강도도 이 JSON으로 독립 확인할 수 없다. Content-Type 부재·암호화 변화의 내부 원인은 미확정이다.

## message phase 지원 전 조건

공개 [ResponseOutputMessage 스키마](https://developers.openai.com/api/reference/typescript/__sdk_schema?declaration=%28resource%29+responses+%3E+%28model%29+response_output_message+%3E+%28schema%29&selected=%28resource%29+responses)는 선택적 phase를 commentary/final_answer/null로 정의하며, assistant 메시지를 후속 입력에 다시 보낼 때 phase를 보존하도록 설명한다. 읽기 전용 OmniRoute의 [schema](../../_ref/OmniRoute/open-sse/vendor/codex-chatgpt-web/responses/schema.ts)와 [parser](../../_ref/OmniRoute/open-sse/vendor/codex-chatgpt-web/responses/parser.ts)에도 commentary/final_answer와 보존 경로가 있다. 공개 계약과 참조 구현의 확인이며 이번 실제 응답에 phase가 있었다는 증거는 아니다.

1. **진단 완료:** 사용자 v4의 messageExtraFields=phase-only/messagePhase=final_answer로 확인했다. other-only/phase-and-other는 계속 실패하며 임의 필드명·값은 출력하지 않는다. 이 확인은 해당 v4 응답에 대한 것으로 이전 v3 응답의 원문 동일성을 입증하지 않는다.
2. **구현 완료 — 단일 최종 메시지:** 시작/item.done/최종 output의 phase는 부재/null/final_answer를 허용하고 commentary·미지 타입/값은 UNSUPPORTED_MESSAGE_PHASE로 거부한다. phase 유무와 관계없이 정상 item.done/response.completed, ID·순서·텍스트·usage 및 선택한 모델/effort를 검사한다. 시작 phase의 부재/null을 불변 ID처럼 비교하거나 final_answer를 임의 보충하지 않는다. phase는 프로토콜 메타데이터로 검사하며 Claude content에 추가하지 않는다.
3. **commentary 경계:** commentary는 중간 메시지이므로 단독으로 end_turn이나 최종 marker 성공을 만들면 안 된다. 현재 한 표시용 항목 PoC에서 처리할 수 없는 commentary→도구/최종 답변 순서는 명시적으로 미지원으로 유지한다. 다중 표시용 메시지와 phase 전환은 상태·순서·종료 계약을 별도로 확정한 뒤 구현한다.
4. **수용 검사와 남은 경계:** 단계별 phase 부재/null/final_answer, 잘못된 phase·추가 키 거부, commentary 최종 완료 방지, 완료 누락/미완료·ID/텍스트/usage 충돌은 로컬 통과했다. 두 low 모델의 정확한 요청/응답 대응과 합성 marker·2회 요청·정리도 통과했다. 현재 텍스트 완료 후 세션을 끝내므로 여러 후속 assistant 메시지의 provider phase 보존은 구현하지 않았다. 일반 대화 이력 지원 전 보존 계약과 수용 검사가 필요하며 user 메시지에 phase를 만들어 넣지 않는다. 실제 계정 전체 왕복은 모델별 사용자 증거로 따로 확인한다.

현재 구현은 단일 최종 메시지 phase 처리와 두 고정 low 선택, 사용자 운영의 제한된 실제 모델 왕복까지다. 여러 표시용 메시지 흐름은 추가하지 않았다. 이번 사용자 증거로 해당 두 시나리오의 계정 접근은 확인했으며 다른 모델·지속적 접근으로 일반화하지 않는다.

**Blocked by:** 기존 credential-path guard와 PTY 거부는 에이전트의 실제 인증/라이브 실행에 계속 적용된다. 이번 두 모델의 사용자 결과 대기는 해소됐다. 실제 Claude UI 연결·세션 비밀 전달·토큰 한도 정책은 별도의 미통합 조건이며 인증 guard와 구별한다.

헤더 누락의 원인은 미확정이며 기존 사용자 비교는 세 방식 모두 전체 FAIL이다. 동일 목적의 반복 라이브 검사를 종료한 결정을 유지한다. 인증 변경을 해결책으로 단정하지 않으며 G10의 공식 지원 한계는 로컬 검사로 없앨 수 없다.

## 다음 구현 단계 — 실제 Claude 요청 경계

1. **요청 계약:** 기존 제한된 게이트웨이에 실제 Claude 요청의 모델 별칭·Read 스키마·옵션·max_tokens/count_tokens·오류 응답을 대응한다. 이미 있는 요청 검사기와 합성 검증을 재사용하고 미지원 필드를 조용히 삭제하지 않는다. 기존 Claude OAuth 환경변수의 값을 조사하거나 변경하는 작업은 포함하지 않는다.
2. **사용자 프로세스 연결:** 기존 Codex OAuth와 로컬 게이트웨이 세션 비밀을 분리한다. 실제 Claude CLI에 세션 비밀을 안전하게 전달할 수 있는 경로를 먼저 확인하고, 토큰을 채팅·argv·로그·임시 파일로 옮기지 않는다. 지원되는 전달 경로가 확인되기 전 launcher 완료를 선언하지 않는다. 기존 guard·PTY 거부를 우회하지 않는다.
3. **실제 도구 수용 기준:** Claude가 도구 실행과 권한 판단을 소유한 상태에서 지정한 무해한 fixture 읽기 1회, tool_use ID→tool_result ID 일치, 정확한 최종 marker, 사용자 거부·취소·종료의 정리와 추가 요청 없음을 검증해야 한다. 이번 synthetic 결과로 이 수용 기준을 통과 처리하지 않는다. 실제 사용자 실행 전에는 가능한 계약 구현·합성 검사·실행 안내를 준비한다.

현재 결과 반영에서는 위 통합 코드나 실제 Claude 실행을 추가하지 않았다. Astra/Luna low의 동일 프로토콜 검사를 반복하는 것이 다음 단계의 필수 조건은 아니다.
