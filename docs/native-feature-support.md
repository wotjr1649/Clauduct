# Claude native 기능과 Clauduct 지원 범위 전수 대조

기준일 2026-09-11. 설치 Claude `C:/Users/JS/.local/bin/claude.exe`, 설치 Codex standalone `0.154.0`, Clauduct HEAD 기준이다. 실행 파일은 읽기 전용으로 조사했고 인증 저장 파일은 읽지 않았다.

## 1. 원칙 — 무엇이 바뀌고 무엇이 안 바뀌는가

Clauduct는 **추론 backend만 교체**한다. Claude Code → loopback gateway → 직접 HTTPS Codex responses 엔드포인트다. 따라서 판정은 딱 두 갈래로 갈린다.

- **클라이언트 로컬 기능**: Claude Code 프로세스 안에서 끝나는 것. Clauduct는 건드리지 않으므로 native와 동일하게 동작한다.
- **서버 의존 기능**: Anthropic 서버가 무언가를 해 줘야 하는 것. upstream이 Codex이므로 동작할 수 없고, 대부분 `anthropic-beta` 헤더로 요청되며 gateway가 거부한다. 예외는 상류에 동등한 기능이 있어 번역할 수 있는 경우이며, 현재 웹 검색이 그렇다.

## 2. 설정 소스 — 전역 settings인가, codex config.toml인가

**전역 Claude settings를 따른다. codex `config.toml`은 Claude 설정 소스가 아니다.**

[공식 우선순위](https://code.claude.com/docs/en/settings)는 높은 것부터 managed settings → **명령줄 `--settings`** → `.claude/settings.local.json` → `.claude/settings.json` → `~/.claude/settings.json`이다. `--settings`로 넘긴 JSON은 다른 소스를 대체하지 않고 **키 단위로 병합**된다. 설정한 키만 하위를 덮고 생략한 키는 하위 값을 그대로 쓴다.

Clauduct는 `--settings`에 다음만 넣는다: `env`(9개 키), `modelPicker`, `statusLine`, `hooks`(3종). 나머지 전역·프로젝트·로컬 설정은 평소대로 적용된다. 단 `modelPicker`는 공식적으로 병합되지 않는 키라 Clauduct 값이 사용자 설정을 대체한다(managed settings는 여전히 우선).

`--setting-sources`, `--settings`, `--agents`, `--system-prompt`는 사용자 입력에서 차단한다. `--setting-sources`를 Clauduct가 직접 지정하지도 않으므로 기본 소스 집합이 그대로 활성화된다. `CLAUDE_CONFIG_DIR`은 보존한다.

codex `config.toml`은 `poc/user-session.mjs`가 **`cli_auth_credentials_store` 한 줄만** 읽는다. 값이 `file`이 아니면 `CREDENTIAL_STORE_UNSUPPORTED`로 중단한다. 인증 캐시 위치 호환성 검사이며 모델·도구·권한·프롬프트 설정과 무관하다. codex의 `model`, `approval_policy`, `sandbox`, `project_doc_*`, `[profiles]` 같은 항목은 **읽지도 적용되지도 않는다.**

## 3. Hooks

**동작한다. 사용자 hook과 Clauduct hook이 함께 실행된다.**

hook은 Claude Code 프로세스가 직접 실행하며 gateway를 거치지 않는다. 설정 병합 규칙에서 [리스트 키는 병합](https://code.claude.com/docs/en/settings)되므로, `--settings`의 `hooks`는 사용자 `~/.claude/settings.json`과 프로젝트 설정의 hook을 대체하지 않고 더한다.

설치 바이너리에 존재하는 hook 이벤트는 13종이다: PreToolUse, PostToolUse, UserPromptSubmit, Notification, Stop, SubagentStart, SubagentStop, PreCompact, SessionStart, SessionEnd, InstructionsLoaded, ConfigChange, PermissionRequest.

Clauduct가 추가하는 것은 3종뿐이다.

| 이벤트 | matcher | 용도 |
|---|---|---|
| SubagentStart | `*` | 자식 등록과 역할·컨텍스트 정책 전달 |
| SubagentStop | `*` | 등록 해제 |
| PostToolUse | `Skill|SendMessage|Workflow` | skill fork·재개·workflow 결과 연결 |

나머지 10종은 Clauduct가 손대지 않으므로 사용자 설정 그대로 동작한다. SubagentStart/Stop이 실제로 발화한다는 증거는 실사용 세션의 `roleRegistered=true`와 `agentContextPolicy.evidence=subagent-start-hook-environment`다. 나머지 이벤트의 실제 발화는 이 저장소에서 별도로 검증하지 않았다(미검증).

hook이 누락되면 역할별 모델 배정만 적용되지 않고 요청은 계속된다. 이때 launcher가 stderr로 한 번 알린다.

## 4. CLAUDE.md / AGENTS.md

**CLAUDE.md는 로드한다. AGENTS.md는 Claude Code가 직접 읽지 않는다 — Clauduct와 무관한 native 동작이다.**

[공식 문서](https://code.claude.com/docs/en/memory)는 "Claude Code reads `CLAUDE.md`, not `AGENTS.md`"라고 명시한다. AGENTS.md를 쓰려면 CLAUDE.md에서 `@AGENTS.md`로 import하거나 symlink한다. Windows에서는 symlink에 관리자 권한이 필요하므로 import를 쓴다.

로드 대상은 managed policy CLAUDE.md, `~/.claude/CLAUDE.md`, `./CLAUDE.md` 또는 `./.claude/CLAUDE.md`, `./CLAUDE.local.md`이며 작업 디렉터리에서 위로 올라가며 모두 이어 붙인다. `.claude/rules/`와 auto memory도 native 기능이다.

Clauduct는 자식의 cwd를 `resolve(process.cwd())`로 그대로 넘기고 메모리 탐색에 개입하지 않으므로 **native와 동일하게 로드된다.** 예외는 `--document-first`를 켰을 때 `--append-system-prompt`로 문서 선로드 지침 한 개가 추가되는 것뿐이며, 이는 메모리 파일 탐색을 바꾸지 않는다.

설치 바이너리의 AGENTS.md 문자열은 codex→Claude Code 마이그레이션 기능(`/import`)의 것이다. 그 기능은 AGENTS.md 내용을 CLAUDE.md에 한 번 복사하며, 상시 로드 경로가 아니다.

## 5. anthropic-beta — 무엇이고 몇 개가 통과하는가

`anthropic-beta`는 [Anthropic API의 beta opt-in 헤더](https://platform.claude.com/docs/en/api/beta-headers)다. 아직 정식 출시되지 않은 기능을 `feature-name-YYYY-MM-DD` 형식의 이름으로 요청하며, 쉼표로 여러 개를 함께 보낸다. Claude Code는 켜져 있는 기능에 맞춰 이 헤더를 구성한다.

Clauduct에서 이 헤더는 **upstream으로 전달되지 않는다.** upstream이 Anthropic이 아니라 Codex이기 때문이다. gateway가 헤더를 읽어 세 갈래로 처리한다.

| 구분 | 개수 | 처리 |
|---|---|---|
| 허용 목록(`NATIVE_BETAS`) | **15** | 통과. `mid-conversation-tool-changes-2026-07-01`만 실제 동작(턴 중 도구 추가·제거)을 켠다 |
| 비호환 판정 목록 | **27** | 400 `UNSUPPORTED_BETA`로 거부. upstream 시도 없음 |
| 그 외(처음 보는 이름) | 가변 | 통과시키고 이름을 종료 JSON `unknownBetaNames`에 최대 8개 기록 |

허용 15개: `claude-code-20250219`, `interleaved-thinking-2025-05-14`, `context-management-2025-06-27`, `effort-2025-11-24`, `redact-thinking-2026-02-12`, `prompt-caching-scope-2026-01-05`, `mid-conversation-system-2026-04-07`, `thinking-token-count-2026-05-13`, `advanced-tool-use-2025-11-20`, `tool-search-tool-2025-10-19`, `per-turn-control-2026-07-01`, `mid-conversation-output-config-2026-07-01`, `mid-conversation-tool-changes-2026-07-01`, `oauth-2025-04-20`, `web-search-2025-03-05`.

거부 27개와 그 기능:

| 라벨 | 기능 | 라벨 | 기능 |
|---|---|---|---|
| EXTENDED_CACHE_TTL | 1시간 프롬프트 캐시 | AGENT_MEMORY | 서버측 agent 메모리 |
| CACHE_DIAGNOSIS | 캐시 진단 | MCP_SERVERS | 서버 실행 MCP 커넥터 |
| CACHE_EVICT | 캐시 축출 제어 | SERVER_SKILLS | 서버 호스팅 skill |
| THINKING_BINDING | thinking 바인딩 제어 | FILES_API | Files API 첨부 |
| THINKING_DISPLAY_UPDATES | thinking 표시 갱신 | SERVER_FALLBACK_V1/V2 | 서버측 모델 fallback |
| STRUCTURED_OUTPUTS | 구조화 출력 스키마 | FALLBACK_CREDIT | fallback 크레딧 |
| FAST_MODE | Fast mode | AFK_MODE | AFK 모드 |
| TASK_BUDGETS | 서버 task 예산 | DREAMING | dreaming |
| ADVISOR_TOOL | 서버 advisor 도구 | MANAGED_AGENTS | Managed Agents |
| USER_PROFILES | 사용자 프로필 | (이동) | `WEB_SEARCH`는 Codex 내장 검색으로 브리지해 허용으로 옮겼다 |
| TOKEN_COUNTING | count_tokens API | CONTEXT_HINT | context hint |
| SYSTEM_CLEAR_AT | 턴 중 system clear | SERVER_COMPACT | 서버측 압축 |
| CONTEXT_1M | 1M 컨텍스트 창 | AUTO_CLASSIFIER | auto 모드 분류기 |
| DANGEROUS_TOOL_USE | dangerous tool use | | |

전부 Anthropic 서버가 수행해야 하는 기능이라 Codex backend에서 구현할 수 없다. `ADVISOR_TOOL`은 `CLAUDE_CODE_DISABLE_ADVISOR_TOOL=1`로 자식에서 미리 끈다. 나머지는 해당 기능을 켜고 요청하면 그 요청이 400으로 거부된다.

## 6. 요청 표면 — gateway가 받아들이는 것

`prepareNative`가 허용하는 최상위 필드는 `model`, `messages`, `system`, `max_tokens`, `stream`, `tools`, `tool_choice`, `thinking`, `metadata`, `output_config`, `context_management`다. 콘텐츠 블록은 `text`, `image`, `tool_use`, `tool_result`, `redacted_thinking`, `tool_addition`, `tool_removal`의 7종이다.

| 기능 | 판정 | 근거 |
|---|---|---|
| 스트리밍 대화 | 지원 | `stream: true`만 허용. false/누락/비정상은 `REQUEST_STREAM_*`로 거부하고 upstream 시도 없음 |
| 도구 호출·결과 왕복 | 지원 | tool_use/tool_result, 병렬 호출, `tool_choice` auto/any/none/tool |
| ToolSearch·지연 로딩 도구 | 지원 | `defer_loading`과 `tool_reference` |
| 턴 중 도구 추가·제거 | 조건부 | `mid-conversation-tool-changes` beta가 있을 때만 |
| 이미지 입력 | 지원 | base64 png/jpeg/gif/webp만 |
| thinking / reasoning 재생 | 지원 | `adaptive`/`enabled`/`disabled`, encrypted reasoning 왕복 |
| effort 지정 | 지원 | `output_config.effort`, system 메시지의 턴별 effort |
| 로컬 MCP 도구 | 지원(미검증) | `--mcp-config` 전달, `mcp__*` 이름의 일반 도구로 변환 |
| 프롬프트 캐시 지시 | 부분 | `cache_control` type=ephemeral, ttl 5m/1h만 통과. 실제 캐시는 Codex 정책 |
| temperature / top_p / stop_sequences | 미지원 | `UNSUPPORTED_SAMPLING` |
| 비스트리밍 요청 | 미지원 | 자식에 `CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK=1` 적용 |
| `/v1/messages` 외 경로 | 미지원 | `UNSUPPORTED_ROUTE`. count_tokens 포함 |
| `context_management` 편집 | 부분 | `clear_thinking_20251015 / keep:all` 무연산 하나만 |

## 7. 클라이언트 로컬 기능 — 영향 없음

다음은 Claude Code 프로세스 안에서 끝나므로 Clauduct가 바꾸지 않는다. 파일 도구(Read/Edit/Write/Glob/Grep), Bash와 백그라운드 실행, 권한 검사와 permission mode, plan mode, 슬래시 명령, 로컬 skill과 plugin, 로컬 MCP 서버 기동, hooks, CLAUDE.md·rules·auto memory, `/context`·`/memory`·`/doctor` 같은 진단 명령, 세션 저장과 `--continue`/`--resume`, statusline, 터미널 UI.

단 이들이 만들어 내는 **모델 요청**은 모두 gateway를 지나므로 6장의 제약을 함께 받는다. 예를 들어 skill 자체는 로컬 파일이지만 skill이 유도한 대화는 Codex 모델이 처리한다.

## 8. 집계

| 축 | 사용 가능 | 사용 불가 | 비고 |
|---|---|---|---|
| beta 기능 | 15 허용 + 서버의존 7 + 처음 보는 이름 통과 | **27** 명시 거부 | 거부는 전부 서버 의존 기능 |
| hook 이벤트 | **13 전부** | 0 | Clauduct는 3종을 추가할 뿐 |
| 메모리 파일 | CLAUDE.md 계층 전부 | AGENTS.md 직접 로드 | native 자체 제약이며 import로 우회 |
| 설정 소스 | 5개 계층 전부 | 없음 | `--settings`는 병합, `modelPicker`만 대체 |
| 요청 콘텐츠 블록 | 7종 | 그 외 `UNSUPPORTED_CONTENT` | |
| 샘플링 파라미터 | effort | temperature/top_p/stop_sequences | |

## 9. 미검증으로 남는 것

hook 13종 중 SubagentStart/SubagentStop/PostToolUse 외 10종의 실제 발화, 로컬 MCP 도구의 실제 왕복, 이미지 입력의 실제 왕복, 프롬프트 캐시의 실제 적중, plan mode·permission mode의 실제 동작, 설치된 skill·plugin 각각의 실제 실행. 모두 이 저장소에서 직접 확인하지 않았으며 정상 사용 중 관측되면 그때 기록한다.

## 10. 추가 확정 사항 — 2026-09-11 2차 조사

### 웹 검색과 웹 가져오기

`WebFetch`는 클라이언트가 직접 가져와 모델 호출로 요약하므로 **이미 Codex로 간다**(실제 왕복 미검증). `WebSearch`는 Anthropic 서버 도구였으나 이제 **Codex 내장 검색으로 브리지**한다. [상세](audit-2026-09-11-web-search-bridge.md). 사용자는 답변 텍스트로 결과를 받지만 출처 카드·인용 링크는 아직 보이지 않는다.

### 서버 의존이지만 거부하지 않는 7개

`ccr-byoc-2025-07-29`(BYOC), `ccr-triggers-2026-01-30`(원격 트리거), `environments-2025-11-01`(클라우드 환경), `mcp-tunnels-2026-06-22`(MCP 터널), `message-batches-2024-09-24`(Batch API), `message-threads-2026-08-12`(메시지 스레드), `oidc-federation-2026-04-01`(OIDC 연합). 모두 Anthropic 서버가 필요해 이 백엔드에서는 이미 무력하다. 헤더를 거부하면 기능이 꺼지는 게 아니라 요청 전체가 실패하므로 **표시만 하고 통과시킨다.**

### 신규 기능 감지

`node src/scan-native-features.mjs`가 설치 바이너리를 읽기 전용으로 훑어 beta 형태 이름을 추출하고 허용·거부·서버의존 목록과 대조한다. 실행하지 않으며 인증 파일도 읽지 않는다. 현재 결과는 관측 50 / 허용 15 / 거부 27 / 서버의존 7 / 미분류 0이다. 미분류가 나오면 stderr로 알리지만 **회귀 테스트를 실패시키지는 않는다.** Claude를 업데이트한 뒤 한 번 돌려 보는 용도다.

### Anthropic 측 보고 차단

자식 세션에만 `DISABLE_TELEMETRY=1`, `DISABLE_ERROR_REPORTING=1`을 적용한다. 전역 설정은 바꾸지 않는다.
