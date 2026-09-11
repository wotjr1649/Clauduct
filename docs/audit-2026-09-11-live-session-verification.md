# 실사용 세션 검증 — 46b6af24 (luna/max)

사용자가 변경된 게이트웨이로 Clauduct를 다시 띄우고 그 세션과 메시지를 주고받아 확인했다. 이 기록의 값은 해당 세션이 제공한 `request-status` 출력이 출처다.

## 1. 웹 검색: 상류가 검색을 하지 않는다 — 확정

| 값 | 관측 |
|---|---|
| `lifetime.webSearchRequests` | 1 |
| `lifetime.webSearchCalls` | 0 |
| request 7 | `webSearchRequested: true`, `webSearchCalls: 0`, `success: true`, `failureCategory: null` |

게이트웨이는 Anthropic 서버 검색 도구를 받아 상류에 `{type:'web_search'}`로 번역해 보냈고, 요청은 정상 완료됐으며, **상류 Codex가 검색을 0회 수행했다.** 게이트웨이 결함이 아니다.

앞선 기록에서 "브리지가 호출되지 않았다"고 적었던 것은 틀렸고, 설치 바이너리의 side query 구조(`{type:"web_search_20250305", name:"web_search", max_uses:8}`)로 정정한 바 있다. 이번 실사용 카운터가 그 정정을 확증한다.

원인은 상류 쪽이며 오프라인으로 더 좁힐 수 없다. 근거 없이 도구 타입 토큰을 바꿔 재시도하지 않는다.

## 2. WebFetch 실패의 정체 — `UNSUPPORTED_BETA`

같은 세션에서 WebFetch가 `No response from model`로 실패했고 진단에 정식 기록이 남았다.

```
request 3, model null, effort null, failureStage "request",
failureCategory "UNSUPPORTED_BETA", requestFailure null,
retryScheduledMs [], attempts 0, clientDisconnected false, success false
```

`failureHistory.omitted`는 0이다. Claude Code가 그 요청에 이 프로젝트가 "비호환"으로 판정해 둔 beta를 실었고 게이트웨이가 400으로 거부했다. upstream 시도는 0회다.

**이 기록은 이번 세션에서 추가한 변경 덕분에 남았다.** 요청 진단 기록을 버전·beta·인코딩 검사보다 먼저 만들도록 바꾸기 전이었다면 이 실패는 `requestOutcome: all-succeeded` 아래 보이지 않는 400으로 사라졌을 것이다.

## 3. 수용 기준 대조 — 실패 1회와 그 복구

| 조건 | 관측 |
|---|---|
| 안전 중단 | `failureCategory`가 고정 라벨, `attempts` 0(상류 시도 없음), `retryScheduledMs` 빈 배열, `clientDisconnected` false |
| 데이터 보존 | 실패가 `failureHistory`에 보존되고 `omitted` 0. 실패 직후 WebSearch 결과가 반환되고 대화가 계속됐으며 이후 요청 7건이 모두 성공 |
| 도구 미중복 | upstream 시도 0회이므로 중복 실행이 성립할 수 없음 |

요청 수준의 세 조건은 모두 관측됐다. 남은 것은 `cleanup` 9개 항목이며 이는 세션이 정상 종료해야 나오는 값이다. **그 종료 JSON을 확보하면 감독하 판정을 PASS로 올릴 조건이 충족된다.**

## 4. beta 거부 정책 변경 — 사용자 결정

증거가 나온 뒤 사용자가 "판정 beta도 통과 + 라벨 기록"을 선택했다.

- `betaFailure`는 이제 **형식이 잘못된 헤더만** 거부한다(빈 항목·중복). 이름만으로 거부하지 않는다.
- 판정 27개는 거부 대신 고정 라벨로 기록한다. 요청별 `judgedBetaLabels`와 세션 누계 `judgedBetaLabels`로 남으며, 고정 어휘라 세션 내 상태 API에서도 보인다(원문 이름인 `unknownBetaNames`는 계속 종료 JSON 전용이다).
- 근거: beta 이름을 거부해도 그 기능이 이 백엔드에서 켜지지는 않는다. 거부는 기능을 끄는 게 아니라 **요청 전체를 죽인다.** 실제로 WebFetch가 그렇게 죽었다. 요청 본문의 모든 필드는 `prepareNative`가 독립적으로 엄격 검증하므로, beta가 실제로 계약을 바꾸면 `REQUEST_FIELDS` 같은 더 구체적인 라벨로 거부된다.
- `UNSUPPORTED_BETA` 분류는 과거 진단 projection 호환을 위해 목록에 남기지만 더 이상 생성되지 않는다.

한계: 이 변경으로 WebFetch가 실제로 동작하는지는 다음 실행에서 확인해야 한다. 어떤 beta가 원인이었는지도 그때 `judgedBetaLabels`에 드러난다.

## 검증

2026-09-11, Node.js v24.19.0, `Invoke-ClauductNodeTests`, 60초 제한. 기준 11개 파일과 선택·완료 표면 4개가 모두 통과했다. `test-native-gateway` 55개 검사, `test-native` 47개 검사다. 새 검사는 판정 beta가 200으로 통과하며 라벨이 요청별·세션 누계로 기록되는지, 형식이 잘못된 헤더는 여전히 `INVALID_BETA_HEADER`로 거부되며 upstream 시도가 없는지, 원문 헤더 텍스트가 진단에 남지 않는지를 확인한다. 외부 추론 요청·실제 인증 조회·실제 Claude 실행은 0이다.

## 5. 정상 종료 증거 — 감독하 PASS 확정

세션 46b6af24를 정상 종료해 얻은 종료 JSON의 `cleanup`이 9개 항목 모두 true였다.

`childClosed`, `gatewaySocketsClosed`, `gatewayJobsClosed`, `gatewayTimersClosed`, `gatewayDeliveriesClosed`, `gatewayIdle`, `gatewayCleanupCompleted`, `transportSocketsClosed`, `transportRequestsClosed`.

이로써 3장의 세 조건이 모두 충족됐다. **감독하 판정을 CONDITIONAL에서 PASS로 올린다.**

범위를 분명히 한다. PASS는 **감독하 사용**에 대한 것이며, 한 번의 실제 실패와 그 복구가 계약이 요구한 세 조건을 만족했다는 뜻이다. 다음은 여기에 포함되지 않는다.

- 무인 연속 개발은 계속 HOLD다. 하나의 실행에서 새 기능 3개의 무개입 주기를 마친 증거가 없다.
- 웹 검색은 상류 제약으로 동작하지 않는다. 최초 `UNSUPPORTED_EVENT other/identifier`의 원인도 여전히 미확정이다.
- 자동 압축 실발동, 3주기 종료 후 자원 실측, 동적 symlink 검사는 그대로 미검증·차단이다.

## 6. WebFetch — 동작한다. beta 가설과 텍스트 가설은 둘 다 틀렸다

세션 4851a91a에서 요청 번호로 귀속을 잡아 측정했다. WebFetch 시도 전후의 `lifetime.started`를 찍고 그 사이에 생긴 요청만 골랐다.

| 요청 | effort | `firstTextDeltaMs` | 역할 |
|---|---|---|---|
| 15 | max | null | 모델이 WebFetch를 호출하기로 결정한 턴. 도구 호출만 있고 텍스트 없음 |
| 16 | **high** | 975.02 | **apply 호출.** 가져온 페이지에 prompt를 적용하는 요청 |
| 17 | max | null | 도구 결과를 받은 다음 턴 |

**WebFetch 한 번이 요청 3개를 만들고, `effort: high`인 것이 apply 호출이다.** `https://example.com`에 대해 정상 성공했고 결과는 `Example Domain`이었다. 세 요청 모두 게이트웨이에서 성공 처리됐고 실패 기록도 없다.

### 틀린 가설 두 개를 기록해 둔다

1. **"UNSUPPORTED_BETA가 WebFetch를 죽였다"** — 틀렸다. 이번 세션의 `judgedBetaLabels`는 전부 빈 배열인데 첫 WebFetch는 똑같이 실패했다. 이전 세션의 `UNSUPPORTED_BETA` 실패는 실재하지만 어느 기능의 것인지는 미상이다. 당시 상대 세션이 "도구별 correlation이 없어 단정할 수 없다"고 명시했는데 그것을 확정으로 취급한 것이 오류였다.
2. **"apply 호출이 텍스트를 내지 못했다"** — 틀렸다. 실패한 시도의 apply 호출도 `firstTextDeltaMs: 2755.43`으로 텍스트를 냈다.

두 오류의 공통 원인은 같다. **어느 요청이 어느 도구의 것인지 귀속되지 않은 상태에서 추론했다.** 요청 번호 구간으로 귀속을 잡자 한 번에 확정됐다.

### 남는 것

첫 시도가 `No response from model`로 끝난 원인은 미확정이다. 그 값은 오류가 아니라 설치 바이너리의 apply 코드가 `content[0]`에 `text`가 없을 때 반환하는 값이다.

세 번째 가설인 "대상 페이지 특성"도 탈락했다. 상대 세션에 확인한 결과 **실패한 첫 시도의 URL도 `https://example.com`으로 동일**했다. 같은 URL·같은 게이트웨이·같은 경로에서 한 번은 실패하고 한 번은 성공했으므로 **비결정적**이다. 두 시도 모두 apply 호출이 텍스트를 냈고(2755.43ms / 975.02ms) 게이트웨이는 둘 다 성공 처리했다.

여기서 멈춘다. 같은 목표로 세 번 추론해 세 번 다 틀렸고 새 증거가 없다. **비결정적·원인 미확정으로 남긴다.**

다음에 재발했을 때 한 번에 끝내려면 전달한 응답의 콘텐츠 블록 종류(text / tool_use / redacted_thinking의 개수)를 고정 라벨로 기록하는 진단이 필요하다. 지금 진단으로는 `firstTextDeltaMs`가 텍스트 델타의 유무만 알려줄 뿐 최종 `content[0]`이 무엇이었는지는 알 수 없다. 이번에는 추가하지 않았다.

이 결과는 감독하 PASS 판정을 바꾸지 않는다. 게이트웨이는 세 요청을 모두 정상 처리했고 거부하거나 실패시킨 것이 없다.
