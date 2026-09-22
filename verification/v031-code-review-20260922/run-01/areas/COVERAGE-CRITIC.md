# COVERAGE-CRITIC — v0.3.1 리뷰에서 빠진 것

- 대상 루트: `D:\AIDEV\clauduct-v031` (git worktree, branch `fix/v031`)
- 기준 commit: `149068edd693fb860a03244a2ea15764bcd68c34`. HEAD 동일, v0.3.1 전체가 미커밋 작업트리
- 역할: 커버리지 비판자. **새 제품 결함을 찾지 않는다.** 이번 라운드가 무엇을 보지 않았는지만 판정한다
- 쓰기: 이 파일 하나. 제품 소스·테스트·문서·기존 `verification/` 파일을 수정·생성·삭제하지 않았다.
  `git add|commit|stash|checkout|reset|restore|clean|rebase` 미사용, 읽기 전용 git 명령만 사용.
  훅·필터·설정·driver·설치본 변경 0건, push/PR/게시 0건, 작업트리 루트의 `%SystemDrive%/` 미접촉.

---

## 0. 요약 수치

| 항목 | 값 |
|---|---|
| manifest 파일 수 / 현재 `git diff` 파일 수 | 118 / 118 (+7895 / −636, 시작 스냅숏과 동일) |
| manifest sha256 대조 | **drift 0 / missing 0** (118개 전부 시작 시점 그대로) |
| 분야 배정 합계 | 22+33+23+26+14 = 118 (중복 배정 0, 누락 0) |
| **어느 분야도 검토하지 않은 파일** | **0개** |
| **"검토함" 주장만 있고 내용 언급이 0인 파일** | **10개** |
| NOT_RUN 기재 건수 | 42건 (A1 6 / A2 8 / A3 5 / A4 6 / A5 8 / INTEGRATION 9) → 중복 제거 **21건**, 이 중 해소 3건(1건 부분) |
| 배정 공백(두 분야 사이에서 아무도 안 본 계약) | 3건 (S1~S3) |
| 근거 없이 PASS/무해로 처리된 주장 | 7건 (P1~P7) |

**가장 무거운 셋**: G1(태그 게이트 증거 사슬 전체가 미실행·미감사), G2(릴리스 게이트가 한 번도 녹색이 아니고
유일한 잠금 경계 변경에 race 증거가 없음), G3(`messages.go:574-592` SDK/print 분기 — 변경과 그 회귀 검사가
서로 다른 분야로 갈라져 **양쪽 다 검토 0**). 자세한 내용은 §5.

---

## 1. 파일 커버리지

### 1.1 미검토 파일: 0개

manifest 118개가 5개 분야 커버리지 표에 정확히 한 번씩 등장한다(중복 0, 누락 0). 표 밖으로 새는 파일도 없다:
현재 작업트리의 `git diff --name-only`도 정확히 같은 118개이고, 미추적은 `%SystemDrive%/`(v1 shell 측정
잔재로 `docs/v1/README.md:242`·`docs/v1/reference/development-history.md:7`에 기록된 기존 물건)와 이번 리뷰
작업공간 두 개뿐이다. 즉 "리뷰 도중 생긴 변경을 아무도 안 봤다"는 형태의 누락은 없다.

### 1.2 "주장만 있음" 10개 — 표에 방식만 적혔고 내용 언급이 0

판정 기준: (a) 파일명이 자기 분야 보고서의 **커버리지 표 안에서만** 등장하고, (b) 표의 칸에 검증 가능한
구체값(행 범위, 수치, 테스트 이름, 대조 결과)이 없으며, (c) 명령표에도 그 파일을 개별적으로 지목한 실행이 없다.

| # | 파일 | 분야 | 표의 서술 | 실제로 남은 근거 |
|---|---|---|---|---|
| 1 | `go/internal/gateway/client_capability_test.go` | A3 | "신규, 전체 읽음 + 실행" | 실행은 A3 #9 정규식과 #14 패키지 전체 실행에 묻혀 있음. 신규 3개 테스트의 단언을 논한 문장 0 |
| 2 | `go/internal/gateway/compaction_runtime_evidence_test.go` | A3 | "신규, 전체 읽음. 실행은 NOT_RUN" | 신규 파일인데 내용 서술 0. 실행도 0 |
| 3 | `go/internal/gateway/context_journal_test.go` | A3 | "diff 읽음 + 실행" | 변경 함수 1개에 대한 언급 0 |
| 4 | `go/internal/gateway/context_test.go` | A3 | "diff + helper 전체 읽음 + 실행" | 변경 함수 1개(`TestContextPolicyRequiresClassAndNativeSessionEvidence`의 새 단언)에 대한 언급 0 |
| 5 | `go/internal/gateway/count_tokens_test.go` | A3 | "diff 읽음 + 실행" | 신규 `TestCountBudgetRefusalsKeepTheirCategory`에 대한 언급 0 |
| 6 | `go/internal/gateway/features_test.go` | A3 | "diff 읽음 + 실행" | A3-03이 `features.go`를 다루지만 그 대조 테스트 변경 3개는 언급 0 |
| 7 | `go/internal/gateway/messages_test.go` | A3 | "diff 읽음 + 실행" | 변경 함수 2개 언급 0. §5 G3과 직결 |
| 8 | `verification/v031-close-20260922/component-snapshot.txt` | A4 | "보고서 수치와 일치" | 어떤 수치가 일치하는지 값이 없음 |
| 9 | `verification/v031-close-20260922/shutdown-path.txt` | A4 | "callout 열거 형식 일관" | 형식 외 내용 대조 없음 |
| 10 | `verification/v031-close-20260922/UPSTREAM-ISSUE.md` | A4 | "개인 경로·자격증명 없음 확인" | 개인정보 점검만. 문서 주장과 코드·트레이스의 대조 없음 |

참고로 같은 "표에만 등장" 조건에 걸렸지만 **표 자체가 검증 가능한 값을 인용해 통과시킨 것 8개**가 있다
(`kernel-trace.json` "captured=17", `socket-trace.txt` "15/200·7/200", `evidence.json` "10 payload filters",
`driver-call-sites.txt` "정적 RVA만", `native_cancellation_test.go` "active/root·active/child-*",
`compaction_effort_test.go` "mutation 1건 재현", `app/context_test.go`·`context_resume_test.go` "native 검사 2·3개").
이 중 둘을 표본으로 직접 대조했고 둘 다 사실이었다(§6 #7, #8). 즉 A4 표는 신뢰할 만하고, 문제는 A3의 테스트
파일 행과 A4 close 묶음의 서술형 산출물 3개에 몰려 있다.

### 1.3 실행 커버리지 — 개별 주장과 실제 실행의 괴리

분야 명령표의 `-run` 정규식을 전부 뽑아(63개) v0.3.1이 **변경한 테스트 함수**와 대조했다. 결과:

- 변경된 테스트 함수 중 자기 분야의 좁은 실행에 걸리지 않은 것: `TestNativePrintPreservesStreamResult`(A1),
  `TestPromptSubmissionRegistersSessionWithoutForwardingPrompt`(A2, 패키지 실행으로 커버),
  `TestCanonicalRolePreservesNamesOutsideTheBuiltinTables`(A1, 패키지 실행으로 커버),
  `TestNativeProgressRejectsForeignAndIncompleteReceipts`·`TestUpstreamFailuresMapToStatuses…`(A3 #14 패키지 실행으로 커버),
  그리고 태그 파일 5종(§2).
- 즉 **untagged 테스트의 실행 커버리지는 결과적으로 100%**다. 단 그 근거는 분야가 아니라
  A3 #14(overlay로 gateway 패키지 전체)와 INTEGRATION의 `go test ./...` 1회다.
  `TestNativePrintPreservesStreamResult`는 **오직 INTEGRATION 전체 회귀에서만** 돌았고 `-v`가 없어
  그 실행이 무엇을 지나갔는지는 기록에 없다.
- `go/internal/app/native_print_test.go`는 v0.3.1 **신규 파일**인데 A1이 "역할·인자와 무관, 지적 없음"으로
  배제했다(A1 보고서 87행·437행). 배제 근거는 제시되지 않았다. §5 G3 참조.

---

## 2. NOT_RUN 통합표 (중복 제거 21건)

출처 표기: A1~A5는 각 분야 8절, INT는 INTEGRATION 5절. 상태는 이 라운드 종료 시점 기준.

| # | 실행되지 않은 검사 | 출처 | 상태 | 판정에 미치는 영향 |
|---|---|---|---|---|
| 1 | `go test ./...` 전체 회귀 | A1~A5 전원 | **부분 해소** — INT가 1회 실행, exit 1 | CI(`go.yml:62`)와 같은 명령이 빨간불. 실패(INT-01)의 원인이 제품인지 검사인지 미판정 → 배포 판정의 최대 미결 |
| 2 | `go test -race ./...` 전체 | INT | 미해소 | CI(`go.yml:69`)가 도는 게이트가 이 트리에서 한 번도 실행되지 않음 |
| 3 | `delegation.go route()`/`prepare()` 경로의 좁은 `-race` | A1(명시적 생략) | 미해소 | v0.3.1에서 **잠금 경계를 실제로 옮긴 유일한 변경**(`route()`의 Unlock 위치). A2 #29·A3 #18·A4 #20 race 정규식 어디에도 `TestDelegationConcurrentFirstRequestsKeepOneChoice`가 없다. A1-03의 `hasPending` 판정이 이 잠금 구간 안에 있다 |
| 4 | `TestRuntimeEvidenceAutomaticCompactionEffort` / `…LongCompactionEffort` | A3 | 미해소(과금) | 이슈 #56 상한 정책의 실호출 근거는 `v031-compaction-20260921/live.json` 기록뿐 |
| 5 | `TestRuntimeEvidenceNativeRolesAndRestart` / `…NativeTUI` | A5 | 미해소(과금+PTY) | 역할·재시작 종단 근거가 기존 기록 승계 |
| 6 | `TestRuntimeEvidenceNativeTUILocal` | A5 | 미해소(PTY) | A5-05 hold가 여기서만 풀린다 |
| 7 | `TestRuntimeEvidenceSocketClose` / `…SocketShutdown` | A4 | 미해소(env 스위치) | ARCHITECTURE §6.1(채택 정책)의 raw TCP 증거를 만든 검사. §5 G1 |
| 8 | `TestPublicMixedPDFCount` / `…Luna` | A3 | 미해소(과금) | tool-output 미디어 계수 범위 판단의 유일한 실호출 근거 |
| 9 | `-tags policy_evidence` 컴파일·vet | **아무도 기재조차 안 함** | **해소(내가 실행, exit 0)** | `internal/upstream`의 유일한 변경 파일이 이 라운드 전까지 컴파일된 적 없음. §5 G1 |
| 10 | C 진단기 3종(`socket-half-close.c`, `socket-shutdown-path.c`, `socket-shutdown-etw.c`) 빌드·ETW 실행 | A4, INT | 미해소 | 커널 경로 주장(KERNEL-ANALYSIS)이 기존 산출물 승계 |
| 11 | `verification/test-http-transport.mjs` 실행(Node 31) / .NET 35 / raw TCP 400 | A4, A5, INT | 미해소 | 보호 On/Off 대조 전부 기존 기록 승계. 해시 대조만 이번에 수행 |
| 12 | `mutations.json` 재실행 — 6묶음 56건 중 55건 | A3(9건), A1·A2(전량) | 미해소 | 기존 검증의 **검출력** 주장이 1.8%만 재현됨(A3 `automatic_cap_removed` 1건) |
| 13 | roles 묶음 12건 변이 재현 | A1-12 | 구조적 미해소 | `mutations.json`에 source/test가 없어 재현 자체가 불가. 이슈 #42·#45 판정의 검출력 근거가 감사 불가 |
| 14 | native 종단 재현 — A1-01/A1-03/A1-04, A2-01, A2-02, A2-13, A4-03 | A1, A2, A4 | 미해소 | high 3건(A1-01, A1-03, A2-01)이 전부 단위 재현까지만. A4-03은 실제 `Connection: close` 관측이 없어 hold 유지 |
| 15 | workflow 자식 요청의 실제 request class 관측(A1-22) | A1, INT | 대체 판정 | INT가 바이너리 추적 + 실세션 감사 110건으로 전제는 확보. **순서 의존 잔여는 그대로** |
| 16 | native가 보고하는 내장 역할 철자 관측(A1-21/A1-04) | A1 | 대체 판정 | 감사 기록에 canonical만 존재 → A1-04 강등 근거. 반례 부재 증명은 아님 |
| 17 | A5-06 round-trip probe(v0.3.1 journal → v0.3.0 reader) | A5, INT | 미해소(코드로 반박) | 되돌리기 문서의 완전성이 실행이 아니라 코드 독해로 판정됨 |
| 18 | `TestNativeEventPluginPassesInstalledValidator` / `…NoNodeOnChildPATH` | A2 | **해소** — INT 전체 회귀에 포함(`internal/app` ok 263s) | 단 개별 PASS 기록은 없음. A2가 "events 묶음 보고서에 PASS 기록" 을 근거로 넘긴 것은 전문(傳聞)이었다(P2) |
| 19 | INT-01의 원인 판정 | INT | 미해소 | 3회 시도 후 중단. 배포 판정의 최대 미결(#1과 한 묶음) |
| 20 | `/code-review` 스킬 결과 수집 | A4, A5, INT | 미해소(프로세스) | 두 분야에서 스킬 결과가 세션에 도달하지 않았고 재실행하지 않음. 해당 분야의 "스킬 커버" 열은 근거 없음 |
| 21 | `gofmt -l .` 전체 트리 | A5, (A4는 2개 디렉터리만) | 미해소(경미) | 형식 게이트. CI가 별도로 돌리지 않음 |

---

## 3. 분야 배정 자체의 공백

### S1 — `messages.go:574-592`: 변경과 그 회귀 검사가 서로 다른 분야로 갈라져 양쪽 다 검토 0

- v0.3.1은 `relay()`의 텍스트 지연 조건을 `RequestClass == "workflow"` 하나에서
  `record.ParentReadiness.ControlMode == "sdk"`까지 넓혔다(`DeferTextUntilComplete`).
  즉 **SDK/print 세션의 응답 조립 순서가 바뀌었다.**
- 이 훅은 A3(`messages.go` 담당)의 파일에 있지만, A3 커버리지 표가 선언한 독서 범위는
  `1–260 / 270–345 / 800–930`이다. **574–592는 그 밖이다.**
- `DeferTextUntilComplete`, `ControlMode`, `ParentReadiness` — 세 식별자 모두
  **6개 보고서(A1~A5+INTEGRATION) 어디에도 0회 등장**한다.
- 대응하는 회귀 검사로 유력한 신규 파일 `go/internal/app/native_print_test.go`
  (`-p` print 모드 + `text` / `text_and_reasoning` 두 하위검사)는 A1에 배정됐고,
  A1은 "역할·인자와 무관, 지적 없음"으로 배제하고 실행하지 않았다.
- 결과: 동작 변경도, 그 변경을 지키는 유일한 검사도 아무도 읽지 않았다. 도달 조건(native hook이
  `waitControlMode:"sdk"`를 보고하는 경로, `parent_wait.go:89`)이 print 모드에서 실제로 성립하는지,
  그래서 그 신규 테스트가 이 분기를 지나가는지는 **아무도 판정하지 않았다**.
- 확인 방법: `CGO_ENABLED=0 go test ./internal/app/ -run TestNativePrintPreservesStreamResult -count=1 -v -timeout 600s`
  와 `go test ./internal/gateway/ -run 'TestTextRoundTrip|TestByteAtATime' -count=1 -v` 를 대조하고,
  `messages.go:584` 분기에 도달 카운터를 overlay로 심어 print 세션에서 증가하는지 본다.

### S2 — 태그 증거 사슬: A4는 코드만, A5는 해시만, 둘을 합친 사람은 없다

`go/internal/gateway/socket_runtime_evidence_test.go`(A4) → 그 실행 산출물
`verification/v031-close-20260922/socket-trace.txt`(A4) → 보호 On/Off 대조 기록
`verification/v031-recheck-20260922/evidence.json`(A5) → 채택 정책 `docs/v2/ARCHITECTURE.md` §6.1(A5)
가 한 줄로 이어진다. 그런데

- A4는 그 테스트 파일을 "전체 읽음 + `go vet -tags runtime_evidence` PASS"로 처리하고 실행도 해시 대조도 하지 않았다,
- A5는 해시 불일치를 찾았지만(A5-01: 기록 `fdcb21ed…` vs 실제 `7dc3741c…`, 내가 재확인) 그것이
  **§6.1 근거의 감사 가능성**에 무엇을 뜻하는지까지는 연결하지 않았고,
- INTEGRATION은 A5-01을 "검증 기록 무결성 결함, 제품 결함 아님"으로 유지하며 §6.1과 묶지 않았다.

세 사람 모두 자기 칸에서는 맞다. 합치면 **"보호 On 상태의 제품 실제 동작"이라는 채택 기준의 기계 근거가
이번 라운드에서 재현 불가이고, 그 코드가 시험 당시와 같다는 기록도 틀렸다**가 된다. 이 결론을 낸 사람이 없다.

### S3 — 신규 `bridge.KnownRole`의 소비처 2곳이 소비 분야에서 미검토

`KnownRole`은 v0.3.1 신규 API다(기준 commit의 `route.go`에 없음). 소비처 3곳 중
`delegation.go:616`(A1)만 A1-06 맥락에서 다뤄졌고, `messages.go:339`(A3, `unroutedRoles` 카운터)와
`selection_records.go:92`(A2, `"unlisted"` 라벨)는 해당 분야 보고서에 **0회** 등장한다
(보고서 전체에서 `KnownRole`은 A1에 2회뿐). 두 곳 모두 진단·라벨 경로라 영향은 낮고,
`selection_records.go`의 치환은 기존 식(`RoleRoute||InheritsParent`)과 동치임을 내가 diff로 확인했다.
그래도 **새 계약을 그 계약의 소비 파일 담당자가 본 적은 없다**는 사실은 남는다.

### (공백이 아니라 중복인 것, 기록만)

`app/settings.go`(A1) ↔ `cmd/clauduct-hook/main.go`(A2)의 UserPromptSubmit 차단은 두 분야가 각각 절반씩
제안해 INT가 M3로 병합했다. 문서 신규 절 ↔ 코드 분리(ARCHITECTURE §4/§6.1/§7.1)는 INTEGRATION §6-5가
이미 같은 진단을 냈다. 여기서는 중복 서술하지 않는다.

---

## 4. 근거 없이 PASS / 무해로 처리된 주장

| # | 주장 | 어디 | 왜 근거가 없나 |
|---|---|---|---|
| P1 | "`native_print_test.go`는 역할·인자와 무관한 print 회귀 검사, 결함 없음" | A1 87행·437행 | 신규 파일을 읽었다는 서술뿐. 실행 0회, 어느 분야가 봐야 하는지 재배정 제안 0. 실제로는 S1의 미검토 동작 변경과 같은 주제다 |
| P2 | native event 검사 2건은 "events 묶음 보고서에 PASS 기록이 있음"이라 넘김 | A2 NOT_RUN | 타인 기록을 자기 PASS로 승계했다. 결과적으로 INT 전체 회귀가 덮었지만 그 사실은 A2가 알 수 없었다 |
| P3 | mixed PDF 검사 변경은 "단언 약화 없음(정적 확인)" | A3 NOT_RUN | 그 파일은 `policy_evidence && windows` 태그라 **컴파일된 적이 없었다**. 컴파일 가능 여부를 이번에 내가 처음 확인했다(exit 0). 태그명도 보고서에 `runtime_evidence`처럼 뭉뚱그려져 있다 |
| P4 | close 묶음 산출물 3종을 "전체 읽음 / 형식 일관 / 수치 일치"로 통과 | A4 표 19·23·18행 | 인용된 값이 없어 재검증 불가(§1.2) |
| P5 | "기존 검사 재검증 결과: 담당 범위의 제품 테스트는 전부 PASS" | A1 8절 | 맞지만, 그 PASS가 **변이 검출력**을 뜻하지 않는다. A1은 두 묶음 변이 19건 중 0건을 재실행했고 그중 12건은 재현 불가(A1-12)임을 스스로 적었다 |
| P6 | "반박이 부실했던 것: 없음. 반박 43건을 **표본** 점검했고 뒤집을 근거를 찾지 못했다" | INTEGRATION §4.5 | 표본 크기가 없다. 표본 점검 결과를 전수 판정처럼 쓰는 문장이다 |
| P7 | 각 분야 "미검토 없음" 선언 | A1~A5 표 말미 | §1.2의 10개가 반례다. 방식 칸이 곧 근거로 계산되고 있다 |

---

## 5. 가장 중요한 공백 셋

### G1 — 태그로 가려진 증거 사슬 전체가 미실행이고, 그중 하나는 기록과 코드가 어긋난다

v0.3.1은 태그 파일을 5개 건드렸다: `runtime_evidence` 4개
(`app/integration_runtime_evidence_test.go`, `app/integration_tui_test.go`,
`gateway/compaction_runtime_evidence_test.go`, `gateway/socket_runtime_evidence_test.go`)와
`policy_evidence && windows` 1개(`upstream/mixed_pdf_evidence_test.go`).

- 이 5개 파일의 **테스트는 이번 라운드에서 0회 실행**됐다(NOT_RUN #4~#8).
- CI는 `-tags`를 전혀 쓰지 않는다(`go.yml` 56·59·62·69행) — A5-03·INT가 확인한 그대로다.
- INTEGRATION의 `go vet -tags runtime_evidence ./...`는 4개를 컴파일했지만 **`policy_evidence`는 아무도
  컴파일하지 않았다.** 내가 `go vet -tags policy_evidence ./internal/upstream/`를 돌려 exit 0을 얻었다(§6 #4).
  즉 `internal/upstream`의 유일한 v0.3.1 변경은 이 라운드 전까지 타입 검사조차 통과한 적이 없었다.
- 그리고 넷 중 하나(`socket_runtime_evidence_test.go`)는 보호 On/Off 대조 기록이 주장하는 sha256과
  실제 파일이 다르다(A5-01, 내가 재확인). 이 파일이 만든 수치가 `v031-close-20260922/socket-trace.txt`와
  `v031-recheck-20260922/evidence.json`의 raw TCP 200회 통계이고, 그것이 ARCHITECTURE §6.1 채택 정책의 근거다.

**결론**: 채택 정책의 실측 근거 계층은 (a) 이번에 재현되지 않았고, (b) CI가 앞으로도 잡지 않으며,
(c) 최소 한 건은 "같은 코드였다"는 기록이 사실과 다르다. 배포 전 최소한 태그 컴파일을 CI에 넣고
(`go vet -tags runtime_evidence ./...`, `go vet -tags policy_evidence ./internal/upstream/`),
socket 증거는 재실행하거나 기록의 해시를 정정해야 한다.

### G2 — 릴리스 게이트가 한 번도 녹색이 아니고, 유일한 잠금 경계 변경에는 race 증거가 없다

- `go test ./...`: 5개 분야 전원 NOT_RUN → INTEGRATION 1회 실행 → **exit 1**, 원인 미판정(INT-01).
- `go test -race ./...`: **0회**. CI가 도는 두 게이트(`go.yml:62`, `:69`) 중 하나는 빨간불, 하나는 미실행이다.
- 좁은 race는 A2 #29 / A3 #18 / A4 #20 셋뿐이고, 세 정규식 어디에도 delegation 경로가 없다.
  그런데 v0.3.1에서 **실제로 잠금 구간을 옮긴 변경은 `delegation.go route()`의 Unlock 위치 하나**이고
  (A1이 "새 동시성 구조 변경 없음"으로 race를 생략했다), A1-03(high)의 `hasPending` 판정이 바로 그 구간에 있다.
  전용 동시성 검사 `TestDelegationConcurrentFirstRequestsKeepOneChoice`는 어떤 race 실행에도 포함되지 않았다.
- 최소 추가: `CGO_ENABLED=1 CC=C:/msys64/ucrt64/bin/gcc.exe go test ./internal/gateway/ -race -run 'TestDelegation|TestWorkflowSelection|TestNativeChoice' -count=1 -timeout 900s`
  와, INT-01 수정 후 `go test ./...` 1회 녹색.

### G3 — `messages.go:574-592`(SDK/print 텍스트 지연): 변경도 검사도 무주공산

§3 S1 전문. 요약하면 동작이 바뀐 줄은 담당 분야의 선언된 독서 범위 밖이고, 그 동작을 지키는 신규 테스트는
다른 분야가 "무관"으로 배제했으며, 관련 식별자 3개가 6개 보고서에 0회 등장한다. 이번 라운드에서 **누구도
읽지 않은 v0.3.1 동작 변경**은 내가 찾은 범위에서 이것 하나다(변경 hunk 전수를 각 분야가 선언한 독서 범위와
대조한 결과 — §6 #9).

---

## 6. 내가 실행한 검사 전부

작업 디렉터리 표기: `[R]` = `/d/AIDEV/clauduct-v031`, `[G]` = `/d/AIDEV/clauduct-v031/go`.
별도 표기가 없으면 환경 변경 없음. 전부 읽기 전용이거나 임시 스크립트(`/tmp`)만 생성한다.

| # | 명령 | cwd / 환경 | 예상 | 실제 | exit |
|---|---|---|---|---|---|
| 1 | `git rev-parse HEAD`, `git diff --stat`, `git status --porcelain=v1` | [R] | 기준 commit, 118 파일 | `149068e…`, 118 files +7895/−636, 미추적 2(`%SystemDrive%/`, 리뷰 작업공간) | 0 |
| 2 | node 스크립트로 manifest 118개 sha256 재계산·대조 | [R] | 전부 일치 | `drift=0 missing=0 total=118` | 0 |
| 3 | `git diff --numstat` / `-U0` (제품 소스 37개, 테스트 40개) | [R] | 변경 행 범위 파악 | §1.3·§5 G3 근거 | 0 |
| 4 | `CGO_ENABLED=0 go vet -tags policy_evidence ./internal/upstream/` | [G] / `CGO_ENABLED=0` | 컴파일 여부 판정 | **exit 0 — 컴파일 통과**(이 라운드 최초) | 0 |
| 5 | `go list -tags policy_evidence -f '{{.TestGoFiles}}' ./internal/upstream/` | [G] | 태그 파일 포함 확인 | `mixed_pdf_evidence_test.go` 포함 | 0 |
| 6 | `sha256sum go/internal/gateway/socket_runtime_evidence_test.go` + evidence 기록 대조 | [R] | A5-01 검증 | 실제 `7dc3741c…` vs 기록 `fdcb21ed…` → **불일치 확인** | 0 |
| 7 | `sha256sum verification/test-http-transport.mjs` (+ CRLF 제거 변형) | [R] | A4-R6 검증 | `304098b8…` = 기록과 **일치** | 0 |
| 8 | `grep`로 `kernel-trace.json captured=17`, `socket-trace.txt 15/200·7/200`, `evidence.json filter_setup_check` 대조 | [R] | A4 표 수치 검증 | 셋 다 일치 | 0 |
| 9 | node 스크립트: 커버리지 표 파싱 → 파일별 표 안/밖 언급 수, `-run` 정규식 63개 ↔ 변경 테스트 함수 매칭, 변경 hunk ↔ 선언된 독서 범위 대조 | [R] | 미검토·주장만 판정 | §1.2·§1.3·§5 G3 | 0 |
| 10 | 6개 `mutations.json` 항목 수 집계 | [R] | 재현율 산출 | 7/10/11/8/12/8 = **56건**, 재실행 1건 | 0 |
| 11 | `grep -c` 로 보고서 6종에서 `DeferTextUntilComplete` / `ControlMode` / `ParentReadiness` / `KnownRole` 검색 | [R] | 언급 유무 | 앞 셋 **0회**, `KnownRole`은 A1만 2회 | 0/1 |
| 12 | `grep`로 `.github/workflows/go.yml`의 vet/build/test/race 단계 확인 | [R] | `-tags` 유무 | 4단계 전부 `-tags` 없음 | 0 |
| 13 | 제품 소스 확인 읽기: `projects.go`, `context_journal.go`, `delegation.go`(roleMatches/metadataModelMatches), `route.go`(KnownRole), `messages.go:574-592`, `parent_wait.go:89`, `native_print_test.go` | [R] | 배정 공백 판정 | §3 | 0 |

### 내가 실행하지 않은 것 (NOT_RUN)

| 검사 | 이유 | 추가 확인 방법 |
|---|---|---|
| `go test ./...` / `-race ./...` 재실행 | 같은 단위를 두 번 돌리지 않는다. 전체 회귀는 INTEGRATION이 1회 실행했고 race는 이 라운드 범위 밖의 비용 | G2의 두 명령 |
| `TestNativePrintPreservesStreamResult` 단독 실행 | 설치본 native를 구동하는 검사이고, 내 과제는 미실행 사실의 확인이지 분기 도달 판정이 아니다. INT 전체 회귀에서 1회 통과했으나 `-v`가 없어 분기 도달은 알 수 없다 | S1의 확인 방법 |
| `messages.go:584` 분기 도달 probe | 새 결함 탐색은 내 범위 밖 | overlay 카운터 + print 세션 1회 |
| 태그·env 게이트 검사 실제 실행(#4~#8, #10, #11) | 과금·PTY·ETW 세션·호스트 상태 변경을 수반 | 각 분야 8절에 기재된 명령 |
| `mutations.json` 55건 재실행 | 커버리지 판정에는 미재현 사실만으로 충분 | 각 `.json` overlay + 기록된 `-run` |
| `%SystemDrive%/` 내용 확인 | 하드 금지(미접촉). v1 문서에 기존 잔재로 기록돼 있어 v0.3.1 산출물이 아님을 문서로 확인 | 사용자 승인 후 별도 처리 |

---

## 7. 권고 (커버리지 관점, 짧게)

1. 배포 전 필수: `go test ./...` 1회 녹색(INT-01 원인 판정 포함) + `go test -race ./...` 1회.
   race에 delegation 경로를 반드시 포함한다(G2).
2. CI에 태그 컴파일 2줄 추가(`runtime_evidence`, `policy_evidence`). 지금은 깨져도 아무도 모른다(G1, A5-03).
3. `socket_runtime_evidence_test.go`의 기록 해시를 정정하거나 증거를 재수집한다. §6.1 채택 정책의 근거다(G1, S2).
4. `messages.go:574-592`의 SDK/print 지연과 `native_print_test.go`를 한 사람에게 함께 배정해 다시 본다(G3).
5. 다음 라운드 커버리지 표 규칙: 칸에 **검증 가능한 값**(행 범위·수치·테스트 이름·대조 결과)을 반드시 적는다.
   "diff 읽음 + 실행"은 근거가 아니다(§1.2, P7). 선언한 독서 범위 밖에 변경 hunk가 있으면 그 자체가 미검토다.
