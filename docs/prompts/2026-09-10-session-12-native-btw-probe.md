# Native /btw 읽기 전용 시험

작업 위치는 D:/AIDEV/Clauduct/verification/dev-sandbox/run-01이다. 사용자는 새 Clauduct 프로세스를 이미 실행했다. 이 시험은 메인이 다른 작업을 실행하지 않는 상태에서 native /btw를 사용자가 직접 입력해 확인한다. --gpt-agents는 /btw의 필수 조건이 아니며 기존 실행 설정을 바꾸지 않는다.

Verified: 설치 2.1.266에서 /btw의 side_question → 문맥 복제 → 공통 모델 요청 경로를 정적 확인했다. 메인 문맥의 agentContext를 유지하고 skipTranscript=true를 사용한다. Not verified: 실제 모델·effort·반환·beta 호환성. 이번 시험은 일반 Agent/Workflow 상속 시험이 아니다.

## 1. 최초 로드: 준비만 수행

이 문서를 처음 읽은 메인은 /btw를 직접 실행하거나 질문에 대신 답하지 않는다. 현재 작업·자식이 실행 중이면 중단하고 이번 시험을 시작하지 않는다. 상태 조회가 필요하면 D:/AIDEV/Clauduct/src/request-status.mjs를 Read로 읽는다. 허용된 셸에서 아래 명령을 timeout 60초 이하로 정확히 한 번 실행한다.

```text
node D:/AIDEV/Clauduct/src/request-status.mjs
```

필터링된 JSON 전체를 출력하고 현재 메인 model/effort, sessionRef, correlationScope, 최신 request/startedAt, lifetime.started/succeeded/failed를 기록한다. 메인 route나 세션을 식별할 수 없으면 실패를 보고하고 중단한다. 특정 모델·effort를 가정하거나 변경하지 않는다.

준비 답변에는 다음 문맥 식별자와 READY-FOR-BTW를 출력한다.

```text
문맥 식별자: BTWS12-CONTEXT
READY-FOR-BTW
```

그리고 사용자에게 2번 명령을 직접 입력하도록 안내하고 이번 응답을 끝낸다. 이후 자동 대기·polling·추가 상태 조회를 하지 않는다.

## 2. 사용자 수동 입력

사용자가 native 입력창에 아래 한 줄을 직접 입력한다. 메인은 이를 Bash, Skill, Agent, Workflow나 다른 호출로 대신하지 않는다.

```text
/btw 앞서 준비 답변에 제시한 문맥 식별자와 7 곱하기 8의 결과를 사용해 한 줄로 답해. 형식은 <문맥 식별자> | <계산 결과> | BTW-SIDE-COMPLETED 이다. 도구를 사용하거나 파일을 읽지 말고 현재 문맥으로만 답해. 식별자를 확인할 수 없으면 완료 표식 없이 확인 불가라고 답해.
```

기대 답변은 BTWS12-CONTEXT | 56 | BTW-SIDE-COMPLETED다. 사용자는 실제 UI 답변을 보관하고, 아래 형식으로 메인에 새 메시지를 보낸다. 답변이 없거나 실패했으면 그대로 실패 내용을 적으며 기대 답변을 대신 복사하지 않는다.

```text
session-12 종료 진단을 수행하라.
/btw UI 실제 결과: <내가 실제로 본 답변 또는 오류>
```

UI 닫기 조작은 현재 native 화면 안내를 따른다. /btw가 없거나 거부되면 다른 명령으로 대체하지 않는다. 권한/guard 거부 또는 사용자 중단이면 3번 진단도 실행하지 않고 그 사실만 보고한다.

## 3. 종료 메시지를 받은 메인

사용자가 명시적으로 위 종료 진단을 요청한 경우에만 1번 상태 명령을 한 번 더 실행해 JSON 전체를 출력한다. 이것이 마지막 도구 호출이다. 첫 준비 요청 때 이 단계를 선행 실행하지 않는다.

최종 답변은 다음을 구분한다.

- 사용자 제공 UI 결과가 기대 문자열과 일치하는지. 직접 관찰이 아니라 사용자 제공 증거임을 표시한다.
- 기준점 이후 관찰한 request, startedAt, sessionRef, agentRef, parentRef, model, effort, selectionSource, success. 메인의 기준 model/effort와 비교한다.
- correlationScope/sessionRef 유지, lifetime 증분과 실패. 실패가 있으면 failureStage, failureCategory, selectionFailure, completionFailure, attempts를 null도 포함해 보고한다.
- /btw는 skipTranscript를 사용하므로 별도 자식 JSONL이 없다는 이유로 실패라고 하지 않는다. status에 querySource가 없으므로 요청 번호만으로 /btw를 특정하지 않는다. 귀속을 확인할 수 없는 요청은 Not verified로 남긴다.

준비 답변과 종료 진단 요청 자체도 모델 요청이므로 증가분을 모두 /btw 호출 수로 세지 않는다. 마지막 상태 이후 최종 답변은 누계 밖이다. 성공 누계만으로 UI 반환 성공을 대신하지 않는다.

## 범위와 중단

메인은 준비/종료 상태 명령 각 1회, 필요한 request-status.mjs Read와 보고만 수행한다. /btw는 사용자가 1회 입력한다. 모델/effort/설정/권한 변경, 파일 생성·수정·삭제, 설치, 테스트, commit/push, 외부 조회를 하지 않는다. native 통상 세션 기록 외의 파일은 저장하지 않는다. 인증값·전체 환경·원본 세션·SSE·reasoning은 출력하지 않는다.

Skill(verification-before-completion 포함), Agent, Workflow, TaskOutput, SendMessage, background 작업, 재시도·수동 resume은 금지한다. 상위 지침과 충돌하면 준비 전에 보고하고 중단하며 지침·hook을 변경하지 않는다. 상태 명령 실패 시 다른 로그·인증 경로로 우회하지 않는다. 일반 API 실패는 허용된 종료 진단 후 멈춘다.

완료 보고는 기능 / 모델·effort / 요청 귀속 / 절차를 Verified / Not verified / Blocked by로 나눈다. 메인과의 동시 실행·취소·remote control·fallback 및 전체 제품 완료는 이번 범위가 아니다. 사용자는 종료 후 세션 ID와 /btw UI 실제 답변을 조사 담당자에게 제공한다.
