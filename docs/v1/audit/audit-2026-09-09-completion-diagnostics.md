# 완료 복귀 재실패와 진단 수정

대상 실제 세션: 35985327-8b23-49ee-95ba-779019077224. 기준 HEAD: 8a7df25. 브랜치: fix/native-completion-resume. 이번 수정은 완료 복귀의 실패 원인이 CALL로만 표시되는 문제를 해결한다. 실제 복귀 장애의 근본 원인 해결 또는 실제 통과를 선언하지 않는다.

## 실제 결과

| UTC | 기록 | 결과 |
|---|---|---|
| 04:25:15.028 | 부모 a18b2ed4bd83eaad9 19행 | end_turn, 부모 선종료 |
| 04:26:14.301 | 자식 a161dcd6b762b5177 28행 | 45초 대기 Bash 정상 반환 |
| 04:26:16.810 | 자식 32행 | models.mjs Read 정상 반환 |
| 04:26:24.515 | 자식 35행 | end_turn |
| 04:26:24.674 | 부모 20행 | native origin.kind=task-notification, completed |
| 04:26:26.407 | 부모 22행 | isApiErrorMessage=true, AGENT_SELECTION_UNVERIFIED_CALL |

세션·수신 agent·자식 ID·toolUseId·parentAgentId는 일치했다. 양쪽 metadata에 stoppedByUser는 없고 단일 알림 헤더는 현재 정규식에 일치한다. 오류 이전 기록을 메모리에서 재구성한 검사는 완료 증거 보존·hook 경로 일치·정상 전달 시각을 가정했을 때 통과했다. 이는 실제 당시 gateway 메모리와 파일 가용 시점의 증거가 아니다.

request-status 실행은 0회다. 메인 86/87행에 SendFeedback 호출과 success=true 결과가 있다. 외부 수신·저장까지 확인한 것은 아니다. Skill은 superpowers:using-superpowers 2회, claude-api 1회 실행되어 요청한 최소 절차와 달랐다. 이를 완료 복귀의 근본 원인으로 단정하지 않는다. 해당 세션 시작 시각에 실행된 Clauduct 프로세스는 da2e050 커밋 이후 생성됐다.

## 변경

- completionFailure로 19개 고정 검증 단계를 구분한다. metadata 읽기·identity 비교, 부모/자식 완료 증거, 알림 출처·시간·헤더, 최종 응답 및 동시 변경을 구분한다.
- completionParentState/completionChildState는 UNRECORDED, REQUEST_STARTED, RECORDED, CONSUMED, NONTERMINAL, INVALID_RESPONSE 중 하나다. 마지막 관찰 상태이며 REQUEST_STARTED가 현재 요청의 실행 지속을 뜻하지 않는다.
- 세 필드가 gateway 진단·request-status 및 native API 오류 문자열에 전달된다. 진단 명령이 빠져도 API 오류 행에 남는다.
- 각 출력 경계에서 허용 목록 밖 값을 버린다. 메시지·원래 오류·경로·인증값·응답 ID를 추가로 출력하지 않는다.
- 부모 transcript를 읽는 동안 변경될 수 있는 completion.at 대신 이미 잡아 둔 parentCompletion.at을 비교한다. 마지막 동일성 검사와 1회 소비는 유지한다.

아래는 합성 예시이며 실제 세션에서 관찰된 원인 코드가 아니다.

```text
AGENT_SELECTION_UNVERIFIED_CALL completion=PARENT_COMPLETION parent=REQUEST_STARTED child=NONE
```

제한 시간 종료 시 마지막으로 관찰한 실패 단계를 표시한다. 과거 세션의 원인을 소급해 복원하지 않는다. 성공 수용 조건, 1.5초 대기, identity·취소·완료 증거 소비 정책은 완화하지 않았다.

설치 native 2.1.266의 koe → insertMessageChain → appendEntry → enqueueWrite를 추가 확인했다. transcript 쓰기는 비동기 queue이며 기본 flush 간격 100ms, remote/internal writer 연결 시 10ms다. 이 코드만으로 실제 파일이 1.5초 뒤에 나타났다고 결론낼 수 없어 대기시간을 임의로 늘리지 않았다.

## 검증과 남은 일

Verified:

- test-completion-selection.mjs 46개 통과. 거부 사례의 단계별 코드, 완료 증거 시작·중간 응답·소비·잘못된 ID, 지연·취소·동시 소비를 검사했다.
- 실제 transport와 같은 onEvent 스트리밍 및 기존 배열 응답 양쪽에서 loopback 복귀 성공과 중복 거부를 확인했다. 오류 문자열·request-status의 고정 진단값 일치, 임의 문자열 비노출, 거부 뒤 upstream 호출 수 불변을 확인했다.
- test-agent-selection.mjs 통과. 기존 모델 선택·Skill·SendMessage·peer 복귀 회귀 포함.
- test-native-gateway.mjs 21개 통과.
- git diff --check 및 수정 파일·호출자 검토.

Not verified: 새 진단 버전의 실제 완료 복귀와 상세 원인. 인증된 Claude/Codex 호출은 실행하지 않았고 기존 인증/live 실행 거부를 유지한다. symlink 검사는 기존 Node 권한 거부로 남아 있으며 재시도하지 않았다. 관련 없는 전체 모델·compact 테스트는 반복하지 않았다.

다음 실제 검사는 새 gateway에서 부모 1개/자식 1개 검증 한 번이다. 성공이면 verified-completion-resume을 확인한다. 실패하면 API 오류의 completion/parent/child로 해당 조건을 좁힌다. 새 세션 ID만 제공돼도 원본 오류 행에서 세 값을 확인할 수 있다. 구버전 세션에서 CALL만 반복 수집하는 것은 도움이 되지 않는다.
