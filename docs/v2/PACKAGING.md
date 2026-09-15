# V2 패키징 — 무엇이 나가고, 무엇이 필요하고, 어떻게 되돌리는가

이 문서는 **출하되는 것**을 기술한다. 설계는 [ARCHITECTURE.md](ARCHITECTURE.md), 증거는 [VALIDATION.md](VALIDATION.md)가 소유한다.

G8 단계이며 **기본 전환(G9)은 아직 요청하지 않았다.** `clauduct-go`는 `clauduct`가 아니고, 이름을 공유하기 전까지는 PATH 순서가 어느 구현이 도는지 결정할 일이 없다.

## 1. 나가는 것

| 파일 | 하는 일 |
|---|---|
| `clauduct-go.exe` | 제품. 설치된 `claude.exe`를 띄우고 모델 요청을 loopback gateway로 돌린다 |
| `clauduct-dev.exe` | 이 프로젝트 자신의 명령. `version` · `doctor` · `probe` |

두 개뿐이다. 설정 파일도, 스크립트도, 데이터 디렉터리도 없다.

**이름이 둘인 이유**는 `clauduct-go`가 아무 옵션도 소유하지 않기 때문이다. 모든 인자가 native로 그대로 간다 — `--version`과 `--help`를 포함해서. 그래서 이 빌드 자신에 대한 질문은 **다른 바이너리**로 물어야 하고, 그러면 native 옵션이나 그 값과 충돌할 수 없다.

## 2. 실행에 필요한 것

`clauduct-go`는 단일 정적 Go 바이너리지만 **혼자 동작하지는 않는다.**

| 필요한 것 | 왜 | 없으면 |
|---|---|---|
| `claude.exe` | 띄울 대상이다. `~/.local/bin` 우선, 그다음 PATH | `CLAUDE_NOT_FOUND`, 찾아본 경로를 함께 출력 |
| `codex.exe` | **버전이 모든 요청의 header에 들어간다.** 없으면 보낼 것을 만들 수 없다 | 요청이 `CODEX_NOT_FOUND`로 거부된다. 세션은 시작되고 `--version`·`--help`는 정상 |
| `~/.codex/auth.json` | ChatGPT 구독 credential. 읽기 전용으로만 접근한다 | 요청이 `CREDENTIAL_UNAVAILABLE_OR_EXPIRED`(503). `codex login`으로 회복 |

**`codex.exe` 의존은 G7이 추가한 것이다.** 이 문서가 그것을 적는 이유는 REL03이 "Node adapter·.NET probe에 의존하지 않음"만 묻기 때문이다 — 그 둘은 실제로 없지만, **런타임 의존이 0이라는 뜻은 아니다.** 정직한 목록은 위의 셋이다.

Node도 .NET도 필요 없다. `internal/app`의 스캔이 제품 소스에 `.mjs` 경로·`node.exe`·`npm.cmd`·.NET probe 이름이 들어가지 못하게 막는다.

## 3. ⚠ 이 바이너리는 실제로 과금된다

**2026-09-15 G7부터** `clauduct-go`가 시작한 모든 추론 요청은 사용자의 Codex 구독에 도달한다. 제품 세션에는 **요청 수 상한이 없다** — Node 기준선에도 없고, 상한을 두면 긴 세션이 중간에 멈춘다.

모델을 호출하지 않는 명령은 아무것도 쓰지 않는다. `--version`·`--help`는 credential을 읽지 않고 `codex --version`도 띄우지 않는다. 둘 다 **첫 요청**에서만 일어난다. 테스트가 그것을 고정한다.

검증용 실호출은 별개의 예산이다 — `clauduct-dev probe`, 경로 고정, 누적 상한. VALIDATION.md 1.6.1절.

## 4. 빌드와 신원

```powershell
cd go
go build -trimpath -o clauduct-go.exe  ./cmd/clauduct-go
go build -trimpath -o clauduct-dev.exe ./cmd/clauduct-dev
```

`-trimpath`는 빌드 머신의 디렉터리 배치가 바이너리에 남지 않게 한다. commit stamp는 Go toolchain의 VCS 기록에서 나오므로 **릴리스 스크립트가 잊을 수 없다.**

```powershell
clauduct-dev version
# clauduct-go 0.0.0-wp01
# commit       <40자 hash>
# go           go1.27.0 windows/amd64
```

worktree가 수정된 상태로 빌드하면 commit 뒤에 `+dirty`가 붙는다. **그런 빌드는 릴리스 후보가 아니다.**

### 재현성

같은 소스·같은 Go 버전에서 **두 번 빌드하면 바이트가 같다.** 테스트가 두 번 빌드해 SHA256을 비교한다.

이것이 checksum을 의미 있게 만든다. "이 commit에서 빌드했다"는 바이너리가 자기에 대해 하는 주장이지만, 해시가 같다는 것은 **누구나 확인할 수 있는 주장**이다.

## 5. 설치

PATH에 있는 디렉터리에 두 파일을 복사한다. 그게 전부다.

**설치 디렉터리에 쓰지 않는다.** 테스트가 확인한다 — 다른 cwd에서 실행한 뒤 설치 디렉터리에 무엇이 생겼는지 전후 비교한다. 따라서 공유 경로나 쓰기 금지 경로에 둘 수 있다.

세션 상태는 전부 native가 소유하고 `CLAUDE_CONFIG_DIR`(기본 `~/.claude`) 아래에 있다. 이 wrapper는 자기 것을 어디에도 쓰지 않는다.

## 6. 되돌리기

**이름이 다르므로 되돌릴 것이 없다.** `clauduct`(Node)와 `clauduct-go`는 서로를 가리지 않고, 동시에 실행된다 — 측정으로 확인했다(VALIDATION.md).

| 상황 | 하는 일 |
|---|---|
| Go 빌드를 쓰고 싶지 않다 | 바이너리를 지운다. Node `clauduct`는 건드려지지 않았다 |
| 기본 전환 후 되돌리고 싶다 | **아직 그런 상태가 없다.** 기본 전환은 G9이고 요청하지 않았다 |

기준선 소스는 이 작업 내내 tracked 변경 0으로 유지됐다. 비교 기준이 바뀌면 비교가 아니다.

## 7. 아직 없는 것

| 항목 | 상태 |
|---|---|
| installer | 없다. 파일 복사가 설치다 |
| 자동 업데이트 | 없다 |
| Windows 외 대상 | `internal/platform`에 windows 태그 파일 하나뿐이다. 다른 대상은 이식이 아니라 **새 설계**다 |
| 서명 | 없다. 코드 서명 인증서는 이 프로젝트가 가진 것이 아니다 |
| CI 실행 결과 | `.github/workflows/go.yml`은 작성돼 있고 **한 번도 실행된 적이 없다.** push가 별도 승인 사항이다 (REL10) |

**미실행은 통과가 아니다.** 각 항목은 없다고 적혀 있지 괜찮다고 적혀 있지 않다.
