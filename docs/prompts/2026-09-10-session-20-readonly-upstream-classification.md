# Session-20: 메인 읽기 전용 오류 분류 시험 1회

## 목적과 고정 조건

이 작업은 개발 계획 실행이나 SDD가 아니라, 메인의 읽기 전용 도구 호출과 그 결과를 받은 후속 모델 응답을 관찰하는 단일 시험이다. session-18/19의 개발·검토 절차를 재개하지 않는다. 새 Clauduct 프로세스·새 세션에서 한 번만 수행한다.

Clauduct 기준 커밋은 832b665175e00843864a56d41ad402a24d7df135다. 현재 연결은 Claude Code → 로컬 gateway → ChatGPT Codex backend 직접 HTTPS이며 app-server가 아니다. 최신 진단은 upstreamErrorCode, upstreamErrorType, upstreamIncompleteReason을 고정 허용 목록으로 보존한다. 과거 실제 실패의 세부 원인은 미확인이다. 이번 시험도 무오류나 근본 원인 규명을 보장하지 않는다.

실행 폴더는 D:/AIDEV/Clauduct/verification/dev-sandbox/run-02다. 이 저장소의 기존 미커밋 GREEN 구현을 보존한다. 예상 상태는 수정된 README.md, cli.mjs, tasks.mjs, test/boundaries.test.mjs와 untracked test/untag.test.mjs다. 차이는 보고만 하고 정리하지 않는다.

메인 시작 기본값은 GPT-6 Astra/low, window 400K, auto compact 목표 320K다. 이 문서는 모델을 바꾸는 명령이 아니다. 실제 모델·effort는 진단이 제공할 때만 확정한다. 앱서버·SDK·다른 인증 경로로 전환하지 않는다.

## 실행: 아래 두 단계만 순차 수행

이 문서는 이미 읽었으므로 다시 읽지 않는다. 아직 읽지 않은 경우 먼저 문서 전체를 Read로 읽는다.

1. Read로 D:/AIDEV/Clauduct/src/request-status.mjs를 전체 한 번 읽는다. 현재 세션의 제한된 loopback 진단 스크립트임을 확인한다. imported 파일까지 탐색하거나 스크립트를 수정·실행하지 않는다.
2. native Bash에서 다음 명령을 한 번 실행한다. timeout은 10000ms로 설정한다.

```bash
git -C D:/AIDEV/Clauduct/verification/dev-sandbox/run-02 status --short
```

결과를 받은 뒤 추가 도구 호출 없이 짧게 최종 보고하고 종료한다. 두 단계가 성공했으면 `READONLY-ROUNDTRIP-COMPLETED`와 관측된 Git 상태, `과거 API 오류 원인은 미확인`을 적는다. 기준과 다른 파일 상태가 보이면 그 차이를 보고하며 개발을 시작하지 않는다. 이 문서 Read를 포함해 계획된 작업 도구 호출은 총 3회다. 이는 모델 행동의 범위 제한이지 gateway의 요청 수 hard limit이 아니다.

선택적 Skill, Workflow, Agent/Task 생성, 테스트 실행, diff/소스 추가 탐색, 대기·polling, 파일 작성, Git stage/commit은 수행하지 않는다. 이 문서 이름이나 continue 요청을 일반 개발 실행 지시로 확대하지 않는다. 상위의 필수 지침·자동 hook·실제 권한 검사는 그대로 따른다. 상위 지침 때문에 이 최소 범위를 지킬 수 없으면 그 충돌을 보고하고 SAFE-STOP한다.

## 실패와 종료

첫 API 오류·도구 오류·timeout·guard 거부·사용자 취소에서 남은 단계를 실행하지 않는다. 재시도·continue·resume·모델 변경·재로그인·설정 변경·다른 shell/interpreter 우회를 하지 않는다. 원문 세션 로그, 인증 파일, 전체 환경, upstream 응답 본문은 탐색하지 않는다. 기존 transport의 retry/timeout 정책도 변경하지 않는다.

모델이 오류 때문에 최종 답변을 생성하지 못하는 것 자체가 관측 결과다. 진단을 얻기 위해 모델에 다시 요청하지 않는다. 메인은 성공/실패 어느 경우에도 request-status.mjs를 자동 실행하지 않는다. 진단 조회는 아래 사용자 단계 한 번으로 분리한다.

## 사용자 단계: 동일 프로세스에서 진단 1회

정상 최종 응답 또는 일반 API 오류가 나온 후 프로세스를 열어 둔 채, 사용자가 동일 Clauduct 입력창의 Bash 모드에서 다음을 한 번 실행한다. 별도 터미널에서는 현재 세션 환경이 없으므로 실행하지 않는다.

```text
! node D:/AIDEV/Clauduct/src/request-status.mjs
```

guard 거부나 사용자 취소가 발생했다면 이 진단도 실행하지 않고 해당 제한만 보고한다. 조회가 실패하면 반복하거나 원본 로그/인증 정보로 우회하지 않는다. 이미 프로세스가 종료됐으면 진단을 얻기 위해 새 시험을 자동 시작하지 않는다.

사용자는 세션 ID, 진단 JSON, 표시된 오류 문구(있을 때)를 별도 검증 세션에 전달한다. code/type/reason과 failureStage, attempts의 HTTP status/terminalState, retryScheduledMs, model/effort를 대조한다. 출력에 없는 값을 추측하지 않는다.

판정은 다음과 같이 분리한다. 구체적인 허용된 오류 분류가 있으면 원인 가설을 좁힐 근거이며 근본 원인 확정은 아니다. OTHER/null이면 관측기 보존 정보 부족 가능성을 포함한 미확인 종료다. 오류가 없으면 이번 짧은 왕복 성공과 과거 오류 원인 미확인을 함께 기록한다. 어떤 경우에도 이번 시험을 반복하거나 전체 SDD·무인 개발·제품 안정화 완료로 확대하지 않는다.
