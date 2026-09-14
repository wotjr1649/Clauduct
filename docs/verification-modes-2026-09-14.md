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

## 로컬 모드: 미판정4개

아래 네 모드는 **실행에 실패한 것이 아니라 필요한 선행 상태를 만들지 못했다**. 각 거부는 잘못된 상태에서 실행을 막는 설계된 방어이며 제품 결함의 증거가 아니다.

| 모드 | 요구 조건 | 근거 |
|---|---|---|
| `--local-continue` | 계획의 2번째 단계 이상(`plannedStep.index > 0`) | managed-development.mjs:99 |
| `--local-recover` | 직전 증거가 `MANAGER_INTERRUPTED` | managed-development.mjs:116 |
| `--local-interrupt-after-result` | — | 출력 마지막 행이 결과가 아니라 event 행이어서 판정 실패 |
| `--local-interrupt-after-step` | — | 위와 동일. 2844ms에 오류 없이 종료했다 |

세 가지 구성을 시도했고 매번 다른 거부가 나왔다.

1. 빈 계정에 직접 실행 → `MANAGED_DEVELOPMENT_EMPTY`
2. 같은 계정에서 `--local-task` 뒤 체인 → `MANAGED_DEVELOPMENT_PREDECESSOR`, `INTERRUPTION_RESULT_PRESENT`
3. 계획 생성 후 단계 진행 → `MANAGED_PLAN_STEP_INVALID`

계획·단계·원장·바인딩 검증이 서로 물려 있어 정확한 상태 재현에는 이 도구의 상태 모델을 먼저 파악해야 한다. 해당 로직 자체는 `src/test-managed-plan.mjs`가 함수 수준에서 거부 사례를 다수 검증한다. 미검증으로 남는 범위는 **CLI 진입점의 상태 전이 시나리오**다.

## CI 도입에서 드러난 환경 가정

Windows CI(`.github/workflows/tests.yml`)를 넣자 이 개발 머신에서는 구조적으로 드러날 수 없던 세 가지가 첫날에 나왔다.

- `.tmp` 작업 디렉터리를 fixture가 만들지 않고 실행기에만 의존했다. 새 checkout에서 ENOENT였고, 전체 실행에서는 어느 test가 먼저 만드느냐에 결과가 달렸다. `verification/temporary-dir.mjs`로22곳을 정리했다.
- Node24.20이 `--allow-child-process` 경고에 `[PERM0002]` 코드를 붙인다. 로컬24.19에는 없어 앵커된 정규식이 CI에서만 깨졌다.
- runner의 PowerShell 콜드 스타트가 `spawnSync`의7초 제한을 넘는다. 로컬은1.5~1.9초다.

`src/test-managed-plan.mjs`의 낡은 기대값(40 대 41)도 전체 실행으로 드러났다. `7972f2f`가 목록에 파일을 추가하면서 갱신하지 않은 것으로 main에도 있었다. 자동 회귀 관문이 없던 기간의 산물이다.

## live 모드

미완이다. 결과는 확보하지 못했다.
