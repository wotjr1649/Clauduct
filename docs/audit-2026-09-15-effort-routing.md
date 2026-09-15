# 감사 — effort 상승 경로와 추론 지연 여지

2026-09-15. 판정: **기록된 요청의 `high`/`max`는 결함이 아니라 사용자가 실행 시 지정한 값이다.** 다만 사용자가 지정하지 않는 자리에 effort를 최댓값으로 올리는 경로가 별개로 존재하며, 그중 하나는 "명시적으로 싼 모델을 고른 자식이 가장 비싼 effort를 받는다"는 형태다.

이 문서의 `파일:줄` 인용은 main `e75051e` 기준으로 전수 확인했다.

1~2장은 판정만 기록한다. 3장의 후보 중 사용자가 채택한 것은 6장에 적용 결과를 적었다.

## 1. effort가 정해지는 자리 — 전수

| # | 지점 | 코드 | effort의 출처 |
|---|---|---|---|
| 1 | 런처 시작값 | `src/clauduct.mjs:128` | `--effort` 명시값 > (`--model` 명시 시) 모델 기본값 > `DEFAULT_SELECTION.effort='low'` |
| 2 | 런처→native 전달 | `src/clauduct.mjs:213` | 1의 결과를 `--effort <값>`으로 claude.exe에 넘긴다 |
| 3 | 메인 요청 | `src/native-protocol.mjs:258` | `doc.output_config.effort` (native가 보낸 값) ?? 모델 기본값 |
| 4 | 턴별 재지정 | `src/native-protocol.mjs:345-347` | system 메시지의 `output_config.effort`. `!subagent && !route`일 때만 적용 |
| 5 | 자식 요청 | `src/native-protocol.mjs:258`의 `subagent ? undefined : ...` | **`doc.output_config.effort`를 무시한다.** route가 있으면 route, 없으면 모델 기본값 |
| 6 | 자식 route — 명시 모델 | `src/agent-selection.mjs:467`의 `model(selected)` → `src/models.mjs:13-16` | effort 인자 없이 `selectModel` 호출 → **해당 모델의 기본 effort** |
| 7 | 자식 route — 정의/상속 | `src/agent-selection.mjs:41, 116` | 런처 `--agents` 정의값 / 부모 route 그대로 |
| 8 | 자식 route — 역할 기본값 | `src/agent-selection.mjs:476` → `src/models.mjs:20` | `ROLE_MODELS`: Explore=luna/**max**, Plan=sol/**xhigh**, general-purpose=luna/**max**. Plan은 6장에서 astra/low로 바꿨다 |
| 9 | Workflow 자식 | `src/workflow-selection.mjs:293-294` | 요청 effort ?? (모델이 부모와 같으면) 부모 effort |
| 10 | compact 템플릿 | `src/native-protocol.mjs:423-424` | **유일한 하향.** `purpose==='compact-template'`이고 effort가 low/medium이 아니면 medium으로 낮춘다 |

**코드베이스 전체에 effort를 올리는 무조건 분기는 없다.** 10번만이 조건부 하향이다. `high`/`max`는 전부 "지정된 값" 또는 "모델·역할 기본값"이다.

프롬프트가 참조한 `docs/audit-2026-09-08.md:266`의 서술(일반 routing 이후 high/xhigh/max를 medium으로 낮춘다)은 **위치가 낡았다.** 현재 `src/compact-policy.mjs`에는 effort 로직이 없고 템플릿 형태 판별(`inspectCompactTemplate`)만 있다. 하향 로직은 `src/native-protocol.mjs:423-424`로 옮겨져 있으며 동작 자체는 그대로다.

## 2. 관측된 16건의 `high` — 판정: 의도된 값

### 근거 A — 실측: 어떤 기본값도 sol에 `high`를 주지 않는다

`--dry-run`(모델 호출 없음)으로 직접 확인했다.

```
node src/clauduct.mjs --dry-run                           → gpt-6-astra / low
node src/clauduct.mjs --dry-run --model sol               → gpt-5.6-sol / xhigh
node src/clauduct.mjs --dry-run --model sol --effort high → gpt-5.6-sol / high
```

sol의 기본값은 `xhigh`다(`src/models.mjs:6`). 관측값이 `high`라는 것은 `--effort high`가 명시됐다는 뜻이다.

### 근거 B — 기록: 그 실행의 명령줄이 남아 있다

- `docs/audit-2026-09-11-three-cycle-run-03.md:10`, `docs/audit-2026-09-11-three-cycle-run-04.md:12`: `clauduct.cmd --model sol --effort high` **(사용자 지정)**
- `docs/audit-2026-09-12-workflow-routing.md:59`: "`effort: high`는 스크립트가 준 값이 아니다. 옵션에 effort는 없었고 부모 세션이 `--effort high`였다."

해당 감사가 인용한 req 10/11(luna/high, terra/high, `workflow-result`)은 문제의 snapshot에 있는 값과 완전히 일치한다. 즉 `2026-09-12T04:59:52` snapshot은 그 감독하 관측 실행의 기록이다.

### 근거 C — 기록된 전 구간에서 `DEFAULT_SELECTION`은 한 번도 적용 대상이 아니었다

8개 snapshot을 (sessionRef, request, startedAt)로 중복 제거하면 고유 요청은 **34건**이다(프롬프트의 16건은 그중 한 snapshot).

| 요청 모델 | requestedEffort → effort | 건수 | 출처 |
|---|---|---|---|
| gpt-5.6-sol | high → high | 14 | 메인 (`--effort high`) |
| gpt-5.6-luna | max → max | 14 | 메인 (luna 기본값 또는 `--effort max`) |
| gpt-5.6-luna | high → high | 1 | workflow-result (부모 effort 상속) |
| gpt-5.6-terra | high → high | 1 | workflow-result (부모 effort 상속) |
| gpt-5.6-luna | (미완료) | 4 | model 미기록 |

`DEFAULT_SELECTION.effort='low'`는 `--model`과 `--effort`를 **둘 다** 생략했을 때만 적용된다(`src/clauduct.mjs:128`). 34건 전부 `--model`이 명시된 실행이므로 애초에 적용 대상이 아니다. 이 규칙은 `README.md:17`, `docs/native.md:5`, `RELEASE.md:28`에 동일하게 문서화돼 있다.

`requestedEffort`와 `effort`가 34건 전부 같다 — 게이트웨이는 아무것도 바꾸지 않았다. 10번 하향도 발동하지 않았다(전부 `purpose: conversation`).

**결론: 결함 아님. 기본값 미적용도 아님. 사용자가 그 세션에 대해 high/max를 선택한 결과다.**

## 3. 사용자가 지정하지 않는 자리의 상승 — 여기가 실제 레버

1~2장과 달리, 아래 세 지점은 실행 시 `--effort`와 무관하게 effort를 올린다.

### 3-1. 자식이 명시 모델을 고르면 그 모델의 기본 effort가 붙는다 — 개선 후보 1순위

`src/native-protocol.mjs:258`의 `subagent ? undefined : doc.output_config?.effort`가 자식 요청의 effort를 버리고, `src/agent-selection.mjs:467`의 `model(selected)`가 effort 인자 없이 `selectModel`을 부른다. 결과는 모델 기본값이다.

실측(합성 요청, 모델 호출 없음):

```
prepareNative({ model:'gpt-5.6-luna', output_config:{ effort:'low' }, ... })
  subagent:false → reasoning.effort = low
  subagent:true  → reasoning.effort = max
```

`Agent({ model: 'haiku' })`는 별칭이 luna로 풀리고(`src/agent-selection.mjs:20,25-27`) luna의 기본 effort는 `max`다(`src/models.mjs:8`). **가장 싸고 빠른 모델을 명시적으로 고른 자식이 가장 비싼 effort를 받는다.** 부모가 `--effort low`여도 그렇다.

이 공백 자체는 `docs/gpt-agent-selection-contract.md:53`에 이미 기록돼 있다(당시엔 `inherit` 관점). 위 실측은 `inherit`가 아닌 **명시 모델**에서도 같은 일이 일어남을 보인다.

### 3-2. 역할 기본값이 전부 최상단이다 — Plan은 6장에서 변경했다

`src/models.mjs:20` — Explore=luna/**max**, general-purpose=luna/**max**, Plan=sol/**xhigh**.

Explore는 읽기 전용 검색 에이전트다. grep/glob 훑기에 `max`가 필요하다는 근거는 없다. 역할별 하향의 선례는 이미 코드에 있다 — compact 템플릿은 `medium`으로 내린다(`src/native-protocol.mjs:423-424`).

**단 이 경로는 기록된 34건에서 한 번도 발동하지 않았다**(role이 전부 null 또는 workflow-subagent). 실측 근거 없음, 코드 근거만 있다.

### 3-3. 모델 기본값 자체가 높다

sol=xhigh, luna=max(`src/models.mjs:6,8`). `--model luna`만 치면 `max`다. 이것은 의도이고 문서화돼 있다(`README.md:17`). 바꾸려면 정책 결정이 필요하다.

### 공식 문서 근거

[OpenAI Reasoning guide](https://developers.openai.com/api/docs/guides/reasoning) (2026-09-15 확인): "Lower effort favors speed and lower token usage, while at higher effort the model thinks more completely to provide higher quality responses." `low`는 "efficient reasoning with a modest latency increase … optimizing for speed and cost", `medium`은 "a well-balanced point on the pareto curve of latency, performance and cost", `xhigh`는 "long runs" where "quality and intelligence matters more than latency"로 설명된다. 같은 문서가 "the models also reason adaptively across reasoning efforts, using fewer tokens for simpler tasks"라고도 적는다 — **effort를 내려도 절감량은 과제 복잡도에 따라 달라진다.**

## 4. 추론 시간 — 표본을 넓혔고, 여전히 인용에 주의가 필요하다

프롬프트의 n=2를 snapshot 8개 전체로 넓혔다. 고유 요청 34건 중 `firstTextDeltaMs`가 있는 것은 **6건**이다.

| 시각 | 모델/effort | TTFT | 추론 | 텍스트 생성 | 합계 |
|---|---|---|---|---|---|
| 09-11T21:41:33 | luna/max | 1,946 | 4,096 | 1,259 | 7,301 |
| 09-11T21:41:45 | luna/max | 890 | 3,358 | 1,288 | 5,536 |
| 09-11T21:42:00 | luna/max | 926 | 15,127 | 3,651 | 19,704 |
| 09-11T21:45:20 | luna/max | 992 | 7,191 | 1,232 | 9,414 |
| 09-11T23:51:09 | sol/high | 1,624 | 13,640 | 18,232 | 33,497 |
| 09-11T23:54:43 | sol/high | 1,198 | 5,876 | 1,420 | 8,494 |
| **합계** | | **7,575 (9.0%)** | **49,288 (58.7%)** | **27,082 (32.3%)** | 83,945 |

단위 ms. 추론 = `firstTextDeltaMs - firstEventMs`, 생성 = `transportFinishedMs - firstTextDeltaMs`.

n=2의 46.5%보다 높은 58.7%가 나왔다. **추론이 상류 시간의 절반 이상이라는 방향은 유지된다.**

### 이 수치를 인용할 때 반드시 함께 적을 것

1. **n=6이고, 결측이 무작위가 아니다 — 이후 계측을 고쳤다.** `firstTextDeltaMs`는 `response.output_text.delta`에서만 기록되므로 **텍스트 없이 도구 호출로 끝난 턴에는 존재할 수 없다.** 34건 중 text 블록이 0인 18건은 전부 결측이고, text가 있는 8건 중 6건에 값이 있다. 즉 이 분해는 텍스트로 끝난 소수 턴만 본 것이다.

   수정: reasoning이 아닌 첫 `response.output_item.added` 시각을 `firstOutputItemMs`로 기록한다. 같은 경계(추론 끝, 출력 시작)이면서 뒤에 무엇이 오든 존재한다. 합성 도구 호출 턴(`text: 0, toolUse: 1`)에서 `firstTextDeltaMs: null`인 채 `firstOutputItemMs`가 잡히는 것을 확인했다. **모델 호출 0회이며, 다음 실행부터 실사용 데이터가 전 구간에서 쌓인다.** 위 58.7%는 고치기 전 표본이므로 그대로 편향된 값이다.

2. **두 arm이 교란돼 있다.** luna/max 4건과 sol/high 2건은 서로 다른 세션·다른 과제다. 이 데이터로 "effort를 내리면 얼마나 빨라지는가"는 **측정할 수 없다.**
3. 로컬 처리 51ms(0.02%) 결론은 이 표본 문제와 무관하게 유지된다 — 그것은 34건 전부에 기록된 필드로 계산된 값이다.

### 개선 여지 판정

- **측정된 것:** 추론이 텍스트 종료 턴 상류 시간의 58.7%(n=6, 편향 있음). effort는 그 구간의 유일한 직접 조절 손잡이다.
- **추정(미측정):** effort를 한 단계 내렸을 때의 실제 단축폭. 공식 문서가 방향은 보증하지만 폭은 과제 복잡도에 따른다고 명시한다. **수치를 주장하려면 같은 과제·같은 프롬프트로 effort만 바꾼 A/B 실행이 필요하며, 이는 live 호출 비용이 든다.**
- **비용 없이 얻을 수 있는 것:** 3-1의 자식 effort 역전은 합성 요청으로 재현·수정·검증이 가능하다. 지연 개선 여부와 무관하게 "low를 요청한 자식이 max를 받는다"는 것 자체가 손볼 가치가 있다.

## 5. 채택한 결정 — 3-1의 전제가 뒤집혔다

3-1을 "자식 요청의 effort를 살릴지"의 문제로 적었으나, 후속 확인에서 **Agent 호출에는 effort를 명시할 입력 필드 자체가 없다**는 것이 확인됐다.

- `src/agent-selection.mjs:104-113`의 `remember()`가 읽는 입력은 `model`, `subagent_type`, `skill`, `to/message`, `script`뿐이다.
- Agent 도구 스키마 자체가 `description / isolation / model / prompt / subagent_type` 5개다.
- `docs/gpt-agent-selection-contract.md:69`(공식 subagents 문서 확인 기록): effort는 **정의(definition)** 수준에서만 설정한다.
- 그리고 정의 수준의 effort 존중은 `src/agent-selection.mjs:41`이 이미 구현하고 있다.

즉 3-1은 "게이트웨이가 값을 버린다"가 아니라 **"명시할 채널이 정의 하나뿐인데 그 채널이 모델 기본 effort 1개만 노출했다"**가 정확한 진단이다. 자식 요청의 `output_config.effort`를 살리는 안은 채택하지 않았다 — 실측상 그 값은 자식의 명시값이 아니라 부모 세션의 effort가 곱대로 실려 온 값이고(workflow 자식 luna가 `high`, luna 기본값은 `max`, 부모가 `high`), sidecar에 effort가 없어 대조 증거도 없다.

## 6. 적용한 변경

**정의를 effort별로 넓혔다.** `--gpt-agents` 정의가 5개에서 14개로 바뀜다. 이름은 전부 `clauduct-<모델>-<effort>`이며 bare 이름은 제거했다.

| | |
|---|---|
| astra / sol / terra | low, medium, high, xhigh (각 4개 = 12개) |
| luna | max 1개 |
| inherit | 변경 없음 |

규칙은 `src/clauduct.mjs`에 한 줄로 있다 — `name === 'luna' ? ['max'] : EFFORTS.filter(e => e !== 'max')`. 별도 테이블 없이 "max는 luna 전용"이 한 곳에만 적혀 있다.

**`ROLE_MODELS.Plan`을 sol/xhigh → astra/low로 바꿨다**(`src/models.mjs:18-21`). Explore와 general-purpose는 luna/max 그대로다. Plan은 `DEFAULT_SELECTION`을 alias하지 않고 자기 값을 적는다 — 두 규칙은 값이 같을 뿐 독립적으로 바뀔 수 있어야 한다.

**설명문은 바꾸지 않았다.** "Select this agent type when that model choice is requested" 그대로다. 따라서 **이 변경은 자동 effort 절감이 아니다** — 사용자가 이름을 지정해야 효과가 난다. 모델이 스스로 낮은 effort를 고르게 하는 안은 검증 방법이 없어 채택하지 않았다.

**`--gpt-agents` 옵션을 제거하고 기본 등록으로 바꿨다.** 사용자의 기본 사용 방식이 옵션 없는 `clauduct` 단일 실행이므로, opt-in을 유지하면 위 14개가 평소 워크플로에서 도달 불가능했다. 정의는 이제 모든 실행에 등록된다. Read 전용 probe 5개는 `--verify-agent-models`로 남는다.

옵션을 지우면서 `--gpt-agents`를 `blockedOptions`에 넣었다. 그러지 않으면 제거된 래퍼 옵션이 알 수 없는 native 옵션으로 **claude.exe에 전달되어** 자식이 엉뚱하게 실패한다. 이제 래퍼에서 `INVALID_ARGUMENTS`로 거부된다.

기본 등록의 근거:
- 원래 opt-in이던 사유(`docs/gpt-agent-selection-contract.md:107` — "Read 전용 시험 권한을 넘으므로 범위를 확정하기 전 적용하지 않는다")는 권한 범위 확정 전의 보수적 단계였고 이미 해소됐다.
- [공식 subagents 문서](https://code.claude.com/docs/en/sub-agents) 확인(2026-09-15): `--agents`는 우선순위 2로 프로젝트(3)·사용자(4) 정의보다 높지만 **이름이 같을 때만** 이긴다. 병합을 막거나 대체하지 않으며, 등록 이름이 전부 `clauduct-` 접두사라 충돌 경로가 없다.
- 비용 실측: 모델이 매 턴 보는 목록 약 1,886자(≈540 토큰), `--agents` JSON 9,736 bytes, cmdline 11,685 / 32767.

### 검증 — AdGuard를 끄고 전수 재실행 (keep-alive 변경 포함 최종본)

`--dry-run`(옵션 없음)에서 14개 이름을, `--dry-run --gpt-agents`에서 `INVALID_ARGUMENTS` 거부를 확인했다. `--agents` JSON 9,736 bytes · cmdline 11,685 / 32767.

AdGuard를 끈 뒤 세 단계를 전수 실행했다.

| 단계 | 결과 |
|---|---|
| `src/test-*.mjs` 23개 | **23 PASS / 0 FAIL** |
| `verification/test-*.mjs` 4개 | **4 PASS / 0 FAIL** |
| `poc/test-*.mjs` 6개 | **6 PASS / 0 FAIL** |
| PowerShell (`src/test-run-node-tests.ps1`) | **PASS** |

`verification/test-dotnet-http-transport.mjs`는 처음에 `pwsh.exe` spawn ENOENT로 실패했다. 원인은 런타임 핀 두 겹이었고 아래에 따로 적었다.

**AdGuard가 켜진 동안 회귀 2건이 가려져 있었다.** loopback이 죽어 해당 파일이 assertion에 도달하지 못했기 때문이다. AdGuard를 끄자 baseline이 clean(chat 28/0, native 47/0)으로 나오면서 드러났다.

| 회귀 | 원인 | 수정 |
|---|---|---|
| `test-chat.mjs` `launch_preserves_cwd_and_hides_secret` | `launch.args.at(-1)`을 settings JSON으로 가정했는데, `--agents`가 항상 그 뒤에 붙으면서 꼬리가 바뀌었다 | `indexOf('--settings') + 1`로 위치를 찾게 했다 |
| `test-native.mjs` 2건 | `Plan` 역할 기대값이 `MODELS.sol.model`로 하드코딩돼 있었다 | `ROLE_MODELS.Plan.model`로 바꿔 역할 기본값이 또 바뀌어도 따라가게 했다 |

이 두 건은 **간헐적 loopback 실패를 "환경 탓"으로 분류하면 진짜 회귀를 놓칠 수 있다는 실례**다. 원인 분류만으로는 부족하고, 차단을 끈 상태의 전수 실행이 필요했다.

## 7. AdGuard loopback filter — 2026-09-14의 "실사용 무영향" 결론을 뒤집는다

`docs/native-startup-diagnostics-2026-09-14.md`(commit `d014b1c`)는 필터를 켠 채 측정해 **"실사용 경로는 영향이 없다. filter가 건드리는 것은 기형 요청뿐이다"**로 끝맺었다. 그 측정은 유효하지만 **응답의 연결 종료 방식을 다루지 않았다.** 그 축에서 실사용 경로가 깨진다.

### 측정 (필터 ON/OFF 동일 스크립트, 모델 호출 0, 로컬 127.0.0.1 전용)

| 항목 | 필터 OFF | 필터 ON |
|---|---|---|
| 순차 POST | 40/40 | **40/40** |
| 병렬 POST (25×4) | 100/100 | **100/100** |
| chunked SSE (GET, keep-alive) | 8 frame, 202~207ms | **8 frame, 202~205ms** |
| 16초 유휴 SSE | 3/3, 정상 종료 | **3/3, 정상 종료** |
| 기형 요청(대조군) | Node `400`, 47 bytes | **AdGuard 차단 페이지 161,372 bytes** |
| **chunked SSE + keep-alive** | — | **80/80** |
| **chunked SSE + `Connection: close`** | — | **49/80 (61%)** — ECONNRESET·TimeoutError |

### 원인

`Connection: close`와 chunked를 함께 쓰는 응답이 필터를 지날 때 종료 처리가 깨진다. 정상 FIN이 RST로 바뀌거나 간헐적으로 멈춘다. 같은 응답을 keep-alive로 보내면 80/80이다. 요청 형태(POST/GET, 본문 크기 8KB, `Authorization`·`anthropic-version`·`x-claude-code-*` 헤더), 포트(0·3000·5000·8080·8443·8700·8888·9066·9090·49200·55000 전부 12/12)는 무관하다.

### 왜 실사용에 닿는가

- `src/native-gateway.mjs:374`가 **모든** SSE 응답에 `Connection: close`를 붙이고 있었다. 오류 응답(`:100`)과 204(`:148`)도 같았다. 조건부가 아니었다. 줄 번호는 아래에서 고친 뒤의 현재 위치다.
- `src/clauduct.mjs:177`이 `CLAUDE_CODE_MAX_RETRIES: '0'`을 설정한다. **재시도가 없다.**

실제 게이트웨이로 확인했다 — 게이트웨이는 `started 5 / succeeded 5 / failed 0`으로 전부 성공 기록을 남겼는데 **클라이언트는 3/5만 받았다.** 게이트웨이의 성공 카운터는 이 손실을 보지 못한다.

### 이 판정의 한계

위는 전부 Node `fetch` 클라이언트 관측이다. **실제 `claude.exe`가 같은 비율로 잃는지는 직접 확인하지 않았다** — 실행에 모델 호출 비용이 든다. 기전(게이트웨이가 무조건 `Connection: close`, 재시도 0)과 전송 계층 측정에서 **추론한 것**이며 end-to-end 실측이 아니다.

테스트 전수에서도 같은 크기로 나타난다. 필터 ON에서 `src/test-*.mjs` **11/23 실패**, OFF에서 **0/23 실패**. 실패한 파일은 전부 게이트웨이 loopback을 쓰는 것들이다.

### 적용한 수정 — `Connection: close` → keep-alive

사용자가 범용 호환 계층 대신 이 한 곳을 고치기로 정했다. `src/native-gateway.mjs`의 세 곳(`:100` 오류 응답, `:148` 204, `:374` SSE)을 `keep-alive`로 바꿨다.

**원래 의도는 기록이 없다.** `git log -S`로 추적하면 최초 baseline 커밋 `cd84c9e`부터 존재하며 주석도 근거 문서도 없다.

**정리 계약은 이 헤더에 의존하지 않는다** — 가정하지 않고 측정했다. `close()`(`:557-568`)가 추적 소켓을 전부 `destroy()`하고 각 `close` 이벤트를 기다린다. 유휴 keep-alive 소켓은 Node 기본 `keepAliveTimeout` 5초로 회수된다.

| 측정 (필터 ON, 실제 게이트웨이, 20요청) | close | keep-alive |
|---|---|---|
| 클라이언트 수신 | 14/20 | **20/20** |
| `openSockets` | — | **0** |
| 잔여 핸들 · 프로세스 exit | — | **0개 · exit 0** |
| `close()` 소요 | — | **1ms** |
| `src/test-*.mjs` 실패 | 11/23 | **3/23** |

`close()` 1ms는 부수 효과다. 2026-09-14 문서가 기록한 "종료 처리의 1000ms fallback이 공개 HTTP 비교 3회 중 2회 작동"은 지연된 FIN에 peer-drain 마감이 걸린 것이었고, keep-alive에서는 발생하지 않는다.

### 필터 ON에서 남는 테스트 3건 — 고칠 수 없다고 판정했다

`test-chat`, `test-client-version`, `test-native-gateway`. 셋 다 필터 ON에서 5회 전부 실패하고(0/5), 필터 OFF에서는 전부 통과한다. 각각 수정을 시도했고 **세 건 모두 되돌렸다.**

**`test-client-version` — 고쳤다가 되돌렸다.** `poc/codex-transport.mjs:91`의 `agent: false`가 원인이었다(Node가 일회성 연결에 `Connection: close`를 붙인다). keepAlive agent로 바꾸자 0/5 → 5/5가 됐다. 그러나 필터를 끄고 전수를 돌리니 `poc/test-claude-read-once.mjs`의 `read_normal_truncated`가 깨졌다(원복 후 3/3 통과로 확인).

그 시험(`poc/test-claude-read-once.mjs:71`)은 Content-Length를 실제 본문보다 10바이트 크게 선언한다. `Connection: close`로 연결이 닫히기 때문에 짧은 본문이 `UPSTREAM_IO_ERROR`로 잡힌다. keep-alive면 남은 10바이트를 계속 기다린다. **`agent: false`는 절단 탐지를 떠받치는 동작이었다.** 필터 호환을 위해 실제 결함 탐지를 없애는 거래이므로 되돌렸다. 운영 transport(`src/native-transport.mjs:111`)는 이미 `keepAlive: true`이고 다른 방식으로 절단을 검증한다.

**`test-chat` — PoC 게이트웨이가 구조적으로 요청당 1연결이다.** `poc/gateway.mjs:91`이 현재 요청의 소켓이 아닌 것을 전부 `destroy()`하고 `:9`에 연결 예산이 있다. 헤더만 keep-alive로 바꾸면 **더 나빠진다** — 실측 4건 실패 → 8건, `socket hang up`. keep-alive를 광고하면서 소켓을 닫으니 클라이언트가 죽은 소켓을 재사용한다. 고치려면 PoC의 연결 모델 재설계가 필요하고 운영 코드가 아니다.

**`src/test-native-gateway.mjs:383` — 원리적으로 불가능하다.** `transportRejections`의 정확한 개수를 단언하는데 필터가 같은 포트로 자기 트래픽을 보낸다. 델타 단언으로 바꿔도 실패가 다른 줄로 옮겨갈 뿐이다(delta actual 2, expected 1) — 필터 트래픽이 측정 구간 안에도 들어온다. 임의의 다른 트래픽이 같은 포트에 도달하는 한 "내 행위가 만든 거부 수"는 셀 수 없다. `>= 1`로 낮추면 통과하지만 "정확히 하나"라는 검증이 사라진다. 이득 없이 단언만 약해지므로 되돌렸다.

**결론: 이 3건은 필터가 켜진 환경이 오염됐다는 사실을 정직하게 드러내는 것이며 제품 결함이 아니다.** 개발 중에는 필터를 끄는 것이 맞다.

### 실사용 end-to-end — 필터 ON에서 실제 `clauduct` 실행으로 확인

앞선 판정은 전부 Node 클라이언트 관측이었다. 사용자 승인 아래 실제 `claude.exe`를 띄워 확인했다. 두 실행 모두 AdGuard를 켠 상태다.

| 실행 | 결과 |
|---|---|
| `clauduct -p --model luna --effort low --max-turns 3 -- "Reply with exactly READY"` | `result: "READY"`, `is_error: false`, exit 0, `cleanup` 9/9 true |
| `clauduct -p --model luna --effort low --verify-agent-models --max-turns 6` (자식 1개 생성) | `subagent_stats: spawned 1 / completed 1 / failed 0`, `result: "MODEL-PROBE-COMPLETED"`, exit 0 |

게이트웨이 기록(두 번째 실행): started 5 / succeeded 4 / failed 1, `cleanup` 전부 true, `transportRejections 2`. 자식 요청 두 건(req 6·7)은 `selectionSource: definition-model`, luna/max로 정상 라우팅됐다. **ECONNRESET·응답 유실·`AGENT_SELECTION_UNVERIFIED`는 없었다.**

비용 0 probe도 같은 방향이다 — 필터 ON에서 `registerBinding` 20/20, `readRequestStatus` 20/20.

**판정: keep-alive 변경 이후 실사용 경로는 AdGuard를 켜도 동작한다.** 개발용 테스트 전수는 여전히 필터를 꺼야 한다(위 3건).

#### 이 실행에서 드러난 별개 결함 1건 — 이후 고쳤다

두 실행 모두 req 3이 `prepare` 단계에서 `OUTPUT_CONFIG_FIELDS` / `UNSUPPORTED_REQUEST`로 실패했다. native의 세션 제목 생성 요청이 `output_config`에 `format`을 싣는데 `src/native-protocol.mjs:259`가 `effort`만 허용했다. 본 작업은 정상 완료되지만 `requestOutcome`이 `has-failures`가 되어 무인 판정을 오염시킨다.

**클라이언트가 보내는 것** (claude.exe 2.1.270 정적 분석, side-query 빌더): `output_config: { format: { type:'json_schema', schema:{…} } }`. 제목 스키마는 `{title:string}` 하나이고 응답은 **클라이언트가 직접 파싱한다** — 게이트웨이가 `parsed_output`을 만들 필요는 없다.

**상류가 요구하는 것** ([Responses API structured outputs](https://developers.openai.com/api/docs/guides/structured-outputs)): `text: { format: { type, name, schema, strict } }`. `name`과 `strict`는 필수인데 **클라이언트는 둘 다 보내지 않는다.**

수정: 전송 계층이 둘을 채우고 `output_config.format` → `text.format`으로 옮긴다. **schema는 읽지 않고 통과시킨다** — JSON Schema 의미론은 상류 소관이고, 여기에 두 번째 약한 검증기를 두면 결정권을 가진 쪽과 어긋난다. 거부는 기존 설계대로 클라이언트에 `UNSUPPORTED_REQUEST` 고정 라벨만 나가고 진단에만 사유(`OUTPUT_FORMAT_SHAPE`/`FIELDS`/`TYPE`/`SCHEMA`/`NAME`)가 남는다.

| 검증 | 결과 |
|---|---|
| `src/test-native-protocol.mjs` 회귀 12건 | 변환 결과·schema 객체 동일성·effort 독립성·format 없을 때 body 불변·거부 6종 |
| 전수 (필터 OFF) | **33/33 + PowerShell PASS**, 회귀 0 |
| **실제 `clauduct -p` 실행** | **`requestOutcome: all-succeeded`, started 2 / succeeded 2 / failed 0** |

실제 실행의 req 3은 `judgedBetaLabels: ["STRUCTURED_OUTPUTS"]`와 함께 `success: true`였고 상류 오류가 없었다. **Codex backend가 `text.format`을 수용한다는 실측이다** — 합성 검사로는 얻을 수 없는 증거이고, 이 매핑의 유일한 미지수였다.

### 배경 — 실사용 경로가 아니다

`test-chat`, `test-client-version`, `test-native-gateway`. 셋 다 **변경 전에도 필터 ON에서 실패하던 파일**이다(11건에 포함).

`test-native-gateway.mjs:362`는 `lifetime.transportRejections === 0`을 단언하는데 실제값이 2다. `transportRejections`는 `clientError`/`connect`/`upgrade`/`checkContinue`/`checkExpectation`에서 증가한다 — 필터가 보내는 트래픽이 게이트웨이의 HTTP 경계를 건드린 결과이며, 2026-09-14 문서의 `checkExpectation` 실패와 같은 현상이다.

**확인했다.** 필터를 끈 전수 실행에서 세 파일 모두 통과했다 — src 23/23, verification 4/4, poc 6/6, PowerShell PASS. 셋 다 순수한 필터 artifact였고 이 변경이 더한 실패는 없다. `close()` 소요는 필터를 꺼도 2ms이며 `openSockets` 0, 잔여 핸들 0이다.

### 만들지 않은 것

범용 호환 계층(간섭 감지 후 전송 자동 전환·재시도)은 만들지 않았다. 알려진 실사용 실패 모드가 이 하나뿐이고 헤더 한 줄로 끝나기 때문이다. 나머지 알려진 필터 효과(비표준 `Expect` 제거, 잘못된 method에 400 페이지)는 test가 일부러 보내는 기형 요청에만 해당한다. 재시도 계층은 `src/clauduct.mjs:177`의 `CLAUDE_CODE_MAX_RETRIES: '0'`과 충돌하며, 이미 도구 효과가 발생한 뒤의 재전송은 중복 실행 위험을 만든다.

판별 스크립트는 `.tmp/adguard-loopback-check.mjs`, `.tmp/adguard-close.mjs`에 있다(추적하지 않음).

## 8. 남는 것

- **Plan을 astra/low로 내린 계획 품질 영향은 미측정이다.** 이득도 손실도 근거가 없다는 점을 확인한 뒤의 선택으로 기록한다. 되돌리려면 `src/models.mjs:18-21` 한 곳이다.
- Explore=luna/max, general-purpose=luna/max는 그대로다. 이 경로의 effort 하향 근거는 여전히 없다.
- **`verification/test-dotnet-http-transport.mjs`의 런타임 핀 — 하나로 둘 수 없다는 것이 CI에서 드러났다.** 처음엔 `pwsh.exe` spawn ENOENT였고(Store 설치 경로에 버전이 박혀 있어 PowerShell 업데이트로 무효화), 경로를 MSI·MSIX 양쪽 탐색으로 바꾸고 핀을 7.6.6/10.0.12로 올렸다. 그러자 **CI가 `:82`에서 실패**했다 — `offline.code` 1≠0, 스크립트가 `TRANSPORT_RUNTIME_UNSUPPORTED`로 종료. `origin/main`(5956453)의 CI는 초록불이고 main의 핀은 7.6.5/10.0.11이므로 **CI 러너는 7.6.5/10.0.11**이다. 제 머신은 7.6.6/10.0.12다. 한쪽만 보고 올린 것이 회귀를 만들었다.

  수정: 핀을 **검증된 런타임 집합**으로 바꿨다 — PowerShell `{7.6.5, 7.6.6}`, .NET `{10.0.11, 10.0.12}`. 완화가 아니다. 목록 밖 런타임은 여전히 거부하며, 목록에 넣는 조건은 "그 런타임에서 probe를 실제로 돌려 관측을 확인했을 것"으로 기존 핀의 의도와 같다. 7.6.5는 main의 CI 초록불이, 7.6.6은 이번 35 loopback 케이스 실행이 근거다. 결과의 `powerShellVersion`도 리터럴에서 **실측값**으로 바꿨다 — 측정하지 않은 값을 단언하지 않는다.
- 프로젝트 `.claude/agents/*.md` 정의의 effort는 여전히 게이트웨이에 도달하지 않는다(`src/clauduct.mjs:214`가 런처 주입 정의만 넘긴다). 미관측.
- **effort A/B는 하지 않기로 했다.** 적대적 검토에서 셋이 걸렸다. ① 지연 수치가 바꿀 결정이 없다 — 메인 effort는 사용자 지정이고 역할 기본값은 지연·품질 트레이드오프라 지연만으로 정할 수 없다. ② 같은 성격 요청의 관측 편차가 1.5~33초라 n=3은 노이즈에 묻힌다. ③ 텍스트 종료 턴을 강제하는 설계는 이 문서가 지적한 편향을 오히려 고착시킨다. 대신 계측을 고쳐 실사용에서 비용 없이 쌓이게 했다.
- Plan을 astra/low로 내린 계획 품질은 자동 판정 기준이 없어 A/B 대상이 아니다. 실사용에서 나빠졌다고 느끼면 `src/models.mjs:18-21` 한 곳으로 되돌린다.
- `docs/audit-2026-09-08.md:266`의 compact 하향 위치 서술이 낡았다(현재 `src/native-protocol.mjs:423-424`).
