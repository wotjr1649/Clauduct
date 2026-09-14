# 설치 native 모델 호출 경로 조사

현재 버전 주의: 후속 조사에서 활성 실행 파일과 session-12가 2.1.267임을 확인했다. 아래 최초 목록의 2.1.266 전체 근거를 새 버전 전체 검증으로 해석하지 않는다. away_summary/side_question 일부 경로는 [2.1.267 대조](audit-2026-09-10-away-summary.md)를 참조한다.

기준: 2026-09-09, 코드 수정 da2e050 이후. 현재 실행 대상 C:/Users/JS/.local/bin/claude.exe와 C:/Users/JS/.local/share/claude/versions/2.1.266은 SHA256이 같고 크기는 218971808 bytes다. 실행 파일은 실행하지 않고 읽었다. 활성 plugin 설정의 이름·버전·scope·설치 경로와 hook 종류만 추출했다. 인증값·설정 원문·사용자 대화·reasoning은 기록하지 않았다.

이 문서는 발견 목록과 판정의 첫 대조표다. 설치 skill 23개는 목록화했지만 native 전체 기능의 동적 활성화, 모든 호출자의 header·provider·fallback 추적 및 실제 반환 검증은 미완료다. 문자열 검색에서 발견하지 못한 동적 querySource나 원격 기능을 없다고 단정하지 않는다.

## 경로별 모델 정책

- M: 메인 요청의 model/effort를 선택한다. astra/medium, sol/xhigh, terra/high, luna/max가 기본값이다.
- A: 검증된 Agent/Task 호출·metadata와 실행기 등록 정의로 모델을 선택한다. 명시 inherit는 생성 시점 직접 부모의 실제 model/effort snapshot을 적용한다. 단순 native 요청 모델 유지가 아니다. 역할 기본값은 Explore/general-purpose=luna/max, Plan=sol/xhigh다. role=claude는 역할 강제 모델이 없어 요청 모델을 사용한다. 모든 자식의 컨텍스트 환경 정책은 500K/400K이며 backend 실제 수용량을 증명하지 않는다.
- C: 일치하는 compact 템플릿에서만 같은 모델의 effort를 최대 medium으로 낮춘다. 일반 요청에는 적용하지 않는다.

gateway는 native 도구를 실행하지 않고 모델 요청·응답을 변환한다. 따라서 Read/Edit/Bash/Skill 등의 도구 실행 권한과 실행 결과는 native 쪽 책임이다. native의 command type=local/local-jsx는 모델 미호출을 뜻하지 않는다.

## 핵심 경로 판정

| 경로 | 실행 주체·gateway 경유 | 모델·결과 연결 | 판정·근거 |
|---|---|---|---|
| 일반 대화 | native 메시지 API → gateway → Codex | M, 스트리밍 최종 반환 | 실제 Read 왕복 등 부분 통과, 전체 도구 행렬 미완료 |
| Read/Edit/Write/Bash | 실행은 native, 선택·결과를 읽는 모델 요청은 gateway | M/A, tool_use_id ↔ tool_result | 프로토콜 합성 검사, 실제 Read/Bash 부분 증거. 도구 자체를 모델 호출로 세지 않음 |
| ToolSearch·지연 도구 | 검색 실행 native, 현재 도구 목록을 gateway가 변환 | M/A, tool_reference 및 turn tool addition/removal | 로컬 검사 통과, 설치된 모든 도구별 실제 미검증 |
| Agent/Task: Explore/general-purpose | native 생성·hook·metadata → gateway | A, 원래 부모/호출 ID 검증 | 순차·병렬 실제 부분 통과 |
| Agent: clauduct-<모델>-<effort> 14개 | 세션 한정 정의·native 생성·hook·metadata → gateway | definition-model / definition-inherit, 직접 부모 snapshot | 네 GPT 직접 선택, 직접 부모 기준 손자·비기본 effort 및 완료 복귀 실제 확인. [범위별 증거](audit-2026-09-10-agent-acceptance.md). 모든 조합·Task 도구 실제 실행 증거는 아님 |
| Agent/Task: Plan | 동일 | 기본 sol/xhigh | c94bb0ac request 10/11에서 무명시 Plan/role-default/sol/xhigh 실제 통과. [근거](audit-2026-09-09-plan-lifetime-success.md) |
| native role=claude | HIGH-03 metadata에서 실제 확인 | 요청 모델, 부모 연결 유지 | 78986b2에서 status whitelist 보완, 새 표시의 실제 출력은 미검증 |
| inline Skill | native가 스킬 내용을 모델 문맥에 제공 | M/A, 별도 fork가 없으면 현재 요청 정책 | 일반 변환 지원, 개별 skill 전체 실행 미검증 |
| background fork Skill | native fork 결과·PostToolUse → gateway | A, skill-result | 실제 생성/Read 통과. 모든 skill fork 조합 미검증 |
| 직접 code-review 명령 fork | native fresh marker/scope·Start → gateway | luna/max, native-fork | low/high 실제 최종 반환 증거. high 오류 없는 전체 완료는 미검증 |
| SendMessage 복귀 | native 성공 PostToolUse → gateway | 기존 모델·부모·reviewContext, 1회 소비 | verified-resume 및 verified-peer-resume 실제 부분 통과 |
| completed task-notification 복귀 | native JSONL origin·최종 응답 ID → gateway | 기존 정책, verified-completion-resume | c19c8b14 실제 성공 확인(adb4d26), 복수·실패 알림은 범위 밖 |
| 결합 완료/failed/killed/blocked 알림 복귀 | native wake router | 완료 증거로 사용하지 않음 | 신규 완료 복귀 경로에서 미지원, 조용한 성공 처리 없음 |
| Workflow | native Workflow → local inline run → workflow-subagent | 인증된 호출·run·nested metadata 연결. 명시 sidecar 모델은 요청과 대조. 최종 text는 reasoning 뒤로 전달 | 5cd82157 순차·992c0794 병렬 혼합 자식 2개 실제 통과. 병렬 요청 약 4.77초 겹침·결과 복귀 확인. 중첩·resume·custom agentType은 별도 범위. [실제 병렬 성공](audit-2026-09-10-workflow-parallel-success.md) |
| /compact·자동 compact | 명령은 local, 내부 querySource=compact는 모델 호출 | C, 요약 뒤 새 요청 연결 | 수동·낮춘 임계값 자동 압축 실제 통과, 기본400K·각 자식은 미검증 |
| /btw | local-jsx → side_question → 공통 query/client | cacheSafeParams의 agentContext 유지. 메인 문맥이면 자식 header 없이 메인 경로. skipTranscript=true | [session-12 결과와 정적 경로](audit-2026-09-10-native-btw-path.md). 사용자 제공 UI 답변 성공, 관찰 요청 sol/high. 취소 2건의 귀속·전체 beta 호환성은 미확정 |
| away_summary | 저장된 메인 문맥 → 보조 query → system 요약 기록 | 2.1.267 b4e/XD/hk/kSn/oB/knr 경로. 특정 모델 강제 없음 | d9752fc6에 실제 요약 172자 기록. 직전 요청 13 sol/high 성공과 약 393ms 차이, 정확한 귀속은 추론. [증거·버전 경계](audit-2026-09-10-away-summary.md) |
| /fork·/subtask | native 정의에 백그라운드 에이전트/세션 생성 존재 | fork 유형에 따른 ID·모델 경로 미추적 | code-review fork와 동일 지원이라고 단정하지 않음 |
| /init | builtin prompt 등록 확인 | 모델이 문서·설정 작업을 생성 | GPT 경로 실제 미검증, 설정/지침 쓰기는 별도 권한 대상 |
| /batch | builtin prompt 등록, 병렬 worktree 및 PR 작업을 기술 | 하위 Agent 정책·원격 쓰기는 별도 | 미검증, PR/push를 자동 실행하지 않음 |
| /debug | builtin prompt 및 debug 저장 활성화 경로 | 진단 프롬프트 뒤 현재 모델 | 미검증, 로그 원문 노출 위험 때문에 실행하지 않음 |
| /claude-api | builtin skill 등록·allowed WebFetch·files 로더 확인 | 문맥/도구를 공급, 자체 등록을 Claude API 호출로 세지 않음 | HIGH-03 Skill 호출 3회. 모든 후속 코드의 provider까지 입증한 것은 아님 |
| /security-review | plugin 연결 또는 내장 프롬프트 대체 경로 존재 | 현재 실행 경로별 확인 필요 | 미검증 |
| /model·/context·/usage(cost/stats)·/skills | 로컬 설정/표시 명령 등록 확인 | 후속 요청 모델 선택·표시 또는 계정 조회와 구분 | 명령 전체를 GPT 변환 대상으로 세지 않음. /usage의 외부 조회 여부 전수 추적 미완료 |
| /agents | 설치 버전에서는 제거 안내를 내는 local 명령 | 후속 자연어 요청이 별도 Agent 작업을 만들 수 있음 | 과거 wizard 존재를 가정하지 않음 |
| /hooks·/permissions·/plugin(plugins/marketplace) | native 설정 관리 UI | 관리 행위 자체와 후속 모델 호출을 구분 | 전역 설정·trust·설치 변경은 이번에 실행하지 않음 |
| /plan·/resume·/clear | 로컬 상태/세션 관리 후 모델 작업 가능 | M/A 및 복원된 문맥 | 각 세션 재개·자식 재기동·hook 재발행을 구분해야 함 |
| 서버 Advisor·서버 tools/MCP·서버 fallback | Anthropic 서버 기능 | Codex 대체 계약 없음 | Advisor는 Clauduct 자식에서 비활성화, 알려진 미지원 beta는 명시 거부 |

## native 내부 보조 호출 발견 목록

설치 코드의 querySource 문자열 리터럴을 중복 제거한 34종이다. 이는 실제 발생 요청 수나 전체 호출 경로 수가 아니다. 동적 agent 역할, 기능 플래그, 계정별 활성화, 다른 provider로 직접 나가는 경로를 이 검색만으로 확정할 수 없다. compact 외의 아래 보조 경로는 개별 GPT 반환·선택 모델·연결 header를 실제 확인하지 않았다.

| 분류 | 발견한 querySource | 현재 판정 |
|---|---|---|
| 일반·에이전트 | sdk, agent:custom, agent_classifier, agent_namer, agent_summary | 일반/Agent 실제 증거와 별개로 보조 classifier/namer/summary 미검증 |
| 압축·메모리 | compact, extract_memories, auto_dream | compact 일부 통과, 메모리·dream 미검증 |
| 권한 자동화 | auto_mode, auto_mode_critique, auto_mode_setup_propose | 자동 권한 분류와 실제 사용자 허용을 혼동하지 않음, 미검증 |
| hook | hook_agent, hook_prompt | 모델을 부르는 hook 경로 존재. 현재 사용자/plugin 설정에서 prompt/agent hook은 발견되지 않음 |
| 이름·요약·보조 UI | away_summary, generate_session_title, narration, prompt_suggestion, rename_generate_name, side_question, teleport_generate_title, tool_use_summary_generation | 미검증 |
| 웹·시간 | web_fetch_apply, web_search_tool, mcp_datetime_parse | 일반 function WebFetch와 서버 WebSearch의 경계 별도 확인 필요, 미검증 |
| artifact 댓글 | artifact_comment_analyst, artifact_comment_fast_ack, artifact_comment_reply, artifact_comment_triage | 외부 공유 상태·입출력이 포함될 수 있어 실행하지 않음, 미검증 |
| 진단·검증·sampling | feedback, insights, model_validation, plugin_eval_judge, plugin_eval_mock, repl_sampling | 미검증. 플러그인 평가용 mock 문자열을 실제 gateway 성공 증거로 쓰지 않음 |

## 설치 skill 23개 대조

설치·활성 plugin은 superpowers@claude-plugins-official 6.3.0 및 ponytail@ponytail 4.9.0 두 개, 모두 user scope다. 사용자 skill 디렉터리에는 3개가 있다. 별도 사용자 commands/agents 디렉터리와 작업 루트의 .claude는 발견되지 않았다. 설치 cache의 다른 플랫폼 설정·테스트·benchmark·MCP 구현 파일이 있다는 사실만으로 Claude에 등록됐다고 판정하지 않는다.

23개 SKILL.md frontmatter 모두 명시 model/context/agent 값이 없다. 기본 실행 및 fork 여부는 native 로더·호출 방식에 따른다. 아래의 '현재 문맥'은 독립 provider 설정이 없다는 뜻이며 skill 자체가 모델 없이 완료된다는 뜻이 아니다. 본문에서 직접 provider 주소나 codex exec/claude -p 문자열은 발견되지 않았으나 모든 참조 스크립트의 무우회를 입증한 것은 아니다. 각 skill의 전체 GPT 실행은 미검증이다.

| 제공자 | skill | 명시 설정·호출 특성 |
|---|---|---|
| superpowers | brainstorming | 현재 문맥; visual companion 보조 서버는 모델 호출과 별도 |
| superpowers | dispatching-parallel-agents | 위임 지침; 실제 생성되면 Agent/Task 경로 |
| superpowers | executing-plans | 하위 skill/위임 경로 언급 |
| superpowers | finishing-a-development-branch | 현재 문맥; 원격 branch 작업 권한과 별도 |
| superpowers | receiving-code-review | 현재 문맥 |
| superpowers | requesting-code-review | general-purpose reviewer 위임 지침 확인 |
| superpowers | subagent-driven-development | 위임 경로 포함 |
| superpowers | systematic-debugging | 현재 문맥 |
| superpowers | test-driven-development | 현재 문맥 |
| superpowers | using-git-worktrees | 현재 문맥; Git 작업은 native 도구 실행 |
| superpowers | using-superpowers | SessionStart에서 지침 공급, 다른 skill/위임 언급 |
| superpowers | verification-before-completion | 현재 문맥 |
| superpowers | writing-plans | 문서 검토 위임 경로 포함 |
| superpowers | writing-skills | subagent 기반 검사 경로 포함 |
| ponytail | ponytail | disable-model-invocation:true |
| ponytail | ponytail-audit | disable-model-invocation:true |
| ponytail | ponytail-debt | disable-model-invocation:true |
| ponytail | ponytail-gain | disable-model-invocation:true |
| ponytail | ponytail-help | disable-model-invocation:true |
| ponytail | ponytail-review | disable-model-invocation:true |
| 사용자 | grilling | 현재 문맥 |
| 사용자 | prompt-generator | 현재 문맥 |
| 사용자 | writing-for-agents | 현재 문맥 |

ponytail의 disable-model-invocation은 설치 Claude skill에서 읽은 값이다. Codex 쪽 동일 이름 skill과 파일 내용이 같다고 가정하지 않는다. 이 설정은 모델의 자동 Skill 선택과 관련된 선언이며 사용자가 실행한 slash command의 답변 생성이 모델을 쓰지 않는다는 증거가 아니다.

superpowers manifest는 SessionStart command hook 하나로 run-hook.cmd → session-start → SKILL.md 읽기·추가 문맥 출력을 연결한다. ponytail manifest는 SessionStart/SubagentStart/UserPromptSubmit의 Node command hook 3개를 연결한다. 이들은 모델 API용 prompt/agent hook으로 등록돼 있지 않다. ponytail hook의 로컬 상태 쓰기와 지침 공급은 GPT 변환과 별개이며 이번 조사에서 실행하지 않았다.

사용자 settings의 hook은 11개 이벤트, 13개 항목 모두 type=command다. 이는 hook 스크립트 내부의 모델 호출 부재까지 입증하지 않는다. 사용자 설정에는 mcpServers 필드가 없지만 다른 설정 소스·프로젝트 등록·동적 연결의 부재를 뜻하지 않는다.

## 이벤트·라우팅 진단의 남은 설계

현재 recentRequests는 마지막 16개만 보존한다. 과거 other의 원래 이벤트명과 전체 요청의 성공/실패를 복원할 수 없다. auxiliaryMetadataEvents=0인 마지막 16개 기록은 전체 세션에서 보조 이벤트가 없었다는 증거가 아니다.

아래는 최초 조사 당시의 계측 요구다. 이후 요청 누적 카운터는 c94bb0ac에서 실제 확인했고, 불투명 관계 참조·role=claude 표시·실패 단계별 집계를 구현했다. [구현 근거](audit-2026-09-09-request-correlation.md). 새 관계 참조와 failuresByStage 필드 출력은 [session-10·11](audit-2026-09-10-agent-acceptance.md)에서 확인했다. 이 두 정상 세션의 실패 집계는 모두 0이므로 각 실패 단계의 실제 증가나 보조 이벤트 발생까지 입증하지 않는다.

1. gateway 수명 동안 고정 항목으로 성공·실패 단계·unsupportedEvent 분류·보조 이벤트 처리 횟수를 누적한다. 원래 이벤트명·본문·오류 문자열을 저장하지 않는다.
2. 세션/agent/부모는 gateway마다 바뀌는 키를 쓰는 불투명 상관관계 값으로 연결한다. request 번호만으로 특정 자식을 추정하지 않는다.
3. role=claude를 고정 진단 어휘에 추가한다. 임의 custom role 이름은 별도 고정 분류로 처리한다.
4. 실제 사용자 세션에서 새 source와 누적 카운터를 확인한다. legacy 합성 transport 배열과 실제 onEvent 경로가 모두 같은 계측을 거치는지 검사한다.

Verified: 설치 실행 파일 동일성, 활성 plugin 2개, skill 14+6+3=23개 및 frontmatter 대조, 사용자 hook 종류 13개, native querySource 리터럴 34종·일부 명령의 실제 등록 코드, 현재 Clauduct 모델/프로토콜 코드.

Not verified: 전체 동적 명령/도구/skill 목록의 완전성, 별도 설정 소스·모든 hook 내부·외부 provider 우회/fallback 전수 추적, 보조 호출별 GPT 반환과 각 실패 단계·보조 이벤트의 실제 발생 검증. 계정·외부 쓰기는 실행하지 않았다.
