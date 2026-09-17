# Session 32 — 열려 있는 검증 항목 3건

## 목표

세션 31이 닫지 못하고 **열린 상태로 기록한 3건**을 진행한다. 셋 다 곧바로 구현할 대상이 아니라 **먼저 결정이 필요한 항목**이다. `/grilling`으로 각 건의 결정을 사용자와 확정하고, 확정된 것만 구현·검증·PR 한다.

세 건은 서로 독립이다. 하나가 막혀도 나머지는 진행한다.

## 저장소 상태 (지금 참인 것)

- main `0ee70b4`. 로컬·원격 동기화, 열린 PR 없음, CI 초록.
- 작업 브랜치 `work/session-32-open-items`가 이미 생성·푸시돼 있다(main과 동일 커밋). 시작점으로 쓰거나, 건별로 따로 브랜치를 내도 된다.
- 워크플로는 `push: branches: [main]` + `pull_request:`다. **커밋당 CI 런이 1개**이며, PR이 열리지 않은 브랜치는 빌드되지 않는다.
- 마지막 전수 통과: 로컬 스텝1 `tests 130 / pass 129 / fail 0 / skipped 1`(67.2초), 스텝2 10/10, 스텝3 6/6.
- `verification/test-dotnet-http-transport.mjs`는 정상 통과에도 **49초**가 걸린다. `deadline-45s`가 의도적으로 45초를 쓰기 때문이며 지연이 아니다.

## 항목 1 — `deadline-45s` 상한 7초 여유

**상태: 미판정. 새 증거가 없으면 결정을 강행하지 않는다.**

`verification/test-dotnet-http-transport.mjs:215`:

```js
if (testCase.mode === 'deadline-45s') assert(elapsedMs >= 44000 && elapsedMs < 52000);
```

예산 자체는 `verification/DotnetHttpProbe.cs:209`의 45000이다. 상한 여유가 7초뿐인데, 세션 31이 이 러너에서 **프로세스 기동 17~40배 지연**을 관측했다. 지연이 이 케이스 도중에 발생하면 어서션이 **정당하게** 실패할 수 있다.

넓히지 않은 이유: 이것은 가드가 아니라 **검증 대상**이다. 하한 44000은 "일찍 포기하지 않았다", 상한 52000은 "아예 타임아웃이 안 난 게 아니다"를 각각 확인한다. 임의로 넓히면 후자가 약해진다.

선택지 3개와 각각의 대가는 `docs/audit-2026-09-15-ci-spawn-latency.md` 5-2에 적혀 있고 **어느 것도 채택되지 않았다.**

**판단에 필요한 것은 실제 실패 1건의 `elapsedMs`이며, 아직 없다(미측정).** 그 값이 52000을 얼마나 넘는지에 따라 선택이 갈린다. 증거 없이 상한을 넓히는 것은 검증 약화다. grilling에서 "지금 결정하지 않고 재발을 기다린다"도 유효한 결론이다.

## 항목 2 — `poc/test-claude-read-once.mjs`의 `read_extra_normal`

**상태: CI에서만 실패. 워크플로에서 제외 중. 다음 수순이 이미 문서에 적혀 있다.**

- 제외 위치: `.github/workflows/tests.yml:43`
- 실패 지점: `poc/test-claude-read-once.mjs:90`의 `assert.equal(result.resourcesClosed, true)`
- `resourcesClosed`는 `poc/claude-read-once.mjs:117`에서 **9개 조건의 논리곱**이다 — `activeSockets`, `activeJobs`, `activeTimers`, `activeDeliveries`, `busy`, `transport.activeRequests`, `transport.activeSockets`, `localSessionSecretCleared`, 자식 종료.
- **어느 조건이 false인지 기록하지 않는다.** `docs/verification-modes-2026-09-14.md:154`가 "이어받을 때는 그 진단부터 넣는 것이 순서"라고 남겼다.

세션 31의 프롬프트는 이 건을 "이번 작업 대상이 아니다"로 명시했다. **이제는 대상이다.**

권장 순서는 세션 31이 `sse-fixed` flake에서 실제로 통한 방법과 같다 — **먼저 실패가 스스로 원인을 말하게 하고**(어느 조건이 false인지), 그 값을 보고 고친다. 진단 없이 원인을 추정해 고치면 세션 31이 겪은 "증상만 옮겨 다니는" 반복이 재현된다.

로컬에서는 통과하므로 진단을 넣은 뒤 CI에 올려 값을 받아야 한다.

## 항목 3 — 마크다운 링크 검사 부재

**상태: 미채택 제안. 셋 중 가장 작고 확실하다.**

`verification/test-doc-citations.mjs`(PR #14, 세션 31)가 `파일:줄` 인용만 검사한다 — 정규식은 그 파일 `:12`. 검사 항목은 파일 존재·범위·빈 줄·역순 범위·저장소 밖이다.

**마크다운 링크는 아무것도 검사하지 않는다.** PR #6이 worktree를 가리키던 끊어진 링크 4건을 고쳤지만 그때 쓴 검사기는 커밋되지 않았다. 같은 결함이 다시 나면 또 조용히 지나간다.

기존 검사기에 링크 해석을 붙이는 것이 자연스럽다(문서 85개가 이미 스캔 대상이다). 결정할 것은 범위다 — 저장소 내부 상대 경로만 볼지, 앵커(`#heading`)까지 볼지, 외부 URL은 어떻게 할지(네트워크 요청은 하지 않는 것이 이 저장소 관례다).

## 제약 (세션 31에서 그대로 유효)

- **사용자의 기존 untracked 파일은 임의 삭제·stage·덮어쓰지 않는다.** `HANDOFF.md:141` 참조. 현재 untracked 32개, worktree 6개가 있고 전부 유지 대상이다. `verification` 아래에는 tracked 자료와 untracked 작업이 섞여 있다.
- **`git add -A`나 `git add -- docs`를 쓰지 않는다.** 커밋할 때는 변경한 파일을 하나씩 이름으로 stage한다.
- **권한 계층이 거부하는 것들**: `git push origin --delete`, `git reset --hard`, `git push --force-with-lease`, `C:\Users\js\.agents\scripts\claude` 쓰기. **다른 도구로 우회하지 않는다.** 원격 브랜치 정리는 사용자에게 명령을 제시한다. 리베이스를 발행해야 하면 force-push 대신 이전 원격 팁을 머지해 fast-forward로 만든다(세션 31이 쓴 방법).
- `verification/fixture-token-budget.mjs:12-14`가 `maxOutputTokens`를 기본 32768로 두고 그 위를 거부한다(세션 31 프롬프트는 이 위치를 `:13`으로 적었으나 그 줄은 `maxInputTokens` 검사다). **이 상한을 완화해 통과시키지 않는다.**
- `src/agent-selection.review-fixture.mjs`는 **이전 코드의 실행경로 검증용 복사본(166행)**이다. 그 안의 발견을 현재 코드의 회귀로 오인하지 않는다.
- **AdGuard가 켜져 있으면** loopback HTTP test 5건이 실패한다(`test-chat`, `test-http-close`, `test-native-gateway`, `checkExpectation`, `invalid-method`). 제품 결함이 아니다. 개발 중에는 끄는 것이 맞다. 판별법은 `docs/native-startup-diagnostics-2026-09-14.md`.
- 문서를 쓰면 `파일:줄` 인용이 CI 검사를 통과해야 한다. 인용한 문서가 미추적이면 `HANDOFF.md`의 인용 예외 규칙에 따라 함께 커밋한다.

## 세션 31이 남긴 것 (배경)

CI가 간헐적으로, 매번 다른 지점에서 실패하던 원인을 닫았다 — **바운드가 따뜻한 개발 머신 기준이었고, 러너는 프로세스 기동을 17~40배 지연시킨다.** PR #8(진단) → #9(pwsh spawn 가드 3곳) → #11(중첩 7초 가드가 바깥 예산을 가로챔) → #10(커밋당 런 2개→1개). 전 과정과 배제한 가설, 정정한 판독은 `docs/audit-2026-09-15-ci-spawn-latency.md`에 있다.

이어서 `docs/audit-2026-09-15-effort-routing.md`의 코드 인용 24건 중 15건이 엉뚱한 줄을 가리키던 것을 고치고(PR #13), 재발 방지로 `파일:줄` 검사기를 넣었다(PR #14). **그 검사기는 15건 중 1건만 잡았다** — 나머지는 "존재하는 다른 코드"를 가리켜 기계가 구분하지 못한다. 한계를 알고 쓴다.

## 참조 문서

- `docs/audit-2026-09-15-ci-spawn-latency.md` — CI 기동 지연 전말, 5장에 항목 1이 열린 채로 기록
- `docs/verification-modes-2026-09-14.md` — 검증 모드 전수, `:144` 이하가 항목 2
- `docs/audit-2026-09-15-effort-routing.md` — effort 경로 판정(세션 31의 원 과제)
- `HANDOFF.md` — 보존 규칙과 인용 예외

## 완료 기준

3건 각각에 대해 **결정이 확정되고, 확정된 것이 구현·검증되고, 확정하지 않기로 한 것은 그 이유와 재개 조건이 기록되면** 완료다.

- 측정 없이 추정한 항목은 **추정으로 표시**한다.
- 항목 1은 새 증거(실패 1건의 `elapsedMs`)가 없으면 **결정을 강행하지 않는다.** "기다린다"가 결론이면 그렇게 기록하고 닫는다.
- 검증 어서션을 약화시켜 통과시키지 않는다. 가드와 검증 대상을 먼저 구분한다.
