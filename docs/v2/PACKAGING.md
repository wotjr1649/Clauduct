# V2 패키징 — 무엇이 나가고, 무엇이 필요하고, 어떻게 되돌리는가

이 문서는 **출하되는 것**을 기술한다. 설계는 [ARCHITECTURE.md](ARCHITECTURE.md), v0.3.2까지의 증거는 [VALIDATION.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/VALIDATION.md)가 소유한다.

**설치된 `clauduct`는 이 Go 빌드다**(G9, 2026-09-17). 이전 Node 구현(v1)은 v0.3.3에서 저장소에서 은퇴했다(6장).

## 1. 나가는 것

| 파일 | 하는 일 |
|---|---|
| `clauduct.exe` | 제품. 설치된 `claude.exe`를 띄우고 모델 요청을 loopback gateway로 돌린다. 같은 파일이 hook·PDF 렌더러·`--dev` 명령이다 |
| `clauduct-hook.exe` · `clauduct-dev.exe` | **v0.4.x 릴리스에만** 올라가는 `clauduct.exe`의 바이트 동일 사본. 새 설치·업데이트는 받지 않는다(아래) |

하나뿐이다(v0.4.0부터, #112). 별도로 설치하는 설정 파일·스크립트·데이터 디렉터리는 없다. 이 빌드 자신에 대한 질문은 `clauduct --dev`로 한다(`--version`·`--doctor`·`--usage`·`--probe`). 첫 인자의 관리 명령(`--update`·`--usage`·`--uninstall`·`--dev`·`--background-stop`)과 내부 `--clauduct-*` 역할은 제품이 처리한다. `--bg`·`--background`는 native에 전달하면서 연결 유지 프로세스를 시작한다. 나머지는 `--version`·`--help`를 포함해 native로 그대로 간다. 전체 CLI 계약은 [ARCHITECTURE.md](ARCHITECTURE.md#4-cli-계약)를 따른다.

**사본을 올리는 이유**: 0.3.x의 `--update`는 세 이름(그 버전의 `update.Binaries`)이 모두 있는 릴리스만 받는다. 사본은 자기 이름이 말하는 역할을 하므로 0.3.x에서 업데이트한 설치는 세 파일로도 동작한다. 새 `--update`와 `install.ps1`은 `clauduct.exe`만 받고, 교체가 끝난 뒤 두 이름을 지운다. v0.5.0부터는 사본을 올리지 않으며, 그때 남은 0.3.x 설치는 `install.ps1`로 다시 설치한다.

**hook은 실행 중인 파일 자신이다.** `findHook()`은 `os.Executable()`이고, 세션 설정의 hook 명령은 `"<clauduct.exe>" --clauduct-hook`, gateway의 PDF 렌더러는 `--clauduct-render-pdf`, native가 찾는 `pdftoppm.exe`는 세션 임시 폴더에 둔 하드링크다(이름으로 역할을 고른다). PATH는 뒤지지 않는다 — 클라이언트는 그 결과를 실행하라는 말을 듣게 된다.


## 2. 실행에 필요한 것

`clauduct`는 단일 정적 Go 바이너리지만 **혼자 동작하지는 않는다.**

| 필요한 것 | 왜 | 없으면 |
|---|---|---|
| `claude.exe` | 띄울 대상이다. `~/.local/bin` 우선, 그다음 PATH | `CLAUDE_NOT_FOUND`, 찾아본 경로를 함께 출력 |
| `codex.exe` | **버전이 모든 요청의 header에 들어간다.** 없으면 보낼 것을 만들 수 없다 | 요청이 `CODEX_NOT_FOUND`로 거부된다. 세션은 시작되고 `--version`·`--help`는 정상 |
| `~/.codex/auth.json` | ChatGPT 구독 credential. 읽기 전용으로만 접근한다 | 요청이 `CREDENTIAL_UNAVAILABLE_OR_EXPIRED`(503). `codex login`으로 회복 |

**`codex.exe` 의존은 G7이 추가한 것이다.** 이 문서가 그것을 적는 이유는 REL03이 "Node adapter·.NET probe에 의존하지 않음"만 묻기 때문이다 — 그 둘은 실제로 없지만, **런타임 의존이 0이라는 뜻은 아니다.** 정직한 목록은 위의 셋이다.

Node도 .NET도 필요 없다. `internal/app`의 스캔이 제품 소스에 `.mjs` 경로·`node.exe`·`npm.cmd`·.NET probe 이름이 들어가지 못하게 막는다.

## 3. ⚠ 이 바이너리는 실제로 과금된다

**2026-09-15 G7부터** `clauduct`가 시작한 모든 추론 요청은 사용자의 Codex 구독에 도달한다. 제품 세션에는 **요청 수 상한이 없다** — Node 기준선에도 없고, 상한을 두면 긴 세션이 중간에 멈춘다.

모델을 호출하지 않는 명령은 아무것도 쓰지 않는다. `--version`·`--help`는 credential을 읽지 않고 `codex --version`도 띄우지 않는다. 둘 다 **첫 요청**에서만 일어난다. 테스트가 그것을 고정한다.

검증용 실호출은 별개의 예산이다. v0.4.3의 `CLAUDUCT_VERIFICATION_BUDGET`은 여러 프로세스·worker 재기동에 걸쳐 모델·effort와 시도 상한을 전송 전에 강제한다. 실제 출하 바이너리의 protocol 버전도 실행 전에 확인한다. 형식과 운영 조건은 [개발 안내](../../go/README.md#검증용-실호출은-별개의-예산이다)를 따른다.

## 4. 빌드와 신원

```powershell
cd go
$env:CGO_ENABLED = '0'
go build -trimpath -o clauduct.exe      ./cmd/clauduct
# 하나다. hook·PDF 렌더러·`--dev` 명령이 같은 파일이다(#112, 1장).
# v0.4.x 릴리스는 이것을 clauduct-hook.exe·clauduct-dev.exe로도 복사해 올린다(4.1).
```

`-trimpath`는 빌드 머신의 디렉터리 배치가 바이너리에 남지 않게 한다. `CGO_ENABLED=0`은
**빌드 머신에 C 툴체인이 있는지에 산출물이 의존하지 않게** 한다 — 이 모듈은 `import "C"`가
없어서 Windows에서 동작은 같지만, cgo가 켜진 채로 빌드하면 바이트가 달라진다. 재현성 테스트는
같은 환경에서 두 번 빌드해 비교하므로 이 차이를 볼 수 없고, 그래서 산출물 쪽에서 고정한다
(`TestTheShippedBinaryIsBuiltWithoutCgo`). 예외는 race job 하나이고, 그 바이너리는 출하 대상이
아니다. 버전과 commit stamp는 Go toolchain의 VCS 기록에서 나오므로 **릴리스 스크립트가 잊을 수 없다.**

```powershell
clauduct --dev --version
# clauduct     <태그. 태그 없는 commit은 pseudo-version, 수정된 worktree는 끝에 +dirty>
# commit       <40자 hash>
# go           go1.27.1 windows/amd64
```

worktree가 수정된 상태로 빌드하면 commit 뒤에 `+dirty`가 붙는다. **그런 빌드는 릴리스 후보가 아니다.**

### 재현성

같은 소스·같은 Go 버전에서 **두 번 빌드하면 바이트가 같다.** 테스트가 두 번 빌드해 SHA256을 비교한다.

이것이 checksum을 의미 있게 만든다. "이 commit에서 빌드했다"는 바이너리가 자기에 대해 하는 주장이지만, 해시가 같다는 것은 **누구나 확인할 수 있는 주장**이다.

## 4.1 릴리스 만들기

`--update`가 읽는 것은 **최신 태그 릴리스의 자산**이므로, 이름과 형식이 계약이다. 태그 전 로컬 검사에는 과금 없는 TUI 검사를 `go/cmd/ptydrive`로 돌리는 것이 들어간다([go/README.md](../../go/README.md#tui-검사-conpty)). 이 도구는 자산이 아니다.

```powershell
# 0. 버전을 올리는 PR은 v0.4.0부터 없다. go.mod가 저장소 루트에 있어 toolchain이 태그를 그대로
#    버전으로 stamp한다(#111). v0.3.x까지는 태그보다 먼저 상수를 올리는 커밋이 필요했고, v0.3.2는
#    상수가 0.3.1인 커밋에 태그가 붙을 뻔했다. 아래 검사가 버전과 태그를 그대로 비교한다.
$tag = 'v0.4.0'

# 1. 깨끗한 체크아웃에서 빌드한다. 작업 트리에 untracked 파일만 있어도 commit 스탬프에
#    +dirty가 붙고, 그런 빌드는 릴리스 후보가 아니다(4장).
git worktree add ../clauduct-release $tag
cd ../clauduct-release/go
$env:CGO_ENABLED = '0'
# 산출물은 반드시 트리 **바깥**으로. 안에 쓰면 두 번째 빌드부터 자기가 만든 exe 때문에
# 트리가 dirty가 되고 commit 스탬프에 +dirty가 붙는다 — 이 절차를 처음 실행하면서 실제로
# 겪었다.
go build -trimpath -o ../../release-assets/clauduct.exe      ./cmd/clauduct
# v0.4.x 동안(1장): 0.3.x의 --update가 받을 수 있도록 같은 바이트를 옛 이름으로도 둔다.
'clauduct-hook.exe', 'clauduct-dev.exe' | ForEach-Object { Copy-Item ../../release-assets/clauduct.exe ../../release-assets/$_ }

# 버전이 태그와 같고 스탬프에 +dirty가 없는지 확인한다. 어긋나면 그 빌드는 릴리스 후보가 아니다.
$version = & ../../release-assets/clauduct.exe --dev --version
$version
if (-not ($version -match "^clauduct\s+$([regex]::Escape($tag))$")) { throw "VERSION_MISMATCH $tag" }
if ($version -match '\+dirty') { throw 'DIRTY_BUILD' }

# 2. 자산 이름 그대로 SHA256SUMS를 만든다. 파서는 공백으로 나뉜 두 필드를 읽고 이름 앞의
#    `*`(sha256sum의 binary 표시)를 떼므로 `<hex>  <name>`과 `<hex> *<name>` 둘 다 받는다.
#    v0.2.0과 v0.2.1은 Windows sha256sum이 기본으로 내는 `*` 형식으로 나갔다.
# 3. 스크립트 둘을 자산에 함께 올린다. 저장소가 없는 머신이 설치하는 경로가 그것이다.
#    cp <checkout>/scripts/install.ps1 <checkout>/scripts/uninstall.ps1 ../../release-assets/
# 4. gh release create $tag clauduct.exe clauduct-hook.exe clauduct-dev.exe `
#        install.ps1 uninstall.ps1 SHA256SUMS      # 사본 둘은 v0.4.x까지(1장)
```

| 자산 이름 | 왜 이 이름이어야 하나 |
|---|---|
| `clauduct.exe` | `update.Binaries`가 이 이름으로 찾는다. 없으면 이름으로 말하고 멈춘다. **v0.4.x까지는 `clauduct-hook.exe`·`clauduct-dev.exe`도 같은 바이트로 올리고 `SHA256SUMS`에 셋을 적는다** — 0.3.x의 `update.Binaries`가 세 이름을 모두 요구한다 |
| `install.ps1` · `uninstall.ps1` | `releases/latest/download/install.ps1`이 동작하게 하는 것이 이 자산이다. `--update`는 이 둘을 건드리지 않는다 — 설치된 집합이 아니다 |
| `SHA256SUMS` | 유일한 무결성 근거다. 서명이 없으므로 여기에 적힌 digest와 릴리스 API가 말하는 digest **둘 다** 대조한다. **스크립트 둘도 여기 적는다** — 받아서 실행하라고 안내하는 파일이므로 대조할 수단이 있어야 한다. `update.Sums`는 이름으로 찾으므로 추가 항목은 무시된다 |

**드래프트나 pre-release로 두면 `--update`가 보지 못한다.** GitHub의 `releases/latest`가 그 둘을
건너뛰기 때문이다.

재현 빌드이므로 같은 태그·같은 Go 버전이면 누가 빌드해도 digest가 같아야 한다. 다르면 둘 중
하나가 위 절차를 벗어난 것이다. v0.2.0에서 세 바이너리 모두 두 번 빌드해 바이트 일치를 확인했다.

**발행 뒤에는 실제로 받아 본다.** `clauduct --update`가 릴리스를 읽고, digest를 대조하고, 교체하는
것까지가 이 절차의 끝이다.

**구버전→신버전 경로는 v0.2.2에서 실측됐다.** 설치된 0.2.1이 릴리스를 읽고 화면에 digest 3개를
띄운 뒤 교체했고, 그 세 값은 발행된 `SHA256SUMS`와 같았다. 이어서 네 가지가 순서대로 확인됐다:

| 잰 것 | 결과 |
|---|---|
| 0.2.1 → 0.2.2 교체 | `updated to v0.2.2`, `clauduct.exe.old` 하나 남음(교체한 것이 sweep 없는 구버전이므로 정상) |
| 다음 `clauduct` 실행 | 그 `.old`가 **자동으로 사라짐** |
| 이미 최신에서 다시 `--update` | 교체하지 않고 `already current`, **새 `.old` 0개** |
| 설치본 | `0.2.2`/`8730671` |

세 번째 줄이 v0.2.2가 고친 전부다. 그 전에는 `--update`를 돌릴 때마다 `.old`가 하나씩 쌓였다 —
실사용에서 보고됐다.

`--update`는 대화형 stdin이 없으면 **묻고 EOF를 거부로 읽어 아무것도 바꾸지 않는다.** 실측:
`!` 프롬프트처럼 입력이 붙지 않는 셸에서 `clauduct: nothing was changed`로 끝났다. 무인 실행에는
`--yes`가 필요하고, 그것이 이 옵션이 존재하는 이유다.

## 5. 설치

PATH에 있는 디렉터리에 **`clauduct.exe` 하나**를 복사한다. 0.3.x가 남긴 `clauduct-hook.exe`·`clauduct-dev.exe`는
교체가 끝난 뒤 지운다(1장). hook은 실행 중인 `clauduct.exe` 자신이라 옆에 둘 것이 없다.

**그 디렉터리에 `clauduct`라는 이름의 폴더가 있으면 안 된다.** MSYS/Git Bash의 PATH 탐색은 같은
이름의 디렉터리에서 멈추고 `clauduct.exe`에 도달하지 못한다 — cmd는 PATHEXT로 찾으므로 셸에 따라
동작이 갈린다. v1 설치 프로그램이 `$bin/clauduct`를 버전 저장소로 쓰므로 실제로 일어났고,
2026-09-17에 그 저장소를 `clauduct-node-store`로 옮겨 해결했다(PARITY 11장 G9).

**설치 디렉터리에 쓰지 않는다.** 테스트가 확인한다 — 다른 cwd에서 실행한 뒤 설치 디렉터리에 무엇이 생겼는지 전후 비교한다. 따라서 공유 경로나 쓰기 금지 경로에 둘 수 있다.

대화 transcript는 native가 소유하며 `CLAUDE_CONFIG_DIR`(기본 `~/.claude`) 아래에 있다.
Clauduct도 같은 projects 트리에 선택·context·Workflow 복원용 metadata를 저장한다.
설치 파일 교체와 이 세션 기록의 호환성은 별도로 확인해야 한다.

**그 결정의 대가 (2026-09-18 관측).** 격리하지 않으므로 clauduct 세션이 고른 모델이 사용자의
`~/.claude.json`에 남는다 — `clientDataCacheSlots.bi1-*.model`에 `gpt-6-astra`가 들어가고, 그 다음
native `claude`가 "이 버전 카탈로그에 없는 모델"이라고 경고한다. 첫 실사용에서 실제로 나왔다.

완화는 `behavesAs`가 **아니다.** 넣으면 경고는 사라지지만 effort를 함께 가져가서 low가 medium이 된다고
실측돼 있다(`app/settings.go`의 `pickerRows` 주석). 경고가 가리키는 컨텍스트 창 문제는
`CLAUDE_CODE_MAX_CONTEXT_TOKENS`가 이미 따로 해결한다.

지금의 완화는 안내다 — clauduct에서 모델을 바꾸면 그 선택이 native 쪽에도 보인다. 구조적 해결(설정
격리)은 결정거리로 남아 있고 아직 하지 않았다.

### 5.0 스크립트

`scripts/install.ps1`과 `scripts/uninstall.ps1`이 그 복사를 대신한다. v0.3.2까지 저장소 루트에
있던 `install.ps1`은 v1(Node) 설치기였고 v0.3.3에서 저장소와 함께 은퇴했다.

```powershell
# 릴리스에서 설치 (기본 최신 태그, 기본 위치 ~\.local\bin)
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\install.ps1
scripts\install.ps1 -Tag v0.2.0                 # 태그 고정
scripts\install.ps1 -FromPath .\dist            # 로컬 빌드 설치 (clauduct.exe + SHA256SUMS 필요)
scripts\install.ps1 -NoPathUpdate               # PATH를 건드리지 않는다

scripts\uninstall.ps1                           # clauduct.exe, 0.3.x의 두 이름, --update가 남긴 *.old
scripts\uninstall.ps1 -Purge                    # %TEMP%\clauduct 진단 파일까지
scripts\uninstall.ps1 -RemovePath               # 그 디렉터리에 다른 실행 파일이 없을 때만
```

**내려받기 전에 Claude Code와 Codex CLI를 찾는다.** 둘 중 하나라도 없으면
`INSTALL_PREREQUISITE_MISSING`과 함께 런타임과 **같은 이름**(`CLAUDE_NOT_FOUND`·`CODEX_NOT_FOUND`)을
내고 멈춘다 — 어느 표면이 보고했든 검색하면 같은 답에 닿게 하기 위해서다. 찾는 방식은
`platform.Resolver`를 그대로 따른다: 표준 위치(`~\.local\bin`) 먼저, 그다음 PATH를 **64개까지**,
따옴표 제거, 절대 경로만, **작업 디렉터리 제외**, 중복 제거. Codex는 두 곳을 더 본다(v0.3.5) — PATH 앞에 Codex 앱의 설치 위치(`~\AppData\Local\Programs\OpenAI\Codex\bin`), 맨 끝에 npm 설치(`~\AppData\Roaming\npm`과 위 PATH 항목 아래)가 싣는 native `codex.exe`(`node_modules\@openai\codex\node_modules\@openai\codex-win32-x64\vendor\x86_64-pc-windows-msvc\bin`). Node로 `codex.js`를 돌리지 않는다. 런처보다 넓게 찾으면 통과시켜 놓고
첫 실행에서 `CLAUDE_NOT_FOUND`가 나는데, **런타임과 어긋나는 사전 검증은 없느니만 못하다.**
`-SkipPreflight`로 건너뛴다. 한 가지 차이는 숨기지 않고 적는다 — 런처는 `%USERPROFILE%`이
오염된 채 넘어올 수 있어 OS 사용자 기록에서 홈을 읽지만, 스크립트는 사용자 자신의 셸에서 도므로
`$env:USERPROFILE`을 쓴다.

**검증이 끝나기 전에는 하나도 복사하지 않는다.** digest가 어긋나면 `INSTALL_DIGEST_MISMATCH`로
멈추고 대상 디렉터리는 손대지 않은 상태로 남는다. 새 바이너리 옆에 옛 바이너리는 어떤
릴리스도 그 조합으로 시험된 적이 없다. 복사가 시작된 뒤에도 같다(v0.3.5): 새 파일을 대상 옆에 `.new`로 먼저 복사하고, 현재 것을 `.old`로 이름을 바꾼 뒤 `.new`를 제자리에 놓는다(`--update`의 `update.Apply`와 같은 방식). 실행 중인 바이너리는 덮어쓸 수 없지만 이름은 바꿀 수 있어 설치가 통과하고, 지우지 못한 `.old`는 이름을 알린다. 어느 단계든 실패하면 원래 파일을 되돌리고 `INSTALL_SWAP_FAILED`로 멈춘다. 한계도 `--update`와 같다 — 이전 교체가 남긴 `.old`가 아직 실행 중이면 그 이름을 비울 수 없어 거부하고 원래 파일을 둔다. 그 세션이 끝난 뒤 다시 설치한다. 0.3.x가 남긴 두 이름은 교체가 끝난 뒤에만 지운다(v0.4.0) — 업데이트 전에 시작된 세션이 hook으로 실행 중이면 남기고 이름을 알린다. 지워진 뒤에는 그 세션의 하위 에이전트와 PDF가 hook 없이 거부되므로, **설치 전에 시작한 세션은 다시 시작한다.** 설치할 파일은 릴리스의 digest로 정한다: `clauduct-hook.exe`가 `clauduct.exe`와 다른 0.3.x 릴리스(`-Tag`로 되돌릴 때)는 셋을 모두 두고 아무것도 지우지 않는다.

**설치 디렉터리에 `clauduct`라는 이름의 폴더가 있으면 거부한다**(`INSTALL_DIRECTORY_SHADOW`).
이유는 1장과 같다 — Git Bash가 거기서 멈춘다.

**PATH는 레지스트리에서 원문으로 읽고 같은 값 종류로 되돌려 쓴다.** .NET의
`GetEnvironmentVariable`은 `%USERPROFILE%`을 펼쳐서 주고 `SetEnvironmentVariable`은 `REG_SZ`로
저장하므로, 순진한 read-modify-write는 남은 `%VAR%` 항목을 **영구히** 죽인다. `setx`는 1024자에서
자른다. 쓰고 나서 `WM_SETTINGCHANGE`를 뿌리는데, 이게 없으면 작업 표시줄에서 연 터미널이 다음
로그인까지 옛 PATH를 쓴다.

**제거는 자기 것만 지운다.** `clauduct-node.cmd`·`clauduct-node-store`·`CLAUDE_CONFIG_DIR` 아래는
건드리지 않는다. PATH 항목은 기본으로 두며, `-RemovePath`를 줘도 그 디렉터리에 다른 실행 파일이
남아 있으면 `UNINSTALL_PATH_SHARED`로 거부한다 — 기본 설치에서 `claude.exe`가 거기 산다.

**digest는 BCL로 계산하고 `-cne`로 비교한다.** `Get-FileHash`는 스냅인이 아니라
`Microsoft.PowerShell.Utility` **모듈**이 얹어주는 cmdlet이라, PowerShell 7 세션에서 시작된
`powershell.exe`가 PS7의 모듈 디렉터리를 먼저 보게 되면 **그 이름이 사라진다.** 2026-09-17 실측:
같은 실행 파일, 같은 5.1.26100.8870, FullLanguage인데 PSModulePath 항목이 3개에서 6개가 되고
`Get-FileHash`만 없어진다 — `Unblock-File`·`Invoke-WebRequest`·`Add-Type`·`New-Object`는 멀쩡하다.
README가 시키는 `powershell -File install.ps1`을 PowerShell 7 터미널에서 실행하는 것이 정확히 그
모양이고, CI의 go 스텝이 pwsh로 도는 덕에 잡혔다. 비교는 `-ne`가 아니라 `-cne`다 — PowerShell의
문자열 비교는 기본이 대소문자 무시라 정규화가 깨져도 조용히 통과한다. 테스트는 pwsh가 있으면 그
그림자를 **일부러 만들어** 돌므로 bash에서 돌려도 CI와 같은 것을 잰다.

**설치 후 `Unblock-File`을 건다.** 방금 릴리스의 digest로 확인한 바이트이고, 그것이 SmartScreen
대화상자가 묻는 질문이다.

| 검사됨 | `internal/app/install_windows_test.go`(비공개 테스트) 11건 — 셋 배치, 변조 거부(부분 복사 0), 폴더 그림자 거부, 대문자 digest 수용, 제거가 남의 파일을 안 지움, 그리고 사전 검증 3건(둘 다 있으면 통과, 없으면 이름과 함께 거부, **작업 디렉터리에만 있는 것은 못 본 척**). 돌연변이 11건 전부 잡힌다. v0.3.5에 교체 2건 — 배타 잠금된 대상이면 이전 셋 복원, 실행 중인 대상은 이름을 바꿔 교체. v0.3.4 스크립트에서 둘 다 실패하고, 복원 줄을 지운 변이는 잠금 쪽이 잡는다. 사전 검증 1건 — Codex가 앱 설치 위치나 npm 설치에만 있어도 찾는다(v0.3.4 스크립트에서 실패). v0.4.0(#112): v0.4 릴리스는 `clauduct.exe` 하나를 두고 옛 두 이름을 지우며, 0.3.x 릴리스는 셋을 둔다(각각 검사). 잠금 복원은 0.3.x 릴리스의 세 대상으로 검사하고, 실행 중인 옛 사본은 남기고 이름을 알린다 |
|---|---|
| 실측 2026-09-17 | **다운로드 경로가 실환경에서 돌았다.** v0.2.1 자산을 릴리스 URL에서 받아 digest를 대조하고 설치했고, 받은 세 파일이 `SHA256SUMS`와 일치했다. 사전 검증도 이때 처음 실제 머신에서 돌아 `claude.exe`와 `codex.exe`를 찾았다. 임시 폴더로 한 번, 이어서 공식 경로 `~\.local\bin`으로 한 번 — 두 번째는 PATH가 이미 있어 `already on it`으로 끝났다 |
|---|---|
| **검사 안 됨** | **PATH 쓰기**(테스트가 실행 머신의 레지스트리를 고쳐서는 안 된다). v1도 같은 이유로 제외했다. 사전 검증은 PATH와 `USERPROFILE`을 테스트가 소유한 값으로 갈아끼워 검사하므로 러너에 무엇이 깔렸는지에 좌우되지 않는다 |

### 5.0.1 SmartScreen — 재보고 나서 서명하지 않기로 했다

**언제 뜨는가.** 브라우저로 내려받은 exe를 탐색기에서 더블클릭할 때만이다. 2026-09-17 실측:

| 잰 것 | 결과 |
|---|---|
| 설치본 3개의 `Zone.Identifier` | **없다.** `--update`는 Go의 파일 쓰기로 내려받으므로 MOTW가 붙지 않는다 |
| MOTW(`ZoneId=3`)를 일부러 붙인 사본을 터미널에서 실행 | **경고 없이 `exit=0`.** SmartScreen 평판 검사는 `ShellExecute` 경로이지 `CreateProcess`가 아니다 |

이 제품은 터미널에서 이름을 쳐서 쓰는 런처다. **실사용 경로에는 그 대화상자가 존재하지 않는다.**
설치 스크립트는 digest를 대조한 직후 `Unblock-File`을 걸어 수동 다운로드 경로까지 덮는다.

**결정(2026-09-17, 사용자): 서명하지 않는다.** 돈이 아니라 효과가 없어서다.

- **EV 인증서의 즉시 SmartScreen 평판은 2024년에 폐지됐다.** 지금은 EV도 OV도 평판을 새로
  쌓아야 한다 — 어떤 가격에도 대화상자를 즉시 없애는 상품이 없다.
- 평판은 **다운로드 수**로 쌓인다. v0.2.0의 `clauduct.exe`는 3회이고 그중 대부분이 개발자 자신이다.
- Azure Artifact Signing($9.99/월)은 개인 개발자 기준 **미국·캐나다 한정**이고 법인도 미국·캐나다·
  EU·영국이다. 한국은 어느 쪽도 아니다.
- 남는 현실적 경로인 Certum 오픈소스 인증서(클라우드 연 €49)는 **게시자 줄이 "Open Source
  Developer &lt;이름&gt;"으로 고정**되고 **상용 배포에 쓰면 취소**되며, 2026-02-27부터 인증서 유효
  상한이 459일이라 **사는 순간 시계가 돈다.**

**이 결정이 받아들이는 것.** WDAC·AppLocker·Smart App Control이 강제된 머신에서는 서명 없는
바이너리에 「실행」 버튼조차 없다 — 그런 머신에는 설치할 수 없고, 이것은 회피가 아니라 한계다.
Defender 오탐이 나도 내놓을 근거가 `SHA256SUMS`뿐이다. 이 결정이 틀려지는 조건은 그 둘 중 하나가
실제로 보고되는 것이다.

## 5.1 업데이트

```powershell
clauduct --update        # 무엇이 바뀌는지 보여주고 확인을 받는다
clauduct --update --yes  # 무인
```

기준은 **최신 태그 릴리스**다. 순서가 안전의 전부다: 릴리스 메타데이터 → `SHA256SUMS` →
**사용자에게 태그와 digest를 보여주고 확인** → 내려받아 전부 검증 → 그 다음에야 교체.
digest가 하나라도 어긋나면 디스크는 손도 대지 않았다고 말하고 끝난다. 릴리스 API가 자기 digest를
싣고 있으면 그것과도 대조한다 — 둘이 일치하는 것은 값이 적지만, **둘이 다르면 멈출 이유**다.

교체 대상은 `clauduct.exe` 하나다(v0.4.0). 실행 중인 exe는 덮어쓸 수
없지만 이름은 바꿀 수 있으므로 `*.old`로 옮기고 새 파일을 쓴다. 중간에 실패하면 옮긴 것을 전부
되돌린다. 성공 후 `clauduct.exe.old`는 **이 프로세스가 끝나야** 지울 수 있고, 그래서 지우라고
출력한다. 교체가 끝난 뒤 0.3.x가 남긴 `clauduct-hook.exe`·`clauduct-dev.exe`를 지우고, 실행 중이라 못 지운 것은 이름을 알린다. 이미 최신이면 그 둘도 건드리지 않는다. 지울 사본은 확인 질문 전에 이름으로 보여준다 — 업데이트 전에 시작한 세션은 그 뒤 hook을 잃으므로 다시 시작한다.

**이미 최신이면 아무것도 하지 않는다.** 설치된 `clauduct.exe`의 digest가 릴리스가 말하는 것과 전부
같으면 묻지 않고 `already current`로 끝난다. 버전 문자열이 아니라 digest로 보는 이유는, 버전은
빌드가 자기를 부르는 이름이고 두 빌드가 같은 이름을 쓸 수 있기 때문이다 — 손으로 갈아끼웠거나
반만 받아진 파일은 일치하지 않으므로 그때는 고쳐진다. 이 검사가 없으면 같은 바이트로 교체하고
`clauduct.exe.old`를 남기는데, **아무것도 바꾸지 않은 업데이트가 남긴 잔여물은 고장으로 읽힌다.**

**그 잔여물은 다음 실행이 치운다.** 실행 중인 이미지를 Windows가 붙들고 있어 자기 선대를 지울 수
있는 첫 프로세스는 다음 실행이다. 자기 옆의 자기 이름 `.old` 하나만, 조용히, 실패는 무시한다 —
housekeeping 때문에 세션이 죽는 것이 잔여물보다 나쁘다.

`clauduct update`(하이픈 없음)는 이 명령이 아니다 — 그대로 클라이언트에 전달되어 **Claude Code**가
갱신된다. 클라이언트에는 `--update` 옵션이 없으므로(2.1.274 실측) 이 이름은 아무것도 가리지 않는다.

**릴리스는 있다.** `v0.2.0`이 `clauduct.exe`·`clauduct-hook.exe`·`clauduct-dev.exe`와
`SHA256SUMS`를 싣고 있고 draft도 pre-release도 아니다(2026-09-17 확인). `v0.1.0`은 Node 구현의
zip이고 그대로 둔다. 릴리스에 자산이 빠져 있으면 `--update`는 무엇이 빠졌는지 이름으로 말하고
종료하며, 인증 없는 GitHub API는 주소당 시간당 60회이므로 한도에 걸리면 그것도 이름으로
말한다(`RATE_LIMITED`, 재시도 시각 포함).

## 5.1.1 제거

```powershell
clauduct --uninstall        # 무엇을 지울지 보여주고 확인을 받는다
clauduct --uninstall --yes  # 무인
```

**스크립트가 아니라 바이너리가 정식 경로다.** `scripts/uninstall.ps1`은 저장소를 필요로 하는데,
릴리스로 설치한 머신에는 바이너리만 있고 그게 바로 지우고 싶은 그 머신이다. 스크립트는 바이너리가
깨졌을 때의 예비로 남는다.

인식 규칙은 `--update`와 같다 — **첫 인자일 때만**, 뒤에 올 수 있는 것은 `--yes`뿐이다. 전체 인자를
훑으면 `clauduct -p "how do I --uninstall"`이 설치를 지운다. 프롬프트는 인자다.

**자기를 먼저 치운다.** hook만 사라지고 `clauduct.exe`가 남으면 그건 부분 제거가 아니라
**역할 라우팅이 조용히 죽은 멀쩡한 설치**다(1장). 그래서 실행 중인 자신을 옮기지 못하면 나머지도
손대지 않고 `nothing was removed`로 끝난다.

**실측 2026-09-17 (v0.2.3).** 실제 설치본에서 제거했더니 `~\.local\bin`에 `.old`가 0개였고
`%TEMP%\clauduct-removed-15792.exe`가 그 자리를 대신했다. 재설치 후 셋만 남았다 — 이 경로가
합성 폴더가 아니라 진짜 설치에서 확인된 지점이다.

**어디로 옮기느냐가 `.old`가 남느냐를 가른다.** Windows는 실행 중인 이미지의 삭제는 거부하지만
rename은 허용하고, **같은 볼륨이면 다른 폴더로도 옮겨진다** — 복사가 아니라 NTFS rename 한 번이기
때문이다. 2026-09-17 실측: 원래 폴더는 비고 프로세스는 계속 돈다. 그래서 `%TEMP%\clauduct-removed-<pid>.exe`로
보낸다. **제거는 다음 실행이 치워줄 수 없는 유일한 경우다** — 제거 뒤에는 다음 실행이 없다. OS가
이미 비우는 곳에 두는 것이 설치 폴더를 실제로 비우는 유일한 방법이다.

볼륨이 다르면 rename이 복사가 되어 실패하므로 그때는 옛 방식대로 옆에 `.old`로 두고 지우는 한 줄을
출력한다.

**PATH는 건드리지 않는다.** 안전한 편집은 레지스트리 원문 읽기와 값 종류 보존이 필요하고(5.0절),
그건 이 모듈에 없는 의존성을 요구하며 설치 경로에서 **유일하게 사용자가 부탁하지 않은 것을 부술 수
있는 자리**다. 게다가 기본 설치에서는 `claude.exe`가 같은 폴더에 살아 어차피 거절될 상황이 거의
전부다. 무엇이 남았는지 알려주고, 비었으면 `scripts/uninstall.ps1 -RemovePath`를 가리킨다.

**진단 파일도 지우지 않는다.** 개수와 경로만 알린다 — 3장의 판단 그대로, 패턴으로 OS 임시
디렉터리의 파일을 지우는 것은 더 나쁜 거래다. 쓸어내려면 `scripts/uninstall.ps1 -Purge`다.

`clauduct-node.cmd`·버전 저장소·`CLAUDE_CONFIG_DIR` 아래는 어느 경로로도 지우지 않는다.

| 검사됨 | `internal/update/uninstall_test.go`(비공개 테스트)의 제거 검사 8건 — 첫 인자 규칙(프롬프트 안의 옵션 포함), 셋+`*.old` 제거와 남의 파일 보존, 설치 밖으로 옮기지 못하면 옆 이름으로 옮기고 붙여 넣을 삭제 명령 출력, 무응답은 거부, 묻기 전에 목록을 먼저 출력, 자기 이동 실패 시 전부 보존, 빈 디렉터리, 진단은 세기만 함. 돌연변이 6건은 옆 이름 대체가 들어오기 전의 7건 기준으로 전부 잡혔다 |
|---|---|
| 실측 2026-09-17 | **실제 설치본에서 돌렸다.** `~\.local\bin`의 셋이 사라지고 `clauduct` 명령이 해소되지 않게 됐다. `clauduct-node.cmd`·버전 저장소·`claude.exe`·PATH 항목은 그대로였고, 진단 194개도 세기만 했다. 실행 중인 exe를 옮기는 경로 — 테스트가 잠긴 파일로는 재현하지 못하던 그것 — 가 이때 처음 확인됐다 |

## 5.2 이 계정이 얼마나 썼는지

```powershell
clauduct --usage       # 또는 clauduct --dev --usage — 같은 뷰다
```

주간/보조 창의 사용률, 남은 시간, 어느 family가 in force인지를 보여준다. **요청을 만들지 않는다** —
세션이 남긴 계정 파일에서 읽고, 그 값이 얼마나 오래된 것인지를 함께 적는다. 현재 값으로 제시된
낡은 숫자는 숫자가 없는 것보다 나쁘다.

같은 값의 한 조각이 **모든 세션 종료 줄**에도 붙는다(`quota=47%/7d`). 묻지 않아도 보이라는 뜻이고,
그 이상은 위 명령이 맡는다.

클라이언트의 `/usage`·`/cost`는 이 숫자를 보여주지 못한다 — 커스텀 base URL에는 계정 엔드포인트를
묻지 않는 것으로 실측됐다. 5.1절의 `--update`와 마찬가지로 이 옵션도 **첫 인자일 때만** 인식한다.

## 6. 되돌리기

**v0.3.1 개발본 → v0.3.0의 세션 기록 제한.** `customRole` 필드 또는 `native-selection`
출처를 가진 새 선택 journal은 v0.3.0 reader가 거부한다. 구형 형식 대조군은 그대로 읽혔다.
바이너리를 되돌려도 이 자식의 같은 세션 재개가 보장되지는 않는다. v0.3.1에서 해당 세션을
계속하거나, v0.3.0에서는 새 세션을 시작한다. 기록을 삭제하거나 필드를 제거해 검증을
통과시키지 않는다. [합성 journal 대조 근거](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-integration-20260921/REPORT.md).

내장 역할의 대소문자 정규화도 역호환 경계다. 저장된 `Plan`과 native metadata의 `plan`처럼
철자가 다르면 v0.3.1은 일치로 검증하지만 v0.3.0은 정확 비교에서 거부한다.
v0.3.1 writer가 만든 bytes를 그대로 v0.3.0 reader에 넣어 확인했다.
[대조군과 검사 범위](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-release-20260923/README.md). 구버전으로 되돌릴 때는
새 세션을 시작하는 경로를 사용하고, 기존 세션은 해당 기록을 쓴 버전에서 계속한다.

**Node 구현으로 되돌리는 경로는 v0.3.3에서 닫았다.** 이 저장소는 더 이상 Node 구현을 싣지 않는다.
이미 설치된 `clauduct-node`는 이 문서의 설치·업데이트·제거가 건드리지 않으므로 그대로 동작한다.
그 소스가 필요하면 [분리 직전 커밋](https://github.com/wotjr1649/Clauduct/tree/1b1c5e19b3f33fda63254b2da7c9d0b372553481)에서 받는다.
같은 디렉터리에 `clauduct.exe`와 `clauduct.cmd`가 함께 있으면 `PATHEXT` 순서상 `.exe`가 이긴다.

## 7. 아직 없는 것

| 항목 | 상태 |
|---|---|
| installer | MSI·서비스·레지스트리 등록은 없다. 파일 복사가 설치이고 `scripts/install.ps1`이 그 복사와 PATH를 맡는다(5.0절) |
| 자동 업데이트 | 배경에서 도는 것은 없다. 사용자가 부르는 `clauduct --update`는 있다(5.1절) |
| Windows 외 대상 | `internal/platform`에 windows 태그 파일 하나뿐이다. 다른 대상은 이식이 아니라 **새 설계**다 |
| 서명 | **없고, 넣지 않기로 했다**(5.0.1절). 이유는 비용이 아니라 효과다 — 2024년 이후 어떤 인증서도 SmartScreen을 즉시 통과시키지 못한다 |
| 공개 CI의 테스트 | **없다(v0.3.3부터).** 공개 CI는 Windows gofmt·vet·build와 httpguard의 Linux·macOS build만 본다. 테스트·race·문서 인용 검사는 출하 전 로컬 검사로만 돈다 |

**미실행은 통과가 아니다.** 각 항목은 없다고 적혀 있지 괜찮다고 적혀 있지 않다.
