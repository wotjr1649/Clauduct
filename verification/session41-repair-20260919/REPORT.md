# S41 취소 회복·중첩 Agent·Workflow 수리 결과

2026-09-19. 대상 실패 세션은 `e5fdc752-53f9-49dd-a779-7573275133e4`다. 원인 분석은 [이전 보고서](../session41-analysis-20260919/REPORT.md), 이번 변경은 `fix/context-fork-acceptance`의 `cbd0621`부터 `17532f7`까지 8개 제품 커밋이다. native Claude Code는 **2.1.278**이다.

**확인된 결함을 수정했고, 같은 최종 바이너리로 실제 native TUI 통합 검증과 launcher 강제 종료 검증을 통과했다.** 개발 중 실패도 있었으며 아래에 모두 남긴다. 모든 입력·모델 응답·향후 native 버전에 대한 무결점 또는 flaky 0을 입증한 것은 아니다. `process=SUCCESS`, `acceptance=not_assessed`를 기능 합격으로 바꾸지 않았다.

## 수정한 원인과 적용 범위

| 문제 | 근본 원인 | 변경 |
|---|---|---|
| Esc 이후 일반 프롬프트가 계속 TEXT_FIELDS | native가 중단된 text에 저장한 `citations:null`을 공통 decoder가 거부 | `null`과 빈 배열만 검증해 수용. 본문·이력을 삭제하지 않는다. 실제 인용 또는 잘못된 형식은 별도로 거부한다. generation/count_tokens가 같은 decoder를 쓴다. |
| 자식 완료 뒤 중간 Agent 자동 재진입 거부 | 생성 시 parent와 자동 재진입 요청의 빈 parent header를 같은 조건으로 비교 | 원래 선택·계보·native metadata·새 active turn·완료된 자식을 대조한 경우에만 기존 선택을 유지. `verified-continuation`, `verifiedParentAgentId`로 구분한다. 빈 헤더 자체는 실행 권한이 아니다. |
| native 기본 역할도 선택 실패 | built-in `plan`/`Plan` 차이와 생략 가능한 `subagent_type`을 잘못 처리 | 알려진 built-in 역할만 canonical 이름으로 맞춘다. 생략은 native 기본 `general-purpose`로 처리하며 생략 사실은 보존한다. 임의 custom 역할을 대소문자로 합치거나 null/잘못된 형식을 수용하지 않는다. |
| 상속된 선택과 자식 도구 선택지가 충돌 | 모델에게 노출된 도구 schema가 이미 고정된 선택 범위를 설명하지 않음 | 고정된 model/effort 또는 inherit 선택지를 노출하고 기존 자식을 기다릴 때 짧은 대기 응답을 남기도록 안내한다. 무응답을 성공으로 조작하지 않는다. |
| Workflow 명시적 모델·effort 거부 | 이전 구현은 부모 route만 허용하고 runtime opts와 실제 child를 연결하지 않음 | native VM의 `agent()`에 작은 wrapper를 적용해 실제 opts의 제공/생략 여부를 보존. run/script digest/journal child ID/metadata/독립 native active turn을 일치시켜 확정한다. native 실행·승인·도구 제한은 유지한다. |
| Windows Workflow 승인 전 input 거부 | embed된 자체 helper의 CRLF에 native가 거부하는 CR 포함 | 자체 helper만 LF로 정규화. 사용자 script는 변경하지 않는다. |
| 정상 StructuredOutput을 결과 미확보로 판정 | native가 텍스트 없이 끝난 뒤 journal에 결과를 쓰는데, 기존 회수는 텍스트만 찾음 | `awaiting_workflow_result`를 추가. 검증된 journal의 해당 child/key/result를 회수한다. SubagentStop과 journal의 두 도착 순서를 처리한다. 실제 부모 요청·종료 시 확인하며 모델 polling이나 자식 재실행은 하지 않는다. |
| 실패가 종료 시 unverified로만 남음 | 선택 실패가 새 native turn 연결보다 먼저 발생 | 독립적인 신원이 맞는 실패 회차를 `requestFailure`에 연결. 실행 거부·본문 생성·부모 수신을 분리하고, 회수하지 않았는데 회수했다고 설명하지 않는다. |

공유 입력 decoder, 선택 해석, native turn 연결, 결과 상태 전이를 수정했다. 단일 오류 메시지만 숨기는 처리를 추가하지 않았다. unsupported 조합, stale turn, 다른 session/parent, 변조 script/journal은 계속 거부한다. 새 외부 서비스·API 키·의존성은 추가하지 않았다.

## 최종 실제 TUI 통합 검증

UUID **`febe7e62-d829-4e63-a275-e167b2e8529f`**, [세션 근거](run-febe7e62-d829-4e63-a275-e167b2e8529f.json). `-p`가 아닌 대화형 native TUI에서 Win32 키 입력으로 실행했다. 기존에 승인된 공개 테스트 project/profile을 사용했다.

| 항목 | 관측 결과 |
|---|---|
| 요청 | 전체 49, generation 27 |
| API 오류 / native 도구 오류 | 0 / 0 |
| 의도한 Esc 취소 | 2: delivery 단계 1, count 단계 1 |
| 현재 결과 상태 | `parent_received=7`, 미확보 0 |
| 정확 계수 대조 | backend usage가 있는 25건 모두 일치, 불일치 0 |
| 종료 | exit 0, active 0, nativeReaped true, watchdogForced false |

계수 일치 25건의 모델별 내역은 Astra 11, Terra 8, Luna 6이다. 이 세션에서 Sol 생성은 없으므로 4개 모델 모두 실측했다고 쓰지 않는다. 27회의 preflight에는 의도한 취소 2건이 포함된다.

**중첩 Agent:** ROOT → A → B → C. A는 명시적 Terra/medium, B와 C는 원래 인자에서 model/effort를 모두 생략했다. C의 `LEAF=1530`을 B가 받아 `MIDDLE=1530`, A가 받아 `OUTER=1530`, root가 `TREE_OK=1530`을 반환했다. 모든 결과가 부모에게 전달됐다. B의 seq 19는 parent header가 빠진 실제 자동 재진입이며, 검증된 원래 parent와 Terra/medium으로 통과했다. [계보·재진입 근거](tree-proof.json)

**병렬 Workflow:** 단일 native Workflow에서 아래 네 child가 각각 한 번의 backend 완료로 끝났다. leaf에는 native `disallowedTools`와 StructuredOutput schema를 적용했다. 결과는 부모가 실제로 받은 `[{"value":23},{"value":29},{"value":31},{"value":37}]`이다. journal 회수와 정상 부모 수신이 모두 관측됐다. [선택·결과 근거](workflow-proof.json)

| 원래 지정 | 확정 실행 | 반환 |
|---|---|---|
| model+effort | Terra/medium | 23 |
| model만 | Luna/max | 29 |
| effort만 | Astra/high | 31 |
| 둘 다 생략 | Astra/low | 37 |

**Esc:** seq 44는 일부 텍스트가 나온 delivery에서 중단됐다. terminal 이벤트 없이 client context 취소가 기록됐고, 다음 seq 46이 `RECOVERY_FINAL_OK`로 완료됐다. seq 48은 정확 계수 도중 취소됐으며 다음 seq 49가 `RECOVERY_AFTER_COUNT_OK`로 완료됐다. 두 취소 시점은 status로 구분했다. 단순히 Esc 키를 보냈다는 사실로 취소 성공을 판정하지 않았다.

**추가 실제 TUI:** 같은 제품 커밋·해시의 UUID `fb24a803-a013-45e4-9845-9a4318c67b14`에서 Sol/high 자식 `ae5dbc722f9a9c292`가 `SOL_PHASE_ONE=187`을 반환한 뒤, SendMessage 한 번으로 같은 ID를 재개해 `SOL_PHASE_TWO=221`을 받았다. 후자는 실제 `verified-resume` route였다. 새로운 대체 Agent는 생성하지 않았다. [재개·취소 근거](resume-cancellation-proof.json)

이 추가 세션은 전체 21요청, generation 14, 의도한 취소 1, API 오류 0, native 도구 오류 0, 미확보 0, exit 0이었다. backend usage가 있는 13건이 모두 계수와 일치했고 Sol 2건을 포함한다. 따라서 두 최종 TUI 세션 합계는 **70요청, 38건 계수 일치, 불일치 0, API 오류 0, 의도한 취소 3**이다. 서로 다른 세션을 한 세션의 수치로 합쳐 표시하지 않는다.

추가 취소는 긴 공개 Bash 인자를 생성하도록 요청한 뒤 수행했다. seq 17에서 backend 이벤트 398개를 받은 delivery가 CANCELLED로 끝났으며 native에는 아직 도구 호출이 전달되지 않았고 Bash 실행도 없었다. 이후 `RECOVERY_AFTER_TOOL_PREP_OK`와 별도 새 프롬프트의 `CLEAN_NEXT_TURN_OK`가 실제 완료됐다. 취소 후 native가 편집기로 돌려놓은 원문에서 Ctrl+U가 마지막 시각적 줄만 지워 첫 회복 지시가 앞선 문장에 붙은 사실도 기록한다. 두 번째 확인은 독립된 새 프롬프트였다.

**이 시험으로 backend의 부분 도구 인자 이벤트 자체까지 입증하지는 못했다.** 기존 translator가 완성된 도구 인자를 검증한 뒤 native에 한꺼번에 전달하고 status에는 backend 이벤트 종류를 저장하지 않기 때문이다. 도구 전달 전 생성 취소·동일 세션 회복은 확인됐지만, 취소 직전 이벤트가 `response.function_call_arguments.delta`였다는 증거는 없다. 미완성 인자가 native에 전달되는 시점의 실제 TUI 취소 검증은 미확인으로 남긴다. 이 한 항목을 증명하려고 제품 로그에 원문 인자를 저장하거나 같은 생성을 반복하지 않았다.

## launcher 강제 종료

별도 UUID **`261642e5-d5c9-4a70-8a35-41eb11c1a45a`**에서 native Bash가 실제 `powershell.exe -NoProfile -Command "Start-Sleep -Seconds 90"`을 실행한 뒤 시험했다. 일회성 native 명령 승인만 사용했고 기억된 허용 규칙이나 auto mode는 설정하지 않았다.

launcher 하나만 종료했을 때 캡처한 launcher와 하위 프로세스 6개의 handle이 모두 **36ms 이내**에 종료 신호를 보였다. 별도 비교용 decoy는 살아 있었다. 이후 테스트가 자신이 만든 decoy만 정리했다. `taskkill /T`로 결과를 만들어낸 것이 아니다. [프로세스 handle 근거](hard-kill-261642e5-d5c9-4a70-8a35-41eb11c1a45a.json)

이 실행의 exit `57005`는 시험 도구가 지정한 강제 종료 코드다. 정상 종료 성공으로 집계하지 않는다. `run.json`의 watchdogForced는 false다. 이 시험은 **launcher 프로세스 강제 종료** 범위이며 콘솔 창 닫기·OS 종료를 모두 시험한 것은 아니다.

## 자동 검사와 검수

- 수정 전 `citations:null`/`[]` 거부 재현 후, 같은 의미의 빈 citations와 잘못된/실제 인용을 구분하는 회귀 검사.
- 자동 재진입의 정상 경로와 잘못된 parent/session/model, stale 또는 완료 turn, 명시적 resume 충돌 검사.
- 역할 생략/잘못된 값, 원래 model·effort 생략 기록, 고정 선택지 검사.
- Workflow 네 선택 방식, unsupported 값, native receipt 불일치, script 변조, 잘못된 journal key/중복 result/foreign child, SubagentStop·journal 순서 검사.
- 새 native 실패 회차가 진단·terminal 결과에 연결되는지 검사.
- 정확한 제품 커밋의 깨끗한 별도 snapshot에서 `go test ./... -count=1`: **17 packages, 1,525 test/subtest pass 기록, failure 0**. `go vet ./...` exit 0. [검사 요약·원본 로그 SHA256](test-summary.json). 전체 로그 `all-1789818346696.jsonl`은 원래 `context-fork-repair-20260919/verification/session41-repair-20260919`에 로컬 보존하며 저장소에 포함하지 않는다.
- Go 검사 중 skip 3개: `TestConsoleCloseIsNotRunHere`, `TestALiveSessionCompletes`, `TestWhatSurvivesTheLauncherBeingKilled`. native TUI와 launcher kill은 위 별도 실험으로 확인했다. skip을 실행으로 바꾸어 보고하지 않는다. Go race detector는 실행하지 않았다.
- 개발 바이너리 반영 전 검사 장치의 정상/실패 근거 8가지 확인: API 실패, 미확보 결과, 누락된 취소 단계, 회복 응답 부재, 남은 프로세스, 실패 검사, 다른 커밋을 거부한다. 파일 변경은 가로채고 판정만 검사했다. 실제 반영은 이어서 별도로 수행했다.
- 제품 변경의 `git diff --check` 통과. 원래 작업 디렉터리 및 다른 untracked 작업은 수정·흡수하지 않았다.

## 중간 실패를 보존한 기록

| UUID | 결과와 이후 조치 |
|---|---|
| `1f5e05d6-15a6-463a-9ffb-50d08890571c` | P0 부분 텍스트 취소 후 회복 확인, API 0. 먼저 시도한 짧은 출력은 Esc 전에 끝났으므로 취소 근거로 세지 않았다. |
| `282efa3f-5a35-460a-afa0-12db2cfdab80` | built-in 역할 대소문자 불일치 등으로 API 2. canonical 역할 처리 후 다른 binary로 확인했다. |
| `e0d01331-3a69-4c93-a3b9-b6deaaab0415` | Agent 준비 거부 API 1. 이후 진단 보강과 생략 역할 수정으로 원인을 해결했다. 별도 native wrapper 기전 실험만 성공했다. |
| `a0c98c88-447e-461f-8dc5-afb720cd63b3` | 중간 coordinator가 가시적인 본문 없는 terminal을 반환하여 `EMPTY_REPLY`/502, API 1. 실패를 성공으로 변환하지 않았다. 가시적 대기 응답 안내를 보완했지만 모든 모델 출력의 재발 부재를 입증하지는 못했다. |
| `10439ade-7a59-4e81-8649-7a6982344d17` | 자체 helper CRLF 때문에 native 승인 전 입력 거부. gateway API 오류 0이어도 Workflow 검증 실패다. LF 수정 뒤 진행했다. |
| `e5007e93-5ae8-4cf8-a6db-eab11e6e4af0` | 4개 route는 맞았지만 Luna가 root/leaf 지시를 혼동하고 ToolSearch/Bash를 호출했다. 중단했고 기능 통과로 세지 않았다. 최종 leaf 시험은 native 도구 제한과 결과 schema를 사용했다. |
| `c79d1260-85eb-4ebf-8330-0dd8849e776b` | 생략된 Agent 역할 거부 API 1. Workflow 4개는 값을 생성했지만 gateway는 본문 미확보로 잘못 판정했다. 기본 역할 처리와 journal 결과 회수 구현 후 최종 통합을 수행했다. |
| `febe7e62-d829-4e63-a275-e167b2e8529f` | 최종 17532f7의 위 통합 시나리오 통과. 앞선 실패를 지우거나 반복 실행에서 나온 성공으로 대체하지 않았다. |

각 UUID의 `run-<UUID>.json`을 보존했다. 전체 Go 검사에서 나온 기존 기대값 충돌 2건도 변경된 source attribution/거부 진단을 명시적으로 검증하도록 고쳤으며, 실패 로그 `all-1789817233624.jsonl`을 남겼다. 검증 TTY에 긴 프롬프트를 한 번에 보낼 때 일부 입력이 잘린 사례는 chunked bracketed paste로 바로잡았다. 해당 입력 손실을 제품 정상/실패 근거로 사용하지 않았다.

## 남은 한계와 이번에 확인하지 않은 범위

1. **모든 Workflow 형태 지원은 아니다.** 실제 지원·검증은 inline script의 native `agent()`와 병렬 반환 경로다. `scriptPath`, named workflow, `resumeFromRunID`, remote/nested Workflow, custom `agentType` 등은 이 구현으로 지원 완료했다고 볼 수 없다. 임의 script의 모든 JavaScript 사용 방식도 검증하지 않았다.
2. **모델의 가시적 본문 없는 응답과 자연어 지시 불이행은 0을 보장하지 못한다.** 확인한 EMPTY_REPLY는 오류로 남긴다. 결과가 없는데 성공 본문을 만들거나 자동으로 자식을 재실행하지 않는다. 도구 금지는 자연어만으로 강제되지 않으므로 native 도구 제한을 사용한다. 실제 leaf 4개 결과의 성공은 이 제한된 조건의 증거다.
3. **native 화면은 모델별 정책의 확정 근거가 아니다.** 이번 작업은 `/context` 출력·렌더러를 바꾸지 않았다. 공통 500K와 native effort 표시의 한계는 남는다. effective 선택과 preflight 적용은 Clauduct status에서 확인한다. native `/context`의 모든 표시를 동적으로 모델별 값으로 고쳤다고 말하지 않는다.
4. **성능 무저하는 입증하지 않았다.** wrapper는 추가 모델 호출을 만들지 않고 결과 회수도 모델 polling을 하지 않는다. 그러나 정확 계수의 네트워크 지연은 남는다. 최종 통합의 countMs 누계는 36,726ms/27 preflight이며 동시 요청 누계라 체감 지연 총합과 같지 않다. journal 회수는 child당 최대 16MB로 제한된 읽기다. 전후 성능 benchmark는 이번에 수행하지 않았다.
5. **239K/450K 압축 경계, 전체 멀티모달/tokenizer, 모든 모델·effort 조합을 새로 실측하지 않았다.** 기존 자동 회귀와 이번 실제 요청의 일치 확인 범위를 구분한다. 원래 사용자 S41 UUID를 새 binary로 resume하는 복구 실행도 하지 않았다. 기존 원본 이력은 보존했다.

## 개발 바이너리 반영과 되돌리기 근거

- 반영 위치: `D:\AIDEV\clauduct-s36-build\clauduct.exe` 및 같은 디렉터리의 hook/dev companion.
- 제품 커밋: **17532f7dc9daf7d5c47b0539af62a243c43df5a3**, Go 1.27.1 windows/amd64, `vcs.modified=false`.
- main SHA256: `84bdbdf85de0c5e555dc01053979ee7eb8e5f9a5cfd9872a97a86a8033c60011`.
- 최종 TUI·kill 시험에 사용한 세 실행 파일과 설치된 세 파일의 해시가 각각 동일하다. [build](build.json), [반영 기록](delivery.json).
- 이전 세 실행 파일은 `D:\AIDEV\clauduct-s36-build\before-17532f7-20260919`에 보존했다. 이전 제품 커밋은 `051177d220dbfd6277f05c76d7669a25f83e6d55`다. 되돌릴 경우 해당 세 파일을 한 세트로 사용하고 manifest도 같은 버전으로 맞춰야 한다. 자동 rollback이나 원본 사용자 세션 변경은 하지 않았다.
- 기존 사람용 `repair-20260919\verification\policy-repair-20260919\interactive.mjs`는 evidence 경로 한 줄만 이번 디렉터리로 갱신했다. 이전 파일은 `interactive-before-session41.mjs`에 보존했다. native profile/trust/global 설정과 `C:\Users\js\.local\bin`의 릴리즈는 변경하지 않았다.

사람용 검증 실행은 기존과 같다. 다음 명령은 실제 개발 binary 세 파일의 hash를 확인한 뒤, 승인된 테스트 폴더/profile에서 최대 2시간의 native TUI를 연다.

```powershell
node D:\AIDEV\clauduct-s36-build\repair-20260919\verification\policy-repair-20260919\interactive.mjs --installed --mode=human
```
