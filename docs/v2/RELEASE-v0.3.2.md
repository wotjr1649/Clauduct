# Clauduct v0.3.2

Windows x64용 Go V2 유지보수 릴리스 후보다. 배포 여부와 검사 결과는 해당 후보 commit의
출하 검증 기록으로 확인한다. Node V1은 비교 기준으로 유지한다.

- 기준 클라이언트를 Claude Code 2.1.280으로 올렸다. 공개 옵션 65개의 이름과 값 형태는
  2.1.278과 같고, native 이벤트 모듈이 쓰는 plugin API는 선택 필드만 추가됐다.
  진단의 `verified`는 계속 버전 문자열 일치만 뜻한다. 이 후보는 2.1.280의 실제 backend TUI에서
  생성·압축·취소·복구·종료를 확인했다.
  [재측정 기록](../../verification/v032-client-2.1.280-20260923/README.md),
  [TUI 기록](../../verification/v032-tui-20260923/README.md)
- 요청 하나가 turn 영수증을 한 번만 읽고, 업로드 취소·실행 예약·선택이 그 값을 함께 쓴다.
  읽는 사이에 새 turn이 게시되면 정상 요청이 `NATIVE_TURN_UNVERIFIED`로 거부되던 경합을 고쳤다.
- 긴 세션이 요청 16,384개에서 `NATIVE_REQUEST_CAPACITY`로 멈추던 한도를 풀었다. agent의 새 turn이
  예약되면 그 agent의 이전 turn 실행 기록을 지운다. 새 turn보다 먼저 영수증을 읽은 늦은 요청은
  지운 기록으로 다시 실행되지 않도록 `NATIVE_TURN_UNVERIFIED`로 거부한다. 남는 한도는 agent마다
  마지막 turn의 기록과 `--bare` 기록의 합이며, 끝난 자식이 매우 많은 세션의 후속 과제는
  [#70](https://github.com/wotjr1649/Clauduct/issues/70)에서 다룬다.
- 도구 없는 독립 `auxiliary` 요청(제목·분류 등)은 현재 root turn 안에서만 한 번 실행한다. 다음 turn의
  같은 요청은 새로 실행하고, 같은 turn의 재전송은 계속 막는다.
- native 이벤트 모듈이 다시 등록되면 게시 순번을 시계에서 시작한다. 새 게시가 이전 게시보다 작은
  순번을 받아 모든 요청이 거부되는 경우를 막는다. 알려진 발생 경로는 없으며 합성 재현으로 확인했다.
  [재실행 방지 묶음 기록](../../verification/v032-replay-ledger-20260923/README.md)
- 취소된 읽기, 한도를 넘은 본문, 동시 요청 한도(`TOO_MANY_REQUESTS`)의 거부는 남은 본문을 기다리지
  않고 바로 연결을 닫는다. 멈춘 업로드가 거부 응답을 최대 1초 늦추던 문제가 없어졌다.
  동시 요청 한도의 거부 뒤에는 native가 새 연결을 쓴다. 끝난 요청의 기록은 내부 참조를 놓는다.
  [정리 묶음 기록](../../verification/v032-cleanups-20260923/README.md)

출하 구성은 Go 1.27.1, `CGO_ENABLED=0`, `-trimpath`다. `clauduct.exe`,
`clauduct-hook.exe`, `clauduct-dev.exe` 세 파일과 `SHA256SUMS`를 함께 사용한다.
설치와 업데이트는 [패키징 안내](PACKAGING.md)를 따른다.

v0.3.2는 세션 기록 형식을 바꾸지 않는다. native 영수증은 프로세스마다 새로 만드는 임시
폴더에 있다. 따라서 v0.3.1로 되돌릴 때는 바이너리만 교체한다. v0.3.0으로 되돌릴 때의 제한은
[v0.3.1 안내](RELEASE-v0.3.1.md)와 같다.

Windows 전용이며 서명하지 않는다. 지원 기능과 남은 조건은 [호환성 문서](COMPATIBILITY.md)를
참조한다.
