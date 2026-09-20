# Clauduct

Claude Code 인터페이스의 모델 요청을 로컬 게이트웨이를 통해 기존 Codex 로그인으로 라우팅합니다.
Read/Edit/Bash/MCP 실행과 사용자 승인은 Claude Code가 그대로 담당합니다.

추론 경로는 **Claude Code → Clauduct의 127.0.0.1 gateway → ChatGPT Codex backend 직접 HTTPS**입니다.
Codex app-server를 호출하거나 실행 중인 Codex 앱 세션에 요청을 넘기는 구현이 아닙니다.

## 구현이 둘입니다

이름이 다르므로 동시에 설치해도 서로를 가리지 않습니다.

| 이름 | 무엇 | 상태 | 현행 문서 |
|---|---|---|---|
| `clauduct` | Go 단일 바이너리 (+`clauduct-hook`, `clauduct-dev`) | **현재 제품** | [docs/v2/README.md](docs/v2/README.md) |
| `clauduct-node` | Node 구현 | 이전 제품 · 비교 기준선 · 되돌리기 경로 | [HANDOFF.md](HANDOFF.md) |

**어느 쪽 문서를 읽고 있는지가 중요합니다.** 이 저장소의 v1 문서는 Node 구현을 기술하며 Go 빌드에
그대로 적용되지 않습니다 — 게이트웨이 재시도 횟수, 자원·선택 실패 코드, Node 런타임 요구,
`--dry-run` 같은 항목이 서로 다릅니다. Go 빌드의 **현행** 동작·제약·미지원은
[docs/v2/COMPATIBILITY.md](docs/v2/COMPATIBILITY.md) 하나가 소유합니다.

## 설치

Windows 전용입니다. **Claude Code와 Codex CLI가 먼저 설치돼 있어야 합니다** — 이 빌드는
`claude.exe`를 띄우고, 내보내는 모든 요청이 설치된 Codex CLI의 버전으로 자신을 밝힙니다. 둘 중
하나라도 없으면 설치 스크립트가 `CLAUDE_NOT_FOUND`·`CODEX_NOT_FOUND`로 **내려받기 전에** 멈춥니다.
실사용에는 기존 Codex 로그인(`~/.codex/auth.json`)도 필요합니다. Node도 .NET도 필요하지 않습니다.

설치 스크립트는 릴리스 자산입니다. 받아서 **읽어보고** 실행합니다 — 어느 셸에서 시작하든 실행은
`powershell`이 합니다. 아래 셋 중 **당신이 지금 있는 셸의 블록 하나만** 통째로 붙여넣습니다.

```powershell
# PowerShell
irm https://github.com/wotjr1649/Clauduct/releases/latest/download/install.ps1 -OutFile clauduct-install.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File .\clauduct-install.ps1
```

```bat
rem cmd
curl -fsSL -o clauduct-install.ps1 https://github.com/wotjr1649/Clauduct/releases/latest/download/install.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File clauduct-install.ps1
```

```bash
# Git Bash
curl -fsSL -o clauduct-install.ps1 https://github.com/wotjr1649/Clauduct/releases/latest/download/install.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File clauduct-install.ps1
```

**세 가지가 다른 이유는 셸이 다르기 때문이고, 셋 다 실제로 겪은 것입니다.**

| 차이 | 왜 |
|---|---|
| PowerShell만 `irm` | PowerShell 5.1에서 `curl`은 `Invoke-WebRequest`의 별칭이라 `-fsSL -o`를 받지 못합니다. Git Bash에는 반대로 `irm`이 없습니다 |
| PowerShell만 `.\` | Git Bash는 `.\`의 백슬래시를 이스케이프로 먹어 `.install.ps1`을 넘깁니다. cmd는 둘 다 되지만 맞춰서 뺐습니다 |
| 받는 이름이 `clauduct-install.ps1` | 이 저장소 루트에 **v1(Node) 설치기인 `install.ps1`이 있습니다.** 클론 안에서 그 이름으로 받으면 기준선 파일을 덮어씁니다 |

cmd와 Git Bash가 같은 것은 `curl.exe`가 Windows 10 1803부터 기본 탑재이기 때문입니다.

최신 릴리스에서 바이너리 3개와 `SHA256SUMS`를 받아 **셋이 모두 대조된 뒤에** `~\.local\bin`에
넣고, 그 경로가 사용자 PATH에 없으면 추가합니다. 받은 바이트가 릴리스가 말하는 digest와 다르면
`INSTALL_DIGEST_MISMATCH`로 멈추고 **대상 폴더는 손대지 않은 상태로 남습니다.**

저장소를 클론했다면 `scripts/install.ps1`이 같은 파일이며, 옵션도 같습니다.

```powershell
.\scripts\install.ps1 -Tag v0.3.0        # 태그 고정
.\scripts\install.ps1 -FromPath .\dist   # 직접 빌드한 것으로 (3개 + SHA256SUMS 필요)
.\scripts\install.ps1 -NoPathUpdate      # PATH는 직접 관리
.\scripts\install.ps1 -SkipPreflight     # Claude Code·Codex CLI를 나중에 설치할 때
```

설치 뒤 **새 터미널**을 열고 확인합니다. Windows는 대소문자를 가리지 않으므로 `Clauduct`도 같은
명령입니다.

```powershell
clauduct-dev version   # Clauduct 자신: 0.3.0, commit, Go 버전
clauduct --version     # Claude Code의 버전. 실행 경로 전체가 도는지를 봅니다
```

**둘이 다른 것을 확인합니다.** 이 런처는 자기가 소유한 세 옵션(`--update`·`--usage`·`--uninstall`)
말고는 **전부 클라이언트에 그대로 넘깁니다.** `--version`도 그중 하나라 Claude Code가 답합니다 —
버그가 아니라 설계이고, 덕분에 그 명령은 "런처가 클라이언트를 띄울 수 있다"까지 증명합니다.
Clauduct 자신에 대한 질문은 `clauduct-dev`가 받습니다.

### 업데이트

```powershell
clauduct --update        # 태그와 digest 3개를 보여주고 확인을 받습니다
clauduct --update --yes  # 무인
```

`clauduct update`(하이픈 없음)는 **다른 명령**입니다 — 그대로 전달되어 Claude Code가 갱신됩니다.

### 제거

```powershell
clauduct --uninstall        # 무엇을 지울지 보여주고 확인을 받습니다
clauduct --uninstall --yes  # 무인
```

**저장소가 없어도 되고 셸도 가리지 않습니다.** PATH의 실행 파일이라 bash·cmd·PowerShell 어디서든
같은 한 줄입니다. 릴리스로 설치한 머신에는 바이너리만 있고 그게 바로 지우고 싶은 그 머신이므로,
제거는 바이너리가 합니다. `--update`와 같은 규칙이라 **첫 인자일 때만** 인식하고,
`clauduct -p "how do I --uninstall"`은 아무것도 지우지 않습니다.

실행 중인 자기 자신은 지울 수 없으므로 `.old`로 옮기고 그 한 줄을 출력합니다. **PATH와 진단
파일은 건드리지 않고** 무엇이 남았는지만 알립니다 — 기본 설치 폴더에는 `claude.exe`도 살기
때문입니다. 그것까지 지우려면 저장소의 스크립트를 씁니다.

```powershell
scripts\uninstall.ps1 -Purge       # 진단 파일(%TEMP%\clauduct)까지
scripts\uninstall.ps1 -RemovePath  # PATH 항목까지 — 그 폴더에 다른 실행 파일이 없을 때만
```

`clauduct-node`와 그 버전 저장소, `~/.claude` 아래 세션 상태는 어느 쪽도 건드리지 않습니다. 설치와
제거가 무엇을 검사하고 **무엇을 검사하지 않는지**는
[docs/v2/PACKAGING.md](docs/v2/PACKAGING.md) 5장에 있습니다.

## Go 빌드 (`clauduct`)

```powershell
clauduct
clauduct --model sol --effort xhigh
clauduct --continue
clauduct -p "Summarize the current task"
clauduct --update      # Clauduct 자신을 갱신. `clauduct update`는 그대로 전달되어 Claude Code를 갱신
clauduct --usage       # 이 계정이 주간 한도를 얼마나 썼는지. 요청 0회
clauduct-dev doctor    # 이 빌드 자신에 대한 질문은 별도 바이너리로
```

거의 모든 인자는 그대로 native로 전달됩니다. 이 런처가 소유하는 옵션은 `--update`와 `--usage`
둘이고(둘 다 **첫 인자일 때만** 인식), 거부하는 것은 네 개입니다(권한 해제 2종,
`--settings`·`--setting-sources`).

클라이언트의 `/usage`·`/cost`는 GPT 플랜 사용량을 보여주지 못합니다 — 커스텀 base URL에는 계정
엔드포인트를 묻지 않는 것으로 실측됐습니다. `clauduct --usage`가 그 질문에 답합니다.

필요한 것과 설치는 [설치](#설치) 절에 있습니다. 출하물·빌드·되돌리기는
[docs/v2/PACKAGING.md](docs/v2/PACKAGING.md), 모듈의 빌드 명령과 runtime 계약은
[go/README.md](go/README.md)에 있습니다.

## Node 구현 (`clauduct-node`)

Windows 설치는 [설치 안내](docs/v1/installation.md)를 따릅니다. Node.js 24 이상, Claude Code,
Codex CLI와 기존 Codex 로그인이 필요합니다. 동작·검증 구분과 자원 제한은
[native 구현 안내](docs/v1/README.md), 설치·복구·배포 무결성은 [릴리즈 안내](RELEASE.md)에 있습니다.

기준선으로서의 역할이 남아 있어 저장소에 그대로 둡니다. 정리 여부와 그 조건은
[docs/v2/MIGRATION.md](docs/v2/MIGRATION.md) 6장이 기록합니다.

## 무엇이 검증됐는가

증거는 [docs/v2/VALIDATION.md](docs/v2/VALIDATION.md)가 소유합니다. 요구 ID, 게이트, 실행한 검사와
**실행하지 않은 검사**를 함께 적습니다 — 미실행은 통과가 아닙니다.

Claude 공식 문서는 gateway를 통한 non-Claude 모델 라우팅을 공식 지원하지 않는다고 명시합니다.
이것은 제3자 호환 구현이며 "공식 지원"이나 "전체 기능 100% 보장"으로 설명하지 않습니다.

## 라이선스

[Apache License 2.0](LICENSE)입니다. 특허를 부여하고(§3), 상표는 부여하지 않으며(§6), 개작본은
바꿨다는 사실을 밝혀야 합니다(§4). 귀속과 상표 고지는 [NOTICE](NOTICE)에 있습니다.

이것이 다루는 것은 **이 소스에 대한 저작권뿐**입니다. Clauduct를 실행하는 것이 그것이 닿는
서비스들의 약관에 맞는지는 별개의 질문이고, 그 답은 저 서비스들이 가지고 있습니다.
