# v0.3.1 정책 명문화·종료 순서 계측

2026-09-22, `fix/v031`, 기준 `149068e`. 기존 미커밋 작업을 보존했다.
사용자가 켠 AdGuard 보호 On 상태를 유지했으며 설정·서비스·driver·설치본 변경은 0건이다.
이번 실행의 보호 상태 근거는 사용자의 직접 전환 통지이며 UI 값은 다시 읽지 못했다.

**정책 반영과 실제 backend TUI 재검증은 PASS. 요청한 전송 검사 전체는 HOLD다.**
전송 계층의 half-close 오동작을 직접 Winsock으로 재현했다. 후속 [커널 분석](KERNEL-ANALYSIS.md)에서
상대가 열려 있는데 송신 종료한 소켓에 수신 FIN이 주입되는 실행 경로와 외부 driver의 API 호출
위치를 확인했다. 내부 결함 명령·런타임 인자와 수정 패치는 아직 미확정이다. 전체 Success가 아니다.

## 채택 정책 반영

[ARCHITECTURE.md](../../docs/v2/ARCHITECTURE.md)의 7.1절에 일반 생성·압축의 원격 사전 계수
금지, 실제 backend usage 기록, 미확보 usage의 unknown 처리, 예방 추정과 정확 계수의 구분을
명시했다. [S46 채택 정책](../release-repair-20260920/POLICY-REVIEW.md)을 상세 근거로 연결했다.
tool-output 이미지/PDF warmup 불일치는 기존 별도 명시적 계수의 미지원 범위이며 일반
생성·압축의 필수 미해결 결함이 아니다. 이전의 잘못된 분류를 다시 완료 조건으로 올리지 않는다.
압축 후 이전 문맥 usage를 최소값으로 이월하지 않는 #50 결정도 명시했다.

사전 계수 0회, counter 부재와 무관한 생성, base64 계수 금지, 불일치 후 정상 응답·실측 캐시,
미확보 usage에 관한 기존 회귀 검사 5개를 다시 실행하여 모두 PASS했다(0.112초).
제품 동작은 이미 채택 정책과 일치하므로 이번 정책 반영을 위해 제품 코드를 바꾸지 않았다.

## 최소 재현의 종료 순서

[raw TCP 검사](../../go/internal/gateway/socket_runtime_evidence_test.go)에 고정 이벤트명,
상대 시각, byte 수, 오류 분류만 기록했다. 실패 유형별 첫 3건만 출력한다. 원래 두 모드의
각 200회, payload 일치, client/server 오류 판정, 1초 I/O deadline은 보존했다.
서버의 결과 채널 전달이 deferred Close보다 앞서던 검수 수명주기는 별도 완료 채널을 기다리도록
보완했다. 이 보완은 다음 반복 전에 서버 정리를 확인하며 reset을 해결하지는 않았다.
동일 clock tick의 서로 다른 goroutine 이벤트 간 정밀 순서는 주장하지 않는다.

첫 계측은 즉시 종료 14/200, 수신 ACK 후 종료 2/200에서 실패했다.

- 서버 `Write(45056)`와 `Close`가 모두 성공한 뒤 client는 0 bytes와 `errno_10054`를 받았다.
- client가 전체 payload를 받은 뒤 `Write(1)`와 `Close`를 성공했지만 server는 ACK 대신
  `errno_10054`를 받았다. 이 방향의 payload 손실은 없었다.

별도 `TestRuntimeEvidenceSocketShutdown` 대조군은 서버와 client의 송신 종료를 명시하고
양쪽 EOF까지 요구한다. 원래 두 검사를 대체하지 않는다. 200/200 실패했으며 서버의 ACK
읽기가 모두 조기 EOF였다. 단순 CloseWrite·drain 추가로 해결되지 않는 근거다.

이를 애플리케이션 경쟁과 분리하려고 [직접 Winsock 재현기](../socket-half-close.c)를 만들었다.
단일 스레드에서 loopback 연결을 만들고 서버가 공개 6-byte 문자열을 쓴다. client는 결과를
기록할 때까지 `send`, `shutdown`, `closesocket`을 한 번도 호출하지 않는다.

| 직접 Winsock 대조 | 서버 recv 결과 | 관측 시간 |
|---|---|---:|
| 송신 종료 없음 | `SOCKET_ERROR / WSAETIMEDOUT(10060)`, 예상대로 수신 방향 유지 | 512,180µs |
| 서버 `shutdown(SD_SEND)`만 실행 | `0 / EOF`, 상대는 여전히 열려 있음 — **FAIL** | 4,451µs |

Go·Node·.NET·Clauduct 없이도 송신 전용 종료가 수신 EOF로 돌아왔다. 따라서 이 반례는
제품의 goroutine·HTTP 종료 코드에서 발생한 것이 아니다. Microsoft 계약에서 `SD_SEND`는
송신 방향을 종료하며, 수신 방향을 함께 종료하는 `SD_BOTH`와 다르다.
[shutdown 공식 계약](https://learn.microsoft.com/en-us/windows/win32/api/winsock/nf-winsock-shutdown).

이 직접 재현과 [이전 동일 코드 On/Off 대조](../v031-recheck-20260922/REPORT.md)는 보호가
활성화된 전송 경로의 종료 간섭을 지지한다. 특정 AdGuard 내부 함수·WFP callback이나 원래
즉시 종료에서 RST를 처음 만든 위치까지 확정한 것은 아니다. 이 단계 당시에는 packet/driver
trace를 실행하지 않았다. 이후 ETW로 확인한 범위는 위 커널 분석에 분리했다.
이 구분 없이 “AdGuard 내부 원인 수정 완료”로 표현해서는 안 된다.

재현기는 기존 `C:\msys64\ucrt64\bin\gcc.exe`로 `-Wall -Wextra -Werror`와 `-lws2_32`를
사용해 빌드했다. 새 설치·의존성·제품 runtime 추가는 없다. 외부 요청과 credential 읽기는 0건이다.

### 외부 구성요소 후속 확인

후속 요청에서 현재 서비스·loaded module·driver 파일을 읽기 전용으로 확인했다.
Windows 11 Pro build 26200이며 서비스 `8.0.5570`, `Adguard.Core.dll` `1.22.29`,
loaded `AdGuardNetLibWfp.dll`과 설치된 `adgnetworkwfpdrv.sys`는 `8.0.91.0`이다.
Core DLL과 driver의 Authenticode 판정은 Valid였다. driver 파일 버전이 현재 커널에 로드된
바이트의 버전을 직접 측정한 값이라고 주장하지 않는다.

[최신 안정판 공식 asset](https://github.com/AdguardTeam/AdguardForWindows/releases/expanded_assets/v8.0.1)은
`AdGuard-8.0.1.5570.exe`다. 현재 설치 빌드 번호와 일치하지만 파일 전체의 동일성을 비교한
것은 아니다. [공식 nightly 목록](https://adguard.com/en/versions/windows/nightly.html)에는
2026-09-21의 `8.1.0 Nightly 4 (5673)`가 있다. 해당 build가 이 결함을 수정했다는 근거는
확보하지 못했다. 패키지 다운로드·실행·설치·서비스 재시작은 하지 않았다.

Winsock 재현기에 읽기 전용 `GetExtendedTcpTable` 관측을 추가하고 `-liphlpapi`로 링크했다.
프로브의 loopback 포트와 관련된 행만 출력하며 host TCP table 전체를 기록하지 않는다.
빌드 경고 0으로 실행됐고 송신 종료 없는 대조는 513,160µs 뒤 예상 timeout이었다.
송신 전용 종료는 2,476µs 뒤 EOF였으며, 종료 전 양쪽 ESTABLISHED에서 관측 후 서버
LAST_ACK / client ESTABLISHED로 바뀌었다. 두 행의 PID는 모두 재현 프로세스였다.
[관측 stdout](component-snapshot.txt). 별도 user-space 중계 프로세스 소유 연결은 이 관측에서
나타나지 않았다. 이 snapshot만으로 특정 WFP callback을 식별할 수는 없다.

[공식 저장소](https://github.com/AdguardTeam/AdguardForWindows/releases/tag/v8.0.1)는 제품이
오픈 소스가 아니며 GitHub를 공개 bug tracker로 사용한다고 명시한다. 검토된 수정 패치나
공식 해결 build가 없는 현재 상태에서는 외부 내부 코드를 수정 완료했다고 할 수 없다.
[공개 합성 이슈 본문](UPSTREAM-ISSUE.md)은 사용자 승인 후 공식
[AdGuard 이슈 #6233](https://github.com/AdguardTeam/AdguardForWindows/issues/6233)으로 게시했다.
작성자는 `wotjr1649`, 생성 시각은 `2026-09-22T01:47:07Z`다. GitHub API로 본문이 승인된
로컬 파일과 일치함을 확인했다(줄바꿈 정규화·끝 공백 제외). 게시 직후 상태는 open,
댓글은 0건이었다. 개인 경로·인증 정보·프로젝트 데이터는 포함하지 않았다.
공식 수정 빌드 또는 이 합성 연결에 한정된 진단 절차를 요청했으며, 수정 답변은 아직 없다.
원래 세 검사의 마지막 결과는 아래에 보존했다. 외부 구성요소가 바뀌지
않았고 최소 재현이 여전히 실패하므로 전체 검사와 실제 backend를 근거 없이 반복하지 않았다.

## 보호 On 재검증

Node와 .NET은 기존 standalone 파일 그대로 실행했다. 두 파일의 hash는 이전 대조와 같다.
raw TCP는 종료 관측만 보완했고 원래 400개를 별도로 실행했다. 실패 후 재시도하지 않았다.
[계측 stdout](socket-trace.txt)은 당시 줄 번호를 포함한 원래 출력이며, 이후 별도 shutdown
검사 이름을 분리하면서 이동한 현재 코드의 줄 인용이 아니다.

| 검사 | 이번 결과 |
|---|---|
| Node 31개 | **FAIL**. 5개 통과 후 `missing-fixed` / `node-fetch`에서 `ECONNRESET`. 요청 6건, 잘못된 요청 0건. 나머지 25개는 fail-fast로 미실행 |
| .NET 35개 | **FAIL**. 2개 통과 후 `mixed-case`에서 `NETWORK_OR_TLS_ERROR`, 33ms. 나머지 32개는 미실행이며 이번 45초 deadline 사례도 미실행 |
| raw TCP 즉시 종료 | **FAIL**, 15/200. client `errno_10054`와 payload 불일치 15건 |
| raw TCP 수신 ACK 후 종료 | **FAIL**, 7/200. server `errno_10054` 7건, payload 불일치 0건 |
| 실제 native TUI + 구독 backend | **PASS**, 211.99초, 실제 호출 5회 |

실제 TUI는 Claude Code 2.1.278과 `gpt-5.6-luna`/`low`를 사용했다. 승인된 임시 검증 루트
아래 새 profile·공개 프로젝트만 신뢰 등록했다. 같은 세션에서 `PUBLIC_TUI_47`, 압축 경계,
`PUBLIC_TUI_47 blue 47 PUBLIC_COMPACT_DONE`, 실제 text delta 이후 Esc 중단,
`PUBLIC_TUI_RECOVERED`를 기존 엄격한 검수기가 확인했다. 생성 완료 3회·압축 완료 1회·
생성 취소 1회·exit 0·cleanup 성공이다. 검증 route 밖 보조 요청 4건의 거부는 그대로 기록했다.
이번 실제 호출 5회는 [이전 원장](../v031-recheck-20260922/evidence.json)의 22회에 더해 기록하며,
이 둘의 합계를 프로젝트 전체 사용량으로 표현하지 않는다. 이번 warmup 비교 호출은 0회다.

## 남은 완료 조건

보호 On에서 원래 Node 31개·.NET 35개·raw TCP 400개를 모두 통과하려면, 외부 전송 계층의
종료 처리 결함이 해결된 뒤 같은 검사로 확인해야 한다. 직접 Winsock 재현기는 그 수정의
독립 판정 기준으로 사용할 수 있다. 확인된 해결 버전이나 검증된 로컬 제품 패치는 없다.
AdGuard 예외·보호 해제·timeout 확대·재시도·실패 삼키기·판정 완화는 넣지 않았다.
외부 bug report는 승인 후 #6233으로 게시했다. 서비스·driver 업데이트는 하지 않았다.

이번 검수 수명주기 보완과 정책 문서 반영을 외부 결함 수정으로 세지 않는다. 실제 TUI의
성공도 독립 전송 실패를 지우지 않는다. 최종 문서·diff·정리 검사는 [기계 기록](evidence.json)을 따른다.
