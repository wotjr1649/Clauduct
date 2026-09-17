# Agent 직접 부모 기준 손자 inherit 검증

이 문서를 먼저 읽고 실행한다. 사용자는 --gpt-agents를 포함한 새 Clauduct 프로세스를 이미 실행했다. 작업 위치는 D:/AIDEV/Clauduct/verification/dev-sandbox/run-01이다. 기존 등록 Agent로 부모 1개와 그 부모가 생성하는 손자 1개, 총 2개만 읽기 전용 시험에 사용하도록 요청한다.

Verified: Workflow 병렬 A/B 시험은 992c0794에서 성공했다. 직접 부모의 실제 route로 손자 snapshot을 만드는 gateway 통합 검사는 통과했다. Not verified: 실제 native Agent 다단계 생성·직접 부모 상속·결과 복귀. 이번에는 Workflow가 아닌 Agent 경로를 확인한다.

## 1. 기준점과 부모 선택

필요하면 D:/AIDEV/Clauduct/src/request-status.mjs를 읽고 메인의 허용된 셸에서 다음 명령을 한 번 실행한다. timeout은 60초 이하로 지정한다.

```text
node D:/AIDEV/Clauduct/src/request-status.mjs
```

필터링된 JSON 전체를 출력하고 현재 메인 model/effort, sessionRef, correlationScope, lifetime.started/succeeded/failed를 기록한다. 현재 메인을 식별할 수 없으면 중단한다. 메인의 모델·effort를 바꾸지 않는다.

부모 선택:
- 메인이 gpt-5.6-sol이 아니면 subagent_type='clauduct-sol', 기대값 gpt-5.6-sol/xhigh.
- 메인이 gpt-5.6-sol이면 subagent_type='clauduct-terra', 기대값 gpt-5.6-terra/high.

선택한 부모와 clauduct-inherit가 native Agent에 등록됐는지 현재 도구 설명에서 확인한다. 없으면 중단하고, 임의 등록·설정 변경·다른 역할 대체는 하지 않는다. 이 기대값은 정의 기준이며 실제 실행 값은 종료 진단으로 판단한다.

## 2. 부모 Agent 한 번 호출

메인은 위 부모 유형을 Agent의 subagent_type으로 선택하고 model 인수는 생략한다. description은 'Direct-parent inherit probe'로 한다. 현재 schema가 지원하는 foreground 호출을 사용하며 background, resume, team, worktree 관련 옵션은 지정하지 않는다. 호출 형식을 확인할 수 없으면 추정하지 않고 중단한다.

부모 prompt에는 아래 작업 전체를 전달한다.

“사용자는 네가 직접 부모가 되어 clauduct-inherit 손자 1개를 생성하는 읽기 전용 시험을 요청했다. 네 허용 작업은 Agent 1회와 그 결과 보고뿐이다. Agent 도구와 clauduct-inherit가 현재 문맥에 없거나 중첩이 거부되면 우회하지 말고 실패를 반환해. 네가 models.mjs를 대신 읽지 마.

Agent(subagent_type='clauduct-inherit')를 정확히 한 번 호출하고 model 인수는 생략해. description은 'Inherited grandchild read probe'로 하며 foreground 호출을 사용해. background/resume/team/isolation은 지정하지 마.

손자에게 전달할 prompt:
Read 도구로 D:/AIDEV/Clauduct/src/models.mjs를 정확히 한 번 읽어. MODELS의 네 키를 첫 줄에 쉼표로 구분해 모두 출력하고 두 번째이자 마지막 줄에 DIRECT-PARENT-GRANDCHILD-COMPLETED를 출력해. 읽기에 성공하고 네 키를 확인했을 때만 표식을 출력해. 코드의 정의를 실제 실행 모델의 증거로 주장하지 마. 파일 수정, Bash, Skill, Agent, Workflow, 외부 조회는 하지 마. 거부·실패 시 재시도하지 말고 실패만 보고해.

손자 결과가 네 키와 완료 문구를 포함하면 그 원문을 수정 없이 보고하고 마지막 줄에 DIRECT-PARENT-RETURN-COMPLETED를 출력해. 결과가 비었거나 실패하면 표식을 만들지 말고 실패를 보고해. 추가 Agent, TaskOutput, SendMessage, Read, Bash, Skill, Workflow는 실행하지 마. 권한/guard 거부 시 즉시 중단해. 네 실제 모델·effort를 자기소개로 단정하지 마.”

부모가 손자 결과를 반환하기 전 성공을 선언하지 않는다. native가 예상과 달리 background 반환을 주면 기존 완료 알림만 기다리고 재호출·polling·TaskOutput·resume으로 대체하지 않는다.

## 3. 종료 진단과 관계 판정

부모의 성공 또는 terminal 실패를 확인하면 메인이 동일 상태 명령을 한 번 더 실행해 JSON 전체를 출력한다. 이 명령이 마지막 도구 호출이다. 이후에는 추가 검증 도구 없이 최종 보고한다.

관찰 항목:
- 부모·손자의 실제 반환과 완료 표식. 누락 내용을 대신 작성하지 않는다.
- 현재 세션의 subagent 요청을 agentRef별로 묶어 request, parentRef, requestedModel, model, effort, selectionSource, roleRegistered, success를 보고한다.
- 부모는 definition-model이며 선택한 고정 모델/effort와 일치해야 한다.
- 손자는 definition-inherit이며 손자 parentRef가 부모 agentRef와 같아야 한다. model/effort가 실제 부모와 같고 최상위 메인 모델과는 달라야 한다.
- 메인 model/effort 유지, sessionRef/correlationScope 일치, lifetime.failed 증분.
- 실패 요청의 failureStage, failureCategory, selectionFailure, selectionIoCode, completionFailure, attempts. null도 그대로 표시한다.

요청 번호 순서나 모델 자기소개로 부모 관계를 추정하지 않는다. parentRef가 없거나 다른 경우 관계는 Not verified다. 부모·손자의 이름이 status.role에 없더라도 임의로 채우지 말고 selectionSource·agentRef·parentRef로 판정한다. 직접 Read 기록이 보이지 않으면 반환 결과와 직접 관찰 증거를 구분한다.

이 시험은 부모 정의의 기본 effort를 사용한다. 성공해도 비기본 effort의 실제 다단계 상속까지 입증했다고 말하지 않는다. 기존 비기본 snapshot 로컬 검사와 구분한다. 마지막 상태 이후 요청은 누계 밖이다.

## 절차와 중단 규칙

이번 시험에는 Skill 호출이 필요 없다. verification-before-completion을 포함한 모든 Skill과 Workflow 호출은 하지 않는다. 상위 지침과 충돌해 범위를 지킬 수 없으면 생성 전에 충돌을 보고하고 중단하며 지침·hook을 변경하지 않는다.

파일 생성·수정·삭제, 테스트, 설치, 웹 조회, 브랜치·worktree, commit/push, 설정·권한 변경을 금지한다. native의 통상적인 로컬 세션 기록 외에 파일을 저장하지 않는다. 인증값·전체 환경·원본 SSE·원본 세션 파일을 출력하지 않는다.

권한/guard 거부나 사용자 중단이면 추가 진단 없이 중단한다. 다른 도구·셸·옵션·역할로 우회하지 않는다. 일반 API/자식 실패·빈 결과·모델 불일치는 허용된 종료 진단 후 끝내고 재시도하지 않는다. 상태 명령 실패 시 다른 인증·로그 경로를 시도하지 않는다.

최종 답변은 기능 / 직접 부모 상속 / 절차 준수를 나누고 Verified / Not verified / Blocked by로 표시한다. 실패 누계 0을 업무 성공으로 대신하지 않는다. 병렬 손자·재개·추가 깊이·장기 안정성은 범위 밖이다.
