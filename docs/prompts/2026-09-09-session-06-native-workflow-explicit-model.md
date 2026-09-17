# native Workflow 명시 GPT model/effort 시험

이 문서를 먼저 읽고 단일 읽기 전용 시험을 수행하라. 사용자는 새 Clauduct 프로세스를 이미 실행했다. 재시작 확인 없이 진행한다. 작업 위치는 D:/AIDEV/Clauduct/verification/dev-sandbox/run-01이다.

## 목적과 상태

사용자는 native Workflow 1회와 그 내부 읽기 전용 자식 1개에 GPT model/effort를 명시하는 시험을 요청한다. 일반 Agent나 Superpowers 개발 workflow로 대체하지 않는다. 기존 역할별 고정 기본값과 메인 모델 설정은 변경하지 않는다.

Verified: 08594a7e의 기본 Workflow 자식은 메인 sol/high를 전달받고 Read·네 모델 이름·완료 문구·completed=true·메인 복귀까지 성공했다. 빈 결과 수정은 c81ca43에 있다. 로컬 loopback에서 명시 luna/high, terra/xhigh가 실제 전송 값과 status에 보존되는 검사도 통과했다.

Not verified: native agent()의 명시 model/effort가 실제 요청까지 전달되는지. 이번 시험은 이 차이만 추가한다. 근거가 필요하면 D:/AIDEV/Clauduct/docs/audit-2026-09-09-workflow-success.md를 읽되 문서의 과거 테스트 명령은 실행하지 않는다.

## 1. 작성 참조와 기준점

native Workflow가 있는지 확인하고 Skill로 workflow-authoring을 로드한다. agent(prompt, opts)의 model/effort 명시 계약을 확인할 수 없으면 중단한다. executing-plans, subagent-driven-development, using-git-worktrees는 실행하지 않는다.

필요하면 D:/AIDEV/Clauduct/src/request-status.mjs를 읽고 메인의 허용된 셸에서 다음 명령을 한 번 실행한다. timeout은 60초 이하로 지정한다.

```text
node D:/AIDEV/Clauduct/src/request-status.mjs
```

필터링된 JSON 전체를 출력하고 현재 메인의 실제 model/effort, sessionRef, correlationScope, lifetime.started/succeeded/failed를 기록한다. 현재 메인 값을 식별하지 못하면 추정하지 않고 중단한다.

자식의 이번 호출에만 적용할 목표값:
- 메인 model이 gpt-5.6-luna가 아니면 model='gpt-5.6-luna', effort='high'.
- 메인 model이 gpt-5.6-luna이면 model='gpt-5.6-terra', effort='xhigh'.

선택한 목표값을 먼저 보고한다. 이는 부모와 다른 모델·자식 모델의 비기본 effort를 구별하기 위한 시험값이다. 메인의 현재 모델·effort를 바꾸지 않는다.

## 2. Workflow 한 번 실행

작성 참조와 일치하는 inline script만 Workflow에 전달한다. name, scriptPath, resumeFromRunId 입력을 함께 지정하거나 script 파일을 저장하지 않는다.

script 구조:
- 첫 문장은 순수 리터럴 export const meta: name='clauduct-workflow-explicit-probe', description='Read once with an explicit GPT model and effort'.
- await agent(prompt, opts)를 정확히 한 번 호출한다.
- opts는 label='explicit-read-once', 위에서 선택한 model과 effort만 포함한다. agentType, schema, isolation은 생략한다.
- 반환은 { completed: boolean, result: string|null }이다. 실제 문자열을 수정하지 않는다. 비문자열이면 null이다.
- completed는 결과에 astra, sol, terra, luna가 모두 있고 마지막 줄이 WORKFLOW-EXPLICIT-COMPLETED일 때만 true다. 불완전한 결과를 보충하거나 재호출하지 않는다.

자식 prompt:
“Read 도구로 D:/AIDEV/Clauduct/src/models.mjs를 정확히 한 번 읽어. MODELS 객체의 네 키를 확인하고 첫 줄에 쉼표로 구분해 모두 출력해. 현재 네 실행 모델 하나만 답하지 마. 두 번째이자 마지막 줄에는 WORKFLOW-EXPLICIT-COMPLETED를 출력해. 읽기에 성공하고 네 키를 모두 확인한 경우에만 완료 문구를 출력해. 코드의 정의를 네 실제 실행 모델의 증거로 주장하지 마. 파일 수정, Bash, Skill, Agent, Workflow, 외부 조회는 하지 마. 거부·실패 시 우회·재시도하지 말고 실패만 보고해.”

background task는 native 완료 알림을 기다린다. TaskOutput, 반복 polling, 추가 자식, 병렬, 중첩, resume, 재호출은 하지 않는다. 알림 전에는 완료 대기 상태만 보고하고 성공 판정을 보류한다.

## 3. 종료 진단과 판정

성공 또는 terminal 실패를 확인하면 동일 상태 명령을 한 번 더 실행하고 JSON 전체를 출력한다. 다음을 관찰값 그대로 보고한다.

- Workflow 호출 ID·task/run ID, 완료 알림 상태, 실제 result와 completed. 보이지 않는 값은 추정하지 않는다.
- 새 자식 요청의 request, agentRef, parentRef, requestedModel, model, effort, role, roleRegistered, selectionSource, success.
- 종료점의 메인 model/effort가 기준점과 같은지. 현재 세션 메인 요청만 비교한다.
- correlationScope/sessionRef 일치, lifetime.failed 증분. 마지막 상태 이후 요청은 집계 밖이다.
- 실패 요청 전부의 failureStage, failureCategory, selectionFailure, selectionIoCode, completionFailure, attempts. null은 그대로 남긴다.

통과하려면 자식의 실제 model/effort가 명시한 목표값과 일치하고 workflow-subagent·roleRegistered=true·workflow-result·success=true여야 한다. result에 네 이름과 완료 문구가 보존되고 completed=true로 메인에 복귀하며 메인의 기존 모델·effort도 유지되어야 한다. 부모 상속으로 성공한 것을 명시 선택 성공으로 대체하지 않는다. Read 실행 기록이 직접 보이지 않으면 반환 내용과 직접 Read 증거를 구분한다.

## 중단과 보존

파일 생성·수정·삭제, 테스트, 설치, 웹 조회, worktree·브랜치 작업, commit/push, 설정·권한·hook 변경을 금지한다. native가 통상 남기는 로컬 세션 기록 외에 script나 보고서를 저장하지 않는다. 인증값·전체 환경·원본 SSE·원본 세션 파일을 출력하지 않는다.

권한/guard 거부 또는 사용자 중단이면 추가 진단도 실행하지 않고 즉시 중단한다. 대체 도구·셸·옵션으로 우회하지 않는다. Workflow API/자식 실패·빈 결과·모델 불일치에서는 허용된 종료 진단만 수집하고 종료한다. 상태 명령 자체가 실패하면 다른 경로로 인증이나 로그를 읽지 않는다.

최종 보고는 Verified / Not verified / Blocked by로 구분한다. 실패 누계 0을 업무 성공으로 대신하지 않는다. 다중 자식·명시 inherit 문자열·custom agentType·중첩·재개·장기 안정성은 이번 시험 범위 밖이다.
