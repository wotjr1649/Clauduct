**Claude v0.3.1 리뷰에 대한 Codex 평가 — 2026-09-22**

판정은 **CHANGES_REQUIRED 유지, 배포 보류**다. 새 turn의 상태·결과를 이전 요청이
덮어쓰는 문제와 압축 기록 저장 실패 뒤 복구 불능을 현재 소스에서 재현했다.
다만 Claude의 confirmed 18개를 독립된 제품 결함 18개로 받아들여 일괄 수정하면 안 된다.
같은 원인의 중복, 검수 공백, 기존 설계와 충돌하는 권고, 근거보다 강한 결론이 섞여 있다.

이번 작업은 세션·문서 추적, 코드 대조, 작은 로컬 재현, 후속 작업 제안이다.
제품 소스와 Claude의 원본 보고서는 변경하지 않았다. 새 산출물은 이 문서와
`codex-assessment/`의 로그·provenance뿐이다. 실제 backend·TUI·전체 회귀·전체 race는
이번 평가에서 재실행하지 않았고, 보호 설정·사용자 profile·설치본도 변경하지 않았다.

**세션과 산출물의 연결**

| 항목 | 독립 확인 결과 |
|---|---|
| 지정 세션 | `a0616359-5dc7-45d4-9eb3-fd1b761c4af7` |
| 로컬 transcript | `C:\Users\js\.claude\projects\D--AIDEV-clauduct-v031\a0616359-5dc7-45d4-9eb3-fd1b761c4af7.jsonl` |
| 기록 시간 | 2026-09-22 13:42:55–16:06:18 KST |
| 실제 대상 | `D:\AIDEV\clauduct-v031`, branch `fix/v031` |
| 기준 | `149068edd693fb860a03244a2ea15764bcd68c34` / v0.3.0 |
| Workflow | `wf_61ab1ba6-494`, task `w6rmdoesi`, `completed`, 43/43 `done` |
| 실행 모델 | main 및 Workflow 기록은 `claude-opus-5` |
| 리뷰 호출 | 분야별 `/code-review`에 `xhigh` 전달 5회 확인. 실제 적용 effort는 미확인 |
| 최종 보고서 | 이 디렉터리의 `REPORT.md`; 지정 세션에서 Write 1회·Edit 1회 확인 |
| 소스 무결성 | manifest 118개: 시작→종료 변경 0, 종료→이번 평가 변경 0. 현재 diff 118개와 일치 |

Workflow 진행 기록의 phase별 agent 수는 Review 5 / Verify 36 / Integrate 2다.
원본의 Verify 32는 계획한 검증 수와 실제 진행 수를 구분하여 읽어야 한다.
43개 모두 완료됐다는 최종 상태는 확인됐다. transcript는 로컬에서 필요한 metadata만
추출했으며 원문을 평가 산출물에 복사하지 않았다. 집계는 `codex-assessment/provenance.json`에 있다.

문서 추적 순서는 `REPORT.md` → `areas/A1-roles-selection-args.md`,
`areas/A2-session-turn-recovery.md`, `areas/A3-compaction-usage.md`,
`areas/A4-transport-process.md`, `areas/A5-verification-compat.md` →
`areas/INTEGRATION.md`, `areas/COVERAGE-CRITIC.md` → 각 `repro/` 증거다.
원본의 최종 집계는 confirmed 18 / hold 5다. 118개 파일의 배정 완료가 모든 변경의
동일한 검토 깊이를 뜻하지는 않으며, coverage critic이 남긴 검토 공백도 함께 읽었다.

**직접 실행한 재현과 그 의미**

Go 1.27.1, `CGO_ENABLED=0`, 각 명령에 `-count=1 -timeout=60s -v`를 사용했다.
overlay는 리뷰가 만든 테스트 파일만 추가한다. 제품 구현 대체나 실제 backend 호출은 없다.
HTTP 검사는 loopback gateway와 합성 upstream 입력을 사용하며, 상태 전이 자체는 실제
gateway 구현에서 실행된다. 보호 On은 사용자의 마지막 확인 상태이며 이번에 UI를 다시
측정한 것은 아니다.

| 실행 / 로그 | 관측 | 판단 |
|---|---|---|
| A2 두 검사 / `codex-assessment/A2-recheck.txt` | 둘 다 FAIL, exit 1. 이전 실패가 `second`를 `first`로 되돌리고 본문·종료 증거를 지움. 이전 완료가 새 결과를 `PREVIOUS TURN STOP PAYLOAD`로 완료함 | A2-01·02의 상태 전이 결함 재현. 정상 동작 assertion 실패이며 검증 실패를 숨기지 않음 |
| A3 세 검사 / `codex-assessment/A3-recheck.txt` | 생성 200 vs 선택적 count 400; 저장 장애 해소 후에도 `compacting` 고착; 거부된 Workflow 요청이 workflow 통계에서 0건 | A3-01·02·03 재현. 이 probe들은 결함 상태를 기대하므로 PASS가 제품 정상 판정은 아님 |
| A1 gateway 두 probe / `codex-assessment/A1-recheck.txt` | sibling 없을 때 Workflow 선택 성공, sibling 있으면 `AGENT_SELECTION_UNVERIFIED`; projects 루트 부재와 기록만 부재의 차이 | A1-03 분기 결함 확인. A1-06은 아래 설계 근거와 대조해야 함 |
| A1 app 세 probe / `codex-assessment/A1-app-recheck.txt` | 읽힌 `reviewer` 보존, 미확정 이름 거부; `-cp`·`-pv`·`-dapi`와 settings 조합 거부; plugin의 미확정 `p:reviewer`를 오류 없이 부재로 처리 | A1-01의 결론 범위 제한, A1-02·05 현상 확인 |
| 기존 projects 회귀 두 검사 / `codex-assessment/projects-policy-recheck.txt` | 루트 부재·파일·접근 실패 및 정상 루트 대조군 모두 PASS, backend 0회 | A1-06 권고가 기존 설계를 되돌린다는 근거 |

합계는 최상위 검사 12개: 10 PASS / 2 FAIL이다. PASS 중 관측·결함 재현용 probe를
제품 회귀 PASS와 혼동하지 않는다. A2 검사는 의도한 상태 순서를 구성한 재현이며,
이번 평가에서 실제 native 세션의 동일 interleaving까지 관측한 것은 아니다.

재실행 시 cwd는 `D:\AIDEV\clauduct-v031\go`다. 아래 경로는 모두
`../verification/v031-code-review-20260922/run-01/` 기준이다.

```powershell
$env:CGO_ENABLED='0'
$env:CLAUDUCT_FAKE_CHILD=''
$review='../verification/v031-code-review-20260922/run-01'
go test "-overlay=$review/repro/A2-session-turn-recovery/overlay.json" -run '^TestA2(StalePinnedTurnDestroysNewerAwaitingChildren|OrphanedCompletionSettlesTheNewTurn)$' -count=1 -timeout=60s -v ./internal/gateway
go test "-overlay=$review/repro/A3-compaction-usage/overlay.json" -run '^TestA3(CountRefusesAuxiliaryCompactionTextThatGenerationRuns|JournalFailureDuringCompactionWedgesTheSession|RefusedWorkflowChildDropsOutOfWorkflowFeatureEvidence)$' -count=1 -timeout=60s -v ./internal/gateway
go test "-overlay=$review/repro/A1-roles-selection-args/overlay-gateway.json" -run '^(TestProbeWorkflowChildDivertedByPendingSibling|TestProbeAbsentProjectsTreeLoadChoice)$' -count=1 -timeout=60s -v ./internal/gateway
go test "-overlay=$review/repro/A1-roles-selection-args/overlay-app.json" -run '^TestProbe(StrayNoteRefusesUnmatchedRoles|CombinedShortOptionsAndSettings|UnreadablePluginNameIsSilentlyAbsent)$' -count=1 -timeout=60s -v ./internal/app
go test -run '^TestProjects(AbsenceDiffersFromFileAndSharingFailure|FailureDoesNotCountAsAnUnroutedRole)$' -count=1 -timeout=60s -v ./internal/gateway
```

**confirmed 18개에 대한 처리 제안**

| ID | Codex 판단 | 처리 |
|---|---|---|
| A2-01 | 수정 필요. 늦은 실패가 새 turn을 교체할 수 있음 | `recordFailedAgentRequest`에서 현재 result/turn의 소유권을 검증한 뒤 상태 변경. 실패 정리에서 이전 turn을 새로 시작하지 않도록 함 |
| A2-02 | 수정 필요. 분리된 result의 완료 callback이 현재 result를 종료함 | `beginAnswer` 완료를 원래 result/turn에 묶고 stale completion이 새 결과·본문·byte 누계를 바꾸지 않도록 함 |
| A3-02 | 수정 필요. 기존 결함이지만 프로세스 유지 요구에 직접 영향 | compact 진입의 저장 실패가 `compacting`에 고착되지 않게 상태·ticket 처리 정합성 복구. 장애 해소 뒤 명시적 manual compact 성공 검사 |
| A1-03 | 분기 결함 재현. 실제 native 첫 요청 순서는 추가 관측 | 형제 전체의 pending 여부로 해당 Workflow 자식의 경로를 결정하지 않음. 자식 식별·검증된 Workflow 증거 유지 |
| A3-03 | 진단 누락 재현 | 선택 확정 전 거부도 검증된 Workflow 소속 근거로 집계. 단순 역할 문자열 신뢰로 되돌리지 않음 |
| INT-02 | A1-03 + A3-03의 결합 결과 | 별도 제품 결함으로 중복 계산하지 않고 위 둘과 같은 묶음에서 수정·검증 |
| A3-01 | 생성과 선택적 count의 요청 분류 불일치 재현 | 두 경로의 compact 판정을 일치시킴. 일반 생성에 사전 계수를 추가하지 않음 |
| A1-02 | short option 조합의 호환성 공백 확인 | 공유 argv 경계 분석을 native 의미와 맞추고 settings·role 스캔을 함께 검사. 보호 옵션 거부와 모호한 값 경계는 유지 |
| INT-05 | A1-02의 지원 문서 문제 | 같은 수정에서 실제 지원 범위와 문서 일치 |
| A1-05 | 미확정 plugin 이름을 부재로 단정하는 비대칭 확인 | 불확실성이 속한 plugin namespace에 적용하고 정상 정의·우선순위 보존. unrelated 역할 전체 거부로 확대하지 않음 |
| A1-01 | 무조건 high 회귀로 채택하지 않음 | #42의 정상 정의 보존·미확정 이름 거부와 현재 재현은 정합. 완전히 읽은 잘못된 frontmatter를 native가 무시한다는 증거와, 명시 모델이 있어도 사용자 역할 식별이 필요한 이유를 먼저 대조 |
| A1-06 | 현재 설계상 결함이라는 판정 반박 | projects 루트 실패를 역할 부재로 취급하지 않는 기존 회귀가 명시적으로 존재. `choiceAbsent` fallback 복구 권고는 채택하지 않음 |
| A4-01 | 메서드 집합 변화와 500ms 종료 경로는 확인. 제안 수정은 검증 불충분 | 완전한 HTTP 오류 응답 전달, native의 오류 인식·후속 요청, 실제 종료 실패를 함께 확인한 뒤 수정 위치 결정 |
| INT-01 | 검사 진단 결함 확인; 간헐적 전송 실패 원인은 미확정 | SSE 파싱 전에 read/close 오류·상태·수신 byte·iteration을 보존. timeout 증가나 재실행 PASS만으로 종결하지 않음 |
| A5-03 | CI 검증 공백, 현재 제품 고장 증명은 아님 | 태그 파일 compile/vet와 네트워크 없는 검수 순수 로직을 CI에서 검사. 실제 backend·PTY 검사는 별도 명시 실행 유지 |
| A5-04 | 취소 누계 assertion의 판별력 공백 | 누적 CANCELLED가 빠진 대조군을 추가하여 해당 assertion 제거 mutation이 실패하는지 확인 |
| A5-01 | 과거 실행 증거와 현재 파일의 판본 연결 부족 | 과거 hash를 현재 hash로 덮지 않음. 당시 snapshot/patch를 추적하고 시점·현재와의 차이·재현 제한을 추가. 불일치만으로 당시 On/Off 결과 무효를 단정하지 않음 |
| INT-04 | 지적의 문서 인용 전제 반박 | 실제 ARCHITECTURE §6.1은 framing flush 후 drain을 설명하며 half-close를 약속하지 않음. 오류 경로의 기존 계약 유지 문구는 A4-01 판정 후 필요하면 별도로 좁힘 |

A1-06의 근거는 `verification/v031-projects-20260921/REPORT.md`와
`go/internal/gateway/projects_root_test.go`다. 루트가 실제 없는 경우도 선택 복원에서 거부하고,
정상 루트에 저널만 없을 때 역할을 구분한다. 이를 단순 회귀로 되돌리면 #48 수정의 검사를 깨뜨린다.

A1-01은 사용자의 #42 결정을 우선한다. 손상 파일이 어떤 이름을 선언했는지 모르면
내장 이름도 사용자 정의에 의해 덮였는지 불확실할 수 있다. 이번 probe에서도 정상 정의는
보존됐다. `roleDefaults` 호출을 명시 모델 요청에서 무조건 제거하는 것은 사용자 정의 역할
식별까지 제거할 수 있어 별도 검토 없이 채택하지 않는다.

A3-01은 선택적 count endpoint의 요청 분류 불일치다. 일반 생성·압축의 사전 계수를
복원하는 수정이 아니다. `docs/v2/ARCHITECTURE.md` §7.1의 실제 backend usage 정책과
#50의 관측 전 추정 유지, #56의 자동 compact만 medium 상한을 그대로 유지한다.

**통신 문제에서 현재 결론과 부족한 증거**

`responseConn`이 `CloseWrite`를 노출하지 않아 `net/http`의 선택적 half-close 경로가
달라지는 것은 코드로 확인된다. 그러나 원본 보고서의 “위임 추가 후 제품 경로 전부
즉시 EOF”라는 결론은 인용한 증거 전체와 맞지 않는다.

- `repro/A4-transport-process/evidence-with-proposed-fix.txt`: 위임을 추가한 상태의
  `TestA4FinLatencyOnRequestClassRefusal`도 약 4.998초 timeout으로 FAIL한다.
- `repro/A4-transport-process/evidence-repeat-runs.txt`: 위임 추가 상태에서 성공과
  timeout이 모두 남는다. 별도 `verify-A4-TRANSPORT-01-repro/02-repro-with-proposed-fix.txt`에는
  전체 PASS 실행도 있다. 성공 실행만 택하여 확정 수정으로 볼 수 없다.
- 현재 경로의 probe는 401/400 오류 JSON을 완전히 읽은 뒤 raw socket의 EOF/RST를
  검사한다. 이는 유효한 종료 계측이지만 곧바로 native가 오류 응답을 잃었다는 증명은 아니다.
- `repro/A4-transport-process/verify-A4-TRANSPORT-03-repro/evidence.txt`에는 실제 native와
  합성 backend의 9세션·176요청에서 `r.Close=true` 0회, drain 실행 0회가 기록돼 있다.
  이는 실제 interactive TUI의 모든 경로를 증명하지 않지만, drain의 실제 도달 관측이
  전혀 없다는 설명보다 구체적인 증거다. 기본 keep-alive 세션 PASS만으로 새 drain의
  복구 효과를 입증할 수는 없다.
- `repro/INTEGRATION/full-regression.txt`는 closed/stream의 `invalid SSE JSON`으로 FAIL,
  `full-regression-verify-01.txt`는 전체 PASS다. 현재 로그는 timeout·전달 손실·다른 원인을
  구분할 진단을 잃고 있어 원인이 해결됐다고 볼 수 없다.

따라서 `CloseWrite` 추가나 `r.Close` 조건 제거를 바로 배포하는 것은 권하지 않는다.
성공 응답, 큰 미소비 body의 오류 응답, 서버 주도 종료, 취소, 강제 종료를 나눠
native가 응답과 후속 요청을 처리하는지 확인해야 한다. 비교 중에도 보호 On, 단일 native
프로세스, 생성·도구 부작용의 중복 실행 금지를 유지한다. 특정 필터 버전에 분기하지 않는다.

이전 `verification/v031-transport-20260922/REPORT.md`의 제품 800건, 실제 backend TUI
246.76초·5호출·동일 PID, 전체 race PASS 기록은 과거 관측으로 보존한다. 이번 Claude
라운드에서 전체 race/backend를 재실행하지 않았다고 해서 “한 번도 검증하지 않음”으로
바꾸지 않는다. 반대로 그 과거 성공도 이번 상태 전이 반례나 간헐적 SSE 실패를 종결하지 못한다.

**보류 5건의 다음 판별 검사**

| ID | 다음 작업 |
|---|---|
| A1-20 | native subcommand별 argv 경계를 공개 help·로컬 parse 검사로 대조. 제품 지원 범위와 함께 판정 |
| A1-23 | 현재 버전 고장과 미래 버전 위험을 구분. argv 수정 묶음에 설치 native의 옵션 계약 대조를 포함하고 버전 상승 때 재실행 |
| A2-13 | `.json` 성공 / `.ready` 실패를 task 임시 폴더에서 주입한 뒤 재발행·후속 요청 복구 검사. 미완료 최신 영수증을 무시하고 이전 turn을 채택하지 않음 |
| A4-03 | 위 176요청 관측을 반영하고, 실제 TUI의 의도된 연결 단절에서 복구 경로 실행과 같은 프로세스 후속 응답을 계측 |
| A5-05 | 합성 body로 실제 `privateIntegrationPath` 및 live transport의 거부 전 경계를 직접 검사. 로컬 PTY transport에는 이 guard가 없어 `CLAUDUCT_EVIDENCE_TUI_LOCAL=1`만으로 판정할 수 없음. guard 유지, task TEMP를 명시 |

**권장 진행 순서와 종료 조건**

1. **상태 손실과 복구 고착부터 수정한다.** A2-01·02, A3-02를 처리하고 현재 실패 재현을
   정상 동작 assertion으로 고정한다. 늦은 실패·완료, 자식 대기→재개, 일시적 저장 오류 뒤
   같은 프로세스에서 새 결과와 manual compact가 보존되는지 검사한다.
2. **통신 진단을 보완하고 원인을 판별한다.** INT-01에서 원래 오류를 먼저 남긴 뒤
   보호 On의 실제 native 전송 경로와 오류 응답·서버 종료를 대조한다. 확인된 원인 위치만
   수정하고, 수정 전 실패·수정 후 성공·수정 제거 시 실패의 연결을 확보한다. 자동 replay나
   native 재시작으로 통과시키지 않는다.
3. **선택·호환성·진단을 묶어 고친다.** A1-03+A3-03+INT-02, A1-02+INT-05,
   A1-05, A3-01을 해당 경계의 최소 수정으로 처리한다. A1-01·06의 기존 정책을 보존하고
   보류 항목의 도달 조건을 함께 판정한다.
4. **검수와 증거를 정리한 동일 후보에서 최종 확인한다.** A5-03·04와 과거 hash의 판본
   연결을 보완한다. 일반 Go·전체 race·필요한 태그 검사와 실제 backend TUI를 실행한다.
   TUI는 생성·압축·취소·후속 응답뿐 아니라 이번 수정에 해당하는 자식 결과 수거·재개와
   실제 도구 전달도 검증한다. 이전 `--tools ""` TUI를 도구 검증으로 재해석하지 않는다.

릴리스 판단에는 모든 관련 재현의 정상 동작, 설명되지 않은 전체 회귀 실패의 종결,
보호 On에서 실제 작업과 복구의 관측, 각 HOLD의 해결 또는 근거 있는 지원 범위 판정이
필요하다. 독립 Node 31개·.NET 35개·raw TCP는 환경 진단으로 별도 결과를 유지한다.
제품을 거치지 않는 검사 결과를 Clauduct 수정의 성공으로 바꾸거나 필수 제품 합격 조건으로
다시 끌어오지 않는다. 문서·버전·패키징 정리는 그 뒤이며 출하 승인은 별개다.

우선 시작할 묶음은 **A2-01·A2-02·A3-02의 수정과 INT-01의 원인 보존**을 권한다.
제품 사용 중 결과를 잃거나 세션이 막히는 문제를 먼저 줄이면서, 반드시 해결해야 할
통신 문제의 다음 실험이 원인을 판별할 수 있게 만드는 순서다.
