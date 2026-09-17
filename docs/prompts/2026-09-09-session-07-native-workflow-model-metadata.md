# Workflow 명시 모델 metadata 수정 후 시험

이 문서를 먼저 읽고 실행하라. 사용자는 새 Clauduct 프로세스를 이미 실행했다. 위치는 D:/AIDEV/Clauduct/verification/dev-sandbox/run-01이다. native Workflow 1회·읽기 전용 자식 1개에 명시 GPT model/effort를 적용하는 시험을 요청한다. 메인 설정과 기존 역할 기본값을 변경하지 않는다.

Verified: 기본 상속·Read·결과 복귀는 08594a7e에서 성공했다. 927eefdf의 명시 luna/high는 metadata.model 존재를 잘못 거부해 IDENTITY로 실패했다. 요청과 sidecar 모델이 일치하면 허용하도록 수정했고 실제 형식 fixture로 Workflow 32개 검사가 통과했다. Not verified: 수정 후 실제 명시 선택·Read·완료 복귀. 이번에는 이 경로만 확인한다.

## 1. 기준점

native Workflow 존재와 Skill workflow-authoring의 agent(prompt, opts) model/effort 계약을 확인한다. 없거나 확인 실패면 중단한다. 다른 개발 workflow 스킬은 실행하지 않는다.

필요하면 D:/AIDEV/Clauduct/src/request-status.mjs를 읽고 메인의 허용된 셸에서 다음 명령을 한 번 실행한다. timeout은 60초 이하로 한다.

```text
node D:/AIDEV/Clauduct/src/request-status.mjs
```

필터링된 JSON 전체를 출력하고 현재 메인의 실제 model/effort, sessionRef, correlationScope, lifetime.started/succeeded/failed를 기록한다. 메인 값을 식별할 수 없으면 추정하지 말고 중단한다.

이번 자식 목표: 메인이 gpt-5.6-luna가 아니면 gpt-5.6-luna/high, 메인이 gpt-5.6-luna이면 gpt-5.6-terra/xhigh. 선택한 목표를 보고하되 메인의 설정은 그대로 유지한다.

## 2. 단일 Workflow

Workflow에는 inline script만 전달한다. name/scriptPath/resumeFromRunId 입력이나 script 파일 저장은 하지 않는다.

첫 문장은 순수 리터럴 export const meta이며 name='clauduct-workflow-explicit-probe', description='Read once with an explicit GPT model and effort'다. await agent를 한 번 호출하고 opts에는 label='explicit-read-once', 위에서 선택한 전체 GPT model ID, effort만 명시한다. agentType/schema/isolation은 생략한다.

자식 prompt:
“Read 도구로 D:/AIDEV/Clauduct/src/models.mjs를 정확히 한 번 읽어. MODELS 객체의 네 키를 첫 줄에 쉼표로 구분해 모두 출력해. 현재 네 실행 모델 하나만 답하지 마. 두 번째이자 마지막 줄에 WORKFLOW-EXPLICIT-COMPLETED를 출력해. 읽기에 성공하고 네 키를 모두 확인한 경우에만 완료 문구를 출력해. 코드의 정의를 실제 실행 모델의 증거라고 주장하지 마. 파일 수정, Bash, Skill, Agent, Workflow, 외부 조회는 하지 마. 실패·거부 시 우회·재시도하지 말고 실패를 보고해.”

Workflow는 { completed: boolean, result: string|null }를 반환한다. result는 실제 문자열 그대로, 비문자열이면 null이다. completed는 네 이름 astra, sol, terra, luna가 모두 있고 마지막 줄이 WORKFLOW-EXPLICIT-COMPLETED일 때만 true다. 누락 내용을 덧붙이지 않는다.

native 완료 알림을 기다리되 TaskOutput·반복 polling·추가 자식·병렬·중첩·resume·재호출은 하지 않는다. 알림 전에는 대기 상태만 보고하고 성공 판정을 보류한다.

## 3. 종료 진단

성공 또는 terminal 실패를 확인하면 같은 상태 명령을 한 번 더 실행하고 JSON 전체를 출력한다. 다음을 보고한다.

- Workflow 호출 ID·task/run ID·알림 상태·실제 result·completed. 보이지 않는 값은 추정하지 않는다.
- 새 자식 request, agentRef, requestedModel, model, effort, role, roleRegistered, selectionSource, success.
- 종료점 메인 model/effort가 기준점과 같은지, sessionRef/correlationScope 일치, lifetime.failed 증분.
- 실패 요청 전부의 failureStage, failureCategory, selectionFailure, selectionIoCode, completionFailure, attempts. null도 그대로 표시한다.

통과 조건은 자식이 명시 목표 model/effort로 실행되고 workflow-subagent·roleRegistered=true·workflow-result·success=true이며, 네 이름·완료 문구·completed=true가 메인까지 복귀하는 것이다. 메인 선택도 유지되어야 한다. requestedModel만으로 backend 실행을 입증하지 않는다. Read 기록이 직접 보이지 않으면 반환 내용과 직접 관찰 증거를 구분한다. 마지막 상태 이후 요청은 누계 밖이다.

## 중단·보존

파일 생성·수정·삭제, 테스트, 설치, 웹 조회, worktree·브랜치 작업, commit/push, 설정·권한·hook 변경을 금지한다. native의 통상적인 로컬 세션 기록 외에 파일을 저장하지 않는다. 인증값·전체 환경·원본 SSE·원본 세션 파일은 출력하지 않는다.

권한/guard 거부나 사용자 중단이면 추가 진단 없이 중단한다. 대체 도구·셸·옵션으로 우회하지 않는다. Workflow API/자식 실패·빈 결과·모델 불일치는 허용된 종료 진단 후 종료한다. 상태 명령 실패 시 다른 인증·로그 경로를 시도하지 않는다. 실패한 시험을 재호출하지 않는다.

최종 보고는 Verified / Not verified / Blocked by로 구분한다. 실패 누계 0을 업무 성공으로 대신하지 않는다. 다중 자식·명시 inherit 문자열·custom agentType·중첩·재개·장기 안정성은 이번 범위 밖이다.
