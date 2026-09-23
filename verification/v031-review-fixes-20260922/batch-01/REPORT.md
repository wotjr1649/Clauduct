**v0.3.1 리뷰 수리 묶음 1 — A2-01·A2-02·A3-02·INT-01**

2026-09-22, `D:\AIDEV\clauduct-v031`, branch `fix/v031`.
기준 commit은 `149068edd693fb860a03244a2ea15764bcd68c34`이며 기존 미커밋 v0.3.1 변경 위에
적용했다. 대상은 Claude 세션 `a0616359-5dc7-45d4-9eb3-fd1b761c4af7`의 리뷰를 평가한 뒤
사용자가 지정한 첫 수정 묶음이다. 원본 리뷰는 `verification/v031-code-review-20260922/run-01/`에 보존한다.

**이 묶음의 수정·로컬 검수는 PASS다.** 세 제품 결함과 검사 진단을 수정했고,
최소 재현·수정 제거 대조·전체 Go·전체 race 검사를 통과했다.
v0.3.1 전체의 릴리스 판정은 다른 미해결 리뷰 항목 때문에
**CHANGES_REQUIRED**를 유지한다. 배포·설치본·보호 설정은 변경하지 않았다.

**수정과 보존한 계약**

| ID | 원인 | 수정 |
|---|---|---|
| A2-01 | 요청의 native turn만 기억하고 실패 처리 시 현재 결과가 그 요청의 것인지 확인하지 않아, 새 `awaiting_children` 결과를 이전 turn으로 교체함 | admission에서 결과 객체와 그때의 turn도 포착한다. 실패 처리의 metadata 확인 뒤 같은 객체·turn·대기 상태인지 잠금 안에서 재검증하고, 교체·turn 연결을 한 임계 구간에서 수행한다 |
| A2-02 | `beginAnswer`가 이전 객체를 보유한 채 완료되고, 대기 중이던 stop을 agent ID만으로 현재 객체에 적용함 | 완료 시 원래 객체·turn을 확인하고, stop 처리로 잠금을 넘긴 뒤에도 원래 결과 객체인지 확인한다. 새 결과·본문·byte 누계를 보존한다 |
| A3-02 | compact 진입에서 `phase=compacting`을 먼저 설정한 뒤 journal 저장에 실패하면 종료 callback이 설치되지 않아 복구 상태로 전환되지 않음 | 진입 저장 실패를 `failed`로 전환한다. 저장 장애가 계속되면 계속 거부하고, 해소 뒤 새 manual receipt로 같은 프로세스에서 복구한다 |
| INT-01 | 잘린 SSE의 JSON 파싱에서 먼저 Fatal하여 `io.ReadAll`의 원래 오류를 버림 | 상태·수신 byte·읽기/close/파싱 오류·경과 시간·text byte·terminal 개수를 함께 기록하고 실패 누계에 반영한다 |

제품 구현은 [native_events.go](../../../go/internal/gateway/native_events.go),
[results.go](../../../go/internal/gateway/results.go), [context.go](../../../go/internal/gateway/context.go),
[messages.go](../../../go/internal/gateway/messages.go), [diagnostics.go](../../../go/internal/gateway/diagnostics.go)를 수정했다.
추가된 admission 정보는 handler 내부용으로 진단 JSON에 직렬화되지 않는다.

실패 처리는 최신 영수증을 다시 읽어 요청을 다른 turn으로 돌리지 않는다. 자신이 포착한
결과가 보존된 경우의 새 turn 거부·종료·부모 전달은 기존 검사로 유지했다. 결과 교체가
발생했을 때는 이전 요청이 새 상태를 바꿀 권한이 없는 것으로 처리한다.

선택·역할·metadata·session 검증은 유지한다. 일반 생성과 자동 compact는 실패 복구를
임의로 시작하지 못한다. 사용한 compact ticket을 재사용하거나 영수증 없이 재시도해도
거부된다. 기존 모델·effort·usage를 보존하고 정상 수동 압축 뒤 새 usage를 수집한다.
원격 사전 계수, 자동 생성 replay, native 재시작을 추가하지 않았다.

INT-01은 [connection_test.go](../../../go/internal/gateway/connection_test.go)의 검사 진단만
바꿨다. 2초 client timeout, 각 경우 200회, 본문 길이·terminal·backend 호출 수 assertion은
유지한다. JSON 파싱 오류를 실패 누계에 포함하며, 오류를 허용하여 PASS시키지 않는다.
응답 본문과 인증 정보는 로그에 추가하지 않았다. 제품 `connection.go`는 변경하지 않았다.

**관측한 검증**

| 검사 | 결과 / 근거 |
|---|---|
| 새 결함 회귀의 수정 전 실행 | A2-01·A2-02·A3-02 모두 FAIL, `before-tests.txt` |
| 수정 후 관련 회귀 | PASS, `focused-tests.txt`; 기존 pinned turn 실패 전달·정상 stop/stream 순서·compact effort 경계 포함 |
| stop 잠금 대기 중 자식 재개 | `TestStreamCompletionRechecksResultAfterWaitingForStop` PASS. callback 첫 검사 뒤 결과가 바뀌는 순서를 동기화하여 재현 |
| A2-01 수정 제거 | 새 결과 보존 assertion FAIL, `mutation-A2-01.txt` |
| A2-02 최종 소유권 확인 제거 | 잠금을 기다리던 stop이 새 결과를 종료하여 FAIL, `mutation-A2-02.txt` |
| A3-02 수정 제거 | `compacting` 고착 assertion FAIL, `mutation-A3-02.txt` |
| INT-01 전후 오류 주입 | 둘 다 기대한 FAIL. 보완 후에는 원래 오류가 보존됨, `diagnostic-before.txt` / `diagnostic-after.txt` |
| `gofmt -l .`, `CGO_ENABLED=0 go vet ./...`, `go build ./...` | PASS |
| `go vet -tags runtime_evidence ./...` / `go vet -tags policy_evidence ./internal/upstream` | PASS. 실제 backend 검사를 실행했다는 뜻은 아님 |
| `CGO_ENABLED=0 go test -count=1 -timeout=12m ./...` | PASS, 18개 패키지. app 286.682초 / gateway 40.533초, `go-test-all.txt` |
| `CGO_ENABLED=1 go test -count=1 -race -timeout=12m ./...` | PASS, 18개 패키지. app 293.954초 / gateway 54.476초, `go-test-race-all.txt` |
| 문서 인용 검사 | PASS, 추적 문서 128개·로컬 링크 724개·오류 0. 이 새 보고서의 상대 링크 6개도 별도 확인 |
| 실제 backend / interactive TUI | NOT_RUN — 이번에는 로컬 결함 재현·수정·회귀를 수행 |

INT-01의 대조는 첫 streaming 응답의 읽기 결과에 공개 합성 조각과 `io.ErrUnexpectedEOF`를
주입한 검사 진단 테스트다. 실제 필터가 잘린 응답을 만들었다는 재현으로 해석하지 않는다.
기존 검사는 `invalid SSE JSON`만 남겼고, 수정 후에는 다음을 기록했다.

```text
iteration=0 elapsed=8.8223ms status=200 bytes=14 read_error=unexpected EOF close_error=<nil> parse_error=unexpected end of JSON input text_bytes=0 stops=0
requests=200 failures=1
product responses lost on 1 connections
```

세 제품 수정의 mutation은 Go overlay로만 적용했다. 작업트리의 제품 소스를 되돌리지
않았으며, mutation에서 예상한 FAIL은 실제 정상 회귀의 FAIL과 구분한다.
명령·overlay·예상/관측 종료 코드는 `mutations.json`, 각 로그에 있다.
overlay와 수정 전 원본은 task 전용 `.tmp/v031-review-batch1-20260922/`에 있다.
이번에 변경한 11개 파일의 전후 SHA256은 `before.json` / `after.json`, 기존 미커밋 변경과
구분한 patch는 `task-changes.patch`에 있다. `git apply --check --reverse`로 현재 파일에서
이번 변경만 되돌릴 수 있음을 확인했으며 실제 되돌리기는 실행하지 않았다.
race는 설치된 `C:\msys64\ucrt64\bin\gcc.exe`와 그 디렉터리를 해당 검사 프로세스의
`CC`·`PATH`에만 지정했다. 전역 설치·환경 설정은 바꾸지 않았다. 일반 회귀와 race를
동시에 실행하지 않았으며, 둘 다 첫 실행에서 통과했다. 정상 전체 회귀에는 기존 제품
HTTP/SSE 800건 검사가 포함된다. 이 성공은 이전 간헐적 전송 실패의 원인 해결을 뜻하지 않는다.

**재현 범위와 다음 작업**

상태 검사는 실제 gateway 상태 전이와 task 임시 journal을 사용한다. A3-02는 실제 파일
rename 실패를 유발하고 장애 지속·해소·수동 압축·후속 생성을 같은 gateway에서 검사했다.
정상 생성 1회·복구 compact 1회·후속 생성 1회만 합성 upstream에 도달했고 사전 계수는 0회다.
A2는 제어한 turn/완료 순서의 회귀이며 실제 backend에서 동일 interleaving을 관측했다는
뜻은 아니다. 보호 On은 사용자의 마지막 확인 상태이며 이번에 UI를 재측정하지 않았다.

후속 통신 작업은 보완된 로그로 간헐적인 전송 실패의 원인을 판별하는 것이다. A4-01의
`CloseWrite` 제안과 실제 native drain 도달 여부는 이 묶음에서 종결하지 않았다.
다음에는 보호 On의 실제 native 경로에서 오류 응답·서버 종료·취소·후속 요청을 확인한다.
다른 Workflow·argv·plugin 역할·count 분류·검수 공백의 리뷰 항목도 별도 묶음으로 남는다.
