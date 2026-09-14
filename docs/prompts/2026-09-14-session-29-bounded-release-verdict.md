# Session-29: 제한된 예산으로 Clauduct 수정·커밋·출하 판정

독자는 같은 Windows PC에서 파일·셸·Git·목표 도구를 사용할 수 있는 다음 Codex 세션이다. 이전 대화는 없다. 사용자가 현재 요청에서 이 문서를 채택해 구현을 지시할 때 적용할 인계다. 문서 자체가 전송·보안 변경 권한을 만들지는 않는다. 현재 요청과 호스트가 로드한 운영 계약을 따르며, Codex-home AGENTS.md의 전체 S1~S8/W1~W11을 사용할 수 없다면 해당 로드 실패를 먼저 처리한다.

현재 상태는 **출하 HOLD**다. 이번 준비 세션에서는 인계와 grilling만 수행했으며 제품 수정·새 실호출은 없다. 다음 세션의 목표는 계획 작성이 아니라 아래 범위의 구현·검증·작업 전용 커밋과 근거 있는 로컬 출하 판정이다.

## 1. 확정된 목표와 종료 조건

사용자는 다음을 확정했다.

- 정상 Workflow 재개는 필수다. native 제약 때문에 Clauduct에서 해결 불가능하다는 근거가 확인되면 HOLD로 마감한다.
- 저장 기록이 남고 이전 worker가 종료된 상태에서, 새 프로세스로 **동일 저장 세션**을 열어 Workflow를 재실행한다. 저장 결과 재사용과 필요한 agent 재실행을 구분해 원래 과제를 완료해야 한다. 이미 발생한 파일 효과를 확인하며, 기록 소실·효과 불명확을 성공으로 처리하지 않는다.
- 검증은 실제 native에 공개 오류를 주입하는 시험과 핵심 복구·개발의 두 모델 실호출을 조합한다. 두 증거를 구분한다. 모든 장애를 실제 backend와 결합하거나 자연 발생 장애를 기다리는 요구는 없다.
- Clauduct 실호출 조합은 `gpt-5.6-luna/max`, `gpt-5.6-sol/low`다. 이는 바깥 Codex 담당자의 모델 지정이 아니다.
- 조사·구현·검증·정리 전체 **3시간**, 추가 실호출 **64요청**, 추가 입력 **400000토큰**, 추가 출력 **30000토큰**이다. 어느 한도가 먼저 도달해도 자동 증액하지 않는다. 필수 증거가 없으면 HOLD로 종료한다. 토큰 상한은 Clauduct 검증 호출에 적용하고 담당 Codex 자체 사용량과 구분한다.
- 일반 개발 판단은 위임됐다. 진행을 기능 하나마다 끊거나 승인·continue를 반복 요청하지 않는다. 실제 차단은 해당 효과에 한정하고 예산 안의 독립 작업을 계속한다.
- **검증된 완결 변경 단위마다 작업 전용 브랜치에 커밋하면서 진행한다.** 끝까지 모든 변경을 한 덩어리로 쌓아두지 않는다.

출하 판정에서 제외한 것은 HTTP/TCP 문제, 동적 symlink/junction 실행 검사, Clauduct의 정상 인증 갱신 판정, 기본 400K/320K 압축 발동의 출하 전 실호출, 4h/24h×3/72h 장기 시험이다. 정상 인증 갱신은 사용자의 Codex CLI 로그인으로 판단하고 기본 압축은 사용자 실사용 검증으로 이관했다. 기존 경로·신원·401/403·예산·취소·도구 오류·압축 설정/변환/라우팅 검증과 실패 이력은 유지한다. 제외를 PASS로 바꾸지 않는다.

부모의 native 직접 알림 방식은 필수가 아니다. 자식 결과·실패를 올바른 작업에 연결해 안전하게 후속 처리하는 기능은 유지한다. 기존 명시적 수집·메인 중계를 재사용하는 것이 현재 해결 방향이다.

## 2. 작업 위치와 시작 상태

2026-09-14 인계 작성 시 직접 재확인한 상태다. 시작할 때 현 상태와 대조한다.

| 구분 | 경로·상태 |
|---|---|
| 사용자 루트 | `D:\AIDEV\Clauduct` |
| 사용자 브랜치/HEAD | `fix/native-completion-resume` / `aa317c75c7bad0cf9d16641ca7db4edb43016c19` |
| 보존할 사용자 상태 | README.md +2/-0 및 다수 기존 untracked. 기존 자료를 일괄 stage·정리·복사하지 않는다 |
| 실제 구현 worktree | `D:\AIDEV\Clauduct\.tmp\unattended-release\implementation` |
| 구현 브랜치/HEAD | `work/unattended-release-2026-09-13` / `f425cf0ead479a7914ce9b4381bbcff854fae13f` |
| 구현 상태 | tracked clean, `.tmp/` untracked |
| 최근 제품 수정 | `e189590`: 실패한 현재 자식 요청의 완료 증거 연결. `da073556`: 편집된 이전 Workflow script가 무관한 신규 run을 막던 결함 수정 |
| 후속 문서/패키징 | `91e72bd`, `fea90000`, `f425cf0`. 최신 HEAD 전체가 기존 ZIP의 commit과 같다는 뜻은 아니다 |

구현·테스트·릴리즈 작업의 cwd는 위 구현 worktree로 한다. 사용자 루트의 launcher 교체나 브랜치 통합은 하지 않았으며 다음 세션의 로컬 ZIP 출하 판정에도 필수로 추가하지 않는다.

처음에는 이 문서와 [현재 판정](../release-verdict-2026-09-14.md), 구현 worktree의 `docs/release-completion-2026-09-14.md`를 읽는다. [진행 기록](../unattended-release-progress.md)의 최신 부분과 F01~F23 표를 최신 증거에 대조한다. 기능별 확인이 필요하면 구현 worktree의 `docs/remaining-verification.md`, `docs/native-feature-support.md`, `docs/claude-option-classification.md`, `docs/release-readiness.md`를 해당 범위만 읽는다.

[Session-28](2026-09-13-session-28-unattended-release-goal-loop.md)은 이전 명세의 출처다. 긴 이력 전체를 시작할 때 재독하거나 제외된 장기 요구·과거 예산을 되살리지 않는다. 이 문서에 정리한 최신 사용자 결정을 적용하되, 나머지 약속 기능을 자동 면제하지 않는다.

## 3. 첫 작업과 구현 순서

**먼저 현재 출하 차단 목록을 확정한다.** F12/F14는 주요 공백이지 잔여 전부라고 검증된 목록이 아니다. 기존 표에는 F15 간헐 실패 등 다른 IN_PROGRESS/NOT_RUN도 남아 있다. 각 적용 요구를 다음과 연결한다.

`요구 → 현재 후보의 동작 → 기존/추가 검사 → 실제 관측 → PASS/FAIL/NOT_RUN/BLOCKED → 근거 경로`

과거 이력·사용자 제외·검증 방식 변경으로 충족된 항목·현재 결함을 구분한다. “실제 backend 결합 미완료”라는 옛 문구만으로 Q2에서 허용한 native 장애 주입 증거를 무효화하지 않는다. 반대로 다른 층의 단위 검사나 정상 신규 Workflow 성공으로 필수 재개를 대체하지 않는다. 결과를 본 뒤 성공 조건을 낮추지 않는다.

다음 구현의 출발점은 다음과 같다. 일반 세부 설계는 담당자가 현재 코드와 최소 반증 검사로 정한다.

### 부모 결과·실패 처리

- 현재 일반 main→Agent→main은 실제 통과했다. 문제는 main→중간 부모→비동기 자식에서 중간 부모 턴이 끝난 뒤의 처리다.
- 부모가 실행 중이면 반환된 task ID의 결과 수집을, 턴이 끝났으면 검증된 관계를 근거로 메인 중계→동일 부모 재개를 검토한다. 새 자식을 다시 실행해 이전 결과 회수를 대체하지 않는다.
- `verification/completion-relay-target.mjs`는 고정 성공 문구와 자식 2개를 검사하는 **검증용 helper**다. 이것만으로 일반 제품의 실패·취소·중복 처리가 구현됐다고 하지 않는다.
- 최소 사례: 성공, 자식 실패, 중복 결과, 오래된 요청, 형제 혼입, 사용자 취소. 원래 작업의 복구 가능성과 완료를 대조하고, 사용자 취소는 자동 재개하지 않는다.
- 먼저 읽을 코드: `src/agent-selection.mjs`, `src/agent-route.mjs`, `verification/completion-relay-target.mjs`.
- 관련 검사: `src/test-failed-completion-resume.mjs`, `src/test-completion-relay-target.mjs` 및 해당 caller의 기존 검사.

### Workflow 정상 재개

서로 다른 두 문제를 분리한다.

1. 기존 공개 probe는 최초 Workflow 완료 뒤 script 편집·재실행에서 native2.1.269의 **반환 script 경로/허용된 읽기 범위 검사**에 거부됐다. 당시 인수와 native 등록값의 정확한 일치는 미확인이다. 관리자 권한 부족·Windows 결함·기록 소실·native 미지원으로 단정하지 않는다. 설명문 파싱과 반환된 구조화 필드를 대조하고 정상 접근 조건을 확인한다.
2. Clauduct의 `src/agent-route.mjs`, `src/agent-selection.mjs`는 신규 inline Workflow를 중심으로 연결하고 `src/workflow-selection.mjs`는 이미 등록한 run을 다시 받지 않는다. native 접근 문제만 해결해도 재개 호출·이전 실행·새 자식/실행 세대의 신원 연결 공백이 남는다.

정상 재개가 반환하는 run/task ID와 journal 형태는 아직 관측되지 않았다. 같은 ID 또는 append 형식이라고 가정하지 말고, native 계약과 허용된 실제 관측을 근거로 구현한다. 이전 자식의 기록으로 새 요청을 승인하지 않는다.

핵심 성공 사례는 저장 결과가 있는 Workflow를 중단하고 이전 worker의 종료를 확인한 다음, 새 프로세스에서 동일 저장 세션을 열어 재실행하여 원래 산출물까지 완성하는 것이다. 재사용 결과와 재실행 대상을 관측하고 새 자식의 model/effort를 대조한다. 이미 발생한 파일 효과도 독립 검사로 확인한다. 관련 음성 사례는 이전 worker 생존, 다른 세션·변조·오래된 기록, 저장 결과 전체 소실이다. 전체 기록 소실은 명확한 재개 불가가 기대 결과이며 자동 복원을 요구하지 않는다.

관련 검사: `src/test-workflow-selection.mjs`, `src/test-workflow-journal.mjs`, Agent 선택/재개 및 admission 검사.

기존 거부 효과를 다른 경로 표기·셸·wrapper·새 권한으로 재현하지 않는다. 전역 권한이나 native 바이너리를 바꾸는 해결은 현재 범위가 아니다. 해당 경계에서 정상 재개를 입증할 수 없으면 근거를 남겨 HOLD하고 독립 작업을 수행한다.

## 4. 공식 native 계약과 미확인 범위

2026-09-14 공식 문서를 웹으로 확인했다. 문서상 지원과 이 머신의 Clauduct 통과 증거는 구분한다. 구현에 영향을 주는 버전 차이가 있으면 해당 공식 자료를 다시 확인한다.

- [세션 재개](https://code.claude.com/docs/en/sessions#resume-a-session): 명시적인 session ID로 프로세스 종료 후 같은 저장 대화를 재개할 수 있다. [복원 상태](https://code.claude.com/docs/en/sessions#what-a-resumed-session-restores)상 진행 중이던 일반 도구의 출력까지 자동 복구되는 것은 아니다. 일부 launch 설정은 재개 때 다시 제공해야 하므로 기존 승인 범위의 실행 구성을 대조한다.
- [Workflow 재개](https://code.claude.com/docs/en/workflows#resume-after-a-pause): 같은 세션의 저장 결과를 재사용한다. 중단 당시 실행 중인 agent는 다시 시작하며, 실패하거나 prompt가 달라진 지점 이후의 완료 agent도 다시 실행될 수 있다. 살아 있는 이전 worker는 중복 실행을 막고, 결과 전체가 없으면 `nothing to resume`로 끝난다.
- [비대화형 재개](https://code.claude.com/docs/en/headless#continue-conversations): `-p --resume`으로 특정 대화를 이어갈 수 있다.
- [Checkpoint 한계](https://code.claude.com/docs/en/checkpointing#limitations): Bash/외부 효과 전체의 복구를 보장하지 않는다. agent 재실행 허용과 파일 효과 중복 방지는 따로 검증한다.

직전 실행 기록의 환경은 Node24.19.0/Claude2.1.270/Codex0.154.0이다. 현재 설치 상태를 다시 실행해 확인한 것은 아니다. `CLI_VERSION_UNVERIFIED`는 기존 비차단 안내이며 그 문구만으로 실패로 세지 않는다.

## 5. 재사용할 증거

증거 디렉터리는 **구현 worktree의 `.tmp/release-completion-20260914/`**다. 표의 짧은 파일명은 이 디렉터리 기준이며, `.tmp/`로 시작하는 경로는 구현 worktree 기준이다. 원자료를 필요한 필드만 읽고, 예전 결과를 새 코드의 결과처럼 바꾸지 않는다.

| 증거 | 경로 및 적용 범위 |
|---|---|
| 통합 과거 판정 | `.tmp/release-completion-20260914/shipping-evidence.json`. 당시 F22 포함 판정은 이력 |
| 회귀 68파일 | 같은 디렉터리의 `candidate-regression-summary.json`. selectedFileChecksPassed=true, 최초 전체 runner timeout과 aggregateRunnerPassed=false 보존 |
| 배포본 관련 회귀 | `package-related-regression.json` |
| 실제 개발 | `development-verified.json`. 두 조합에서 모델이 두 파일을 구현하고 각 독립 81개 검사 통과 |
| 일반 Agent / 신규 Workflow 실호출 | `live-agent-final/result.json`, `live-workflow-final/result.json` |
| 공개 native 출력 단절 복구 | `archive-output-recovery-sol.json`, `archive-output-recovery-luna.json`. 동일 세션 복구·재쓰기0, backend0 |
| 실패/취소·선택 | `failed-completion-final-regression.json`, `native-agent-cancel-verified.json`, `selection-required-regression.json` |
| 패키지/동일 바이트 | `final-package-verified.json`, `final-runtime-equivalence.json` |
| 이전 프로세스 회수 | `final-process-census.json`은 과거 snapshot. 현재 소유 프로세스는 실행 전에 다시 확인 |
| 부모 중계 실제 성공 | `.tmp/native-headless-c0a4d46816e24aeaa2c84814c8ed6f13/result-single.json`(luna/max15요청), `.tmp/native-headless-c5f0f183a82f4bd986ca0f6b52ad24d4/result-single.json`(sol/low14요청). 고정 성공 fixture의 중계/동일 부모 재개 PASS, 직접 notification0 |
| Workflow 재개 실패 | `.tmp/workflow-cache-miss-local-47d2843a848141c7a5067012fa952be2/entry.mjs`, `observation.json`, `result.json`. 최초 완료 true, 두 번째 시작 false, backend0 |

위 여섯 실제 Agent/Workflow/개발 실행은 26요청·입력111655·출력4653, wrapper 합계253031ms였다. 부모 중계 두 실행은 29요청·입력138460·출력1815였다. 합산 입력은 개별 CLI 최종 응답 usage가 아니라 fixture 전체 footer/원장으로 집계한다.

기존 로컬 ZIP:
`.tmp/release-artifacts-7c475d04656848a8940a9ec69ed5d502/Clauduct-fea90000ef01.zip`

- SHA256: `72068e06b257973e92eea01801f465dbbaa21f352c2a501d5f7a0c957435e3a8` — 이번 인계 작성 때 재확인.
- 기존 manifest 기준 208파일. 실제 모델 실행 ZIP은 `91e72bd`였고 `fea90000` ZIP까지는 문서 2개만 달라 실행 파일 206개가 동일했다.
- 이 증거는 다음 제품 수정에 자동 적용되지 않는다. 최종 후보를 다시 커밋하고 그 후보와 필요한 증거를 연결한다.

## 6. 예산 집행과 목표 도구

최신 실제 원장은 `.tmp/release-completion-20260914/live-agent-final/result.json`이다. 시작 시 더 최신의 적법한 실행 기록이 있는지 대조한다.

| 기준 | 기존 누적 상태 |
|---|---:|
| 요청 | 323~327, 보수적으로 327 사용 |
| 관측 입력/출력 | 1124010 / 47690 |
| 이전 미관측 입력/출력 예약 | 786432 / 196608 |
| 예약 포함 입력/출력 | 1910442 / 244298 |
| 관측 native 시간 + 미관측 예약 | 3019404 + 160000 = 3179404ms |

이번 추가 상한은 별도 기록하고 기존 소비·미확정 예약을 초기화하지 않는다. 이 snapshot 그대로 시작하면 요청 누적 최대391, 예약 포함 입력 최대2310442/출력 최대274298이다. 인계 후 다른 실행이 생겼으면 실제 잔여량을 먼저 재산정한다.

다음 세션 시작 시 최초 작업 시각과 3시간 deadline을 기록한다. 읽기·조사·검토·도구 대기·정리도 포함하며 context 교체나 재시작으로 예산을 리셋하지 않는다. 종료/보고 시간을 남기고 완료할 수 없는 신규 실행은 시작하지 않는다.

실호출 전에 실행별 요청·입출력·시간을 예약한다. 재시도와 실패한 시도도 포함하고, 관측이 없는 실행은 예약을 보수적으로 유지한다. 반환 후에만 한도 초과를 발견하는 방식으로 준수를 주장하지 않는다. **기존 코드에 출력32768 전제가 있어 신규 총상한30000과의 정합성을 먼저 확인해야 한다.** `verification/fixture-tool-policy.mjs`, `verification/native-development-entry.mjs`, `verification/verify-native-development.mjs`, `verification/read-transport-progress.ps1` 등이 관련된다. 예산을 지키는 사전 제한과 관측 방법이 확인되지 않으면 해당 실호출은 하지 않는다.

현재 `src/run-node-tests.ps1`의 timeout 허용 최대는60초, `verification/verify-native-headless.ps1`은120초이며 기본 RequestLimit16이다. 존재하는 인수와 효과를 읽고 사례에 맞게 사용한다. 출력이 잘린 검사나 timeout을 성공으로 세지 않는다. 완료된 동일 검사를 근거 없이 다시 반복하지 않는다.

시작할 때 목표 도구로 현재 상태를 확인한다. 이 준비 세션의 기존 목표는 blocked이며 오래된 장기 시험 objective가 남아 있었다. 새 세션에 미완료 목표가 없으면 이 문서의 변경된 최신 목표를 등록한다. Clauduct 검증 토큰 상한을 담당 Codex의 goal token_budget으로 혼동하지 않는다. 기존 미완료 목표가 새 등록을 막으면 완료로 속이지 말고 로컬 진행 기록을 사용한다. 목표 complete는 전체 PASS일 때만 사용하며, 시간 종료를 실제 기능 완료로 표시하지 않는다. blocked 상태 변경은 호스트 도구의 조건을 따른다.

## 7. 커밋·릴리즈·최종 보고

의미 있는 수정 하나 → 해당 변경을 반증할 검사 → 의도된 diff 확인 → **그 변경만 명시적으로 stage하고 커밋** → 진행 기록 갱신 순서로 진행한다. 구현 worktree의 기존 branch를 유지한다. 사용자 루트 변경, auth/profile, 개인 세션, 실행 증거 임시 디렉터리를 일괄 stage하지 않는다. 검증 전 변경을 완료 커밋으로 포장하지 않는다. 이 인계 파일은 준비 시점에 uncommitted로 남긴다.

프로젝트 코드의 소유·데이터·도구 실행 경계를 검토하고, 악성 입력 시험은 외부 전송 능력과 무관한 비밀을 제거한 로컬 fixture로 수행한다. 실호출 payload는 새 공개/합성 fixture와 검토한 비밀 없는 필요한 코드만 사용한다. 기존 로그인 사용은 인증 값의 출력·복사·profile 입력 복제 권한이 아니다. 이전 guard/소유권/원장 등록 거부를 우회하지 않으며, 과거 PID만으로 현재 프로세스를 종료하지 않는다.

전체 적용 요구의 증거가 있으면 커밋된 최종 후보에서 `verification/build-release.ps1`로 로컬 ZIP을 만들고 manifest·hash·새 경로 실행·변경 영향 검증을 연결한다. 현재 build-release는 tracked clean 후보를 요구한다. 이전 ZIP 또는 다른 코드의 PASS를 새 후보에 복사하지 않는다. 실패 원자료는 보존하고 새 결과와 연결한다.

진행은 사용자 루트의 `docs/unattended-release-progress.md` 최신 부분과 구현 worktree의 `docs/release-completion-2026-09-14.md`에 남긴다. 다음 행동·commit·사용량·남은 필수 항목을 기록해 세션이 바뀌어도 이어갈 수 있게 한다.

최종 답변은 한국어로 전체 PASS 또는 HOLD를 먼저 밝히고, 적용 범위/제외, 수정과 커밋, 각 검증의 실제 관측, ZIP 경로·해시, 예산 사용과 잔여, 미실행·차단·재개 조건을 제시한다. 전체 PASS가 아니면 OK/PASS/FINAL로 포장하지 않는다. native의 정상 재개가 해결되지 않거나 다른 현재 필수 공백이 남으면 HOLD다. 로컬 릴리즈와 외부 게시·배포는 별개이며 후자는 현재 범위에 없다.
