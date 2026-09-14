# Claude native 기능과 Clauduct 지원 범위 전수 대조

현행 대조일 2026-09-12. 실제 검증 버전은 Claude `2.1.269`, Codex standalone `0.154.0`이다. 2026-09-11의 정적 조사에 새 비대화형 실행 증거를 반영했다. [릴리즈 검증](release-readiness.md)에 성공·실패·미검증 범위를 나누어 기록한다.

2026-09-14 Session-29에서는 Claude2.1.270의 공개 native 중첩 결과 중계와 새 프로세스의 일반 개발 복구를 검사했다. JPEG/GIF/WebP 각 두 모델의 기존 실제 왕복도 원결과로 대조했다. 동일 저장 세션에서의 Workflow 재개도 현재 bypass에서 두 모델의 공개 응답으로 확인했다. 저장 결과를 재사용하고 실패 단계만 재실행했다. 다른 잔여 요구의 현재 판정은 [Session-29](session-29-release-verdict.md)에 있으며 전체 HOLD다.

현재 사용자는 이 머신의 전역 `settings.json`에 있는 bypass 모드만 사용한다. 실제 `permissions.defaultMode=bypassPermissions`를 읽어 확인했으며 다른 permission 모드·전환 조합은 Session-29 출하 범위에서 제외한다. 아래 다른 모드에 관한 미검증 설명은 이력이고 현재 차단이 아니다. Clauduct의 launcher는 permissions를 재정의하거나 설정 소스를 대체하지 않는다.

## 1. 원칙 — 무엇이 바뀌고 무엇이 안 바뀌는가

Clauduct는 **추론 backend만 교체**한다. Claude Code → loopback gateway → 직접 HTTPS Codex responses 엔드포인트다. 따라서 판정은 딱 두 갈래로 갈린다.

- **클라이언트 로컬 기능**: 도구 실행·권한 검사·프로필 관리는 native가 담당한다. 모델 요청·결과 소비가 gateway와 맞아야 하므로 로컬 기능이라는 이유만으로 동일 동작을 단정하지 않는다.
- **서버 의존 기능**: Anthropic 전용 실행·저장·계정 서비스는 제공하지 않는다. beta 이름만으로 요청을 거부하지는 않으며, 실제 요청/응답의 미지원 필드·경로는 계속 거부한다. 웹 검색은 Codex 검색 endpoint로 별도 변환한다.

## 2. 설정 소스 — 전역 settings인가, codex config.toml인가

**전역 Claude settings를 따른다. codex `config.toml`은 Claude 설정 소스가 아니다.**

[공식 우선순위](https://code.claude.com/docs/en/settings)는 높은 것부터 managed settings → **명령줄 `--settings`** → `.claude/settings.local.json` → `.claude/settings.json` → `~/.claude/settings.json`이다. `--settings`로 넘긴 JSON은 다른 소스를 대체하지 않고 **키 단위로 병합**된다. 설정한 키만 하위를 덮고 생략한 키는 하위 값을 그대로 쓴다.

Clauduct의 `--settings` 최상위 키는 `env`, `modelPicker`, `hooks`(3종)다. 정확한 환경 키는 `src/clauduct.mjs`의 `interactiveLaunch`에 있다. 개인 statusLine 스크립트의 강제 지정은 제거했고 native 사용자 설정을 따른다. 나머지 소스는 유지하며 managed 설정의 우선순위와 보안 예외를 따른다.

`--setting-sources`, `--settings`, `--agents`, `--system-prompt`는 사용자 입력에서 차단한다. `--setting-sources`를 Clauduct가 직접 지정하지도 않으므로 기본 소스 집합이 그대로 활성화된다. `CLAUDE_CONFIG_DIR`은 보존한다.

codex `config.toml`은 `poc/user-session.mjs`가 **루트 `cli_auth_credentials_store` 선택에만 사용한다.** `auth-store-selection.mjs`는 따옴표·escape·주석·여러 줄 문자열·중첩 값과 테이블의 구조를 읽어 해당 키를 찾는다. 명시한 다른 저장소는 `CREDENTIAL_STORE_UNSUPPORTED`, 중복·잘못된 타입·지원하지 않는 인증 설정은 `CONFIG_UNSUPPORTED`로 캐시 조회 전에 중단한다. 전체 TOML 설정 검증기는 아니다. codex의 `model`, `approval_policy`, `sandbox`, `project_doc_*`, `[profiles]` 같은 다른 값은 **구조를 건너뛰며 Clauduct의 모델·도구·권한·프롬프트 설정으로 적용하지 않는다.**

## 3. Hooks

**동작한다. 사용자 hook과 Clauduct hook이 함께 실행된다.**

hook은 Claude Code 프로세스가 직접 실행하며 gateway를 거치지 않는다. 설정 병합 규칙에서 [리스트 키는 병합](https://code.claude.com/docs/en/settings)되므로, `--settings`의 `hooks`는 사용자 `~/.claude/settings.json`과 프로젝트 설정의 hook을 대체하지 않고 더한다.

과거 정적 조사에서 찾은 13개 이름을 현재 native의 전체 이벤트 수로 사용하지 않는다. Clauduct가 추가하는 이벤트와 실제 검증된 발화를 구분한다.

Clauduct가 추가하는 것은 3종뿐이다.

| 이벤트 | matcher | 용도 |
|---|---|---|
| SubagentStart | `*` | 자식 등록과 역할·컨텍스트 정책 전달 |
| SubagentStop | `*` | 등록 해제 |
| PostToolUse | `Skill|SendMessage|Workflow|TaskOutput` | skill fork·재개·workflow 및 수집한 자식 결과 연결 |

SubagentStart/Stop이 실제로 발화한다는 증거는 실사용 세션의 `roleRegistered=true`와 `agentContextPolicy.evidence=subagent-start-hook-environment`다. 2026-09-12 새 프로필의 실제 Agent도 `definition-model` 라우팅과 정상 종료를 확인했다. 임의 사용자 hook 각각의 발화는 별도 미검증이다.

정식 launcher의 자식 선택 검증에서는 등록·호출·metadata를 연결하지 못한 자식이 `AGENT_SELECTION_UNVERIFIED`로 거부된다. 다른 모델로 임의 배정하거나 hook 신뢰·권한을 자동 변경하지 않는다. 선택 검증기를 넣지 않은 합성 fixture의 fallback 동작을 실제 launcher의 계약으로 해석하지 않는다.

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
| 비호환 기능 판정 목록(`UNSUPPORTED_BETAS`) | **27** | 이름만으로 거부하지 않고 고정 `judgedBetaLabels`를 기록. 해당 서버 기능의 구현을 뜻하지 않음 |
| 그 외(처음 보는 이름) | 가변 | 통과시키고 이름을 종료 JSON `unknownBetaNames`에 최대 8개 기록 |

허용 15개: `claude-code-20250219`, `interleaved-thinking-2025-05-14`, `context-management-2025-06-27`, `effort-2025-11-24`, `redact-thinking-2026-02-12`, `prompt-caching-scope-2026-01-05`, `mid-conversation-system-2026-04-07`, `thinking-token-count-2026-05-13`, `advanced-tool-use-2025-11-20`, `tool-search-tool-2025-10-19`, `per-turn-control-2026-07-01`, `mid-conversation-output-config-2026-07-01`, `mid-conversation-tool-changes-2026-07-01`, `oauth-2025-04-20`, `web-search-2025-03-05`.

판정 라벨 27개와 그 기능(헤더 거부 목록이 아니다):

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

이 표는 Anthropic 기능 이름에 대한 진단 분류이며 각 기능을 구현한 목록이 아니다. `ADVISOR_TOOL`은 `CLAUDE_CODE_DISABLE_ADVISOR_TOOL=1`로 자식에서 미리 끈다. `betaFailure`가 거부하는 것은 빈 항목·중복 항목 같은 헤더 형식 오류다. 나머지는 실제 payload·경로의 지원 여부로 판정한다.

## 6. 요청 표면 — gateway가 받아들이는 것

`prepareNative`의 최상위 필드 검사는 `model`, `messages`, `system`, `max_tokens`, `stream`, `tools`, `tool_choice`, `thinking`, `metadata`, `output_config`, `context_management`, `temperature`, `top_p`, `stop_sequences`를 인식한다. 마지막 세 필드는 별도 샘플링 값 검사에서 제한된다. 콘텐츠 블록은 `text`, `image`, `tool_use`, `tool_result`, `redacted_thinking`, `tool_addition`, `tool_removal`의 7종이다. `tool_reference`는 tool_result 안에서만 처리한다.

| 기능 | 판정 | 근거 |
|---|---|---|
| 스트리밍 대화 | 지원 | `stream: true`만 허용. false/누락/비정상은 `REQUEST_STREAM_*`로 거부하고 upstream 시도 없음 |
| 도구 호출·결과 왕복 | 지원 | tool_use/tool_result, 병렬 호출, `tool_choice` auto/any/none/tool. 실제 Read→Edit, Bash 검사 통과 |
| ToolSearch·지연 로딩 도구 | 지원 | `defer_loading`과 `tool_reference` |
| 턴 중 도구 추가·제거 | 조건부 | `mid-conversation-tool-changes` beta가 있을 때만 |
| 이미지 입력 | 지원 | base64 png/jpeg/gif/webp. 실제 Read→PNG 이미지 입력→색상 답변 검증. 다른 세 포맷의 실제 왕복은 미검증 |
| thinking / reasoning 재생 | 지원 | `adaptive`/`enabled`/`disabled`, encrypted reasoning 왕복 |
| effort 지정 | 지원 | `output_config.effort`, system 메시지의 턴별 effort |
| 로컬 MCP 도구 | 지원 | 프로젝트 `.mcp.json`으로 발견한 stdio 도구 실제 1회 왕복 검증. `--mcp-config`는 차단 옵션이며 전달하지 않음 |
| 비대화형 실행·세션 재개 | 지원 | `-p`의 JSON stdout 분리, 실제 새 프로세스 `--resume`, 비활성 도구의 과거 결과 보존, 오류 후 재개·도구 미중복 검증 |
| 프롬프트 캐시 지시 | 부분 | `cache_control` type=ephemeral, ttl 5m/1h만 통과. 실제 캐시는 Codex 정책 |
| temperature / top_p / stop_sequences | 미지원 | `UNSUPPORTED_SAMPLING` |
| 비스트리밍 요청 | 미지원 | 자식에 `CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK=1` 적용 |
| HTTP 경로 | 제한 | `/v1/messages`, `/v1/models`, 내부 상태·라우팅 등록, `HEAD /api/hello`만 구현. count_tokens 등 그 외는 `UNSUPPORTED_ROUTE` |
| `context_management` 편집 | 부분 | `clear_thinking_20251015 / keep:all` 무연산 하나만 |

## 7. 클라이언트 로컬 기능 — native 소유, 호환성은 별도 검증

파일 도구(Read/Edit/Write/Glob/Grep), Bash와 백그라운드 실행, 권한 검사·plan mode, 슬래시 명령, 로컬 skill/plugin/MCP, hooks, 메모리 파일·진단 명령, 세션 저장·재개, statusline, 터미널 UI는 native가 실행한다. 사용자·프로젝트 설정을 유지하지만, launcher의 [옵션 경계](claude-option-classification.md)와 provider/secret 계열 환경 변수 제거는 적용된다.

단 이들이 만들어 내는 **모델 요청**은 모두 gateway를 지나므로 6장의 제약을 함께 받는다. 예를 들어 skill 자체는 로컬 파일이지만 skill이 유도한 대화는 Codex 모델이 처리한다.

## 8. 집계

| 축 | 사용 가능 | 사용 불가 | 비고 |
|---|---|---|---|
| beta 헤더 | 정상 형식의 이름 통과 | 빈/중복 항목 거부 | 27개는 기능 진단 분류. 통과가 서버 기능 지원을 뜻하지 않음 |
| hook 이벤트 | native 설정 유지, Clauduct 3종 추가 | native 정책으로 거부될 수 있음 | 사용자 hook 전수 실측 아님 |
| 메모리 파일 | CLAUDE.md 계층 전부 | AGENTS.md 직접 로드 | native 자체 제약이며 import로 우회 |
| 설정 소스 | 5개 계층 전부 | 없음 | `--settings`는 병합, `modelPicker`만 대체 |
| 요청 콘텐츠 블록 | 7종 | 그 외 `UNSUPPORTED_CONTENT` | |
| 샘플링 파라미터 | effort | temperature/top_p/stop_sequences | |

## 9. 미검증으로 남는 것

JPEG/GIF/WebP는 sol/low·luna/max 각각 실제 왕복의 결과·modelMatched·featureVerified·cleanupComplete를 2026-09-14 재대조했다. 사용자 hook 각각의 발화, 프롬프트 캐시의 실제 적중, plan mode·permission mode의 UI 동작, 설치된 skill·plugin 각각의 실제 실행은 미검증이다. 로컬 stdio MCP와 PNG 입력의 기존 실제 왕복도 유지한다. 외부 계정·서버 기능과 [차단된 symlink 검사](remaining-verification.md)는 통과로 표시하지 않는다.

## 10. 추가 확정 사항 — 2026-09-11 2차 조사

### 웹 검색과 웹 가져오기

`WebFetch`는 클라이언트가 가져온 내용을 모델 호출로 처리하고, `WebSearch`의 side query는 **Codex 검색 endpoint로 브리지**한다. [상세](audit-2026-09-11-web-search-bridge.md). 2026-09-12 실제 WebFetch 1회·제목 답변과 WebSearch 1회·검색 링크 반환·최종 답변을 모두 검증했다. 이전 실제 인용 관측은 [현행 기준표](remaining-verification.md)에 있다. 터미널의 출처 카드 렌더링을 새 비대화형 검증으로 입증하지 않는다.

### 서버 의존이지만 거부하지 않는 7개

`ccr-byoc-2025-07-29`(BYOC), `ccr-triggers-2026-01-30`(원격 트리거), `environments-2025-11-01`(클라우드 환경), `mcp-tunnels-2026-06-22`(MCP 터널), `message-batches-2024-09-24`(Batch API), `message-threads-2026-08-12`(메시지 스레드), `oidc-federation-2026-04-01`(OIDC 연합). 모두 Anthropic 서버가 필요해 이 백엔드에서는 이미 무력하다. 헤더를 거부하면 기능이 꺼지는 게 아니라 요청 전체가 실패하므로 **표시만 하고 통과시킨다.**

### 신규 기능 감지

`node src/scan-native-features.mjs`가 설치 바이너리를 읽기 전용으로 훑어 beta 형태 이름을 추출하고 알려진 이름·판정·서버의존 목록과 대조한다. 현재 실측은 관측 50 / allowed 14 / judged 27 / serverDependent 7 / unclassified 0이다. 나머지 2개는 scanner의 제외 후보다. `NATIVE_BETAS`는 15개지만 `claude-code-20250219`는 scanner 날짜 패턴과 달라 관측 allowed 수에 들지 않는다. 이름 집계는 기능 지원 수가 아니다. 미분류가 나오면 stderr로 알리며 Claude 업데이트 후 확인한다.

### Anthropic 측 보고 차단

자식 세션에만 `DISABLE_TELEMETRY=1`, `DISABLE_ERROR_REPORTING=1`을 적용한다. 전역 설정은 바꾸지 않는다.
