# 사용자 중단 metadata의 최초 선택 거부

## 원인과 범위

보안 경계 대조 중 일반 Agent/Task 생성 호출 및 연결된 Skill의 최초 선택에서 metadata.stoppedByUser=true를 거부하지 않는 공백을 발견했다. 생성 호출이 pending에 남아 있고 지연된 자식 요청이 선택기에 도달하면, 중단된 metadata의 호출·역할·모델·부모가 일치한다는 이유로 route를 확정할 수 있었다. Workflow·native fork·기존 완료 복귀에는 중단 검사 일부가 이미 있었으나 일반 최초 생성에는 없었다.

이것은 합성 검사로 확인한 선택기 결함이다. 실제 native가 사용자 중단 후 이런 요청을 보냈다는 증거는 없으며, 모든 사용자 취소가 무시됐다는 뜻도 아니다. SubagentStop이 이미 gateway 등록을 제거한 요청은 별도 기존 경계다.

신뢰 경계: 등록과 pending 생성 호출은 관계 증거이지 사용자 중단을 해제할 권한이 아니다. metadata의 중단 표시를 읽은 선택기는 transport 전에 거부해야 한다. metadata 파일은 실행하지 않고 기존 크기·경로·identity 검사 아래 읽는다.

## 수정과 재현

src/agent-selection.mjs의 공통 비-Workflow 선택 분기에서 stoppedByUser=true이면 IDENTITY로 즉시 거부한다. pending 소비나 route 확정 전에 적용하며, 기존 완료 진단 단계는 보존한다. native hook·권한·전역 설정·모델 기본값·inherit snapshot·Workflow 선택은 변경하지 않았다.

수정 전 test-agent-selection.mjs에 추가한 첫 중단 Agent 검사가 Missing expected rejection으로 실패했다. 수정 후:

- Agent/Task × general-purpose/clauduct-sol/clauduct-inherit 6개 최초 선택 거부.
- 인증된 Skill 결과로 연결된 중단 자식 최초 선택 거부.
- 기존 loopback 행렬 8개 역할/정의의 중단 metadata 요청이 HTTP 400, selection/IDENTITY이며 합성 transport 호출 증분 0.
- 합성 fixture에서 정상 metadata로 교체하고 새 등록한 뒤 기존 정상 선택·병렬 요청·부모 snapshot 검사가 계속 통과. 이 fixture 조작은 실제 사용자 중단을 해제하는 기능이나 운영 지침이 아니다.

## Verified

다음 명령을 Node v24.19.0에서 실행했다. 쓰기가 필요한 검사는 src 아래 임시 fixture만 만들고 정리한다. 실제 Claude·외부 요청은 0이며 gateway 검사의 실제 인증 조회도 0이다.

```text
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-agent-selection.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-completion-selection.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-workflow-selection.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-gateway.mjs
```

결과: agent-selection passed=true, completion-selection 46, workflow-selection 36, native-gateway 35 통과. 완료 복귀의 부모/손자 중단·위조·재사용 거부 검사도 유지됐다.

## Not verified / Blocked by

실제 native의 중단 시점 경쟁 조건, 선택이 이미 캐시된 요청의 모든 취소 조합, metadata 파일 시스템 경쟁 조건 전수 검증, symlink/junction 동적 검사는 미검증이다. 이번 수정은 선택기에 진입해 중단 표시를 읽는 경우를 다루며 이미 실행 중인 모든 요청의 취소를 새로 구현한 것이 아니다.

기존 인증된 자동 native 실행 및 symlink 거부를 유지한다. 실제 세션 metadata를 변조하거나 권한을 늘려 시험하지 않는다. session-10·11의 정상 상속 시험을 반복하는 것만으로 이번 중단 경쟁 조건을 입증할 수 없으므로 추가 사용자 시험은 요청하지 않는다. 전체 호출 경로 감사는 계속 남아 있다.
