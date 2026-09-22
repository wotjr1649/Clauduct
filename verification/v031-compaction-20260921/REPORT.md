# v0.3.1 여섯 번째 묶음 — 자동 압축 effort 상한과 계수 일치

2026-09-21. `D:\AIDEV\clauduct-v031`, `fix/v031`에서
[#56](https://github.com/wotjr1649/Clauduct/issues/56)을 수정했다.
기준 commit은 `149068edd693fb860a03244a2ea15764bcd68c34`이며 앞선 다섯 묶음은 보존했다.
사용자가 선택한 정책은 자동 압축 요청만 medium 이하, 기존 모델 유지, 이후 원래 effort 복귀다.
이번 turn에서 실제 backend 검증도 명시적으로 요청했다. 미출하 개발 변경이다.

## 원인과 수정

압축 시작 시 앞선 라우트를 그대로 override하므로 high/xhigh/max가 압축에도 그대로 적용됐다.
세션의 원래 라우트를 바꾸면 이후 생성과 journal 복원까지 바뀔 수 있어, [압축 경로](../../go/internal/gateway/context_compaction.go)는
검증된 `PreCompact` 영수증의 `trigger=auto`일 때만 요청용 라우트 복사본에 상한을 적용한다.
high/xhigh/max는 medium, low/medium은 그대로다. 모델은 바뀌지 않는다.
변경된 요청의 선택 출처에는 `+auto-compact`가 붙는다.

수동 압축, trigger 미지정, 영수증 부재·만료·다른 세션/자식은 effort를 낮추지 않는다.
native의 인증된 `compaction` 요청 분류에 따른 기존 압축 허용과, 자동 압축 비용 정책을
적용할 근거를 구분한다. 템플릿 문구만으로 자동 압축으로 판단하지 않는다.
모르는 모델/effort는 override가 있어도 계속 거부한다.

추가로 압축 준비가 생성 경로에만 있어 `count_tokens`에는 압축 지침·앞선 모델 고정·영수증
제거가 빠져 있었다. 이 때문에 계수 입력과 실제 생성 입력이 달라지고 이전 계수 캐시도
연결되지 않았다. [계수 처리](../../go/internal/gateway/count_tokens.go)는 같은 영수증·라우트·지침
준비를 사용한다. 재시작 시 journal도 읽지만 상태 map 삽입, phase/busy 변경, 영수증 소비와
journal 쓰기는 하지 않는다. 실제 생성은 [기존 상태 검증 경로](../../go/internal/gateway/context.go)를
거쳐야 한다. 일반 생성에 원격 사전 계수를 추가하지 않았다.

경계 검사에서 영수증 검증 결과와 무관하게 `delete(c.tickets, ticket)`를 실행하던 결함도
재현했다. 다른 세션/자식의 영수증과 만료된 영수증이 그 요청에 의해 소비됐다. 이제 검증된
영수증만 생성 시작 때 소비한다. 또한 잘못된 모델/effort 요청이 실패한 수동 압축의 재시도
phase를 먼저 바꾸지 않도록 라우트 검증 뒤에 상태를 변경한다. 예약된 영수증 표식은 계수와
생성 모두에서 제거한다. 자격 정보·본문·영수증 값은 근거 파일에 저장하지 않았다.

## 실제 backend 검증

[opt-in 검사](../../go/internal/gateway/compaction_runtime_evidence_test.go)를 build tag
`runtime_evidence`와 `CLAUDUCT_EVIDENCE_LIVE=1`로 한 번 실행했다. gateway에서 실제
`upstream.Direct`를 통해 고정된 제품 backend에 요청했다. 공개 합성 상수만 전송했으며
모델 출력은 로컬에서 검사하고 다음 요청에 재사용하거나 저장하지 않았다.

`gpt-5.6-luna/high` 2회, `gpt-5.6-luna/medium` 2회의 별도 ledger가 합계 4회 상한을
강제했다. 실제 사용은 생성 3회와 `generate:false` 계수 1회다. 재시도·예산 거부는 없었다.
각 생성은 60초, 계수는 기존 30초 상한을 사용했다. 원래 환경의 runtime/home 검사,
credential 읽기·검증, TLS, redirect 거부와 목적지 고정을 그대로 사용했다.

| 단계 | 실제 요청 effort | backend input/output tokens | 결과 |
|---|---|---|---|
| 이전 일반 생성 | high | 28 / 7 | HTTP 200, 지정한 응답 표식 보존 |
| 자동 압축 사전 계수 | medium | input 295, 생성 없음 | 다음 생성과 `prior-count-cache` / `matched` |
| 자동 압축 생성 | medium | 295 / 114 | HTTP 200, 결정·제약·실패·다음 행동의 식별자 4개 모두 보존 |
| 이후 일반 생성 | high | 28 / 20 | HTTP 200, 지정한 응답 표식 보존 |

검사 본문 10.32초, 패키지 10.493초에 PASS했다. 수치 기록은 [live.json](live.json)에 있다.
영수증 소비의 추가 수정은 이 live 실행 뒤 발견되어 로컬 전체 gateway·race·mutation으로
검증했다. backend를 추가로 호출하지 않았다.

이 live 검사는 짧은 텍스트의 실제 protocol·effort·계수·필수 정보 보존을 확인한다.
native가 대용량 대화를 스스로 압축한 live TUI 검사는 아니다. 요약 품질 전반, 비용 절감량,
물리적 모델 정체성, 미디어가 포함된 압축 후 최초 초과까지 입증하지 않는다.

## native와 반증 검사

[실제 native 검사](../../go/internal/app/context_test.go)는 설치된 Claude Code와 빌드한 제품
hook, 임시 profile/project, loopback gateway를 실행한다. 여기의 응답과 usage는 합성이다.
네 모델 각각에서 high 일반 생성 → medium 자동 압축 1회 → high 재개를 확인했다.
병렬 자식 두 경우에서도 Sol/high의 압축만 medium으로 바뀌고 Terra/medium 및 자식 연결이
유지됐다. 실패한 압축은 자동 재실행하거나 정상 생성으로 이어지지 않았다.
[재시작 검사](../../go/internal/app/context_resume_test.go)는 저장된 Astra/high를 복원하여
Astra/medium으로 먼저 압축하고, 새로 선택한 Sol/medium으로 생성하는 경로를 확인했다.
이 묶음의 native 집중 검사 결과는 PASS, 23.320초다.

[gateway 검사](../../go/internal/gateway/compaction_effort_test.go)는 모든 effort와 auto/manual/
미지정 trigger, 영수증 범위·만료, 알 수 없는 라우트 거부와 수동 재시도 보존을 대조한다.
압축 계수와 생성의 encoded payload가 완전히 같고, 계수 동안 영수증과 journal/state가
바뀌지 않음을 검사한다. 디스크 journal 복원 뒤에도 같은 조건을 검사한다.

[mutation 기록](mutations.json)의 **10/10을 assertion 실패로 검출**했다. Go overlay로만
변경하여 작업 파일을 덮어쓰지 않았고 컴파일 오류는 검출로 세지 않았다.

- 자동 상한 제거, 수동 압축까지 상한 적용, 저장된 원래 라우트 덮어쓰기.
- 계수 지침 제거, 앞선 라우트 무시, 계수 시 영수증 소비 또는 표식 전달, journal 복원 생략.
- 잘못된 라우트가 수동 재시도 상태를 변경, 검증 실패한 영수증 소비.

## 검사 결과와 남은 범위

| 검사 | 결과 |
|---|---|
| 전체 `go test -count=1 -timeout=12m ./...`, CGO 0 | 전체 패키지 PASS. app 359.607초, gateway 23.475초 |
| 영수증 소비 수정 후 전체 gateway 재검사 | PASS, 26.277초 |
| 전체 gateway `-race` | PASS, 28.407초 |
| 압축·context 및 실제 native 압축/재시작 집중 `-race` | PASS. gateway 2.375초, app 33.979초 |
| `gofmt -l .`, `go vet ./...`, `go build ./...` | PASS, format 출력 없음 |
| `node verification/test-doc-citations.mjs` | 문서 121개, 인용 89개, 로컬 링크 664개, 실패 0 |
| `git diff --check` | PASS |

Windows / Go 1.27.1, `GOPROXY=off`, `GOTOOLCHAIN=local`에서 실행했다. native를 포함한
로컬 검사에서는 불필요한 상속 환경을 제외했다. race는 `CGO_ENABLED=1`과 기존
`C:\msys64\ucrt64\bin\gcc.exe`를 사용했다. 전체 모듈 race 재실행은 아니다.
실제 backend 실행은 환경을 우회하지 않고 제품 guard로 검증했다.

Node V1·PowerShell 전체 검증과 수동 TUI 검수는 이번 묶음에서 실행하지 않았다.
`#50`은 실제 미디어 초과 1회 관측 전까지 현행 추정·카운터를 유지하며 `#52`도 기존 wontfix다.
호스트 설정·설치본·원격 저장소는 변경하지 않았다. 원본 작업 트리의 추적 파일과 index는
깨끗하게 유지했다. 이번 변경도 commit/push/PR/태그/Release 없이 개발 worktree에 남겼다.
