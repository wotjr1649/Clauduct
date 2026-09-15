# Go V2 착수 판정 — 세션 34 (G0·G1)

기준선 `node-bfbdf23-g0` / commit `bfbdf2385175…` / 2026-09-15 / 실호출 0회.

## 1. 판정

```text
DESIGN:             GO                      — 핸드오프 설계를 채택, 근거 있는 수정 9건 반영
OFFLINE_BUILD:      CONDITIONAL_GO          — 5개 조건 전부 충족. 남은 것은 G2 실행 권한뿐
LIVE_VALIDATION:    NOT_AUTHORIZED          — 잔여 예산 0, 그리고 별도로 BLOCKED 사유가 있다
DEFAULT_SWITCH:     NOT_REQUESTED
RELEASE:            USER_DECISION_REQUIRED
ARCHIVAL_MOVE:      DEFERRED                — 이익 < 비용. 3장 참조
```

핸드오프 4.1절의 구현 착수 조건 다섯 가지를 로컬 근거로 판정한다.

| 조건 | 판정 | 증거 |
|---|---|---|
| 기준선 식별 | 충족 | commit·tree SHA 확정, index == HEAD tree, tracked 738개 dirty 0건, 실행 명령을 `.github/workflows/tests.yml`에서 확인 |
| Go 작업 공간 격리 | 충족 | `go/`·`comparison/`·`docs/v2/`는 기존 tracked 경로와 충돌 0. 기존 worktree 7개는 전부 `.tmp/` 아래여서 sibling worktree와 경로 충돌 없음 |
| 최소 호환 경로 검증 가능 | **충족 — 예상보다 강함** | V1이 이미 "hook 미등록" 경로에서 실패하지 않고 동작한다. 아래 2.7 |
| 인증·프로토콜 지식 확보 | 충족 | endpoint 4개, blocked option 30개, 모델 catalog 4개, context policy 값을 코드와 무인증 실행으로 확인. credential 파일은 열지 않음 |
| 평가 기준 명확 | 충족 | 전수 manifest 738/738, missing 0, duplicate 0. 게이트·예산 경계는 `VALIDATION.md` |

**이 판정이 뜻하지 않는 것**: Go 코드가 존재하지 않으며, Go 바이너리를 빌드·실행한 적이 없고, native와 함께 실행한 적이 없다. 이것은 착수 판정이지 구현 증거가 아니다.

## 2. 설계 검증 — 그대로 채택 / 수정 / 미확인

### 2.1 그대로 채택한 것

D01(Go 단독)·D02(Node 원위치 보존)·D03(`go/`·`docs/v2/`)·D05(pass-through)·D06(dev CLI 분리)·D07(ephemeral gateway)·D08(direct Codex 재구현)·D09(Claude가 유일한 도구 실행자)·D11(언어 중립 비교 계약)·D12(실호출 0)·D13(전환·출하 별도 승인)·D14(이동은 나중)를 로컬 근거와 모순 없이 채택한다.

특히 확인된 것:

- **`poc/`·`verification/`가 제품 런타임 의존이다.** 핸드오프 9.5절의 경고가 맞다. `src/clauduct.mjs`에서 출발한 import closure 28개 중 `poc/` 6개(`adapter` `user-session` `claude-inspection` `codex-transport` `gateway` `request-inspector`)와 `verification/` 2개(`manual-http-probe` `auth-store-selection`)가 들어 있다. 폴더 이름으로 "PoC = 폐기", "verification = 테스트 전용"이라 분류했다면 8개 파일의 런타임 계약을 놓쳤다.
- **제3자 npm 의존성이 0이다.** 전 저장소의 bare import specifier가 전부 `node:` 내장이다. Go 이식에서 재현해야 할 의존성 표면이 없다 — 핸드오프 12.3절의 "표준 라이브러리 우선"이 현실적인 목표다.
- **gateway endpoint는 4개다.** `HEAD /api/hello`, `GET /v1/models`, `POST /v1/messages`, 그리고 `POST /clauduct/agents`(`src/native-gateway.mjs:159`). 앞의 셋은 핸드오프 15.1절 표와 일치한다.
- **주입 agent는 정확히 14개다.** 무인증 `--dry-run -p` 출력의 `generalAgentModels`가 14행이다. 핸드오프 20.2절의 수치가 맞다.

### 2.2 수정 1 — 10.2절 이관표가 런타임 closure를 4개 빠뜨렸다

핸드오프 10.2절 표에 없는데 실제 런타임 closure에 있는 파일:

| 누락된 파일 | 왜 런타임인가 |
|---|---|
| `src/workflow-selection.mjs` | `src/clauduct.mjs`의 import closure 안. Workflow 자식 선택·resume 신원 |
| `src/retry-after-seconds.mjs` | 같은 closure. `retry-after.mjs`와 별개 파일 |
| `verification/auth-store-selection.mjs` | 같은 closure. 10.2절은 `verification/`에서 `manual-http-probe.mjs`만 언급했다 |
| `src/review-diff.mjs` | **import edge가 아예 없다.** 아래 2.3 |

실제 런타임 closure는 **29개**이며 10.2절 표가 시사하는 규모보다 크다. 이관 계약 추출은 이 29개를 분모로 한다.

### 2.3 수정 2 — Node 런타임 의존이 두 곳에서 wire로 새어 나간다 (최우선 블로커 후보)

정적 import 그래프에 보이지 않는 런타임 의존이 두 개 있고, 둘 다 **Node 실행 파일 경로를 문자열로 조립해 native child에게 넘긴다.**

```text
src/native-protocol.mjs:139-140
  fileReviewCommand = [process.execPath, ".../src/review-diff.mjs", target] → shell 문자열

src/clauduct.mjs:204-209
  hookPath = ".../src/agent-route.mjs"
  settings.hooks[*] = { command: "<process.execPath>" "<hookPath>" }
```

`src/review-diff.mjs`는 어떤 파일도 import하지 않는다. 그런데도 제품이 동작하려면 디스크에 있어야 한다. 파일명·import만 본 이관표는 이 파일을 테스트 유틸로 분류했을 것이다.

이것이 왜 중요한가: 불변조건 H01과 REL03은 "Go 제품이 Node에 runtime 의존하지 않음"을 요구한다. 위 두 곳을 기계적으로 포팅하면 **Go 바이너리가 Node 설치와 저장소 경로를 요구하게 된다.** 포팅 대상이 아니라 결정 대상이다.

| 결정 필요 | 선택지 |
|---|---|
| `review-diff` 기능 | (a) `clauduct-dev`/제품 바이너리의 하위 명령으로 재구현 → 문자열은 Go 바이너리 자기 경로를 가리킨다 (b) capability로 명시 미지원 |
| `settings.hooks` 주입 | D10을 적용해 **기본 경로에서 제거**한다. 그러면 Node 의존과 overlay 문제가 한 번에 사라진다 |

권고는 hooks는 (기본 제거), review-diff는 (a)다. 하나의 변경이 두 목표를 만족시키는 쪽을 택한다.

### 2.4 수정 3 — 제품이 설치 디렉터리 안에 상태를 쓴다

```text
src/clauduct.mjs:229
  const target = directory ?? fileURLToPath(new URL('../.clauduct-status/', import.meta.url));
```

저장소 상대 경로다. 핸드오프 21.4절은 Go 제품 상태를 "설치 디렉터리가 아니라 OS가 제공하는 사용자별 state/cache 경로"에 두라고 요구한다. 두 요구는 정면으로 충돌하므로 이것은 **이관이 아니라 신규 설계 항목**이다. `.clauduct-status`의 기존 내용을 V2가 자동으로 읽거나 합치지 않는다(21.4절과 동일).

### 2.5 수정 4 — 2.4절의 보정을 다시 보정한다

핸드오프 2.4절은 "`--settings` 주입이 사용자 설정 전체 삭제와 같다는 설명"을 과장으로 보정했다. 실측하면 절반만 맞다.

`src/clauduct.mjs`가 `--settings`로 주입하는 것은 `env`·`modelPicker`·`hooks` 세 키다. 사용자 설정 파일을 지우지 않는 것은 맞다. 그러나 `settings.modelPicker = { replaceBuiltInOptions: true, ... }`는 **picker 선택지를 실제로 전체 교체**한다. "주입은 전체 교체가 아니다"를 picker에까지 확장하면 틀린다. 핸드오프 14.4절이 금지한 "picker 전체 교체"를 V1은 지금 하고 있다.

### 2.6 수정 5 — 차단 옵션 30개의 인과가 밝혀졌다. 설계가 아니라 승인 문제다

`src/clauduct.mjs:24`의 `blockedOptions`는 정확히 30개이며, [옵션 분류](../claude-option-classification.md)의 A~F 범주와 일치한다. 차단 사유를 분해하면:

| 범주 | 개수 | V2에서의 처리 |
|---|---|---|
| wrapper가 그 flag를 직접 씀 (`--settings` `--agents` `--setting-sources` `--system-prompt`) | 4 | **D10이 원인을 제거한다.** 기본 경로에서 주입하지 않으면 차단할 이유가 없다 |
| routing hook이 사라져서 (`--bare` `--safe-mode`) | 2 | 같은 이유로 해소. CAP10이 이것을 검증한다 |
| wrapper가 값을 소유 (`--autocompact` `--fallback-model`) | 2 | contextPolicy·모델 계약 재설계 후 판단 |
| 도움말 원문이 cloud/remote/download를 명시 (`--cloud` `--environment` `--teleport` `--remote-control` 외) | 9 | **유지.** 핸드오프 2.4절은 "이름만으로 확정하지 말라"고 했는데, 이 9개는 이름이 아니라 도움말 원문 근거다. 보정 대상이 아니다 |
| 기술적으로 로컬이지만 정책 판단 (`--permission-mode` `--mcp-config` `--plugin-dir` `--worktree` `-w` `--restricted` `--betas` `--prompt-suggestions` `--dangerously-skip-permissions` 외) | 10 | **사용자 결정 사항이다.** 핸드오프 13.1절의 목표 사용례가 바로 이 넷이다 |
| 도움말만으로 미판정 (`--chrome` `--no-chrome` `--json-schema` `--brief`) | 4 | 관측 후 판정. TOOL14·TOOL16 |
| 폐기된 wrapper 옵션 (`--gpt-agents`) | 1 | V2에 없음 |

핸드오프 13.1절은 `clauduct-go --permission-mode plan`, `--mcp-config`, `--plugin-dir`, `--worktree experiment`가 동작하는 것을 목표 사용례로 든다. 이 넷은 **코드가 막는 게 아니라 정책이 막고 있다.** V2가 pass-through를 채택하면 자동으로 열린다. 열 것인지가 사용자 결정이며 8장에 질문으로 남긴다.

### 2.7 수정 6 — overlay 없는 기본 실행은 이미 동작한다 (조건 3의 결정적 증거)

핸드오프 D10·CAP04는 "기본 실행에서 agent/hook 주입 0"을 요구한다. 그때 무슨 일이 일어나는지가 미지수였다면 조건부 GO의 위험이 컸을 것이다. 실측하면 이미 구현돼 있다.

```text
src/native-gateway.mjs:347-348   등록 없는 요청 → unregisteredAgentRequests++ ; 1회만 알림 콜백
                                 그 다음 줄에서 upstream = true — 요청은 그대로 진행된다
src/clauduct.mjs:343             알림 문구: 역할별 배정 미적용, Claude가 요청한 모델과
                                 그 모델 기본 effort 사용, hook 신뢰/설정은 자동 변경하지 않음
```

즉 hook overlay가 없으면 **요청이 실패하는 게 아니라 역할별 effort 배정만 잃는다.** 이것은 미지의 설계 위험이 아니라 이름이 붙은 `EXPECTED_DELTA`다. V2의 기본 모드는 V1의 이 경로와 같은 의미를 가지며, overlay는 여기에 얹는다. 핸드오프 20.4절의 "overlay를 끈 상태가 정상적인 제품 모드여야 한다"는 요구가 기존 코드로 이미 지지된다.

### 2.8 수정 7 — macOS/Linux는 이관이 아니라 신규 설계다

`src/runtime-paths.mjs:19`는 PATH를 `;`로 분리하고, `.exe`를 찾고, `AppData/Local/...`와 `.local/bin/claude.exe`를 본다. 다른 OS 분기는 존재하지 않는다. 핸드오프 24.3절은 "각 대상의 실제 검증 없이 Windows 결과로 확장 금지"라고 했는데, 그 이전에 **확장할 원본 계약 자체가 없다.** 비-Windows는 포팅 항목이 아니라 새 요구로 세운다.

### 2.9 수정 8 — Go toolchain은 1.27.0으로 고정한다

핸드오프 12.1절은 릴리즈 이력의 1.27.1을 후보로 들었다. 이 머신의 설치본은 `go1.27.0 windows/amd64`이고 `GOTOOLCHAIN=auto`, `CGO_ENABLED=0`이다. toolchain 업그레이드는 host 설치 변경이므로 승인 없이 하지 않는다. `go.mod`는 `go 1.27.0`으로 쓰고 CI도 같은 값을 고정한다. 업그레이드가 필요해지면 그때 근거와 함께 별도로 요청한다.

### 2.10 수정 9 — 실호출은 "예산 0"이 아니라 "BLOCKED"다

핸드오프는 실호출 예산 기본값 0을 말한다. [현행 검증표](../remaining-verification.md) 3.2절은 그보다 강하다: 새 모델 실호출 예산이 **BLOCKED**이며 사유는 "사용자가 요구한 사전 출력 상한을 현재 구독 전송이 보장하지 못한다"이다. 세션 33은 잔여 0(최종 cap 327)을 기록한다.

따라서 `LIVE_VALIDATION`은 "아직 승인 안 받음"이 아니라 **선행 전송 계약이 해결되기 전에는 요청해도 안 되는 상태**다. G6→G7 전이 조건에 이 항목을 명시한다.

### 2.11 아직 확인하지 못한 것

| ID | 미확인 | 왜 |
|---|---|---|
| U1 | `verification/schema/` 416개 JSON의 개별 내용 | 역할(Codex app-server 스키마, `verification/probe.mjs`만 참조)은 확인했으나 파일을 열지 않았다 |
| U2 | manifest `status: pending` 270개의 개별 역할 | 규칙으로 분류했다. 물리 이동 전에 개별 확인이 필요하다 |
| U3 | Node 회귀 113개 파일의 현재 통과 여부 | 이번 세션에서 실행하지 않았다. 사유는 기준선 JSON의 `tests_not_run` |
| U4 | `poc/adapter.mjs:17`의 `FIXTURE_PATH`가 가리키는 `poc/fixture.txt` | tracked도 아니고 디스크에도 없다. 런타임 경로에서 실제로 읽히는지 미확인 |
| U5 | untracked 143개의 개별 소유·역할 | HANDOFF.md가 지정한 보존 대상으로만 취급했다. 읽거나 옮기지 않았다 |
| U6 | 실제 Claude·Codex 조합에서의 V2 동작 | Go 코드가 없다 |

## 3. ARCHIVAL_MOVE는 DEFERRED — 하지 말라는 쪽에 가깝다

핸드오프 11.4절은 "검증 비용이 정리의 이익보다 크면 M3를 하지 않는 것이 정상적인 결론"이라고 했다. 이 저장소에서는 그쪽이다.

- `poc/`와 `verification/`을 `legacy/node/` 아래로 옮기면 **런타임 closure 8개 파일의 상대 경로가 전부 바뀐다.** `src → poc`, `src → verification` 방향 import가 실재하기 때문이다.
- `verification/` 506개 중 416개가 `schema/` JSON이고, 이들을 참조하는 것은 문서 4개와 `verification/probe.mjs` 하나다. 옮겨서 얻는 것이 거의 없다.
- 7개 worktree가 전부 `.tmp/` 아래에 있고 그중 셋은 branch에 붙어 있다. 대규모 rename은 그 worktree들과의 비교를 무효화한다.
- Git tag와 기준선 worktree만으로 "기존 Node를 따로 본다"는 목적이 이미 달성된다.

권고: **M3를 계획에서 제거하지 말되, 기본은 실행하지 않음으로 둔다.** 사용자가 구조 정리를 명시적으로 요청할 때만 WP10을 연다.

## 4. 최소 Go vertical slice의 정확한 범위

WP01+WP02의 핵심만. 프로토콜은 한 줄도 쓰지 않는다.

```text
go/go.mod                              module github.com/wotjr1649/Clauduct/go, go 1.27.0
go/cmd/clauduct-go/main.go             제품 런처
go/internal/launch/                    argv/env/cwd 사양
go/internal/platform/runtime_windows.go  claude.exe 해석 (runtime-paths.mjs의 Windows 계약)
go/internal/gateway/                   127.0.0.1:0 bind + session token + HEAD /api/hello 만
go/internal/testkit/fakeclaude/        받은 argv/env/cwd를 JSON으로 되돌려주는 가짜 child
go/internal/buildinfo/
```

동작: cwd·인자·환경 수집 → `claude.exe` 해석 → listener bind → token 생성 → readiness 확인 → child 실행(stdio 상속, `shell: false` 동등) → 종료 관찰 → owned 자원 정리 → native exit code 반환.

**포함하지 않는 것**: `/v1/messages`, 프로토콜 변환, SSE, upstream, 인증, retry, agent 등록, overlay. 이것들은 slice가 통과한 뒤에 붙인다.

통과 기준: ARG01–ARG05, ARG08, ARG10, ENV01–ENV03, ENV09, HTTP01, LIFE01–LIFE03, REL02, REL03.

## 5. 가장 먼저 증명할 3개 위험

| 순위 | 위험 | 왜 먼저인가 | 증명 방법 | 어디서 |
|---|---|---|---|---|
| 1 | **Windows argv/env/cwd 동일성** (R08 / ARG09) | 틀리면 pass-through 전제 전체가 무너지고, 그 위에 쌓은 프로토콜 작업이 전부 헛것이 된다. offline으로 완전히 증명 가능하다 | 가짜 Claude가 받은 argv 배열·env·cwd를 JSON으로 되돌리고 입력 배열과 의미 동일성 비교. 공백·따옴표·역슬래시·한국어·이모지·빈 인자·trailing backslash·`& \| < > ^ % !` fixture | WP01 |
| 2 | **Node 런타임 의존 제거** (H01 / REL03) | 2.3의 두 곳이 해결되지 않으면 Go 바이너리가 standalone이 아니다. 늦게 발견하면 protocol 계층까지 되돌려야 한다 | 빌드된 바이너리가 내보내는 모든 command 문자열에 Node 경로·저장소 상대 경로가 없음을 assert. 설치 디렉터리 밖에서 실행해 숨은 repository-relative 의존 검사 | WP01 assert + WP04 결정 |
| 3 | **completion 이전 tool_use 미전달** (H11 / TOOL05 / WIRE11) | 틀리면 사용자 파일에 중복·잘못된 실제 부작용이 생긴다. 안전 불변조건이고, V1에 포팅할 계약과 회귀 사례가 이미 있다 | synthetic SSE로 terminal 누락·조기 EOF·중복 terminal·malformed tool arguments를 주입하고 가짜 native tool executor의 fixture 변화가 0인지 관측 | WP03/WP04 |

2번을 1·3 사이에 두는 이유는 비용이다. WP01에서 assert 한 줄이면 잡히고, WP04까지 미루면 protocol DTO가 그 위에 얹힌 뒤다.

## 6. 다음 하나의 bounded work package

**WP01 — Go workspace와 독립 launcher skeleton.** 4장의 slice를 5장 위험 1·2까지 증명하는 범위로 구현한다.

선행 조건(사용자 승인 필요):

1. **G2 격리 실행 권한** — `git worktree add -b redesign/go-v2-native-host <sibling> bfbdf2385175…`. 기존 worktree 7개는 전부 `.tmp/` 아래이므로 sibling 경로와 충돌하지 않는다. 승인 없이는 실행하지 않았다.
2. **root 변경 allowlist 확인** — WP01이 건드릴 root 파일은 `.gitignore`(`/artifacts/` 한 줄 추가)와 `.github/workflows/tests.yml`(별도 Go job 추가) 둘뿐이다. 기존 Node job의 trigger·path filter·timeout은 건드리지 않는다.

중단 조건: 위험 1이 fixture 세 개 이상에서 재현 불가능하게 실패하면 프로토콜 작업으로 넘어가지 않고 보고한다.

## 7. 새 요구로 세우는 것

기존 Node 범위에서 제외·이관된 항목을 "V2이므로 필수"로 부활시키지 않는다(핸드오프 2.3절). V2가 **새로 만드는** 위험에만 새 ID를 붙인다.

| 새 ID | 요구 | 왜 새로운가 |
|---|---|---|
| V2-01 | 제품 상태를 설치 디렉터리 밖 OS state 경로에 둔다 | 2.4. V1은 저장소 상대 경로에 쓴다 |
| V2-02 | 빌드 산출물의 command 문자열에 Node 경로가 없다 | 2.3. Node에는 존재하지 않던 요구 |
| V2-03 | 기본 실행의 주입 키 수가 0이다 | 2.7. V1은 3키 + agent 14개 + hook 3이벤트 |
| V2-04 | 비-Windows 지원은 신규 설계로 취급한다 | 2.8. 이관할 원본이 없다 |

## 8. 사용자 결정이 필요한 것

이 세션에서 자율로 정할 수 없고, 답에 따라 다음 작업이 달라지는 것만 적는다.

1. **G2 실행 권한.** 새 branch `redesign/go-v2-native-host`와 sibling worktree를 만들어도 되는가. 만들지 않고 현재 작업 트리의 `go/`에서 진행하는 것도 가능하지만, 그러면 비교 실행 시 두 구현이 같은 작업 트리를 공유한다.
2. **정책 옵션 10개의 pass-through 여부** (2.6). `--permission-mode` `--mcp-config` `--plugin-dir` `--worktree` `--restricted` `--betas` `--prompt-suggestions` `--dangerously-skip-permissions` 계열. 핸드오프 13.1절의 목표 사용례는 이들이 열려야 성립한다. 기술적 장애는 없고 권한 정책 판단이다.
3. **`review-diff` 기능의 처분** (2.3). Go 하위 명령으로 재구현할지, capability 미지원으로 선언할지.
4. **Node 회귀 기준선을 지금 찍을지** (U3). 113개 파일의 offline 실행은 가능하다. 비교 기준으로 필요해지는 시점은 G4 이후라 지금은 보류했다.

`docs/v2/`와 `comparison/` 산출물의 stage·commit 여부도 사용자 판단으로 남긴다 — 이번 세션은 파일 생성까지만 했다.
