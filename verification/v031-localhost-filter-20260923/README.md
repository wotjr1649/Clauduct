# AdGuard localhost 필터링과 즉시 FIN 응답 유실

2026-09-23. v0.3.1 후보 `ad01745`에 남은 즉시 TCP FIN 응답 유실
([내장 필터 검증](../v031-httpguard-20260923/REPORT.md)의 raw FIN 진단)이 AdGuard for Windows의
[로컬 호스트 필터링](https://adguard.com/kb/ko/adguard-for-windows/settings/app-settings/advanced-settings/)
설정에 따라 달라지는지 확인했다. 제품 소스는 바꾸지 않았다.

환경은 Windows 11 Pro 26200, AdGuard for Windows 8.0.5570(`Adguard Service` Running), Go 1.27.1이다.
AdGuard 전체 보호는 계속 켜져 있었다. 설정은 사용자가 AdGuard 화면에서 바꿨고, 각 단계의 상태는
사용자가 제공한 설정 화면으로 확인했다. 검사 실행기는 AdGuard 설정을 읽거나 바꾸지 않는다.

## 결과

| 시각(UTC) | localhost 필터링 | `claude.exe` 앱별 라우팅·필터링 | 검사 | 결과 |
|---|---|---|---|---|
| 09-22 22:46 | 켬 | 끔(앱별 제외) | [반닫기 probe](half-close-probe.cjs) 20회 | 응답 0/20 |
| 09-22 22:51 | 끔 | 끔 | 같은 probe | 응답 20/20 |
| 09-23 00:01 | 끔 | 켬 | [실제 backend 1회](live-backend.txt) | PASS |
| 09-23 00:30 | 끔 | 켬 | 같은 probe | 응답 20/20 |
| 09-23 00:31 | 끔 | 켬 | `TestRuntimeEvidenceImmediateFINReply` 20회 | [20/20 PASS](immediate-fin-after.txt) |

00:30과 00:31 행의 설정은 23:51에 마지막으로 보고된 상태이며, 그 뒤 따로 확인하지 않았다.
probe 출력은 [half-close-probe.txt](half-close-probe.txt)에 있다. probe와 Go 진단은 `node.exe`와 Go 테스트
실행 파일이 연결하므로 `claude.exe` 앱별 제외의 영향을 받지 않는다. 마지막 행의 진단은 localhost
필터링이 켜진 상태에서 [20/20 FAIL](../v031-httpguard-20260923/raw-fin-diagnostic-retained.txt)이었던
것과 같은 tracked 검사다. 실행은
`CLAUDUCT_SOCKET_EVIDENCE=1 go test -tags runtime_evidence -count=1 -v -run '^TestRuntimeEvidenceImmediateFINReply$' ./internal/gateway`,
`CGO_ENABLED=0`이다.

실제 backend 검사는 [임시 검사](adguard_live_manual_test.go)로 `gpt-5.6-luna`/`low`를 ledger 한도
1회로 보냈다. backend 시도 1회, 지정 문구 일치, native exit 0, 정리 오류 없음, gateway 거부·끊김 0이다.
첫 구성(`--bare`)은 SessionStart hook이 실행되지 않아 gateway가 `CONTEXT_SESSION_UNVERIFIED`로
거부했고 backend 시도는 0회였다. 제품 검증을 끄지 않고 실제 hook을 쓰도록 검사 구성을 고친 뒤 통과했다.

## 출처

00:01까지의 행은 이전 Codex 세션의 도구 출력에서 옮겼다. [half-close-probe.cjs](half-close-probe.cjs)는
그 세션이 실행한 스크립트와 바이트 단위로 같고, 켬·끔 두 실행의 스크립트도 서로 같다.
[임시 검사](adguard_live_manual_test.go)는 그 세션의 patch를 순서대로 다시 적용해 복원했다.
복원본의 로그 행 번호(78)는 기록된 출력의 행 번호와 일치한다. 00:30과 00:31 행은 이 기록을
작성하면서 직접 다시 실행했다.

## 판정 범위

- 이 PC와 AdGuard 8.0.5570의 결과다. 다른 필터 제품이나 버전으로 확장하지 않는다.
- localhost 필터링을 켠 상태에서는 실제 backend 요청을 비교하지 않았다. 켠 상태의 영향은
  위 raw FIN 진단과 [내장 필터 검증](../v031-httpguard-20260923/REPORT.md)을 따른다.
- 설치 바이너리, TUI, 장기 사용 안정성은 이 기록의 범위가 아니다.
