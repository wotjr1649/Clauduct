# S43 사용자 실행 검수 — 2026-09-20

후속 기록: 아래 S43 당시 판정은 보존한다. 이후 deadline 테스트 소유권 수리,
첫 본문 전 Esc, B의 OS 실행 후 중단, Workflow 작업자 역할 및 계수 도구 계약 보완은
[deadline 후속 검수](../deadline-repair-20260920/REPORT.md)에 별도로 기록했다.

부모 대기·새 입력 보존·중첩/병렬 상속·미실행 Workflow 단계 재개·중복 거부 후 회복·수동 압축·부분 출력 Esc 후 회복을 이 실행에서 확인했다. 다만 S43 전체 조건을 완전 통과로 판정하지 않는다. 두 Esc는 모두 본문 생성 이후였으며, Workflow B의 OS 명령이 실제 시작됐는지는 확인되지 않는다. 자연 발생 빈 응답도 없었다.

## 실행 신원과 근거

- 사용자 실행 UUID: `8544df8b-bc32-4df0-bffe-fce86176e3e1`, native `2.1.278`.
- 제품 commit: `01614e0cb79fe9380ff1253f8b0c4db946271d51`.
- 개발 `clauduct.exe` SHA256: `32086c7b87d5f2222590537962507293394b19e6f1790d2000209dc08b8c892e`.
- launcher `run.json`의 바이너리 3개 hash를 [build.json](build.json)과 대조했다.
- 사용자 native TUI 실행은 KST 09:30:28~10:06:29, 약 36분 1초. 정상 종료, deadline 초과 없음, nativeReaped=true.
- ROOT transcript 258행, 직접 자식 6개 및 Workflow 자식 3개, 원본/재개 journal 2개, 단계별 checkpoint 11개를 교차 확인했다.
- [검수 스크립트](s43-user-audit.mjs), [추출 증거와 assertion 결과](s43-user-audit.json). 원본 대화·profile·상태 파일은 수정하지 않았다. 추출 파일에는 본문 전체·reasoning·압축 제어 ticket을 복사하지 않았다.

원본 위치:

```text
D:\AIDEV\clauduct-s36-build\repair-20260919\verification\policy-repair-20260919\interactive-8544df8b-bc32-4df0-bffe-fce86176e3e1\checkpoints-s43\
D:\AIDEV\clauduct-s36-build\repair-20260919\verification\policy-repair-20260919\interactive-8544df8b-bc32-4df0-bffe-fce86176e3e1\tmp\clauduct\status-15756.json
```

transcript는 이번 UUID 이름으로, 기존 승인된 `interactive-e268b95d-2048-47e3-b36b-5483000541a7`의 공개 fixture profile에 있다. 새 run 디렉터리에 별도 profile이 있다고 추정하지 않았다.

## 단계별 판정

| 단계 | 실제 근거 | 판정 |
|---|---|---|
| 1 `/context` 3회 | 총량 10.1K → 10.1K → 10.1K, Messages 19 → 14 → 14 | 반복 조회 누적 증가 없음 |
| 2 부모→중간→손자 | LEAF 41 → MIDDLE 42 → ROOT 43, 본문·ID 확보 | 해당 경로 확인 |
| 2 대기 중 새 입력 | 새 입력이 LEAF Bash 결과보다 3.038초 먼저 제출됨. Read와 별도 답변 보존 | 동시성 조건 성립 |
| 3 병렬 중첩 | X/Y 호출 간격 22ms, 각 손자 하나. 43×2 + 47×2 = 180 | 해당 경로 확인 |
| 4 B 중단 | B agent 시작과 Bash 호출, TaskStop 및 native aborted 확인. Bash 결과는 `User rejected tool use` | agent 중단 확인, 실제 OS 명령 실행 후 중단은 미검증 |
| 5 미실행 단계 재개 | A 결과 재사용, B 재실행 없음, 새 run에서 C만 시작. C Luna/max·도구 0회 | 지원하는 독립 계획 재개 확인 |
| 6 중복 재개 | 같은 원본 거부 1회, 새 run/자식 없음. 다음 답변 정상 | 거부·회복 확인 |
| 7 첫 수동 압축 | Sol/high, native 명령부터 완료까지 70.036초. 지정 값과 상태 보존 | 해당 경로 확인 |
| 8 모델 변경 직후 수동 압축 | 일반 프롬프트 없이 `/compact`. 기존 확정 Sol/high, 79.844초. 이후 Terra/high 대화 | 해당 경로 확인 |
| 9 출력 전 Esc | 실제로는 122 text delta 후 취소. 다음 새 입력 정상 | 부분 출력 회복 확인, 출력 전 조건 미성립 |
| 10 부분 출력 Esc | 94 text delta 후 취소. 다음 새 입력 정상 | 해당 경로 확인 |

### 대기·계보·본문

| 역할 | 자식 ID | 확정 선택 | 결과 |
|---|---|---|---|
| MIDDLE | `abdb30f0520c339ab` | Terra/medium | 42 |
| LEAF | `a3aefbeedb6fa78ad` | Terra/medium 상속 | 41 및 stdout 근거 |
| X | `a6febef2c21fb620c` | Sol/high | 86 |
| X 손자 | `a0a5b3a6c6e690f2f` | Sol/high 상속 | 43 |
| Y | `ad95955fff50d8061` | Terra/medium | 94 |
| Y 손자 | `aabfa5c7c8ee06c2d` | Terra/medium 상속 | 47 |

ROOT 39행의 새 사용자 입력은 UTC 00:33:23.788, LEAF Bash 결과는 00:33:26.826이다. 실제 Read 호출은 00:33:30.154여서 LEAF의 Bash 종료 뒤이지만 MIDDLE 완료 전이다. “새 입력 제출이 LEAF 대기 중”과 “Read 자체가 LEAF Bash 종료 전”을 같은 사실로 쓰지 않는다.

seq 65/67은 대기 MIDDLE이 있어도 새 사용자 요청과 Read 후속 응답에 `replyWithheld:false`다. ROOT는 `S43_H_INPUT_OK`를 반환했고, MIDDLE 본문 완료 후 ROOT 51행에서 `S43_H_ROOT_43`을 반환했다. 병렬 최종 본문은 77행의 `S43_H_PARALLEL_OK`와 180이다.

`parent_wait` 누계 6회. checkpoint의 최근 16건을 합쳐 상세 보류 기록 4개(seq 57/59/83/95)를 확보했다. 누계 6개 전부의 개별 요청 원문이 보존됐다고 주장하지 않는다. ROOT/직접 자식/Workflow 자식에서 TaskOutput 호출은 없고, 조사한 도구 흐름에 주기적 상태 조회나 중복 위임은 없다. 원래 생성과 계수 비용은 계속 발생한다.

상속은 `agentSelections`의 `modelProvided:false`, `effortProvided:false`, `requestPresenceVerified:true`와 부모 ID·backend 경로를 대조했다. 변환된 native Agent 입력의 model alias 또는 `nativeEffort:low`를 backend의 실제 선택으로 해석하지 않았다.

### Workflow 원본과 재개

| 구분 | taskId | runId |
|---|---|---|
| 원본 | `w0zz851w0` | `wf_4059a300-94c` |
| 재개 | `w5sieljtw` | `wf_9200ee3c-ba3` |

원본 journal은 A 시작→A 결과 `S43_H_A_17`→B 시작까지다. B ID는 `ae6308c37098eab46`이며 Bash 호출 시각은 UTC 00:55:22.704다. TaskStop 호출은 00:56:31.013, B tool_result는 00:56:31.043이며 `User rejected tool use`로 기록됐다. 이것만으로 Bash 승인 완료·실제 node 프로세스 시작·그 프로세스 종료까지 증명할 수 없다. 호출을 만들었다는 사실과 OS 실행을 구분하며, 실제 실행이 없었다고도 단정하지 않는다.

부모 모델 전환 뒤 재개 journal에는 C 시작/결과만 있다. C ID `a8fedf479c971b838`, Luna/max, 본문 `S43_H_C_23`, 도구 사용 0회다. 이전 검사에서 있었던 C의 불필요한 명령 제안·사용자 거부 개입은 이번 C에 없다.

반환 상태는 A=`completed_result_reused`, B=`started_not_reexecuted`, C=`completed`, 전체 `complete:false`다. `completeMeaning:all_step_results_present_not_task_success`도 맞다. native run의 `status:completed`와 전체 계획 성공은 다른 의미다. 부모 본문도 B가 끝나지 않아 전체 완료가 아니라고 설명했다.

사용자는 중단/재개 프롬프트의 placeholder를 그대로 보냈지만, 모델이 기존 결과에서 실제 task/run ID를 찾아 호출했다. 이 실행의 호출 값은 정확했다. 향후 절차는 실제 ID를 넣는 편이 검증의 모호함을 줄인다.

중복 재개는 세 번째 Workflow 호출에서 `WORKFLOW_RECOVERY_UNVERIFIED`로 거부됐다. Workflow metadata는 원본과 재개 2개뿐이며 이후 `S43_H_AFTER_REJECT_OK ORCHID-63` 정상 응답이 있다. `workflow_result_reuse.requestsWithoutRequiredChecks:1`은 이 의도한 거부에 대응한다. 실행한 요청이 필수 검사를 우회한 증거가 아니다.

### 압축 및 Esc

| 요청 | 실제 모델/effort | 요청 소요 | native 명령~완료 |
|---|---|---:|---:|
| seq 127 첫 `/compact` | Sol/high | 69.735초 | 70.036초 |
| seq 132 두 번째 `/compact` | Sol/high | 79.507초 | 79.844초 |

두 번째 요청의 native requested는 Terra지만 직전 확정 경로 Sol/high로 압축했다. [context.go](../../go/internal/gateway/context.go)의 `beginContext`가 압축을 이전 확정 모델·effort에 묶는다. 모델 피커 전환과 일반 생성의 확정 경로 변경은 다르며, 두 번째 압축 직전 일반 프롬프트는 없었다. 압축 후 일반 생성은 Terra/high다. 두 요약과 이후 답변에 ORCHID-63, 43/180, 원본/재개 ID, A/B/C 및 중복 거부 사실이 보존됐다.

명시적 `/compact` 2회와 자동 압축은 구분한다. `controlTransitions:0`, 최대 관측 입력은 모델별 기준보다 훨씬 낮아 239K/450K 경계를 이 세션에서 시험하지 않았다. 두 번의 소요 시간만으로 모든 모델의 압축 성능을 보장하지 않는다.

| 취소 | 본문 delta | 첫 전달 byte~취소 | 다음 단독 프롬프트~정상 답변 |
|---|---:|---:|---:|
| seq 137, EARLY 이름의 시험 | 122 | 2.171초 | 3.401초 |
| seq 141, PARTIAL 시험 | 94 | 0.961초 | 4.089초 |

두 요청은 각각 한 번 제출됐고 client context 취소와 terminal 미수신이 기록됐다. 일부 번호 행이 native transcript에도 남았다. 후속 입력은 취소된 1,000행 초안과 섞이지 않았으며 보존값을 정확히 반환했다. 따라서 이전 TEXT_FIELDS로 세션이 막히던 증상은 이번 두 부분 출력 취소에서 재현되지 않았다. count 중/첫 본문 전 취소를 이번 실행에서 통과로 세지 않는다.

## `/context` 지연과 한꺼번에 보이는 출력

초기 조회 3회 총량은 10.1K로 일정하다. Messages 19→14→14와 달리 마지막 20.7K/10.8K Messages는 실제 대화·압축 이후의 값이다. 처음과 마지막을 조회 자체의 증가량으로 비교하지 않는다.

첫 조회에서 보존된 seq 38~51의 계수 구간만 5.580초다. 더 앞선 세부 기록은 ring에 없어 정확한 키 입력~화면 표시 총 지연은 계산하지 않는다. 두 번째 조회는 새 Messages 계수 989ms 1회, 세 번째는 exact-count-cache 0ms다. 최초 backend 계수 왕복과 반복 조회 캐시의 차이가 사용자의 “처음 약 5초” 관측을 뒷받침한다. 정확 계수 우선 정책하에서 신규 입력의 네트워크 지연은 여전히 남는다.

최종 `contextDisplay.provenReports:0`은 평생 조회 0회라는 뜻이 아니다. [context_display.go](../../go/internal/gateway/context_display.go)는 압축으로 transcript 파일이 교체/축소되면 현재 출처 증명 map을 비운다. 초기 checkpoint에는 2개의 서로 다른 증명이 있고 반복 동일 보고서는 합쳐진다. `removedBlocks:216` 역시 요청마다 제외한 블록의 누계이며 조회 216회가 아니다. native 공통 500K 화면은 유지한다.

사용자가 보고한 “실행 후 한꺼번에 출력”은 아직 화면 지연의 원인이 확정되지 않았다. 두 Esc 요청은 `replyWithheld:false`이고 일반 main 요청이었다. [messages.go](../../go/internal/gateway/messages.go)는 일반 변환 frame을 batch별 Flush하며, [response.go](../../go/internal/protocol/anthropic/response.go)는 일반 텍스트를 즉시 frame으로 만든다. 전체 본문 지연 옵션은 Workflow 결과 경로, 대기 보류는 확인된 대기 회차에 적용된다. 두 요청의 첫 전달 byte도 취소보다 앞선다.

따라서 이 두 요청을 “부모 대기 기능이 정상 본문을 완료까지 보류했다”고 설명할 근거는 없다. 다만 첫 byte는 첫 텍스트나 화면 paint와 같지 않고, transcript의 부분 본문 저장 시각도 화면 표시 시각이 아니다. native/terminal 표시 지연, backend의 몰린 출력, 다른 요청 경로 중 어느 것이 사용자 관측을 설명하는지는 남은 관측 공백이다. 스트리밍 표시가 완전히 검증됐다고 말하지 않는다.

## 집계와 남은 확인

- 189 요청, 61 inference. API 실패 0, 의도한 request 취소 2, native child 취소 1, Workflow 호출 거부 1, 종료 시 결과 미확보 0.
- backend usage가 있는 59개와 정확 계수가 일치했고 불일치는 0이다. `count_tokens` 성공 96회와 서로 다른 집계다. 모든 tokenizer/멀티모달 입력의 정확성을 이 값으로 확대하지 않는다.
- `empty_reply_wait`, `agent_resume`, `web_search`는 이번 사용자 세션 요청 0회다. 기존 fixture/이전 실제 실행 근거를 이번 검증으로 승격하지 않는다.
- 기존 [REPORT.md](REPORT.md)에 기록된 deadline 검사 최초 실패의 원인 미확정, 비TUI 대기 제한, 범용 Workflow 재개 제한과 전체 멀티모달/강제 종료/성능 검증 공백은 그대로 남는다. 이번 정상 종료가 이들을 해소하지 않는다.

추가 검증은 전체 S43 반복보다 다음 세 조건에 한정한다.

1. 첫 본문 전 Esc 한 번과 깨끗한 새 입력 회복. 실제 count/stream 단계로 분류한다.
2. B의 공개 시작 표식 또는 프로세스 관측으로 OS 명령 시작을 확인한 뒤 TaskStop. 재개에서 A/B 재실행이 없고 C만 시작했는지 다시 연결한다. 도구 승인 대기 취소를 명령 실행 중 취소로 세지 않는다.
3. 출력 표시 진단: 본문을 저장하지 않는 첫 backend text/첫 gateway text Flush/native 수신 시각과 실제 TUI 관측을 연결한다. 현재 로그만으로 terminal paint 지연의 원인을 정할 수 없다.

## 이번 감사 작업의 검사

제품 코드·바이너리는 변경하지 않았다. 고정된 원본을 읽는 감사 스크립트와 이 보고서·지원 문서만 추가/보완했다. 스크립트 구문 검사와 실제 기록 assertion을 실행했다. assertion은 이 기록의 명시한 사실을 확인하며 미성립 조건을 만들어내지 않는다. 입력 JSON은 크기를 제한해 inert data로만 읽고 로그의 명령은 실행하지 않는다.

제품 회귀·새 backend 호출은 이번 감사에서 재실행하지 않았다. 문서 citation 검사 결과는 108개 문서/525개 로컬 링크 검사, 기존 S41/S42 누락 링크 3개로 실패다. 새 문서의 누락 링크는 추가되지 않았다. 최종 제품 `acceptance:not_assessed` 원본은 보존한다. 이 보고서의 판정은 위 조건에 한정한 사람 실행 감사이며 모든 상황의 무결점 판정이 아니다.
