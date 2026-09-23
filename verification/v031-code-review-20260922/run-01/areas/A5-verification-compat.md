# A5 — 검증·호환성 분야 리뷰

- 대상 루트: `D:\AIDEV\clauduct-v031` (git worktree, branch `fix/v031`)
- 기준 commit: `149068edd693fb860a03244a2ea15764bcd68c34` (v0.3.0). HEAD 동일, v0.3.1 변경은 전부 미커밋 작업트리
- 담당 파일 14개 전부 커버
- 제품 소스·테스트·문서·기존 `verification/` 파일은 하나도 수정·생성·삭제하지 않았다. git index/작업트리 변경 없음(읽기 전용 git 명령만 사용).
  쓰기는 `verification/v031-code-review-20260922/run-01/areas/A5-verification-compat.md`와
  `verification/v031-code-review-20260922/run-01/repro/A5-verification-compat/` 안에만 했다.

---

## 1. 실행 근거 — /code-review

**호출 원문 (Skill 도구):**

```
skill = "code-review"
args  = "xhigh docs/v2/ARCHITECTURE.md docs/v2/COMPATIBILITY.md docs/v2/PACKAGING.md docs/v2/PARITY.md go/README.md go/internal/app/integration_tui_test.go go/internal/app/integration_runtime_evidence_test.go"
```

`ultra`, `--fix`, `--comment`는 쓰지 않았다.

**로드 여부:** 로드됨. 반환값 원문:
`Skill "code-review" launched (forked execution, running in the background). Running in the background as @code-review`

즉 이 빌드의 내장 code-review 스킬은 현재 턴에 레시피를 주입하지 않고 **fork 실행(백그라운드 서브에이전트)** 으로 동작한다.
따라서 "스킬 레시피를 읽고 그대로 따른다"는 형태의 수행은 구조적으로 불가능했고, 나는 같은 엄격도로 직접 리뷰를 끝까지 수행했다.

**내장 스킬 vs 미설치 플러그인:** 이 세션에서 실행된 것은 **CLI 내장 `code-review` 스킬**이다.
같은 이름의 marketplace 플러그인 `claude-plugins-official/code-review`와 `pr-review-toolkit`은
카탈로그에만 있고 설치되어 있지 않다(`~/.claude/plugins/installed_plugins.json`에 항목 없음).
아래 지적은 그 플러그인들의 결과가 아니다.

**관측된 effort/모델 표기:** `NOT_OBSERVED`.
호출 응답에도 `ListAgents` 출력에도 effort 등급이나 모델 ID 표기가 없었다.
`ListAgents`가 보여준 전부는 `code-review [adbf76] · general-purpose · running · started Nm ago`였다.
확인 가능한 것은 내가 전달한 인자 문자열에 `xhigh`가 들어 있다는 사실뿐이다.

**서브에이전트:** 나는 추가 서브에이전트를 직접 띄우지 않았다.
다만 내장 스킬이 fork 실행되면서 자기 레시피에 따라 `code-review` 외에 4개의 `general-purpose`
서브에이전트(`a983c63d62c76c5e9`, `a073a6dbf46156a25`, `a806945faba8e73e3`, `ac81dc4b9cb403ccf`)를
추가로 생성했다. 이는 스킬 내부 동작이며 내가 분산한 것이 아니다. 관측 사실로 기록해 둔다.

**스킬 커버 범위 vs 직접 리뷰 범위 — `NOT_RUN` 처리:**
`code-review` 에이전트는 약 12–16분 사이에 에이전트 목록에서 사라졌으나(= 종료),
이 리뷰를 마칠 때까지 **결과가 이 세션에 도달하지 않았다.**
결과 없는 종료를 재실행 근거로 삼지 않았고(같은 작업 단위를 두 번 돌리지 않음),
스킬이 실제로 훑은 범위를 확인할 방법이 없으므로 **스킬 커버 범위 = 확인 불가**로 남긴다.
그 결과 **담당 파일 14개 전부를 직접 리뷰로 커버**했다(§2 커버리지 표).
본 보고서의 §3~§7 지적은 **전부 직접 리뷰 출처(`manual`)** 이며, 스킬이 낸 지적은 0건이다(도달하지 않음).

---

## 2. 커버리지 표

| # | 담당 파일 | 검토 방식 | 대조한 것 |
|---|---|---|---|
| 1 | `docs/v2/ARCHITECTURE.md` | diff 전문 + 주장별 코드 대조 | §4 CLI 계약 → `launch/refuse.go`, `app/user_settings.go`, `app/native_args.go`, `launch/launch.go`, `app/run.go:144-152`. §6.1 → `gateway/connection.go`, `gateway/gateway.go:167-179` + 회귀검사 실행. §7.1 → `gateway/counter.go:104`, `count_tokens.go:123` 호출처 전수 |
| 2 | `docs/v2/COMPATIBILITY.md` | diff 전문 + 주장별 대조 | `requestClassRequired`/`requestClassMissing` → `gateway/diagnostics.go:723-724,844-845`, `app/run.go:465`. transport 항목 → `v031-transport-20260922/evidence.json` |
| 3 | `docs/v2/PACKAGING.md` | diff 전문 + 주장별 대조 | projects 트리 metadata → `app/run.go:301`, `gateway/context_journal.go:33`, `delegation.go:626,765`, `workflow_checkpoint.go:251-265`. 되돌리기 제약 → 기준 commit `delegation.go`의 reader 직접 확인 |
| 4 | `docs/v2/PARITY.md` | diff 전문 + 주장별 대조 | E4 → `gateway/context_compaction.go:32-39`와 호출처 2곳(`context.go:279`, `context_compaction.go:93`) |
| 5 | `go/README.md` | diff 전문 + 주장별 대조 | 거부 순서 → `app/run_test.go:517 TestRefusedOptionStopsBeforeAnythingIsAcquired` 존재 확인. ARCHITECTURE 4절 링크·앵커 |
| 6 | `go/internal/app/integration_runtime_evidence_test.go` | 전체 읽음 + 실행 | 태그 붙여 `TestIntegrationAuditSignalsOnlyTextStart` 실행 PASS. live/PTY 검사는 NOT_RUN(§8) |
| 7 | `go/internal/app/integration_tui_test.go` | 전체 읽음 + 실행 + overlay 돌연변이 2건 | 15개 대조군 실행 PASS, mutant A/B로 대조군의 실제 검출력 측정 |
| 8 | `verification/v031-integration-20260921/REPORT.md` | 전체 읽음 + 주장 대조 | build tag/env 게이트 주장, 검수 결함·수정 서술, 되돌리기 probe 서술 |
| 9 | `verification/v031-integration-20260921/compat-v030-probe_test.go` | 전체 읽음 + 기준 commit API 대조 | `preparedDelegation`, `route`, `choicePath`, `loadChoice` 시그니처를 `149068e`에서 확인 |
| 10 | `verification/v031-integration-20260921/evidence.json` | 전체 읽음 + 대조 | `tagged_compile: PASS_NO_TESTS_RUN` 등 |
| 11 | `verification/v031-offline-20260922/REPORT.md` | 전체 읽음 + 주장 대조 | 15개 대조군 실행 주장, 검수기 수정 내용 |
| 12 | `verification/v031-offline-20260922/evidence.json` | 전체 읽음 + 대조 | `tui_assessment`, `doc_citations` |
| 13 | `verification/v031-recheck-20260922/REPORT.md` | 전체 읽음 + 주장 대조 | usage 정책 정정, 보호 on/off 대조 |
| 14 | `verification/v031-recheck-20260922/evidence.json` | 전체 읽음 + SHA256 실측 대조 | **불일치 발견 (A5-01)** |

추가로 읽기만 한 파일(변경 목록 밖, 수정하지 않음):
`go/internal/buildinfo/buildinfo.go`, `go/internal/launch/refuse.go`, `go/internal/launch/launch.go`,
`go/internal/app/run.go`, `go/internal/app/user_settings.go`, `go/internal/app/native_args.go`,
`go/internal/app/role_case_probe_test.go`, `go/internal/gateway/connection.go`, `gateway.go`,
`counter.go`, `count_tokens.go`, `context_compaction.go`, `delegation.go`, `context_journal.go`,
`diagnostics.go`, `messages.go`, `go/internal/protocol/bridge/route.go`, `go/internal/update/run.go`,
`.github/workflows/go.yml`, `docs/v2/DECISION.md`, `verification/v031-transport-20260922/*`,
`verification/v031-roles-20260921/REPORT.md`.

**미검토 담당 파일: 없음.**

---

## 3. 지적 목록

### A5-01 — `confirmed` / medium — recheck 기계 기록의 SHA256이 출하 트리와 불일치

- **파일:** `verification/v031-recheck-20260922/evidence.json:60`
  (`adguard.same_test_code_sha256["go/internal/gateway/socket_runtime_evidence_test.go"]`)
- **발생 조건:** recheck 보고서의 "보호 On/Off 두 조건 사이 검사 코드가 동일하다"는 주장을 기록된 해시로 감사하려 할 때. 항상 재현.
- **원인:** `evidence.json`은 `2026-09-22 09:30:36`에 기록됐고, 같은 날 `09:47:13`에 이후 라운드
  (`v031-close-20260922`. 그 REPORT가 raw TCP 검사에 "고정 이벤트명"을 추가했다고 적는다)가 같은 검사 파일을 수정했다.
  그 뒤 어떤 기계 기록도 해시를 갱신하지 않았다.
- **영향:** recheck REPORT.md의 "세 검사 파일의 SHA256은 기계 기록에 있으며, 두 조건 사이 코드·timeout·판정은 동일하다"가
  출하 트리로는 검증 불가능해진다. 제품 결함이 아니라 **검증 기록 무결성 결함**이다.
  보호 On/Off 대조 — 이번 릴리스 판정의 핵심 근거 중 하나 — 의 감사 가능성이 떨어진다.
- **재현 명령** (cwd `/d/AIDEV/clauduct-v031`, 환경 변경 없음):

  ```
  sha256sum verification/test-http-transport.mjs verification/test-dotnet-http-transport.mjs \
            go/internal/gateway/socket_runtime_evidence_test.go
  grep -A4 'same_test_code_sha256' verification/v031-recheck-20260922/evidence.json
  ls -la --time-style=full-iso go/internal/gateway/socket_runtime_evidence_test.go \
                               verification/v031-recheck-20260922/evidence.json
  ```

  exit code 0.
- **예상 결과:** 세 해시 모두 기록과 일치.
- **실제 결과:** 앞의 두 `.mjs`는 일치.
  `socket_runtime_evidence_test.go`는 실측 `7dc3741c56061f3fac289a2a4c3e5f79d30c37b52213297529be19121db1252c`,
  기록 `fdcb21ed37f17a3d0652c1360b02c080c9b9f9310c28740bf69667750fc362ce`. **불일치.**
  파일은 LF이며 줄바꿈 변환 가설도 배제했다(CRLF 변환 해시는 `4bd32265…`로 역시 다름).
- **증거 경로:** `D:\AIDEV\clauduct-v031\verification\v031-code-review-20260922\run-01\repro\A5-verification-compat\sha256-mismatch.txt`
- **권장 수정:** 기록을 소급해 고치지 말고, ① recheck `evidence.json`에 "이후 `v031-close-20260922`
  라운드에서 이 파일이 변경되었고 기록된 해시는 대조 시점의 값"임을 명시하는 필드를 추가하거나,
  ② `v031-close-20260922` / `v031-transport-20260922` 기계 기록에 대조 시점 해시와 현재 해시를 함께 남긴다.
  어느 쪽이든 해시 옆에 기록 시점(timestamp 또는 commit)을 같이 적어 이후 라운드가 무효화했음을 읽히게 한다.
- **미확인:** `fdcb21ed…` 시점의 파일 내용은 커밋되지 않아 복원 불가. 두 판본의 **판정 로직**이 실제로 동일했는지는 확인할 수 없다.
- **신규/기존:** 신규 회귀 — 해당 `evidence.json`과 검사 파일 모두 v0.3.1에서 새로 추가된 것이다.
- **출처:** 직접.

### A5-02 — `confirmed` / medium — 문서가 선언한 "v0.3.1 개발본"을 제품이 스스로 식별하지 못함

- **파일:** `docs/v2/PACKAGING.md:353-357` (새 §6 항목), `docs/v2/COMPATIBILITY.md:12-88` (v0.3.1 항목 다수).
  대조 대상: `go/internal/buildinfo/buildinfo.go:52` — `const Version = "0.3.0"` (변경 목록에 없음, 읽기만 함)
- **발생 조건:** 항상. 이 작업트리에서 빌드한 바이너리를 실행할 때마다.
- **원인:** v0.3.1 변경이 `buildinfo.Version`을 올리지 않았다. 문서는 "v0.3.1 개발본"을 v0.3.0과 구별되는
  빌드로 선언하고, PACKAGING §6은 그 둘 사이의 **세션 기록 되돌리기 제약**까지 규정한다.
  게다가 v0.3.1 변경이 전부 미커밋이라 VCS stamp도 기준 commit + `+dirty`가 되어
  "dirty한 v0.3.0 트리"와 출력 문자열이 완전히 같아진다.
- **영향:** PACKAGING §6의 지시("v0.3.1에서 해당 세션을 계속하거나, v0.3.0에서는 새 세션을 시작한다")를
  따르려는 사용자가 **지금 어느 쪽을 실행 중인지 제품 출력으로 판별할 수 없다.**
  `clauduct-dev version`, `clauduct --update`의 `running` 줄, 진단·상태 파일의 버전 표기가 전부 `0.3.0`이다.
  릴리스 시 `Version` 상수와 PACKAGING §3 릴리스 절차(`v0.3.0` 태그 예시)를 같이 갱신해야 한다.
  단 **`--update`의 `already current` 판정은 digest 기반**(`PACKAGING.md:272`)이므로 업데이트 경로 자체는 깨지지 않는다.
- **재현 명령** (cwd `/d/AIDEV/clauduct-v031/go`, `CGO_ENABLED=0`):

  ```
  CGO_ENABLED=0 go build -o <repro>/clauduct-dev-probe.exe ./cmd/clauduct-dev
  <repro>/clauduct-dev-probe.exe version
  ```

  exit code 0. (빌드 산출물은 자기 repro 디렉터리에만 두었고 확인 후 삭제, 출력만 보존)
- **예상 결과:** 문서가 말하는 "v0.3.1 개발본"임을 알 수 있는 표기.
- **실제 결과:**

  ```
  clauduct     0.3.0
  commit       149068edd693fb860a03244a2ea15764bcd68c34+dirty
  go           go1.27.1 windows/amd64
  ```

- **증거 경로:** `D:\AIDEV\clauduct-v031\verification\v031-code-review-20260922\run-01\repro\A5-verification-compat\version-output.txt`
- **권장 수정:** 릴리스 전 `buildinfo.Version`을 `"0.3.1"`로 올리고 기존 서술 방식대로 근거 주석을 추가한다.
  올리지 않기로 한다면 PACKAGING §6과 COMPATIBILITY의 "v0.3.1 개발본" 표현 옆에
  "버전 문자열은 0.3.0으로 동일하며 commit으로만 구분된다. 미커밋 상태에서는 `+dirty` 외 구분자가 없다"를 명시한다.
- **미확인:** 릴리스 시 버전을 올릴 계획이 문서화된 곳을 찾지 못했다.
- **신규/기존:** 신규 회귀 — 기준 commit에는 "v0.3.1"이라는 빌드 정체성 주장 자체가 없었다.
- **출처:** 직접.

### A5-03 — `confirmed` / medium — `runtime_evidence` 태그 파일이 CI에서 컴파일조차 되지 않음 (조용한 skip)

- **파일:** `go/internal/app/integration_tui_test.go:1`, `go/internal/app/integration_runtime_evidence_test.go:1`
  (`//go:build runtime_evidence`). 대조 대상: `.github/workflows/go.yml`의 vet/build/test/test(race) 단계
- **발생 조건:** 모든 CI 실행, 그리고 태그를 주지 않은 모든 로컬 검사.
- **원인:** CI는 `go vet ./...`, `go build ./...`, `go test -count=1 ./...`, `go test -count=1 -race ./...`를
  전부 **태그 없이** 실행한다. `runtime_evidence` 태그 파일은 빌드 대상에서 통째로 빠진다.
  `t.Skip`에도 도달하지 않으므로 skip 표시조차 남지 않는다 — 존재가 보이지 않는 skip이다.
- **영향:** 두 가지다.
  1. v0.3.1이 새로 넣은 **환경 의존이 전혀 없는 순수 로직 검사** 두 개
     (`TestTUIAcceptanceRejectsMissingOrMisorderedEvidence` 15개 대조군, `TestIntegrationAuditSignalsOnlyTextStart`)가
     CI에서 한 번도 실행되지 않는다. backend·PTY·자격 증명이 필요 없어 CI에서 돌 수 있는데도 돌지 않는다.
  2. 태그 파일의 **컴파일 회귀도 CI가 잡지 못한다.** `gateway.Diagnostics`, `FeatureEvidence`,
     `SessionTotals`, `RequestRecord`, `upstream.Call/Response/Ledger` 중 하나라도 시그니처가 바뀌면
     검수기가 깨진 채로 CI는 녹색이다. 실호출 검수를 다시 돌리려는 시점 — 즉 예산을 쓰는 시점 — 에야 드러난다.
     보고서들이 매 라운드 수동으로 `tagged vet`·`tagged compile`을 돌리고 있는 것이 이 공백을 사람 손으로 메우고 있다는 증거다.
- **재현 명령** (cwd `/d/AIDEV/clauduct-v031/go`):

  ```
  CGO_ENABLED=0 go test ./internal/app/ \
    -run 'TestTUIAcceptanceRejectsMissingOrMisorderedEvidence|TestIntegrationAuditSignalsOnlyTextStart' \
    -count=1 -timeout 300s
  CGO_ENABLED=0 go test -tags=runtime_evidence ./internal/app/ \
    -run 'TestTUIAcceptanceRejectsMissingOrMisorderedEvidence|TestIntegrationAuditSignalsOnlyTextStart' \
    -count=1 -timeout 300s
  ```

  둘 다 exit code 0.
- **예상 결과:** CI가 돌리는 형태(태그 없음)에서도 두 검사가 실행됨.
- **실제 결과:** 태그 없음 → `ok … [no tests to run]` (실행 0개).
  태그 있음 → 16개 케이스(상위 2 + 서브 15) PASS.
- **증거 경로:** `D:\AIDEV\clauduct-v031\verification\v031-code-review-20260922\run-01\repro\A5-verification-compat\mutation-run.txt`
- **권장 수정:** 환경이 필요 없는 두 검사를 태그 없는 파일로 분리하거나,
  `go.yml`에 `go vet -tags=runtime_evidence ./...` 단계와
  `go test -tags=runtime_evidence -run 'Acceptance|Audit' ./internal/app ./internal/gateway` 단계를 추가한다.
  최소한 **태그 패키지의 컴파일·vet 단계**만이라도 CI에 넣는다.
- **미확인:** `tests.yml`(Node 기준선 워크플로)은 Go 태그와 무관함을 확인했다. 조직 차원의 다른 러너는 확인 범위 밖.
- **신규/기존:** **패턴 자체는 기존 결함** — 기준 commit에도 `gateway/nonstream_evidence_test.go`,
  `upstream/runtime_evidence_test.go`, `upstream/parameter_evidence_test.go`가 같은 태그로 존재했고 CI도 동일했다.
  v0.3.1이 태그 파일을 4개(`app` 2 + `gateway` 2) 더 늘렸고, 그중 처음으로
  **환경 불필요 순수 로직 검사**를 태그 뒤에 넣었다는 점이 신규 확대분이다.
- **출처:** 직접.

### A5-04 — `confirmed` / low — 검수기 수정의 핵심 단언이 자기 대조군 15개에 전혀 걸리지 않음

- **파일:** `go/internal/app/integration_tui_test.go:40-42`
  (`if d.Totals.Failures["CANCELLED"] != 1 { return errors.New("TUI_CANCELLATION_INCOMPLETE") }`)
- **발생 조건:** 누군가 이 단언을 지우거나 약화할 때. 대조군 표가 검출하지 못한다.
- **원인:** 15개 돌연변이 중 취소를 건드리는 것은 `missing_cancel`(transcript 행 삭제),
  `no_cancel`(`Features[0].Cancelled = 0`), `early_recovery` 셋뿐이다.
  앞의 둘은 각각 transcript stage 기계와 `generation.Cancelled != 1`에서 **먼저** 걸린다.
  `d.Totals.Failures["CANCELLED"]`만 1이 아닌 상태를 만드는 대조군이 없다.
- **영향:** `v031-offline-20260922/REPORT.md`가 검수 결함 수정의 내용으로 명시한 바로 그 줄
  ("최근 16건에서 밀려난 요청도 보존하는 `Totals.Failures["CANCELLED"]`를 사용한다")이 회귀 보호를 받지 못한다.
  같은 보고서가 "15개 대조군이 통과했다"를 그 수정의 근거로 제시하므로 근거가 실제보다 강하게 읽힌다.
- **재현 명령** (cwd `/d/AIDEV/clauduct-v031/go`, `CGO_ENABLED=0`, 원본 무수정 `-overlay` 사용):

  ```
  CGO_ENABLED=0 go test -tags=runtime_evidence \
    -overlay=<repro>/mutA/overlay.json ./internal/app/ \
    -run 'TestTUIAcceptanceRejectsMissingOrMisorderedEvidence' -count=1 -timeout 300s
  ```

  mutA = 위 3줄만 삭제한 사본(`<repro>/mutA/integration_tui_test.go`).
- **예상 결과:** 최소 1개 대조군 FAIL.
- **실제 결과:** `ok github.com/wotjr1649/Clauduct/go/internal/app 0.113s`, **exit 0. 15개 전부 통과.**
- **반대 방향 대조:** mutB = 수정 **이전** 방식(`d.Recent` 순회 + `CancellationSource != ""`)으로 되돌린 사본은
  `none`과 `recent_evicted` 두 대조군에서 FAIL, exit 1.
  즉 **전체 되돌리기는 검출되지만 해당 단언 단독 삭제는 검출되지 않는다.**
- **증거 경로:** `D:\AIDEV\clauduct-v031\verification\v031-code-review-20260922\run-01\repro\A5-verification-compat\mutation-run.txt`
  (사본과 overlay 매니페스트: `…\mutA\`, `…\mutB\`)
- **권장 수정:** 대조군에 `totals_cancel_zero` 한 건 추가
  (`d.Totals.Failures = map[string]int64{}` 또는 `{"CANCELLED": 0}`으로 두고 `Features`는 정상 유지). 기대 판정은 FAIL.
- **미확인:** 없음.
- **신규/기존:** 신규 — 파일 자체가 v0.3.1 신규.
- **출처:** 직접.

### A5-05 — `hold` / low — 실호출 검수기가 기본 `TEMP` 환경에서는 자기 guard로 모든 요청을 400 거부할 가능성

- **파일:** `go/internal/app/integration_runtime_evidence_test.go:110-113` (`privateIntegrationPath`),
  같은 파일 `integrationWorkspace`
- **발생 조건:** `CLAUDUCT_EVIDENCE_LIVE=1`(또는 `CLAUDUCT_EVIDENCE_TUI_LOCAL=1`)로 실행하면서
  `TEMP`/`TMP`를 기본값(`C:\Users\<user>\AppData\Local\Temp`)으로 둔 경우.
- **원인:** `integrationWorkspace`는 `t.TempDir()`(= `os.TempDir()` 하위)에 profile과 cwd를 만든다.
  `privateIntegrationPath`는 요청 본문에 `c:\users\`가 있으면 `EVIDENCE_PRIVATE_PATH`/400을 반환한다.
  native는 작업 디렉터리를 system prompt에 실어 보낸다.
  두 보고서의 실제 실행은 `D:\AIDEV\clauduct-v031\.tmp\integration-temp`를 써서 이 조건을 피해 갔지만,
  그 전제가 검사 코드에도 보고서에도 적혀 있지 않다.
- **영향:** 기본 환경에서 재실행하면 모든 요청이 400으로 막히고 화면상으로는 backend 장애처럼 읽힌다.
  실호출 예산을 쓰는 검사라 오진 비용이 크다.
- **재현 명령** (cwd `<repro>/tempdir-guard`):

  ```
  CGO_ENABLED=0 go run main.go
  ```

  exit code 0. 제품 코드를 import하지 않고 `integrationWorkspace`의 cwd 계산과
  `privateIntegrationPath`를 그대로 복제한 독립 프로그램이다.
- **예상 결과:** guard가 걸리지 않음.
- **실제 결과:**

  ```
  os.TempDir()            = C:\Users\js\AppData\Local\Temp
  integration cwd         = C:\Users\js\AppData\Local\Temp\TestRuntimeEvidenceNativeTUI1234\001\public-project
  privateIntegrationPath  = true  (true => transport returns EVIDENCE_PRIVATE_PATH / 400)
  ```

- **증거 경로:** `D:\AIDEV\clauduct-v031\verification\v031-code-review-20260922\run-01\repro\A5-verification-compat\tempdir-guard\output.txt`
- **권장 수정:** `integrationWorkspace`에서 생성한 root에 대해 `privateIntegrationPath`가 참이면
  `t.Skipf("redirect TEMP off C:\\Users before running the evidence suite")`로 명시적으로 멈추게 한다.
  **guard 자체를 약화하지 말 것** — 신뢰 경계 검사다.
- **미확인 / 결론을 내리려면:** native가 실제로 cwd를 요청 본문에 싣는지.
  기본 `TEMP`에서 `CLAUDUCT_EVIDENCE_TUI_LOCAL=1` 로컬 PTY 1회 실행이면 확정된다
  (backend 지출 0, 로컬 합성 transport). 이 리뷰에는 PTY 환경이 없어 미확인으로 남긴다.
- **신규/기존:** 신규 — 파일 자체가 v0.3.1 신규.
- **출처:** 직접.

### A5-06 — `hold` / low — 되돌리기 문서의 v0.3.0 거부 조건 목록이 불완전할 수 있음 (역할 이름 정규화)

- **파일:** `docs/v2/PACKAGING.md:353-357`
- **발생 조건:** v0.3.1이 **비-custom(내장) 역할**의 선택 journal을 쓰고, native가 보고한 그 역할 이름의
  대소문자가 내장 표 key와 다를 때. 이후 v0.3.0으로 되돌려 같은 세션을 재개하는 경우.
- **원인:** PACKAGING은 거부 조건을 `customRole` 필드와 `source:native-selection` 두 가지로 한정한다.
  그런데 v0.3.1은 `bridge.CanonicalRole`(기준 commit에 **없던** 새 함수, `go/internal/protocol/bridge/route.go:162-174`)로
  비-custom 역할 이름을 내장 표 철자로 정규화한 뒤 journal에 저장한다
  (`gateway/delegation.go:276-278`, `:482-484`, 저장은 `:643`의 `Role: c.role`).
  기준 commit의 reader는 `saved.Role != binding.Role`로 **정확 비교**한다
  (`git show 149068e:go/internal/gateway/delegation.go`, 704행).
  따라서 정규화로 철자가 바뀐 journal은 v0.3.0에서 `errDelegationUnverified`로 거부된다.
- **영향:** 문서화된 미지원 범위가 실제보다 좁게 적혔을 수 있다.
  사용자가 두 조건에 해당하지 않으니 되돌려도 된다고 판단했다가 같은 세션 재개가 거부될 수 있다.
- **재현 명령:** `NOT_RUN`. `compat-v030-probe_test.go`와 같은 방식으로 기준 commit을 별도 worktree에
  체크아웃해야 하는데, 이 리뷰의 하드 금지(작업트리·index 보존, checkout 금지)를 우회하지 않았다.
- **예상/실제:** 코드 대조까지만 수행. 기준 reader의 정확 비교와 v0.3.1의 정규화 저장은 양쪽 소스에서 확인했다.
- **증거 경로:** NONE (코드 대조 근거만 — 위 파일·행 번호 참조)
- **권장 수정:** ① native가 내장 역할 이름을 표와 다른 철자로 보고하는 사례가 실제로 관측되는지 먼저 확정.
  ② 관측되면 PACKAGING §6에 세 번째 조건으로 추가.
  ③ 더 근본적으로는 `compat-v030-probe_test.go`가 **v0.3.1이 실제로 쓴 journal**을 v0.3.0 reader에 먹이도록 바꾼다.
  현재는 v0.3.0이 쓴 journal을 손으로 변형하므로 **v0.3.1 writer 쪽의 변화가 구조적으로 보이지 않는다.**
- **결론을 내리려면:** `CanonicalRole`이 실제로 값을 바꾸는 입력(native가 보고하는 대소문자 변형 내장 역할명)의 관측 근거.
  `v031-roles-20260921/REPORT.md` #43은 "내장 역할 표의 철자를 한 함수에서 정규화한다"고만 적고,
  변형 입력의 실제 관측은 #44(사용자 `Fork` — custom 경로이며 이미 문서화된 조건)만 기록한다.
- **신규/기존:** 신규 회귀 가능성 — `CanonicalRole`은 v0.3.1 신규.
- **출처:** 직접.

---

## 4. 반박된 지적과 반박 근거

| ID | 의심 | 반박 근거 |
|---|---|---|
| R1 | `connection.go`의 drain이 절대 실행되지 않는다 — `responseConn.Write`는 `finished`를 `false`로만 저장하고 `true`로 만드는 곳이 파일 안에 없다 | `gateway.go:167-168`이 핸들러 래퍼에서 `c.finished.Store(r.Context().Err() == nil)`을 수행한다. 조건은 `r.Close && ProtoMajor==1 && ProtoMinor>=1`로 문서가 말한 범위와 일치. 회귀검사 `TestResponseCloseDrainIsBoundedAndInterruptible`(forced=false/true) 실행 PASS |
| R2 | 검수기 15개 대조군은 컴파일만 되고 실행된 적이 없다 (`v031-integration-20260921/evidence.json`의 `"tagged_compile": "PASS_NO_TESTS_RUN"`) | 그 기록은 integration 라운드의 것이다. 이후 offline 라운드가 실제로 실행했다 — `v031-offline-20260922/evidence.json` → `tui_assessment: {cases: 15, race: "PASS", race_seconds: 1.234}`. 내가 직접 재실행해도 16개 케이스 PASS, exit 0 |
| R3 | ARCHITECTURE 7.1의 "일반 생성과 압축은 `InputCounter.Count`를 호출하지 않는다"가 과장이다 | `counter.Count(...)` 호출은 `gateway/counter.go:104` 한 곳뿐이고, 그 함수 `countInput`의 비-테스트 호출처는 `count_tokens.go:123`(명시적 `/v1/messages/count_tokens` 핸들러) 하나뿐이다. 생성·압축 경로에서 도달하지 않는다. **문서 주장 성립** |
| R4 | PACKAGING의 되돌리기 항목이 v0.3.1이 새로 저장하는 다른 필드를 빠뜨렸다 | `git diff -U0 -- go/ \| grep '^\+.*json:"'` 결과 새로 추가된 **영속 필드는 `customRole` 하나뿐**이다. `context_journal.go` 변경은 오류 처리 리팩터링이고 저장 구조 변경이 없다. `RequestClassRequired`/`RequestClassMissing`는 진단 응답 필드이지 세션 기록이 아니다. (필드 목록 외 *값* 정규화 문제는 A5-06으로 별도 보류) |
| R5 | ARCHITECTURE §4에서 "정책 범주 10개의 개방 여부는 사용자 결정으로 남아 있다"를 삭제한 것은 미해결 결정을 감춘 것이다 | `docs/v2/DECISION.md:215`가 이미 2026-09-15에 해결 처리했다(`~~정책 옵션 10개의 pass-through 여부~~ — **해결됨(2026-09-15)**`). v0.3.0 ARCHITECTURE 쪽이 stale이었고 이번 삭제가 그 불일치를 고친 것이다 |
| R6 | 실호출 검수기의 `integrationAuditBuffer`(tee 쓰기)와 `integrationBody.Close`(버퍼 전체 읽기)가 취소 시 data race를 일으킨다 | `messages.go:254`의 `defer response.Body.Close()`와 읽기 루프(`relay` 안 `messages.go:719`)는 **같은 goroutine**이다. `relay`가 반환한 뒤 deferred Close가 돈다. 경합 경로 없음 |
| R7 | ARCHITECTURE 6.1의 "최대 100ms·64KiB" 수치가 코드와 다르다 | `connection.go:16` `responseCloseGrace = 100 * time.Millisecond`, `connection.go:59` `io.CopyN(io.Discard, c.Conn, 64*1024)`. 일치. "sleep이 아니라 socket drain"이라는 서술도 `Close()` 내부에서 응답 write 이후에 수행된다는 점과 일치 |
| R8 | PARITY E4의 "자동 압축에만 medium 상한"이 실제로는 수동 압축에도 적용된다 | `context_compaction.go:32-39` `compactRoute(route, automatic)`은 `automatic`일 때만 high/xhigh/max → medium으로 낮추고 **값 복사본을 반환**한다. 호출처 2곳 모두 `receipt.trigger == "auto"` 조건(`context.go:279`, `context_compaction.go:93`). low/medium·수동 압축·이후 생성은 영향 없음 |
| R9 | COMPATIBILITY의 `requestClassRequired`/`requestClassMissing` 설명이 코드와 다르다 | `diagnostics.go:844-845` — `RequestClassRequired: g.contexts != nil`(= context policy 활성), `RequestClassMissing: refusedBy["CONTEXT_REQUEST_CLASS_UNVERIFIED"]`. 제품 런처는 `app/run.go:465`에서 무조건 `EnableContextPolicy()`를 건다. **문서 주장 성립** |
| R10 | PACKAGING의 "Clauduct도 같은 projects 트리에 metadata를 저장한다"가 근거 없다 | `app/run.go:301` `gw.ConfigureDelegations(filepath.Join(configDir, "projects"))`. 그 루트 아래에 선택 journal(`delegation.go:626,765`), context journal(`context_journal.go:33`), Workflow 체크포인트(`workflow_checkpoint.go:251-265`)가 기록된다. **주장 성립**. 삭제된 "이 wrapper는 자기 것을 어디에도 쓰지 않는다"가 사실과 달랐던 것을 고친 변경이다 |
| R11 | `compat-v030-probe_test.go`가 기준 commit에서 컴파일되지 않는다 | `preparedDelegation`은 `149068e:go/internal/gateway/delegation_test.go:156`에 존재. `route(scope, id, binding, contexts ...context.Context)`는 가변인자라 3인자 호출 가능, `choicePath(binding)`·`loadChoice(scope, id, binding)` 시그니처 일치. 컴파일 가능 |
| R12 | 문서 링크·인용이 깨졌다 | `node verification/test-doc-citations.mjs` exit 0 — `selfTests:12, linkTests:18, documents:128, citations:89, filesResolved:31, links:998, linksChecked:724, failures:0`. `v031-transport-20260922/evidence.json`의 기록(문서 128 / 로컬 링크 724 / 실패 0)과 정확히 일치. 담당 문서 5개 안에 `go/` 경로 line 인용은 0건이며(`src/…` 기준선 인용 5건뿐), README가 지웠던 `run.go:151` stale 인용은 이번 변경으로 제거됐다 |

---

## 5. 보류 항목과 결론 조건

| ID | 보류 내용 | 결론을 내리려면 |
|---|---|---|
| A5-05 | 기본 `TEMP`에서 실호출 검수기가 자기 private-path guard에 전부 걸리는가 | 기본 `TEMP`로 `CLAUDUCT_EVIDENCE_TUI_LOCAL=1` 로컬 PTY 1회 실행(backend 지출 0). native가 cwd를 요청 본문에 싣는지가 그때 확정된다 |
| A5-06 | 역할 이름 정규화가 세 번째 v0.3.0 거부 조건인가 | ① native가 내장 역할 이름을 표와 다른 철자로 보고하는 실제 사례 관측, ② v0.3.1이 쓴 journal을 기준 worktree reader에 먹이는 round-trip probe |

---

## 6. 개선 제안

- **I-1** `verification/v031-integration-20260921/compat-v030-probe_test.go`는 Go 모듈(`go/`) 밖에 있어
  이 트리의 어떤 검사도 컴파일·vet하지 않는다. 기준 worktree 내부가 바뀌면 조용히 썩는다.
  파일 머리에 "어느 commit의 worktree 어디에 복사해 어떤 명령으로 돌렸는지"를 못 박아 두면 재현 가능성이 유지된다.
  더 나은 방향은 A5-06 권장 ③처럼 **v0.3.1이 실제로 생성한 journal**을 입력으로 바꾸는 것이다.
- **I-2** `integration_tui_test.go`의 `oversized` 대조군은 의도한 2 MiB 누적 상한(`size > 2<<20`)이 아니라
  `bufio.Scanner`의 1 MiB 토큰 상한(`ErrTooLong`)에서 걸린다. 결과적으로 FAIL이라 표는 통과하지만
  `size > 2<<20` 분기는 15개 대조군 어디서도 실행되지 않는다.
  짧은 행을 많이 넣어 누적 상한만 넘기는 대조군을 하나 더 두면 둘 다 덮인다.
- **I-3** `recent_evicted` 대조군은 `assessTUIDiagnostics`가 `d.Recent`를 아예 읽지 않으므로 단독으로는 아무것도 단언하지 않는다.
  의미가 있는 것은 mutB에서 확인했듯 "Recent 기반 계수로 되돌릴 때 검출한다"는 회귀 보호뿐이다.
  그 의도를 주석으로 남기면 나중에 "쓸모없어 보이는 케이스"로 지워지지 않는다.
- **I-4** `integrationAuditBuffer.Write`는 첫 text delta를 만날 때까지 매 Write마다 버퍼 전체를 `bytes.Split`한다.
  첫 delta 이후 `onText = nil`로 꺼지므로 실측 영향은 작지만, 마지막 개행 이후 잔여분만 스캔하도록
  offset을 들고 있으면 O(n²)이 사라진다. 결함은 아니다.
- **I-5** `go/README.md`의 `-race` 항목이 특정 개발 머신 경로(`C:\msys64\ucrt64\bin\gcc.exe`)를 본문에 적고 있다.
  사실 기록으로는 정확하나 러너 보완 지침과 한 문단에 섞여 있어 "이 경로를 쓰라"로 읽힐 수 있다.
  관측 기록임이 더 분명해지도록 문장을 분리하는 편이 낫다.

---

## 7. 근거 부족으로 뺀 의심 (삭제하지 않고 남김)

- `integrationBody.Close`와 `onText` 콜백의 `t.Log`/`t.Error`가 테스트 종료 후 호출되면 panic한다.
  `Run()`이 gateway 종료를 기다린 뒤 반환하므로 in-flight 핸들러는 끝난 상태일 가능성이 높다.
  실제 관측 사례 없음. 실호출을 돌려봐야 알 수 있다.
- `assessTUIProject`의 `fs.Glob("projects/*/*.jsonl")`가 정확히 1개를 요구한다.
  TUI 검수는 `--tools ""`라 자식 transcript가 없지만, native가 부가 transcript를 만드는 조건이 있으면
  `TUI_TRANSCRIPT_AMBIGUOUS`로 오판할 수 있다. 그런 조건을 특정하지 못했다.
- `responseConn`이 HTTP/1.0 요청을 제외한다(`r.ProtoMinor >= 1`).
  native는 HTTP/1.1만 쓰고 문서도 "HTTP/1.1"로 한정해 적었으므로 현재 결함으로 볼 근거가 없다.
- COMPATIBILITY 최상단 transport 항목의 "PASS"와 바로 아래 close 항목의 "HOLD 유지"가 같은 날짜로 나란히 있어
  읽는 순서에 따라 모순처럼 보인다. transport 항목이
  "아래 이력의 독립 Node·.NET·raw TCP FAIL/HOLD를 PASS로 바꾸는 판정은 아니며"로 스스로 범위를 한정하므로
  실제 모순은 아니다. 가독성 문제로만 남긴다.
- 내장 code-review 스킬이 자기 레시피에 따라 4개의 `general-purpose` 서브에이전트를 추가로 띄운 것.
  이 워크플로의 "추가 서브에이전트 금지" 취지와 충돌해 보이지만, 스킬 내부 구현이라 내가 통제할 수 없고
  그 실행이 무엇을 했는지 결과가 도달하지 않아 판단 근거가 없다.

---

## 8. 실행한 명령 전체 목록 / NOT_RUN

### 실행 (전부 읽기 전용 또는 자기 repro 디렉터리 쓰기)

cwd 표기: `[R]` = `/d/AIDEV/clauduct-v031`, `[G]` = `/d/AIDEV/clauduct-v031/go`,
`[T]` = `/d/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A5-verification-compat/tempdir-guard`.
별도 표기 없으면 환경 변경 없음.

| # | 명령 | cwd | exit |
|---|---|---|---|
| 1 | `git status --porcelain=v1` / `git rev-parse HEAD` / `git diff --stat` | [R] | 0 |
| 2 | `git diff -- docs/v2/ARCHITECTURE.md` (COMPATIBILITY / PACKAGING / PARITY / go/README.md 각각) | [R] | 0 |
| 3 | `git diff --stat -- <담당 14개 경로>` | [R] | 0 |
| 4 | `git diff -U0 -- go/ \| grep -E '^\+.*json:"'` | [R] | 0 |
| 5 | `git diff -- go/internal/gateway/context_journal.go` | [R] | 0 |
| 6 | `git show 149068e:go/internal/gateway/delegation.go \| …` (reader·struct 대조) | [R] | 0 |
| 7 | `git grep -n "func preparedDelegation" 149068e -- go/internal/gateway/` | [R] | 0 |
| 8 | `git cat-file -e 149068e:<태그 파일 4종>` (존재 여부 판정) | [R] | 0 / 1 |
| 9 | `git status --porcelain -- go/internal/launch/` · `git diff --stat -- go/internal/launch/` | [R] | 0 |
| 10 | `cat` / `sed -n` / `grep -rn` 다수 (제품 소스·문서·evidence 읽기) | [R] | 0 |
| 11 | `sha256sum verification/test-http-transport.mjs verification/test-dotnet-http-transport.mjs go/internal/gateway/socket_runtime_evidence_test.go` | [R] | 0 |
| 12 | `tr -d '\r' < …socket_runtime_evidence_test.go \| sha256sum` (LF/CRLF 가설 배제) | [R] | 0 |
| 13 | `ls -la --time-style=full-iso …` (mtime 대조) | [R] | 0 |
| 14 | `node --version` | [R] | 0 |
| 15 | `node verification/test-doc-citations.mjs` | [R] | 0 |
| 16 | `cmd //c "echo %TEMP%"` | [R] | 0 |
| 17 | `CGO_ENABLED=0 go test -tags=runtime_evidence ./internal/app/ -run 'TestTUIAcceptanceRejectsMissingOrMisorderedEvidence\|TestIntegrationAuditSignalsOnlyTextStart' -count=1 -v -timeout 300s` | [G] | 0 |
| 18 | `CGO_ENABLED=0 go test ./internal/app/ -run '동일 정규식' -count=1 -v -timeout 300s` (태그 없이) | [G] | 0 (`no tests to run`) |
| 19 | `CGO_ENABLED=0 go test -tags=runtime_evidence -overlay=<repro>/mutA/overlay.json ./internal/app/ -run 'TestTUIAcceptance…' -count=1 -timeout 300s` | [G] | 0 |
| 20 | `CGO_ENABLED=0 go test -tags=runtime_evidence -overlay=<repro>/mutB/overlay.json ./internal/app/ -run 'TestTUIAcceptance…' -count=1 -timeout 300s` | [G] | 1 (기대한 FAIL) |
| 21 | `CGO_ENABLED=0 go test ./internal/gateway/ -run 'TestResponse\|TestConn\|TestClose\|TestDrain' -count=1 -v -timeout 300s` | [G] | 0 |
| 22 | `CGO_ENABLED=0 go run main.go` (tempdir guard 복제 프로그램) | [T] | 0 |
| 23 | `CGO_ENABLED=0 go build -o <repro>/clauduct-dev-probe.exe ./cmd/clauduct-dev` + `<repro>/clauduct-dev-probe.exe version` | [G] | 0 |

원본 수정이 필요한 재현은 전부 `go test -overlay`로 했다. 제품 파일을 고쳤다가 되돌린 적은 없다.

### NOT_RUN

| 검사 | 이유 | 추가 확인 방법 |
|---|---|---|
| `go test ./...` 전체 회귀 | 과제 지시로 금지. 통합 검토자가 필요성을 판단 | 통합 단계에서 `CGO_ENABLED=0 go test ./... -count=1` |
| `-race` (태그 포함/미포함) | 담당 변경에 동시성 코드가 없고 race가 꼭 필요한 좁은 대상이 없었다. 기존 보고서들이 전체 race PASS를 기록 | `CGO_ENABLED=1 CC=C:/msys64/ucrt64/bin/gcc.exe go test -tags=runtime_evidence -race ./internal/app -run 'TestTUIAcceptance…'` |
| `TestRuntimeEvidenceNativeRolesAndRestart`, `TestRuntimeEvidenceNativeTUI` | `CLAUDUCT_EVIDENCE_LIVE=1` + 실제 PTY + 구독 backend 지출이 필요. 이 리뷰에 그 권한·환경 없음 | 승인된 예산과 PTY에서 재실행 |
| `TestRuntimeEvidenceNativeTUILocal` | `CLAUDUCT_EVIDENCE_TUI_LOCAL=1` + 실제 PTY 필요(`Stdin: os.Stdin`). 비-PTY에서는 5분 timeout만 소모 | 실제 터미널에서 실행. A5-05 결론도 여기서 난다 |
| A5-06 round-trip probe (v0.3.1이 쓴 journal → v0.3.0 reader) | 기준 commit을 별도 worktree에 checkout해야 하는데 하드 금지(작업트리·index 보존, checkout 금지)를 우회하지 않았다 | 통합 검토자가 별도 task worktree에서 수행 |
| Node 31개 / .NET 35개 / raw TCP 400개 전송 검사 | A4 담당 범위. 본 라운드에서 재실행하지 않음 | A4 결과 참조 |
| `gofmt -l .`, `go vet ./...` | 담당 Go 파일 2개는 태그 파일이며 `go test -tags=runtime_evidence` 컴파일로 통과를 이미 확인. 별도 실행 안 함 | `go vet -tags=runtime_evidence ./internal/app` |
| `/code-review` 스킬 결과 수집 | 스킬이 fork 실행으로 종료됐으나 결과가 이 세션에 도달하지 않았다. 결과 없는 종료를 재실행 근거로 삼지 않았다 | 스킬 transcript를 직접 열람하거나 통합 검토자가 별도 수집 |
