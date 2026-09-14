# Workflow 기본 자식 실제 성공과 명시 선택 후속 검증

## 실제 판정

세션 `08594a7e-3023-4ee2-97f7-83ce93498841`의 메인 JSONL 106행과 연결된 자식 18행, metadata, journal 3행, run JSON, script를 읽기 전용으로 확인했다. c81ca43의 빈 결과 수정 이후 local inline Workflow 기본 자식 1개 경로는 종단 간 통과했다.

| 근거 | 관찰 |
|---|---|
| 메인 30–31행 | 첫 작업 도구가 보완 문서 Read |
| 메인 40–41행 | workflow-authoring 로드 |
| 메인 57행 | 기준점 sol/high, lifetime started/succeeded/failed=4/4/0 |
| 메인 66–67행 | Workflow 1회, call_drqgbRFZwkUKjhdZNfHkWIzW, task wh86ej1st, run wf_c1934798-aff |
| 자식 13–14행 | models.mjs Read 1회, 네 모델 정의가 포함된 1162자 결과, 오류 없음 |
| 자식 17–18행 | 같은 response ID의 redacted_thinking 다음 text, end_turn. 마지막 text에 네 이름과 완료 문구 모두 존재 |
| journal / run JSON | 자식 결과와 Workflow result가 동일한 44자·2줄 문자열. completed=true, agentCount=1, totalToolCalls=1 |
| 메인 82행 | native 완료 알림, completed=true, agents_empty_result=0 |
| 메인 86행 | 자식 request 13/14 모두 sol/high, workflow-result, workflow-subagent, roleRegistered=true, success=true. lifetime=9/9/0 |
| 메인 93행 | 자식의 44자 문자열이 그대로 최종 보고에 포함됨 |

correlationScope는 두 상태에서 `a8668de586f2f6e7dce2a1658b17b7a6`, sessionRef는 `5b44b06eed235d362edbcfc17ea38ca9`로 일치한다. 자식 agentRef는 `68ff9a50ab3c431e3168a275ba2a2d08`다. sol 기본 xhigh가 아닌 high가 전달됐으며, 이는 이 세션에서 관찰한 값이지 다른 세션의 모델 설정이 아니다. 마지막 상태 이후 메인 최종 응답은 9/9/0 집계 밖이다.

Workflow 입력 script·저장 script·run JSON의 script가 정확히 일치하며 agent 호출은 1개다. 메인 도구는 Read 2회, Skill 1회, 상태 수집 Bash 2회, Workflow 1회이며 자식 도구는 Read 1회다. 기록에서 재시도·대체 Agent·쓰기 도구는 관찰되지 않았다. 전체 파일시스템의 무변경을 이 도구 목록만으로 보증하지는 않는다.

이전 c18a7379의 빈 문자열과 네 모델 이름 누락은 이번 실행에서 재현되지 않았다. 신규 런타임 결함은 확인되지 않아 제품 동작은 변경하지 않았다.

## 보완과 로컬 검증

기존 test-workflow-selection.mjs의 실제 loopback 검사를 부모 sol/medium 상속 외에 명시 luna/high, terra/xhigh로 확장했다. 각 경우 native 요청의 output_config.effort가 selector를 거쳐 실제 전송 model/reasoning.effort 및 필터링된 status에 그대로 남는지 확인한다. 모든 경우 reasoning 보존, 마지막 text 및 end_turn, 원문 비노출을 함께 검사한다. 응답 transport는 합성이며 실제 native 명시 선택의 성공으로 대체하지 않는다.

Verified: 아래 명령을 실행하여 Workflow 27개, gateway 35개 및 protocol suite 통과. 외부 요청·실제 인증 조회·Claude 실행은 0이다.

```powershell
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-workflow-selection.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-protocol.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-gateway.mjs
```

Not verified: native Workflow에서 명시한 GPT model/effort의 실제 실행, 다중 자식·병렬, 중첩·resume·custom agentType, 장기 안정성. Blocked by: 기존 실제 인증 실행 및 symlink 검사 제한을 유지했다. 이미 성공한 기본 자식 시험을 무조건 반복하지 않고 다음에는 명시 선택만 추가한다.

다음 사용자 시험은 local inline Workflow 1회·읽기 전용 자식 1개로 model/effort를 직접 지정한다. 부모가 luna가 아니면 luna/high, 부모가 luna이면 terra/xhigh로 선택하여 부모와 다른 모델 및 해당 자식 모델의 비기본 effort를 구별한다. 메인 선택은 바꾸지 않는다. 이것은 고정 역할 기본값을 변경하는 것이 아니라 해당 Workflow 호출의 명시 override 검사다. 정상 result와 종료 후 메인의 기존 model/effort 유지까지 확인한다.
