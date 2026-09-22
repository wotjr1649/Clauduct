# A4 — 전송·프로세스 분야 리뷰 (v0.3.1 전체 변경)

- 리뷰 대상 루트: `D:\AIDEV\clauduct-v031` (git worktree). 기준 commit `149068edd693fb860a03244a2ea15764bcd68c34`.
- HEAD == 기준 commit, v0.3.1 변경 전체가 미커밋 작업트리에 있음. 읽기 전용 git 명령만 사용했고 index/작업트리를 변경하지 않았다.
- 환경: Windows 11 Pro 26200 / `go version go1.27.1 windows/amd64` / 관측 native 2.1.278.
- 담당 초점: 연결 수명주기와 close/half-close, gateway 서빙, 앱 실행과 정리, native 취소, 오류 분류, 진단.
- 제품 파일은 하나도 수정하지 않았다. 재현이 필요한 부분은 전부 `go test -overlay`로만 주입했다.

---

## 1. 실행 근거 (/code-review)

**호출 원문 (Skill 도구, 1회):**

```
Skill(skill='code-review',
      args='xhigh go/internal/gateway/connection.go go/internal/gateway/gateway.go go/internal/gateway/native_cancellation.go go/internal/gateway/errors.go go/internal/gateway/diagnostics.go go/internal/app/run.go')
```

`ultra`, `--fix`, `--comment`는 쓰지 않았다.

**반환 원문:**

```
Skill "code-review" completed (forked execution).

Result: Skill execution completed
```

**관측 사실과 판정**

| 항목 | 관측 |
|---|---|
| 스킬 로드 | 호출은 수락되어 forked execution으로 실행됨. 이 세션 대화에는 스킬 본문(레시피)이 주입되지 않았다. |
| 스킬이 낸 지적 | **0건.** 지적 목록·파일 목록·요약 어느 것도 이 세션으로 돌아오지 않았다. `ReportFindings` 결과도 관측되지 않았다. |
| 스킬이 실제 훑은 범위 | **판정 불가.** 반환값에 범위 정보가 없다. 커버리지 관점에서는 담당 6개 제품 파일 전부를 스킬 미커버로 취급했다. |
| 관측 가능한 effort / 모델 표기 | **없음.** `xhigh`를 인자로 넘겼지만 반환값·출력 어디에도 effort나 모델 문자열이 표기되지 않았다. 세션 자체는 Opus 5(`claude-opus-5`)로 동작한다. |
| 재호출 | 하지 않았다. 지시상 추가 서브에이전트를 띄우지 않고, 같은 단위를 두 번 실행하지 않는다. |

→ **결론: `/code-review`는 호출했으나 결과가 없으므로 지적 산출 측면에서는 `NOT_RUN`으로 기록한다.** 아래 2절 커버리지와 3절 지적은 **전부 직접 리뷰(source=manual)** 로 얻은 것이다. 스킬 기인 지적은 0건이므로 스킬/직접 구분은 자명하다. 스킬이 담당 파일을 하나도 다루지 않은 것으로 간주하고 26개 전부를 직접 리뷰해 커버리지를 채웠다.

**내장 스킬 vs 미설치 플러그인 구분 (직접 확인)**

- 이 세션에서 쓴 `code-review`는 **Claude Code CLI 내장 스킬**이다(available-skills 목록 항목).
- 같은 이름의 마켓플레이스 플러그인 `claude-plugins-official/code-review`와 `pr-review-toolkit`은 **설치되어 있지 않다.** `C:\Users\js\.claude\plugins\installed_plugins.json`의 설치 목록은 다음 7개뿐이고 두 이름 모두 없다:
  `superpowers@superpowers-marketplace`, `ponytail@ponytail`, `codex@openai-codex`, `typescript-lsp@claude-plugins-official`, `gopls-lsp@claude-plugins-official`, `claude-mem@thedotmack`, `skill-creator@claude-plugins-official`.
  (확인 명령은 8절의 `plugins-check`, exit 0.)

---

## 2. 커버리지 표 (담당 26개 파일, 전부 커버)

| # | 파일 | 검토 방식 | 비고 |
|---:|---|---|---|
| 1 | `go/internal/app/native_cleanup_test.go` | 전체 읽음 + 실행 | 5개 mode 전부 PASS. `locked` mode는 실제 Windows `CreateFile` 핸들로 삭제 실패를 만든다(mock 아님) |
| 2 | `go/internal/app/run.go` | diff + 본문 전체(1–60, 122–520) 읽음 | 정리 defer·settings slot·대기 오류 분류 추적 |
| 3 | `go/internal/gateway/connection.go` | 신규 파일 전체 읽음 + overlay 재현 2종 | A4-01/02/04의 근거 |
| 4 | `go/internal/gateway/connection_test.go` | 전체 읽음 + 실행(+`-race`) | PASS |
| 5 | `go/internal/gateway/diagnostics.go` | diff + `record`/`ClientReport`/`Snapshot` 주변 전체 읽음 | A4-05 |
| 6 | `go/internal/gateway/diagnostics_test.go` | diff 읽음 + 실행 | 단언이 `Totals.Failures`까지 **강화**됨(약화 없음) |
| 7 | `go/internal/gateway/errors.go` | 전체 읽음 | 새 두 카테고리 메시지. `refuse()`의 1초 read deadline 포함 검토 |
| 8 | `go/internal/gateway/gateway.go` | diff + `Start`/`handle`/`Close`/`RefusalsByCategory` 전체 읽음 | A4-01/02/03 |
| 9 | `go/internal/gateway/native_cancellation.go` | 전체 읽음 + 호출처 grep | 파일 읽기 → `entry.nativeTurn` 전환 검증 |
| 10 | `go/internal/gateway/native_cancellation_test.go` | diff 읽음 + 실행(+`-race`) | 새 `active/root`·`active/child-*` 경로 반영. PASS |
| 11 | `go/internal/gateway/socket_runtime_evidence_test.go` | 전체 읽음 + `go vet -tags runtime_evidence` PASS | 실행은 `CLAUDUCT_SOCKET_EVIDENCE=1` 스위치가 없어 **NOT_RUN**(8절) |
| 12 | `verification/socket-half-close.c` | 전체 읽음 | 빌드/실행 안 함(8절 NOT_RUN) |
| 13 | `verification/socket-shutdown-etw.c` | 전체 읽음 | A4-07(도달 불가 개선 제안) |
| 14 | `verification/socket-shutdown-path.c` | 전체 읽음 | `#define main …` + `#include "socket-half-close.c"` 재사용 방식 확인 |
| 15 | `verification/test-http-transport.mjs` | diff + 관련 전 구간 읽음 | 새 `catch`가 참조하는 `helperTests`(9행)·`activeClient`(71행)가 모듈 스코프임을 확인 → TDZ 없음 |
| 16 | `verification/v031-close-20260922/KERNEL-ANALYSIS.md` | 전체 읽음 | A4-01 권장안의 환경 주의사항 근거 |
| 17 | `verification/v031-close-20260922/REPORT.md` | 전체 읽음 | 채택 정책과의 충돌 여부 판정 |
| 18 | `verification/v031-close-20260922/UPSTREAM-ISSUE.md` | 전체 읽음 | 개인 경로·자격증명 없음 확인 |
| 19 | `verification/v031-close-20260922/component-snapshot.txt` | 전체 읽음 | 보고서 수치와 일치 |
| 20 | `verification/v031-close-20260922/driver-call-sites.txt` | 전체 읽음 | 정적 RVA만, 런타임 적중 주장 없음 |
| 21 | `verification/v031-close-20260922/evidence.json` | 전체 읽음 | `filter_setup_check: PASS_10_PAYLOAD_FILTERS`가 etw.c의 필터 생성 수(2×2 + 2×3 = 10)와 일치 |
| 22 | `verification/v031-close-20260922/kernel-trace.json` | 구조·앞뒤·불변식 읽음(683행, 이벤트 배열은 표본) | `captured=17`이 KERNEL-ANALYSIS와 일치 |
| 23 | `verification/v031-close-20260922/shutdown-path.txt` | 읽음(형식 전체 + 앞 25행 상세) | callout 열거 형식 일관 |
| 24 | `verification/v031-close-20260922/socket-trace.txt` | 전체 읽음 | 15/200·7/200이 REPORT/evidence와 일치 |
| 25 | `verification/v031-transport-20260922/REPORT.md` | 전체 읽음 | 구현 주장 3건을 코드와 대조(A4-04) |
| 26 | `verification/v031-transport-20260922/evidence.json` | 전체 읽음 | `close.grace_ms=100`, `drain_bytes=65536`이 `connection.go` 상수와 일치 |

**호출자/피호출자 확장 검토(담당 파일 밖, 판정 근거로만 읽음):**
`go/internal/gateway/messages.go`(handler 순서·`agentSelection`), `native_events.go`(`readCurrentNativeTurn`/`validActiveReceipt`/`recordFailedAgentRequest`), `context.go`(`EnableContextPolicy`), `go/internal/app/session_lifecycle.go`(`waitForSession`), `go/internal/app/user_settings.go`(`takeUserSettings`), `go/internal/app/native_events.go`(`prepareNativeEvents`), `go/internal/childprocess/process_windows.go`(`Process.Wait`의 `errors.Join` 모양), `go/internal/gateway/client_capability_test.go`, `$(go env GOROOT)/src/net/http/server.go`.

---

## 3. 지적 목록

### A4-TRANSPORT-01 — `confirmed` / **medium** / 신규 회귀 / 출처: 직접

**제목:** `responseConn`이 `net.Conn` 인터페이스를 임베드해 `CloseWrite()`를 숨긴다. net/http의 RST 회피 half-close가 v0.3.1에서 사라졌다.

- **파일:** 검토 당시 `go/internal/gateway/connection.go` 33–39행 (구조체), `go/internal/gateway/gateway.go:179` (listener 교체)
- **발생 조건:** HTTP/1.1 요청이 body를 남긴 채 조기에 거부되어 net/http가 `conn.closeWriteAndWait()` 경로로 들어갈 때. 구체적으로
  (a) 선언된 `Content-Length`가 net/http의 `maxPostHandlerReadBytes`(256 KiB)를 넘고 핸들러가 body를 읽지 않은 경우 → `requestTooLarge()`,
  (b) `refuse()`가 건 1초 read deadline 안에 남은 body를 버리지 못한 경우 → `closedRequestBodyEarly()`,
  (c) 헤더 초과(431) / 미지원 Transfer-Encoding.
  게이트웨이의 조기 거부 경로는 전부 body를 읽기 전에 `return`한다: `UNSUPPORTED_METHOD`, `UNSUPPORTED_MEDIA_TYPE`, `INVALID_HEADER`, `LOCAL_SESSION_REQUIRED`, `TOO_MANY_REQUESTS`, `GATEWAY_CLOSED`, 그리고 **v0.3.1이 새로 추가한 `CONTEXT_REQUEST_CLASS_UNVERIFIED`**(`messages.go:48-50`). 실제 Claude Code `/v1/messages` 본문은 256 KiB를 쉽게 넘는다. `maxRequestBytes`는 32 MiB라 MaxBytesReader가 먼저 막지도 않는다.
- **원인:** `responseConn`이 구체 타입이 아니라 `net.Conn` **인터페이스**를 임베드한다. 따라서 `*responseConn`의 메서드 집합에 `CloseWrite()`가 없고, `$(go env GOROOT)/src/net/http/server.go:1822`의 `if tcp, ok := c.rwc.(closeWriter); ok { tcp.CloseWrite() }`가 항상 실패한다. v0.3.0은 `g.server.Serve(listener)`로 원본 `*net.TCPConn`을 그대로 넘겼으므로(`git show 149068e:go/internal/gateway/gateway.go`의 172행) 이 단언이 성공했다. `connection.go`의 100ms drain은 이 경로를 대신 덮지 못한다(A4-02: 게이트가 `r.Close`뿐).
- **영향:** `closeWriteAndWait()`가 FIN 없는 500ms `rstAvoidanceDelay` sleep으로 전락하고, 그 뒤 미수신 데이터가 남은 소켓을 완전 close 한다 → 상대는 FIN을 한 번도 보지 못하고 **RST**를 받는다. stdlib 주석이 명시한 "클라이언트가 FIN을 먼저 보고 마지막 응답을 처리할 기회"가 사라진다. 이 저장소가 문서화한 바로 그 환경(필터 활성)에서 응답 손실이 관측되는 경로이며, 하필 v0.3.1이 새로 넣은 안내 메시지("Update Claude Code or repair the local integration")가 이 경로로 나간다.
- **재현 명령** (작업 디렉터리 `D:/AIDEV/clauduct-v031/go`, 환경 `CGO_ENABLED=0`, 원본 무수정 overlay):
  ```
  CGO_ENABLED=0 go test \
    -overlay=D:/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A4-transport-process/overlay.json \
    ./internal/gateway/ -run 'TestA4' -count=1 -timeout 300s -v
  ```
  exit code 1.
- **예상 결과:** stock net/http와 동일하게 응답 프레이밍 직후 FIN(클라이언트 `io.EOF`)이 도착한다.
- **실제 결과:**
  - `TestA4ResponseConnHidesCloseWrite` — **결정적 FAIL.** `responseConn hides CloseWrite; net/http closeWriteAndWait can no longer half-close (inner *net.TCPConn does have it)`. 내부 conn에는 `CloseWrite`가 있고 wrapper에는 없다.
  - `TestA4FinLatencyAfterEarlyRefusalWithLargeBody/product_response_listener` — 401 본문 수신 후 **499–500ms 무응답 뒤 `wsarecv: An existing connection was forcibly closed by the remote host`(RST)**. 제품 경로 관측 11회 전부 ≥499ms, 응답 직후 FIN 0회.
  - `TestA4FinLatencyOnRequestClassRefusal` — `CONTEXT_REQUEST_CLASS_UNVERIFIED` 안내 본문(backend 호출 0회) 수신 후 **500–507ms 침묵 후 RST/EOF**(관측 4회 전부 ≥500ms).
  - 대조군(래핑 없는 stock `net/http` 서버, 같은 조기 거부 모양) — 관측 6회 중 4회 **0–3ms `io.EOF`**(FIN 즉시), 2회는 이 호스트의 문서화된 필터 간섭(5s timeout / 499ms RST).
  - 제안 수정(overlay로만 `CloseWrite` 위임 추가) 적용 시 — `TestA4ResponseConnHidesCloseWrite` PASS, 제품 경로 관측 4회 전부 **0–1ms `io.EOF`**.
- **증거 경로:**
  - `D:/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A4-transport-process/a4_closewrite_probe_test.go`
  - `.../overlay.json`, `.../overlay-fix.json`, `.../connection_with_closewrite.go`
  - `.../evidence-current.txt`, `.../evidence-with-proposed-fix.txt`, `.../evidence-repeat-runs.txt`
- **권장 수정 (글로만):** `responseConn`에 위임 메서드를 추가한다.
  ```go
  func (c *responseConn) CloseWrite() error {
      if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok { return cw.CloseWrite() }
      return errors.ErrUnsupported
  }
  ```
  이 한 메서드만으로 위 세 검사가 모두 통과함을 overlay로 확인했다(`evidence-with-proposed-fix.txt`, `evidence-repeat-runs.txt`). 대안(또는 병행)은 A4-02의 게이트 확장이다.
- **미확인 부분 / 주의:**
  - `KERNEL-ANALYSIS.md`는 **이 호스트에서 `shutdown(SD_SEND)` 자체가 자기 소켓에 가짜 수신 FIN을 주입당한다**고 기록한다. 다만 `closeWriteAndWait()`는 FIN 직후 곧바로 완전 close 하는 지점이라 서버가 그 수신 방향을 더 쓰지 않으므로 무해하며, 실제 관측도 개선(RST→EOF)이었다. 그래도 수정 채택 시 이 상호작용을 한 번 더 확인하는 것이 맞다.
  - 실제 native 클라이언트가 이 경로에서 응답 본문을 실제로 잃는지는 이번에 live TUI로 재측정하지 않았다(이 리뷰의 backend 호출 0회).
  - 채택 정책과의 관계: 이것은 "독립 Node/.NET/raw TCP 검사의 이전 FAIL"이 아니라 **제품 listener + 제품 handler**의 v0.3.0 대비 회귀이므로 `ARCHITECTURE 6.1`의 완료 기준(보호 On 상태의 제품 실제 동작) 안쪽이며, 재보고 금지 대상이 아니다.

---

### A4-TRANSPORT-02 — `improvement` / **medium** / 신규(설계 공백) / 출처: 직접

**제목:** 새 drain이 `r.Close`(클라이언트가 `Connection: close`를 보낸 경우)에만 걸려 있어 서버 주도 종료를 전혀 덮지 않는다.

- **파일:** `go/internal/gateway/gateway.go:165-170`
- **발생 조건:** net/http가 스스로 연결을 닫기로 결정한 모든 경우 — 미소비 body, 쓰기 오류, keep-alive 비활성, shutdown 중 응답. 이때 요청 헤더에는 `Connection: close`가 없으므로 `r.Close == false`이고 `finished`는 false로 남는다.
- **원인:** `finished`를 세우는 조건이 `r.Close && r.ProtoMajor == 1 && r.ProtoMinor >= 1`이다. "요청이 close를 요구했는가"이지 "이 연결을 지금 닫는가"가 아니다. 제품은 응답에 `Connection: close`를 **직접 붙이지 않으므로**(`gateway.go:398`, `messages.go:458,665`는 전부 `keep-alive`) 서버 주도 종료는 순전히 net/http의 `closeAfterReply`로 결정되고, 그 경로는 drain을 보지 못한다.
- **영향:** `v031-transport-20260922/REPORT.md`가 말한 "FIN을 보내기 **전**에 drain" 보호가 서버 주도 종료에는 전혀 적용되지 않는다. A4-01과 겹쳐 그 경로는 drain도 half-close도 없는 상태가 된다.
- **재현 명령:** A4-01과 동일(위 overlay 검사).
- **예상 결과:** drain이 걸렸다면 종료가 `rstAvoidanceDelay`(500ms) + `responseCloseGrace`(100ms) ≈ 600ms 근처가 된다.
- **실제 결과:** 제품 경로 관측 11회가 전부 **499–507ms**, 즉 100ms drain이 붙지 않았다 → `finished == false`가 관측으로 뒷받침된다.
- **증거 경로:** `D:/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A4-transport-process/evidence-repeat-runs.txt`
- **권장 수정:** `finished` 판정을 요청 헤더가 아니라 "이 연결이 종료로 확정되었고 응답 프레이밍이 끝났는가"로 바꾼다. 예: `Server.ConnState`의 종료 직전 신호를 쓰거나, 최종 응답 `Connection` 값을 확인하거나, 최소한 "선언된 body가 남았는데 읽지 않았다"를 조건에 추가한다. 다만 A4-01의 `CloseWrite` 위임이 더 작고 확실한 수정이며, 둘 다 넣는 편이 안전하다.
- **미확인 부분:** drain을 서버 주도 종료까지 확장했을 때 이 호스트의 필터가 어떻게 반응하는지는 측정하지 않았다.

---

### A4-TRANSPORT-03 — `hold` / **low** / 신규 / 출처: 직접

**제목:** 새 drain이 실제 native 세션에서 한 번이라도 발동하는지에 대한 근거가 없다.

- **파일:** `go/internal/gateway/gateway.go:165-170`, `verification/v031-transport-20260922/evidence.json`
- **구체적 코드 근거:** drain의 필요조건은 `r.Close == true`, 즉 **클라이언트가** 요청에 `Connection: close`를 실어 보내는 것이다. 제품은 응답으로 close를 요구하지 않는다(A4-02). `before` 측정의 42/200 실패는 `connection_test.go:37`의 `&http.Transport{DisableKeepAlives: closeEach}`, 즉 **Go 검사 클라이언트가 스스로 선택한** `Connection: close`에서 나왔다.
- **왜 결론을 못 냈나:** Claude Code 2.1.278(및 readiness 프로브 `Bun/1.4.3`)이 `Connection: close`를 보내는지에 대한 관측이 어느 evidence에도 없다. `live_tui`는 수정 전/후 모두 PASS이므로 이 경로의 발동 여부를 가르지 못한다.
- **결론을 내리려면 필요한 것:** 실제 세션 1회에서 `r.Close == true`인 요청 수(또는 요청 `Connection` 헤더 분포) 관측. 진단 계정에 카운터 한 개(예: `requests.clientCloseRequested`)를 넣으면 이후 세션에서 자동으로 답이 나온다.
- **재현 명령:** NOT_RUN — 실제 backend/TUI 실행은 이 리뷰 범위 밖이며 이번 리뷰의 backend 호출은 0회다.
- **영향(가정 성립 시):** 100ms drain이 프로덕션에서 죽은 코드가 되고, 실제 사용자 경로를 지키는 것은 A4-01의 half-close뿐이게 된다.

---

### A4-TRANSPORT-04 — `improvement` / **low** / 신규 / 출처: 직접

**제목:** 스트리밍 응답에서는 write 오류 표시가 handler 반환 시 무조건 덮인다. `REPORT.md`의 "write 오류…는 일반 완료 대기와 구분한다" 주장이 스트리밍에는 성립하지 않는다.

- **파일:** 검토 당시 `go/internal/gateway/connection.go` 41–47행, `go/internal/gateway/gateway.go:165-170`
- **발생 조건:** SSE 등 handler 안에서 flush 하며 쓰는 응답에서 write가 실패했고, handler가 `r.Context().Err() == nil`인 채로 돌아온 경우.
- **원인:** `Write`는 오류 시 `finished`를 false로 내리지만, 스트리밍의 모든 write는 handler **안에서** 일어난다. handler가 돌아온 직후 `gateway.go:168`이 `c.finished.Store(r.Context().Err() == nil)`로 **무조건 다시 세운다.** 버퍼링되는 비스트리밍 응답은 flush가 handler 반환 뒤라 의도대로 동작한다.
- **영향:** 이미 깨진 소켓에 대해 최대 100ms의 불필요한 drain. 정확성 손실은 없다(깨진 소켓의 read는 즉시 실패). 문서 주장과 구현의 불일치가 본질.
- **재현 명령:** NOT_RUN — 코드만으로 성립하며 실패 주입 비용 대비 가치가 낮다고 판단했다. 확인하려면 `connection_test.go`의 스트리밍 사례에서 중간 write를 실패시키고 `finished`를 읽으면 된다.
- **권장 수정:** `finished`를 세울 때 기존 값을 지우지 말고, `r.Context().Err() != nil`이면 false로 내리고 아니면 `CompareAndSwap(false, true)`처럼 "write 오류가 이미 내린 값"을 존중하거나, 별도 `writeFailed` 플래그를 둔다.

---

### A4-TRANSPORT-05 — `improvement` / **low** / 신규(기존 패턴 확대) / 출처: 직접

**제목:** `Snapshot()`이 `g.contexts`를 동기화 없이 읽는다.

- **파일:** `go/internal/gateway/diagnostics.go:844`, 관련 `go/internal/gateway/messages.go:48`, 쓰기는 `go/internal/gateway/context.go:97`(`EnableContextPolicy`)
- **발생 조건:** `Start()`가 이미 accept 루프 goroutine을 띄운 뒤에 `EnableContextPolicy()`가 `g.contexts`를 쓴다(`run.go`의 기본 `StartGateway` 클로저). 그 사이에 요청이 도착하면 Go 메모리 모델상 동기화 없는 읽기가 된다.
- **원인:** `g.contexts`가 plain 포인터 필드이고 잠금/atomic이 없다.
- **영향:** 실사용에서는 그 창 동안 포트가 아직 child에 전달되지 않아 도달 불가에 가깝고 `-race`로도 잡히지 않는다. 다만 v0.3.1이 이 필드를 읽는 지점을 2곳(요청 admission, 진단 보고) 늘렸다.
- **재현 명령:** NOT_RUN — 창을 인위적으로 벌리지 않으면 관측되지 않는다. 확인하려면 `Start()`와 `EnableContextPolicy()` 사이에 요청을 밀어 넣는 `-race` 검사를 쓰면 된다.
- **권장 수정:** `EnableContextPolicy`를 `Start` 이전 구성으로 옮기거나 `g.contexts`를 `atomic.Pointer[contextGuard]`로 둔다.

---

### A4-TRANSPORT-06 — `improvement` / **low** / 신규 / 출처: 직접

**제목:** 정리 판정이 같은 함수가 명시적으로 금지한 `==` 비교와 수제 `Unwrap() []error` 분해를 쓴다.

- **파일:** `go/internal/app/run.go:344-350`
- **현재 코드:**
  ```go
  cleanupWaitErr := waitErr
  if joined, ok := waitErr.(interface{ Unwrap() []error }); ok && len(joined.Unwrap()) == 1 {
      cleanupWaitErr = joined.Unwrap()[0]
  }
  _, nativeExit := cleanupWaitErr.(*exec.ExitError)
  nativeCleanupReady = reaped && (waitErr == nil || nativeExit || waitErr == ctx.Err() || waitErr == context.Canceled)
  ```
- **왜 지금은 결함이 아닌가:** 프로덕션 모양이 마침 맞는다. `childprocess.Process.Wait()`는 `errors.Join(err, jobErr, p.copyErr)`를 돌려주므로 정상 종료는 `nil`, 비정상 종료 단독은 원소 1개짜리 join이라 위 분해가 정확히 그것을 노린다. 취소는 `waitFor`/`waitForSession`이 `ctx.Err()`를 감싸지 않고 그대로 돌려준다. `native_cleanup_test.go` 5개 mode 전부 PASS.
- **왜 지적하는가:** 같은 함수 **12행 아래**에 `// errors.Is, not ==. A cancellation that reached here wrapped -- which is the ordinary shape once it has passed through a layer that annotates it`라는 주석과 `errors.Is(waitErr, context.Canceled)`가 있다. 새 코드는 그 규칙을 어기고 `==`를 쓴다. `Process`는 주입 가능한 인터페이스이므로, 누군가 `Wait()`에 `fmt.Errorf("...: %w", err)` 한 줄을 더하거나 `Join` 원소가 2개가 되는 순간 정상 세션마다 `nativeCleanupReady=false`가 되어 임시 plugin 디렉터리가 매번 남고 `NATIVE_EVENT_CLEANUP_UNVERIFIED`가 항상 붙는다. 조용히 틀리는 종류의 취약함이다.
- **영향(현재):** 없음. **영향(회귀 시):** 세션마다 `%TEMP%\clauduct-native-events-*` 누수 + 계정의 거짓 경보.
- **재현 명령:** NOT_RUN — 현재 트리에서는 트리거되지 않는다. 확인하려면 `Options.StartProcess`에 `Wait()`가 `fmt.Errorf("x: %w", exitErr)`를 돌려주는 fake Process를 넣고 `native_cleanup_test.go`와 같은 단언을 돌리면 된다.
- **권장 수정:** `var exitErr *exec.ExitError; nativeCleanupReady = reaped && (waitErr == nil || errors.As(waitErr, &exitErr) || errors.Is(waitErr, context.Canceled) || errors.Is(waitErr, context.DeadlineExceeded))`. 수제 분해는 삭제한다.

---

### A4-TRANSPORT-07 — `improvement` / **low** / 신규 / 출처: 직접

**제목:** `socket-shutdown-etw.c`의 `length` 누적이 `snprintf`의 "잘림 무시" 반환값이다.

- **파일:** `verification/socket-shutdown-etw.c` (detail 필드 루프, `length += snprintf(line+length, sizeof(line)-(size_t)length, …)`)
- **원인:** `snprintf`는 잘린 경우 *쓰려고 했던* 길이를 돌려준다. `length >= sizeof(line)`이 되면 `sizeof(line)-(size_t)length`가 `size_t` 언더플로로 거대한 값이 되어 오버플로가 된다.
- **왜 지금은 도달 불가인가:** 헤더 포맷 상한 ~200B, detail 루프는 6필드 × ~24B, `line`은 4096B다. 스택 프레임 루프는 쓰기 직전에 `if ((size_t)length + 18 >= sizeof(line))` 가드가 있다.
- **재현 명령:** NOT_RUN — C 진단기는 이 리뷰에서 빌드/실행하지 않았다(8절).
- **권장 수정:** 매 `snprintf` 뒤에 `if (length < 0 || (size_t)length >= sizeof(line)) return;`를 넣거나 `length`를 `sizeof(line)-1`로 클램프한다.

---

## 4. 반박된 지적과 반박 근거

| ID | 의심 | 반박 근거 |
|---|---|---|
| R1 | v0.3.1의 새 정리 defer가 "불확실하면 보류"라서 v0.3.0보다 디렉터리를 더 많이 남기는 회귀 | `git show 149068e:go/internal/app/run.go`에는 `RemoveAll`이 **아예 없다**(`nativePlugin`은 192행에서 만들고 정리 코드 없음). v0.3.0은 세션마다 `%TEMP%\clauduct-native-events-*`를 무조건 누수했다. v0.3.1은 순수 개선이고 보류 경로도 v0.3.0과 동일한 수준이다. `prepareNativeEvents`가 중간 실패로 반환한 부분 디렉터리까지 새로 정리된다. |
| R2 | `run.go:236`의 `settings = ""`가 병합된 설정을 통째로 잃게 한다 | `takeUserSettings`는 `present == true`일 때만 `userSettings`를 non-nil로 돌려주고, 그 `present`를 세우는 같은 분기에서 `slots = append(slots, len(forward))`를 반드시 실행한다(`user_settings.go`). `userSettings != nil && len(settingsSlots) == 0`인 상태를 만들 수 없다. 병합 결과는 사용자의 원래 `--settings` 위치에 그대로 들어간다. |
| R3 | `record`가 ring에서 재사용되어 `nativeTurn` 포인터가 요청 간에 남는다 | `ring.open`(`diagnostics.go:652`)이 요청마다 `&record{...}`를 새로 할당한다. 재사용 없음. `agentSelection`의 `entry.nativeTurn = nil`은 방어적 중복이다. |
| R4 | `RequestClassMissing`이 `RefusedBy`와 같은 맵을 공유해 진단 읽기에서 경합 | `RefusalsByCategory()`(`gateway.go:345-356`)가 `refusalMu` 아래에서 새 맵으로 복사해 돌려준다. `Snapshot`이 이를 한 번만 호출해 두 필드에 쓰는 것은 오히려 스냅숏 일관성을 높인다. |
| R5 | `bindNativeCancellation`이 영수증 파일 직접 읽기를 버려서 root(main) Esc 취소가 깨진다 | 같은 요청에서 `agentSelection`(`messages.go:126`)이 먼저 `readCurrentNativeTurn("")` → `active/root`를 읽어 `validActiveReceipt`로 검증한 뒤 `entry.nativeTurn`에 고정하고, `bindNativeCancellation`은 그 **뒤**(`messages.go:183`)에만 호출된다. 구 코드가 root에 대해 `validActiveReceipt`를 건너뛰던 차이는 `agentSelection`이 이미 더 엄격하게 막는다. `TestNativeAbortIsBoundToExactRequestWithoutDependingOnSocket`·`TestNativeAbortStopsUpstreamWhileHTTPConnectionRemainsOpen`가 `-race`로 PASS. |
| R6 | `v031-close-20260922/evidence.json`의 `unchanged_http_sha256`가 v0.3.1에서 수정된 `test-http-transport.mjs`와 불일치 | 현재 작업트리 파일의 sha256 = `304098b8cd9f…` = 기록값이며 `v031-recheck-20260922/evidence.json:54`의 `same_test_code_sha256`과도 같다. hash는 수정 **후** 파일 기준으로 일관되게 기록되어 있다(v0.3.0 blob은 `10686a4a…`로 다르지만 그것을 "unchanged"라고 주장하지 않았다). |
| R7 | `ReconcileNativeCancellations`가 `n.mu`를 쥔 채 `p.cancel()`을 불러 교착 | `context.CancelFunc`는 context 내부 잠금만 쓰고 `nativeEvents.mu`를 다시 잡지 않는다. handler의 `finishCancellation`은 `defer` 순서상 나중이며 경쟁 시 대기만 한다. 잠금 순서도 항상 `n.mu → record.mu` 한 방향이다. `-race`로 관련 검사 PASS. |
| R8 | 100ms drain의 `io.CopyN`이 net/http의 background read와 같은 소켓을 동시에 읽어 데이터가 갈린다 | `conn.serve`는 `finishRequest()`에서 `c.r.abortPendingRead()`로 background read를 끝내고 read deadline을 0으로 되돌린 **뒤에** `c.close()`(= 우리 `Close`)를 부른다(GOROOT `net/http/server.go`). 동시 read 없음. `-race`로 `TestGatewayResponsesSurviveConnectionTurnover` 등 PASS. |
| R9 | `record` 구조체 필드 순서 변경(`nativeTurn`을 맨 앞으로)이 위치 기반 복합 리터럴을 깬다 | `record{` 리터럴 8곳 전부 키를 명시한다(grep). `go vet` PASS. |
| R10 | `diagnostics_test.go` 변경이 단언을 약화했다 | 반대다. 두 곳 모두 기존 조건에 `len(got.Totals.Failures) != 1 || got.Totals.Failures[want] != 1`을 **추가**했다. 약화·mock 대체 없음. |
| R11 | `test-http-transport.mjs`의 새 `catch`가 TDZ/미정의 참조로 원래 실패를 가린다 | `helperTests`(9행), `activeClient`·`passed`·`received`·`invalidRequests`(71행) 전부 `try`(146행) 이전 모듈 스코프 선언이며, `catch`는 `throw error`로 원래 실패와 종료 상태를 보존한다. `finally`의 `clearTimeout(deadline)`도 유지된다. |

---

## 5. 보류 항목과 결론 조건

- **A4-TRANSPORT-03** (3절). 결론에 필요한 것: 실제 세션에서 요청의 `Connection: close`(= `r.Close`) 발생 횟수 관측 1회. 진단 계정에 카운터 하나를 추가하면 이후 자동으로 답이 나온다.
- **A4-TRANSPORT-01의 사용자 영향 크기.** RST로 400/401 본문이 실제로 유실되는 빈도는 필터·버전·타이밍에 따라 달라진다. 이번엔 loopback에서 FIN 부재만 결정적으로 보였다. 결론에 필요한 것: 조기 거부 + 256 KiB 초과 body 조건에서 실제 native 클라이언트가 본문을 읽었는지 확인하는 live 관측.

---

## 6. 개선 제안 (결함 아님)

1. **A4-04 / A4-05 / A4-06 / A4-07** (3절의 improvement 항목).
2. `connection.go`에 "왜 `net.Conn` 인터페이스를 임베드하는가, 그래서 어떤 optional 인터페이스(`CloseWrite`, `io.ReaderFrom`)를 잃는가"를 한 줄로 남기면 A4-01 같은 회귀가 리뷰에서 바로 걸린다. (`io.ReaderFrom` 상실은 SSE가 chunked라 net/http가 애초에 그 경로를 쓰지 않으므로 실질 영향 없음 — 7절 참조.)
3. `bindNativeCancellation`의 `validActiveReceipt(id, session, agent)` 재검사는 `agentSelection`이 같은 헤더로 이미 통과시킨 값에 대한 중복이다. 남길 거면 "방어적 중복" 주석을, 지울 거면 그대로 삭제해도 동작은 같다.
4. `socket_runtime_evidence_test.go`의 `socketEvidenceError`는 `syscall.Errno`를 timeout보다 먼저 검사한다. Go deadline 타임아웃은 errno가 아니어서 현재 오분류는 없지만, `SO_RCVTIMEO` 기반 오류가 섞이면 `errno_10060`이 `timeout` 라벨을 가린다. 순서를 바꾸는 편이 라벨 의미가 안정적이다.
5. `verification/test-http-transport.mjs`의 성공 경로는 `loopbackTests`, 실패 경로는 `loopbackTestsPassed`로 키 이름이 다르다. 집계 도구가 생기면 걸린다.
6. `socket_runtime_evidence_test.go`의 `t.Fatal("probe deadline")`은 그 반복의 서버 goroutine과 listener를 남긴 채 끝난다. 진단용이라 실해는 없으나 `t.Cleanup`으로 정리하면 반복 간 간섭이 사라진다.

---

## 7. 근거 부족으로 뺀 의심 (삭제하지 않고 기록)

- **`responseConn`이 `io.ReaderFrom`을 숨겨 sendfile/splice 최적화를 잃는다.** net/http의 `(*response).ReadFrom`은 `!w.cw.chunking`일 때만 그 경로를 타고, 이 게이트웨이의 SSE/JSON 응답은 chunked이거나 작은 버퍼 쓰기다. 측정 근거 없음.
- **100ms drain이 세션 처리량을 깎는다.** `TestGatewayResponsesSurviveConnectionTurnover`의 800요청이 3초 안에 끝난다(Go 클라이언트가 먼저 닫아 drain이 즉시 EOF로 종료). 근거 없음.
- **`ReconcileNativeCancellations`가 `p.record`를 붙든 채 ring에서 밀려난 record에 `CancellationSource`를 써서 증거가 사라진다.** 잠금 순서와 defer 순서상 handler의 `entry.finish()`가 항상 나중이라 실제 손실 시나리오를 구성하지 못했다.
- **`refusalMessage`의 새 문장이 어떤 소비자의 파싱을 깬다.** `refusalMessage`는 JSON 오류 본문에만 쓰이고 기록되는 카테고리는 `r.category` 상수다. 파싱 소비자를 찾지 못했다.
- **`g.refuse`의 1초 read deadline이 새 drain과 충돌한다.** `Close`가 자체 deadline을 다시 설정하므로 충돌 경로를 구성하지 못했다.
- **`socket-shutdown-path.c`의 `trial()`이 setup 오류를 2로 세어 `diagnostic_failures` 집계를 부풀린다.** 진단기 집계 스타일 문제이고 보고된 수치(6/6 premature EOF)와 모순되지 않는다.
- **`responseConn.Close`의 강제 경로가 `once`를 표시하지 않아 이중 close가 된다.** `*net.TCPConn`의 두 번째 `Close`는 `ErrClosed`를 돌려줄 뿐이고 net/http는 그 반환을 쓰지 않는다. 실해 구성 실패.

---

## 8. 실행한 명령 전체 목록 / NOT_RUN

작업 디렉터리는 표기한 것 외에는 `D:/AIDEV/clauduct-v031`. Go 검사는 모두 `D:/AIDEV/clauduct-v031/go`에서 `CGO_ENABLED=0`으로 돌렸고, race 검사만 그 명령 환경에만 `CGO_ENABLED=1 CC=C:/msys64/ucrt64/bin/gcc.exe`를 줬다(전역 설정 변경 0건).

| # | 명령 | exit |
|---:|---|---:|
| 1 | `git status --porcelain=v1` | 0 |
| 2 | `git rev-parse HEAD` / `git diff --stat` | 0 |
| 3 | `git diff --stat -- <담당 26개 경로>` | 0 |
| 4 | `git diff -- go/internal/gateway/{connection,gateway,errors,native_cancellation,diagnostics}.go go/internal/app/run.go` | 0 |
| 5 | `git diff -- go/internal/gateway/{diagnostics_test,native_cancellation_test}.go verification/test-http-transport.mjs` | 0 |
| 6 | `git diff -- go/internal/app/user_settings.go` | 0 |
| 7 | `git show 149068edd693fb860a03244a2ea15764bcd68c34:go/internal/gateway/gateway.go` (Serve/ConnContext/closing grep) | 0 |
| 8 | `git show 149068edd693fb860a03244a2ea15764bcd68c34:go/internal/app/run.go` (nativePlugin/RemoveAll/CleanupErr grep) | 0 |
| 9 | `git show 149068e…:verification/test-http-transport.mjs \| sha256sum` | 0 |
| 10 | `sha256sum verification/test-http-transport.mjs verification/test-dotnet-http-transport.mjs` (+ CRLF 변형 비교) | 0 |
| 11 | `grep -rn` 다수 (호출처·상수·복합 리터럴·개인정보 스캔) | 0 |
| 12 | `go env GOROOT` / `grep -n "closeWriter\|CloseWrite\|ReaderFrom" "$GOROOT/src/net/http/server.go"` | 0 |
| 13 | `sed -n` 다수 (제품 소스·GOROOT `net/http/server.go`·verification 산출물 읽기) | 0 |
| 14 | (cwd `go`) `CGO_ENABLED=0 go vet -overlay=…/overlay.json ./internal/gateway/` | 0 |
| 15 | (cwd `go`) `CGO_ENABLED=0 go test -overlay=…/overlay.json ./internal/gateway/ -run 'TestA4' -count=1 -timeout 300s -v` | **1** (A4-01 재현) |
| 16 | (cwd `go`) 위 명령 `-run 'TestA4FinLatency'` 5회 반복 | **1** ×5 |
| 17 | (cwd `go`) `CGO_ENABLED=0 go test -overlay=…/overlay-fix.json ./internal/gateway/ -run 'TestA4' …` 4회 | 0 또는 1(환경 간섭 시) |
| 18 | (cwd `go`) `CGO_ENABLED=0 go test ./internal/gateway/ -run 'TestGatewayResponsesSurviveConnectionTurnover\|TestResponseCloseDrainIsBoundedAndInterruptible\|TestCompletedResponseDoesNotWaitForASilentPeer\|TestNativeAbort\|TestMissingRequestClass\|TestRequestClassCapability\|TestRequestClassRequirement\|TestDeliveryCancellationRequiresContextEvidence\|TestStreamReadCancellationAndFailureEvidenceSurviveEviction' -count=1 -timeout 300s` | 0 |
| 19 | (cwd `go`) `CGO_ENABLED=0 go test ./internal/app/ -run 'TestNativeSessionDirectoryCleanupAndFinalEvidence' -count=1 -timeout 300s -v` | 0 |
| 20 | (cwd `go`) `CGO_ENABLED=1 CC=C:/msys64/ucrt64/bin/gcc.exe go test -race ./internal/gateway/ -run 'TestGatewayResponsesSurviveConnectionTurnover\|TestResponseCloseDrainIsBoundedAndInterruptible\|TestCompletedResponseDoesNotWaitForASilentPeer\|TestNativeAbort' -count=1 -timeout 600s` | 0 |
| 21 | (cwd `go`) `CGO_ENABLED=0 go vet -tags runtime_evidence ./internal/gateway/ ./internal/app/` | 0 |
| 22 | (cwd `go`) `gofmt -l internal/gateway internal/app` (출력 없음) | 0 |
| 23 | `plugins-check`: `python -c "import json; …"` on `~/.claude/plugins/installed_plugins.json` | 0 |

**NOT_RUN 목록**

| 검사 | 이유 | 추가 확인 방법 |
|---|---|---|
| `go test ./...` 전체 회귀 | 지시상 금지(통합 검토자 판단 사항) | 통합 단계에서 결정 |
| `TestRuntimeEvidenceSocketClose` / `TestRuntimeEvidenceSocketShutdown` | `//go:build runtime_evidence` + `CLAUDUCT_SOCKET_EVIDENCE=1` 스위치가 없으면 skip. 이 리뷰에서 환경 스위치를 켜지 않았다 | `CGO_ENABLED=0 CLAUDUCT_SOCKET_EVIDENCE=1 go test -tags runtime_evidence ./internal/gateway/ -run TestRuntimeEvidenceSocket -count=1` |
| `verification/socket-half-close.c` / `socket-shutdown-path.c` / `socket-shutdown-etw.c` 빌드·실행 | 컴파일과 ETW 세션 생성은 코드 리뷰 범위 밖이고 ETW 세션은 호스트 상태를 건드린다 | 보고서에 기록된 `gcc -Wall -Wextra -Werror … -lws2_32 -liphlpapi` (ETW는 추가로 `-ltdh -ladvapi32`), `etw.exe --check` |
| 실제 native TUI / 실제 backend 재검증 | 이 리뷰의 backend 호출은 0회. 기존 `live_tui` 결과를 새 실행으로 대체하지 않는다 | 기존 수용 검수기(`go/internal/app/integration_tui_test.go`) 재실행 |
| A4-04 / A4-05 / A4-06 / A4-07의 실패 주입 재현 | 현재 트리에서 트리거되지 않거나(A4-06) 관측 가치가 낮다고 판단(A4-04/05/07) | 각 항목의 "재현 명령" 칸 참조 |
| A4-01의 live 본문 유실률 측정 | 필터·타이밍 의존 확률 관측이라 리뷰 범위에서 신뢰할 수치를 만들 수 없다 | 조기 거부 + 256 KiB 초과 body 조건의 실제 클라이언트 관측 |

**재현 자산 (전부 내 repro 디렉터리 안에만 생성)**

```
D:/AIDEV/clauduct-v031/verification/v031-code-review-20260922/run-01/repro/A4-transport-process/
  a4_closewrite_probe_test.go        # overlay로만 주입되는 리뷰 프로브
  overlay.json                       # 현재 트리 + 프로브
  overlay-fix.json                   # 현재 트리 + 프로브 + 제안 CloseWrite 위임
  connection_with_closewrite.go      # connection.go 사본 + 제안 메서드 (제품 아님)
  evidence-current.txt               # 현재 트리 실행 기록
  evidence-with-proposed-fix.txt     # 제안 수정 실행 기록
  evidence-repeat-runs.txt           # 현재 5회 / 제안 3회 반복 기록
```

제품 소스·테스트·문서·기존 `verification/` 파일은 하나도 수정·생성·삭제하지 않았다. `git add|commit|stash|checkout|reset|restore|clean|rebase`를 쓰지 않았고, 훅·필터·설정·driver·설치본을 바꾸지 않았으며, push/PR/댓글/태그/게시는 없다. 작업트리 루트의 미추적 `%SystemDrive%/`는 건드리지 않았다. 다른 분야의 디렉터리에는 쓰지 않았다.
