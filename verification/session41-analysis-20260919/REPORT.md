# S41 실제 세션 실패 원인과 다음 수정 범위

대상 UUID: `e5fdc752-53f9-49dd-a779-7573275133e4`. native `2.1.278`, 개발 바이너리 `051177d220dbfd6277f05c76d7669a25f83e6d55`, SHA256 `5bc5ac8a5c3f7467f13a74d418abcaab05f430201f176e7784ea794fd0f14aeb`를 run.json과 대조했다. 아래 시간은 KST다.

이번 작업은 기존 세션의 진단·원인 재현·수정 설계다. 제품 동작이나 바이너리는 변경하지 않았다. 기존 대화·profile·설정은 보존했으며 backend 호출과 실패 작업 재실행도 하지 않았다. 새 진단 자료는 이 디렉터리에만 보존한다.

## 판정

**복합 검증은 실패다.** process=SUCCESS/exit=0은 `/exit`에 따른 프로세스 종료 사실이다. 기능 승인이 아니다. API 실패 4건은 선택 검증 실패 2건과 텍스트 형식 거부 2건이다. 취소 4건은 별도로 집계되어 있다. 종료 시 active=0, nativeReaped=true, watchdog 강제 종료와 deadline 만료는 없었다.

핵심 결함은 세 가지다. (1) 중단된 텍스트의 native 형식 미지원으로 후속 대화 전체가 거부된다. (2) 중첩 Agent의 자동 재진입에서 부모 식별이 빠져 이미 검증한 선택을 거부한다. (3) Workflow 선택 검증은 현재 부모 선택 상속만 구현되어 있어 다른 모델의 명시적 요청을 거부한다. 이와 함께 실패·대기·보고 수신 상태의 연결이 부족해 결과 집계와 부모 설명에도 오류가 생겼다.

## 1. Esc 뒤 모든 일반 프롬프트가 거부된 이유

| 시각 | 실제 기록 | 해석 |
|---|---|---|
| 18:41:37 | seq 151, stage=count, CANCELLED/499, countMs=814 | 첫 취소는 정확 계수 중 발생했다. API 실패로 잘못 분류되지는 않았다. |
| 18:42:09~16 | 같은 S41_ESC_TARGET 재전송. seq 153은 firstByte 이후 CANCELLED, events=41, terminalObserved=false | 실제 답변 텍스트가 일부 나온 뒤 두 번째 취소가 있었다. |
| 18:42:16.435 | native transcript index 208의 assistant text에 `citations:null` 저장 | 이전 정상 text에는 없던 필드다. |
| 18:43:04 | S41_ESC_TARGET 재전송 → seq 155, request 단계 TEXT_FIELDS/400 | 해당 이력을 포함한 새 요청이 backend 실행 전에 거부된다. |
| 18:44:12 | S41_RECOVERY → seq 157, request 단계 TEXT_FIELDS/400 | 다른 프롬프트로 바꾸어도 동일 이력이 포함되어 거부된다. |

현재 `go/internal/protocol/anthropic/request.go:532`는 text 블록의 `type`, `text`, `cache_control`만 허용한다. `citations`가 null이어도 알 수 없는 필드로 거부한다. native 2.1.278의 내장 스트림 처리 코드에도 부분 텍스트를 `{type:"text",text:...,citations:null}`로 구성하는 경로가 있으며, 이 세션에 그 결과가 실제로 저장됐다.

같은 공개 텍스트에서 citations만 바꾼 오프라인 재현:

- 필드 없음: DecodeRequest 성공.
- `citations:null`: TEXT_FIELDS.
- `citations:[]`: TEXT_FIELDS. 이 배열 변형은 비교 시험이며 실제 세션에서 관측한 값은 null이다.

이후 auxiliary 요청 154/156은 계속 성공했다. 따라서 네트워크 전체 중단이나 launcher 종료에 대한 증거는 없다. 일반 대화가 반복해서 보내는 이력의 형식이 거부를 지속시키는 직접 원인이다. 최초 count 취소의 성공이 스트리밍 취소 후 회복까지 보장하지 않는다.

원본 HTTP 본문은 저장되지 않았다. 원본 이력의 해당 필드, native의 생성 코드, 실제 parser의 동일 형식 거부, 연속된 request 단계 오류를 대조했다. 실패 요청 전체를 byte 단위로 재생하거나 이 세션을 새 바이너리로 복구한 시험은 아직 하지 않았다.

인접한 transcript index 207/208에는 같은 부분 텍스트가 두 번 저장돼 있다. native가 다음 전송 전에 어느 기록을 선택·병합하는지는 이 보존 기록만으로 확정하지 않는다. 형식 호환성 수정 뒤 부분 답변이 중복 입력되는지도 별도로 확인해야 한다. 이를 추측으로 삭제하면 원래 응답을 훼손할 수 있다.

수정: 공통 Anthropic text decoder에서 의미가 빈 citations(null/빈 배열)를 엄격히 확인하고 수용한다. 실제 인용이 든 배열은 의미 보존 변환을 구현한 범위만 수용하고 나머지는 명시적으로 미지원 처리한다. 임의 필드를 전부 무시하거나, 부분 답변·이력 전체를 삭제하는 방식은 사용하지 않는다. messages와 count_tokens의 공통 decoder에 적용해 두 경로가 달라지지 않게 한다.

## 2. 깊이 3은 실행됐고, 중간 Agent의 자동 재진입에서 실패했다

실제 계보:

```
root
└─ A  a01d879441dcafe7c   general-purpose / Terra medium
   └─ A1 aa91de1182225625c general-purpose / Terra medium
      └─ A2 a85b21b3786f17b46 Plan / Terra medium
```

A2 metadata의 spawnDepth=3이다. 18:29:10의 완료 본문에는 MAPLE-27, 총 순액 465, 고유 event 10, 검산값 1390이 실제로 있다. 그러므로 부모가 나중에 말한 “A1 시작 직후 실패, A2 생성되지 않음”은 사실과 다르다.

A1은 18:28:59에 A2를 호출하고 18:29:03에 대기 문장을 출력했다. 18:29:10에 A2 완료 알림을 받아 다시 실행될 때 seq 73에서 AGENT_SELECTION_UNVERIFIED가 발생했다.

선택 journal과 native metadata에는 A1의 원래 parent=`a01d879441dcafe7c`가 있다. 실패 request의 status에는 parentAgentId가 없다. `messages.go`의 agentSelection은 요청 헤더를 `record.agent`와 delegationScope에 넣으며, `diagnostics.go`는 빈 parent만 생략한다. `delegation.go`의 이미 확정된 선택 경로는 원래 parent와 현재 scope.parent가 다르고 검증된 SendMessage resume도 없으면 거부한다.

동일한 확정 child에 대해 원래 parent가 있으면 통과하고, parent만 비우면 AGENT_SELECTION_UNVERIFIED가 발생하는 현재 경로를 오프라인 재현했다. native 2.1.278도 parentAgentId가 있을 때만 해당 헤더를 생성한다. 이 세션은 명시적 SendMessage가 아닌 하위 완료 이벤트에 의한 자동 재진입이다. native 내부의 어느 호출 지점에서 parent 값이 소실되는지는 추가로 작은 native 실험으로 확인해야 한다. 이미 선택이 검증된 ID를 다시 거부하는 Clauduct 조건과 그 입력 차이는 확인했다.

수정: 생성 시 계보와 각 실행 회차의 호출자 관계를 분리한다. 자동 재진입은 session/Agent ID, 기존 선택 journal, 원래 native metadata, 현재 native turn 및 완료 이벤트의 연결로 검증해야 한다. 그 검증을 통과한 경우에만 기존 선택을 유지한다. 빈 헤더를 무조건 허용하거나 클라이언트 요청 모델을 대신 쓰지 않는다. 다른 세션·위조 parent·stale turn·명시적 충돌은 계속 거부해야 한다.

추가 사실: A→A1의 원래 선택 receipt에는 model·effort가 모두 명시돼 있다. 사용자가 둘 다 생략하라고 한 지시와 달랐다. A1→A2는 원래 인자 둘 다 생략된 것으로 검증됐다. 실제 native transcript에는 어댑터가 넣은 alias가 보이므로 원래 생략 여부는 그 alias만으로 판단하지 않았다. A1에서의 API 거부는 이 지시 불일치와 분리해야 한다. 지정한 Terra/medium 자체는 첫 실행에 성공했다.

## 3. Workflow의 명시적 모델 요청은 현재 Go 경로에 구현되지 않았다

run ID=`wf_1c181b9b-259`.

- A=`a87c671e81d84501c`: script opts는 Terra/medium, native metadata의 model은 gpt-5.6-terra. seq 119에서 선택 단계 거부. backend 생성 실행 근거 없음.
- B=`a71ecd3dd2e15f11c`: opts의 model/effort 생략, 부모 Astra/low 상속, 실제 `WF_B 11` 반환.
- journal은 A의 started→failed, B의 started→result를 보존한다. Workflow 반환은 `{"a":null,"b":"WF_B 11"}`이다.

`go/internal/gateway/workflow.go`의 findWorkflow는 metadata의 model이 부모 모델과 다르면 거부하고, 최종 route도 부모 route로 만든다. 명시적 opts의 모델·effort와 child ID를 연결하는 구현이 없다. 기존 `TestWorkflowSelectionRequiresOriginAndUnmodifiedNativeEvidence/model`도 다른 모델을 거부하는 것을 기대한다. 이 테스트 통과는 명시적 모델 지원의 근거가 아니었다.

따라서 이번 실패는 단순 확률성 네트워크 오류가 아니다. 현 구현이 표현하는 범위 밖의 기능을 정상 경로로 시험하면서 드러난 지원 공백이다. 새 프롬프트를 만들 때 이 Go 구현의 제한을 분명히 구분하지 않은 점은 검증 설계의 누락이다.

수정 방향: Workflow 실제 실행에서 확정된 child별 model·effort를 run/child와 묶는 실행 전 증거를 확보하고, 일반 Agent와 같은 선택 검증·status 표현에 연결한다. 임의 script를 정규식으로 해석하거나 native request 값만 믿는 대체는 하지 않는다. 현재 확보한 journal에는 effort가 없으므로 metadata의 model만으로 전체 선택을 검증했다고 할 수 없다. native 확장 경로에서 실행 전 opts·child ID를 연결하는 실험이 먼저 필요하다. 이 연결의 전체 구현·검증은 아직 완료되지 않았다.

## 4. 결과 미확보 3건은 서로 다른 상태다

- A2는 계산 완료 본문 577바이트가 있는데 `awaiting_parent`다. 결과 자체가 생성되지 않은 것이 아니다.
- A1은 완료 알림 후 선택 검증에서 막혔다. `agentSelection` 거부가 새 실행의 `beginResult`/`bindNativeTurn`보다 앞서 발생하여, 이전 대기 회차와 새 실패 회차의 연결을 놓칠 수 있다.
- A는 native 대화에 실패 보고 본문을 실제로 남겼지만, gateway 최종 상태는 `session_ended_unverified/result_unavailable`다. results.go는 자식 상태가 아직 보고되지 않았으면 부모를 계속 awaiting_children으로 처리하고 본문을 비운다. 실패한 하위 단계와 이미 작성한 부모의 실패 보고를 구별할 필요가 있다.

이 때문에 세 건을 같은 “전송 중 본문 손실”로 해석하면 안 된다. 실패한 Workflow A는 선택 단계에서 막혔으므로 정상적으로 등록된 agentResults 목록과도 범위가 다르다.

수정: 선택 거부처럼 backend 실행 전에 끝난 native turn도, 검증 가능한 identity 범위에서 명시적 실패로 연결한다. 실행 실패, 완료 본문 존재, 부모 수신 여부를 구분하고, terminal 실패를 계속 살아 있는 자식으로 취급하지 않도록 한다. 기존 A2 결과는 원본에서 한 번 회수할 수 있지만 완료 계산을 자동 재실행하지 않는다. 부모에게 확정된 계보·완료/실패 사실을 전달해 “생성 안 됨” 같은 근거 없는 설명을 줄여야 한다. 모델의 자연어 정확성까지 강제 보장했다고 주장하지 않는다.

## 이번 세션에서 확인된 정상 동작과 미검증

| 항목 | 판정 근거 |
|---|---|
| /context 실제 입력 제외 | BEFORE=36623, 세 번 조회 뒤 AFTER=36672, 증가 49. 진단 블록 제외 165개/31요청, 읽기 오류 0. 수천 토큰 조회 본문이 실제 입력에 누적되지 않았다. |
| /context native 표시 | 전체 36.6K 유지. Messages는 26K→28K→30.1K. 항목별 로컬 추정 오차는 여전히 남아 있다. |
| 정확 계수 | backend usage가 있는 완료 생성 67건 모두 일치, mismatch 0. 모든 형식·모든 시점을 보증하지 않는다. |
| model-only/effort-only | Luna/max와 Luna/high 확정 선택, 파일/PDF 결과 본문 확인. |
| fork 재개 | B1 같은 ID에서 Sol/high 유지, 새 PHASE_2 결과 510 확보. |
| 부모 모델 전환 | main 요청은 계속 Astra/low, local command에는 /context 4회만 기록. 이 실행으로 전환 후 유지까지 검증할 수 없다. 선택을 하지 않은 것인지 UI에서 적용되지 않은 것인지는 확정하지 않는다. |
| WebSearch | 정확한 공개 쿼리 1회, Sol/high 검색·생성 경로, 최종 본문 확보. auxiliary 취소 130은 정상 본문 완료와 별도로 남긴다. |
| TaskStop 격리 | Luna 자식 취소와 Terra SURVIVOR 완료. 142 취소 시점에 144 정상 요청이 겹치며 이후 145/147도 성공. |
| Esc 회복 | 계수 중 취소 분류는 정상. 스트리밍 중 취소 뒤 다음 일반 요청은 실패. |
| 압축/종료 | compact controls=0, 정상 종료. 실제 239K/450K 경계 압축은 시험하지 않았다. |

## 왜 기존 확인으로 발견하지 못했는가

1. 마지막 통과 TUI는 계수 중 취소 뒤 회복을 확인했다. native가 부분 텍스트를 재구성한 뒤의 다음 요청은 이번에 새로 드러난 경로다.
2. 중첩 결과 단위 검사는 결과 상태 함수를 직접 진행시킨다. 실제 native 자동 재진입에서 HTTP parent header가 어떻게 오는지까지 확인하지 않는다.
3. Workflow 단일 상속 성공을 명시적 child 선택 지원으로 확장할 수 없다. 현재 Go 테스트의 기대값 자체가 그 좁은 범위다.
4. 자연어 최종 보고, API 정상 종료, result 전달 상태가 서로 다른 증거인데 이를 한 번의 성공으로 묶으면 이번 같은 공백을 놓친다.

`client.verified=true`도 이 버전이 보내는 모든 요청 형식을 지원한다는 증거로 사용할 수 없다. 실제 취소 후 형식이 현재 decoder에서 거부되는 것이 이번 반례다.

이는 회귀 테스트를 더 많이 반복하면 사라지는 문제가 아니다. 서로 다른 취소 시점·계층·선택 경로를 명시적으로 검증해야 한다.

## 다음 진행 순서와 완료 조건

1. **P0 — text decoder 호환성.** null/빈 citations 처리와 unknown field 방어를 함께 검사한다. 실제 TUI에서 계수 중 취소, 첫 텍스트 이후 취소, 도구 인자 생성 중 취소를 분리하고, 각각 다음 프롬프트가 새 backend 완료까지 가는지 확인한다. 정상 답변 뒤 이력 재사용과 count_tokens도 확인한다.
2. **P1 — 자동 재진입의 계보 검증.** 깊이 3에서 A2→A1→A 결과 전달을 끝까지 관측한다. 명시적 SendMessage 재개와 자동 완료 이벤트 재진입을 각각 검증한다. 헤더 누락, 위조 parent, 다른 세션, stale receipt에 대한 정상·부정 검사를 함께 통과시킨다.
3. **P1 — Workflow 선택 구현.** 실행 전 확정 선택을 child와 연결하는 작은 실험부터 한다. 지원 범위를 확인한 뒤 explicit A + inherited B의 병렬 실제 반환을 검증한다. 선택 근거를 얻지 못하면 실행하지 않는 원칙을 유지한다.
4. **P1 — terminal 실패/결과 상태와 진단.** 위 오류의 후속 상태를 바로 정리하고, 안전한 세부 실패 이유를 status에 기록한다. 요청 본문·인증값은 저장하지 않는다. 부모가 실제 확보한 오류 보고와 자식 완료 본문을 구별한다.
5. 위 네 범위가 각각 검증된 뒤 복합 시험을 다시 진행한다. 변경 없는 반복 실행에서 우연히 통과한 결과로 최초 실패를 덮지 않는다. 정확한 binary hash, 처음 실패한 시점, 취소 단계, 회복 요청 완료, 남은 프로세스와 미확보 결과를 함께 판정한다.

이번 진단의 기계적 근거는 `session-facts.json`, `baseline-probes.json`, `session41_diagnosis_test.go.txt`다. baseline probes의 PASS는 **현재 결함과 거부 조건이 재현되었다는 뜻**이며 수리 성공이 아니다. 새 TUI 수리 검증과 개발 바이너리 교체는 아직 수행하지 않았다.
