# v0.3.1 세 번째 묶음 — projects 루트 분류와 진단

2026-09-21. `D:\AIDEV\clauduct-v031`, `fix/v031`에서
[#46](https://github.com/wotjr1649/Clauduct/issues/46)과
[#48](https://github.com/wotjr1649/Clauduct/issues/48)을 수정했다.
기준 commit은 `149068edd693fb860a03244a2ea15764bcd68c34`이며,
[첫 묶음](../v031-roles-20260921/REPORT.md)과
[두 번째 묶음](../v031-events-20260921/REPORT.md)의 변경을 보존했다.
로컬 개발 검사 결과이며 출시 판정은 아니다.

## 변경한 동작

projects 루트를 여는 14개 호출 지점을 [공통 opener](../../go/internal/gateway/projects.go)로
모았다. 상대 경로 이탈, 실제 루트 부재, 그 밖의 접근 실패를 구분한다. 읽기로 디렉터리를
만들지 않으며, 검증된 context 저장 경로만 기존 생성 동작을 유지한다. 이후 파일 접근은
`os.Root`를 사용하여 symlink/reparse 경계를 유지한다.

Windows에서는 일반 파일 아래의 존재할 수 없는 경로도 `os.ErrNotExist`로 보일 수 있다.
OpenRoot 실패가 이 오류일 때만 기존 상위 경로를 확인한다. 실제 디렉터리 아래의 부재만
`PROJECTS_ABSENT`로 분류하고, 일반 파일·확인할 수 없는 링크·공유 위반·접근 실패를
정상적인 첫 실행으로 간주하지 않는다. 이 분류는 각 reader의 긴 설명과 개별 OS 오류
판단을 대신한다.

| 상황 | 처리 |
|---|---|
| 실제 루트 부재, metadata 읽기 | 기존의 제한된 metadata 재시도 유지 |
| 실제 루트 부재, context 복원 | 저장된 이력이 없는 첫 실행으로 처리 |
| 실제 루트 부재, display provenance | 읽을 transcript가 없으므로 `unreadable`을 증가시키지 않음 |
| 일반 파일/파일 아래 경로/잠긴 루트 | metadata·context는 확인 실패, provenance는 `unreadable` 증가 |
| 루트 접근 실패, 선택 기록 복원 | 역할과 무관하게 확인 실패. `choiceAbsent`로 넘기지 않아 `agents.unrouted`가 증가하지 않음 |
| 정상 루트에서 선택 기록만 부재 | 기존 내장/사용자 역할 구분과 진단 유지 |
| Workflow 선택·복원·claim·checkpoint·결과 읽기 | 기존 증거 요구와 실패 처리를 유지하며 같은 루트 분류 사용 |
| 초기 생성 실패 뒤 루트 복구 | 현재 파일 시스템을 다시 확인하므로 과거 `projectsErr`가 재접근을 막지 않음 |

`projectsUnavailable`은 기존처럼 초기 생성 실패의 진단이다. 실시간 상태 필드로 바꾸지
않았으며, 이번 변경은 개별 transcript/journal 파일의 모든 부재 정책을 바꾸는 작업도 아니다.

## 실패 재현과 검사

[회귀 검사](../../go/internal/gateway/projects_root_test.go)는 실제 임시 경로와 Windows의
공유를 허용하지 않는 디렉터리 핸들을 사용한다. 파일 시스템 동작을 mock으로 대체하지 않는다.

수정 전 다음 assertion 실패를 관측했다.

- 없는 루트에서 사용자 역할의 선택 복원이 오류 없이 fallthrough했다.
- 일반 파일 아래 projects 경로가 `AGENT_METADATA_PENDING`이 되었다.
- 실제 HTTP 요청에서 루트 부재와 파일 아래 경로가 각각 `agents.unrouted=1`로 기록되었다.

수정 후에는 루트가 없는 경우·일반 파일인 경우·상위 구성 요소가 파일인 경우·잠긴 경우를
구분하고, production과 같은 context policy를 켠 HTTP 경로에서 backend 호출 0과 올바른
카운터를 확인했다. 정상 루트의 모르는 역할에는 여전히 카운터가 증가하는 대조군도 통과했다.
경로 이탈을 부재보다 먼저 거부하고, 읽기가 루트를 만들지 않으며, 초기 실패 뒤의 복구를
다시 확인하는 검사도 통과했다.

첫 묶음의 `TestAbsentProjectsChoiceDistinguishesKnownAndCustomRoles`는 #48의 변경에 맞춰
`TestMissingChoiceInAvailableProjectsDistinguishesKnownAndCustomRoles`로 바꿨다.
역할별 정책은 정상 루트의 기록 부재에서 검사하고, 루트 부재 자체의 거부는 새 검사에서
명시적으로 확인한다. metadata 재시도·context 첫 실행·저장 시 생성의 기존 검사는 보존했다.

| 검사 | 결과 |
|---|---|
| Gateway 전체, CGO_ENABLED=0 | PASS, 19.964s |
| `go test -count=1 -timeout=8m ./...`, CGO_ENABLED=0 | PASS. app 209.557s, gateway 20.223s |
| `go test -count=1 -timeout=3m -race ./internal/gateway`, CGO_ENABLED=1 | PASS, 23.157s |
| `go vet ./...`, `go build ./...`, `gofmt -l .` | PASS, gofmt 출력 없음 |
| `node verification/test-doc-citations.mjs`, `git diff --check` | PASS. 문서 118개, 인용 89개, 로컬 링크 627개, 실패 0 |

Go 1.27.1을 사용했고, 전체 제품 검사는 관련 없는 환경변수를 제거한 프로세스에서
`GOPROXY=off`, `GOTOOLCHAIN=local`로 실행했다. native 검사는 기존 임시 config/project와
로컬 fixture를 사용하며 실제 과금 backend를 호출하지 않는다.

## 수정을 되돌린 검사

[mutations.json](mutations.json)에 **8/8 assertion 실패**를 기록했다. 작업 소스를 변경하지
않는 Go `-overlay`를 사용했다.

1. 일반 파일인 상위 경로를 디렉터리로 간주.
2. metadata reader의 기존 개별 부재 판단 복원.
3. context reader의 기존 개별 부재 판단 복원.
4. provenance reader의 기존 개별 부재 판단 복원.
5. 없는 루트를 역할 선택의 fallthrough로 다시 연결.
6. 경로 이탈을 부재로 분류.
7. 읽기에서 projects 디렉터리 생성.
8. 과거 setup 오류로 복구된 루트 차단.

첫 mutation 작성 중 발생한 미사용 변수 컴파일 실패는 검출로 세지 않았다. 컴파일되는
변형으로 고친 뒤 실제 회귀 assertion 실패를 확인했다.

## 범위와 다음 묶음

변경은 Gateway의 루트 접근과 그 검사에 한정했다. 모르는 모델·effort 거부, Workflow의
복원/재실행 조건, `os.Root`의 reparse 차단을 완화하지 않았다. Windows 공유 위반을
실제로 검사했으며, ACL 자체를 변경하는 검사는 하지 않았다.

이번 묶음에서 수동 TUI 수용, 유료 backend, Node V1 전체, PowerShell 전체 검사와
Gateway 외 패키지의 race 재검사는 수행하지 않았다. commit·push·PR·태그·Release·설치본
변경도 하지 않았다. 원래 작업 루트의 미추적 자료는 변경하지 않았다.

다음 묶음은 **#54·#55 — argv 경계 처리와 launcher 문서 계약**이다.
#53·#58과 승인된 #56은 이후 묶음으로 남으며, #50의 실제 관측 조건과 #52의 wontfix를 유지한다.
