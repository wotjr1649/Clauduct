# 무명시 Plan 및 누적 진단 실제 성공

세션 c94bb0ac-3d65-4e2a-87fa-36192334d0bb, 실행 코드 기준 78986b2. 메인과 연결 자식 acae443bffeab376d 원본을 읽기 전용으로 대조했다.

메인 41행 Agent 호출은 subagent_type=Plan이며 model 필드가 없다. 자식 metadata의 agentType=Plan, toolUseId=call_Sv9DFHEhcJ32FiOxgz9CqHNC가 원래 호출과 일치하고 model 필드도 없다. 자식 11/12행에서 models.mjs Read가 정상 반환됐다. 메인 50행에 최종 완료 결과가 도착했다.

메인 57행의 원본 request-status 결과에서 request 10(05:05:00.534Z)과 11(05:05:07.611Z)은 role=Plan, roleRegistered=true, subagent=true, selectionSource=role-default, model=gpt-5.6-sol, effort=xhigh, success=true다. 코드상 모델 기본값이나 에이전트 자기보고가 아닌 실제 gateway 라우팅 증거다.

| lifetime 필드 | 시작: 메인 38행 | 종료: 메인 57행 | 차이 |
|---|---:|---:|---:|
| started | 2 | 7 | +5 |
| succeeded | 2 | 7 | +5 |
| failed | 0 | 0 | 0 |
| auxiliaryMetadataEvents | 0 | 0 | 0 |
| unsupportedEvents | 0 | 0 | 0 |

두 결과 모두 scope=gateway-lifetime이다. 증가량에는 메인 모델 요청도 포함된다. 수집은 정확히 두 번이며 Agent는 하나, 자식 Read는 하나다. Skill·SendFeedback·추가 에이전트·API 오류는 관찰되지 않았다.

Verified: 모델 무명시 Plan의 sol/xhigh 라우팅, Read 왕복·최종 결과 반환, 새 누적 진단의 실제 출력 및 정상 증가. 문서 diff와 원본 고정 필드를 확인했다.

Not verified: 실제 보조 이벤트 발생, 모든 모델의 500K 수용과 자식 자동 압축, 모든 동적 호출 경로의 반환. 성공한 검증을 단순 문서 기록 때문에 다시 실행하지 않았다.
