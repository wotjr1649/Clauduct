# 배포 완료 및 native 시작 진단 인계 — 2026-09-14

## 다음 세션의 목적

독자는 같은 Windows 로컬 파일 시스템과 PowerShell/Git 도구를 사용할 수 있는 Codex 세션이다. 이번 세션의 설치 개발·PR·리뷰·머지·Release 게시 목적은 완료했다. 다음 작업은 실사용에서 남은 `claude-code:unrecognized_model`과 `transportRejections: 2`의 원인을 구분하고 필요한 최소 수정을 구현·검증하는 것이다. 아직 두 진단의 수정이나 새 브랜치 생성은 하지 않았다. 단순히 출력을 숨기거나 거부 횟수를0으로 만드는 작업이 아니다.

## 확정된 소스와 작업 폴더

- 사용자 루트: `D:\AIDEV\Clauduct`. HEAD `aa317c75c7bad0cf9d16641ca7db4edb43016c19`, 브랜치 `fix/native-completion-resume`. README 미커밋 변경과 다수 untracked 자료가 있으므로 그대로 보존한다. README SHA256 `38f3d4252880099592e73987c81c04f60c93e787ca482aaf185f2f4bd1496c08`.
- 기존 구현: `D:\AIDEV\Clauduct\.tmp\unattended-release\implementation`, 브랜치 `work/unattended-release-2026-09-13`, HEAD `c47ccb78b84907407c48965817b4322c385413be`. tracked clean, 기존 `.tmp/` 증거는 untracked로 보존.
- main 작업 폴더: `D:\AIDEV\Clauduct\.tmp\unattended-release\published-main`. `main`을 이미 checkout하고 있으며 tracked clean.
- 로컬 `main`, `origin/main`, `v0.1.0`은 인계 시 `4f3e37535075b662de8d23cfed8f6bf8e23f3933`로 일치했다. 원격 freshness는 다음 세션에서 읽기 전용 확인한다.
- origin: `https://github.com/wotjr1649/Clauduct.git`.
- PR #1: https://github.com/wotjr1649/Clauduct/pull/1 — MERGED. 자체 리뷰 COMMENTED이며 독립 사람/별도 에이전트의 승인은 아니다. 당시 등록 CI 없음, MERGEABLE/CLEAN, 보호 우회 없이 정확한 head 조건으로 merge.
- Release: https://github.com/wotjr1649/Clauduct/releases/tag/v0.1.0 — 게시 완료.
- 다음 수정은 최신 main에서 `fix/native-startup-diagnostics` 등 새 브랜치와 별도 worktree를 만드는 것이 권장된다. 사용자 루트를 main으로 강제 전환하지 않는다. 새 이름/경로의 기존 상태를 확인하고 충돌하면 보존하며 다른 이름을 선택한다. 기존 main worktree를 무시하는 강제 checkout, reset/stash/clean은 사용하지 않는다.

## 완료한 변경과 증거

- `4224365`: native Claude 및 standalone/npm Codex 탐색, 설치 전 CLI version 확인. credential home은 OS 사용자에 고정, 개인 profile 복제 없음.
- `185a16e`: install.ps1, `.local\bin\clauduct.cmd`, 커밋별 versions 폴더, 업데이트/이전 버전 보존, hash/경로/충돌/잠금 검증, 배포 builder.
- `ec261e2`: 공개 리뷰 기록과 README/설치 설명 정리.
- `c47ccb7`: 설치 검사기가 fixture cwd에서 결과를 출력하여 상대 Tee-Object 경로 기록이 실패하던 문제 수정. 결과 출력 전에 원래 cwd로 복귀한다.
- merged commit `4f3e375`은 위 PR head와 tree가 동일하다.
- 핵심 회귀14파일 PASS: credential-recovery, credential-store-selection, collected-result-relay, workflow-resume, failed-completion-resume, completion-selection, workflow-selection, native-protocol, request-admission, fixture-token-budget, runtime-discovery, runtime-paths, install-check, launcher-native. 모두 `src/test-<name>.mjs`. 기존 runner `src/run-node-tests.ps1`, 60초 제한에서 약9.9초 완료. 시험 내부의 기존 symlink NOT_RUN은 유지.
- `verification/test-installer.ps1`: 공개 fixture26검사 PASS. 실제 사용자 PATH 쓰기0.
- 실제 CLI 버전 확인: Node24.19.0, Claude2.1.270, Codex0.154.0.
- merged ZIP227파일/2197467bytes, 설치 후 모든 파일 SHA 대조, 다른 cwd의 clauduct/Clauduct dry-run, 재현 ZIP 일치.
- 공개 latest bootstrap 다운로드 후 hash 확인 및 PowerShell7.6.6에서 실제 온라인 설치(프로젝트 내부 -NoPathUpdate) PASS. 온라인 설치227파일 hash 및 두 명령 dry-run PASS. GitHub4asset digest도 로컬과 일치.
- 릴리즈 로컬 폴더: `D:\AIDEV\Clauduct\.tmp\unattended-release\published-main\output\releases\4f3e37535075-060181a442b94b9c9c380e70bea42493`.
- asset: install.ps1, Clauduct-windows-x64.zip, manifest.json, SHA256SUMS.
- ZIP SHA256: `61738c6e9a03f604421cb132eed14b8c4dcd587b1ace5291ab96b16b3617bdd6`.
- 실행 코드가 이전 설치 후보185a16e와 동일하다. packaged 변경은 RELEASE.md/docs/installation.md/verification/test-installer.ps1만이었다.
- 최종 근거: `D:\AIDEV\Clauduct\.tmp\unattended-release\implementation\.tmp\publish-review\publication-verdict.json`, SHA256 `9a8b40cd9781f4011d4f35c0f10b061cad456e62a08f89d19ed2524270dccee0`. 같은 폴더에 회귀/PR/Release/설치/온라인 검사 JSON이 있다.
- 이전 설치 증거: implementation/.tmp/installer-release/. 이전 비용 원장/무인 판정: implementation/.tmp/session-29-release/. 누적 진행 기록: `D:\AIDEV\Clauduct\docs\unattended-release-progress.md`.
- main의 문서: docs/public-release-review-2026-09-14.md, docs/installation.md, docs/local-use-release-decision.md, docs/session-29-release-verdict.md.

## 사용자 제공 후속 설치 증거 — 앞선 NOT_RUN 기록의 보완

사용자가 현재 머신 PowerShell7과 다른 머신 Windows PowerShell5.1의 온라인 설치 결과를 제공했다. 둘 다 installed=true/sourceCommit=4f3e375, 사용자 홈 .local/bin, dependency passed=true, 위 동일 CLI 버전, loginChecked=false/modelRequests=0이었다. 사용자명 js/JS 차이는 각 머신 홈의 정상 차이다. 두 경우 pathUpdated=false/restartParentTerminal=false로 사용자 PATH에 이미 항목이 있었다. 따라서 PATH 신규 쓰기를 검증한 증거는 아니다.

PowerShell5.1의 정상 설치는 이제 사용자 제공 성공 증거가 있다. 에이전트가 이전 개발 머신에서 겪은 UnauthorizedAccess 실행 정책 차단과는 별개이며 그 거부를 재시도/우회한 것이 아니다. 기존 publication-verdict.json/Release 문서는 이 사용자 보고 이전 snapshot이므로 수정 없이 보존하고, 새 기록에서 증거 출처와 시간 순서를 구분한다. 다른 모든 머신의 호환성 PASS로 확대하지 않는다.

## 사용자 제공 실제 실행 — 원문 세션 복제 없이 필요한 사실만

실행 명령: `clauduct --model luna --effort low --print "hello"`.
Exit0, 약8.6721초, 정상 인사말 출력. 설치본 `4f3e375`의 `.clauduct-status/request-status.jsonl` 경로를 안내했다. 두 요청 모두 gpt-5.6-luna/low, 각 HTTP200/completed=true, lifetime started2/succeeded2/failed0, requestOutcome=all-succeeded, 실패이력 빈 배열, 모든 cleanup=true. 첫 응답은 toolUse1/thinking1/text0, 두 번째는 text1/thinking1. 이 요약에 도구 이름은 없으므로 무엇을 실행했는지 추정하지 않는다.

동시에 아래 메시지가 있었다.
- `CLI_VERSION_UNVERIFIED detected=0.154.0 reference=0.153.4`: 기존 비차단 version 안내. 이번 이슈의 핵심 두 항목과 구분한다.
- `[claude-code:unrecognized_model] {"model":"gpt-5.6-luna","query_source":"sdk"}`.
- `lifetime.transportRejections=2`, rejectedBeforeStart=0, firstRejectedCategory=null, rejectedCategories={}, transport 실패로 기록된 모델 요청0.

답변 성공을 진단 문제가 없다는 의미로 확대했던 설명은 보완했다. 두 항목 모두 조사 대상이며 원인은 아직 확정되지 않았다. 개인 status/session 원문을 무단으로 읽거나 인계/공개 fixture에 복제하지 않는다. 이 문서의 고정 사실만 먼저 사용한다.

## 확인한 발생 경로와 다음 구현 방향

1. `src/native-gateway.mjs` 약592행: transportReject가 rejected와 lifetime.transportRejections를 증가시킨다. clientError, connect, upgrade, checkContinue, checkExpectation 이벤트 모두 같은 숫자로 합산하며 clientError의 error는 버린다. 따라서 backend 실패2회가 아니며 기존 로그만으로 두 건의 종류나 발신자를 복원할 수 없다. 정상 탐색/건강검사라고 단정하지 않는다. 같은 socket의 여러 이벤트인지도 미확인이다.
2. 고정 event label/허용된 parser error code별 bounded 진단을 추가하고 src/request-status.mjs와 관련 호출자/검사를 함께 확인하는 방향이 적절하다. 원문 헤더/URL/query/body/토큰/Node 오류문구는 기록하지 않는다. 기존 합계와 요청 실패 의미를 보존하고 필요한 경우 사건수와 연결수를 구분한다.
3. `src/test-native-gateway.mjs`에는 Expect:100-continue가417로 거부되고 transportRejections1/rejectedBeforeStart0이 되는 기존 검사가 있다. 이 거부 자체는 의도된 보호다. 정상 실행 중 발생 이유를 밝힌 뒤 수정 여부를 정하고 원인 규명 없이 CONNECT/Upgrade/Expect 허용으로 바꾸지 않는다.
4. unrecognized_model은 프로젝트 소스에서 문자열 발생기를 찾지 못했다. native 경고 발생 조건과 공식 custom model 설정/metadata를 먼저 확인한다. 설치된 native code의 task-needed 비밀 없는 정적 확인이나 공개 공식 자료를 사용하고 출처/버전을 기록한다. CLI 바이너리/전역 설정을 직접 패치하지 않는다.
5. native가 GPT를 인식하지 않을 때 컨텍스트/출력한도/기능 선택의 fallback에 실제 영향이 있는지 확인해야 한다. 단순 출력 필터 금지. 경고만 없애려고 GPT를 Claude identity로 위장하지 않는다. docs/native.md의 과거 modelPicker.behavesAs 제거 이유(Claude 알려진 용량으로 덮이는 문제)를 보존한다.
6. 관련 시작점: src/native-gateway.mjs, src/request-status.mjs, src/native-protocol.mjs, src/clauduct.mjs, src/test-native.mjs, src/models.mjs, src/runtime-paths.mjs 및 corresponding tests. 미래 line번호는 재검색한다.

## 범위·비용·권한 경계

이번 문서는 상태 인계이며 새 권한을 만들지 않는다. 완료한 PR1/v0.1.0의 원격 작업은 사용자가 그 작업에 명시 승인했다. 다음 patch의 원격 push/PR merge/Release는 별도 후속 효과로 범위를 확인하며, 이전 승인만으로 v0.1.0 덮어쓰기·전역 설정 변경·보호 우회를 정당화하지 않는다. 다음 세션의 기본 산출물은 새 브랜치의 로컬 수정·검사·커밋과 근거다. v0.1.1은 제안된 다음 배포 번호이며 아직 생성/승인된 결과가 아니다.

일반 로컬 구현 판단은 자율적으로 수행하되 개인 profile/auth/전역 settings를 수정하거나 복제하지 않는다. native 모드 범위는 현재 bypass만이다. 디스크 부족 복구·일반 HTTP/TCP 안정성·동적 링크 실행검사·정상 auth갱신·기본400K/320K 실호출·장기 시험은 이전 제외를 되살리지 않는다. 이번 transportRejections의 좁은 원인 진단은 사용자 후속 요청 범위이며 일반 TCP 안정성 재검증과 구분한다.

설치/공개 작업 중 에이전트 추가 Clauduct 실모델 요청/입력/출력은0/0/0이었다. 이후 사용자의 hello 실행에는 성공 upstream2요청이 보이나 token usage는 제공되지 않았다. 이를 이전 원장에0토큰으로 합산하거나 원장 snapshot을 고치지 말고 별도 사용자 실행·토큰 미관측 사실로 유지한다. 과거 보수 누적327/1910442/244298과 미관측 예약 및 누적 요청cap391은 역사 snapshot이다. 이전3시간/추가64요청/입력400000/출력30000 과제는 종료되어 goal blocked이며 새로운 세션 예산으로 자동 재사용하지 않는다. 먼저 정적·로컬 공개 fixture로 진행한다. 새 모델 실호출이 필수라면 유효한 실행 권한과 잔여 비용 상한을 확인한 뒤 정확한 부족 항목만 사용자에게 제시한다. 원장/예약을 context 변경으로 초기화하지 않는다.

이번 배포 목적 완료와 과거 전체 무인 안정성 HOLD/goal blocked를 구분한다. 미완료 옛 goal을 complete로 표시하지 않는다. 새 goal은 명시 요청 없이 만들지 않는다.

## 완료 기준

두 진단 각각에 관측된 원인/재현/영향/수정 또는 유지 근거가 있고, 로컬 회귀·음성 검사가 기존 보호를 보존하며 통과해야 한다. 모델 경고를 안전하게 해소할 수 없으면 정확한 외부 제약과 미확인 범위를 남긴다. 실제 native에서 재현하지 않은 결과를 실사용 해결로 표시하지 않는다. 수정 단위의 의도한 diff만 명시 stage/commit하고 변경/증거/추가사용량/남은작업을 기록한다. 최종 보고는 한국어로 브랜치·커밋·검사·잔여문제를 설명한다.
