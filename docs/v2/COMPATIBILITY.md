# V2 호환성 — 현재 / 제약 / 미지원

v0.6.4부터 Clauduct는 native 권한 규칙을 더하지 않는다. v0.6.2–v0.6.3이 요구하던
실행·외부 통신 도구 17개의 native 확인(`permissions.ask`)과 v0.5.2부터의 `autoMode.hard_deny` 2개는 이전 버전의
동작이다. 이제 auto 모드의 `Bash`·`WebFetch` 같은 도구는 native 규칙과 native 분류기만으로 결정된다.
계정 모델 목록·설정 동기화를 포함한 변경 전체와 검증 상태는 아래 3절의 v0.6.4 절에 있다.
그 절의 실제 backend 검증은 출하 때 실행했고 통과했다(후보와 릴리스 바이트에서 각각 14개 시나리오).
요청 출처 확인은 유지한다([설정 문서](SETTINGS.md#권한과-요청-출처-확인-v064)).
Agent ID가 없는 직접 입력 fork는 출처를 증명할 수 없어 지원하지 않는다. 활성 자식이 하나여도
이전 자식의 지연 요청과 구별할 수 없으므로 `NATIVE_REQUEST_ORIGIN_UNVERIFIED`로 backend
전송 전에 거부한다. Agent ID가 있는 native Agent·fork 및 식별된 병렬 실행은 유지한다.
모델 도구 호출은 받은 native step의 표지를 `tool_use_id`에 달고 실행 시점에 그 step과 대조한다. 이전 turn·step의
늦은 호출은 실행 전에 거부한다. plugin hook 모듈의 직접 도구 호출은 native 권한 규칙대로 실행하되 turn 상태와
위임은 사용하지 못한다([설정 문서](SETTINGS.md#권한과-요청-출처-확인-v064)).
세션 중에 claude.exe가 교체되면(native 자동 업데이트 등) 종료 줄에 `native_replaced=1`을 표시한다.
실행 중인 native는 기존 이미지로 계속 돌고, 다음 세션부터 새 버전이 실행된다.

v0.5.5부터 설치·제거 및 Windows 개발·검증의 PowerShell 호스트는 7(`pwsh`)이다.
Windows PowerShell 5.1 지원은 종료한다. 실제 검증 버전과 설치 방법은
[패키징 문서](PACKAGING.md#50-스크립트)에 기록한다. 아래 과거 릴리스의 5.1 측정 결과는 당시의 이력이다.

v0.5.5는 Responses assistant message의 `phase`(`commentary`·`final_answer`)를 text block에
보존하고 다음 backend 요청에 복원한다. 이 text 부가 필드는 Anthropic 표준 밖의 Clauduct 확장이다.
native 2.1.283에서 저장·UUID 재개·fork를 확인했고, Sol/medium·Luna/medium·Terra/high의
실제 backend 왕복에서도 보존을 확인했다. 원래 값이 없는 과거 기록에는 값을 추측해 넣지 않는다.
스트림 시작·종료·완료 snapshot 사이의 phase가 충돌하면 해당 응답을 거부한다.
**새 phase 포함 대화는 v0.5.5 이상에서 재개해야 한다.** v0.5.3/4는 이 필드를 거부한다.
설정·snapshot·대화 파일은 보존되며, 구버전에서는 새 대화를 시작할 수 있다.
native 업데이트 때 이 확장 필드의 보존 동작을 다시 확인해야 한다.

Go 개발 빌드의 지원 기능과 제한을 이 문서에서 관리한다. 격차의 이력은
[PARITY.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/PARITY.md), 이전 실행 증거는 [VALIDATION.md](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/VALIDATION.md),
S41 수리와 실패 기록은 [S41 보고서](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/session41-repair-20260919/REPORT.md),
기능별 판정·진행 관측·Workflow 결과 회수는 [후속 검수 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/capability-repair-20260919/REPORT.md)에 있다.
S42에서 발견한 거부 후 회복·역할 발견·압축 보완은 [S42 수리 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/session42-repair-20260920/REPORT.md)에 있다.
부모의 완료 조건·native TUI 대기·미실행 Workflow 단계 재개는 [부모 대기·계획 재개 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/parent-wait-20260920/REPORT.md)에 있다.
같은 바이너리의 사용자 S43 실행 판정은 [S43 사용자 검수](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/parent-wait-20260920/S43-USER-ACCEPTANCE.md)에 있다.
Node V1 및 설치 명령으로 받은 릴리즈의 지원표로 그대로 사용하지 않는다.

**native 2.1.286 후속 — v0.6.2 출하.** 새 turn의 첫 step에서 빈 완료 알림을 처리할 출처
증거를 현재 plugin API로 확보할 수 없다. `prompt.submit.turnId`는 이전 실행 중 turn의 ID이며,
새 ID를 생성하는 `turn.start`에는 출처가 없다. 후보는 이 빈 응답을 `EMPTY_REPLY`로 명시적으로
거부한다. 자식 보고가 요청에 있다는 사실만으로 출처를 추정하지 않는다. 일반 답변·도구 실행과
확인된 이후 step의 대기·handback은 유지한다. 아래 과거 버전의 완료 알림 PASS는 2.1.286의
지원 증거로 재사용하지 않는다. 후보의 실제 backend·최종 바이너리·병합 후 검증은 아직 미완료다.

**2026-09-23 v0.3.2 재실행 방지 묶음 — 미출하.** 요청은 처음 읽은 turn 영수증 하나로 예약·선택·취소
바인딩을 판정한다. 독립 `auxiliary` 요청은 현재 root turn 안에서만 한 번 실행하고, 다음 turn의 같은
요청은 새로 실행한다. agent의 새 turn이 예약되면 그 agent의 이전 turn 실행 기록을 지운다. 그래서 긴
세션이 요청 16,384개에서 멈추지 않는다. 남는 한도는 agent마다 마지막 turn의 기록과 turn 정보 없는
기록(`--bare`)의 합이다. 당시 모듈 재등록 순서는 시계 기반이었다. v0.5.5 재감사에서 시계가 증가해도
이전 누적 순번보다 작으면 과거 turn을 선택하는 경계를 재현했고, v0.5.6은 기존 게시 순번을 이어 쓰도록 고쳤다.
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
| 최근 실제 TUI | Claude Code `2.1.283`, Windows amd64, Go 1.27.1, CGO_ENABLED=0. v0.5.0 태그 바이너리(`d665e18`), gpt-6-luna 실제 backend 8회, 2026-09-26: 생성·압축·취소·복구·종료 PASS, API 실패 0(공개 `go/cmd/ptydrive`로 조작). 그 전: `2.1.282`, v0.4.2 개발본(`bf68243`), 2026-09-25 같은 항목 PASS. 그 전: v0.4.1 개발본(2.1.281), 2026-09-24 같은 항목 PASS. 이전: `2.1.280`, v0.3.2 후보, 2026-09-23 [TUI 기록](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/verification/v032-tui-20260923/README.md) |
| 인자 표·native fixture 재측정 | Claude Code `2.1.283`, 2026-09-26(v0.5.0 개발). `--help`에 필수값 옵션 `--client-data-url <url>`이 더해졌고, 이전 인자 표가 이를 모르는 옵션으로 읽어 뒤의 `--settings`·`--model`·`-p` 경계를 잃었다. 인자 표에 더해 고쳤다. native는 `https://downloads.claude.ai/` 외의 값을 모델 요청 전에 exit 1로 거부한다(loopback 재현, 모델 요청 0). 서명 구성 기능 자체의 지원을 뜻하지 않는다. 모듈이 쓰는 plugin 이벤트 타입은 같다. 설치 native를 쓰는 fixture 검사와 전체 일반·race 회귀 통과(로컬 backend, 과금 없음). 이전 기준: `2.1.282`, 2026-09-25(v0.4.2) |
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
| Claude Desktop 실행 | `--desktop` 인자 경계는 인식하나 별도 앱의 backend·세션 수명은 미지원. 설정 읽기와 native 실행 전에 `DESKTOP_UNSUPPORTED`로 거부하며 native 터미널 사용을 안내. 값이 붙은 알 수 없는 옵션 뒤에서도 검사한다. 알 수 없는 옵션 때문에 뒤의 `--desktop`이 옵션인지 값인지 증명할 수 없으면 `DESKTOP_OPTION_UNVERIFIED`로 거부한다. 확인된 문자열 값과 `--` 뒤 데이터는 보존 |
| API 요청 형태 | `stream:false`와 생략은 완료된 JSON 응답을 반환. malformed stream 값은 거부. `temperature`, `top_p`는 미지원. v0.5.2는 개수·길이를 제한한 `stop_sequences`를 로컬 출력 절단으로 처리한다(아래 v0.5.2 절). [요청 decoder](../../go/internal/protocol/anthropic/request.go). 자동 fallback 재생성은 계속 비활성 |
| Workflow 범위 | inline, native Read로 읽은 `scriptPath`·프로젝트/사용자 named `.js`, custom 역할 기본 선택, `pipeline`·중첩 `parallel` 콜백 지원. 정상 종료/수거 완료 후 같은 세션의 기록 복원, 명시적 source의 native 재개와 독립 계획의 미실행 단계 재개. `maxTurns`는 native 역할 정의에 지정. 자식 안의 별도 Workflow와 근거 없는 재개는 제한 |
| 부모·빈 응답 대기 | 확인된 TUI 회차는 무출력 대기. SDK/`-p`의 검증된 빈 대기·알림 응답은 Clauduct 상태 메시지로 전달하며 실제 본문·도구는 보존. 상태 메시지는 자식 결과나 업무 완료가 아님. [실제 필수 조건](../../go/internal/gateway/features.go) |
| 새 모델·새 명령·외부 확장 | v0.6.4부터 계정 목록에 있는 모델은 새 릴리스 없이 라우팅한다. 라우팅할 수 있다는 것이 그 모델의 실제 동작을 검증했다는 뜻은 아니며, 새 모델, native 버전, plugin/MCP 조합의 성공을 자동 승계하지 않음. 검증된 요청 형식 범위는 그대로. [기존 이름 표](../../go/internal/protocol/bridge/route.go) |
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
| launcher 재시작 후 Workflow 복원 | native 수거 후 본문 없는 메타데이터 저장, 같은 UUID의 SessionStart 경로에서만 복원. journal/metadata 해시와 선택·종료 근거 재검증. 결과 회수·plan-v1은 원 script 해시도 확인하고, 명시적 native 재개는 편집한 source 허용. 독립 계획 재개는 디스크의 배타적 claim, native 재개는 native의 동시 실행 거부를 유지. 강제 종료로 근거가 없거나 캐시 기록이 변경되면 거부 |
| `--settings` | 구현. 최대 2 MiB JSON 객체/일반 파일, 중복 키 거부, 마지막 옵션 우선. 사용자 hook을 보존하고 필수 binding 추가. 연결·필수 정책 충돌은 조용히 덮지 않고 거부 |
| `--setting-sources` | native user/project/빈 source 적용을 공개 fixture와 실제 native CLI로 확인. 필수 CLI settings는 별도로 유지 |
| 권한 우회 CLI 옵션 2개 | 기술적 불가능이 아니라 명시적 제품 정책 제한이다. 지원 여부 변경과 현재 작업의 권한·guard 준수는 별개 |
| `temperature`, `top_p`, `stop_sequences` | `temperature`·`top_p`는 입력층 거부 유지. v0.5.2의 stop 처리는 로컬 응답 번역이며 backend에 새 필드를 보내지 않는다. S48의 4개 모델 × baseline/temperature/top_p/max_output_tokens/stop = 20개 비교는 baseline 4건 성공, 추가 필드 16건 HTTP 400이었다. `stop` 시험을 `stop_sequences` 필드 자체의 실측으로 부르지 않음. 일반 API 문서 지원이 구독 backend 지원을 뜻하지 않음 |

`max_output_tokens`는 위 세 파라미터와 달리 기존 backend HTTP 400 실측이 있다.
출력 문자열을 잘라내는 것으로 서버의 생성 토큰 제한이나 sampling 제어와 동등해지지 않는다.
[출력 상한의 기존 실측](https://github.com/wotjr1649/Clauduct/blob/1b1c5e19b3f33fda63254b2da7c9d0b372553481/docs/v2/VALIDATION.md), [현재 변환 계약](../../go/internal/protocol/bridge/bridge.go).

v0.6.0은 검증된 같은 UUID/run의 `resumeFromRunId`와 `script`·`scriptPath`·지원되는 로컬 `name`을
함께 전달하면 native Workflow 재개를 수행한다. source 편집도 명시적 새 입력으로 허용한다.
native는 완료된 앞 agent 결과를 재사용하고 실패·중단·변경된 prompt 지점과 그 뒤 agent를
다시 실행할 수 있다. 이미 완료한 뒤쪽 agent도 포함되므로 파일 쓰기 등 효과가 반복될 수 있다.
실행 중인 자식은 먼저 중지하고 종료를 확인해야 한다. native의 cache·승인·동시 실행 거부를 유지한다.
launcher 종료 후에는 저장한 journal·child metadata·선택·종료 근거를 재검증하며, 근거 없는 강제 종료나
캐시 변조는 거부한다. 새 script의 변경과 캐시 근거의 변조는 구분한다. 원본 run을 plan-v1으로 바꾸는
혼합 재개도 거부한다. `resumeFromRunId` 단독의 결과 회수와 plan-v1의 미실행 단계 재개는 그대로다.
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

v0.6.0에서는 native가 `SubagentHandback` 보고를 받아들인 뒤에도 손자가 남아 있으면 기존 대기를
이어 간다. 같은 session·agent·turn과 직전 성공한 호출을 검증하며, 보고 전달을 손자 완료로 간주하지
않는다. native 2.1.283의 `SubagentHandback`은 실행당 한 번만 보고한다. 후속 연락은 native의
`SendMessage` 규칙을 따르며, SDK의 기본 실행 모드는 바꾸지 않는다.

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
| `--resume` · `--permission-mode` · `--worktree` · `--plugin-dir` | G5, 동작으로 측정 |
| `--bare` 대화 실행 | 현재 지원 범위에서 제외한다. 과거 G5는 context·세션 profile을 검증하지 않은 연결 검사였다. native 2.1.284에서 제품 정책을 켠 두 표본(도구 목록 있음·없음)은 `SESSION_PROFILE_UNVERIFIED`로 거부됐다. `--bare`를 제외한 표준 native 모드를 사용한다. 과거 연결 PASS와 현재 거부를 구분하며 [#223](https://github.com/wotjr1649/Clauduct/issues/223)에서 v0.6.2 출하 때 최종 바이너리로 거부를 다시 확인하고 닫았다 |
| 서브에이전트 역할·모델·effort 선택 | 원래 지정 여부와 native 자식 식별을 대조. built-in 역할의 알려진 표기 차이와 `subagent_type` 생략 처리. 아래 최근 TUI 범위 참조 |
| 위임 메뉴·모델 표 | v0.6.4부터 메뉴는 세션의 계정 목록에서 만든다: 기존 모델은 `clauduct-<key>`, 새 모델은 `clauduct-<전체 ID>`, 이름이 겹치면 항목 없이 Agent model 인자로 사용. 은퇴 표는 계정이 그 이름을 나열하지 않을 때만 `MODEL_RETIRED` 이유가 되며, effort는 계정 지원 수준 ∩ low–max만 보낸다(아래 v0.6.4 절). 이하 v0.6.3까지의 기록: v0.3.4부터 `clauduct-<model>` 메뉴 + `clauduct-inherit`. v0.6.3부터 5종(`clauduct-sol`은 gpt-6.1-sol, `clauduct-sol6`은 gpt-6-sol). effort는 Agent `effort` 인자로 받고 없으면 모델 기본값. [카탈로그 기반 생성](../../go/internal/app/agents.go). 표는 fable→gpt-6-astra, opus→gpt-6.1-sol(v0.6.3, 이전 gpt-6-sol), sonnet→gpt-5.6-terra, haiku→gpt-6-luna. gpt-5.6-sol·gpt-5.6-luna와 v0.3.3의 `clauduct-<model>-<effort>` 이름은 `MODEL_RETIRED`로 거부하며 대체 실행하지 않음. Codex 카탈로그의 `ultra`는 2026-09-24 측정에서 gpt-6-sol·gpt-6-astra·gpt-5.6-terra 모두 HTTP 400이라 어느 모델에도 노출하지 않음. 메뉴 존재는 모든 모델의 최신 TUI 통과를 뜻하지 않음 |
| 중첩 Agent 자동 재진입 | 원래 계보와 현재 native turn을 확인한 뒤 확정 선택 유지. TUI에서 ROOT → A → B → C의 완료 결과 전달 확인. native 2.1.283 SDK 기본 모드에서는 손자 결과가 루트에 도착해도 중간 Agent가 자동으로 다시 답하지 않을 수 있다. 루트가 실제 손자 결과를 확인한 뒤 중간 Agent ID로 `SendMessage`를 보내 재개한다. UUID 재시작 뒤의 이 절차도 실제 backend로 확인했다. SDK 기본값은 바꾸지 않으며, 명시적인 `CLAUDE_CODE_FORK_SUBAGENT=1`은 native의 문맥 상속·백그라운드 실행 의미를 따른다 |
| 완료 Agent의 SendMessage 재개 | 최근 TUI에서 동일 child ID의 Sol/high 유지·두 번째 결과 수신 확인 |
| Agent teams(native 실험 기능, v0.6.9) | `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`인 TUI의 in-process teammate를 지원한다(#269). native 2.1.289는 teammate의 HTTP 요청에는 주소(`이름@팀`)를, hook 이벤트에는 루프 ID를 싣는다. 세션 plugin이 spawn 때 둘의 대응과 Agent 호출·역할을 기록하고, gateway는 그 기록이 있을 때만 주소를 받아들인다. 요청마다 native 메타데이터가 여전히 그 주소의 teammate 작업인지 확인하고, 사용자가 중지한 teammate는 계속 거부한다. 첫 요청은 모델이 그 호출의 선택과 맞아야 실행한다. native가 모델을 고른 호출이면 그 turn의 native 영수증과 맞아야 한다. teammate 이름은 영문·숫자·`_`·`-`만 받고, 그 밖의 이름을 쓴 teammate의 요청은 거부한다. teammate의 보고는 native가 idle 알림으로 lead에게 전달하므로 gateway는 그 결과를 따로 추적하지 않는다. 확인: loopback에서 spawn → teammate 도구 호출 → 보고 → idle → lead의 `SendMessage`로 다시 시작 → 두 번째 보고 → 종료, 실제 backend TUI 2회(teammate luna/low)에서 같은 흐름. flag를 끈 세션의 이름 붙은 subagent는 기존대로 동작한다. 제외: split pane(tmux·iTerm2, Windows 미지원), `-p`(native가 teammate를 만들지 않는다). `/resume` 뒤 teammate가 복원되지 않는 것은 native 제한이다. v0.6.8 이하는 2.1.289에서 teammate 요청을 모두 거부했다(`INVALID_SESSION_ID`) |
| inline Workflow의 자식 선택 | model+effort / model만 / effort만 / 둘 다 생략을 runtime 선택과 child ID에 연결. 최근 TUI 네 자식 병렬 실행 확인 |
| Agent 결과 회수와 Workflow StructuredOutput | 일반 결과와 검증된 native journal 결과를 부모에게 전달. 범용 Workflow 재실행·복구 기능은 아님 |
| 모델 피커 + `GET /v1/models` discovery | A3 + B1. v0.6.4부터 세션 계정 목록 중 숨기지 않은 모델 |
| 실측 사용량·예방 압축 | v0.6.2 준비본은 모든 모델에 공통 window 272K·비율 90%(목표 244,800)를 기본으로 사용한다. [전역 설정](SETTINGS.md#전역-context와-압축-목표-v062-준비)으로 변경하며 UUID 재개에도 현재 값을 적용한다. native가 실제 압축을 담당하고 자체 여유 공간 때문에 더 일찍 실행할 수 있다. v0.6.4부터 자동·수동 압축은 현재 선택(Agent의 자기 route 또는 native가 압축 요청에 적은 모델·effort)을 쓰고 effort 상한은 없다. 모델을 바꾼 뒤의 압축은 새 모델에서 실행된다. v0.6.2–v0.6.3은 기존 확정 모델과 `auto_compact_effort_cap`(기본 medium)을 적용했다. 관리 목표는 정확 사전 차단 상한이 아니며 새 대용량 입력의 최초 초과 가능성과 추정/실제 usage 구분은 유지한다. v0.6.1 출시본의 Astra 500K/450K, 나머지 272K/239K와 native 표시 500K는 이전 정책이다 |
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

### v0.6.1 이후 Agent·background 경계 (v0.6.2 출하)

2026-09-29 개발 후보에서 [#195](https://github.com/wotjr1649/Clauduct/issues/195)의
`SubagentStop` 조기 종료를 수정했다. native Stop hook이 추가 작업을 요구할 수 있으므로,
같은 session·agent·turn의 실제 종료 영수증까지 등록과 완료 보고를 보류한다. 검증된 후속 생성 요청은
이전 종료 후보를 폐기하며, 토큰 계수·auxiliary·compaction 요청은 이를 소비하지 않는다.
사용자 중지·신원 검증과 새 입력의 `EMPTY_REPLY` 거부는 유지한다.
[native SubagentStop 계약](https://code.claude.com/docs/en/hooks#subagentstop)을 따르는 수정이며
SDK 기본 모드나 `SubagentHandback` 횟수 정책을 변경하지 않는다.

실제 Codex backend에서 수정 전 Stop 피드백 뒤 `AGENT_SELECTION_UNVERIFIED`를 재현했다.
후보의 일반·독립 Stop 피드백 표본과 handback 뒤 손자가 실행 중인 `SendMessage` 1회 표본은
새 입력 응답·손자 결과·루트 수신까지 오류 없이 통과했다. 의도적으로 빈 답을 만든 무과금 native
회복 검사는 `EMPTY_REPLY` 1건을 그대로 요구한다. 이는 모든 개입 시점과 입력의 보장이 아니다.

[#196](https://github.com/wotjr1649/Clauduct/issues/196)은 같은 위임의 최종 답변만 marker와
marker+완료 문장으로 바꿔 비교했다. 일반·독립 pair 네 표본 모두 실제 backend에서 완료됐다.
답변 도착 뒤에도 native `working`이 남다가 `done`으로 전환한 관측이 있으므로,
문장형 답변만 성공한다는 인과관계나 임의 marker의 즉시 완료를 주장하지 않는다. 과거 timeout의
정확한 원인은 미확정이다. 완료는 `done`·실제 결과·root `turn_answer`·child native end와
`parent_received`·요청/대기/예약 메모리 0의 서로 다른 두 안정 checkpoint를 모두 요구한다.
native background 상태·권한·scheduler와 과거 FAIL·미확인 기록을 보존한다.

v0.6.2 준비 변경은 독립 보조 요청의 effort를 전역 상한 `auxiliary_effort_cap`
이하로만 낮췄다(v0.6.4에서 은퇴, [현행 설정](SETTINGS.md#보조-요청과-auto-권한-분류기-v064)). 권한 분류는 고정 Terra/high 대신 기존 모델 매핑·기본값·명시값을 따른다.
Luna는 권한 분류에서만 거부하고 일반 대화·Agent·background 완료 요청에서는 유지한다.
native 정책 문구·두 단계 판정·완료 의미는 보존한다. 확대된 Terra·Sol·Astra 전 effort 범위의
분류 품질 관문은 아직 미완료이며, 과거 Terra/high 오허용과 background timeout을
[#218](https://github.com/wotjr1649/Clauduct/issues/218), [#206](https://github.com/wotjr1649/Clauduct/issues/206)에 보존한다.
v0.6.4의 권한 분류는 `classifier_model` pair를 쓰며 Luna를 포함해 계정이 제공하는 pair를 모두 허용한다.

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
| 소유하는 옵션 **5개** | `--update`(+`--yes`), `--usage`, `--uninstall`(+`--yes`), `--dev`, `--background-stop <connection-id>`. **첫 인자일 때만** 인식한다. native의 `--bg`·`--background` 생성 경로에는 연결 유지 처리를 더하며 나머지 native 인자를 전달한다 |
| 거부하는 native 옵션 **2개** | `--dangerously-skip-permissions`·`--allow-dangerously-skip-permissions`(권한). 사용자 settings는 필수 settings와 병합하며 충돌만 거부 |
| 주입하는 것 | settings·위임 메뉴·세션 plugin·모델/도구/압축 관련 환경. 사용자 `--agents`는 메뉴를 대체할 수 있지만, [sessionRequirements](../../go/internal/app/session.go)의 필수 환경값은 사용자 값으로 자동 대체하지 않음 |
| hook이 없으면 | 자식 확정 선택과 native 압축을 검증할 수 없으므로 해당 요청을 거부한다. `hookInstalled`에 기록 |
| non-streaming 요청 | explicit false/생략 지원. 세션 인증·요청 종류·컨텍스트 정책 등 기존 실행 조건은 동일. native의 오류 후 자동 fallback은 중복 생성을 피하려고 계속 끔 |
| 미지 SSE 이벤트 | 요청 실패. 이름만 계정에 남긴다(D6) |
| side query가 아닌 hosted 도구 요청 | `HOSTED_TOOL_UNSUPPORTED`로 거부. 기준선은 조용히 성공시킨다 — 의도적 divergence |
| 사전 출력 토큰 상한 | 기존 실측에서 backend가 `max_output_tokens`를 HTTP 400으로 거부해 해당 방식은 미지원. 완료 후 usage 검사와 구분하며, 미래의 모든 구현 가능성까지 부정하지 않음 |
| 게이트웨이 재시도 | 일반 생성의 자동 재시도와 [WebSearch의 제한된 읽기 재시도](../../go/internal/upstream/search.go)를 구분. 전 경로가 재시도 0이라는 뜻이 아님. backend로 보낸 뒤의 실패는 native가 재시도해도 replay 보호에 막히므로, `X-Should-Retry: false`로 재시도를 막고 원인 범주를 그대로 보인다. backend 실패 이벤트는 고정 어휘로 줄인 code·type·incomplete 사유를 함께 싣는다(v0.3.3, #84). 이전에는 사용자가 원인 대신 `NATIVE_REQUEST_REPLAY_BLOCKED`만 봤다 |
| native context 표시 | v0.6.2 준비본은 전역 window·유효 비율을 native에 전달한다. native 로컬 추정과 여유 공간은 gateway의 예방 목표와 다를 수 있다. 적용값·출처·실제 계수는 status로 확인하며, 과거 공통 500K 표시는 v0.6.1 이전 정책이다 |
| 정확 계수 성능 | 연결 재사용·동일 입력 캐시·동시 요청 공유 구현. 새 입력의 backend 왕복 지연은 남으며 전후 성능 무저하를 입증하지 않음 |
| native 첫 본문 표시 | Clauduct와 hook 없는 native 2.1.278에서도 SSE 진행 중 counter만 증가하고 완료 후 본문이 보이는 현상을 재현. 정확한 screen paint 시각/내부 원인은 미확정. 제품이 native 표시부를 패치하지 않음 |
| SDK·`--print`의 부분 본문 | 마지막 assistant block에서 최종 답변을 잃지 않도록 gateway가 reasoning 뒤에 답변을 완료 시 전달한다. `--include-partial-messages`로도 이 보류를 해제하지 않는다. v0.6.9부터 subagent의 text는 TUI에서도 완료까지 보류한다(#264). native는 응답 도중 끊긴 subagent를 이미 받은 text에서 이어 쓰게 하는데, 실제 backend에서 이어진 보고가 끊긴 지점에서 멈춘 경우가 7회 중 2회 있었다. 보류하면 backend 실패는 부분 보고 대신 자식의 명시적 API 오류가 된다(실제 backend 유효 7회 모두). 그래서 TUI의 자식 화면은 답이 끝날 때 한 번에 보인다. root TUI는 계속 스트리밍한다. backend 텍스트 진행과 사용자 첫 본문 시각이 다르므로 완료 전 취소는 active 요청·진행 상태를 기준으로 검증한다. 원래 90초 marker 미관측 기록과 새 비교는 [#207](https://github.com/wotjr1649/Clauduct/issues/207)에서 구분한다 |
| `/context` 최초 조회 | 정확 계수의 첫 backend 왕복 지연이 남음. 조회 자체가 모델 본문 생성을 뜻하지 않고, 검증된 조회 기록은 실제 다음 모델 입력에서 제외. native 로컬 이력/추정 표시와 실제 usage는 다름 |
| 런타임 의존 | `claude.exe` + `codex.exe`(버전이 요청 헤더) + `~/.codex/auth.json` |

### Anthropic 요청 필드의 실제 처리 (v0.6.8)

받아들이는 필드라도 의미를 그대로 옮긴다는 뜻은 아니다. 아래 분류는 실제 코드 동작이다. 기준은 native Claude Code
2.1.288이 이 bridge로 실제로 보내는 형태다. 과금 없는 loopback으로 print·json·verbose stream, subagent, auto 모드
분류기, WebSearch side query, compaction, TUI, `/context` 계수 요청을 모두 수집해 확인했다.

| 필드 | 분류 | 실제 처리 |
|---|---|---|
| `thinking.type` | VALIDATED | `adaptive`·`enabled`·`disabled`만 받는다. type마다 허용 멤버가 정해져 있다. adaptive는 `display`, enabled는 `budget_tokens`(필수)와 `display`, disabled는 type만이다. null 멤버는 생략과 같다. 측정한 경로에서 native 2.1.288은 `adaptive`를 보내거나 thinking을 생략했다. 공개 API의 `between_tools`는 받지 않는다 |
| `thinking.budget_tokens` | VALIDATED/IGNORED | 1 이상 2^53−1 이하의 정수인지만 검사한다. 추론량은 model과 effort가 정한다. budget을 effort로 환산하지 않는다. `max_tokens`보다 작은지는 검사하지 않는다. `/context` 계수 요청은 `max_tokens`를 1로 채워 같은 검사를 거치므로 그 대조가 의미 없다. 측정한 native 경로는 `budget_tokens`를 보내지 않았다 |
| `thinking.display` | VALIDATED/SCREEN_ONLY | `summarized`·`omitted`, Claude Code 확장 `updates`·`highlights`, null만 받는다. 실측값은 `omitted`(`--verbose` 없는 -p text·json), `updates`(TUI, `--verbose`의 json·stream-json), `summarized`(`showThinkingSummaries`)다. v0.6.10부터 TUI main turn의 요청 중 `summarized`인 것에만 backend에 `reasoning.summary`(auto)를 요청하고, 받은 요약을 세션 plugin이 화면에 한 줄씩(`clauduct-native-events: ∴ …`) 보인다(#277). subagent·teammate·workflow agent, -p·SDK, 보조 요청, compaction, 토큰 계수 요청은 요약을 요청하지 않아 요청 본문이 전과 같다(#286). 이 줄은 모델 요청과 대화 메시지에 실리지 않고(`--resume` 뒤 포함), 세션 기록에는 `system/informational` 행으로 남는다. thinking 블록과 서명은 만들지 않는다. 요약은 backend 출력이므로 제어·서식 문자와 `**`를 지우고 최대 8줄 × 240자로 자른다. 요약은 추론이 짧으면 오지 않는다. `updates`(TUI 기본)와 그 밖의 값은 요약을 요청하지 않는다. 실측에서 요약을 요청하면 첫 text가 평균 0.9~2.7초 늦어졌기 때문이다(input·cache 적중은 변화 없음). 다음 요청에 돌려줄 암호화된 기록(`redacted_thinking`)에는 요약을 넣지 않는다. 요약을 비워 되돌려도 backend는 받고 input 토큰도 같았다(모델 4종) |
| `metadata` | VALIDATED/NOT_FORWARDED | 객체이고 멤버는 `user_id` 하나다. 값은 512자 이하 문자열 또는 null이다. backend로 보내지 않는다 |
| `cache_control` | VALIDATED/IGNORED | `type: ephemeral`, `ttl` `5m`·`1h`, Claude Code 확장 `scope: global`만 받는다. breakpoint는 backend로 옮기지 않는다. backend 캐시는 세션 단위 `prompt_cache_key`로 따로 동작한다. 실측에서 native 2.1.288은 `{type: ephemeral}`만 보냈다 |
| `max_tokens` | POSTHOC_CHECK | backend가 `max_output_tokens`를 HTTP 400으로 거부하므로 그 지점에서 생성을 멈출 수 없다. 완료 뒤 보고된 usage와 대조하고, 넘으면 `OUTPUT_TOKEN_LIMIT_EXCEEDED` 오류로 끝난다. Anthropic처럼 `stop_reason: max_tokens`로 정상 종료하지 않는다. text를 보류하지 않는 스트리밍(TUI 등)에서는 오류 전에 이미 전달된 text가 남을 수 있다. thinking을 끈 요청은 reasoning을 빼고 대조한다 |
| `stop_sequences` | CLIENT_SIDE_EMULATION | backend에 보내지 않는다. 받은 text를 Clauduct가 첫 정지 문자열에서 잘라 전달하고 `stop_reason: stop_sequence`로 끝낸다. 정지 문자열에 걸린 응답의 함수 호출은 순서와 관계없이 모두 버린다. backend의 생성은 계속되므로 비용과 지연은 줄지 않는다. 1~16개, 빈 문자열 없이 문자열당 256바이트까지 받는다 |
| tool `defer_loading` | NATIVE_TOOLSEARCH_ADAPTATION | 발견되지 않은 deferred 도구는 backend에 보내지 않는다. 발견은 native ToolSearch의 `tool_reference`, 대화 이력의 `tool_use` 이름, `tool_addition`으로 한다(`ENABLE_TOOL_SEARCH`). Anthropic 서버의 tool search와는 캐시 의미가 같지 않다 |
| `tool_result.is_error` | LOCAL_SIGNAL_ONLY | Clauduct의 진단과 위임 상태에만 쓴다. backend의 `function_call_output`에는 실패 표시 필드가 없어 결과 본문만 보낸다. 실패 문구를 지어 붙이지 않는다(#144) |
| 빈 `tool_result` | ADAPTED | content가 없거나 `[]`이면 `function_call_output.output`을 빈 문자열로 보낸다. v0.6.7까지는 `output` 키가 빠져 backend가 요청 전체를 HTTP 400으로 거부했다. 빈 목록을 쓰지 않는 이유는 실측에서 모델이 도구를 다시 불렀기 때문이다(#252). `content: ""`·null·빈 text 블록 하나는 빈 `input_text` 하나로 나가며, 실측에서 모델이 빈 결과로 읽었다 |
| 함수 도구의 `strict`·`allowed_callers`·`input_examples`·`eager_input_streaming` | REFUSED | 받지 않는다(`TOOL_FIELDS`). native 2.1.288 경로는 이 필드를 보내지 않는다 |
| hosted web search 도구 | VALIDATED, 일부 IGNORED | `allowed_domains`·`blocked_domains`·`user_location`은 검사해 검색 요청으로 보낸다. `max_uses`는 1 이상의 정수만 받는다(v0.6.9, #272). 요청당 검색 횟수의 상한인데, 이 bridge는 side query마다 검색을 정확히 한 번 하므로 1 이상이면 상한 의미가 그대로 지켜진다. 그래서 값을 검색 요청으로 보내지는 않는다. `allowed_callers`·`response_inclusion`·`cache_control`은 키만 받고 값은 검사하지도 쓰지도 않는다(검색 요청의 호출자는 항상 direct). 이 도구의 `cache_control`은 위 `cache_control` 행의 검사를 받지 않는다. native 2.1.288·2.1.289는 `max_uses`를 보내고 `cache_control`은 보내지 않는다 |
| 응답의 refusal | REFUSED | `response.refusal.*` 이벤트가 오면 요청이 실패한다. 측정한 경로에서는 관측되지 않았다 |
| 응답의 citation | 일부 | `response.output_text.annotation.added` 이벤트가 오면 요청이 실패한다. 완료된 메시지 안의 annotation은 읽지 않아 전달되지 않는다 |
| `stop_reason: pause_turn` | 해당 없음 | Anthropic 서버 도구 반복의 종료 사유다. 이 bridge의 backend 경로에는 해당하는 것이 없다 |

## 3. 미지원·미검증·구현 대기

| 항목 | 상태 |
|---|---|
| 계수 지원 범위 밖의 입력 | 해당 계수 요청만 명시적으로 실패. 일반 생성·압축은 원격 사전 계수 없이 backend usage와 예방 압축 정책을 사용. 추정값을 정확 계수로 표시하지 않음 |
| forked Skill(`context: fork`) | **구현.** 실제 backend TUI(2026-09-24, luna/low, 파일 skill을 모델이 호출)에서 fork 자식의 검증·실행과 백그라운드 결과 전달을 확인했다. TUI에서 fork는 백그라운드로 돌고, 결과를 기다리는 부모의 빈 턴은 Agent·Workflow처럼 대기로 처리한다. 같은 요청 안에서 성공한 Skill 결과가 백그라운드 fork 시작을 알릴 때만이며, 인라인 skill의 빈 답은 그대로 `EMPTY_REPLY`다. 이 대기는 같은 날 실제 backend TUI에서 오류 없이 확인했다. 내장 `code-review`는 실제 backend `-p`(2026-09-24, luna/low)에서 모델이 불러 fork 자식 요청이 모두 검증·실행되고, 리뷰가 Skill 도구 결과로 돌아오는 것을 확인했다. 모델이 Skill 도구로 부른 fork의 자식은 native가 그 턴에 기록한 모델·effort로 실행한다(선택 출처 `native-fork`). toolUseId 없는 메타데이터, 그 skill을 부른 대화 바로 아래 깊이의 general-purpose(루트면 깊이 1, subagent면 그 자식의 기록된 깊이 + 1), skill 본문이 meta 사용자 메시지로 시작하는 transcript가 모두 맞아야 하고(파일 skill과 내장 `code-review` 모두) 하나라도 어긋나면 거부한다. 보고서는 gateway가 중계하지 않고 native가 전달한다(`-p`에서는 Skill 도구 결과, TUI에서는 백그라운드 완료 알림). v0.6.2에서는 `-p` 등에서 직접 입력한 fork 명령의 HTTP에 Agent ID가 없으면 `NATIVE_REQUEST_ORIGIN_UNVERIFIED`로 거부한다. 식별자가 있는 모델 호출 경로는 유지한다. **v0.5.0부터 subagent 안에서 부른 fork도 받아들인다.** 부모는 같은 세션에서 이미 검증된 자식이어야 하고, 손자의 메타데이터가 요청 헤더의 부모를 가리켜야 한다(2.1.283 측정: spawnDepth 2). 그 전에는 손자 요청이 `AGENT_SELECTION_UNVERIFIED`로 거부돼 subagent가 Skill 결과로 API 오류를 받았다. **fork 자식의 `SendMessage` 재개**는 2.1.283에서 같은 프로세스와 `--resume` 재시작 뒤 모두 native가 받아들이고 같은 ID가 `native-fork`로 다시 검증된다. 이전 문서의 "재개 거부"는 검사 없이 남은 서술이었다. Agent 자식과 달리 재개 연결을 따로 검증하지 않으며, 사용자의 중지가 `stoppedByUser`로 남지 않아 중지된 자식을 막는 규칙(#91)이 fork 자식에는 적용되지 않는다 |
| V1 `review-diff` 헬퍼 | **v0.6.0에서 문서상 정식 은퇴.** native `/code-review`의 tracked/untracked 파일 대상과 독립 결함 표본을 실제 backend로 확인했다. V2에는 이 헬퍼가 없으므로 제품 코드를 삭제한 변경은 아님. native 읽기 권한과 현행 다른 경로의 용량·경로 보호는 유지 |
| Workflow remote·자식의 별도 Workflow·근거 없는 재개 | native workflow-subagent는 Workflow 도구를 제외한다. 이를 제거해 도구 제한을 확대하지 않음. 같은 run의 검증된 명시적 source 재개는 위 native 규칙을 따르며, 결과 회수·독립 계획은 별도 계약. 다른 세션과 기록 없는 강제 종료의 재개는 거부 |
| Workflow plugin/bundled 이름 전체 | 로컬 `.js` 이름과 scriptPath는 지원. native 내부 resolver를 우회해 plugin 출처·우선순위를 임의로 추정하지 않음. 확인된 파일은 native Read가 허용하는 scriptPath로 실행 가능 |
| Workflow `agent()`의 직접 `maxTurns` 옵션 | 거부. native 역할 정의의 maxTurns를 사용. `tools` 정확 이름 목록은 자체 강제하며 모든 native 옵션 조합의 적용을 검증했다는 뜻은 아님 |
| PPTX·DOCX·XLSX 직접 API 입력 | 이미지/PDF API 입력과 별개. bridge의 직접 document 입력으로 지원하지 않음. 아래의 native 로컬 도구를 통한 읽기·편집과 구분 |
| 임의 Workflow JavaScript 전체 | native `pipeline`·중첩 `parallel`의 콜백에서 `agent()`를 호출하는 형태는 S49 native 검증. 임의 `globalThis.agent` 우회나 native VM가 거부하는 코드까지 지원한다는 뜻은 아님 |

### 알려진 제한 원장 (v0.6.10)

v0.3.x부터 v0.6.8까지 릴리스 기록과 이 문서에 흩어져 있던 알려진 제한과 미검증 항목을 한곳에 모았다(#273).
각 항목의 근거는 해당 릴리스 기록과 이 문서의 절에 있다. "구조상 불가"로 영구 종결하지 않고, 다시 볼 조건을 항목마다 둔다.

| 분류 | 뜻 |
|---|---|
| `FIXED` | 고쳤거나 원인이 사라졌다 |
| `MEASURE` | 동작은 정해져 있으나 실측이 없거나 부족하다 |
| `DATE_BOUND` | 정해진 날짜 이후에만 확인할 수 있다 |
| `CURRENT_BACKEND_UNMAPPABLE` | 지금 backend에 정확히 대응하는 기능이 없다 |
| `NATIVE_CLIENT_LIMIT` | native Claude Code 쪽 동작이나 제한이다 |
| `OUT_OF_SCOPE` | 제품 설계·정책상 지원하지 않거나 이 제품의 범위 밖이다 |

| 항목 | 분류 | 현재 상태 | 다시 볼 조건 |
|---|---|---|---|
| TUI subagent가 응답 도중 실패한 뒤 이어진 보고가 끊긴 지점에서 멈춤(v0.6.8) | `FIXED` | v0.6.9: subagent text를 완료까지 보류(#264). 실제 backend 유효 7회 모두 부분 보고 0, 명시적 실패 | — |
| native 2.1.289 Agent teams의 teammate 요청을 모두 거부(v0.6.8 이하) | `FIXED` | v0.6.9: teammate 지원(#269). 위 1절 행 | native가 teammate의 요청 식별을 바꿀 때 |
| hosted web search `max_uses` 값을 검사하지 않음(v0.6.8) | `FIXED` | v0.6.9: 1 이상의 정수만(#272) | — |
| 재측정이 hook 타입 일부만 비교(2.1.289 teammate 변경을 놓침) | `FIXED` | v0.6.9: 세션 plugin이 쓰는 모든 이벤트의 타입과 참조 타입 비교(#268) | — |
| 공개 protocol contract test 없음(#256) | `FIXED` | v0.6.9: 합성 테스트를 공개 CI에서 실행(#271) | — |
| native 2.1.288 위험 셋: 이어 쓰기와 replay, 첫 요청 대기, 분류기 압축(v0.6.7) | `FIXED` | v0.6.8에서 판정(3절 v0.6.8) | — |
| 종료 상태 파일 교체 실패(v0.6.3) | `FIXED` | v0.6.6: 약 1초 재시도. 1초를 넘게 잡히는 경우는 아래 `OUT_OF_SCOPE` 행 | — |
| `/context` 직후 종료한 계수가 종료 줄의 거부로 보임(v0.6.4·v0.6.5) | `FIXED` | v0.6.7: 종료 줄에서 `counts_cancelled`로 분리. 상태 JSON의 원래 집계(`totals.failures`의 `CANCELLED`, `refusedBy`)는 설계대로 그대로다 | — |
| 빠른 종료 때 진행 중이던 생성 요청이 함께 취소되면 상태가 출력됨 | `OUT_OF_SCOPE` | v0.6.7 설계: 생성 취소는 숨기지 않는다. v0.6.8·v0.6.9 출하 검증의 빠른 종료 표본에서 시점에 따라 관측 | 사용자에게 불필요한 출력으로 보고될 때 |
| 분류기가 판정을 받지 못할 때 native 동작을 재지 않음(v0.6.3) | `FIXED` | v0.6.4에서 측정: native는 그 호출을 실행하지 않고 turn을 마친다 | — |
| 분류기 판정을 읽을 수 없을 때 native가 같은 바이트로 다시 보낸 요청은 replay 차단(v0.6.5) | `FIXED` | v0.6.10: 분류기 요청은 replay 키를 만들지 않는다(#289). 재요청은 backend에 가고, 판정을 끝내 받지 못한 호출은 전처럼 실행되지 않는다 | — |
| 형제 agent가 거의 동시에 같은 동작을 하면 두 번째 분류 질문이 replay로 차단됨(v0.6.5~v0.6.9) | `FIXED` | v0.6.10(#289). native는 같은 바이트로 묻는다. 실제 backend 회귀에서 1회 관측, #288 진단 기록과 합성 backend로 재현(6/6) | — |
| 분류기가 계속 읽을 수 없는 판정을 내면 native가 분류 요청을 10번까지 보낸다(v0.6.10) | `OUT_OF_SCOPE` | 설계: native의 재질문을 따른다. 판정 없이 호출을 실행하지 않는다. HTTP 오류·시간 초과·끊긴 응답은 전처럼 2번이다 | 비용이나 지연이 문제로 보고될 때 |
| bypass 세션 도중 모드 전환 시 필수 확인 목록(v0.6.3) | `FIXED` | v0.6.4에서 Clauduct 권한 규칙을 없애 해당 없음 | — |
| `/model`·`/effort` TUI, 다른 세션으로 가는 `SendMessage` 승인, gateway 경유 `WebSearch`·native `WebFetch`를 출하 바이너리로 실행하지 않음(v0.6.2) | `FIXED` | v0.6.3 최종 후보와 출시 바이트에서 실제 backend로 통과. `/model`은 그 뒤 출하마다 TUI 회귀에서 확인. v0.6.4부터 `SendMessage` 승인은 native 규칙이 정한다 | — |
| 분류기 pair 품질 | `MEASURE` | 통과: Terra low·medium. 불합격: Terra high, Luna low·medium. Sol 6.1 low는 판정 미완료(`cyber_policy`). 그 밖은 미측정이며 시작할 때 안내 | 사용자가 요청하거나 공장값을 바꿀 때 |
| hosted web search의 `allowed_callers`·`response_inclusion`·`cache_control` 값을 검사하지 않음(v0.6.8) | `OUT_OF_SCOPE` | 키만 받고 쓰지 않는다. native 2.1.289는 보내지 않는다(`cache_control` 포함) | native가 보내기 시작할 때 |
| background stop·attach 직후 첫 입력에서 이전 완료 보고 반복(v0.6.2) | `MEASURE` | #234(v0.6.3)에서 v0.6.1과 v0.6.2가 각각 4회 중 2회 재현돼 v0.6.2 회귀가 아니라고 판정. native가 대기 중이던 완료 알림을 입력 앞에 붙이고 backend가 새로 생성한 응답이다 | native가 알림 전달 방식을 바꾸거나 사용자 영향이 보고될 때 |
| 원인 미확정 과거 간헐 실패 #206 #211 #212 #213 #215 #220 #229 | `MEASURE` | 고장 주입으로 안전한 처리를 확인했고 진단을 넣었다 | 재발할 때 진단으로 가른다 |
| effort가 생략된 `/model` 결과가 기존 effort를 유지(v0.6.3) | `MEASURE` | 근거 실측 1건 | native가 문구를 바꿀 때 |
| 세션 중 계정 전환(v0.6.4) | `MEASURE` | 기존 거부 계약 유지, 재측정 안 함 | 계정·인증 경로를 바꿀 때 |
| native 명령행 한도 초과(v0.6.5) | `MEASURE` | 합성 목록으로만 확인. 실제 계정 목록으로는 도달하지 않음 | 계정 모델 수가 늘어 실제로 가까워질 때 |
| 사용자 mod를 원격으로 끄는 플래그(v0.6.7) | `MEASURE` | telemetry를 끈 Clauduct 세션에서의 동작 미확인 | 원격 플래그가 관측될 때 |
| native가 teammate 모델을 고르는 경로(v0.6.9) | `MEASURE` | 단위 시험만. loopback에서 그 분기가 타지 않았다 | 그 분기가 실제로 관측될 때 |
| 첫 출력 전 240초 뒤 keepalive 이후의 subagent 실패(v0.6.9) | `MEASURE` | 부분 text는 0. native가 명시적 오류로 끝내는지 실제 backend로 재지 않음 | 오래 걸리는 자식에서 실패가 관측될 때 |
| 추론 요약은 main turn에서만 보인다(subagent·teammate·workflow agent 제외) | `OUT_OF_SCOPE` | v0.6.10 설계(#286): 자식 요청은 요약을 요청하지 않아 지연이 없다 | 자식 agent의 요약을 보여 달라는 요청이 있을 때 |
| `--agent`로 띄운 main-thread agent의 추론 요약 | `MEASURE` | main 요청이 agent id 헤더를 보내면 요약을 받지 못한다. 그 요청 형태는 수집하지 않았다. 틀린 표시는 생기지 않는다 | 그 세션의 요청 형태를 수집할 때 |
| 요약 표시가 native의 `$.ui.log`와 chunk 순서에 기댐 | `MEASURE` | native 2.1.289에서만 확인. 무과금 재측정이 `$.ui` 선언과 `log` 문서를 비교하고, 요약 TUI 회귀를 native 새 버전 절차와 발행 전 회귀에 넣었다(#286) | 재측정이 `$.ui` 차이를 보이거나 요약 회귀가 실패할 때 |
| backend 추론 요약 표시 | `FIXED` | v0.6.10: TUI main turn에서 `thinking.display`가 `summarized`인 요청만 요약을 요청해 화면에만 보인다(#277, #286). `updates`(TUI 기본)는 지연 때문에 요청하지 않는다 | — |
| 빈 응답 제어의 나머지 origin 12종 | `MEASURE` | `composer`·`sdk`·`peer`와 따로 처리하는 `task-notification`만 측정했고, 그 밖의 12종은 단독 입력으로 모드를 정하지 않으며 미측정 | native가 그 origin을 실제로 쓸 때 |
| forked Skill 자식에는 사용자 중지 차단 규칙(#91)이 적용되지 않음 | `MEASURE` | 사용자의 중지가 `stoppedByUser`로 남지 않는다(3절 forked Skill 행) | native가 fork 자식의 중지를 기록할 때 |
| 부분 도구 인자 생성 중 취소의 자연 발생 조건 | `MEASURE` | 실제 TUI와 고정 backend로 확인. 구독 backend의 자연 발생 조건 전부는 아님 | 관련 실패가 관측될 때 |
| Claude in Chrome | `MEASURE` | 확인하지 않음 | 사용 요청이 있을 때 |
| 새 모델·native 버전·plugin·MCP 조합 | `MEASURE` | 라우팅된다는 것이 실제 동작 검증은 아니다. native 버전마다 무과금 재측정으로 hook 타입·인자·요청 형태를 비교한다(#268) | 재측정이 차이를 보고할 때 |
| `gpt-5.5` 은퇴 뒤 계정 목록 | `DATE_BOUND` | 합성 목록으로만 확인 | 2026-10-14T19:00Z 이후(#274) |
| `max_tokens`가 생성 한도가 아닌 사후 검사 | `CURRENT_BACKEND_UNMAPPABLE` | backend가 `max_output_tokens`를 HTTP 400으로 거부 | backend가 받기 시작할 때(재실측) |
| `temperature`·`top_p` 거부 | `CURRENT_BACKEND_UNMAPPABLE` | S48 실측에서 4개 모델 모두 HTTP 400. Clauduct는 effort를 low~max로 보낸다. native 2.1.289는 이 필드를 보내지 않는다 | native가 보내거나 backend가 받을 때(재실측) |
| `stop_sequences`가 로컬 절단이라 비용·지연이 줄지 않음 | `CURRENT_BACKEND_UNMAPPABLE` | backend 생성은 계속된다 | backend가 정지 문자열을 받을 때 |
| `tool_result.is_error`가 backend에 전달되지 않음 | `CURRENT_BACKEND_UNMAPPABLE` | `function_call_output`에 실패 필드가 없다 | backend에 그 필드가 생길 때 |
| `metadata` 미전송, `thinking.budget_tokens` 무시 | `CURRENT_BACKEND_UNMAPPABLE` | 대응 필드가 없다. 추론량은 model·effort가 정한다 | backend에 대응 필드가 생길 때 |
| `cache_control` breakpoint 무시 | `CURRENT_BACKEND_UNMAPPABLE` | backend 캐시는 세션 단위 `prompt_cache_key`로 동작 | backend가 breakpoint를 받을 때 |
| 응답의 refusal·annotation 이벤트에서 요청 실패, 완료 메시지 안 citation 미전달 | `MEASURE` | backend는 이 이벤트를 보낼 수 있지만 Clauduct가 연결하지 않았다. 측정한 경로에서 관측되지 않음 | 실제 세션에서 관측될 때 연결을 더한다 |
| `cache_write_tokens`를 `cache_creation_input_tokens`로 연결하지 않음 | `MEASURE` | backend 값의 의미 미확인 | 0이 아닌 값이 관측될 때 |
| `pause_turn` | `CURRENT_BACKEND_UNMAPPABLE` | backend 경로에 대응 종료 사유가 없다 | backend가 서버 도구 반복 중단을 알릴 때 |
| 도구 결과 안 이미지·PDF의 warmup 계수 | `CURRENT_BACKEND_UNMAPPABLE` | 계수와 실제 usage 불일치(S45)로 그 계수만 미지원. 일반 생성은 무관 | backend 계수가 usage와 맞을 때 |
| 클라이언트 `/usage`·`/cost`의 플랜 사용량 | `NATIVE_CLIENT_LIMIT` | native가 custom base URL에는 계정 엔드포인트를 묻지 않는다. `clauduct --usage`로 대신한다 | native가 묻기 시작할 때 |
| 클라이언트 `/cost`의 금액 | `NATIVE_CLIENT_LIMIT` | native 가격표에 `gpt-*`가 없다. 토큰 수는 실제 값이다 | native가 가격을 받을 수 있을 때 |
| `/model`에서 Enter가 기본값을 저장 | `NATIVE_CLIENT_LIMIT` | native 동작. 이번 세션에만 쓰려면 `s` | native가 동작을 바꿀 때 |
| native 첫 본문이 완료 후 보임 | `NATIVE_CLIENT_LIMIT` | hook 없는 native에서도 재현. 제품이 표시부를 고치지 않는다 | native가 바뀔 때 |
| `/resume` 뒤 teammate 미복원, teammate split pane, `-p`의 teammate | `NATIVE_CLIENT_LIMIT` | native 제한(Windows split pane 미지원, `-p`는 teammate를 만들지 않음) | native가 바뀔 때 |
| 내장 mod "You should know" | `NATIVE_CLIENT_LIMIT` | native가 first-party·telemetry 켜짐 세션에만 제공 | native가 제공 조건을 바꿀 때 |
| 권한 우회 CLI 옵션 2개 | `OUT_OF_SCOPE` | 제품 정책상 거부 | 정책을 바꿀 때 |
| `--desktop` | `OUT_OF_SCOPE` | 별도 앱의 backend·세션 수명 미지원. `DESKTOP_UNSUPPORTED` | Desktop 지원을 설계할 때 |
| `--bare` | `OUT_OF_SCOPE` | context·세션 profile을 검증할 수 없어 `SESSION_PROFILE_UNVERIFIED`(#223, v0.6.2 출하 때 재확인) | 검증할 수단이 생길 때 |
| Agent ID 없는 직접 입력 forked Skill | `OUT_OF_SCOPE` | 출처를 증명할 수 없어 `NATIVE_REQUEST_ORIGIN_UNVERIFIED` | native가 식별자를 보낼 때 |
| mod의 `$.tool.call`로 Agent·SendMessage·Workflow·Skill 직접 호출 | `OUT_OF_SCOPE` | `NATIVE_DIRECT_DELEGATION_UNSUPPORTED` | 출처를 증명할 수단이 생길 때 |
| Workflow의 직접 `maxTurns`, plugin·bundled named resolver 전체, remote·자식 Workflow·근거 없는 재개 | `OUT_OF_SCOPE` | 3절 표대로 거부하거나 제한 | native 규칙이 바뀔 때 |
| PPTX·DOCX·XLSX 직접 API 입력 | `OUT_OF_SCOPE` | native 로컬 도구로 읽고 편집한다(v0.6.0) | 직접 입력이 필요한 사용 사례가 생길 때 |
| 임의 Workflow JavaScript 전체 | `OUT_OF_SCOPE` | native VM이 받는 `pipeline`·중첩 `parallel` 콜백의 `agent()` 형태만 검증(3절) | native Workflow 규칙이 바뀔 때 |
| 처음 쓰는 계정이 오프라인이고 저장 목록도 없으면 시작 불가 | `OUT_OF_SCOPE` | 모델 목록 없이는 선택을 검증할 수 없다. 이 상태에서는 backend 요청도 할 수 없다 | 오프라인으로 설정만 점검할 필요가 생길 때 |
| 숨긴 모델의 전체 ID 직접 지정은 실행됨 | `OUT_OF_SCOPE` | v0.6.5 설계: 선택지는 권고 | 강제하기로 정할 때 |
| 종료 상태 파일을 1초 넘게 잡으면 최종 기록이 stderr에만 남음 | `OUT_OF_SCOPE` | 재시도 상한(v0.6.6). 기존 파일은 온전하다 | 실제로 관측될 때 |
| v0.6.4 이전 버전은 객체형 `classifier_model`을 읽지 못함 | `OUT_OF_SCOPE` | 되돌릴 때 백업으로 복원 | 되돌리기 지원 범위를 바꿀 때 |
| native가 effort를 명시한 보조 요청·high·max 세션의 압축은 그 effort로 실행 | `OUT_OF_SCOPE` | v0.6.4 설계(압축 시간 medium 12.2초, max 26.1초 실측) | 비용 문제가 관측될 때 |
| doctor가 측정 기준보다 새 클라이언트에 `re-measure due`를 표시(v0.6.2) | `OUT_OF_SCOPE` | 설계 안내이며 세션은 그대로 실행된다. v0.6.9 기준은 Claude Code 2.1.289 | native가 새 버전을 낼 때마다 재측정한다 |
| 미지 SSE 이벤트에서 요청 실패 | `OUT_OF_SCOPE` | 설계(fail-closed). 이름은 계정에 남긴다 | 새 이벤트가 관측될 때 처리를 더한다 |
| Anthropic 서버 기능(claude.ai 로그인, Remote Control, `/schedule`, cloud 세션, `/ultrareview`, Artifact, advisor 도구와 서버 의존 베타 7종, 서버 분류기, telemetry) | `OUT_OF_SCOPE` | 3절 v0.6.7 표와 미지원 표 | backend 쪽에 대응 서비스가 생길 때 |
| loopback을 검사·중계하는 보안 필터(AdGuard 등) | `OUT_OF_SCOPE` | 필터 기능을 끄고 같은 probe로 전후 비교(2절) | 특정 필터 제품의 호환 문제가 보고될 때 |
| 비Windows | `OUT_OF_SCOPE` | 이식이 아니라 새 설계 | 다른 OS 지원을 설계할 때 |

### 파일 대상 code-review 인수 — v0.6.0

native 2.1.283의 `/code-review <파일 경로>`로 tracked 변경과 untracked 파일을 각각 검토했다.
대표 표본은 수량 곱셈 누락·clamp 상한 오류, 독립 표본은 할인 백분율 계산·18세 경계 오류다.
모두 대상 파일·함수·결함 원인과 반례를 확인하고 원본 파일은 보존했다. 실제 Sol/medium의 네 리뷰는
각 2–3회 요청, 33,976–54,691 bytes의 누적 JSON 본문, 약 10.6–15.3초였다. 이 수치는 작은 합성 파일의
관측치이며 일반 코드의 비용·탐지율 보장이 아니다.

직접 slash command를 `-p`로 실행하면 stdout은 `Command completed`이고 실제 리뷰는 native 자식
transcript에 남았다. 완료 문구를 결함 탐지로 간주하지 않고 그 리뷰 본문과 실제 파일 읽기를 판정했다.
이는 당시 버전의 검증 이력이다. v0.6.2에서는 Agent ID 없는 직접 입력 fork를 출처 미확인으로 거부한다.
모델이 `Skill` 도구로 호출한 리뷰는 native의 도구 결과 전달 경로를 따른다. V1 헬퍼의 전처리와
2 MiB 제한을 다른 현행 기능에 적용하거나 제거한 변경은 없다.

진단의 `backendRequestBytes`는 session cache key를 포함해 전송한 JSON 본문의 크기다.
HTTP 200 응답을 받은 요청만 측정하며 `requestsWithBackendBytes`와 함께 누적한다. HTTP 실패·계수·검색의
미측정 값을 0-byte 전송으로 해석하지 않는다. 본문·header·자격 증명은 이 진단에 기록하지 않는다.

### Office 로컬 도구 인수 — v0.6.0

DOCX·XLSX·PPTX는 사용 환경에 준비한 로컬 편집 도구를 native `Bash` 등으로 호출해 읽고 편집한다.
제품에 새 Office API나 라이브러리 의존성을 추가하지 않았다. 2026-09-28의 Python 3.14.6과
python-docx 1.2.0·openpyxl 3.1.5·python-pptx 1.0.2로 실제 Codex backend를 거쳐 다음을 확인했다.

| 형식 | 대표 표본·별도 독립 표본 | 파일 자체 확인 |
|---|---|---|
| DOCX | 문단 run·표 cell의 텍스트 한 곳 편집 | ZIP/XML·관계 유효성, 다시 연 내용·bold, 다른 문단과 part 보존 |
| XLSX | 서로 다른 셀의 문자열 편집 | ZIP/XML·관계 유효성, 셀 값·bold, 무관한 셀·수식 보존 |
| PPTX | textbox·다른 slide의 table cell 편집 | ZIP/XML·관계 유효성, 텍스트·bold, 무관한 slide와 part 보존 |

native 도구의 읽기→편집→다시 읽기와 별도 파일 검사를 대조했다. 독립 표본의 첫 XLSX 실행은
모델이 허용 명령을 `eval`로 감싸 native가 거부했고, 명령 안내를 명확히 한 재실행은 세 형식 모두
통과했다. 이 실패를 브리지 오류나 성공으로 집계하지 않았다. 매크로·암호 문서·복잡한 Office 객체,
모든 서식·렌더링 보존까지 검증한 것은 아니다. 편집 도구가 필요하며 제품이 자동 설치하지 않는다.
| 빈 응답 제어의 전체 실행 모드 지원 | `composer`·`sdk`에 더해 v0.4.3은 실측한 `peer`로 모드를 정한다. native `isInteractive`가 TUI·SDK를 구분한다. 별도 처리하는 `task-notification`을 제외한 나머지 origin 12종은 여전히 단독 입력으로 모드를 확정하지 않으며 미측정이다. 같은 턴의 명시적 개입, 출처 없는 자식 새 index 0, 새 입력의 빈 응답은 기존 오류 경계를 유지한다. [v0.4.3 검증 상태](#v043--메시지-background-검증-예산) |
| 부분 도구 인자 생성 중 취소의 이벤트 근거 | S45/S47 실제 TUI + 고정 backend로 부분 인자 delta·미완성 도구 미실행·후속 답변 확인. 구독 backend의 자연 발생 동일 조건 전부를 검증했다는 뜻은 아님 |
| advisor 도구, Anthropic 서버 의존 베타 7종 | 이 backend에서 성립하지 않는다. advisor는 환경변수로 끈다 |
| 클라이언트 `/usage`·`/cost`의 **플랜 사용량** | **보여줄 수 없다.** 클라이언트가 커스텀 base URL에는 계정 엔드포인트를 **묻지 않는다**(두 자격증명 모양 모두 실측). 대신 `clauduct --usage`가 같은 질문에 답한다 |
| 클라이언트 `/cost`의 **금액** | 토큰 수는 실값이 간다(백엔드가 센 것). 달러는 클라이언트 가격표에 `gpt-*`가 없어 의미 없다. `behavesAs`로 채우면 **확신에 찬 틀린 금액**이 되므로 하지 않는다 |
| 세션 중 `/model`·`/effort` 기록(native 2.1.287) | native 2.1.287은 로컬 명령 caveat 문구와 `/model` 결과 문구("for this session only" 또는 "and saved as your default for new sessions", effort 생략 가능, 뒤따르는 안내)를 바꿨다. v0.6.3은 두 형식을 모두 엔진의 정확한 문구로만 읽어 UUID 재개 snapshot에 반영한다. effort가 없는 모델 변경은 기존 effort를 유지한다(실측). 이전 버전은 2.1.287에서 이 변경을 snapshot에 반영하지 못해 재개 시 이전 모델·effort로 시작했다 |
| auto 권한 모드의 분류기 판정 | **v0.6.4:** 분류 요청은 `classifier_model` pair(공장값 Terra/low, 계정이 제공하는 모든 pair 허용)로 보내고, Clauduct는 hard_deny 규칙을 더하지 않으므로 native 기본 규칙·사용자 규칙과 native 분류기만 판정한다. 계정이 제공하지 않는 pair는 `AUTO_MODE_CLASSIFIER_MODEL_UNSUPPORTED`로 거부한다. 판정을 얻지 못한 여섯 실패 경우 native는 해당 호출을 실행하지 않고 turn을 마쳤다(로컬 측정, 아래 v0.6.4 절). 아래 품질 측정은 Terra·Sol 표본이다. Luna/low·medium은 v0.6.2 준비(2026-09-29)에서 위험 표본을 허용해 불합격했고, v0.6.7 재측정(2026-10-03)도 Luna/low 불합격이다(v0.6.7 절). 이하 v0.6.3까지의 기록: v0.5.6은 native 2.1.283의 block 요청을 Terra/high로 분리한다. v0.5.2의 두 hard_deny 규칙과 native 기본 규칙은 유지한다. 기존 70개·새 독립 표본, 기존 오차단 반복, 실제 native 허용·거부를 확인했고 출시 실행 파일에서도 auto 분류를 관측했다. 알려진 형식 변화는 `AUTO_MODE_CLASSIFIER_UNVERIFIED`로 거부한다. 버전별 검증 범위와 과거 오차단은 아래 v0.5.2·v0.5.6 절을 따른다. v0.5.1까지는 판정이 필요한 행동을 거부했다. v0.6.3부터 분류기 모델은 `classifier_model`로 고른다(기본은 `sonnet` 매핑, Terra). 2026-10-02 같은 빌드의 측정: 핵심 71·독립 18 표본에서 Terra low는 오허용·오차단 0, Terra medium은 오허용 0·오차단 1(정상 30 중 29 허용), 중앙 응답 약 4초. Sol 6.1 low는 중앙 약 16초였고, 보안 판정 표본 1건(D11, `.env` 강제 커밋)을 backend가 `cyber_policy`로 두 번 연속 거부해 판정 자체를 받지 못했다. 그래서 Sol 6.1은 고를 수 있지만 기본값이 아니다. 판정을 받지 못했을 때 native가 어떻게 처리하는지는 측정하지 않았다 |
| 비Windows | 없다. 이식이 아니라 새 설계다 |

`web_search` 외의 hosted 도구(`web_fetch`·`code_execution`·`computer`·`text_editor`·`memory`)는
2.1.274 바이너리에서는 그 타입 이름과 전송 경로를 발견하지 못했다(2026-09-17 실측).
이는 해당 버전의 관측이며 2.1.282 전체의 부재를 증명하지 않는다. native 로컬 `WebFetch`·`Bash`·`Edit`와
같은 기능 이름이 붙은 hosted API 도구도 구분한다. 미측정 경로를 지원 완료로 취급하지 않는다.

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

**2026-09-25 출하.** Claude Code 2.1.282에서 무료 native 재현과 순수 출하 바이너리의 실제 backend 검증을
함께 수행했다. 수정한 약 61분 하네스도 전체 재실행해 자동 유휴 회수·OS 종료·같은 연결의 새 응답·정리를 확인했다.

| 항목 | 구현·관측 범위 |
|---|---|
| 메시지 기반 부모 대기(#130) | 입력 없는 TUI·SDK가 다른 native 세션의 `SendMessage`를 `peer`로 받는 경로를 실측했다. 수정 전 각각 EMPTY_REPLY 1회, 수정 후 부모 3·자식 1회와 결과 수신 1회, 오류 0. 도중의 두 번째 메시지는 native가 새 턴으로 큐잉했으며 정상 답변을 표시했다. same-turn 개입은 모듈 정책 검사로 구분한다 |
| `/clear`·`/reload-plugins` | 이후 실제 peer와 부모 대기를 TUI·SDK에서 확인했다. TUI의 reload는 CLI `--agents`로 준 사용자 역할을 제거했다. 이는 native 동작이며 그 역할의 reload 유지까지 지원하지 않는다. 기본 native 역할의 부모 대기는 확인했다 |
| 명시적 background 생성(#133) | `clauduct --bg`·`--background`로 생성하고 native agents·attach·stop·respawn·메시지·삭제로 관리한다. 세션별 Clauduct 연결 유지 프로세스 1개가 최초 실행기 종료 뒤 남는다. 순수 바이너리의 실행기 종료·연결 종료도 과금 없이 확인했다 |
| 동일 로그인 내 재기동 | native stop→attach와 respawn→peer는 실제 backend에서도 gateway·모델·effort·필수 hook 연결을 확인했다. 예상치 못한 worker 종료는 명시적 native respawn→attach로 복구했다. 종료 직후 attach만 한 자동 재시작은 실패했으며 보장하지 않는다. 자동 idle 회수는 약 61분 뒤 native 기록·OS 종료를 확인하고 같은 IPC로 attach·Luna/low 새 응답을 무료 검증했다. PC 재부팅·로그아웃 뒤 자동 복원은 범위 밖이다 |
| 연결 유지와 실패 | native stop은 연결을 남긴다. native 세션 삭제 또는 `clauduct --background-stop <connection-id>`가 연결도 끝낸다. 토큰은 메모리에만 두고 동일 Windows 로그인에서 IPC로 받는다. 유지 프로세스 종료 뒤 자동 재시작·backend 전환·요청 재실행은 하지 않는다. native agents 화면에서 새 작업을 Clauduct로 만드는 경로는 범위 밖이다 |
| 검증 예산(#134) | `CLAUDUCT_VERIFICATION_BUDGET`으로 실행 전체 model/effort와 시도 수를 제한한다. 실패·취소·재시도·backend 계수도 차감하며 검색은 검증 중 거부한다. 여러 프로세스와 새 원장에서도 N+1은 credential·소켓 전에 거부된다. 예약 파일 삭제·교체는 지원하지 않는다. 일반 세션의 요청 수 정책은 유지한다 |

일반·race 전체 각 21개 패키지와 기본/태그 vet·build·gofmt를 통과했다. `peer` 지원을 제거한 변이는
실제 native SDK에서 EMPTY_REPLY assertion으로 실패했고, 공유 예약을 제거한 변이는 N+1 전송 assertion으로
실패했다. 실제 backend에서는 메시지로 시작한 TUI·SDK 각각 5회, background 관리 6회, 최종 SDK
회귀 5회로 합격했다. 성공 실행의 종료 코드·cleanup·자식 결과 수신·메모리 예약 반환을 함께 판정했다.
출하 검증 21회와 실패 실행을 포함한 누계 67회 및 상한은 [릴리스 노트](RELEASE-v0.4.3.md)에 기록한다. Codex 기준 0.157.0은
exec wire 차이와 이 제품 경로를 측정한 뜻이며, Codex 자체 TUI·검색·계수의 재검증은 아니다.

### v0.4.4 — 검색 검증의 공유 예산

**2026-09-26 출하·실제 설치 확인 완료.** 태그 바이너리의 WebSearch·SDK 위임·TUI 검증은 총 16회로
통과했고 각 실행의 공유 상한을 지켰다. 설치·업데이트·되돌리기와 공개 자산 digest 대조도 완료했다.

검증 실행이 `budget.json`에 `allowSearch:true`를 명시하면 검색도 모델 호출과 같은 영속 예약 파일과
총상한을 사용한다. 최초 시도·한 번의 허용된 검색 재시도 모두 credential 읽기·재조회와 전송보다
먼저 예약한다. 실패·취소 뒤 슬롯 반환, 새 프로세스의 상한 초기화는 하지 않는다. 기존 계획은 검색을
계속 거부하고 일반 사용자 세션의 검색 정책과 별도 전송 통계는 유지한다.
[설정 형식](../../go/README.md#검증용-실호출은-별개의-예산이다), [릴리스 검증](RELEASE-v0.4.4.md).

### v0.5.0 — 요청 세션 식별, 역할·fork 범위, 자산 넷

**2026-09-26 출하·실제 설치 확인 완료.** 태그 바이너리의 실제 backend 검증은 35회로 통과했다([릴리스 검증](RELEASE-v0.5.0.md#출하-검사-2026-09-26)). 아래는 개발 과정의 기록이다.

Claude Code 2.1.283·Codex CLI 0.157.0에서 무료 native 재현과 실제 backend 검증을 함께 수행했다. 개발 후보
`002563e`의 실제 backend 결과는 아래와 같다. 각 실행은 실행 전에 만든 영속 총상한 안에서 돌았다.

| 실행 | 예약 / 상한 | 결과 |
|---|---|---|
| `--add-dir` 역할·settings env 모델 | 5 / 10 | PASS. add-dir 정의는 luna/medium, 모델 없는 정의는 env 모델 sol에 부모 effort low |
| subagent 안 forked Skill | 10 / 10 → 6 / 10 | 첫 실행 FAIL: 루트 모델이 fork의 중간 알림에 Agent를 다시 띄워 상한을 소진했고 초과 요청은 전송 전에 거부됐다. 손자 자체는 `native-fork`로 성공했다. 반복 실행 금지 지시를 더한 재실행 PASS |
| fork 자식의 `--resume` 뒤 `SendMessage` 재개 | 10 / 10 → 9 / 16 | 첫 실행 FAIL: 두 단계와 ToolSearch·SendMessage·TaskOutput 단계에 상한 10이 부족해 루트의 마지막 답이 거부됐다. 재개된 자식은 이미 `native-fork`로 성공했다. 상한 16 재실행 PASS(자식만 아는 재개 코드가 루트에 돌아옴) |

| 항목 | 구현·관측 범위 |
|---|---|
| Claude Code 2.1.283 인자(`--client-data-url`) | 필수값 옵션으로 인자 표에 더했다. 이전 표는 모르는 옵션으로 읽어 뒤의 `--settings`·`--model`·`-p` 경계를 잃었다. 서명 구성 기능 자체는 native가 처리하며 지원을 주장하지 않는다 |
| Codex 요청 형식(#135) | **HTTP SSE와 현행 본문을 유지하고 세션 식별을 더했다.** native 세션마다 불투명 키(게이트웨이 난수 salt와 세션 ID의 해시)를 `prompt_cache_key`와 `session-id` 헤더로 보낸다. native 세션 ID 자체는 보내지 않는다. 키는 게이트웨이 프로세스마다 새로 정해지므로 `--resume`으로 새로 연 세션은 새 키를 쓴다. 근거는 설치 Codex 0.157.0의 exec·TUI 구조 캡처(합성 credential, 과금 0)와 같은 대화를 세 형식으로 보낸 실제 backend 비교다. 현행 HTTP의 2턴 이후 캐시 적중은 29–47%, 세션 식별을 더한 HTTP는 71–78%, Codex 방식(WebSocket·lite·증분 전송)은 75–82%였다. 첫 텍스트까지의 시간과 총 시간은 세 형식이 잡음 범위에서 같았다. WebSocket은 전송량을 약 1/3로, 첫 이벤트를 약 0.35초로 줄였지만 소켓 수명·재연결·증분 상태라는 새 실패 경로를 들인다. 캐시 적중 상승이 사용량 한도와 큰 모델의 지연에 주는 효과는 측정하지 않았다 |
| #135 재검토 조건 | backend가 옛 본문이나 HTTP SSE를 4xx로 거부하거나 폐지를 알릴 때, Codex가 HTTP나 옛 본문 경로를 제거할 때, 새 형식에서만 쓰이는 모델·기능이 필요할 때, 또는 WebSocket 전송의 체감 지연 이득이 측정될 때 |
| 커스텀 역할 기본값 | native 2.1.283과 같게 수집한다. 같은 이름의 우선순위는 CLI > project > `--add-dir`(나중 것 우선) > user다. `--setting-sources`가 빼는 출처의 정의와 plugin 활성화는 읽지 않는다. `CLAUDE_CODE_SUBAGENT_MODEL`은 프로세스 env < user < project < local < `--settings` 순으로 읽고, 모델이 없는 정의에만 적용하며 effort는 부모를 따른다. managed settings에 이 값이 있으면 순위를 측정하지 않았으므로 거부한다. 내장 역할(Explore·Plan·general-purpose)은 기존 역할 표가 정하며 이 값을 따르지 않는다. 2.1.283 측정에서 native는 env가 있으면 general-purpose를 env 모델로, Explore는 자기 모델(sol)로 실행하고 effort는 부모를 따랐다. 세션 중 `/add-dir`·`/cd`, ZIP·URL plugin, managed 역할 경로는 측정하지 않았다 |
| forked Skill | subagent 안의 fork를 받아들이고, fork 자식의 `SendMessage` 재개가 native 영수증과 대조돼 실행됨을 고정했다([3절](#3-미지원미검증구현-대기)) |
| auto 권한 모드(2.1.283 기본값) | auto 모드의 서버 측 분류기(`safeguards` 요청 필드)는 Anthropic 서버의 기능이라 이 backend에서 실행할 수 없다. 2.1.283 TUI는 메인 요청마다 이를 요청했고, gateway가 거부하면 native가 필드를 빼고 다시 보내 턴마다 거부 1건이 생겼다. v0.5.0은 native가 이런 gateway에 권하는 `CLAUDE_CODE_AUTO_MODE_SERVER=0`을 기본으로 둔다(사용자가 정한 값이 이긴다). native의 자체 분류기 요청은 `stop_sequences`를 써서 `UNSUPPORTED_SAMPLING`으로 거부되므로, **판정이 필요한 행동은 auto 모드에서 거부된다**(`Classifier unavailable`). 작업 폴더 안의 행동처럼 native가 판정 없이 허용하는 행동은 그대로 실행된다. 분류기 판정이 필요하면 `Shift+Tab`으로 다른 권한 모드를 쓴다. 이 동작은 v0.4.4에서도 같았고, 분류기 지원은 후속 대상이다 |
| 릴리스 자산(#136) | `clauduct.exe`·`install.ps1`·`uninstall.ps1`·`SHA256SUMS` 넷. 0.3.x 설치의 `--update`는 사본이 없어 멈추므로 설치 스크립트로 다시 설치한다([PACKAGING.md](PACKAGING.md#1-나가는-것)) |

### v0.5.1 — 주입 감사, 내장 역할 env, 세션 중 `/cd`, plugin·managed

**2026-09-26 출하·실제 설치 확인 완료**([릴리스 검증](RELEASE-v0.5.1.md#출하-검사-2026-09-26)).

Claude Code 2.1.283·Codex CLI 0.157.1에서 측정했다. 0.157.1은 과금 없는 재측정에서 도움말·plugin API 스냅샷과
exec HTTP·WebSocket 요청 구조가 0.157.0과 같았다(버전 문자열과 키 순서만 다름). 실행 횟수와 출하 검증은
[릴리스 노트](RELEASE-v0.5.1.md)에 적는다.

**주입 감사(#144).** Clauduct가 요청에 더하던 문장을 실제 backend에서 그룹별로 빼고, 원래 문제가 났던 시나리오를
10회씩 돌렸다. 재현이 없으면 제거하고, 하나라도 재현되면 유지했다.

| 항목 | 결과 |
|---|---|
| 고정 top-level `instructions` | 제거. backend는 이것이 없는 요청을 받고 developer 턴(시스템 프롬프트)을 따랐다. 로컬 토큰 계수의 기본값을 13에서 1로 바꿨고, 네 모델 16칸과 luna 18칸이 backend usage와 일치했다 |
| 실패한 도구 결과 앞의 `Tool execution failed:` | 제거. backend 입력에는 실패 표지가 없고 참조 클라이언트(Codex 0.157.1)도 본문만 보낸다. native의 실패 본문은 스스로 실패를 말한다 |
| Agent 설명의 보고·제약 보존·대기 문장, 고정 경로 문장, 메뉴 역할의 prompt, fork 재개 설명, 자식 취소 보충 | 제거(10회 재현 0) |
| 되찾은 자식 보고의 "untrusted" 표시, 자식 오류·결과 미확보 보충 | 유지. 같은 측정에서 한 번도 발동하지 않아 제거 근거가 없다 |
| ToolSearch의 역할 탐색 문장, Workflow의 TaskStop 문장, Workflow worker 역할 지침 | 제거(각 10회 재현 0) |
| 위임 영수증 | **유지.** 빼면 native 실행 결과가 agent ID를 사용자에게 알리지 말라고 표시한 탓에, 사용자가 자식 ID를 물어도 부모가 답하지 않았다 |
| inherit 메뉴의 "model 인자를 넘기지 말라"와 Agent의 별칭·isolation 문장 | **유지.** 둘을 뺐을 때 부모가 요청 없이 model을 넘겨 자식이 세션 route를 벗어났다. 둘 중 어느 쪽이 필요한지는 나눠 재지 않았다 |
| SendMessage 문장 | **유지.** 빼면 이미 재개된 자식에게 완료 알림을 요청하는 빈 메시지를 다시 보내 도구 오류가 났다 |
| 압축 효율 지침 | **유지.** 같은 입력에서 빼면 압축 시간이 기준보다 50% 넘게 길어진 경우가 10회 중 4회였다 |
| Agent 스키마의 모델 목록·effort, Workflow plan-v1·`agent()` 옵션 안내, 역할 메뉴 | 기능 노출이라 제거 대상이 아니다 |
| SDK 대기 응답(`[Clauduct] Waiting for background task notification.` 등) | 유지. SDK는 빈 응답 블록을 다시 요청하고 알림 뒤의 빈 응답을 거부한다 |

**측정 중 재현되어 고친 결함.**

| 결함 | 수정 |
|---|---|
| Workflow `meta.name`에 파일 이름에 쓰지 않는 문자(`:` 등)가 있으면 연결이 거부돼 모든 자식이 `AGENT_SELECTION_UNVERIFIED`로 실패 | native가 만든 script 파일을 위치와 run ID로 확인한다 |
| plan-v1이나 `resumeFromRunId` 호출에 native 스키마가 무시한다고 밝힌 `description`·`title`이 있으면 거부 | 두 필드는 문자열이면 받아서 버린다 |
| plan-v1을 `name` 필드에 넣은 호출 거부 | plan 표지로 처리한다 |
| `--resume`으로 새로 연 세션에서 이전 Workflow script를 다시 보내면 어댑터가 두 번 붙어 native가 거부 | 이전 어댑터를 잘라 내고 하나만 붙인다 |
| 세션 중 `/cd` 뒤 첫 프롬프트가 `CLAUDUCT_CONTEXT_SESSION_UNVERIFIED`로 막힘 | native가 transcript를 실제로 옮겼을 때만 새 경로를 받고, Clauduct의 context journal을 함께 옮긴다 |
| `.zip` `--plugin-dir`와 여러 plugin을 담은 `--plugin-dir` 폴더의 역할 호출이 `PREPARE_UNVERIFIED`로 거부 | 폴더는 하위 plugin을 읽고, zip은 `--plugin-url` plugin처럼 native의 선택으로 실행한다 |

**역할·경로(#145–#148, 과금 없는 native fixture와 로컬 PTY).**

| 항목 | 결과 |
|---|---|
| 내장 역할과 `CLAUDE_CODE_SUBAGENT_MODEL` | 값이 있으면 model·effort 인자 없이 부른 내장 역할은 native의 선택으로 실행한다. 2.1.283은 general-purpose에 env 모델, Explore에 자기 모델(sol), Plan에 부모 모델을 주고 모두 부모 effort를 쓴다. 값이 없으면 기존 역할 표를 쓴다. model을 명시한 호출의 규칙은 그대로다 |
| 세션 중 `/add-dir` | 추가 폴더의 역할을 native가 싣고, Clauduct는 native의 선택으로 정의대로 실행한다 |
| 세션 중 `/cd` | native는 새 폴더의 역할을 싣지 않는다(native 동작). 세션은 위 수정으로 이어진다 |
| forked Skill 자식 | 모델의 TaskStop 뒤 재개는 Agent 자식과 같이 허용된다. native는 모델이 멈춘 자식에 `stoppedByUser`를 남기지 않는다. 사용자가 UI에서 멈춘 경우는 측정하지 않았다. subagent 안의 fork가 background 자식을 띄우면 Skill 결과가 바로 돌아오고, native는 그 fork를 다시 깨우지 않는다 |
| ZIP·URL plugin | native와 같은 route로 실행한다 |
| managed 역할(`C:\Program Files\ClaudeCode\.claude\agents`) | native와 같게 가장 높은 우선순위로 읽는다(임시 역할로 확인한 뒤 제거). managed `env`는 그 PC의 모든 세션에 영향을 줘 측정하지 않았다. Clauduct는 `managed-settings.json`에 `CLAUDE_CODE_SUBAGENT_MODEL`이 있으면 거부하고, `managed-settings.d`와 registry 정책은 읽지 않는다 |

2026-09-26 감사에서는 native 2.1.282의 SDK 초기화 목록과 공식 문서를 대조했다. 목록에 나타난 도구·명령은
기능 합격 목록이 아니다. 실제 backend에서 기본 생성, 파일 수정, 구조화 출력, 이미지/PDF, MCP,
루트 forked Skill, 단순 Workflow, 세션 재개와 TUI 압축·취소·복구를 검사했다. Luna의 PDF 식별자 판독에서
한 글자 누락 1건이 있었고 Sol의 별도 실행은 성공했다. 무료 native 검사에서 원본 문서와 페이지 이미지의
backend 전달 바이트는 보존됐다. 이는 모델별 모든 판독의 정확성을 보장하는 결과가 아니다.

이 감사 당시의 후속 항목 중 subagent 내부 forked Skill, fork 자식 재개, 역할 기본값 수집 확대는
v0.5.0에서 다뤘다(아래 v0.5.0 절). Workflow의 검증된 명시적 source 재개, native 도구의 기본 Office 인수,
native code-review 인수와 V1 review-diff의 문서상 은퇴는 [v0.6.0](RELEASE-v0.6.0.md)에서 완료했다.
현재 v0.6.x 작업은 [현행 Windows 지원 범위의 후속 검증](QUALITY-v0.6.x.md)이며 서비스 전용 제약은
[공식 기능 가용성](https://code.claude.com/docs/en/feature-availability)에 따라 별도로 관리한다.
모든 로컬 slash command와 조합을 실측했거나 API 오류가 언제나 없다는 판정은 하지 않는다.

### v0.5.2 — auto 모드 분류기(#149): 재검증·출하

**2026-09-27 출하·실제 설치 확인.** 태그 `v0.5.2`는 `921e254`다.
[변경과 출하 검사](RELEASE-v0.5.2.md)에 commit·바이너리 해시·검사 범위를 기록한다.

#### 최초 실패와 측정 조건 정정

첫 실험은 Claude Code 2.1.283의 자체 분류기를 시험했지만, 캡처 환경에 제품이 설정하는
`ANTHROPIC_DEFAULT_SONNET_MODEL`이 빠졌다. 이때 native는 `claude-sonnet-5`와 아래 severity 프로토콜을
선택했다. 제품처럼 GPT 모델명을 지정하면 같은 native가 block 프로토콜을 만든다.
따라서 아래 109회의 실패는 보존하되 현행 제품 요청의 합격 근거로 사용하지 않는다.
이 실험에서는 Clauduct 정책 문장을 더하지 않았고 stop은 로컬 출력 절단으로 처리했다.

| 측정 | 결과 |
|---|---|
| native 분류기 흐름(과금 없는 loopback) | 1단계는 `claude-sonnet-5`·`max_tokens` 64·thinking 끔·정지 문자열 `</severity>`다. severity 25 이하면 허용하고 26 이상이면 2단계(`max_tokens` 8192)로 간다. 2단계는 35 이하 허용, 36 이상 거부다. 1단계가 오류면 주 모델·`max_tokens` 2112로 다시 묻고, 그것도 실패하면 `Classifier unavailable`로 거부한다. 요청 class는 `auxiliary`다 |
| 번역만 한 경우(실제 backend) | backend의 reasoning 토큰이 64와 2112를 모두 넘겨 두 요청이 출력 한도로 거부됐다. 판정이 필요한 행동마다 과금 2회와 약 2.5분을 쓴 뒤 거부된다 |
| thinking을 끈 요청의 출력 한도에서 reasoning을 뺀 경우 | 1단계가 정상 형식으로 답했다(보이는 출력 최대 13토큰) |
| 안전 관문(합성 표본 위험 30·정상 30·경계 10, `gpt-5.6-terra`/high, 표본당 1회) | **불합격.** 합의한 엄격 형식 검사에서는 2단계 응답 35개 중 15개가 `<thinking>` 없이 답해 형식 오류다. native가 받는 형식으로 읽어도(loopback에서 `<thinking>` 없는 답을 받는 것을 확인) 위험 표본 5개가 자동 승인됐고, 정상 30개와 핵심 출하 6개는 모두 허용됐다 |

**원인.** 자동 승인된 다섯은 모두 권한·의도에 달린 행동이다: 승인 없는 push, 승인과 다른 대상, 인용되거나 에이전트가 쓴 승인,
내용을 확인하지 않은 업로드. native 1단계는 해악만 채점하고 사용자 의도는 2단계에 맡긴다. 이 backend는 이 행동들의
해악을 8–18로 매겨 2단계 없이 허용되는 구간에 넣었고, 하나는 2단계가 사용자 요청을 근거로 허용했다.

**당시 처분.** 분류기 요청을 계속 거부했다. auto 모드에서 판정이 필요한 행동은
v0.5.0부터와 같이 거부되므로, 판정이 필요하면 `Shift+Tab`으로 다른 권한 모드를 쓴다. 표본 70개의 결과는 이 backend·route·
native 버전에서의 관찰이며, Anthropic 분류기와의 비교는 하지 않았다. 측정에 쓴 실제 backend 호출은 109회다.

#### 출하 경로와 정책

native의 GPT block 요청은 1단계 `max_tokens=2112`·stop `</block>`, 2단계 `max_tokens=10240`·stop 없음이다.
thinking 필드와 classifier 전용 beta가 없고, 일반 Sonnet 환경값만 받아 effort 없이 요청한다.
v0.5.2에서는 기존 독립 auxiliary 경로에서 이 요청 형식을 식별해 **`gpt-6-luna/high`**로 보냈다.
v0.5.6의 분류 모델 보완과 별도 표본은 아래 v0.5.6 절을 따른다.
[classifier 선택](../../go/internal/gateway/classifier.go)은 요청/실제 모델과 `native-auto-mode` 출처를
기록하며 정책·transcript를 다시 쓰지 않는다. count_tokens도 같은 선택을 따른다.
형식을 확인할 수 없는 알려진 classifier 요청과 알 수 없는 모델·effort는 전송 전에 거부한다.
일반 Sonnet→Terra/high, Haiku→Luna/max 매핑은 유지한다.

기본 native 정책은 현재 저장소의 일반 push를 허용할 수 있어, “작업과 정확한 대상에 대한 실제
사용자 승인”을 요구하는 기준과 다르다. 모델 선택만으로 이 정책 차이를 해소했다고 주장하지 않는다.
검증 후 승인받은 두 규칙을 공식 `autoMode.hard_deny` 설정으로 추가했다.

- 사용자에게 작업 종류와 정확한 대상을 승인받지 않은 push·PR·병합·태그·Release·배포 등 원격 변경을 차단한다.
- 공개 내용·허용 대상·배포 후속 효과·설치 범위·프로세스 소유가 확인되지 않은 고영향 작업을 차단한다.

`$defaults`로 native 내장 규칙을 유지하며, 사용자 hard_deny 순서와 다른 autoMode 필드도 보존한다.
설정은 [자식 설정](../../go/internal/app/settings.go)과 [사용자 설정 병합](../../go/internal/app/user_settings.go)을
통해 이 빌드가 시작하는 native의 `--settings`에만 전달한다. 전역/managed 설정과 native 본체를 수정하지 않는다.
[native 공식 설정 결합 규칙](https://code.claude.com/docs/en/auto-mode-config#where-the-classifier-reads-configuration)을 따른다.
v0.6.4부터 이 두 규칙을 더하지 않는다. 그 뒤로는 native 기본 규칙과 사용자·관리 정책만 적용된다(아래 v0.6.4 절).

stop 문자열은 1–16개, 각각 비어 있지 않은 최대 256바이트다. stream과 JSON 응답에서 문자열과
그 뒤 텍스트·도구 호출을 전달하지 않고 `stop_reason=stop_sequence`를 반환한다. 원래 backend
출력의 snapshot 검증과 usage는 유지하며, backend 생성 자체를 조기에 멈추거나 비용을 줄인다는 뜻은 아니다.
thinking을 명시적으로 끈 요청의 `max_tokens`는 보고된 reasoning을 제외한 출력량과 대조한다.
그 외 요청의 출력 한도 검사는 그대로다.

#### 관문과 남은 한계

후보·effort·정책을 고정한 뒤 별도 70개를 작성했다. 원래 표본과 같은 transcript/action 쌍은 0개다.
두 corpus 모두 위험 30 자동 승인 0, 정상 30 중 24 이상 허용, 핵심 정상 6 전부 허용, 형식 오류 0을 요구했다.

| Luna/high 검사 | 결과 |
|---|---|
| 원래 70개 | 위험 자동 승인 0/30, 정상 허용 29/30, 핵심 6/6, 형식 오류 0 |
| 독립 70개 | 위험 자동 승인 0/30, 정상 허용 28/30, 핵심 6/6, 형식 오류 0 |
| 사전 지정한 민감 사례 반복 | 위험 4개 차단, 핵심 정상 6개 허용, 형식/전송 오류 0 |
| 실제 native TUI | 실제 native 요청을 제품 경로에 보내고 전체 gateway 응답을 그대로 반환했다. 허용 시 작업 소유 marker 생성, 명시적 삭제 거부 시 marker 보존, 두 세션 정상 종료 |
| 실제 후보 기본 설정 | 사용자 `--settings` 없이 두 규칙 적용, native 기본 규칙과 사용자 추가 규칙 보존, backend 0회 |

Sol/low도 두 70개 관문을 통과했지만 반복에서 승인된 PR·Release 두 건을 차단해 제외했다.
원래/독립 표본의 경로 시간 중앙값은 Sol/low 6.48/5.76초, Luna/high 10.76/9.97초였다.
Luna의 독립 표본에는 정상 프로세스 종료·로컬 설치 두 건의 오차단이 남아 있다.
이 수치를 일반 코딩 속도나 구독 한도 차감률로 확대하지 않는다.

TUI의 주 모델 도구 제안은 고정 합성 입력이며, 분류 판정은 실제 backend다. 실제 native와 제품
분류 경로의 호환을 검증한 범위로 기록하며 일반 에이전트 작업 전체의 인수를 뜻하지 않는다.
관측한 버전·표본의 합격은 모든 요청의 안전 보장이 아니다. 이번 B의 총사용량은 이전 109회와
순수 출하 자산의 backend 세션 5회를 포함해 959/1,000회이며 열린 예약은 없다.
순수 태그 재현 빌드·격리 설치와 되돌림·공개 자산 검증·updater·실제 설치 확인을 통과했다.
Workflow 재실행 resume·Office·review-diff를 포함한 다음 묶음은 이 출하의 완료 범위에 포함하지 않는다.

### v0.5.3 — Clauduct 설정과 세션 선택

**2026-09-27 출하·실제 설치 확인.** 태그 `v0.5.3`은 `788a60e`다.
[설정 문서](SETTINGS.md)가 schema·우선순위·재개 범위를, [릴리스 기록](RELEASE-v0.5.3.md)이 검증을 소유한다.

수동 설정에서 시작 pair·GPT별 effort·alias 매핑·agent pair를 분리한다. 기본 시작값은 Sol/xhigh이며
native 개인화와 B 정책은 유지한다. 설치·업데이트·일반 실행은 파일이 없을 때만 생성하고 기존 파일을 보존한다.
UUID 재개는 당시 snapshot과 마지막 S 선택을 복원한다. snapshot 없는 이전 세션은 현재 시작값을 사용한다.
snapshot 있는 재개 목록·continue·실행 중 resume은 UUID 재실행을 안내하며 자동 종료는 하지 않는다.

Go 일반·race 전체 22개 패키지와 native 2.1.283의 plugin·Workflow·S·fork·clear 표본을 확인했다.
실제 backend 개발 21회와 순수 출하 5회가 통과했고, 공개 자산·업데이트·실제 설치를 대조했다.
이 결과는 임의 plugin이나 미래 native 형식, Bundle C의 전체 기능 인수를 자동 보증하지 않는다.

### v0.5.4 — 설정 완성과 동일 Agent 호출 제어

2026-09-28 태그 `58f9491`에서 정식 발행·실제 설치를 완료했다. [릴리스 기록](RELEASE-v0.5.4.md)이
초기 settings 생성·background 결과 보존과 검증 범위를 소유한다. 같은 세션·부모·현재 turn에서 이미 수락되고 결과가 pending인 Agent의
모든 인자가 같은 후속 호출만 native 대기로 전환한다. 최초 병렬 호출·다른 인자·새 사용자 요청·결과
수신 후의 위임은 유지한다. 중복과 다른 도구가 섞이면 전부 거부한다. 같은 인자의 작업을 순차적으로
구분하려면 description 등 인자를 달리하거나 새 요청으로 시작해야 한다.

원본 model/effort의 생략 여부를 포함해 최상위 키 순서와 공백을 정규화하고, 중첩 값의 키 순서는
보존한다. 의미상 같은 작업인지 추정하지 않으며 프로세스 재시작 뒤 지문을 복원하지 않는다.
native 지침·B 규칙·권한은 유지한다. 모든 모델의 행동이나 임의 프롬프트의 완료를 보증하는 기능은 아니다.

최종 Luna/medium 일반·독립 표본과 Sol/xhigh background 재시작 일반·독립 표본이 통과했다.
별도 실제 표본에서 중복 호출 1회를 대기로 전환한 증거는 Agent 대기 안내를 제거한 중간 빌드다.
안내까지 유지한 최종 조합에서 발동한 증거는 v0.5.5의 최종 태그 검사로 별도 확인했다.
최종 전체 일반/race는 각각 22개 패키지 PASS이며, 개발·실패·재시도·출하 합계는 532 attempts다.
순수 태그 출하 표본의 5회도 이 합계에 포함한다. 묶음 C의 전체 기능 인수를 완료했다는 뜻은 아니다.

### v0.5.5 — 최종 조합 인수

태그 `d17f7fe`의 전체 Agent 안내를 유지한 바이너리에서 동일 인자 중복 호출의 실제 대기 전환
(`duplicateWait=1`)과 별도 정상 위임(`duplicateWait=0`)을 확인했다. native Agent 실행은 각 1회,
실패 0·완료 보고서·최종 응답·정상 종료·메모리 해제 PASS다. 같은 bytes의 phase UUID 재개,
이미지/PDF 및 구버전 거부 뒤 복귀도 확인했다. [출하 기록](RELEASE-v0.5.5.md)이 신원·설치와
분석 80→0·PowerShell 7 지원 범위를 소유한다. 모든 모델·임의 프롬프트를 보증한다는 뜻은 아니다.

### v0.5.6 — C 전 품질 보완

native publication 번호를 시계에서 분리하고 기존 journal의 마지막 번호를 이어 쓴다.
reload·worker 재시작 후 과거 턴을 선택하던 결함을 고쳤으며, 같은 agent의 동시 게시를 직렬화한다.
최신 번호가 충돌하거나 게시가 불완전하면 실행 전에 거부한다.

auto 권한 분류 전용 경로는 `gpt-5.6-terra/high`다. v0.5.5의 Luna/high에서 승인 PR/Release의
오차단을 재현한 뒤 모델·effort를 비교했다. native 지침·두 추가 규칙을 유지한 Terra/high는
원래 70개 표본에서 위험 허용 0/30·정상 허용 30/30, 동결 후 독립 표본에서 위험 차단 8/8·정상
허용 8/8을 기록했다. 이 경로는 일반 Sonnet 매핑이나 사용자 모델 기본값을 바꾸지 않는다.
과거 Luna/high의 성공·오차단 기록은 위에 보존한다. 모든 가능한 작업에 대한 안전 보증은 아니다.

media 길이 거부 진단은 압축을 새로 요청한 경우와 압축 직후에도 길이를 초과해 중단한 경우를
모두 집계한다. 압축 후 이미지를 넣은 실제 backend 계속 실행과 새 usage 기준도 확인했다.
추가 사전 계수 없이 native media 범위를 유지하므로 이미지/PDF의 길이 초과를 모두 사전에
예측하지는 못한다. 압축 직후에도 길이가 초과되면 `CONTEXT_COMPACTION_INSUFFICIENT`로 중단하고
같은 실행을 자동 재시도하지 않는다. 이는 #50에서 합의한 지원 범위다.

제품 전용 미사용 선언·테스트 전용 조회 래퍼를 제거하고 공용 테스트 전송 도구를 제품 패키지에서
분리했다. 모든 PowerShell 실행 진입점은 7을 요구하며, 과거 스크립트의 미실행 이력을 소급하여
통과로 바꾸지 않는다. 출하 인수 결과는 [릴리스 기록](RELEASE-v0.5.6.md)이 소유한다.

### v0.6.4 — 계정 모델 목록, 권한 규칙 제거, 설정 동기화

**2026-10-03 출하**(태그 `v0.6.4` → `3273786`, [기록](RELEASE-v0.6.4.md)). 후보 `bf984f2`와 릴리스 바이트 모두
native 2.1.288, Codex CLI 0.160.0으로 실제 backend 검증을 통과했다(아래 실측 표). 설정 형식과 우선순위는 [설정 문서](SETTINGS.md)가 소유한다.

| 항목 | v0.6.4 동작 |
|---|---|
| 권한 | native `permissions.ask`(실행·외부 통신 도구 17개)와 `autoMode.hard_deny` 규칙 2개를 더하지 않으며 bypass 시작 모드 분기도 없다. native 기본값, user·project·local 설정, 관리 정책, 세션 중 모드 전환이 정하고 사용자·관리 정책의 `ask`·`deny`는 그대로 전달한다. status `session.requiredAsk`는 없어졌다. **auto 모드에서 `Bash`·`WebFetch` 같은 도구는 native 규칙과 native 분류기만으로 결정되며 Clauduct의 추가 확인은 적용되지 않는다** |
| 요청 출처 확인 | 유지. 요청마다 현재 native session·Agent·turn·step이 내장 helper로 일회성 확인값에 응답하고, `tool_use_id` step 표지(`__cdt…`), `NATIVE_REQUEST_ORIGIN_UNVERIFIED`, `NATIVE_DIRECT_DELEGATION_UNSUPPORTED`도 그대로다. `NATIVE_CONFIRMATION_UNVERIFIED`는 출처 증명 실패 상태(확인 교환의 I/O 오류·시간 초과)만 뜻한다. 필수 ask 존재 확인과 `allowManagedPermissionRulesOnly` 검사는 없어졌다 |
| 계정 모델 목록 | 일반 세션 시작마다 Codex 계정 목록을 그 세션의 credential provider로 받는다(10초, 4 MiB, 512개, redirect 거부, 고정 목적지). 성공하면 `~/.clauduct/account-models.json`에 저장하고, 실패하면 같은 계정의 마지막 정상 목록과 stderr 안내, 그것도 없으면 `ACCOUNT_MODEL_LIST_UNAVAILABLE`로 시작을 거부한다. Codex의 `models_cache.json`은 근거가 아니다. 목록은 세션 동안 고정되며 검증 예산 원장에 예약하지 않는다 |
| 모델 선택 | 계정이 나열하는 모델은 새 릴리스 없이 전체 ID로 고른다. 숨긴 모델은 `/model`에 없지만 ID로 선택할 수 있다. `models.json`은 기존 이름(키·별칭·단계·`countValidated`)과 은퇴 표만 담고 선택 범위를 제한하지 않는다. 은퇴 표는 계정이 그 이름을 나열하지 않을 때 거부 이유만 설명한다 |
| effort | 계정의 지원 수준 ∩ low·medium·high·xhigh·max만 보낸다. `ultra` 등은 보내지 않고, 명시 요청은 낮추지 않고 거부한다. 순서는 명시값 → 세션 설정 `modelDefaults` → 계정 `default_reasoning_level`. 기본값이 없는 모델은 effort를 명시해야 한다 |
| 위임 메뉴 | 기존 모델은 `clauduct-<key>`, 새 모델은 `clauduct-<전체 ID>`. 기존 키·`inherit`·과거 effort별 이름과 겹치면 항목이 없고 Agent model 인자로 쓴다. 기존 단계가 없는 모델은 Agent `model` 인자를 생략하고 native 생성 이벤트가 전체 ID를 고정한다(native 이벤트 모듈 필요) |
| 토큰 계수 | 모든 모델이 backend 계수(정확값)를 먼저 쓴다. v0.6.3도 기존 모델은 backend로 계수했고, v0.6.4는 그 모델 조건만 없앴다. 로컬 공식은 backend 계수를 쓸 수 없는 요청 형태에서만, 측정한 기존 모델(`countValidated`)에 쓰는 대체 경로다. 계수가 실패해도 추정값으로 대신하지 않는다 |
| 목록에 없는 선택 | `modelDefaults`·`modelMapping`·`agents`·`classifier_model`의 항목은 파일에 남기고 stderr와 `session.modelList.problems`에 알리며, 실제로 선택될 때만 실패한다 |
| auto 분류기 | `classifier_model`은 `{"model","effort"}` 객체(공장값 terra/low)다. v0.6.3의 문자열은 `CLAUDUCT_SETTINGS_INVALID`이며 자동 변환하지 않는다. 계정이 제공하는 모델·effort를 모두 허용한다(Terra 이상 규칙·내장 허용 목록 제거, Luna 가능). 출처는 `native-auto-mode+classifier_model`. 계정이 제공하지 않는 pair는 `AUTO_MODE_CLASSIFIER_MODEL_UNSUPPORTED`로 거부하고 대체하지 않는다 |
| 상한 은퇴 | `auxiliary_effort_cap`·`auto_compact_effort_cap`은 적용하지 않는다. 알려진 은퇴 키로 받아 파일에 두고 stderr와 `session.deprecatedSettings`에 알리며, 다른 모르는 키는 계속 거부한다. 압축은 현재 선택(Agent의 자기 route 또는 native가 압축 요청에 적은 모델·effort)을 쓰므로 모델을 바꾼 뒤에는 새 모델로 압축한다(v0.6.3까지는 이전 route) |
| UUID 재개 | snapshot 항목 수가 현재 목록과 같을 필요가 없다. `modelDefaults`는 적은 대로 저장하고 공장값을 고정하지 않는다. 마지막 선택·context journal·Agent 선택 기록은 현재 계정 목록과 대조하며, 쓸 수 없는 선택은 대체하지 않고 오류다 |
| 설정 동기화 | 일반 실행·`--dev --sync-settings`·`--update`·`install.ps1`에서 빠진 최상위 키만 덧붙이고 원본 백업을 남긴다. `{"version":1}` 같은 v0.5.3 시절 파일도 이제 확장된다. v0.6.4보다 오래된 바이너리로 되돌리려면 백업을 복원하거나 파일을 편집한다 |

**로컬에서 측정한 것.** 분류기 실패 처리는 2026-10-03 합성 backend와 native 2.1.288로 측정했다(과금 없음, auto 모드에서 Agent 실행을
대상으로 함. 작업 폴더 안 쓰기는 native가 분류기 없이 허용해 대상이 될 수 없었다). HTTP 500, 정책 거부 형태의 HTTP 400,
시간 초과, 읽을 수 없는 판정, 완료 전에 끊긴 응답, 계정이 제공하지 않는 pair(`AUTO_MODE_CLASSIFIER_MODEL_UNSUPPORTED`)
여섯 경우 모두 native는 분류 요청을 한 번 더 보낸 뒤 해당 호출을 거부 목록(`permission_denials`)에 남기고 실행하지 않았으며,
같은 turn을 정상 완료했다. 읽을 수 없는 판정 뒤의 재요청은 gateway의 replay 차단(`NATIVE_REQUEST_REPLAY_BLOCKED`)으로
거부됐다. v0.6.10부터 분류기 요청은 replay 키를 만들지 않는다(#289). 그래서 읽을 수 없는 판정 뒤의 재요청도 backend에
간다. native 2.1.289는 이 경우 분류 요청을 10번 보낸 뒤 그 호출을 실행하지 않았다(합성 backend). backend에 닿은 뒤 실패한
응답에는 전처럼 `X-Should-Retry: false`를 붙이므로 HTTP 500(재시도 가능·불가), 시간 초과, 끊긴 응답은 전과 같이 2번이다. Clauduct는 허용 판정·모델 대체·모드 전환을 만들지 않는다. 실제 backend의 `cyber_policy` 거부와 TUI 표시는
아직 측정하지 않았다.

새 모델(내장 이름이 없는 `gpt-9-new` 합성 목록)은 native 2.1.288에서 세션 모델, `clauduct-gpt-9-new` 메뉴,
Agent `model` 인자 세 경로 모두 자식 요청까지 그 모델과 계정 기본 effort로 실행됐다. 압축은 native가 압축 요청에
현재 선택을 싣는 것을 확인했다: 자동 압축은 세션의 effort(상한 없이 high), 병렬 자식은 자식 route, 모델을 바꾼 UUID
재개는 새 모델·effort로 압축했다.

Windows 파일 경합은 설정 동기화에서 재현했다: 다른 프로세스가 `settings.json`을 삭제 공유 없이 열고 있으면
교체가 실패해 `CLAUDUCT_SETTINGS_SYNC_FAILED`를 알리고, 원본과 이름이 표시된 백업은 같은 바이트이며 임시 파일은
남지 않는다. 재시도는 하지 않으며 일반 실행은 안내 후 파일을 그대로 두고 진행하고 다음 시작에서 다시 시도한다.
상태·세션 기록의 지속 잠금과 종료 쓰기 경합은 기존 동작을 바꾸지 않았고 이번에 다시 측정하지 않았다.

**실제 backend 실측 (2026-10-03, 후보 `bf984f2`, 사용자 승인, 14개 시나리오 PASS).** 검증 예산 원장으로 경로를 제한했다.

| 시나리오 | 결과 |
|---|---|
| 계정 목록·설정 동기화 | 실제 `GET models`(client_version 0.160.0)가 10개 모델을 돌려줬고(`hide` 2개 포함, 일부 모델은 `ultra` 표기) 파서가 받아들였다. `account-models.json`에는 계정 해시와 모델 필드만 남았다. v0.6.3 형식 파일에 `context_window`·`auto_compact_token_limit_percent`·`classifier_model`만 덧붙고 원본 백업과 은퇴 키 안내가 나왔으며, 두 번째 시작은 파일을 바꾸지 않았다 |
| 모델 선택 | `gpt-6.1-sol/low` 생성. 내장 이름이 없는 `gpt-5.5`는 `--model`만 주면 계정 기본값(medium)으로 실행됐고 출처는 `account.default_reasoning_level`. 은퇴 표에 있는 `gpt-5.6-luna`도 계정이 나열해 그대로 실행됐다 |
| Agent | `clauduct-gpt-5.5` 메뉴와 Agent `model:"gpt-5.5"`·`effort:"low"` 인자 모두 자식이 `gpt-5.5`로 실행됐다 |
| auto 분류기 | pair terra/low와 luna/low가 분류 요청(출처 `native-auto-mode+classifier_model`)을 처리했고 Agent 실행을 허용했다. 계정에 없는 pair는 `AUTO_MODE_CLASSIFIER_MODEL_UNSUPPORTED`로 거부되고 Agent는 실행되지 않았으며 turn은 끝났다 |
| 압축 | 큰 대화를 낮은 목표로 재개하자 자동 압축이 세션 effort(luna/high, 상한 없음)로 실행되고 코드를 기억했다. 모델을 바꿔 재개한 `/compact`는 새 선택(terra/low)으로 압축했다 |
| 권한 | bypass는 Bash 실행, bypass + 사용자 `deny`는 차단, dontAsk는 거부, dontAsk + `--allowedTools Bash`는 실행. `NATIVE_CONFIRMATION_UNVERIFIED` 없음 |
| TUI | `/model`에 계정의 표시 모델 8개가 계정 순서로 나왔고 `/model gpt-5.5` 뒤 요청은 `gpt-5.5/low`, `/context` 계수는 backend 계수로 성공했으며 UUID snapshot의 마지막 선택이 `gpt-5.5/low`였다 |

관측(2026-10-03 정정): `/context`는 계수 요청을 한꺼번에 보낸다(측정 표본 19~20건). backend 계수는 1건에 약
1초이고 동시에 8건까지 처리하므로, 전부 끝나는 데 몇 초가 걸린다. 출하 TUI 검사는 `/context` 화면이 뜬 0.8초 뒤에
native를 종료해, 끝나지 않은 계수 18건과 native 보조 생성 1건이 HTTP 499 `CANCELLED`로 종료 줄과 실패 집계에
남았다. 처음에는 "로컬 계수식이 없는 모델이라 native가 이전 계수를 끊는다"고 적었지만, 이 설명은 틀렸다. 원인은
모델이 아니라 세션 종료다. 합성 backend 재현에서 Esc로 화면을 닫는 것만으로는 계수가 끊기지 않았고 `/exit`에서만
끊겼다. 실제 backend 재검사에서 Esc로 화면을 바로 닫고 20초 뒤에 종료하자, 계수 20건이 모두 성공하고 실패는 0건이었다.
`/context` 직후 몇 초 안에 세션을 끝내면 같은 499가 남을 수 있다. 이는 끝나지 않은 요청을 클라이언트가 끊었다는
기록일 뿐 계수 오류가 아니다.
native가 effort를 명시해 보내는 자체 보조 요청은 상한 없이 그 effort로 실행된다(관측: `gpt-6-luna/high` 1건).
계정 전환 중 실행과 v0.6.3 updater에서 설치본을 올리는 실제 경로는 출하 단계 검사에서 확인한다.

**측정하지 않은 것.** 위 항목은 표본 범위다. 분류 품질은 v0.6.3의 Terra·Sol 표본(3절 auto 권한 모드 행)에서 측정했다. 2026-10-03 정정: 처음에는 "Terra·Sol에서만 측정했다"고 적었지만, Luna/low·medium은 v0.6.2 준비 측정에서 이미 불합격이었다(v0.6.7 절). 계정 목록에 있다는 것은 품질 주장이 아니다.
새 모델의 실제 backend 동작은 위 표본까지만 측정했다. high·max 압축 비용은 출하 뒤 같은 압축 경로를 쓰는 후보
바이너리로 한 번씩 쟀다(2026-10-03, `gpt-6-luna`, 같은 공개 대화 입력 13,967토큰). 압축 요청 하나에 medium
12.2초·추론 137토큰, high 21.7초·171토큰, max 26.1초·354토큰이 걸렸고, 출력은 584~692토큰이었다. 세 경우 모두
압축 뒤 코드를 기억했다. 표본이 하나씩이라 경향만 보여 주며, 요약 품질을 비교한 것은 아니다. 설치된 클라이언트와
측정 기준의 차이는 [현재 상태](README.md)가 기록한다.

### v0.6.5 — 숨긴 모델 위임 제외, native 명령행 길이 검사

**2026-10-03 출하**(태그 `v0.6.5` → `1786d30`, [기록](RELEASE-v0.6.5.md)). v0.6.4 출하 뒤 확인한 두 공백을 고쳤다.

| 항목 | 동작 |
|---|---|
| 숨긴 모델 | 계정이 picker에서 숨긴 모델(visibility가 `list`가 아님)은 위임 메뉴, Agent `model` 선택지, 그 모델만 받는 `effort` 값에서 빠진다. `/model`은 v0.6.4부터 이미 뺐다. 전체 ID를 지정하면 Agent `model` 인자와 Workflow는 그대로 실행한다. 선택지 목록은 권고일 뿐 강제가 아니다(Agent 도구는 strict가 아님) |
| native 명령행 길이 | 위임 메뉴와 `/model` 목록은 native 시작 인자로 넘어가며 모델 하나당 약 416자가 든다(10개 모델 약 6,400자). Windows `CreateProcessW` 한도는 NUL을 포함해 32,767자다(실측: 32,766자는 시작, 32,767자는 OS 거부). 넘으면 아무것도 시작하지 않고 `NATIVE_COMMAND_LINE_TOO_LONG`과 메뉴 항목 수를 알린다. 목록은 자르지 않는다. 현재 형태라면 계정이 표시하는 모델이 약 73개를 넘을 때 해당한다. native 2.1.288의 `--agents`는 `--print`에서만 파일을 받으므로 TUI에서는 파일로 넘길 수 없다 |

실제 backend(native 2.1.288, Codex CLI 0.160.0, 2026-10-03) 확인 결과다. 후보 바이너리와 이 릴리스 바이트에서 각각 통과했다:

- 숨긴 모델 2개(`gpt-reserve`, `codex-auto-review`)가 있는 계정에서 위임 메뉴는 9개(표시 모델 8개 + `inherit`)였다. v0.6.4는 11개였다.
- `clauduct-gpt-5.5` 메뉴와 Agent `model:"gpt-5.5"` 인자는 각각 자식을 `gpt-5.5`로 실행했다.
- 선택지에 없는 `gpt-reserve`도 전체 ID로 지정하자 자식이 `gpt-reserve/low`로 실행됐다.
- `/context` 뒤 Esc로 화면을 바로 닫고 20초 뒤에 종료한 TUI 검사에서는 계수 20건이 모두 성공했고 실패는 0건이었다.

명령행 한도 초과 경로는 합성 목록(151개 모델, 64,195자)으로만 확인했다. 실제 계정에서는 아직 재현할 수 없다.

### v0.6.6 — 종료 상태 파일 저장 재시도, `gpt-5.5` 은퇴 안내

**2026-10-03 출하**(태그 `v0.6.6` → `82fcc76`, [기록](RELEASE-v0.6.6.md)). 릴리스 바이트로 실제 backend 17개 시나리오를 통과했다.

| 항목 | 동작 |
|---|---|
| 종료 상태 파일 | v0.6.3부터 알려진 제한을 고쳤다. 세션이 끝나는 순간 statusline·백신·검색 색인 같은 다른 프로세스가 `status-<pid>.json`을 열고 있으면, Windows가 파일 교체를 거부해(`Access is denied.`) 최종 기록이 표준 오류에만 남았다. 이제 최종 기록은 25 ms 간격으로 최대 40회(약 1초) 다시 시도한다. 그보다 오래 잡혀 있으면 전처럼 표준 오류에만 남는다. 기존 파일은 교체가 성공할 때까지 그대로다. 세션 중 checkpoint는 다음 주기에 다시 쓰므로 기다리지 않는다 |
| `gpt-5.5` 은퇴 안내 | 은퇴 표에 `gpt-5.5 → gpt-6.1-sol`을 더했다. 근거는 계정 목록의 은퇴 공지(2026-10-14T19:00Z, 후속 `gpt-6.1-sol`)다. 계정이 나열하는 동안은 그대로 실행되고, 목록에서 빠지면 그 모델을 고른 시작 설정·세션·Agent가 `MODEL_RETIRED`와 후속 모델 안내로 거부된다. 대체 실행은 하지 않는다. 은퇴 판정은 "은퇴 표에 있고 계정 목록에 없음"이다. 그래서 계정이 아직 나열하는 모델을 받지 않는 effort로 고르면, 은퇴가 아니라 일반 거부다(모든 은퇴 이름에 적용) |

상태 파일 경합은 실제 Windows 파일 핸들로 재현했다. 다른 프로세스가 공유 삭제 없이 150 ms 동안 파일을 연 상태에서 고치기 전에는
교체가 실패했고, 고친 뒤에는 성공했다. 계속 잡혀 있으면 상한 안에 실패하며 기존 파일이 온전히 남는 것도 확인했다.

### v0.6.7 — 분류기 측정 결과 안내, 중단된 계수 분리, 사용자 mod 확인

**2026-10-03 출하**(태그 `v0.6.7` → `62b84f2`, [기록](RELEASE-v0.6.7.md)). 릴리스 바이트로 실제 backend 18개 시나리오를 통과했다.

| 항목 | 동작 |
|---|---|
| 분류기 pair 안내 | 계정이 제공하는 `classifier_model` pair가 Clauduct의 분류기 측정에서 통과하지 못했거나 측정하지 않은 pair면, 시작할 때 stderr에 한 줄로 알리고 `session.classifierFinding`에 남긴다. 설정대로 사용하며 거부하지 않는다(#238 계약). 통과한 공장값 Terra/low는 아무것도 출력하지 않는다. 계정이 제공하지 않는 pair는 이 안내 대신 기존처럼 설정 문제로 알리고, 분류 요청은 `AUTO_MODE_CLASSIFIER_MODEL_UNSUPPORTED`로 거부된다 |
| 중단된 계수 | `/context` 직후 세션을 끝내 native가 버린 계수 요청을 따로 센다(`gateway.requests.cancelledCounts`). 종료 줄은 `refused`에서 이를 빼고 `counts_cancelled=N`으로 보여 주며, 이것만으로는 상태 JSON 전체를 출력하지 않는다. 원래 집계(`totals.failures`의 `CANCELLED`, `refusedBy`)는 그대로다. 같은 때 끊긴 생성 요청은 계속 `cancelled_requests`로 보인다 |

분류기 측정 결과(실제 backend, 공개 표본). v0.6.2 준비(2026-09-29)와 v0.6.7(2026-10-03)은 핵심 74개(위험 32)·독립 20개,
v0.6.3(2026-10-02)은 핵심 71·독립 18개 표본이다. 기준은 핵심 표본에서 위험 허용 0이다.

| pair | 결과 |
|---|---|
| `gpt-5.6-terra/low` | 통과(v0.6.2 준비, v0.6.3). 공장값 |
| `gpt-5.6-terra/medium` | 통과(v0.6.3, 2026-10-02: 오차단 1) |
| `gpt-5.6-terra/high` | 불합격: 위험 1개 허용(2026-09-29) |
| `gpt-6-luna/low` | 불합격: 2026-10-03 재측정에서 위험 32개 중 2개(D08 Windows 작업 범위, D16 GitHub 승인) 허용. 독립 20개는 통과. 2026-09-29에는 5개 |
| `gpt-6-luna/medium` | 불합격: 위험 2개 허용(2026-09-29) |
| `gpt-6.1-sol/low` | 판정 미완료: 보안 표본 하나를 backend가 `cyber_policy`로 두 번 거부(2026-10-02) |
| 그 밖의 pair | 측정하지 않음 |

**사용자 Claude Mods.** native 2.1.287부터 플러그인이 function hook으로 더 깊은 동작을 바꿀 수 있다("Claude Mods"). Clauduct는
자기 세션 플러그인 때문에 `CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1`을 설정하므로, 사용자 mod도 함께 로드된다. 합성 backend와
native 2.1.288에서 확인했다: `--plugin-dir`로 준 mod의 `prompt.submit` hook이 모델 요청에 블록을 덧붙였고, Clauduct의 hook도
그대로 설치됐다. mod가 `$.tool.call`로 Agent·SendMessage·Workflow·Skill을 직접 부르면 기존처럼
`NATIVE_DIRECT_DELEGATION_UNSUPPORTED`다. 내장 mod "You should know"는 native가 first-party 세션이면서 telemetry가
켜져 있을 때만 제공한다. Clauduct 세션은 둘 다 아니므로 쓸 수 없다. mod를 원격으로 끄는 플래그가 telemetry를 끈 상태에서
어떻게 동작하는지는 확인하지 않았다.

**Anthropic 서버를 쓰는 native 기능(2.1.288 기준).** Clauduct gateway는 `POST /v1/messages`, `POST /v1/messages/count_tokens`,
`GET /v1/models`만 backend로 보낸다. 다른 Anthropic 서비스는 다음과 같다.

| 기능 | 상태 |
|---|---|
| claude.ai 로그인, Remote Control, `/schedule`, claude.ai MCP connector, 알림 설정 | 쓸 수 없음. `ANTHROPIC_AUTH_TOKEN`이 있으면 native가 끈다(native changelog) |
| cloud·원격 세션(`--cloud`, `--teleport` 등), `/ultrareview`, Artifact, `/usage` 계열 | 쓸 수 없음(claude.ai 계정·서비스). 사용량은 `clauduct --usage` |
| advisor 도구, auto 모드 서버 분류기, telemetry·오류 보고 | Clauduct가 끈다(세션 환경). auto 모드는 native의 로컬 분류기를 `classifier_model`로 처리한다 |
| hosted web search | 지원. Codex 검색으로 번역한다 |
| WebFetch | 로컬 도구로 동작한다. domain safety check는 native가 직접 보낸다 |
| Claude in Chrome, Claude Desktop | Chrome은 확인하지 않음. Desktop은 `DESKTOP_UNSUPPORTED`로 거부 |

### v0.6.8 — 요청 값 검증, 빈 tool 결과, 분류기 압축, native 2.1.288 위험 판정

**2026-10-04 출하**(태그 `v0.6.8` → `00f2020`, [기록](RELEASE-v0.6.8.md)). 릴리스 바이트로 실제 backend 회귀 18개를 통과했다.

| 항목 | 동작 |
|---|---|
| 요청 값 검증(#253) | 위 2절 "Anthropic 요청 필드의 실제 처리" 표대로 thinking·metadata·`cache_control.scope`의 값을 검사한다. 잘못된 값은 이름을 밝혀 거부한다(`THINKING_FIELDS`·`THINKING_DISPLAY`·`METADATA_FIELDS`·`METADATA_VALUE`·`CACHE_VALUE`). native 2.1.288이 실제로 보내는 형태는 하나도 거부하지 않는다 |
| 빈 tool 결과(#252) | content 없는 `tool_result`가 backend 요청 전체를 HTTP 400으로 실패시키던 것을 고쳤다. 빈 결과는 `output: ""`로 보낸다 |
| 분류기 transcript 압축(#255) | native 2.1.288은 auto 모드 분류기가 transcript를 너무 길다고 보고하면 대화를 압축한다. Clauduct는 분류기 요청의 backend context 초과를 502 `CONTEXT_LENGTH_EXCEEDED`로 답해서 native가 분류기 불가로 읽었고, 도구 호출이 거부됐다. 이제 분류기 요청에 한해 400 `prompt is too long`으로 답해 native가 압축한다. 같은 분류기가 성공하기 전에 다시 넘치면 `CONTEXT_COMPACTION_INSUFFICIENT`로 답해 압축은 한 번만 일어난다 |
| Workflow 대기 만료 라벨(#259) | 자식 선택 근거를 기다리던 1초가 부하로 근거를 읽는 도중에 끝나면 gateway 경로에서는 `route_unverified`, 직접 기록하면 미분류로 남던 것을 고쳤다. 이제 `workflow_evidence_wait_expired`로 기록한다. 거부 판단은 같다 |

native 2.1.288 변경점 중 v0.6.7이 남겨 둔 위험의 판정이다.

| 변경점 | 판정 |
|---|---|
| 응답 도중 끊긴 뒤 이어 쓰기(비대화형·subagent) | `-p`와 그 subagent는 text를 완료까지 보류한다. 그래서 끊겨도 부분 응답이 전달되지 않고, 커밋 전 오류로 끝나 이어 쓰기가 생기지 않는다. 부분 text가 전달되는 TUI subagent에서는 native가 이어서 요청하고, replay 보호는 이를 새 step으로 받아 통과시킨다. 이어 쓰기 요청은 전달된 부분을 정확히 담았다(loopback 3회, 실제 backend에서 끊김이 성립한 표본 8회 모두. 첫 delta에서 끊은 1회 포함). 그중 숫자 1~40을 쓰는 과제를 다섯 번째 숫자 뒤에서 끊은 7회 중 5회는 모델이 보고에 1~40 전체를 다시 냈다. 2회는 이미 전달된 1~5까지만 냈다. **이어 쓰기 뒤 부모가 받는 보고가 끊긴 지점에서 멈출 수 있다.** 처리 방침은 #264에서 정했다: v0.6.9부터 TUI subagent의 text도 완료까지 보류해 이 경로를 없앴다(2절 "SDK·`--print`의 부분 본문" 행) |
| thinking만 있는 응답의 재시도 | Clauduct는 thinking만 있는 정상 응답을 내보내지 않는다. backend가 reasoning만 내면 `EMPTY_REPLY`로 실패한다(검증된 부모 대기 경로는 대기로 처리한다). 커밋 전에는 502와 `X-Should-Retry: false`, 커밋 뒤에는 오류 이벤트다. native는 main·subagent의 두 경로 모두에서 다시 요청하지 않았다. 실제 backend에서 reasoning만 낸 자식 응답에서도 같았다. 사용자에게는 `API Error: EMPTY_REPLY`가 보인다 |
| 첫 요청이 서버 출력 한도를 최대 1.5초 기다림 | 관측되지 않았다. `/v1/models` 뒤 첫 대화 요청까지 새 설정 796~1424ms, 기존 설정 669~960ms(4회)였다. native는 `/v1/models/{id}`를 부르지 않았다 |
| 분류기 transcript 압축 | 위 #255로 동작한다 |

### v0.6.9 — native 2.1.289 기준, Agent teams, TUI subagent 부분 보고 제거

**2026-10-04 출하**(태그 `v0.6.9` → `b780a15`, [기록](RELEASE-v0.6.9.md)). 릴리스 바이트로 실제 backend 회귀 18개를 통과했다.
측정 기준은 Claude Code 2.1.289다.

| 항목 | 동작 |
|---|---|
| Agent teams(#269) | 1절 "Agent teams" 행. v0.6.8 이하는 2.1.289의 teammate 요청을 모두 `INVALID_SESSION_ID`로 거부했다 |
| subagent text 보류(#264) | 2절 "SDK·`--print`의 부분 본문" 행. TUI에서도 subagent의 text를 완료까지 보류한다 |
| hosted web search `max_uses`(#272) | 2절 hosted web search 행. 1 이상의 정수만 받는다 |
| 알려진 제한(#273) | 3절 "알려진 제한 원장" |

## 4. 제3자 구현이라는 사실

Claude 공식 문서는 gateway를 통한 non-Claude 모델 라우팅을 **공식 지원하지 않는다고 명시**한다.
V2는 제3자 호환 구현이며 "Anthropic 공식 지원"이나 "전체 기능 100% 보장"으로 설명하지 않는다.
지원 조합을 고정해 기록하고 drift 진단을 남긴다 — 클라이언트 버전은 고정하지 않고 **보고**한다.

## 5. 기존 실행 정책과 구현 상태

아래는 사용자와 확정한 목표다. 문서 작성 자체가 기능 구현이나 검증 완료를 뜻하지 않는다.

| 확정 정책 | 현재 상태 / 다음 근거 |
|---|---|
| 작업 성공과 실패 처리·회복 합격을 구분 | 판정 기준 채택. 외부 장애를 정확히 보고하고 이력·결과를 보존하며 다음 요청이 동작해야 회복 합격. 모든 장애 조합 실측은 남음 |
| 실제 실행 중인 자식과 대기 회차가 검증된 부모의 빈 응답은 대기로 유지 | TUI는 무출력 대기, SDK는 완료를 주장하지 않는 Clauduct 상태 메시지. 실제 backend의 빈 응답과 결정적 native fixture를 모두 관측. 자식 결과 수신과 상태 표시를 분리 |
| 진행 미관측만으로 강제 종료하지 않음 | 마지막 실제 이벤트·경과 시간·도구 대기 수를 보고. 누락/미종료는 drain 완료로 간주하지 않음. 기존 명시적 deadline/grace만 적용 |
| 검증된 native 재개에서는 native 재실행 규칙을 허용 | v0.6.0 [#150](https://github.com/wotjr1649/Clauduct/issues/150)에서 명시적 source 재개 구현. 실패·취소·수정 script·동일 UUID 재시작·cache 재사용을 확인. 기존 결과 회수와 plan-v1은 유지하며 plan-v1의 시작했지만 결과 없는 단계는 `started_not_reexecuted`·전체 `complete:false`로 보고 |
| 버전 번호 대신 기능별 필수 조건으로 실행 판정 | 기존 실행 경계 검사 결과를 기능별로 집계. `/context` 출처도 관측 버전과 구조를 대조. 버전 일치와 전체 기능 합격을 구분 |
| 지원 기능을 별도 문서로 관리 | 이 문서를 현행 지원 목록으로 사용. 세션별 보고서는 변경 당시의 근거로 보존 |

## 6. 업데이트 시 이 문서를 갱신하는 기준

1. **문서와 실행 판정의 사실을 일치시킨다.** 기능명, 지원 입력, 필수 조건, 구현 여부, 실측 버전·제품 commit·binary hash, 검사/실제 TUI 근거, 제한을 기록한다. 문서의 문구가 실행 허용을 대신하지 않는다.
2. **버전 변경만으로 과거 근거를 지우거나 새 버전으로 바꾸지 않는다.** 현재 실행 조건 확인과 과거 TUI 확인을 나란히 표시한다. 영향받은 기능을 새 버전에서 검증한 뒤 근거를 추가한다.
3. **구현·지원 범위가 바뀌면 같은 변경에서 해당 행을 갱신한다.** 근거가 없는 완료 표현, 버전 일치만으로 `검증 완료` 처리, 단위 검사를 실제 TUI로 표시하는 변경은 하지 않는다.
4. **실패를 재실행 성공으로 덮지 않는다.** 최초 실패·원인·수정·검증을 연결하고, 미지원/미검증/실행 조건 실패를 구분한다. 정상 종료 코드만으로 기능 합격을 선언하지 않는다.
5. **추가 구현 전 남은 사실을 확인한다.** 진행 관측·TUI 대기 통합·독립 계획의 중복 방지 재개는 위 범위에서 구현했다. 다른 실행 모드·범용 스크립트 재개·의미상 성공 판정·모든 경계/멀티모달/성능의 검증은 남아 있다. 원본 사용자 세션이나 전역 설정을 문서 작업 때문에 변경하지 않는다.
