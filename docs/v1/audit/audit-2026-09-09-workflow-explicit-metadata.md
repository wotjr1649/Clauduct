# Workflow 명시 모델 metadata 수정

## 실제 실패

세션 `927eefdf-0022-4bec-b69d-d776b21ebacb`의 메인 77행, 연결된 자식 12행, metadata, journal 3행, run JSON, script를 확인했다.

| 근거 | 관찰 |
|---|---|
| 메인 30행 / 40행 | 프롬프트 Read 후 workflow-authoring 로드 |
| 메인 47행 | 기준점 sol/high, lifetime started/succeeded/failed=3/3/0 |
| 메인 55–56행 | Workflow 1회, call_epYOEKltOVPlPqxtjHlk86kZ, task wyaf835bz, run wf_0e7247ee-422 |
| 입력·저장·run script | 정확히 일치, agent 1회, model=gpt-5.6-luna, effort=high 명시 |
| 자식 metadata | workflow-subagent, spawnDepth=1, description=explicit-read-once, model=gpt-5.6-luna. effort 필드는 없음 |
| 메인 67행 request 11 | requestedModel=gpt-5.6-luna, selectionFailure=IDENTITY, success=false, attempts=[] |
| 자식 / journal | Read 포함 도구 호출 0회, journal은 launched → started → failed |
| 메인 64행 완료 알림 / run JSON | completed=false, result=null, agents_error=1. task status=completed는 성공 증거 아님 |
| 메인 67행 / 73행 | lifetime=7/6/1, 메인 sol/high 유지, 실패를 보고하고 재호출하지 않음 |

correlationScope=`0cbb24ef93a33407517e47f1b2c519aa`, sessionRef=`fd57098ae6674487075eaeb5551a295a`는 기준점과 종료점에서 일치한다. 자식 agentRef는 `f691434a93168950643cff4ad618e8cf`다. 마지막 상태 이후 최종 응답은 이 누계 밖이다. roleRegistered=false는 진단이 선택 성공 이후 채워지는 구조 때문이므로 hook 미등록의 증거로 단정하지 않는다.

## 원인과 수정

기존 Workflow 검증기는 metadata.model이 있으면 무조건 IDENTITY로 거부했다. 기본 상속 자식에서는 필드가 없었지만 native 명시 선택에서는 model을 저장한다. 이전 loopback 명시 선택 시험도 metadata에 model을 넣지 않아 이 차이를 놓쳤다. 따라서 이전 27개 합성 검사 통과는 실제 native metadata 호환성을 입증하지 못했다.

이제 관계 검증이 끝난 metadata.model은 문자열이며 기존 네 GPT 선택 규칙으로 해석 가능하고, native 요청에서 선택된 모델과 일치할 때만 허용한다. metadata로 요청 모델을 덮어쓰지 않는다. 미지원·불일치·null·객체 값은 MODEL로 거부한다. 관찰되지 않은 metadata.effort 필드는 계속 거부하며 effort는 검증된 자식 요청으로 처리한다.

기존 인증된 Workflow 호출·script 해시·run/journal·live Start·세션·자식 ID·경로·시각·metadata 재확인과 중단 검증은 유지한다. 전역 설정·hook·일반 Agent 고정 기본값·모델 목록은 변경하지 않았다.

## 검증과 한계

Verified: 명시 loopback fixture에 실제처럼 metadata.model을 추가하자 수정 전 HTTP 400 AGENT_SELECTION_UNVERIFIED_IDENTITY를 재현했다. 이 실패 실행 끝에 Node/Windows의 UV_HANDLE_CLOSING assertion도 출력됐다. 별도 원인은 미확정이며 수정 후 정상 종료 검사에서는 재현되지 않았다. 해당 오류를 숨기는 처리는 추가하지 않았다.

Verified: 수정 후 Workflow 32개, gateway 35개, completion-selection 46개, agent-selection 및 native-protocol suite 통과. 명시 luna/high·terra/xhigh에 실제 형태의 sidecar를 사용하며 모델 불일치·미지원·null·객체·예상 밖 effort 거부를 추가했다. 기존 상속과 reasoning/text 최종 반환 검사도 유지한다.

```powershell
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-workflow-selection.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-agent-selection.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-gateway.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-protocol.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-completion-selection.mjs
```

외부 요청·실제 인증 조회·Claude 실행은 0이다. Not verified: 수정 후 실제 native 명시 모델·effort·Read·정상 result 복귀. 명시 high가 backend까지 전달된 증거는 이번 실패 기록에 없다. 다중 자식·중첩·재개·장기 안정성도 미검증이다. Blocked by: 기존 실제 인증 실행 및 symlink 검사 제한을 유지했다.

다음은 같은 단일 명시 선택 시험을 수정본으로 수행한다. 이미 성공한 기본 상속 전체 시험이나 다중 자식으로 범위를 넓히지 않는다. 메인 선택은 유지하고 자식 model/effort만 명시한다. 실제 실행 요청은 사용자가 제공하는 후속 프롬프트로 수행한다.
