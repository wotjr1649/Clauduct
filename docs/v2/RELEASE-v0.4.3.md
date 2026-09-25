# v0.4.3 — 개발 초안

개발 검증을 마친 미출하 후보다. 최신 배포·설치 버전은 v0.4.2이며, 이 문서는 Release 발행이나 설치본 변경을 뜻하지 않는다.

- #130: 실제 native에서 `peer`로 시작하는 TUI·SDK 세션의 부모 대기 모드를 인식한다. 새 입력과 자식 결과 알림을 구분하고 기존 오류 경계를 유지한다.
- #133: `clauduct --bg`로 생성한 세션마다 연결 유지 프로세스를 두어 native stop·재기동 사이에 gateway·필수 hook과 모델 경로를 유지한다. 인증은 동일 Windows 로그인에 제한된 IPC로 복원한다.
- #134: 검증 실행의 모델·effort와 backend 시도 상한을 여러 프로세스·재기동에 걸쳐 전송 전에 강제한다. 일반 사용자 세션의 Unlimited는 유지한다.

Claude Code 2.1.282에서 실제 native와 로컬 backend로 메시지 대기, `/clear`·`/reload-plugins`, background
stop→attach·respawn→메시지·삭제를 검사했다. 순수 제품 바이너리의 생성 실행기 종료 뒤 연결 유지와 명시적
종료도 과금 없이 확인했다. 최종 일반·race 각 21개 패키지, 기본·policy_evidence·runtime_evidence vet, build,
gofmt를 통과했다. 과거 비공개 테스트 4개의 형식 차이도 기능 변경 없이 정리했다.

순수 제품 `3b1d3c6`은 실제 backend 메시지 대기를 TUI·SDK 각각 5회, background 생성·stop/attach·
respawn/메시지·명시적 연결 종료를 6회로 통과했다. 빈 응답 제어, 자식 결과 1회 수신, 후속 명시 입력,
정상 종료를 확인했다. 최종 제품 `4b160ad`의 기존 SDK 입력·`/clear`·자식 위임도 5회로 통과했다.
성공 실행 모두 cleanup 오류 0, active 요청·메모리 예약·실행·대기 수 0이었다.

실제 시도는 실패·불완전한 실행을 포함해 **46회**다. 실행마다 별도 6·8·10회 상한을 전송 전에
강제했고 초과하지 않았다. 초기 SDK 실행의 모델 추가 호출, 빈 응답 미발생, background 상태 분류의
미허용 `max` 요청 거부를 보존했다. 검증 입력을 비동기 실행·대기·완료 상태로 명시한 뒤 통과했으며,
모델의 도구 호출 횟수나 의미상 성공을 제품이 보장한다는 뜻은 아니다.

예상치 못한 worker 종료 뒤 명시적 native `respawn`과 attach는 무료로 확인했다. 종료 직후 attach만
사용한 자동 재시작은 실패했으므로 그 경로의 복구까지 보장하지 않는다. 연결 유지 프로세스 종료 뒤
IPC·키 조회는 실패하며 자동 복원하지 않는다.

자동 유휴 경로는 과금 없이 관찰했다. native daemon은 완료 뒤 **3,658초(약 61분)**에 idle worker를
회수했다. 원래 worker의 OS 종료를 확인한 뒤 같은 메모리 IPC 연결로 attach·새 응답과 Luna/low 유지를
검증했다. native 목록이 종료된 PID를 계속 표시해 최초 장시간 하네스는 65분 한도에서 실패했다.
원인 확인 후 연속 검증은 통과했고, 하네스 판정은 원래 프로세스 handle을 확인하도록 고쳤다.
수정한 1시간 하네스 전체를 다시 실행한 것은 아니며, daemon 기록·OS 종료·실제 재연결의 근거를 보존한다.

Codex CLI 측정 기준은 **0.157.0**으로 옮겼다. 무료 재측정에서 CLI/API 선언 네 개와 exec 요청의 주요
구조가 유지됐고, turn metadata의 `workspaces` 추가를 확인했다. 실제 backend 검사도 같은 설치 버전에서
수행했다. 제품의 HTTP SSE 요청 형식은 유지하며 새 Codex 형식 채택은 #135의 범위다.
미측정 Codex TUI·검색·계수까지 합격으로 선언하지 않는다. 상세 지원 범위는
[COMPATIBILITY.md](COMPATIBILITY.md#v043--메시지-background-검증-예산)를 따른다.
