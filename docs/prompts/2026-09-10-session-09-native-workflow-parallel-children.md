# 동일 Workflow 병렬 자식 2개 검증

이 문서를 먼저 읽고 시험하라. 사용자는 새 Clauduct 프로세스를 이미 실행했다. 작업 위치는 D:/AIDEV/Clauduct/verification/dev-sandbox/run-01이다. native Workflow 1회에서 읽기 전용 자식 A 명시 GPT 선택과 B 기본 상속을 병렬 실행하도록 요청한다. 최대 자식 2개다.

Verified: 5cd82157에서 순차 A luna/high, B sol/high의 Read·결과·메인 복귀가 성공했다. 로컬 동시 진입·결과 격리 검사도 통과했다. Not verified: 실제 native 병렬 실행과 최소 종료 절차 준수. 이번에는 이 차이만 추가한다.

## 1. 참조와 기준점

Skill 호출은 이번 시험 전체에서 workflow-authoring 한 번만 허용한다. 그 참조에서 native Workflow, agent(prompt, opts), parallel(thunks)의 계약을 확인한다. 다른 Skill은 호출하지 않는다. 특히 verification-before-completion, executing-plans, subagent-driven-development, using-git-worktrees는 이 시험의 단계가 아니다. 상위 지침과 충돌하여 이 범위를 지킬 수 없다면 실행 전에 충돌만 보고하고 중단한다. 전역 skill/plugin/hook을 변경하거나 비활성화하지 않는다.

필요하면 D:/AIDEV/Clauduct/src/request-status.mjs를 읽고 메인의 허용된 셸에서 다음 명령을 한 번 실행한다. timeout은 60초 이하로 지정한다.

```text
node D:/AIDEV/Clauduct/src/request-status.mjs
```

필터링된 JSON 전체를 출력하고 현재 메인 model/effort, sessionRef, correlationScope, lifetime.started/succeeded/failed를 기록한다. 현재 메인을 식별할 수 없으면 중단한다. 메인 설정은 변경하지 않는다.

A 목표는 메인이 gpt-5.6-luna가 아니면 gpt-5.6-luna/high, 메인이 gpt-5.6-luna이면 gpt-5.6-terra/xhigh다. B 목표는 현재 메인 model/effort다. 두 목표를 보고하고 실행한다.

## 2. 정확히 두 작업을 병렬 제출

Workflow에는 inline script만 전달한다. name/scriptPath/resumeFromRunId 입력이나 script 파일 저장은 사용하지 않는다.

첫 문장은 순수 리터럴 export const meta로 name='clauduct-workflow-parallel-probe', description='Verify two concurrent read-only GPT children'을 선언한다.

await parallel([A를 시작하는 함수, B를 시작하는 함수])를 한 번 호출한다. 배열에는 Promise가 아니라 두 함수만 넣고, 각 함수는 agent를 정확히 한 번 호출하여 그 Promise를 반환한다. A를 먼저 await한 뒤 B를 시작하는 순차 실행은 하지 않는다.

- A opts: label='A-parallel-explicit', 목표 전체 GPT model ID와 effort.
- B opts: label='B-parallel-inherit'만 지정하고 model/effort 생략.
- 두 자식 모두 agentType/schema/isolation 생략. model='inherit' 문자열은 쓰지 않는다.
- 결과 배열의 첫 슬롯은 A, 두 번째는 B다. null을 filter로 제거하거나 결과 순서를 바꾸지 않는다. 실패 결과를 성공으로 바꾸지 않는다.

두 자식은 각각 Read로 D:/AIDEV/Clauduct/src/models.mjs를 정확히 한 번 읽는다. 각 자식 prompt에 다음 작업과 자기 표식만 전달하며 상대 결과를 전달하지 않는다.

“Read 도구로 D:/AIDEV/Clauduct/src/models.mjs를 한 번 읽어. MODELS 객체의 네 키를 첫 줄에 쉼표로 구분해 모두 출력해. 두 번째이자 마지막 줄에 <자기 표식>을 출력해. 읽기에 성공하고 네 키를 확인했을 때만 표식을 출력해. 현재 실행 모델 하나만 답하거나 코드 정의를 실제 실행 모델의 증거로 주장하지 마. 파일 수정, Bash, Skill, Agent, Workflow, 외부 조회는 하지 마. 실패·거부 시 우회·재시도하지 말고 실패만 보고해.”

A 자기 표식은 WORKFLOW-PARALLEL-A-COMPLETED, B 자기 표식은 WORKFLOW-PARALLEL-B-COMPLETED다.

Workflow 반환은 { completed:boolean, A:string|null, B:string|null }다. 두 실제 결과에 네 키와 자기 표식이 모두 있고 상대 표식이 없을 때만 completed=true다. 원문을 보충·변경하지 않는다.

parallel은 이미 시작한 두 작업의 결과를 기다리는 장벽이다. 한쪽 실패 시 새 자식·재시도·대체 작업을 만들지 않는다. 사용자 중단이나 권한/guard 거부 시 새 작업과 추가 진단을 하지 않고 native의 기존 취소 처리를 존중한다. 임의 취소 API를 만들지 않는다.

완료 알림을 기다리고 TaskOutput·반복 polling·추가 Workflow·반복문·pipeline·sleep·중첩·resume을 사용하지 않는다. 완료 전에는 성공을 선언하지 않는다.

## 3. 종료 진단은 한 번, 그 뒤 바로 보고

두 작업의 완료 또는 terminal 실패가 확인되면 같은 request-status 명령을 한 번 더 실행해 JSON 전체를 출력한다. 이 명령이 마지막 도구 호출이다. 이후 Skill·Read·Bash·추가 검증 호출 없이 확보한 증거로 최종 답변한다.

보고할 항목:
- Workflow 호출 ID·task/run ID·completed·A/B 실제 결과.
- 자식 agentRef별 request, requestedModel, model, effort, role, roleRegistered, selectionSource, success.
- 메인 model/effort 유지, sessionRef/correlationScope 일치, lifetime.failed 증분.
- 실패 요청 전부의 failureStage, failureCategory, selectionFailure, selectionIoCode, completionFailure, attempts. null도 보존한다.
- 서로 다른 자식 요청의 startedAt, admissionStartedMs, transportStartedMs, transportFinishedMs, finishedMs. 기존 상태 JSON에서만 읽고 시간을 새 도구로 측정하지 않는다.

기능 통과는 A/B가 각각 목표 모델·effort로 성공하고 서로 다른 agentRef·자기 결과가 보존되며 completed=true로 메인에 복귀하는 것이다. 두 자식 모두 workflow-subagent, roleRegistered=true, workflow-result여야 한다.

병렬성은 별도 판정한다. parallel 호출만으로 실제 겹침을 단정하지 않는다. 서로 다른 자식의 gateway 요청 시간 구간이 겹치는 근거가 있어야 한다. startedAt과 상대 시간 필드로 근사 비교하되 경계 수준 차이나 필드 누락이면 Not verified다. gateway 요청 겹침은 backend 내부 연산의 동시성 증거가 아니다. 겹침이 관찰되지 않아도 지연·반복 실행으로 강제하지 않는다.

직접 Read 기록이 보이지 않으면 결과 문자열과 직접 관찰 증거를 구분한다. 마지막 상태 이후 요청은 누계 밖이다. 최종 답변은 기능 / 병렬성 / 절차 준수를 나누고 각각 Verified / Not verified / Blocked by를 표시한다.

## 보존·중단

파일 생성·수정·삭제, 테스트, 설치, 웹 조회, worktree·브랜치, commit/push, 설정·권한·hook 변경은 금지한다. native의 통상적인 로컬 세션 기록 외에 파일을 저장하지 않는다. 인증값·전체 환경·원본 SSE·원본 세션 파일은 출력하지 않는다.

권한/guard 거부 또는 사용자 중단이면 추가 진단도 하지 않는다. 다른 도구·셸·옵션으로 우회하지 않는다. 일반 API/자식 실패·빈 결과·모델 불일치는 허용된 종료 진단 후 끝내고 재시도하지 않는다. 상태 명령 실패 시 다른 인증·로그 경로를 시도하지 않는다. 실패 누계 0을 업무 성공으로 대신하지 않는다. 중첩·resume·custom agentType·장기 안정성은 범위 밖이다.
