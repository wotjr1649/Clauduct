# v0.6.19 — 설정 v2와 계정 우선순위 기반 모델 선택, 업데이트 중단 복구

**호환성이 바뀌는 릴리스다.** `~/.clauduct/settings.json`이 v1 형식이면 v0.6.19는 native를 시작하기 전에 거부한다. 파일을 v2로 고치거나 지워야 한다. 방법은 [업그레이드 안내](#v0618에서-올라올-때)에 있다. 새 세션의 시작 모델도 Clauduct 설정이 아니라 native의 선택과 계정의 우선순위 목록으로 정해진다. [#409](https://github.com/wotjr1649/Clauduct/issues/409), [PR #413](https://github.com/wotjr1649/Clauduct/pull/413).

## 바뀐 것

- **설정 파일은 v2 형식만 읽는다.** 파일이 없으면 `{"version":2}`와 같은 기본값으로 실행한다. 파일이 있으면 `version`이 `2`여야 한다. v1 파일, 모르는 키·값, 중복 키, `null`은 `CLAUDUCT_SETTINGS_INVALID`로 native 실행 전에 거부한다. v0.6.17·v0.6.18이 v1 파일의 빠진 키를 메모리로 채우던 동작은 없어졌다. 일반 실행, 설치, 업데이트는 이 파일을 만들거나 바꾸지 않는다. `clauduct --dev --init-settings`는 파일이 없을 때만 v2 최소 파일을 만든다. [설정 문서](SETTINGS.md).
- **새 세션의 시작 모델은 native의 선택을 먼저 따른다.** 순서는 native `--model`, `ANTHROPIC_MODEL`, 활성 native 설정의 `model`, `ANTHROPIC_DEFAULT_MODEL`이다. 이 선택이 모두 비었을 때만 신선한 계정 모델 목록에서 고른다. 후보는 표시 대상이고 이 빌드가 라우팅할 수 있으며 쓸 수 있는 기본 effort와 native 정책을 충족하는 모델이고, 그중 priority 숫자가 가장 낮은 모델이 단독으로 확인돼야 한다. 동점, 후보 순위 누락, 유효 후보 없음 또는 마지막 정상 캐시만 있으면 `ACCOUNT_PRIORITY_UNAVAILABLE`로 멈추고 `--model`을 안내한다. gateway가 첫 본 대화 요청의 모델·effort를 확정한 값과 대조한다. `clauduct --dev --doctor`는 저장 목록 기준의 미리보기를 보여 주고, 실제 실행값과 출처는 종료 상태 JSON의 `session.startupModel`·`startupEffort`와 `startupModelSource`·`startupEffortSource`에서 확인한다. Clauduct 설정의 `startup`·`modelMapping`·`agents`는 더는 읽지 않는다.
- **모르는 effort는 일찍 거부한다.** 새 세션을 시작할 때 적용되는 명시 effort가 `low`·`medium`·`high`·`xhigh`·`max` 밖이면 계정 모델 목록 조회와 native 인증 확인보다 먼저 `UNSUPPORTED_MODEL_OR_EFFORT`로 거부한다. 비어 있지 않은 `CLAUDE_CODE_EFFORT_LEVEL`은 `--effort`보다 우선한다. 아는 값이라도 그 모델에서 지원하는지는 계정 목록으로 확인한다. 모르는 모델·effort를 기본값으로 바꿔 실행하지 않는다.
- **재개와 fork는 저장된 선택을 쓴다.** UUID를 명시한 재개·fork는 저장된 마지막 모델·effort를 기본으로 쓴다. 계정과 정책이 허용하는 `--model`·`--effort`를 직접 줄 수도 있다. foreground에서 `--continue`를 붙이면 UUID와 함께 적어도 `SESSION_RESUME_UUID_REQUIRED`로 거부한다. background의 `--continue`는 `BACKGROUND_CONTINUE_UNVERIFIED`로 거부한다.
- **`--update`가 중간에 끊겨도 다음 실행이 복구한다.** 새 `clauduct.exe`와 `clauduct-probe.exe`를 교체하는 도중(교체 직전, exe만 교체된 직후, 둘 다 교체된 직후)에 프로세스가 끝나면 다음 실행이 업데이트 기록과 파일 해시를 대조한다. 교체 전 중단이면 기존 쌍을 유지하고, exe만 바뀌었으면 검증된 준비 probe로 새 쌍을 완성한다. 쌍이 이미 일치하면 남은 업데이트 기록과 준비 파일을 정리한다. 무결성을 확인하거나 복구할 수 없으면 `UPDATE_RECOVERY_REQUIRED`로 시작을 멈춘다. 이때는 실행 중인 Clauduct를 닫고 다시 시도하거나 같은 릴리스의 쌍을 다시 설치한다. 설치 잠금이 경합하면 `UPDATE_BUSY`이므로 잠시 뒤 다시 실행한다. 복구는 이번 업데이트의 백업으로 확인된 이전 probe 링크만 정리한다. 준비 디렉터리가 사라진 기록은 설치된 쌍이 기록과 내장 SHA-256에 일치할 때만 정리하고, 교체 뒤 남은 준비 디렉터리는 지우지 않고 경로를 알린다. 이전 exe 정리만 남은 시작이 잠금과 겹치면 정리를 건너뛰고 계속 시작한다. 병합 전에 최종 HEAD에서 NTFS 합성 설치본의 교체 경계 네 곳과 기존 probe가 없는 변형 한 곳의 강제 종료 복구, 손상된 준비 probe의 거부, 세 중단 상태 뒤 실제 backend 응답(HTTP 200, 상태마다 요청 예약 1건)을 확인했고, 같은 HEAD의 공개 CI(govulncheck·Staticcheck 포함)가 통과했다. 병합 commit은 [`a8c073b69`](https://github.com/wotjr1649/Clauduct/commit/a8c073b69353a805713bb6aa2c97fbba66bc85c7), [공개 CI 실행](https://github.com/wotjr1649/Clauduct/actions/runs/45728507210)이다. [#365](https://github.com/wotjr1649/Clauduct/issues/365), [PR #412](https://github.com/wotjr1649/Clauduct/pull/412), [업데이트 절차](PACKAGING.md#51-업데이트).
- **진단: 첫 terminal 이벤트.** 상태 JSON의 요청 기록 `streamEnd.firstTerminalType`에 검증을 통과한 첫 종료 이벤트의 종류(`response.completed`, `response.failed`, `response.incomplete`, `error`)가 남는다. 관측된 종료 이벤트가 없으면 이 필드를 생략한다. 본문, 응답 ID, 자유 형식 오류 문구는 기록하지 않는다. 사용자 동작은 바뀌지 않는다. 같은 PR의 비교 검증에서 요청 본문이 18바이트 어긋나는 현상이 보였고, 원인은 Clauduct 제품이 아니라 native의 시작 순서다. native는 기동 직후 내장 `claude-artifacts` MCP 서버에 연결하며, 첫 요청을 만들 때 아직 연결 중인 서버 이름을 안내 문구에 한 줄(`claude-artifacts\n`, 18바이트) 적는다. 연결이 첫 요청보다 수십~수백 ms 먼저 끝나는지에 따라 이 줄이 들어가거나 빠진다. Clauduct가 띄우는 native는 gateway 주소가 루프백이라 이 서버를 시작하지 않는다(2.1.332 소스 분석). 그래서 수정은 검증 하네스에만 있다(비교용 native에서 이 서버를 끄는 환경 변수). **제품이 Anthropic이나 backend로 보내는 요청은 바뀌지 않는다.** 이 조치는 18바이트 차이에 대한 것이며 W1 F4 독립 spare 행의 실패(UI에서 pty host 생성까지 지연이 5초 창을 넘음)는 설명하지 못한다. 그 원인은 별도로 조사 중이고, 최종 후보에서 해당 행을 포함한 비교 22행이 모두 통과하기 전에는 #417을 병합하지 않는다. [#416](https://github.com/wotjr1649/Clauduct/issues/416), [PR #417](https://github.com/wotjr1649/Clauduct/pull/417).

## 개발자 변경

- **시작 관문 `AdmitLaunch`를 제거했다.** 제품 경로에서 호출하지 않던 코드와 `Options.AdmittedNativeSHA256`을 지웠다. foreground, background, native provider 조회는 기존 인자로 각 시작 함수를 직접 부른다. provider-control probe와 `doctor` 진단은 그대로다. 동작 변화는 없다. #413 병합 뒤에 이어서 병합한다(#413의 시작 경로 변경과 같은 파일을 다룬다). [#419](https://github.com/wotjr1649/Clauduct/issues/419).
- **계정 모델 목록 거부 검증 명령.** `clauduct --dev --verify-catalogue-refusal`은 계정 모델 목록 요청이 401·403·503을 받을 때의 시작 경계를 네트워크 없이 확인하는 검증 전용 명령이다. 인자를 받지 않고, 임시 폴더·메모리 안의 합성 자격 증명·프로세스 전용 TLS 수신기만 쓴다. `127.0.0.1` 밖의 연결은 거부하며 native·gateway를 시작하거나 실제 backend와 OS 자격 증명 저장소를 읽지 않는다. 401·403은 `ACCOUNT_MODEL_LIST_UNAVAILABLE`로 거부하고 저장 목록을 바꾸지 않으며, 503은 같은 계정의 마지막 정상 목록을 쓴다. 검증용 응답 캡처(`CLAUDUCT_VERIFICATION_HTTP_CAPTURE=1`)도 함께 들어가며 변수가 없으면 실행되지 않는다. 일반 사용에는 쓸 일이 없고 동작도 바뀌지 않는다. [PR #413](https://github.com/wotjr1649/Clauduct/pull/413).
- **죽은 코드 정리(45줄).** 호출이 없던 함수 두 개(`Gateway.CurrentSession` 등)와 항상 거짓인 조건의 분기 세 곳을, 참조가 없음을 확인한 뒤 지웠다. 동작 변화는 없다. [#366](https://github.com/wotjr1649/Clauduct/issues/366).
- **공개 CI에 정적 분석을 더했다.** Windows 작업에서 staticcheck와 govulncheck를 실행한다. 기존 진단 25건은 소스 위치에 결속한 기준선으로 관리하고, 새 진단은 실패로 본다. 사용자 영향은 없다. [PR #411](https://github.com/wotjr1649/Clauduct/pull/411).
- **비공개 시험의 시간 결정성.** `resident` 종료 대기 시험을 가상 시간으로 바꾸고, L1·L3에서 resident 종료를 정해진 시간 안에 관측하지 못하면 해당 행을 실패로 기록하게 했다. 공개 코드는 바뀌지 않는다. [#382](https://github.com/wotjr1649/Clauduct/issues/382).

## v0.6.18에서 올라올 때

1. **업데이트 전에** `~/.clauduct/settings.json`이 있으면 복사본을 둔다. 업데이터와 설치 스크립트는 이 파일을 만들거나 바꾸지 않는다.
2. `clauduct --update`(무인 실행은 `clauduct --update --yes`)로 v0.6.19를 받는다.
3. **첫 실행이 `CLAUDUCT_SETTINGS_INVALID`로 멈추면** 파일의 `version`과 오류 안내를 확인한다. v2 파일이 오타, 잘못된 값, 중복 키, `null` 때문에 같은 코드를 낼 수도 있으므로, 그 경우에는 설정 문서에 따라 잘못된 항목만 고친다. `version`이 `1`인 v1 파일이면 먼저 파일을 백업하고 다음 중 하나를 한다.
   - 기본값으로 충분하면 파일 전체를 `{"version":2}` 한 줄로 바꾼다.
   - 바꿔 둔 값이 있으면 아래 대응표대로 옮긴다. 모르는 키가 하나라도 있으면 거부되므로 v1 전용 키는 지운다.
   - 백업한 뒤 파일을 지워도 된다. 없으면 최소 v2로 실행한다. 다시 만들려면 `clauduct --dev --init-settings`.
4. 새 세션이 다른 모델로 시작할 수 있다. 원하는 모델은 `clauduct --model <전체 모델 ID>`나 native 설정의 `model`로 고정한다. `clauduct --dev --doctor`는 저장 목록 기준의 미리보기를 보여 주고, 실제 실행값과 출처는 종료 상태 JSON의 `session.startupModel`·`startupEffort`와 `startupModelSource`·`startupEffortSource`에서 확인한다.

v1 파일이 이런 모양이었다면:

~~~json
{
  "version": 1,
  "startup": { "model": "gpt-6.1-sol", "effort": "xhigh" },
  "context_window": 272000,
  "auto_compact_token_limit_percent": 90,
  "classifier_model": { "model": "gpt-6.1-terra", "effort": "low" },
  "boundary_profile": "strict",
  "isolation": "on"
}
~~~

v2에서는 이렇게 쓴다. `startup`은 파일에서 빠지고, 시작 모델은 `clauduct --model gpt-6.1-sol --effort xhigh`나 native 설정(`model`·`effortLevel`)으로 정한다.

~~~json
{
  "version": 2,
  "boundaryProfile": "strict",
  "isolation": "on",
  "context": { "window": 272000, "compactPercent": 90 },
  "classifier": { "model": "gpt-6.1-terra", "effort": "low" }
}
~~~

| v1 키 | v2 |
|---|---|
| `context_window`, `auto_compact_token_limit_percent` | `context.window`, `context.compactPercent`(둘 다 적는다) |
| `classifier_model` | `classifier`(`model`과 `effort`를 모두 적는다) |
| `boundary_profile` | `boundaryProfile`(`default` 또는 `strict`) |
| `isolation` | `isolation`(`off` 또는 `on`) |
| `startup` | 파일에서 뺀다. native `--model`·`--effort`나 native 설정으로 옮긴다 |
| `modelMapping` | 파일에서 뺀다. `ANTHROPIC_DEFAULT_<FABLE·OPUS·SONNET·HAIKU>_MODEL` 환경변수에 계정의 전체 모델 ID를 적는다 |
| `modelDefaults`, `agents` | 파일에서 뺀다. effort는 native의 선택이나 계정 기본값을 쓰고, 역할별 선택은 native agent 정의에 둔다 |
| `auto_compact_effort_cap`, `auxiliary_effort_cap` | 지운다(v0.6.4부터 적용하지 않던 키다) |

`clauduct --dev --sync-settings`는 이전 업데이터와의 호환을 위해 남아 있다. 파일을 바꾸지 않고 v2 수동 편집 안내를 출력한 뒤 성공한다.

**v0.6.18로 되돌릴 때**는 v0.6.18이 v2 파일을 읽지 못한다. 릴리스에서 내려받은 `install.ps1`을 `pwsh -NoProfile -File install.ps1 -Tag v0.6.18`로 실행해 설치하고([PACKAGING](PACKAGING.md)), `~/.clauduct/settings.json`을 백업해 둔 v1 파일로 바꾼다. v2 파일이 그대로 남아 있으면 v0.6.18이 거부한다.

## 이전 updater(v0.6.16 이하)에서 올라올 때

v0.6.4–v0.6.16의 `--update`는 설치 뒤 `clauduct --dev --sync-settings`를 한 번 호출한다. 이 호출은 사용자가 따로 실행하는 이전 절차가 아니다. v0.6.19의 이 명령은 설정 파일을 읽거나 바꾸지 않고 v2 수동 편집 안내만 출력한 뒤 성공(exit 0)하므로 업데이트는 실패로 끝나지 않는다. 그 안내가 나오면 위 3번의 백업과 수동 v2 이전을 한다.

- **v0.6.11 이상(예: v0.6.16)**: 첫 `--update`가 `clauduct.exe`와 `clauduct-probe.exe`를 함께 받는다.
- **v0.6.10**: 이 버전의 updater는 `clauduct.exe`만 받는다. 일반 실행 전에 `clauduct --update --yes`를 한 번 더 실행해 `clauduct-probe.exe`를 받아 한 쌍을 맞춘다.

## 출하 검사

태그·commit, 후보 `clauduct.exe`와 `clauduct-probe.exe`, native 버전과 SHA-256에 결속한 `GATE-T1` 결과는 발행 뒤 이 문서에 기록한다. 마지막 유효 T1·T2의 native 버전과 전체 SHA-256이 이번 native와 같은지에 따라 `c1` 도구 표면 재측정 여부를 정하며, 그 결정과 근거도 함께 기록한다. 이번 후보가 확인하는 바뀐 동작은 다음과 같다.

- 설정: v2 최소 파일·파일 없음. 업데이트 뒤 v1 파일은 native 시작과 예약 0건으로 거부하고, 수동으로 v2로 옮긴 뒤에는 실행에 성공한다. 롤백 뒤에는 v2 파일을 거부하고 v1을 복원하면 성공한다. 새 설치는 설정 파일을 만들지 않고 `--dev --init-settings`가 명시적으로 만든다. v1·v2 파일의 바이트·mtime과 PATH는 설치·업데이트·롤백 전후에 보존한다.
- 모델 선택: 계정 priority 기본값, native 명시 선택, 명시 UUID 재개·fork의 pair 복원, 모르는 effort의 조기 거부, `--resume <uuid> --continue` 거부. 같은 설정 표본에 두 Agent View 작업, 분류기 4건, context·역할·inline Workflow, 모델 목록·`count_tokens`·WebSearch 경계를 포함한다.
- 업데이트: 세 중단 상태에서 다음 실행의 복구와 실제 응답(#412 병합 전 검증과 같은 조건을 최종 후보에서 다시 확인한다). 새 설치, v0.6.18에서 업데이트·되돌리기, 설정과 PATH 보존. 원래 v0.6.18·v0.6.16·v0.6.10 updater가 v0.6.19로 갱신하는 전체 경로(`--dev --sync-settings`의 exit 0과 설정 보존, v0.6.10의 probe 보완 업데이트 포함).
- 진단: 정상 streaming 응답의 `firstTerminalType`. #417은 정상 스트리밍과 분류기 허용·차단 세 표본이다.
- 모델 목록 거부(#413): `--dev --verify-catalogue-refusal`의 401·403·503 세 행(거부 시 저장 목록 불변, 503의 마지막 정상 목록 사용, native·gateway 시작 0건).
- 시작 경로(#419): foreground print, foreground Agent View 시작, background 첫 응답과 중지 세 표본. background `stop`·`rm` 직후 재개.
- 공통: `go vet`, 전체 오프라인 Go 테스트, 동시성 코드가 바뀐 패키지의 race, 태그 fixture 두 단계, structural, native smoke 15행, 후보 자산의 재현 빌드, probe 출력 한도 검사.

결과, 예약 수, 호스트 조건(검사 시점의 CPU 고정 여부)은 발행 후 기록한다.

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 | 발행 후 기록 |
| `clauduct.exe` | 발행 후 기록 |
| `clauduct-probe.exe` | 발행 후 기록 |
| `install.ps1` | 발행 후 기록 |
| `uninstall.ps1` | 발행 후 기록 |
| `SHA256SUMS` | 발행 후 기록 |

이 태그까지 `clauduct-probe.exe`를 `clauduct.exe`와 한 쌍으로 출하한다. 진단 도구의 출하 방식은 다음 릴리스에서 바뀔 수 있다. [#420](https://github.com/wotjr1649/Clauduct/issues/420).

## 알려진 제한

- **파일 기반 Workflow(`named`·`scriptPath`)는 Read 권한을 검증한 native 이미지(2.1.325)에서만 받는다.** 다른 이미지는 `CLAUDUCT_WORKFLOW_SOURCE_IMAGE_UNVERIFIED`로 Read 전에 거부한다. native의 mod `$.tool.call` Read가 Read 거부 규칙을 지키지 않는 동작은 2.1.330·2.1.331·2.1.332에서 관측했고 upstream에 보고했다. 그 밖의 이미지에서는 관측하지 않았다. [anthropics/claude-code#110979의 관측](https://github.com/anthropics/claude-code/issues/110979#issuecomment-7162373953), [#403](https://github.com/wotjr1649/Clauduct/issues/403). inline `script`는 계속 지원한다. Claude Code 2.1.332를 쓰는 설치에서는 파일 소스가 거부된다.
- **Code Mode는 gateway 경로에서 제공하지 않는다.** native 2.1.331은 Amazon Bedrock·Google Cloud·Microsoft Foundry·Claude Platform on AWS의 터미널 세션에 Code Mode 지원을 더했다([Claude Code changelog](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md)). Clauduct가 쓰는 custom base URL 경로에서는 지원하지 않으며 이를 구현하지 않았다. 2026-11-26 기준 LLM gateway 문서에서 Code Mode 규약을 찾지 못했고, Code Mode 요청이 이 경로에 실제로 어떻게 나타나는지는 측정하지 않았다.
- **사용자 Claude Mods는 로드·실행된다. 다만 제한이 있다.** 확인은 native 2.1.288의 합성 backend 기준이다([v0.6.7](RELEASE-v0.6.7.md)). mod가 `$.tool.call`로 Agent·SendMessage·Workflow·Skill을 직접 부르면 `NATIVE_DIRECT_DELEGATION_UNSUPPORTED`로 거부한다. 내장 mod "You should know"는 first-party 세션이면서 telemetry가 켜진 경우에만 제공되므로 Clauduct 세션에서는 쓸 수 없다. mod의 Read가 Read 거부 규칙을 지키지 않는 native 동작은 위 Workflow 파일 소스 제한의 이유다.
- **설정 v2의 호환성 경계.** v1 파일을 자동으로 변환하지 않는다. v0.6.18로 되돌리면 v2 파일을 거부한다. 그보다 이전 버전으로 되돌리는 경로는 이번 릴리스에서 확인하지 않았다.
- **`--update` 복구는 하드 링크를 지원하는 설치 볼륨을 전제로 한다.** 하드 링크를 지원하지 않으면 교체 전에 멈추고 기존 쌍을 보존한 채 재설치를 안내한다. Windows는 삭제 대기 중인 파일과 실제 접근 거부에 같은 `ERROR_ACCESS_DENIED`를 반환할 수 있어([CreateFileW](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew)) 이 오류를 `UPDATE_BUSY`로 바꾸지 않는다. 설치 잠금이 닫히는 순간 겹친 병렬 시작은 `UPDATE_BUSY` 또는 복구 안내로 멈출 수 있으며, 어느 쪽도 불일치한 쌍을 시작하지 않는다.
- **복원한 background resident의 프로필 잠금 제한은 v0.6.19에도 남아 있다.** 수정은 이번 릴리스 범위 밖이다. [#351](https://github.com/wotjr1649/Clauduct/issues/351).
- **teammate에게 보낸 두 번째 메시지의 전달과 재개 뒤 이전 reasoning의 재실림은 진단으로 확인하지 못했다.** 진단 필드는 다음 릴리스 이후로 미뤘다. [#329](https://github.com/wotjr1649/Clauduct/issues/329).
- 공통 제한은 [알려진 제한 원장](COMPATIBILITY.md#알려진-제한-원장-v0618-)에 있다. auto 모드 분류 요청 형식의 제한은 [v0.6.18 문서](RELEASE-v0.6.18.md#알려진-제한)와 같다.
