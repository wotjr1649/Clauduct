# Workflow 실행 관계·metadata 연결 보완

## 원인과 변경

사용자 실행 `81ccfd9c-58f8-4a8f-aae1-bab3350a106a`의 Workflow는 `async_launched`였지만 자식은 `AGENT_SELECTION_UNVERIFIED_MISSING`으로 실패했다. native 2.1.266은 metadata를 `<session>/subagents/workflows/<runId>/agent-<id>.meta.json`에 저장한다. 기존 검증기는 일반 Agent의 평면 경로와 toolUseId 관계를 요구했다. Workflow의 completed 상태만으로 자식 성공을 판정하면 안 된다. 이 실행의 반환은 completed=false였다.

기존 자식 한정 PostToolUse matcher에 Workflow를 추가했다. 연결 순서는 다음과 같다.

1. gateway가 반환한 inline Workflow 호출 ID·script 해시·부모의 실제 model/effort를 보관한다.
2. 인증된 PostToolUse가 동일 호출의 local_workflow/async_launched 결과를 전달한다. script 원문은 보내지 않는다. run ID·task ID·정확한 script/transcript 경로를 연결하고 호출의 재사용을 거부한다.
3. live SubagentStart로 등록된 workflow-subagent만 해당 run에서 찾는다. script 해시, journal의 유일한 started 항목, metadata의 역할·설명·깊이·부모 없음, 자식 transcript의 session/agent ID·생성 시각을 확인한다. metadata·script·해당 자식 journal 항목을 재확인한다.
4. 검증된 첫 요청의 GPT 모델과 effort를 선택한다. 같은 모델의 effort 생략은 호출 당시 부모 effort로 보완한다. 다른 GPT 모델의 effort 생략은 기존 모델 기본값을 사용한다. 일반 Agent의 고정 역할 모델과 명시 inherit 정책은 바꾸지 않는다.

진단은 selectionSource=workflow-result와 role=workflow-subagent를 허용한다. 임의 역할명, script·프롬프트·응답 원문은 상태에 추가하지 않는다.

## 신뢰 경계와 제한

동일 사용자 계정의 native 파일과 인증된 loopback hook이 신뢰 경계다. 파일 이름만 일치시키거나 폴더 전체를 재귀 검색하지 않는다. journal key는 형식을 검증하며 native key 알고리즘을 재계산하는 암호학적 증명이 아니다. script는 실행하거나 파싱하지 않으므로 모델 선택은 script 옵션 자체에 대한 정적 증명이 아니라 검증된 native 자식 요청에 근거한다.

지원 범위는 최상위 새 local inline Workflow의 기본 workflow-subagent다. remote, name/scriptPath 입력, resumeFromRunId, 중첩 Workflow, custom agentType, gateway 재시작을 넘는 복원은 이번 지원에 포함하지 않는다. 일반 Agent/Skill 호환성으로 이 공백을 대체하지 않는다.

script 512 KiB, journal 128 KiB, metadata 16 KiB, 자식 transcript 1 MiB를 넘으면 거부한다. run과 자식 기록은 각각 1024개이며 자동 제거하지 않는다. 연결된 run을 순회하므로 많은 Workflow를 사용하는 장기 실행 성능은 미검증이다. 경로·인코딩·크기·관계 오류에서 upstream 호출을 허용하는 fallback은 없다. 실제 native 쓰기와 hook의 순서 차이는 기존 최대 1500 ms 대기 안에서만 수용한다.

## 검증

Verified: 아래 명령을 Node permission 제한으로 실행했다. 외부 요청과 실제 Claude 실행은 0이다. loopback 검사에서 transport 응답은 합성이며 실제 selector·hook 등록·gateway·status 연결을 검증한다.

```powershell
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-workflow-selection.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-agent-selection.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-completion-selection.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-launcher-native.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-gateway.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-protocol.mjs
```

Workflow 25개, completion-selection 46개, native-gateway 35개 통과. agent-selection, launcher-native, native-protocol suite 통과. Workflow 검사는 두 자식의 동일 run 연결, 지연 hook 연결, 비기본 부모 effort·명시 GPT/effort, 호출/경로/해시 불일치·재연결·실패 journal·중단 metadata·위조 관계·크기 초과 거부, 상태 원문 비노출을 포함한다.

Not verified: 변경 후 실제 native Workflow의 Read·GPT 실행·완료 복귀, 실제 명시 model/effort, 다중 자식, 장기 실행. Blocked by: 기존 실제 인증 실행 및 symlink 검사 제한을 유지했다. 제한을 풀거나 다른 경로로 실행하지 않았다. 전역 설정과 임의 --agents 차단은 유지했다.

다음 실제 검사는 새 Clauduct 프로세스에서 기존 읽기 전용 Workflow 시험을 한 번 수행한다. 이전 gateway에는 변경한 hook이 소급 적용되지 않는다. 자식 요청의 selectionSource/model/effort/success, 실제 Read와 완료 문자열, Workflow 반환의 completed=true 및 메인 보고를 모두 확인해야 통과다. 실패 시 자동 재시도하지 않는다.
