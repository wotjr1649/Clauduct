# 웹 검색을 Codex 백엔드로 넘기는 브리지

## 근거

읽기 전용 조사에서 두 경로가 다르다는 것을 확인했다.

- Claude Code의 **WebFetch**는 클라이언트 기능이다. 설치 바이너리에 `WebFetchTool`, `WebFetchCache`, `getWebFetchUserAgent`, `convertHtmlToMarkdown`, `applyPromptToMarkdown`가 있다. URL을 직접 가져와 markdown으로 바꾼 뒤 모델 호출로 요약하며, 그 호출은 base URL을 타므로 **이미 Codex로 간다**. 실제 왕복은 미검증이다.
- Claude Code의 **WebSearch**는 Anthropic 서버 실행 도구다. `web_search_20250305`, `server_tool_use`, `web_search_tool_result` 문자열이 있다.
- 설치 Codex 0.154.0에는 내장 검색이 있다. `web_search` 147건, `web_search_call` 15건, `web_search_mode`·`web_search_tool_type`(값 `web_search_preview`)·`search_context_size`가 모델 계열 설정 키로 존재하고 `response.web_search_call` 이벤트 이름도 있다.

[Anthropic 웹 검색 문서](https://platform.claude.com/docs/en/agents-and-tools/tool-use/web-search-tool)와 [OpenAI Responses 웹 검색 문서](https://developers.openai.com/api/docs/guides/tools-web-search)로 양쪽 형태를 확정했다.

## 설계

요청 방향만 번역하고 결과 블록은 만들지 않는다.

- `web-search-2025-03-05` beta를 거부 목록에서 허용 목록으로 옮긴다(허용 14 → 15).
- `prepareNative`가 `web_search_20\d{6}` 형태의 서버 도구 항목을 받는다. `name`은 `web_search`여야 하고 항목은 하나만 허용한다. `allowed_domains`와 `blocked_domains`는 동시에 올 수 없고 각각 64개·256자 이하, `user_location`은 `approximate` 형태의 다섯 필드만 받는다. `max_uses`·`allowed_callers`·`response_inclusion`은 받되 상류에 대응이 없어 전달하지 않는다.
- 상류로는 `{ type: 'web_search' }`에 `filters`와 `user_location`만 붙여 보낸다. 이 도구는 클라이언트가 호출하는 것이 아니므로 `names` 집합에 넣지 않는다. 따라서 모델이 이 이름으로 `tool_use`를 만들어도 기존 `UNSUPPORTED_TOOL_CALL` 검사에 걸린다.
- 응답에서 `web_search_call` 출력 항목과 `response.web_search_call.*` 이벤트를 받아 검증하지만 downstream 콘텐츠는 만들지 않는다. 검색은 상류에서 실행되고 그 결과는 모델의 답변 텍스트에 반영된다.
- 검색이 켜진 요청에 한해 `output_text`의 `url_citation` annotation을 허용하고 버린다. 다른 annotation과 `logprobs`는 기존대로 거부한다. 검색을 요청하지 않은 응답에 `web_search_call`이 오면 여전히 `UNSUPPORTED_OUTPUT`이다.

**의도적으로 만들지 않은 것**: `server_tool_use`와 `web_search_tool_result` 블록, 그리고 `citations`. Anthropic 형태는 `encrypted_content`/`encrypted_index`를 다음 턴에 되돌려 받기를 요구하는데, 그 값을 합성하면 우리 자신의 검증만 통과하고 native 렌더링에서 어떻게 되는지는 알 수 없다. 지금 설계는 대화 기록에 새 블록을 넣지 않으므로 후속 턴이 그대로 왕복한다.

**천장**: 사용자는 답변 텍스트로 검색 결과를 받지만 출처 카드와 인용 링크는 보이지 않는다. `max_uses` 상한은 상류에 전달되지 않는다.

## 검증

2026-09-11, Node.js v24.19.0, `Invoke-ClauductNodeTests`, 60초 제한. `test-native.mjs`가 45 → 47개 검사다. 새 검사는 도구 번역 결과(`{type:'web_search', filters}`), `names` 비어 있음, 두 도메인 목록 동시 지정·미지 필드·이름 변경·중복 항목의 거부, 검색 항목과 세 진행 이벤트를 포함한 전체 응답이 텍스트만 downstream에 내보내는지, 질의 문자열과 출처 URL과 `web_search`라는 단어가 응답·SSE 어디에도 없는지, 도구를 요청하지 않은 경우 같은 상류 응답이 `UNSUPPORTED_OUTPUT`으로 거부되는지를 확인한다.

기준 11개 파일과 선택·완료·리뷰 표면 5개가 모두 통과했다. 외부 추론 요청·실제 인증 조회·실제 Claude 실행은 0이다.

## Not verified

우리가 쓰는 네 모델이 상류에서 이 내장 도구를 실제로 받아들이는지, 검색이 실제로 실행되는지, WebFetch의 실제 왕복. 모두 실사용 1회로만 확인된다.

## 실사용 관측 — 세션 712e4b63, luna/max (2026-09-11)

사용자가 실행한 Clauduct 세션과 메시지를 주고받아 확인했다. 결론부터: **이 브리지는 이 클라이언트에서 호출되지 않는다.**

관측:

- 요청 13건 전부 성공, `failed` 0, 마지막 요청의 `failureCategory`도 null. 즉 400도 502도 나지 않았다.
- 그런데 검색은 돌지 않았다. WebSearch를 두 번(두 번째는 URL 포함을 명시) 시도했고 둘 다 "I'm unable to perform a live web search in this turn"이라는 모델 텍스트로 끝났으며 결과 URL이 없었다.
- 결정적 단서: 세션 transcript에 렌더링된 것은 **`functions.WebSearch`** 형태의 tool-use였다. 즉 Claude Code는 WebSearch를 Anthropic 서버 도구(`{type:'web_search_20250305', name:'web_search'}`)가 아니라 **일반 function 도구**로 요청에 실었다.

따라서 `prepareNative`의 서버 도구 분기는 한 번도 타지 않았고, 그 도구는 평범한 function 정의로 상류에 전달됐다. 상류 Codex에는 그 이름을 실행할 주체가 없으므로 검색이 일어나지 않았고, 요청 자체는 정상 완료됐다.

이것이 이번 세션이 계속 경계해 온 실패 양식 그대로다 — **초록불인데 기능은 죽어 있는 상태.** 브리지에 관측성이 없어서 "안 보냄 / 보냈지만 상류가 안 씀 / 썼음"을 구분할 수 없었다.

## 관측성 보완

요청 기록에 고정 값 두 개를 추가했다. 질의도 결과도 담지 않는다.

- `webSearchRequested`: 그 요청이 서버 검색 도구 정의를 실어 보냈는가(즉 브리지가 탔는가).
- `webSearchCalls`: 상류가 실제로 실행한 `web_search_call` 항목 수.

세 상태가 구분된다. `requested=false`면 클라이언트가 서버 도구를 보내지 않은 것이고(현재 관측된 상태), `requested=true, calls=0`이면 보냈지만 상류가 검색하지 않은 것이며, `calls>0`이면 검색이 실제로 돈 것이다. 이미 실행 중이던 세션의 게이트웨이는 변경 전 코드라 이 필드가 없으므로, 다음 Clauduct 실행부터 관측된다.

## 판정

브리지 자체는 로컬 검사를 통과하고 서버 도구가 오면 올바르게 번역한다. 그러나 **현재 Claude Code는 그 형태로 보내지 않으므로 실사용에서 웹 검색은 여전히 동작하지 않는다.** function 도구 형태(`WebSearch`)까지 받아 상류 내장 검색으로 바꿔치기하는 것은 클라이언트가 선언한 도구를 게이트웨이가 대체하는 별개의 설계 결정이라 임의로 하지 않았다.

WebFetch는 이번 관측에서 시험하지 않았다.

## 정정 — 브리지는 호출됐다 (2026-09-11, 바이너리 확증)

위 "브리지가 호출되지 않았다"는 판단은 틀렸다. 설치 `claude.exe`의 WebSearch 실행부를 찾아 확인했다.

```
I = Te({content: "Perform a web search for the query: " + _}),
D = {type: "web_search_20250305", name: "web_search",
     allowed_domains: …, blocked_domains: …, max_uses: 8}
```

즉 `functions.WebSearch`는 모델에게 보이는 껍데기이고, Claude Code는 그것을 실행할 때 **서버 도구 형태를 실은 side query를 별도로 보낸다**. 같은 영역에 `web-search-side-query-api-error`, `Qus(q,_,xe)`의 `results` 배열, `mapToolResultToToolResultBlockParam`의 `Web search results for query: "${r}"` 봉투가 함께 있다.

따라서 실제 흐름은 이렇다. side query가 게이트웨이에 도착 → `prepareNative`가 서버 도구를 받아 상류에 `{type:'web_search'}`로 번역 → 상류 Codex가 검색을 수행하지 않고 정상 응답 → `results` 빈 배열 → 빈 봉투가 tool result가 됨. 실패 0건인 이유가 이것이다.

**결론: 브리지는 정상 동작했고, 상류가 검색을 하지 않았다.** 원인 후보는 모델 계열별 도구 타입 토큰 불일치, 계열 미지원, 계정 권한이며 오프라인으로는 좁힐 수 없다. 근거 없이 도구 형태를 바꿔 재시도하지 않는다.

## 무동작 표면화

성공한 요청이 서버 검색 도구를 실었는데 상류 검색이 0회면 세션당 한 번 stderr로 알린다. 질의도 결과도 담지 않는다. 누계는 `lifetime.webSearchRequests`와 `lifetime.webSearchCalls`이며, 요청별로는 `webSearchRequested`/`webSearchCalls`다. `requests>0 && calls===0`이 곧 "보냈는데 상류가 안 썼다"는 판정이다.

검증: `test-native-gateway`가 54 → 55개 검사다. 새 검사는 검색 도구를 실은 요청 2건과 실지 않은 1건에서 누계가 2/0이 되고, 요청별 값이 `[true,0],[true,0],[false,0]`이며, 세 요청 모두 성공이고, 알림이 세션당 1회인지 확인한다.

## 원인 확정과 수정 — 접근 플래그 누락 (2026-09-11)

실사용에서 `webSearchRequests: 1 / webSearchCalls: 0`이 나온 뒤, 설치 Codex 0.154.0의 `tools/src/tool_spec.rs` 문자열에서 `ToolSpec::web_search`의 필드 목록을 찾았다.

```
web_search { external_web_access, indexed_web_access, filters,
             user_location, search_context_size, search_content_types }
```

같은 바이너리에 `WebSearchMode` enum이 `disabled | indexed | live`로 들어 있고, 사용자 codex 설정에는 `web_search = "live"`가 이미 있다. 즉 **접근 모드는 저 두 불리언으로 표현된다.**

우리는 `{type:'web_search'}`만 보내고 두 필드를 모두 생략했다. 백엔드가 기본값을 꺼진 쪽으로 잡으면 도구는 수용되지만 웹에 나가지 못한다 — 오류 없이 검색 0회. 관측과 정확히 일치한다.

수정: `external_web_access: true`, `indexed_web_access: true`를 함께 보낸다. 이는 codex의 `live` 모드와 같고 사용자의 기존 설정과도 일치한다. `search_context_size`와 `max_uses`는 이번에 보내지 않는다(상류 기본값 사용).

이것은 추측이 아니라 기준 클라이언트의 필드 정의에서 나온 수정이지만, **실제로 검색이 도는지는 실행해 봐야 확정된다.** 다음 실행에서 `webSearchCalls > 0`이면 확정이고, 여전히 0이면 모델 계열의 `supports_standalone_web_search`나 계정 제약 쪽을 봐야 한다.

## 콘텐츠 블록 진단

응답으로 전달한 블록의 종류별 개수와 첫 블록 종류를 기록한다. 내용은 담지 않는다.

- `contentBlocks`: `{ text, toolUse, thinking }` 개수
- `firstContentBlock`: `text` / `tool_use` / `redacted_thinking` / null

설치 바이너리의 WebFetch apply 코드가 `content[0]`에 `text`가 없을 때 `No response from model`을 반환하므로, 그 실패가 재발하면 이 값 하나로 즉시 갈린다. 이전에 같은 문제를 세 번 추론해 세 번 틀린 이유가 이 값이 없어서였다.

검증: `test-native-gateway` 55 → 56개 검사, `test-native` 47개 검사. 기준 11개와 선택·완료 4개가 모두 통과했다.

## 접근 플래그도 실패 — 세션 c89e267f (2026-09-11)

`external_web_access`/`indexed_web_access`를 모두 true로 보낸 뒤 실사용에서 다시 측정했다. `lifetime.webSearchRequests: 1`, `webSearchCalls: 0`. **여전히 상류가 검색을 0회 수행한다.** 사용자가 같은 세션에서 모델을 terra로 바꿔 한 번 더 시도했으나 역시 실패했으므로 모델 계열 가설도 약해졌다.

실패한 가설을 순서대로 남긴다. 도구 태그 이름 → 접근 모드 플래그 → 모델 계열. 네 번째로 추측하지 않는다.

남은 후보는 이 게이트웨이가 codex와 다르게 보내는 것들이다. 기준 클라이언트는 요청에 `x-codex-turn-metadata`, `x-codex-routing-hint`, `x-codex-installation-id`, `x-codex-window-id`, `x-codex-server-id`, `x-codex-turn-state` 같은 헤더를 싣는데 Clauduct는 하나도 보내지 않는다(현재 보내는 것은 `Authorization`, `chatgpt-account-id`, `Content-Type`, `Accept`, `Accept-Encoding`, `Version`, `User-Agent`, `originator: codex_cli_rs`, `Openai-Beta: responses=experimental`뿐이다). 다만 그 헤더들의 내용을 알지 못하며, 설치 식별자를 지어내 보내는 것은 이 프로젝트가 해 온 방식이 아니다.

따라서 다음 단계는 추측이 아니라 **전제 측정**이다. 설치된 codex 자체로 같은 계정·같은 모델에서 검색이 실제로 되는지 확인한다. 되면 우리 요청과 codex 요청의 차이를 좁히는 문제이고, 안 되면 계정·엔드포인트 제약이라 게이트웨이로는 해결할 수 없다.

## 콘텐츠 블록 진단의 첫 실사용 출력

같은 세션에서 새 진단이 즉시 값을 했다.

| 요청 | `contentBlocks` | `firstContentBlock` |
|---|---|---|
| 10 | text 0, toolUse 1, thinking 2 | `redacted_thinking` |
| 11 | text 0, toolUse 1, thinking 1 | `redacted_thinking` |
| 12 | text 1, toolUse 0, thinking 1 | `text` |
| 13 | text 1, toolUse 1, thinking 1 | `text` |

블록 순서는 text → thinking → tool_use이고, **텍스트가 없는 응답에서는 첫 블록이 `redacted_thinking`이 된다.** 설치 바이너리의 WebFetch apply는 `content[0]`에 `text`가 없으면 `No response from model`을 반환하므로, 이것이 그 비결정적 실패의 메커니즘으로 유력하다. 다만 실패했던 그 요청은 텍스트를 냈으므로 그 사례 자체의 확정은 아니다.

## 알림 위치 수정

세션 중 stderr 출력이 native의 프롬프트 입력창 안으로 끼어드는 것을 사용자가 화면으로 확인했다. 네 알림(미등록 역할, 미등록 모델명, 미지원 이벤트 캡처, 웹 검색 무동작)을 모두 수집만 하고 **자식 종료 후 종료 JSON 직전에** 출력하도록 바꿨다. TUI를 건드리지 않는다. 강제 종료 시에는 유실되지만 이 알림들은 세션이 죽는 상황을 다루지 않는다.

## 원인 확정 — 봉투가 달랐다 (2026-09-11, 로컬 캡처)

설치 codex를 loopback sink로 향하게 해서 실제 요청을 그대로 받아 봤다. 커스텀 provider(`requires_openai_auth=true`, `wire_api=responses`)로 base_url만 로컬로 돌렸고, 외부로 나간 요청은 없다. `Authorization`과 계정 식별자는 도착 즉시 버려 출력·저장하지 않았다.

관측한 요청:

```
POST /backend-api/codex/responses
headers: x-openai-internal-codex-responses-lite: true
         x-codex-beta-features, x-codex-window-id, x-codex-turn-metadata,
         x-client-request-id, session-id, thread-id, originator: codex_exec
body keys: model input tool_choice parallel_tool_calls reasoning store
           stream include prompt_cache_key text client_metadata
tools: 없음
input item kinds: ["additional_tools", "message"]
```

`additional_tools` 항목의 형태는 `{type, id: "at_<uuid>", role: "developer", tools: [{type:"namespace", name, description, tools:[...]}]}`이고 **그 안에 `web_search`는 없다.**

즉 이 엔드포인트에서 내장 웹 검색은 **클라이언트가 선언하는 도구가 아니다.** 서버가 lite 봉투에서 직접 공급한다. 우리가 `tools`에 `{type:'web_search'}`를 넣어 온 접근 자체가 틀렸고, 그래서 태그 이름·접근 플래그·모델·계정을 아무리 바꿔도 검색이 0회였다.

네 번의 실패한 가설을 남긴다. 도구 태그 이름 → 접근 모드 플래그 → 모델 계열 → 계정·엔드포인트. 전부 요청 **내용**을 의심했는데 실제 차이는 요청 **봉투**였다. 대조 실험(codex CLI에서 같은 모델·같은 계정으로 검색 성공)이 앞의 둘을 죽였고, 로컬 캡처가 답을 줬다.

## 1단계 구현 — 검색 요청에만 lite 봉투

사용자 결정에 따라 검색 side query에만 적용한다. 일반 대화 경로는 건드리지 않는다.

- 그 요청은 최상위 `tools`를 보내지 않는다. 클라이언트 도구는 `additional_tools` 항목의 `functions` namespace 안으로 옮긴다. 선언된 도구가 사라지지는 않는다.
- `text: { verbosity: 'medium' }`를 함께 보낸다.
- 헤더 일곱 개를 덧붙인다: `x-openai-internal-codex-responses-lite`, `x-codex-beta-features`, `session-id`, `thread-id`, `x-client-request-id`, `x-codex-window-id`, `x-codex-turn-metadata`.
- **검색 도구 항목은 어디에도 보내지 않는다.** 기준 요청에도 없다.

식별자는 이 프로세스에서 생성한다. 사용자 codex 설치의 값을 복사하지 않으며, turn metadata에 로컬 경로·저장소·워크스페이스를 담지 않는다(`workspaces` 없음, 백슬래시 없음을 테스트로 고정). transport는 이 일곱 개 이름만 허용하고 값에 CR/LF가 있으면 거부하므로 임의 헤더가 주입될 수 없다.

## 남은 것 — 2단계

백엔드가 검색을 수행해도 그것만으로 WebSearch 결과가 채워지지는 않는다. Claude Code의 side query는 응답에서 Anthropic의 `web_search_tool_result` 블록을 읽어 `results`를 만든다. 우리는 그 블록을 만들지 않으므로 결과는 비어 있을 수 있다. 1단계는 **백엔드가 실제로 검색을 수행하는가**만 판정한다(`lifetime.webSearchCalls > 0`). 그것이 확인되면 2단계로 `url_citation` annotation을 Anthropic 결과 블록으로 옮기는 작업을 한다.

검증: 기준 11개와 선택·완료 4개 suite가 모두 통과했다. `test-native`가 봉투 형태, 헤더 집합, 식별자 생성, 로컬 경로 미포함을 고정한다. 실제 검색 수행 여부는 실행 1회로만 확인된다.

## 1단계 1차 결과 — 상류가 봉투를 거부했다

세션 `53f89b33`에서 관측했다. request 8, `success=false`, `failureStage=upstream`, `failureCategory=UPSTREAM_HTTP_ERROR`. WebSearch는 `Tool execution failed: API Error: 502`를 반환했다. 같은 실행의 request 7과 9(일반 대화)는 정상이었으므로 변경 범위는 의도대로 검색 요청에만 적용됐다.

조용한 무시가 아니라 명시적 거부라는 점이 중요하다. 이전에는 요청이 성공하면서 검색만 0회였다. 봉투가 이제 **평가되고 있다**는 뜻이고, 평가 결과 뭔가가 받아들여지지 않았다는 뜻이다.

이 실행이 계수 버그도 드러냈다. `lifetime.webSearchRequests`를 성공 경로에서만 올리고 있어서 행에는 `webSearchRequested=true`인데 누계는 `webSearchRequests=0`이었다. 요청을 만드는 지점에서 세도록 옮겼다. 상류에 거부당한 검색 요청이야말로 누계가 보여줘야 하는 요청이다.

## 2차 캡처 — 본문 차이 네 곳

로컬 싱크로 codex 요청을 다시 받아 스칼라 필드까지 대조했다.

| 필드 | 기준 클라이언트 | Clauduct 1차 |
| --- | --- | --- |
| `instructions` | 없음 | 있음 |
| `client_metadata` | 7키 | 없음 |
| `prompt_cache_key` | uuid 문자열 | 없음 |
| `reasoning` | `{effort, context:"all_turns"}` | `{effort}` |

네 곳을 한 번에 맞췄다. `instructions` 제거가 손실이 아닌 이유는 Clauduct가 실제 시스템 프롬프트를 `input`의 developer 메시지로 싣고 `instructions`에는 그것을 가리키는 고정 문장만 넣어 왔기 때문이다. 서버가 자기 기본 지시를 공급하는 lite 봉투에서 그 고정 문장은 불필요하고, 기준 클라이언트도 보내지 않는다.

`client_metadata`는 헤더와 같은 turn identity를 반복한다: `x-codex-installation-id`, `session_id`, `thread_id`, `turn_id`, `root_turn_id`, `x-codex-window-id`, `x-codex-turn-metadata`. 헤더와 같은 생성 결과를 쓰므로 둘이 어긋날 수 없다. 여전히 로컬 경로·저장소·워크스페이스는 담지 않는다.

## 측정 도구 — 프로브에 검색 모드

네 번의 실패한 가설이 모두 요청을 **측정하지 않고 추론**해서 나왔고, 유일한 측정 수단이 Claude 세션 한 판이었다. `verification/manual-http-probe.mjs --live --lite`가 SEND 한 번으로 게이트웨이와 같은 봉투를 보내고 다음을 보고한다.

- `httpStatus`와 category
- `webSearchCalls` — 백엔드가 실제로 수행한 검색 횟수
- 거부 시 `rejectedFields` — 상류 메시지가 언급한, **이 프로브가 스스로 보낸** 필드 이름들. 상류 텍스트는 절대 그대로 내보내지 않는다(테스트로 고정).

검색 모드는 가장 싼 모델로 돈다. 측정 대상은 답변 품질이 아니라 봉투 수용 여부와 검색 수행 여부다. 대화형 터미널과 명시적 SEND 확인이라는 기존 경계는 그대로다.

## 판정 갈래

- `webSearchCalls >= 1` → 1단계 성공. 2단계(인용 → Anthropic 블록)로 간다.
- `200`인데 `webSearchCalls = 0` → 봉투는 받아들여졌으나 백엔드가 검색하지 않은 것.
- 다시 오류 → 본문 차이는 소진했으므로 남은 용의자는 헤더와 `originator`다. 기준 클라이언트는 `originator: codex_exec`에 `Openai-Beta` 헤더가 없는데, Clauduct는 `originator: codex_cli_rs`에 `Openai-Beta: responses=experimental`를 보낸다. 일반 요청은 이 조합으로 잘 동작하므로 lite 봉투에서만 문제가 되는지는 측정해야 안다.

## 2차 측정 — 400, `tools`

프로브 검색 모드 첫 실행 결과다.

```
httpStatus 400  category HTTP_ERROR  jsonKind error  responseBytes 262
rejectedFields ["tools"]
clientVersion 0.154.0 (unverified; baseline 0.153.4)
```

상류 메시지가 `tools`를 언급하고 `additional_tools`는 언급하지 않았다. 우리가 보낸 namespace의 `tools`가 **빈 배열**이었다. 기준 클라이언트는 항상 도구를 실어 보내므로 빈 namespace는 캡처에 나타난 적이 없다.

검색 side query는 `web_search` 도구 하나만 선언하고 그 도구는 백엔드가 스스로 공급하므로 우리가 떨어뜨린다. 그러면 실을 게 남지 않는다. 그래서 **도구가 없으면 `additional_tools` 항목을 아예 보내지 않는다.** 항목의 존재 이유가 클라이언트 도구를 싣는 것이고, 실을 게 없으면 항목도 없다. 도구가 있으면 항목은 그대로 나가며 그 경우를 테스트로 고정했다.

라이브 502와 이 400은 같은 원인으로 보인다. 게이트웨이 경로에서도 side query의 도구 목록은 비어 있었다.

### 버전 드리프트가 발현했다

설치된 codex가 `0.154.0`으로 올라갔는데 프로브는 `0.153.4` 리터럴에 하드 핀이 걸려 있어 실행 자체를 거부했다(`CLI_VERSION_CHANGED`). 게이트웨이는 원래 실행 시점에 `codex --version`을 읽어 그 값을 보내고 기준선과 다르면 `CLI_VERSION_UNVERIFIED`만 알린다. 프로브도 같은 방식으로 바꿨다. 중단은 드리프트를 숨기고, 보고는 사용자가 결정하기 전 화면에 올린다. 읽기 불가·형식 오류는 여전히 멈춘다.

`REFERENCE_CLIENT_VERSION`은 `0.153.4`로 둔다. 끝까지 검증된 버전을 기록하는 값이고 `0.154.0`은 아직 검증되지 않았다.

**부수 확인: 502는 낡은 버전 헤더 탓이 아니다.** 게이트웨이는 실행 시점 버전을 보내므로 라이브 세션은 이미 `0.154.0`을 보내고 있었다.

### 측정 정밀도 보강

거부 시 상류 오류의 자체 어휘(`type`, `code`, `param`)를 고정 형태로 검사해 보고한다. `param`은 우리가 보낸 매개변수 경로라서 `input[0].tools` 같은 대괄호 표기를 허용한다. 메시지 본문은 재현하지 않으며 테스트가 이를 고정한다. 다음 거부가 오면 어느 필드인지 추론할 필요가 없다.

## 3차 측정 — 200, 그러나 검색 0회

```
httpStatus 200  category MISSING_CONTENT_TYPE  webSearchCalls 0
eventCount 11  modelMatches true  effortEchoMatches true  unexpectedTool false
usage 21/305/326  streamChars 2
```

**봉투는 받아들여졌다.** 빈 namespace가 400의 원인이었다는 진단이 맞았다. 그런데 백엔드가 검색을 하지 않고 305 토큰을 추론한 뒤 2글자로 답했다. 기억으로 답한 것이다.

## 검색은 요청에 선언되지 않는다

`web_search = "live"`와 `web_search = "disabled"`로 같은 요청을 각각 캡처해 본문을 대조했다.

```
key sets equal: true
차이: client_metadata (실행마다 달라지는 식별자와 타임스탬프뿐)
namespace: 양쪽 모두 functions:exec,wait,request_user_input / mcp__cua_repl:js,js_reset
헤더 이름 집합: 동일
```

**설정을 껐다 켜도 요청이 동일하다.** 즉 codex의 `web_search` 설정은 이 엔드포인트로 가는 요청에 아무 흔적도 남기지 않는다. 내장 검색은 전적으로 서버가 공급하며 요청이 선언하는 것이 아니다. 앞서 "검색 도구를 어떻게 선언할까"를 네 번 틀린 이유가 여기 있다 — 선언하는 물건이 아니었다.

## 남은 차이는 클라이언트 정체성뿐

lite 봉투는 `instructions`를 보내지 않는다. 서버가 지시를 공급한다는 뜻이고, **어떤 지시를 — 따라서 어떤 내장 도구 모음을 — 줄지는 헤더의 클라이언트 정체성을 따른다**고 보는 것이 남은 유일한 설명이다.

| 헤더 | 기준 클라이언트 | Clauduct(이전) |
| --- | --- | --- |
| `originator` | `codex_exec` | `codex_cli_rs` |
| `User-Agent` | `codex_exec/<v> (<os>; x86_64) xterm-256color (codex_exec; <v>)` | `codex-cli/<v> (Windows; x64)` |
| `Version` | 없음 | 있음 |
| `Openai-Beta` | 없음 | `responses=experimental` |

검색 요청만 기준 클라이언트의 정체성으로 보낸다. 일반 요청의 정체성은 한 글자도 건드리지 않으며 테스트가 양쪽을 동시에 고정한다. 선택 기준은 "고정 봉투 헤더를 하나라도 실었는가"다. 처음에 `extraHeaders !== undefined`로 판정했다가 루프백 전송이 항상 빈 객체를 넘기는 탓에 일반 요청까지 lite로 빠졌고, `client-version` suite가 그 자리에서 잡아냈다.

turn metadata에 기준 클라이언트가 담는 `agent_name`, `context_window_id`, `node_repl_auto_review_required`를 추가했다. `workspaces`는 여전히 담지 않는다 — 로컬 경로다.

## 결론 — 내장 검색은 lite에 없다. 전용 엔드포인트가 있다

공개 소스(openai/codex `da20788`, 2026-09-11)가 여섯 번의 가설을 한 번에 정리했다.

```rust
// "Responses Lite accepts schemas for client-executed tools, not hosted Responses tools."
if model_info.use_responses_lite || is_basic_session_source(...) { return Vec::new(); }
```

lite 봉투에서는 호스티드 `ToolSpec::WebSearch`가 **생성 자체가 차단**된다. 대신 클라이언트가 실행하는 `web.run` 도구가 `additional_tools`에 실리고, 코덱스 프로세스가 그 호출을 받아 `POST <base_url>/alpha/search`로 검색한다. 로컬 크롤링은 없다.

### 내 실험이 왜 전부 무효였나

```rust
available: (is_openai() || uses_openai_actor_authorization() || supports_standalone_web_search)
           && web_search_mode != WebSearchMode::Disabled
```

`supports_standalone_web_search`는 `#[serde(default)]` → 커스텀 `model_providers.*`에서 false다. 모든 캡처를 `model_provider=capture`로 떴으므로, 찾던 도구를 관측 방법이 지우고 있었다. "live와 disabled가 동일하다"도 여기서 나왔다 — 양쪽 다 억제된 상태였다. **관측 장치가 대상을 지우고 있는지 먼저 확인한다.**

### 라이브 확인

```
POST https://chatgpt.com/backend-api/codex/alpha/search
200 · results 6 · keys{domain, ref_id, snippet, title, type, url} · output 10003자 · encrypted_output 있음
```

### 채택한 구조

Claude Code의 WebSearch side query는 격리된 요청이다 — 메시지 하나(`Perform a web search for the query: <질의>`), 시스템 프롬프트 한 줄, 도구 하나. 그리고 응답을 줄이는 코드는 결과 블록에서 `title`과 `url`만 읽고 나머지는 주변 `text` 블록에서 가져간다.

```js
if (v.type === "web_search_tool_result") o.push({ tool_use_id: v.tool_use_id,
  content: v.content.map(I => ({ title: I.title, url: I.url })) });
if (v.type === "text") d += v.text;
```

그래서 게이트웨이가 그 side query를 직접 답한다. 질의를 뽑아 `alpha/search`에 한 번 던지고, `results`로 `web_search_tool_result`를, `output` 요약으로 뒤따르는 `text` 블록을 만든다. **추론 턴 없음 · 토큰 0 · HTTP 1회.** 네이티브의 `encrypted_content`(페이지 본문) 자리를 백엔드가 만든 요약이 채우므로 품질 격차도 닫힌다.

경계:

- 탐지는 클라이언트가 실제로 세우는 조건 전부가 맞아야 한다. 하나라도 어긋나면 모델 경로로 흘린다. `tool_choice`는 클라이언트가 항상 보내지는 않으므로(라이브 요청이 upstream 단계까지 갔다는 것이 증거다) 보낼 수 있는 형태만 허용한다.
- 질의 하나만 나간다. 기준 클라이언트는 대화 꼬리를 함께 보내지만 우리는 보내지 않는다.
- 검색 결과는 정의상 공격자가 쓴 웹 콘텐츠다. 길이·형태를 검사하고 비-http 스킴과 제어문자는 결과를 버린다. 링크도 텍스트도 없으면 `SEARCH_RESULTS_EMPTY`로 실패한다 — 조용한 빈 성공은 없다.
- 상태 응답에는 개수만 나간다. 질의도 결과도 나가지 않으며 테스트가 canary로 고정한다.

### 걷어낸 것

responses-lite 봉투 전체(본문·헤더 허용목록·전송 배선·프로브 `--lite` 모드·분기된 클라이언트 정체성). 동작할 수 없는 코드 215줄이다. 남은 것은 검색 요청의 turn identity 하나뿐이고 이름도 그 일에 맞게 바꿨다.

### 검증

`native-search` 8, `native-gateway` 61, `native-transport`(루프백 검색 포함), `native` 47, `manual-http-probe` 88 통과. 루프백 테스트가 버그 하나를 잡았다 — 이미 취소된 signal은 리스너가 발화하지 않아 취소된 검색이 요청을 보내고 있었다.

미검증: 실제 Clauduct 세션에서의 종단 동작. 게이트웨이 경로는 주입 전송으로, 전송 경로는 루프백으로, 엔드포인트는 프로브로 각각 확인했으나 셋을 한 번에 통과시킨 적은 없다.
