# 매핑되지 않은 Agent 모델 이름의 턴 손실

## 재현

2026-09-11 로컬 합성 재현이다. 실제 인증 실행이나 외부 요청은 하지 않았다.

upstream이 완성한 응답에 `Agent` tool call이 있고 그 `model` 인자가 이 gateway가 매핑하지 않는 이름이면, gateway는 라우팅 증거를 기록하는 단계에서 턴 전체를 거부했다. 관측한 결과는 다음과 같다.

- downstream에는 텍스트가 이미 전달된 뒤 `error` 프레임이 붙는다. `tool_use` 블록과 `message_stop`은 전달되지 않는다.
- 오류 메시지는 `PROTOCOL_REJECTED: Partial response; explicit resume required.`였다.
- 진단은 `failureCategory=PROTOCOL_REJECTED`, `failureStage=output-validation`, `selectionFailure=null`이었다.

같은 요청에서 `model`이 매핑된 별칭(`opus`)이면 정상 성공한다. 즉 완성된 답변이 사라지는데 진단에는 선택 실패라는 표시가 없었다.

이 경로는 도달 가능하다. 현재 Claude Code의 Agent 도구 `model` 파라미터는 `sonnet`/`opus`/`haiku` 외에 `fable`도 값으로 제시하지만, `src/agent-selection.mjs`의 별칭표에는 `haiku`/`sonnet`/`opus`만 있다. 이 조사에서는 실제 세션에서 이 실패가 발생한 증거는 확인하지 못했다. 관측된 과거 실패(prepare 2건·upstream 3건)와는 단계가 다르다.

## 이번에 고친 범위

라우팅 증거 기록 실패를 고유한 실패로 보고한다. 모델 매핑이나 전달 정책은 바꾸지 않았다.

- `remember`에서 매핑되지 않은 모델 이름은 라벨 없는 throw 대신 `selectionReason='MODEL'`로 실패한다.
- gateway는 `remember` 거부를 `AGENT_SELECTION_UNVERIFIED_<이유>`로 감싼다. 따라서 `failureCategory=AGENT_SELECTION_UNVERIFIED`, `failureStage=output-validation`, `selectionFailure=MODEL`이 되고 오류 메시지도 이유를 담는다. 원문 모델 이름은 노출하지 않는다.
- 기존 원자성은 유지한다. 거부된 턴은 어떤 pending 라우팅 증거도 남기지 않는다.

## 남은 결정 — 사용자 확인 필요

턴을 잃는 동작 자체는 그대로다. 두 선택지 중 무엇을 택할지는 제품 정책 결정이며 이번 범위(Claude 별칭 대체·전체 모델 목록 확장 제외)에서 임의로 정하지 않았다.

1. 별칭표에 `fable`을 추가한다. 어떤 GPT 모델로 보낼지 정해야 하므로 모델 매핑 결정이다.
2. 현재처럼 실패를 유지한다. 이 경우 매핑되지 않은 이름을 쓴 턴은 계속 손실되며, 모델이 같은 인자를 반복하면 같은 실패가 반복될 수 있다.

세 번째로 "기록하지 않고 턴은 전달한다"도 기술적으로 가능하다. `remember`는 원자적이고 기록되지 않은 호출로 생성된 자식은 첫 요청에서 `AGENT_SELECTION_UNVERIFIED_CALL`로 fail-closed 되므로 검증이 약해지지는 않는다. 다만 전달 계약을 바꾸는 변경이라 사용자 확인 없이 적용하지 않았다.

## 검증

2026-09-11, Node.js v24.19.0, `Invoke-ClauductNodeTests`, 60초 제한.

`test-native-gateway`는 49 → 51개 검사다. 새 검사는 매핑된 별칭의 정상 성공과, 매핑되지 않은 이름에서 `AGENT_SELECTION_UNVERIFIED`/`output-validation`/`selectionFailure=MODEL`, `message_stop` 미전달, 원문 모델 이름 미노출을 확인한다. `test-agent-selection`의 기존 거부 검사는 새 실패 라벨을 확인하도록 갱신했다.

기준 11개 파일과 선택·완료 표면 4개(`test-agent-selection`, `test-completion-selection`, `test-workflow-selection`, `test-request-admission`)가 모두 통과했다. 외부 추론 요청·실제 인증 조회·실제 Claude 실행은 0이다.

## 사용자 결정과 적용 — 2026-09-11

grilling에서 사용자가 `fable → astra` 매핑과 "전체 모델 ID도 별칭과 동일 규칙으로 매핑"을 선택했다. 적용 내용은 다음과 같다.

| 입력 | 라우팅 |
|---|---|
| `fable`, `claude-fable-*` | `gpt-6-astra` / medium |
| `opus`, `claude-opus-*` | `gpt-5.6-sol` / xhigh |
| `sonnet`, `claude-sonnet-*` | `gpt-5.6-luna` / max |
| `haiku`, `claude-haiku-*` | `gpt-5.6-luna` / max |

접두사만 비교하므로 버전 숫자를 고정하지 않는다. [공식 서브에이전트 문서](https://code.claude.com/docs/en/sub-agents) 기준 `model` 필드는 이 네 별칭, 전체 모델 ID, `inherit`을 받는다. 전달 계약과 `remember`의 원자성은 바꾸지 않았다.

한계: 구형 명명(`claude-3-5-sonnet-*` 등)은 이 접두사에 맞지 않아 계속 `AGENT_SELECTION_UNVERIFIED_MODEL`로 실패한다. 매핑되지 않은 이름의 턴 손실 동작 자체도 그대로다. 다만 이제 실패 원인이 진단에 드러난다.

검증: `test-agent-selection`에 여덟 개 별칭·전체 ID의 실제 route 확인과 네 개 거부 사례(`claude-3-5-sonnet-20241022`, `__proto__`, `claude-`, `fable-5`)를 추가했다. 기준 11개와 선택·완료 표면 4개 suite가 모두 통과했다.
