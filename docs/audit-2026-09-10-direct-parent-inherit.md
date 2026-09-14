# 직접 부모 기준 inherit의 gateway 연결 검사

전체 목표 점검에서 직접 부모 기준 손자 상속의 선택기 검사는 있었지만, 등록된 부모의 실제 gateway 요청이 손자 생성 snapshot으로 이어지는 통합 검사를 보강할 여지가 있었다. 기존 test-agent-selection.mjs의 loopback fixture를 재사용했다. 제품 동작은 변경하지 않았다.

## 관찰한 검사

1. 기존 clauduct-inherit 부모를 luna/high로 검증한다.
2. 메인에 astra/max 요청을 보내 최상위 현재 모델과 직접 부모를 구별한다.
3. 등록된 부모의 native 형식 Agent 호출에서 subagent_type=clauduct-inherit를 선택하고 model 인수는 생략한다. 요청 본문이 astra/max여도 부모의 실제 전송은 기존 luna/high다.
4. 그 호출로 생성한 손자의 metadata.parentAgentId를 직접 부모에 연결한다. 잘못된 부모 헤더 요청은 HTTP 400이며 합성 transport 호출 수가 늘지 않는다.
5. 올바른 부모 헤더 요청은 luna/high로 전송되고 definition-inherit·success=true다. 손자 status.parentRef는 생성 호출에서 직접 수집한 부모 agentRef와 일치한다. 요청 번호 차이로 관계를 추정하지 않는다.

Verified: `node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-agent-selection.mjs` 통과. 실제 selector, gateway, hook 등록, 요청 변환, status를 사용하며 metadata와 transport 응답은 합성이다. 외부 요청·실제 Claude 실행은 0이다. 기존 정의 모델 4개·역할 기본값·snapshot·재개·거부 검사도 같은 suite에서 유지된다.

Not verified: 실제 native 다단계 Agent 위임의 직접 부모 모델/effort 상속. Workflow 기본 상속의 성공으로 이 별도 요구사항을 대체하지 않는다. 전체 목표는 완료가 아니다. 준비된 병렬 Workflow 실제 시험은 그대로 유지하며 추가 실제 호출을 자동 실행하지 않았다.

Blocked by: 기존 인증된 실제 native 실행 및 symlink 검사 제한을 유지한다. 해당 효과를 우회하지 않고 사용자 실행 증거로 확인한다. 테스트 fixture 보완만 커밋하며 전역 설정·모델 목록·native 스키마는 변경하지 않는다.
