# v0.3.2 후보의 실제 TUI 검사 (Claude Code 2.1.280)

2026-09-23. 대상은 `release/v0.3.2-candidate`(`dfcfe52`)에 아래의 검사 수정을 더한 트리다.
제품 코드는 후보와 같다. native는 2.1.280, Go 1.27.1, `CGO_ENABLED=0`이다.

## 결과

| 검사 | 실행 | 결과 |
|---|---|---|
| `TestRuntimeEvidenceNativeTUILocal` | 사용자, Windows Terminal | PASS, 53.38초 |
| `TestRuntimeEvidenceNativeTUILocal` | 에이전트, [PTY 조작 도구](ptydrive.go) | PASS, 22.68초 ([화면](local-screen.txt)) |
| `TestRuntimeEvidenceNativeTUI` (실제 backend) | 사용자 | 실패, `EVIDENCE_PRIVATE_PATH`. 아래 원인 |
| `TestRuntimeEvidenceNativeTUI` (실제 backend) | 에이전트, 같은 도구 | **PASS, 56.10초** ([화면](live-screen.txt), [단계](live-driver.txt)) |

실제 backend 실행은 `gpt-5.6-luna`/`low`, ledger 한도 5회였다. backend 시도 5회, 추론 5회였다.
생성 3·압축 1·취소 1, transcript의 사실·순서 검사, native exit 0, 정리 성공이 모두 맞았다.
거부 4건은 모두 auxiliary 요청의 `ROUTE_NOT_AUTHORISED`로, backend에 보내기 전의 로컬 거부다.
입력은 이전 수용과 같은 순서다: 사실 기억 → `/compact` → 사실 확인 → 긴 출력 요청 뒤 Esc →
복구 → `/exit`. 화면 기록에 `C:\Users` 경로는 없다.

## 사용자 실행이 실패한 원인

사용자 실행에서는 첫 입력과 압축이 backend에 갔다. 그다음 입력부터는 `API Error: 400
EVIDENCE_PRIVATE_PATH`로 전송 전에 거부됐다. 검사의 전송 guard는 본문에 `C:\Users` 경로가 있으면
backend에 보내지 않는다. 이 guard는 그대로 둔다.

- 검사가 hook을 검사 바이너리 옆에 만든다(`hookPath`가 `os.Executable()`의 디렉터리를 쓴다).
- 안내한 명령이 검사 바이너리를 `%TEMP%`, 즉 `C:\Users\…\Temp`에 만들었다.
- native 2.1.280은 `/compact` 다음 요청에 명령 출력(`<local-command-stdout>Compacted PreCompact
  ["…/clauduct-hook.exe"] completed successfully…`)을 싣는다. 여기에 hook 경로가 들어간다.

이 순서는 과금 없는 [임시 검사](hookpath-probe_test.go)로 확인했다. hook 경로가 처음 들어가는 요청은
압축 바로 다음 요청이다([기록](hookpath-probe.txt)).

v0.3.1 코드 리뷰도 같은 계열을 지적했다([A5](../v031-code-review-20260922/run-01/areas/A5-verification-compat.md)).
그때는 TEMP가 기본값이면 작업 디렉터리가 첫 요청에서 막힌다는 내용이었고, 요청 전에 멈추라는
권고는 반영되지 않았다. hook 경로 쪽은 이번에 처음 드러났다. v0.3.1의 수용 실행은 검사 바이너리를
저장소의 `.tmp` 아래에 만들어 두 조건을 모두 피했다.

## 수정

- `integrationLive`는 이제 임시 루트나 hook 경로가 `C:\Users` 아래면 인증을 읽기 전에 멈춘다.
  `C:\Users`에 둔 검사 바이너리로 실제 backend 스위치를 켜면 0초에 멈추고 요청은 0건이다
  ([기록](fail-fast.txt)). CI가 돌리는 `TestIntegrationPrivatePathsStopBeforeTransport`가 이 확인도 검사한다.
  확인 함수가 항상 통과하도록 되돌리면 이 검사는 "a private temporary root was accepted"로 실패한다.
  `integrationLive`에서 호출을 빼는 되돌림은 live 실행에서만 드러나며 자동 검사는 잡지 못한다.
- `go/README.md`에 검사 바이너리 위치를 적었다. PowerShell에서 `-test.*` 인자를 따옴표로 묶어야
  한다는 점도 적었다. 처음 안내한 명령은 따옴표가 없어 `flag provided but not defined: -test`로
  시작하지 못했다.

## PTY 조작 도구

에이전트의 셸에는 터미널이 없다. `winpty`도 `stdin is not a tty`로 실행되지 않았다.
[ptydrive.go](ptydrive.go)는 Windows ConPTY로 검사 바이너리를 띄우고,
[단계 파일](steps.json)대로 화면 문구를 기다려 입력한다. 표준 라이브러리만 쓴다.

- 부모의 표준 핸들이 파이프면 자식이 그 핸들을 물려받아 pseudo console을 보지 못한다. 그래서
  `STARTF_USESTDHANDLES`에 무효 핸들을 넣는다.
- TUI는 커서 이동으로 단어를 배치하므로, 공백을 모두 뺀 화면 문자열로 대기 조건을 맞춘다.
- 첫 실행 profile은 테마 선택, 보안 안내, 폴더 신뢰 화면을 차례로 띄운다. 폴더 신뢰의
  기본값은 "No, exit"라서 아래 화살표로 "Yes"를 고른다.
- native는 응답이 끝날 때까지 본문 대신 토큰 수만 보여 준다. 그래서 취소는 `↓N tokens`가
  보이고 2.5초 뒤 Esc를 보낸다.

```bash
go build -o ptydrive.exe ptydrive.go
ptydrive.exe -dir <repo>\go\internal\app -script steps.json -log raw.log -text screen.txt \
  -env TEMP=<public> -env TMP=<public> -env CLAUDUCT_EVIDENCE_LIVE=1 -env CLAUDUCT_EVIDENCE_TUI=1 \
  -- <repo>\.tmp\v032-tui\clauduct-tui.test.exe "-test.run=^TestRuntimeEvidenceNativeTUI$" -test.v
```
