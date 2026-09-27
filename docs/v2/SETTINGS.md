# Clauduct 전용 설정

v0.5.3 개발 브랜치의 설정 형식이다. 정식 출하 검증은 아직 완료되지 않았다.

Clauduct는 실행할 때 사용자 홈의 `.clauduct/settings.json`을 읽는다. 파일은 직접 편집한다.
파일이 없으면 내장 기본값을 사용하며, 있는 파일이 잘못됐으면 실행을 거부한다.
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

## 코드에 유지하는 정책

모델의 지원 기능, context 한도, 입력 검증과 처리 상한, native hook 연결 및 보안 규칙은 코드에서 관리한다.
추가된 두 native 차단 규칙은 `$defaults`와 함께 유지한다. Auto mode classifier의 Luna/high 선택도
개인화 설정으로 변경하지 않는다.
