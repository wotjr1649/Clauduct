# 감사 — Workflow 자식 라우팅 관측

2026-09-12. 세션 `806a34e3`, 감독하 짧은 관측. run ID `wf_7e56e185-2c4`.

판정은 세션의 보고가 아니라 디스크에 남은 산출물을 직접 읽어 확인했다.

## 1. 명시 모델 라우팅 — Verified

Workflow 스크립트가 자식 둘에 서로 다른 모델을 명시했고, 둘 다 그대로 도달했다.

| 요청 | 자식 metadata | 자식 transcript의 실제 모델 |
|---|---|---|
| `{ model: 'luna', label: 'luna-with-nested' }` | `{"agentType":"workflow-subagent","description":"luna-with-nested","workflowPhase":"Route","spawnDepth":1,"model":"luna"}` | **`gpt-5.6-luna`** |
| `{ model: 'terra', label: 'terra-direct' }` | 같은 형태, `"model":"terra"` | **`gpt-5.6-terra`** |

별칭이 전체 ID로 풀려 실제 응답까지 갔다. `MODEL`·`CALL` 거부 없음.

`agentType`이 둘 다 `workflow-subagent`로 고정된 것도 확인됐다. `src/workflow-selection.mjs:62`가 그 값만 받아들이는 것과 일치한다 — **Workflow 자식에는 커스텀 agentType을 붙일 수 없다.** 옵션에 `effort`도 실리지 않으며, 이는 같은 파일이 effort를 부모에게서 상속시키는 구조와 맞는다.

## 2. 중첩 — 경로가 없다

계획은 luna 자식이 `subagent_type: 'clauduct-luna'`로 Agent를 한 번 호출하는 것이었다. 호출되지 않았다.

**Workflow 자식에게 Agent 도구가 없다.** 두 자식의 transcript에 등장하는 도구는 `StructuredOutput` 하나뿐이고 `name: "Agent"`는 0건이다. 세션이 받은 원문은 `Agent tool is unavailable in this session; no nested call was made.`다.

이것은 미검증이 아니라 **그 경로가 제공되지 않는다**는 관측이다. 다만 이 클라이언트 버전의 자식 도구 구성에 대한 관측이며 영구한 성질로 적지 않는다.

## 3. 커스텀 agentType — 적용 지점이 사라졌다

세션 시작 시 `clauduct-astra/sol/terra/luna/inherit`가 **모두 등록돼 있었다**(`--gpt-agents`). 등록은 확인됐다.

그러나 커스텀 타입이 걸릴 수 있는 유일한 자리는 2장의 중첩 Agent 호출이었고, 그 경로가 없으므로 **실제 라우팅은 미검증으로 남는다.** Workflow 자식 자체는 1장대로 타입을 받지 않는다.

## 4. resume — 캐시 경로만 Verified

```
입력:  scriptPath=<최초 실행이 저장한 동일 script>, resumeFromRunId="wf_7e56e185-2c4"
반환:  run ID 동일, agent_count=2, agents_done=2, agents_error=0
       subagent_tokens=0, tool_uses=0, duration_ms=17
       workflowProgress 두 항목 모두 cached: true
```

세션 transcript의 Workflow 호출 2건도 그 모양이다 — 첫 호출은 inline `script`, 둘째는 `scriptPath` + `resumeFromRunId`.

resume이 붙었고 캐시가 적중했다. **새 모델 호출도 도구 실행도 없었으므로, 바뀐 에이전트를 다시 띄우는 resume은 이 실행이 다루지 않는다.**

## 5. 남는 것

- 커스텀 agentType의 실제 라우팅. 적용 지점을 만들려면 Workflow 밖에서 Agent를 직접 부르는 별도 관측이 필요하다. 이를 위해 새 시험을 만들지는 않는다 — 정상 사용 중 관측되면 기록한다.
- 캐시가 적중하지 않는 resume.
- 이 실행의 종료 JSON. 세션이 아직 종료되지 않아 `selectionSource: "workflow-result"` 행과 `unmappedAgentModels` 카운터는 확보하지 못했다. 1장의 metadata·transcript 쌍이 더 직접적인 증거이므로 판정은 그것으로 한다.
