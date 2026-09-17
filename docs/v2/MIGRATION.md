# 파일 처분과 이관 계획

기준선 `node-bfbdf23-g0` (commit `bfbdf2385175…`) 시점의 tracked 738개 전수 분류다.

**source of truth는 이 문서가 아니라 `comparison/baselines/node-bfbdf23-g0-migration.json`이다.** 여기는 그 기계 판본의 요약과, 기계가 표현하지 못하는 판단 근거를 적는다.

## 1. 세 수치

```text
계약 이관 대상 파일 수:      171   (PORT_CONTRACT)
즉시 물리 이동 대상 파일 수:   0   (원칙, 그리고 실제)
후속 archive 후보 파일 수:  미확정 — DEFERRED. 3장 참조
```

"이관"과 "이동"은 다른 말이다. 계약·검증 지식은 Go로 이관하되 원본 파일은 비교를 위해 제자리에 둔다.

## 2. 전수성

| 검사 | 값 |
|---|---|
| baseline tracked paths | 738 |
| manifest entries | 738 |
| `missing_paths` | **0** |
| `duplicate_source_paths` | **0** |
| `unreviewed_physical_moves` | **0** |
| `destination_collisions` | **0** |

index는 HEAD tree와 일치했고 tracked 파일의 dirty 변경은 0건이었다. 따라서 baseline tree와 현재 index를 따로 비교할 필요가 없었다. 신규 V2 파일(`go/` `comparison/` `docs/v2/`)은 `entries`에 넣지 않는다 — 전수성의 분모가 흔들리지 않게 하기 위해서다.

**Windows 경계 확인.** tracked 738개 중 7개가 비-ASCII(한국어) 파일명을 갖는다.

```text
poc/게이트웨이.md              verification/OmniRoute-Codex-인증분석.md
poc/사용자-실행.md             verification/검증결과.md
poc/실제-Read-실행.md          verification/최소-어댑터-PoC-명세.md
poc/요청-검사기.md
```

대소문자만 다른 충돌, 예약 이름, 끝의 점·공백은 발견되지 않았다. tracked symlink와 submodule은 없다(전 항목 mode `100644`). 이동을 검토하는 단계가 오면 이 7개를 개별 확인 대상으로 둔다 — 인코딩·정규화 차이가 rename을 삭제+생성으로 보이게 만들 수 있다.

## 3. action별 분포

| Action | 개수 | 무엇인가 |
|---|---|---|
| `EXCLUDE_FROM_PRODUCT` | 416 | 전부 `verification/schema/*.json`. Codex app-server 프로토콜 스키마이며 app-server는 V2 초기 범위 밖이다. 저장소에서 지운다는 뜻이 아니라 Go runtime/package에 넣지 않는다는 뜻이다 |
| `PORT_CONTRACT` | 171 | 런타임 closure 29 + 테스트/oracle 137 + 현행 계약 문서 5 |
| `KEEP_REFERENCE` | 148 | Node 제품 표면 6, 문서 70, 비런타임 helper 61, dotnet probe 1, 문서 게이트 1 외 |
| `UPDATE_COORDINATION` | 2 | `.gitignore`, `.github/workflows/tests.yml` |
| `REUSE_FIXTURE_REVIEWED` | 1 | `verification/fixtures/public-images.json` |

`status`는 `reviewed` 468 / `pending` 270이다. `pending`은 규칙으로 분류했으나 파일을 개별로 읽지 않았다는 뜻이다. **`pending` 상태의 파일은 물리 이동 후보가 아니다.** `UNKNOWN_REVIEW`는 0건이며, 모르는 것을 삭제 후보로 몰지 않기 위해 기본값을 `KEEP_REFERENCE`로 두었다.

디렉터리별:

```text
(root)      KEEP_REFERENCE 5   UPDATE_COORDINATION 1
.github     UPDATE_COORDINATION 1
bin         KEEP_REFERENCE 1
docs        KEEP_REFERENCE 70  PORT_CONTRACT 5
poc         KEEP_REFERENCE 8   PORT_CONTRACT 12
src         KEEP_REFERENCE 3   PORT_CONTRACT 123
verification EXCLUDE_FROM_PRODUCT 416  KEEP_REFERENCE 61  PORT_CONTRACT 31  REUSE_FIXTURE_REVIEWED 1
```

## 4. 런타임 closure 29개 — 분류의 뼈대

`src/clauduct.mjs`에서 출발한 정적 import closure는 28개이고, 여기에 import edge가 없는 런타임 의존 1개(`src/review-diff.mjs`)를 더해 **29개**다. 이 29개가 `PORT_CONTRACT`의 핵심이며 manifest의 `in_runtime_closure: true`로 표시된다.

```text
src/  (21)  clauduct native-gateway native-protocol native-transport native-delivery
            native-beta native-search compact-policy models agent-selection agent-route
            workflow-selection request-admission request-status http-close runtime-paths
            client-version retry-after retry-after-seconds rate-limit-observation review-diff
poc/  (6)   adapter user-session claude-inspection codex-transport gateway request-inspector
verification/ (2)  manual-http-probe auth-store-selection
```

**폴더 이름으로 분류하면 8개를 놓친다.** `poc/`가 "폐기된 실험"이 아니고 `verification/`이 "테스트 전용"이 아니다. 두 폴더에서 8개 파일이 제품 런타임에 들어간다.

Go target 대응은 manifest의 `go_target` 필드에 있다. 함수를 1:1로 옮기지 않고 입력·출력·오류·상태 전이·회귀 사례 단위로 포팅한다. 같은 이름의 함수를 만들었다는 사실은 계약 보존의 증거가 아니다.

**포팅이 아니라 재설계로 분류한 3건** — 근거는 [DECISION.md](DECISION.md) 2.3·2.4:

| 파일 | 왜 기계적 포팅이 불가한가 |
|---|---|
| `src/review-diff.mjs` | `src/native-protocol.mjs:139-140`이 `process.execPath` + 이 파일 경로를 shell 문자열로 만들어 native child에 넘긴다. 그대로 옮기면 Go 바이너리가 Node를 요구한다(H01·REL03 위반) |
| `src/agent-route.mjs` | `src/clauduct.mjs:204-209`가 같은 방식으로 hook command를 만든다. D10에 따라 **기본 경로에서 제거**하면 Node 의존과 overlay 문제가 함께 사라진다 |
| `src/clauduct.mjs`의 상태 기록 | `src/clauduct.mjs:229`가 `../.clauduct-status/`에 쓴다. 설치 디렉터리 안이라 요구 V2-01과 충돌한다 |

## 5. 제3자 의존성

전 저장소의 bare import specifier가 전부 `node:` 내장이다. `package.json`도 `node_modules`도 tracked에 없다. **재현해야 할 npm 의존성 표면이 0이다.** Go에서 새로 추가하는 의존성은 전부 V2가 새로 만드는 비용이며 [ARCHITECTURE.md](ARCHITECTURE.md) 14장의 허용 기준을 통과해야 한다.

## 6. 이동 단계

### M0 — 설계·인벤토리 (완료)

기존 파일 이동 **0건**. 실제로 0건이다. 새로 만든 것은 `docs/v2/`와 `comparison/baselines/` 아래의 untracked 산출물뿐이다. target tree를 만들기 위해 기존 폴더를 먼저 정리하지 않았다.

### M1 — 공존 개발 (다음)

`go/`·`comparison/`·`docs/v2/`에만 추가한다. 허용된 root 변경은 두 건으로 한정한다.

| 파일 | 허용 변경 | 금지 |
|---|---|---|
| `.gitignore` | `/artifacts/` 한 줄 추가 | 기존 4줄 수정 |
| `.github/workflows/tests.yml` | 별도 Go job 추가 | 기존 Node job의 trigger·path filter·working-directory·timeout 변경 |

비교에서 Node 실행 cwd는 원래 코드가 기대하는 저장소 root로 고정한다. Go는 module 디렉터리에서 build하더라도 제품 실행 cwd는 사용자의 프로젝트 디렉터리를 유지한다. build cwd와 child cwd를 혼동하지 않는다.

### M2 — 기본 실행기 전환 (사용자 승인 필요)

검증과 승인이 끝나면 설치 대상만 Go로 바꿀 수 있다. `go/` 소스를 root로 옮길 필요는 없다.

검증 항목: Windows에서 기존 `clauduct.cmd`와 신규 `clauduct.exe`의 실제 PATH 해석, 현재 directory·PATHEXT·alias·PowerShell function에 따른 shadowing, 새 바이너리 무결성과 사용자에게 보이는 버전 식별, 기존 Node 배포를 이름이 분리된 rollback 대상으로 보존하는 방법, 설치 실패·파일 잠김·권한 부족 때 원래 실행 경로를 망가뜨리지 않는 방법.

모델 응답·도구 실행이 진행된 세션 중에 Node로 자동 failover하지 않는다. 실행 파일 롤백은 **새 실행부터** 적용한다.

### M3 — 선택적 archive (권고: 하지 않음)

[DECISION.md](DECISION.md) 3장이 근거다. 요약하면 이 저장소에서 `legacy/node/`를 만드는 비용이 이익을 넘는다.

- `src → poc`, `src → verification` 방향 import가 실재하므로 폴더를 옮기면 런타임 closure 8개의 상대 경로가 전부 바뀐다.
- `verification/` 506개 중 416개가 `schema/` JSON이고 참조자는 문서 4개와 `verification/probe.mjs` 하나다. 옮겨 얻는 것이 거의 없다.
- 7개 worktree가 전부 `.tmp/` 아래이고 셋은 branch에 붙어 있다. 대규모 rename은 그 worktree와의 비교를 무효화한다.
- Git tag와 기준선 worktree만으로 "기존 Node를 따로 본다"는 목적이 달성된다.

계획에서 지우지는 않되 기본은 실행하지 않음으로 둔다.

**G10을 여는 조건 (2026-09-17 확정).** "언젠가"를 검사할 수 있는 것으로 바꾼다. 셋이 모두 차기
전에는 열지 않는다.

| # | 조건 | 왜 이것이 조건인가 |
|---|---|---|
| 1 | Go를 기본으로 돌린 뒤 **`clauduct-node`로 되돌린 적이 없음**이 기록으로 확인된다 | M3가 치우려는 것이 곧 그 롤백 경로다. G9는 2026-09-17에 있었고, 몇 시간 된 전환의 되돌리기를 먼저 archive하는 것은 순서가 거꾸로다 |
| 2 | **3-실행기 비교 하네스를 만들지 않기로 확정**된다 | 만들 거라면 기준선은 트리에 있어야 한다. 오늘 `comparison/`에는 baselines JSON뿐이고 하네스는 존재한 적이 없다 |
| 3 | docs/v2의 **`파일:줄` 인용 11건과 링크 3건**의 이관 방법이 정해진다 | 인용 테스트가 tracked 파일로 검사하므로, 정하지 않고 옮기면 V2 설계 판단의 근거가 통째로 끊긴다 |

셋이 차면 WP10을 열고, 그때 아래 closure 검증을 먼저 통과시킨다. 조건이 차지 않았는데 정리가
필요해지면, 답은 M3가 아니라 **경로를 바꾸지 않는 표시**다 — 루트 `.gitattributes`와 README가
그 자리다.

| 점검 | 요구 |
|---|---|
| 상대 import | `poc`·`verification` 경유까지 closure 유지 |
| 동적 path | `import.meta.url`·cwd·상대 설정/출력 위치. 최소 `src/clauduct.mjs:204` `src/clauduct.mjs:229` `src/native-protocol.mjs:140` `poc/adapter.mjs:17` |
| 실행 script | root 가정·Node 경로·PowerShell 현재 위치 |
| 문서 | 링크·`파일:줄` 인용·현재/과거 state 인덱스 재검증. `node verification/test-doc-citations.mjs` 통과 |
| 테스트 fixture | 상대 탐색 경로·임시파일 생성 위치·tracked 검사 |
| CI | Node job의 working-directory와 path filter |
| 패키징 | 파일목록·SHA·설치 경로·재현성 |
| local state | `.tmp`·profile·status·사용자 인증을 함께 옮기지 않음 |
| 버전관리 | 파일별 rename mapping·전후 hash·동일내용 검증 |

이동 커밋과 로직 변경 커밋을 분리한다. 이동 전후 Node smoke를 **동일 fixture**로 비교한다. 각 entry는 이전·이후 경로와 content identity를 갖고, 실패하면 승인된 자기 변경만 되돌리는 역방향 변경을 제안한다. `reset --hard`·강제 push·전체 clean으로 원상복구하는 절차는 만들지 않는다.

## 7. untracked·ignored·worktree — 보존만 한다

읽거나 옮기거나 stage하지 않았다. 존재와 개수만 기록한다.

| 범위 | 개수 | 처분 |
|---|---|---|
| untracked (non-ignored) | 143 | `KEEP_LOCAL_PRIVATE`. `verification/` 110, `docs/` 30, root 2, `src/` 1 |
| ignored | 31,542 | `KEEP_LOCAL_PRIVATE`. `.tmp/` 31,091, `.clauduct-profile/` 455, `.clauduct-status/` 1, `.superpowers/` 1 |
| linked worktree | 7 | 보존. 1개 main + 6개 `.tmp/` 아래. 생성·repair·prune·제거 없음 |

HANDOFF.md 3장이 지정한 사용자 상태 위치(`.tmp/`, `clauduct-check.txt`, `clauduct-agent-validation-*.txt`, `docs/prompts/`, `docs/handoff-*.md`, `src/agent-selection.review-fixture.mjs`, `verification/dev-sandbox/`)를 그대로 따른다. `verification/` 아래에는 tracked 자료와 untracked 작업이 섞여 있으므로 폴더째 판단하지 않는다.

credential·세션 transcript·전체 환경변수 덤프를 manifest나 공통 fixture에 넣지 않았다. 불필요한 secret 해시도 만들지 않았다 — manifest의 content identity는 Git blob id를 그대로 쓴다.

사용자 홈의 `~/.claude`·`~/.codex`는 저장소 이관 대상이 아니다. 실행 파일 존재는 `stat`으로만 확인했고 credential 파일은 열지 않았다.

## 8. 재생성

manifest는 규칙 기반이라 재생성 가능하다. 생성 스크립트는 ignored 산출물(`.tmp/g0-inventory/manifest.py`)에 있다. 기준선 commit이 바뀌면 분모가 바뀌므로 manifest를 다시 만들고 `baseline_commit`을 갱신한다. 다른 commit의 manifest를 현재 상태로 인용하지 않는다.
