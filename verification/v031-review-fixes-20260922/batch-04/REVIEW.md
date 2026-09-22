# 직접 검토와 native Claude 리뷰 처분

이번 변경은 최초 502 분류·stream 종료 기록, SDK 빈 응답 처리, Workflow 시작 경계,
한 step의 실행 소유권을 대상으로 한다. 이전 v0.3.1 리뷰 18 confirmed / 5 hold의 처분은
`../batch-03/REVIEW.md`에 보존한다. 그 전체를 새 미해결 목록으로 다시 세지 않는다.

## 독립 리뷰 경로

Clauduct를 거치지 않는 native Claude를 사용했다. 사용자가 선택한 `opus` 설정을 그대로
사용하고 effort는 `xhigh`로 요청했다. backend 보고 모델은 `claude-opus-5`다.
공개 임시 project/profile과 검토한 소스 패킷만 전달했으며 tools/MCP는 비활성이다.
OAuth 값은 출력·파일에 쓰지 않았다. Claude는 정적 리뷰를 했고, 최소 재현·실행·검사는
이 작업자가 로컬에서 수행했다. Claude가 테스트를 실행했다고 표시하지 않는다.

| 단계 | 실제 결과 |
|---|---|
| 연결 smoke | PID 27252, 4.92초, exit 0, 지정 응답 반환 |
| wait 분야 리뷰 | PID 8576, 290.67초, 결과 반환. `wait-review-result.md` |
| routing 분야 리뷰 | PID 13224, 300.18초 한도, 결과 없음. 종료 확인 후 같은 단위 재발송 없이 남은 검토를 로컬에서 완료 |
| 수정 후보 통합 리뷰 | PID 11976, 591.28초, exit 0, 결과 반환. `integrated-review-result.md` |
| 통합 F1 수정 확인 | PID 2224, 264.69초, exit 0, `CLOSED` 반환. `fix-review-result.md` |

각 입력 hash·actual usage·요청 effort·모델·시간·반환 여부는 해당 `*-metadata.json`에 있다.
시간 초과와 빈 결과를 승인으로 세지 않는다. batch-03의 6분 미수신 시도도 역사적 실패로
유지한다. 후속 통합 요청은 새로운 수정 후보를 검토한 별도 단위다.

## 발견 사항의 재현과 처분

| 항목 | 판정과 근거 |
|---|---|
| wait-1: 같은 step의 두 요청이 hold 결정 충돌 | 다른 body/class의 재입장을 실제 HTTP로 확인. `owner-before.txt` FAIL. step의 conversation 소유권을 하나로 고정한 뒤 PASS. auxiliary라는 세부 가정은 `conversationRequest` 경계와 맞지 않지만 다른 body 경로의 실제 결함은 수정 |
| wait-2: 결과 전달 callback 미호출로 예약 누수 | 반박. `results.deliver`는 요청에 본문을 넣을 뿐 예약을 만들지 않으며 `finish(false)`는 no-op. 성공적인 실제 부모 전달 때만 commit한다. `TestAgentResultsRequireBodyAndSuccessfulParentDelivery` PASS |
| 통합 F1: 끝난 TUI 알림이 awaiting_children에 잔류 | 확인. `tui-notification-before.txt`에서 실제 event module의 filesystem 기록이 기대 `turn_ended` 대신 `awaiting_children`. 제어 응답 hold와 실제 대기 여부 wait를 분리. `review-boundaries-after.txt` PASS, Claude 수정 확인 CLOSED |
| 수정 확인의 잔여: 자식 없는 Workflow도 영구 대기할 가능성 | 수용하여 추가 수정. Workflow call의 존재는 빈 제어 응답을 허용하는 근거일 뿐 pending child를 의미하지 않는다. `step.waiting`은 오직 admission snapshot의 Pending으로 정하고 launch 경계는 별도 조건으로 유지. 준비 전/연결 후 둘 다 waiting=false를 강제하는 `TestWorkflowLaunchWaitPrecedesItsFirstChild`와 `final-boundaries.txt` PASS |
| 통합 F2: 429 뒤 재요청이 ledger에 거부됨 | 정책상 의도된 동작. transport 시도 후 자동 재실행을 허용하지 않는 채택 정책을 우선한다. HTTP 상태는 원인 분류이며 실행 허가가 아니다. 첫 RATE_LIMITED 429 → 동일 step 400, backend 1회 → 명시적 새 turn 200/backend 2회가 `TestRateLimitDoesNotAuthoriseNativeReplay`에서 확인됨. 오래된 retry 설명을 명확히 보완 |
| 통합 F3: 빈 제어 응답이 완료됐으나 결과 없는 자식으로 전달됨 | 반박. `beginAnswer`의 delivered는 HTTP 전달이며 task stopped를 만들지 않는다. `stoppedResult`는 pending grandchildren을 확인하고 awaiting_children을 유지한다. `deliver`는 stopped/전달 가능 상태에만 본문 미확보 알림을 만든다. 실제 콜백에 빈 문자열/true를 넘긴 최소 재현과 기존 중첩 결과 검사에서 pending 1, unavailable 0, 부모 메시지 0 확인 |

F2/F3를 숨기거나 경고·assertion을 없애지 않았다. `review-refutations.txt`에 직접 판별 결과를
남겼다. 마지막 Workflow wait 조건은 Claude의 잔여 지적을 반영한 로컬 수정이며, `CLOSED`
답변을 그 수정 이후의 재리뷰로 소급 표시하지 않는다.

## 직접 확인한 경계

- `readNativeStep`의 session/agent/turn/index/mode/eligible 검증과 실행 소유권을 대조했다.
  제어 `wait`는 gateway 내부 필드에서만 오고 native receipt의 입력 필드가 아니다.
- SDK 제어 상태는 `toolUses=0`, `answer=''`, `end_turn`을 확인한 응답에서만 생성한다.
  실제 모델 답변·Read/tool call·명시적 새 입력은 그대로 유지한다.
- 완료 알림의 빈 응답과 정상 사용자 입력의 빈 응답을 구분한다. `Builder.Answer()`의
  빈 값은 child result와 혼동하지 않으며 native의 완료와 결과 전달이 추가로 필요하다.
- replay guard는 dispatch 이전에만 예약을 해제하며 불확실한 실행은 잊지 않는다.
  compaction·독립 auxiliary·새 step/turn은 기존 의미를 보존한다.
- batch-03 routing/설정의 남은 검토를 로컬에서 완료했다. compound option의 값 경계,
  settings 보존·연결 설정 충돌 거부, plugin namespace 내 불확실성, root/nested Workflow
  metadata 분리, 확인된 실제 model/effort, optional count의 같은 class 판정을 재확인했다.
  발견된 추가 제품 결함은 없다. 분야별 timeout을 별도 Claude 승인으로 표현하지 않는다.

원본 소스 백업과 현재 소스 hash, 최소 재현, 제거 mutation, 일반/race, 실제 backend SDK/TUI를
함께 판정한다. 이 범위의 발견 사항은 모두 수정 또는 근거 있는 반박으로 처리했으며,
미출하 상태와 역사적 증거 한계는 `REPORT.md` 및 `evidence.json`에 남긴다.
