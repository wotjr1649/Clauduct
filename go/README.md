# Clauduct Go V2 — 개발 안내

이 디렉터리가 V2 제품 구현의 **유일한 위치**다. 설계·판정·검증 계획은 `docs/v2/`가 소유한다. 여기에는 이 모듈을 어떻게 빌드하고 무엇을 지켜야 하는지만 적는다.

## 현재 범위 — WP01 + WP02

동작하는 것:

- 설치된 `claude.exe` 해석 (standalone 우선, PATH fallback)
- `127.0.0.1:0` 임시 listener + 세션 token 생성
- native 실행: argv 그대로 전달, stdio 상속, cwd 보존, exit code 반환
- `HEAD /api/hello` readiness — 무인증 204, 잘못된 인증은 401
- 요청 경계: 정확한 Host, loopback 원격 주소, 중복 header 거부, proxy/browser header 거부, `cookie`·`proxy-authorization` 거부
- 인증: `Authorization: Bearer` 상수시간 비교. `x-api-key`는 같은 세션 token일 때만 허용
- `POST /v1/messages`: method·content-type·32 MiB 본문 상한 검사, 요청 등록, 취소·300s 상한
- 옵션 거부: `--dangerously-skip-permissions` 계열 2개만. 나머지 native 옵션은 전부 전달
- 요청 registry: 요청별 취소, 형제 비전파, 멱등 해제, 동시 실행 상한 64
- 종료: 새 요청 거부 → in-flight 취소 → drain → listener 해제

**동작하지 않는 것 (아직 구현이 없다):**

- `/v1/messages` 본문 해석 — 검사를 통과한 뒤 501 `NOT_IMPLEMENTED`
- `/v1/models` discovery, upstream 연결, 프로토콜 변환, SSE, 도구 왕복, 모델 라우팅

따라서 **대화형 세션은 아직 성립하지 않는다.** `--version`·`--help`처럼 모델을 호출하지 않는 native 명령은 정상 통과한다.

이 계약의 규칙은 추측이 아니라 설치된 claude 2.1.272에 일회용 listener를 붙여 **측정**한 것이다. 관측값은 `docs/v2/VALIDATION.md` 1.1.2에 있다.

## 명령

```powershell
cd go

go test ./...            # 전체
go test -count=1 ./...   # 캐시 무시
gofmt -l .               # 출력이 비어야 한다
go vet ./...

go run ./cmd/clauduct-dev version
go run ./cmd/clauduct-dev doctor     # 인증·소켓·자식 없이 환경만 본다
```

빌드 산출물은 저장소 밖이나 이미 ignore되는 `.tmp/` 아래에 둔다. `go run`은 VCS 정보를 stamp하지 않으므로 `version`이 `commit unknown`을 말한다. 실제 commit을 확인하려면 빌드한다.

```powershell
go build -trimpath -o $env:TEMP\clauduct-dev.exe ./cmd/clauduct-dev
```

`-race`는 cgo와 C 툴체인을 요구한다. 이 개발 머신에는 gcc가 없어 로컬에서는 실행하지 못했고 CI가 담당한다. 미실행은 통과가 아니다.

## 지켜야 할 계약

- **제품 launcher는 인자를 해석하지 않는다.** `launch.Build`는 argv를 그대로 복사한다. 유일한 예외는 `launch.Refused`의 옵션 2개(`--dangerously-skip-permissions` 계열)이며, 값을 먹지 않는 옵션이라 인자 단위 정확 일치만으로 충분하다 — 값 추적이 없으므로 값이 옵션으로 오인되는 경로가 생기지 않는다. 목록을 늘리려면 그 성질이 유지되는지 먼저 확인한다.
- **거부는 아무것도 얻기 전에 일어난다.** 실행 파일 조회도, bind도 하지 않는다. `internal/app`에 그 순서를 지키는 테스트가 있다.
- **Node·.NET·PowerShell에 runtime 의존하지 않는다.** `internal/app`의 소스 스캔 테스트가 문자열 리터럴 수준에서 이를 강제한다. Node 기준선이 `<node.exe> <repo>/src/review-diff.mjs` 형태의 명령을 native에 넘기던 패턴이 다시 들어오면 그 자리에서 실패한다.
- **제3자 의존성 0.** `go.sum`이 생기거나 `go.mod`에 `require`가 생기면 테스트가 실패한다. 의존성을 추가하려면 `docs/v2/ARCHITECTURE.md` 14장의 허용 기준을 통과시키고 그 결정을 기록한다.
- **child env는 `ANTHROPIC_*`와 `CLAUDE_CODE_OAUTH_TOKEN`만 제거한다.** 나머지는 전부 상속된다. 사용자 결정이며 근거는 `docs/v2/DECISION.md`. Clauduct는 추가 secret 장벽이 아니다.
- **세션 token은 로그·커맨드라인·오류 문자열에 넣지 않는다.**
- **Windows 전용이다.** `internal/platform`에 `_windows.go` 파일만 있어 다른 OS에서는 빌드되지 않는다. 의도된 것이다 — 다른 OS는 이관이 아니라 신규 설계(V2-04)다.

## 패키지

| 경로 | 책임 |
|---|---|
| `cmd/clauduct-go` | 제품 launcher. 인자를 해석하지 않는다 |
| `cmd/clauduct-dev` | Clauduct 자신의 명령. native 옵션과 절대 충돌하지 않도록 별도 바이너리다 |
| `internal/app` | 순서와 생명주기. 자체 업무 규칙은 없다 |
| `internal/launch` | argv/env/cwd 사양 계산. spawn하지 않는다 |
| `internal/gateway` | loopback listener, 요청 경계·인증·registry |
| `internal/platform` | OS 경계. 실행 파일 해석 |
| `internal/buildinfo` | 바이너리 신원 |

`app`, `gateway`, `protocol`, `upstream` 등 나머지 책임 분해도는 `docs/v2/ARCHITECTURE.md` 3장에 있다. 빈 package를 미리 만들지 않는다.
