# v0.3.0 후보 — S47 최종 검수

2026-09-20. **문서화한 Windows 지원 범위에서는 출시 가능한 후보로 판정한다
(`PASS_WITH_DOCUMENTED_LIMITS`).** 모든 필터 제품·파일 형식·종료 상황의 무결점 판정은
아니다. 다음 제한을 숨기거나 지원 범위를 넓혀서 v0.3.0을 설명해서는 안 된다.
태그, push, GitHub Release, 설치된 release 교체는 수행하지 않았다.

제품 commit: `442c3667f050fa4f252def04f19c8b1b627c9d53`.
clean worktree, Go 1.27.1, Windows amd64, CGO=0으로 빌드했다.
`clauduct.exe` SHA256:
`b6ac56dc213b7f195f1c37ff11896a2e2a166e2c90f1e8190ff86af1b6806881`.
[빌드 신원](build.json), [최종 기계 근거](evidence.json), [개발 경로 반영](delivery.json).

## 실제 native TUI와 backend

Claude Code `2.1.278`, UUID `c233d7ec-be84-40b2-96d4-e866aa58660e`.
사용자가 활성화한 AdGuard 설정을 바꾸지 않았고, 승인된 공개 fixture project/profile을
사용했다. fixture upstream이 아닌 실제 구독 backend의 실행이다.

| 항목 | 확인한 결과 |
|---|---|
| 독립 Workflow A | Sol/high, `tools:[]`, 실제 답 `S47_A_43`, 도구 호출 0 |
| B 시작·중단 | Terra/medium의 실제 Bash→PowerShell 120초 대기 명령을 시작. 테스트 launcher의 후손임을 확인하고 OS handle을 유지한 채 종료 관측. 시작 33.080초 후 exit 1, 사용자 중단 프롬프트 전달부터 12.217초. 자연 대기 완료 전 종료 |
| 미실행 C 재개 | TaskStop 뒤 추가 설명 프롬프트 없이 재개 호출. A 재사용, B 재실행 없음, C만 Luna/max·원래 `tools:[]`로 실행해 `S47_C_180`, 도구 호출 0 |
| 전체 완료 판정 | B는 `started_not_reexecuted`/결과 미확보, 계획 전체 `complete:false`. 이것을 모든 단계 성공으로 바꾸지 않음 |
| 혼합 PDF | 3페이지 Read. 코드 Q7M-284, 보정 표 합계 57, raster Red:Blue 3:2, vector Sensor→Integrator, 분수/위첨자 수식 f(3)=10, 교차 합계 67. 모두 원본 공개 PDF와 일치 |
| Esc 회복 | 실제 backend text delta 310개 생성 중 취소. 이후 같은 세션에서 문서 코드·계산값 정상 반환. 미완성 응답을 완료 usage로 만들지 않음 |
| 모델 전환 | Astra→Sol/high, 입력 21,656에서 자동 압축 없음. Sol 239K 관리 목표 미만 |
| 수동 압축 | Sol/high 유지. native 58.222초, backend 57.415초, native pre/post 21,683→3,849 |
| 압축 뒤 회수·중복 거부 | 원본 `wf_d2423432-cf8`, 재개 `wf_9ddd0371-4d8`. 원본 중복 재개는 도구 거부 1건, API 실패 없음. PDF 값과 A/B/C 상태를 회수하고 후속 대화 정상 |
| `/context all` 3회 | 총량 13.5K/500K 유지. native Messages 행 3.5K→5.6K→7.6K는 로컬 이력 추정 제한으로 남음. 실제 backend input은 13,546→13,874(+328); 앞선 output 281 및 새 짧은 입력에 해당. 조회 보고서 3개/관련 block 9개 제외 확인 |
| 계수·종료 | 일반 생성 preflight 0. `/context`의 별도 count 46건. 총 요청 81, 생성 22, 압축 1. API 실패 0, 도구 실패 0, 의도한 Esc 1, native B 취소 1, 중복 거부 1, 결과 미확보 0. exit 0, nativeReaped true, active 0 |

원래 실패를 보존했다. 최초 역할 설명 수정은 실제 무도구 요구를 지키지 못했고,
명시적 도구 제한을 추가한 다음 통과했다. 중간 후보의 부모가 TaskStop 뒤 추가
통지를 기다린 경우도 남겨 두고, 성공 결과가 중단 확인임을 안내한 마지막 후보를
다시 실행했다. [실패→수정→검증 이력](../release-final-20260920/REPORT.md),
[중간 후보 전체 기록](../release-final-tools-20260920/steps/final.json).
모델의 모든 자연어 지시 준수를 보장하는 것으로 이 결과를 확대하지 않는다.

## 필터와 프로세스의 판정

소켓 EOF만으로 취소를 기다리던 의존을 제거했다. native의 명시적 abort를 exact
session/agent/turn으로 확인하여 backend context를 취소한다. 이전 turn의 늦은 중단이
새 요청을 취소하지 않도록 검증한다. 실사용 필터를 끄거나 제외 목록을 추가하지 않는다.

소켓 취소를 의도적으로 전달하지 않는 proxy + 실제 native TUI에서는 부분 도구 인자
생성 중 Esc 후 358ms에 upstream 취소, 미완성 도구 미실행, 다음 답변까지 확인했다.
별도 deadline 완료/유예 만료 TUI도 종료·회수를 확인했다. 이 fault injection은
**고정 공개 backend**이고 위 표의 구독 backend TUI와 구분한다.
[원본 결과와 해시](../release-final-20260920/evidence.json)

독립적인 즉시 TCP 종료 실험에는 필터 활성 반례가 남아 있다. close/drain 실험도
1/200 실패해 제품에 넣지 않았다. 제품은 native 회수 후 gateway를 종료하고 취소를
별도 확인한다. 임의 필터의 패킷 폐기까지 전달 성공으로 바꾸는 보장은 없다.
SetLinger(1)은 해결책이 아니어서 사용하지 않았다.

## 자동 검사와 이전 실측의 사용 범위

전체 회귀 17 packages/1,622 test·subtest, 전체 race 17 packages/1,621을 통과했다.
마지막 기능 판정/안내 보완 뒤 gateway 일반·race 각 548, vet도 통과했다.
전체 검사는 `f9b1955` 구현, 마지막 scoped 검사는 `442c366` 변경 범위를 검증한다.
race는 설치된 GCC 16.2.0과 CGO=1을 사용하며 배포 바이너리는 CGO=0이다.
[검사 수·skip·실패·해시](../release-final-20260920/evidence.json)

콘솔 창 닫기와 opt-in live 검사 2개는 skip이었다. race에는 CGO=0 배포 확인 skip이
하나 더 있으며 일반 검사에서 그 조건을 검증했다. 실제 TUI 실행을 별도로 수행했고,
Windows Job의 정상 종료/Stop/launcher 강제 종료, 소유 후손 종료와 무관 프로세스 생존
검사는 통과했다. 다른 종료 환경 전체를 검증했다고 쓰지 않는다.

최종 TUI에서 다시 발생시키지 않은 중첩·병렬 Agent/부모 대기/빈 응답 분기는
기존 S43/S46 실제 실행, native TUI fault injection 및 이번 회귀 근거를 유지한다.
특히 자연 발생 빈 backend 응답은 관측하지 않았다. 장애 주입 검증을 자연 발생
사례라고 바꾸지 않는다. [지원표](../../docs/v2/COMPATIBILITY.md)

## 출시 설명에 남겨야 하는 제한

- native 첫 텍스트 표시 지연은 Clauduct 없는 대조군에서도 재현했다. 정확한 첫
  screen paint 시각과 내부 원인을 확정하지 못했으며 native 바이너리를 수정하지 않았다.
- native `/context`의 공통 500K·Messages 로컬 추정은 gateway의 모델별 정책/실제 usage가
  아니다. 최초 정확 계수의 backend 왕복 지연도 남는다. 표시를 가짜 수치로 교체하지 않는다.
- 이미지/PDF의 내용 해석은 backend가 수행한다. 검증한 혼합 PDF 전달·해석과 모든
  멀티모달 정확 계수를 구분한다. 도구 결과 이미지/PDF의 불일치 warmup 계수는 미지원이다.
- PPTX/DOCX/XLSX 직접 입력, 모든 PDF 변종, 임의 Workflow JavaScript의 재실행,
  다른 launcher/session으로의 범용 Workflow 복원은 현행 지원 범위 밖이다.
- 기능별 현재 조건을 확인하고 충족하지 못한 요청을 거부한다. 최신 native 버전 번호가
  일치한다는 것만으로 과거 검사나 모든 기능을 통과시켰다고 판정하지 않는다.

따라서 **위 범위의 v0.3.0 후보 출하 준비는 합격**, **모든 환경에서 모든 기능 무결점은
미판정**이다. 남겨 둔 외부/native 제한을 없앤 것처럼 발표하는 출시는 권하지 않는다.

검증한 세 바이너리와 사람용 검증 launcher의 manifest 경로를
`D:/AIDEV/clauduct-s36-build`에 반영했다. 이전 파일은
`D:/AIDEV/clauduct-s36-build/before-442c366-20260920`에 보존했다.
`C:/Users/js/.local/bin`의 설치 release 및 AdGuard 설정은 변경하지 않았다.
