# Clauduct 인수인계

이 폴더를 받은 사람 또는 에이전트의 시작점이다. 프로젝트 디렉터리 전체를 옮기면 새 세션으로 개발을 이어갈 수 있다. **이전 대화의 resume은 별개이며 폴더만으로 보장되지 않는다** — 그것을 선행 조건으로 삼지 않는다.

이 문서는 자동으로 로딩되는 AGENTS.md/CLAUDE.md가 아니다. 새 세션에서 이 파일을 먼저 읽도록 지정한다.

## 1. 먼저 읽을 것

1. **이 문서** — 목표, 지켜야 할 선, 시작 순서.
2. **[현행 검증표](docs/v1/remaining-verification.md)** — 2~5장이 현행 기준이다. **무엇이 남았는지는 3.2절이 소유한다.** 6장은 이력이며 그 수치와 프롬프트를 현재 설정으로 쓰지 않는다.
3. **[무인 개발 심층 리서치](docs/v1/release/research-2026-09-12-unattended-release-gates.md)** — F01~F23의 정의와 합격 기준의 원본이다. F 번호를 판정할 때 요약이 아니라 이 정의를 본다. **F01~F23은 명세이지 기존 테스트 목록이 아니다** — 대응하는 검사가 전부 존재한다고 가정하지 않는다.
4. **[릴리즈 검증](docs/v1/release/release-readiness.md)**과 **[RELEASE.md](RELEASE.md)** — 실행한 항목, 오류 계약, ZIP 구성과 한계.

이 문서와 코드가 작업 근거다. 이전 대화, 사용자 홈 native memory, Engramux 검색은 보조 증거이며 없어도 개발을 이어갈 수 있어야 한다.

## 2. 목표와 현재 판정

최종 목표는 Anthropic 서버 전용 기능을 제외한 약속된 기능을 안정적으로 제공하고, API 오류·취소·재시도·자식 작업·결과 보존·자원 정리를 검증해 사람이 지켜보지 않는 연속 개발을 가능하게 하는 것이다. **'오류 시 안전하게 종료'와 '원래 과제를 사람 없이 완료'는 각각 따로 판정한다.**

판정은 둘로 갈린다.

| 범위 | 판정 |
|---|---|
| 로컬 실사용 | 사용 가능 — [판단 근거](docs/v1/release/local-use-release-decision.md) |
| 무인 전체 출하 | **HOLD** |

**남은 것의 목록을 이 문서에 복제하지 않는다.** [현행 검증표](docs/v1/remaining-verification.md)의 3.2절이 그 목록을 소유하며, 사용자 범위 변경이 반영된 유일한 자리다. 여기에 사본을 두면 둘이 갈라지고, 갈라진 사본을 현행으로 읽는 것이 이 문서가 한 번 겪은 실패다.

HOLD 해제와 출하 PASS 표기는 사용자의 판단이다. 에이전트는 근거를 모아 제시한다. 제외되거나 이관된 항목을 PASS로 바꿔 적지 않는다 — 제외는 검증이 아니다.

## 3. 지켜야 할 선

- **사용자의 untracked 파일을 임의로 삭제·stage·덮어쓰지 않는다.** `.tmp/`, `docs/journal/`, `src/agent-selection.review-fixture.mjs`, `verification/dev-sandbox/`에 사용자 상태가 있다. `verification` 아래에는 tracked 자료와 untracked 작업이 섞여 있으므로 폴더째 판단하지 않는다.
- **`git add -A`나 디렉터리 단위 stage를 쓰지 않는다.** 변경한 파일을 이름으로 stage한다.
- **커밋된 문서가 근거로 인용하는 파일은 함께 커밋한다.** 인용과 부재는 공존할 수 없다 — 감사가 이름을 대는 파일이 저장소에 없으면 클론한 사람은 그 주장을 검증할 수 없고, 그것은 이 문서들이 존재하는 이유를 무너뜨린다. 예외는 여기까지이며 인용되지 않은 것은 그대로 둔다.
- **새 증거 문서는 세션 프롬프트를 인용하지 않는다.** 프롬프트와 인계 문서는 `docs/journal/`에 있고 저장소에 올라가지 않는다(2026-09-17 사용자 결정: 증거는 공개, 지시는 비공개). 위 규칙과 합치면 **프롬프트를 인용하는 순간 그 프롬프트가 공개 대상이 된다** — 경계를 무너뜨리는 것은 결정이 아니라 인용이다. 지시를 근거로 대야 하면 그 내용을 감사 문서에 옮겨 적고 그것을 인용한다. `docs/prompts/`에 남은 다섯 개는 이 규칙 이전에 인용된 것들이라 그대로 둔다.
- **guard·권한·정책의 거부를 다른 셸·도구·인코딩으로 우회하지 않는다.** 거부는 미검증으로 남기고 무엇이 막혔는지 기록한다.
- **검증을 약화시켜 통과시키지 않는다.** assertion 제거, 검사 축소, 실패 삼키기, 검사 대상 mocking으로 얻은 통과는 통과가 아니다. 가드와 검증 대상을 먼저 구분한다.
- **실호출은 사용자가 준 예산 안에서만 한다.** 현재 예산 상태는 3.2절이 적는다. 예산 없이 실호출이 필요한 결론에 이르면 실행하지 말고 블로커로 보고한다.
- **과거 세션의 승인 문구를 새 실행의 권한으로 해석하지 않는다.** 실호출, 인증 변경, 외부 게시, 전역 설정 변경은 그때의 명시적 승인이 필요하다.

문서를 고쳤으면 `node verification/test-doc-citations.mjs`가 통과해야 한다. 이 검사는 `파일:줄` 인용의 존재와 범위만 보고 내용은 보지 않으므로, **줄이 옮겨갔는지는 직접 확인한다.** 표의 행을 제자리에서 고치면 그 파일을 가리키는 인용이 살아남는다.

## 4. 도착한 머신에서 시작할 순서

HEAD, 테스트 수, 통과 개수는 여기 적지 않는다. 아래 명령이 현재 값을 말한다. 문서에 박아둔 숫자는 반드시 뒤처진다.

1. 루트를 열고 상태를 읽는다.

   ```powershell
   git log -1 --oneline
   git status --short
   git worktree list --porcelain
   ```

2. 필수 문서가 있는지 본다.

   ```powershell
   Get-Item HANDOFF.md, docs/remaining-verification.md, docs/research-2026-09-12-unattended-release-gates.md, docs/release-readiness.md, docs/transfer-verification-2026-09-12.md
   ```

3. 인증 없는 구성 검사를 한다.

   ```powershell
   .\clauduct.cmd --dry-run -p
   ```

   JSON 한 줄과 exit 0이면 wrapper 기동과 옵션 해석이 정상이다. `model`·`effort`가 [현행 검증표](docs/v1/remaining-verification.md) 2장의 무옵션 시작값과 같은지 본다.

   같은 출력의 `credentialReads`·`childStarted`·`globalWrites`는 **이 분기가 쓰는 고정값이라 아무것도 증명하지 않는다.** dry-run이 credential·소켓·자식을 열지 않는다는 것은 `src/clauduct.mjs`의 해당 분기가 즉시 반환한다는 코드 사실이며, 그 분기 위로 무언가 옮겨가도 출력은 계속 0을 말한다. 이 세 값을 무접속의 증거로 인용하지 않는다.

4. 실행 범위를 먼저 검토한 뒤 로컬 회귀를 돌린다. 무검토 glob으로 전부 실행하지 않는다 — fixture·자식 프로세스·네트워크·쓰기 대상을 먼저 확인하고 필요한 파일만 나열한다. 대상 선택은 [현행 검증표](docs/v1/remaining-verification.md) 5.1절을 따른다.

   ```powershell
   . ./src/run-node-tests.ps1
   Invoke-ClauductNodeTests -Root . -TestFiles @('src/test-native-gateway.mjs')
   ```

   파일 단위 통과 안의 `notRun`을 실행 성공으로 세지 않는다. 이 실행기는 환경 allowlist·시간 제한·단일 concurrency를 주지만 OS 보안 sandbox가 아니다.

5. 3.2절의 남은 항목 중 하나를 골라 시작한다. 구현 전에 해당 코드·caller·테스트를 읽고, 마지막에 diff와 검증 결과를 남긴다.

## 5. 이 폴더로 따라오지 않는 것

| 항목 | 이유 |
|---|---|
| 과거 Codex/Claude 세션 이력 | 사용자 홈의 세션 저장소에 있다. 폴더 복사로 따라오지 않는다 |
| native memory | 프로젝트 지식이 여기에만 저장된 흔적은 없다. 점검 범위는 [이관 검증](docs/v1/release/transfer-verification-2026-09-12.md)에 있다 |
| 실행 중 RAM 상태 | socket·PID·lease는 머신 종속이며 살아 있는 프로세스는 이관되지 않는다 |
| 인증 | 대상 머신의 유효한 로그인이 필요하다 |

인증과 전역 설정을 프로젝트 폴더에 복사해 전제를 충족시키지 않는다. 대상 머신에 미리 있어야 하는 것:

| 항목 | 조건 |
|---|---|
| OS·도구 | Windows, PowerShell 7+, Git, Node, Claude, Codex |
| 실행 파일 위치 | 사용자 홈의 `.local/bin/claude.exe`, `AppData/Local/Programs/OpenAI/Codex/bin/codex.exe`. Node는 PATH에서 찾는다 |
| 검증된 버전 조합 | 실행별로 [현행 검증표](docs/v1/remaining-verification.md) 4장이 적는다. 2장은 Codex clientVersion 계약만 다룬다. 버전 번호만으로 bridge 호환성을 확정하지 않는다 |
| 환경 지침 | global guidance·hooks·permissions·plugins·MCP는 대상 머신 쪽 설정으로 유지한다 |
| secret 환경 변수 | provider/secret 계열 env는 Clauduct 자식에 전달하지 않는 기존 계약을 유지한다 |

`.clauduct-profile/`과 `.clauduct-status/`는 Git이 무시하는 로컬 상태다. `.tmp`에는 시험용 profile·이력·ZIP·linked worktree가 있다. 폴더를 통째로 복사하면 이런 자료도 따라오지만 `git clone`과 배포 ZIP은 포함하지 않는다.

## 6. worktree 절대경로

주 저장소의 `.git`은 실제 디렉터리이며 외부 object alternates·commondir·submodule을 가리키지 않는다. 다만 보조 linked worktree는 양방향 절대경로로 연결된다 — 예를 들어 `.tmp/release-2026-09-12/.git`과 `.git/worktrees/release-2026-09-12/gitdir`가 서로를 가리킨다.

같은 절대경로에 두면 변경이 없다. 다른 경로로 옮겼다면 주 루트에서 개발을 시작하고, 보조 worktree는 연결을 확인하기 전에 쓰지 않는다.

```powershell
git worktree list --porcelain
git worktree repair .tmp/release-2026-09-12
git worktree list --porcelain
```

대상 폴더가 존재하고 다른 저장소를 가리키지 않는지 확인한 뒤 `repair`를 쓴다. 임시 폴더를 강제 삭제·prune하거나 원래 worktree를 reset하는 방식으로 해결하지 않는다. [Git worktree repair](https://git-scm.com/docs/git-worktree)

## 7. 아직 확인되지 않은 것

- **다른 물리 머신에서의 실제 실행.** 이관 판정은 이 머신에서의 검사에 근거하며 도착 후 위 baseline 검사로 확인해야 한다.
- **폴더 전체의 민감정보 부재.** 보존된 profile·임시 결과·대화 이력의 전수 검사는 하지 않았다. 자격증명 경로 이름이 들어간 전수 점검 명령은 `shell-guard`가 차단했고 우회하지 않았다.
- **기존 세션의 cross-machine resume.** 새 세션으로 개발을 이어가는 것과 구분한다.

이관 검사의 상세와 제약은 [이관 검증 기록](docs/v1/release/transfer-verification-2026-09-12.md)에 있다.
