# 완료 알림 복귀 실제 성공

세션 c19c8b14-a750-4dbf-bae0-a9ce9404da19의 메인과 연결된 두 에이전트 원본을 읽기 전용으로 대조했다. 실행 코드 기준은 208eb1f다.

| UTC | 원본 | 관찰 |
|---|---|---|
| 04:51:55.780 | 부모 a23b1ae572d7a3874 19행 | 최초 end_turn |
| 04:52:41.933 | 자식 a86a2acc754d0fdb7 14행 | 45초 대기 정상 반환 |
| 04:52:44.082 | 자식 20행 | Read 정상 반환 |
| 04:52:49.791 | 자식 23행 | end_turn |
| 04:52:50.098 | 부모 20행 | native completed 알림 |
| 04:52:56.318 | 부모 23행 | 복귀 후 end_turn |
| 04:52:56.684 | 메인 47행 | 부모 최종 완료 결과 도착 |
| 04:53:00.748 | 메인 50행 | request-status 실제 도구 결과 |

원본 진단 request 22는 startedAt=04:52:50.239Z, subagent=true, selectionSource=verified-completion-resume, model=gpt-5.6-luna, effort=max, success=true다. metadata의 부모/자식 및 최초 Agent 호출 ID 연결이 일치한다. 이 실행에서 Skill·SendFeedback·SendMessage·TaskOutput·TaskStop 호출과 isApiErrorMessage 오류는 관찰되지 않았다.

Verified: 부모 선종료 → 자식 완료 → native 알림 → 부모 재기동·최종 결과 반환의 단일 general-purpose 경로. request-status는 모델 자기보고가 아닌 원본 tool_result로 확인했다.

진단에는 이전 c9ac9644 사전 점검 시각의 request 4/5도 포함된다. gateway 수명과 native 세션 수명은 다르며 request 번호 전체를 이 세션에 귀속하지 않는다. 이번 검증 구간 request 7–24의 보존된 모델 요청 11개는 성공이다. 마지막 16개 보존 한계와 관리 endpoint 때문에 request 번호의 공백을 실패로 해석하지 않는다.

Not verified: 과거 35985327 실패의 정확한 원인, 오류 없는 high 전체 재검토, 다중 알림·실패/취소 알림 복귀·장기 안정성. 보조 metadata 이벤트는 반환된 13개 진단에서 모두 0이므로 실제 발생·처리는 아직 입증되지 않았다. 이번 성공으로 이전 실패가 소급해 설명되지는 않는다.

문서 변경은 원본 고정 필드와 diff를 확인했다. 이미 통과한 모델 실행이나 로컬 suite는 이 증거 기록만을 위해 반복하지 않았다.
