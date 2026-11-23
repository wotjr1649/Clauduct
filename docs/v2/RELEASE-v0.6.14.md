# v0.6.14 — 도구 호출 호환성과 요청 대기 보완

출하 후보는 `a2be1014d3d47bb67bed5a16db77136cf07fe276`이다. v0.6.13 이후 도구 인자 수신, 라우팅 증거 대기, 특정 native 빌드의 인자 판정을 손봤다. 2026-11-23 소유자 결정에 따라 이번 출하는 소유자 예외 경로를 따른다.

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

- native 2.1.330 무과금 도구 표면·core 도구 재측정(`c1`) — 수집 완료, native 연속성 PASS. 도구 이름·core 도구 목록·slash command·skill·agent 목록은 2.1.325와 같다. 도구 설명과 선택 입력(`ListAgents`의 `cursor`, `ReportFindings`의 `severity`), system prompt 문구, `--help`, plugin API에 차이가 있다. 독립 검토에서 제품 변경이 필요한 차이는 없다고 판정했다. `Workflow`의 `resumeFromRunId`로 명시 재실행할 때의 캐시 동작은 확인하지 않았다..
- 정식 태그가 가리키는 commit, 발행 자산과 `SHA256SUMS`의 SHA-256, 설치·업데이트 및 발행 뒤 확인 결과는 출하 검사에서 확정한다. [PACKAGING §4.1](PACKAGING.md#41-릴리스-만들기), `docs/v2/PACKAGING.md:151`.

## 출하 신원

| 항목 | 값 |
|---|---|
| 대상 태그 | `v0.6.14` |
| 후보 commit | `a2be1014d3d47bb67bed5a16db77136cf07fe276` |
| 로컬 후보 빌드 `clauduct.exe` SHA-256 | `68e5e886f084834fafe75552968ad94fccc9ae44f7eaa40225b1c97d7616f2d1` |
| 재측정 대상 native | Claude Code 2.1.330, SHA-256 `fa797c1bc7d4a3511da42622bae53a2b7f1276ae328c668f2bfef44635301caa` |

로컬 후보의 빌드 환경은 Windows/amd64, Go 1.27.4, `CGO_ENABLED=0`, `-trimpath`다. 공개 자산의 신원은 발행 뒤 확인한 값으로 기록한다.

## 알려진 제한

- **native 2.1.330에서 #331 재현 형태는 기존 경로를 탄다.** 옵션 값이나 `--` 뒤의 `--bg` 때문에 Clauduct와 native의 판단이 갈리면 `BACKGROUND_DISPATCH_UNVERIFIED`가 날 수 있다. 선행 옵션 뒤 관리 명령은 세션 준비가 실패하면 native에 도달하지 못할 수 있다. `s331`은 2.1.325 고정 빌드의 동작을 확인한 표본이다. [#331](https://github.com/wotjr1649/Clauduct/issues/331), [COMPATIBILITY](COMPATIBILITY.md), `docs/v2/COMPATIBILITY.md:1310`.
- **위 표본의 확인 범위는 해당 경로에 한정된다.** 2.1.325의 표본은 부하별 기한 경합이나 라우팅 대기 알림을 재현하지 않았다. `d1`에서 확인한 effort는 시작값과 같았다. [#383](https://github.com/wotjr1649/Clauduct/issues/383).
- background의 native telemetry·계정 조회와 Clauduct job 종료 시 복원한 resident가 함께 끝날 수 있는 제한은 계속 적용된다. [#379](https://github.com/wotjr1649/Clauduct/issues/379), [#332](https://github.com/wotjr1649/Clauduct/issues/332), [v0.6.13 알려진 제한](RELEASE-v0.6.13.md#알려진-제한).
