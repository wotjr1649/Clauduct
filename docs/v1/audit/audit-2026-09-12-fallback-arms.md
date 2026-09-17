# 감사 — native fallback 차단: 통제 실험

2026-09-12. 같은 자극, 설정 하나만 다른 두 세션. **두 팔이 갈렸다.**

## 1. 설계

분기는 Claude 바이너리 안에 있어 직접 관측할 수 없다. 바깥에서 보이는 것은 스트림이 깨진 뒤 클라이언트가 **비스트리밍 요청을 보내는가**뿐이고, 게이트웨이는 그것을 `REQUEST_STREAM_FALSE`로 이름 붙여 prepare 단계에서 거부한다.

자연 발생을 기다리는 대신 게이트웨이가 자극을 만든다. `--verify-fallback blocked|allowed`는 콘텐츠가 이미 클라이언트에 전달된 뒤 `{type:'error', error:{type:'api_error', code:'server_error'}}`를 게이트웨이당 한 번 주입한다. 두 팔의 자극은 동일하고, 자식이 읽는 `CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK`만 다르다.

**idle timeout을 주입하지 않는 이유**는 사전 확인에서 나왔다. 설치된 클라이언트의 조건식은 이렇다(읽기 전용 조회, 실행하지 않음).

```js
dg = (P("tengu_watchdog_skip_nonstreaming_fallback", !1) || a.CLAUDE_CODE_REMOTE) && Jk
   || Oe(process.env.CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK)
   || P("tengu_disable_streaming_to_non_streaming_fallback", !1)
```

바로 뒤의 `Zb = Jk ? Error("Stream idle timeout ...")`가 `Jk`의 정체를 말해 준다 — **idle timeout 여부**다. 즉 1항은 idle timeout일 때만 작동한다. idle을 주입했다면 검사 대상인 2항이 말하기 전에 1항이 실험을 결정했을 것이다. 2항은 독립 OR이고 3항은 기본값이 false다.

대조군은 키를 `'0'`으로 쓰지 않고 **제거한다.** `Oe()`가 `'0'`을 어떻게 읽는지는 클라이언트의 파서 사정이며 실험이 거기 기대면 안 된다.

## 2. 관측

| | 실험군 `68c2bd7e` (blocked) | 대조군 `6d309884` (allowed) |
|---|---|---|
| 자극 | 동일 (주입 1회) | 동일 (주입 1회) |
| 자식이 읽는 설정 | `...DISABLE_NONSTREAMING_FALLBACK=1` | 키 없음 |
| 클라이언트가 보인 것 | `API Error: UPSTREAM_ERROR_EVENT event=error upstream_code=server_error upstream_type=api_error: Partial respon…` | `API Error: 400 UNSUPPORTED_REQUEST request=REQUEST_STREAM_FALSE` |
| 비스트리밍 전환 | **없음** | **시도됨 — 게이트웨이가 거부** |

실험군은 원래 스트리밍 오류를 그대로 올리고 멈췄다. 이것이 공식 문서와 바이너리 조사가 말한 비활성 분기의 동작이다 — "원래 오류를 throw한다".

대조군은 스트림이 깨지자 `stream: false` 요청을 새로 보냈고, 게이트웨이가 `REQUEST_STREAM_FALSE`로 거부했다. 이것이 활성 분기다.

**두 분기가 같은 자극 아래 서로 다른 결과를 냈다.** run-04의 음성 관측("전환이 없었다")과 달리, 이번에는 전환이 **일어나는 조건**을 함께 확보했으므로 "전환 조건이 아니었을 가능성"이 배제된다.

## 2.1 종료 JSON — 같은 자극, 갈린 결과

두 세션 모두 `injectedStreamErrors: 1`, 모델 `gpt-5.6-luna`/effort `max`, `cleanup` 9개 전부 true다. 자극도 경로도 같았다.

| | 실험군 `68c2bd7e` | 대조군 `6d309884` |
|---|---|---|
| `nonStreamingFallbackDisabled` | `true` | `null` (키 없음) |
| `injectedStreamErrors` | 1 | 1 |
| `started` / `succeeded` / `failed` | 2 / 1 / 1 | 3 / 1 / 2 |
| `failuresByStage.upstream` | 1 | 1 |
| `failuresByStage.prepare` | **0** | **1** |

**차이는 요청 하나다.** 대조군에만 있는 요청 5가 그것이다.

```
request 5  failureStage: prepare   requestFailure: REQUEST_STREAM_FALSE
           failureCategory: UNSUPPORTED_REQUEST   attempts: []
```

주입(요청 4) 뒤 4.2초 만에 클라이언트가 `stream: false` 요청을 새로 보냈고, prepare 단계에서 거부돼 upstream에 닿지도 않았다(`attempts` 비어 있음). 실험군에는 이 요청이 아예 없다.

주입이 콘텐츠 뒤에 걸린 것도 기록에 남아 있다 — 요청 4의 `firstTextDeltaMs`가 실험군 4823.72, 대조군 4180.31이고 `firstDownstreamWriteMs`가 그 직후다. 클라이언트는 부분 응답을 이미 받은 상태에서 오류를 만났다.

## 3. 한계

- 여전히 **효과의 관측**이지 분기 실행의 직접 관측이 아니다. 바이너리 내부는 볼 수 없다.
- `P()` 게이트가 원격 조회라면 기본값이 쓰였다고 가정한다. 게이트 서비스가 닿는 환경에서는 달라질 수 있다.
- 이 판정은 이 클라이언트 버전에 대한 것이다. 조건식이 바뀌면 다시 돌려야 하며, `--verify-fallback`은 그래서 코드에 남겨 두었다.
- 실험군의 결과는 "fallback이 차단됐다"를 보이지만 그 세션의 원래 오류가 **주입된 것**임을 잊지 않는다. 이 두 세션의 종료 JSON은 실제 장애 기록이 아니다. `lifetime.injectedStreamErrors`가 그것을 구분한다.

## 4. 재현

```
clauduct.cmd --model luna --effort max --verify-fallback blocked
clauduct.cmd --model luna --effort max --verify-fallback allowed
```

각 세션에 아무 짧은 프롬프트 하나를 보내면 첫 응답 도중 주입이 걸린다. 인자 없이는 돌지 않으며 기본 실행은 주입하지 않는다(`src/test-fallback-verification.mjs` 19개 검사).
