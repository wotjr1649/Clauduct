# TCP 반닫기 문제와 셸 변경 판정

현재 PC에서는 CMD 전환으로 문제가 해결되지 않는다. 동일 Windows Node 실행 파일·동일 공개 검사·동일 최소 환경 변수로 PowerShell 직접 실행과 cmd.exe 실행을 비교했고, 두 경로 모두 일반 연결은 정상1바이트, 반닫기는4ms에0바이트 EOF였다. 원인 표기는 **현재 Windows 환경에서 재현되는 셸과 무관한 TCP 반닫기 이상 — 구성요소 미확정**이다. 사용자의 설정 실수나 Windows 전체의 공통 결함으로 단정하지 않는다.

사용자가 이 문제를 문서에 남기고 다음 작업으로 진행할 수 있다는 방향을 제시했다. 추가 OS/셸 비교는 보류하고 독립 출하 작업을 진행한다. 이 결정은 TCP 검사의 FAIL을 PASS로 바꾸거나 관련 HTTP 회귀 전체가 환경 탓이라고 확정하는 것이 아니다.

## 관측

검사 소스는 Clauduct의 HTTP 서버를 사용하지 않고 Node 내장 net의 loopback에 공개1바이트를 보내는 별도 fixture다. 서버와 클라이언트의 allowHalfOpen은 true이며 서버는 요청 후100ms에 응답을 쓰고400ms에 송신을 종료한다. 사례별1800ms, 프로세스별10000ms 한도다. Node `24.19.0`·Windows 빌드 `26200` 환경이며 새 설치·인증·외부 요청은 없다.

| 실행 경로 | 일반 연결 | 반닫기(sendFin=true) | 정리 |
|---|---|---|---|
| PowerShell이 Node를 직접 시작 | 응답1바이트 | 4ms에EOF·응답0바이트 | 잔여 소켓0·프로세스 exit0 |
| cmd.exe /c가 같은 Node를 시작 | 응답1바이트 | 4ms에EOF·응답0바이트 | 잔여 소켓0·프로세스 exit0 |

exit0은 검사 프로세스의 수집·정리가 완료됐다는 뜻이며, 반닫기 기능 검사는 두 경로 모두 FAIL이다. PowerShell에서 CMD를 자식으로 실행한 비교이고 별도 사용자 CMD 창에서 실행한 결과는 아니다. 기존 사용자 일반 PowerShell의 .NET 검사도 같은 반닫기 유실을 보였으므로 에이전트 경로만의 현상이라는 해석은 맞지 않는다.

두 실행은 같은 node.exe SHA256 `3602f2bb1a10f2cbab4c36886218a33c1ab3db87290e73b033c46c77147d0237`과 같은 검사 SHA256 `55f915a1d9160bca6003ed74d512362451b255528a3e0c91afc69f15af7cce34`를 사용했다. 사용자/시스템 CMD AutoRun 설정은 존재 여부만 읽어 미설정을 확인했다. AutoRun·실행 정책을 끄는 옵션은 사용하지 않았다.

증거는 `.tmp/release-completion-20260914/shell-comparison-result.json`이다. 첫 실행은 wrapper가 출력 저장 전에 실패하여 원래 하위 오류를 관측하지 못했다(`shell-comparison-first-failure.json`). 수집기를 보완한 비교는 TCP 증상을 재현했지만 close 이벤트 전 잔여 소켓 집계 때문에 exit1이었다(`shell-comparison-baseline.json`). 명시적인 모든 close 이벤트를 기다리도록 fixture를 수정한 뒤 위 최종 결과를 얻었다. assertion이나 응답 대기를 완화하지 않았고 제품 코드는 변경하지 않았다.

## 판단 범위

- PowerShell 실행 정책: 구형 Windows PowerShell 스크립트를 로드하지 못한 별도 문제다. 이미 실행된 Node/.NET의 TCP EOF 현상을 설명하지 않는다. 거부된 구형 스크립트는 이번 CMD 비교로 다시 실행하지 않았다.
- CMD: 같은 Node에 대해 실측상 해결되지 않았다. 현재 clauduct.cmd도 실제 작업을 Node에 넘기므로 실행 셸 교체만으로 통신 구현이 바뀌지 않는다.
- Git Bash 등에서 같은 Windows node.exe를 실행: 동일한 Windows 실행 파일/통신 계층을 사용하므로 해결을 기대할 근거가 없다. 이 셸 자체는 실측하지 않았다.
- WSL2의 Linux Node: Linux 커널을 사용하는 별도 실행 환경이다. 단순 셸 변경과 다르며 현재 Windows용 launcher·native 실행·인증 경로를 그대로 지원한다고 주장할 수 없다. 설치·전환·실행은 하지 않았고 해결 여부도 미검증이다.
- PC 설정과 OS: 여러 런타임과 IPv4/IPv6에서 같은 현상이라는 증거는 공통 통신 계층 쪽 원인을 의심하게 하지만, OS 구현·업데이트·네트워크 필터 중 하나를 특정하지 못한다. 사용자 모드 모듈 점검만으로 커널 필터를 배제할 수도 없다. 다른 깨끗한 Windows 환경의 비교나 해당 연결에 한정한 계층별 관측 없이는 Windows 전반의 결함으로 일반화하지 않는다.

[Microsoft shutdown 문서](https://learn.microsoft.com/en-us/windows/win32/api/winsock/nf-winsock-shutdown)는 SD_SEND가 송신만 끝내고 소켓을 닫지는 않는다고 설명한다. [WFP 구조](https://learn.microsoft.com/en-us/windows/win32/fwp/windows-filtering-platform-architecture-overview)는 통신 필터에 사용자 모드와 커널 모드 구성요소가 있음을 설명하며, 특정 필터가 이번 원인이라는 증거는 아니다. [WSL 비교 문서](https://learn.microsoft.com/en-us/windows/wsl/compare-versions)는 WSL2의 실제 Linux 커널 사용을 설명한다.

## 실제 Clauduct 사용에 미치는 영향

사용자가 실제 사용에 큰 문제가 되는지 물었고, 영향이 있으면 문서에 남기고 다음 출하 작업을 계속하도록 요청했다. 판정은 **일반 사용이 항상 실패하는 상태라는 증거는 없지만, 특정 통신 종료 조건에서 작업을 끊을 수 있는 중요한 안정성 제약**이다. 발생 빈도와 영향받는 사용 비율은 측정하지 못했다. 추가 OS 진단은 보류한다.

일반 HTTP 요청의 본문 완료와 TCP 송신 방향의 종료는 다르다. HTTP는 Content-Length나 chunked framing으로 본문 끝을 표현하므로, 요청 뒤 즉시 TCP FIN을 보내는 공개 반닫기 probe가 모든 정상 요청을 대표하지는 않는다. 이는 [RFC 9112의 본문 길이 규칙](https://www.rfc-editor.org/rfc/rfc9112.html#name-message-body-length)과 현재 HTTP 구현의 구분이다. 기존 실제 모델 개발·text/stream-json의 성공, native 자식 취소4사례 및 후속 신규 Workflow 실행의 성공도 정상 사용 경로가 존재한다는 증거다. 장기간 안정성이나 낮은 장애율의 증명은 아니다.

| 영향 | 관측과 실제 사용에서의 의미 |
|---|---|
| 응답 유실 | 공개 TCP/HTTP 반닫기 검사에서 응답0바이트를 확인했다. 같은 종료 조건에 들어가면 답변·도구 결과를 받지 못하고 해당 턴이 실패할 수 있다 |
| 종료 지연 | 현재 종료 처리의1000ms fallback이 공개 HTTP 비교3회 중2회 작동했고 전체 응답은 수신했다. 이 경우 연결 정리가 약1초 늦어질 수 있다. 지속적인 지연 빈도는 미측정이다 |
| 리셋·자식 시작 실패 | 과거 HTTP/native 동시성 검사에 ECONNRESET이 있었다. 자식 등록 응답이 실패하면 해당 자식의 라우팅 검증/시작이 실패할 수 있다. TCP 반닫기가 그 모든 실패의 원인이라고 확정하지 않았다. 철회한 FIN 실험에서의 등록 실패도 현재 코드의 실패율로 세지 않는다 |
| 무인 작업의 중단·복구 | 응답 단절은 자동 작업 진행을 멈추게 할 수 있다. 코드의 전달 후 자동 재시도 금지와 실패 기록은 유지한다. 이미 완료된 파일/도구 효과가 있을 수 있으므로 응답 부재를 작업 미실행으로 가정할 수 없다 |
| 파일·결과의 무결성 | 이번 TCP 문제로 사용자 파일이 손상됐다는 관측은 없다. 임의 Bash/MCP 효과의 중복·유실이 모두 방지된다는 보증도 없다. 기존 효과 대조와 독립 완료 판정을 별도로 유지한다 |

사용 중 오류가 나면 native is_error·Clauduct requestOutcome/failureHistory/cleanup과 실제 산출물을 함께 확인해야 한다. exit0 또는 일부 답변만으로 전체 작업 완료를 판단하지 않는다. 같은 세션을 재개하더라도 이미 발생한 효과의 확인이 선행되어야 한다. 정상 HTTP 일부 성공이나 이번 Workflow 수정의 PASS로 기존 통신 FAIL을 덮지 않는다.

## 후속 작업

TCP/셸 변경 조사는 보류하고 Workflow 재개 계약을 다음 대상으로 삼았다. 현재 코드의 agent-route.bindingFrom과 agent-selection.remember는 scriptPath 또는 resumeFromRunId 입력을 라우팅 근거로 수용하지 않는다. 기존 native scriptPath 재개 거부는 그보다 앞선 별개 경계다. native의 접근 거부를 우회하지 않고, 필요한 재개 신원·이전 run 연결과 현재 제품의 라우팅 공백을 구분하여 다음 구현 범위를 정한다. 기존7파일 회귀와 자식 취소4사례의 PASS는 유지하고 전체 출하 판정은 아직 HOLD다.

후속 조사에서는 이전 Workflow 스크립트 편집이 별개의 신규 Workflow 자식까지 막는 결함을 재현·수정했다. 새 journal10사례와 기존20사례, 관련 선택 회귀 및 실제 native sol/low·luna/max의 신규 Workflow 두 개 연속 실행이 통과했다. 이 수정은 TCP 문제 해결이나 cache-miss resume 성공의 증거가 아니다. 세부 결과는 [현재 출하 기록](release-completion-2026-09-14.md)에 있다.

## 원인 확정(2026-09-14 후속): loopback HTTP 필터

위 "구성요소 미확정"은 해소됐다. 원인은 이 PC에 설치된 AdGuard의 loopback 필터이며, OS나 셸 문제가 아니다. 같은 반닫기 fixture를 보호 on/off로 비교했다.

| 보호 | IPv4 반닫기 | IPv6 반닫기 |
|---|---|---|
| on | 9ms에EOF·응답0바이트 | 2ms에EOF·응답0바이트 |
| off | 405ms·응답1바이트 | 405ms·응답1바이트 |

일반 연결은 양쪽 모두1바이트로 정상이며, 차이는 반닫기에서만 나타난다. 보호를 끈 상태에서 `test-http-close`는 roundtrip20·`halfClosedClientReply` true·pending0으로 통과했고, `test-chat`은28/28 통과했다. 같은 필터가 비표준 `Expect` header를 제거하고 잘못된 method에 자체400 page를 반환하는 것도 확인했다. 근거와 판별법은 [native 시작 진단](../reference/native-startup-diagnostics-2026-09-14.md)에 있다.

실사용 경로는 이 필터가 켜져 있어도 영향이 없다. 필터를 켠 상태에서 chunked SSE stream8/8 frame이200~207ms 간격으로 변형 없이 도착했고, heartbeat 주기를 넘기는16초 간격 유휴 stream도3/3 도착에 socket error 없이 종료했다. 필터가 건드리는 것은 기형 HTTP 요청과 TCP 반닫기다.

`close()`의 산발적 약1000ms 지연도 같은 원인이다. `http-close.mjs`의 peer-drain deadline이 필터 때문에 지연된 FIN을 기다리다 발화하며, 보호를 끄면 발화가0/10으로 사라진다. 이 항목들은 제품 결함이 아니다. 다만 다른 머신의 보안·백신 제품에서 같은 계열 간섭이 재발할 수 있으므로 위험 자체가 사라졌다고 보지는 않는다.
