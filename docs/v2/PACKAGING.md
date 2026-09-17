# V2 패키징 — 무엇이 나가고, 무엇이 필요하고, 어떻게 되돌리는가

이 문서는 **출하되는 것**을 기술한다. 설계는 [ARCHITECTURE.md](ARCHITECTURE.md), 증거는 [VALIDATION.md](VALIDATION.md)가 소유한다.

**G9 완료 2026-09-17: 기본이 Go다.** 설치된 `clauduct`는 이 빌드이고, Node 구현은 `clauduct-node`로 남는다. 그 전까지 이름을 나눠 둔 이유는 이름을 공유하면 PATH 순서가 어느 구현이 도는지 결정하기 때문이었다. 이제 그건 결정 사항이고, 이름이 그 결정의 답이다.

## 1. 나가는 것

| 파일 | 하는 일 |
|---|---|
| `clauduct.exe` | 제품. 설치된 `claude.exe`를 띄우고 모델 요청을 loopback gateway로 돌린다 |
| `clauduct-hook.exe` | 클라이언트가 subagent 시작·종료에 실행한다. 게이트웨이에 역할을 보고한다 |
| `clauduct-dev.exe` | 이 프로젝트 자신의 명령. `version` · `doctor` · `probe` |

세 개뿐이다. 설정 파일도, 스크립트도, 데이터 디렉터리도 없다.

**`clauduct-hook`은 `clauduct` 옆에 있어야 한다.** `findHook()`이 실행 파일 옆만 본다 — PATH를 뒤지면 이 빌드가 내보내지 않은 동명 프로그램을 찾을 수 있고, 클라이언트는 그 결과를 실행하라는 말을 듣게 된다. 옆에 없으면 hook이 설치되지 않고, **역할별 라우팅과 위임 메뉴의 effort가 조용히 동작하지 않는다.**

**`clauduct-dev`가 따로 있는 이유**는 `clauduct`가 아무 옵션도 소유하지 않기 때문이다. 모든 인자가 native로 그대로 간다 — `--version`과 `--help`를 포함해서. 그래서 이 빌드 자신에 대한 질문은 **다른 바이너리**로 물어야 하고, 그러면 native 옵션이나 그 값과 충돌할 수 없다.

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

검증용 실호출은 별개의 예산이다 — `clauduct-dev probe`, 경로 고정, 누적 상한. VALIDATION.md 1.6.1절.

## 4. 빌드와 신원

```powershell
cd go
$env:CGO_ENABLED = '0'
go build -trimpath -o clauduct.exe      ./cmd/clauduct
go build -trimpath -o clauduct-hook.exe ./cmd/clauduct-hook
go build -trimpath -o clauduct-dev.exe  ./cmd/clauduct-dev
```

`-trimpath`는 빌드 머신의 디렉터리 배치가 바이너리에 남지 않게 한다. `CGO_ENABLED=0`은
**빌드 머신에 C 툴체인이 있는지에 산출물이 의존하지 않게** 한다 — 이 모듈은 `import "C"`가
없어서 Windows에서 동작은 같지만, cgo가 켜진 채로 빌드하면 바이트가 달라진다. 재현성 테스트는
같은 환경에서 두 번 빌드해 비교하므로 이 차이를 볼 수 없고, 그래서 산출물 쪽에서 고정한다
(`TestTheShippedBinaryIsBuiltWithoutCgo`). 예외는 race job 하나이고, 그 바이너리는 출하 대상이
아니다. commit stamp는 Go toolchain의 VCS 기록에서 나오므로 **릴리스 스크립트가 잊을 수 없다.**

```powershell
clauduct-dev version
# clauduct     0.0.0-wp01
# commit       <40자 hash>
# go           go1.27.1 windows/amd64
```

worktree가 수정된 상태로 빌드하면 commit 뒤에 `+dirty`가 붙는다. **그런 빌드는 릴리스 후보가 아니다.**

### 재현성

같은 소스·같은 Go 버전에서 **두 번 빌드하면 바이트가 같다.** 테스트가 두 번 빌드해 SHA256을 비교한다.

이것이 checksum을 의미 있게 만든다. "이 commit에서 빌드했다"는 바이너리가 자기에 대해 하는 주장이지만, 해시가 같다는 것은 **누구나 확인할 수 있는 주장**이다.

## 5. 설치

PATH에 있는 디렉터리에 **세 파일**을 복사한다. `clauduct-hook`이 `clauduct` 옆에 없으면 역할
라우팅과 메뉴의 effort가 조용히 동작하지 않는다(1장).

**그 디렉터리에 `clauduct`라는 이름의 폴더가 있으면 안 된다.** MSYS/Git Bash의 PATH 탐색은 같은
이름의 디렉터리에서 멈추고 `clauduct.exe`에 도달하지 못한다 — cmd는 PATHEXT로 찾으므로 셸에 따라
동작이 갈린다. v1 설치 프로그램이 `$bin/clauduct`를 버전 저장소로 쓰므로 실제로 일어났고,
2026-09-17에 그 저장소를 `clauduct-node-store`로 옮겨 해결했다(PARITY 11장 G9).

**설치 디렉터리에 쓰지 않는다.** 테스트가 확인한다 — 다른 cwd에서 실행한 뒤 설치 디렉터리에 무엇이 생겼는지 전후 비교한다. 따라서 공유 경로나 쓰기 금지 경로에 둘 수 있다.

세션 상태는 전부 native가 소유하고 `CLAUDE_CONFIG_DIR`(기본 `~/.claude`) 아래에 있다. 이 wrapper는 자기 것을 어디에도 쓰지 않는다.

## 5.1 업데이트

```powershell
clauduct --update        # 무엇이 바뀌는지 보여주고 확인을 받는다
clauduct --update --yes  # 무인
```

기준은 **최신 태그 릴리스**다. 순서가 안전의 전부다: 릴리스 메타데이터 → `SHA256SUMS` →
**사용자에게 태그와 digest 3개를 보여주고 확인** → 내려받아 전부 검증 → 그 다음에야 교체.
digest가 하나라도 어긋나면 디스크는 손도 대지 않았다고 말하고 끝난다. 릴리스 API가 자기 digest를
싣고 있으면 그것과도 대조한다 — 둘이 일치하는 것은 값이 적지만, **둘이 다르면 멈출 이유**다.

교체는 셋을 함께 한다(`clauduct`·`clauduct-hook`·`clauduct-dev`). 실행 중인 exe는 덮어쓸 수
없지만 이름은 바꿀 수 있으므로 `*.old`로 옮기고 새 파일을 쓴다. 중간에 실패하면 옮긴 것을 전부
되돌린다. 성공 후 `clauduct.exe.old`는 **이 프로세스가 끝나야** 지울 수 있고, 그래서 지우라고
출력한다.

`clauduct update`(하이픈 없음)는 이 명령이 아니다 — 그대로 클라이언트에 전달되어 **Claude Code**가
갱신된다. 클라이언트에는 `--update` 옵션이 없으므로(2.1.274 실측) 이 이름은 아무것도 가리지 않는다.

**아직 없는 것**: 이 릴리스. 현재 공개된 `v0.1.0`은 Node 구현의 zip을 싣고 있고 Go 바이너리 3개와
그 `SHA256SUMS`는 없다. 그래서 `--update`는 지금 실행하면 "릴리스에 무엇이 빠졌는지"를 이름으로
말하고 종료한다. 인증 없는 GitHub API는 주소당 시간당 60회이므로 한도에 걸리면 그것도 이름으로
말한다(`RATE_LIMITED`, 재시도 시각 포함).

## 5.2 이 계정이 얼마나 썼는지

```powershell
clauduct --usage       # 또는 clauduct-dev usage — 같은 뷰다
```

주간/보조 창의 사용률, 남은 시간, 어느 family가 in force인지를 보여준다. **요청을 만들지 않는다** —
세션이 남긴 계정 파일에서 읽고, 그 값이 얼마나 오래된 것인지를 함께 적는다. 현재 값으로 제시된
낡은 숫자는 숫자가 없는 것보다 나쁘다.

같은 값의 한 조각이 **모든 세션 종료 줄**에도 붙는다(`quota=47%/7d`). 묻지 않아도 보이라는 뜻이고,
그 이상은 위 명령이 맡는다.

클라이언트의 `/usage`·`/cost`는 이 숫자를 보여주지 못한다 — 커스텀 base URL에는 계정 엔드포인트를
묻지 않는 것으로 실측됐다. 5.1절의 `--update`와 마찬가지로 이 옵션도 **첫 인자일 때만** 인식한다.

## 6. 되돌리기

**두 파일 이름을 바꾸면 끝난다.** Node 구현은 지워지지 않았고 `clauduct-node`로 그대로 있다.

| 상황 | 하는 일 |
|---|---|
| Node로 되돌린다 | `clauduct.exe`를 치우고 `clauduct-node.cmd`를 `clauduct.cmd`로 되돌린다. Go 바이너리는 지울 필요도 없다 |
| 둘 다 쓴다 | 그대로 둔다. 이름이 다르므로 서로를 가리지 않고 동시에 실행된다 — 측정으로 확인했다(VALIDATION.md) |
| Go 빌드를 완전히 뺀다 | 세 바이너리를 지우고 위의 되돌리기를 한다 |

**저장소 안에서는 `clauduct`가 여전히 Node를 가리킨다.** cmd는 PATH보다 **현재 디렉터리를 먼저**
보고, 저장소 루트에는 v1 개발용 `clauduct.cmd`가 있다. 그 파일의 해시와 경로가 동결된 migration
manifest(738 entries)와 기준선 문서 여러 곳에 박혀 있어 지금 개명하지 않는다 — 이득보다 파급이
크다. 영향은 **cmd/PowerShell에서 저장소를 cwd로 둔 경우뿐**이고(bash는 cwd를 탐색하지 않는다),
정리 시점은 Node 파일을 실제로 옮기는 G10/M3다.

Windows `PATHEXT`는 `.EXE`를 `.CMD`보다 먼저 본다. 같은 디렉터리에 `clauduct.exe`와 `clauduct.cmd`가 동시에 있으면 `.exe`가 이긴다 — 그래서 되돌릴 때는 **`clauduct.exe`를 치우는 것이 필수**이고, `.cmd`를 되살리는 것만으로는 부족하다.

기준선 소스는 이 작업 내내 tracked 변경 0으로 유지됐다. 비교 기준이 바뀌면 비교가 아니다.

## 7. 아직 없는 것

| 항목 | 상태 |
|---|---|
| installer | 없다. 파일 복사가 설치다 |
| 자동 업데이트 | 없다 |
| Windows 외 대상 | `internal/platform`에 windows 태그 파일 하나뿐이다. 다른 대상은 이식이 아니라 **새 설계**다 |
| 서명 | 없다. 코드 서명 인증서는 이 프로젝트가 가진 것이 아니다 |
| CI 실행 결과 | **있다.** `redesign/go-v2-native-host`에서 gofmt·vet·build·test·race 전부 green (최근 run `35161187084`). 2026-09-17에 `CGO_ENABLED=0` 핀을 추가했으므로 다음 run이 출하 구성과 같은 것을 검사한다 |

**미실행은 통과가 아니다.** 각 항목은 없다고 적혀 있지 괜찮다고 적혀 있지 않다.
