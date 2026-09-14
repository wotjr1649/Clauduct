# 검증 모드 전수 실행 — 2026-09-14

`--local-*`와 `--live-*` 검증 모드를 CLI 진입점으로 직접 실행한 기록이다. 로컬 모드는 고정 공개 응답 fixture를 loopback transport로 공급하므로 backend 추론이 없다. 실행한 모든 로컬 모드가 `actualModelRequests:0`이었다. 이 문서는 미실행·미판정을 PASS로 바꾸지 않는다.

## 로컬 모드: 정상 동작 확인

| 모드 | 진입점 | 결과 |
|---|---|---|
| `--local-task` | verify-native-development | passed, 1555ms,5 attempts |
| `--local-sequence` | 〃 | 3단계 모두 passed |
| `--local-output-recovery` | 〃 | 주입한 `OUTPUT_PIPE_CLOSED` 뒤 복구 passed |
| `--local-incomplete-recovery` | 〃 | 주입한 `DEVELOPMENT_TASK_INCOMPLETE` 뒤 복구 passed |
| `--local-native` | 〃 | passed |
| `--local-native` | verify-native-permissions | passed |
| `--local-native` | verify-native-task-isolation | passed (`alpha` 취소) |
| `--local-native` | verify-native-service-recovery | passed (`error-200`) |
| `--local-task` | managed-development | passed,2706ms |
| `--local-hold-after-source` | 〃 | passed,8154ms |
| `--local-hold-after-read` | 〃 | passed,7128ms |
| `--local-plan` | managed-plan-entry | passed,6472ms |

`--local-output-cut`은 단독으로 `passed:false`·`OUTPUT_PIPE_CLOSED`를 보고하지만 `outputCut:true`·`cutReleased:true`로 주입과 해제가 확인됐다. 이 모드는 단절을 만드는 쪽이고 복구 판정은 `--local-output-recovery`가 맡는다. 실패로 세지 않는다.

`verify-native-service-recovery`의 `SERVICE_FAULTS`는 `flapping-503`·`deferred-503`·`error-200`·`truncated`·`invalid-utf8`·`sequence-gap`6종이며 이번에는 `error-200`만 실행했다. 나머지5종은 미실행이다.

## managed 계열 네 모드의 재현 절차

`--local-continue`·`--local-recover`·`--local-interrupt-after-result`·`--local-interrupt-after-step`은 처음에 선행 상태를 만들지 못해 세 번 거부됐다. 이후 조건을 코드에서 확인해 **네 모드 모두 `passed:true`로 실행했다**. 전부 `actualModelRequests:0`이다.

**거부의 실제 원인**은 계획 파일이었다. CLI 단발 경로는 `plannedStep`을 전달하지 않는데(managed-development.mjs:254-257) `:91`은 root에 `development-plan.json`이 있기만 하면 `plannedStep` 검증에 진입한다. 따라서 **계획이 있는 root에서는 `--local-task`를 포함한 모든 단발 모드가 `MANAGED_PLAN_STEP_INVALID`로 거부된다**. 계획을 만들어 해결하려던 시도가 오히려 원인이었다.

| 모드 | 선행 조건 | 근거 |
|---|---|---|
| `--local-continue` | 계획 **없는** root. 마지막 entry가 닫히고 `passed`, 같은 model·localNative, **다른 taskId**, `account.pending === null` | managed-development.mjs:119,121-126 |
| `--local-recover` | 계획 없는 root. 중단된 native root에 **`result.json`이 없어야** 한다. taskId·model은 중단된 실행과 동일 | :104-107, verify-native-development.mjs:262 |
| `--local-interrupt-after-result` | 특별한 선행 상태 없음. `--local-task`와 동일 | :144 |
| `--local-interrupt-after-step` | root에 `development-plan.json`과 `development-plan-ready.json`, `count ≤ 전체 단계 수` | managed-plan-entry.mjs:5 |

`--local-recover`는 CLI 단독으로 구성할 수 없다. `result.json`은 관리기만 쓰고(verify-native-development.mjs:754) hold 모드는 native만 붙잡을 뿐 관리기는 완주하므로, 그냥 두면 항상 `result.json`이 생겨 `INTERRUPTION_RESULT_PRESENT`로 영구 회수 불가가 된다. **hold 창 안에서 관리기 프로세스만 외부에서 종료**해야 한다. hold 마커는 `<nativeRoot>/work/events.jsonl`의 `SOURCE_WRITE_WAIT`다.

두 interrupt 모드는 **판정 행을 출력하지 않는다**. `process.exit(73)`이 `console.log`보다 먼저 실행된다. **exit code73이 주입 성공 신호**이고 판정은 후속 명령에서 읽는다 — `--local-interrupt-after-result`는 `--reconcile <root>`, `--local-interrupt-after-step`은 `--local-plan <root> <pwsh>`의 마지막 행이다.

계정과 계획 생성에는 CLI가 없어 `createManagedLedgerAccount`·`createManagedPlan`을 `node -e`로 호출해야 한다.

## CI 도입에서 드러난 환경 가정

Windows CI(`.github/workflows/tests.yml`)를 넣자 이 개발 머신에서는 구조적으로 드러날 수 없던 세 가지가 첫날에 나왔다.

- `.tmp` 작업 디렉터리를 fixture가 만들지 않고 실행기에만 의존했다. 새 checkout에서 ENOENT였고, 전체 실행에서는 어느 test가 먼저 만드느냐에 결과가 달렸다. `verification/temporary-dir.mjs`로22곳을 정리했다.
- Node24.20이 `--allow-child-process` 경고에 `[PERM0002]` 코드를 붙인다. 로컬24.19에는 없어 앵커된 정규식이 CI에서만 깨졌다.
- runner의 PowerShell 콜드 스타트가 `spawnSync`의7초 제한을 넘는다. 로컬은1.5~1.9초다.

`src/test-managed-plan.mjs`의 낡은 기대값(40 대 41)도 전체 실행으로 드러났다. `7972f2f`가 목록에 파일을 추가하면서 갱신하지 않은 것으로 main에도 있었다. 자동 회귀 관문이 없던 기간의 산물이다.

## live 모드: 1회 실측

`--live-task sol/low`,taskId `retry-delay-window`를1회 완주했다. 출력 상한은 fixture의 기본32768이다. settings.json의 `CLAUDE_CODE_MAX_OUTPUT_TOKENS=64000`을 적용하려 했으나 `fixture-token-budget.mjs:13`이32768을 하드 상한으로 강제해 거부된다(`VERIFICATION_TOKEN_BUDGET_INVALID`). 그 상한을 완화해 통과시키지 않았다.

| 항목 | 실측 |
|---|---|
| 벽시계 | 425,476ms |
| 요청 | 6 |
| 입력 토큰 | 17,274 |
| 출력 토큰 | 1,050 |
| 예산 한도 | attempts16, 입력131072, 출력32768, 시간630000ms |
| 한도 내 | true |

첫 시도는420초 제한으로 끊겼다. 제품이 멈춘 것이 아니라 단계 예산이 `phaseMs=600000`(verify-native-development.mjs:510)이어서 제한 자체가 부족했다.

판정은 `passed:false`였고 전송 계층 지표는 모두 정상이었다 — `contextMatched`·`routeMatched`·`nativeJson`·`attemptsComplete`·`cleanupComplete`·`completedUsageObserved` 모두 true, `nativeError` false.

## live 판정 실패의 원인: 무인 실행은 구조적으로 통과할 수 없다

`run_tests`는 `control/review.json`의 승인을 요구한다(development-mcp.mjs:92-107). fixture는 **기준 소스 해시로만 사전 승인**되어 있어(development-fixture.mjs:35) 모델이 소스를 쓰는 순간 해시가 어긋나고, 이후 모든 `run_tests`는 새 승인을 최대180초 기다리다 `TESTS_UNRUN`을 남기고 `SOURCE_REVIEW_REQUIRED`로 끝난다.

그 승인을 기록하는 코드는 `verify-native-development.mjs:601-604`의 **`if (localNative)` 안에만** 있다. live는 `:605`에서 `SOURCE_REVIEW_PENDING`을 stdout에 출력만 하며, **이 이벤트를 소비하는 코드는 저장소에 존재하지 않는다**. 관측된180초 대기 두 번이 이것이다. `RELEASE.md:101`의 "시간에는 외부 소스 검토 대기가 포함된다"는 과거 live 성공 시 사람이 승인했음을 뜻한다.

**따라서 무인 live 개발 모드는 몇 번을 돌려도 통과할 수 없다.** 제품 전송 경로의 결함이 아니라 검증 절차가 사람 개입을 전제로 설계된 것이다.

### 앞선 판독 두 가지를 정정한다

`baselineFailed`는 "기준선이 통과했다"는 뜻이 **아니다**.

```js
// verify-native-development.mjs:693-694
const tests = events.filter(row => row.event === 'TESTS_EXECUTED');
const baselineFailed = tests.length >= 2 && tests[0].passed === false;
```

`TESTS_EXECUTED`가2건 미만이면 무조건 false다. 이번은 `testsExecuted:1`이었으므로 **길이 조건에서 걸린 것**이고, 실행된 그 한 건은 사전 승인된 기준선이며 실패했을 가능성이 높다. 이 값만으로는 "기준선 통과"와 "테스트 미실행"을 구분할 수 없다.

"과거32-case 대 이번1"은 **서로 다른 축의 비교였다**. 32는 `task.checks`로 한 번의 `TESTS_EXECUTED` 안의 케이스 수이고(development-tasks.mjs:38), `testsExecuted`는 오라클이 돌아간 횟수로 정상 최대값이2다.

### 코드 건전성 문제2건을 수정했다

둘 다 이번 오독의 직접 원인이었다.

- `baselineFailed`는 `tests[0]`이 실제 기준선인지 해시로 확인하지 않는다. 같은 파일의 중단 분류기(`:133`,`:146`)는 `baselineHash`를 검사하는데 판정 경로는 하지 않는다.
- "검토 미승인 → 테스트 미실행"에 대응하는 failure 코드가 없다. 그래서 `passed:false`·`failure:null`이라는 무진단 실패가 나오고 사유는 `work/events.jsonl`의 `TESTS_UNRUN` 행에만 남는다.

### 승인 대행 실행: 가설 입증

같은 조합(`--live-task sol/low`, `retry-delay-window`)을 감시 스크립트와 함께 한 번 더 실행했다. 스크립트는 `SOURCE_REVIEW_PENDING`을 감시하다 모델이 쓴 소스를 읽고 **실행 안전성만** 검사한 뒤 `control/review.json`에 승인을 기록한다. 정답 대조는 하지 않는다 — 그것은 oracle의 역할이고 가져오면 live 실행의 의미가 사라진다.

| 항목 | 무인 실행 | 승인 대행 실행 |
|---|---|---|
| passed | false | **true** |
| testsExecuted | 1 | **2** |
| baselineFailed | false | **true** |
| revisedPassed / independentPassed | false / false | **true / true** |
| 요청 | 6 | 5 |
| 입력·출력 토큰 | 17,274 /1,050 | 13,977 /559 |
| elapsedMs | 425,444 | **26,585** |

시간이16배 줄어든 것은180초 검토 대기2회가 사라졌기 때문이다. 요청 수5는 `RELEASE.md:101`이 기록한 과거 성공 사례와 정확히 일치한다. **무인 실행이 통과하지 못한 원인이 승인 게이트라는 진단이 입증됐다.**

승인 주체는 사람이 아니라 에이전트였다. 모델이 쓴 소스는 `retryDelayWithinBudget` 순수 함수로 외부 접근·무한 루프가 없었고, oracle의32-case를 통과했다. 이 실행은 "무인 가능"을 뜻하지 않는다. 검토자가 있을 때 live 경로가 끝까지 동작함을 보인 것이다.

### 수정 검증

`--local-task`와 승인 대행 live는 수정 뒤에도 `passed:true`이고 `testsUnrun:0`이다. 전체130건 회귀도 없다(잔여 실패5건은 별개의 loopback filter 사안).

미승인 경로는 무인 live1회로 직접 관측했다.

| 항목 | 수정 전 | 수정 후 |
|---|---|---|
| failure | `null` | **`SOURCE_REVIEW_UNAPPROVED`** |
| testsUnrun | 없음 | **2** |

`testsUnrun:2`는 승인 대기가 두 번 만료됐다는 뜻으로, 전송 기록에서 관측한180초 대기2회와 일치한다. 수정 전이었다면 이 실행도 `passed:false`·`failure:null`로 끝나 같은 역추적과 같은 오독을 반복했을 것이다.

나머지8개 live 모드는 실행하지 않았다.
