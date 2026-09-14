# Claude CLI 옵션 분류 — Clauduct에서 무엇을 쓸 수 있는가

분류 기준 `claude --help`: 2026-09-12 수집본. 도움말이 말하지 않는 것은 **불확실**로 표시한다. 8장은 현재 구현과 수정 전 관측을 구분한다.

## 1. 왜 가르는가

Clauduct는 자식 Claude에게 `ANTHROPIC_BASE_URL`을 로컬 게이트웨이로 주고, 그 게이트웨이가 Codex 계열 백엔드로 보낸다. 따라서 옵션 하나가 세 가지 중 하나를 깰 수 있다.

1. **경유 이탈** — 클라이언트가 게이트웨이를 지나지 않고 Anthropic·claude.ai·외부 URL에 직접 닿는다. 데이터 경계가 깨지고 인증도 맞지 않는다.
2. **계약 파괴** — Clauduct가 고정한 설정(`--settings`, hook, 모델·effort, 컨텍스트 창)을 덮거나 없앤다.
3. **파싱 이탈** — 값을 먹는 옵션인데 래퍼가 그걸 모르면 **그 값이 다음 플래그로 오인된다.** 3장이 이 문제다.

분류는 1·2를 기준으로 한다. 값을 소비하는 전달 옵션은 `nativeValueOptions`로 따로 추적한다.

## 2. A — Clauduct가 소유 (자식에게 전달하지 않음)

| 옵션 | 비고 |
|---|---|
| `--model` `--effort` | 래퍼가 별칭을 실제 ID로 바꿔 자식에 직접 넘긴다 |
| `--dry-run` `--help` | 래퍼 전용 |
| `--verify-auto-compact` | 축소 창(100K) 검증 |
| `--verify-agent-models` | 모델 진입점 프로브 등록 |
| `--verify-fallback blocked\|allowed` | 스트림 오류 주입 + fallback 설정 팔. 인자 없이는 안 돈다 |
| `--document-first` | 래퍼 전용 |
| `-p` / `--print` | 래퍼가 비대화형 진입·stdout 분리를 선택하고 같은 flag는 native에 전달 |

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
| `--settings` | 래퍼가 modelPicker·env·hook을 이 경로로 전달한다. statusLine은 사용자 설정을 유지한다. **차단 중** |
| `--agents` | 래퍼가 검증 프로브 정의를 넣는다. **이미 차단 중** |
| `--system-prompt` | 기본 시스템 프롬프트 교체. **이미 차단 중** |
| `--setting-sources` | 설정 출처 선택. **이미 차단 중** |
| `--bare` | "skip hooks" — Clauduct의 `SubagentStart`/`PostToolUse` 라우팅 hook이 사라져 자식 모델 선택이 무너진다. 인증도 `ANTHROPIC_API_KEY`/apiKeyHelper로 좁힌다 |
| `--safe-mode` | hook·플러그인·에이전트 정의를 전부 끈다. 같은 이유 |
| `--autocompact` | 래퍼가 `CLAUDE_CODE_AUTO_COMPACT_WINDOW`를 정한다. 5.3 절차도 이 값을 지정하지 말라고 못박았다 |
| `--fallback-model` | 과부하 시 다른 모델로 자동 전환 — 게이트웨이의 모델 계약 밖으로 나간다(`unmappedAgentModels`로 잡히지만 애초에 보내지 않는 편이 낫다) |

## 5. D — native에 그대로 전달하는 옵션

도움말 분류상 경유 이탈·설정 교체 옵션이 아니다. 전달 여부는 각 기능의 실사용 검증 완료와 다르다. 값을 소비하는 것은 `nativeValueOptions`에 등록한다.

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

## 8. 현재 구현과 수정 전 관측

### 현재

`src/clauduct.mjs`는 옵션 이름 30개를 차단한다. 래퍼 소유 4개, 경유 이탈 8개, 계약 파괴 4개, 정책 판단 10개, 불확실 4개다. `--model`·`--effort`는 래퍼가 처리하며 명시적 print flag를 옵션의 값이나 `--` 뒤 프롬프트와 혼동하지 않는다. `--max-turns`는 native의 비대화형 실제 검사에서 사용했다.

`auth`·`install` 같은 bare-word 서브커맨드는 현재 parser가 명시적으로 차단하지 않는다. 프롬프트의 첫 단어와 구분하기 어렵기 때문이다. 이들을 Clauduct의 지원 세션 기능으로 간주하거나 gateway가 모든 부명령의 외부 효과를 차단한다고 해석하지 않는다.

여러 값을 받는 native 옵션 다음에 프롬프트를 놓을 때는 `--`로 구분한다. 예: `clauduct -p --allowedTools Read --max-turns 3 -- "Read the task file"`.

### 수정 전 이력

`src/clauduct.mjs`의 세 집합을 2026-09-12 도움말과 대조했다.

당시 차단은 `--settings` `--agents` `--system-prompt` `--setting-sources` 넷뿐이었다. 이 아래의 누락 목록은 수정 전 조사 결과이며 현재 상태가 아니다.

**값을 먹는데 추적되지 않는 옵션 21개** — 그 값이 다음 플래그로 오인된다.

```
--agent --allowed-tools --autocompact --cloud --debug --disallowed-tools
--environment --file --from-pr --json-schema --name --permission-prompts
--plugin-dir --plugin-url --prompt-suggestions --remote-control
--remote-control-session-name-prefix --session-id --system-prompt-snapshot
--teleport --worktree
```

예컨대 당시 `--name --model`에서 값과 래퍼 옵션을 구분하지 못했다. 현재는 `--name`의 값을 추적한다.

**더 이상 도움말에 없는데 추적 중인 것 2개**: `--max-turns` `--permission-prompt-tool`. 무해하지만 목록이 낡았다는 표시다.

## 9. 이 문서가 하지 않는 것

정확한 강제 경계는 `blockedOptions`와 `nativeValueOptions` 및 회귀 검사다. 현재는 6장 정책 옵션도 차단한다. 이 문서는 전역 권한·hook·신뢰 설정을 바꾸거나 모든 native 옵션의 실제 성공을 보증하지 않는다.
