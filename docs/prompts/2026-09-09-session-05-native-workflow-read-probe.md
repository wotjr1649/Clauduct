# native Workflow 단일 읽기 시험

이 문서를 먼저 읽고, native Workflow 도구로 읽기 전용 자식 한 개를 실행해. 사용자는 이 Workflow 1회·자식 1개 규모를 명시적으로 요청한다. Superpowers 구현 workflow나 일반 Agent 호출로 대체하지 마.

## 현재 상태와 범위

Clauduct ee4858d의 --document-first와 --gpt-agents로 시작한 새 세션을 사용한다. 이전 092ff9ab 세션에서는 문서 선로드, 강화된 테스트 17개, sol/medium의 직접 Agent inherit가 성공했다. native Workflow 내부 자식의 GPT 라우팅·metadata 연결·완료 복귀는 아직 미검증이다. 이 시험의 성공을 미리 전제하지 마.

메인 모델·effort는 사용자가 선택한 값을 유지한다. 모델 이름·effort는 자기소개가 아니라 request-status의 실제 요청으로 판정한다. 네 GPT 모델 이외의 Claude 별칭으로 전환하지 않는다.

읽기 대상은 이 문서, native workflow-authoring 작성 참조, 적용 지침, D:/AIDEV/Clauduct/src/models.mjs, 상태 프로그램 확인에 필요한 D:/AIDEV/Clauduct/src/request-status.mjs로 제한한다. 저장소 파일 생성·수정·삭제, 테스트 실행, worktree·브랜치 작업, 설치·웹 조회·설정·권한·hook 변경·commit/push는 하지 않는다. native가 Workflow 실행에 통상적으로 남기는 로컬 세션 기록 외에 스크립트·보고서를 직접 파일로 저장하지 마. 원격 Workflow는 실행하지 않는다.

## 1. native 기능 확인

현재 도구 목록에서 Workflow 존재를 확인한다. 없으면 미지원으로 보고하고 중단해. 일반 Agent 성공으로 대체하지 마.

Skill 도구로 정확히 workflow-authoring을 로드하고 작성 참조를 확인해. 이것은 이 시험에 필요한 native 참조이며 executing-plans, subagent-driven-development, using-git-worktrees는 필요 없다. 참조 로드가 거부되거나 agent()의 아래 계약을 확인할 수 없으면 중단한다.

확인할 계약: inline script는 순수 리터럴 export const meta로 시작하며, agent(prompt, opts)는 자식의 최종 문자열을 반환한다. model/effort 생략 시 native는 세션 값을 상속한다고 설명한다. 기본 Workflow 자식을 사용하고 opts.agentType도 생략한다. 따라서 이번 시험은 clauduct-inherit custom 정의 검증이 아니라 native Workflow 기본 자식의 전달 경로 검증이다.

## 2. 기준점 수집

메인의 허용된 셸에서 다음 단일 명령을 한 번 실행해 필터링된 JSON 전체를 출력한다. timeout은 60초 이하로 지정한다.

```text
node D:/AIDEV/Clauduct/src/request-status.mjs
```

correlationScope, 실제 메인 model/effort, lifetime.started/succeeded/failed를 기록해. 메인 값을 식별하지 못하면 중단한다. 메인이 luna/max이면 역할 기본값과 상속을 구별하기 어려우므로 이 시험을 실행하지 말고, 사용자가 다른 조합으로 새 시험을 시작하도록 보고해. 모델을 자동 변경하지 마.

## 3. Workflow 한 번 실행

작성 참조와 일치하는 최소 inline JavaScript를 작성해 Workflow(script=...)를 한 번 호출한다. name이나 scriptPath 입력을 함께 지정하지 않는다. script를 Write로 먼저 저장하지 마.

스크립트의 고정 구조:
- export const meta에 name='clauduct-workflow-read-probe', description='Read src/models.mjs once with one local child'를 순수 리터럴로 선언.
- await agent(...)를 정확히 한 번 호출. opts에는 label='read-models-once'만 지정한다. model, effort, agentType, isolation, schema는 생략한다.
- 자식 결과가 null이거나 예상 완료 문구가 없으면 명확한 실패로 반환한다. null을 걸러내 성공으로 바꾸지 마.
- 결과는 { completed: boolean, result: string|null }만 반환한다.
- parallel, pipeline, 반복문, 다른 workflow, 추가 agent 호출, 재시도·resume은 사용하지 않는다.

자식에게 전달할 정확한 작업:
“Read 도구로 D:/AIDEV/Clauduct/src/models.mjs를 한 번만 읽어. 모델 네 개의 코드상 이름을 한 줄로 적고 마지막 줄에 WORKFLOW-READ-COMPLETED를 출력해. 이 코드의 모델 정의를 네 실제 실행 모델의 증거라고 주장하지 마. 파일 수정, Bash, Skill, Agent, Workflow, 외부 조회는 하지 마. 읽기가 거부되거나 실패하면 우회·재시도하지 말고 실패를 보고해.”

Workflow가 background task ID를 반환하면 완료 알림을 기다린다. TaskOutput·반복 polling·재호출·resumeFromRunId는 하지 마. 완료 알림이 오기 전에 성공이라고 보고하지 않는다. 사용자 중단이 오면 새 작업을 시작하지 않는다.

## 4. 종료 진단과 판정

Workflow의 성공 또는 terminal 실패가 확인되면 메인이 같은 상태 명령을 한 번 실행해 JSON 전체를 출력한다. 이는 실패 원인 수집용이며 Workflow 재시도는 아니다. 단, 도구/guard 권한 거부가 발생한 경우에는 아래 중단 규칙을 우선한다.

다음을 보고해.
- Workflow 호출 ID와 반환된 task/run ID, 완료 알림의 성공/실패 상태. 보이지 않는 ID는 추정하지 마.
- 자식의 Read 성공과 WORKFLOW-READ-COMPLETED, Workflow 반환의 completed 값.
- 기준점과 같은 correlationScope인지, lifetime.failed 증분.
- 새 요청의 request, subagent, agentRef, parentRef, role, roleRegistered, model, effort, selectionSource, success.
- 실패 요청 전부의 failureStage, failureCategory, selectionFailure, selectionIoCode, completionFailure, attempts. 값이 null이면 그대로 보고해.

기대 결과는 Workflow 내부 자식이 기준점의 메인 GPT 모델·effort로 실행되고, 실제 Read 결과가 Workflow 완료와 메인 최종 보고까지 연결되는 것이다. selectionSource의 특정 문자열을 미리 정답으로 강제하지 말고 실제 값을 보고한다. 메인 요청만 있고 자식 진단이 없으면 GPT 자식 실행은 Not verified다. 다른 모델로 성공했으면 작업 성공과 모델 전달 불일치를 분리한다.

누계는 lifetime 필드 그대로 사용하고 request 번호로 계산하지 마. 마지막 상태 수집 이후 최종 답변 요청은 해당 누계에 포함되지 않는다. Verified / Not verified / Blocked by를 구분해 보고하고 종료한다. 다중 자식·명시 모델 선택·Workflow 재개·장기 안정성은 이번 범위 밖이다.

## 중단 규칙

도구 호출은 순차 수행한다. 권한/guard 거부 시 대체 경로·옵션·스크립트를 시도하지 말고, 추가 진단 호출도 없이 현재 증거로 중단한다. 임의 도구 오류를 성공으로 무시하지 않는다. Workflow API 실패·자식 실패·모델 불일치는 같은 작업을 재호출하지 않고 허용된 종료 진단 후 종료한다. 상태 명령 자체가 실패하면 다른 경로로 인증이나 로그를 읽지 마. 인증값·전체 환경·원본 SSE·원본 세션 파일은 출력하지 않는다.
