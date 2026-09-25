# v0.4.3 — 개발 초안

미출하 개발본이다. 최신 배포·설치 버전은 v0.4.2이며, 이 문서는 Release 발행이나 설치본 변경을 뜻하지 않는다.

- #130: 실제 native에서 `peer`로 시작하는 TUI·SDK 세션의 부모 대기 모드를 인식한다. 새 입력과 자식 결과 알림을 구분하고 기존 오류 경계를 유지한다.
- #133: `clauduct --bg`로 생성한 세션마다 연결 유지 프로세스를 두어 native stop·재기동 사이에 gateway·필수 hook과 모델 경로를 유지한다. 인증은 동일 Windows 로그인에 제한된 IPC로 복원한다.
- #134: 검증 실행의 모델·effort와 backend 시도 상한을 여러 프로세스·재기동에 걸쳐 전송 전에 강제한다. 일반 사용자 세션의 Unlimited는 유지한다.

Claude Code 2.1.282에서 실제 native와 로컬 backend로 메시지 대기, `/clear`·`/reload-plugins`, background
stop→attach·respawn→메시지·삭제를 검사했다. 순수 제품 바이너리의 생성 실행기 종료 뒤 연결 유지와 명시적
종료도 과금 없이 확인했다. 최종 일반·race 각 21개 패키지, 기본·policy_evidence·runtime_evidence vet, build,
gofmt를 통과했다. 과거 비공개 테스트 4개의 형식 차이도 기능 변경 없이 정리했다.

실제 backend 검증과 native 자동 유휴 종료 후 재기동은 아직 완료하지 않았다. Codex CLI 0.157.0의 무료
재측정에서는 기존 0.156.1과 CLI/API 선언이 같았으며, 직접 캡처한 exec 요청의 구조도 유지됐다.
미측정 Codex TUI·검색·계수까지 새 호환성을 선언하지 않는다. 상세 지원 범위는
[COMPATIBILITY.md](COMPATIBILITY.md#v043--메시지-background-검증-예산)를 따른다.
