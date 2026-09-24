# Clauduct Go V2 — 개발 안내

이 디렉터리가 V2 제품 구현의 **유일한 위치**다. 설계·판정·검증 계획은 `docs/v2/`가 소유한다. 여기에는 이 모듈을 어떻게 빌드하고 무엇을 지켜야 하는지만 적는다.

현재 지원 기능·버전별 근거·제약은 [호환성 문서](../docs/v2/COMPATIBILITY.md)가 관리한다.
아래 WP01–WP06 목록은 초기 구현 기록이며 최신 지원 여부를 판정하는 목록이 아니다.

## 초기 구현 기록 — WP01–WP06, G7 연결됨, G8 패키징

동작하는 것:

- 설치된 `claude.exe` 해석 (standalone 우선, PATH fallback)
- `127.0.0.1:0` 임시 listener + 세션 token 생성
- native 실행: argv 그대로 전달, stdio 상속, cwd 보존, exit code 반환
- `HEAD /api/hello` readiness — 무인증 204, 잘못된 인증은 401
- 요청 경계: 정확한 Host, loopback 원격 주소, 중복 header 거부, proxy/browser header 거부, `cookie`·`proxy-authorization` 거부
- 인증: `Authorization: Bearer` 상수시간 비교. `x-api-key`는 같은 세션 token일 때만 허용
- `POST /v1/messages`: method·content-type·32 MiB 본문 상한 검사, 요청 등록, 취소·300s 상한
- 옵션 거부: `--dangerously-skip-permissions` 계열 2개만. 나머지 native 옵션은 전부 전달
- SSE parser: frame 경계·UTF-8·중복 key·trailing JSON·terminal 순서·`[DONE]`·크기와 개수 상한
- 요청 registry: 요청별 취소, 형제 비전파, 멱등 해제, 동시 실행 상한 64
- 종료: 새 요청 거부 → in-flight 취소 → drain → listener 해제
- 요청 해독: top-level 허용목록, absent/null/present 구분, 미지원 기능의 명시적 거부
- text 경로 end-to-end: 요청 → backend 요청 → SSE 파싱 → Anthropic 프레임 스트리밍
- 도구 왕복: 정의·`tool_choice`·기록된 호출과 결과, 그리고 backend 호출의 검증과 방출
- delivery barrier: 호출은 `response.completed`의 output에서만 만들어진다 — 중간에 실패한 스트림은 실행할 것을 건넨 적이 없다
- 읽기 전용 credential provider: `auth.json` 해독, 세션 내 계정 고정, 만료 여백 60초, 어디서 출력해도 token은 가려진다
- 실제 HTTPS 전송: 인증서 검증 불가역, redirect 거부, 압축 비요청, phase별 timeout
- attempt 원장: 경로(model+effort)와 누적 횟수를 함께 승인한다. 예약이 credential 읽기보다도 먼저다
- 실패 분류: 상태 코드와 연결 오류를 terminal/retryable/deferred로 나눈다. `Retry-After`는 두 형식 모두

- 모델 라우팅: Claude alias·버전 id → Codex 모델. 모르는 모델·effort는 **거부하며 기본값으로 대체하지 않는다**

**당시 미구현 항목(현재 지원 여부는 호환성 문서 참조):**

- 이미지·문서·hosted search·structured output 결과 검증·`/v1/models` discovery

설치·런타임 요구사항·되돌리기는 [docs/v2/PACKAGING.md](../docs/v2/PACKAGING.md)가 소유한다.

## ⚠ 이 바이너리는 실제로 과금된다

**2026-09-15 G7**: `clauduct`가 시작한 모든 추론 요청은 사용자의 Codex 구독에 도달한다. 제품 세션에는 요청 수 상한이 없다 — Node 기준선에도 없고, 상한을 두면 긴 세션이 중간에 멈춘다.

모델을 호출하지 않는 명령은 여전히 아무것도 쓰지 않는다. `--version`·`--help`는 credential을 읽지 않고 `codex --version`도 띄우지 않는다. 둘 다 첫 **요청**에서만 일어난다.

### 검증용 실호출은 별개의 예산이다

```powershell
clauduct-dev probe          # 무엇을 쓸지 출력하고 아무것도 보내지 않는다 (exit 2)
clauduct-dev probe <name> --send
```

probe의 예산(`upstream.ApprovedBudget`)은 **이 프로젝트가 검증에 쓸 수 있는 양**이지 사용자 세션의 상한이 아니다. 경로가 `gpt-6-luna`/`low`로 고정돼 있고 누적 100회다. 제품 세션은 `upstream.Unlimited()`로 돌며 클라이언트가 요청한 모델을 쓴다. `probe accept <model> [effort...]`(표에 넣기 전 모델 수용 점검)는 이 예산 대신 Codex 캐시의 effort마다 경로별 상한을 두고, 보내기 전에 그 상한을 출력한다.

이 계약의 규칙은 추측이 아니라 설치된 claude 2.1.272에 일회용 listener를 붙여 **측정**한 것이다. 관측값은 [`docs/v2/VALIDATION.md`](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/VALIDATION.md) 1.1.2에 있다.

## 명령

```powershell
cd go
$env:CGO_ENABLED = '0'

gofmt -l .               # 출력이 비어야 한다
go vet ./...
go build ./...

go run ./cmd/clauduct-dev version
go run ./cmd/clauduct-dev doctor     # 인증·소켓 없이 환경과 Codex 모델 캐시(~/.codex/models_cache.json)의 표 차이를 본다. 자식은 codex --version 하나
```

v0.3.3부터 테스트·race·evidence 검사와 그 입력은 공개 트리에 없다. 유지보수자가 로컬에서 돌리며,
공개 CI는 위 세 검사와 `internal/httpguard`의 Linux·macOS build만 본다.

빌드 산출물은 저장소 밖이나 이미 ignore되는 `.tmp/` 아래에 둔다. `go run`은 VCS 정보를 stamp하지 않으므로 `version`이 `commit unknown`을 말한다. 실제 commit을 확인하려면 빌드한다.

```powershell
go build -trimpath -o $env:TEMP\clauduct-dev.exe ./cmd/clauduct-dev
```

출하 빌드는 `CGO_ENABLED=0`이다.

## 지켜야 할 계약

- **CLI 계약은 [ARCHITECTURE.md 4절](../docs/v2/ARCHITECTURE.md#4-cli-계약)이 소유한다.** `app.Run`이 거부 검사와 설정 병합을 적용하고, `launch.Build`는 세션 overlay 뒤에 argv를 그대로 복사한다. 값 경계 변경은 settings 추출·역할 검색·실제 native 대조 검사로 확인한다.
- **거부는 아무것도 얻기 전에 일어난다.** 실행 파일 조회도, bind도 하지 않는다. `internal/app`에 그 순서를 지키는 테스트가 있다.
- **Node·.NET·PowerShell에 runtime 의존하지 않는다.** `internal/app`의 소스 스캔 테스트가 문자열 리터럴 수준에서 이를 강제한다. Node 기준선이 `<node.exe> <repo>/src/review-diff.mjs` 형태의 명령을 native에 넘기던 패턴이 다시 들어오면 그 자리에서 실패한다.
- **검토·고정한 의존성만 허용.** 4개 모듈의 버전을 테스트로 고정한다 — 정확 계수를 위한 3개와, 역할 정의 frontmatter를 읽는 `go.yaml.in/yaml/v3`. [추가 결정과 검토](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/policy-evidence-20260918/DEPENDENCIES.md), [라이선스](THIRD_PARTY_NOTICES.txt). 새로운 의존성은 `docs/v2/ARCHITECTURE.md` 14장의 기준에 따라 별도 검토·기록한다.
- **child env는 `ANTHROPIC_*`와 `CLAUDE_CODE_OAUTH_TOKEN`만 제거한다.** 나머지는 전부 상속된다. 사용자 결정이며 근거는 [`docs/v2/DECISION.md`](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/DECISION.md). Clauduct는 추가 secret 장벽이 아니다.
- **세션 token은 로그·커맨드라인·오류 문자열에 넣지 않는다.**
- **tool call은 `response.completed` 이전에 만들어지지 않는다.** 스트리밍 이벤트에서 호출을 조립하는 arm을 추가하면 barrier가 사라진다. `internal/protocol/bridge`에 그것을 잡는 테스트가 있다.
- **상태 코드 계열은 재시도 지시다.** 측정상 5xx는 종류를 가리지 않고 재시도된다. 영구적인 로컬 조건은 4xx로 답한다.
- **현재 제품은 Windows 전용이다.** `internal/platform`의 실행 파일 해석 등은 Windows 구현만 있다. 공통 `internal/httpguard`는 표준 Go만 사용하며 macOS·Linux 지원 때 재사용한다. 공통 패키지의 검사 통과가 다른 OS의 제품 실행·설치 지원을 의미하지는 않는다.

## 패키지

| 경로 | 책임 |
|---|---|
| `cmd/clauduct` | 제품 launcher. 인자를 해석하지 않는다 |
| `cmd/clauduct-dev` | Clauduct 자신의 명령. native 옵션과 절대 충돌하지 않도록 별도 바이너리다 |
| `internal/app` | 순서와 생명주기. 자체 업무 규칙은 없다 |
| `internal/launch` | argv/env/cwd 사양 계산. spawn하지 않는다 |
| `internal/gateway` | loopback listener, 요청 경계·인증·registry |
| `internal/httpguard` | OS와 무관한 HTTP 응답 framing·제한된 연결 drain·취소 시 읽기 중단 |
| `internal/stream` | backend SSE 파싱과 전달 상태. 의미 해석은 하지 않는다 |
| `internal/wire` | 두 wire 형식이 공유하는 JSON 엄격성. 중복 key 거부가 한 곳에만 있다 |
| `internal/protocol/anthropic` | Claude 쪽 요청 해독과 이벤트 방출 |
| `internal/protocol/codex` | backend 쪽 이벤트 어휘 |
| `internal/protocol/bridge` | 두 형식 사이 변환과 모델 라우팅. 양쪽을 import하는 유일한 package |
| `internal/upstream` | backend 실행 인터페이스, fixture, 그리고 실제 HTTPS 전송. 이 모듈에서 네트워크에 닿는 **유일한** 곳이다 |
| `internal/auth` | 읽기 전용 credential provider. 아무것도 쓰지 않고 갱신하지 않는다 |
| `internal/platform` | OS 경계. 실행 파일 해석 |
| `internal/buildinfo` | 바이너리 신원. commit을 지어내지 않고, 수정된 worktree는 반드시 그렇게 말한다 |

`app`, `gateway`, `protocol`, `upstream` 등 나머지 책임 분해도는 `docs/v2/ARCHITECTURE.md` 3장에 있다. 빈 package를 미리 만들지 않는다.
