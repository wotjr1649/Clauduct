# v0.3.1 다섯 번째 묶음 — 세션 등록 복구와 client 기능 진단

2026-09-21. `D:\AIDEV\clauduct-v031`, `fix/v031`에서
[#53](https://github.com/wotjr1649/Clauduct/issues/53)과
[#58](https://github.com/wotjr1649/Clauduct/issues/58)을 수정했다.
기준 commit은 `149068edd693fb860a03244a2ea15764bcd68c34`이며 앞선 네 묶음의 변경을
보존했다. 미출하 개발 변경이고 수동 TUI 수용 판정은 아니다.

## #53 — 실패한 시작 등록을 프롬프트 제출 때 복구

[세션 설정](../../go/internal/app/settings.go)에 같은 제품 hook의 `UserPromptSubmit`
등록을 추가했다. [hook](../../go/cmd/clauduct-hook/main.go)은 이 이벤트의 세션 식별자와
transcript 경로만 기존 `SessionStart` 등록 API로 보낸다. prompt·기타 입력 필드는 전달하지
않는다. 매 제출마다 한 번 실행하며 기존 3초 HTTP 상한과 5초 hook 상한을 유지한다.

시작 이벤트 실패 후에도 사용자 프롬프트 직전에 다시 등록하므로 일시적인 실패를 복구한다.
등록에 실패하면 exit 2와 고정된 `CLAUDUCT_CONTEXT_SESSION_UNVERIFIED` 안내로 프롬프트를
차단하고, 연결 복구 후 다시 제출하도록 알린다. 잘못된 transcript 입력도 진단을 남긴다.
[공식 hook 계약](https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event)은
`SessionStart` 오류가 시작을 막지 않고 `UserPromptSubmit`의 exit 2는 프롬프트를 막는다고
명시한다. 이 차이를 실제 설치된 Claude Code 2.1.278에서도 확인했다.

이슈가 제안한 nonpersistent 자동 전환은 적용하지 않았다. 재시작한 세션에 저장된
`failed`/`compacting` 상태가 있을 때 등록 부재를 빈 이력으로 처리하면 복원을 건너뛸 수
있다. 기존 [세션 경로 검증과 journal 복원](../../go/internal/gateway/context_journal.go),
등록 전 생성 거부를 유지하면서 재등록 경로를 제공했다. 같은 등록은 상태를 초기화하지
않으며 경로 이탈·동일 세션의 경로 변경도 여전히 거부한다.

## #58 — 버전 문자열과 필수 기능을 구분

[메시지 처리](../../go/internal/gateway/messages.go)에서 context policy가 요구하는
`X-Claude-Code-Request-Class`를 역할 선택·결과 복원·본문 처리보다 먼저 확인한다.
누락 시 기존 `CONTEXT_REQUEST_CLASS_UNVERIFIED` 분류를 유지하고, [오류 메시지](../../go/internal/gateway/errors.go)에
필수 헤더, Claude Code 업데이트 또는 로컬 연동 점검, 기준 client 버전을 명시한다.
다른 역할 선택 오류가 실제 원인을 가리지 않는다.

[client 진단](../../go/internal/gateway/diagnostics.go)에 `requestClassRequired`와
누적 누락 거부 횟수 `requestClassMissing`을 추가했다. `verified`는 기존대로 버전 문자열
일치만 뜻한다. 같은 버전이어도 헤더가 없으면 거부하며, 다른 버전 문자열이어도 필요한
헤더와 나머지 검증을 통과하면 허용한다. 새 버전을 문자열만으로 막는 제한은 추가하지 않았다.
정책이 비활성인 검사 gateway의 fallback을 구버전 제품 지원처럼 설명하던 주석도 정정했다.

## 관측과 반증

[실제 native 검사](../../go/internal/app/session_registration_test.go)는 제품 hook을 빌드해
실행하고 임시 profile/project, strict MCP, loopback gateway, 합성 backend를 사용한다.
중간의 로컬 relay는 시작 등록 POST만 503으로 한 번 거부하거나 모든 등록을 거부한다.
gateway는 그동안 계속 연결 가능하다. 과금 backend는 사용하지 않았다.

| 입력·실패 조건 | 수정 전 | 수정 후 |
|---|---|---|
| 최초 등록 POST 한 번 실패 | POST 1회, 재등록 없음. `CONTEXT_SESSION_UNVERIFIED` | 프롬프트 제출 시 재등록, 정상 응답 1회, persistent context 저장 |
| 모든 등록 POST 실패 | 재등록 없음 | 프롬프트 차단 안내, backend 호출 0 |
| `UserPromptSubmit`을 hook에 입력 | 무시, POST 0회·exit 0 | 세션 등록 1회. HTTP 204는 성공, 400/503은 exit 2와 재제출 안내 |
| 미등록 자식 + 요청 분류 헤더 누락 | `AGENT_SELECTION_UNVERIFIED`가 원인을 가림 | 필수 헤더 누락으로 먼저 거부, 선택·backend 작업 0 |

[#58 검사](../../go/internal/gateway/client_capability_test.go)는 2.1.272, 기준 버전,
미래 버전, 빈 버전의 **합성 User-Agent**를 사용했다. 옛 native 바이너리를 실행한 증거는
아니다. 기준 버전 일치와 헤더 누락을 동시에 표시하고, 다른 버전 문자열의 정상 요청,
무인증 readiness, 정책 비활성 진단도 대조했다.

[hook 검사](../../go/cmd/clauduct-hook/main_test.go)는 전달 필드를 제한하고 실패 안내·종료
코드를 확인한다. [journal 검사](../../go/internal/gateway/context_journal_test.go)는 실제
저장된 실패 상태가 재등록 후에도 유지되고 경로 변경이 거부되는지 확인한다.

[mutation 기록](mutations.json)의 **8/8을 assertion 실패로 검출**했다. Go overlay를 사용하여
작업 파일은 바꾸지 않았으며, 컴파일 오류를 검출로 세지 않았다.

| 되돌린 변경 | 실패한 근거 |
|---|---|
| `UserPromptSubmit` 설치 제거 | 실제 native의 등록 POST 1회, 세션 미등록 거부 |
| hook에서 제출 이벤트 무시 | POST 0회와 잘못된 성공 |
| 제출 이벤트를 기존 등록 API로 변환하지 않음 | 전달 이벤트 불일치 |
| 등록 실패를 exit 0으로 처리 | 실패를 차단하지 못함 |
| 요청 분류 확인을 처리 초기에 수행하지 않음 | 역할 선택 오류가 원인을 가림 |
| 진단의 누락 횟수를 0으로 고정 | 버전 일치가 누락을 숨김 |
| 헤더 누락 안내 제거 | 필수 기능·조치 안내 소실 |
| 세션 미등록 안내 제거 | 복구 방법 안내 소실 |

| 실행한 검사 | 결과 |
|---|---|
| `go test -count=1 -timeout=8m ./...`, `CGO_ENABLED=0` | 전체 패키지 PASS. app 369.557초, gateway 23.635초 |
| `go test -count=1 -timeout=90s -race ./internal/gateway ./cmd/clauduct-hook` | PASS. gateway 27.690초, hook 3.232초 |
| app의 native 등록 복구·저장된 이전 모델로 재개 후 압축 검사, `-race` | PASS, 15.364초 |
| `go vet ./...`, `go build ./...`, `gofmt -l .` | PASS, format 출력 없음 |
| `node verification/test-doc-citations.mjs` | 문서 120개, 인용 89개, 로컬 링크 653개, 실패 0 |
| `git diff --check` | PASS |

Windows / Go 1.27.1에서 실행했다. native 실행을 포함하는 Go 검사에서는 불필요한 상속
환경을 제외하고 `GOPROXY=off`, `GOTOOLCHAIN=local`을 사용했다. race는 `CGO_ENABLED=1`,
기존 `C:\msys64\ucrt64\bin\gcc.exe`를 프로세스 PATH에 추가하여 실행했다. 전체 모듈 race
재실행은 아니다. 실제 native 검사 당시 시스템 managed 설정 경로 두 곳은 존재하지 않았다.

## 한계와 다음 묶음

실제 장애 이력이 발견된 것은 아니다. 이번에는 연결 가능한 gateway 앞에서 등록 응답
실패를 주입하여 도달 가능한 경로와 복구를 재현했다. hook 미설치·비활성화·timeout 또는
지속적인 잘못된 경로/연결은 별도 원인 해결이 필요하다. 등록 근거가 없으면 gateway가
생성을 거부하며, 저장된 context 복원을 생략해 성공시키지 않는다.

구버전 바이너리, 수동 TUI, 과금 backend, Node V1·PowerShell 전체 검증은 이번에 실행하지
않았다. 호스트 설정·설치본·원격 저장소는 변경하지 않았다. 다음 묶음은 승인된 `#56`의
자동 압축 요청에만 medium effort 상한을 복원하는 작업이다. `#50`은 실제 1회 관측 전까지
현행 추정을 유지하며 `#52`는 기존 wontfix 분류를 유지한다.
