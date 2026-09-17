# 비기본 effort의 native 다단계 inherit 시험

이 문서를 먼저 읽고 실행한다. 사용자는 --gpt-agents를 포함한 새 Clauduct 프로세스를 이미 실행했다. 작업 위치는 D:/AIDEV/Clauduct/verification/dev-sandbox/run-01이다. 기존 clauduct-inherit 부모 1개와 그 부모가 생성하는 clauduct-inherit 손자 1개, 총 Agent 2개만 읽기 전용 시험에 사용한다.

Verified: session-10(db34be24)의 메인 sol/high → 부모 terra/high → 손자 terra/high와 완료 알림 후 부모 복귀는 성공했다. Not verified: 비기본 effort의 실제 다단계 전달. 이번에는 현재 메인의 값을 관찰하며 특정 모델/effort로 단정하지 않는다. 과거 시험을 재실행하지 않는다.

## 1. 기준점

Read로 D:/AIDEV/Clauduct/src/models.mjs를 한 번 읽어 모델별 기본 effort를 확인한다. 필요하면 D:/AIDEV/Clauduct/src/request-status.mjs를 읽고 허용된 메인 셸에서 다음을 한 번 실행한다. timeout은 60초 이하로 지정한다.

```text
node D:/AIDEV/Clauduct/src/request-status.mjs
```

필터링된 JSON 전체를 출력하고 현재 메인 model/effort, sessionRef, correlationScope, lifetime.started/succeeded/failed를 기록한다. 메인 route를 식별할 수 없거나 그 effort가 models.mjs의 해당 모델 기본값과 같으면 Agent 생성 전에 중단한다. 이 경우 사용자가 비기본 effort를 선택해야 한다고 보고하며 스스로 모델·effort·설정을 바꾸지 않는다. 예컨대 sol/high는 현재 sol 기본 xhigh와 다르지만 이 예를 관찰값으로 대신하지 않는다.

현재 Agent 도구 설명에서 clauduct-inherit 등록과 foreground 호출 형식을 확인한다. 없거나 불명확하면 추정·임의 등록·역할 대체 없이 중단한다.

## 2. 부모와 손자

메인은 Agent(subagent_type='clauduct-inherit')를 정확히 한 번 호출한다. model 인수는 생략하고 description은 'Nondefault inherited parent probe'로 한다. 지원된 foreground 호출을 사용하고 background/resume/team/isolation 옵션은 지정하지 않는다. 부모 prompt에 다음 내용을 전달한다.

“사용자는 네가 clauduct-inherit 손자 하나를 만드는 읽기 전용 상속 시험을 요청했다. 허용 작업은 Agent 1회와 결과 보고뿐이다. Agent와 clauduct-inherit가 없거나 중첩이 거부되면 실패를 반환해. 파일을 네가 대신 읽지 마.

Agent(subagent_type='clauduct-inherit')를 정확히 한 번 호출해. model은 생략하고 description은 'Nondefault inherited grandchild probe'로 해. 지원된 foreground 호출을 사용하고 background/resume/team/isolation은 지정하지 마. 손자 prompt는 다음과 같아.

Read로 D:/AIDEV/Clauduct/src/models.mjs를 정확히 한 번 읽어. MODELS의 네 키를 첫 줄에 쉼표로 구분해 출력하고 두 번째이자 마지막 줄에 NONDEFAULT-GRANDCHILD-COMPLETED를 출력해. 읽기 성공과 네 키 확인 전에는 표식을 만들지 마. 코드 정의나 자기소개를 실제 실행 모델의 증거로 주장하지 마. 파일 변경, Bash, Skill, Agent, Workflow, 외부 조회는 하지 마. 거부·실패 시 재시도하지 말고 실패만 보고해.

손자가 네 키와 완료 표식을 반환하면 그 원문을 수정 없이 보고하고 마지막 줄에 NONDEFAULT-PARENT-COMPLETED를 출력해. 빈 결과·실패이면 표식을 만들지 마. 추가 Agent, TaskOutput, SendMessage, Read, Bash, Skill, Workflow는 금지야. 권한/guard 거부나 사용자 중단이면 즉시 멈춰. native가 background 결과를 반환하면 기존 완료 알림만 기다리고 재호출·polling·수동 resume으로 대체하지 마.”

메인도 부모의 실제 결과가 오기 전에 성공을 선언하지 않는다. native 자동 background 전환 시 기존 완료 알림만 사용한다. 활성 작업 근거 없이 대기하거나 재호출하지 않는다.

## 3. 종료 진단

부모 성공 또는 terminal 실패 후 메인은 위 상태 명령을 한 번 더 실행하고 JSON 전체를 출력한다. 이것이 마지막 도구 호출이다. 이후 추가 검증 도구 없이 다음을 보고한다.

- 부모·손자 반환 원문과 두 완료 표식. 누락 내용을 대신 작성하지 않는다.
- 현재 세션 subagent 요청의 request, agentRef, parentRef, requestedModel, model, effort, selectionSource, roleRegistered, success.
- 부모·손자의 초기 선택은 definition-inherit여야 한다. 둘 모두 기준점의 실제 메인 model/effort와 일치하고 그 effort는 해당 모델 기본값과 달라야 한다. 부모가 native 완료 알림으로 복귀했다면 verified-completion-resume 요청도 같은 route인지 구분해 기록한다.
- 손자 parentRef는 부모 agentRef와 같아야 한다. 없거나 다르면 관계는 Not verified다. 요청 순서·자기소개로 추정하지 않고 status.role에 없는 custom 이름을 채우지 않는다.
- 메인 route 유지, sessionRef/correlationScope 일치, lifetime.failed 증분. 실패 요청의 failureStage, failureCategory, selectionFailure, selectionIoCode, completionFailure, attempts는 null도 그대로 표시한다.

이 시험에서는 메인과 부모 모델이 같으므로 결과만으로 직접 부모 대 최상위 구분을 입증하지 않는다. session-10의 서로 다른 모델 관계 증거와 결합해 평가한다. 직접 Read 기록이 보이지 않으면 반환 결과와 직접 관찰을 구분한다. 마지막 상태 이후 요청은 누계 밖이다.

## 중단과 완료

Skill(verification-before-completion 포함), Workflow, 파일 생성·수정·삭제, 테스트, 설치, 웹 조회, 브랜치·worktree, commit/push, 설정·권한 변경을 금지한다. 상위 지침과 충돌해 이 범위를 지킬 수 없으면 Agent 생성 전에 보고하고 중단하며 hook·지침을 변경하지 않는다. 통상적인 native 세션 기록 외에 파일을 저장하지 않는다. 인증값·전체 환경·원본 SSE·원본 세션 파일은 출력하지 않는다.

권한/guard 거부나 사용자 중단은 추가 진단 없이 중단한다. 다른 도구·셸·옵션·역할로 우회하지 않는다. 일반 API/자식 실패·빈 결과·모델 불일치는 허용된 종료 진단 후 끝내고 재시도하지 않는다. 상태 명령 실패 시 다른 인증·로그 경로를 시도하지 않는다.

최종 답변은 기능 / 비기본 effort 상속 / 관계 / 절차 준수를 Verified / Not verified / Blocked by로 구분한다. 완료 조건은 두 Agent의 실제 반환, 두 단계 비기본 route 유지, parentRef 일치, 종료 진단과 절차 준수다. 실패 누계 0만으로 성공을 선언하지 않는다. 병렬 손자·추가 깊이·수동 재개·장기 안정성 및 전체 목표 완료는 범위 밖이다.
