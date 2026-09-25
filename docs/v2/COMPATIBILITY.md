# V2 호환성 — 현재 / 제약 / 미지원

Go 개발 빌드의 지원 기능과 제한을 이 문서에서 관리한다. 격차의 이력은
[PARITY.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/PARITY.md), 이전 실행 증거는 [VALIDATION.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/VALIDATION.md),
S41 수리와 실패 기록은 [S41 보고서](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/session41-repair-20260919/REPORT.md),
기능별 판정·진행 관측·Workflow 결과 회수는 [후속 검수 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/capability-repair-20260919/REPORT.md)에 있다.
S42에서 발견한 거부 후 회복·역할 발견·압축 보완은 [S42 수리 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/session42-repair-20260920/REPORT.md)에 있다.
부모의 완료 조건·native TUI 대기·미실행 Workflow 단계 재개는 [부모 대기·계획 재개 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/parent-wait-20260920/REPORT.md)에 있다.
같은 바이너리의 사용자 S43 실행 판정은 [S43 사용자 검수](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/parent-wait-20260920/S43-USER-ACCEPTANCE.md)에 있다.
Node V1 및 설치 명령으로 받은 릴리즈의 지원표로 그대로 사용하지 않는다.

**2026-09-23 v0.3.2 재실행 방지 묶음 — 미출하.** 요청은 처음 읽은 turn 영수증 하나로 예약·선택·취소
바인딩을 판정한다. 독립 `auxiliary` 요청은 현재 root turn 안에서만 한 번 실행하고, 다음 turn의 같은
요청은 새로 실행한다. agent의 새 turn이 예약되면 그 agent의 이전 turn 실행 기록을 지운다. 그래서 긴
세션이 요청 16,384개에서 멈추지 않는다. 남는 한도는 agent마다 마지막 turn의 기록과 turn 정보 없는
기록(`--bare`)의 합이다. native 모듈이 다시 등록돼도 시계가 크게 되돌아가지 않는 한 새 게시의 순번이 이전 게시보다 크다.
후보 전체의 실제 backend TUI(생성·압축·취소·복구·종료)는 2.1.280에서 PASS했다.
[TUI 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v032-tui-20260923/README.md)
[재실행 방지 묶음 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v032-replay-ledger-20260923/README.md)

**2026-09-22 네 번째 수리·검증 — 현재 검수 범위 SUCCESS, 미출하.** 보완된 실제 backend 기록에서
최초 502의 `EMPTY_REPLY`와 terminal/event/byte 정보를 확보했다. SDK 빈 대기·완료 알림,
Workflow 자식 등록 전 경계, 동일 step의 결정 충돌, TUI 완료 알림의 진행 상태를 수정했다.
최종 일반·race 전체 18개 패키지, 정적/태그 검사, 최소 재현·수정 제거 검사가 통과했다.
실제 backend SDK는 PID 28224·12회 호출로 도구·자식 결과·손실 후 중복 방지·후속 작업을,
TUI는 PID 26160·5회 호출로 생성·압축·취소·복구·정상 종료를 확인했다. 마지막 Workflow-only
대기 상태 조건은 그 뒤 직접 경계 검사와 최종 전체 일반/race로 검증했다. native Claude의
통합 리뷰와 F1 수정 확인을 받았고, 추가 지적은 수정 또는 직접 재현에 따른 반박으로 처리했다.
현재 범위의 미해결 확정 결함·판정 차단 항목은 없다. 보호 설정은 변경하지 않았으며 On은
사용자의 마지막 확인 상태다. 일반 생성·압축은 실제 backend usage를 계속 사용한다.
과거 batch-03에서 유실된 category와 raw TCP source는 역사적 증거 한계로 남긴다.
현재 결과를 임의의 미래 버전 전체나 릴리스 승인으로 확장하지 않는다. 자세한 근거는
[`verification/v031-review-fixes-20260922/batch-04/REPORT.md`](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-review-fixes-20260922/batch-04/REPORT.md), `REVIEW.md`, `evidence.json`에 있다.

**2026-09-22 세 번째 수리 당시 — 변경 범위 PASS, 전체 판정 HOLD, 미출하.** Workflow 선택과
거부 집계, 복합 short option, 손상된 plugin 역할의 namespace, 선택적 count 분류를 보완했다.
일반·race 전체 18개 패키지와 태그 검수기가 통과했다. 실제 backend에서 native PID 27120의
파일 도구·Agent/Workflow 결과 수거·응답 손실 후 재전송 차단·후속 작업을 13회 호출로,
PID 276의 TUI 생성·압축·취소·복구·정상 종료를 5회 호출로 검증했다. 보호 설정은 변경하지
않았으며 On은 사용자의 마지막 확인 상태다. 일반 생성·압축은 계속 실제 backend usage를 쓴다.
한 선행 SDK 실행의 HTTP 502는 최초 오류명이 기록되지 않아 원인 미확정이다. 재실행 PASS가
그 실패를 종결하지 않았다. 당시 별도 Claude 재리뷰는 6분 안에 결과를 반환하지 못했다.
그때의 HOLD와 실패 이력은 보존하며 현재 판정은 위 네 번째 수리를 따른다. 당시 근거는
[`verification/v031-review-fixes-20260922/batch-03/REPORT.md`](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-review-fixes-20260922/batch-03/REPORT.md)에 있다.

**2026-09-22 첫째·둘째 수리 당시 — CHANGES_REQUIRED, 미출하.** 첫 수리 묶음에서 늦은 요청의
실패·완료가 새 turn의 결과를 덮어쓰는 문제와 압축 journal 저장 실패 뒤 복구 고착을 수정했다.
전송 검사에는 읽기·파싱 오류를 함께 남기도록 보완했다. 두 번째 수리에서는 큰 요청 거부 뒤
연결 유지, 종료 대기 중단, native의 불확실한 요청 재전송 차단을 검증했다. 당시 남은 리뷰
지적의 후속 판정은 위 세 번째 수리 기록을 따른다. 아래 PASS는 전체 릴리스 승인이 아니다.
근거는 [`verification/v031-review-fixes-20260922/batch-01/REPORT.md`](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-review-fixes-20260922/batch-01/REPORT.md)와 `batch-02/REPORT.md`에 있다.

**2026-09-22 전송 후속 수리 — 제품 수용 범위 PASS, 미출하.** 큰 body 거부를 bounded drain으로
처리하여 같은 연결을 유지하고, 완료 응답의 socket 종료 상한을 500ms로 보완했다. 같은 native
turn·step의 재전송은 실행 지문으로 차단한다. 응답 전 손실·부분 응답·upstream 오류 뒤에도
같은 PID에서 후속 요청이 성공했고, 실제 backend TUI는 PID 19376·5회 실호출·정상 종료를
80.08초에 검증했다. 13단계 실제 native 대조, HTTP/SSE 800건, 일반·race 전체 18개 패키지가
통과했다. 보호 설정은 변경하지 않았다. 구현·실패 이력·남는 경계는
[`verification/v031-review-fixes-20260922/batch-02/REPORT.md`](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-review-fixes-20260922/batch-02/REPORT.md)가 관리한다.

**2026-09-22 이전 전송 보완의 관측 수용 범위 — PASS, 미출하.** 사용자가 완료 기준을 보호 On에서의
제품 실제 동작으로 정하고 native 프로세스를 유지하는 복구를 우선했다. 응답 framing 완료 뒤
socket을 즉시 닫지 않고 상대 종료를 최대 100ms·64KiB까지 기다리도록 제품 공통 listener를
보완했다. 제품 HTTP/SSE 800건, 수정 제거 시 reset 재현, 실제 backend TUI의 동일 프로세스
생성·압축·취소·후속 응답·종료와 전체 Go race 검사가 통과했다. 보호 설정은 바꾸지 않았다.
아래 이력의 독립 Node·.NET·raw TCP FAIL/HOLD를 PASS로 바꾸는 판정은 아니며, 외부 driver의
수정 또는 모든 필터·버전의 호환성을 뜻하지 않는다. 범위·초기 검수 실패·재실행 근거는
[프로세스 유지 수리 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-transport-20260922/REPORT.md)을 따른다.

**2026-09-21 v0.3.1 개발 묶음 1 — 미출하.** 사용자 역할을 먼저 확인한 뒤 내장 이름만
정규화하고, 사용자 `Fork`의 모델 선택과 복원 시 구분을 보존한다. 자체 라우트가 없는 역할은
native의 실제 모델·effort를 확인하며 완료 추적을 유지한다. 손상된 역할 정의의 이름을
확정하지 못하면 읽힌 정의는 보존하되 부재를 단정하지 않는다. `input_audio`의 미디어 분류도
보완했다. 이번 변경은 로컬 fixture를 연결한 native 2.1.278에서 검사했으며 수동 TUI 수용
판정은 아니다. 근거와 남은 항목은 [v0.3.1 첫 묶음 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-roles-20260921/REPORT.md)에 있다.

**2026-09-21 v0.3.1 개발 묶음 2 — 미출하.** native 턴 영수증을 턴별 파일과 완료 표시로
기록하고, 한 요청의 선택·취소·실패 기록이 같은 턴을 사용한다. 재개 시 이전 턴의 복구 여부와
native 모델·effort를 초기화하며 검증된 continuation은 보존한다. 세션 전용 plugin/PDF 임시
폴더는 종료·drain·최종 진단 뒤에 정리하고, 정리 불확실성과 실패를 별도로 보고한다.
실행 근거와 한계는 [v0.3.1 두 번째 묶음 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-events-20260921/REPORT.md)에 있다.

**2026-09-21 v0.3.1 개발 묶음 3 — 미출하.** projects 루트의 실제 부재와 파일·접근 오류를
공통 경로에서 구분한다. 첫 실행의 context 복원과 metadata 재시도는 유지하고, 루트 접근
실패를 모르는 역할로 집계하지 않는다. [v0.3.1 세 번째 묶음 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-projects-20260921/REPORT.md).

**2026-09-21 v0.3.1 개발 묶음 4 — 미출하.** 설정·역할 탐색의 옵션 값과 `--` 경계를
공유하고, 설정 옵션의 자리를 유지하여 앞 옵션이 뒤 프롬프트를 흡수하지 않게 한다.
권한 우회 옵션의 기존 전역 거부와 실제 설정 옵션의 병합을 CLI 계약에서 구분했다.
[v0.3.1 네 번째 묶음 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-argv-20260921/REPORT.md).

**2026-09-21 v0.3.1 개발 묶음 5 — 미출하.** 시작 시 세션 등록이 누락되면 프롬프트 제출
hook에서 재확인하여 복구한다. 등록 실패가 계속되면 안내와 함께 해당 프롬프트를 막는다.
요청 분류 헤더 누락은 역할 선택 전에 진단하며, 버전 일치와 필요한 기능을 구분한다.
[v0.3.1 다섯 번째 묶음 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-session-20260921/REPORT.md).

**2026-09-21 v0.3.1 개발 묶음 6 — 미출하.** 검증된 자동 압축만 기존 모델에서 effort를
medium 이하로 제한하고, 세션의 원래 선택과 이후 생성 effort를 보존한다. 수동 압축은
원래 effort를 유지한다. 압축 계수와 생성의 모델·지침·영수증 제거 경로를 맞췄다.
실제 backend의 high → medium → high, 계수 일치, 필수 정보 보존을 확인했다.
[v0.3.1 여섯 번째 묶음 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-compaction-20260921/REPORT.md).

**2026-09-21–22 v0.3.1 통합 검수 — 미출하.** 실제 native `-p`에서 답변 뒤의 reasoning 블록으로
최종 출력이 비는 결함을 재현했다. 검증된 SDK 경로에서도 reasoning 다음에 최종 텍스트를
전달하여, 실제 backend의 병렬 자식 결과와 같은 세션 재시작 출력을 확인했다.
도구 결과 이미지의 warmup 계수 불일치는 Luna에서도 재현되어 기존 미지원 처리를 유지한다.
승인된 임시 profile의 실제 TUI에서 수동 압축·사실 보존·Esc 취소·같은 세션 복구를 관측했다.
취소 집계의 검수 오류를 수정했으나 요청 상한을 소진해 수정 후 실호출 재실행은 하지 않았다.
v0.3.0의 새 선택 journal 거부, 최초 검수 실패와 보조 요청 거부를 포함한 범위는
[통합 검수 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-integration-20260921/REPORT.md)을 따른다.

**2026-09-22 무과금 보완 — 미출하.** 검증 예산·라우트 거부를 400과 고유 오류명으로 구분했다.
도구·검색·자식·부모 ID가 없는 루트 `auxiliary` 요청은 독립적인 보조 요청으로 처리하며,
대화 턴 게시 중에도 그 턴의 선택·취소 기록을 소유하지 않는다. 자식·도구·일반 대화 검증은 유지한다.
누적 집계와 native transcript의 사실·순서를 함께 검사하는 실제 TUI + 로컬 합성 응답은 PASS했다.
AdGuard 활성 상태의 raw TCP reset은 재현됐다. 승인된 standalone HTTP 검사도 Node의
`ECONNRESET`과 .NET의 `RESPONSE_TRUNCATED`로 실패하여 전체 무오류 판정은 HOLD다.
실호출을 추가하지 않은 범위와 실패 기록은 [무과금 보완 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-offline-20260922/REPORT.md)을 따른다.

**2026-09-22 실제 backend 재검증 — 미출하.** 자동 압축 effort 복귀·계수·사실 보존과
실제 TUI의 압축·Esc·같은 세션 복구 검수가 통과했다. 사용자가 AdGuard 보호를 켠 상태의
실제 TUI도 PASS다. 동일 Node 31개·.NET 35개·raw TCP 400개 검사는 보호 Off에서 모두
통과하고 On에서 다시 실패했다. 특정 보안 제품에 의존하는 제품 수정은 하지 않았다.
보호 On의 독립 전송 반례 때문에 요청된 전송 검사 전체 판정은 HOLD다. 도구 결과 이미지의
warmup 2,828 / usage 3,525 불일치는 기존 명시적 계수의 미지원 범위이며 일반 생성·압축의
합격 조건이 아니다. 이 두 경로는 원격 사전 계수 없이 실제 backend usage를 사용한다.
[실호출·보호 대조 및 판정 정정](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-recheck-20260922/REPORT.md).

**2026-09-22 종료 순서 후속 검수 — HOLD 유지.** 채택된 usage 정책을
[ARCHITECTURE.md](ARCHITECTURE.md) 7.1절에 명시했다. 보호 On의 직접 Winsock 대조에서
상대가 종료하지 않았는데도 송신 전용 shutdown 뒤 수신 EOF가 발생했다. 특정 driver 내부
위치까지 확정하거나 수정한 것은 아니다. 기존 Node·.NET 검사는 다시 실패했고 raw TCP는
22/400 실패했다. 실제 backend TUI는 압축·중단·같은 세션 복구·정리까지 PASS했다.
[계측·재검증 근거와 남은 조건](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-close-20260922/REPORT.md).

**2026-09-20 S48 개발 보완.** settings 병합, Workflow 도구 제한·명시적 모델의 커스텀 역할,
native 역할의 `maxTurns`, 빈 종료 결과 통지, non-streaming JSON 응답을 구현했다.
최종 제품 source `313a78c`, 실제 TUI UUID `4ce18162-9c5f-4c1d-9647-df0c2da00ee2`의
복합 시나리오와 후속 대화를 확인하고 개발용 바이너리에 반영했다.
새 범위의 실제 검사·실패 기록·필터 한계는 [S48 보고서](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/support-expansion-20260920/REPORT.md)를 따른다.
S49 제품 source `31ff118`은 수거 완료 후 동일 세션의 Workflow 근거 복원,
`scriptPath`·로컬 named 파일, custom 역할 기본 선택을 보완한다. 실제 TUI의
동일 UUID 종료·재시작에서 완료 결과 회수와 미실행 단계만 재개를 확인했다. 범위와 검증은
[S49 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/workflow-completion-20260920/REPORT.md)을 따른다.
사용자 S44의 복원·중첩 위임·압축·취소 후 회복 관측에 이어, 같은 제품/native 조합의
보충 TUI에서 B 실제 OS 실행 중 취소·프로세스 회수와 재시작 후 A 재사용/B 재실행 0/C만
실행을 확인했다. **합의한 지원 범위의 S44 수용 및 v0.3.0 기능 출시 적합 판정은 합격**이다.
원래 S44의 미실행 Esc 세부 단계를 수행했다고 소급하지 않는다. 근거·선행 시험 실패·한계는
[B OS 최종 수용](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/workflow-completion-20260920/S44-B-OS-ACCEPTANCE.md)을 따른다.
2026-09-21에는 승인된 Process 범위 RemoteSigned로 Windows PowerShell 5.1과 PowerShell 7의
공개 파일·자식 실행, TaskStop 회수, 후속 응답과 영구 정책 불변까지 확인했다.
[PowerShell 보충 검수](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/workflow-completion-20260920/S44-POWERSHELL-ACCEPTANCE.md).
S47 출하 판정을 이후 확대 범위 전체의 합격으로 승계하지 않는다.

**S47 후보 이력.** 제품 source는 `442c366`이며 당시 출시 판정과 지원 범위는
[최종 후보 보고서](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/release-candidate-20260920/REPORT.md)를 따른다.
사용자가 일반 생성의 정확 사전 차단 보장을 철회하고 실측 usage 기록·예방 압축을
채택한 구현은 [S46 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/release-repair-20260920/REPORT.md)에 있다.
이전 제품 source `3df87d3`의 실제 혼합 PDF
Read TUI에서 사전 계수 15,136 / backend 16,554로 `COUNT_INPUT_MISMATCH`가 발생했다.
같은 이미지의 tool-output 계수 경로에서 차이 1,418을 독립 재현했으며, 이미지/PDF
전체의 정확 계수 지원 주장을 할 수 없다. 부분 도구 인자 Esc는 실제 TUI에서 회복까지
확인했고, Windows 소켓 반례는 사용자 AdGuard 필터 비활성화 전후 대조로 간섭을 확인했다.
실패의 원래 근거는 [S45 조사 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/release-investigation-20260920/REPORT.md),
필터 활성 취소·종료·화면 표시 대조와 최초 Workflow 수정 실패는
[S47 조사 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/release-final-20260920/REPORT.md)에 보존한다.

## 0. 기준과 판정 방법 — 2026-09-20 (실행: 2026-09-19)

| 구분 | 확인한 기준 |
|---|---|
| 개발 바이너리 | 제품 commit `31ff1184c21d7dac0fccd03394081aacd78b9db5`. [빌드 신원](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/workflow-completion-20260920/build.json), [개발 경로 반영](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/workflow-completion-20260920/promotion.json). 설치 명령으로 받은 release와 구분 |
| 최근 실제 TUI | Claude Code `2.1.282`, Windows amd64, Go 1.27.1, CGO_ENABLED=0. v0.4.2 개발본(`bf68243`), gpt-6-luna/low 실제 backend 5회, 2026-09-25: 생성·압축·취소·복구·종료 PASS(공개 `go/cmd/ptydrive`로 조작). 그 전: v0.4.1 개발본(2.1.281), 2026-09-24 같은 항목 PASS. 이전: `2.1.280`, v0.3.2 후보, 2026-09-23 [TUI 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v032-tui-20260923/README.md) |
| 인자 표·native fixture 재측정 | Claude Code `2.1.282`, 2026-09-25(v0.4.2). `--help`와 모듈이 쓰는 plugin 이벤트 타입은 2.1.281과 같다. 설치 native를 쓰는 fixture 검사와 첫 출력 전·뒤 무출력 7분 검사 통과(로컬 backend, 과금 없음). 이전 기준: 2.1.281 |
| 이전 근거 | 2.1.275 등에서 수행한 검사는 해당 버전·빌드의 근거로 보존. 최신 버전의 재검증으로 승격하지 않음 |
| 최근 검사 | S49 전체 회귀 17 packages/1,668 통과/3 skip. gateway race 567개, native Workflow/settings race 21개, 최종 부모 low 조건의 Workflow 105개 통과, vet exit 0. [검사 이력](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/workflow-completion-20260920/evidence.json), [TUI 검수](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/workflow-completion-20260920/REPORT.md). 최초 실패는 보존 |

**구현 여부, 현재 실행 조건 확인, 실제 TUI 검증은 서로 다른 판정이다.**
현재 `gateway.client.verified`는 관측 버전과 기준 버전의 문자열 일치만 뜻한다.
기능 전체 검사나 무결점 보장이 아니다. [실제 판정 코드](../../go/internal/gateway/diagnostics.go)

v0.3.1 개발 빌드의 `gateway.client.requestClassRequired`는 context policy의 헤더 요구이고,
`requestClassMissing`은 그 헤더가 없어 거부한 요청의 누적 횟수다. 헤더를 보내지 않는
client는 제품 생성 경로를 사용할 수 없다. `verified=true`도 이 요구를 대신하지 않는다.

- **범위 내 TUI 확인:** 아래에 특정 입력·시나리오·버전의 실제 실행 근거가 있다.
- **이전 실측:** 과거 빌드에 실제 근거가 있지만 이번 최종 TUI에서 다시 시험하지 않았다.
- **미검증:** 필요한 실제 근거가 없다. 코드 존재나 단위 검사만으로 승격하지 않는다.
- **미지원:** 현행 코드가 해당 경로를 처리하지 않거나 명시적으로 거부한다. 구현 불가능과 같은 뜻이 아니다.
- **정책 확정·구현 대기:** 사용자가 선택한 설계이며 현재 바이너리의 기능으로 표시하지 않는다.

### 지원 범위 재확인 — 2026-09-20

**Anthropic 서버 의존 명령을 제외한 모든 기능을 완벽 지원한다는 판정은 아니다.**
S48 코드와 실행 근거를 재대조했다. 위에서 수용한 native 표시·멀티모달·종료 환경
제한 외에도 다음 조건이 남는다. 이번 대조는 모든 slash command를 실제 TUI에서 다시
실행한 전수 검사가 아니다.

| 서버 전용 기능을 제외해도 남는 조건 | 현재 구현 |
|---|---|
| CLI 옵션 | `--settings` JSON/파일을 필수 settings와 병합하고 `--setting-sources`는 native로 전달. 필수 연결·hook 충돌은 거부. 권한 우회 CLI 옵션 2개는 계속 거부. [설정 병합](../../go/internal/app/user_settings.go) |
| API 요청 형태 | `stream:false`와 생략은 완료된 JSON 응답을 반환. malformed stream 값은 거부. `temperature`, `top_p`, `stop_sequences`는 미지원. [요청 decoder](../../go/internal/protocol/anthropic/request.go). 자동 fallback 재생성은 계속 비활성 |
| Workflow 범위 | inline, native Read로 읽은 `scriptPath`·프로젝트/사용자 named `.js`, custom 역할 기본 선택, `pipeline`·중첩 `parallel` 콜백 지원. 정상 종료/수거 완료 후 같은 세션의 기록 복원과 독립 계획의 미실행 단계 재개. `maxTurns`는 native 역할 정의에 지정. 자식 안의 별도 Workflow 및 임의 JS 재실행은 별도 제한 |
| 부모·빈 응답 대기 | 확인된 TUI 회차는 무출력 대기. SDK/`-p`의 검증된 빈 대기·알림 응답은 Clauduct 상태 메시지로 전달하며 실제 본문·도구는 보존. 상태 메시지는 자식 결과나 업무 완료가 아님. [실제 필수 조건](../../go/internal/gateway/features.go) |
| 새 모델·새 명령·외부 확장 | 현재 모델 카탈로그와 검증된 요청 형식 범위만 지원. 새 모델, native 버전, plugin/MCP 조합의 성공을 자동 승계하지 않음. [모델 카탈로그](../../go/internal/protocol/bridge/route.go) |
| native 전역 effort 고정 | `CLAUDE_CODE_EFFORT_LEVEL`은 native의 명시적 자식 effort·피커보다 우선할 수 있음. S49 실제 영수증으로 재확인. 부모 시작 기본값에는 `--effort` 사용. 전역 고정과 자식 선택이 충돌하면 선택 검증을 우회하지 않음 |

인자를 native로 전달하거나 메뉴가 나타나는 것은 그 명령의 모든 후속 경로를 검증했다는
뜻이 아니다. `gateway.features`도 계측한 기능군의 조건 관측이며 전체 slash command
합격 목록이 아니다. 공식 명령은 로컬 UI, 모델에 전달되는 skill, 동적 Workflow가 섞여 있고
사용 가능 여부도 환경에 따라 달라진다. [공식 명령 구분](https://code.claude.com/docs/en/commands)

필터 문제에서도 **임의 필터가 통신 경로 전체를 계속 차단하는 동안 전달 성공을 보장할 수
없다는 한계**와 **특정 필터 호환 문제는 더 수정할 수 있다는 가능성**을 구분한다.
현재 남은 즉시 소켓 종료 반례를 근본적으로 해결 불가능하다고 확정하지 않았다.
취소·회수의 제품 보완 및 반례는 [S47 조사 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/release-final-20260920/REPORT.md)을 따른다.

**AdGuard localhost 필터링(2026-09-23).** AdGuard for Windows 8.0.5570에서는
[로컬 호스트 필터링](https://adguard.com/kb/ko/adguard-for-windows/settings/app-settings/advanced-settings/)이
위 즉시 반닫기 응답 유실의 조건이었다. 전체 보호를 켠 채 이 설정만 끄자 공개 반닫기 probe와
`TestRuntimeEvidenceImmediateFINReply`가 각각 20회 중 응답 0회에서 20회 모두 정상으로 바뀌었다.
`claude.exe`의 앱별 라우팅·트래픽 필터링을 켠 상태에서 v0.3.1 후보 `ad01745`의 실제
`claude.exe` → Clauduct → backend 왕복도 `gpt-5.6-luna`/`low` 1회로 통과했다.
한 PC·한 버전의 결과이며, 켠 상태의 실제 요청 비교와 설치 바이너리·장기 안정성은 검증하지 않았다.
[전후 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-localhost-filter-20260923/README.md)

다른 필터 제품이 loopback 통신을 검사·중계한다면 그 기능만 제외한 뒤 **같은 반닫기 probe와
실제 요청을 전후 비교**한다. 설정 이름과 적용 범위는 제품·버전마다 다르므로 기능을 껐다는
사실만으로 응답 유실이 해결됐다고 판정하지 않는다. localhost 제외는 그 제품이 검사하지 않는
로컬 통신 범위를 넓히므로 적용 범위를 확인한다.

### 현재 거부와 구현 가능성은 별개

다음은 S48 구현과 남은 구현 방향이다. 범위별 근거를 따르며 거부문 삭제만으로 지원을 선언하지 않는다.

| 항목 | 거부 근거와 구현 가능성 |
|---|---|
| non-streaming API | 구현. 동일 backend SSE를 한 번 소비해 검증된 완료만 JSON으로 반환. 16 MiB/1,024 block 상한, 실패 시 부분 성공 미반환. 4개 모델의 공개 live 요청에서 text/usage/완료 확인. 도구·opaque reasoning·검색 결과는 HTTP/프로토콜 회귀로 확인 |
| Workflow `tools` | 구현. raw agent와 계획 step 모두 최대 64개 정확 이름 목록 또는 빈 목록. native 도구 목록과 교집합이며 과거 도구 기록이 새 호출 권한이 되지 않음. 지연 발견 도구가 필요하면 `ToolSearch` 자체도 허용 목록에 포함해야 함 |
| Workflow `maxTurns`, custom `agentType` | 역할 지침·도구·`maxTurns`는 native 유지. S49는 모델 생략 시 역할 정의와 실제 native turn을 대조한다. native metadata의 빈 model은 요청에서 모델을 생략했다는 뜻일 수 있으므로 실제 turn과 구분. 직접 `agent(...,{maxTurns})`는 native가 적용하지 않아 거부하며 역할 정의를 사용 |
| named/scriptPath Workflow | S49 구현. native `Read`의 권한·훅·실제 전체 결과를 통과한 텍스트에만 adapter 적용. `scriptPath` 우선; named 파일은 cwd부터 Git root까지 `.claude/workflows` 및 profile의 `workflows` 순서로 탐색. 최대 500 KiB, native Read 부분 결과는 거부. plugin/bundled named resolver 전체와 동일하다는 주장은 하지 않음 |
| launcher 재시작 후 Workflow 복원 | S49 구현. native 수거 후 본문 없는 메타데이터 저장, 같은 UUID의 SessionStart 경로에서만 복원, native script/journal/metadata 해시와 선택·종료 근거 재검증. 독립 계획 재개는 디스크의 배타적 claim으로 중복 실행 방지. 강제 종료로 최종 근거를 저장하지 못했거나 기록이 변경되면 재개 거부 |
| `--settings` | 구현. 최대 2 MiB JSON 객체/일반 파일, 중복 키 거부, 마지막 옵션 우선. 사용자 hook을 보존하고 필수 binding 추가. 연결·필수 정책 충돌은 조용히 덮지 않고 거부 |
| `--setting-sources` | native user/project/빈 source 적용을 공개 fixture와 실제 native CLI로 확인. 필수 CLI settings는 별도로 유지 |
| 권한 우회 CLI 옵션 2개 | 기술적 불가능이 아니라 명시적 제품 정책 제한이다. 지원 여부 변경과 현재 작업의 권한·guard 준수는 별개 |
| `temperature`, `top_p`, `stop_sequences` | 입력층 거부 유지. S48에서 4개 모델 × baseline/temperature/top_p/max_output_tokens/stop = 20개 공개 요청 비교: baseline 4건 성공, 추가 필드 16건 HTTP 400. `stop` 시험을 `stop_sequences` 필드 자체의 실측으로 부르지 않음. 일반 API 문서 지원이 구독 backend 지원을 뜻하지 않음 |

`max_output_tokens`는 위 세 파라미터와 달리 기존 backend HTTP 400 실측이 있다.
출력 문자열을 잘라내는 것으로 서버의 생성 토큰 제한이나 sampling 제어와 동등해지지 않는다.
[출력 상한의 기존 실측](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/VALIDATION.md), [현재 변환 계약](../../go/internal/protocol/bridge/bridge.go).

native 공식 Workflow 재개는 중단·실패한 agent를 다시 실행할 수 있다. Clauduct의
현재 정책은 시작한 단계의 자동 재실행을 금지하므로 native 재개와 완전히 같은 의미가 아니다.
동일 의미로 바꾸려면 재실행 정책도 별도로 정해야 한다. 실행 여부를 확인할 수 없는
외부 효과에 대해 기록·멱등성 협력 없이 무조건 중복 없는 재실행을 보장하지 않는다.
[native 재개 의미](https://code.claude.com/docs/en/workflows#resume-after-a-pause),
[settings와 setting-sources의 구분](https://code.claude.com/docs/en/cli-reference#cli-flags).

### 채택한 버전 변경 정책 — 기능별 근거 집계 구현

버전 번호만으로 전체 실행을 허용하거나 차단하지 않는다. 해당 기능의 필수 조건을 확인해
실행 여부를 정하고, 조건이 깨진 기능만 차단한다. 세션 전체에 필요한 조건이 깨지면 세션을 중단한다.
`34ebcb1`은 기존 decoder·선택·회차·정확 계수 검사의 실제 통과 지점을 `gateway.features`에 모은다.
최근 16건 외의 세션 전체 누계도 유지한다. 과거 성공이 미래 요청을 허가하지 않으며,
TUI 판정은 계속 `not_assessed`로 구분한다. native 회차 영수증이 설정됐는데 없으면 자식 실행을 거부한다.
버전 일치 필드는 `verifiedMeaning:version_match_only`로 명확히 했다.
제품 반영과 실제 검증 범위는 [후속 검수 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/capability-repair-20260919/REPORT.md)을 따른다.
다음 설계의 코드·공식 문서·native TUI 근거와 반례는
[설계 검토 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/design-evidence-20260919/REPORT.md)에 있다.
빈 응답 제어는 native 출처·회차와 대기 자식, 직전 Workflow 실행 또는 루트 완료 알림을
확인한 범위에 적용한다. TUI는 무출력 대기, SDK/`-p`는 `[Clauduct]` 상태 메시지를 사용한다.
SDK는 실제 본문을 억제하지 않는다. 새 명시적 사용자 입력·분류되지 않은 출처에는 적용하지
않으며, 실제 결과 수신은 `parent_received`로 별도 확인한다. 이번 실제 backend에서 정상
terminal 뒤 `EMPTY_REPLY`를 관측했고, 고정 fixture와 구독 backend 검증을 각각 기록했다.
근거는 [`verification/v031-review-fixes-20260922/batch-04/REPORT.md`](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-review-fixes-20260922/batch-04/REPORT.md)에 있다.

| 기능군 | 실행 전에 확보해야 할 조건 | 실행 중·후에 확인할 사항 |
|---|---|---|
| 일반 생성 | 지원 입력 형식, 확정 모델·effort, 모델별 예방 압축 정책과 agent 식별 | backend input/output usage 기록, 미확보는 unknown, 전달·취소 분류 |
| native Agent | 역할과 원래 지정 여부, 확정 선택, session/child/parent 및 현재 회차 연결 | 완료·실패·취소 이벤트, 결과 존재와 부모 수신 |
| 자동 재진입·SendMessage 재개 | 원래 계보·선택과 새로운 실행 회차의 연결 | 이전 회차의 늦은 이벤트·결과 혼입 방지 |
| Workflow | 실제 script/역할/선택과 run/child 식별 연결 | native journal의 해당 결과, 결과 회수와 재실행 구분 |
| 명시적 계수 | 독립적으로 검증한 모델·입력·계수 경로 | 계수 요청만 성공/미지원/실패 판정. 일반 생성과 분리 |
| 예방 압축 | 사용량 anchor·명시적 추정, 모델별 관리 목표, 압축 대상 연결 | 압축 완료·요약 재평가·재개 순서, 실패·반복 overflow 시 해당 생성 차단 |
| `/context` 입력 제외 | native 기록의 출처·session·명령/출력 연결 | 일반 사용자 입력을 보존했는지, 새 형식의 출처를 검증할 수 있는지 |

미래 응답의 유효성은 실행 전 검사만으로 증명할 수 없다. 실행 후에만 알 수 있는 조건은
요청·도구 전달·완료 판정의 해당 경계에서 검사한다. 일부 조건 통과를 전체 호환으로 표시하지 않는다.
필수 조건을 알 수 없으면 추측으로 통과시키지 않고 영향을 받는 범위와 이유를 남긴다.

## 1. 구현된 기능과 근거 범위

| 기능 | 근거 |
|---|---|
| 스트리밍 대화·도구 왕복·도구 결과 재실행 방지 | TOOL01/02/06, 실클라이언트 |
| 이미지 입력 | A1, 실백엔드 왕복 |
| **PDF 입력**(`document` 블록 → `input_file`) | 직접 document 변환과 native Read 페이지 이미지 경로를 구분. native 2.1.278의 혼합 PDF(텍스트·표·raster chart·vector diagram·수식) 3페이지는 S46/S47 실제 TUI에서 내용 확인. S45의 tool-output 이미지 계수 불일치는 남아 있으며 해당 warmup 계수는 미지원 |
| 대화 중 도구 추가/삭제(`tool_addition`/`tool_removal`) | A4a, 베타 게이트 포함 |
| 추론 왕복(`redacted_thinking` ↔ `reasoning.encrypted_content`) | A4b, 실백엔드가 자기 기록을 되받음 |
| hosted **WebSearch** | A2 + 2026-09-17 실백엔드 재확인(32,060 bytes, 20 links) |
| **구조화 출력**(`output_config.format` → `text.format`) | 2026-09-17. 그 전까지는 검증만 하고 버렸다 |
| MCP 서버(`--mcp-config`) | G5. 서버가 뜨고 툴이 제공되고 **상속 환경이 살아남는다** |
| `--resume` · `--permission-mode` · `--worktree` · `--plugin-dir` · `--bare` | G5, 전부 동작으로 측정 |
| 서브에이전트 역할·모델·effort 선택 | 원래 지정 여부와 native 자식 식별을 대조. built-in 역할의 알려진 표기 차이와 `subagent_type` 생략 처리. 아래 최근 TUI 범위 참조 |
| 위임 메뉴 5종·모델 표 | v0.3.4부터 `clauduct-<model>` 4종 + `clauduct-inherit`. effort는 Agent `effort` 인자로 받고 없으면 모델 기본값. [카탈로그 기반 생성](../../go/internal/app/agents.go). 표는 fable→gpt-6-astra, opus→gpt-6-sol, sonnet→gpt-5.6-terra, haiku→gpt-6-luna. gpt-5.6-sol·gpt-5.6-luna와 v0.3.3의 `clauduct-<model>-<effort>` 이름은 `MODEL_RETIRED`로 거부하며 대체 실행하지 않음. Codex 카탈로그의 `ultra`는 2026-09-24 측정에서 gpt-6-sol·gpt-6-astra·gpt-5.6-terra 모두 HTTP 400이라 어느 모델에도 노출하지 않음. 메뉴 존재는 모든 모델의 최신 TUI 통과를 뜻하지 않음 |
| 중첩 Agent 자동 재진입 | 원래 계보와 현재 native turn을 확인한 뒤 확정 선택 유지. 최근 TUI에서 ROOT → A → B → C의 완료 결과 전달 확인 |
| 완료 Agent의 SendMessage 재개 | 최근 TUI에서 동일 child ID의 Sol/high 유지·두 번째 결과 수신 확인 |
| inline Workflow의 자식 선택 | model+effort / model만 / effort만 / 둘 다 생략을 runtime 선택과 child ID에 연결. 최근 TUI 네 자식 병렬 실행 확인 |
| Agent 결과 회수와 Workflow StructuredOutput | 일반 결과와 검증된 native journal 결과를 부모에게 전달. 범용 Workflow 재실행·복구 기능은 아님 |
| 모델 피커 + `GET /v1/models` discovery | A3 + B1 |
| 모델별 실측 사용량·예방 압축 | Astra 500K/450K, Sol·Terra·Luna 272K/239K는 관리 목표이며 정확 사전 차단 상한이 아니다. 기존 확정 모델을 유지하며 v0.3.1 개발본은 검증된 자동 압축만 medium effort 상한을 적용한다. 이후 생성·수동 압축은 원래 effort 유지. native 공통 표시/환경 기본값은 500K. 새 대용량 입력의 최초 초과 가능성이 있으며 추정과 실제 usage를 구분 |
| `POST /v1/messages/count_tokens` | 검증 범위의 로컬 텍스트 계수 또는 구독 backend `generate:false`, 동일 입력의 실제 usage 캐시. 도구 결과 안의 이미지/PDF warmup은 S45 불일치로 미지원. 일반 생성의 필수 조건이 아님. 이전 근거: [COUNT-TOKENS.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/policy-evidence-20260918/COUNT-TOKENS.md) |
| 진단(`GET /clauduct/status`)·종료 요약·상태 파일·rate limit 헤더 관찰 | D1–D6 |
| 취소·프로세스 트리 정리·동시 세션 격리 | LIFE·REL 계열 |
| **`clauduct --update`** | 태그 릴리스의 `clauduct.exe`를 SHA256SUMS로 검증한 뒤 교체하고, 0.3.x가 남긴 `clauduct-hook.exe`·`clauduct-dev.exe`를 지운다. 확인을 받고, `--yes`로 무인. `clauduct update`는 그대로 통과해 **클라이언트**를 갱신한다 |
| **`clauduct --usage`** (= `clauduct --dev --usage`) | 이 **계정**의 주간/보조 한도 사용률·리셋·in force family를, 세션이 남긴 계정에서 읽어 보여준다. 요청 0회 |
| 종료 줄의 `quota=47%/7d` | 묻지 않아도 매 세션 보인다. 백엔드가 응답 헤더로 말한 값 |
| Esc 이후 같은 세션 회복 | 정확 계수 중·부분 텍스트 출력 중·도구 전달 전 생성 취소 후 회복 확인. S45에서는 부분 인자 delta 563개/본문 0개 상태 취소, tool 실행·파일 생성 0, 같은 세션 후속 답변을 확인 |
| `/context` 보고서의 실제 backend 입력 제외 | 검증된 native transcript 출처가 있는 조회 기록만 제외. [이전 수리 근거](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/context-fork-20260919/REPORT.md). native 화면·로컬 이력 추정치는 변경하지 않음 |
| 클라이언트 버전과 기능 조건 | `gateway.client`의 버전 일치와 `gateway.features`의 요청별 필수 조건 관측을 구분. 미관측을 통과로 승격하지 않음 |
| 본문 없는 진행 관측 | `gateway.progress`, 요청의 `lastObservedMs`와 고정 backend delta 수. 오래된 관측은 생존 증명이 아니며, 승인 요청 횟수는 현재 승인 대기를 뜻하지 않음 |
| Workflow 완료 결과 회수 | 같은 세션의 검증된 원본 run에 `resumeFromRunId` 하나만 전달. S49부터 launcher 수거 후 저장된 근거를 재검증해 재시작 뒤에도 지원. 기존 자식 보고서를 반환하고 미확보는 명시. 새 자식/원본 스크립트 실행은 하지 않음 |
| 부모 입력의 완료 조건·대기 | 해당 요청에 실제 포함된 자식 결과 ID와 아직 대기/미확보인 ID를 `parentReadiness`에 기록. 알려진 자식 보고서가 모두 입력에 있는 조건이며, 내용의 타당성·업무 성공 판정이 아님 |
| native TUI 빈 응답 대기 | 확인된 위임 후 회차·루트 완료 알림에서 미완료 자식이 있으면 일반 본문/빈 본문을 대기로 처리. 별도 모델 상태 확인 요청 없이 native 완료 이벤트로 재진입. 새 사용자 요청·Read 왕복 보존 실측 |
| native SDK 빈 응답 처리 | 확인된 대기·Workflow 실행 직후·루트 완료 알림의 빈 응답에 출처가 명시된 상태 메시지 전달. 실제 답변·도구와 native background scheduler 보존. 빈 완료 알림과 중간 SDK 오류까지 실제 native fixture로 검사 |
| 독립 단계 계획의 미실행 단계 재개 | `script:"clauduct:plan-v1"` + `args.steps`. 같은 세션에서 TaskStop·자식 종료 또는 수거 후 저장된 근거를 재검증한 원본에 `resumeFromRunId`만 전달. 완료 결과 재사용, 시작한 단계 재실행 금지, 시작하지 않은 단계만 실행. 디스크 claim으로 launcher 재시작 후에도 원본 run당 한 번만 소비 |
| 독립 계획의 무도구 단계 | step에 `tools:[]`를 지정하면 검증된 자식의 backend 도구 목록과 downstream callable set 모두 제한. 생략하면 기존 native 도구 유지. 재개에도 동일 제한 유지. `toolPolicy:workflow_step_none`와 `workflow_tool_policy` 기능 누계로 관측 |
| 필터가 소켓 취소를 지연시키는 경우 | exact session/agent/turn의 native abort·error·refusal 종료 기록으로 추론·검색 취소. 다음 turn에도 남는 기록으로 이전 요청을 정리하며 성공 응답·다른 turn은 취소하지 않는다. [내장 필터 검증](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v031-httpguard-20260923/REPORT.md). 기존 실제 TUI의 부분 도구 인자 Esc·후속 답변 근거와 최종 출하물 검증은 구분한다 |

A/G/LIFE 등의 식별자는 [이전 검증 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/VALIDATION.md)의 범위를 가리킨다.
그 행을 최근 빌드에서 모두 다시 실측했다는 뜻은 아니다. 이미지·PDF·MCP·WebSearch 등은
이전 실측을 보존하며, 전체 형식·환경·plugin 조합의 보장으로 확대하지 않는다.

### 부모 대기·계획 재개의 실제 근거

사용자 UUID `8544df8b-bc32-4df0-bffe-fce86176e3e1`에서 새 입력이 LEAF 도구 완료 전에
제출됐고 Read 답변과 뒤이은 중첩 결과 43, 병렬 중첩 합계 180을 확인했다. 계획 재개는
A 재사용/B 미재실행/C Luna/max·도구 없이 완료 및 중복 거부 후 회복을 확인했다.
수동 압축 2회는 약 70초/80초이며 모델 변경 직후 일반 요청 없이 한 두 번째 압축도 기존 Sol/high를
유지했다. 이후 Terra/high 대화와 보존값 회수는 정상이다. [실제 기록 감사](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/parent-wait-20260920/s43-user-audit.json)

S43의 두 Esc는 모두 부분 본문 뒤여서 첫 본문 전 취소 재검증으로 세지 않는다. B의 Bash 결과는
`User rejected tool use`로 OS 명령 실행 후 중단 여부가 미확정이다. 자연 발생 빈 응답은 0회이며,
정상 본문이 한꺼번에 보인다는 관측의 실제 화면 표시 시각도 확보되지 않았다. 초기 `/context`는
10.1K가 세 번 유지됐지만 최초 backend 계수의 약 5초 구간은 남는다. API 실패 0/계수 59건 일치를
이 미검증 조건의 해결로 확대하지 않는다. deadline 최초 실패와 후속 보완은 아래 별도 근거를 따른다.

### Deadline 후속 수리와 실제 실행 확인

`53482a4`에서 deadline 테스트의 가짜 프로세스가 HTTP 요청 종료 전에 회수 완료를 반환하던
소유권 오류를 수정했다. 80회 grace 만료 검사와 실제 native TUI의 유예 내 응답/유예 만료 종료를
확인했다. Windows 원격 TCP close 통지의 별도 반례를 제품에서 해결했다고 주장하지 않는다.

실제 backend에서 첫 본문 전 Esc 후 새 입력 회복을 확인했고, B의 OS 프로세스 시작과
TaskStop 후 종료를 확보했다. Workflow 재개 자식이 부모의 조정 지시를 자기 일로 해석한 실패는
검증된 계획 자식의 작업자 역할 설명으로 보완했다. 이후 A 재사용/B 미재실행/C Luna/max·도구
0회·지정 결과 반환을 확인했다. 설명은 정확 계수에도 포함하며 사용자 제한을 대체하지 않는다.

native hook 없는 도구 검증 오류는 다음 요청의 실제 call/result 쌍으로 추가 집계한다.
`source:tool_result`와 `source:native_failure_hook`를 구분하고 중복 집계하지 않는다.
`3df87d3`는 자식 계수와 생성이 같은 agent ID로 도구 스키마를 구성하도록 추가 수정했다.
최종 바이너리 TUI에서 두 `/context all`의 10.1K 유지, Terra/medium 중첩 상속과 결과 전달,
API/도구 실패 0, 계수 일치 11건을 확인했다.
[원인·실패·검증·한계 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/deadline-repair-20260920/REPORT.md)

[TUI 감사 자료](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/parent-wait-20260920/tui-audit.json)는 실제 구독 backend 실행과
native TUI+고정 upstream fixture를 분리한다. 부모→중간→손자 결과 전달, 대기 중 별도 사용자
Read, 자식 종료와 대기 응답의 경합을 확인했다. 무출력 제어는 native composer/task-notification
출처와 현재 회차를 검증한 범위에 한정한다. SDK·분류되지 않은 입력·출처 없는 자식 새 회차의
첫 요청에는 적용하지 않는다. 추가 상태 확인용 모델 호출을 만들지 않지만 기존 생성·정확 계수
비용이 사라지는 것은 아니다.

계획 재개는 독립된 1~16단계이며 각 단계는 `id`, `prompt`, 선택적 `model`, `effort`, `tools`를 받는다.
`tools`는 빈 배열 또는 최대 64개의 중복 없는 정확한 도구 이름을 지원한다. null·wildcard는 거부한다. 원래 모델·effort·도구 제한은
재개 시 부모 모델이 달라져도 유지한다. 원본 script hash, 관측한 자식 ID·순서,
native journal, TaskStop 영수증과 종료를 대조하고 변경/누락/중복 재개는 거부한다.
빈 결과·중단된 결과는 전체 완료로 올리지 않는다. `completeMeaning`은
`all_step_results_present_not_task_success`이며, `complete:true`도 내용상 성공을 보장하지 않는다.
실제 시험 중 작업자가 부모 지시를 자기 일로 해석하거나 불필요한 도구를 제안한 실패를 보존한다.
따라서 실행·회수 기능의 확인을 모델 지시 준수의 무결점 판정으로 확대하지 않는다.

### S42 이후 변경의 실제 TUI 근거

알 수 없는 Workflow run의 회수 요청을 native 도구 거부로 전달하고, 같은 세션의 다음 일반 답변을 확인했다.
원래 거부된 작업을 실행하거나 API 오류를 성공한 작업으로 집계하지 않는다. `rejectedWorkflowCalls`와
기능별 `rejectedToolCalls`로 실행 거부를 별도 기록한다. 이 경로의 `apiFailures=0`은 도구 거부 0건이라는 뜻이 아니다.
native hook이 실행되지 않는 경우에도 전달된 고정 script는 오류만 발생시키며 원래 작업을 실행하지 않는다.
다른 종류의 검증 오류 전체를 이 경로로 바꾸지는 않았다.

지연 로딩된 Agent를 발견하면 역할 목록이 뒤늦게 추가되는 실제 TUI 근거를 확보했다.
`ToolSearch` 안내를 보완했지만 모델의 모든 역할 선택·대기 판단까지 결정적으로 보장하지 않는다.
압축 요약의 반복 설명을 줄이는 지침은 압축에만 적용하며, 필요한 데이터 보존이 1,200단어 목표보다 우선한다.
v0.3.1 개발 묶음 6에서 `count_tokens`에도 같은 지침을 포함하도록 누락을 수정했다.
일반 생성 모델·effort와 계수 검증은 변경하지 않는다.
세션·선택·결과·압축별 실제 근거와 제한은 [S42 수리 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/session42-repair-20260920/REPORT.md)을 따른다.

### 이전 `34ebcb1`의 실제 TUI 근거

`34ebcb1`의 최종 TUI에서 검증된 Workflow 자식 결과를 회수했고 새 자식 실행은 0개였다.
알 수 없는 run ID를 의도적으로 거부한 1건은 API 실패 누계에 남으며, 이후 같은 세션의 정상 응답을 확인했다.
진행 관측·승인 요청·동일 자식 재개 및 보조 요청 집계 수정은 이번 중간 빌드의 TUI 근거와 구분해
[후속 검수 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/capability-repair-20260919/REPORT.md)에 기록했다.
빈 응답 대기 실험은 native TUI와 로컬 응답 fixture이며 구독 backend 제품 검증으로 표시하지 않는다.

### 이전 `17532f7` 빌드의 실제 TUI 근거

| 시나리오 | 근거 | 판정 범위 |
|---|---|---|
| 중첩 Agent·자동 재진입 | [tree-proof](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/session41-repair-20260919/tree-proof.json) | Terra/medium, 깊이 3 자식 계보·결과 전달. 모든 깊이·역할 조합의 검증은 아님 |
| 병렬 Workflow 네 선택 방식 | [workflow-proof](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/session41-repair-20260919/workflow-proof.json) | inline `agent()` 자식 4개. StructuredOutput 및 native 도구 제한 조건 |
| Sol/high 동일 자식 재개·추가 취소 회복 | [resume-cancellation-proof](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/session41-repair-20260919/resume-cancellation-proof.json) | SendMessage 1회 재개. 추가 취소의 정확한 backend 이벤트 종류는 미확인 |
| 계수·텍스트 중 Esc 회복 | [통합 세션](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/session41-repair-20260919/run-febe7e62-d829-4e63-a275-e167b2e8529f.json) | count/delivery 각각 취소 후 새 요청 완료 |
| 계수 정확도 | [통합 세션](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/session41-repair-20260919/run-febe7e62-d829-4e63-a275-e167b2e8529f.json), [추가 세션](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/session41-repair-20260919/run-fb24a803-a013-45e4-9845-9a4318c67b14.json) | backend usage가 있는 합계 38건 일치. 모든 tokenizer·멀티모달 형식의 증거는 아님 |
| launcher 강제 종료 | [handle 관측](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/session41-repair-20260919/hard-kill-261642e5-d5c9-4a70-8a35-41eb11c1a45a.json) | launcher만 종료 후 하위 6개까지 종료. 콘솔 창 닫기·OS 종료 전체는 미검증 |

이전 `17532f7`의 두 정상 종료 TUI는 70요청, API 실패 0, 의도한 취소 3, 종료 시 결과 미확보 0이었다.
개발 중 실패를 없었던 것으로 취급하지 않는다. `EMPTY_REPLY`, 역할 해석, Workflow 입력·결과 처리의
실패와 수정 근거는 [실패 이력](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/session41-repair-20260919/REPORT.md)에 보존한다.

## 2. 제약

| 항목 | 내용 |
|---|---|
| 소유하는 옵션 **4개** | `--update`(+`--yes`), `--usage`, `--uninstall`(+`--yes`), `--dev`(v0.4.0, 이 빌드 자신의 명령). **첫 인자일 때만** 인식한다 — 프롬프트 안의 같은 문자열이 바이너리를 교체하면 안 되기 때문이다. 그 외 모든 인자는 그대로 전달된다 |
| 거부하는 native 옵션 **2개** | `--dangerously-skip-permissions`·`--allow-dangerously-skip-permissions`(권한). 사용자 settings는 필수 settings와 병합하며 충돌만 거부 |
| 주입하는 것 | settings·위임 메뉴·세션 plugin·모델/도구/압축 관련 환경. 사용자 `--agents`는 메뉴를 대체할 수 있지만, [sessionRequirements](../../go/internal/app/session.go)의 필수 환경값은 사용자 값으로 자동 대체하지 않음 |
| hook이 없으면 | 자식 확정 선택과 native 압축을 검증할 수 없으므로 해당 요청을 거부한다. `hookInstalled`에 기록 |
| non-streaming 요청 | explicit false/생략 지원. 세션 인증·요청 종류·컨텍스트 정책 등 기존 실행 조건은 동일. native의 오류 후 자동 fallback은 중복 생성을 피하려고 계속 끔 |
| 미지 SSE 이벤트 | 요청 실패. 이름만 계정에 남긴다(D6) |
| side query가 아닌 hosted 도구 요청 | `HOSTED_TOOL_UNSUPPORTED`로 거부. 기준선은 조용히 성공시킨다 — 의도적 divergence |
| 사전 출력 토큰 상한 | 기존 실측에서 backend가 `max_output_tokens`를 HTTP 400으로 거부해 해당 방식은 미지원. 완료 후 usage 검사와 구분하며, 미래의 모든 구현 가능성까지 부정하지 않음 |
| 게이트웨이 재시도 | 일반 생성의 자동 재시도와 [WebSearch의 제한된 읽기 재시도](../../go/internal/upstream/search.go)를 구분. 전 경로가 재시도 0이라는 뜻이 아님. backend로 보낸 뒤의 실패는 native가 재시도해도 replay 보호에 막히므로, `X-Should-Retry: false`로 재시도를 막고 원인 범주를 그대로 보인다. backend 실패 이벤트는 고정 어휘로 줄인 code·type·incomplete 사유를 함께 싣는다(v0.3.3, #84). 이전에는 사용자가 원인 대신 `NATIVE_REQUEST_REPLAY_BLOCKED`만 봤다 |
| native context 표시 | 공통 500K 환경과 native 로컬 추정은 모델별 gateway 정책의 적용 근거가 아님. 모델별 정책·실제 계수는 status로 확인 |
| 정확 계수 성능 | 연결 재사용·동일 입력 캐시·동시 요청 공유 구현. 새 입력의 backend 왕복 지연은 남으며 전후 성능 무저하를 입증하지 않음 |
| native 첫 본문 표시 | Clauduct와 hook 없는 native 2.1.278에서도 SSE 진행 중 counter만 증가하고 완료 후 본문이 보이는 현상을 재현. 정확한 screen paint 시각/내부 원인은 미확정. 제품이 native 표시부를 패치하지 않음 |
| `/context` 최초 조회 | 정확 계수의 첫 backend 왕복 지연이 남음. 조회 자체가 모델 본문 생성을 뜻하지 않고, 검증된 조회 기록은 실제 다음 모델 입력에서 제외. native 로컬 이력/추정 표시와 실제 usage는 다름 |
| 런타임 의존 | `claude.exe` + `codex.exe`(버전이 요청 헤더) + `~/.codex/auth.json` |

## 3. 미지원·미검증·구현 대기

| 항목 | 상태 |
|---|---|
| 계수 지원 범위 밖의 입력 | 해당 계수 요청만 명시적으로 실패. 일반 생성·압축은 원격 사전 계수 없이 backend usage와 예방 압축 정책을 사용. 추정값을 정확 계수로 표시하지 않음 |
| forked Skill(`context: fork`) | **구현.** 실제 backend TUI(2026-09-24, luna/low, 파일 skill을 모델이 호출)에서 fork 자식의 검증·실행과 백그라운드 결과 전달을 확인했다. TUI에서 fork는 백그라운드로 돌고, 결과를 기다리는 부모의 빈 턴은 Agent·Workflow처럼 대기로 처리한다. 같은 요청 안에서 성공한 Skill 결과가 백그라운드 fork 시작을 알릴 때만이며, 인라인 skill의 빈 답은 그대로 `EMPTY_REPLY`다. 이 대기는 같은 날 실제 backend TUI에서 오류 없이 확인했다. 내장 `code-review`는 실제 backend `-p`(2026-09-24, luna/low)에서 모델이 불러 fork 자식 요청이 모두 검증·실행되고, 리뷰가 Skill 도구 결과로 돌아오는 것을 확인했다. 모델이 Skill 도구로 부른 fork의 자식은 native가 그 턴에 기록한 모델·effort로 실행한다(선택 출처 `native-fork`). toolUseId 없는 메타데이터, 루트 대화 아래 깊이 1의 general-purpose, skill 본문이 meta 사용자 메시지로 시작하는 transcript가 모두 맞아야 하고(파일 skill과 내장 `code-review` 모두) 하나라도 어긋나면 거부한다. 보고서는 gateway가 중계하지 않고 native가 전달한다(`-p`에서는 Skill 도구 결과, TUI에서는 백그라운드 완료 알림). `-p`에서 직접 입력한 명령은 native가 agent ID 없이 실행하므로 루트 요청으로 처리된다. subagent 안에서 부른 fork와 fork 자식의 `SendMessage` 재개는 여전히 거부한다(2026-09-23, native 2.1.280 fixture) |
| `review-diff` 헬퍼 | 미지원. 기준선의 경로 탈출 방지·2 MiB 상한은 없다 |
| Workflow remote·자식의 별도 Workflow·범용 JS 재개 | native workflow-subagent는 Workflow 도구를 제외한다. 이를 제거해 도구 제한을 확대하지 않음. 임의 JS는 `resumeFromRunId` 단독의 결과 회수, 독립 계획은 미실행 단계만 재개. 다른 세션, 기록 없는 강제 종료, 불명확한 시작 단계의 자동 재실행은 거부 |
| Workflow plugin/bundled 이름 전체 | 로컬 `.js` 이름과 scriptPath는 지원. native 내부 resolver를 우회해 plugin 출처·우선순위를 임의로 추정하지 않음. 확인된 파일은 native Read가 허용하는 scriptPath로 실행 가능 |
| Workflow `agent()`의 직접 `maxTurns` 옵션 | 거부. native 역할 정의의 maxTurns를 사용. `tools` 정확 이름 목록은 자체 강제하며 모든 native 옵션 조합의 적용을 검증했다는 뜻은 아님 |
| PPTX·DOCX·XLSX 직접 입력 | 이미지/PDF API 입력과 별개. 현행 bridge의 직접 document 입력으로 지원하지 않음. 별도 native 도구의 텍스트·페이지 추출 결과를 처리하는 것과 원본 Office 형식 지원을 혼동하지 않음 |
| 임의 Workflow JavaScript 전체 | native `pipeline`·중첩 `parallel`의 콜백에서 `agent()`를 호출하는 형태는 S49 native 검증. 임의 `globalThis.agent` 우회나 native VM가 거부하는 코드까지 지원한다는 뜻은 아님 |
| 빈 응답 제어의 전체 실행 모드 지원 | `composer`·`sdk`에 더해 v0.4.3 개발본은 실측한 `peer`로 모드를 정한다. native `isInteractive`가 TUI·SDK를 구분한다. 별도 처리하는 `task-notification`을 제외한 나머지 origin 12종은 여전히 단독 입력으로 모드를 확정하지 않으며 미측정이다. 같은 턴의 명시적 개입, 출처 없는 자식 새 index 0, 새 입력의 빈 응답은 기존 오류 경계를 유지한다. [v0.4.3 검증 상태](#v043--메시지-background-검증-예산) |
| 부분 도구 인자 생성 중 취소의 이벤트 근거 | S45/S47 실제 TUI + 고정 backend로 부분 인자 delta·미완성 도구 미실행·후속 답변 확인. 구독 backend의 자연 발생 동일 조건 전부를 검증했다는 뜻은 아님 |
| advisor 도구, Anthropic 서버 의존 베타 7종 | 이 backend에서 성립하지 않는다. advisor는 환경변수로 끈다 |
| 클라이언트 `/usage`·`/cost`의 **플랜 사용량** | **보여줄 수 없다.** 클라이언트가 커스텀 base URL에는 계정 엔드포인트를 **묻지 않는다**(두 자격증명 모양 모두 실측). 대신 `clauduct --usage`가 같은 질문에 답한다 |
| 클라이언트 `/cost`의 **금액** | 토큰 수는 실값이 간다(백엔드가 센 것). 달러는 클라이언트 가격표에 `gpt-*`가 없어 의미 없다. `behavesAs`로 채우면 **확신에 찬 틀린 금액**이 되므로 하지 않는다 |
| 비Windows | 없다. 이식이 아니라 새 설계다 |

`web_search` 외의 hosted 도구(`web_fetch`·`code_execution`·`computer`·`text_editor`·`memory`)는
**설치된 클라이언트가 보내지 않는다** — 2.1.274 바이너리에 그 타입 이름이 없다(2026-09-17 실측).
API에는 있으나 이 조합에서는 도달 경로가 없다.

### v0.3.5 — 남은 결함·격차의 처분

v0.3.3 재판정과 코드 읽기에서 나온 항목이다. 고친 것은 수정을 되돌리면 실패하는 로컬 테스트가 있다. 런타임을 바꾼 것은
2026-09-24 개발 빌드로 실제 backend를 확인했다(native 2.1.281, 20회: 모델 4종 `-p`, `-p` Ctrl+C, WebSearch, SDK 위임 세션, TUI 수용).

| 항목 | 처분 |
|---|---|
| 모르는 출력 항목(#85) | **고침.** 먼저 응답마다 항목 종류별 개수를 상태의 `events.outputItems`에 남겨 실제 세션에서 모았다(`message`. v0.3.4 probe에서는 `reasoning`·`function_call`도). 그 밖의 종류는 `UNSUPPORTED_OUTPUT`로 거부하고, 종류 이름은 이벤트 이름과 같은 제한 규칙으로 남긴다 |
| `-p`의 Ctrl+C(#86) | **고침.** 런처가 세션 동안 `os.Interrupt`를 받는다. `-p`에서는 `USER_CANCELLED`로 기록하고 같은 이벤트를 받은 자식이 스스로 끝나기를 기다리며, 5초 안에 끝나지 않을 때만 정지한다. 종료 코드는 자식의 것이다(native 2.1.281은 0). interactive에서는 native에 맡긴다. 자식을 띄우기 전의 Ctrl+C는 시작을 취소한다. 실제 backend에서 정리(세션 plugin 디렉터리 삭제)까지 확인 |
| codex.exe 탐색(#88) | **고침.** `~\.local\bin` → Codex 앱 설치 위치 → PATH → npm 패키지가 싣는 native `codex.exe`. Node는 실행하지 않는다. 설치 스크립트의 사전 검증도 같다([PACKAGING 5.0](PACKAGING.md#50-스크립트)) |
| 설치 스크립트 교체(#89) | **고침.** `.new`로 먼저 복사하고 이름을 바꿔 교체하며, 실패하면 원래 셋을 되돌린다. 실행 중인 바이너리도 교체된다([PACKAGING 5.0](PACKAGING.md#50-스크립트)) |
| 손자 판정의 PID 재사용(#73) | **고침**(테스트). 생성 시각이 부모보다 늦은 프로세스만 자식으로 센다 |
| TUI 검사 조작 도구(#75) | **공개로 옮김.** `go/cmd/ptydrive`([go/README.md](../../go/README.md#tui-검사-conpty)). 출하 자산이 아니다 |
| gateway 요청당 비용(#110) | **측정으로 종결.** fixture backend의 대표 main turn(요청 약 230–310 KiB, 자식 0/3/30개)에서 gateway가 쓰는 시간은 요청당 약 15.6/15.6/24.9 ms로, 수 초인 backend 턴의 0.5–2%다. 가장 큰 몫은 요청 해독(DecodeRequest, 약 절반)이고 issue에 적힌 파일 읽기·잠금 항목은 각각 수 % 이하였다. 고치지 않는다. 동시 요청의 경합은 재지 않았다 |
| V1 격차: Retry-After(#91) | **고침.** backend가 이름 붙인 시각까지 이 세션의 추론·검색 시도를 credential·소켓 전에 `UPSTREAM_RETRY_DEFERRED`로 거부하고(시도로 세지 않는다) 클라이언트에 429와 `Retry-After`를 보낸다. 실제 429는 유도하지 못했다 |
| V1 격차: 반환 model·effort(#91) | **고침.** 먼저 기록해 4개 모델 모두 보낸 이름과 정확히 같음을 확인한 뒤, 이름이 다르면 `MODEL_EFFORT_MISMATCH`로 거부한다. 응답이 이름을 싣지 않으면 불일치로 보지 않는다. 요청 기록에 `returnedModel`·`returnedEffort` |
| V1 격차: admission(#91) | **v0.4.2에서 구현(#119).** 측정 뒤 확정한 메모리 예약 예산·현재 여유 메모리 보호·유한 대기열을 본문 읽기 전에 적용한다. 생성·계수와 hook·이벤트의 용량을 분리했다. [정책과 한계](#v042--메모리-수용-제어) |
| V1 격차: `system` block(#91) | **고침.** 문자열이거나, 메시지 text와 같은 규칙(닫힌 키, `cache_control` 검사)을 따르는 text block이어야 한다. 그 밖은 거부 |
| V1 격차: downstream keepalive(#91) | **v0.4.1에서 바꿈(#120).** 측정해 보니 클라이언트가 6분 무출력에서 끊었다. 처분은 아래 v0.4.1 절의 "무출력 대기" |
| V1 격차: WebSearch 401(#91) | **고침.** 저장소를 다시 읽어 토큰이 바뀐 경우에만 1회 재시도한다. 같은 토큰의 재시도는 같은 거절이었다 |
| V1 격차: usage(#91) | **고침.** backend가 `input_tokens`에 포함해 세는 cached 입력을 `cache_read_input_tokens`로 나눠 보낸다. 합계는 같다(실측: 11,926 = 6,294 + 5,632) |
| V1 격차: 지연 텍스트(#91) | **고침.** Workflow·SDK 대기 응답의 text part를 줄바꿈으로 이어 block 하나로 보낸다. 마지막 block만 읽는 쪽이 답의 일부만 보지 않게 한다 |
| V1 격차: web_search 도메인 필터(#91) | **고침.** 형식이 틀리거나 32개·253자를 넘으면 `UNSUPPORTED_TOOLS`로 거부한다. 전에는 버리거나 32개로 잘랐다. 기준선은 64개·256자까지 받고 검색 직전에 32개로 잘랐다 — 이 빌드는 보내는 만큼만 받는다. allowed·blocked 동시 지정은 둘 다 보낸다 |
| V1 격차: 진단(#91) | **고침.** 상태에 등록 만료·퇴출 수(`agents.expired`·`evicted`), 세션 처음 실패 8건 보존(최근 8건과 함께), `cleanupFailed` |
| V1 격차: 이름으로 재개(#91) | **고침.** 자식이 한 번 끝난 뒤의 요청은 재개 방식(id·이름·이전 재개의 바인딩)과 관계없이 metadata를 다시 읽어, 사용자가 멈춘 자식이면 거부한다. native가 이 경로를 보내는지는 관측하지 못했다 |

### v0.4.1 — 클라이언트 업데이트 따라가기

두 클라이언트는 따로 업데이트된다. 버전을 고정하지 않고, 업데이트가 무엇을 바꿨는지 과금 없이 드러나게 한다(#127, #121).
고친 것은 수정을 되돌리면 실패하는 로컬 테스트가 있다. 런타임 변경은 개발 빌드 10회와, 발행 전 격리 설치한 v0.4.1 5회로 실제 backend에서 확인했다.

| 항목 | 처분 |
|---|---|
| 모르는 요청 요소(#127) | **거부 유지, 이름을 남긴다.** 모르는 최상위 필드·키·content block·thinking type은 지금처럼 거부한다 — 버리면 요청의 일부를 무시한 답이 성공처럼 돌아온다. 거부한 이름은 상태의 `requests.refusedElements`에 `범주 이름`으로 남는다(처음 8개, 모양 제한. 아는 필드의 잘못된 값은 남기지 않는다). 모르는 `X-Claude-Code-Request-Class` 값은 `INVALID_HEADER`와 구분해 `REQUEST_CLASS_UNKNOWN`(400)이다 |
| 업데이트 알림(#127) | 측정하지 않은 버전이면 종료 줄에 `unmeasured=claude/<버전>,codex/<버전>`이 붙고, `clauduct --dev --doctor`가 `re-measure due`라고 말한다. 세션은 막지 않는다. 상태의 `gateway.codex`는 요청에 실린 Codex 버전과 판정이다 |
| Codex 요청 모양(#121) | **재측정, 모양은 유지.** 설치된 Codex CLI 0.156.1의 `codex exec` 요청을 과금 없이 캡처해 이 빌드의 요청과 비교했다. 0.156.1은 WebSocket을 먼저 쓰고, `instructions`·`tools` 대신 입력 항목으로 도구를 싣는 모양이며, 세션 식별 헤더와 zstd 본문을 쓴다. 이 빌드는 HTTP SSE와 `instructions`·`tools`로 보낸다 — 0.156.1에서 실제 backend를 통과한 모양이다(v0.4.0 출하 검사). 측정 기준을 0.156.1로 옮겼고, 새 모양을 따를지는 v0.5.0에서 정한다. TUI의 요청은 시작할 때 계정 확인이 필요해 과금 없이 캡처하지 못했다 |
| 무출력 대기(#120) | **keepalive 추가.** 과금 없이 측정했다(로컬 backend): Claude Code 2.1.281은 gateway를 거칠 때 바이트가 360초 동안 오지 않으면 연결을 끊고 재시도한다. 첫 출력 전후, `-p`와 TUI가 같고, `CLAUDE_STREAM_IDLE_TIMEOUT_MS`·`API_TIMEOUT_MS`로는 바뀌지 않았다. 이 빌드는 모델이 생각하는 동안 아무것도 보내지 않으므로, 그 재시도가 replay 차단(`NATIVE_REQUEST_REPLAY_BLOCKED`)에 걸려 턴이 사라졌다. 이제 첫 출력 뒤에는 30초 무출력마다 SSE `ping`을 보낸다. 첫 출력 전에는 240초 동안 아무것도 쓰지 않았으면 메시지를 열고(`message_start`) ping을 보낸다. 240초 전의 실패는 지금처럼 상태 코드(429·400 등)로 답하고, 그 뒤의 실패는 오류 이벤트로 온다. 측정된 가장 긴 요청은 218초였다 |

### v0.4.2 — 메모리 수용 제어

큰 요청이 겹칠 때의 메모리 증폭을 로컬에서 측정하고 사용자와 수용 정책을 정했다([#119](https://github.com/wotjr1649/Clauduct/issues/119)).
32 MiB 요청 한 건은 heap objects가 약 400–503 MiB 늘었고, 작은 완료 알림의 transcript 복구도 약 230 MiB까지 늘었다.
아래 값은 이 관측에 여유를 둔 시작 정책이다. 실제 RSS의 강제 상한이나 모든 입력에 대한 메모리 보장은 아니다.

| 항목 | v0.4.2 정책 |
|---|---|
| 프로세스별 총 예약 예산 | 시작할 때 Windows가 보고한 물리 메모리로 `min(물리 메모리 / 8, 4 GiB)` 계산 |
| 제어용 용량 | 총예산 안에서 `min(총예산 / 2, 512 MiB)`를 hook·이벤트 업로드에 배정. 생성·계수는 나머지를 사용하며 서로 빌려 쓰지 않음 |
| 생성·토큰 계수 한 건 | `16 MiB + 본문 바이트 × 24` 예약. Content-Length를 모르면 본문 상한 32 MiB로 계산. 기존 동시 실행 64건 상한도 유지 |
| 제어 요청 한 건 | 본문 크기와 무관하게 256 MiB 예약. 작은 완료 알림도 transcript 복구를 할 수 있기 때문 |
| 현재 여유 메모리 보호 | 현재 여유 물리 메모리가 기존 예약 전체 + 새 예약 + `max(256 MiB, min(물리 메모리 / 10, 1 GiB))` 이상이어야 수용. 대기 중에는 재조회하며 이미 실행 중인 요청을 메모리 압박만으로 취소하지 않음 |
| 대기 | 본문을 읽기 전 FIFO. 생성·계수 공용 128건·30초, 제어 전용 16건·0.5초. 처리 시간은 별도여서 hook의 3초 HTTP 제한 내 완료를 보장하지 않음 |
| 거부 | 한 요청이 용량을 넘으면 `MEMORY_BUDGET_EXCEEDED`, 대기열 포화는 `MEMORY_QUEUE_FULL`, 시간 초과는 `MEMORY_ADMISSION_TIMEOUT`, Windows 메모리 조회 실패는 `MEMORY_STATUS_UNAVAILABLE`. HTTP 429와 `X-Should-Retry: false`로 자동 재시도를 막음. 시작 시 조회 실패는 gateway 시작 실패 |
| 예약 수명·상태 | 취소 통지만으로 반환하지 않고 handler가 끝날 때 반환. 종료는 양쪽 대기열을 깨움. 상태 파일의 `gateway.admission`에 총예산·여유분, 각 용량의 예약량·실행 수·대기 수·누적 대기·시간 초과를 보고 |
| PromptOrigin 격차(#130) | 알려진 제한으로 기록. [미지원·미검증 표](#3-미지원미검증구현-대기)의 “빈 응답 제어의 전체 실행 모드 지원” 행에서 범위를 정하며, v0.4.3의 #130에서 측정·수정 |

32 MiB 요청은 784 MiB를 예약하므로, 물리 메모리 8 GiB 장비의 생성·계수 예산 512 MiB에는 들어가지 않는다.
Windows가 보고하는 물리 메모리가 4 GiB 미만이면 제어용 용량이 256 MiB보다 작아 제어 요청을 수용할 수 없다.
현재 여유 메모리 검사에서 이미 할당된 예약도 다시 빼므로 일찍 기다릴 수 있다. 여러 Clauduct 프로세스의 예약을 합산하지 않으며,
본문 크기만으로 실제 transport·큰 응답·장기 세션·PDF 자식 프로세스의 모든 비용을 제한하는 것은 아니다.

로컬 전체 test·race, vet 3종·build, 핵심 동작의 변이 9개 검출을 통과했다. Claude Code 2.1.282의 실제 native로
새 429 거부가 요청 1회 뒤 자동 재시도 없이 끝남을 과금 없이 확인했다. 개발 commit `bf68243`은 실제 backend
TUI 5회(생성·압축·취소·복구·종료)와 SDK 5회(입력·`/clear`·자식 위임·완료)를 통과했다. 태그의 격리 설치본도 실제 backend 5회를 통과했으며, [출하 기록](RELEASE-v0.4.2.md#출하-검사-2026-09-25)에 설치·업데이트 검사를 함께 적었다.

### v0.4.3 — 메시지, background, 검증 예산

**개발 중·미출하.** 실제 backend 및 자동 유휴 종료 후 깨우기는 아직 완료하지 않았다.
아래 무료 검증은 실제 native 2.1.282를 사용하며, backend 응답만 합성한 결과다.

| 항목 | 구현·관측 범위 |
|---|---|
| 메시지 기반 부모 대기(#130) | 입력 없는 TUI·SDK가 다른 native 세션의 `SendMessage`를 `peer`로 받는 경로를 실측했다. 수정 전 각각 EMPTY_REPLY 1회, 수정 후 부모 3·자식 1회와 결과 수신 1회, 오류 0. 도중의 두 번째 메시지는 native가 새 턴으로 큐잉했으며 정상 답변을 표시했다. same-turn 개입은 모듈 정책 검사로 구분한다 |
| `/clear`·`/reload-plugins` | 이후 실제 peer와 부모 대기를 TUI·SDK에서 확인했다. TUI의 reload는 CLI `--agents`로 준 사용자 역할을 제거했다. 이는 native 동작이며 그 역할의 reload 유지까지 지원하지 않는다. 기본 native 역할의 부모 대기는 확인했다 |
| 명시적 background 생성(#133) | `clauduct --bg`·`--background`로 생성하고 native agents·attach·stop·respawn·메시지·삭제로 관리한다. 세션별 Clauduct 연결 유지 프로세스 1개가 최초 실행기 종료 뒤 남는다. 순수 바이너리의 실행기 종료·연결 종료도 과금 없이 확인했다 |
| 동일 로그인 내 재기동 | native stop→attach와 respawn→peer에서 gateway·모델·effort·필수 hook 연결을 확인했다. 자동 유휴 종료와 예상치 못한 worker 종료는 별도 관측이 필요하다. PC 재부팅·로그아웃 뒤 자동 복원은 범위 밖이다 |
| 연결 유지와 실패 | native stop은 연결을 남긴다. native 세션 삭제 또는 `clauduct --background-stop <connection-id>`가 연결도 끝낸다. 토큰은 메모리에만 두고 동일 Windows 로그인에서 IPC로 받는다. 유지 프로세스 종료 뒤 자동 재시작·backend 전환·요청 재실행은 하지 않는다. native agents 화면에서 새 작업을 Clauduct로 만드는 경로는 범위 밖이다 |
| 검증 예산(#134) | `CLAUDUCT_VERIFICATION_BUDGET`으로 실행 전체 model/effort와 시도 수를 제한한다. 실패·취소·재시도·backend 계수도 차감하며 검색은 검증 중 거부한다. 여러 프로세스와 새 원장에서도 N+1은 credential·소켓 전에 거부된다. 예약 파일 삭제·교체는 지원하지 않는다. 일반 세션의 요청 수 정책은 유지한다 |

일반·race 전체 각 21개 패키지와 기본/태그 vet·build·gofmt를 통과했다. `peer` 지원을 제거한 변이는
실제 native SDK에서 EMPTY_REPLY assertion으로 실패했고, 공유 예약을 제거한 변이는 N+1 전송 assertion으로
실패했다. 실제 backend의 결과·사용량과 남은 관측은 [릴리스 초안](RELEASE-v0.4.3.md)을 갱신한다.

## 4. 제3자 구현이라는 사실

Claude 공식 문서는 gateway를 통한 non-Claude 모델 라우팅을 **공식 지원하지 않는다고 명시**한다.
V2는 제3자 호환 구현이며 "Anthropic 공식 지원"이나 "전체 기능 100% 보장"으로 설명하지 않는다.
지원 조합을 고정해 기록하고 drift 진단을 남긴다 — 클라이언트 버전은 고정하지 않고 **보고**한다.

## 5. 이번 grilling에서 확정한 정책과 구현 상태

아래는 사용자와 확정한 목표다. 문서 작성 자체가 기능 구현이나 검증 완료를 뜻하지 않는다.

| 확정 정책 | 현재 상태 / 다음 근거 |
|---|---|
| 작업 성공과 실패 처리·회복 합격을 구분 | 판정 기준 채택. 외부 장애를 정확히 보고하고 이력·결과를 보존하며 다음 요청이 동작해야 회복 합격. 모든 장애 조합 실측은 남음 |
| 실제 실행 중인 자식과 대기 회차가 검증된 부모의 빈 응답은 대기로 유지 | TUI는 무출력 대기, SDK는 완료를 주장하지 않는 Clauduct 상태 메시지. 실제 backend의 빈 응답과 결정적 native fixture를 모두 관측. 자식 결과 수신과 상태 표시를 분리 |
| 진행 미관측만으로 강제 종료하지 않음 | 마지막 실제 이벤트·경과 시간·도구 대기 수를 보고. 누락/미종료는 drain 완료로 간주하지 않음. 기존 명시적 deadline/grace만 적용 |
| 재개 요청이 있어도 이전 도구 실행 효과가 불명확한 자식은 자동 재실행하지 않음 | 기존 결과 회수 유지. 독립 계획에서는 중단이 확인된 원본의 미실행 단계만 실행하고, 시작했지만 결과 없는 단계는 `started_not_reexecuted`·전체 `complete:false`로 보고 |
| 버전 번호 대신 기능별 필수 조건으로 실행 판정 | 기존 실행 경계 검사 결과를 기능별로 집계. `/context` 출처도 관측 버전과 구조를 대조. 버전 일치와 전체 기능 합격을 구분 |
| 지원 기능을 별도 문서로 관리 | 이 문서를 현행 지원 목록으로 사용. 세션별 보고서는 변경 당시의 근거로 보존 |

## 6. 업데이트 시 이 문서를 갱신하는 기준

1. **문서와 실행 판정의 사실을 일치시킨다.** 기능명, 지원 입력, 필수 조건, 구현 여부, 실측 버전·제품 commit·binary hash, 검사/실제 TUI 근거, 제한을 기록한다. 문서의 문구가 실행 허용을 대신하지 않는다.
2. **버전 변경만으로 과거 근거를 지우거나 새 버전으로 바꾸지 않는다.** 현재 실행 조건 확인과 과거 TUI 확인을 나란히 표시한다. 영향받은 기능을 새 버전에서 검증한 뒤 근거를 추가한다.
3. **구현·지원 범위가 바뀌면 같은 변경에서 해당 행을 갱신한다.** 근거가 없는 완료 표현, 버전 일치만으로 `검증 완료` 처리, 단위 검사를 실제 TUI로 표시하는 변경은 하지 않는다.
4. **실패를 재실행 성공으로 덮지 않는다.** 최초 실패·원인·수정·검증을 연결하고, 미지원/미검증/실행 조건 실패를 구분한다. 정상 종료 코드만으로 기능 합격을 선언하지 않는다.
5. **추가 구현 전 남은 사실을 확인한다.** 진행 관측·TUI 대기 통합·독립 계획의 중복 방지 재개는 위 범위에서 구현했다. 다른 실행 모드·범용 스크립트 재개·의미상 성공 판정·모든 경계/멀티모달/성능의 검증은 남아 있다. 원본 사용자 세션이나 전역 설정을 문서 작업 때문에 변경하지 않는다.
