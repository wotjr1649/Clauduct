# S46 — 실측 usage 기록·예방 압축 전환

2026-09-20. 사용자가 정확한 사전 차단 보장을 철회하고 실측 기반 기록과 예방 압축을
채택했다. 제품 commit `33359c460a5321ffec65d75276894e41bcae3ba6`를 clean worktree에서 빌드했다.
[빌드 신원](build.json), [기계 검사](evidence.json), [개발 경로 반영](delivery.json).
정책 변경은 관측한 작업 범위에서 통과했고 **v0.3.0 전체 출시는 HOLD**다.

## 바뀐 동작

- 일반 생성·압축에서 원격 사전 계수를 제거했다. backend input/output usage를 그대로
  기록하며 cached input/reasoning output을 다시 더하지 않는다. 누계와 agent별 마지막
  사용량을 분리하고, 취소 등으로 usage가 없으면 unknown으로 남긴다.
- agent별 마지막 실측과 저비용 텍스트 추정으로 예방 압축한다. 수치·출처·미계산
  미디어/추론 포함 여부를 status에 표시한다. 추정은 정확 계수나 상한이 아니다.
  알고리즘과 조기 압축 가능성은 [채택 정책](POLICY-REVIEW.md)에 명시했다.
- Astra 500K/450K, Sol·Terra·Luna 272K/239K는 관리 목표다. 새 대용량 입력의 최초
  초과를 정확히 막는 보장은 없다. 모델 이름 변경만으로 압축하지 않는다.
- 모델·effort·역할·계보·native 회차 확인은 유지한다. 필요할 때의 압축은 기존 확정
  모델·effort를 사용한다. journal v1은 사용량을 조작하지 않고 v2로 이행한다.
- `/count_tokens`는 별도 기능이다. 도구 결과의 이미지/PDF warmup 계수는 S45의
  불일치 때문에 미지원이다. 동일 입력의 실제 usage 캐시는 사용할 수 있다.
  계수의 실패·불일치가 정상 생성 응답을 실패시키지 않는다. 새 모델은 계수 검증을
  별도로 명시해야 하며 모델 목록 추가만으로 과거 토크나이저가 허용되지 않는다.
- 전달 전 구조화된 `context_length_exceeded`만 한정된 native 압축 회복을 허용한다.
  압축 후 다시 초과하면 명시적으로 중단한다. 일반 400/timeout/연결 끊김을 근거로
  작업을 재실행하거나 이력을 잘라내지 않는다. 이 분기는 fixture 검증이다.

## 실제 native TUI 근거

UUID `3af409ba-d6f4-44b8-8313-a61d2231839f`, Claude Code `2.1.278`, Windows,
사용자가 AdGuard 필터를 다시 켠 상태. 승인된 공개 fixture project/profile만 사용했다.
후보 바이너리 SHA256:
`89a93f29e0e5974e04d459fa0966bd047018f4390898be0e7b57c74205b053fe`.

| 검사 | 관측 결과 |
|---|---|
| 혼합 PDF 3페이지 Read | 문서 코드 Q7M-284, 표 합계 57, Red 3:1, Sensor→Integrator, 수식 값 10, 교차 합계 67 모두 정답. 해당 입력 usage 16,586. 사전 계수·API 오류 0 |
| Astra→Sol 전환 | 17,068 input에서 강제 압축 없이 이전 값 회수 |
| 수동 compact 두 번 | 32,964ms / 40,742ms. 두 번째는 Terra로 변경 후 일반 요청 없이 실행했고 Sol/high 유지. 이후 Terra에서 보존값 회수 |
| 중첩·병렬 Agent | Terra/medium 자식→선택 생략 손자에 같은 선택 상속. 손자 91→자식 100, Sol/high 자식 44, 부모 합계 144와 본문 수신 |
| Workflow | Luna/max 선택으로 결과 42와 runId 전달. 단, 작업자가 불필요한 ToolSearch 4회 수행하여 무도구·효율 조건은 실패 |
| Esc 후 회복 | backend text delta 1,824개 생성 중 취소. terminal usage 미확보를 unknown으로 기록. 후속 `S46_ESC_RECOVERED_47` 정상 |
| `/context all` 세 번 후 대화 | 별도 count 요청 46건, 후속 정상 답변. 조회 전후 main input 21,775→21,822. native 화면의 정확한 세 번 수치는 이번 감사에서 확보하지 못함 |
| 종료 | exit 0, nativeReaped true, active 0, API 실패 0, 도구 실패 0, 결과 미확보 0, 의도한 취소 1 |

총 생성 32건 + 압축 2건 중 usage 확보 33건, 취소로 미확보 1건. 네 모델 모두
generation preflight 0. `/context`의 명시적 계수 전에는 count 요청 자체가 0이었다.
[최종 status 투영](steps/final.json), 각 단계의 `steps/*.json`과 원본 해시를 보존했다.
자동압축 경계는 backend usage fixture 및 실제 native 프로세스 결합 검사로 확인했다.
이번 실제 구독 TUI에서 239K/450K를 채운 자동압축 검증을 새로 했다는 뜻은 아니다.

## 자동 검사와 실패 이력

- `all-1789885908664.jsonl`: 17 packages, 1,599 test/subtest 통과. 콘솔 창 닫기,
  opt-in live, 외부 강제 종료의 별도 opt-in 검사 3개는 skip. 무검사 childprocess package는
  테스트 skip 수에 포함하지 않았다. Windows CGO=0이라 race 검사를 실행하지 않았다.
- 최종 journal 필드 검증 보완 후 `gateway-1789886115739.jsonl`: 526 test/subtest 통과.
  `vet-1789886168913.jsonl`: exit 0. source diff 검사도 통과했다.
- 최초 gateway build 실패는 누락된 `countPost` 시험 helper, 최초 전체 검사 4개 실패는
  구 정책 CountAgreement 기대값과 preflight 전용 fixture 입력이었다. 실측 입력으로
  바꾼 뒤 관련 native 검사 25개와 전체 검사를 통과했다. 실패 로그를 삭제하지 않았다.
- 새 HTTP 분류 시험의 첫 실패는 Direct transport에 synthetic-mode provider를 연결한
  시험 설정 오류였다. 기존 HTTP 시험과 같은 공개 placeholder provider로 수정했고
  별도 synthetic credential 거부 검사는 보존했다. 실패를 성공으로 교체 표기하지 않았다.

## 필터 활성 소켓 조사

`socket_diagnostic-1789887206047.jsonl`: Content-Length 응답 직후 서버 Shutdown을
실행한 200회 중 41회가 `*url.Error/*net.OpError/*os.SyscallError/errno_10054`였다.
이는 Windows의 WSAECONNRESET이다. [Microsoft 정의](https://learn.microsoft.com/en-us/windows/win32/winsock/windows-sockets-error-codes-2)

클라이언트가 본문 수신을 완료한 뒤 서버를 닫는 대조군은 200/200 성공했다. 대조군의
200ms 지연 판정을 명시적으로 보완한 `socket_receipt-1789887286995.jsonl`도
200회 실패 0/지연 초과 0이었다. 즉시 종료 실패는 남겨 두었으며 전체 소켓 검사를
통과했다고 주장하지 않는다. 이전 on/off 대조와 합치면 필터 활성 시 종료 순서와
연결 재설정이 관련됐다는 근거다. RST를 발생시킨 driver/peer 내부 구현까지 특정한
packet trace는 없으므로 OS 또는 Go 자체 버그라고 단정하지 않는다.

제품은 이미 native 프로세스를 먼저 회수하고 gateway를 닫는 순서다
(`go/internal/app/run.go`). deadline도 소켓 EOF만 기다리지 않고 소유 프로세스를 관리한다.
이번 TUI는 필터를 켠 상태에서 종료·회복했지만 모든 강제 종료 환경의 전달 보장은 아니다.
SetLinger(1)은 앞선 반례에서 해결 효과가 없고 지연을 늘렸으므로 제품에 추가하지 않았다.

## 남은 판정

1. 실제 화면에서 텍스트가 늦게 나타나는 현상은 재관측했다. Esc 시점에 부분 본문이
   표시됐다. 정확한 첫 paint 시각과 native hook/render 단계의 원인은 아직 확정하지 못했다.
2. Workflow worker가 부모의 Workflow 실행 요청을 다시 수행하려고 도구를 검색한
   흔적이 있다. ToolSearch 4회 후 결과는 정상 수신했으나 무도구 조건은 실패다.
   native 사용자 요청 전달과 worker 역할 구분을 다음에 조사해야 한다.
3. 즉시 소켓 종료의 필터 반례, 전체 파일 형식·종료 환경, 자연 발생 빈 응답까지
   무결점이라는 판정은 없다. 이번 PDF의 성공을 PPTX/DOCX/XLSX 직접 지원으로 확대하지 않는다.

개발 바이너리 3개와 사람용 검증 launcher의 manifest 경로를 이 후보로 맞췄다.
기존 파일은 `D:/AIDEV/clauduct-s36-build/before-33359c4-20260920`에 보존했다.
설치 명령으로 받은 사용자 release 경로는 수정하지 않았다. 태그·push·publish는 하지 않았다.
