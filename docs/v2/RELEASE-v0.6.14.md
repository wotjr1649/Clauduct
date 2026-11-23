# v0.6.14 — 도구 호출 호환성과 요청 대기 보완

2026-11-24 출시(태그 `v0.6.14` → `6782e6affa130a800cd0e7ac9b08df362c29e51d`, [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.14), 출하 기록 [#389](https://github.com/wotjr1649/Clauduct/issues/389)). 제품 소스는 출하 후보 `a2be1014d3d47bb67bed5a16db77136cf07fe276`과 같고 태그 commit은 이 문서만 더했다. v0.6.13 이후 도구 인자 수신, 라우팅 증거 대기, 특정 native 빌드의 인자 판정을 손봤다. 2026-11-23 소유자 결정에 따라 이번 출하는 소유자 예외 경로를 따른다.

## 바뀐 것

- **backend가 도구 인자를 한 번에 보내도 도구 호출을 처리한다.** 해당 호출에 delta가 없으면 `response.function_call_arguments.done`을 전체 인자로 받아들인다. 시작 이벤트의 인자 접두부, delta가 있을 때의 누적 인자, `response.output_item.done`의 최종 인자 일치 검사는 유지한다. 인자가 실제로 어긋나면 `ARGUMENTS_MISMATCH`로 거부한다. [#386](https://github.com/wotjr1649/Clauduct/issues/386), `go/internal/protocol/bridge/bridge.go:890`.
- **라우팅 증거가 들어오면 대기 중인 요청을 깨운다.** gateway는 20ms마다 확인하던 대신 모델 목록 요청의 증거를 기록하거나 resident의 증거를 복원할 때 대기 중인 요청에 알린다. 최대 5초의 상한과 끝에서의 증거 확인은 유지한다. 위임·팀원·분기 metadata와 팀원 receipt는 요청이 취소되지 않았으면 대기 만료 시점에 한 번 더 읽는다. 증거가 여전히 없거나 유효하지 않으면 기존 기준으로 거부한다. [#383](https://github.com/wotjr1649/Clauduct/issues/383), `go/internal/gateway/gateway.go:445`, `go/internal/gateway/gateway.go:763`, `go/internal/gateway/native_identity.go:91`, `go/internal/gateway/delegation.go:614`, `go/internal/gateway/native_events.go:94`.
- **native 2.1.325의 특정 빌드에서만 background와 관리 명령의 인자 판정을 맞춘다.** 실행 파일의 SHA-256이 내장 표와 일치하면 `--name --bg`와 `-- --bg`는 foreground로 처리한다. `clauduct --verbose rm <id>`처럼 허용된 옵션과 8자리 16진수 ID를 갖춘 관리 명령, 그리고 선두 `kill`은 native로 바로 보낸다. **이번 출하 기준인 2.1.330에서는 SHA-256이 달라 두 변경이 꺼진다.** 이 버전에서는 v0.6.13과 같이 argv 어디에 있든 정확한 `--bg`·`--background` 토큰을 background로 판정하고, 선두 `rm`·`stop`·`logs`만 관리 경로 후보로 삼는다. [#331](https://github.com/wotjr1649/Clauduct/issues/331), [ARCHITECTURE §4](ARCHITECTURE.md#4-cli-계약), `go/internal/launch/native_dispatch_tables.go:19`, `go/internal/app/background.go:44`, `go/internal/app/native_args.go:101`.
- **출하 빌드의 Go 정책 읽기를 명시했다.** 루트 `go.mod`의 `toolchain` 줄을 우선하고, 없으면 `go` 줄을 읽는다. 패치 버전까지 정확히 대조하며 이번 후보는 Go 1.27.4로 빌드했다. [#380](https://github.com/wotjr1649/Clauduct/issues/380), [PACKAGING](PACKAGING.md#툴체인-버전-370), `docs/v2/PACKAGING.md:88`.

## 출하 검사

**GATE-FULL, A11, 실제 backend 18묶음(Phase C) 모두 NOT_RUN.** 2026-11-23 소유자가 최소 검증 경로를 선택했다. 예외는 이 태그와 후보에 대한 출하 결정이다. heavy·kit·HARNESS·anchor 및 baseline 갱신도 NOT_RUN이다.

### 실제 backend(신규 영향)

| native | 확인 범위 | 결과 |
|---|---|---|
| 2.1.330 | `p383` foreground print, `a383` 병렬 하위 에이전트, `d1` resident 재생성 후 모델·effort 선택 유지 | **3종 PASS**(2026-11-24, 후보 `a2be1014d` 빌드). 각 표본 1회 실행, 예약 합계 14건. |
| 2.1.325 | 통합 소스 `71c52480d`에서 위 세 표본과 #331의 `s331` 관리·background 판정 표본 | **4종 PASS**. `p383`·`a383`·`d1`은 첫 실행, `s331`은 관리 단계 예약의 PID 귀속 판정기를 고친 뒤 같은 후보로 재실행해 통과했다. |

2.1.325의 `a383`에서 병렬 하위 에이전트 3개와 부모 응답이 완료됐다. `d1`에서는 resident 재생성 후 선택한 모델·effort로 응답을 받았다. `71c52480d`에서 이번 후보까지의 제품 소스 차이는 진단용 probe SHA-256 값 한 줄이다.

### 표면 재측정과 발행 검사

- native 2.1.330 무과금 도구 표면·core 도구 재측정(`c1`) — 수집 완료, native 연속성 PASS. 도구 이름·core 도구 목록·slash command·skill·agent 목록은 2.1.325와 같다. 도구 설명과 선택 입력(`ListAgents`의 `cursor`, `ReportFindings`의 `severity`), system prompt 문구, `--help`, plugin API에 차이가 있다. 독립 검토에서 제품 변경이 필요한 차이는 없다고 판정했다. `Workflow`의 `resumeFromRunId`로 명시 재실행할 때의 캐시 동작은 확인하지 않았다.

### 출하 빌드·발행 검사

[PACKAGING](PACKAGING.md#41-릴리스-만들기)의 절차를 따랐다. native를 쓴 검사는 모두 Claude Code 2.1.330에서 돌았다.

- `build-release`: 두 번의 독립 캐시 빌드 바이트가 같고, probe digest가 `go/cmd/clauduct/probe.sha256`과 일치한다.
- 발행 전 `probe limit --send`(3회): `max_output_tokens` 상한을 유지한다(HONOURED).
- `ship-install`: 새 설치, v0.6.13에서 업데이트, 되돌리기, 설정 보존, 사용자 PATH 불변을 확인했다.
- `ship-session`, TUI 요약 회귀(`tui-summary`), 무과금 표면 관문(`surface`), 실제 backend 18개는 NOT_RUN(소유자 최소 경로). 표면은 위 c1 재측정으로 대신했다.
- 발행 뒤 `verify-release`(내려받은 바이트·API digest·`SHA256SUMS`), `ship-update`(이전 릴리스의 `--update`와 `already current`), `ship-legacy`(v0.3.5)를 모두 통과했다.

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 | `v0.6.14` → `6782e6affa130a800cd0e7ac9b08df362c29e51d` |
| `clauduct.exe` | `10727985122742a22563af1bd23f23a1661971538f91211105d0fff6b955aac3` |
| `clauduct-probe.exe` | `9f5a324b0e1e08e3c5689eb4b19b644883416c8d0371a71fa3e32aabf2a4e687` |
| `install.ps1` | `6720976f89e9016a33bd046f02bcf36e1acfdb411097b93c1d8a443f55356c58` |
| `uninstall.ps1` | `a88d8f56719ee7ca7a199fd30214d8b74171a08c34c2d952cd60990b4d1ba295` |
| `SHA256SUMS` | `05e81b32b1c48682d7af261e86b2c16666b0b4f0897cd51a43234899acb48bfe` |

Go 1.27.4, `CGO_ENABLED=0`, `-trimpath`. 실제 backend 표본의 후보(SHA-256 `68e5e886…`, commit `a2be1014d`)는 같은 제품 소스를 개발 신원으로 빌드한 것이라 출하 `clauduct.exe`와 digest가 다르다.

## 알려진 제한

- **native 2.1.330에서 #331 재현 형태는 기존 경로를 탄다.** 옵션 값이나 `--` 뒤의 `--bg` 때문에 Clauduct와 native의 판단이 갈리면 `BACKGROUND_DISPATCH_UNVERIFIED`가 날 수 있다. 선행 옵션 뒤 관리 명령은 세션 준비가 실패하면 native에 도달하지 못할 수 있다. `s331`은 2.1.325 고정 빌드의 동작을 확인한 표본이다. [#331](https://github.com/wotjr1649/Clauduct/issues/331), [COMPATIBILITY](COMPATIBILITY.md), `docs/v2/COMPATIBILITY.md:1310`.
- **위 표본의 확인 범위는 해당 경로에 한정된다.** 2.1.325의 표본은 부하별 기한 경합이나 라우팅 대기 알림을 재현하지 않았다. `d1`에서 확인한 effort는 시작값과 같았다. [#383](https://github.com/wotjr1649/Clauduct/issues/383).
- background의 native telemetry·계정 조회와 Clauduct job 종료 시 복원한 resident가 함께 끝날 수 있는 제한은 계속 적용된다. [#379](https://github.com/wotjr1649/Clauduct/issues/379), [#332](https://github.com/wotjr1649/Clauduct/issues/332), [v0.6.13 알려진 제한](RELEASE-v0.6.13.md#알려진-제한).
