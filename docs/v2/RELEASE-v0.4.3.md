# v0.4.3 — 메시지 대기·background 연결·검증 예산

2026-09-25 출하했다. 태그 `v0.4.3`은 `8ed1de4c98b9ff8fd70cb1c85270292d1b53427a`이며,
[GitHub Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.4.3)는 latest·정식 릴리스다.

- #130: 실제 native에서 `peer`로 시작하는 TUI·SDK 세션의 부모 대기 모드를 인식한다. 새 입력과 자식 결과 알림을 구분하고 기존 오류 경계를 유지한다.
- #133: `clauduct --bg`로 생성한 세션마다 연결 유지 프로세스를 두어 native stop·재기동 사이에 gateway·필수 hook과 모델 경로를 유지한다. 인증은 동일 Windows 로그인에 제한된 IPC로 복원한다.
- #134: 검증 실행의 모델·effort와 backend 시도 상한을 여러 프로세스·재기동에 걸쳐 전송 전에 강제한다. 일반 사용자 세션의 Unlimited는 유지한다.

출하 전 재리뷰에서 background 판정을 보완했다. native 2.1.282는 `--bg`·`--background` 토큰이
도움말·버전 옵션과 함께 있거나 `--` 뒤에 있어도 작업을 생성한다. 이때 연결을 조기에 닫던 경로를 고쳤고,
실제 native의 생성·attach·새 응답·respawn·메시지 완료로 검증했다. 도움말·버전만 조회하려면 background
인자 없이 실행한다.

Claude Code 2.1.282에서 실제 native와 로컬 backend로 메시지 대기, `/clear`·`/reload-plugins`, background
stop→attach·respawn→메시지·삭제를 검사했다. 순수 제품 바이너리의 생성 실행기 종료 뒤 연결 유지와 명시적
종료도 과금 없이 확인했다. 최종 일반·race 각 21개 패키지, 기본·policy_evidence·runtime_evidence vet, build,
gofmt를 통과했다. 과거 비공개 테스트 4개의 형식 차이도 기능 변경 없이 정리했다.

순수 출하 바이너리 `v0.4.3`/`8ed1de4`의 실제 backend 검증은 네 실행 모두 통과했다.

| 실행 | backend 시도 / 사전 강제 상한 | 확인 |
|---|---|---|
| SDK 입력·`/clear`·자식 위임 | 5 / 6 | Luna/max 부모·Luna/low 자식, 결과 수신·종료 |
| 메시지 기반 SDK 대기 | 5 / 8 | Sol/low 부모·Luna/max 자식, peer·빈 응답 제어·결과 1회·후속 입력 |
| 메시지 기반 TUI 대기 | 5 / 8 | 같은 경로를 실제 ConPTY에서 확인 |
| background 생성·관리 | 6 / 10 | Luna/low 본문·Luna/max 상태 분류, stop/attach·respawn/peer·연결 종료 |

각 실행의 종료 코드·cleanup 오류는 0이고 active 요청·메모리 예약·실행·대기 수도 0이다.
반환 모델·effort와 영속 예약 수를 함께 대조했다. 개발 overlay를 넣지 않은 격리 설치본을 사용했다.

출하 검증은 **21회**, 개발 중 실패·불완전한 실행을 포함한 누계는 **67회**다. 실행마다 별도 6·8·10회 상한을 전송 전에
강제했고 초과하지 않았다. 초기 SDK 실행의 모델 추가 호출, 빈 응답 미발생, background 상태 분류의
미허용 `max` 요청 거부를 보존했다. 검증 입력을 비동기 실행·대기·완료 상태로 명시한 뒤 통과했으며,
모델의 도구 호출 횟수나 의미상 성공을 제품이 보장한다는 뜻은 아니다.

예상치 못한 worker 종료 뒤 명시적 native `respawn`과 attach는 무료로 확인했다. 종료 직후 attach만
사용한 자동 재시작은 실패했으므로 그 경로의 복구까지 보장하지 않는다. 연결 유지 프로세스 종료 뒤
IPC·키 조회는 실패하며 자동 복원하지 않는다.

자동 유휴 경로는 수정한 하네스를 처음부터 끝까지 다시 실행해 통과했다. **3,659.6초(약 61분)** 뒤
원 worker의 Windows handle로 OS 종료를 확인하고 native daemon의 idle 회수 기록과 대조했다.
같은 연결의 attach·Luna/low 새 응답·respawn/peer 완료·삭제·정리까지 검사했으며 외부 요청은 0회다.
최초 하네스는 native 목록에 종료된 PID가 남아 65분 한도에서 실패했다. 이 실패는 보존하고,
현재 판정은 프로세스 handle을 보존하도록 고친 전체 재실행을 따른다.
순수 출하 바이너리의 resident도 생성 실행기 종료 후 70초 이상 연결을 유지하고 명시적으로 종료됐다.

출하 빌드는 Go 1.27.1 windows/amd64·CGO_ENABLED=0·trimpath다. 깨끗한 태그를 별도 빌드 캐시로도
빌드해 동일 바이트를 확인했다. 제품·구형 사본 둘의 SHA-256은 다음과 같다.

```text
44d9cb840773a0c81c198c159364c3caef78dc665c533130926db53116a30576
```

격리 새 설치·v0.4.2에서 교체·되돌리기, 공개 태그 고정 설치, v0.4.2의 실제 updater,
다음 실행의 `.old` 정리와 두 번째 업데이트의 no-op을 통과했고 사용자 PATH는 그대로였다.
공개 자산 여섯 개를 다시 받아 후보 바이트·SHA256SUMS·GitHub API digest를 모두 대조했다.
v0.4.x 규약에 따라 같은 바이트의 구형 이름 사본 둘을 함께 발행했다.

Codex CLI 측정 기준은 **0.157.0**으로 옮겼다. 무료 재측정에서 CLI/API 선언 네 개와 exec 요청의 주요
구조가 유지됐고, turn metadata의 `workspaces` 추가를 확인했다. 실제 backend 검사도 같은 설치 버전에서
수행했다. 제품의 HTTP SSE 요청 형식은 유지하며 새 Codex 형식 채택은 #135의 범위다.
미측정 Codex TUI·검색·계수까지 합격으로 선언하지 않는다. 상세 지원 범위는
[COMPATIBILITY.md](COMPATIBILITY.md#v043--메시지-background-검증-예산)를 따른다.
