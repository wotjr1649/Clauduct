# Clauduct 설정 v2

선택 파일은 `~/.clauduct/settings.json`이다. 파일이 없으면 `{"version":2}`와 같은 기본값으로 실행한다.
일반 실행, 설치, 업데이트는 파일을 만들거나 수정하지 않는다.

## 설정 예

```json
{
  "version": 2,
  "boundaryProfile": "default",
  "isolation": "off",
  "context": {
    "window": 272000,
    "compactPercent": 90
  },
  "classifier": {
    "model": "gpt-6.1-terra",
    "effort": "low"
  }
}
```

| 항목 | 허용값 | 생략 시 |
| --- | --- | --- |
| `version` | 정수 `2` | 파일이 있으면 필수 |
| `boundaryProfile` | `default`, `strict` | `default` |
| `isolation` | `off`, `on` | `off` |
| `context` | 두 키가 모두 필요하다. `window`는 100,000~872,000 사이의 정수, `compactPercent`는 1 이상의 정수다. 90을 넘으면 실행값을 90으로 제한한다 | 272,000과 90 |
| `classifier` | 정확한 backend `model` ID와 `effort`가 모두 필요하다. 허용 effort는 `low`, `medium`, `high`, `xhigh`, `max`다 | `gpt-6.1-terra/low` |

키 이름은 정확히 일치해야 한다. 모르는 키와 값, 중복 키, `null`, v1 파일은 native 실행 전에 `CLAUDUCT_SETTINGS_INVALID`로 거부된다.
오류 메시지의 예를 따라 직접 편집한다. 기존 파일의 `startup`은 native `model`·effort 설정으로, `modelMapping`은 tier 환경변수로 옮길 수 있다.
`agents` 설정의 역할별 선택은 native agent 정의에 넣는다.

## 경계 프로필 (v0.6.11)

`boundaryProfile`은 Clauduct 세션에서 native가 Anthropic 서버로 가는 경로 가운데 무엇을 남길지 정한다(#301).
값은 세션 `--settings`에 들어가며, 이 설정은 user·project·local settings 파일보다 우선한다. 모델 요청은 프로필과
상관없이 항상 이 세션의 gateway로 간다([라우팅 불변식](COMPATIBILITY.md#2-제약)).

두 프로필에 공통인 세션 기본 환경 `CLAUDE_CODE_DISABLE_FAST_MODE=1`은 native의 fast mode와
`penguin_mode` 확인을 끈다(v0.6.13). 사용자가 셸이나 native settings의 `env`에서 같은 이름을 다르게 주면
그 값이 우선하는 기존 기본값 규칙을 따른다. native 2.1.321의 print·TUI·`--bg`에서 확인했다.

| | `default` | `strict` |
|---|---|---|
| claude.ai connector 자동 연결(`disableClaudeAiConnectors`) | 끔 | 끔 |
| DesignSync tool(claude.ai/design) | 끔(`permissions.deny`) | 끔 |
| native 자동 업데이트, 시작 때 MCP registry 목록, release notes | 켬 | 끔(`CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`) |
| WebFetch 사전 domain 점검(`api.anthropic.com`) | 켬 | 끔(`skipWebFetchPreflight`) |
| Artifact 계열 tool | native 기본(flag가 없으면 나오지 않음) | 끔(`permissions.deny`) |
| 공식 marketplace 자동 등록 | native 기본 | 사용자가 `CLAUDE_CODE_DISABLE_OFFICIAL_MARKETPLACE_AUTOINSTALL=1`을 직접 설정해야 시작 |

- `default`는 Anthropic 접속이 없는 프로필이 아니다. 무과금 loopback 측정(native 2.1.289)에서 TUI는 `api.anthropic.com`,
  `downloads.claude.ai`, `github.com`, `api.github.com`, `raw.githubusercontent.com`에 접속하려 했다. `strict`는 같은 측정에서
  외부 호스트 0개였다.
- 이 과거 `strict` 측정은 native 2.1.321의 background 공유 supervisor·dispatch 통신이나 저장 Claude login의 계정 조회까지
  꺼진다는 보장이 아니다. v0.6.13에 남는 경로와 분류는 [서버 경계 원장](COMPATIBILITY.md#서버-경계-원장-v0611)을 따른다.
- `strict`가 WebFetch 사전 점검을 끄면, native가 WebFetch로 가져올 domain을 Anthropic에 묻지 않는다. 그 대신 Anthropic의
  차단 목록 검사도 사라진다. 이 보안상 trade-off를 사용자가 감수한다.
- marketplace 변수는 native가 처음 볼 때 자기 설정에 "정책으로 막힘"을 영구 기록한다. 그래서 그 뒤 Clauduct 없이 쓰는 native에서도
  공식 marketplace 자동 등록이 꺼진 채로 남는다. Clauduct는 이 상태를 대신 바꾸지 않는다. `strict`는 사용자가 이 변수를 시작 환경에
  native가 켬으로 읽는 값(`1`·`true`·`yes`·`on`)으로 둔 경우에만 시작하고, 아니면 `BOUNDARY_PROFILE_STRICT`로 거부한다.
  시작한 뒤에는 사용자의 그 값을 세션 `--settings` env에 다시 넣어, settings 파일 env가 사용자의 선택을 조용히 되돌리지 못하게 한다.
- 사용자 `--settings`의 `permissions`는 유지되고 프로필의 deny 규칙이 더해진다. 프로필 값을 되돌리는 값
  (`disableClaudeAiConnectors: false`, `skipWebFetchPreflight: false`, strict에서 `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC`
  변경)은 `SETTINGS_CONFLICT`로 거부한다.

## 설정 동기화 (v0.6.17)

v0.6.17은 v1 파일의 누락된 키를 메모리로 채웠다. v2는 파일에 있는 키만 읽고 나머지에는 위의 기본값을 적용한다.
`clauduct --dev --init-settings`는 파일이 없을 때 `{"version":2}`를 만든다. 이미 있는 파일은 보존한다.
`clauduct --dev --sync-settings`는 제거되어 오류를 반환한다.

## 모델과 agent

새 세션 모델의 우선순위는 native `--model` → `ANTHROPIC_MODEL` → 활성 native 설정의 `model` → `ANTHROPIC_DEFAULT_MODEL`이다.
Clauduct는 이 선택이 모두 비었을 때만 신선한 계정 목록을 보고 `ANTHROPIC_DEFAULT_MODEL`로 시작값을 전달한다.
`--setting-sources`와 마지막 `--settings`가 선택 범위에 반영된다. 대화형 실행은 native의 workspace 신뢰 상태를 따른다. `-p`는 신뢰 대화 상자 없이 활성 프로젝트 설정을 읽는다.
실제 첫 본 대화 요청이 확정한 pair와 다르면 backend에 보내기 전에 거부한다.

effort는 native의 `CLAUDE_CODE_EFFORT_LEVEL`, `--effort`, 모델별 `modelSettings`, 일반 `effortLevel` 순으로 확인한다.
생략했다면 계정의 `default_reasoning_level`을 쓴다. 계정이 제공하는 정확한 모델 ID와 effort를 사용한다.

| 별칭 | 환경변수 | 비어 있을 때의 호환 매핑 |
| --- | --- | --- |
| `fable` | `ANTHROPIC_DEFAULT_FABLE_MODEL` | `gpt-6-astra` |
| `opus` | `ANTHROPIC_DEFAULT_OPUS_MODEL` | `gpt-6.1-sol` |
| `sonnet` | `ANTHROPIC_DEFAULT_SONNET_MODEL` | `gpt-6.1-terra` |
| `haiku` | `ANTHROPIC_DEFAULT_HAIKU_MODEL` | `gpt-6-luna` |

tier 환경변수에는 계정의 전체 모델 ID를 적는다. 적용된 tier를 사용하려면 그 모델이 계정 목록에 있어야 한다.
내장 Agent의 모델과 effort는 native가 선택한다. Agent·Workflow 호출에서 명시한 선택과 native의 agent 정의는 기존 연결 근거를 통해 적용한다.
사용자가 만든 역할의 도구와 권한은 해당 native 정의를 따른다.
[Claude Code 모델 설정](https://code.claude.com/docs/en/model-config)

계정이 은퇴 모델을 더는 나열하지 않을 때는 `MODEL_RETIRED`와 후속 모델을 안내한다. [실측 근거](COMPATIBILITY.md#알려진-제한-원장-v0618)

## 계정 모델 목록 (v0.6.4)

일반 세션을 시작할 때마다 Codex 계정의 모델 목록을 받는다
(`GET https://chatgpt.com/backend-api/codex/models?client_version=MAJOR.MINOR.PATCH`).
요청은 그 세션의 credential provider로 보내므로 이후 요청과 같은 계정에 묶인다. 세션 도중 계정이 바뀌면
기존처럼 `CREDENTIAL_ACCOUNT_CHANGED`로 거부한다. 요청은 10초, 본문 4 MiB, 모델 512개로 제한하며
redirect는 거부하고 목적지는 고정이다. 각 모델의 ID, 지원 수준, 기본 수준, visibility, priority와 upgrade 대상 ID를 사용한다. 설명·지침 문구는
무시한다. ChatGPT 인증 방식이므로 `supported_in_api`로 거르지 않는다.

받은 목록은 `~/.clauduct/account-models.json`에 저장한다. 계정 키는 계정 ID의 SHA-256이며 token이나 원래 ID는
저장하지 않는다. 받기에 실패하면 같은 계정의 마지막 정상 목록을 쓰고 stderr에 알린다(status
`session.modelList.source`가 `last-good`, `fetchFailure`에 실패 범주). 그 목록도 없으면
`ACCOUNT_MODEL_LIST_UNAVAILABLE`로 시작을 거부한다. Codex 자체의 `models_cache.json`은 근거로 쓰지 않으며
`clauduct --dev --doctor`가 참고로만 보여 준다. `--help`·`--version`과 내부 hook·helper 경로는 목록을 받지 않는다.

자동 시작 후보는 표시 대상이며 이 빌드가 라우팅할 수 있고 계정의 기본 effort를 쓸 수 있는 모델이다.
후보 중 가장 낮은 숫자 priority가 단독으로 확인돼야 한다. 동점, 후보 순위 누락, 유효 후보 없음, last-good만 있으면
`ACCOUNT_PRIORITY_UNAVAILABLE`과 `--model` 안내를 반환한다. 저장 목록은 명시한 pair와 저장 세션 pair의 검증에 쓸 수 있다.
upgrade 대상은 doctor의 안내에 쓰인다.

목록은 세션 동안 고정된다. picker, 라우팅, Agent 스키마, native hook 모듈이 같은 목록을 쓰며, 더 새로운 목록은
다음 세션부터 적용한다. 이 GET은 추론 요청이 아니며 검증 예산 원장에 예약하지 않는다.

## 계정 목록에 없는 선택

지원 여부는 모델과 effort를 함께 대조한다. 관리 `availableModels` 또는 `maxEffortLevel`이 pair를 막거나
native `modelOverrides`가 선택을 바꾸면 시작을 거부한다. 자동 재선택이 설정된 `fallbackModel`과 `--fallback-model`도 거부한다.
기존 프로필의 선택이 목록에서 사라졌으면 복원할 때 오류를 반환한다. 분류기 pair의 사용 불가 오류는 분류 요청에 한정된다.

## 전역 context와 압축 목표

두 context 항목은 모델별 설정이 아니다. Sol·Luna·Astra·Terra와 자식 Agent에 같은 값을 적용한다.
Clauduct는 실행 시 `floor(window * min(compactPercent, 90) / 100)`으로
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

v0.6.4부터 자동·수동 압축 모두 현재 선택을 사용한다. Agent는 자기 route를, 그 밖에는 native가 압축 요청
자체에 적은 모델과 effort를 쓴다(한 번만 쓰는 system turn의 effort는 적용하지 않는다). 이전 요청의 route를 다시 쓰지
않으므로, 모델을 바꾼 뒤의 압축은 새로 선택한 모델에서 실행된다(v0.6.3까지는 이전 route였다).
v0.6.4의 effort별 1회 압축 표본은 [호환성 기록](COMPATIBILITY.md#v064--계정-모델-목록-권한-규칙-제거-설정-동기화)에 있다.

## 보조 요청과 auto 권한 분류기 (v0.6.4)

background 완료 판정 등 도구 없는 독립 보조 요청은 명시 effort가 없으면 계정 기본값을 쓴다.
이전 세션 프로필을 재개하면 당시 설정에 있던 모델별 effort도 읽는다. 같은 요청의 token count와 generation은 같은 선택을 사용한다.

auto 권한 모드의 분류 요청은 `classifier`의 pair로 보낸다. 기본값 `gpt-6.1-terra/low`는 본 대화 모델과 독립이다.
계정이 제공하고 활성 모델 정책이 허용하는 pair로 바꿀 수 있다. 생성과 계수가 같은 pair를 쓰며,
요청 출처는 `native-auto-mode+classifier_model`이다. 적용값은 status의 `session.classifierModel`, `classifierEffort`, `classifierModelSource`에 남는다.

계정이 그 pair를 제공하지 않으면 분류 요청을 `AUTO_MODE_CLASSIFIER_MODEL_UNSUPPORTED`로 거부하고 다른 모델로
대체하지 않는다. 판정을 얻지 못한 경우(HTTP 오류, 정책 거부, 시간 초과, 읽을 수 없거나 끊긴 판정, 제공되지
않는 pair) native는 해당 호출을 실행하지 않고 turn을 마쳤다. 측정 범위는 [호환성 문서](COMPATIBILITY.md)의 v0.6.4 절을 따른다.

분류 품질은 같은 공개 표본(핵심·독립)으로 일부 pair만 측정했다. 통과한 pair는 Terra/low·medium이다.
Terra/high, Luna/low·medium은 위험 표본을 허용했고(Luna/low는 2026-10-03 재측정에서도 32개 중 2개), Sol 6.1/low는
보안 표본 하나를 backend가 `cyber_policy`로 거부해 판정을 받지 못했다. v0.6.12에서 잰 Terra 6.1은 low가 위험 허용 0이지만
핵심 정상 표본 하나를 차단했고, medium·high는 보안 표본 하나를 backend가 `cyber_policy`로 거부해 판정을 마치지 못했다.
v0.6.12는 공장값을 `gpt-5.6-terra`/low로 두었다. v0.6.13은 소유자 결정(#378)으로 `gpt-6.1-terra`/low로 바꿨다. 위험 허용 0·판정
거부 0이라는 안전 기준은 충족하지만, 승인된 태그 push 같은 핵심 정상 동작이 auto 모드에서 막힐 수 있다는 측정 결과를 받아들인
선택이다. 이 공장값 pair는 시작할 때마다 안내하지 않고 `session.classifierFinding`에만 남긴다.  v0.6.7부터는 통과하지 않았거나 측정하지 않은
pair를 고르면 시작할 때 그 사실을 한 줄로 알리고, 설정대로 사용한다([COMPATIBILITY.md](COMPATIBILITY.md) v0.6.7 절).
계정 목록에 있다는 사실은 분류 품질에 대한 주장이 아니다. 과거 오허용과 미검증 조합은
[#218](https://github.com/wotjr1649/Clauduct/issues/218)에 남기며 이 설정의 구현을 안전성 통과로 해석하지 않는다.

## 권한과 요청 출처 확인 (v0.6.4)

v0.6.4부터 Clauduct는 native 권한 규칙을 더하지 않는다. v0.6.2–v0.6.3이 `permissions.ask`에 더하던
실행·외부 통신 도구 17개, v0.5.2부터 더하던 `autoMode.hard_deny` 규칙 2개, v0.6.3의 bypass 시작 모드 분기가 없어졌다.
권한은 native의 기본값, user·project·local 설정, 관리 정책, 세션 중 모드 전환이 정한다. 사용자·관리 정책의
`ask`·`deny`는 바꾸지 않고 전달하며 전역 설정 파일은 수정하지 않는다. status의 `session.requiredAsk`는 없어졌다.
`--dangerously-skip-permissions`는 계속 전달하지 않는다.

**보안상 결과.** auto 모드에서 `Bash`·`WebFetch` 같은 도구는 이제 native 규칙과 native 분류기만으로 결정된다.
Clauduct가 더하던 확인은 더 이상 적용되지 않는다. 확인이 필요한 도구는 native 설정의 `ask`·`deny` 규칙이나
다른 권한 모드로 정한다. [native 권한 규칙](https://code.claude.com/docs/en/permissions).

승인 화면과 결정은 native가 관리한다. 사용자가 구성한 `PermissionRequest` hook도 native의
승인 주체가 될 수 있으므로 그런 hook의 효과는 유지한다. 명시적 `deny`는 그 승인보다 우선한다.
대화에 적은 승인 문구만으로 확인 화면을 자동 처리하지 않는다. 확인을 처리할 수 없는 headless나
background 작업은 native 모드에 따라 대기하거나 거부될 수 있다. native 2.1.284의 로컬 검증에서
`default`는 지정된 `PermissionRequest` hook으로 승인했고, `dontAsk`는 같은 hook을 호출하지 않고
거부했다. [native hook 규약](https://code.claude.com/docs/en/hooks#permissionrequest).

native의 Always 선택은 Ask 종류에 따라 남기는 것이 다르다. 명령 Ask의 "don't ask again for: <명령>"은
local 설정의 `permissions.allow`에 그 명령 규칙을 더하고, 재시작 뒤에도 같은 명령을 묻지 않는다.
디렉터리 접근 Ask의 "always allow access"는 명령 승인이 아니므로 같은 명령을 다시 물을 수 있다.
native 2.1.296의 default 모드에서 `echo >>`·`tee -a`처럼 파일을 만드는 쓰기는 작업 디렉터리 안이어도
디렉터리 접근 Ask를 냈고, 승인 뒤에도 같은 명령을 다시 물었다. Clauduct 없이 native만 실행해도 같았다.
Clauduct는 디렉터리 승인을 명령 허용으로 바꾸지 않는다. 반복 확인을 없애려면 명령 Ask에서 Always를
고르거나 native 설정에 `allow` 규칙을 둔다.

요청마다 출처를 확인하는 장치는 유지한다. 아래가 그 범위다. v0.6.4에서 `NATIVE_CONFIRMATION_UNVERIFIED`는
이 출처 증명이 실패한 상태(확인 교환의 I/O 오류·시간 초과)만 뜻하며, ask 규칙의 부재를 뜻하지 않는다.
필수 ask 규칙의 존재 확인과 `allowManagedPermissionRulesOnly` 검사는 없어졌다.
직접 입력한 forked Skill은 native 이벤트에 Agent ID가 있어도 첫 HTTP 요청에는 그 ID가 없다.
활성 자식이 하나여도 이전 자식의 지연 요청과 구별할 수 없으므로, v0.6.2에서는 이 직접 입력 경로를
지원하지 않는다. 도구 유무와 관계없이 생성·계수 요청을 `NATIVE_REQUEST_ORIGIN_UNVERIFIED`로
backend 전송 전에 거부한다. Agent ID가 있는 native Agent·fork 및 식별된 병렬 실행은 유지한다.
출처나 현재 scope가 맞지 않아 거부한 요청은 이후 정상 요청을 막는 저장 실패 상태로 취급하지 않는다.
모델이 낸 도구 호출의 `tool_use_id`에는 gateway가 그 호출을 받은 native step의 표지를 붙인다
(`<backend call_id>__cdt<12자리 16진수>`). backend에는 원래 `call_id`를 그대로 돌려보낸다. native가
도구를 실행할 때 표지가 현재 session·Agent·turn·step과 맞지 않으면, 이전 turn이나 step의 늦은 호출로 보고
`NATIVE_REQUEST_ORIGIN_UNVERIFIED`로 실행 전에 거부한다. 표지가 없는 호출은 plugin hook 모듈이 모델 호출 없이
`$.tool.call`로 직접 실행한 도구다(native 2.1.287 측정: `toolu_plugin_…` ID). 이런 호출은 native 권한
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
승인 화면은 native의 기존 규칙을 따른다. v0.6.11부터 확인 왕복 2초 안에 그 native의 helper가 아무 답도
보내지 못하면(느린 기계, session link 점유) 그 요청만 `NATIVE_CONFIRMATION_TIMEOUT`(HTTP 503)으로 거부한다.
backend로 보내지 않은 요청이므로 native가 같은 step에서 스스로 재시도하고(native 2.1.302 측정: 약 0.5초 뒤),
재시도도 새 확인값을 받아야 한다. native module이 계속 응답하지 않으면 native의 재시도(지수 backoff, 2분 이상)가
끝난 뒤 오류가 표시된다. 확인 저장소 I/O 오류, 형식이 틀리거나 다른 scope를 주장한 답, helper의 비정상 종료가
발생하면 gateway는 실패 표시 파일을 쓰지 못해도 이후 도구 요청을 거부한다. 이 상태는 `/clear`나 plugin reload로
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
관리 정책을 덮어쓰거나 무시하지 않는다.

native 압축은 다음 step보다 먼저 실행될 수 있는 요약 전용 경로다. 기존 context 검증을 거친 뒤
count와 generation 모두 도구 선택을 `none`으로 보내며, 압축 응답에 도구 호출이 오면 거부한다.
이렇게 UUID 재개 시 필요한 압축을 유지하면서 요약 요청이 도구를 실행하지 못하게 한다.

이 출처 확인은 native 권한 판정을 대신하지 않으며 모델의 판정 품질을 바꾸지 않는다.

## 현재 세션의 선택과 저장

native의 모델·effort 화면에서 `S`로 확정하면 현재 세션의 선택이 된다. 이 선택은 수동
`.clauduct/settings.json`을 수정하지 않는다. `Enter`를 비롯한 native 키의 원래 동작은 그대로다.

Clauduct는 `.clauduct/sessions/<UUID>.json`에 실제 선택, 당시 별칭 매핑과 계정 키를 보존한다.
선택은 native transcript를 대조해 확인하고, 동일 기록의 동시 작성은 OS 파일 잠금으로 차단한다. 구형 프로필의 별칭·역할·effort 설정도 읽는다.

`--resume <UUID>`와 `--fork-session`은 저장된 마지막 pair를 사용한다. `--model`이나 `--effort`를 명시하면 현재 계정과 native 정책이 허용하는 선택을 사용한다.
이전 프로필은 현재 계정 목록으로 pair를 확인한다.
복원할 기록이 없거나 손상됐으면 오류를 반환한다. resident 복원도 기록된 pair를 확인한다.

UUID 재개 시 context와 분류기는 현재 전역 v2 설정을 사용한다. 더 작은 window를 설정하면 기존 이력의 압축 시점이 앞당겨질 수 있다.
현재 설정 파일의 형식 오류는 재개도 막는다. `--no-session-persistence`는 선택을 새로 저장하지 않으며,
기존 UUID를 명시하면 저장된 pair를 읽을 수 있다.

`EnterWorktree`·`ExitWorktree`는 native가 옮긴 transcript 경로에 snapshot과 context journal을 연결한다.
기존 transcript가 사라지고 같은 UUID의 새 파일이 native projects 안에 존재하는지 확인한다.
다른 살아 있는 transcript로 바꾸거나 경계를 벗어나는 경로는 거부한다. native의 권한과 worktree 격리
규칙은 그대로 적용한다. v0.6.0에서 도구 실행 직후 경로 갱신 누락을 수정했다([#185](https://github.com/wotjr1649/Clauduct/issues/185)).

`--continue`, 제목, 대상 없는 `--resume`의 경우 시작 전에 pair를 확인할 수 없으므로 UUID를 요구한다. 실행 중 `/resume`에서
다른 기록을 고르면 UUID를 명시한 새 Clauduct 실행을 안내한다. `/clear`는 현재 실행의 선택과 context를 사용한다.

되돌릴 때는 해당 바이너리가 읽을 수 있는 설정 파일을 사용한다. 바이너리 교체는 설정이나 세션 데이터를 변경하지 않는다.

## 적용값 진단

종료 상태 JSON에서 `session.startupModel`·`startupEffort`와 각각의 `Source`를 확인할 수 있다.
`account priority`, `account.default_reasoning_level`, `native.cli.*`, `native.*.model`, `native.*.env.*`,
`session-snapshot.last`가 기록된다. 호환 별칭을 썼다면 모델 출처에 `compat mapping`이 추가된다.

`clauduct --dev --doctor`는 설정과 같은 계정의 저장 목록을 읽어 모델·effort 출처, 분류기 pair, 제공된 upgrade 대상을 보여 준다.
doctor는 추론이나 계정 목록의 실시간 조회를 실행하지 않는다. 자동 시작에는 다음 실행의 신선한 응답이 필요하다.

실행 중 선택은 `gateway.recent`와 `gateway.agentSelections`에서 확인한다. `session.context`에는 window, 요청한 압축 비율,
적용한 비율, 압축 목표 및 출처가 기록된다. `context default`는 각 항목의 기본값을 뜻한다.
`session.modelList`에는 목록의 출처와 조회 시각, 제외된 모델, 적용할 수 없는 설정, 조회 실패 범주가 남는다.

## 격리 모드 (v0.6.14)

`isolation: "on"`은 Clauduct가 띄우는 native가 **저장된 Claude 로그인과 계정에 묶여 Anthropic에 보내는 부수 요청**(telemetry, feature flag
조회, 계정 조회, 시작 때의 quota 확인)을 줄이는 선택 기능이다([#379](https://github.com/wotjr1649/Clauduct/issues/379)). 사용자의 커스터마이즈를
숨기지 않는다. 떼어 내는 것은 (1) 로그인과 계정 정보, (2) supervisor와 실행 중 상태 둘뿐이다. 기본(`off`)은 기존 실행 경로를 사용한다.
격리를 사용한 뒤에는 격리 모드에서 실행 중인 세션에 대한 `--resume`을 기본 모드에서도 거부한다.

켜면 모든 Clauduct 실행(foreground·`--bg`·`attach`·`agents`·`respawn`·`stop`·`rm`·`logs`)이 native에
`CLAUDE_CONFIG_DIR=<전용 폴더>`와 `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`·`DISABLE_TELEMETRY=1`·`DISABLE_ERROR_REPORTING=1`을
강제한다(사용자의 셸 값보다 우선). 전용 폴더는 `~\.clauduct\isolated\claude`이고 지우기 전까지 남는다. native는 설정 폴더마다 자기 supervisor를
따로 띄우므로 격리 job은 별도 supervisor 하나를 공유한다. Clauduct는 그 supervisor를 자동으로 끝내지 않는다.
[Agent View 상태 저장 위치](https://code.claude.com/docs/en/agent-view#where-state-is-stored)

| 사용자 폴더(`~\.claude` 또는 `CLAUDE_CONFIG_DIR`)의 항목 | 격리에서 |
|---|---|
| `rules` `agents` `commands` `skills` `output-styles` `workflows` `themes` `agent-memory` `plans` `tasks` `todos` | 디렉터리 junction으로 **공유** |
| `plugins`(설치본·marketplace·plugin data) | 격리 실행은 `CLAUDE_CODE_PLUGIN_CACHE_DIR`의 비어 있지 않은 셸 값, 원본 `settings.json`의 `env` 값, 사용자 폴더의 `plugins` 순으로 루트를 고른다. `~`, `~/`, `~\`는 OS 홈으로 펼친다. native와 Clauduct의 플러그인 역할 조회가 이 루트를 사용한다. 계정 동기화용 `synced/`와 `.trash/`의 위치는 native 설정 폴더에 속한다. [플러그인 저장 위치](https://code.claude.com/docs/en/plugins/loading#find-plugins-on-disk) |
| `projects`(대화 transcript·프로젝트 auto memory) `file-history` `paste-cache` | junction으로 공유 |
| `history.jsonl` | hard link로 공유(native가 교체 쓰기를 하면 끊길 수 있고, 그때는 시작 때 알린다) |
| `CLAUDE.md` | 전용 폴더의 `CLAUDE.md` 첫 줄에서 공식 `@` import로 사용자 `CLAUDE.md`를 읽는다. 그 아래에 적은 내용은 보존된다(격리 세션에서 `/memory`로 편집하면 거기에 들어간다). [CLAUDE.md import](https://code.claude.com/docs/en/memory#import-additional-files) |
| `settings.json` | **실행마다 파생**: 인증 helper(`apiKeyHelper`·`aws…`·`gcpAuthRefresh`·`otelHeadersHelper`)·로그인 제한 키·endpoint/credential/provider `env`·telemetry 재활성 `env`를 뺀다. Remote Control, 계정 플러그인 동기화·connector, 피드백 초안·설문을 끈다. `syncClaudeAiSkills`와 `ANTHROPIC_MODEL` 같은 모델 선택 `env`는 사용자 값을 유지한다. 격리에는 저장된 Claude 로그인이 없으므로 계정에서 스킬을 내려받지 않는다 |
| `.claude.json` | **실행마다 파생**: 이름으로 고른 키만(user·project `mcpServers`, MCP 토글, 전역 UI 토글, 불리언 온보딩·프로젝트 신뢰). 계정(`oauthAccount` 등)·식별자는 가져오지 않는다 |
| `keybindings.json` | 복사 |
| `.credentials.json`(저장 login), `jobs`·`daemon`·`sessions`·캐시 등 그 밖의 모든 것 | 전용 폴더에 따로 보관한다. 원본 `.credentials.json`은 Claude 로그인도 저장하므로 링크하거나 복사하지 않는다. |

- 위 사용자 설정 폴더의 공유 구성에서 Windows junction과 `history.jsonl` hard link를 사용하는 동작은 native의 공식 문서에 명시되어 있지 않다.
- Clauduct 런처는 격리를 초기화할 때 원본 설정을 읽고, 공유할 디렉터리가 없으면 사용자 폴더에 빈 디렉터리를 만든다. 격리에서 실행된 native는 공유 링크를 통해
  transcript·프롬프트 이력·공유 자산을 사용자 폴더에 직접 쓸 수 있다. 사용자나 관리자 설정이 스킬 동기화를 끄면 native의 정리 정책은 공유 `skills`에도 적용될 수 있다.
  전용 폴더에서 공유 이름이 이미 다른 용도로 쓰이고 있으면 `ISOLATION_LINK_CONFLICT`로 거부한다.
  [스킬 동기화 설정](https://code.claude.com/docs/en/settings-reference#syncclaudeaiskills)
- 이전 빌드의 전용 `plugins` junction은 사용자 폴더의 `plugins`를 가리킬 때 링크만 제거한다. `ISOLATION_PLUGIN_MIGRATION_BUSY`가 나오면
  `CLAUDE_CONFIG_DIR`을 오류에 나온 전용 폴더로 설정하고 `claude daemon stop --any`를 실행한 뒤 다시 시작한다.
  전용 `plugins` 경로가 다른 대상을 가리키거나 별도 데이터가 들어 있으면 `ISOLATION_LINK_CONFLICT`로 시작을 거부한다.
- 격리 모드는 플러그인 루트 변수를 파생 `settings.json`에서 제거하고 launch 환경과 세션에 같은 루트를 전달한다. 다른 루트를 지정한
  `--settings` 또는 관리 설정은 격리 시작을 거부한다. 캐시 루트를 바꿀 때는 기존 격리 supervisor를 종료한 뒤 새 세션을 시작한다.
- 두 모드가 같은 플러그인 루트를 사용하면 설치 목록을 공유한다. 사용자 범위 플러그인의 활성화 상태는 원본 `settings.json`에서 관리한다. 같은 프로젝트에서 두 모드가 범위를
  공유하려면 프로젝트 또는 로컬 범위로 설치한다.
- **전용 폴더를 직접 지우지 않는다.** junction을 따라가는 도구는 사용자 자산을 지울 수 있다. `clauduct --dev --isolation-reset`(`--yes`)가 링크를 먼저
  떼고 지운다. 전용 supervisor가 살아 있으면 거부한다. 원본 속성을 바꾸지 않고 `history.jsonl` 링크를 제거할 수 없으면 reset이 오류를 반환한다.
- 파생 파일은 한 방향이다. 격리 세션 안의 `/config`·`claude mcp add` 변경 중 파생으로 덮이는 값은 다음 실행에서 사라진다.
  시작 경고는 전용 `settings.json`의 해시가 직전 파생본과 다를 때 표시된다. 파생 변경은 원본에 되쓰지 않는다.
- 격리 중인 native가 이미 돌고 있을 때 두 번째 격리 실행이 시작되면, `.claude.json` 파생은 쓰기 직전에 파일을 다시 읽어 native가 그 사이에 쓴 자기 상태를
  유지한 채 병합하고(최대 3번), 가져오는 키가 바뀌지 않았으면 아예 쓰지 않는다. 마지막 읽기와 교체 사이의 짧은 틈은 남는다. native와 공유하는 잠금이 없어 그 틈에
  native가 쓴 값은 사라질 수 있다(native의 자기 상태이며 다음 저장 때 다시 쓰인다).
- 사용자가 `CLAUDE_CONFIG_DIR`을 줬다면 그 폴더가 "사용자 폴더"다. 전용 폴더와 겹치는 값은 `ISOLATION_CONFIG_CONFLICT`로 거부한다. 사용자 폴더가
  네트워크 경로(`\\server\share\…`)이면 junction을 만들 수 없어 `ISOLATION_LINK_TARGET_UNSUPPORTED`로 거부한다(로컬 드라이브 경로여야 한다).
- 전용 `CLAUDE.md`의 첫 줄은 Clauduct가 쓴 import(사용자 `CLAUDE.md`를 가리키는 정확히 그 줄, 대소문자 무시)만 관리한다. 사용자가 직접 쓴 다른 import는 그대로 둔다.
- `boundaryProfile`과 독립이다. 격리만으로는 WebFetch 사전 점검과 공식 marketplace 자동 등록이 남는다. 접속을 최소화하려면 `strict`와 함께 쓴다.
- 대화 이력은 공유하므로 두 모드 사이에 `--resume`이 된다. 다른 모드의 supervisor에서 **실행 중인** job의 세션은 `RESUME_RUNNING_IN_OTHER_MODE`로 거부하고,
  멈춘 세션은 허용한다. 실행 중 `/resume` 선택기와 `--continue`는 native의 것이라 이 검사를 받지 않는다. 사용자가 직접 넘기는 대화는 막지 않는다:
  Anthropic 모델로 만든 대화를 Clauduct에서 이어 가면 그 이력이 Codex/OpenAI backend로 가고, GPT 세션을 Clauduct 없이 여는 native로 이어 가면
  Anthropic으로 간다(격리와 무관하게 기본 모드에서도 같다).
- 다른 모드의 `agents` 목록에는 이 job이 보이지 않는다. `--bg` 안내문이 `clauduct agents`를 가리킨다.
- NONESSENTIAL의 부작용: native auto-update, `/feedback`, release notes, PR 상태 배지, fast mode 등 availability 확인, plugin `command` source의 background 실행,
  marketplace 자동 갱신, **feature flag 조회**(Remote Control 등 flag 의존 기능, claude.ai 계정 plugin·skill 동기화)가 꺼진다.
- OAuth가 필요한 원격 MCP 서버는 격리 세션의 `/mcp`에서 서버별로 한 번 인증한다. 플러그인의 민감한 설정값도 격리에서 다시 입력해야 할 수 있다.
  managed 정책(기계 전체)은 격리되지 않는다. Windows 전용. [원격 MCP 인증](https://code.claude.com/docs/en/mcp#authenticate-with-remote-mcp-servers)
- 원본에서 프로젝트 신뢰 항목을 지워도 이미 전용 파일에 들어간 승인은 남을 수 있다. 전용 신뢰 상태를 초기화하려면 원본에서 해당 항목을 지운 뒤
  `clauduct --dev --isolation-reset`(`--yes`)을 실행한다. 이 명령은 전용 작업과 파생 파일도 지운다.
- 켜진 세션은 시작 때 `clauduct: isolation on: …`을 stderr에 한 줄 내고, 종료 줄에 `isolation=on`, status의 `session.isolation`에 이름·개수를 남긴다.
  `clauduct --dev --doctor`가 모드·폴더·공유 상태·supervisor를 보여 준다.
- `isolation`을 생략하면 `off`를 사용한다.

- MCP 서버 정의는 전용 `.claude.json`으로 미러링된다. 비밀값은 지원되는 필드에서 `${VAR}`로 확장하거나 HTTP 헤더를 `headersHelper`에서 가져온다.
  MCP 정의에 평문 비밀을 넣으면 전용 `.claude.json`에도 평문 복제본이 생긴다. 미러링되는 `settings.json`의 `env`와 hook에 평문 비밀을 넣으면
  전용 `settings.json`에도 복제된다. 비밀은 실행 환경에서 제공하고, 지원되는 곳에서 환경 변수로 참조한다.
  Windows에서는 폴더의 상속 ACL이 파일 접근을 제어한다. `--isolation-reset`이 복제본을 지운다.
  [MCP 변수 확장](https://code.claude.com/docs/en/mcp#environment-variable-expansion-in-mcp-json) · [MCP 동적 헤더](https://code.claude.com/docs/en/mcp#use-dynamic-headers-for-custom-authentication)
- `rm`·`stop`·`logs`는 `~/.clauduct/settings.json`이 없으면 기본 모드를 사용한다. 파일이 있으면 유효한 v2 설정이어야 한다. 접근 오류, 깨진 JSON, 객체가 아닌 값, 중복 키,
  잘못된 설정값, 1 MiB 초과 등으로 검증에 실패하면 `ISOLATION_MODE_UNREADABLE`로 거부한다. `isolation`을 `off`로 바꾼 뒤 격리 job을 정리하려면 잠시
  `on`으로 되돌리고 `clauduct stop <id>` 또는 `clauduct rm <id>`를 실행한 뒤 다시 `off`로 바꾼다.
- 격리 마커가 손상돼도 기본 모드의 `--resume`은 전용 폴더에서 실행 중인 job을 확인한다. 전용 폴더 초기화에서 마커를 읽을 수 없으면 `ISOLATION_MARKER_UNREADABLE`로 거부한다.
- 전용 `.claude.json`을 읽지 못하면(잠김·파일 아님·8 MiB 초과) 새로 만들어 덮어쓰지 않고 `ISOLATION_DEDICATED_CONFIG_UNREADABLE`로 시작을 거부한다.
  전용 `CLAUDE.md`를 읽지 못해도 기존 내용을 보존하고 `ISOLATION_DEDICATED_FILE_UNREADABLE`로 시작을 거부한다.
  `--isolation-reset`은 전용 폴더나 그 상위가 링크이면 `ISOLATION_RESET_REFUSED`로 거부한다.
