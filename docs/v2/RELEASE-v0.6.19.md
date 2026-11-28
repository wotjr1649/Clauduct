# v0.6.19 — 설정 v2와 계정 우선순위 기반 모델 선택, 업데이트 중단 복구

**호환성이 바뀌는 릴리스다.** `~/.clauduct/settings.json`이 v1 형식이면 v0.6.19는 native를 시작하기 전에 거부한다. 파일을 v2로 고치거나 지워야 한다. 방법은 [업그레이드 안내](#v0618에서-올라올-때)에 있다. 새 세션의 시작 모델도 Clauduct 설정이 아니라 native의 선택과 계정의 우선순위 목록으로 정해진다. [#409](https://github.com/wotjr1649/Clauduct/issues/409), [PR #424](https://github.com/wotjr1649/Clauduct/pull/424).

## 바뀐 것

- **설정 파일은 v2 형식만 읽는다.** 파일이 없으면 `{"version":2}`와 같은 기본값으로 실행한다. 파일이 있으면 `version`이 `2`여야 한다. v1 파일, 모르는 키·값, 중복 키, `null`은 `CLAUDUCT_SETTINGS_INVALID`로 native 실행 전에 거부한다. v0.6.17·v0.6.18이 v1 파일의 빠진 키를 메모리로 채우던 동작은 없어졌다. 일반 실행, 설치, 업데이트는 이 파일을 만들거나 바꾸지 않는다. `clauduct --dev --init-settings`는 파일이 없을 때만 v2 최소 파일을 만든다. [설정 문서](SETTINGS.md).
- **새 세션의 시작 모델은 native의 선택을 먼저 따른다.** 순서는 native `--model`, `ANTHROPIC_MODEL`, 활성 native 설정의 `model`, `ANTHROPIC_DEFAULT_MODEL`이다. 이 선택이 모두 비었을 때만 신선한 계정 모델 목록에서 고른다. 후보는 표시 대상이고 이 빌드가 라우팅할 수 있으며 쓸 수 있는 기본 effort와 native 정책을 충족하는 모델이고, 그중 priority 숫자가 가장 낮은 모델이 단독으로 확인돼야 한다. 동점, 후보 순위 누락, 유효 후보 없음 또는 마지막 정상 캐시만 있으면 `ACCOUNT_PRIORITY_UNAVAILABLE`로 멈추고 `--model`을 안내한다. gateway가 첫 본 대화 요청의 모델·effort를 확정한 값과 대조한다. `clauduct --dev --doctor`는 저장 목록 기준의 미리보기를 보여 주고, 실제 실행값과 출처는 종료 상태 JSON의 `session.startupModel`·`startupEffort`와 `startupModelSource`·`startupEffortSource`에서 확인한다. Clauduct 설정의 `startup`·`modelMapping`·`agents`는 더는 읽지 않는다.
- **모르는 effort는 일찍 거부한다.** 새 세션을 시작할 때 적용되는 명시 effort가 `low`·`medium`·`high`·`xhigh`·`max` 밖이면 `UNSUPPORTED_MODEL_OR_EFFORT`로 거부한다. 비어 있지 않은 `CLAUDE_CODE_EFFORT_LEVEL`은 `--effort`보다 우선한다. 아는 값이라도 그 모델에서 지원하는지는 계정 목록으로 확인한다. 모르는 모델·effort를 기본값으로 바꿔 실행하지 않는다.
- **재개와 fork는 저장된 선택을 쓴다.** UUID를 명시한 재개·fork는 저장된 마지막 모델·effort를 기본으로 쓴다. 계정과 정책이 허용하는 `--model`·`--effort`를 직접 줄 수도 있다. foreground에서 `--continue`로 재개하려 하면 `SESSION_RESUME_UUID_REQUIRED`로 거부하고 `--resume <UUID>` 사용을 안내한다. background의 `--continue`는 `BACKGROUND_CONTINUE_UNVERIFIED`로 거부한다.
- **`--update`가 중간에 끊겨도 다음 실행이 복구한다.** 새 `clauduct.exe`와 `clauduct-probe.exe`를 교체하는 도중(교체 직전, exe만 교체된 직후, 둘 다 교체된 직후)에 프로세스가 끝나면 다음 실행이 업데이트 기록과 파일 해시를 대조한다. 교체 전 중단이면 기존 쌍을 유지하고, exe만 바뀌었으면 검증된 준비 probe로 새 쌍을 완성한다. 쌍이 이미 일치하면 남은 업데이트 기록과 준비 파일을 정리한다. 무결성을 확인하거나 복구할 수 없으면 `UPDATE_RECOVERY_REQUIRED`로 시작을 멈춘다. 이때는 실행 중인 Clauduct를 닫고 다시 시도하거나 같은 릴리스의 쌍을 다시 설치한다. 설치 잠금이 경합하면 `UPDATE_BUSY`이므로 잠시 뒤 다시 실행한다. 복구는 이번 업데이트의 백업으로 확인된 이전 probe 링크만 정리한다. 준비 디렉터리가 사라진 기록은 설치된 쌍이 기록과 내장 SHA-256에 일치할 때만 정리하고, 교체 뒤 남은 준비 디렉터리는 지우지 않고 경로를 알린다. 이전 exe 정리만 남은 시작이 잠금과 겹치면 정리를 건너뛰고 계속 시작한다. 병합 전에 최종 HEAD에서 NTFS 합성 설치본의 교체 경계 네 곳과 기존 probe가 없는 변형 한 곳의 강제 종료 복구, 손상된 준비 probe의 거부, 세 중단 상태 뒤 실제 backend 응답(HTTP 200, 상태마다 요청 예약 1건)을 확인했고, 같은 HEAD의 공개 CI(govulncheck·Staticcheck 포함)가 통과했다. 병합 commit은 [`a8c073b69`](https://github.com/wotjr1649/Clauduct/commit/a8c073b69353a805713bb6aa2c97fbba66bc85c7), [공개 CI 실행](https://github.com/wotjr1649/Clauduct/actions/runs/45728507210)이다. [#365](https://github.com/wotjr1649/Clauduct/issues/365), [PR #412](https://github.com/wotjr1649/Clauduct/pull/412), [업데이트 절차](PACKAGING.md#51-업데이트).
- **진단: 첫 terminal 이벤트.** 상태 JSON의 요청 기록 `streamEnd.firstTerminalType`에 검증을 통과한 첫 종료 이벤트의 종류(`response.completed`, `response.failed`, `response.incomplete`, `error`)가 남는다. 관측된 종료 이벤트가 없으면 이 필드를 생략하고, 본문·응답 ID·자유 형식 오류 문구는 기록하지 않으며 사용자 동작은 바뀌지 않는다. 병합 commit은 [`d5a981a08`](https://github.com/wotjr1649/Clauduct/commit/d5a981a083504dc372c7fd7a726ba58869d9d007), 같은 최종 HEAD의 공개 CI(go·httpguard-portable)는 [실행](https://github.com/wotjr1649/Clauduct/actions/runs/45928671981)이다. [#416](https://github.com/wotjr1649/Clauduct/issues/416), [PR #417](https://github.com/wotjr1649/Clauduct/pull/417).

## 개발자 변경

- **시작 관문 `AdmitLaunch`를 제거했다.** 제품 경로에서 호출하지 않던 코드와 `Options.AdmittedNativeSHA256`을 지웠다. foreground, background, native provider 조회는 기존 인자로 각 시작 함수를 직접 부른다. provider-control probe와 `doctor` 진단은 그대로다. 동작 변화는 없다. [#419](https://github.com/wotjr1649/Clauduct/issues/419), [PR #425](https://github.com/wotjr1649/Clauduct/pull/425).
- **계정 모델 목록 거부 검증 명령.** `clauduct --dev --verify-catalogue-refusal`은 계정 모델 목록 요청이 401·403·503을 받을 때의 시작 경계를 네트워크 없이 확인하는 검증 전용 명령이다. 인자를 받지 않고, 임시 폴더·메모리 안의 합성 자격 증명·프로세스 전용 TLS 수신기만 쓴다. `127.0.0.1` 밖의 연결은 거부하며 native·gateway를 시작하거나 실제 backend와 OS 자격 증명 저장소를 읽지 않는다. 401·403은 `ACCOUNT_MODEL_LIST_UNAVAILABLE`로 거부하고 저장 목록을 바꾸지 않으며, 503은 같은 계정의 마지막 정상 목록을 쓴다. 검증용 응답 캡처(`CLAUDUCT_VERIFICATION_HTTP_CAPTURE=1`)도 함께 들어간다. 인증된 native 연결의 응답만 예산 디렉터리에 기록하고 요청 header와 본문은 저장하지 않는다. 변수가 없으면 실행되지 않고, 예산 디렉터리(절대 경로)가 없으면 `VERIFICATION_HTTP_CAPTURE_REQUIRES_BUDGET`, native 신원 검사가 켜져 있지 않으면 `VERIFICATION_HTTP_CAPTURE_REQUIRES_NATIVE_IDENTITY`로 native를 시작하기 전에 거부하며, 캡처를 잃으면 실행이 실패한다. 합성 자격 증명 공급자는 `internal/app`이 아니라 `internal/devcmd`가 조립한다. 일반 사용에는 쓸 일이 없고 동작도 바뀌지 않는다. [PR #424](https://github.com/wotjr1649/Clauduct/pull/424).
- **죽은 코드 정리(45줄).** 참조가 없는 함수 두 개(`Gateway.CurrentSession`, `providercert.ImageOfProcess`)와 항상 `false`인 상수 `residentRecheckAcrossRuns` 및 그 분기 세 곳을 지웠다. 남는 경로는 지금 실행되는 경로와 같고 동작 변화는 없다. #366의 나머지 항목은 이번 릴리스에 없다. [#366](https://github.com/wotjr1649/Clauduct/issues/366), [PR #426](https://github.com/wotjr1649/Clauduct/pull/426).
- **공개 CI에 정적 분석을 더했다.** Windows 작업에서 staticcheck와 govulncheck를 실행한다. 기존 진단 25건은 소스 위치에 결속한 기준선으로 관리하고, 새 진단은 실패로 본다. 사용자 영향은 없다. [PR #411](https://github.com/wotjr1649/Clauduct/pull/411).

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

태그·commit, 후보 `clauduct.exe`와 `clauduct-probe.exe`, native 버전과 SHA-256에 결속한 `GATE-T1` 결과는 발행 뒤 이 문서에 기록한다. 마지막 유효 T1·T2의 native 버전과 전체 SHA-256이 이번 native와 같은지에 따라 `c1` 도구 표면 재측정 여부를 정하며, 그 결정과 근거도 함께 기록한다. c1에서 도구 이름이나 핵심 도구의 차이가 확인되면 해당 native 이미지의 T2-core PASS를 확인해 출하 전에 원장에 기록하고, 확인되지 않으면 그 사실과 이유를 `NOT_RUN`으로 적는다. 이번 출하의 필수 범위는 핵심 동작(모델 요청 중계, 계정 모델 선택, 서브에이전트·Workflow·재개·auto 권한·plugin/hook/MCP 공존, 압축, 자격증명·목적지 보호)이다. Agent View·`--bg`·resident·warm spare와 Agent teams는 best-effort 확장으로, 이번 후보에서 시험하지 않았고 그 결과는 출하를 막지 않는다. 이 기능들의 이전 측정은 [호환성 문서](COMPATIBILITY.md#확장-기능의-시험-상태)에서 현재 시험하지 않음 — 이전 실측 기록이다. 확장의 진단 단계를 실행하지 않은 경우 `NOT_RUN`과 이유를 이 절 끝에 적는다. 이번 후보가 확인하는 바뀐 동작은 다음과 같다.

- 설정: v2 최소 파일·파일 없음. 업데이트 뒤 v1 파일은 native 시작과 예약 0건으로 거부하고, 수동으로 v2로 옮긴 뒤에는 실행에 성공한다. 롤백 뒤에는 v2 파일을 거부하고 v1을 복원하면 성공한다. 새 설치는 설정 파일을 만들지 않고 `--dev --init-settings`가 명시적으로 만든다. v1·v2 파일의 바이트·mtime과 PATH는 설치·업데이트·롤백 전후에 보존한다.
- 모델 선택: 계정 priority 기본값, native 명시 선택, 명시 UUID 재개·fork의 pair 복원, 모르는 effort의 조기 거부, `--continue` 재개 거부. 같은 설정 표본에 분류기 4건(기본 조합·명시 override 각각의 허용과 차단), 압축 뒤 회복, 역할별 위임과 부모 인계(환경·설정 경로 2건), 같은 UUID의 inline Workflow 재개, 계정 모델 목록과 native picker, 허용된 `count_tokens` 한 건, 모델 목록 인증 거부(401·403은 시작 거부, 일시 장애는 마지막 정상 목록 유지)를 포함한다. foreground에서 모르는 effort를 native와 계정 조회 전에 거부하는지(요청 예약 0)도 확인한다. `count_tokens`의 시작 선택 불일치 거부와 foreground 목적지 차단은 실제 backend가 아니라 오프라인 Go 시험으로 확인한다. Agent View와 WebSearch 기능 표본은 이번에 실행하지 않았다. foreground 재개 흐름은 핵심 행(명시 재개·`--continue` 거부·복원)만 확인하며, 장기 print 단계(RP01)는 이 후보에서 다시 실행하지 않았다(프로젝트 결정).
- 업데이트: 세 중단 상태에서 다음 실행의 복구와 실제 응답(#412 병합 전 검증과 같은 조건을 최종 후보에서 다시 확인한다). 새 설치, v0.6.18에서 업데이트·되돌리기, 설정과 PATH 보존. 원래 v0.6.18·v0.6.16·v0.6.10 updater가 v0.6.19로 갱신하는 전체 경로(`--dev --sync-settings`의 exit 0과 설정 보존, v0.6.10의 probe 보완 업데이트 포함).
- 진단: 정상 streaming 응답의 `firstTerminalType`. #417은 정상 스트리밍과 분류기 허용·차단 세 표본이다.
- 모델 목록 거부(#413): `--dev --verify-catalogue-refusal`로 401·403의 시작 거부(저장 목록 불변)와 503의 마지막 정상 목록 사용을 확인하고(native·gateway 시작 0건), 실제 backend 표본은 401·403 시작 거부와 일시 장애 때 마지막 정상 목록 유지다.
- 시작 경로(#419): foreground print 시작의 실제 backend 표본과 영향 범위 native 시험. 호출이 없던 `AdmitLaunch`를 지운 변경이라 Agent View·background의 시작 경로는 이번에 시험하지 않았다. `--bg` 실제 backend 표본은 resident 프로세스 결속 단계에서 멈춰 진단으로만 기록했고(backend 호출 0), 같은 표본이 #419 이전 main에서도 같은 결과여서 이 변경의 회귀 근거는 없다. 원인은 후속 작업에서 진단한다. [PR #425](https://github.com/wotjr1649/Clauduct/pull/425).
- 공통: `go vet`, 전체 오프라인 Go 테스트, 동시성 코드가 바뀐 패키지의 race, 태그 fixture 두 단계(`fixtures-bg`는 보안·argv 시험만), structural, foreground native smoke 6행, 후보 자산의 재현 빌드, probe 출력 한도 검사.
- 확장 진단: `--bg` native smoke 5행과 `fixtures-bg`의 확장 가용성 시험은 확장 코드나 해당 native 표면이 바뀐 출하에서만 실행하며, 실행하지 않으면 `NOT_RUN`과 이유를 적는다. 이 단계의 결과는 출하 종합 판정에 넣지 않는다. 의도하지 않은 자격증명·목적지 송신이 관측되면 확장 행이어도 핵심 보안 실패로 기록한다.

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

- **세션 시작 뒤 저장된 Claude apps gateway 로그인은 다시 검사하지 않는다.** 기본 신규 세션은 이미 저장된 gateway 로그인이 있으면 `NATIVE_STORED_GATEWAY`로 모델 세션 시작을 거부한다. Clauduct가 시작한 뒤 해당 로그인이 새로 저장되고 그 gateway의 원격 관리 정책이 훅을 끄면, Claude Code가 Clauduct gateway를 우회해 저장된 로그인의 자격증명과 대화 내용을 해당 저장 gateway로 직접 보낼 수 있다. v0.6.19는 이 실행 중 송신을 막지 못한다. Clauduct 세션을 사용하는 동안 해당 gateway에 로그인하지 않는다. 로그인 상태가 바뀌었다면 프롬프트를 더 보내기 전에 Clauduct를 종료하고, 독립 실행한 Claude Code에서 해당 gateway 로그인을 해제한 뒤 Clauduct를 다시 시작한다. [지원 환경과 보장 한계](COMPATIBILITY.md#v0611--지원-환경과-보장-한계-319)
- **파일 기반 Workflow(`named`·`scriptPath`)는 Read 권한을 검증한 native 이미지(2.1.325)에서만 받는다.** 다른 이미지는 `CLAUDUCT_WORKFLOW_SOURCE_IMAGE_UNVERIFIED`로 Read 전에 거부한다. native의 mod `$.tool.call` Read가 Read 거부 규칙을 지키지 않는 동작은 2.1.330·2.1.331·2.1.332에서 관측했고 upstream에 보고했다. 그 밖의 이미지에서는 관측하지 않았다. [anthropics/claude-code#110979의 관측](https://github.com/anthropics/claude-code/issues/110979#issuecomment-7162373953), [#403](https://github.com/wotjr1649/Clauduct/issues/403). inline `script`는 계속 지원한다. Claude Code 2.1.332를 쓰는 설치에서는 파일 소스가 거부된다. 당분간 변경 계획은 없고 upstream 동작이 바뀌면 다시 검토한다.
- **Code Mode는 gateway 경로에서 제공하지 않는다.** native 2.1.331은 Amazon Bedrock·Google Cloud·Microsoft Foundry·Claude Platform on AWS의 터미널 세션에 Code Mode 지원을 더했다([Claude Code changelog](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md)). Clauduct가 쓰는 custom base URL 경로에서는 지원하지 않으며 이를 구현하지 않았다. 2026-11-26 기준 LLM gateway 문서에서 Code Mode 규약을 찾지 못했고, Code Mode 요청이 이 경로에 실제로 어떻게 나타나는지는 측정하지 않았다.
- **사용자 Claude Mods는 로드·실행된다. 다만 제한이 있다.** 확인은 native 2.1.288의 합성 backend 기준이다([v0.6.7](RELEASE-v0.6.7.md)). mod가 `$.tool.call`로 Agent·SendMessage·Workflow·Skill을 직접 부르면 `NATIVE_DIRECT_DELEGATION_UNSUPPORTED`로 거부한다. 내장 mod "You should know"는 first-party 세션이면서 telemetry가 켜진 경우에만 제공되므로 Clauduct 세션에서는 쓸 수 없다. mod의 Read가 Read 거부 규칙을 지키지 않는 native 동작은 위 Workflow 파일 소스 제한의 이유다.
- **설정 v2의 호환성 경계.** v1 파일을 자동으로 변환하지 않는다. v0.6.18로 되돌리면 v2 파일을 거부한다. 그보다 이전 버전으로 되돌리는 경로는 이번 릴리스에서 확인하지 않았다.
- **`--update` 복구는 하드 링크를 지원하는 설치 볼륨을 전제로 한다.** 하드 링크를 지원하지 않으면 교체 전에 멈추고 기존 쌍을 보존한 채 재설치를 안내한다. Windows는 삭제 대기 중인 파일과 실제 접근 거부에 같은 `ERROR_ACCESS_DENIED`를 반환할 수 있어([CreateFileW](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew)) 이 오류를 `UPDATE_BUSY`로 바꾸지 않는다. 설치 잠금이 닫히는 순간 겹친 병렬 시작은 `UPDATE_BUSY` 또는 복구 안내로 멈출 수 있으며, 어느 쪽도 불일치한 쌍을 시작하지 않는다.
- **복원한 background resident의 프로필 잠금 제한은 v0.6.19에도 남아 있다.** 수정은 보류했고 당분간 계획이 없다. 복원 resident와 `--bg` 관리는 best-effort 확장으로 이번 릴리스에서 시험하지 않았다. [#351](https://github.com/wotjr1649/Clauduct/issues/351).
- **Agent teams는 이번 릴리스에서 시험하지 않았다.** teammate에게 보낸 두 번째 메시지의 전달과 재개 뒤 이전 reasoning의 재실림은 진단하지 않았고, 이 작업은 보류했다. [#329](https://github.com/wotjr1649/Clauduct/issues/329).
- **확장 기능은 이번 릴리스에서 시험하지 않았다.** Agent View, `--bg`·resident·warm spare, Agent teams는 best-effort 확장이다. 이 기능의 이전 측정은 [호환성 문서](COMPATIBILITY.md#확장-기능의-시험-상태)에서 **현재 시험하지 않음 — 이전 실측 기록**이다. 현재 native와 이 빌드에서 다시 확인하지 않았다. 이 기능이 새 native에서 동작하지 않아도 출하를 막지 않는다. 자격증명 보호와 목적지 검사는 이 기능에도 같은 설계 불변식으로 적용한다.
- 공통 제한은 [알려진 제한 원장](COMPATIBILITY.md#알려진-제한-원장-v0618-)에 있다. auto 모드 분류 요청 형식의 제한은 [v0.6.18 문서](RELEASE-v0.6.18.md#알려진-제한)와 같다.
