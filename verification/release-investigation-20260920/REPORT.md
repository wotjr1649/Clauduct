# S45: release investigation — 2026-09-20

**v0.3.0 판정: HOLD.** 실제 혼합 PDF TUI에서 `COUNT_INPUT_MISMATCH`가 발생했다.
소켓 반례는 사용자의 AdGuard 필터 비활성화 전후 비교로 환경 원인을 확인했고,
부분 도구 인자 생성 중 Esc는 실제 TUI에서 회복까지 확인했다. 제품 소스·개발 바이너리·
native 설치·보호 설정은 이번 조사에서 변경하지 않았다. 테스트 코드와 증거만 추가했다.

대상 제품 source는 `3df87d369445796ba94da7880d0e4852247d3d90`, 개발 executable
SHA256은 `e43235288e3cd9c1b634bfb94a81af1ffb6bda58ba35df324aed066cbbe3cafb`이다.
두 TUI 모두 Claude Code `2.1.278`, Windows amd64, 기존 승인 project/profile을 사용했다.
native binary SHA256은 `006ea5c8638f67f10a5ae66bb232fd267c9f6af294e3f03f4cfcf1fd3f2cced8`이다.

## 1. Windows 소켓: 필터 간섭 확인, SetLinger 제품 적용 불필요

같은 `socket-probe.mjs`로 공개 loopback 입력만 보냈다. 사용자가 필터를 비활성화했고,
시험은 설정을 읽어 복제하거나 수정하지 않았다. 비활성화 전후 Node source는 동일하다.

| 관측 | 필터 활성 상태 | 사용자가 필터를 끈 후 |
|---|---:|---:|
| raw TCP 응답 뒤 종료 대기 오류 | 8/40 | 0/40 |
| raw TCP 최대 소요 | 1,528 ms | 31 ms |
| 정상 HTTP 응답 오류 | 0/40 | 0/40 |
| 공개 `Expect` 헤더 변경 | 3/3 | 0/3 |

활성 상태의 raw TCP 오류는 **응답 바이트가 이미 온 뒤 종료 대기가 timeout된 사례**다.
이 수치를 응답 유실 8건으로 바꾸지 않는다. 비활성화 후에는 헤더 원문도 보존됐다.
원장: `socket-1789880533558.json`, `socket-1789880770551.json`.

같은 Go 1.27.1 환경에서 기존 독립 반례도 다시 실행했다. `TestCancelledResponse`는
CPU 1/4 각각 기본 Close 200건과 linger 200건, 합계 800건에서 stall 0이었다.
기본 Close만으로 400건이 통과했다. 각 200건은 기본 0.47/0.74초, linger 0.46/0.72초였다.
`TestRawTCP`의 plain/halfclose/linger도 각각 400건, 총 1,200건에서 오류 0이었다.
원장: `../deadline-repair-20260920/stdlib-1789880774977.jsonl`,
`../deadline-repair-20260920/tcp-1789880801095.jsonl`.

이전 활성 상태의 Go HTTP 관측은 기본 Close 1/200·4/200 stall, linger 0/200이지만
27.47/23.48초였다. 이전 FAIL은 보존한다. 전후 결과는 **이 PC의 반례 원인을 AdGuard
필터 간섭으로 판단**할 근거다. 모든 Windows/Go 종료 오류가 같은 원인이라는 뜻은 아니다.
AdGuard 내부 어느 driver 함수가 FIN을 보류하는지까지 packet/driver trace로 확인하지 않았다.

Go `internal/poll.FD.Close`는 poller를 깨우고 참조가 끝나면 socket handle을 닫는다.
원격 peer의 애플리케이션이 EOF까지 소비했다는 확인은 하지 않는다. WinSock 기본 close는
미송신 데이터의 처리를 배경에서 끝내며, 로컬 handle 반환과 상대 통지는 다른 사건이다.
중간 필터가 연결을 중계·보류하면 로컬 서버 종료 뒤 peer가 더 기다릴 수 있다.
[Microsoft close 의미](https://learn.microsoft.com/en-us/windows/win32/api/winsock/nf-winsock-closesocket),
[AdGuard localhost 필터 설정](https://kb.adguard.com/kb/adguard-for-windows/settings/app-settings/advanced-settings/).

`SetLinger(1)`은 데이터 전달 보증이나 보안 필터 수리가 아니다. 대기를 늘리거나 timeout 후
미송신 데이터를 버릴 수 있다. 이번에는 기본 Close가 통과하므로 제품에 적용할 이유가 없다.
향후 깨끗한 환경에서도 독립 반례가 재현될 때만 다른 원인에 대한 실험으로 사용한다.
[Go SetLinger](https://pkg.go.dev/net#TCPConn.SetLinger).
제품의 소유 프로세스 회수는 이미 검증한 cancel/Stop/Wait/Job 경로로 유지한다.

## 2. 실제 혼합 PDF: 내용 인식 성공, 정확 계수 실패

이미지·문서 내용을 분석하고 답하는 주체는 backend 모델이다. native Read의 페이지 변환과
Clauduct의 요청 구성·계수·전달은 그 입력을 모델에 연결하는 계층이다. 아래 실패는 모델의
시각 인식 실패가 아니라, 그 전달 경로의 사전 계수를 정확하다고 취급한 호환성 문제다.

`mixed-pdf.mjs`가 만든 [3페이지 PDF](mixed-report-s45.pdf)는 실제 PDF 객체로 이루어진
공개 합성 기술 보고서다. 스캔 이미지 하나가 아니다. 선택 가능한 텍스트·숫자 표,
raster 막대 차트, vector 화살표, 위첨자와 분수선으로 조판한 수식이 포함된다.
PDF SHA256은 `d76e570eaabac43021acde46fe05bcb2dec00492e13e0eec6962ad5a5ed32fb4`이다.
이는 암호화·양식·첨부·임베디드 스크립트·모든 폰트/수식의 시험을 뜻하지 않는다.

실제 TUI UUID `97a13fcd-cc18-433a-99ad-e0fe0948b885`에서 Read pages 1–3을 사용했다.
native가 반환한 것은 텍스트 199자와 JPEG 3개였다. 따라서 이 요청은 원본 PDF를 보내는
`document -> input_file` 경로가 아니라 **이미지가 포함된 function_call_output** 경로다.
이 차이를 놓친 작은 PDF 시험으로 전체 PDF 지원을 판정할 수 없다.

| 확인 항목 | 결과 |
|---|---|
| 문서 code | Q7M-284 |
| 보정된 표 합계 | 57 |
| 차트 최고 막대 / red:green | red / 3:1 |
| vector 방향 | Sensor → Integrator |
| 수식 / f(3) | `(x²+3x+2)/2` / 10 |
| 페이지 간 합계 | 67 |
| 정확 계수 | **15,136 ≠ 실제 16,554: FAIL** |
| TUI | 답변 이후 API Error, `COUNT_INPUT_MISMATCH` |

정답 값의 존재를 검사한 결과이며 문서 이해 정확도를 일반화하지 않는다.
`pdf-audit.mjs`와 `pdf-tui-failure.json`에 실제 도구 결과 shape, transcript hash,
고정 정답 검사, 실패 요청, 최종 프로세스 회수 근거를 저장했다. API failure 1을 보존한다.

독립 공개 입력으로 같은 subscription backend에 count 1회+generation 1회씩 비교했다.
원본 PDF만, PNG 192dpi 3장, JPEG 100dpi 3장, PDF+PNG는 모두 일치했다.
JPEG quality 90으로 native 변환을 맞춘 대조는 다음과 같다.

| 동일 이미지의 입력 위치 | 사전 계수 | 생성 usage | 결과 |
|---|---:|---:|---|
| user content | 3,497 | 3,497 | PASS |
| function_call_output | 2,107 | 3,525 | **FAIL, 차이 1,418** |
| function_call_output, detail=high | 2,107 | 3,525 | FAIL |
| function_call_output, detail=auto | 2,107 | 3,525 | FAIL |

도구 결과 대조의 차이는 TUI 실패의 차이와 정확히 같다. 동일 encoded 입력을 count와
generation에 보냈으므로 별도 local tokenizer의 문자열 오차나 PDF 그림 인식 문제로
설명되지 않는다. **backend warmup의 tool-output image 계수 동작 차이**가 확인됐다.
provider 내부 구현까지 확정한 것은 아니며 다른 모델·이미지 조합 전체로 일반화하지 않는다.

원장: `mixed-count-1789880852934.jsonl`(4 PASS),
`mixed-count-1789881017091.jsonl`(user PASS/tool FAIL),
`mixed-count-1789881110407.jsonl`(high/auto FAIL).
원래 실패 assertion은 그대로 두었다. 기본 회귀에서 실행되지 않는 opt-in live 재현기다.

공식 `generate:false`는 출력 없이 request state를 준비하는 기능이며, 해당 페이지는 이를
모든 입력의 정확 계수 계약으로 정의하지 않는다. 별도의 공식 input_tokens API는 정확
계수를 명시한다. 하지만 같은 구독 backend의 `/responses/input_tokens`와
`/responses/count_tokens` 재확인은 둘 다 404였다(`mixed-count-1789881450419.jsonl`).
별도 API key·유료 외부 서비스는 사용하지 않았다.
[warmup 의미](https://developers.openai.com/api/docs/guides/websocket-mode),
[공식 계수 API](https://developers.openai.com/api/docs/guides/token-counting).

권장 보완은 먼저 이 입력을 정확 계수 지원으로 홍보하지 않고, 실행 전 지원 판정과
오류 설명을 추가하는 것이다. 현재 mismatch 후 모델 전체 counter quarantine은 같은
모델의 이후 요청도 막는다. 형식이 입증된 실패와 모델 전체 tokenizer drift의 격리 범위를
분리할 필요가 있다. 단, 실패 이미지를 계속 포함하는 대화는 여전히 정확 계수를 보장할
수 없으므로 정상 실행으로 몰래 넘기거나 이미지 이력을 삭제해서는 안 된다.

완전 지원은 tool-output media도 일치하는 계수 경로 확보 또는 모든 관련 입력을 같은
의미·권한·이미지 품질로 처리하는 변환의 검증이 선행돼야 한다. 고정 +1,418 보정,
quality 저하, 에러 무시, tool data를 사용자 지시로 격상하는 변환은 해결책으로 채택하지
않았다. user media 대조 PASS만으로 native Read를 고쳤다고 말하지 않는다.

## 3. 부분 인자 Esc와 본문 표시

TUI UUID `28b23d3f-c9c7-42af-b161-4e775c4028a0`에서 Write content를 길게 생성시켰다.
모델 상태를 묻지 않고 상태 파일의 event 숫자만 유한 시간 관측했다. 인자 delta 164개,
본문 0개, 미종료 상태를 확인한 뒤 Esc를 보냈다. 최종 기록은 인자 delta 563개,
본문 0개, terminal 미관측, client cancellation이었다. native tool call 0,
파일 생성 0, 같은 세션의 `S45_ARGS_RECOVERED_47` 및 별도 후속 프롬프트 완료를 확인했다.
API failure 0, 취소 1, 최종 nativeReaped true/active 0이다.
`args-observed.json`, `esc-audit.mjs`, `esc-evidence.json` 참조.

입력 복원은 실제 관측됐다. `Ctrl+U`가 마지막 화면 행만 지워 이전 프롬프트 앞부분이
남았고, 명시적인 취소·no-tools 지시를 덧붙인 입력이 회복했다. 따라서 이 시험은
**완전히 빈 초안으로 지웠다는 증거가 아니다.** 별도의 다음 입력은 깨끗한 composer에서
완료했다. 프롬프트 복원과 자동 재전송은 다르며, 원치 않는 재전송 방지를 위해 실제
composer 내용을 확인해야 한다.

일반 텍스트 80행 시험에서는 API text delta가 진행 중인 동안 TUI 토큰 숫자가 증가하고
본문은 늦게 나타나는 현상을 재현했다. gateway는 첫 byte 이후 계속 text를 전달했고
`replyWithheld:false`였다. 소스의 일반 `AppendText -> emit -> Flush`는 즉시 전달한다.
명시적 보류는 도구 인자/추론 기록의 완료 검증, Workflow 최종 결과, 검증된 자식 대기다.
native `turn.step` 모듈도 일반 chunk를 yield하며 전체 답변을 저장하는 구현이 아니다.

**실제 첫 화면 paint의 정확한 ms와 native UI 내부 지연의 최종 원인은 아직 미확정**이다.
ConPTY 결과 회수 시각을 화면 paint 시각으로 사용하지 않는다. 도구 인자 보류는 현재
무중복·완료 검증 설계의 일부지만, 일반 텍스트 표시 지연은 불가피한 특성으로 판정하지
않았다. 다음 계측은 본문을 저장하지 않고 backend 첫 text, gateway 첫 text Flush,
native 첫 text 수신과 terminal 첫 표시를 한 요청에 연결해야 한다. 그 결과가 있어야
어느 층을 수정할지 정할 수 있다.

## 4. 자연 빈 응답, 문서 형식, 종료 환경과 출시 조건

자연 발생 빈 응답은 이번에 관측하지 않았다. 반환 자체는 모델/backend가 결정하므로
성공할 때까지 실호출을 반복하지 않는다. 기존 실제 TUI+통제된 upstream 빈 응답 시험은
대기 자식·완료 이벤트·새 입력 보존을 검증하는 유효한 장애 주입 근거다. 그것을 자연
발생 관측으로 바꾸지는 않는다. 대기 근거 없는 빈 응답은 성공으로 위장하지 않고 별도
실패로 남기는 계약을 유지한다.

| 범위 | 현재 의미와 검증 방법 |
|---|---|
| PNG/JPEG/GIF/WebP | 이미지 입력 변환 지원. MIME·크기·여러 장·tool result 등 조합별 계수/인식을 구분해야 함 |
| PDF | text, page image, 수식/표/레이아웃을 포함하는 문서 컨테이너. 이번 혼합 3페이지는 실제 TUI 계수 FAIL |
| Word DOC/DOCX, PPT/PPTX, Excel XLS/XLSX | 현재 Clauduct 직접 document MIME 허용 목록에는 없음. 별도 도구의 추출/변환과 직접 모델 입력은 다른 지원 범위 |
| Office 변환 | DOCX/PPTX의 그림·차트는 PDF 변환 후 시각 검증, XLSX는 cell/수식/캐시값/시트/차트의 보존을 따로 검증해야 함 |
| audio/video | 이번 4개 모델 라우팅/변환의 지원 근거 없음. 문서 확장자 목록과 같은 뜻이 아님 |

공식 Responses API도 non-PDF 문서의 text 추출과 PDF의 text+page image, spreadsheet
처리를 구분한다. 공식 API의 지원을 subscription backend/Clauduct 지원으로 복사하지
않는다. [공식 파일 입력 문서](https://developers.openai.com/api/docs/guides/file-inputs).

사용자가 모든 파일과 모든 종료 상황을 직접 조작할 필요는 없다. 공개 corpus와 정답표,
실제 native TUI 자동 조작, 독립 상태/파일/프로세스 관측으로 반복 가능하게 검사하고,
사용자 고유 terminal·drag/drop·실문서·권한 설정만 대표 수동 수용 시험으로 남기는 쪽을
권한다. 로그오프·OS shutdown·전원 손실은 별도 격리 VM/외부 관측기가 필요하다.
현재 사용자 작업 PC에서 세션 전체를 끊는 시험은 수행하지 않았다. 유한한 시험을
‘모든 종료 환경’의 보증으로 표현할 수 없다.

v0.3.0은 다음 조건 이후에만 재판정한다.

1. 혼합 PDF/native Read의 계수 실패를 해결하거나, 사용자가 수용한 제한 범위를 실행 전
   판정·진단·문서에 일치시킨다. 현재의 답변 후 API Error는 합격이 아니다.
2. 일반 text 표시 지연을 계측해 원인을 확정하고, 채택한 지연 기준의 실제 TUI 근거를 남긴다.
3. 같은 최종 candidate bytes에서 관련 TUI 회복/중첩/Workflow/압축과 필요한 회귀를 수행한다.
   기존 다른 commit의 PASS 및 opt-in skip은 이번 후보 PASS로 바꾸지 않는다.
4. CI에 있는 race 검사, 깨끗한 재현 빌드, 세 바이너리/설치·삭제 스크립트/SHA256SUMS,
   격리 설치·업데이트·rollback과 현재 지원 문서를 최종 release artifact 기준으로 검수한다.
   이번에는 새 release package/설치/업데이트/race 검사를 실행하지 않았다.

소켓 반례에 SetLinger를 추가하거나 자연 빈 응답을 운 좋게 얻는 일이 별도의 필수
출하 관문은 아니다. 현재 알려진 기능 실패와 명시한 지원 범위의 미검증이 관문이다.
이번 조사에서 tag/push/publish/설치 릴리즈 변경은 하지 않았다.

최종 산출물 검수: 조사 JavaScript 문법 검사, 새 opt-in Go 시험의 offline compile,
`git diff --check`, 문서 검사(112개 문서, citation 89개, 내부 링크 537개)는 통과했다.
이 검수 결과는 보존한 live PDF 계수 FAIL을 PASS로 바꾸지 않는다.
