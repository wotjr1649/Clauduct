# 같은 Workflow의 명시 선택·기본 상속 자식 격리 시험

이 문서를 먼저 읽고 실행한다. 사용자는 새 Clauduct 프로세스를 이미 실행했다. 작업 위치는 D:/AIDEV/Clauduct/verification/dev-sandbox/run-01이다. native Workflow 1회에서 읽기 전용 자식 A 명시 선택, A 성공 시 자식 B 기본 상속을 순차 실행하는 시험을 요청한다. 최대 자식 2개이며 병렬 시험이 아니다.

Verified: 기본 상속은 08594a7e, 명시 luna/high는 660a5d7d에서 Read·정상 result·메인 복귀까지 성공했다. 같은 run의 혼합 자식은 로컬 검사만 통과했다. Not verified: 실제 두 자식의 관계·모델·결과가 섞이지 않는지. 이번에는 이 차이만 추가한다.

## 1. 기준점

native Workflow 존재와 Skill workflow-authoring의 inline script 및 agent(prompt, opts) 계약을 확인한다. 없거나 참조 로드 실패면 중단한다. 일반 Agent·다른 개발 workflow 스킬로 대체하지 않는다.

필요하면 D:/AIDEV/Clauduct/src/request-status.mjs를 읽고 메인의 허용된 셸에서 아래 명령을 한 번 실행한다. timeout은 60초 이하로 지정한다.

```text
node D:/AIDEV/Clauduct/src/request-status.mjs
```

필터링된 JSON 전체와 현재 메인 model/effort, sessionRef, correlationScope, lifetime.started/succeeded/failed를 기록한다. 현재 메인 값을 식별하지 못하면 중단한다. 자기소개·UI 배너로 대체하지 않는다.

A 목표는 메인이 gpt-5.6-luna가 아니면 gpt-5.6-luna/high, 메인이 gpt-5.6-luna이면 gpt-5.6-terra/xhigh다. B 목표는 현재 메인의 model/effort 그대로다. 두 목표를 먼저 보고하고 메인 선택을 바꾸지 않는다.

## 2. Workflow 1회, 자식 최대 2개

Workflow에 inline script만 전달한다. name/scriptPath/resumeFromRunId 입력을 함께 지정하거나 script 파일을 저장하지 않는다.

첫 문장은 순수 리터럴 export const meta로 name='clauduct-workflow-mixed-probe', description='Verify explicit and inherited children in one local run'을 선언한다.

script는 다음 순서로 작성한다.
1. await agent로 A를 한 번 호출한다. opts에는 label='A-explicit', A의 전체 GPT model ID와 effort만 넣는다.
2. A 결과가 문자열이며 네 키와 마지막 줄 WORKFLOW-A-EXPLICIT-COMPLETED를 포함하는지 검사한다. 실패·null·누락이면 B를 호출하지 않고 { completed:false, A:실제문자열또는null, B:null }을 반환한다. 권한/guard 거부가 보고돼도 B를 시작하지 않는다.
3. A가 성공했을 때만 await agent로 B를 한 번 호출한다. opts에는 label='B-inherit'만 넣고 model/effort를 모두 생략한다. model='inherit' 문자열이나 custom agentType을 쓰지 않는다.
4. B도 네 키와 마지막 줄 WORKFLOW-B-INHERIT-COMPLETED를 확인한다. 두 결과 모두 자기 표식만 포함하고 상대 표식은 없을 때만 completed=true다. { completed:boolean, A:실제문자열또는null, B:실제문자열또는null }를 반환한다. 결과를 수정·보충하지 않는다.

두 자식의 작업은 같다. 각각에게 자기 완료 표식만 전달한다. A의 결과를 B prompt에 넣지 않는다.

자식 prompt는 다음 문장을 사용하되 <자기 완료 표식>만 A 또는 B의 위 표식으로 바꾼다:
“Read 도구로 D:/AIDEV/Clauduct/src/models.mjs를 정확히 한 번 읽어. MODELS 객체의 네 키를 첫 줄에 쉼표로 구분해 모두 출력해. 현재 네 실행 모델 하나만 답하지 마. 두 번째이자 마지막 줄에는 <자기 완료 표식>을 출력해. 읽기에 성공하고 네 키를 모두 확인한 경우에만 완료 문구를 출력해. 코드의 모델 정의를 네 실제 모델의 증거라고 주장하지 마. 파일 수정, Bash, Skill, Agent, Workflow, 외부 조회는 하지 마. 거부·실패 시 우회·재시도하지 말고 실패만 보고해.”

재호출·반복문·Promise.all·parallel·pipeline·추가 자식·중첩·resume은 사용하지 않는다. native 완료 알림을 기다리고 TaskOutput·반복 polling은 하지 않는다. 알림 전에는 대기 상태만 보고하며 성공을 선언하지 않는다.

## 3. 종료 진단과 판정

성공 또는 terminal 실패를 확인하면 메인에서 같은 상태 명령을 한 번 더 실행하고 JSON 전체를 출력한다. 아래를 관찰값으로 보고한다.

- Workflow 호출 ID, task/run ID, completed와 A/B 실제 결과. 없는 값은 추정하지 않는다.
- 자식 agentRef별 request, requestedModel, model, effort, role, roleRegistered, selectionSource, success.
- A는 명시 목표값, B는 기준점 메인 값인지. 서로 다른 모델을 기준으로 A/B를 연결하고 요청 번호 순서만으로 추정하지 않는다.
- 두 agentRef가 구별되며 각 결과의 표식이 올바른지. 다른 자식 결과를 대신 사용하지 않는다.
- 메인 model/effort 유지, sessionRef/correlationScope 일치, lifetime.failed 증분.
- 실패 요청 전부의 failureStage, failureCategory, selectionFailure, selectionIoCode, completionFailure, attempts. null도 보존한다.

통과하려면 두 자식 모두 workflow-subagent·roleRegistered=true·workflow-result·success=true이며 각 모델·effort가 목표와 일치해야 한다. A/B의 서로 다른 결과가 completed=true로 메인까지 복귀하고 메인 설정도 유지돼야 한다. 직접 Read 기록이 보이지 않으면 반환 내용과 직접 관찰 증거를 구분한다. 마지막 상태 이후 요청은 누계 밖이다.

## 중단·보존

파일 생성·수정·삭제, 테스트, 설치, 웹 조회, worktree·브랜치 작업, commit/push, 설정·권한·hook 변경은 금지한다. native의 통상적인 로컬 세션 기록 외에 파일을 저장하지 않는다. 인증값·전체 환경·원본 SSE·원본 세션 파일을 출력하지 않는다.

권한/guard 거부 또는 사용자 중단이면 새 작업과 추가 진단 없이 중단한다. 다른 도구·셸·옵션으로 우회하지 않는다. 일반 API/자식 실패·모델 불일치는 허용된 종료 진단 후 종료하고 재시도하지 않는다. 상태 명령 자체가 실패하면 대체 인증·로그 경로를 시도하지 않는다.

최종 답변은 Verified / Not verified / Blocked by로 구분한다. 실패 누계 0을 업무 성공으로 대신하지 않는다. 병렬·중첩·재개·custom agentType·장기 안정성은 이번 범위 밖이다.
