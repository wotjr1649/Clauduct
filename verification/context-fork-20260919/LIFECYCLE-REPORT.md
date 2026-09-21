# 요청 취소·종료 보완 및 native /context 한계

2026-09-19. 제품 코드: `051177d220dbfd6277f05c76d7669a25f83e6d55`.
기존 `9ebb722`에서 시작했으며, 시험 후보 `253a9c5`에서 실제 TUI의 계수 취소 오분류를 발견해 추가 수정했다. `253a9c5`는 설치 대상으로 승인하지 않았다.

## 원인과 수정

1. **취소 감시가 요청보다 오래 살아남을 수 있었다.** 기존 done 채널 재확인은 검사와 deadline 갱신 사이의 경쟁을 막지 못했다. 일반 응답과 count_tokens 모두에서 요청 handler가 반환된 뒤에도 deadline 작업이 남는 순서를 재현했다. `context.AfterFunc`의 중지 함수와 완료 채널로 이미 시작된 작업까지 기다리게 했다. 정상 요청마다 대기 goroutine을 만드는 코드도 제거했다. Go의 [AfterFunc 연결 예제](https://pkg.go.dev/context#example-AfterFunc-Connection)가 설명하는 완료 대기 방식이다.
2. **네 이벤트 업로드에 종료 취소와 시간 상한이 빠져 있었다.** `/clauduct/agents`, `/clauduct/context`, `/clauduct/workflows`, `/clauduct/tool-failures`가 길이 제한만 적용했다. 중간에 멈춘 업로드가 shutdown을 붙잡는 현상을 네 경로 모두 재현했다. 공통 `readBounded`에 기존 admission·300초 본문 deadline·취소 감시를 적용했다. 원래 길이 제한과 검증은 유지했다.
3. **초기 거부도 미완성 본문을 기다렸다.** net/http는 응답 전 남은 본문을 읽을 수 있다. 공통 거부 응답에서 이 읽기에 1초 상한을 두었다. 이미 본문을 보낸 정상적인 오류 요청에는 고정 대기를 추가하지 않는다. 중간에 업로드를 멈춘 잘못된 요청은 응답까지 약 1초, 서버 정리까지 추가 시간이 들 수 있다. `Connection: close` 강제 적용과 Request.Body wrapper를 사용한 시험안은 회귀가 있어 제거했다.
4. **정확 계수 중 사용자 취소가 API 실패로 집계됐다.** TUI 세션 `aef5ccfa-9d0b-4654-9d1f-df9de0610e60`에서 Esc를 누르자 `CONTEXT_COUNT_FAILED_CANCELLED / 400`이 기록됐다. 일반 사전 계수와 count_tokens의 오류 분기에 실제 요청 context 취소 검사를 추가했다. 실제 취소는 `CANCELLED / 499`이며, 요청은 취소되지 않았는데 backend 오류 이름만 Cancelled인 경우는 계속 계수 실패로 기록한다.

수정된 범위는 gateway 요청 읽기·취소·거부 분류다. native 실행 파일, renderer, 모델별 정책, 도구 기능, 인증·신뢰 설정, 구독 backend를 교체하지 않았다. launcher는 원래 승인된 공개 fixture profile을 재사용한다. native /model·/effort 명령이 그 격리 profile에 저장하는 선택값은 실제 TUI 동작의 일부다.

## /context에서 해결된 것과 해결할 수 없는 것

native 2.1.278 실행 파일을 읽어서 `VBt`, `QFt`, `Lg`, `Bfo`의 실제 경로를 확인했다. 공식 [명령 문서](https://code.claude.com/docs/en/commands)는 `/context`를 CLI 내장 시각화 명령으로 설명한다. API는 관찰·별도 명령 구현을 제공하지만, 검토한 현재 API에는 기본 renderer를 유지하면서 해당 로컬 이력을 제거하는 지원 경로가 없었다.

`VBt`는 backend usage가 있으면 전체 표시에는 그 값을 사용하고, Messages 항목에는 `QFt`가 계산한 usage 이후의 로컬 이력 추정을 더한다. `/context` 자체가 생성한 hidden Markdown도 로컬 이력에 남는다. 이 계산은 gateway의 count_tokens를 호출하지 않는다. 따라서 **현재 native 코드·renderer를 유지하고 gateway만 수정하는 조건에서는 이 항목별 표시 오류를 완전히 고칠 수 없다.** 같은 목표로 계속 우회 구현을 반복하지 않는다. native 자체 수정 또는 upstream에서 지원하는 이력/계산 변경이 필요하다.

시험 후보의 실제 TUI에서 재확인한 수치:

| 순서 | native 전체 표시 | native Messages 표시 | 실제 backend 입력 |
|---|---:|---:|---:|
| 처음 조회 | 9.9K | 19 | 생성 없음 |
| 두 번째 조회 | 9.8K | 14 | 생성 없음 |
| 짧은 일반 응답 후 조회 | 12.6K | 2.8K | 직전 생성 12,607 |
| 바로 이어서 조회 | 12.6K | 4.8K | 생성 없음 |
| 다음 짧은 일반 응답 | — | — | 12,655, 정확 계수 일치 |

두 조회 사이의 native Messages 표시는 약 2K 늘었지만, 그 뒤 실제 생성 입력은 새 질문·이전 답변 등을 포함해 48토큰 늘었다. 기존 provenance 기반 출력 제외가 계속 작동한다. 이 후보에서 10개 요청에 진단 블록 105개를 제외했고 읽기 실패·용량 초과는 없었다. 이 제외 로직은 이번 제품 변경에서 수정하지 않았다.

초기 `/context`는 정확 backend 계수에 네트워크 왕복을 쓴다. 초기값 또는 바뀐 도구 구성의 계수가 캐시에 없을 때 native 로컬 추정과 같은 즉시 응답을 보장할 수 없다. 조회에 생성 inference를 요구하지 않는다는 사실과, 계수 왕복에 시간이 든다는 사실은 구분해야 한다.

`Lg`는 backend usage 이후의 메시지를 추정하며 native 자동 압축 검사 `Bfo`에서도 사용된다. 따라서 표시 오차가 native의 다른 로컬 판단과 완전히 무관하다고도 단정하지 않는다. 대량 조회만으로 조기 압축되는 경계는 이번에 실측하지 않았다. gateway가 관측한 정확 계수·정책 적용과 native 내부 추정은 status에서 구분해 읽어야 한다.

## 실패 기록과 검증의 한계

- 수정 전 수명 경쟁: `cancellation-1789806012525.jsonl`, 두 endpoint 실패.
- 수정 전 이벤트 종료/거부: `eventdrain-1789806166940.jsonl`, 네 이벤트 종료 실패와 미완성 본문 거부 지연.
- 거부 연결 강제 종료 시험안: `gateway-1789806272167.jsonl`, `gateway-1789806342653.jsonl`, `gateway-1789806460243.jsonl` 등에 Windows 연결 재설정 회귀를 보존했다. 그 구현은 제거했다.
- Clauduct를 사용하지 않는 Go HTTP 서버 진단에서도 종료 시 전송 오류가 발생했다. `transportprobe-1789806970569.jsonl`: keep-alive 0/20, close 2/20, close+flush 4/20, close+length 4/20, CloseWrite 후 Close 8/20, positive linger 7/20. `transport-probe.go.txt`는 이 진단 소스다. 이 프로그램의 pass 표시는 관측 루프가 끝났다는 뜻이며 제품 합격 검사에 포함하지 않는다.
- 별도 시험안의 peer FIN 대기는 서버 goroutine이 이미 종료됐는데도 간헐 timeout을 관측했다. `refusals-1789806837734.jsonl`에 stack을 보존했다. 최종 거부 검사는 온전한 오류 응답과 서버 Shutdown 완료를 확인한다. peer FIN 전달의 간헐 현상까지 해결됐다고 주장하지 않는다.
- 기존 `gateway-1789803907427.jsonl`의 단 한 번의 `active=1` 실패는 당시 stack이 없어 이번 수명 경쟁이나 TCP 현상과 인과관계를 확정할 수 없다. 반복 성공을 근원 해결의 증거로 대체하지 않는다.
- TUI의 자연어 보고도 기계적 증거와 분리했다. 후보 세션의 부모는 실행 직후 agent ID 공개를 거부하는 문장을 출력했지만, status는 `a184309aa5988842c`와 `gpt-5.6-luna/high`를 검증된 선택으로 기록했다. TaskStop 성공·native aborted·cancellation_reported는 확인했다. 모든 자연어 지시 준수까지 보장한 것은 아니다.

## 최종 확인 기록

최종 바이너리의 TUI 세션은 **`930b657c-59e6-43ff-9dec-01f6c587c43a`**이다. 첫 사전 계수 중 Esc 취소는 `CANCELLED / 499`, `apiFailures=0`, `cancelledRequests=1`로 기록됐다. 이후 `FINAL_RECOVERY_OK` 응답, Astra에서 Sol로 전환, Terra/medium Plan Agent 실행, 완료 이벤트를 통한 결과·파일 1행 근거·검증된 자식 ID 전달, `/context all`, `/exit`를 확인했다. 자식은 `aa7bf311c6bb4990f`이며 상태는 `parent_received`다. 부모가 TaskOutput 또는 SendMessage로 주기적으로 확인한 호출은 없었다.

최종 status는 요청 62건, count_tokens 47건, generation 10건 중 취소 1건, backend usage가 있는 완료 생성 9건의 정확 계수 모두 일치였다. API 실패·native 도구 실패·결과 미확보·불필요한 압축 제어는 각각 0, exit 0, `nativeReaped=true`, watchdog 강제 종료 없음이다. native 자체 표시까지 승인한 것은 아니며 `acceptance=not_assessed`를 유지한다.

- 전체 자동 회귀: **1,492개 테스트/하위 테스트, 17개 package 통과, 실패 0**. `all-1789808116600.jsonl`.
- `go vet ./...`: exit 0. `vet-1789808116598.jsonl`.
- 일반 응답 및 count_tokens 미완성 업로드 취소: 각각 30회 통과. `drain-1789808203853.jsonl`.
- 최초 종료 수정의 거부 응답 회귀 5회: `refusals-1789807107853.jsonl`. 이후 계수 취소 분류 수정도 최종 전체 회귀에 포함했다.
- 계수 취소 오분류의 수정 전 두 endpoint 실패: `cancellation-1789807983427.jsonl`. 수정 후 실제 취소와 취소 근거 없는 오류를 구별하는 검사 통과: `cancellation-1789808037724.jsonl`.
- 전체 회귀에서 기존 조건에 따라 console-close, opt-in live, launcher-kill 3개 테스트는 skip됐다. 실제 TUI와 launcher-kill은 아래 별도 관측으로 확인했다. console-close 및 race detector는 이번에 실행하지 않았다. race detector에 필요한 C 컴파일러는 현재 PATH에서 확보하지 못했으며 전역 도구를 설치하지 않았다.

239K/450K 경계를 실제로 채우는 시험과 모든 멀티모달 입력의 재실행은 이번 TUI 범위에 포함하지 않았다. 관련 자동 회귀의 통과를 모든 모델·입력 형식의 실제 backend 계수 검증으로 확대 해석하지 않는다.

강제 종료 세션 **`fd5c5979-7838-4fde-8ef9-96029da9c4d3`**에서는 TUI에서 공개 `Start-Sleep -Seconds 90`을 실행한 상태로 검증된 launcher handle 하나만 종료했다. launcher와 자손 6개가 관측 기준 **43ms 이내 종료**됐고 별도로 만든 대조 프로세스는 살아 있었다. 종료 도구는 이후 자기 대조 프로세스도 정리했다. 의도한 exit 57005이며 정상 exit로 해석하지 않는다. 근거는 `hard-kill-fd5c5979-7838-4fde-8ef9-96029da9c4d3.json`이다.

검증한 세 바이너리를 `D:\AIDEV\clauduct-s36-build`에 배치하고 각각의 SHA256을 비교했다. `clauduct.exe`의 SHA256은 **`5bc5ac8a5c3f7467f13a74d418abcaab05f430201f176e7784ea794fd0f14aeb`**이다. 이전 세 바이너리는 `D:\AIDEV\clauduct-s36-build\before-051177d-20260919`에 보존했다. 설치 기록은 `delivery.json`, 이전 설치 기록과 두 후보의 build/test 식별자는 `*-9ebb722.json`, `*-253a9c5.json`에 보존했다. `C:\Users\js\.local\bin` 릴리즈는 교체하지 않았다.

소스는 `D:\AIDEV\clauduct-s36-build\context-fork-repair-20260919`의 `fix/context-fork-acceptance` branch에 커밋했다. 원래 `D:\AIDEV\Clauduct` checkout을 병합하거나 원격에 push하지 않았다. 재빌드할 때는 위 소스/commit과 build 기록을 사용해야 같은 제품 코드가 된다.

`process=SUCCESS`는 프로세스 종료 결과다. native 표시, 간헐 TCP 종료, 모든 입력 형식과 모든 모델 응답의 무결함까지 승인하는 표시는 아니다.
