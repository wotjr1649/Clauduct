# S42 사용자 대화형 실행 검수 — 2026-09-20

판정: **부분 통과. S42 전체 합격 아님.** Esc 이후 회복 두 건, 수동 압축 두 건과 후속 대화, 기존 fork 재개는 관측됐다. 깊이 3의 마지막 자식은 생성되지 않았고, Workflow 정상 실행과 완료 결과 재사용은 수행되지 않았다. 선택 15단계는 첫 거부만 예상된 결과이며, 이어진 일반 회복 프롬프트까지 다시 거부된 것은 회복 검증 실패다.

## 근거와 조사 범위

- 사용자 실행 UUID: `df320410-f4cd-443d-86f6-290ac649f634`.
- 실행 binary SHA-256: `bbc498a694fbfa39c74c9d6f2339d289b34ace6a51660247e0485c58cb94633b`, 제품 commit `34ebcb1608e0dc712a9ae9f887427fdc48a3bcfd`, native `2.1.278`.
- 원본 실행: `D:\AIDEV\clauduct-s36-build\repair-20260919\verification\policy-repair-20260919\interactive-df320410-f4cd-443d-86f6-290ac649f634`.
- 최종 status: 위 실행 폴더의 `tmp\clauduct\status-18872.json`.
- `run.json`, 최종 status, `checkpoints-s42`의 12개 checkpoint, 승인된 profile에 저장된 부모 transcript 411행과 자식 10개 transcript 총 179행을 대조했다. 답변·도구 호출·완료·오류·압축 기록을 검토했으며 비공개 추론과 이미지 원문은 분석 산출물에 복사하지 않았다.
- 모든 checkpoint와 최종 status를 합쳐 **272건 중 121건의 개별 요청 상세**를 확보했다. 나머지 151건은 세션 누계를 사용할 수 있지만 개별 전체 이력을 복원했다고 주장하지 않는다.
- 사용자 확인: 13-A의 동일 프롬프트 네 번 제출·취소는 **사람이 직접 반복한 것**이다. 자동 재시도 결함으로 분류하지 않는다.
- 본 조사는 제품 코드, 배포 binary, native profile, 신뢰 설정을 변경하지 않았다. 추가 backend 실행 없이 기존 실측을 검수했다. 이 폴더의 분석 스크립트·파생 근거·보고서만 작성했다.

[evidence.json](evidence.json)은 요청 번호·시간·판정 근거와 원본 파일 hash를 보관한다. [audit.mjs](audit.mjs)는 이 UUID와 승인 profile, binary hash를 확인한 뒤 같은 분석을 재현한다. 본문에서 `root:N`은 부모 JSONL의 실제 행 번호, `seq N`은 gateway 요청 번호다.

## 단계별 판정

| 단계 | 실제 관측 | 판정과 제한 |
|---|---|---|
| 0 공통 조건 | 기억 표식 `CEDAR-58`, 공개 fixture 기준, REV-A 원장 유지 | 후속 회복 답변과 대조 가능 |
| 1 병렬 원장·깊이 3 | A→A1, B→B1 실행. A2 미생성 | **부분 수행**. 깊이 3와 두 leaf 결과 비교 미검증 |
| 2 모델만·effort만 | Plan+Luna만 지정 → Luna/max. Explore+high만 지정 → Luna/high. 텍스트/PDF 결과 확보 | 이번 입력의 라우팅·결과 전달 관측. 모든 멀티모달 형식 지원의 증거는 아님 |
| 3 Workflow 4→1 | authoring 계약에 요구 옵션이 없어 실행 안 함 | **미수행**. Workflow 자식 요청 0 |
| 4 압축 전 Workflow 회수 | 원본 run ID 없어서 호출 안 함 | **미수행**, 결과 재사용 성공 아님 |
| 5 Astra→Sol | Sol/high 일반 응답, 전환만으로 강제 압축 없음 | 이번 기준 미만 입력에서 관측 |
| 6 첫 수동 압축 | Sol/high 압축 완료, 기억·원장·B1 ID 보존, 일반 응답 성공 | 통과한 관측 범위. 소요 약 196초 |
| 7 기존 손자 두 명 재개 | B1 같은 ID에 SendMessage 1회, REV-B 새 결과. A2 ID 없음 | **부분 수행**. 동시 두 손자 재개 미검증 |
| 8 모델 선택 직후 수동 압축 | Terra 선택 후 Sol/high로 압축, 다음 일반 요청 Terra/medium | 기존 모델 압축 후 전환 관측. 두 번째 명령은 사용자 지정 설명 없는 `/compact` |
| 9 압축 후 Workflow 회수 | 원본 run ID 없음 | **미수행** |
| 10 Luna 부모·Sol 검색 자식 | Sol/high WebSearch 1회, search transport 1 attempt/0 retries | 검색 라우팅·완료 본문 관측 |
| 11 `/context all` 반복 | 마지막 세 조회의 총량 30.7k로 동일. Messages는 증가 | 전송 전 정리 관측, native 카테고리 표시 한계 잔존 |
| 12 취소 격리 | Luna 자식 TaskStop, 겹쳐 실행 중이던 Terra 자식은 완료 | 이번 취소 격리 관측 |
| 13-A Esc 회복 | 네 번 직접 전송·취소 후 `S42_EARLY_RECOVERED` 정상 응답 | 회복 관측. **계수 도중 취소는 이번에 관측 안 됨** |
| 13-B 본문 중 Esc | 부분 본문 후 취소, 후속 Read 호출과 `S42_TEXT_RECOVERED` 정상 응답 | 회복 관측. Read는 native의 기존 결과 재사용 응답 |
| 14 Astra 복귀·새 Plan | model/effort 생략 Plan → Astra/medium, 1461 결과와 부모 수신 | 관측된 정상 수행 |
| 선택 15 거부 후 회복 | 알 수 없는 Workflow run 거부 후, 일반 회복 요청도 같은 분류로 거부 | **거부는 예상대로, 회복은 실패** |

`process=SUCCESS`, `exit=0`, `nativeReaped=true`, `watchdogForced=false`는 정상 종료 근거다. 기능 전체 합격이나 모든 하위 프로세스가 없다는 별도 운영체제 검사 결과는 아니다. `acceptance=not_assessed`는 이를 전체 합격으로 과장하지 않는다.

## 15단계: 거부 뒤 대화 회복 실패

실제 순서는 다음과 같다.

1. `root:399`: `wf_s42_missing`을 사용한 의도적 결과 회수 요청.
2. `seq 270`: 정확 계수 40,701 완료 후 새 backend 응답에서 Workflow 회수 호출 감지. `workflow_recovery_request`까지만 확인하고 `WORKFLOW_RECOVERY_UNVERIFIED`, HTTP 400으로 거부.
3. `root:401`: native가 합성한 API 오류 메시지. 실행된 Workflow 도구 호출/결과는 없다.
4. `root:404`: 도구 없이 기억·revision을 보고하고 실패한 Workflow를 다시 실행하지 말라는 **새로운** 회복 프롬프트.
5. `seq 272`: 입력 40,757의 별도 요청에서도 backend가 Workflow 회수 호출을 생성하여 동일 분류로 거부. `root:406`에 두 번째 API 오류. `S42_REFUSAL_RECOVERED`는 없다.

두 요청 모두 backend event 17개, tool argument delta 11개, text delta 0개다. 새로운 요청·새 입력·새 backend 응답이 확인되므로 연결 전체 정지나 동일 status를 두 번 읽은 현상으로 볼 수 없다. 두 번째 요청 사이에 성공한 `seq 271`은 auxiliary 요청으로, 부모 회복 성공을 대신하지 않는다.

코드 경로는 `go/internal/gateway/messages.go`의 `PrepareToolCall` → `delegations.prepare` → `workflow_selection.go`의 Workflow 처리 → `workflow_recovery.go`의 회수 검증이다. 검증 오류가 응답 전송 전에 반환되면 `messages.go`의 `fail`이 해당 모델 응답 전체를 HTTP 오류로 돌린다. 금지된 Workflow를 native에 전달하지 않은 것은 올바르다. 하지만 native에 실제 `tool_use`/실패 `tool_result`가 남지 않는 이 경계에서, 후속 일반 대화가 회복된다는 보장은 확보되지 않았다.

확정하지 못한 부분도 있다. 거부된 두 번째 도구 인자의 원문과 요청의 `tool_choice`가 근거에 남아 있지 않다. 따라서 두 번째 `resumeFromRunId`가 정확히 같은 값이었다거나, 모델의 지시 불이행·native의 도구 강제·오류 뒤 이력 표현 중 어느 하나가 단독 원인이라고 단정하지 않는다. 확인된 것은 **새 회복 요청에서도 Workflow 회수 호출이 다시 만들어져 차단됐다는 사실**이다.

수정 우선순위는 이 경계다. 금지된 실행은 계속 막으면서, 해당 거부를 native가 이해하는 도구 실패/turn 종료로 남기고 다음 일반 turn으로 넘어가는 경로를 실증해야 한다. native의 기존 도구 거부 기능으로 이를 전달할 수 있는지 먼저 확인한다. 실패를 성공으로 위장하거나 알 수 없는 run을 허용하는 방식은 해결책이 아니다. 원인 확인용 진단은 요청·turn·call 연결, 도구 이름, 거부 단계와 이유, 인자 hash 및 허용한 비밀 없는 분류 필드로 제한한다. 전체 프롬프트나 도구 인자를 무차별 저장할 필요는 없다.

재검증은 `예상 거부 → 도구 없는 일반 답변 → 정상 새 Agent`가 같은 세션에서 성립하는지 확인해야 한다. Workflow라는 단어가 없는 회복 문장과 명시적으로 재실행을 금지한 회복 문장을 나누어 원인을 구분한다. 이번 마지막 상태에는 거부 뒤 성공한 일반 turn이 없다.

## Agent와 Workflow: 실행 경로와 모델에게 보이는 계약의 공백

실제 최초 계보는 다음과 같다.

```text
ROOT
├─ A  a6a9877d1560b601c  Terra/medium
│  └─ A1 a1c1b93dc55bf4f40  Terra/medium
│     └─ A2 Plan: 생성되지 않음
└─ B  a0a64e2419a0a1aca  Sol/high
   └─ B1 a4607dbae2a1a6689  fork, Sol/high
```

A1은 자신의 도구 계약에 Plan이 열거되지 않았다고 보고하며 A2를 만들지 않았다. 당시 A1에게 실제 제공된 도구 역할 schema 원문은 저장되지 않았으므로, Plan의 실행 자체가 불가능했다는 결론까지 확대할 수 없다. 확정된 사실은 **A2 호출이 없고 결과도 없다는 것**이다. A의 A1 호출에서는 명시하도록 요청한 `subagent_type`도 생략됐다. 실제 native 기본 역할과 모델·effort 상속은 status로 확인되지만 프롬프트 준수와는 별개다.

기존 `go/internal/app/delegation_adapter_test.go`의 `TestNativeNestedAgentKeepsDelegationModelAndEffort`는 fixture가 `ToolSearch`와 `Agent(subagent_type=Plan)` 호출을 직접 구성한다. 이 테스트는 구성된 호출의 native 실행·상속을 확인할 수 있으나 **실제 모델이 자식의 도구 설명에서 Plan을 발견하고 자율적으로 호출하는 과정**을 검증하지 않는다. 기존 테스트 통과를 그 범위까지 확장했다면 검증 해석이 잘못된 것이다.

B1은 실제로 같은 ID에 재개됐고 REV-B의 고유 event 19개, 실재고 76, 예약 15, 가용 61, 가중 가용 108을 새 본문으로 반환했다. 추가된 I08은 중복으로 제외됐다. 과거 답변 재출력만 한 것이 아니다. 다만 A2가 없어 두 손자의 동시 재개·상호 검산은 성립하지 않았다.

Workflow는 S42 프롬프트 준비의 결함이 확인됐다. 모델이 실제로 읽은 authoring 본문(`root:95`)에는 다음 signature가 있었다.

```ts
agent(prompt: string, opts?: {
  label?: string; phase?: string; schema?: object; model?: string;
  effort?: string; isolation?: 'worktree'; agentType?: string;
}): Promise<any>
```

S42가 요구한 `tools: []`, `maxTurns: 4`는 이 문서에 없었다. 계약을 확인하지 못하면 실행하지 말라는 지시와 결합해 모델이 중단했다. 옵션이 runtime에 실제로 없는지 여부는 문서의 부재만으로 확정할 수 없다. **제공한 검증 프롬프트의 옵션과 실제 모델이 읽는 계약을 함께 검증하지 않은 준비상의 책임이 있다.** 과거 제한된 실험에서 같은 형태가 실행됐다는 사실만으로 해당 제한의 강제 적용이나 모델에게 보이는 계약의 정확성까지 증명되지는 않는다.

보완은 모델에게 보이는 역할 목록·Workflow 옵션·조건별 지원 상태를 실제 적용 조건과 맞추는 데 집중한다. 검증되지 않은 옵션을 지원한다고 문서에만 추가하거나, 없는 계약을 무시하도록 프롬프트를 바꾸면 안 된다. 옵션이 없어도 안전하게 수행 가능한 별도 최소 시나리오를 쓴다면 `tools`/`maxTurns` 제한 검증을 했다고 주장하지 않는다. 이번에 실제 생성된 자식은 총 10개이며, 계획한 16개에서 A2 1개와 Workflow 5개가 빠졌다.

## 수동 압축의 모델·효과·지연

| 항목 | 첫 압축, seq 153 | 둘째 압축, seq 167 |
|---|---:|---:|
| 실제 모델·effort | Sol/high | Sol/high |
| 정확 입력 계수 = backend usage | 33,704 | 28,723 |
| 계수 시간 | 1.176초 | 0.932초 |
| gateway 요청 전체 시간 | 195.983초 | 230.506초 |
| 첫 client byte까지 | 69.538초 | 40.061초 |
| 첫 byte 이후 완료까지 | 126.445초 | 190.445초 |
| native compact 전체 시간 | 196.110초 | 230.636초 |
| native 추정 pre → post | 36,080 → 13,546 | 34,001 → 16,116 |
| 저장된 요약 문자 수 | 16,253 | 23,491 |
| 다음 일반 요청 | seq 156, Sol/high, 입력 24,347 | seq 170, Terra/medium, 입력 28,321 |

두 건 모두 native `trigger=manual`이고 정상 완료됐다. `compaction_controls=0`은 gateway의 자동 제어 전환이 없었다는 뜻이지, 수동 압축이 없었다는 뜻이 아니다. 입력이 239K/450K에 가깝지 않았으므로 자동 압축 경계의 성공도 이번 실행으로 판정할 수 없다.

둘째 압축 직전의 실제 순서(UTC)는 `16:25:48.273 Terra/medium 선택` → `16:25:52.430 /compact`다. 두 명령 사이의 일반 프롬프트·일반 생성 요청은 기록에 없다. 사용자의 “일반 프롬프트를 보냈다”는 회상과 이 구간의 순서가 다르므로, 판정에는 저장된 순서를 사용한다. 둘째는 설명 없는 `/compact`였으며 S42 문서의 상세 보존 문장을 그대로 실행한 경우와 구분한다.

`go/internal/gateway/context.go`는 수동 압축일 때 마지막으로 확정된 기존 대화 모델·effort를 사용하고, 압축 뒤 새 일반 요청에서 선택한 모델을 적용한다. 따라서 둘째 압축의 Sol/high와 다음 응답의 Terra/medium은 기존 모델로 압축한 뒤 전환하는 정책의 관측 결과다.

3분 이상 걸린 주원인을 정확 계수로 돌릴 수 없다. 계수는 전체 시간의 약 0.6%, 0.4%에 불과하다. 긴 요약 생성·전달과 첫 출력 전 대기가 대부분을 차지했다. 두 번째 요약은 첫 번째보다 길다. 다만 현재 기록만으로 첫 출력 전 시간을 backend 대기·추론·gateway 처리에 정확하게 나눌 수는 없다. text delta 수는 출력 토큰 수가 아니다.

개선은 원장 원문·규칙·ID·선택값·완료 결과·누락 사실처럼 재개에 필요한 상태를 보존하면서 반복된 장문의 보고/미실행 스크립트 설명을 줄이는 압축 형식부터 평가한다. 사용자가 정한 effort를 임의로 낮추거나 정확 계수를 제거할 이유는 없다. 압축 전후 payload 구성이 다르므로 native 추정 post와 다음 backend 입력 차이를 곧바로 계수 오류 또는 압축률로 해석하지 않는다. 동일 구성 기준의 실제 입력 크기와 기억·재개 결과를 함께 측정해야 한다.

## Esc와 취소: 회복이 실제로 관측된 범위

| 구분 | 요청 번호 | 관측 |
|---|---|---|
| auxiliary 취소 | 60, 80, 163 | native 보조 요청 취소. 사람의 Esc 또는 retry로 단정할 근거 없음 |
| 지정 자식 TaskStop | 239 | 취소 대상 종료, 별도 Terra 자식은 계속 실행·완료 |
| 13-A 직접 반복 | 247, 249, 251, 253 | 앞의 세 건은 계수 뒤 본문 이전 취소. 네 번째는 text delta 26개 뒤 취소 |
| 13-B 본문 중 취소 | 257 | text delta 1,098개 뒤 취소 |

13-A 뒤 `root:351`에서 `CEDAR-58 / REV-B / 61`과 `S42_EARLY_RECOVERED`가 실제 응답했다. 최종 요약의 사후 주장만 읽은 것이 아니다. 13-B 뒤 `root:372`에서 `CEDAR-58 / REV-B / 108`, fixture 값과 `S42_TEXT_RECOVERED`가 응답했다. 그 뒤 새 Plan도 정상 완료했다. 이전 세션의 `TEXT_FIELDS`로 모든 다음 요청이 막히던 양상은 이번 Esc 이후에는 나타나지 않았다.

두 번째 회복의 Read 호출은 실행됐지만 `root:370`의 native 응답은 파일이 바뀌지 않았으므로 기존 Read 결과를 참조하라는 내용이다. 새로운 파일 본문을 디스크에서 다시 읽어 받은 검증으로 과장하지 않는다. 이번 13-A에는 `stage=count` 취소가 없으므로 정확 계수 중 취소/회복은 별도 미검증이다. 네 번 반복의 출처는 사용자가 직접 전송·취소했다고 확인했다.

TaskStop 대상과 생존 자식은 실제 요청 시간이 겹쳤다. 생존 Terra 자식의 요청은 대상 Luna 자식의 시작 13ms 뒤에 시작했으며 대상 취소 뒤에도 후속 응답을 완료했다. 단순 순차 실행 후 각각 성공한 사례와 구분된다.

## `/context`, 계수 누계, status 해석

마지막 연속 세 조회의 총량은 모두 `30.7k/500k`다. 그러나 native Messages 카테고리는 `21k → 23k → 25.1k`로 증가했다. 따라서 native 표시까지 완전히 일관되게 수정됐다고 판정할 수 없다. gateway는 native transcript provenance를 바탕으로 조회 표시 블록을 전송 전에 제거한 사실을 보고한다: 최종 `removedBlocks=225`, `requests=35`, `unreadable=0`. 225는 여러 요청에서 반복 제거한 누계이지 서로 다른 조회 225회가 아니다. `provenReports=3`을 세션 전체 조회 총수로 해석해서도 안 된다. 저장된 root에는 압축 이전 조회까지 5개가 있다.

사용자 관측의 첫 약 5초·이후 즉시 응답은 warmup과 캐시 사용이라는 관측과 부합한다. 다만 5초의 내부 구간별 원인을 이 기록만으로 모두 계측했다고 주장하지 않는다. native의 공통 500K 표시와 카테고리 추정값은 gateway의 모델별 실제 압축 기준과 별개다. gateway 전송 경로의 정리 관측과 native 화면 표시 잔여 한계를 구분해야 한다.

backend usage가 존재한 **87건은 정확 계수와 전부 일치**, 불일치 0건이다. 성공한 generation 85건과 수동 compaction 2건에 해당한다. 취소·거부된 나머지 요청까지 실제 backend usage와 일치했다고 확대하지 않는다. `count_tokens` 141건은 모두 성공했다.

`attempts=330`은 330번의 일반 생성 재시도를 뜻하지 않는다. status 누계는 다음과 같이 정확히 맞는다.

```text
98 inference attempts (generation 96 + compaction 2)
+ 140 fresh backend counts from count_tokens (141 requests, one shared)
+ 92 fresh preflight counts (98 preflights - 6 cache hits)
= 330 attempts
```

count connection의 dials 116 + reused 116 = 232도 위 backend count 140 + 92와 일치한다. 이 실행에서는 계수 호출의 존재가 전체 시도 수의 상당 부분을 설명한다.

추가로 status의 용어 한계가 있다. `agentContexts.phase=ready`는 압축 정책 상태이며 마지막 대화 turn의 성공을 뜻하지 않는다. `nativeToolFailures=0`은 실행된 native 도구 실패가 없다는 뜻이고 A2/Workflow 미실행이나 gateway의 도구 전달 전 거부를 검증하지 않는다. `features.generation.lastRequest=271`은 `features.go`가 완료 관측 순서로 갱신한 값으로, seq 272가 없었다는 뜻이 아니다. 동시 요청에서 최대 요청 번호와 마지막 완료 번호를 구분하는 명칭/필드 보완이 유용하다.

## 다음 수정·검증 순서

1. **Workflow 거부 뒤 일반 turn 회복**: 현재 두 번째 거부의 정확한 도구 선택 입력을 구분할 최소 진단을 확보하고, native의 기존 도구 거부/결과 전달 방식으로 실행 금지와 대화 회복을 함께 보장하는 경로를 검증한다. 성공 응답 위장이나 미검증 run 허용은 금지한다.
2. **모델에게 제공되는 계약과 실제 기능 조건 정합성**: root뿐 아니라 자식 깊이별 역할 발견, Workflow 옵션 제공·강제 적용을 실제 TUI에서 확인한다. 이후 A→A1→A2, 두 기존 leaf 동시 재개, Workflow 4→1과 완료 결과 재사용을 각각 다시 수행한다. S42의 정상 구간 전체를 반복할 필요는 없다.
3. **압축 지연과 요약 크기**: 기존 모델·effort 보존 조건을 유지한 채 요약 형식과 필요한 상태 범위를 정리하고, 기억·같은 ID 재개·실제 입력 크기·시간을 함께 비교한다. 이번 3~4분은 기능 성공과 별도로 남은 성능 문제다.
4. **검수 관측 보완**: 제품의 요청 필수 조건과 시험 계획의 예정/실제 단계를 분리해 기록한다. 실행하지 않은 작업을 성공으로 세지 않으며, native 표시 추정·gateway 정확 계수·마지막 turn 결과·정책 상태를 명확하게 구분한다.

이 버전이 명시적으로 `unsupported/not_implemented`로 보고하는 `empty_reply_wait`, `workflow_resume`는 이번에 지원 기능으로 바뀌지 않았다. 완료된 Workflow 결과 재사용과 실패/중단 Workflow의 실행 재개는 서로 다른 기능이다. 이번 Workflow 0건으로 둘 중 어느 것도 추가 입증되지 않았다.

단 한 번의 복합 실행으로 무결점·모든 취소 시점·모든 모델/멀티모달 입력·비플래키를 선언할 수 없다. 이번 실행은 기존 테스트가 놓친 계약 노출과 거부 후 회복 공백을 드러냈고, 동시에 Esc 회복·모델 전환·수동 압축의 실제 성공 범위를 넓힌 근거다.

## 산출물 검수

`node --check verification/session42-analysis-20260920/audit.mjs`와 실제 audit 실행을 완료했다. 회복 표식은 해당 회복 프롬프트와 다음 사용자 단계 사이의 실제 답변에 한정해 판정한다. 따라서 마지막 모델 요약이 성공을 주장한 것만으로 합격하지 않는다. 제품 regression/TUI를 새로 실행한 것은 아니며, 이번 검수 대상은 사용자가 실행한 S42의 원본 근거다. 산출물 검수 완료가 미수행·실패한 S42 단계의 통과를 의미하지 않는다.
