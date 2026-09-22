# A1 — 역할·선택·인자 분야 리뷰

- 대상 루트: `D:\AIDEV\clauduct-v031` (worktree, 브랜치 `fix/v031`)
- 기준 commit: `149068edd693fb860a03244a2ea15764bcd68c34` (HEAD와 동일, v0.3.1 변경은 전부 미커밋 작업트리)
- 환경: Windows 11, Go 1.27.1 windows/amd64, 설치된 Claude Code **2.1.278** (`C:\Users\js\.local\bin\claude`)
- 제품 소스·테스트·문서·기존 `verification/` 파일은 수정하지 않았다. 재현은 전부 `go test -overlay`로만 주입했다.
- git은 읽기 전용 명령(`status`, `rev-parse`, `diff`, `show`)만 사용했다.

---

## 1. 실행 근거 — `/code-review`

### 호출 원문

```
Skill(skill='code-review',
      args='xhigh go/internal/app/roles.go go/internal/app/native_args.go go/internal/app/user_settings.go go/internal/app/settings.go go/internal/gateway/delegation.go go/internal/launch/ go/internal/protocol/bridge/route.go')
```

`ultra`, `--fix`, `--comment`는 사용하지 않았다. 추가 서브에이전트도 직접 띄우지 않았다.

### 로드 여부와 실행 형태

- 로드됨. 도구 결과 헤더는 `Skill "code-review" completed (forked execution).`였고, 본문으로 findings JSON 13건과
  scope/evidence 노트를 반환했다. 즉 이 빌드의 `code-review`는 레시피를 본문에 주입하는 방식이 아니라
  **fork 실행 후 결과만 반환**하는 형태였다.
- **관측된 effort/모델 표기: 없음(NOT_OBSERVED).** 도구 결과 어디에도 effort 레벨 문자열이나 모델 ID가 표기되지 않았다.
  `xhigh`는 내가 넘긴 인자일 뿐이고, 스킬이 그 레벨로 돌았다는 기계적 근거는 세션에서 관측하지 못했다.

### 내장 스킬과 미설치 플러그인 구분

- 이 세션에서 쓴 것은 **CLI 내장 스킬** `code-review`다(available-skills 목록에 플러그인 접두사 없이 나열됨).
- 같은 이름의 marketplace 플러그인 `claude-plugins-official/code-review`와 `pr-review-toolkit`은 **설치되어 있지 않다.**
  `C:\Users\js\.claude\plugins\installed_plugins.json`의 설치 목록은
  `claude-mem@thedotmack`, `codex@openai-codex`, `gopls-lsp@claude-plugins-official`, `ponytail@ponytail`,
  `skill-creator@claude-plugins-official`, `superpowers@superpowers-marketplace`, `typescript-lsp@claude-plugins-official`
  뿐이다. 둘 다 카탈로그에만 있다.

### 스킬이 실제로 훑은 범위

스킬은 인자로 준 7개 경로(제품 소스)만 다뤘고, 그 밖에 `run.go`, `results.go`, `messages.go`, `count_tokens.go`,
`workflow.go`, `cmd/clauduct-hook/main.go`를 근거로 인용했다. 스킬 보고에 따르면
`go build ./...`, `go vet ./...`, `go test ./internal/launch/... ./internal/protocol/bridge/... ./internal/gateway/...`가
모두 통과했고 `claude -vp`로 2.1.278을 확인했다고 한다(그 실행 로그 자체는 내 세션에 남지 않았다).

**스킬이 다루지 않은 담당 파일(= 내가 직접 리뷰한 파일):**

| 파일 | 사유 |
|---|---|
| `go/internal/app/argv_native_test.go` | 인자에 포함되지 않음(테스트) |
| `go/internal/app/native_print_test.go` | 동일 |
| `go/internal/app/role_case_probe_test.go` | 동일 |
| `go/internal/app/role_scan_test.go` | 동일 |
| `go/internal/app/roles_test.go` | 동일 |
| `go/internal/app/user_settings_test.go` | 동일 |
| `go/internal/gateway/delegation_test.go` | 동일 |
| `go/internal/gateway/unrouted_role_test.go` | 동일 |
| `go/internal/protocol/bridge/route_test.go` | 동일 |
| `go/internal/gateway/workflow_selection.go` | 인자 경로에 없음 |
| `verification/v031-argv-20260921/{REPORT.md,mutations.json}` | 제품 소스 아님 |
| `verification/v031-roles-20260921/{REPORT.md,mutations.json}` | 제품 소스 아님 |

### 스킬 지적 vs 직접 지적

- 스킬 13건 중 **6건을 confirmed로 확정**(A1-01/02/03/04/05/06), **나머지는 refuted 또는 improvement로 강등**(R1~R4, A1-07, A1-08, A1-09).
- 스킬이 낸 지적은 전부 내가 독립 재현(probe)으로 재검증했다. 스킬의 재현 코드는 세션에 남지 않았으므로 증거는 내 probe다.
- 직접 찾은 추가 내용: A1-01의 폭발 반경 확대(명시 모델·`subagent_type` 생략·`clauduct-*` 메뉴 역할까지 거부),
  R2(수신증 레이스 반증), R5(`run.go`의 슬롯 인덱스 의심 반증), R6(2.1.278 `--help` 전수 대조),
  R7(launch 동작 무변경 확인), A1-10, A1-12, H1.

---

## 2. 커버리지 표

| 담당 파일 | 검토 방식 | 스킬 커버 |
|---|---|---|
| `go/internal/app/roles.go` | 전체 읽음 + base(`git show`) 대조 + probe 3건 | O |
| `go/internal/app/native_args.go` | 전체 읽음 + 2.1.278 `--help` 전수 대조 | O |
| `go/internal/app/user_settings.go` | diff 전체 + 전체 흐름 읽음 + probe 2건 | O |
| `go/internal/app/settings.go` | diff 전체 + `cmd/clauduct-hook/main.go` 호출처 확인 | O |
| `go/internal/gateway/delegation.go` | diff 전체 + `prepare`/`route`/`loadChoice`/`cacheChoice` 전체 읽음 + probe 5건 | O |
| `go/internal/launch/launch.go` | diff 전체(주석만 변경) + `Build` 본문 읽음 | O |
| `go/internal/launch/refuse.go` | 전체 읽음(주석만 변경) | O |
| `go/internal/protocol/bridge/route.go` | diff 전체 + `roleRoutes`/`inheritRoles`/`menuRoute` 읽음 | O |
| `go/internal/gateway/workflow_selection.go` | diff + `workflowLabelSelection` 전체 읽음 | X → 직접 |
| `go/internal/app/argv_native_test.go` | 전체 읽음 + 실제 실행(14/14 PASS) | X → 직접 |
| `go/internal/app/native_print_test.go` | 전체 읽음 (역할·인자와 무관한 print 회귀 검사, 결함 없음) | X → 직접 |
| `go/internal/app/role_case_probe_test.go` | 전체 읽음 + 실제 실행(PASS) | X → 직접 |
| `go/internal/app/role_scan_test.go` | diff 전체 + 실행 | X → 직접 |
| `go/internal/app/roles_test.go` | diff 전체 + 실제 native 검사 2건 실행(PASS) | X → 직접 |
| `go/internal/app/user_settings_test.go` | diff 전체 + 실행(16 subtest PASS) | X → 직접 |
| `go/internal/gateway/delegation_test.go` | diff 전체 + 실행 | X → 직접 |
| `go/internal/gateway/unrouted_role_test.go` | diff 전체 + 실행 | X → 직접 |
| `go/internal/protocol/bridge/route_test.go` | diff 전체 + 실행 | X → 직접 |
| `verification/v031-argv-20260921/REPORT.md` | 전체 읽음 + 주장 재검증(14/14 재실행) | X → 직접 |
| `verification/v031-argv-20260921/mutations.json` | 전체 구조 확인(7건, source/test/실패 로그 포함) | X → 직접 |
| `verification/v031-roles-20260921/REPORT.md` | 전체 읽음 + 주장 재검증(native 3건 재실행) | X → 직접 |
| `verification/v031-roles-20260921/mutations.json` | 전체 구조 확인(12건, id/실패 로그만) | X → 직접 |

**미검토 파일: 없음.** 담당 22개 전부 커버했다.

---

## 3. 지적 목록

### A1-01 — 읽지 못한 일반 역할 파일 1개가 세션의 모든 미매칭 역할을 거부시킨다

- **분류/심각도/출처/기원**: confirmed / high / 스킬+직접 / 신규 회귀
- **파일**: `go/internal/app/roles.go:47-49,63,68-74,168-174` + `go/internal/gateway/delegation.go:266-276`
- **발생 조건**: `<cwd>/.claude/agents`(또는 상위 프로젝트·config의 같은 디렉터리) 안에 frontmatter를 파싱할 수 없는
  `.md` 파일이 하나라도 있을 때. 예: 첫 줄이 `---`인 메모(닫는 fence 없음), 64 KiB를 넘는 frontmatter, 열 수 없는 파일.
- **원인**: `claim()`이 `incomplete = incomplete || dir.prefix == ""`로 일반 디렉터리 전체를 "이름을 알 수 없음"으로 표시하고,
  `resolve()`가 `!found && unverified`면 `errRoleDefaults`를 돌려준다. `prepare()`는 `d.roleDefaults`를
  **모든 Agent 호출의 맨 앞에서 무조건** 호출하므로(delegation.go:268) 그 오류가 곧 `AGENT_SELECTION_UNVERIFIED`가 된다.
- **영향**: 정의 파일이 있는 역할만 살아남고, 정의가 없는 내장 역할(`Explore`, `Plan`, `general-purpose`, `fork`,
  `workflow-subagent`, `clauduct-inherit`)은 전부 거부된다. 직접 확인한 결과 **명시 모델을 준 호출,
  `subagent_type`을 생략한 호출, 이 빌드 자신의 `clauduct-terra-high` 메뉴 역할까지 모두 거부**된다.
  v0.3.0에서는 같은 입력으로 전부 정상 동작했다. 삭제된 주석이 "one stray markdown file ending every delegation
  in the session"이라고 부른 폭발 반경이 그대로 돌아왔다.
- **재현 명령** (작업 디렉터리 `D:/AIDEV/clauduct-v031/go`, 환경 `CGO_ENABLED=0`,
  `OVL=D:/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A1-roles-selection-args`):

```
CGO_ENABLED=0 go test ./internal/app/ -overlay=$OVL/overlay-app.json \
  -run 'TestProbeStrayNoteRefusesUnmatchedRoles' -count=1 -timeout 300s -v
CGO_ENABLED=0 go test ./internal/gateway/ -overlay=$OVL/overlay-gateway2.json \
  -run 'TestProbeUnverifiedScanRefusesEveryAgentShape' -count=1 -timeout 300s -v
```

  두 명령 모두 exit code 0 (probe는 관측만 하고 단언하지 않는다).
- **예상 결과(v0.3.0 기준)**: 정의가 없는 역할은 `found=false, err=nil`로 답하고, 읽은 정의는 그대로 쓴다.
- **실제 결과**:

```
role="Explore"           found=false err=ROLE_DEFAULTS_UNVERIFIED
role="Plan"              found=false err=ROLE_DEFAULTS_UNVERIFIED
role="general-purpose"   found=false err=ROLE_DEFAULTS_UNVERIFIED
role="fork"              found=false err=ROLE_DEFAULTS_UNVERIFIED
role="workflow-subagent" found=false err=ROLE_DEFAULTS_UNVERIFIED
role="clauduct-inherit"  found=false err=ROLE_DEFAULTS_UNVERIFIED
role="reviewer"          found=true  err=<nil> route={gpt-5.6-terra high}
---
{"subagent_type":"Explore",...}                       -> err=AGENT_SELECTION_UNVERIFIED
{"subagent_type":"Explore","model":"gpt-6-astra",...} -> err=AGENT_SELECTION_UNVERIFIED
{"prompt":"public","description":"proof"}             -> err=AGENT_SELECTION_UNVERIFIED
{"subagent_type":"clauduct-terra-high",...}           -> err=AGENT_SELECTION_UNVERIFIED
```

- **증거 경로**: `repro/A1-roles-selection-args/probe_app_test.go` (`TestProbeStrayNoteRefusesUnmatchedRoles`),
  `repro/A1-roles-selection-args/probe_gateway2_test.go`
- **권장 수정**:
  1. `incomplete`는 **바이트를 읽지 못한 경우에만** 세운다(= `os.Open`/`Stat` 실패, 64 KiB 경계에서 잘린 경우).
     내용을 전부 읽었는데 frontmatter가 잘못된 파일은 native도 건너뛴다(`roles.go:198-200`의 주석이 그렇게 말한다).
     그런 파일이 다른 이름을 선언했을 가능성은 없으므로 부재를 증명할 수 있다.
  2. 그래도 남는 "읽지 못한" 경우에는, `prepare()`가 명시 모델·부모 상속·메뉴 역할 경로에서는 정의를 필요로 하지
     않으므로 A1-07의 수정(필요할 때만 `roleDefaults` 호출)을 함께 적용하면 폭발 반경이 정의가 실제로 필요한
     호출로 좁혀진다.
- **미확인 부분**: 이 동작이 이슈 #42의 사용자 결정("손상된 정의 때문에 존재 여부를 알 수 없는 역할만 거부")을
  넘어서는지에 대한 사용자 판단. 문자 그대로는 "모든 미매칭 이름이 알 수 없음"이 성립하지만,
  `verification/v031-roles-20260921/REPORT.md`는 내장 역할 전부가 함께 거부된다는 사실을 적지 않았다.
  문서 정정 또는 정책 재확인이 필요하다.

### A1-02 — 결합된 short 옵션 + `--settings`이면 세션이 시작조차 못 한다

- **분류/심각도/출처/기원**: confirmed / medium / 스킬+직접 / 신규 회귀
- **파일**: `go/internal/app/native_args.go:34-36`(default 분기) + `go/internal/app/user_settings.go:31-41`
- **발생 조건**: argv에 `-cp`, `-pv`, `-cv`, `-dapi` 같은 결합/붙임 short 옵션이 있고, 그 뒤에 실제 `--settings`가 있을 때.
- **원인**: `nativeArgEnd`는 `-cp`를 알 수 없는 옵션으로 보고(`known=false`), `takeUserSettings`는 알 수 없는 옵션 뒤의
  설정 후보를 전부 `SETTINGS_INVALID`로 거부한다. 그런데 결합 short 옵션은 native가 모르는 형태가 아니라
  **문서화된 표준 형태**다. 설치된 2.1.278에서 `claude -cz` → `error: unknown option '-z'`가 나오는 것이
  commander가 `-c`와 `-z`로 분해한다는 직접 증거다.
- **영향**: `clauduct -cp --settings=my.json "프롬프트"`가 `SETTINGS_INVALID`로 실행 자체가 막힌다.
  같은 argv를 native는 정상 처리한다. v0.3.0의 `takeUserSettings`는 `--settings` 이외 토큰을 그대로 흘려보냈으므로 동작했다.
  (`roleCLI` 쪽 동일 증상은 base에도 있어 신규 회귀가 아니다.)
- **재현 명령**:

```
# native (작업 디렉터리 D:/AIDEV/clauduct-v031)
claude -cz                 -> "error: unknown option '-z'"      (결합 분해 증거)
claude -pv --settings={}   -> "2.1.278 (Claude Code)", exit 0
# 제품 (작업 디렉터리 D:/AIDEV/clauduct-v031/go, CGO_ENABLED=0)
CGO_ENABLED=0 go test ./internal/app/ -overlay=$OVL/overlay-app.json \
  -run 'TestProbeCombinedShortOptionsAndSettings' -count=1 -timeout 300s -v   # exit 0
```

- **예상 결과**: native와 같은 argv를 받아 gateway를 띄우고 설정을 병합한다.
- **실제 결과**:

```
args=[-cp --settings={} PROMPT]   -> err=SETTINGS_INVALID: expected one bounded JSON object or regular JSON file
args=[-pv --settings {}]          -> err=SETTINGS_INVALID: ...
args=[-dapi --settings={} PROMPT] -> err=SETTINGS_INVALID: ...
args=[-p --settings={} PROMPT]    -> err=<nil> slots=[1]   (대조군: 정상)
args=[-cp PROMPT]                 -> err=<nil>             (설정 후보가 없으면 통과)
```

- **증거 경로**: `repro/A1-roles-selection-args/probe_app_test.go` (`TestProbeCombinedShortOptionsAndSettings`)
- **권장 수정**: `nativeArgEnd`에서 `-` 하나로 시작하고 길이가 2를 넘는 토큰을 commander 규칙으로 전개한다
  (첫 글자가 알려진 boolean short 옵션일 때만 분해, 아니면 기존대로 unknown). 그리고
  `TestUserSettingsArgumentBoundaries`와 `argv_native_test.go`에 결합 short 사례를 추가한다.
- **미확인 부분**: 없음. 단, `verification/v031-argv-20260921/REPORT.md`의 "범위와 남은 항목"에
  "새 옵션이나 결합된 short 옵션을 모르면 뒤의 설정 후보를 보수적으로 거부할 수 있다"로 **이미 공개돼 있다.**
  다만 `docs/v2/ARCHITECTURE.md` 4절과 `docs/v2/COMPATIBILITY.md`에는 이 형태가 미지원이라는 문장이 없다.
  결함으로 고치든 미지원으로 문서화하든 둘 중 하나가 필요하다.

### A1-03 — pending 형제 하나가 실제 workflow 자식을 workflowRoute에서 떼어낸다

- **분류/심각도/출처/기원**: confirmed / high / 스킬+직접 / 신규 회귀
- **파일**: `go/internal/gateway/delegation.go:408-422`
- **발생 조건**: 요청의 `X-Claude-Code-Request-Class`가 `workflow`가 아니고(`scope.workflow == false`),
  같은 세션·같은 `scope.parent` 아래에 pending 상태인 다른 Agent 선택이 하나라도 있을 때.
- **원인**: base는 `if scope.workflow || binding.Role == "workflow-subagent"`를 `hasPending` 계산 **앞**에서 판정했다.
  v0.3.1은 이를 `hasPending` 뒤로 옮기고 `!hasPending &&` 조건을 붙였다. 그래서 무관한 형제 pending이
  workflow 분기를 통째로 끈다. 그 뒤 경로는 `d.metadata(binding)`로 `subagents/agent-<id>.meta.json`을 찾는데,
  workflow 자식의 meta는 `run.directory` 아래에 있어 찾지 못하고 최대 1초 대기 후 거부된다.
- **영향**: 루트 턴이 Workflow와 Agent를 함께 내보낸 흔한 상황에서, workflow 자식의 비-`workflow` class 요청
  (예: `count_tokens`는 `auxiliary`로 같은 `agentSelection`을 지난다 — `count_tokens.go:77`)이
  `AGENT_SELECTION_UNVERIFIED` 400으로 떨어진다.
- **재현 명령**:

```
cd D:/AIDEV/clauduct-v031/go
CGO_ENABLED=0 go test ./internal/gateway/ -overlay=$OVL/overlay-gateway.json \
  -run 'TestProbeWorkflowChildDivertedByPendingSibling' -count=1 -timeout 300s -v   # exit 0
```

- **예상 결과**: 형제 pending 유무와 무관하게 workflow 자식은 `workflow-parent` 경로를 받는다.
- **실제 결과**:

```
sibling=false scope.workflow=false -> route={gpt-6-astra low workflow-parent} found=true err=<nil>
sibling=true  scope.workflow=false -> route={} found=false err=AGENT_SELECTION_UNVERIFIED
```

- **증거 경로**: `repro/A1-roles-selection-args/probe_gateway_test.go` (`TestProbeWorkflowChildDivertedByPendingSibling`)
- **권장 수정**: "형제가 아무거나 pending인가"가 아니라 **이 binding에 대응하는 pending이 있는가**로 게이트한다.
  즉 workflow 분기를 원래 자리(맨 앞)로 되돌리고, `Agent(subagent_type:"workflow-subagent")` 호출만 구제하려면
  `d.pending`에서 이 자식의 `meta.ToolUseID` 키가 실제로 존재할 때만 pending 경로로 보낸다.
- **미확인 부분**: 실제 native가 workflow 자식의 어떤 요청에 어떤 class를 붙이는지 관측하지 못했다.
  base의 두 번째 disjunct(`binding.Role == "workflow-subagent"`)가 존재했다는 사실 자체가
  "workflow 자식의 모든 요청이 class=workflow는 아니다"를 전제한다. 실제 class 분포 1회 관측이면 결론이 난다.

### A1-04 — `route()`는 canonical 철자를 저장하는데 `stoppedTurn`만 정확 비교로 남았다

- **분류/심각도/출처/기원**: confirmed / medium / 스킬+직접 / 신규 회귀
- **파일**: `go/internal/gateway/delegation.go:482-485,521` vs `go/internal/gateway/results.go:302`
- **발생 조건**: 내장(비-custom) 역할에서 hook이 보고한 `binding.Role`의 대소문자가 `bridge.CanonicalRole` 결과와 다를 때.
- **원인**: base는 `role = binding.Role`로 **일부러** 두 값을 같게 맞췄다(삭제된 주석이 그 이유를 설명한다).
  v0.3.1은 `role = bridge.CanonicalRole(role)`로 바꿨고 선택·복원·재개·metadata 비교는 전부 `roleMatches`
  fold 비교로 전환했지만, `results.go:302`의 `choice.role != binding.Role`만 정확 비교로 남았다.
  gateway 전체에서 남은 유일한 정확 비교다(grep 확인).
- **영향**: `route()`는 통과하는데 `stoppedTurn`이 조용히 early-return 한다 → 자식 턴이 종료 처리되지 않고
  결과가 기록되지 않는다. 부모의 완료 증거가 미결로 남는다.
- **재현 명령**:

```
cd D:/AIDEV/clauduct-v031/go
CGO_ENABLED=0 go test ./internal/gateway/ -overlay=$OVL/overlay-gateway.json \
  -run 'TestProbeStoppedTurnRoleCasing' -count=1 -timeout 300s -v   # exit 0
```

- **예상 결과**: `route()`가 받아들인 binding은 `stoppedTurn`에서도 같은 자식으로 인식된다.
- **실제 결과**:

```
route: {gpt-5.6-luna max agent-call-role} found=true err=<nil>
resolved role="Explore" binding.Role="explore" equal=false
before stoppedTurn: stopped=false state="running"
after  stoppedTurn: stopped=false state="running"      <-- 변화 없음
```

- **증거 경로**: `repro/A1-roles-selection-args/probe_gateway_test.go` (`TestProbeStoppedTurnRoleCasing`)
- **권장 수정**: `results.go:302`를 `!roleMatches(choice.role, binding.Role, choice.custom)`으로 바꾼다.
- **미확인 부분**: 실제 native가 canonical과 다른 철자를 hook/meta로 보고하는지 직접 관측하지 못했다.
  `prepare()`가 `fields["subagent_type"]`을 canonical로 다시 써서 보내므로 보통은 일치할 것이다.
  다만 코드 전체가 fold 비교로 전환된 것 자체가 불일치를 전제하고 있고, 이 한 곳만 그 전제를 따르지 않는다.

### A1-05 — plugin 정의의 frontmatter를 읽지 못하면 그 역할이 오류 없이 "부재"가 된다

- **분류/심각도/출처/기원**: confirmed / low / 스킬+직접 / 신규 회귀
- **파일**: `go/internal/app/roles.go:162-174` (특히 169행 `incomplete = incomplete || dir.prefix == ""`)
- **발생 조건**: plugin의 `agents/*.md`에서 frontmatter 파싱이 실패하고(예: 64 KiB 경계 초과로 닫는 fence가 잘림),
  그 frontmatter가 파일명과 다른 `name:`을 선언했을 때.
- **원인**: 억제 근거는 "plugin 기본값은 파일명을 쓴다"인데, `roles.go:194-203`을 보면 **frontmatter가 파싱되면
  plugin도 frontmatter의 이름을 쓴다.** 파일명은 파싱 실패 시의 fallback일 뿐이므로 전제가 성립하지 않는다.
- **영향**: `p:reviewer`가 오류 없이 부재로 답해진다. v0.3.1에서는 그 뒤 `native-selection`으로 떨어져
  native의 실제 선택을 검증하므로 실질 피해는 작지만, 같은 파일이 일반 디렉터리에 있으면 전 역할이 거부되는
  A1-01과 정반대 답이 나온다(정책 비대칭).
- **재현 명령**:

```
cd D:/AIDEV/clauduct-v031/go
CGO_ENABLED=0 go test ./internal/app/ -overlay=$OVL/overlay-app.json \
  -run 'TestProbeUnreadablePluginNameIsSilentlyAbsent' -count=1 -timeout 300s -v   # exit 0
```

- **예상 결과**: 이름을 확정할 수 없으므로 최소한 A1-01과 같은 기준으로 답한다.
- **실제 결과**:

```
plugin role="p:reviewer" found=false err=<nil>                    <-- 조용한 부재
plugin role="p:draft"    found=false err=ROLE_DEFAULTS_UNVERIFIED
ordinary role="reviewer" found=false err=ROLE_DEFAULTS_UNVERIFIED
```

- **증거 경로**: `repro/A1-roles-selection-args/probe_app_test.go` (`TestProbeUnreadablePluginNameIsSilentlyAbsent`)
- **권장 수정**: A1-01의 1번 수정(바이트를 읽지 못한 경우에만 `incomplete`)을 적용하면 plugin/일반 구분 없이
  한 규칙으로 정리된다. 169행의 `dir.prefix == ""` 예외는 제거하고 주석의 잘못된 전제도 함께 고친다.
- **미확인 부분**: 없음.

### A1-06 — projects 트리 자체가 없을 때 loadChoice가 모든 역할을 거부한다

- **분류/심각도/출처/기원**: confirmed / low / 스킬+직접 / 신규 회귀
- **파일**: `go/internal/gateway/delegation.go:663-676`(fs.ErrNotExist fall-through 삭제), `:612-620`, `:622-635`
- **발생 조건**: `ConfigureDelegations`의 `MkdirAll`이 실패했거나(경로에 일반 파일이 있음, 권한 거부)
  세션 중 트리가 삭제되어 `openProjects`가 `errProjectsAbsent`를 돌려줄 때.
- **원인**: base는 `choicePath`의 오류가 `fs.ErrNotExist`면 `choiceAbsent`로 흘려보내 역할별로 구분했다.
  v0.3.1은 새 `errProjectsAbsent` 센티널을 쓰면서 그 분기를 삭제했고, `choiceAbsent`는 "root가 열렸는데
  저널만 없는" 경우에만 도달한다(주석도 그렇게 고쳐졌다).
- **영향**: 트리가 없으면 custom/미라우팅 역할까지 `AGENT_SELECTION_UNVERIFIED`가 된다. base에서는
  `(empty, false, nil)`로 native 라우팅에 맡겼다. `ConfigureDelegations`가 실패를 진단으로만 남기고
  계속 진행하도록 설계돼 있으므로 이 상태 자체는 지원 범위 안이다.
- **재현 명령**:

```
cd D:/AIDEV/clauduct-v031/go
CGO_ENABLED=0 go test ./internal/gateway/ -overlay=$OVL/overlay-gateway.json \
  -run 'TestProbeAbsentProjectsTreeLoadChoice' -count=1 -timeout 300s -v   # exit 0
```

- **예상 결과**: 없는 트리는 저널을 가질 수 없으므로 역할별 정책(`choiceAbsent`)으로 답한다.
- **실제 결과**:

```
absent tree  role="custom-unrouted" found=false err=AGENT_SELECTION_UNVERIFIED: PROJECTS_ABSENT
absent tree  role="Plan"            found=false err=AGENT_SELECTION_UNVERIFIED: PROJECTS_ABSENT
present tree role="custom-unrouted" found=false err=<nil>              (대조군)
present tree role="Plan"            found=false err=AGENT_SELECTION_UNVERIFIED
```

- **증거 경로**: `repro/A1-roles-selection-args/probe_gateway_test.go` (`TestProbeAbsentProjectsTreeLoadChoice`)
- **권장 수정**: `loadChoice`에서 `errors.Is(err, errProjectsAbsent)`일 때 `d.choiceAbsent(binding)`으로
  떨어뜨린다(`metadata`가 이미 같은 센티널을 `errMetadataPending`으로 구분해서 쓰고 있다).
- **미확인 부분**: `MkdirAll` 실패의 실제 발생 빈도. 심각도를 low로 둔 이유다.

---

## 4. 반박된 지적과 반박 근거

| ID | 지적(출처) | 반박 근거 |
|---|---|---|
| R1 | hook 바이너리가 없으면 native events가 꺼져 모든 미라우팅 역할이 거부된다 (스킬) | hook이 없으면 `SubagentStart`가 없어 `g.agents`에 자식이 등록조차 되지 않는다. `messages.go:296-348`에서 `registered=false` → override 없음 → 프로덕션은 `g.contexts != nil`이라 `errDelegationUnverified`로 이미 거부된다. 즉 hook 부재 배포에서는 v0.3.0에서도 subagent가 동작하지 않았다. **증분 회귀가 아니다.** 또 `run.go:239-245`는 hook이 있으면 `prepareNativeEvents` 실패 시 `NATIVE_EVENT_SETUP_FAILED`로 시작을 중단하므로, hook이 있는데 events만 꺼진 프로덕션 상태는 만들어지지 않는다. |
| R2 | 자식의 첫 요청이 turn 수신증 발행보다 앞설 수 있다(레이스) (직접 의심) | `go/internal/app/native-events.mjs:61-81`이 `await publication` 이후에 `next(e)`로 요청을 진행시킨다. 수신증 본문과 `.ready` 마커 쓰기가 요청보다 먼저 끝난다. 레이스 없음. |
| R3 | `saveChoice`가 `receipt.CustomRole` 반영 전 값으로 저널을 써서 캐시와 어긋난다 (스킬) | `custom`이 설정되는 세 경로 모두 `choice.custom`과 `receipt.CustomRole`이 같은 값이다: `prepare`(delegation.go:389-390), `workflowRoute`(workflow.go:315-318), `loadChoice`(delegation.go:712,734). 어긋나는 경로를 만들 수 없다. `cacheChoice:540`의 `|| choice.receipt.CustomRole`은 중복 표현일 뿐이다(A1-09 참고). |
| R4 | native-selection 선택 기록의 `Model`/`Effort`가 빈 문자열이라 진단이 잘못 보인다 (스킬) | 의도된 설계다. `unrouted_role_test.go`가 `pending.route.Model != ""` → `"an unobserved native selection was reported as known"`으로 **빈 값을 명시적으로 요구**한다. 관측 전에 모델을 지어내지 않는 것이 #45 결정이다. probe에서도 `source="native-selection" model="" effort=""`로 정직하게 기록된다. |
| R5 | `run.go:232-233`이 `forward` 인덱스를 `o.Args`에 적용해 슬롯이 어긋난다 (직접 의심) | `run.go:151`에서 `o.Args = forward`로 교체한 뒤에 슬롯을 적용한다. 인덱스 기준이 일치한다. |
| R6 | `nativeArgEnd`의 공개 옵션 arity가 2.1.278과 어긋난다 (직접 의심) | 설치된 `claude --help` 전체와 전수 대조했다. 필수값·optional·boolean 공개 옵션이 모두 표에 있다. 빠진 것은 `--dangerously-skip-permissions`/`--allow-dangerously-skip-permissions` 둘뿐인데 `launch.Refused`가 `takeUserSettings`보다 먼저(run.go:144) 전역 거부한다. variadic을 arity 1로 본 것은 두 스캐너 모두 토큰을 제거하지 않으므로 무해하다(`--tools Read Glob --settings X` 검사로 확인). |
| R7 | `launch.Build`/`launch.Refused`의 동작이 바뀌었다 (직접 의심) | diff는 주석만 바뀌었다. `verification/v031-argv-20260921/REPORT.md`의 "실행 코드는 바꾸지 않았다" 주장과 일치한다. |

---

## 5. 보류 항목

| ID | 내용 | 결론을 내리려면 |
|---|---|---|
| H1 | subcommand argv(`clauduct mcp ...`, `clauduct agents ...`, `clauduct plugin ...` 등 20여 개)의 옵션도 top-level 표로 해석된다. subcommand 고유 옵션은 unknown이 되어 뒤에 `--settings`가 있으면 `SETTINGS_INVALID`가 된다. | clauduct가 native subcommand를 감싸는 것이 지원 범위인지 `docs/v2/ARCHITECTURE.md` 4절에 명시. 지원이면 subcommand 감지 후 스캔 중단이 필요하다. |
| H2 | A1-04의 실제 도달성 | native 2.1.278이 hook/meta로 보고하는 내장 역할 철자를 1회 관측(소문자 `explore` 등이 나오는지). 나오면 실사용 결함, 안 나오면 잠재 결함. |
| H3 | A1-03의 실제 도달성 | workflow 자식이 보내는 요청들의 `X-Claude-Code-Request-Class` 분포를 1회 관측. `workflow` 외 값이 하나라도 있으면 실사용 결함. |
| H4 | `native_args.go` 표는 2.1.278 `--help`에 고정돼 있다. 어떤 옵션이 다음 버전에서 boolean↔값-필수로 바뀌면 실제 `--settings`를 값으로 삼켜 **사용자 설정 병합이 조용히 누락**될 수 있다(필수 hook 미설치). | 버전 상향 시 `argv_native_test.go` 대조 실행. 현재 2.1.278에서는 불일치 없음(R6). |

---

## 6. 개선 제안

### A1-07 — `prepare()`가 모든 Agent 호출에서 역할 정의를 무조건 해석한다

`delegation.go:266-276`. 명시 모델 호출, 부모 상속 고정 호출, `modelID=="inherit"` 호출에서도
`roleDefaults`가 호출된다. 그 클로저(`run.go:320-322`)는 호출마다 새 `sessionRoleSources`를 만들어
argv 전체를 다시 파싱하고(이제 argv에는 인라인된 `--settings=<병합 blob>`이 들어 있다),
managed·cwd 상위 전 경로·config의 `.claude/agents`를 전부 WalkDir 한다. probe로 확인:
명시 모델 호출에서도 `roleDefaults calls: 1`. 필요한 분기에서만 호출하면 I/O가 사라지고 A1-01의 폭발 반경도 줄어든다.
(재현: `-run 'TestProbeRoleDefaultsCalledForExplicitModel'`, exit 0)

### A1-08 — 반복 `--settings` 슬롯마다 병합 blob 전체를 넣는다

`user_settings.go:58-60` + `run.go:232-234`. probe에서 `slots=[1 2]`로 두 자리 모두 같은 blob을 받는다.
설정 소스가 수백 KB면 자식 명령줄이 배수로 커져 Windows 32767자 한계에 먼저 걸린다.
native가 마지막 `--settings`를 쓰므로 **마지막 슬롯에만 병합 결과를 넣고 앞 슬롯은 원래 값 그대로** 두면
자리 보존(프롬프트 흡수 방지)이라는 원래 목적은 유지하면서 사본이 하나로 준다.
단 이는 `TestUserSettingsMergedAtEveryOriginalBoundary`가 명시적으로 요구하는 동작이므로
결함이 아니라 정책 변경 제안이다. (재현: `-run 'TestProbeRepeatedSettingsSlots'`, exit 0)

### A1-09 — `CanonicalRole`의 선형 fold 스캔과 `custom`의 중복 유도

`bridge/route.go:162-174`는 호출마다 두 map을 `EqualFold`로 선형 스캔한다. `roleMatches`가 비교당 2회,
`route()`가 추가로 호출하므로 선택 1건에 열 번 넘게 돈다. 소문자 키 map을 패키지 초기화 때 한 번 만들면
조회 1회로 끝나고, fold-동일 키가 둘 이상일 때 map 반복 순서에 따라 답이 달라지는 (현재는 잠재적인)
비결정성도 사라진다. 현재 `roleRoutes`/`inheritRoles`에 fold-동일 키는 없다(확인함).
같은 맥락에서 `custom`이 네 군데(`prepare`, `route:481`, `cacheChoice:540`, `loadChoice:695`)에서
재유도되는데 값은 항상 같다(R3). 한 곳에서 정하고 나머지를 지우는 편이 규칙을 한 번만 말한다.

### A1-10 — `workflowLabelSelection`이 내장 역할 이름에도 `CustomRole=true`를 붙인다

`workflow_selection.go:142-145`. `options.AgentType`가 `Explore` 같은 내장 이름이어도 custom으로 기록된다.
그 값은 선택 기록·저널(`saveChoice`의 `CustomRole`)로 전파되고 복원 시 `roleMatches`의 fold 비교를 끈다.
현재 `workflow.go:269,279`가 이미 정확 비교를 요구하므로 관측되는 실패는 없지만
"custom = 사용자 정의"라는 이름이 뜻과 달라진다. `!bridge.KnownRole(options.AgentType)`일 때만 세우는 편이 정확하다.

### A1-11 — `UserPromptSubmit` hook 실패가 사용자 프롬프트 자체를 막는다

`settings.go:78` + `cmd/clauduct-hook/main.go:191-193`. gateway 왕복이 실패하면 exit 2로 프롬프트가 차단된다.
이는 `docs/v2/COMPATIBILITY.md`(묶음 5, 이슈 #53)에 "등록 실패가 계속되면 안내와 함께 해당 프롬프트를 막는다"로
**채택된 결정**이므로 결함으로 재보고하지 않는다. 다만 `SessionStart`에서는 같은 실패가 아무것도 막지 않았고
등록 자체는 멱등·저비용이므로, 차단 전에 1회 재시도를 넣으면 일시적 왕복 실패로 사용자 턴을 잃지 않는다.

### A1-12 — `verification/v031-roles-20260921/mutations.json`에 변이 내용이 없다

argv 쪽(`v031-argv-20260921/mutations.json`)은 항목마다 `mutation`/`source`/`test`/`exit_code`/`failures`를 담아
어떤 파일의 어떤 변이였는지 재현할 수 있다. roles 쪽은 `id`/`exitCode`/`assertionFailure`/`output`뿐이라
`42-unknown-name` 같은 id만으로는 변이를 복원할 수 없다. 같은 스키마로 맞추는 편이 좋다.

---

## 7. 근거 부족으로 뺀 의심

- `roleCLI`의 `hasCLI` 판정이 `--` 뒤 프롬프트 안의 `--agents` 문자열로도 켜진다 → 그 앞의 알 수 없는 옵션이
  하드 오류가 된다. **base와 동일**해서 신규 회귀가 아니고, 실제 프롬프트에 그 문자열을 넣는 사용 사례를 구성하지 못했다.
- `CanonicalRole`이 `CLAUDUCT-TERRA-HIGH` 같은 대문자 메뉴 이름을 정규화하지 않아 `menuRoute`가 놓친다.
  **base도 동일**(`menuRoute`의 접두사 비교가 원래 대소문자 구분). 메뉴 이름은 런처가 생성하므로 철자가 고정이다.
- `launch.Refused`가 옵션 값과 `--` 뒤에서도 두 이름을 거부하는 과잉 거부. `docs/v2/ARCHITECTURE.md` 4절 1번에
  "이 과잉 거부는 의도된 정책"으로 명시된 채택 사항이다.
- `metadataModelMatches(source=="native-selection")`이 `model == ""`만 허용하는 것이 native가 모델을 기록하는
  경우를 놓칠 수 있다는 의심. `roles_test.go`의 실제 native 검사(`TestNativeUnlistedRoleKeepsNativeModelAndCompletion`,
  2.1.278에서 PASS)가 빈 값을 확인해 준다. 다른 값이 나오는 입력을 구성하지 못했다.
- `--settings`의 값이 `--`로 시작하는 실제 파일일 때의 처리. `argv_native_test.go`의 `option_shaped_file`이
  native와 제품을 직접 대조하고 두 경로 다 PASS했다(내가 재실행해 확인).
- `native_print_test.go`는 담당 파일이지만 역할·선택·인자 계약과 무관한 print 모드 회귀 검사다. 지적 없음.

---

## 8. 실행한 명령 전체와 NOT_RUN

Go 검사는 모두 작업 디렉터리 `D:/AIDEV/clauduct-v031/go`, 환경 `CGO_ENABLED=0`에서 실행했다.
`OVL=D:/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A1-roles-selection-args`.

| # | 명령 | exit |
|---|---|---|
| 1 | `git status --porcelain=v1` / `git rev-parse HEAD` / `git diff --stat` (`/d/AIDEV/clauduct-v031`) | 0 |
| 2 | `git diff --stat -- <담당 22개 경로>` | 0 |
| 3 | `git diff -- go/internal/app/{roles.go,settings.go,user_settings.go}` | 0 |
| 4 | `git diff -- go/internal/{launch/launch.go,launch/refuse.go,protocol/bridge/route.go,gateway/workflow_selection.go}` | 0 |
| 5 | `git diff -- go/internal/gateway/delegation.go` | 0 |
| 6 | `git diff -- go/internal/app/{role_scan_test.go,roles_test.go,user_settings_test.go}` | 0 |
| 7 | `git diff -- go/internal/gateway/{delegation_test.go,unrouted_role_test.go,absent_tree_test.go}`, `go/internal/protocol/bridge/route_test.go` | 0 |
| 8 | `git show 149068edd693fb860a03244a2ea15764bcd68c34:go/internal/app/roles.go` | 0 |
| 9 | `cat verification/v031-roles-20260921/REPORT.md`, `verification/v031-argv-20260921/REPORT.md`, 두 `mutations.json` | 0 |
| 10 | `go version` → `go1.27.1 windows/amd64` | 0 |
| 11 | `claude --version` → `2.1.278 (Claude Code)` | 0 |
| 12 | `claude --help` (공개 옵션 전수 대조) | 0 |
| 13 | `claude -vz / -vp / -pv / -cv / -cz / -dv / -pz / -hv / -zv / -pv --settings={}` (결합 short 분해 확인) | 0 |
| 14 | `CGO_ENABLED=0 go test ./internal/app/ -overlay=$OVL/overlay-app.json -run 'TestProbe' -count=1 -timeout 300s -v` | 0 |
| 15 | `CGO_ENABLED=0 go test ./internal/gateway/ -overlay=$OVL/overlay-gateway.json -run 'TestProbe' -count=1 -timeout 300s -v` | 0 |
| 16 | `CGO_ENABLED=0 go test ./internal/gateway/ -overlay=$OVL/overlay-gateway2.json -run 'TestProbeUnverifiedScanRefusesEveryAgentShape' -count=1 -timeout 300s -v` | 0 |
| 17 | `CGO_ENABLED=0 go test ./internal/launch/... ./internal/protocol/bridge/... -count=1 -timeout 300s` | 0 |
| 18 | `CGO_ENABLED=0 go test ./internal/app/ -run 'TestRoleDefaultsCLIHonoursValueBoundaries|TestUserSettings|TestOneUnreadableDefinitionPreservesKnownRoles|TestUnreadableRoleWithDifferentFilenameDoesNotFallBack|TestAnUnreadableDuplicateUnderAnotherFilenameIsNotCaught|TestUnreadableNestedPluginRoleClaimsItsBaseName|TestASkipOutranksAMatchOnlyWhenItSharesItsName|TestARoleWithoutAModelStillInheritsTheParentRoute' -count=1 -timeout 600s` | 0 |
| 19 | `CGO_ENABLED=0 go test ./internal/gateway/ -run 'TestARoleWithNoRouteKeepsNativeChoiceAndIsTracked|TestNativeChoiceRefusesUnverifiedIdentityModelAndEffort|TestNativeChoiceChecksTheActualRequestEffort|TestInheritingRoleCaseSurvivesPreparationAndRestore|TestCustomForkIdentitySurvivesExplicitSelectionAndRestore|TestMenuDefinitionStillPinsTheDescendantSelection|TestMissingChoiceInAvailableProjectsDistinguishesKnownAndCustomRoles|TestWorkflowSelectionRequiresOriginAndUnmodifiedNativeEvidence|TestNativeBuiltinRoleCaseKeepsVerifiedRoute|TestOmittedNativeRoleKeepsGeneralPurposePolicyAndProof' -count=1 -timeout 600s` | 0 |
| 20 | `CGO_ENABLED=0 go test ./internal/app/ -run 'TestNativeSettingsArgumentBoundaries' -count=1 -timeout 900s -v` → 14/14 subtest PASS (4.98s) | 0 |
| 21 | `CGO_ENABLED=0 go test ./internal/app/ -run 'TestNativeCustomForkRetainsItsDefinitionAndExplicitChoice|TestNativeUnlistedRoleKeepsNativeModelAndCompletion|TestNativeCaseDistinctRole' -count=1 -timeout 900s -v` → 전부 PASS, `native 2.1.278 selected gpt-5.6-terra/low` 로그 확인 | 0 |
| 22 | `cat ~/.claude/plugins/installed_plugins.json` (플러그인 설치 목록 확인) | 0 |

**기존 검사 재검증 결과: 담당 범위의 제품 테스트는 전부 PASS.** 즉 위 confirmed 6건은 현재 스위트가 잡지 못한다.
`v031-argv-20260921/REPORT.md`의 "14개 대조 항목 PASS"와 `v031-roles-20260921/REPORT.md`의 native 관찰 3건은
내가 직접 재실행해 재현했다.

### NOT_RUN

| 검사 | 사유 | 추가 확인 방법 |
|---|---|---|
| `go test ./...` 전체 회귀 | 과제 지시로 금지. 통합 검토자가 필요성을 판단한다. | 통합 단계에서 1회 |
| `-race` 실행 | 담당 파일에 새 동시성 구조 변경이 없다(`route()`가 `hasPending` 계산 후 `Unlock`하도록 순서가 바뀌었으나 그 사이 상태를 재사용하지 않는다). 좁은 race 실행은 시간 대비 판별력이 낮아 생략. | `CGO_ENABLED=1 CC=C:/msys64/ucrt64/bin/gcc.exe go test ./internal/gateway/ -race -run 'TestARoleWithNoRoute|TestWorkflow' -count=1` |
| A1-01/A1-03/A1-04의 실제 native 종단 재현 | 각각 실제 `claude.exe` 세션 + 특정 상태(형제 pending, 역할 철자, stray note)를 동시에 만들어야 한다. 단위 수준에서 제품 API로 재현했고 결론이 바뀌지 않는다. | `roles_test.go`의 `nativeRun` 하네스를 overlay로 확장해 stray note + Agent 호출 1회 |
| workflow 자식 요청의 실제 `X-Claude-Code-Request-Class` 관측 | 실제 Workflow 실행 세션이 필요하고, 관측 없이도 A1-03의 코드 분기 차이는 확정된다. | 실제 TUI/`-p` 세션에서 gateway 진단의 요청 class 1회 수집 |
| native가 hook/meta로 보고하는 내장 역할 철자 관측 | 위와 동일. A1-04의 도달성만 좌우한다. | `SubagentStart` payload 1회 수집 |
| 유료 backend, 수동 TUI, Node V1/PowerShell 검증 | 이 분야 범위 밖 | 통합 검토자 |
