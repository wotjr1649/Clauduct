# v0.3.1 출하 후보 검증 입력

2026-09-23. 원본 `D:\AIDEV\clauduct-v031`의 mixed 상태와 Node V1 기준선은 보존했다.
독립 후보 worktree에서 필요한 제품·검사·문서 입력을 선택했다. 기존 batch-04의 Go source
hash와 비교하고, 이후 공통 필터·취소·재연결 변경은 별도로 검토·검사했다.
[내장 필터 및 재연결 결과](../v031-httpguard-20260923/REPORT.md)를 참조한다.

## 이전 리뷰 A5-06의 journal 역호환 경계

[writer 검사](journal-writer_test.go)를 v0.3.1 gateway package에 overlay하여 실제
`d.route` → `saveChoice`가 만든 journal을 저장했다. 새 writer는 native metadata의 `Plan`과
`plan`을 모두 검증하고 canonical `Plan`으로 저장하며, 같은 버전의 reader는 둘 다 읽는다.
[writer 결과](journal-writer.txt).

[reader 검사](journal-reader_test.go)를 깨끗한 v0.3.0
`149068edd693fb860a03244a2ea15764bcd68c34`에 overlay했다. 저장 bytes는 바꾸지 않았다.
`Plan` 대조군은 읽혔으며 `plan` metadata는 정확 비교에서 거부됐다.
[reader 결과](journal-reader.txt). native metadata는 공개 합성 입력이다. 실제 native가
이번 실행에서 소문자 내장 역할을 발행했다는 관측으로 확장하지 않는다.

이 조건을 [되돌리기 안내](../../docs/v2/PACKAGING.md#6-되돌리기)와
[릴리스 안내](../../docs/v2/RELEASE-v0.3.1.md)에 추가하여, 기존 두 조건만으로 역호환 범위를
추정할 수 있던 문서 결함을 닫았다. 구버전의 검증을 느슨하게 하거나 journal을 변환하지 않는다.

재현은 각 test 파일을 해당 checkout의 `go/internal/gateway/zz_release_journal_test.go`에
매핑한 Go overlay를 사용한다. `CLAUDUCT_JOURNAL_EVIDENCE`는 이 입력 디렉터리의 절대 경로다.
writer 후 reader 순서로 각각 해당 `TestRelease*Journals`만 실행한다. 기존 생성 입력을
바꾸지 않으려면 새 task 디렉터리를 사용한다.

## 출하 바이너리 SDK 검사

[ship-sdk.ps1](ship-sdk.ps1)은 제품의 세 바이너리가 있는 `-PackageDir`와 이 스크립트 하위의
아직 없는 `-RunDir`를 받는다. 공개 project/profile/TEMP를 만들고 `clauduct.exe` 자체를 실행한다.
실제 구독 backend를 사용하므로 자동 CI에는 등록하지 않는다. 프로세스 세션 상한 240초,
native `--max-turns 16`, harness 전체 270초로 제한한다. 관련 없는 환경은 상속하지 않고,
strict MCP·외부 경로 Read 제한·두 공개 파일의 Edit 허용 범위를 사용한다.

Write 2개·Agent 1개·Workflow 1개와 완료 알림, 같은 native PID의 네 입력을 검수한다.
`-InjectLoss`는 공개 파일 Write 뒤 테스트 launcher listener와 native PID 양쪽 소유가 확인된
loopback TCP만 종료한다. AdGuard·WFP·방화벽 설정을 바꾸지 않는다. 이 harness는 계측용
제품 코드·mock transport·응답 replay를 주입하지 않는다. 원문 응답 대신 event 종류·counter와
공개 marker 일치 여부를 사용하며 최종 status의 누적 실패를 검수한다.

첫 출하물 실검증은 첫 Write의 권한 거부로 FAIL이었다(실 backend 3회, 생성 파일 0).
범위가 있는 Edit 규칙을 inline settings에 둔 경우 직접 native에서도 같은 거부가 재현됐다.
같은 규칙을 CLI `--allowedTools`에 두면 지정 파일만 생성되고 범위 밖 파일은 계속 거부됐다.
[검사 소스](permission-scope_test.go), [결과](permission-scope-final.txt). 이 측정에 따라 harness의
규칙 전달 위치만 변경했다. `dontAsk`와 두 파일의 경로 범위를 유지한다. 제품 Go 변경은 없다.

두 번째 실행은 네 입력과 의도적 손실·후속 작업을 실제로 마쳤으나 마지막 검수기의
Workflow JSON 필드 오기로 FAIL 종료했다. Go 필드 `Completed`의 wire 이름은
`successfulResponses`다. 원문 status에서 해당 값 1, 결과 수신 2, 실제 Write 성공 2와
의도한 두 failure 각 1을 확인한 뒤 검수기의 필드명을 수정했다. 실패 실행을 덮어쓰지 않는다.

이 디렉터리의 입력과 이전 계측 결과는 최종 commit의 출하물 성공 증거가 아니다.
빌드 hash, 실제 SDK/TUI 실행 및 격리 설치 결과는 해당 산출물과 함께 별도로 기록한다.
