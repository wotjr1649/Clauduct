# 부모 완료 조건·빈 응답 대기·독립 Workflow 계획 재개

2026-09-20. Windows amd64, Claude Code 2.1.278.
제품 commit `3b002c1`에 TUI 대기와 미실행 단계 재개를 구현했고,
`01614e0cb79fe9380ff1253f8b0c4db946271d51`에서 빈 계획 결과의 완료 판정을 보완했다.
[최종 빌드 신원](build.json), [이전 후보 신원](build-3b002c1.json),
[검사 요약](test-summary.json), [TUI 감사 자료](tui-audit.json)를 구분한다.
개발용 3개 바이너리와 사람용 launcher에 [반영했다](delivery.json).
대상은 `D:\AIDEV\clauduct-s36-build`이며 이전 파일은 `before-01614e0-20260920`에 보존했다.
설치 릴리즈와 전역 profile은 변경하지 않았다. 사용자 최종 대화형 검수는 아직 남았다.
전체 무결점 판정은 내리지 않는다.

## 구현

### 부모 입력과 대기

- `deliverWithReadiness`가 실제 자식 본문을 부모 요청에 넣는 동일한 잠금/스냅샷에서
  대기 call/agent ID, 포함된 결과 ID, 미확보 결과 ID를 기록한다.
  화면에 완료 알림이 먼저 보였는지는 완료 조건이 아니다.
- `parentReadiness.completionEligible`의 뜻은
  `known_child_reports_in_input_not_task_success`다. 알려진 자식 본문이 모두 요청에
  포함될 수 있는 조건이며, 보고서의 근거·정확성·업무 성공을 대신 심사하지 않는다.
- native `prompt.submit.origin.kind`, turn/step/agent 영수증으로 TUI의 위임 후 회차와
  루트 완료 알림 회차를 구분한다. pending 자식이 있는 적격 요청의 일반/빈/추론뿐인
  응답을 검증 후 빈 제어 응답으로 바꾸고, native streaming hook은 이를 대기로 소비한다.
  응답 형식·종료·정확 계수 검증은 생략하지 않는다.
- 제어 판단은 backend 응답 완료 뒤에 확정한다. HTTP 전에 생기는 native engine chunk가
  지난 판단을 읽지 않도록 첫 의미 있는 chunk까지 제한된 prefix를 보관한다.
- 별도 모델 polling을 추가하지 않는다. native 완료 이벤트가 기존 부모를 재진입시킨다.
  새 사용자 입력과 해당 입력의 Read 왕복은 기존 자식 때문에 지워지지 않는다.
  기다리는 turn의 `turn.complete(answer)`만으로 진행을 종료 처리하지 않는다.

**적용 한계:** SDK/`-p`, 분류되지 않은 입력, 출처가 없는 자식 새 회차 index 0에는 같은
무출력 제어를 적용하지 않는다. SDK 실험에서 이 제어를 적용하면 실제 native가 exit 1
`Execution error`로 끝났기 때문이다. 기존 SDK 생성/중첩 실행 경로는 보존하고 회귀 확인했다.
이 범위를 TUI와 똑같이 지원한다고 표현하지 않는다. 대기 조건 없는 빈 응답도 성공으로 바꾸지 않는다.

### 미완료 Workflow

native `Workflow` 안에서 아래 확장만 추가했다. 별도 Claude Code 프로세스로 대체하지 않는다.

```json
{"script":"clauduct:plan-v1","args":{"steps":[
  {"id":"A","prompt":"독립 작업 A","model":"gpt-5.6-sol","effort":"high"},
  {"id":"B","prompt":"독립 작업 B"}
]}}
```

- 독립 단계 1~16개. 각 단계는 고유 `id`와 `prompt`, 선택적 `model`/`effort`만 허용한다.
  ID 80자, prompt 각 32,768 bytes, 합계 200,000 bytes 상한. 다른 필드·중복·잘못된 조합은 거부한다.
- 단계별 모델·effort를 최초 선택 때 고정한다. JSON literal의 native `await agent()`로
  컴파일하므로 프롬프트가 JavaScript나 승인 지침으로 실행되지 않는다.
- 재개는 `{"resumeFromRunId":"wf_..."}` 하나만 전달한다. 같은 launcher/session의
  원본 script hash, 관측한 자식 ID/key/label·순서, native journal, TaskStop 영수증,
  시작했던 모든 자식의 실제 종료를 확인한다. journal에서 시작 기록을 삭제해 새 작업처럼
  만드는 경우도 최초 관측 지도와 대조하여 거부한다.
- 완료 본문은 `completed_result_reused`, 시작했으나 완료 본문이 없으면
  `started_not_reexecuted`다. 시작하지 않은 suffix만 새 native Workflow에서 실행한다.
  원본 run의 소비권은 원자적으로 한 번만 부여해 동시/후속 중복 재개를 거부한다.
- 재개 중 부모 모델이 바뀌어도 남은 단계의 원래 모델·effort를 유지한다.
  본문 없는 native 반환값은 `result_unavailable`이며 `complete:false`다.
  `completeMeaning:all_step_results_present_not_task_success`를 반환하므로
  `complete:true`는 모든 본문 존재 조건일 뿐 의미상 성공 보증이 아니다.

**적용 한계:** 임의 JavaScript를 처음부터 다시 실행하지 않는다. 기존 arbitrary inline script는
완료 결과 회수만 지원한다. 다른 세션/launcher 재시작 후 복원, 변경된 script, 결과가 불명확한
시작 단계의 재실행, 단계 결과를 다음 prompt에 계산해 넣는 의존식, custom role 및 미검증
`tools`/`maxTurns` 강제 제한은 이번 범위에 없다. 호출 거부를 우회하는 대체 script도 만들지 않는다.

## 실제 실행 근거

### 실제 native TUI + 구독 backend

`4648bee3-1752-4934-8e36-7641ea4cd626` / `3b002c1`:

- 77 요청, 54 inference, backend usage 대조 54 일치/0 불일치.
  API 실패 0, native 취소 1, 의도한 중복 Workflow 거부 1, 종료 시 결과 미확보 0.
- ROOT→MIDDLE(Terra/low)→LEAF(Terra/low), 결과 41→42→43.
  대기 제어는 ROOT/MIDDLE 각각 1회. 별도 사용자 Read가 pending 중 처리됐고,
  이어 LEAF/MIDDLE 결과 ID가 실제 부모 입력에 포함됨을 request 스냅샷으로 확인했다.
- 최초 `wf_703d681f-312` 시험에서는 A/B가 부모의 Workflow 실행 지시를 자기 작업으로
  해석해 실패했다. native가 원래 사용자 요청도 각 자식에게 전달함을 transcript에서 확인했다.
  이 시도는 B 명령 실행/미완료 재개 검증으로 인정하지 않는다.
- 부모와 작업자 지시를 구분한 새 `wf_9a3fd71d-484`에서는 A 본문 확보, B의 공개 timer
  명령 시작 뒤 TaskStop, C 미실행을 확인했다. 재개 `wf_584d8b3d-2cf`의 journal에는
  C 시작 1건만 있다. A 재사용, B 미재실행, C 본문 확보와 `complete:false` 확인.
- 같은 원본의 두 번째 재개는 native 도구 단계에서 거부됐고,
  같은 세션의 다음 일반 요청은 `S43_AFTER_REJECT_OK`를 반환했다.

`117719c7-d224-4ece-9700-fd7ee458644b` / 최종 `01614e0`:

- 43 요청, 30 inference, backend usage 대조 29 일치/0 불일치. API 실패 0,
  요청 취소 2, native 자식 취소 1, 종료 시 결과 미확보 0. 최종 exit 0·watchdog 미작동.
  계수 중 취소 1건은 필수 계수 검사를 끝내기 전에 종료됐으므로 기능 조건 미확인 누계에 남는다.
- 원본 `wf_b0162ec8-3e7`에서 A 완료 후 B 실제 명령 실행 중 TaskStop.
  부모를 Astra→Sol로 바꾼 다음 재개 `wf_ae4f1a7d-6a9`에서 C만 실행했다.
  C는 원래 Luna/low를 유지했고 A 재사용/B 미재실행 및 `complete:false`를 반환했다.
- C가 불필요한 `env | sort | ls`를 제안해 승인하지 않았다. 이후 C 본문은
  `S43_F_C_23`으로 끝났다. **사람의 거부 개입이 있었으므로 무개입 작업 성공으로 세지 않는다.**
  라우팅·중복 방지·본문 회수 확인과 모델의 지시 불이행은 별도 사실이다.
- Sol/low 수동 `/compact` 요청은 약 53.9초에 완료됐고 backend usage와 정확 계수가 일치했다.
  이후 ORCHID-63, A/B/C 상태와 `S43_AFTER_COMPACT_OK`를 반환했다.
- seq 39는 count 단계에서 Esc 취소됐다. 다음 요청을 입력할 때 검사기가 취소 초안을
  완전히 지우지 못해 두 프롬프트가 섞였다. 의도한 깨끗한 early-recovery 시험으로 세지 않는다.
  이 실제 요청은 정상 생성됐으며 부분 출력 2,191 text delta 뒤 seq 41에서 Esc 취소됐다.
  이후 새 단독 요청은 `S43_FINAL_RECOVERED ORCHID-63`를 반환했다.
  입력 조작 오류와 제품 회복을 구분하며 사용자 절차에는 전송 전 초안 확인을 추가했다.

### 실제 native TUI + 고정 upstream fixture

빈 backend 응답은 실제 모델에게 안정적으로 요구할 수 없어 고정 응답으로 발생시켰다.
실제 native TUI, 제품 gateway, 제품 hook을 사용했으나 backend 계수/응답은 fixture다.
이를 실제 모델의 자연 발생 빈 응답이나 실제 계수 정확도 증거로 표시하지 않는다.

- `19258ef5-b61c-433b-804a-31b97ab68405`: 빈 본문을 대기로 연결하고 손자→중간→루트 전달.
- `3d1ac679-d235-4a95-80d1-62947bf76128`: 일반 대기 본문 억제와 별도 사용자 Read 왕복 보존.
- `69412496-eb80-41d8-8084-42980f008142`: 경합 시험. 손자 응답은 24,572ms에 끝났고,
  중간의 빈 대기 응답은 26,177ms, 루트 대기 응답은 26,355ms에 끝났다.
  이미 도착한 완료 이벤트를 잃지 않고 기존 중간/루트가 각각 한 번 이어져 최종 본문을 반환했다.
  parent_wait 2, empty_reply_wait 1. 1회의 지정 경합 관측이며 모든 순서 조합의 보장이 아니다.

## 검사와 실패 이력

[검사 요약](test-summary.json)에 성공/실패 로그 이름과 hash, skip을 모두 남겼다.
raw 로그와 임시 profile은 로컬 증거 디렉터리에 보존하며 전체를 git에 넣지 않는다.

- 전체 회귀: 17 packages, 1,569 test/subtest 통과, 3 skip.
  그 뒤 계획 compiler 정리/빈 결과 판정을 수정하여 관련 protocol/gateway 925개와 vet를 재실행했다.
  마지막 소규모 수정 이후 전체 회귀를 다시 돌린 것으로 표현하지 않는다.
- SDK 중첩 검사 3개는 대기 제어를 무출력으로 적용했을 때 실제 native `Execution error`로
  반복 실패했다. 종료값 모양 변경만으로 해결되지 않아 입력 모드 경계를 조사했고,
  TUI에서만 제어하도록 수정한 뒤 해당 native 검사와 전체 회귀를 통과했다.
- 최초 전체 회귀의 `grace_exhausted`는 request 누수 timeout으로 실패했다.
  부모 test까지 함께 fail로 기록된다. 이후 같은 검사에 client phase/경과 시간 계측만 추가했고
  25회 연속 통과했지만 **최초 실패의 원인은 미확정**이다. 제품 종료 코드를 고친 것도 아니다.
  이 실패를 해결·비플래키로 선언하지 않는다.
- plan 본문/완료 의미 검사 추가 직후 기존 출력이 새 조건을 충족하지 않아 실패했고,
  수정 후 null/빈 문자열/공백/비문자열 결과의 미완료 판정과 정상 본문을 확인했다.
  이 분기는 VM 검사이며, 모든 실패 반환형을 실제 TUI에서 발생시킨 검사는 아니다.
- 초기 fixture는 `ResolveClaude` 누락으로 실행 전 실패했고, 이어 상속과 충돌하는 자식 모델을
  잘못 지정해 거부됐다. Workflow fixture의 native taskId 추출 오류도 있었다.
  각각 fixture 원인을 수정했으며 제품 실패/성공으로 바꿔 세지 않는다.
- 자동 suite의 skip 3개: 실제 live session, launcher kill, console close 검사는 별도 실증이
  필요한 opt-in이다. 이번 TUI를 console close/외부 강제 종료 재검증으로 간주하지 않는다.
- 문서 citation 검사는 기존 S41 분석 보고서·S41 raw 검사 로그·S42 분석 보고서 링크 3개가
  git에 없어 실패했다. 이번 수정 전의 깨끗한 `01614e0` checkout에서도 같은 3개가 실패했다.
  새 보고서/지원표의 누락 링크는 추가되지 않았지만 전체 문서 검사가 통과했다고 표시하지 않는다.
  원본 증거를 확인하지 않고 빈 파일을 만들어 검사만 통과시키지 않았다.

## 이전 판단 정정

이전 `16dfd222-e9b6-44a7-94d6-27f383d84850`에 대해 부모의 완료 문장이 native 알림보다
먼저 보여 조기 완료라고 기록했으나, 이번에 자식 원문을 다시 확인했다.
A 본문은 17:37:41.843Z, B 본문은 17:37:41.788Z에 이미 있었고 부모의 다음 입력은
17:37:41.973Z였다. 기존 gateway는 native UI 알림보다 먼저 본문을 넣을 수 있다.
따라서 화면 순서와 모델의 자기 정정만으로 실제 조기 완료를 확정한 판단은 근거가 부족했다.
당시 status에는 실제 포함 ID가 없어서 반대 방향의 완전한 입력 증명도 할 수 없다.
이번 `parentReadiness`가 그 관측 공백을 보완한다. 과거 로그는 수정하지 않는다.

## 남은 범위와 사용자 검수

최종 사용자 TUI는 새로운 단계별 프롬프트로 진행한다. 정상 경로와 의도한 거부·취소는
따로 판정하며, 실패 후 같은 요청을 반복해 성공한 한 번만 채택하지 않는다.
원본/재개 run ID, 자식 계보, 단계별 checkpoint, 최종 status, 실제 대화 본문을 함께 대조한다.
모델 지시 불이행, 미확정 deadline 실패, 비TUI 대기 제어, 범용 Workflow 재개,
모든 멀티모달/자동 압축 경계/성능의 무결점 보장은 여전히 별도 과제다.

고정 후보와 최종 실행의 상태 원본은 각각
[첫 실제 실행](run-4648bee3-1752-4934-8e36-7641ea4cd626.json),
[최종 실제 실행](run-117719c7-d224-4ece-9700-fd7ee458644b.json)에 있다.
