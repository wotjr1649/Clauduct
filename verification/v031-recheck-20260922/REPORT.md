# v0.3.1 실제 backend 재검증과 보호 on/off 대조

2026-09-22, `fix/v031`, 기준 `149068e`. 기존 미커밋 개발 작업을 보존했다.
사용자가 실제 구독 backend 재검증을 승인했다. 이번 재검증의 예약·전송 시도는
22회(생성 19회, warmup 3회)이며, 이전 [통합 원장](../v031-integration-20260921/evidence.json)의
40회와 [무과금 검사](../v031-offline-20260922/REPORT.md)의 실패 이력은 유지한다.

**실제 backend TUI와 보호 Off의 전송 검사는 PASS다. 보호 On을 포함한 요청된 전송 검사 전체 판정은 HOLD다.**
보호 On의 독립 전송 반례가 남는다. tool-output 이미지 warmup 불일치는 기존 미지원 범위의
별도 비교 결과이며, 현행 일반 생성·압축의 합격을 막는 제품 결함으로 분류하지 않는다.

**판정 정정:** 최초 보고에서 이 warmup 불일치를 필수 미해결 항목으로 함께 올린 것은
분류 오류였다. [S46에서 채택한 실측 usage 정책](../release-repair-20260920/REPORT.md)에 따라
일반 생성·압축은 원격 사전 계수 없이 backend usage를 사용한다. 아래 raw 비교의 FAIL은
보존하되, 그 비교가 현재 제품 경로의 필수 선행 조건이라는 해석을 바로잡았다.

## 실제 backend 결과

| 검사 | 결과 | 실제 시도 |
|---|---|---:|
| 자동 압축 effort | **PASS**, 9.07초. high → medium → high, 필수 사실 보존, 압축 계수 295 = usage 295 | 4 |
| 동일 JPEG 3개, user content | **PASS**, 사전 계수 3,497 = 생성 usage 3,497 | 2 |
| 동일 JPEG 3개, tool output | **FAIL**, 사전 계수 2,828 ≠ 생성 usage 3,525 | 2 |
| 최초 TUI | **FAIL**, 301.37초. 입력 제출 지연·취소용 긴 목록에 대한 짧은 답변 후 검수 deadline 소진 | 4 |
| 입력·스트림 관측 보완 후 TUI | **PASS**, 174.14초. 이 실행 중 보호 상태는 별도 샘플링하지 않음 | 5 |
| 사용자가 보호를 켠 뒤 TUI | **PASS**, 129.08초 | 5 |

모델은 `gpt-5.6-luna`이며, 자동 압축 비교 외에는 `low`다. 각 실행은 기존 route·요청 상한,
runtime/home 검사, 인증서 검증과 고정 backend 목적지를 사용했다. 계정 갱신이나 인증 설정은
변경하지 않았다. 과금 금액은 측정하지 않았으며 구독 사용량과 요청 횟수만 구분한다.

두 성공 TUI 모두 실제 Claude Code 2.1.278, 실제 PTY, 제품 gateway·hook, 실제 구독 backend를
사용했다. assistant의 `PUBLIC_TUI_47`, native 압축 경계, `PUBLIC_TUI_47 blue 47 PUBLIC_COMPACT_DONE`,
생성 중단 기록, `PUBLIC_TUI_RECOVERED`의 같은 세션 순서를 엄격한 기존 검수기로 확인했다.
일반 생성 완료 3회·압축 완료 1회·생성 취소 1회, exit 0·정리 성공을 각각 요구했다.
검증용 Luna/low 범위 밖 보조 요청 4건의 `ROUTE_NOT_AUTHORISED`도 각 성공 실행에 남아 있다.
이 거부를 실제 보조 모델 호출 성공으로 세지 않는다.

## 취소 관측 보완

[실호출 검수기](../../go/internal/app/integration_runtime_evidence_test.go)는 완전한
`response.output_text.delta` JSON 이벤트를 처음 받은 때에 고정 메시지
`backend text_stream_started`를 한 번 출력한다. 원문·token·영수증 값은 이 진단에 포함하지 않는다.
이 관측 후 실제 Esc를 보내므로, 이미 완료된 짧은 응답을 취소했다고 오인하지 않는다.
TUI 입력 제출에는 이 터미널에서 동작을 확인한 CSI-u Enter(`ESC [ 13 ; 1 u`)를 사용했다.

reasoning 내용 안의 유사 문자열, 조각난 이벤트, 중복 delta에 대한 검사가 통과했다.
관측 호출을 제거한 overlay는 실제 assertion으로 실패했다. 제품 전송과 취소 판정은 변경하지 않았다.
초기 PowerShell의 인자 전달 오류는 검수 시작 전 실패로 별도 구분하며 backend 시도는 0회다.

## 동일 코드의 AdGuard 보호 대조

사용자의 최종 범위는 **전체 보호 켜기·끄기만**이다. `Filter localhost`, HTTPS 필터,
DNS, 방화벽, 앱 제외, driver 선택 등 다른 설정을 바꾸지 않았다.

대조 직전 실제 UI에서 전역 `Protection` 값 `Off`를 읽었다. 보호를 켜려는 UI Automation
`SetValue`는 지원되지 않아 실패했고, 직후 값은 `Off`였다. 이후 사용자가 직접 보호를 켰다고
알렸고 On 대조를 실행했다. 에이전트가 적용한 설정 변경은 0건이며 사용자가 마지막에 고른
On 상태를 유지했다. `AdguardSvc.exe`는 8.0.5570이고 `adgnetworkwfpdrv`가 Running이었다.

| 동일 검사 | 보호 Off | 사용자가 보호 On으로 전환한 뒤 |
|---|---|---|
| Node HTTP/fetch | **31/31 PASS**, helper 5/5, 수신 31건 | **FAIL**. 9개 통과 후 `empty`/`node-fetch`에서 `ECONNRESET`, 수신 10건·잘못된 요청 0건 |
| .NET HTTP | **35/35 PASS**, parser 8/8·self-test 17/17, 요청·TCP 연결 35건, 남은 socket 0 | **FAIL**. 첫 2개 통과 후 `mixed-case`에서 `NETWORK_OR_TLS_ERROR`, 사례 34ms |
| raw TCP 즉시 종료 | **200/200 PASS** | 실패 32/200, client `errno_10054` 32건, payload 불일치 31건 |
| raw TCP 수신 ACK 후 종료 | **200/200 PASS** | 실패 17/200, server `errno_10054` 17건, payload 불일치 0건 |

.NET Off 실행은 실제 45초 timeout도 45,020ms로 통과했다. 모든 전송 검사는 합성 데이터·
`127.0.0.1`만 사용했고 외부 요청·자격 정보 읽기는 0건이다. Node On 실패 후 libuv의
`UV_HANDLE_CLOSING` assertion도 출력됐다. reset과 별도로 원문 이력을 보존하며,
이 추가 native assertion의 libuv 내부 원인까지 확인했다고 주장하지 않는다.

Off에서 원래 검사가 끝까지 통과했으므로 fail-fast 구조를 바꾸지 않았다. 세 검사 파일의
당시 SHA256은 [기계 기록](evidence.json)에 있다. 당시에는 두 조건 사이 코드·timeout·판정이
동일하다고 기록했다. **2026-09-22 후속 감사:** Node·.NET 파일은 기록 해시와 일치하지만,
raw TCP 파일은 후속 계측을 거친 현재 파일과 다르고 당시 해시의 원본은 복구하지 못했다.
따라서 raw TCP의 당시 동일 판본 주장은 현재 산출물만으로 독립 재현할 수 없다.
원래 hash와 On/Off 결과는 보존한다. 현재 제품 합격 근거는 별도로 해시를 고정한 후속 수리
묶음이며, 이번 제한과 대조 결과는
`verification/v031-review-fixes-20260922/batch-03/historical-provenance.json`에 기록했다.
실패를 줄이는 재시도·linger·추가 drain·필터 제외를 제품이나 검사에 넣지 않았다.

Clauduct를 사용하지 않는 Node/.NET/Go 표준 전송 경로 모두 Off에서 통과하고 On에서
실패했다. 이는 이 머신의 보호 활성화와 연결 종료 간섭을 분리한 근거다. 특정 WFP callback,
packet 폐기 위치나 다른 사용자 머신의 보안 제품 동작까지 검증한 것은 아니다.
실제 Clauduct TUI는 On에서도 통과했으므로, 독립 반례를 모든 제품 요청의 실패로 일반화하지 않는다.

AdGuard 공식 문서는 localhost filtering과 redirect driver mode를 각각 설명한다.
이번에는 사용자의 범위에 따라 이 설정들을 변경하지 않았다.
[AdGuard 고급 설정](https://adguard.com/kb/adguard-for-windows/settings/app-settings/advanced-settings/).

## 남은 경계와 최종 확인

- **기존 지원 경계:** tool-output 이미지 warmup은 이번에도 697 tokens 차이가 났다.
  일반 생성·압축은 이 계수를 호출하지 않고 backend usage를 기록한다. 별도 명시적
  `count_tokens`에서 해당 warmup은 미지원이며, 동일 입력의 실제 usage 캐시는 사용할 수 있다.
  새 입력의 정확 계수 지원을 추가하는 작업은 현재 정책의 완료 조건이 아니다.
  사전 계수 0회·실측 usage 누계·선택적 계수 불일치 후 정상 답변·실측 캐시·미확보 usage의
  unknown 처리에 관한 기존 검사 5개를 다시 실행하여 PASS했다(0.171초).
- 이 머신의 보호 On 독립 전송 검사는 실패 상태다. On에서도 모든 소켓 종료를 보장하려면
  해당 필터/런타임의 문제 해결과 동일 재현기의 재검증이 필요하다. 특정 보안 제품에 의존하는
  제품 패치를 넣지 않았다.
- #50의 미관측 조건·#52 junction 경계·v0.3.0 새 journal 거부 및 기존 V1 probe skip은
  앞선 기록의 별도 범위로 유지한다.

| 변경 표면 검증 | 결과 |
|---|---|
| 스트림 시작 관측 검사 | PASS, 0.170초 |
| 관측 호출 제거 overlay | 예상 assertion FAIL, 0.127초 |
| 관측 검사와 TUI 판정 대조군 `-race` | PASS, 1.202초 |
| tagged `go vet ./internal/app`, TUI 검수 바이너리 빌드 | PASS |
| 문서 인용·JSON·diff | PASS. 문서 124개·인용 89개·로컬 링크 691개, 실패 0 |

제품 코드는 이번 재검증에서 바꾸지 않았고 전체 Go race 검사는 앞선 무과금 기록의 PASS를
유지한다. 이번 변경은 opt-in 실호출 검수기의 관측과 문서다. 최종 문서 인용·JSON·diff 확인은
기계 기록에 남겼다. 실제 TUI의 임시 루트 3개 제거와 남은 검사 프로세스 0건을 확인했다.
push·배포·설치본 변경은 하지 않았다.
