# Claude CLI 옵션 분류 — Clauduct에서 무엇을 쓸 수 있는가

기준 `claude --help`: 2026-09-12 수집본. 출처는 그 도움말 텍스트이며, 도움말이 말하지 않는 것은 아래에서 **불확실**로 표시한다.

## 1. 왜 가르는가

Clauduct는 자식 Claude에게 `ANTHROPIC_BASE_URL`을 로컬 게이트웨이로 주고, 그 게이트웨이가 Codex 계열 백엔드로 보낸다. 따라서 옵션 하나가 세 가지 중 하나를 깰 수 있다.

1. **경유 이탈** — 클라이언트가 게이트웨이를 지나지 않고 Anthropic·claude.ai·외부 URL에 직접 닿는다. 데이터 경계가 깨지고 인증도 맞지 않는다.
2. **계약 파괴** — Clauduct가 고정한 설정(`--settings`, hook, 모델·effort, 컨텍스트 창)을 덮거나 없앤다.
3. **파싱 이탈** — 값을 먹는 옵션인데 래퍼가 그걸 모르면 **그 값이 다음 플래그로 오인된다.** 3장이 이 문제다.

분류는 1·2를 기준으로 하고, 3은 별도로 목록화한다.

## 2. A — Clauduct가 소유 (자식에게 전달하지 않음)

| 옵션 | 비고 |
|---|---|
| `--model` `--effort` | 래퍼가 별칭을 실제 ID로 바꿔 자식에 직접 넘긴다 |
| `--dry-run` `--help` | 래퍼 전용 |
| `--verify-auto-compact` | 축소 창(100K) 검증 |
| `--verify-agent-models` | 모델 진입점 프로브 등록 |
| `--verify-fallback blocked\|allowed` | 스트림 오류 주입 + fallback 설정 팔. 인자 없이는 안 돈다 |
| `--gpt-agents` `--document-first` | 래퍼 전용 |

## 3. B — 차단해야 함: 경유 이탈 (도움말이 스스로 말함)

여기 있는 것은 추정이 아니라 도움말 원문이 cloud·remote·download·install을 명시한 것들이다.

| 옵션/명령 | 도움말 근거 |
|---|---|
| `--cloud` | "Create a cloud session" |
| `--environment` | "Create a new cloud session ... self-hosted environment (ccpool_...)" |
| `--teleport` | "Resume a teleport session" |
| `--remote-control` / `--remote-control-session-name-prefix` | Remote Control 릴레이 |
| `--from-pr` | PR에 연결된 세션 복귀 — 외부 조회 |
| `--file` | "File resources to download at startup" (`file_id:` 형식) |
| `--plugin-url` | "Fetch a plugin .zip from a URL" — 외부 egress + 원격 코드 |
| `auth` `setup-token` | Anthropic 인증 |
| `install` `update`/`upgrade` | 네이티브 빌드 내려받기 |
| `ultrareview` | "cloud-hosted multi-agent code review" |
| `gateway` | "enterprise auth/telemetry gateway" |

## 4. C — 차단해야 함: Clauduct 계약 파괴

| 옵션 | 무엇이 깨지는가 |
|---|---|
| `--settings` | 래퍼가 모델·env·hook·statusLine을 이 경로로 심는다. **이미 차단 중** |
| `--agents` | 래퍼가 검증 프로브 정의를 넣는다. **이미 차단 중** |
| `--system-prompt` | 기본 시스템 프롬프트 교체. **이미 차단 중** |
| `--setting-sources` | 설정 출처 선택. **이미 차단 중** |
| `--bare` | "skip hooks" — Clauduct의 `SubagentStart`/`PostToolUse` 라우팅 hook이 사라져 자식 모델 선택이 무너진다. 인증도 `ANTHROPIC_API_KEY`/apiKeyHelper로 좁힌다 |
| `--safe-mode` | hook·플러그인·에이전트 정의를 전부 끈다. 같은 이유 |
| `--autocompact` | 래퍼가 `CLAUDE_CODE_AUTO_COMPACT_WINDOW`를 정한다. 5.3 절차도 이 값을 지정하지 말라고 못박았다 |
| `--fallback-model` | 과부하 시 다른 모델로 자동 전환 — 게이트웨이의 모델 계약 밖으로 나간다(`unmappedAgentModels`로 잡히지만 애초에 보내지 않는 편이 낫다) |

## 5. D — 전달해도 됨: 로컬에서 끝남

경유 이탈도 계약 파괴도 아니다. 값을 먹는 것은 3장 목록에 반드시 등록되어야 한다.

`--add-dir` `--agent` `--allowedTools`/`--allowed-tools` `--append-system-prompt`
`--ax-screen-reader` `--bg`/`--background` `--continue`/`-c` `--debug`/`-d` `--debug-file`
`--disable-slash-commands` `--disallowedTools`/`--disallowed-tools`
`--exclude-dynamic-system-prompt-sections` `--fork-session` `--forward-subagent-text`
`--ide` `--include-hook-events` `--include-partial-messages` `--input-format`
`--max-budget-usd` `--name`/`-n` `--no-session-persistence` `--output-format`
`--permission-prompts` `--print`/`-p` `--replay-user-messages` `--resume`/`-r`
`--session-id` `--strict-mcp-config` `--system-prompt-snapshot` `--tmux` `--tools`
`--verbose` `--version`/`-v`

`--append-system-prompt`는 `--document-first`와 동시에 쓰면 래퍼가 이미 거부한다.

## 6. E — 정책 판단이 필요한 것 (기술적으로는 로컬)

| 옵션 | 무엇을 물어야 하나 |
|---|---|
| `--dangerously-skip-permissions` / `--allow-dangerously-skip-permissions` | 권한 검사를 끈다. 로컬이지만 S8 경계와 정면으로 만난다 |
| `--permission-mode` | 같은 축의 약한 버전 |
| `--mcp-config` | MCP 서버는 원격일 수 있다. 로컬 파일을 읽지만 그 안의 서버가 egress다 |
| `--plugin-dir` | 로컬이지만 플러그인 코드를 실행한다 |
| `--worktree` / `-w` | 저장소에 워크트리를 만든다. 사용자 상태를 건드린다 |
| `--restricted` | 도구를 줄이고 user/project/local 설정을 무시한다. `--settings`는 살아남으므로 계약은 유지되지만 도구 계약이 바뀐다 |
| `--betas` | 게이트웨이가 미지의 beta를 이름만으로 거부하지 않는 것은 검증됐다. 다만 계약을 바꾸는 beta면 뒤에서 거부된다 |
| `--prompt-suggestions` | 턴마다 예측 프롬프트를 만든다 — 게이트웨이를 거치는 **추가 모델 요청**이다. 경유 이탈은 아니고 비용 문제다 |

## 7. F — 불확실 (도움말만으로 판정 불가)

| 옵션 | 왜 불확실한가 |
|---|---|
| `--chrome` / `--no-chrome` | "Claude in Chrome integration"이 확장·중계 서버를 쓰는지 도움말이 말하지 않는다. 확인 전에는 쓰지 않는다 |
| `--json-schema` | 구조화 출력 검증이 요청 본문에 실리면 게이트웨이의 고정 필드 허용목록(`REQUEST_FIELDS`)에 걸린다. 조용히 새지는 않고 시끄럽게 거부될 가능성이 크지만 실측하지 않았다 |
| `--brief` | `SendUserMessage` 도구를 켠다. 로컬 도구로 보이나 전달 경로를 확인하지 않았다 |

서브커맨드(`agents` `attach` `logs` `rm` `stop` `respawn` `doctor` `mcp` `plugin` `project` `import` `auto-mode`)는 경유 문제는 없어 보이나, **Clauduct는 대화형 세션을 띄우는 래퍼라 서브커맨드를 받으면 그 세션 자체가 성립하지 않는다.** 분류와 별개로 래퍼에서는 의미가 없다.

## 8. 현재 코드와의 차이 — 실측

`src/clauduct.mjs`의 세 집합을 2026-09-12 도움말과 대조했다.

**차단 중**: `--settings` `--agents` `--system-prompt` `--setting-sources` 넷뿐이다. 3장(경유 이탈) 11개와 4장의 나머지 4개는 **지금 그대로 자식에게 전달된다.**

**값을 먹는데 추적되지 않는 옵션 21개** — 그 값이 다음 플래그로 오인된다.

```
--agent --allowed-tools --autocompact --cloud --debug --disallowed-tools
--environment --file --from-pr --json-schema --name --permission-prompts
--plugin-dir --plugin-url --prompt-suggestions --remote-control
--remote-control-session-name-prefix --session-id --system-prompt-snapshot
--teleport --worktree
```

주석이 경고하는 상황이 실재한다 — 예컨대 `--name --model` 같은 입력에서 `--model`이 값인지 래퍼 옵션인지 지금은 구분하지 못한다.

**더 이상 도움말에 없는데 추적 중인 것 2개**: `--max-turns` `--permission-prompt-tool`. 무해하지만 목록이 낡았다는 표시다.

## 9. 이 문서가 하지 않는 것

분류일 뿐 강제가 아니다. `blockedOptions`와 `nativeValueOptions`를 여기에 맞추는 것은 별도 결정이며, 특히 6장(정책 판단)은 사용자가 정할 문제다. 강제하기 전에는 위 3·4장 항목이 계속 전달된다는 사실을 그대로 둔다.
