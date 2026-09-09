# Claude → Codex → 실제 Read 1회 실행

**현재: 실제 Read 왕복 PoC 완료 — 사용자 diagnosticVersion=3 / SUCCESS.** clientKind=claude-code, passed=true, clientExitCode=0, readCalls=1, linkedReadResults=1, gatewayExactMarker/claudeExactMarker=true, 요청/연결 각 2회, resourcesClosed/fixtureRemoved=true. gatewayToolExecutions=0으로 Claude가 도구 실행을 맡는 구조를 유지했다. 동일 검사의 재실행은 필요하지 않다.

실제 후속 메시지는 5개였으며 previousPrefixMatches=false, canonicalPrefixMatches=true, optionsMatch=true다. 원문 표현은 달라도 정규화한 이력과 옵션이 일치했고 연결된 fixture 결과와 최종 답변 검증을 통과했다. backend-default, HTTP 200/Content-Type missing 호환 경로의 성공이다. 이 결과는 OS 감사나 모든 hooks·일반 대화·Read 외 도구·장기 안정성 검증을 뜻하지 않는다. 아래 수정 및 미검증 기록은 성공 전 시점의 기록이며, 실제 왕복 대기는 이번 결과로 해소됐다.

최신 수정: 첫 HTTP 200 응답의 UNSUPPORTED_CACHED_USAGE를 해결했다. OpenAI input_tokens의 캐시 부분을 분리해 Anthropic input_tokens=전체−캐시, cache_read_input_tokens=캐시, cache_creation_input_tokens=0으로 전달한다. 총 입력과 출력 수를 보존하며 Anthropic 요금/캐시 생성 동등성을 주장하지 않는다. 잘못된 캐시 수치는 거부한다. adapter 318/318, Read 33/33(캐시 있는 첫/최종 응답과 5개 후속 메시지 결합) 통과. 실제 왕복은 아직 미검증이다.

현재 상태: **사용자 v3의 5개 후속 메시지 구조에 맞춰 변환 수정·로컬 검증 완료, 실제 왕복 확인 대기**. 후속은 기존 4개 또는 마지막 system이 추가된 5개를 받는다. 캐시 위치와 문자열/단일 텍스트 배열 표현을 정규화한 뒤 기존 텍스트·역할·옵션·호출을 비교한다. 추가 system은 도구 결과 뒤 developer로 전달한다. 진단 requestShape에 optionsMatch/canonicalPrefixMatches를 추가했으며 원문은 출력하지 않는다. 실제 사용자 SUCCESS 전에는 완료로 표시하지 않는다.

이번 원인 분석: 사용자 v3에서 user→system→assistant(tool_use)→user(linked tool_result, isError=false)→system과 previousPrefixMatches=false를 확인했다. 설치된 Claude의 api_system 직렬화는 캐시 유무에 따라 문자열/배열을 선택하며 user/assistant 블록의 캐시 위치도 바뀐다. 이 차이는 정규화하되 실제 prefix의 텍스트가 달라졌다면 계속 HISTORY_MISMATCH로 거부한다. 실제 텍스트가 동일하다는 증거까지 얻은 것은 아니다. [공식 Claude 캐시 설명](https://code.claude.com/docs/en/prompt-caching).

**Verified — 후속 수정:** Read 32/32, adapter 303/303, gateway 74/74, user-session 89/89. 5개 구조·캐시 이동·표현 차이·새 system 전달·reasoning/호출 ID 보존 및 이력/옵션/도구/ID 변조·중복 결과·잘못된 캐시/후속 블록 거부를 검사했다. 실제 계정·Claude 실행 0회. inspector 회귀와 45/180초 장시간 검사는 이번에 재실행하지 않았다. 아래 이전 검사 기록은 당시 결과다.

## 실행

사용자의 별도 PowerShell 터미널에서 다음 명령을 실행하고 안내를 읽은 뒤 `SEND`를 입력한다. 환경변수를 직접 입력하거나 토큰을 복사할 필요가 없다.

```powershell
node D:\AIDEV\Clauduct\poc\claude-read-once.mjs --live-read-once
```

기본 모델은 gpt-6-astra/low이며 한 프로세스에서 다른 모델로 fallback하지 않는다. 기존 Codex 사용자 메모리 로더를 재사용한다. Codex CLI 0.153.4 검사·만료 검사·TLS 검증을 유지하고 refresh·인증 쓰기·재시도는 하지 않는다. 에이전트는 이 명령·인증 로더·실제 Claude를 실행하지 않았다. 기존 credential/PTY/live 거부를 해제하거나 다른 경로로 재현한 것이 아니다.

## 실제로 수행하는 일

1. `poc/fixture.txt`를 배타적으로 새로 만든다. 기존 파일이 있으면 FIXTURE_EXISTS로 중단하며 덮어쓰지 않는다. 파일에는 실행마다 생성한 공개 challenge 한 줄만 들어간다. challenge는 인증 토큰이 아니며 첫 모델 요청과 Claude 명령 인수에는 포함하지 않는다.
2. 기존 Codex OAuth는 부모 사용자 프로세스의 전송 객체에만 둔다. gateway가 생성한 별도 로컬 세션 비밀만 Claude 자식의 ANTHROPIC_AUTH_TOKEN으로 전달한다. OAuth를 이 변수로 옮기지 않는다. 실제 로컬 비밀은 argv·inline settings·로그·임시 파일에 넣지 않는다. 이 전달 방식은 이번 실제 실행 모드의 명시적 선택이며 SEND 전에도 안내한다. 부모 환경·전역 설정을 변경하지 않는다. 환경을 읽을 수 있는 프로세스나 자손에게 로컬 비밀이 노출될 수 있다는 한계는 있다.
3. Claude는 `-p`, `--effort low`, `--tools Read`, `--disallowedTools mcp__*`, `--max-turns 2`, `--output-format json`, `--no-session-persistence`로 실행한다. 허용 권한을 추가하거나 기존 deny/hook/지침을 제거하지 않는다. 사용자 기존 권한이 Read를 거부하면 실패로 종료한다.
4. 첫 요청을 Codex에 변환·전송한다. 검증된 Read 호출의 경로는 고정 fixture와 정확히 같아야 한다. gateway는 fixture를 읽어 도구 결과를 대신 만들지 않는다.
5. Claude가 보낸 연결된 tool_result에서 공개 challenge 한 줄을 확인한 뒤에만 두 번째 요청을 보낸다. 오류 tool_result·다른 ID·변조된 이력·틀린 challenge는 추가 upstream 없이 거부한다. reasoning과 도구 결과를 연결하고, 두 번째 모델 답변은 정확한 challenge여야 한다.
6. Claude 자신의 최종 JSON 결과에서도 같은 challenge와 정상 종료를 확인한다. 모델 응답 전달 직후 Claude를 죽이지 않고 결과 출력을 기다린다. 종료 뒤 fixture의 동일 파일 identity·길이·내용을 확인해 이번에 만든 그대로일 때만 제거한다. 정리 검사의 파일 읽기는 결과 생성에 사용되지 않는다. 바뀐 파일은 보존하고 FIXTURE_CHANGED로 보고한다.

## 요청 변환과 제한

| 항목 | 실제 실행 모드의 처리 |
|---|---|
| 입력 정책 | claude-code-read-once. 기존 fixture 및 claude-code-v2-offline 모드는 유지 |
| system/대화 | 최초 developer→user→developer 순서를 보존. 후속 캐시 표시와 단일 텍스트 표현 차이를 정규화하고 기존 텍스트/역할/옵션 변경은 거부. 마지막 추가 system은 function_call_output 뒤 developer로 전달 |
| effort | CLI에 low를 명시하고 요청·전송·응답의 선택 profile 일치를 검사 |
| 출력 한도 | 실제 실행은 tokenLimitPolicy=backend-default. max_tokens 1–64000을 검증한 뒤 backend에 max_output_tokens를 보내지 않는다. 완료 usage가 클라이언트 한도를 넘으면 응답 전달 전에 거부한다. 이는 생성량·과금 상한 보장이 아니다. 요청별 시간/응답 크기 제한은 유지 |
| backend의 한도 지원 | 사용자 v2 실행에서 max_output_tokens 미지원 HTTP 400을 확인했다. backend-default는 처음부터 해당 필드를 생략하며 실패 후 자동 재시도하지 않는다. 기존 reject 기본값과 명시적 preserve의 원문 전송/400 분류는 보존 |
| thinking/cache/metadata | adaptive/display=omitted와 keep=all의 좁은 계약. reasoning은 기존 메모리 이력으로 보존. 캐시 힌트는 검증하되 모사하지 않고 client metadata는 upstream에 보내지 않음. cache/Claude signature 의미의 완전한 동등성을 주장하지 않음 |
| beta | claude-code-20250219, interleaved-thinking-2025-05-14, context-management-2025-06-27, effort-2025-11-24, redact-thinking-2026-02-12, prompt-caching-scope-2026-01-05, mid-conversation-system-2026-04-07, thinking-token-count-2026-05-13만 이 좁은 본문 계약과 함께 소비. 새/중복 beta는 거부. 실제 헤더 전체를 확인한 것은 아님 |
| 요청/시간 | upstream 최대 2회, 각각 45초. gateway 180초, request 5초, tool_result 45초, delivery 5초. gateway 종료 후 Claude 결과 대기 최대 5초·자식 정리 최대 2초. SEND 대기는 별도 60초 |
| 크기 | 입력 64 KiB, 응답 256 KiB, Claude stdout/stderr 합계 256 KiB. backend 400 정제 분류용 본문은 최대 8 KiB |
| 연결 | 기존 loopback·세션 비밀·Host/Origin/forwarded 검사 유지. Claude의 proxy는 외부로 중계하지 않는 로컬 gateway를 가리킴. gateway의 Codex 전송은 별도 고정 HTTPS 경로 |
| 헤더 | 명시적 codex-missing-content-type 호환 정책. 빈/잘못된 헤더는 여전히 실패. 기존 strict FAIL과 헤더 원인 미확정은 유지 |

beta 상수는 설치된 claude.exe에서 날짜 형식의 코드 상수만 정적으로 추출해 존재를 확인하고, body 변환 의미를 함께 검토했다. 바이너리를 실행하거나 인증 값을 검색하지 않았다. 임의의 새 beta를 무조건 통과시키는 구현은 아니다.

사용자 첫 실행은 UNSUPPORTED_CLIENT_VERSION_OR_BETA, unknownBetaCount=1, upstream 요청/연결 0회, Read 0회였고 자원/fixture 정리는 완료됐다. 설치된 Claude 2.1.263.0의 beta 조립 경로에서 thinking-token-count-2026-05-13 추가를 확인했다. 이것이 실제 미지원 1개였는지는 원래 진단에 이름이 없어 추정이다. [공식 estimated_tokens 정의](https://platform.claude.com/docs/fr/api/http/beta/messages)에 따르면 이 기능은 사고 진행량 표시용 추정치이며 과금량은 usage.output_tokens가 기준이다. 이 브리지는 사고 진행 프레임이나 추정치를 생성하지 않으며 기존 usage/출력 한도 검증을 유지한다. 진단 v2의 고정 boolean thinkingTokenCountRequested로 다음 실행에서 해당 헤더의 존재를 확인한다. 미지 헤더 원문은 출력하지 않는다.

## 결과 제출과 성공 조건

마지막 **정제 JSON 한 개만** 공유한다. 코드·Claude 출력 원문·토큰·헤더·fixture 내용은 제출하지 않는다.

성공에는 다음 값이 모두 필요하다: `passed=true`, `category=SUCCESS`, `clientKind=claude-code`, `clientExitCode=0`, `readCalls=1`, `linkedReadResults=1`, `gatewayExactMarker=true`, `claudeExactMarker=true`, `requestAttempts=2`, `resourcesClosed=true`, `fixtureRemoved=true`.

`gatewayToolExecutions=0`은 정상이다. 도구 실행 소유자는 Claude다. 이번 증거는 gateway의 검증된 호출/연결된 결과와 fixture 전용 challenge, 별도 Claude 결과의 일치다. OS 파일 I/O 감사나 모든 hooks가 올바르게 실행됐다는 증거는 아니다.

credentialWrites/retries는 Clauduct 전송 코드의 카운터이며 Claude 및 hooks의 모든 OS 동작을 감사한 값이 아니다. 긴 공백/탭으로 위장한 challenge 행도 거부하는 합성 검사를 포함했다.

실패는 실패 그대로 공유하고 같은 명령을 자동 반복하지 않는다. backend-default는 미지원 한도를 처음부터 생략하는 정책이며 SEND 전에 한계를 안내한다. preserve 검사 경로의 UPSTREAM_TOKEN_LIMIT_REJECTED 분류는 유지한다. UNSUPPORTED_CLIENT_VERSION_OR_BETA의 unknownBetaCount, 기타 고정 분류로 다음 원인을 구분한다. FIXTURE_EXISTS는 기존 파일을 보존한 상태이고, FIXTURE_CHANGED는 실행 중 달라진 파일을 보존한 상태다. 파일을 자동 삭제해 재시도하지 않는다.

## 검증 증거와 미검증

**Verified — beta 수정 후 로컬 검사:** 통합 **23/23**, 실제 합성 Node 자식 **18개**, 합성 upstream 수신 **22회**, fixture 잔여 0개. 새 beta가 포함된 정상 왕복과 미지/중복 beta의 upstream 0회 거부를 확인했다. 정상/늦은 최종 출력/추가 요청 차단/헤더 부재 호환, 권한 거부·틀린 결과/최종 답변·출력 토큰 초과·도구 미요청·일반 400/한도 미지원 400·절단·timeout·실행 전/중 취소·기존/변경 fixture 보존도 검사했다. gateway 회귀 **74/74**, 전달 제한 **5005ms / DELIVERY_TIMEOUT**. 합성 클라이언트는 실제 task fixture를 파일로 읽었지만 Claude Read 도구 실행은 아니다.

최초 구현 당시 변경 JavaScript 7개 구문 검사 통과. 당시 회귀: adapter **296/296**, gateway **74/74**, user-session **89/89**, request-inspector **90/90**, Claude 요청 관찰 실행 관리자 **14/14**. beta 수정에서는 변경하지 않은 adapter/user-session/inspector 회귀를 재실행하지 않았다. 에이전트의 실제 인증 읽기·외부 모델 요청·Claude 실행 0회. Node 자식 프로세스 권한 경고를 숨기지 않았으며 합성 자식에도 task 파일 읽기 제한을 적용했다. OS 네트워크 sandbox라고 설명하지 않는다.

```powershell
node --permission --allow-fs-read=D:\AIDEV\Clauduct\poc --allow-fs-read=D:\AIDEV\Clauduct\verification\manual-http-probe.mjs --allow-fs-write=D:\AIDEV\Clauduct\poc --allow-child-process D:\AIDEV\Clauduct\poc\test-claude-read-once.mjs
```

**Not verified:** 실제 Codex backend의 max_output_tokens 수락, 실제 Read/최종 결과, 실제 beta/display/schema 세부 값, Claude/관리형 설정의 재주입과 proxy 적용·자체 로그/부가 통신·hooks 자손 종료. 기존 설정 파일의 ANTHROPIC_AUTH_TOKEN이 자식 env를 덮으면 gateway가 거부할 수 있다. 관리형 정책을 우회하지 않는다. proxy는 OS 격리가 아니며 `--no-session-persistence`도 모든 Claude 자체 파일 쓰기를 금지하는 옵션은 아니다. 파일 생성 도중 OS 쓰기 오류가 나면 부분 생성된 파일이 남을 수 있으며 덮어쓰지 않는다. 45/180초 장시간 재실측과 모든 OS 오류는 이번에 검사하지 않았다.

공식 근거: [Responses 출력 한도](https://developers.openai.com/api/reference/typescript/resources/responses/methods/create), [Claude CLI 옵션](https://code.claude.com/docs/en/cli-reference), [gateway 인증과 설정](https://code.claude.com/docs/en/llm-gateway-connect), [thinking 표시](https://platform.claude.com/docs/en/build-with-claude/thinking). 공개 API와 실제 Codex backend 관찰은 구분한다.
