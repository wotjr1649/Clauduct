# S44 보충 B OS 실행·취소·회수 최종 검수

2026-09-20. **B의 실제 OS 명령 실행 중 TaskStop·회수는 합격이다.** 사용자 S44의
관측 결과, Esc 일부 단계를 의도적으로 생략했다는 후속 설명, compact 집중 검수와 이번
보충 실증을 합쳐 **합의한 지원 범위의 S44 수용 및 v0.3.0 기능 출시 적합 판정을 합격으로 변경한다.**
원래 S44에서 실행하지 않은 단계를 실행했다고 소급 기록하지 않는다. 모든 환경·입력에서
무결점이라는 뜻은 아니며, 실제 태그·패키지 배포·공개 출시는 수행하지 않았다.

## 대상과 변경

- 제품 source: `31ff1184c21d7dac0fccd03394081aacd78b9db5`.
- 검증 바이너리: `D:\AIDEV\clauduct-s36-build\clauduct.exe`.
- SHA256: `7feb620944578d85982f8ecac20fe3ffba0096207da0e9b81682f603b5b1036f`.
- Native: `2.1.278`, Windows. 사용자 설명상 AdGuard 활성 상태이며 설정을 변경하지 않았다.
- 실제 TUI UUID: `cf295c16-790b-4aa0-aa69-9886323964c8`, 최초 실행과 정상 종료 후 같은 UUID 재개.

제품/native 코드는 수정하거나 재빌드하지 않았다. TaskStop 결함이 확인된 것이 아니라
기존 검증의 OS 실행·종료 증거 공백을 해결했다. 기존 공개 TUI launcher를 사용하고
[읽기 전용 OS 관측기](observe-s44b.go), [근거 대조 검사](review-s44b.mjs)를 추가했다.
세 개발 바이너리의 해시를 시작 시와 최종 검수에서 기존 build manifest와 대조했다.

## 실제 OS 근거

시험 명령은 native Workflow 자식의 foreground Bash에서 한 번씩 실행한 Windows 기본
`C:/Windows/System32/ping.exe -n 180 127.0.0.1`, Bash timeout `240000`이다.
일회성 native 실행 승인을 선택했고 전역 allow 규칙·auto mode를 켜지 않았다.

관측기는 해당 launcher의 PID·시작 시각을 먼저 검증한다. 그 후 자식 프로세스 계층에서
실제 `ping.exe`와 shell을 발견하고 `OpenProcess`로 핸들을 유지한다. `GetProcessTimes`의
시작·종료 시각과 `WaitForSingleObject`의 종료 신호를 기록하므로 PID 재사용과 단순
프로세스 목록 부재를 혼동하지 않는다. 종료 권한과 `TerminateProcess` 호출이 없다.
관측기는 실제 명령의 종료를 돕지 않았으며 launcher도 두 관측 완료 뒤 종료했다.

아래 시각은 UTC, 지연은 native transcript의 TaskStop tool_use 시각에서 마지막 관측
프로세스의 OS 종료 시각까지다. 키 입력부터의 지연이나 통계적 상한이 아니다.

| 시험 | OS 실행 근거 | TaskStop 호출 | 마지막 OS 종료 | 차이 |
|---|---|---|---|---:|
| A/B/C 계획의 B | ping PID 14360, parent bash 20248; ping 시작 14:37:51.553 | 14:38:13.808 | 14:38:14.041 | 약 233ms |
| 별도 전체 shell 계층 시험 | ping PID 22224, bash 14372·11348·22464; ping 시작 14:44:14.755 | 14:44:34.073 | 14:44:34.289 | 약 215ms |

첫 ping은 약 22.5초, 두 번째는 약 19.5초 실행됐다. 약 3분인 자연 완료 또는 240초의
Bash 제한 시간에 도달한 종료가 아니다. 두 TaskStop 모두 실제 실행 확인 뒤 호출했다.
두 번째 시험은 native 바로 아래부터 ping까지 명령의 전체 shell 계층을 확인했으며
네 핸들이 모두 종료 신호를 냈다. 첫 시험을 두 번 했다고 합치거나 중간 shell 두 개까지
첫 시험에서 관측했다고 쓰지 않는다. 유효한 OS 취소 실증은 2건 모두 성공했다.

TaskStop 성공 응답은 첫 시험 14:38:13.843, 두 번째 14:44:34.115다. 실제 OS 종료는
그보다 약 0.2초 뒤다. 따라서 **성공 응답 자체를 OS 회수 완료의 증거로 대체하지 않는다.**
이번 합격은 응답과 별도로 유지한 프로세스 핸들의 종료 근거까지 대조한 결과다.

## Workflow 재시작·재개·회복

첫 계획은 `wf_7d8da92f-56f` / task `wri4ms74o`다.

| 범위 | 실제 결과 |
|---|---|
| A | `a162d7bd28c9951fd`, Sol/high, `S44B_A=303` |
| B | `ac2d53e5b4e192a7a`, Terra/medium, 위 첫 OS 명령 실행 후 TaskStop |
| 첫 종료 | native 회수 완료, checkpoint 저장 1, 정상 exit 0 |
| 새 launcher 재개 | checkpoint 복원 1, 실패 0 |
| 재개 계획 | `wf_170642f6-012`, A는 `completed_result_reused`, B는 `started_not_reexecuted` |
| C | 새 자식 `a412ea78877119863` 하나만 Luna/max로 실행, `S44B_C=144` |
| 원본 보존 | 원본 journal SHA256가 첫 종료 시점과 재개·추가 시험 후 모두 동일 |
| 중복 재개 | 1건을 도구 수준에서 거부, 대체 계획·원본 재실행 없음, 이후 `S44B_AFTER_DENIAL=47` |
| 별도 전체 shell 취소 | `wf_492ca0e4-c49` / task `wel7owafn`, B 자식 `a6141a2ddf3cf7fa3` |
| 취소 뒤 회복 | `S44B_AFTER_STOP=53`, 별도 새 사용자 입력에 도구 없이 `S44B_FINAL=71` |

재개 결과의 `complete:false`는 정상이다. 중단한 B의 완료 결과가 없으므로 전체 단계의
결과가 모두 확보됐다는 표시를 하지 않는다. C의 성공으로 B를 성공 처리하지 않았다.

첫 실행/재개 합계 36 requests. API 실패 0, native 도구 실패 0, 결과 미확보 0,
예정된 native 취소 2, 예정된 중복 Workflow 거부 1. 두 launcher 모두 정상 종료했고
watchdog 강제 종료가 없었다. gateway의 `acceptance:not_assessed` 원본은 수정하지 않았다.
최종 검수 판정은 [별도 근거 JSON](s44b-evidence.json)의 `verdict`다.

## 실패 이력도 보존

앞선 UUID `fd73e62b-840a-47ae-b562-4876344d41b5`의 공개 `.ps1` 시험은 Windows
PowerShell이 실행 정책 사유로 파일 로드를 거부했다. B의 시작 marker가 없었고 관측기는
`START_MARKERS_TIMEOUT`으로 종료했다. TaskStop까지 도달하지 않았으므로 취소 합격에
포함하지 않는다. 해당 세션은 API 실패 0 / native 도구 실패 1이며
[검사 기록](inspection-fd73e62b-840a-47ae-b562-4876344d41b5.json)에 남겼다.

별도의 `Get-ExecutionPolicy -List` 조회 시도도 호스트 공유 실행 정책이 PowerShell
실행을 거부했다. 유효 정책의 상세 출처는 확인하지 못했다. 정책을 낮추거나 차단된
PowerShell 스크립트를 다른 경로로 실행하지 않았다. 이후 시험은 별도 Windows 기본
ping 명령으로 구성했으며 PowerShell 파일 실행 지원을 입증했다고 주장하지 않는다.
이 실패는 원인이 확인된 시험 선행 조건 실패이며 통과할 때까지 같은 시험을 반복한
플래키 재시도에서 성공 하나를 고른 것이 아니다.

## 검수와 출시 범위

최종 대조 검사 12개가 통과했다. 여기에는 OS 실행→TaskStop→회수의 시간 순서,
PID·시작 시각 동일성, 실제 Bash 2건, 원본 B 재실행 0, C 하나만 실행, 모델·effort,
원본 journal 보존, 후속 회복, 정상 종료, 개발 바이너리 신원이 포함된다.
이 검사는 실제 TUI/OS 기록을 검산하며 그 자체를 별도 대화형 시험으로 세지 않는다.

```text
node verification/workflow-completion-20260920/review-s44b.mjs --assert
```

관측기는 Go 표준 라이브러리로 빌드해 실제 관측에 사용했다. 새 대조 스크립트의 문법 검사와
최종 diff 검사를 수행했다. 제품 코드가 바뀌지 않아 전체 제품 회귀를 다시 실행하지 않았다.
기존 최종 1,668 pass / 3 skip 회귀와 105 pass Workflow 회귀의 원본 로그 해시를 재검증했다.
기존 skip·실패 수정 이력은 [S49 보고서](REPORT.md)와 `evidence.json`에 그대로 남아 있다.

**이번에 남아 있던 B OS 실행·회수의 출시 차단 근거 공백은 닫혔다.**
[S44 사용자 관측](S44-USER-ACCEPTANCE.md), [compact/취소 의미 분석](S44-B-COMPACT-REVIEW.md),
이번 보충 실증을 합친 판정이다. 사용자 미실행 Esc 세부 단계, native 표시 한계, 임의의
모든 문서·모든 종료 환경·모든 필터에서의 보장은 합격 범위로 확대하지 않는다.
지원 범위와 한계는 [COMPATIBILITY](../../docs/v2/COMPATIBILITY.md)에 계속 공개한다.
제품/native 버전이나 바이너리가 달라지면 이 신원의 판정을 새 조합으로 자동 승계하지 않는다.
