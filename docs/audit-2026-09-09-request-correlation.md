# 요청 관계와 실패 단계 누적 진단

무명시 Plan 및 누적 진단의 실제 성공 기록 37237d2 이후 후속 구현이다. 모델 라우팅이나 native 권한 정책은 변경하지 않았다.

## 추가 필드

gateway마다 correlationScope와 별도 무작위 HMAC 키를 생성한다. 각 요청의 검증된 형식의 세션/agent/부모 header로 sessionRef, agentRef, parentRef를 계산한다. JSON tuple에 종류·세션·agent를 구분하여 넣고 HMAC-SHA256 결과의 앞 128bit를 쓴다. 인증 토큰과 HMAC 키는 별개이며 키는 메모리에만 두고 종료 시 지운다. 원래 ID는 진단에 저장하지 않는다. 참조값을 위한 누적 Map도 만들지 않는다.

같은 correlationScope에서 자식 parentRef와 부모 agentRef가 같으면 요청들의 header 관계를 연결할 수 있다. 같은 agent 문자열도 세션이 다르면 다르며 gateway가 재시작되면 scope와 참조값 모두 바뀐다. 세션 header가 없으면 모든 참조값이 null이다. agent/부모 header가 없으면 해당 참조값은 null이다.

이 값은 상관관계 표시이며 권한·신뢰 증명이 아니다. 실패한 요청의 header도 표시될 수 있으므로 성공·selectionSource·기존 metadata 검증 결과와 함께 해석해야 한다. 임의 header로 라우팅 검증을 통과시켜 주지 않는다. 과거 ID를 참조값에서 복원하거나 서로 다른 gateway 기록을 동일 값으로 연결하지 않는다.

lifetime.failuresByStage는 request, selection, prepare, review, upstream, output-validation, delivery의 일곱 고정 카운터다. 각 계측 요청이 실패로 종료할 때 한 번 증가한다. 합계는 lifetime.failed와 같다. 인증·헤더 단계에서 계측 전에 거부된 요청 및 관리 endpoint는 기존 lifetime 범위와 같이 제외한다.

request-status는 32자리 소문자 hex 참조값과 비음수 안전 정수 카운터만 출력한다. 이전 버전에 새 필드가 없으면 null을 반환한다. 기존 최근 16개와 수명 카운터의 의미는 유지한다.

## 검증

Verified: test-native-gateway.mjs 24개, test-completion-selection.mjs 46개, test-native-protocol.mjs 통과. 두 gateway에서 같은 세션·부모자식·반복 요청 연결, 다른 세션 분리, 재시작 분리, 실패 요청 연결, 원문 ID 비노출을 확인했다. prepare/upstream 실패 누적 분류, 오래된 실패의 유지, snapshot을 변경해도 내부 카운터가 바뀌지 않음, 실패 단계 합계 일치를 검사했다. diff 검사 완료.

Not verified: 새 참조값과 단계별 카운터의 실제 사용자 세션 출력. 실제 보조 이벤트는 여전히 미관찰이며 전체 동적 호출 경로·provider/fallback 검증은 미완료다. 기존 인증/live·symlink 거부를 우회하지 않았다.

다음 실제 검증은 역할 기본값과 다른 명시 모델의 우선순위·inherit 경로다. 그때 새 참조값으로 각 agent의 요청을 구분한다. 이미 통과한 무명시 Plan과 단일 완료 복귀만을 반복할 필요는 없다.
