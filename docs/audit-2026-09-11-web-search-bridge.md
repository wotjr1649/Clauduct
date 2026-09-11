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
