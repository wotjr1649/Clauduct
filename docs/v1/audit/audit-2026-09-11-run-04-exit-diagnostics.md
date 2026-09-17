# 감사 — run-04 종료 JSON 분석

2026-09-11. 세션 `2c4ceab9`, 408요청 / 성공 398 / 실패 10, 2시간 37분, 자식 9명. `requestOutcome: has-failures`.

실패 10건은 전부 **실행이 이어진 뒤** 남은 것이다. 어느 것도 3주기 완주를 막지 않았다. 아래는 그 10건이 무엇이었는지와, 무엇까지 분석되고 무엇이 분석 불가인지다.

## 1. 가장 중요한 것 — 이름 포착 장치가 비어서 돌아왔다

```
lifetime.unsupportedEvents: 1
failureHistory[req 77]: unsupportedEvent "other", unsupportedEventTypeFormat "identifier"
unsupportedEventNames: []        ← 비어 있다
```

`UNSUPPORTED_EVENT`가 **재발했는데 이름을 못 잡았다.**

원인은 같은 값을 보는 두 정규식이 다르다는 것이다.

| 쓰임 | 정규식 | 점 |
|---|---|---|
| 형식 분류 (`eventTypeFormat`) | `/^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)*$/` | **0개 허용** |
| 이름 포착 (`capturableEventName`) | `/^[a-z][a-z0-9_]{0,23}(?:\.[a-z][a-z0-9_]{0,23}){1,4}$/` | **1개 이상 필수** |

점 없는 소문자 식별자는 `identifier`로 **분류되지만 절대 포착되지 않는다.** 관측값이 정확히 그 조합이다 — `format=identifier`, `kind=other`, `names=[]`.

점 없는 이름이 예외적 형태도 아니다. `EVENT_DIAGNOSTIC_TYPES`에 이미 `ping`, `message_start`, `message_delta`, `message_stop`, `content_block_start`가 들어 있다. upstream이 실제로 쓰는 네임스페이스다. 거기서 새 이름 하나가 늘면 이 사각지대에 그대로 빠진다.

`kind=other`는 그 타입이 `response.` `codex.` `responsesapi.` `thread|turn|item.` 중 어느 접두사도 아니었고 알려진 목록에도 없었다는 뜻이다. 점 있는 미지의 이름이었다면 24자 제한에 걸리지 않는 한 포착됐을 것이다. 남는 가장 단순한 설명은 **점 없는 새 식별자**다.

**선행 조건 1을 닫은 근거가 이걸로 반증됐다.** "원인은 미확정이지만 재발하면 `unsupportedEventNames`가 이름을 잡는다"가 닫음의 논거였다. 재발했고, 못 잡았다. 원인은 여전히 미확정이며 이제는 장치를 고치기 전까지 다음 재발도 못 잡는다.

해결은 포착 정규식이 형식 분류와 같은 공간을 덮게 하는 것이다. 점 0개를 허용해도 `^[a-z][a-z0-9_]{0,23}$`는 소문자 24자 이하라 본문도 해시도 실어 나를 수 없다 — 경계 주석이 지키려던 성질은 유지된다.

## 2. 중단 3건 — 전부 자식, 서로 다른 선택 경로

| 요청 | selectionSource | clientDisconnected | upstream |
|---|---|---|---|
| 43 | `explicit-metadata` | **false** | 첫 이벤트 2076ms, 마지막 2176ms, 연결 `open` 상태에서 취소 |
| 81 | `skill-result` | **true** (794ms) | 이벤트 0건, 헤더도 못 받음 |
| 194 | `verified-resume` | **false** | 첫 이벤트 1737ms, `open` 상태에서 취소 |

셋 다 자식이고 `gpt-5.6-luna`/`max`다. **43과 194는 클라이언트 연결이 살아 있는 상태의 취소**다 — 호스트가 자식을 능동적으로 중단시킨 경우이고, 81은 반대로 클라이언트가 먼저 사라진 경우다. 두 형태가 한 실행에 다 들어 있다.

"실제 취소가 발생한 세션의 진단"이 이 3건이다.

## 3. 메인의 `invalid_prompt` 2건 — 스트리밍이 시작된 뒤

| 요청 | 모델 | 첫 텍스트 | 종료 | 코드 |
|---|---|---|---|---|
| 390 | `gpt-5.6-sol`/high | 2894ms | 6781ms, `terminalState: error` | `invalid_prompt` / `invalid_request_error` |
| 550 | `gpt-5.6-sol`/high | 16204ms | 22169ms, `terminalState: error` | 같음 |

둘 다 **HTTP 200으로 열려 본문이 흐르기 시작한 뒤** error 이벤트로 끝났다. 사전 거부가 아니라 스트리밍 도중 거부다.

`retryScheduledMs: []`이고 `attempts`는 1회뿐이다. 비스트리밍 전환 시도가 없었고 이는 `clientExecutionPolicy.nonStreamingFallbackDisabled: true`와 일치한다.

**다만 이것은 "차단 분기가 실행됐다"는 양성 관측이 아니다.** fallback이 일어나지 않았다는 음성 관측이며, 애초에 전환 조건이 아니었을 가능성과 구분되지 않는다. 그 구분은 이 JSON으로는 불가능하다.

## 4. admission 대기 7.4분 — 메모리 압력

| 요청 | 대기(admittedMs) | attempts | 시각 |
|---|---|---|---|
| 526 | **443239ms (7.4분)** | 없음 | 08:52:09Z |
| 529 | 376462ms (6.3분) | 없음 | 08:53:16Z |
| 528 | 376463ms (6.3분) | 없음 | 08:53:16Z |
| 530 | 79628ms (1.3분) | 없음 | 08:58:12Z |

네 건 모두 `failureStage: request`, `INVALID_REQUEST`, `clientDisconnected: true`이고 전송 시도 자체가 없다. 큐에서 몇 분을 기다렸고 슬롯이 열렸을 때는 클라이언트가 이미 없었다.

admission은 고정 동시성이 아니라 **메모리 기반**이다.

```
room() = free − active × 128MiB ≥ headroom + 128MiB
headroom = max(256MiB, min(총메모리/10, 1024MiB))
```

즉 자식 9명 + 메인 + 게이트웨이가 상주하는 동안 **여유 메모리가 수 분씩 모자랐다**는 뜻이다. 게이트는 제 일을 했다 — 죽지도 넘치지도 않고 큐에 세웠다(`MEMORY_QUEUE_FULL`은 없었다). 포기한 쪽은 클라이언트다.

이건 3주기 부하에서 처음 드러난 실측이며 합성 검사로는 나오지 않던 값이다.

## 5. 자원 정리는 완전했다

```
cleanup: childClosed, gatewaySocketsClosed, gatewayJobsClosed, gatewayTimersClosed,
         gatewayDeliveriesClosed, gatewayIdle, gatewayCleanupCompleted,
         transportSocketsClosed, transportRequestsClosed  — 9개 전부 true
agentRegistrationsEvicted: 0, agentRegistrationsExpired: 1, transportRejections: 5
```

2시간 37분 / 408요청 / 자식 9명 뒤의 실측이다. 등록표는 만료 1건뿐이고 상한 축출은 없었다.

## 6. 분석할 수 없는 것

- **`rejectedBeforeStart: 97`의 내역.** 97건이 타이밍 시작 전에 거부됐는데 남는 것은 `firstRejectedCategory: INVALID_AGENT_BINDING` 하나뿐이다. 나머지 96건의 분류는 복원 불가다. 진단 공백이다.
- **`unsupportedEvent`의 실제 이름.** 1장의 사각지대 때문에 영구 소실이다.
- **fallback 차단 분기의 양성 실행.** 3장 참조.
- **자식의 effort.** `failureHistory`의 자식 기록은 `effort: max`, `requestedEffort: max`로 남아 있다 — 이건 게이트웨이가 본 값이며 자식 프로세스가 실제로 그 effort로 추론했다는 증거는 아니다.

## 7. 그 밖의 관측

- `clientVersionStatus: "unverified"` — 실행 클라이언트 0.154.0, 기준 0.153.4.
- `clientContextPolicy.autoCompactWindow: 400000` — 정상 모드다(검증 모드의 100000이 아님). 이번 실행에 `compact_boundary`는 없었고 압축이 일어나지 않았다.
- `webSearchRequests/Calls/Links` 전부 0 — 이번 작업에 검색이 필요 없었다. 탐지가 빗나가 0이 된 것이 아니라 요청 자체가 없었다.
- `unknownBetaNames: []`, `judgedBetaLabels: []` — beta allowlist 보완 대상 없음.

## 8. 고친 것 (2026-09-11)

이 분석이 찾아낸 것 중 셋을 같은 날 고쳤다. 전부 진단이 자기 질문에 답하지 못하던 자리다.

### 8.1 포착 정규식이 점을 요구하던 것

`eventNameShape`의 점 요구를 `{1,4}`에서 `{0,4}`로 바꿨다. 24자 소문자 세그먼트, 전체 48자, 알려진 라벨 제외는 그대로다 — 값을 묶는 것은 세그먼트 길이지 점이 아니다.

근거는 추정이 아니다. 이 계열의 SSE 이벤트 이름은 점이 없다. 공식 문서가 `event: message_stop`, `event: error`, `event: ping`을 그대로 쓰고 `overloaded_error` 같은 점 없는 오류 타입도 있다. 이 저장소의 `EVENT_DIAGNOSTIC_TYPES`에도 `error`·`ping`·`message_start`·`content_block_delta`가 점 없이 들어 있다. 점을 요구하는 포착은 프로토콜이 실제로 쓰는 네임스페이스 하나를 통째로 못 본다.

**점 없는 이름 거부는 사고가 아니라 의도였다.** 테스트가 `'nodot'`을 거부 대상으로 고정하고 있었고, 사례표에도 `['private_event', 'other', 'identifier', null]`과 `['threadprivate_event', 'other', 'identifier', null]`이 기대값으로 박혀 있었다 — run-04에서 관측한 그 조합 그대로다. 진단이 확정되는 지점이자, 시험이 결함을 정답으로 굳히고 있던 지점이다. 두 행의 기대값을 포착으로 바꾸고 계약 변경을 주석에 남겼다.

### 8.2 빈 목록이 두 가지를 뜻하던 것

`unsupportedEventNames: []`는 "미지원 이벤트가 없었다"와 "있었는데 이름을 못 잡았다"를 구분하지 못했다. run-04를 읽을 때 실제로 막힌 지점이다. `lifetime.unsupportedEventNamesWithheld`를 추가해 갈랐다.

세그먼트가 24자를 넘거나 점이 5개 이상인 이름은 **여전히 포착되지 않는다.** 잔여 구멍을 없앤 것이 아니라 보이게 만든 것이다. 사례표에 `['x'.repeat(25), 'other', 'identifier', null]`을 넣어 고정했다.

### 8.3 97건의 거부에 라벨이 하나뿐이던 것

`rejectedBeforeStart: 97`인데 `firstRejectedCategory` 하나만 남아 나머지 96건을 복원할 수 없었다(6장). `lifetime.rejectedCategories` 집계를 추가했다. 키는 `diagnosticCategory`가 답하는 고정 어휘에서만 나오므로 무한히 늘지 않고, 0인 항목은 출력에서 뺀다.

### 8.4 종료 JSON이 stdout에만 있던 것

`recordRequestStatus`가 종료 시 `/.clauduct-status/request-status.jsonl`에 한 줄을 덧붙이고 `CLAUDUCT_REQUEST_STATUS_FILE` 줄로 경로를 알린다. `.gitignore`에 넣었고 `.clauduct-profile`(클라이언트 설정 홈처럼 보이는 옛 디렉터리) 안에는 쓰지 않는다. 쓰기 실패는 치명적이지 않다 — stdout 줄은 그대로 나가고 stderr로 사본이 하나뿐임을 알린다. 종료 출력 줄을 검사하는 테스트가 그때까지 **하나도 없었다.**

### 검증

`src/test-*.mjs` 19개 중 17 pass / 2 fail. 실패 2건은 인계 문서 6장의 환경 실패(`test-chat`, `test-review-diff` — 이 셸에서 자식 프로세스 spawn 불가)이며 이번 변경과 무관하다. `unsupported-event-diagnostics`는 61 → **63**으로 늘었고 `native-gateway` 61, `request-diagnostics` 69, `verification/test-manual-http-probe` 88/88이 기준선과 같다. 기본 경로 기록은 실제로 한 번 써서 확인한 뒤 지웠다.

### 고치지 않은 것

- **admission 메모리 압력.** 4장의 6~7분 대기는 정책 설계 문제이며 실패 동작이 아니다 — 죽지 않고 큐잉했다. 임계값은 측정 없이 손대지 않는다.
- **지난 두 건의 이벤트 이름.** 복구 불가다. 이번 수정은 다음 재발을 잡을 뿐 지난 것을 밝히지 않는다.
- `clientVersionStatus: "unverified"` (0.154.0 / 기준 0.153.4). 결함이 아니라 버전 흐름이다.
