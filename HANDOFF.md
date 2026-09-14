# Clauduct 다른 머신 이관 및 작업 재개

이 폴더를 받은 사람 또는 에이전트는 이 문서부터 읽는다. **동일한 Windows·계정·도구 환경에서, 프로젝트 폴더와 아래 문서를 함께 옮기면 새 세션으로 개발을 이어갈 수 있다. 기존 Codex/Claude 대화를 그대로 resume하는 것은 별도 문제이며 이 폴더만으로 보장되지 않는다.**

이관 기준은 2026-09-12, 코드 HEAD `aa317c75c7bad0cf9d16641ca7db4edb43016c19`, 기본 브랜치 `fix/native-completion-resume`이다. 본 문서 작성은 문서화·이관 가능성 점검이며 무인 개발 개선의 구현이나 HOLD 해제를 수행한 것은 아니다.

## 기존 대화가 전혀 없는 새 머신의 전제

이관 대상은 **이 파일 하나가 아니라 프로젝트 디렉터리 전체**다. `HANDOFF.md`는 유일한 시작점이며, 아래에 연결된 로컬 문서를 따라 읽어 작업을 계속한다. 이전 대화나 native memory를 먼저 가져오거나 이전 사용자에게 맥락을 다시 설명해 달라고 요청할 필요가 없도록 목표·상태·제한·다음 작업을 문서화했다.

- 기존 구현·테스트·기능별 검증 기록과 앞으로의 구현 제안은 모두 프로젝트 안에 있다. 실제 테스트 파일은 `src/test-*.mjs` 23개와 러너 자기 검사 `.ps1` 1개, `poc/test-*.mjs` 6개, `verification/test-*.mjs` 4개를 확인했다.
- **F01~F23과 4/24/72시간 시험은 새로 구현·실행해야 할 명세다.** 대응하는 신규 시험이 전부 완성됐다는 뜻이 아니다. 기존 테스트에서 충족하는 항목을 먼저 대응시키고 부족한 부분만 구현한다.
- 과거 native 세션 원문·프로세스 RAM·사용자 홈 설정은 이 폴더의 일부가 아니다. 과거 관측의 필요한 결론과 한계는 로컬 감사에 기록돼 있다. 원문이 없어 더 이상 확정할 수 없는 사실은 미확정으로 유지하며 원래 세션 복원을 개발의 선행 조건으로 삼지 않는다.
- 공식 웹 문서는 출처 링크이며 웹페이지 전체를 오프라인 보관한 것은 아니다. 초기 작업 파악은 로컬 문서로 가능하고, 이후 버전 민감한 변경을 구현할 때 공식 근거를 다시 확인한다.

추가 검사에서 이 문서로부터 연결되는 Markdown 문서 44개를 재귀적으로 확인했고, 누락된 로컬 링크·프로젝트 밖으로 나가는 로컬 링크는 0개였다. 실행·검증용 `.mjs` 67개의 정적 상대 import 171개도 모두 프로젝트 내부에 존재했다. 동적으로 생성하는 fixture와 실행 시 환경은 이 정적 검사 범위와 구분한다.

## 1. 이관 판정

| 원하는 결과 | 판정 | 조건 |
|---|---|---|
| 다른 머신에서 Clauduct 실행·개발 | 조건부 가능 | 동일 환경이 실제로 준비돼 있고, 도착 후 baseline 검사 통과 |
| 이전 대화 없이 남은 개발을 계속 | 가능하도록 문서화 완료 | 이 문서와 상세 리서치·현행 검증표·코드를 함께 읽음 |
| 같은 절대경로 `D:/AIDEV/Clauduct`로 전체 폴더 복사 | 경로 변경이 가장 적음 | `.git`·최신 untracked 문서·필요한 사용자 작업도 포함. 복사 중 파일 변경을 피함 |
| 다른 드라이브·다른 폴더로 이동 | 주 실행 경로는 이동 가능 | 보조 linked worktree의 Git 절대경로 연결은 확인·복구 필요 |
| 기존 Codex 세션 `01a093fe-2026-7341-ac3c-ffe6f16e72fd` 그대로 재개 | 폴더만으로 불가 | 해당 머신의 Codex 세션 저장소에 원래 이력이 있어야 함 |
| 기존 Claude native 세션 그대로 재개 | 폴더만으로 보장 불가 | 사용자 홈 또는 해당 `CLAUDE_CONFIG_DIR`의 세션 파일·메타데이터가 필요 |
| 폴더 전체에 민감정보가 전혀 없다고 보증 | 미검증 | 보존된 profile·임시 결과·대화 이력의 전수 검사는 완료하지 않음 |
| API 오류·작업 중단 없는 무제한 무인 개발 | HOLD 유지 | 상세 리서치의 구현·장애·장기 시험이 아직 남음 |

상세 검사 사실과 제약은 [이관 검증 기록](docs/transfer-verification-2026-09-12.md)에 있다. 이관 대상 머신에서 실제 실행한 결과는 아직 없다.

## 2. 필수 문서와 읽는 순서

1. **현재 문서**: 목적·완료 범위·이관 조건·다음 행동을 확인한다.
2. **[무인 개발 심층 리서치 전체](docs/research-2026-09-12-unattended-release-gates.md)**: 19개 1차 출처, 구현 제안, 장애 시험 F01~F23, 단계별 합격 기준, 통계적 한계의 원본이다. 요약만으로 개발 범위를 정하지 않는다.
3. **[현행 검증표](docs/remaining-verification.md)**: 2~5장이 현행 기준이며 뒤쪽 과거 기록과 구분한다. ‘완료’ 행에 적힌 증거 범위도 읽는다.
4. **[릴리즈 검증](docs/release-readiness.md)** 및 **[RELEASE.md](RELEASE.md)**: 실제 실행한 항목, 버전, 오류 계약, ZIP 구성과 남은 한계를 확인한다.
5. **관련 구현과 감사**: 선택한 변경의 코드·caller·테스트와 연결된 감사 문서를 읽는다. 과거 인수인계의 `6c263c0`·20/20을 현재 HEAD·새 시험 결과로 오인하지 않는다.

위 문서와 코드가 이 프로젝트의 작업 근거다. 이전 대화, 사용자 홈 native memory, Engramux 검색 결과가 있어야만 다음 개발을 할 수 있는 구조로 두지 않는다. 필요 시 그런 기록은 보조 증거로 사용하며 현재 코드와 검증 결과를 우선한다.

## 3. 목표와 완료 상태

최종 목표는 Anthropic 서버 전용 기능을 제외한 약속된 기능을 안정적으로 제공하고, API 오류·취소·재시도·자식 작업·결과 보존·자원 정리를 검증하여 사람이 지켜보지 않는 연속 개발이 가능하게 하는 것이다. ‘오류 시 안전하게 종료’와 ‘원래 개발 과제를 사람 없이 완료’는 각각 별도로 판정한다.

### 이미 구현·검증된 릴리즈

| 커밋 | 핵심 내용 |
|---|---|
| `032281d` | 명시적 `-p`/`--print`와 JSON stdout 보존 |
| `7e8d365` | 개인 PC 고정 경로 제거, 진단 예외 원문 노출 방지 |
| `f064667` | 빈 최종 JSON 답변 수정, 도구 축소 후 resume의 완료 이력 보존 |
| `294cb18` | delta 없는 빈 text 완료 처리와 mismatch 이벤트 진단 |
| `a3c8a7c` | 비대화형 실제 검증기와 지원 근거 정리 |
| `c310c59` | 실제 Workflow 및 재현 가능한 릴리즈 ZIP |
| `e216ad8` | 배포본에서 실행 가능한 검사·빌더 경계 보완 |
| `aa317c7` | 최종 검증·잔여 제한 문서 반영 |

기존 세션의 관측은 전체 `src` 회귀 23/23, PoC 6/6, HTTP 88/88이다. 실제 Read/Edit, Bash·PowerShell, MCP, PNG, WebFetch·WebSearch, Agent, Workflow, resume, background 결과 회수·TaskStop을 확인했다. 파일 단위 23/23 내부의 `notRun`을 실행 성공으로 세지 않는다.

실패 복구에서는 실제 MCP 효과 1회 후 오류를 주입하고 명시적 `--resume`으로 복구하여 총 효과가 1회인 것을 확인했다. **자동 재개기가 판단·재시작하는 전체 과정이나 effect 발생 직후 기록 전 crash를 검증한 것은 아니다.**

이전 run-04에는 2시간 37분·408요청·개발 3주기·개입 0의 근거가 있다. 실패 10건과 최대 약 7.4분 admission 대기도 함께 있었다. 과거 빌드의 이 결과와 최종 릴리즈를 섞지 않는다. [run-04 감사](docs/audit-2026-09-11-run-04-exit-diagnostics.md)

### 아직 남은 필수 개발

1. 작업 원장·독립 완료 판정·단일 세션 소유·효과 대조를 사용하는 자동 재개.
2. 공식 인증 유지 주체, 긴 `Retry-After` 대기, jitter와 재시도 책임 분리.
3. 기본 400K/320K의 반복 압축 및 긴 agent/Workflow 이력 경계 검증.
4. 메모리 admission 대기·Windows 프로세스 생존·bounded 관측 기록·검증기 출력 처리.
5. 같은 후보 빌드의 장애 시험, 4시간 → 24시간 3회 → 72시간 시험과 구성별 최종 판정.

이 다섯 묶음은 리서치에서 제안한 것이며 아직 구현하지 않았다. 첫 번째 묶음부터 기존 native 검증기·session resume·request status를 재사용하는 최소 변경으로 진행한다. 계정/인증 경로 전환이나 새 프레임워크를 기본 전제로 삼지 않는다.

### 누락 없이 넘겨야 할 제한

| 분류 | 남은 내용 |
|---|---|
| 실제 오류 | Astra 공개 단문 최초 upstream 오류 1건의 원인 미확정. 이후 성공을 원인 수정으로 취급하지 않음 |
| 압축 | 기본 320K 발동·압축 후 전체 작업/도구 이력 보존 미검증 |
| 인증 | supplier는 캐시 읽기 전용. 실제 정상 갱신·회전·만료를 넘는 무인 실행 미검증 |
| 사용자 환경 | hook/plugin·permission/plan·MCP 구성 전체, UI 취소·등록 교체·형제 격리 전체 실측 미완료 |
| 입력·캐시 | JPEG/GIF/WebP 실제 왕복·실제 캐시 적중 미검증. PNG 성공과 구분 |
| background | 세션 분리 `--bg`와 검증된 도구 background/TaskOutput/TaskStop을 구분 |
| 알림 | 단일 completed 복귀만 검증된 범위. 다중·실패·취소 알림 자동 복귀는 미지원 |
| Workflow | 캐시 미적중 resume·중첩·custom agentType은 미보장 |
| API 지원 | 비스트리밍, 서버 실행 도구·첨부/PDF·일부 context edit/sampling 필드는 미지원 |
| 보안 시험 | 동적 symlink/junction 시험은 정책 차단. 다른 도구·셸로 같은 거부를 우회하지 않음 |
| 버전 | native와 비공개 backend 변경 시 재검증. 버전 번호나 공식 공개 API 문서만으로 bridge 호환성 확정 불가 |
| 강제 종료 | OS 강제 종료 시 마지막 상태 기록·모든 프로세스 회수 보장 미검증 |

과거 문서에서 ‘실제 인증 갱신·수시간 실행’을 범위밖으로 뒀더라도 새 무인 개발 목표에서는 필요한 검증이다. 과거 범위 제외를 근거로 전체 목표를 축소하지 않는다. 반대로 과거 세션의 승인 문구를 새 머신에서의 무기한 유료 실행·인증 변경·외부 게시 승인으로 해석하지 않는다.

## 4. 연구 내용 완전성 대응표

| 전달해야 할 내용 | 상세 리서치 위치 |
|---|---|
| 무인성의 정의·HOLD 해제 범위 | 1장 |
| 현재 코드의 공백 14개와 기존 관측 | 2장 |
| 장기 harness·반복 평가·효과 중복·API 차이 근거 | 3장 |
| 최소 구현 다섯 묶음 | 4장 |
| 장애·경계 시험 F01~F23 전부 | 5장 |
| effect 전후 crash와 idempotency 없는 도구의 처리 | 5장 |
| 4h·24h×3·72h 단계와 사건 수·반복 기준 | 6장 |
| 유실·중복·잘못된 완료·완주율·복구율·지연·자원·비용 지표 | 7장 |
| 2995 독립 성공 예시·72h 통계 한계·148h 최소 실행 시간 | 6~7장 |
| 구현 우선순위·전 제품 HOLD 잔여 범위 | 8장 |
| 효과·대상·비용·시간·데이터·동시성 전제 | 9장 |
| Verified/Not verified/Blocked와 출처 19개 | 10장 |

상세 리서치는 이 문서 작성 시 344행이며 F01~F23과 출처 19개가 모두 존재함을 확인했다. 여기서 복사 요약으로 대체하지 않고 원본을 같은 폴더에 보존했다.

## 5. native memory와 세션 이력 점검

**현재 확인한 위치에는 이 프로젝트의 필수 지식이 native memory에만 저장된 흔적이 없다.** 프로젝트 작업 상태·연구·남은 검증은 위 문서와 Git 안에 있다. ‘native memory가 비어 있음’과 ‘세션 이력이 없음’은 다르다.

| 위치/종류 | 이번 확인 | 폴더 이관 영향 |
|---|---|---|
| Claude 사용자 설정의 auto memory | `autoMemoryEnabled=false`, 사용자 설정의 custom directory 없음, 현재 `CLAUDE_CONFIG_DIR` override 없음 | 현재 사용자 설정 기준으로 auto memory 사용 안 함 |
| Claude 기본 project memory | 사용자 홈의 Clauduct 관련 project 6개 범위에서 memory 문서 없음 | 이 위치에서 별도로 옮길 memory 문서 없음 |
| 프로젝트 `.clauduct-profile` | `MEMORY.md`·memory 디렉터리 내 문서 없음 | 이 안의 profile/이력과 memory를 혼동하지 않음 |
| Claude 사용자 agent memory | `agent-memory`·`agent-memory-local` 없음 | 확인한 사용자 범위에서 추가 memory 없음 |
| Codex native memory DB | 사용자 홈 `memories_1.sqlite`의 `stage1_outputs=0`, `jobs=0` | 이 DB에서 별도로 옮길 생성 memory 없음 |
| Codex memory 파일 후보 | `memories/`, `memory/`, `memory.md` 없음 | 확인한 후보 위치에 별도 파일 없음 |
| Claude 전역 지침 | 사용자 홈에 존재, `Clauduct` 언급 없음 | 환경 지침이며 프로젝트 작업의 유일한 저장소는 아님 |
| Claude 세션 이력 | 사용자 홈의 Clauduct project JSONL·자식 디렉터리가 존재 | 이 프로젝트 폴더만 옮기면 자동으로 따라오지 않음 |
| Codex 세션 이력 | 앞서 분석한 대상 JSONL은 사용자 홈 `.codex/sessions/`에 존재 | 동일 계정만으로 대상 머신에 이력이 있다고 가정하지 않음 |
| 실행 중 RAM 상태 | Git·문서에 없는 현재 socket·PID·lease 등은 머신 종속 | 살아 있는 프로세스의 연속 실행은 폴더 복사로 이관되지 않음 |

사용자 홈 memory 확인은 read-only로 수행했다. DB 원문, 인증 값, 세션 본문을 문서에 복사하지 않았다. 임의의 과거 실행별 custom profile, 관리형 설정, 모든 plugin의 자체 메모리 저장소를 전수 조사한 것은 아니다. Engramux 등 선택적 통합은 이 문서 기반 개발 재개의 필수 의존성으로 두지 않는다.

공식 Claude 문서도 auto memory와 transcript를 로컬 저장으로 설명한다. 같은 계정·같은 도구 버전이라는 전제는 저장된 대화 내용까지 동일함을 뜻하지 않는다. [Memory](https://code.claude.com/docs/en/memory) · [Sessions](https://code.claude.com/docs/en/sessions)

## 6. 폴더에 포함할 것과 외부 환경

### 디렉터리 자체로 개발을 이관할 때

프로젝트 안의 `.git`, `src`, `poc`, `verification`, `docs`, `clauduct.cmd`, `bin`, `README.md`, `RELEASE.md`, 이 문서를 함께 보존한다. hidden·untracked 파일이 자동 제외되는 전송 방식인지 확인한다. **상세 리서치와 새 이관 문서는 아직 commit되지 않았으므로 `git clone`이나 기존 릴리즈 ZIP만으로는 따라가지 않는다.**

사용자의 기존 untracked 파일은 임의 삭제·stage·덮어쓰지 않는다. `%SystemDrive%/`, `.tmp/`, `clauduct-check.txt`, `clauduct-agent-validation-*.txt`, 과거 handoff·prompts, `src/agent-selection.review-fixture.mjs`, `verification/dev-sandbox/`에 기존 상태가 있다. 특히 `verification` 아래에는 tracked 자료와 untracked 작업이 함께 있으므로 폴더째 불필요하다고 판단하지 않는다.

`.clauduct-profile/`과 `.clauduct-status/`는 Git에서 무시하는 로컬 상태다. `.tmp`에는 시험용 profile·이력·ZIP·linked worktree가 있다. 실제 폴더 복사는 이런 자료도 포함할 수 있지만 Git clone과 68파일 배포 ZIP은 포함하지 않는다. 전체 폴더를 그대로 옮기는 기술적 가능성과 전수 민감정보 검사 완료는 별개다. 현재 폴더를 외부 서비스에 업로드하는 작업은 수행하지 않았다.

### 대상 머신에서 이미 준비돼 있어야 하는 것

| 항목 | 기대 조건 |
|---|---|
| OS와 도구 | Windows, Node `24.19.0`, Claude `2.1.269`, Codex `0.154.0`가 기존 검증 조합. PowerShell 7+·Git 필요 |
| 실행 파일 위치 | OS 사용자 홈 기준 `.local/bin/claude.exe`, `AppData/Local/Programs/OpenAI/Codex/bin/codex.exe`. Node는 PATH에서 찾음 |
| 계정 | 대상 머신의 정상 로그인과 지원되는 파일 credential store. 동일 계정이어도 해당 머신에 유효한 로그인 상태 필요 |
| 환경 지침 | 사용자가 동일하다고 제시한 global guidance·hooks·permissions·plugins·MCP·keybindings는 대상 머신 쪽 설정으로 유지 |
| secret 환경 변수 | provider/secret 계열 env는 Clauduct 자식에 전달하지 않는 기존 계약 유지 |
| 세션 저장소 | 새 세션으로 개발할 때 과거 native memory/대화는 불필요. 과거 세션 자체의 resume은 별도 저장소 필요 |

인증·전역 설정을 프로젝트 폴더에 복사해서 이 전제를 충족시키지 않는다. 과거 transcript를 새 머신에 추가로 옮길 필요가 생기면 세션별 대상·민감정보·경로를 별도 검토한다. 이번 이관 문서만으로 새 세션에서 작업을 시작할 수 있도록 핵심 상태를 파일에 남겼다.

## 7. Git 절대경로 주의

주 작업 루트의 `.git`은 실제 디렉터리다. 외부 object alternates·commondir·submodule 파일은 발견되지 않았다. 주 저장소 자체가 외부 Git 디렉터리를 가리키는 구성은 아니다.

다만 다음 두 파일은 원래 절대경로를 가진다.

```text
.tmp/release-2026-09-12/.git
  → D:/AIDEV/Clauduct/.git/worktrees/release-2026-09-12
.git/worktrees/release-2026-09-12/gitdir
  → D:/AIDEV/Clauduct/.tmp/release-2026-09-12/.git
```

대상 머신에서도 정확히 같은 경로에 두면 이 경로의 변경을 줄일 수 있다. 다른 경로로 옮겼다면 주 루트에서 개발을 시작하고, 보조 worktree는 연결을 확인하기 전 사용하지 않는다. 양쪽 폴더를 모두 옮긴 경우 Git이 문서화한 복구 방법은 새 주 루트에서 아래처럼 실제로 옮겨진 보조 경로를 지정하는 것이다.

```powershell
git worktree list --porcelain
git worktree repair .tmp/release-2026-09-12
git worktree list --porcelain
```

`repair`는 이관 후 경로 불일치가 확인됐을 때의 방법이며 이번 원본에서는 실행하지 않았다. 대상 폴더가 존재하고 다른 사용자 저장소를 가리키지 않는지 확인한 뒤 사용한다. 검증용 임시 폴더를 강제로 삭제·prune하거나 원래 워크트리를 reset하는 방식으로 해결하지 않는다. [Git worktree repair](https://git-scm.com/docs/git-worktree)

## 8. 도착한 머신에서 시작할 순서

1. 복사된 프로젝트 루트를 연다. 아래 명령은 그 루트에서 실행한다.

   ```powershell
   Get-Location
   git log -1 --oneline
   git status --short
   git worktree list --porcelain
   ```

   코드 기준 HEAD는 `aa317c7`이다. 문서 작성 이후 `README.md` 변경과 새 문서들이 존재하는 것은 의도한 상태다. 실제 차이가 있으면 현재 diff를 먼저 확인한다.

2. 필수 문서의 존재와 내용을 확인한다.

   ```powershell
   Get-Item HANDOFF.md, docs/research-2026-09-12-unattended-release-gates.md, docs/transfer-verification-2026-09-12.md, docs/remaining-verification.md, docs/release-readiness.md
   ```

3. 인증 없는 구성 검사를 한다.

   ```powershell
   .\clauduct.cmd --dry-run -p
   ```

   exit 0, `mode=print`, `model=gpt-6-astra`, `effort=low`, `credentialReads=0`, `childStarted=false`, `globalWrites=0`이 기준이다. 이것은 backend 접속 시험이 아니다.

4. 관련 코드·러너의 실행 범위를 검토한 뒤 로컬 회귀를 실행한다.

   ```powershell
   pwsh -NoProfile -NonInteractive -File src/run-node-tests.ps1 -Root . -TestFiles 'src/test-*.mjs'
   ```

   기존 기록은 23/23이다. 내부 notRun과 실제 실패를 분리한다. 새로운 검사 결과를 과거 기록으로 대체하지 않는다. 실제 `-Live` 검사는 사용량과 외부 전송이 있으므로 별도의 구체적인 실행 범위 안에서 수행한다.

5. 새 Codex/Claude 세션에서 이 문서와 상세 리서치를 읽고, 3장의 남은 개발 첫 번째 묶음을 시작한다. 원래 세션 ID를 복원하는 일은 선행 조건이 아니다. 새 구현 전에 해당 코드·caller·테스트를 확인하고 마지막으로 기준 대비 diff와 검증 결과를 남긴다.

이 문서는 자동 로딩되는 AGENTS.md/CLAUDE.md가 아니다. 새 세션에서 `HANDOFF.md`를 먼저 읽도록 명시하면 된다. 기존 global guidance나 hooks를 새로 만들거나 대체할 필요는 없다.

## 9. 이번 이관 점검의 증거

**Verified:** 주 Git 구조와 linked worktree 경로, native memory 위치·DB 건수, 리서치 F01~F23·출처 19개를 확인했다. 기존 릴리즈 ZIP SHA256을 확인하고 현재 68개 배포 파일의 내용을 대조했다. 25개는 바이트 일치, 43개는 CRLF/LF 차이만 있었다. 공백이 있는 새 로컬 경로에 검증된 배포본을 풀어 `clauduct.cmd --dry-run -p`를 실행했고 exit 0과 인증/자식 실행/전역 쓰기 0을 확인했다.

**Not verified:** 다른 물리 머신 실행, 그 머신의 실제 계정·환경 동일성, 기존 세션의 cross-machine resume, 전체 폴더의 민감정보 부재, 새 장시간 시험. 이번 변경은 문서와 로컬 이관 검사이며 제품 코드·인증·global 설정은 바꾸지 않았다.

**Blocked by:** 자동 승인 검토 `shell-guard`가 자격증명 경로 이름이 들어간 폴더 전체 점검 명령을 차단했다. 그 민감정보 검사는 우회하지 않았다. 독립적인 memory 설정 필드 확인과 코드/문서/ZIP 경로 검사는 수행했다. 기존 동적 symlink/junction 시험의 차단도 계속 별도 제한으로 남는다.
