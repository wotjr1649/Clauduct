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
  "startup": {
    "model": "gpt-6-sol",
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

시작 pair와 개별 agent pair는 공통 GPT effort와 독립적이다. 위 예에서 Luna의 공통 effort는 low지만
Explore는 medium이고 새 실행의 시작값은 Sol/xhigh다. 각각을 바꾸려면 해당 항목을 직접 편집한다.

## 모델과 agent

지원 모델은 `gpt-6-astra`, `gpt-6-sol`, `gpt-5.6-terra`, `gpt-6-luna`다.
설정 파일에는 이 전체 ID를 사용한다. 지원 effort는 `low`, `medium`, `high`, `xhigh`, `max`다.

| Claude alias | 내장 GPT 매핑 | GPT 공통 기본 effort |
|---|---|---|
| `fable` | `gpt-6-astra` | medium |
| `opus` | `gpt-6-sol` | xhigh |
| `sonnet` | `gpt-5.6-terra` | high |
| `haiku` | `gpt-6-luna` | max |

여러 alias를 같은 GPT 모델에 매핑해도 된다. 전체 GPT ID를 명시한 선택은 그 모델을 직접 선택한다.
지원 모델·effort 목록과 context 한도를 설정 파일로 늘릴 수는 없다.

외부 플러그인 agent는 `plugin-name:agent-name`처럼 native의 실제 이름을 사용한다.
Clauduct의 pair는 역할의 실행 선택을 바꾼다. 역할의 원래 prompt·tools·권한은 native가 읽고 적용한다.
정의가 모델 alias를 사용하면 Clauduct 매핑으로 변환하고, 전체 GPT ID를 사용하면 직접 선택한다.

설정한 agent pair는 일반 역할 정의보다 우선하며, 호출에서 명시한 모델·effort와 관리되는 역할 정의의
우선순위는 보존한다. 모델 또는 effort를 명시한 Agent·Workflow 호출은 해당 명시값을 적용한다.
내장 역할은 `Explore`, `Plan`, `general-purpose`의 이름으로 설정한다.
native의 부모 상속 역할(`fork`, `workflow-subagent`, `clauduct-inherit`)은 별도 pair로 바꾸지 않는다.

## 현재 세션의 선택과 저장

native의 모델·effort 화면에서 `S`로 확정하면 현재 세션의 선택이 된다. 이 선택은 수동
`.clauduct/settings.json`을 수정하지 않는다. `Enter`를 비롯한 native 키의 원래 동작은 그대로다.

Clauduct는 `.clauduct/sessions/<UUID>.json`에 당시 모델 매핑·기본값·agent 설정과 마지막 선택을 보존한다.
대화 본문은 이 파일에 복사하지 않는다. 질문을 보내지 않고 `S` 선택 후 종료한 경우도 native 기록에서
선택을 확인한다. 같은 기록의 동시 작성은 OS 파일 잠금으로 차단한다.

`--resume <UUID>`로 재개하면 그 snapshot과 마지막 선택을 사용한다. 이후 새로 만드는 agent도 같은
snapshot을 따른다. `--model`, `--effort`와 사용자가 지정한 native effort 환경변수는 명시적인 선택으로 적용한다.
`--fork-session`은 원본의 snapshot과 선택을 복제한다.

`/clear`는 현재 실행의 snapshot과 선택을 유지한다. 파일 편집은 새로운 Clauduct 실행에서 읽으며,
저장된 세션을 재개할 때는 그 세션의 snapshot을 사용한다.

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

## 코드에 유지하는 정책

모델의 지원 기능, context 한도, 입력 검증과 처리 상한, native hook 연결 및 보안 규칙은 코드에서 관리한다.
추가된 두 native 차단 규칙은 `$defaults`와 함께 유지한다. Auto mode classifier의 Luna/high 선택도
개인화 설정으로 변경하지 않는다.
