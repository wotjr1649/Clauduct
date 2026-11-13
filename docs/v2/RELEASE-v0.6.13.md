# v0.6.13 — 견고성 수정과 소유자 결정

2026-11-13 출시(태그 `v0.6.13` → `725ac5c1945a17adf82a8df5885aebcae2ab76fd`, [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.13)). 범위는 [#378](https://github.com/wotjr1649/Clauduct/issues/378)과 관련 작업 [#365](https://github.com/wotjr1649/Clauduct/issues/365)(A–D), [#341](https://github.com/wotjr1649/Clauduct/issues/341), [#358](https://github.com/wotjr1649/Clauduct/issues/358), [#370](https://github.com/wotjr1649/Clauduct/issues/370), [#372](https://github.com/wotjr1649/Clauduct/issues/372), [#373](https://github.com/wotjr1649/Clauduct/issues/373), [#363](https://github.com/wotjr1649/Clauduct/issues/363), [#364](https://github.com/wotjr1649/Clauduct/issues/364), [#350](https://github.com/wotjr1649/Clauduct/issues/350), 분류기 공장값 결정 10, 종료 계약 결정 14와 [#332](https://github.com/wotjr1649/Clauduct/issues/332) 제한 정정이다.

판단 근거는 공개 [소유자 결정 기록](https://github.com/wotjr1649/Clauduct/issues/378#issuecomment-6760268582)과 [#332 후속 결정](https://github.com/wotjr1649/Clauduct/issues/378#issuecomment-6760521694)이다. 출하 판단과 소유자 예외는 [출하 결정 기록](https://github.com/wotjr1649/Clauduct/issues/378#issuecomment-6840040797)에 있다. 실제 backend 결과는 "출하 검사"의 "실제 backend(신규 영향)"에 모은다.

## 바뀐 것

- **streaming 응답의 tool과 완료 신호를 정상 종료 확인 뒤 전달한다.** 발행 기록과 부모 대기 결정도 본문 EOF와 parser 종료를 확인할 때까지 보류한다. 실패 응답의 사용량 기록과 이미 준비된 선택의 정리는 유지한다([#365 A](https://github.com/wotjr1649/Clauduct/issues/365), [ARCHITECTURE §8](ARCHITECTURE.md)).
- **resident 재생성 중 link 응답을 준비 완료까지 보류한다.** 이 경로는 `RestartHeld`를 사용하고 설정이 끝난 뒤 요청을 연다. 기존 `Restart`는 즉시 응답한다. 재생성 후 resume할 때 세션의 마지막 `/model` model·effort pair도 유지한다([#365 B](https://github.com/wotjr1649/Clauduct/issues/365), [#341](https://github.com/wotjr1649/Clauduct/issues/341)).
- **background 상태 파일 읽기 오류의 중지 기준을 완화한다.** 연속 세 번 읽기에 실패할 때 해당 link의 작업 중지를 시도한다([#365 C](https://github.com/wotjr1649/Clauduct/issues/365)).
- **self-update 파일 교체를 보강한다.** 새 파일을 임시 파일에 완전히 쓴 뒤 교체하고, 잠긴 백업은 보존하며 정리할 경로를 안내한다([#365 D](https://github.com/wotjr1649/Clauduct/issues/365)).
- **background stop 뒤 worker 종료를 확인한다.** ACK와 작업 상태를 실제 OS 종료와 구분한다. 등록된 worker는 정확한 PID와 생성 시각으로 최대 5초 확인하며, 그때까지 살아 있으면 PID를 알리고 추가 종료를 시도하지 않는다. worker 자손의 종료는 native 책임으로 둔다([#358](https://github.com/wotjr1649/Clauduct/issues/358), 결정 14, [ARCHITECTURE §11](ARCHITECTURE.md)).
- **native fast mode 확인 요청을 끈다.** native 세션의 환경에 `CLAUDE_CODE_DISABLE_FAST_MODE=1`을 넣는다. native가 시작할 때 `api.anthropic.com`에 보내던 fast mode 확인(`claude_code_penguin_mode`) 요청을 없애는 것이 목적이다. 무과금 측정(native 2.1.321, 합성 login, 차단 proxy)에서 이 값을 넣으면 그 요청이 0건이었고, 넣지 않은 대조는 1건이었다. 실제 backend 확인은 "출하 검사"에 있다([#372](https://github.com/wotjr1649/Clauduct/issues/372), 소유자 결정 3).
- **분류기 공장값을 `gpt-6.1-terra/low`로 바꾼다.** 위험 동작 허용 0, 판정 거부 0을 충족하지만 승인된 태그 push를 차단한 측정 결과를 소유자 결정 10으로 받아들였다. 이 공장 pair는 시작 안내 대신 `session.classifierFinding`에 남는다. 이미 있는 `settings.json`의 `classifier_model` 값은 바뀌지 않는다. 이전 pair(`gpt-5.6-terra/low`)로 쓰려면 `classifier_model`에 명시한다([#378](https://github.com/wotjr1649/Clauduct/issues/378), [SETTINGS](SETTINGS.md#보조-요청과-auto-권한-분류기-v064), [분류기 측정](COMPATIBILITY.md)).

  | pair | 결과 |
  |---|---|
  | `gpt-6.1-terra/low` | 위험 동작 허용 0·판정 거부 0. 정상 표본 하나(승인된 태그 push)를 차단. v0.6.13 공장값(결정 10) |
  | `gpt-6.1-terra/medium`·`high` | 13번째 요청의 보안 표본을 backend가 `cyber_policy`로 거부해 판정 미완료. 완료한 12개에서 위험 동작 허용 0 |

- **Go 툴체인 정책을 단일 출처에서 읽는다.** 루트 `go.mod`의 Go 1.27.4 값을 CI·빌드·검증에서 사용하며 정확한 패치 버전 검사는 유지한다. 하네스 Node 정책은 `.node-version`의 24로 정했고 Node 26은 보류했다. 툴체인 업그레이드 시 영향받는 표준 소스 hash 검토, 재현 빌드 digest, 새 검증이 필요하다([#370](https://github.com/wotjr1649/Clauduct/issues/370), [PACKAGING](PACKAGING.md)).

## 출하 검사

**전체 관문(A11): 이번 후보로 PASS 판정을 남기지 않았다(소유자 예외, [결정 기록](https://github.com/wotjr1649/Clauduct/issues/378#issuecomment-6840040797)).**

- 소유자가 리허설로 고른 A11 시도(`v0613-b14-a2`)가 이번 출하 후보와 바이트가 같은 후보로 모든 단계를 끝까지 실행했다.
  - vet, go, fixtures, fixtures-bg, structural, race, fg, background 네 world, av는 모두 통과했다.
  - 마지막 F123 모음에서는 한 행(F4 spare)만 gating 실패였다. 원인은 시험 하네스 결함이다. 하네스가 자기 정리 명령(`claude daemon stop --any`)을 제품 supervisor로 셌고, 제품 supervisor의 환경은 모든 항목이 비어 있었다.
  - 하네스를 고친 뒤 기록된 증거를 다시 판정하면 실패가 0건이다([기록](https://github.com/wotjr1649/Clauduct/issues/378#issuecomment-6823470023)).
  - 같은 모음의 W1S-F3-ctl-bg-idle-v2 실패는 기존 non-gating 후속 항목이다(소유자 결정 2026-11-03).
- 새 승인으로 다시 시작한 A11은 vet부터 structural까지 통과한 뒤 소유자 결정으로 중단했다.
  - 두 시도 사이에는 시험 하네스만 바뀌었고 제품은 바뀌지 않았기 때문이다.
  - 고친 F4 검사는 아직 실제 native에서 실행되지 않았다.
- 실제 backend 18묶음(Phase C)과 closure 판정은 실행하지 않았다. v0.6.12와 같은 소유자 예외다. 대신 바뀐 동작은 아래 "실제 backend(신규 영향)"로 직접 확인했다.

| 항목 | 확인 범위·기준 | 결과 |
|---|---|---|
| 실제 backend·closure | 바뀐 동작의 표본을 실제 backend로 확인한다. 18묶음과 closure는 실행하지 않는다(소유자 예외) | 아래 표. 18묶음·closure NOT_RUN |
| [#372](https://github.com/wotjr1649/Clauduct/issues/372) | 승인된 A군 7개 template을 경계 판정기에 반영했다. fast mode 확인을 껐고(위 "바뀐 것"), 남은 요청은 알려진 제한으로 기록한다(결정 1–3) | 리허설 A11의 모든 단계 통과. fast mode 요청 0건은 E1에서 확인 |
| [#373](https://github.com/wotjr1649/Clauduct/issues/373) | 판정 속도·행 분류·명시적 profile과 환경 복원을 보완한다. job 밖 프로세스는 ETW의 이미지·PID·생성 시각이 정확히 결속된 경우만 소유로 인정한다. A06은 실제 native 상태 파일 형태에 맞추되 규칙을 약화하지 않는다. F4 spare는 하네스 자신의 정리 명령을 빼고 제품 supervisor만 판정한다. F1 ctl-bg-idle-v2는 판정식을 수정한다(결정 4–6) | 리허설 A11에서 F4 spare만 하네스 결함으로 실패, 수정 뒤 재판정 0건. 고친 F4의 native 실행은 없음 |
| [#363](https://github.com/wotjr1649/Clauduct/issues/363) | AST 기반 변이 위치 선택과 실행 전 일치 수 검사, worker별 자원 격리 및 직렬 결과 대조 | 전체 self-test(변이 포함) PASS, 의미 변이 142/142 KILLED |
| [#364](https://github.com/wotjr1649/Clauduct/issues/364) | fail-fast 보완. 관문 단계 분리와 호스트 변경은 v0.6.13에서 하지 않는다(결정 13) | 리허설 A11이 첫 gating 실패에서 멈추고 원인을 기록함 |
| [#350](https://github.com/wotjr1649/Clauduct/issues/350) | console encoding과 무관하게 승인 원문을 UTF-8로 hash하며 기존 승인 tuple·본문 변경 거부를 유지한다. UTF-8/cp949·Unicode·줄바꿈을 비교한다 | A11 실행기의 진입 검사 PASS(두 승인 모두) |

### 실제 backend(신규 영향)

실행 조건:
- 후보는 공개 commit `fd22484d9`이고 SHA-256은 `174150b2efb3e1972d97ecf641f7a95db2453afc10debdabe2426aee0cc2cad4`이다.
- native 2.1.321을 고정했다. 실행마다 설치본 native를 sha256으로 대조한다.
- 실제 backend 사용은 소유자가 상시 승인했다.
- 상태는 PASS·FAIL·INCONCLUSIVE·NOT_RUN만 쓰고, INCONCLUSIVE와 NOT_RUN을 PASS로 적지 않는다.

| 묶음 | 확인한 동작 | 판정 기준 | 결과 |
|---|---|---|---|
| 18묶음(Phase C) | 바뀐 동작 전체의 회귀 | 18개 계약 모두 PASS | NOT_RUN(소유자 예외) |
| E1+C1 | fast mode 끔(E)과 분류기 안내 미반복(C). print·TUI·`--bg` 각 1회 | 측정 구간의 `claude_code_penguin_mode` 요청이 0건이어야 한다. 이때 해독된 api.anthropic.com 요청이 1건 이상이고 해독 불가 행은 0이어야 한다. 후보 stderr(TUI는 화면)의 `clauduct: classifier_model` 줄도 0건이어야 한다. 정상 종료·관측 완전·정리 완료·native 불변이어야 한다 | **PASS**(3/3). penguin 0·0·0, 안내 줄 0·0·0, 해독된 요청 3·3·11, 해독 불가 0 |
| D1 | `/model`로 바꾼 pair가 resident 재생성 뒤에도 유지(#341) | 첫 resident의 요청에서 고른 pair를 얻는다. 재생성한 resident의 main 요청이 모두 그 pair(모델·effort)와 같아야 한다. 재생성 resident의 생성 예약이 1건 이상이고, 종료 뒤 시작 pair(sol/low) 예약은 0이어야 한다. 응답이 있고 정리가 확인돼야 한다 | **PASS**(모델 유지로 한정). 고른 pair `gpt-6-luna/low`, 재생성 resident의 main 요청 모두 같은 pair, 종료 뒤 sol 예약 0, 재생성 resident 생성 예약 있음, 응답·정리 확인. native가 `/model` 뒤에도 effort를 low로 유지해 effort 유지는 구분하지 못했다 |

### 출하 빌드·발행 검사

[PACKAGING](PACKAGING.md#41-릴리스-만들기)의 절차를 따랐다.

- `build-release`: 두 번의 독립 캐시 빌드 바이트가 같고, probe digest가 `go/cmd/clauduct/probe.sha256`과 일치한다.
- 발행 전 `probe limit --send`(3회): `max_output_tokens` 상한을 유지한다(HONOURED).
- `ship-install`: 새 설치, v0.6.12에서 업데이트, 되돌리기, 설정 보존, 사용자 PATH 불변을 확인했다.
- `ship-session`(5회): 위임, `/clear`, 두 번째 turn, 부모 luna/max·자식 luna/low route를 확인했다.
- 무과금 표면 관문(`surface`): 모든 차이가 분류됐다. 첫 실행은 native 단독 기준 실행의 요청 본문을 비밀값 패턴으로 오인해 거부했고, 다시 실행해 통과했다.
- TUI 요약 회귀(`tui-summary`)는 v0.6.11·v0.6.12와 같은 구형 하네스 결함 때문에 실행하지 않았다.
- 발행 뒤 `verify-release`(내려받은 바이트·API digest·`SHA256SUMS`), `ship-update`(이전 릴리스의 `--update`와 `already current`), `ship-legacy`(v0.3.5)를 모두 통과했다.

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 | `v0.6.13` → `725ac5c1945a17adf82a8df5885aebcae2ab76fd` |
| `clauduct.exe` | `58ef34a54e22a7f5e8aac3d332cb46e88519845af2454b6bcad26b88fa275368` |
| `clauduct-probe.exe` | `4d299e56192c903917bd541ba96b4f54973ce775b6f48ad2f605761d7fc33ecc` |
| `install.ps1` | `6720976f89e9016a33bd046f02bcf36e1acfdb411097b93c1d8a443f55356c58` |
| `uninstall.ps1` | `a88d8f56719ee7ca7a199fd30214d8b74171a08c34c2d952cd60990b4d1ba295` |
| `SHA256SUMS` | `c4514062b8970de685ad9c1e42f5a60dea56b3180fe22d0f624760521d957884` |

Go 1.27.4, `CGO_ENABLED=0`, `-trimpath`. native를 쓴 검사(실제 backend 표본, 출하 검사)는 모두 Claude Code 2.1.321에서 돌았다. 실제 backend 표본의 후보(SHA-256 `174150b2…`, commit `fd22484d9`)는 같은 소스를 개발 신원으로 빌드한 것이라 출하 `clauduct.exe`와 digest가 다르다.

## 알려진 제한

- **이번 후보로 전체 관문(A11) PASS 판정을 남기지 않았다.** 같은 후보의 리허설 A11은 모든 단계를 끝까지 실행했고, 유일한 gating 실패는 하네스 결함이었다(수정 뒤 재판정 0건). 다시 시작한 A11은 소유자 결정으로 structural 단계 뒤 중단했다. 고친 F4 검사는 실제 native에서 실행되지 않았다([출하 결정 기록](https://github.com/wotjr1649/Clauduct/issues/378#issuecomment-6840040797)).
- **실제 backend 18묶음(Phase C)을 실행하지 않았다.** 바뀐 동작은 위 표의 표본(E1+C1, D1)으로 실제 backend에서 확인했지만, 바뀌지 않은 동작의 실제 backend 회귀는 v0.6.11 이후 다시 확인하지 않았다(소유자 예외, v0.6.12와 같다).
- **`--bg` 작업에서는 native의 telemetry와 계정 조회가 남는다.** background의 공유 supervisor와 dispatch 프로세스가 Anthropic으로 telemetry(`event_logging`)와 feature flag 조회(`eval`)를 보낸다. 저장된 Claude login이 있으면 이 요청에 그 login이 `Authorization`으로 실리고, native가 시작할 때 계정 조회(`bootstrap`, `oauth/profile`)도 한 번씩 보낸다. foreground(print·TUI)의 telemetry·eval과 fast mode 확인은 측정(native 2.1.321, 합성 login, 요청을 전달하지 않는 proxy)에서 0건이다. supervisor는 사용자의 다른 Claude Code 세션과 공유되어 세션 환경으로 끌 수 없다. 별도 설정 폴더를 쓰는 opt-in 격리 모드는 v0.6.14 후보다(소유자 결정 2026-11-10, [#372](https://github.com/wotjr1649/Clauduct/issues/372), [#379](https://github.com/wotjr1649/Clauduct/issues/379)). v0.6.11에서 적은 발신 계보 제한은 [RELEASE-v0.6.11](RELEASE-v0.6.11.md)에 그대로 있다([COMPATIBILITY](COMPATIBILITY.md)).
- **복원한 resident가 Clauduct job 종료와 함께 끝날 수 있다.** hook 트리 강제 종료는 relay가 먼저 끝나는 경로에서 견디지만, Clauduct가 띄운 native 아래에서 복원된 resident는 해당 job이 닫힐 때 함께 끝난다. 다음 hook이 다시 복원한다. 소유자 결정에 따라 해결은 v0.6.14의 job 밖 link 전용 복원 담당 작업으로 미뤘다([#332](https://github.com/wotjr1649/Clauduct/issues/332), [COMPATIBILITY](COMPATIBILITY.md)).
- worker 자손 종료는 native 책임으로 남는다. Clauduct는 보고만 하고 종료하지 않는다([#358](https://github.com/wotjr1649/Clauduct/issues/358), [ARCHITECTURE](ARCHITECTURE.md)).
- **분류기 공장 pair `gpt-6.1-terra/low`는 정상 동작을 오차단할 수 있다**(소유자 결정 10). 같은 핵심 표본(74개, 위험 32, [RELEASE-v0.6.12](RELEASE-v0.6.12.md)의 측정표)에서 위험 동작 허용 0, 판정 거부 0이었지만 정상 표본 하나(승인된 태그 push)를 차단했다. 공장 pair는 시작 안내를 내지 않고 `session.classifierFinding`에만 남는다. 이미 있는 `settings.json`의 `classifier_model`은 바뀌지 않으며, 같은 값을 명시하면 이 한계 없이 이전 pair로 돌릴 수 있다([SETTINGS](SETTINGS.md)). `medium`·`high`는 보안 표본 하나를 backend가 `cyber_policy`로 거부해 판정이 끝나지 않았다. 위 "바뀐 것"의 표 참고.
- **background stop 뒤 남은 worker 알림은 실제 backend로 확인하지 못했다.** stop이 ACK한 뒤에도 등록된 worker가 5초 안에 끝나지 않으면 PID를 알리는 동작([#358](https://github.com/wotjr1649/Clauduct/issues/358), 결정 14)은 단위 시험과 실제 HTTP 경로를 쓰는 오프라인 회귀로 확인했다. 정상 stop 경로에서는 worker가 끝나므로 알림이 나오지 않는다. 이 알림이 실제 native·backend에서 나오는 양성 사례는 만들지 못했다.
- **완료 응답 보류의 비정상 종료 분기는 실제 backend로 확인하지 못했다.** streaming 응답의 tool 호출·완료 신호를 본문 정상 종료 확인 뒤에 전달하는 동작([#365 A](https://github.com/wotjr1649/Clauduct/issues/365))에서, 본문이 비정상으로 끊기는 분기는 실제 backend로 만들 수 없었다. 이 분기는 오프라인 회귀로 확인했고, 정상 경로는 리허설 A11의 오프라인 회귀로 확인했고, 실제 backend 18묶음은 실행하지 않았다. v0.6.11의 같은 주제 제한(응답 본문이 정상 종료됐는지 확인하기 전에 도구 호출이 전달될 수 있음)은 정상 경로에서 이 변경으로 해소되지만, 비정상 분기의 실제 backend 확인은 없다.
- 상태 파일 읽기 3회 연속 실패에서만 중지하는 동작([#365 C](https://github.com/wotjr1649/Clauduct/issues/365))과 update의 `writeSynced`([#365 D](https://github.com/wotjr1649/Clauduct/issues/365))는 실제 backend로 재현하기 어렵다. 단위 시험과 출하 검사의 update 검사로 확인한다.
