# 다음 설계의 근거와 반례 — 2026-09-19

## 판정

사용자가 채택한 기능별 필수 조건 검사·실패 회복 판정·중복 실행 방지 방향은 유지한다.
다만 **빈 응답을 대기로 바꾸는 최소 구현은 아직 확정할 수 없다.** 실제 native TUI에서
자식 완료 후 부모 재진입을 관측했지만, 빈 응답 뒤 native가 만드는 추가 모델 요청도 확인했다.
단순히 `EMPTY_REPLY`를 제거하거나 빈 text 블록을 보내는 변경으로 완료 처리하면 안 된다.

이 문서는 다음 설계의 근거 목록이다. 미확보 사실도 별도로 기재했으며,
모든 기능의 실제 구현·통합 검증을 끝냈다는 보고서가 아니다.
제품 코드·기존 native 설정·설치 바이너리는 바꾸지 않았다.

## 기준과 조사 범위

- 작업 root: `D:\AIDEV\clauduct-s36-build\context-fork-repair-20260919`.
- 검토 시작 HEAD: `ffcee08`. 제품 코드 commit: `17532f7dc9daf7d5c47b0539af62a243c43df5a3`.
- 개발 `clauduct.exe` SHA256: `84bdbdf85de0c5e555dc01053979ee7eb8e5f9a5cfd9872a97a86a8033c60011`.
- 현재 native: `2.1.278`, SHA256 `006ea5c8638f67f10a5ae66bb232fd267c9f6af294e3f03f4cfcf1fd3f2cced8`.
- [현재 native API 선언](native-function-api.d.ts)은 해당 executable의 `/plugin-types`로 새로 생성했다.
  [생성 기록](native-types.json): 499,393 bytes, SHA256 `4d7d287eede4f20e3b84907189508c83ad05bda65609d70183963164821f815c`.
  이전 `2.1.276` 선언은 최신 근거로 사용하지 않았다. function hooks 활성화가 없는 첫 TUI에서는
  이 명령이 `Unknown command`였고, 별도의 한정된 선언 생성 프로세스에서 활성화해 생성했다.
  새 plugin/hook은 설치하거나 활성화하지 않았다.
- [실험 코드](native-wait.mjs): 승인된 공개 fixture 프로젝트/profile, 별도 session UUID,
  로컬 HTTP 응답, 실제 native TUI. 외부 모델 응답·정확 계수·Clauduct gateway를 시험한 것은 아니다.
  Agent 외의 실행 도구 호출을 생성하지 않았다. 요청 상한 18, 실행 상한 180초.
- [기계 감사](audit.mjs)와 [감사 결과](native-wait-audit.json)는 지정한 4개 공개 실험 session만 읽고
  본문 대신 결과 표식·추가 보정 횟수·실패 알림·요청 수를 보존한다.

## 1. 빈 응답 → 자식 대기

### 실제 결과

ROOT → MIDDLE → LEAF를 native Agent로 실행했다. LEAF의 로컬 응답을 12초 늦추고,
MIDDLE에 먼저 지정한 응답 형태를 보냈다. 이후 동일 MIDDLE ID의 자동 재진입과
ROOT가 최종 결과를 받는지 검사했다. status 조회를 위한 모델 호출은 넣지 않았다.

| 실행 UUID / 입력 | native의 빈 응답 보정 요청 | 최종 ROOT 결과 | 판정 |
|---|---:|---:|---|
| `77805b16-e954-414b-99ba-74763eaa97f2` / content 0개 | 1 | 없음 | 실패. 마지막에 native `ECONNRESET` 알림 |
| `ed9b4683-3fcb-4a7b-8fbc-1407ac7a7e3a` / 본문 있는 대조군 | 0 | 없음 | 대조군도 같은 오류. 빈 응답만의 오류라고 단정할 수 없음 |
| `45defe83-2b79-40ff-a908-82e77418b0eb` / 전달 완료 시각을 기록한 본문 대조군 | 0 | 1 | 이 실행에서는 자식 완료·부모 재진입·ROOT 수신 확인 |
| `fb99d281-ed62-4357-9ca0-fd6457997d99` / text 블록의 문자열만 비움 | 1 | 1 | 재진입 가능. 추가 모델 요청을 없애는 안으로는 불합격 |

원본 로컬 응답 기록:
[첫 빈 응답](run-77805b16-e954-414b-99ba-74763eaa97f2/result.json),
[첫 대조](run-ed9b4683-3fcb-4a7b-8fbc-1407ac7a7e3a/result.json),
[관측을 보강한 대조](run-45defe83-2b79-40ff-a908-82e77418b0eb/result.json),
[빈 text](run-fb99d281-ed62-4357-9ca0-fd6457997d99/result.json).

네 실행은 모두 `/exit`으로 exit 0 종료했다. 앞의 두 실행은 **작업 실패**다.
나중의 성공으로 첫 실패를 삭제하거나 안정성 합격으로 바꾸지 않는다.
두 성공은 각각 1회이며 반복 안정성을 증명하지 않는다.
앞의 두 실행에서 LEAF 응답 뒤 MIDDLE 요청까지 약 30초가 걸렸고, 뒤의 두 실행은 29/35ms였다.
`ECONNRESET`의 원인은 확정하지 못했다. 응답 완료 시각 관측을 추가한 이후 성공했다는
상관관계만 있으며, 계측 추가를 수정으로 인정하지 않는다. 이 실패를 현재 Clauduct의 장애로
그대로 귀속할 수도 없다. 실제 제품 경로를 통과하지 않은 native/로컬 fixture 실험이기 때문이다.

### 확인한 원인과 적용 한계

1. [Builder.Complete](../../go/internal/protocol/anthropic/response.go)는 text/tool이 없으면
   `ErrEmptyReply`를 반환한다. [gateway](../../go/internal/gateway/messages.go)는 `EMPTY_REPLY`로 분류한다.
   [결과 관리](../../go/internal/gateway/results.go)의 `awaiting_children`은 결과 판정 상태이며,
   이 프로토콜 오류를 native 대기로 전환하지 않는다.
2. 현재 native 바이너리에는 완료된 content 블록이 없는 SSE를 오류로 처리하는 경로가 있다.
   또 보이는 text가 없는 `end_turn`/`stop_sequence`에 내부 보정 메시지를 넣고 재요청하는
   `thinking_only_retry` 경로가 있다. 두 경로는 구분해야 한다.
   [감사 결과](native-wait-audit.json)의 바이너리 offset과 해시로 해당 사실을 다시 찾을 수 있다.
3. 빈 text 실험의 native transcript에는 내부 보정 메시지 1개가 있었고,
   MIDDLE은 본문 대조군보다 요청을 1회 더 보냈다. 로컬 fixture이므로 실제 요금이나
   추가 토큰 수를 측정한 것은 아니다. 실제 backend로 그 요청을 보내면 추가 모델 실행이 된다.
4. 현재 API 선언의 `turn.step`은 모델 요청 전 관찰/개입 지점이고, `agent.list()`는
   `id/status/parentId`를 제공한다. `turn.complete`의 결과는 text/usage이며 대기 제어가 아니다.
   `turn.step` 반환 객체만 바꾸는 것도 이미 engine이 스트리밍한 내용을 바꾸지 않는다.
5. 공식 [StopFailure](https://code.claude.com/docs/en/hooks#stopfailure)는 실패 관찰용이다.
   반환값으로 실패를 대기로 전환할 수 없다. 같은 문서의 `PreToolUse` `defer`는 TUI에서 적용되지 않는다.
6. `pause_turn`은 공식적으로 서버 도구의 장시간 실행을 이어가는 의미다.
   이를 native Agent 대기로 바꾸어 부르는 것은 검증된 해법이 아니다.
   [공식 stop reason 의미](https://platform.claude.com/docs/en/build-with-claude/handling-stop-reasons#pause_turn)

**권고:** 기존 오류 검증은 유지한다. 다음 한정 실험은 native `turn.step` 경계에서
검증된 대기 관계에만 적용되는 제어가 가능한지 증명하는 것이다. 가짜 본문·종료 이유·사용량을
만들거나 native 내부 문자열에 의존해 응답을 성공으로 바꾸는 방식은 채택하지 않는다.
추가 요청의 실제 backend 실행을 막을 수 있는지, 막은 뒤 자식 완료가 동일 부모를 깨우는지,
Esc/실패/늦은 완료 때 대기가 남지 않는지가 필수 증거다. 이 경로의 실제 hook 실험은 아직 하지 않았다.

## 2. 진행 확인과 종료 판정

| 확보한 사실 | 설계에 주는 제약 |
|---|---|
| 현재 [native-events.mjs](../../go/internal/app/native-events.mjs)는 회차 시작/종료 영수증만 쓴다 | 시작 기록 존재를 지속 진행으로 해석하면 안 됨 |
| [native_events.go](../../go/internal/gateway/native_events.go)의 영수증에는 시간·도구 진행·승인 대기 정보가 없다 | 현재 자료만으로 장기 정상 작업/정체/승인 대기를 항상 구분할 수 없음 |
| 현재 native 선언은 `tool.call`, `tool.check`, `turn.step`, `turn.complete`, `agent.list()`를 제공 | 기존 session plugin의 제한된 관측을 확대할 수 있는 후보. 각 실제 호출·중단 순서는 아직 실측 필요 |
| 공식 Stop/SubagentStop의 `background_tasks`는 session 범위 | 개별 부모의 자식 증거로 쓰려면 parentId/계보를 별도로 대조해야 함 |
| [lifecycle](../../go/internal/app/session_lifecycle.go)는 기본 실행에 강제 시간을 만들지 않음 | 새 무진행 타이머로 정상 작업을 취소할 이유가 없음 |
| 같은 코드의 `drainReady`는 HTTP active·압축 phase만 확인 | “HTTP가 없다 = native 도구도 끝났다”는 판정은 아직 불충분 |

공식 [hook 입력](https://code.claude.com/docs/en/hooks#stop)과 현재 선언을 함께 확인했다.
`PermissionRequest`는 승인 질문 관찰에 도움이 되지만 모든 종류의 native 승인 UI를 보장하는
단일 신호는 아니다. 선택 권한은 그대로 두고 관측만 추가해야 한다.

**최소 설계:** 마지막 관측 시각/종류와 현재 요청·도구·자식 ID를 별도로 저장한다.
승인 질문을 실제로 보았으면 승인 대기, 종료 이벤트를 보았으면 종료,
나머지는 경과 시간과 함께 `진행 확인 불가`로 표시한다. 로컬 프로세스/파일/이벤트 관찰은
모델 상태 확인 프롬프트와 구분한다. 새 고정 종료 시간은 넣지 않는다.
UI 알림 임계 시간은 아직 확정하지 않았으며, 관측 누락을 실패로 바꾸는 기준으로 사용하지 않는다.

## 3. Workflow 재개와 중복 효과 방지

공식 [Workflow 재개](https://code.claude.com/docs/en/workflows#resume-after-a-pause)는
실패한 agent와 그 뒤의 agent를 다시 실행할 수 있다. prompt가 달라진 경우에도 후속 작업이
재실행될 수 있다. 따라서 native 재개를 그대로 연결하면 사용자가 채택한 정책을 만족하지 못한다.

현재 [prepareWorkflow/linkWorkflow](../../go/internal/gateway/workflow.go)는 inline·root 호출만
받으며 `scriptPath`, named, `resumeFromRunID`, remote, nested를 거부한다.
[wrapper](../../go/internal/gateway/workflow-agent.js)는 custom `agentType`도 거부한다.
[journal 회수](../../go/internal/gateway/workflow_results.go)는 script digest와 run/child/key를
검증한 저장 결과를 회수한다. 과거 도구의 외부 실행 효과나 안전한 재실행을 증명하는 기능은 아니다.

**권고 순서:**

1. 동일 run/child/script/input에 연결된 완료 결과를 재사용한다. 결과 회수는 새 실행으로 대체하지 않는다.
2. 도구 실행 전에 호출 ID·대상·효과 종류를 기록하고, 정상 결과·확인 가능한 사후 상태를 연결한다.
   시작 기록만 있고 종료 증거가 없으면 `실행 결과 불명`으로 남긴다.
3. 읽기 전용이거나 실제 멱등성이 검증된 작업만 정해진 조건에서 재개한다.
   임의 Bash/외부 API/DB 쓰기의 효과가 불명확하면 자동 실행하지 않는다.
4. 중단된 native run의 자식이 모두 끝났는지 확인한 뒤 재개를 허용한다.
   이 검사도 완료 결과의 중복 사용 방지와 별도로 필요하다.

**확실한 한계:** 효과가 외부에서 발생했는지 확인할 방법이 없는 임의 도구에 대해,
Clauduct의 로컬 기록만으로 모든 장애 시점의 정확히 한 번 실행을 보장할 수는 없다.
외부 시스템의 멱등 키·거래/확정 조회 또는 사람의 확인이 있어야 그 공백을 줄일 수 있다.
이것은 Workflow 전체 구현 불가능 판정이 아니라 자동 재실행 허용 범위의 조건이다.
안전한 결과 재사용부터 구현할 수 있으며, 불명확한 쓰기까지 재실행 가능하다고 표시하지 않는다.

## 4. 버전 대신 기능별 필수 조건 확인

### 현행 구현에서 확인한 차이

- [ClientReport.Verified](../../go/internal/gateway/diagnostics.go)는 버전 문자열 일치다.
  이 필드가 true여도 기능 시험 통과를 뜻하지 않는다.
- [context_display.go](../../go/internal/gateway/context_display.go)는 native row의 버전이
  `ReferenceClient`와 같을 때만 출처를 수집한다. 전체 세션은 버전으로 차단하지 않지만,
  이 최적화에는 버전 동등 조건이 남아 있다.
- [bindNativeTurn](../../go/internal/gateway/native_events.go)은 영수증이 없으면 일부 경로에서
  그대로 진행한다. 원래 선택의 검증과 현재 native 회차 연결은 서로 다른 조건이다.
  기본 Agent 선택이 검증됐다는 사실만으로 자동 재진입·빈 응답 대기까지 허용하면 안 된다.

### 최소 설계안

새 범용 검증 프레임워크 대신 기존 검사 함수들의 결과를 status에 모은다.
기능마다 `구현 여부`, `현재 필수 조건`, `관측 근거`, `과거 TUI 근거`를 분리한다.
실행 조건은 `미관측/충족/불충족`으로 두고, 과거 근거는 실제 version/hash/session/입력 범위로 남긴다.
미관측을 충족으로 승격하거나, 버전이 같다는 이유로 이전 실행의 결과를 복사하지 않는다.

| 기능 | 실행을 허용하는 경계와 조건 | 실패 영향 범위 |
|---|---|---|
| 일반 생성 | 입력 decoder·확정 route·정확 계수·해당 모델 정책 | 해당 요청. 공통 초기화가 실패하면 session |
| native Agent | 원래 지정 여부·역할·session/child/parent 연결·확정 model/effort | 해당 자식 추론 시작 전 중단 |
| 자동 재진입/SendMessage | 기존 계보·선택 + 새 native turn, 이전 회차 종료와 분리 | 해당 재진입 |
| 빈 응답 대기 | 위 조건 + 실제 자식 관계/상태 + 검증된 native 대기 연결 | 아직 기능 비활성. 일반 빈 응답을 성공으로 승격하지 않음 |
| Workflow | script/run/child/선택·도구 제한·journal 형식과 연결 | 해당 Workflow 호출/자식. 재개 조건은 별도 |
| `/context` 입력 제외 | session·명령·출력·UUID 연결·출처 구조 | 출처를 못 증명하면 삭제 최적화만 하지 않고 원문 보존. native 조회 자체와 구분 |
| 압축/전환 | 정확 계수·모델 임계치·기존 모델/effort·완료/재계수 연결 | 해당 agent의 다음 생성 |

응답 종료·usage 일치·부모 수신은 실행 후에만 검사할 수 있다.
모든 미래 결과를 실행 전에 보장한다는 조건을 넣으면 어떤 기능도 정직하게 허용할 수 없다.
사전 조건을 통과시킨 뒤, 응답·도구 전달·완료 각각의 경계에서 후속 조건을 검증한다.

`/context`의 버전 조건을 제거할 때는 **출처·계보 검증을 구조 검사로 대체**해야 한다.
단순 문자열 패턴으로 사용자 본문까지 삭제하는 변경은 금지한다.
서버/클라이언트 지문이 달라지면 현재 조건을 다시 관측하되, 구버전 TUI 기록은 구버전 증거로 보존한다.

## 5. 회복 판정과 나머지 근거 공백

작업 결과와 장애 처리 결과를 각각 판정한다. 예를 들어 backend 오류로 작업은 실패했지만,
오류 보고·기존 결과 보존·같은 session의 다음 정상 요청이 모두 관측되면 그 장애의 회복 검사는 합격할 수 있다.
`process=SUCCESS`나 exit 0만으로 두 판정을 대신하지 않는다.

| 항목 | 확보한 근거 | 더 필요한 증거 |
|---|---|---|
| 모델/effort·하위 상속·역할 유지 | [기존 지원표](../../docs/v2/COMPATIBILITY.md)의 실제 tree/Workflow/resume 기록 | 모든 역할·동시 순서·지원 조합의 무제한 보장은 없음. drift 조건별 거부와 영향 범위 TUI |
| 부분 인자 생성 중 Esc | 기존 streamEnd는 총 이벤트 수만 있음. 도구 인자는 완료까지 버퍼링 | 고정 이벤트 종류/개수만 기록해 실제 argument-delta 구간을 식별한 TUI. 도구 선실행 방지 유지 |
| 외부 오류·전달/flush 실패 | [분류 코드](../../go/internal/gateway/messages.go), [취소 검사](../../go/internal/gateway/cancellation_test.go) | 429/5xx/단절/잘린 SSE/계수 실패 각각의 동일 session 회복 TUI |
| 정확 계수 | 기존 최종 두 TUI에서 backend usage 대조 38건 일치 | 최신 빌드의 이미지 해상도/상세도·다중 이미지·PDF·혼합 tool_result별 count/usage. 전체 멀티모달 완벽 지원 주장 금지 |
| 계수 성능 | [cache/shared](../../go/internal/gateway/counter.go), [연결 재사용](../../go/internal/upstream/count.go) | cold/warm/동시·취소별 p50/p95와 실패율. 새 입력의 네트워크 왕복을 0으로 보장하지 않음 |
| 압축 경계/모델 전환 | 기존 별도 실측과 정책 코드, [지원표](../../docs/v2/COMPATIBILITY.md) | 최종 빌드에서 경계 직전/도달/초과, 자식별 병렬 압축·Esc·실패·모델 전환 회복 TUI |
| launcher 강제 종료 | [기존 handle 관측](../session41-repair-20260919/hard-kill-261642e5-d5c9-4a70-8a35-41eb11c1a45a.json): 하위 6개 종료·decoy 생존 | 콘솔 창 닫기·OS 종료·모든 외부 도구의 breakaway 조합은 별도 |
| `/context` | 출처가 검증된 조회 기록을 backend 입력에서 제외. native 표시 미변경 | 새 native 구조의 출처 확인 실패 진단과 최적화 비활성 표시. 공통 500K/로컬 추정 표시의 변경은 사용자 필수 조건에서 제외됨 |
| 기능별 호환성 문서 | [COMPATIBILITY.md](../../docs/v2/COMPATIBILITY.md) | 위 runtime 판정과 문서 행의 연결 구현. 문서 작성만으로 실행 조건 검사 구현 완료 처리 금지 |

이 표는 다음 설계에 연결된 미완료 항목 목록이다. 알려지지 않은 모든 버그가 없다는 보증은 아니다.
[기존 검사 요약](../session41-repair-20260919/test-summary.json)의 1,525 test/subtest 기록과 3 skip은
이전 제품 commit의 근거로 유지한다. 이번 문서/fixture 변경 때문에 제품 전체 검사를 반복하지 않았다.

## 권하는 다음 순서와 중단 기준

1. 상태 판정을 분리하고 기존 검사 결과를 기능별로 모은다. 버전 일치와 TUI 통과를 별도 표기한다.
2. 본문을 저장하지 않는 도구/승인/스트림 단계 관측을 보완한다. 정보가 없으면 모른다고 표시한다.
3. 빈 응답 대기의 native 제어 가능성을 한정 실험한다. 추가 backend 실행·가짜 본문·결과 혼입·Esc 후 정지가
   없어야 한다. 현재의 native 자동 보정과 fixture 연결 오류가 남으면 지원 완료로 승격하지 않는다.
4. Workflow의 검증된 결과 재사용부터 확대한다. 불명확한 쓰기 재실행은 차단한다.
5. 위 근거 공백별 실제 TUI를 실행한다. 최초 실패를 보존하고 원인 변경 후 재검사한다.
   제품 binary/실행 조건이 바뀌지 않은 재시도 성공만으로 flaky를 해결했다고 판정하지 않는다.

현재 즉시 구현 설계를 확정할 수 있는 범위는 **상태 판정 분리·기능별 조건 보고·진행 관측·보수적인 결과 재사용**이다.
**추가 호출 없는 빈 응답 대기**와 **효과 불명 쓰기의 자동 재개**는 필요한 증거 없이 지원을 약속하지 않는다.
전자는 native 제어 실증이 남았고, 후자는 대상 도구/외부 시스템의 확정 가능한 계약이 필요하다.
