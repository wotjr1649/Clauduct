# v0.3.1 네 번째 묶음 — argv 값 경계와 CLI 계약

2026-09-21. `D:\AIDEV\clauduct-v031`, `fix/v031`에서
[#54](https://github.com/wotjr1649/Clauduct/issues/54)와
[#55](https://github.com/wotjr1649/Clauduct/issues/55)를 수정했다.
기준 commit은 `149068edd693fb860a03244a2ea15764bcd68c34`이며 앞선 세 묶음의
미커밋 변경을 보존했다. 로컬 개발 검사이며 출시·수동 TUI 수용 판정은 아니다.

## 변경

[takeUserSettings](../../go/internal/app/user_settings.go)와
[roleCLI](../../go/internal/app/roles.go)가
[같은 값 경계 탐색](../../go/internal/app/native_args.go)을 사용한다.
공개 옵션의 값 형태는 설치된 Claude Code 2.1.278의 `--help`로 확인했다.
기존 hidden 옵션은 기존 역할 탐색 목록을 유지한다.

- 필수 값은 `--settings`나 `--`처럼 보여도 소비한다. `--option=value`도 구분한다.
- 독립된 `--`에서 설정·역할 탐색을 멈춘다. 프롬프트 뒤의 실제 옵션은 계속 탐색한다.
- `--settings --public.json`처럼 옵션 모양인 파일 이름도 일반 파일·크기·JSON 검증을 적용한다.
- 모르는 옵션 이후 설정 후보가 있으면 `SETTINGS_INVALID`로 거부한다. 모르는 옵션이
  다음 `--`나 알려진 옵션 이름을 값으로 소비할 수도 있으므로 이후 경계를 신뢰하지 않는다.
  설정 후보가 없으면 native에 그대로 전달한다.
- 반복된 설정의 마지막 소스를 읽고 필수 설정과 병합한다. [Run](../../go/internal/app/run.go)은
  각 실제 설정 옵션의 자리를 `--settings=<병합 결과>`로 교체한다. 나머지 인자의 순서와
  철자는 유지한다. 설정 옵션을 제거하지 않으므로 앞의 optional/variadic 옵션이 뒤의
  프롬프트를 추가 값으로 가져가지 않는다.

설정 파일의 2 MiB 상한, 중복 JSON key 거부, 연결 설정 보호, 필수 hook 병합은 유지했다.
반복 옵션에는 동일한 병합 결과를 넣으므로 마지막 옵션이 필수 설정을 덮어쓰지 않는다.

`#55`는 [ARCHITECTURE.md 4절](../../docs/v2/ARCHITECTURE.md#4-cli-계약)을 계약 소유자로
명시하고 README와 소스 주석의 모순을 제거했다. `launch.Refused`와 `launch.Build`의
실행 코드는 바꾸지 않았다. 권한 우회 옵션 2개는 여전히 옵션 값이나 `--` 뒤에서도 거부한다.

## 실제 native 대조와 수정 전 실패

[native 대조 검사](../../go/internal/app/argv_native_test.go)는 설치된 `claude.exe`를 직접
실행한 결과와 `app.Run`을 거친 결과를 비교한다. 임시 사용자·프로젝트 경로,
`--strict-mcp-config`, `--bare`, 합성 설정, loopback gateway와 fixture 응답을 사용했다.
실제 요청의 user/developer 메시지에서 프롬프트와 system prompt 값을 확인했다.
실행 시 시스템 managed 설정 경로 두 곳은 존재하지 않았다.

| 입력의 핵심 형태 | 직접 native | 수정 후 Clauduct |
|---|---|---|
| `-p --settings` | 필수 값 누락 거부, backend 호출 0 | 같은 거부 결과, backend 호출 0 |
| `-p -- --settings` | 리터럴 프롬프트 전달 | 동일 |
| `--append-system-prompt --settings -p PUBLIC_ARGV_PROMPT` | system prompt 값 보존 | 동일 |
| `--append-system-prompt -- --settings={} -p PUBLIC_ARGV_PROMPT` | `--`는 필수 값, 실제 설정 인식 | 동일 |
| `--settings --public.json -p PUBLIC_ARGV_PROMPT` | 옵션 모양의 파일 이름 허용 | 동일 |
| `--debug --settings={} PUBLIC_ARGV_PROMPT -p` | optional 값 없이 프롬프트 전달 | 동일 |
| `--tools Read --settings={} PUBLIC_ARGV_PROMPT -p` | 도구 목록과 프롬프트 분리 | 동일 |

위 7개 입력 × 2개 경로, **14개 대조 항목 PASS**. 첫 입력은 이슈 예시와 달리 native도
거부했다. `-p`는 prompt 값을 소비하는 옵션이 아니라 boolean 플래그다.

수정 전에는 terminator 뒤 설정 토큰을 소비했고, 필수 값인 `--settings`와 옵션 모양 파일을
거부했다. 실제 native 대조에서는 `--debug`·`--tools` 뒤 설정을 제거하면 프롬프트가 사라져
native의 입력 누락 오류가 발생했다. 이 결과 때문에 단순한 탐색 수정에 그치지 않고
설정 옵션의 자리도 보존했다. 초기 probe의 system prompt assertion은 고정된 top-level
`instructions` 대신 변환된 developer 메시지를 검사하도록 정정했다.

## 검사와 반증

[단위·실제 자식 프로세스 검사](../../go/internal/app/user_settings_test.go)는 값/terminator,
attached 값, optional 값 유무, variadic 옵션, `-n`, 프롬프트 뒤 옵션, 모르는 경계,
반복 설정 병합, 필수 설정 보존을 검사한다. 기존 역할·거부 검사도 함께 실행했다.

[mutation 결과](mutations.json)의 **7/7을 assertion 실패로 검출**했다. 컴파일 실패를
검출로 세지 않았다. Go overlay로만 바꿔 실제 작업 파일과 앞선 묶음은 유지했다.

| 되돌린 동작 | 관측한 실패 |
|---|---|
| 설정 탐색의 `--` 종료 무시 | positional 설정 토큰 보존 실패 |
| 필수 값 경계 무시 | system prompt 값을 설정으로 오인 |
| optional 값이 다음 옵션까지 소비 | 실제 설정 누락 |
| 모르는 옵션 이후 경계 신뢰 | 불확실한 설정 후보 허용 |
| 설정 옵션 자리 제거 | 실제 native에서 optional/variadic 뒤 프롬프트 소실 |
| 첫 설정 자리만 병합 | 반복 설정 값 불일치 |
| 역할 탐색에서 값까지 재탐색 | 프롬프트 값을 역할 정책으로 오인 |

mutation runner 첫 실행은 PowerShell이 overlay 경로 변수를 리터럴로 넘겨 검사에 도달하지
못했다. 인자를 명시적으로 인용한 뒤 위 7개를 실행했으며 최초 실행은 근거에서 제외했다.

| 실행한 검사 | 결과 |
|---|---|
| `go test -count=1 -timeout=8m ./...` (`CGO_ENABLED=0`) | 전체 패키지 PASS. app 312.913초, gateway 25.988초 |
| 변경한 app 경로의 `go test -count=1 -timeout=120s -race` (`CGO_ENABLED=1`) | PASS, 15.281초. settings·native 대조·설정 소스·역할 CLI·리소스 획득 전 거부 검사 |
| `go test -count=1 -timeout=30s -race ./internal/launch` | PASS, 1.443초 |
| `go vet ./...`, `go build ./...`, `gofmt -l .` | PASS, format 출력 없음 |
| `node verification/test-doc-citations.mjs` | 문서 119개, 인용 89개, 로컬 링크 641개, 실패 0 |
| `git diff --check` | PASS |

Go 1.27.1, Windows에서 실행했다. race에는 기존 `C:\msys64\ucrt64\bin\gcc.exe`를
프로세스 PATH에 추가했다. 설치나 영구 환경 변경은 하지 않았다. native 실행을 포함한
Go 회귀·race·mutation 검사에서는 불필요한 상속 환경을 제외했다. Go 검사는
`GOPROXY=off`, `GOTOOLCHAIN=local`로 실행했다.
이번 race 범위는 변경한 app 경로와 launch이며 전체 모듈 race 재실행은 아니다.

## 범위와 남은 항목

알려진 native 옵션의 값 형태를 사용한다. 새 옵션이나 결합된 short 옵션을 모르면 뒤의
설정 후보를 보수적으로 거부할 수 있다. native 전체 parser 지원을 선언하지 않는다.
native 버전이 바뀌면 help와 위 대조 검사를 다시 확인해야 한다.

유료 backend, 수동 TUI, Node V1 전체 검사, PowerShell 제품 검증 묶음은 이번 변경에서
실행하지 않았다. push·PR·병합·태그·Release·설치본 변경도 하지 않았다.
다음 구현 묶음은 `#53`·`#58`이다. `#56`은 승인된 압축 요청만 medium 상한 정책을 적용할
후속 항목이며, `#50`의 추정 방식은 실제 1회 관측 전까지 그대로 유지한다.
