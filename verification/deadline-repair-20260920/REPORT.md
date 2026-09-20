# Deadline・Workflow follow-up evidence

This report separates the original failure, its correction, and the observed
acceptance scope. A later green run does not erase a prior failure.

## Identity

- Final source: `3df87d369445796ba94da7880d0e4852247d3d90`.
- Final SHA256: `e43235288e3cd9c1b634bfb94a81af1ffb6bda58ba35df324aed066cbbe3cafb`.
- Go `1.27.1`, Windows amd64; native Claude Code `2.1.278`.
- [Build manifest](build.json) records all three artifacts and clean VCS stamp.
- Original product: `01614e0`, SHA256 `32086c7b87d5f2222590537962507293394b19e6f1790d2000209dc08b8c892e`.

## Deadline failure: reproduced and corrected at its owning layer

The original `TestSessionDeadlineDrainsBeforeStoppingAndBoundsStalledWork`
failure was not an unbounded native TUI session. Its fake process returned from
`Wait()` after closing a `stopped` channel while its independent HTTP goroutine
could still be blocked in `Client.Do`. The test therefore reported a reaped
child without actually ending the work that represented that child.

Fresh repetitions reproduced `request leaked` three times. In one, Run returned
at about 204ms with active requests zero; the client was still waiting for headers
at 15s. Bounded goroutine and connection inspection found client persistConn
loops and a remaining client endpoint, not a live gateway handler.

The corrected fake owns its HTTP request cancellation; `Stop()` cancels it and
`Wait()` joins it. The test now additionally checks that no child-owned request
exists after `NativeReaped`. Completed-turn delivery, drain ordering, grace
expiry, checkpoint failures and elapsed-time assertions remain. No production
deadline value, assertion timeout, retry or security setting was relaxed.

After that change, 80 grace-expiry cases across CPU settings 1 and 4 passed;
the full suite passed. The `53482a4` suite has 1,584 passing test/subtest
nodes, 17 passing packages and zero failures. Three opt-in tests remained skipped:
console close, account-live-session and launcher-kill. These skips are not passes.
`go vet ./...` also passed. [Machine evidence](evidence.json) retains failed and
successful runs separately, with hashes; large raw logs stay local.

Actual native TUI with the product lifecycle and a controlled upstream fixture:

| Variant | UUID | Observed outcome |
|---|---|---|
| Upstream remains stalled | `d26d86e4-4c28-4ad4-a3c6-1ab3193aae43` | Active request at deadline; grace expires; native reaped 30ms after grace deadline; active zero; no cleanup error |
| Response completes during grace | `48457572-77c6-441e-9b37-f6a9d55a4beb` | `PUBLIC_DEADLINE_COMPLETED` visible; native reaped 3.034s after deadline, 6.966s before grace end; no cleanup error |

Both correctly report `SESSION_DEADLINE`, not successful task completion.
They use native TUI and the real launcher lifecycle but a fixture upstream;
they do not establish subscription-backend token accuracy.

## Separate Windows TCP counterexample

A standalone Go TCP/HTTP experiment, with no Clauduct imports, also reproduced
occasional peer notification/data loss under immediate server close. The precise
Windows/provider/Go cause is not established. `SetLinger(1)` eliminated observed
loss in its sampled cases but increased 200 HTTP iterations to about 23.48/27.47s.
It was **not** installed as a product fix. Neither those samples nor the corrected
fake process prove reliable immediate remote-close notification in every Windows
network stack. The product's owned native process does not depend on that
notification to terminate: Stop terminates its process tree and Wait reaps it.

Microsoft distinguishes socket handle closure from graceful data delivery and
documents positive-linger blocking behavior: [socket closure](https://learn.microsoft.com/en-us/windows/win32/winsock/graceful-shutdown-linger-options-and-socket-closure-2),
[closesocket](https://learn.microsoft.com/en-us/windows/win32/api/winsock2/nf-winsock2-closesocket).

## Actual backend baseline: additional failures preserved

UUID `1e82df3c-b997-41fd-af44-a7b45922069a` used the old product and the previously
approved isolated project/profile. It ended normally with API failures zero,
one cancelled request and one native tool failure. Normal exit is not acceptance.

- Before-body Esc: request 5 cancelled in exact-count stage, backend generation
  events zero. Native restored the submitted prompt to the composer. Clearing
  that draft and submitting new input produced exactly `S44_RECOVERED_29`.
- First B experiment: the test controller used ordinary CR in a multiline draft;
  its stop instruction was not submitted in time. B ran its natural 90s lifetime.
  This is a failed test, not OS cancellation evidence. Kitty Enter (`CSI 13;1u`)
  matched this TUI's negotiated input protocol; the second independent experiment
  used it. No product input handler was changed.
- Second B: a held process handle proves PID 11884 existed and terminated after
  16.467s of its requested 180s hold, following native TaskStop. A was reused,
  B was not rerun, and only C was created on resume.
- C nevertheless failed its assigned behavior: it made 12 tool calls while asked
  for a literal tool-free result. Its native user relay contained the parent's
  coordination request; its computed task contained the narrower C task. It
  interpreted itself as the coordinator. This is not counted as task success.
- Two late TaskStop validation errors returned `is_error` without a failure hook;
  the previous status count omitted them. A separate rejected Glob produced the
  one observed native hook failure. Zero API failures did not mean zero tool errors.

## Corrections

Verified plan children receive one fixed worker-role instruction before exact
counting and generation. It distinguishes worker execution from parent run
bookkeeping. It contains no dynamic saved prompt/result and does not promote
computed task text to user authority. Native user restrictions, role and tool
permissions remain in place. Ordinary Agent, unverified/foreign child, inline
Workflow and compaction requests do not acquire this instruction.

Tool-result observation now joins real assistant tool calls to `is_error`
results as well as native failure hooks. It stores only bounded identity and
source, deduplicates both sources and retains later cancellation evidence.
Controlled Workflow rejections retain their dedicated counter and original
history. Error-like text alone is not a failure. Result-only errors become
observable when the following request carries their history; this is not a
claim that every native validation failure emits a hook immediately.

## 실제 후보 TUI 검수

[기계적 감사 결과](candidate-evidence.json), [감사 코드](candidate-audit.mjs):
UUID `432ae808-ff52-435e-aed2-e0ca5c8c983a`, 제품 `53482a4`.

- 원본 `wf_1fe400a8-48d` / `wwluym7hn`: A Sol/high `S44_G_17` 반환,
  B Terra/medium 지정 Bash 한 번 실행.
- 열린 OS process handle로 PID 15048의 시작과 종료를 확인했다.
  `03:33:27.7258466Z` → `03:33:48.8848478Z`, 180초 예정 작업이 TaskStop 후
  21.159초에 종료됐다. 승인 대기 취소가 아니라 실행 중인 OS 프로세스 중단이다.
- 재개 `wf_e05d9f58-c37`: 새 자식 정확히 1개, Luna/max, 도구 호출 0,
  정확한 `S44_H_23` 반환. A 재사용, B 재실행 없음. B 결과가 없어 전체
  `complete:false`가 유지됐다.
- 중복 재개는 실행 없이 거부. 없는 TaskStop의 오류는 `source:tool_result`로
  한 번 집계됐다. 이후 `S44_FAILURE_RECOVERED_31`, `S44_FINAL_ALIVE_37` 응답 확인.
- API 실패 0, backend usage/사전 계수 일치 22건·불일치 0, 결과 미확보 0,
  native 정상 회수.

추가 권한 경계 시험은 자식에서 검증됐다고 표시하지 않는다. 첫 요청은 도구 계약에서 거부됐다.
모델이 이후 인용한 복원 이력에는 미지원 `args.constraints`가 있었지만, 이 인용은 모델 출력이며
당시 원본 wire payload를 저장한 근거는 아니다. 별도의 정확한 입력 시험은 제한 전파와 충돌한다며
부모가 위임 전에 거절했다. 자식 실행은 두 경우 모두 0이다. 시험을 통과시키기 위해 제한을
제거하지 않았다. 따라서 이 세션의 Workflow 거부는 2건이며, 이를 무오류 실행으로 표시하지 않는다.

## 추가 발견: 자식 계수와 생성의 도구 계약 차이

`count_tokens`는 공통 Agent 도구 설명 함수에 자식 ID를 전달하지 않았다. 생성은 ID를 전달해
상속된 모델·effort로 선택지를 좁혔으므로 계수한 도구 스키마가 실제 생성 스키마와 달라질 수 있었다.
`TestChildCountAndGenerationUseTheSameToolContract`로 수정 전 실패를 확인했고, 같은 ID를
전달하는 한 줄 수정 후 실제 HTTP 경로의 input/tools payload 비교가 통과했다. 임의 토큰 추정치를
맞추는 검사가 아니다. `3df87d3`에는 이 수정과 회귀 검사가 들어 있다.

1,584 test/subtest·17개 package의 전체 검사는 `53482a4` 결과이며, 최종 한 줄 수정 후에는
count/Workflow 역할/게이트웨이 취소 집중 검사와 vet를 실행했다. 이를 전체 검사 두 번으로 세지 않는다.

최종 바이너리 UUID `f55faaae-fef5-41c2-95fa-e6a8acbc1b76`에서는 다음을 실제 TUI로 확인했다.
[최종 감사](final-evidence.json), [재현 가능한 감사 코드](final-audit.mjs)

- `/context all` 두 번 모두 `10.1k / 500k`, Messages 19→14. 실제 backend 입력에서
  출처가 확인된 조회 보고서가 제외됐다. 키 입력~화면 paint 전체 지연의 정밀 측정은 아니다.
- 부모→Terra/medium 자식→모델·effort 생략 손자. 손자도 Terra/medium으로 실행됐다.
  손자 `S44_LEAF_41`, 중간 `S44_MIDDLE_41`, 부모 `S44_FINAL_41` 전달 확인.
  TaskOutput polling과 파일·명령 실행은 없었다.
- API 실패·도구 실패·결과 미확보 모두 0. backend usage/사전 계수 11건 일치,
  불일치 0. 최종 native 회수 완료, watchdog 종료 아님.

## 합격 범위와 남은 한계

deadline drain/reap, 관측된 A/B/C 중단·재개, 첫 본문 전 Esc 후 새 입력 회복,
도구 오류 집계·회복, 최종 바이너리 중첩 상속·결과 전달은 명시한 조건에서 합격이다.
자식 계수/생성 도구 계약은 직접 HTTP 비교로 확인했다. 세 개의 누락 문서 인용은
원본 보고서와 로그 hash 근거를 복구해 해결했으며 검사기를 수정하지 않았다.

모든 상황의 무결점 판정은 아니다. native 첫 텍스트 수신부터 terminal paint까지의 지연,
부분 도구 인자 생성 중 정확한 Esc 시점, 자연 발생 backend 빈 응답, 전체 멀티모달 형식,
모든 Windows 종료 모드, launcher 재시작을 넘는 범용 Workflow 복구를 새로 입증한 것은 아니다.
모델의 지시 준수가 모든 입력에서 결정적이라는 보장도 하지 않는다. 원본
`acceptance:not_assessed`는 유지하고 별도 감사에서 합격 범위를 기록한다.
Windows TCP close 통지의 독립 반례는 가짜 프로세스 소유권 수정과 구분하며 해결됐다고 쓰지 않는다.

개발 실행 경로 `D:\AIDEV\clauduct-s36-build\clauduct.exe`와 동반 바이너리 두 개를
최종 build manifest와 같은 파일로 반영했다. 기존 사람용 `interactive.mjs`는 새 manifest를
참조하도록 경로 한 줄만 변경했다. 이전 파일들은 `D:\AIDEV\clauduct-s36-build\before-3df87d3-20260920`
에 보존했다. [반영 기록](delivery.json). 설치 릴리스와 승인된 profile의 설정·신뢰는 변경하지 않았다.
