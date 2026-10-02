# Clauduct 전용 설정

v0.5.3에서 도입한 설정 형식이다. v0.5.4에서 초기 문서와 적용값 진단을 보완해 출하했다.
출하 범위는 [v0.5.4](RELEASE-v0.5.4.md), v0.5.5의 phase 대화 호환 범위는 아래 재개 절에 있다.

Clauduct는 실행할 때 사용자 홈의 `.clauduct/settings.json`을 읽는다. 파일은 직접 편집한다.
v0.5.4의 설치·업데이트와 일반 실행은 파일이 없을 때만 startup, modelDefaults, modelMapping,
내장 agents를 모두 담은 [기본 문서](../../go/internal/settingsfile/defaults.json)를 생성한다.
생성과 생략값 처리는 이 원본을 공유한다. v0.5.3이 만든 `{"version":1}` 파일도 기존 파일이므로 보존한다.
생략한 항목은 아래의 내장 기본값을 사용한다. 기존 파일은 자동으로 덮어쓰거나 병합하지 않으며,
잘못된 파일은 보존한 채 새 세션 실행을 거부한다. `--help`·`--version`은 파일을 생성하지 않는다.
v0.5.2 이전 updater로 올린 경우에는 새 버전의 첫 일반 실행 또는 다시 실행한 `--update`에서 생성한다.
v0.5.3 updater가 먼저 최소 파일을 만들었다면 새 버전도 이를 자동 확장하지 않는다.
기존 파일을 완전한 문서로 바꾸려면 원본을 백업하고 위 기본 문서를 참고해 직접 편집한다.
Claude Code의 개인화·권한·알림·플러그인은 기존 native 설정에서 관리한다.

## 설정 예

```json
{
  "version": 1,
  "context_window": 272000,
  "auto_compact_token_limit_percent": 90,
  "auto_compact_effort_cap": "medium",
  "auxiliary_effort_cap": "medium",
  "classifier_model": "gpt-5.6-terra",
  "startup": {
    "model": "gpt-6.1-sol",
    "effort": "xhigh"
  },
  "modelDefaults": {
    "gpt-6-luna": {
      "effort": "low"
    }
  },
  "modelMapping": {
    "sonnet": "gpt-6-luna",
    "haiku": "gpt-6-luna"
  },
  "agents": {
    "Explore": {
      "model": "gpt-6-luna",
      "effort": "medium"
    },
    "my-plugin:reviewer": {
      "model": "gpt-5.6-terra",
      "effort": "high"
    }
  }
}
```

`version`은 필수이며 현재 값은 `1`이다. 나머지 항목은 생략할 수 있다.
`startup`과 각 `agents` 항목을 쓰면 `model`과 `effort`를 모두 적는다.
알 수 없는 필드·모델·effort, 중복 JSON 키, 부분적인 pair, `null` 및 1 MiB를 넘는 파일은 거부한다.

| 항목 | 적용 범위 |
|---|---|
| `startup` | 인자 없이 시작하는 새 실행의 모델·effort. 기본값은 Sol/xhigh |
| `modelDefaults` | 모델만 선택하고 effort를 명시하지 않은 경로의 GPT 기본 effort |
| `modelMapping` | Claude alias 및 해당 버전형 이름이 가리키는 GPT 모델 |
| `agents` | 해당 agent의 모델·effort 기본 pair |
| `context_window` | 모든 모델에 공통인 context 관리값. 기본 272,000, 정수 100,000–872,000 |
| `auto_compact_token_limit_percent` | 위 window에 대한 예방 압축 비율. 기본 90, 정수 1 이상. 90 초과는 90으로 clamp |
| `auto_compact_effort_cap` | 검증된 자동 압축 요청의 effort 상한. 기본 `medium`. 현재 대화 effort를 높이지 않음 |
| `auxiliary_effort_cap` | native의 독립 보조 요청 effort 상한. 기본 `medium`. 기존 매핑·기본값·명시값으로 선택한 effort를 높이지 않음 |
| `classifier_model` | auto 권한 모드 분류기가 쓸 GPT 모델의 전체 ID. 분류기 지원 범위(`gpt-5.6-terra`·`gpt-6.1-sol`·`gpt-6-sol`·`gpt-6-astra`, Terra 이상) 밖이나 모르는 값은 시작 시 거부한다. 없으면 native의 Sonnet 요청을 `modelMapping.sonnet`대로 보낸다(기본 Terra). 분류기 모델만 바꾸며 `sonnet` 별칭은 그대로다. effort는 그 모델의 기본 effort와 `auxiliary_effort_cap`을 따른다 |

시작 pair와 개별 agent pair는 공통 GPT effort와 독립적이다. 위 예에서 Luna의 공통 effort는 low지만
Explore는 medium이고 새 실행의 시작값은 Sol/xhigh다. 각각을 바꾸려면 해당 항목을 직접 편집한다.

## 모델과 agent

지원 모델은 `gpt-6-astra`, `gpt-6.1-sol`, `gpt-6-sol`, `gpt-5.6-terra`, `gpt-6-luna`다.
설정 파일에는 이 전체 ID를 사용한다. 지원 effort는 `low`, `medium`, `high`, `xhigh`, `max`다.
v0.6.3부터 `sol` 키·`opus` 별칭·공장 시작 모델은 `gpt-6.1-sol`이다. `gpt-6-sol`은 전체 ID,
`sol6` 키, `clauduct-sol6` 위임 메뉴로 계속 쓴다(native Agent 인자에는 `opus` 단계로 적지만 실제
경로는 native 생성 이벤트가 전체 ID로 고정한다).

| Claude alias | 내장 GPT 매핑 | GPT 공통 기본 effort |
|---|---|---|
| `fable` | `gpt-6-astra` | medium |
| `opus` | `gpt-6.1-sol` | xhigh |
| `sonnet` | `gpt-5.6-terra` | high |
| `haiku` | `gpt-6-luna` | max |

모델 목록·키·별칭·effort·계수 허용·은퇴 매핑은 코드가 아니라 내장 제품 자료
(`go/internal/protocol/bridge/models.json`)다. 실행 시작 때 검증하고, 잘못된 자료면 실행하지 않는다.
사용자 설정으로 모델을 추가할 수는 없다(측정하지 않은 모델을 실행하지 않기 위해서다).
측정 기준 Claude Code·Codex CLI 버전도 내장 자료(`go/internal/upstream/measured-clients.json`)이며,
설치된 버전은 실행할 때마다 기계에서 읽는다.

여러 alias를 같은 GPT 모델에 매핑해도 된다. 전체 GPT ID를 명시한 선택은 그 모델을 직접 선택한다.
지원 모델·effort 목록과 backend 자체의 최대 용량을 설정 파일로 늘릴 수는 없다.

## 전역 context와 압축 목표 (v0.6.2 준비)

두 context 항목은 모델별 설정이 아니다. Sol·Luna·Astra·Terra와 자식 Agent에 같은 값을 적용한다.
Clauduct는 실행 시 `floor(context_window * min(auto_compact_token_limit_percent, 90) / 100)`으로
gateway의 예방 압축 목표를 한 번 계산한다. 기본값은 244,800 tokens다. 절대값을 따로 저장하는
`auto_compact_token_limit` 항목은 받지 않는다. 1% 미만, 소수, 문자열, null은 오류이며,
90% 초과는 입력 파일을 수정하지 않고 실행값만 90%로 제한한다. window의 범위 밖 값은 거부한다.

window 하한은 native `CLAUDE_CODE_AUTO_COMPACT_WINDOW`의 100,000에 맞췄다.
상한 872,000과 기본 272,000은 Codex CLI 0.158.0의 관리 범위를 기준으로 정한 제품 설정 범위다.
이는 모델 API의 최대 context 용량이나 모든 크기에서 실측한 성공 보장이 아니다.

압축은 Claude Code(native)가 수행한다. Clauduct는 해당 window와 유효 비율을 native 환경에
전달하고, 관측된 사용량·예방 추정이 gateway 목표에 이르면 기존 native 압축 경로를 요청한다.
별도 Codex 압축 엔진이나 대화 강제 절삭을 추가하지 않는다. native는 출력 여유 공간과 자체
정책 때문에 더 일찍 압축할 수 있다. native 2.1.284의 기본 `/context` 실측은 272k와 33k의
autocompact buffer였으며, 이를 gateway 목표 244,800과 같은 trigger라고 해석하지 않는다.
설정값은 정확한 요청 차단 상한이 아니며 큰 신규 입력의 최초 초과 가능성은 남는다.
도구 없는 보조 요청도 이 공통 목표를 사용한다. 보조 요청은 대화 이력을 직접 압축할 수 없으므로
예방 추정값이 목표에 도달하면 기존 `CONTEXT_COMPACTION_UNAVAILABLE` 거부를 유지한다.

자동 압축의 effort는 현재 대화 effort와 `auto_compact_effort_cap` 중 낮은 값이다.
지원값은 `low`, `medium`, `high`, `xhigh`, `max`이며 잘못된 값은 거부한다.
기본 상한 `medium`에서 대화가 `low`이면 압축도 `low`, 대화가 `max`이면 압축은 `medium`이다.
상한을 `max`로 설정해도 `low` 대화의 압축을 높이지 않는다. `modelDefaults`를 다시 적용하지 않는다.
압축 모델은 기존 확정 모델을 유지하고 이후 생성은 원래 effort로 돌아간다. 수동 `/compact`는
기존 대화 effort를 유지한다. 이 상한은 native가 실행하는 요약 요청의 backend effort에만 적용하며,
압축 시작·권한·완료 판단을 바꾸지 않는다. 낮은 effort가 기억 보존 품질까지 보장하지는 않는다.

외부 플러그인 agent는 `plugin-name:agent-name`처럼 native의 실제 이름을 사용한다.
Clauduct의 pair는 역할의 실행 선택을 바꾼다. 역할의 원래 prompt·tools·권한은 native가 읽고 적용한다.
정의가 모델 alias를 사용하면 Clauduct 매핑으로 변환하고, 전체 GPT ID를 사용하면 직접 선택한다.

설정한 agent pair는 일반 역할 정의보다 우선하며, 호출에서 명시한 모델·effort와 관리되는 역할 정의의
우선순위는 보존한다. 모델 또는 effort를 명시한 Agent·Workflow 호출은 해당 명시값을 적용한다.
내장 역할은 `Explore`, `Plan`, `general-purpose`의 이름으로 설정한다.
native의 부모 상속 역할(`fork`, `workflow-subagent`, `clauduct-inherit`)은 별도 pair로 바꾸지 않는다.

## 보조 요청의 effort 상한 (v0.6.2 준비)

권한 분류와 background 완료 판정 등 도구 없는 독립 보조 요청은 기존 `modelMapping`,
`modelDefaults`와 요청의 명시 effort로 선택한 뒤 `auxiliary_effort_cap` 이하로만 낮춘다.
상한의 지원값은 `low`, `medium`, `high`, `xhigh`, `max`이며 잘못된 값은 거부한다.
기본 상한에서 `low`는 `low`, `max`는 `medium`으로 실행된다. 같은 요청의 token count와
generation은 같은 선택을 사용한다. 일반 대화·Agent와 자동 압축의 별도 상한은 바꾸지 않는다.

Auto 권한 분류의 지원 목표는 Terra·Sol·Astra의 유효 effort 전부다. Haiku에 대응하는 Luna는
분류 요청만 `AUTO_MODE_CLASSIFIER_MODEL_UNSUPPORTED`로 전송 전에 거부한다. 다른 모델로
대체하지 않으며 일반 대화·Agent·background 완료 보조 요청의 Luna 사용은 유지한다.
현재 확대 범위의 품질 관문은 미완료다. 과거 오허용과 미검증 조합은
[#218](https://github.com/wotjr1649/Clauduct/issues/218)에 남기며 이 설정의 구현을 안전성 통과로 해석하지 않는다.

## 실행·외부 통신의 native 확인 (v0.6.2 준비)

Clauduct는 실행할 때 native `permissions.ask`에 다음 도구를 추가한다.
기존 사용자 `allow`·`ask`·`deny`와 다른 native 설정은 보존하며 전역 설정 파일은 수정하지 않는다.

- 실행: `Bash`, `PowerShell`, `Monitor`, `Workflow`, `Skill`
- 외부 통신: `mcp__*`, `WebFetch`, `WebSearch`, `ListMcpResourcesTool`, `ReadMcpResourceTool`,
  `Artifact`, `PushNotification`, `RemoteTrigger`, `SendUserFile`, `ShareOnboardingGuide`, `SendMessage`, `DesignSync`

사용자가 native bypass 모드로 시작한 세션에는 이 목록을 추가하지 않는다(v0.6.3). 시작 모드는
`--permission-mode bypassPermissions` 또는 native 설정의 `permissions.defaultMode`(user < project <
local < `--settings`, `--setting-sources`가 읽는 범위)로 정한다. 관리 정책이 모드를 정하거나 bypass를
끄는 경우, 또는 설정을 읽을 수 없는 경우에는 목록을 유지한다. 세션 도중 모드를 바꿔도 시작 때의 결정을
따른다. 실행 상태 파일의 `requiredAsk`가 그 세션의 결정을 기록한다. `--dangerously-skip-permissions`는
계속 전달하지 않는다.

native의 우선순위는 `deny` → `ask` → `allow`다. 따라서 빌드·테스트·읽기 전용 MCP와 같은
작업도 확인 대상이며, 더 좁은 `allow` 규칙이나 auto 분류기의 허용 판정으로 확인을 생략하지 않는다.
`SendMessage`는 다른 세션에도 전송할 수 있어 같은 세션의 Agent 메시지까지 확인한다.
일반 파일 `Read`·`Edit`·`Write` 및 `Agent` 생성에는 이 목록만으로 새로운 확인 규칙을 추가하지 않는다.
[native 권한 규칙](https://code.claude.com/docs/en/permissions).

승인 화면과 결정은 native가 관리한다. 사용자가 구성한 `PermissionRequest` hook도 native의
승인 주체가 될 수 있으므로 그런 hook의 효과는 유지한다. 명시적 `deny`는 그 승인보다 우선한다.
대화에 적은 승인 문구만으로 확인 화면을 자동 처리하지 않는다. 확인을 처리할 수 없는 headless나
background 작업은 native 모드에 따라 대기하거나 거부될 수 있다. native 2.1.284의 로컬 검증에서
`default`는 지정된 `PermissionRequest` hook으로 승인했고, `dontAsk`는 같은 hook을 호출하지 않고
거부했다. [native hook 규약](https://code.claude.com/docs/en/hooks#permissionrequest).

매 native step과 실행·외부 통신 도구의 실행 직전에 필수 `ask` 또는 `deny` 규칙을 확인한다. 관리 정책의
`allowManagedPermissionRulesOnly`가 활성화되어 있으면 관리 규칙 자체를 확인한다. 필수 규칙을
확인하지 못하면 해당 native 추론을 `NATIVE_CONFIRMATION_UNVERIFIED`로 거부한다.
직접 입력한 forked Skill은 native 이벤트에 Agent ID가 있어도 첫 HTTP 요청에는 그 ID가 없다.
활성 자식이 하나여도 이전 자식의 지연 요청과 구별할 수 없으므로, v0.6.2에서는 이 직접 입력 경로를
지원하지 않는다. 도구 유무와 관계없이 생성·계수 요청을 `NATIVE_REQUEST_ORIGIN_UNVERIFIED`로
backend 전송 전에 거부한다. Agent ID가 있는 native Agent·fork 및 식별된 병렬 실행은 유지한다.
출처나 현재 scope가 맞지 않아 거부한 요청은 이후 정상 요청을 막는 저장 실패 상태로 취급하지 않는다.
모델이 낸 도구 호출의 `tool_use_id`에는 gateway가 그 호출을 받은 native step의 표지를 붙인다
(`<backend call_id>__cdt<12자리 16진수>`). backend에는 원래 `call_id`를 그대로 돌려보낸다. native가
도구를 실행할 때 표지가 현재 session·Agent·turn·step과 맞지 않으면, 이전 turn이나 step의 늦은 호출로 보고
`NATIVE_REQUEST_ORIGIN_UNVERIFIED`로 실행 전에 거부한다. 표지가 없는 호출은 plugin hook 모듈이 모델 호출 없이
`$.tool.call`로 직접 실행한 도구다(native 2.1.287 측정: `toolu_plugin_…` ID). 이런 호출은 위 native 확인
규칙을 그대로 거쳐 실행되지만, 현재 turn의 진행 상태·보조 요청 확인·위임 판정을 사용하지 못한다. 직접 실행한
`Agent`·`SendMessage`·`Workflow`·`Skill`은 `NATIVE_DIRECT_DELEGATION_UNSUPPORTED`로 거부한다. 표지가 없는
이전 대화의 도구 기록은 resume 후에도 그대로 backend로 전달된다.
도구 요청마다 새로운 일회성 확인값에 대해 현재 native step이 응답해야 한다. 세션 시작이나
과거의 응답만으로 새 요청을 허용하지 않는다. native `WebSearch`의 별도 검색 요청은 해당
도구가 실행 중인 동안 확인하며 일반 추론 요청과 구분한다.
긴 도구의 background 판정용 `auxiliary`도 확인된 session·Agent·turn·step의 활성 도구 구간에서
처리한다. 같은 step이 아직 추론 중일 때 native가 보내는 `auxiliary`(예: 30초가 지난 자식 Agent의 진행 확인)도
그 step의 확인으로 처리한다. native 요청에 원래 `tool_use_id`가 없으므로 같은 step의 개별 도구를 구별하는 증명은
제공하지 않는다. 이 확인은 보조 요청의 출처 확인이며, 실제 도구 실행 승인은 위 native 규칙을
계속 따른다. 일반 추론·검색·다른 Agent·turn·step은 그 보조 요청의 확인을 빌리지 못한다.
취소·오류·거부로 종료된 turn의 확인은 사용할 수 없으며, 사용자 재입력으로 시작한 새 turn은
별도 요청으로 처리한다. 확인 응답은 현재 Clauduct 바이너리의 내장 helper가 고정 인자로 실행되어
전달한다. 일회성 확인값과 scope는 stdin으로 전달하고, 인증된 loopback 연결은 원래 요청이 끝나거나
native가 helper stream을 반환할 때까지 유지한다. 연결 종료만으로 이전 확인값을 새 요청에 사용할
수 없으며, 확인된 연결의 종료는 현재 요청을 거부하거나 취소하고 이후 사용자 재입력을 막지 않는다.
임의 명령·외부 runtime·전역 설정 변경은 사용하지 않는다. 내부 정책 확인 통신은 왕복당2초이며,
대기열 전체에는 최대64건×2초 상한을 적용한다. 더 짧은 요청 deadline·사용자 취소·launcher 종료는
대기 중에도 적용한다. 대기 중인 요청의 취소·시간 초과만으로 이후 정상 요청을 막지는 않는다.
승인 화면은 native의 기존 규칙을 따른다. 확인 저장소 I/O 오류나 실제 확인 왕복 시간 초과가 발생하면 gateway는
실패 표시 파일을 쓰지 못해도 이후 도구 요청을 거부한다. 이 상태는 `/clear`나 plugin reload로
해제하지 않으며 launcher를 다시 실행해야 한다. native 모듈도 확인 오류 후 대기 중인 도구를
거부하고, 검증되지 않은 step이 기본 실행으로 이어지지 않도록 명시적 refusal을 반환한다.
사용자에게는 `NATIVE_CONFIRMATION_UNVERIFIED`를 표시한다.
native가 제자리에서 쓰는 취소 영수증을 쓰는 중에 읽었다면(일부만 쓰였거나 읽기가 막힌 경우) 판정을 미루고
다음 확인에서 다시 읽는다. 2초가 넘도록 읽을 수 없을 때만 위 실패 상태로 처리한다. 같은 step에서 같은 종류의
도구가 동시에 실행되면(예: 검색 두 개) 요청으로는 둘을 구별할 수 없으므로 같은 step의 확인으로 처리한다.
step이나 도구 구간이 정상으로 끝나도 이미 받아들인 요청의 확인 연결은 그 HTTP 요청이 끝날 때 함께 닫힌다.
turn이 중단·오류·거부로 끝나거나 세션이 끝나면 그 turn의 확인 연결을 즉시 닫아 진행 중인 요청을 취소한다.
native가 더는 필요 없는 요청을 스스로 끊은 경우는 클라이언트 취소로 기록하며 확인 연결 종료로 표시하지 않는다. gateway가 이미 기다리지 않는 확인값에 늦게 연결한 helper는 410을 받고 종료 코드 3으로
끝나며, 이는 실패 상태가 아니다. helper 연결은 종료 줄의 요청 수와 최근 요청 기록에 넣지 않는다.
동시에 유지하는 확인 연결은 64개까지이며, 넘으면 위 실패 상태가 된다.
백그라운드 작업(`--bg`)의 native 환경에는 gateway 토큰이 없으므로, 그때 helper는 백그라운드 hook과 같은
세션 연결에서 주소와 토큰을 읽는다. helper 인자는 실행 시점에 정해지며 요청이 바꿀 수 없다.
`/context`처럼 turn 밖에서 보내는 루트 `auxiliary` 토큰 계수 요청은 숫자만 돌려주므로 step 확인 증명을 요구하지
않는다. 생성 요청, turn 안의 `main` 계수와 자식 Agent의 계수 요청은 증명을 계속 요구한다.
관리 정책을 덮어쓰거나 무시하지 않는다. 관리자가 동등한 확인·거부 규칙을 제공하지 않는 환경에서는
이 경로를 실행할 수 없다. [관리 규칙의 적용 범위](https://code.claude.com/docs/en/settings-reference#allowmanagedpermissionrulesonly).

native 압축은 다음 step보다 먼저 실행될 수 있는 요약 전용 경로다. 기존 context 검증을 거친 뒤
count와 generation 모두 도구 선택을 `none`으로 보내며, 압축 응답에 도구 호출이 오면 거부한다.
이렇게 UUID 재개 시 필요한 압축을 유지하면서 요약 요청이 도구를 실행하지 못하게 한다.

이 확인 절차는 모델의 원래 판정 품질을 바꾸지 않는다. raw 분류 오허용의 이력과 최종 실행 차단의
검증 결과를 구분하며, 확대 지원의 전체 검증이 끝나기 전까지 관련 Issue를 완료 처리하지 않는다.

## 현재 세션의 선택과 저장

native의 모델·effort 화면에서 `S`로 확정하면 현재 세션의 선택이 된다. 이 선택은 수동
`.clauduct/settings.json`을 수정하지 않는다. `Enter`를 비롯한 native 키의 원래 동작은 그대로다.

Clauduct는 `.clauduct/sessions/<UUID>.json`에 당시 모델 매핑·기본값·agent 설정과 마지막 선택을 보존한다.
대화 본문은 이 파일에 복사하지 않는다. 질문을 보내지 않고 `S` 선택 후 종료한 경우도 native 기록에서
선택을 확인한다. 같은 기록의 동시 작성은 OS 파일 잠금으로 차단한다.

`--resume <UUID>`로 재개하면 그 snapshot과 마지막 선택을 사용한다. 이후 새로 만드는 agent도 같은
snapshot을 따른다. `--model`, `--effort`와 사용자가 지정한 native effort 환경변수는 명시적인 선택으로 적용한다.
`--fork-session`은 원본의 snapshot과 선택을 복제한다.

context window·비율·자동 압축 상한·보조 요청 상한은 snapshot에 저장하지 않는다. UUID 재개 시에도
**현재 전역 settings.json**의 네 값을 읽는다. 따라서 재개 전에 window를 줄이면 기존 사용량이 새 목표에 도달하여 압축이
필요할 수 있다. 모델·effort snapshot과 대화는 그대로 유지한다. 현재 설정 파일이 잘못되었다면
UUID 재개도 native 실행 전에 거부하며 원본 파일은 보존한다.
이 검증은 파일 전체에 적용한다. 현재 파일에 모르는 모델·effort가 있으면 이를 무시하지 않고 오류로
알린다. 유효한 파일을 읽은 뒤에는 저장된 모델·effort 선택을 사용하며 현재 파일의 선택값으로 바꾸지 않는다.

`/clear`는 현재 실행의 snapshot과 선택을 유지한다. 파일 편집은 새로운 Clauduct 실행에서 읽으며,
저장된 세션의 모델·effort는 snapshot, context는 현재 전역 설정을 사용한다.
실행 중인 세션과 `/clear`에는 편집한 context를 즉시 다시 읽어 적용하지 않는다.

`EnterWorktree`·`ExitWorktree`는 native가 옮긴 transcript 경로에 snapshot과 context journal을 연결한다.
기존 transcript가 사라지고 같은 UUID의 새 파일이 native projects 안에 존재하는지 확인한다.
다른 살아 있는 transcript로 바꾸거나 경계를 벗어나는 경로는 거부한다. native의 권한과 worktree 격리
규칙은 그대로 적용한다. v0.6.0에서 도구 실행 직후 경로 갱신 누락을 수정했다([#185](https://github.com/wotjr1649/Clauduct/issues/185)).

snapshot이 있는 세션을 재개 목록, `--continue`·`-c`, 실행 중 `/resume`에서 선택하면 첫 backend
요청을 차단하고 해당 UUID의 `clauduct --resume <UUID>` 명령을 안내한다. 초안을 정리하고 native를
종료한 뒤 그 명령에 필요한 native 옵션을 함께 지정한다. Clauduct가 화면을 자동 종료하지 않는다.

snapshot이 없는 이전 세션은 현재 설정의 시작값으로 연다. `--resume <UUID>`, `-r <UUID>`뿐 아니라
재개 목록과 `--continue`·`-c`도 별도 가져오기 옵션이 필요 없다. 원하는 모델·effort는 `S`로 바꿔
계속 사용한다. 첫 재개 시점의 설정을 새 기준으로 저장하며, 과거 설정을 복원한 것으로 간주하지 않는다.
실행 중 `/resume`은 새 실행이 아니므로 위의 재실행 안내를 따른다.

`--no-session-persistence`로 실행한 작업은 새 선택을 세션 기록에 보존하지 않는다.
기존 UUID를 명시하면 그 snapshot을 읽어 시작값을 복원한다.

설정 snapshot을 복원하는 되돌림 대상은 v0.5.3 이상이다. v0.5.2 이하 바이너리는 snapshot을
읽지 않으므로 이전 버전의 기본 모델·effort로 시작할 수 있다. v0.5.2 실측에서는 Sol/medium
대화가 Astra/low로 열렸다. 그 버전에서 계속하려면 S로 원하는 값을 선택한다.
바이너리 되돌림 때문에 설정이나 snapshot을 지우거나 변환하지 않는다. 기존 파일을 보존한 채
현재 버전으로 돌아오면 UUID snapshot으로 재개할 수 있다. 다만 v0.5.5부터 새 응답에 보존한
`phase`가 포함된 대화는 v0.5.5 이상이 필요하다. v0.5.3/4는 그 대화의 후속 요청을 거부한다.
설정 snapshot 호환성과 대화 payload 호환성은 별개다. 구버전 backend 응답 자체는 이
무과금 호환성 검사의 범위가 아니다.

v0.6.1 이하 설정 parser는 새 context·effort 상한 키를 알 수 없는 필드로 거부한다. 그 버전으로 되돌리려면
현재 파일을 백업한 뒤 네 키를 제외한 구버전 호환 설정을 직접 사용한다. 바이너리 되돌림이
설정·대화·snapshot을 자동 삭제하거나 변환하지 않는다.

v0.5.4의 background worker는 같은 연결·같은 세션으로 재시작하고 마지막 선택이 시작 인자와
일치할 때 현재 snapshot으로 재연결한다. 실행 중 settings.json 편집은 이 worker의 설정을 바꾸지 않는다.
S로 시작값과 다른 선택을 저장했다면 원래 인자로 되살린 worker는 첫 요청을 거부하고 UUID 재개를 안내한다.
다른 세션이나 일반 interactive resume에 이 재연결 예외를 적용하지 않는다.

## 적용값 진단

종료 상태 JSON의 `session.startupModel`과 `startupEffort`는 이 실행의 시작 선택이다.
v0.5.4는 각각 `startupModelSource`, `startupEffortSource`도 기록한다.

| 출처 | 의미 |
|---|---|
| `factory.startup` | 파일에서 생략한 시작 pair의 제품 기본값 |
| `settings.startup` | 수동 파일에 명시한 시작 pair |
| `session-snapshot.last` | UUID snapshot에서 복원한 마지막 선택 |
| `cli.model`, `cli.effort` | 해당 CLI 인자에 명시한 값 |
| `environment.CLAUDE_CODE_EFFORT_LEVEL` | native effort 환경변수로 고정한 값 |
| `native.user.env.*`, `native.project.env.*`, `native.local.env.*`, `native.settings.env.*` | native 설정의 `env`가 선택한 모델 또는 effort. `--setting-sources`와 마지막 `--settings`를 반영 |
| `factory.modelDefaults`, `settings.modelDefaults`, `session-snapshot.modelDefaults` | 모델만 명시한 선택의 effort 기본값 |

alias나 버전형 Claude 이름으로 선택하면 model 출처에 `+factory.modelMapping`,
`+settings.modelMapping` 또는 `+session-snapshot.modelMapping`이 붙는다.
한쪽만 명시한 경우 두 출처는 다를 수 있다. 이 진단은 수동 파일을 쓰거나 모델 요청을 만들지 않는다.
Windows 환경변수 이름의 대소문자는 구분하지 않는다. 같은 선택 키의 대소문자 중복이나 잘못된 값은
거부한다. native managed 설정에서 선택 환경변수를 강제한 경우의 순위는 아직 검증되지 않았으므로
추정한 시작값을 기록하지 않고 거부한다.

실행 중 S 선택과 Agent의 실제 요청값은 `gateway.recent`와 `gateway.agentSelections`에서 확인한다.
Agent 기록은 호출에 model·effort가 있었는지, 적용 pair와 선택 경로, 실제 자식 연결·완료를 구분한다.
기존 Agent의 `source`는 선택 경로 분류이며 원본 설정 파일 경로를 뜻하지 않는다.
대화 본문·설정 원문·credential은 출처 진단에 넣지 않는다.

`session.context`는 window, 요청한 `requestedPercent`, clamp 후 `effectivePercent`, 계산한
`autoCompactTokenLimit`, `autoCompactEffortCap`과 각 설정의 `factory.*` 또는 `settings.*` 출처를 기록한다.
실제 압축 요청에 사용한 effort는 `gateway.recent`의 `kind: compaction` 기록에서 확인한다.
`session.auxiliaryEffortCap`과 `auxiliaryEffortCapSource`는 보조 요청의 상한과 출처다.
상한으로 effort가 낮아진 실제 요청은 `gateway.recent[].source`에 `+auxiliary-cap`을 기록한다.
`gateway.modelContexts[].target`은 이 실행의 공통 목표다. `nativeContextDefaults`는 전달한
native 값이며, `applicationVerified:false`는 native가 그 정확한 시점에 압축했다는 증거가
아니라는 뜻이다. 낮은 비율에서 반복 압축이 발생하면 비율과 실제 입력 크기를 함께 확인한다.

## 코드에 유지하는 정책

모델의 지원 기능, context 설정의 허용 범위, 입력 검증과 처리 상한, native hook 연결 및 보안 규칙은 코드에서 관리한다.
추가된 두 native 차단 규칙은 `$defaults`와 함께 유지한다. Auto mode classifier의 실행값은
위 매핑·기본값·상한을 따르며, 지원 모델 목록은 별도 제품 자료로 관리한다.
개발용 probe의 지출 범위는 일반 세션 설정과 독립된 제품 자료이며 설정 변경으로 확대되지 않는다.
