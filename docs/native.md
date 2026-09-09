# Clauduct native 경로

## 실행과 모델

```powershell
D:\AIDEV\Clauduct\clauduct.cmd
D:\AIDEV\Clauduct\clauduct.cmd --model sol --effort xhigh
D:\AIDEV\Clauduct\clauduct.cmd --continue
D:\AIDEV\Clauduct\clauduct.cmd --resume <session-id>
D:\AIDEV\Clauduct\clauduct.cmd --dry-run
```

현재 프로젝트 폴더와 터미널을 native Claude 자식에 연결하며 SEND 입력이나 별도 gateway 실행이 필요하지 않습니다. `--dry-run`은 인증·소켓·Claude 실행 없이 구성을 표시합니다. 사용자 PATH/PowerShell 프로필은 변경하지 않았습니다. Windows Terminal의 PowerShell 7을 편의상 권장하나 cmd/Git Bash에서도 같은 Node gateway가 실행됩니다. shell 선택이 모델 지연·메모리 안정성을 개선한다는 측정 근거는 없습니다.

### 작업 문서 선로드

`clauduct --gpt-agents --document-first`는 이 native 자식 세션에만 고정 문서 선로드 지침을 `--append-system-prompt`로 전달합니다. 사용자가 작업 문서를 읽고 이어가라고 요청하면, 선택적 Skill/workflow·셸 준비·위임보다 먼저 문서를 Read로 읽고 범위와 중단 조건을 확정하도록 합니다. 문서 내용에 독립적인 권한을 부여하지 않습니다.

기본 실행은 바꾸지 않으며 옵션 없이 새로 시작하면 지침을 추가하지 않습니다. 전역 설정·plugin·자동 hook·권한 검사·모델·effort·agent 정의는 그대로입니다. 이것은 모델 행동 지침이지 강제 sandbox나 tool gate가 아닙니다. 실제 첫 도구가 Read인지, 범위 밖 쓰기가 없는지는 native 세션에서 별도로 검증해야 합니다. 기존 세션에 소급 적용되지 않습니다.

중복 native 옵션에 의한 지침 교체를 피하기 위해 `--document-first`와 사용자 `--append-system-prompt`는 함께 사용할 수 없습니다. raw `--agents`, `--settings`, `--setting-sources`, `--system-prompt` 차단은 유지합니다. `--dry-run`의 `documentFirst`는 시작 구성만 나타내며 실제 준수 증거가 아닙니다.

### GPT 선택용 일반 작업 agent

`clauduct --gpt-agents`로 시작하면 해당 자식 세션에만 일반 작업 agent 5개를 추가합니다. 메인 모델·effort는 평소처럼 실행 옵션이나 세션 안에서 선택하며, 이 옵션이 메인 값을 강제하지 않습니다. 일반 실행에는 추가하지 않으며 옵션 없이 다시 시작하면 추가 등록하지 않습니다. 전역 agent 파일은 생성하지 않습니다.

| subagent_type | 모델·effort | selectionSource |
|---|---|---|
| clauduct-astra | astra/medium | definition-model |
| clauduct-sol | sol/xhigh | definition-model |
| clauduct-terra | terra/high | definition-model |
| clauduct-luna | luna/max | definition-model |
| clauduct-inherit | 생성 시점 직접 부모의 실제 모델·effort | definition-inherit |

Agent에서 위 subagent_type을 선택하고 model 인수는 생략합니다. 예: `Agent(subagent_type="clauduct-sol", description="Review implementation", prompt="...")`. 이들은 자체 지침을 사용하는 일반 작업 agent이며 내장 Plan/Explore의 복제본이 아닙니다. 기존 Explore/general-purpose=luna/max, Plan=sol/xhigh는 그대로입니다. 기존 명시 model 인수가 있는 호출의 우선순위도 변경하지 않습니다.

도구 목록은 Read, Grep, Glob, Bash, Edit, Write, Agent, TaskOutput, SendMessage입니다. native가 현재 문맥에서 제공하는 도구와 기존 권한 검사 아래에서만 사용할 수 있습니다. permissionMode, hook, MCP, 전역 설정을 추가/완화하지 않습니다. 상속 모델과 작업 권한의 상속은 다른 문제이며 도구가 보인다고 외부 쓰기가 허용되는 것은 아닙니다. 임의 --agents 입력은 계속 차단합니다.

--verify-agent-models와 함께 켜면 별도 이름의 Read 전용 시험 정의도 유지하며, 단일 --agents JSON에 두 집합을 합칩니다. 선택기는 실행기가 생성한 정의의 모델·effort만 불변 복사하고 기존 생성 호출/metadata/세션/역할/부모 검증 후 적용합니다. 정의 자체를 요청이나 대화에서 받아들이지 않습니다. 로컬 검증 완료, 새 일반 작업용 정의의 실제 native 수행은 미검증입니다.

### 읽기 전용 시험용 agent

`--verify-agent-models`는 명시적으로 허용된 세션 한정 시험 옵션입니다. `clauduct-probe-astra/sol/terra/luna/inherit`라는 별도 agent 5개를 native `--agents` 정의로 전달합니다. 각 정의의 도구는 Read 하나, maxTurns는 3이며 대상은 이 실행기의 src/models.mjs입니다. 네 GPT 모델은 모델 기본 effort를 정의하고 inherit는 모델만 inherit로 정의합니다. 기존 내장 역할·settings·환경과 임의 사용자 `--agents` 차단은 유지합니다. 옵션 없이 다시 시작하면 시험 정의를 추가하지 않습니다. 전역/프로젝트 agent 파일은 생성하지 않지만 native 세션 기록 자체가 남지 않는다는 뜻은 아닙니다.

사용자 실행은 원하는 메인 모델·effort를 선택한 기존 명령에 `--verify-agent-models`만 추가합니다. 이 옵션은 메인 모델을 강제하지 않습니다. inherit 기대값은 생성 호출 시점의 실제 부모 모델·effort이며 고정된 astra/max가 아닙니다. 시험 agent 호출에서는 subagent_type으로 위 이름을 선택하고 model 인수는 생략해야 합니다. 등록 정의의 inherit 의도와 검증된 생성 호출을 연결해 부모 snapshot을 적용하며 진단은 definition-inherit입니다. 이 수정은 로컬 검증됐고 실제 native 재검증은 남아 있습니다. 일반 Agent(model=GPT/inherit) 인터페이스의 구현 완료는 아닙니다. 요청별 status를 함께 수집하고 불일치를 숨기거나 별칭으로 대체하지 않습니다. sessionRef는 불투명 상관관계 값이며 native 세션 UUID와 다릅니다.

| 모델 | 기본 effort |
|---|---|
| gpt-6-astra | medium |
| gpt-5.6-sol | xhigh |
| gpt-5.6-terra | high |
| gpt-5.6-luna | max |

메인은 선택한 모델과 effort를 존중합니다. 모델 미지정 Explore/일반 작업은 luna/max, Plan은 sol/xhigh를 사용합니다. 기존 Haiku/Sonnet 별칭은 luna, Opus는 sol로 변환하지만 현재 GPT 직접 선택 계약의 대체 검증에는 사용하지 않습니다. 검증된 부모 호출과 자식 metadata의 명시 모델이 역할 기본값보다 우선하며 선택 모델의 기본 effort를 적용합니다. gateway의 명시 inherit 처리는 생성 호출에 기록한 직접 부모의 실제 모델·effort를 유지하며 snapshot이 없으면 거부합니다. 설치 native Agent 입력 스키마의 GPT/inherit 지원은 아직 미해결입니다. [현재 계약과 검증](gpt-agent-selection-contract.md). 실제 계정에서 모든 조합이 수락되는지는 합성 검사로 입증되지 않습니다. 아래 과거 세션의 sol/high 통과 기록은 변경 전 증거이며 sol/xhigh의 실제 검증으로 확대하지 않습니다.

**메타데이터 준비와 검증:** 설치 native는 sidecar 저장 완료를 기다리지 않고 SubagentStart로 진행합니다. 실제 기록에서 sidecar는 hook 반환 뒤에 생성됐으므로 hook 안에서 파일을 기다리지 않습니다. gateway는 검증 완료 후 전달하는 Agent/Task/Skill/SendMessage/Workflow 호출의 ID와 모델·역할·부모 ID만 메모리에 남깁니다. 기존 SubagentStart hook은 세션 ID와 transcript 위치를 등록하고 즉시 반환합니다. 첫 자식 모델 요청에서 gateway가 설정된 Claude projects 루트 안의 해당 agent metadata만 최대 16KiB 읽습니다. 동시 첫 요청은 같은 검증 Promise를 공유합니다. 부모 호출·역할·부모 agent가 일치해야 배정하며 사용한 호출 ID는 소비합니다. 파일 누락·쓰기 중 JSON은 최대 1.5초 재확인하고, 권한 거부는 재시도하지 않습니다. 확인 실패 시 AGENT_SELECTION_UNVERIFIED와 고정 원인 코드로 upstream 전송 전에 중단합니다. 이전 snapshot을 새 재개 호출로 재사용하지 않습니다. 파일·prompt·인증 원문은 진단에 포함하지 않습니다.

대기 중 호출 정보는 최대 1024개이며 새 호출 기록 시 5분 지난 미사용 항목을 정리합니다. 이는 진행 중 에이전트의 실행 시간 제한이 아닙니다. 오래 지연된 생성이나 지원하지 않는 native 생성 경로는 검증 불가 오류가 날 수 있습니다. selectionSource=explicit-metadata/role-default/native-inherit/skill-result/verified-resume으로 적용 근거를 구분합니다. 직접 Agent의 실제 순차·병렬 A/B 및 fork Skill의 모델 요청은 확인했고 전체 code-review 완료·workflow·수정 후 재개는 미검증입니다.

SendMessage 재개에서는 native가 최초 생성 toolUseId를 유지합니다. 세션 전용 PostToolUse:SendMessage hook이 성공한 실제 메시지 전달의 호출 ID·대상·부모만 연결합니다. 기존 자식의 검증된 metadata와 일치하고 새 성공 호출이 있어야 이전 snapshot을 재개에 사용할 수 있습니다. 검증 이력은 최대 1024개이며 프로세스 재시작·이력 퇴출 후에는 이 경로를 사용할 수 없습니다. 공유 metadata 검증은 각 HTTP 요청의 취소와 분리하며 Stop·등록 교체·gateway 종료 시 취소합니다.

단일 completed task-notification으로 이미 종료한 부모가 복귀하는 경로는 `verified-completion-resume`으로 구분합니다. native transcript의 최상위 origin·세션·수신자·알림 UUID, 검증된 부모자식 metadata, gateway가 전달한 자식의 마지막 end_turn 응답 ID를 함께 확인합니다. 부모·자식 JSONL은 각각 끝 1MiB만 읽고 원문을 저장하거나 진단에 노출하지 않습니다. 완료 증거는 한 번 소비하고 원래 모델·부모·reviewContext를 유지합니다. 중단 표식·변조·중복·5분 초과 지연은 거부하며, 결합된 여러 알림과 실패/취소 알림 복귀는 지원하지 않습니다. 로컬 합성/loopback은 통과했고 실제 수정 버전 복귀는 미검증입니다. [상세 근거와 한계](audit-2026-09-09-completion-resume.md).

background fork Skill은 meta.toolUseId가 없는 native 경로다. Clauduct 세션 전용 PostToolUse:Skill hook이 성공한 fork 결과의 agentId와 해당 tool_use_id, commandName, 부모 ID만 전달한다. gateway는 원래 전달한 Skill 호출과 대조한 뒤 자식 metadata.name과 연결한다. 스킬 이름이나 시간 순서만으로 연결하지 않으며 직접 Agent의 toolUseId 검증은 유지한다. 화면에 표시되는 결과 문자열에서 ID를 추출하지 않는다. inline 및 background가 아닌 Skill 결과는 연결 hook에서 무시하며, 동기 fork·다른 workflow 경로를 지원한다고 주장하지 않는다.

**compact 전용 최적화:** 설치 Claude 2.1.263의 compact 요약 요청 시작·끝 문구가 최종 user 메시지에서 공백·줄바꿈 차이를 제외하고 일치할 때만 같은 모델의 effort를 최대 medium으로 낮춥니다. 비교만 정규화하며 upstream에 보내는 원문은 바꾸지 않습니다. low는 유지하고, 일반 작업 및 다음 요청의 기본값은 변경하지 않습니다. `compact-policy.mjs`는 텍스트 호환 분류이며 native 출처 인증이나 도구 권한 판단이 아닙니다. 동일 템플릿을 직접 입력해도 분류될 수 있고, native 템플릿 문구가 변경되면 자동 적용하지 않고 기존 effort를 유지합니다. 공식 compact 전용 effort API는 확인되지 않았습니다. 헤더나 전역 설정을 위장·변경하지 않습니다.

`request-status.mjs`의 `purpose`, `requestedEffort`, `effort`로 실제 적용을 확인합니다. `role`, `roleRegistered`는 역할 연결을, `clientContextPolicy`는 실행 자식에 상속된 수치만 보여줍니다. 환경값 확인을 실제 backend 용량이나 자동 발동의 증거로 대체하지 않습니다.

자동 압축을 낮은 비용으로 확인하려면 실행기에 `--verify-auto-compact`를 붙입니다. 이 실행의 AUTO_COMPACT_WINDOW만 100000이며 500K 모델 창과 83.3333% 정책은 유지합니다. 기본 출력 예약량 20K이면 약 66,666토큰에 자동 발동하는 조건입니다. 이 옵션 없이 다음 실행하면 원래 500K/400K 정책으로 돌아갑니다. 전역 설정이나 disable compact 설정을 덮어쓰지 않습니다. 실제 400K 발동 검증을 대신하지 않습니다. [후속 검증 절차](remaining-verification.md).

## 설정과 역할 hook

Workflow 명시 선택의 metadata.model은 기존 지원 GPT이며 native 요청 모델과 일치할 때만 허용합니다. sidecar가 요청을 덮어쓰지는 않습니다. 927eefdf의 IDENTITY 수정 후 660a5d7d에서 명시 luna/high·Read·결과 복귀가 실제 통과했습니다. 같은 run 혼합 자식은 로컬 검사만 통과했습니다. [검증 범위](audit-2026-09-09-workflow-mixed-children.md).

local inline Workflow는 인증된 PostToolUse 결과의 호출 ID·script SHA256·run 경로와 live SubagentStart를 연결하고, 해당 run의 journal·nested metadata·자식 transcript를 확인합니다. 검증 후 `selectionSource=workflow-result`, `role=workflow-subagent`로 기록하며 native 첫 요청의 GPT 모델/effort를 고정합니다. 같은 모델에서 effort가 생략되면 호출 당시 부모의 실제 effort를 사용합니다. 이 경로만 text를 완료 검증까지 지연하고 reasoning 뒤에 하나의 text 블록으로 전달합니다. 08594a7e에서 기본 자식 sol/high·Read·정상 result·메인 복귀를 실제 확인했습니다. 다중 자식은 실제 미검증이고 중첩·resume·custom agentType은 지원 범위 밖입니다. 전역 설정은 변경하지 않습니다. [검증과 한계](audit-2026-09-09-workflow-mixed-children.md).

직접 입력한 slash 명령의 최상위 background fork는 모델의 Skill 호출 ID가 없을 수 있습니다. 이 경우 인증된 SubagentStart 등록과 정확한 자식 경로의 metadata, `.forked-skill.marker.json`, `.forked-skill.json`을 함께 확인합니다. 부모 없음·spawnDepth=1·general-purpose, 세 파일의 스킬 이름 일치, gateway 검증기 생성 이후에 만들어진 두 fork 파일을 요구합니다. 이름만으로 연결하지 않으며 각 파일은 기존 projects 루트 검사와 16KiB 제한으로 읽습니다. 이름이 같은 대기 중 모델 Skill 호출이 있으면 이 경로를 사용하지 않고 기존 PostToolUse 연결을 기다립니다. 진단 근거는 `native-fork`이며 스킬의 low/high 검토 수준과 별개로 기존 역할 모델·effort 정책을 적용합니다.

이 경로는 native 자식의 생성 근거를 확인하는 것이며 사용자 직접 입력을 암호학적으로 증명하는 기능은 아닙니다. 동일 사용자 계정의 Claude 파일과 인증된 로컬 등록을 신뢰 경계로 사용합니다. gateway 재시작 전의 fork 파일, 중첩된 ID 없는 fork, 표식이 없는 다른 생성 경로는 지원 범위에 포함하지 않습니다. 기존 검증 이력의 1024개 한계도 유지합니다. 전역 설정이나 새로운 hook은 추가하지 않습니다.

기존 CLAUDE_CONFIG_DIR를 상속하며 없으면 native 기본 설정 경로를 사용합니다. 전용 프로필을 강제하지 않습니다. 전역 settings·keybindings·신뢰·권한은 변경하지 않습니다. `/model`의 세션 선택 동작은 사용자가 해결한 keybindings에 따르며 gateway가 native의 설정 파일 쓰기를 가로채지 않습니다.

SubagentStart/Stop hook은 자식 `--settings`에만 추가합니다. agent_id/agent_type/이벤트 종류와 Start 시 알려진 컨텍스트 환경 숫자 세 개만 인증된 loopback 등록 endpoint에 전달하며 프롬프트·transcript_path·답변은 보내지 않습니다. 환경 증거는 진단의 agentContextPolicy에 보존합니다. 신뢰를 자동 승인하거나 disableAllHooks/관리형 정책을 우회하지 않습니다. 미등록 에이전트는 요청 모델의 기본 effort로 처리하고 한 번 경고하며 `unregisteredAgentRequests`에 집계합니다. 다른 역할의 오래된 Stop은 기존 등록을 삭제하지 못합니다. 같은 ID·같은 역할 재사용의 세대 구분, resume의 hook 재발행과 native Stop 순서는 실제 관찰이 필요합니다.

statusline은 사용자가 검증한 `C:/Users/JS/.agents/scripts/claude/clauduct_statusline.sh`를 연결하며 변경하지 않았습니다. provider와 TOKEN/SECRET/PASSWORD/API_KEY 계열 환경 변수는 자식에 복사하지 않아 해당 값에 의존하는 MCP/도구는 별도 제약이 있습니다. 충돌 방지를 위해 wrapper의 `--settings`, `--agents`, `--setting-sources`, `--system-prompt` 입력은 거부합니다. 반복 native 인수와 `--` 구분자, `--model=`, `--effort=`는 처리합니다.

Anthropic 서버 Advisor는 Codex에 구현되지 않아 자식에서 공식 `CLAUDE_CODE_DISABLE_ADVISOR_TOOL=1`을 적용합니다. 서버 도구·미지원 beta를 무조건 허용하지 않습니다. 지원하는 client-side beta는 로컬에서 소비하며 Codex로 전달하지 않습니다. HTTP 의미상 중복 가능 부가 헤더는 허용하고 Content-Type/Content-Encoding/Content-Length/Transfer-Encoding 중복은 거부합니다.

## 스트리밍·도구·복귀

첫 정상 upstream 이벤트 이후에는 15초 간격으로 표준 SSE ping을 전송합니다. 추론 중 텍스트가 없어도 native 클라이언트에 연결 상태를 전달합니다. ping과 응답 프레임은 같은 쓰기 순서로 직렬화하고 backpressure를 기다립니다. ping 자체는 응답 내용으로 세지 않아 최초 내용 전의 최대 5회 재시도는 유지하며, 내용 전달 이후에는 재시도하지 않습니다. 종료·취소·실패 시 ping 타이머를 정리하고 upstream 유휴 제한은 유지합니다. 첫 내용 전송 시간(firstDownstreamWriteMs)과 pingCount/lastPingMs를 구분합니다. lastUpstreamEventMs, failureCategory, clientDisconnected로 다음 실패의 위치를 확인할 수 있습니다. clientDisconnected는 연결 종료 관찰이지 사용자가 직접 취소했다는 증거는 아닙니다.

확인된 code-review fork에서 절대 파일 경로를 대상으로 지정하면, native Bash를 통해 review-diff.mjs를 먼저 실행합니다. untracked 파일은 빈 기준 대비 전체 추가 diff로, 추적 파일은 HEAD 대비 현재 변경 diff로 제공합니다. 전체 파일 Read 강제를 제거해 low의 hunk-only/추가 탐색 없음 지침을 유지합니다. 다른 수준은 같은 diff 준비 이후 각자의 스킬 본문을 따릅니다. helper는 gateway가 직접 실행하지 않으며 native 도구 권한 검사를 통과해야 합니다. Bash가 없거나 비활성화된 경우 REVIEW_DIFF_UNAVAILABLE, 다른 명령/대상의 첫 호출은 REVIEW_DIFF_REQUIRED, 실패·누락·잘린 helper 결과는 REVIEW_DIFF_FAILED로 미완료를 드러냅니다. PR·브랜치·상대 경로 대상과 compact는 이 보완 대상이 아닙니다.

helper는 현재 작업 디렉터리 안의 일반 파일만 읽고 파일/diff를 2MiB로 제한합니다. Git 호출당 10초 제한, 외부 diff/textconv와 fsmonitor 비활성화, Git 자식의 최소 환경, filter 설정 거부를 적용합니다. Git 파일·index·HEAD를 쓰지 않습니다. 추적된 파일인데 HEAD가 없는 경우와 binary untracked 파일은 지원하지 않습니다. tracked-working-tree는 커밋 간 비교가 아니라 HEAD 대비 현재 변경이라는 범위입니다. low 실제 통과는 다른 수준의 후보 탐색·서브에이전트·검증 단계 전체 통과를 뜻하지 않습니다.

native의 custom model 표시는 실행 시 설정이라는 뜻으로 `[startup configuration]`을 붙입니다. 이는 상단 native UI를 실시간 gateway 표시로 바꾸는 기능이 아닙니다. 모델 선택기와 기존 statusline은 유지하며 request-status의 requestedModel은 native 요청 모델, model/effort는 gateway가 선택한 실제 전송 값입니다. 두 값은 역할 배정 때문에 다를 수 있습니다.

`mid-conversation-tool-changes-2026-07-01`은 native gateway에서 처리합니다. system 메시지의 `tool_addition`/`tool_removal`을 순서대로 적용해 현재 Codex 요청의 tools 목록을 만듭니다. 대상은 최상위 tools에 선언된 `tool_reference.name`이며 제거된 도구는 과거 호출 이력이 있어도 현재 목록에서 제외합니다. beta가 있는 요청에서는 defer_loading 도구를 향후 addition까지 숨겨 둘 수 있습니다. beta와 변경 블록은 upstream에 전달하지 않습니다. Anthropic 서버 MCP의 `mcp_tool_reference`/`mcp_toolset_reference`는 미지원으로 거부합니다. native client MCP가 일반 function으로 제공하는 경로와는 다릅니다.

Claude Code → gateway → Codex 텍스트 delta를 즉시 전달합니다. 느린 클라이언트에는 write backpressure를 기다립니다. 도구 인수·최종 snapshot·usage 및 정상 EOF를 검증한 뒤에만 tool_use와 message_stop을 보냅니다. 전송한 텍스트 뒤에 검증 실패/연결 단절이 생기면 SSE 오류를 표시하고 자동 재실행하지 않습니다. 텍스트를 되돌릴 수는 없으며 실패 응답의 도구 실행을 차단합니다.

게이트웨이는 도구를 실행하지 않습니다. native가 제공한 schema를 Codex function으로 변환하며 호출 ID와 결과 연결을 검사합니다. 병렬 도구 호출과 disable_parallel_tool_use를 처리합니다. 지연 도구는 ToolSearch로 발견된 tool_reference, 기존 실행 이력 또는 명시적 tool_choice가 있는 경우만 노출합니다. 발견 경로가 없는 지연 도구는 명시적으로 거부합니다.

각 요청의 현재 transcript를 변환하므로 앞 요청의 prefix/길이 일치를 강제하지 않습니다. 압축 요약·모델 전환·resume 형태의 이력을 처리합니다. opaque reasoning은 검증·병합해 버전 표시된 redacted_thinking 데이터로 돌려주며 별도 gateway 디스크 캐시는 만들지 않습니다. native의 실제 저장/복원은 별도 검증 대상입니다. 지원하지 않는 annotations/logprobs·문서/PDF·서버 도구·context edit는 조용히 버리지 않고 거부합니다.

## 인증과 재시도

요청마다 고정된 기존 Codex 인증 저장소를 재검사합니다. Codex가 갱신한 토큰은 다음 요청부터 재사용합니다. 401이면 한 번 강제 재읽기를 시도하며 계정 변경은 거부합니다. 인증 부재/만료는 재로그인을 안내합니다. 자체 refresh·자동 로그인·credential 쓰기는 하지 않습니다. Codex CLI 0.153.4 버전 검증과 기존 runtime 검사를 유지합니다.

일시적인 I/O·429·5xx만 **최초 시도 + 최대 5회 재시도**합니다. 인증 재읽기 후 재전송도 이 예산에 포함됩니다. 본문 검증 오류는 재시도하지 않습니다. gateway가 사용자 응답 전송을 시작하면 자동 재시도를 금지하고 명시적 재개를 안내합니다. Claude의 자체 재시도가 이 횟수를 곱하지 않도록 자식 `CLAUDE_CODE_MAX_RETRIES=0`, `CLAUDE_CODE_RETRY_WATCHDOG=0`, `CLAUDE_CODE_RESUME_INTERRUPTED_TURN=0`을 설정합니다. 부모 환경과 일반 Claude 실행은 바꾸지 않으며 명시적 `--continue`/`--resume`는 유지합니다.

재시도 대기는 취소 가능하며 exponential delay는 100 ms부터 최대 2초, Retry-After는 최대 5초로 제한합니다. 연결은 keep-alive로 재사용하며 유휴 연결은 최대 2개입니다. API 오류 본문이나 인증 값은 진단에 포함하지 않습니다.

## 메모리와 장기 실행

세션 수명·누적 요청·누적 연결 예산은 없습니다. 사용 가능 메모리에서 OS 여유분과 진행 중 요청의 예약량을 고려해 새 요청을 FIFO 대기시킵니다. 기본 OS 여유분은 물리 메모리의 10%(최소 256 MiB, 최대 1 GiB), 요청당 예약량은 128 MiB입니다. 실제 할당량의 엄밀한 상한이 아닌 보수적 입장 제어이며 진행 중 작업을 강제 중단하지 않습니다. 대기 요청은 최대 128개이고 초과 시 명시적 오류를 반환합니다. 누적 실행 횟수 제한과는 다릅니다.

| 요청별 보호 장치 | 값 |
|---|---|
| 요청 본문 | 32 MiB |
| 응답 전체 | 16 MiB |
| SSE frame / output item | 8 MiB |
| 응답 이벤트 | 100,000 |
| 요청 본문 수신 watchdog | 입장 후 5분 |
| 요청 header watchdog | 1분 |
| upstream 무응답 watchdog | 10분 |
| downstream backpressure 대기 | 30초 |
| gateway 종료 정리 | 기본 2초, 실패는 CLEANUP_FAILED |

응답 본문 전체를 중복 배열로 쌓지 않고 incremental UTF-8/SSE parser와 제한된 item 상태를 사용합니다. 최종 snapshot 검증에 필요한 데이터는 유지합니다. 종료 시 대기 요청·진행 요청·소켓을 정리하며 transport가 정리에 실패하거나 멈추면 성공으로 숨기지 않습니다.

이 정책이 native Claude 자체의 모든 제한을 없애지는 않습니다. 공식 문서상 동시 subagent 기본 20개, 중첩 깊이 기본 3단계이며 해당 제한은 조정 가능하지만 해제 불가입니다. 총 subagent 누적 생성 제한은 v2.1.224부터 제거됐습니다. 사용자/관리형 설정의 명시적 turn·budget 제한과 도구별 timeout은 유지합니다.

2026-09-08 로컬 CIM 확인 당시 `C:\pagefile.sys`가 **32 GiB 할당**되어 활성 상태였습니다. 자동 관리 설정은 false, 당시 사용량은 1,717 MiB였습니다. 설정은 바꾸지 않았습니다. pagefile 존재는 메모리 부족/성능 저하가 없다는 보장이 아닙니다.

## 컨텍스트 500K / 자동 압축 400K

`src/models.mjs`의 CONTEXT_POLICY를 자식 환경과 settings.env로 메인·서브에이전트에 전달합니다.

```text
CLAUDE_CODE_MAX_CONTEXT_TOKENS=500000
CLAUDE_CODE_AUTO_COMPACT_WINDOW=500000
CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=83.33333333333334
```

설치 Claude 2.1.263에서 출력 예약량 min(outputTokens,20000)을 제외하고 `min(floor(effectiveWindow*pct/100),effectiveWindow-13000)`으로 계산하는 경로를 정적 확인했습니다. 기본 예약량 20K일 때 400K이며 80%는 384K이므로 사용하지 않습니다. 다른 출력 예약량·선제 압축·자동 압축 비활성화 설정은 실제 시점을 바꿀 수 있습니다. modelPicker의 Claude behavesAs identity를 제거해 알려진 Claude 용량으로 덮이지 않게 했습니다.

인증된 로컬 `/clauduct/status`는 main/subagent의 관찰된 최대 입력 토큰(캐시 포함), contextPolicy, 자원·재시도 진단을 제공합니다. 토큰 관찰만으로 실제 압축 성공을 선언하지 않으며 `contextPolicyRuntimeVerified:false`를 유지합니다. 이 설정은 backend의 물리적인 context 수용량을 확대하지 않습니다.

backend가 max_output_tokens를 거부했던 기존 증거 때문에 이 필드를 보내지 않습니다. 완료 usage의 출력 한도를 도구/최종 완료 전달 전에 검사하지만 **서버 생성량·과금의 사전 상한은 보장하지 않습니다**. 이미 스트리밍한 텍스트는 출력 한도 실패 시에도 남을 수 있습니다.

## 검증 범위와 남은 공백

신규 protocol/transport/gateway/launcher/admission 합성 검사와 기존 native/chat/PoC 회귀를 사용합니다. 테스트는 고정 합성 데이터와 loopback HTTP이며 실제 Claude·인증 파일·외부 모델을 실행하지 않습니다. **Verified:** native 41/41, gateway 15/15, chat 26/26, adapter 318/318, user-session 89/89, 신규 protocol/transport/launcher/admission suite 통과. 60초 soak 포함 native 42/42, soak 요청 3,908회, activeJobs 0, RSS 증가 50,774,016 bytes였습니다. 최종 launch retry 환경 보완 뒤 native 41/41을 다시 확인했습니다. [감사 수정·검증 기록](audit-2026-09-08.md)에 항목별 결과와 미검증 범위를 기록했습니다.

```powershell
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-protocol.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-transport.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-gateway.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-launcher-native.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-request-admission.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-child-process src/test-native.mjs --soak-seconds 60
```

**실제 검증:** 사용자 화면에서 점진적 출력 확인, 저장된 세션에서 Read 왕복·수동 compact 후 새 Read·일반 effort 복귀를 확인했습니다. f236 세션에서는 Explore/general-purpose=luna/max, Plan=sol/high와 각 역할의 hook 컨텍스트 환경 상속을 확인했습니다. b6d81841 세션에서는 검증용 autoCompactWindow=100000으로 trigger=auto, 98097→39330 토큰, 68608ms, compact medium 및 후속 max·새 Read 성공을 확인했습니다. 환경 상속은 native 내부 계산이나 backend 용량의 증거가 아닙니다. 진단은 모델이 재작성한 JSON이 아닌 원본 tool_result로 검사합니다.

**Not verified:** opaque reasoning의 완전 보존, 중첩 서브에이전트 상속과 각 서브에이전트 내부 auto compact, 실제 기본 400K 압축/복귀, backend의 모든 모델/effort/500K 수락, 실제 갱신 인증 재사용, 실제 429/네트워크 단절 복구, 수시간·수일 운영. 실제 인증 갱신과 수시간 장기 실행은 이번 검증 목표에서 제외했습니다. **Blocked by:** 이전 실제 인증/live/PTY 실행 거부를 유지하므로 다른 shell/Python 경로로 우회하지 않았습니다. 후속 실제 증거는 사용자가 실행한 세션을 읽기 전용으로 확인한 것입니다.

합성 부하 검사로 누적 카운터 한도·작업/소켓 정리·메모리 추이를 저비용으로 검사할 수 있지만 장기간 무결함을 입증하지는 못합니다. 기존 shell 측정 중 생성된 `%SystemDrive%` 폴더는 삭제 guard가 거부하여 그대로 보존했습니다.

공식 근거: [환경 변수](https://code.claude.com/docs/en/env-vars), [재시도 오류 처리](https://code.claude.com/docs/en/errors), [hooks](https://code.claude.com/docs/en/hooks), [서브에이전트](https://code.claude.com/docs/en/sub-agents), [Advisor 비활성화](https://code.claude.com/docs/en/advisor#turn-the-advisor-off), [HTTP 필드 중복 규칙](https://www.rfc-editor.org/rfc/rfc9110.html#section-5.3).
