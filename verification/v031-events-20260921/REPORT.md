# v0.3.1 두 번째 묶음 — 턴 영수증·재개·세션 정리

2026-09-21. `D:\AIDEV\clauduct-v031`, `fix/v031`에서
[#49](https://github.com/wotjr1649/Clauduct/issues/49),
[#47](https://github.com/wotjr1649/Clauduct/issues/47),
[#57](https://github.com/wotjr1649/Clauduct/issues/57)을 구현했다.
기준 commit은 `149068edd693fb860a03244a2ea15764bcd68c34`이며,
[첫 묶음](../v031-roles-20260921/REPORT.md)의 변경을 보존했다.
이 기록은 로컬 개발 검사 결과이며 출시·수동 TUI 수용 판정이 아니다.

## 변경

| 이슈 | 변경과 근거 |
|---|---|
| #49 | `active/root` 또는 `active/child-<agent>` 아래에 `<sequence>-<turn>.json`을 한 번 쓰고, 쓰기가 끝나면 빈 `.ready` 파일을 만든다. 최신 시도의 완료 표시가 없으면 이전 턴을 대신 선택하지 않는다. 요청에서 읽은 영수증을 선택·continuation·결과 binding·취소·실패 기록이 공유하므로, 처리 중 다른 턴이 생겨도 실패를 그 턴에 옮겨 붙이지 않는다. [native 기록 코드](../../go/internal/app/native-events.mjs), [읽기·실패 기록](../../go/internal/gateway/native_events.go), [턴 경계 검사](../../go/internal/gateway/turn_receipt_test.go) |
| #47 | `awaiting_children`와 stopped 재개가 같은 새 결과 생성식을 사용한다. task identity·선택·부모만 보존하고, 이전 턴의 native 모델·effort·복구 여부·실패·본문·streaming 상태를 초기화한다. 이미 확인한 다음 턴의 `continuationTurn`은 waiting 경로에서 보존한다. stopped 결과는 기존 archive 정책을 유지하며 현재 보고로 재전달하지 않는다. 용량 회수 시 본문 byte 집계도 맞춘다. [재개 검사](../../go/internal/gateway/result_restart_test.go), [continuation 검사](../../go/internal/gateway/continuation_test.go) |
| #57 | `Run`이 이번 호출에서 만든 plugin 폴더를 소유한다. native 회수·Gateway drain·최종 진단 뒤에 폴더와 PDF 보조 실행 파일을 정리한다. spawn 실패와 setup 이후 조기 반환도 같은 정리 경로를 탄다. 회수/drain이 불확실하면 삭제하지 않고 `NATIVE_EVENT_CLEANUP_UNVERIFIED`, 실제 삭제 실패는 `NATIVE_EVENT_CLEANUP_FAILED`를 `CleanupErr`에 기록한다. 기존 세션 폴더를 검색하거나 일괄 삭제하지 않는다. [수명주기 코드](../../go/internal/app/run.go), [실제 OS 정리 검사](../../go/internal/app/native_cleanup_test.go) |

## Native API 확인과 실패에서 수정한 부분

설치된 Claude Code는 **2.1.278**, Go는 **1.27.1 windows/amd64**였다.
native의 `/plugin-types`를 임시 프로젝트에서 생성해 확인했다. `fs.write`는 필요한
하위 폴더를 생성하며 `read`, `list`, `exists`, `stat`가 있다. **`fs.append`와 `rename`은 없다.**
이슈·인계에 있던 append 가정은 맞지 않았다.

첫 append 방식은 Node filesystem fixture에서는 통과했지만 실제 native에서는
`PARENT_WAIT_UNVERIFIED`, backend 호출 0으로 실패했다. 생성한 선언을 읽고 append 방식을
제거했으며, 최종 구현은 native가 지원하는 `write`만 사용한다. plugin validator와
Node가 없는 child PATH의 실제 native 위임 검사가 최종 구현으로 통과했다.
생성한 선언 전체와 임시 probe는 저장소에 남기지 않았다.

턴 기록은 진행 중인 write도 용량에 포함한다. 실패한 Promise는 캐시에서 제거하여
동일 턴의 다음 step이 다시 시도할 수 있다. 완료·진행 중인 턴은 합계 **4096개**,
실패 재시도를 포함한 파일 생성 시도는 **8192회**로 제한한다. reader도 최대 16384개의
항목과 2048-byte JSON만 읽는다. `os.Root`, 식별자·파일명·session·agent·turn 검증과
모델·effort 거부 정책을 유지한다. 영수증에는 prompt·answer·도구 본문을 쓰지 않는다.

## 관측한 검사

검사 프로세스에서 관련 없는 환경변수를 제거하고 `GOPROXY=off`, `GOTOOLCHAIN=local`을
사용했다. 실제 native는 임시 config/project와 로컬 fixture backend로 실행했다.
검사 시 시스템 managed settings 디렉터리는 없었다. 유료 backend는 호출하지 않았다.

| 검사 | 결과 |
|---|---|
| `CGO_ENABLED=0 go test -count=1 -timeout=8m ./...` | PASS. app 159.509s, gateway 20.045s |
| `CGO_ENABLED=1 go test -count=1 -timeout=8m -race ./...` | PASS. app 177.240s, gateway 22.353s |
| 새 상태 초기화·완료 표시·요청 고정·root identity 검사 | PASS |
| `TestNativeSessionDirectoryCleanupAndFinalEvidence` | 정상 종료, native exit 7, spawn 실패, 실제 OS 취소·회수, 실제 Windows 파일 잠금 모두 PASS |
| `TestNativeEventModuleRunsWithNoNodeOnChildPATH` | 실제 native 위임과 영수증 관측 PASS |
| `TestNativeEventPluginPassesInstalledValidator` | PASS |
| `node go/internal/app/native_events_publication_test.mjs` | 실패한 본문/완료 표시 쓰기 재시도, 이전 파일 보존, 중복 기록 방지, 동시 마지막 용량 경쟁·상한 PASS |
| `go vet ./...`, `go build ./...`, `gofmt -l .` | PASS, gofmt 출력 없음 |
| `node verification/test-doc-citations.mjs`, `git diff --check` | PASS. 문서 117개, 인용 89개, 로컬 링크 621개, 실패 0 |

JS 검사는 제품 모듈을 그대로 로드하고 실제 파일 시스템을 사용한다. 실패는 파일 위치에
디렉터리를 놓아 발생시킨다. Go의 `TestNativeEventPublicationFailureAndCapacity`에서도
실행되며, Node가 없는 개발 환경에서는 이 검사만 명시적으로 skip한다. 제품 runtime에는
Node 의존성을 추가하지 않았다.

## 변경을 되돌린 검사

[mutations.json](mutations.json)에 실패 assertion을 기록했다. Go는 `-overlay`로 9개,
JS는 별도 임시 사본으로 2개 결함을 재주입했고 **11/11 검출**했다. 작업 소스나
설치본에 mutation을 활성화하지 않았다.

- 이전 `begin()` 복원, `Recovered` 이월, 다음 continuation 식별자 누락.
- 재개 중 용량 회수에서 제거된 본문의 byte 집계를 유지하는 결함.
- 완료 표시 검사 제거, 실패 기록 시 최신 턴 재조회.
- 폴더 정리 제거, 정리 오류 은폐, 최종 진단 전 폴더 삭제.
- 실패한 native write 캐시 유지, 4096개 턴 상한 제거.

기존 `stillOnTurn` 비교 전용 검사는 요청을 먼저 접수한 뒤 디스크를 다음 턴으로 진행시키고,
실패 category·옛 end 영수증 배제·부모 전달까지 확인하는 검사로 대체했다.
새 객체 생성에 맞춰 기존 검사도 현재 entry를 다시 참조한다. 거부·취소·archive·완료
assertion을 제거해 통과시키지 않았다.

## 한계와 다음 작업

- 턴 조회는 agent별 디렉터리의 제한된 스캔이다. 최대 용량의 지연·처리량은 측정하지 않았다.
- plugin 영수증은 세션 전용 임시 형식이며 구버전 실행 중인 프로세스와 혼용하지 않는다.
  progress/step 영수증까지 모두 불변 파일로 바꾼 것은 아니다.
- OS가 정리를 거부하거나 종료/drain을 확인할 수 없으면 폴더가 남을 수 있다. 정상 성공으로
  숨기지 않으며 자동으로 다른 세션의 폴더를 수거하지 않는다.
- setup의 모든 개별 filesystem 실패 지점을 주입하지 않았다. 공유 defer와 반환 경로를 검토했고,
  실제 spawn 실패·실제 삭제 실패·진단 순서를 실행으로 확인했다.
- 수동 TUI Esc, 실제 과금 backend, Node V1 전체, PowerShell 전체 검사는 이번 묶음에서 하지 않았다.
- commit·push·PR·태그·Release·설치본 변경은 하지 않았다. 원래 `D:\AIDEV\Clauduct`의
  미추적 자료는 변경하지 않았다.

다음 묶음은 **#46·#48**의 projects 루트 부재 정책과 진단 분류다.
#50은 실제 1회 관측 전까지 현행 유지, #52는 기존 wontfix,
#56은 승인된 압축 전용 medium 상한 구현이 대기 중이다.
