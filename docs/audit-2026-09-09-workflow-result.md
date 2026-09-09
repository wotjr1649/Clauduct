# Workflow 빈 결과 원인과 응답 순서 수정

## 실제 세션 판정

대상은 `c18a7379-f1ac-4a8a-a738-75636956c296`다. 메인 JSONL 95행, 직접 연결된 Workflow 자식 JSONL 18행, metadata, journal 3행, run JSON, script를 확인했다. 원본 추론·인증값은 기록하지 않는다.

| 근거 | 관찰 |
|---|---|
| 메인 30–31행 | 첫 작업 도구 Read로 보완 문서 읽음 |
| 메인 40–41행 | workflow-authoring 로드 |
| 메인 52행 | 기준점 sol/high, lifetime started/succeeded/failed = 3/3/0 |
| 메인 56–57행 | Workflow 1회, call_g4MZXmoIOeSKtT5uIxoXKpqU, task wixtdjld7, run wf_a6003657-be6 |
| 자식 13–14행 | models.mjs Read 1회 성공, 결과에 네 모델 정의 존재 |
| 자식 17행 | text 블록에 완료 문자열 존재. 이름 답변은 sol만 포함하여 네 이름 출력 조건은 미충족 |
| 자식 18행 | 같은 response ID의 redacted_thinking 블록이 마지막 assistant로 기록되고 end_turn으로 종료 |
| journal 3행 / run JSON | result가 빈 문자열. run status=completed지만 result.completed=false |
| 메인 71행 | 완료 알림 도착, agents_done=1, agents_empty_result=1, tool_uses=1 |
| 메인 75행 | request 12/13 모두 sol/high, workflow-result, workflow-subagent, roleRegistered=true, success=true. lifetime 8/8/0 |
| 메인 82행 | 성공으로 숨기지 않고 Not verified 보고. 자식 원본을 추가 확인한 이번 조사에서는 Read 자체는 성공으로 확정 |

두 상태 수집의 correlationScope는 `90d9b17407f5c717a8b20804fc36a838`, sessionRef는 `931822e6ba957e64ce078c1b500fedb4`로 같다. 자식 agentRef는 `90c1fef4fc479f4bf6520e50eb133f43`다. sol 기본 effort xhigh와 다른 high가 전달되었으므로 비기본 effort 전달의 실제 증거다. 마지막 상태 이후 최종 응답은 8/8/0 집계 밖이다.

판정: 이전 metadata MISSING 문제는 이 실행에서 해결되었다. GPT 라우팅·Read·native 완료 알림은 성공했지만, 자식 텍스트의 Workflow 반환 연결은 실패했다. 실패 누계 0은 gateway 요청 성공을 뜻하며 Workflow 업무 결과 성공을 보장하지 않는다. 임의 반복 실행·대체 Agent·파일 쓰기 도구는 기록에서 관찰되지 않았다.

## 원인

활성 native 2.1.266 실행 파일 SHA256은 `d2c5f7b3b6a12819097ceb6efbce2a390157166003fcaee32dbde0e6d7b45ef7`로 기존 조사와 같다. 실행하지 않고 번들 코드를 정적으로 확인했다.

Workflow runner는 assistant가 yield될 때마다 마지막 메시지를 교체하고, 종료 시 그 메시지 content의 text만 추출한다. 기존 Clauduct 응답기는 text delta를 먼저 전달한 뒤 finish에서 redacted_thinking을 붙인다. 실제 자식 17/18행이 이 순서와 일치한다. 따라서 모델이 완료 문자열을 생성해도 마지막 reasoning-only 메시지에서 추출되는 Workflow 결과는 빈 문자열이다.

## 수정과 한계

공통 응답 변환기에 내부 deferText 옵션을 추가하고 gateway의 검증된 selectionSource=workflow-result에만 적용했다. 재시도 시 새 응답 변환기에도 같은 옵션을 유지한다. 다른 역할명이나 요청 본문이 이 옵션을 켜지 못한다.

Workflow 자식은 정상 완료·snapshot·usage 검증 후 opaque reasoning, 하나로 합친 text, tool_use 순서로 전달한다. 텍스트 부분은 순서를 보존하며 줄바꿈으로 결합하고 기존 item 크기 제한을 적용한다. tool_use가 없는 최종 응답은 text가 마지막이다. 텍스트를 복제하거나 reasoning을 버리거나 native 설치 파일·journal·세션 기록을 고치지 않는다.

절충: 이 경로의 텍스트는 완료 검증까지 지연된다. 메인과 일반 Agent의 기존 조기 text delta 전달은 변경하지 않았다. 기존 ping, 취소, 크기 제한, 모델 검증, 내용 전달 후 무재시도 정책은 유지한다. 다른 native 결과 소비자의 동일 문제 여부는 별도 미검증이며 전체 호환성 해결이라고 주장하지 않는다.

## 검증

Verified: 수정 전 새 protocol 회귀 검사가 실패하는 것을 확인한 뒤 수정 후 통과했다. Workflow 최종 블록 순서, text 단일 전달, 여러 text 부분 보존, opaque reasoning의 다음 요청 복원, tool_use 순서, 불완전 응답·snapshot 불일치·출력 한도 거부를 검사했다. 기존 일반 응답의 조기 delta 검사도 통과했다.

Verified: 인증된 loopback의 실제 hook 등록 → selector → gateway → SSE → status 검사에서 뒤늦은 reasoning이 있어도 마지막 블록은 text이며 결과 OK가 보존됐다. transport 이벤트는 합성으로, native 실행을 대신한 성공 증거가 아니다.

```powershell
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-protocol.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-workflow-selection.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-gateway.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-transport.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-completion-selection.mjs
```

Workflow 25개, gateway 35개, 완료 복귀 46개 및 protocol/transport suite 통과. 외부 요청과 실제 인증 조회·Claude 실행은 0이다.

Not verified: 수정 후 실제 Workflow result가 비어 있지 않은지, 완료 문자열과 네 모델 이름이 메인까지 복귀하는지. 실제 GPT 명시 선택·다중 자식·중첩·재개·장기 실행도 이번 증거에 포함하지 않는다. Blocked by: 기존 인증 실행 및 symlink 검사 제한을 유지하여 직접 재실행하지 않았다.

다음 실제 검사는 새 Clauduct 프로세스에서 동일한 읽기 전용 Workflow 1회·자식 1개로 수행한다. 현재 사용자가 선택한 메인 model/effort를 기준으로 하고 하드코딩하지 않는다. gateway 성공뿐 아니라 비어 있지 않은 Workflow result, completed=true, 네 이름과 완료 문자열, 메인 보고를 각각 확인한다. 실패 시 자동 반복하지 않는다.
