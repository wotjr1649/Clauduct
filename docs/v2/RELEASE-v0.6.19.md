# v0.6.19 — 설정 v2와 계정 우선순위 기반 모델 선택, 업데이트 중단 복구

**호환성이 바뀌는 릴리스다.** `~/.clauduct/settings.json`이 v1 형식이면 v0.6.19는 native를 시작하기 전에 거부한다. 파일을 v2로 고치거나 지워야 한다. 방법은 [업그레이드 안내](#v0618에서-올라올-때)에 있다. 새 세션의 시작 모델도 Clauduct 설정이 아니라 native의 선택과 계정의 우선순위 목록으로 정해진다. [#409](https://github.com/wotjr1649/Clauduct/issues/409), [PR #413](https://github.com/wotjr1649/Clauduct/pull/413).

## 바뀐 것

- **설정 파일은 v2 형식만 읽는다.** 파일이 없으면 `{"version":2}`와 같은 기본값으로 실행한다. 파일이 있으면 `version`이 `2`여야 한다. v1 파일, 모르는 키·값, 중복 키, `null`은 `CLAUDUCT_SETTINGS_INVALID`로 native 실행 전에 거부한다. v0.6.17·v0.6.18이 v1 파일의 빠진 키를 메모리로 채우던 동작은 없어졌다. 일반 실행, 설치, 업데이트는 이 파일을 만들거나 바꾸지 않는다. `clauduct --dev --init-settings`는 파일이 없을 때만 v2 최소 파일을 만든다. [설정 문서](SETTINGS.md).
- **새 세션의 시작 모델은 native의 선택을 먼저 따른다.** 순서는 native `--model`, `ANTHROPIC_MODEL`, 활성 native 설정의 `model`, `ANTHROPIC_DEFAULT_MODEL`이다. 이 선택이 모두 비었을 때만 계정의 최신 모델 목록에서 priority가 **단독 1위**인 표시 모델을 쓴다. 동점이거나 순위가 없거나 마지막 정상 캐시뿐이면 `ACCOUNT_PRIORITY_UNAVAILABLE`로 멈추고 `--model`로 직접 고르라고 안내한다. gateway가 첫 본 요청의 모델·effort를 확정한 값과 대조하고, `clauduct --dev --doctor`가 선택 출처를 보여 준다. Clauduct 설정의 `startup`·`modelMapping`·`agents`는 더는 읽지 않는다.
- **모르는 effort는 일찍 거부한다.** `low`·`medium`·`high`·`xhigh`·`max` 밖의 `--effort`나 `CLAUDE_CODE_EFFORT_LEVEL`은 계정 목록 조회와 native 인증 확인보다 먼저 `UNSUPPORTED_MODEL_OR_EFFORT`로 거부한다. 어휘 안의 값이라도 계정이 그 모델에서 지원하지 않으면 전처럼 목록과 대조해 거부한다. 모르는 모델·effort를 기본값으로 바꿔 실행하지 않는다.
- **재개와 fork는 저장된 선택을 쓴다.** 명시한 세션 UUID를 `--resume`하면 저장된 모델·effort를 복원한다. `--resume <uuid> --continue`나 지원 범위 밖의 `--continue`는 native 시작 전에 `SESSION_RESUME_UUID_REQUIRED`로 거부한다.
- **`--update`가 중간에 끊겨도 다음 실행이 복구한다.** 새 `clauduct.exe`와 `clauduct-probe.exe`를 교체하는 도중(교체 직전, exe만 교체된 직후, 둘 다 교체된 직후)에 프로세스가 끝나도 다음 실행이 쌍을 완성하거나 되돌린다. 복구할 수 없으면 `UPDATE_RECOVERY_REQUIRED`와 재설치 안내를 출력한다. 다른 실행이 정리 중이면 `UPDATE_BUSY`다. 사용자가 둔 이전 링크와 이미 일치하는 probe는 보존한다. [#365](https://github.com/wotjr1649/Clauduct/issues/365), [PR #412](https://github.com/wotjr1649/Clauduct/pull/412), [업데이트 절차](PACKAGING.md#51-업데이트).
- **진단: 첫 terminal 이벤트.** 상태 JSON의 요청 기록 `streamEnd.firstTerminalType`에 응답이 처음 보낸 종료 이벤트 종류(`response.completed`, `response.failed`, `response.incomplete`, `error`)가 남는다. 본문, 응답 ID, 자유 형식 오류 문구는 기록하지 않는다. 사용자 동작은 바뀌지 않는다. [#416](https://github.com/wotjr1649/Clauduct/issues/416), [PR #417](https://github.com/wotjr1649/Clauduct/pull/417).

## 개발자 변경

- **시작 관문 `AdmitLaunch`를 제거했다.** 제품 경로에서 호출하지 않던 코드와 `Options.AdmittedNativeSHA256`을 지웠다. foreground, background, native provider 조회는 기존 인자로 각 시작 함수를 직접 부른다. provider-control probe와 `doctor` 진단은 그대로다. 동작 변화는 없다. [#419](https://github.com/wotjr1649/Clauduct/issues/419).
- **죽은 코드 정리(`Gateway.CurrentSession` 등 45줄).** 호출·참조가 없음을 확인한 뒤 지웠다. 동작 변화는 없다. [#366](https://github.com/wotjr1649/Clauduct/issues/366).
- **공개 CI에 정적 분석을 더했다.** Windows 작업에서 staticcheck와 govulncheck를 실행한다. 기존 진단 25건은 소스 위치에 결속한 기준선으로 관리하고, 새 진단은 실패로 본다. 사용자 영향은 없다. [PR #411](https://github.com/wotjr1649/Clauduct/pull/411).
- **비공개 시험의 시간 결정성.** `resident` 종료 대기 시험을 가상 시간으로 바꾸고, 종료가 제한 시간을 넘기면 검증 행이 실패하게 했다. 공개 코드는 바뀌지 않는다. [#382](https://github.com/wotjr1649/Clauduct/issues/382).

## v0.6.18에서 올라올 때

1. **업데이트 전에** `~/.clauduct/settings.json`이 있으면 복사본을 둔다. 업데이터와 설치 스크립트는 이 파일을 만들거나 바꾸지 않는다.
2. `clauduct --update`(무인 실행은 `clauduct --update --yes`)로 v0.6.19를 받는다.
3. **첫 실행이 `CLAUDUCT_SETTINGS_INVALID`로 멈추면 v1 파일이다.** 다음 중 하나를 한다.
   - 기본값으로 충분하면 파일 내용을 `{"version":2}` 한 줄로 바꾼다.
   - 바꿔 둔 값이 있으면 아래 대응표대로 옮긴다. 모르는 키가 하나라도 있으면 거부되므로 v1 전용 키는 지운다.
   - 파일을 지워도 된다. 없으면 최소 v2로 실행한다. 다시 만들려면 `clauduct --dev --init-settings`.
4. 새 세션이 다른 모델로 시작할 수 있다. 원하는 모델은 `clauduct --model <전체 모델 ID>`나 native 설정의 `model`로 고정한다. 어느 선택이 쓰였는지는 `clauduct --dev --doctor`가 보여 준다.

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

**v0.6.18로 되돌릴 때**는 v0.6.18이 v2 파일을 읽지 못한다. `scripts\install.ps1 -Tag v0.6.18`([PACKAGING](PACKAGING.md))로 설치한 뒤 백업한 v1 파일을 복원한다.

## 출하 검사

태그·commit, 후보 `clauduct.exe`와 `clauduct-probe.exe`, native 버전과 SHA-256에 결속한 `GATE-T1` 결과는 발행 뒤 이 문서에 기록한다. 같은 native 이미지에서 직전 유효 T1을 통과했는지에 따라 `c1` 도구 표면 재측정 여부를 정하며, 그 결정과 근거도 함께 기록한다. 이번 후보가 확인하는 바뀐 동작은 다음과 같다.

- 설정: v2 최소 파일·파일 없음·v1 거부(native 시작과 예약 0건)·업데이트 뒤 v1 파일의 바이트와 mtime 보존.
- 모델 선택: 계정 priority 기본값, native 명시 선택, 명시 UUID 재개·fork의 pair 복원, 모르는 effort의 조기 거부, `--resume <uuid> --continue` 거부.
- 업데이트: 세 중단 상태에서 다음 실행의 복구와 실제 응답, 새 설치·v0.6.18에서 업데이트·되돌리기·설정과 PATH 보존.
- 진단: 정상 streaming 응답의 `firstTerminalType`.
- 시작 경로: foreground print와 `--bg`, background `stop`·`rm` 직후 재개.
- 공통: `go vet`, 전체 오프라인 Go 테스트, 동시성 코드가 바뀐 패키지의 race, 태그 fixture 두 단계, structural, native smoke 15행.

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

- **파일 기반 Workflow(`named`·`scriptPath`)는 Read 권한을 검증한 native 이미지(2.1.325)에서만 받는다.** 2.1.330·2.1.331·2.1.332를 포함한 다른 이미지는 `CLAUDUCT_WORKFLOW_SOURCE_IMAGE_UNVERIFIED`로 Read 전에 거부한다. 원인은 native의 mod `$.tool.call` Read가 Read 거부 규칙을 지키지 않는 동작이며 upstream에 보고했다. [anthropics/claude-code#110979](https://github.com/anthropics/claude-code/issues/110979), [#403](https://github.com/wotjr1649/Clauduct/issues/403). inline `script`는 계속 지원한다.
- **설정 v2의 호환성 경계.** v1 파일을 자동으로 변환하지 않는다. v0.6.18 이하로 되돌리면 v2 파일을 거부한다.
- **`--update` 복구는 NTFS 하드 링크가 되는 설치 드라이브를 전제로 한다.** 하드 링크를 지원하지 않으면 교체 전에 멈추고 기존 쌍을 보존한 채 재설치를 안내한다.
- **복원한 background resident의 profile 잠금 문제는 이번 릴리스에 없다.** [#351](https://github.com/wotjr1649/Clauduct/issues/351).
- **teammate에게 보낸 두 번째 메시지의 전달과 재개 뒤 이전 reasoning의 재실림은 진단으로 확인하지 못했다.** 진단 필드는 다음 릴리스 이후로 미뤘다. [#329](https://github.com/wotjr1649/Clauduct/issues/329).
- 공통 제한은 [알려진 제한 원장](COMPATIBILITY.md#알려진-제한-원장-v0618)에 있다. auto 모드 분류 요청 형식의 제한은 [v0.6.18 문서](RELEASE-v0.6.18.md#알려진-제한)와 같다.
