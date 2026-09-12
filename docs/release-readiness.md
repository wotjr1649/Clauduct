# Clauduct 릴리즈 검증

2026-09-12 시작. 기준 커밋 `6c263c0`, 개발 브랜치 `release/stability-2026-09-12`.

사용자 목표는 Anthropic 서버 전용 기능을 제외한 약속된 기능의 안정적인 사용과 비대화형 실사용 검증이다. 이 문서는 출하 증거를 기록하며, 기존 감사의 과거 관측을 새 실행 결과로 바꾸지 않는다.

## 출하 조건과 진행

1. [x] 완료 알림 복귀의 지원 범위·복원 불가 원인·사용자 UI 취소 관측을 현재 상태표에 분리했다.
2. [ ] 제공 기능과 실제 구현을 전수 대조하고 재현되는 결함을 수정한다. 텍스트·도구·모델/effort·자식/Workflow·취소/재개·검색·이미지·로컬 MCP·컨텍스트·진단을 포함한다.
3. [ ] 비대화형 실행으로 정상 작업 및 실패 뒤 결과 보존·도구 미중복·종료 정리를 검증한다. 로컬 합성과 실제 backend 증거를 구분한다.
4. [ ] 릴리즈 구성의 전체 회귀, 배포 산출물 내용·경로·시작/종료 검사와 최종 판정을 기록한다.

외부 서비스 무장애나 모든 future client 버전 호환성은 입증할 수 없다. 복구 가능한 전달 전 장애는 제한된 재시도로 처리하고, 전달 후 장애·취소는 미완료 상태와 진단을 보존해야 한다. 실패한 응답의 도구를 실행하거나 이미 끝난 작업을 자동 중복 실행하면 불합격이다.

## 기준선

Verified: 사용자 작업 루트에서 다음 명령을 실행했다. 파일 20/20, 실패 0, 종료 코드 0, 16.74초. 검사 전후 tracked·staged 변경 없음과 기존 untracked 목록 동일을 확인했다.

```powershell
pwsh -NoProfile -NonInteractive -File src/run-node-tests.ps1 -Root D:/AIDEV/Clauduct -TestFiles 'src/test-*.mjs'
```

Not verified: 파일 단위 성공 안에 `review-diff`(PATH 부재/ENOENT), native Workflow 실행, completion·Workflow symlink 검사의 `notRun`이 있다. 이들은 실검사 성공으로 세지 않는다.

작업 트리는 `D:/AIDEV/Clauduct/.tmp/release-2026-09-12`다. 기존 사용자 파일을 포함하지 않고 변경 파일을 개별 이름으로 stage한다. 원래 작업 루트로 통합하기 전 최종 변경과 검증 결과를 확인한다.

## 출하 판정

진행 중. 릴리즈 통과나 실제 backend 무오류를 아직 선언하지 않는다.

## 비대화형 실행 보완

기존에는 native에 `-p`를 전달해도 wrapper가 TTY를 요구했다. 명시적 `-p`/`--print`를 식별해 파이프 실행을 허용하고, wrapper 출력은 stderr로 보내 native JSON/stream-json stdout을 보존한다. 기본 대화형 진입점과 과거 PoC의 SEND 절차는 유지한다. 디버그·임의 Node 실행 인수·proxy/TLS 우회·다른 Codex 홈·계정 변경 검사를 제거하지 않았다.

Verified: 새 `test-headless`가 수정 전 print 모드 미인식 assertion으로 실패했다. 수정 후 19개 검사 통과: 두 flag, 값/구분자 혼동 방지, 명시적 비대화형 진입, runtime·인증 경계의 기존 거부, dry-run, 실제 합성 Node 자식의 성공/실패 JSON과 stderr 진단·9개 정리 플래그다. `src/test-*.mjs` 전체 21/21, 종료 코드 0(17.99초). 공유 진입점 회귀 `poc/test-user-session.mjs` 89/89, loopback 72회, 종료 코드 0이다.

Not verified: 이 결과는 실제 Claude 또는 backend 실행 결과가 아니다. 기준선의 세 종류 `notRun`은 그대로 남아 있다.

공식 근거: [Claude 비대화형 실행](https://code.claude.com/docs/en/headless)은 `-p`, JSON/stream-json, 오류 시 비정상 종료 코드를 정의한다. [OpenAI 스트리밍](https://developers.openai.com/api/docs/guides/streaming-responses)은 text delta·정상 완료·error를 별도 이벤트로 정의한다. 공개 Responses 문서가 비공개 Codex backend의 모든 동작을 보증하지는 않는다.
